// This file pins the retry contract of the risk consumer: which errors are
// retried, which are dead-lettered after a single attempt, and the exact
// backoff between attempts. handleWithRetries is factored out of the session
// loop for exactly this reason: the schedule and the classification can be
// tested without a broker connection.
//
// The mechanism is a deliberate copy of the jobs consumer's, so it is tested
// here against the risk consumer's own errors as well.
package messaging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	v1 "firewatch.local/contracts/events/v1"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

// testPermanentError is a typed error that declares itself permanent, the way
// the publisher's and the repository's errors do.
type testPermanentError struct{}

func (testPermanentError) Error() string   { return "permanent test error" }
func (testPermanentError) Permanent() bool { return true }

// TestIsPermanentClassifiesOnlyTypedErrors protects the conservative rule: only
// an error that explicitly declares permanence is permanent. Everything else,
// including a plain error, must be treated as transient and retried.
func TestIsPermanentClassifiesOnlyTypedErrors(t *testing.T) {
	if isPermanent(errors.New("plain")) {
		t.Fatal("a plain error must be classified as transient")
	}
	if !isPermanent(testPermanentError{}) {
		t.Fatal("a typed error declaring permanence must be permanent")
	}
	if !isPermanent(fmt.Errorf("wrapped: %w", testPermanentError{})) {
		t.Fatal("a wrapped permanent error must stay permanent")
	}
}

// TestPermanentErrorSkipsEveryRetry is the live-demo protection: an unknown zone
// is refused by the foreign key, and retrying it only blocks the queue behind it
// with prefetch 1. A permanent error must be attempted exactly once and sleep
// zero times.
func TestPermanentErrorSkipsEveryRetry(t *testing.T) {
	calls, sleeps := 0, 0
	attempts, err := handleWithRetries(context.Background(),
		func(context.Context, v1.Event) error { calls++; return testPermanentError{} },
		func(context.Context, time.Duration) bool { sleeps++; return true },
		v1.Event{})
	if attempts != 1 || calls != 1 {
		t.Fatalf("attempts=%d calls=%d, a permanent error must be attempted exactly once", attempts, calls)
	}
	if sleeps != 0 {
		t.Fatalf("sleeps=%d, a permanent error must skip every retry", sleeps)
	}
	if !isPermanent(err) {
		t.Fatalf("err=%v must remain permanent", err)
	}
}

// TestUnrecognizedErrorKeepsRetrying protects the conservative rule with the
// exact schedule: anything the consumer cannot prove unrepeatable gets all five
// attempts, separated by 1, 2, 4 and 8 seconds and with no useless pause after
// the last one. This test must not be weakened.
func TestUnrecognizedErrorKeepsRetrying(t *testing.T) {
	calls := 0
	var backoff []time.Duration
	attempts, err := handleWithRetries(context.Background(),
		func(context.Context, v1.Event) error { calls++; return errors.New("unrecognized") },
		func(_ context.Context, d time.Duration) bool { backoff = append(backoff, d); return true },
		v1.Event{})
	if attempts != 5 || calls != 5 {
		t.Fatalf("attempts=%d calls=%d, an untagged error must not be discarded early", attempts, calls)
	}
	if err == nil {
		t.Fatal("the last transient error must be returned")
	}
	if !reflect.DeepEqual(backoff, retryBackoff) {
		t.Fatalf("backoff=%v want=%v", backoff, retryBackoff)
	}
	if !reflect.DeepEqual(retryBackoff, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}) {
		t.Fatalf("retryBackoff=%v want [1s 2s 4s 8s]", retryBackoff)
	}
}

// TestSuccessAfterRetries proves the retry loop stops as soon as the work
// succeeds instead of running the whole schedule.
func TestSuccessAfterRetries(t *testing.T) {
	calls := 0
	attempts, err := handleWithRetries(context.Background(),
		func(context.Context, v1.Event) error {
			calls++
			if calls < 3 {
				return errors.New("transient")
			}
			return nil
		},
		func(context.Context, time.Duration) bool { return true },
		v1.Event{})
	if err != nil || attempts != 3 || calls != 3 {
		t.Fatalf("err=%v attempts=%d calls=%d", err, attempts, calls)
	}
}

