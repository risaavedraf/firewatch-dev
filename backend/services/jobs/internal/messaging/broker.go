package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	v1 "firewatch.local/contracts/events/v1"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
)

// deadLetterQueue is this consumer's dead-letter queue: the same name topology
// declares as the consumed queue plus ".dead".
const deadLetterQueue = v1.JobsQueue + ".dead"

// DeadLetterDepthUnknown represents missing, failed, or expired observations. -1 is deliberately outside the range of a real depth
// (>= 0) so an unreachable broker can never be reported as an empty, healthy
// looking dead-letter queue. ReadyReason treats it as not-ready and /metrics
// reports it verbatim.
const DeadLetterDepthUnknown = int64(-1)

// Sampling is independent of delivery processing. Freshness is bounded at read
// time; this polling interval does not promise instantaneous consistency.
const deadLetterRefreshInterval = 3 * time.Second

type Broker struct {
	DB  *pgxpool.Pool
	URL string
	// Ready is the connection signal; HTTP uses the coherent Snapshot instead.
	Ready         atomic.Bool
	Published     atomic.Uint64
	Processed     atomic.Uint64
	Rejected      atomic.Uint64
	depthSnapshot atomic.Pointer[DeadLetterSnapshot]
	Handle        func(context.Context, v1.Event) error
}

// ReadyReason is the single readiness decision for this service, factored out
// so it can be tested without a database or a broker. It returns false with a
// machine-readable reason whenever the service cannot honestly serve traffic:
// the database is unreachable, the broker is not connected, the dead-letter
// queue depth could not be read (UNKNOWN), or that depth exceeds maxDepth (any
// dead letter above the threshold means work is being discarded). maxDepth is a
// non-negative integer; the environment parser rejects anything else at startup.
func ReadyReason(dbOK, brokerReady bool, deadLetterDepth, maxDepth int64) (bool, string) {
	switch {
	case !dbOK:
		return false, "database_unavailable"
	case !brokerReady:
		return false, "broker_unavailable"
	case deadLetterDepth == DeadLetterDepthUnknown:
		return false, "dead_letter_depth_unknown"
	case deadLetterDepth > maxDepth:
		return false, "dead_letter_backlog"
	default:
		return true, ""
	}
}

// MetricsText renders the Prometheus text body used by /metrics. It is a pure
// function so the exact wire format, including the UNKNOWN dead-letter depth, is
// pinned by a test. The pre-existing metric names and their order are unchanged.
func MetricsText(requests uint64, brokerReady bool, published, processed, rejected uint64, deadLetterDepth int64, ageSeconds ...float64) string {
	ready := 0
	if brokerReady {
		ready = 1
	}
	age := float64(-1)
	if len(ageSeconds) > 0 {
		age = ageSeconds[0]
	}
	return fmt.Sprintf("firewatch_http_requests_total %d\nfirewatch_broker_ready %d\nfirewatch_events_published_total %d\nfirewatch_events_processed_total %d\nfirewatch_events_rejected_total %d\nfirewatch_dead_letter_depth %d\nfirewatch_dead_letter_depth_age_seconds %g\n", requests, ready, published, processed, rejected, deadLetterDepth, age)
}

func (b *Broker) Run(ctx context.Context) {
	b.invalidate()
	delay := time.Second
	for ctx.Err() == nil {
		started := time.Now()
		_ = b.session(ctx)
		// The depth read on the dead channel is no longer trustworthy: report
		// UNKNOWN instead of leaving a stale count in the metric.
		b.invalidate()
		if ctx.Err() != nil {
			return
		}
		slog.Warn("broker_reconnecting", "retry_seconds", delay.Seconds())
		if !pause(ctx, delay) {
			return
		}
		if time.Since(started) > time.Minute {
			delay = time.Second
		} else {
			delay = min(delay*2, 30*time.Second)
		}
	}
}

