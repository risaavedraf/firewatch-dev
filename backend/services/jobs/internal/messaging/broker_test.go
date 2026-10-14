package messaging

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	v1 "firewatch.local/contracts/events/v1"
	"github.com/google/uuid"
)

func TestValidSeparatesShapeFromOrigin(t *testing.T) {
	base := v1.Event{ID: uuid.NewString(), Type: v1.BatchPublishedType, ActorID: "admin", ZoneID: "valparaiso-demo", RequestedAt: time.Now(), OccurredAt: time.Now(), JobID: uuid.NewString(), TraceID: uuid.NewString(), BatchID: uuid.NewString(), Demo: true}
	cases := []struct {
		name   string
		mutate func(*v1.Event)
		want   bool
	}{
		{"demo fixture", func(e *v1.Event) {}, true},
		{"non-demo origin is still well formed", func(e *v1.Event) { e.Demo = false }, true},
		{"empty zone is malformed", func(e *v1.Event) { e.ZoneID = "" }, false},
		{"unknown non-empty zone passes the shape gate; the store owns the zone relationship", func(e *v1.Event) { e.ZoneID = "otra-zona" }, true},
		{"wrong event", func(e *v1.Event) { e.Type = v1.JobRequestedType }, false},
		{"empty actor", func(e *v1.Event) { e.ActorID = "" }, false},
		{"bad id", func(e *v1.Event) { e.ID = "invalid" }, false},
		{"nil job id", func(e *v1.Event) { e.JobID = uuid.Nil.String() }, false},
		{"zero requested at", func(e *v1.Event) { e.RequestedAt = time.Time{} }, false},
		{"zero occurred at", func(e *v1.Event) { e.OccurredAt = time.Time{} }, false},
		{"nil batch id", func(e *v1.Event) { e.BatchID = uuid.Nil.String() }, false},
		// Synthetic metadata is optional and pair-shaped: a scenario-bearing result
		// must carry the server-assigned instant it was evaluated with, and an
		// instant without a scenario is equally malformed.
		{"scenario without an evaluation instant is malformed", func(e *v1.Event) { e.ScenarioID = "mild" }, false},
		{"evaluation instant without a scenario is malformed", func(e *v1.Event) { instant := time.Now().UTC(); e.EvaluatedAt = &instant }, false},
		{"zero evaluation instant is malformed", func(e *v1.Event) { instant := time.Time{}; e.ScenarioID = "mild"; e.EvaluatedAt = &instant }, false},
		{"scenario with a nonzero instant is well formed", func(e *v1.Event) { instant := time.Now().UTC(); e.ScenarioID = "mild"; e.EvaluatedAt = &instant }, true},
		// The consumer does not duplicate the wire vocabulary: a vocabulary value
		// this build does not know is a shape-valid message, not a dead letter, so
		// adding a scenario profile never forces a coordinated consumer release.
		{"unknown scenario id passes the shape gate", func(e *v1.Event) {
			instant := time.Now().UTC()
			e.ScenarioID = "future-profile"
			e.EvaluatedAt = &instant
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := base
			tc.mutate(&e)
			if got := valid(e); got != tc.want {
				t.Fatalf("valid=%v want=%v", got, tc.want)
			}
		})
	}
}

type testPermanentError struct{}

func (testPermanentError) Error() string   { return "permanent test error" }
func (testPermanentError) Permanent() bool { return true }

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
}

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