// TestCancelledContextStopsRetrying protects shutdown: once the context ends
// during a pause the loop returns the context error instead of dead-lettering a
// delivery that was never given its full budget.
func TestCancelledContextStopsRetrying(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls, sleeps := 0, 0
	attempts, err := handleWithRetries(ctx,
		func(context.Context, v1.Event) error { calls++; return errors.New("transient") },
		func(context.Context, time.Duration) bool { sleeps++; return false },
		v1.Event{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v want context.Canceled", err)
	}
	if attempts == 5 {
		t.Fatalf("attempts=%d must not exhaust the schedule once the context ends", attempts)
	}
	if sleeps != 1 {
		t.Fatalf("sleeps=%d want=1", sleeps)
	}
	if calls == 0 {
		t.Fatal("the handler must still be attempted once")
	}
}

// TestReadyReasonIsTheWholeReadinessDecision is the decision table for the
// defect this slice closes: a consumer that dead-letters every event must stop
// reporting ready. It runs without a database and without a broker because the
// decision is a pure function.
func TestReadyReasonIsTheWholeReadinessDecision(t *testing.T) {
	cases := []struct {
		name    string
		dbOK    bool
		broker  bool
		depth   int64
		max     int64
		wantOK  bool
		wantWhy string
	}{
		{name: "all healthy", dbOK: true, broker: true, depth: 0, max: 0, wantOK: true, wantWhy: ""},
		{name: "database down", dbOK: false, broker: true, depth: 0, max: 0, wantOK: false, wantWhy: "database_unavailable"},
		{name: "broker down", dbOK: true, broker: false, depth: 0, max: 0, wantOK: false, wantWhy: "broker_unavailable"},
		{name: "depth at threshold", dbOK: true, broker: true, depth: 5, max: 5, wantOK: true, wantWhy: ""},
		{name: "depth above threshold", dbOK: true, broker: true, depth: 6, max: 5, wantOK: false, wantWhy: "dead_letter_backlog"},
		{name: "depth unknown", dbOK: true, broker: true, depth: DeadLetterDepthUnknown, max: 5, wantOK: false, wantWhy: "dead_letter_depth_unknown"},
		{name: "unknown depth with the default threshold", dbOK: true, broker: true, depth: DeadLetterDepthUnknown, max: 0, wantOK: false, wantWhy: "dead_letter_depth_unknown"},
		{name: "database wins over broker", dbOK: false, broker: false, depth: 0, max: 0, wantOK: false, wantWhy: "database_unavailable"},
		{name: "dead-letter backlog with a known depth", dbOK: true, broker: true, depth: 1, max: 0, wantOK: false, wantWhy: "dead_letter_backlog"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, why := ReadyReason(tc.dbOK, tc.broker, tc.depth, tc.max)
			if ok != tc.wantOK || why != tc.wantWhy {
				t.Fatalf("ReadyReason(%v,%v,%d,%d)=(%v,%q) want (%v,%q)", tc.dbOK, tc.broker, tc.depth, tc.max, ok, why, tc.wantOK, tc.wantWhy)
			}
		})
	}
}

// TestMetricsTextPinsTheDeadLetterDepthLine protects the /metrics wire format.
// The pre-existing metric names and their order must not move, and the UNKNOWN
// depth must surface as -1 instead of a healthy-looking zero.
func TestMetricsTextPinsTheDeadLetterDepthLine(t *testing.T) {
	want := "firewatch_http_requests_total 11\n" +
		"firewatch_broker_ready 1\n" +
		"firewatch_events_published_total 21\n" +
		"firewatch_events_processed_total 31\n" +
		"firewatch_events_rejected_total 41\n" +
		"firewatch_dead_letter_depth 0\n" +
		"firewatch_dead_letter_depth_age_seconds -1\n"
	if got := MetricsText(11, true, 21, 31, 41, 0); got != want {
		t.Fatalf("metrics text mismatch\n got=%q\nwant=%q", got, want)
	}
	if got := MetricsText(0, true, 0, 0, 0, -1, 7); !strings.Contains(got, "firewatch_dead_letter_depth_age_seconds 7\n") {
		t.Fatalf("age metric missing: %q", got)
	}
	unknown := MetricsText(0, false, 1, 2, 3, DeadLetterDepthUnknown)
	if !strings.Contains(unknown, "firewatch_dead_letter_depth -1\n") {
		t.Fatalf("unknown depth must be reported as -1, got %q", unknown)
	}
	if !strings.Contains(unknown, "firewatch_broker_ready 0\n") {
		t.Fatalf("broker readiness line changed: %q", unknown)
	}
	if !strings.Contains(unknown, "firewatch_events_processed_total 2\n") {
		t.Fatalf("processed counter line changed: %q", unknown)
	}
	if !strings.Contains(unknown, "firewatch_events_rejected_total 3\n") {
		t.Fatalf("rejected counter line changed: %q", unknown)
	}
}