func (b *Broker) session(ctx context.Context) error {
	conn, err := amqp.DialConfig(b.URL, amqp.Config{Heartbeat: 10 * time.Second, Dial: amqp.DefaultDial(5 * time.Second)})
	if err != nil {
		return err
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	if err = topology(ch); err != nil {
		return err
	}
	if err = ch.Qos(1, 0, false); err != nil {
		return err
	}
	deliveries, err := ch.Consume(v1.JobsQueue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	monitor, err := conn.Channel()
	if err != nil {
		return err
	}
	defer monitor.Close()
	connClosed := conn.NotifyClose(make(chan *amqp.Error, 1))
	consumerClosed := ch.NotifyClose(make(chan *amqp.Error, 1))
	monitorClosed := monitor.NotifyClose(make(chan *amqp.Error, 1))
	failures := make(chan error, 1)
	watchStop, watchDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(watchDone)
		select {
		case <-watchStop:
			return
		case <-ctx.Done():
			return
		case <-connClosed:
		case <-consumerClosed:
		case <-monitorClosed:
		}
		failures <- errors.New("AMQP connection or channel closed")
	}()
	defer func() { close(watchStop); <-watchDone }()
	ticker := time.NewTicker(deadLetterRefreshInterval)
	defer ticker.Stop()
	return b.runSession(ctx, func() (int64, error) {
		q, err := monitor.QueueDeclarePassive(deadLetterQueue, true, false, false, false, nil)
		return int64(q.Messages), err
	}, ticker.C, failures, func() { _ = conn.CloseDeadline(time.Now()) },
		func(c context.Context) error { return b.consume(c, deliveries) },
		func(c context.Context) error { return b.relay(c, conn) })
}

func (b *Broker) consume(ctx context.Context, deliveries <-chan amqp.Delivery) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case delivery, ok := <-deliveries:
			if !ok {
				return errors.New("consumer channel closed")
			}
			var event v1.Event
			if len(delivery.Body) > 16384 || json.Unmarshal(delivery.Body, &event) != nil || !valid(event) {
				b.Rejected.Add(1)
				if err := delivery.Reject(false); err != nil {
					return err
				}
				continue
			}
			attempts, workErr := handleWithRetries(ctx, b.Handle, pause, event)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if workErr != nil {
				b.Rejected.Add(1)
				slog.Error("event_dead_lettered", "event_id", event.ID, "job_id", event.JobID, "trace_id", event.TraceID, "attempts", attempts, "permanent", isPermanent(workErr))
				if err := delivery.Reject(false); err != nil {
					return err
				}
			} else {
				b.Processed.Add(1)
				slog.Info("event_processed", "event_id", event.ID, "job_id", event.JobID, "trace_id", event.TraceID)
				if err := delivery.Ack(false); err != nil {
					return err
				}
			}
		}
	}
}

// valid reports whether a batch-published event is well formed. It checks shape
// only: the type, a non-empty actor of at most 255 bytes, a non-empty zone, both
// instants present, four non-nil UUIDs, and optionally paired synthetic metadata
// (scenario_id with its nonzero evaluated_at, or neither).
//
// demo describes the origin of the request, not the shape of the event, so it is
// not a validity gate here. The batch-zone versus job-zone relationship is
// enforced against the stored job row in Store.Complete and is deliberately NOT
// duplicated here as a literal: a literal would force a migration to extend the
// accepted zones, while the database already answers the question.
func valid(e v1.Event) bool {
	if e.Type != v1.BatchPublishedType || e.ActorID == "" || len(e.ActorID) > 255 || e.ZoneID == "" || e.RequestedAt.IsZero() || e.OccurredAt.IsZero() {
		return false
	}
	// Synthetic metadata is optional but paired: a scenario-bearing message must
	// carry the nonzero instant the job was evaluated with, and an instant without
	// a scenario is equally malformed. The scenario VOCABULARY is deliberately not
	// duplicated here, so adding a profile never forces a coordinated consumer
	// release; only the shape the contract pins is enforced.
	if (e.ScenarioID == "") != (e.EvaluatedAt == nil) {
		return false
	}
	if e.EvaluatedAt != nil && e.EvaluatedAt.IsZero() {
		return false
	}
	for _, value := range []string{e.ID, e.JobID, e.TraceID, e.BatchID} {
		id, err := uuid.Parse(value)
		if err != nil || id == uuid.Nil {
			return false
		}
	}
	return true
}