func TestSnapshotFreshness(t *testing.T) {
	now := time.Unix(100, 0)
	for _, tc := range []struct {
		name     string
		snapshot *DeadLetterSnapshot
		depth    int64
		age      float64
	}{
		{"never observed", nil, -1, -1},
		{"fresh zero", &DeadLetterSnapshot{Depth: 0, LastSuccess: now, Ready: true}, 0, 0},
		{"expiry equality", &DeadLetterSnapshot{Depth: 0, LastSuccess: now.Add(-6 * time.Second), Ready: true}, 0, 6},
		{"stale unknown", &DeadLetterSnapshot{Depth: 0, LastSuccess: now.Add(-6*time.Second - time.Nanosecond), Ready: true}, -1, 6.000000001},
		{"future timestamp", &DeadLetterSnapshot{Depth: 0, LastSuccess: now.Add(time.Second), Ready: true}, -1, -1},
		{"failed observation retains age", &DeadLetterSnapshot{Depth: -1, LastSuccess: now.Add(-2 * time.Second)}, -1, 2},
		{"missing timestamp", &DeadLetterSnapshot{Depth: 0, Ready: true}, -1, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &Broker{}
			b.depthSnapshot.Store(tc.snapshot)
			s := b.Snapshot(now, 6*time.Second)
			if s.Depth != tc.depth || s.AgeSeconds != tc.age {
				t.Fatalf("snapshot=%+v want depth=%d age=%v", s, tc.depth, tc.age)
			}
		})
	}
}

func await(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("session did not make progress")
	}
}