// permanentError is implemented by errors that can never succeed on a retry. The
// classification is explicit and typed on purpose: an unrecognized error is
// treated as TRANSIENT, because discarding work that a later attempt could
// complete is worse than a slow dead-letter. When in doubt, keep the work.
type permanentError interface{ Permanent() bool }

func isPermanent(err error) bool {
	var typed permanentError
	return errors.As(err, &typed) && typed.Permanent()
}

// retryBackoff is the bounded schedule of one delivery: five attempts separated
// by 1, 2, 4 and 8 seconds.
var retryBackoff = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}

// handleWithRetries attempts handle until it succeeds, until the error declares
// itself permanent, or until the schedule is exhausted. A permanent error skips
// EVERY retry: it can never succeed, so retrying only delays the dead-letter and,
// with prefetch 1, blocks every message queued behind it.
func handleWithRetries(ctx context.Context, handle func(context.Context, v1.Event) error, sleep func(context.Context, time.Duration) bool, event v1.Event) (attempts int, err error) {
	for attempt := 0; ; attempt++ {
		attempts = attempt + 1
		workCtx, stop := context.WithTimeout(ctx, 5*time.Second)
		err = handle(workCtx, event)
		stop()
		if err == nil || isPermanent(err) {
			return attempts, err
		}
		if attempt >= len(retryBackoff) {
			return attempts, err
		}
		if !sleep(ctx, retryBackoff[attempt]) {
			return attempts, ctx.Err()
		}
	}
}

// DeadLetterSnapshot is published as one immutable value. LastSuccess survives
// invalidation so the age metric never mistakes a failed query for freshness.
// Ready travels with depth to keep HTTP decisions coherent across teardown.
type DeadLetterSnapshot struct {
	Depth       int64
	LastSuccess time.Time
	Ready       bool
	AgeSeconds  float64
}

func (b *Broker) Snapshot(now time.Time, maxAge time.Duration) DeadLetterSnapshot {
	s := DeadLetterSnapshot{Depth: DeadLetterDepthUnknown, AgeSeconds: -1}
	if stored := b.depthSnapshot.Load(); stored != nil {
		s = *stored
	}
	s.AgeSeconds = -1
	if s.LastSuccess.IsZero() || now.Before(s.LastSuccess) {
		s.Depth = DeadLetterDepthUnknown
		return s
	}
	age := now.Sub(s.LastSuccess)
	s.AgeSeconds = age.Seconds()
	if maxAge <= 0 || age > maxAge {
		s.Depth = DeadLetterDepthUnknown
	}
	return s
}

func (b *Broker) invalidate() {
	s := DeadLetterSnapshot{Depth: DeadLetterDepthUnknown}
	if old := b.depthSnapshot.Load(); old != nil {
		s.LastSuccess = old.LastSuccess
	}
	b.depthSnapshot.Store(&s)
	b.Ready.Store(false)
}