func TestSessionMonitorLifecycle(t *testing.T) {
	for _, cooperative := range []bool{true, false} {
		t.Run(fmt.Sprintf("cooperative=%v", cooperative), func(t *testing.T) {
			b := &Broker{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ticks := make(chan time.Time)
			failures := make(chan error, 1)
			entered, release, closed, done := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
			observed := make(chan struct{}, 2)
			count := 0
			query := func() (int64, error) { count++; observed <- struct{}{}; return int64(count - 1), nil }
			go func() {
				defer close(done)
				_ = b.runSession(ctx, query, ticks, failures, func() { close(closed) }, func(c context.Context) error {
					_, err := handleWithRetries(c, func(work context.Context, _ v1.Event) error {
						close(entered)
						if cooperative {
							<-work.Done()
						} else {
							<-release
						}
						return work.Err()
					}, pause, v1.Event{})
					return err
				}, func(c context.Context) error { <-c.Done(); return c.Err() })
			}()
			await(t, entered)
			if s := b.Snapshot(time.Now(), 6*time.Second); s.Depth != 0 || !s.Ready {
				t.Fatalf("initial snapshot=%+v", s)
			}
			<-observed
			ticks <- time.Now()
			<-observed
			// Receiving another tick proves the previous observation was committed.
			ticks <- time.Now()
			if s := b.Snapshot(time.Now(), 6*time.Second); s.Depth < 1 || !s.Ready {
				t.Fatalf("monitor failed to detect backlog during blocked handler: %+v", s)
			}
			// A query completed while the consumer was blocked. Failure is handled
			// by the independent supervisor, not by the consumer's select loop.
			failures <- errors.New("connection lost")
			await(t, closed)
			if s := b.Snapshot(time.Now(), 6*time.Second); s.Depth != -1 || s.Ready || s.LastSuccess.IsZero() {
				t.Fatalf("invalidated snapshot=%+v", s)
			}
			if !cooperative {
				select {
				case <-done:
					t.Fatal("session returned before joining handler")
				default:
				}
				close(release)
			}
			await(t, done)
		})
	}
}

func TestBlockedPassiveQueryCannotRestoreInvalidatedSnapshot(t *testing.T) {
	b := &Broker{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, release, closed, done := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	failures := make(chan error, 1)
	go func() {
		defer close(done)
		_ = b.runSession(ctx, func() (int64, error) { close(started); <-release; return 0, nil }, make(chan time.Time), failures,
			func() { close(closed); close(release) }, func(context.Context) error { t.Error("consumer started before initial observation"); return nil },
			func(c context.Context) error { <-c.Done(); return c.Err() })
	}()
	await(t, started)
	failures <- errors.New("monitor channel closed")
	await(t, closed)
	await(t, done)
	if s := b.Snapshot(time.Now(), 6*time.Second); s.Depth != -1 || s.Ready || !s.LastSuccess.IsZero() {
		t.Fatalf("late query restored health: %+v", s)
	}
}

func TestFailedQueryRetainsSuccessfulObservationAge(t *testing.T) {
	b := &Broker{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time)
	entered, closed, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	calls := 0
	go func() {
		defer close(done)
		_ = b.runSession(ctx, func() (int64, error) {
			calls++
			if calls == 1 {
				return 0, nil
			}
			return 0, errors.New("passive query failed")
		}, ticks, make(chan error),
			func() { close(closed) }, func(c context.Context) error { close(entered); <-c.Done(); return c.Err() },
			func(c context.Context) error { <-c.Done(); return c.Err() })
	}()
	await(t, entered)
	before := b.Snapshot(time.Now(), 6*time.Second)
	ticks <- time.Now()
	await(t, closed)
	await(t, done)
	after := b.Snapshot(time.Now(), 6*time.Second)
	if after.Depth != -1 || after.LastSuccess != before.LastSuccess || after.AgeSeconds < before.AgeSeconds {
		t.Fatalf("before=%+v after=%+v", before, after)
	}
}

func TestSessionEarlyFailures(t *testing.T) {
	for _, source := range []string{"initial query", "parent cancellation", "relay failure"} {
		t.Run(source, func(t *testing.T) {
			b := &Broker{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started, release, closed, done := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
			go func() {
				defer close(done)
				_ = b.runSession(ctx, func() (int64, error) {
					close(started)
					if source == "initial query" {
						return 0, errors.New("query failed")
					}
					<-release
					return 0, nil
				}, make(chan time.Time), make(chan error), func() { close(closed); close(release) },
					func(context.Context) error { t.Error("consumer started without successful observation"); return nil },
					func(c context.Context) error {
						if source == "relay failure" {
							<-started
							return errors.New("relay failed")
						}
						<-c.Done()
						return c.Err()
					})
			}()
			await(t, started)
			if source == "parent cancellation" {
				cancel()
			}
			await(t, closed)
			await(t, done)
			if s := b.Snapshot(time.Now(), 6*time.Second); s.Ready || s.Depth != -1 {
				t.Fatalf("snapshot=%+v", s)
			}
		})
	}
}

func TestSessionCancellationInterruptsRetryBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	entered, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		_, err := handleWithRetries(ctx, func(context.Context, v1.Event) error { return errors.New("transient") },
			func(c context.Context, d time.Duration) bool { close(entered); return pause(c, d) }, v1.Event{})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err=%v", err)
		}
	}()
	await(t, entered)
	cancel()
	await(t, done)
}

// refusalError is a permanent error carrying a recognizable message, the way
// the publication guards report an incomplete retrieval or an unidentifiable
// zone on their way to the dead-letter queue.
type refusalError struct{ reason string }

func (e refusalError) Error() string   { return e.reason }
func (e refusalError) Permanent() bool { return true }

// TestFailureReasonClassifiesTerminalCause pins the stable code the cross-service
// report carries: only a genuinely permanent error is permanent_error; anything
// else that reaches the end of the budget is retries_exhausted. It runs without a
// broker and without a database.
func TestFailureReasonClassifiesTerminalCause(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"permanent error", testPermanentError{}, failureReasonPermanent},
		{"wrapped permanent error", fmt.Errorf("wrapped: %w", testPermanentError{}), failureReasonPermanent},
		{"exhausted transient error", errors.New("transient"), failureReasonExhausted},
		{"unrecognized error is exhausted, never permanent", errors.New("unrecognized"), failureReasonExhausted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := failureReason(tc.err); got != tc.want {
				t.Fatalf("failureReason(%v)=%q want %q", tc.err, got, tc.want)
			}
		})
	}
}

// TestGiveUpReportsTheTerminalFailure pins what a give-up sends across services:
// the job id, the trace id, the classified reason and the attempt count, recorded
// through the injected writer. A report that fails is logged and still counts the
// rejection, so the dead letter stays the record and no evidence is invented. It
// exercises the settleDeadLetter seam directly, so it needs neither a broker nor
// a database and does not spend the retry budget's sleeps.
func TestGiveUpReportsTheTerminalFailure(t *testing.T) {
	for _, tc := range []struct {
		name         string
		workErr      error
		attempts     int
		wantReason   string
		reportErr    error
		wantReported bool
	}{
		{"permanent failure reports its own code", testPermanentError{}, 1, failureReasonPermanent, nil, true},
		{"exhausted budget reports the attempt count", errors.New("transient"), 5, failureReasonExhausted, nil, true},
		{"a failed report still counts the rejection", errors.New("transient"), 5, failureReasonExhausted, errors.New("outbox unavailable"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := v1.Event{JobID: uuid.NewString(), TraceID: uuid.NewString()}
			recorded := struct {
				called   bool
				jobID    string
				traceID  string
				reason   string
				attempts int
			}{}
			b := &Broker{ReportFailure: func(_ context.Context, e v1.Event, reason string, attempts int) error {
				recorded.called, recorded.jobID, recorded.traceID, recorded.reason, recorded.attempts = true, e.JobID, e.TraceID, reason, attempts
				return tc.reportErr
			}}
			b.settleDeadLetter(context.Background(), event, tc.attempts, tc.workErr)
			if b.Rejected.Load() != 1 {
				t.Fatalf("rejected counter=%d want 1", b.Rejected.Load())
			}
			if recorded.called != tc.wantReported {
				t.Fatalf("report called=%v want %v", recorded.called, tc.wantReported)
			}
			if recorded.jobID != event.JobID || recorded.traceID != event.TraceID || recorded.reason != tc.wantReason || recorded.attempts != tc.attempts {
				t.Fatalf("reported=%+v want %s/%s/%s/%d", recorded, event.JobID, event.TraceID, tc.wantReason, tc.attempts)
			}
		})
	}
}

// TestTopologyBindsTheJobsQueueToBothKeys protects the additive routing contract
// on the risk side as well: both services must declare the same topology, so
// risk's queue keeps its single key and jobs' queue accepts both the batch result
// and the cross-service failure report.
func TestTopologyBindsTheJobsQueueToBothKeys(t *testing.T) {
	want := []struct {
		queue string
		keys  []string
	}{
		{queue: v1.RiskQueue, keys: []string{v1.JobRequestedType}},
		{queue: v1.JobsQueue, keys: []string{v1.BatchPublishedType, v1.JobFailedType}},
	}
	if !reflect.DeepEqual(queueTopology, want) {
		t.Fatalf("queueTopology=%v want=%v", queueTopology, want)
	}
}

// TestDeadLetterLogCarriesTheFailureReason protects the operator-facing record
// of a dead-lettered delivery: the log entry must name the underlying failure,
// because the batch's guards now refuse work for an incomplete retrieval or an
// unidentifiable zone and that refusal reason would otherwise reach the queue
// without a readable cause.
func TestDeadLetterLogCarriesTheFailureReason(t *testing.T) {
	reason := "guard refused: incomplete retrieval"
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
	defer slog.SetDefault(previous)

	now := time.Now().UTC()
	event := v1.Event{
		ID:          uuid.NewString(),
		Type:        v1.JobRequestedType,
		OccurredAt:  now,
		TraceID:     uuid.NewString(),
		JobID:       uuid.NewString(),
		ActorID:     "unit-test",
		ZoneID:      "valparaiso-demo",
		RequestedAt: now,
		Demo:        true,
	}
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- amqp.Delivery{Body: body, DeliveryTag: 1}
	close(deliveries)

	b := &Broker{Handle: func(context.Context, v1.Event) error { return refusalError{reason: reason} }}
	// The delivery has no acknowledger, so consume returns the Reject error after
	// logging. That outcome is expected; the log entry is what this test pins.
	if err := b.consume(context.Background(), deliveries); err == nil {
		t.Fatal("consume returned no error after a dead-lettered delivery")
	}
	entry := logged.String()
	if !strings.Contains(entry, "event_dead_lettered") {
		t.Fatalf("the dead-letter log entry is missing: %q", entry)
	}
	if !strings.Contains(entry, reason) {
		t.Fatalf("the dead-letter log entry drops the failure reason %q: %q", reason, entry)
	}
}