// runSession supervises failures independently of both consumer work and the
// contextless passive RPC. Closing the connection interrupts that RPC. A gate
// fences late query responses before invalidation; all workers are joined before
// returning, so noncooperative handlers delay reconnection, never health loss.
func (b *Broker) runSession(parent context.Context, query func() (int64, error), ticks <-chan time.Time, failures <-chan error, closeConnection func(), consume, relay func(context.Context) error) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var gate sync.Mutex
	stopped := false
	shutdown := func() {
		gate.Lock()
		if stopped {
			gate.Unlock()
			return
		}
		stopped = true
		b.invalidate()
		cancel()
		gate.Unlock()
		closeConnection()
	}
	errorsCh := make(chan error, 2)
	monitorDone, relayDone, supervisorDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	initial := make(chan struct{})
	go func() {
		defer close(supervisorDone)
		select {
		case <-ctx.Done():
		case <-failures:
		case <-errorsCh:
		}
		shutdown()
	}()
	go func() {
		defer close(monitorDone)
		first := true
		for {
			depth, err := query()
			gate.Lock()
			if stopped || ctx.Err() != nil {
				gate.Unlock()
				return
			}
			if err != nil {
				gate.Unlock()
				shutdown()
				return
			}
			b.depthSnapshot.Store(&DeadLetterSnapshot{Depth: depth, LastSuccess: time.Now(), Ready: true})
			b.Ready.Store(true)
			gate.Unlock()
			if first {
				close(initial)
				first = false
			}
			select {
			case <-ctx.Done():
				return
			case <-ticks:
			}
		}
	}()
	go func() { defer close(relayDone); errorsCh <- relay(ctx) }()
	defer func() { shutdown(); <-supervisorDone; <-monitorDone; <-relayDone }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-initial:
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return consume(ctx)
}

func topology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(v1.Exchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.ExchangeDeclare("firewatch.dead", "direct", true, false, false, false, nil); err != nil {
		return err
	}
	for queue, key := range map[string]string{v1.RiskQueue: v1.JobRequestedType, v1.JobsQueue: v1.BatchPublishedType} {
		if _, err := ch.QueueDeclare(queue+".dead", true, false, false, false, amqp.Table{"x-queue-type": "quorum"}); err != nil {
			return err
		}
		if err := ch.QueueBind(queue+".dead", queue, "firewatch.dead", false, nil); err != nil {
			return err
		}
		if _, err := ch.QueueDeclare(queue, true, false, false, false, amqp.Table{"x-queue-type": "quorum", "x-delivery-limit": int32(5), "x-dead-letter-exchange": "firewatch.dead", "x-dead-letter-routing-key": queue, "x-overflow": "reject-publish", "x-dead-letter-strategy": "at-least-once"}); err != nil {
			return err
		}
		if err := ch.QueueBind(queue, key, v1.Exchange, false, nil); err != nil {
			return err
		}
	}
	return nil
}

func (b *Broker) relay(ctx context.Context, conn *amqp.Connection) error {
	for ctx.Err() == nil {
		workCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		found, err := b.publishOne(workCtx, conn)
		cancel()
		if err != nil {
			return err
		}
		if !found && !pause(ctx, 250*time.Millisecond) {
			return ctx.Err()
		}
	}
	return ctx.Err()
}

func (b *Broker) publishOne(ctx context.Context, conn *amqp.Connection) (bool, error) {
	tx, err := b.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var id, key string
	var body []byte
	err = tx.QueryRow(ctx, `SELECT id::text,routing_key,payload FROM outbox WHERE published_at IS NULL ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&id, &key, &body)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	ch, err := conn.Channel()
	if err != nil {
		return false, err
	}
	defer ch.Close()
	if err = ch.Confirm(false); err != nil {
		return false, err
	}
	returns := ch.NotifyReturn(make(chan amqp.Return, 1))
	confirm, err := ch.PublishWithDeferredConfirmWithContext(ctx, v1.Exchange, key, true, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, MessageId: id, Type: key, Timestamp: time.Now().UTC(), Body: body})
	if err != nil {
		return false, err
	}
	ack, err := confirm.WaitContext(ctx)
	if err != nil {
		return false, err
	}
	if !ack {
		return false, errors.New("publish rejected")
	}
	select {
	case <-returns:
		return false, errors.New("message unroutable")
	default:
	}
	if _, err = tx.Exec(ctx, `UPDATE outbox SET published_at=now() WHERE id=$1`, id); err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	b.Published.Add(1)
	return true, nil
}

func pause(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
