package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"firewatch.local/risk/internal/handler"
	"firewatch.local/risk/internal/messaging"
	"firewatch.local/risk/internal/metrics"
)

func TestStatusEvaluator(t *testing.T) {
	for _, tc := range []struct {
		name          string
		db, broker    bool
		depth         int64
		age           float64
		reason, state string
	}{
		{"boundary", true, true, 2, 6, "", "ready"}, {"backlog", true, true, 3, 6, "dead_letter_backlog", "degraded"},
		{"expired", true, true, -1, 6.01, "dead_letter_depth_unknown", "unknown"}, {"never", true, true, -1, -1, "dead_letter_depth_unknown", "unknown"},
		{"disconnected", true, false, -1, 2, "broker_unavailable", "unknown"}, {"db precedence", false, false, -1, 2, "database_unavailable", "unknown"},
		{"future", true, true, -1, -2, "dead_letter_depth_unknown", "unknown"}, {"broker before backlog", true, false, 3, 2, "broker_unavailable", "degraded"}, {"db before backlog", false, true, 3, 2, "database_unavailable", "degraded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pinged, calls := false, 0
			start := time.Now()
			ping := func(ctx context.Context) error {
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > time.Second {
					t.Fatal("unbounded ping")
				}
				pinged = true
				if !tc.db {
					return context.Canceled
				}
				return nil
			}
			snapshot := func() messaging.DeadLetterSnapshot {
				if !pinged {
					t.Fatal("snapshot before ping")
				}
				calls++
				sample := start.Add(-time.Duration(tc.age * float64(time.Second)))
				if tc.age == -1 {
					sample = time.Time{}
				}
				return messaging.DeadLetterSnapshot{Ready: tc.broker, Depth: tc.depth, AgeSeconds: tc.age, LastSuccess: sample}
			}
			// The authenticated status route keeps the whole dependency decision
			// (ReadyReason): a degraded broker or a dead-letter backlog still
			// answers 503 with a reason there, which is the honest signal S3 keeps.
			check := statusEvaluator(ping, snapshot, 2, 6*time.Second)
			s := check(context.Background())
			d := s.Dependencies.DeadLetter
			if calls != 1 || s.Reason != tc.reason || s.Ready != (tc.reason == "") || d.State != tc.state || s.CheckedAt.Before(start) || s.CheckedAt.Location() != time.UTC || *d.MaxDepth != 2 || *d.MaxAgeSeconds != 6 || (d.Depth == nil) != (tc.depth < 0) || (d.SampledAt == nil) != (tc.age < 0) || (d.AgeSeconds == nil) != (tc.age < 0) {
				t.Fatalf("status=%+v dead_letter=%+v calls=%d", s, d, calls)
			}
			// /health/ready routes by the database alone (S2): the broker state and
			// the dead-letter depth must not change this code. The body is the same
			// status body the authenticated route returns (S3), so its ready field is
			// the full composite decision and may be false while the probe answers 200.
			w := httptest.NewRecorder()
			healthHandler(check).ServeHTTP(w, httptest.NewRequest("GET", "/health/ready", nil))
			body := decodeStatus(t, w.Body.Bytes())
			wantCode := 503
			if tc.db {
				wantCode = 200
			}
			if w.Code != wantCode || body.Ready != s.Ready || body.Reason != tc.reason || calls != 2 {
				t.Fatalf("health disagrees: %d ready=%v reason=%q snapshots=%d body=%+v", w.Code, body.Ready, body.Reason, calls, body)
			}
			if !tc.db && strings.TrimSpace(body.Reason) == "" {
				t.Fatalf("a 503 readiness body carries no reason: %q", w.Body.String())
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			canceled := statusEvaluator(func(c context.Context) error {
				if c.Err() != context.Canceled {
					t.Fatal("lost cancellation")
				}
				return c.Err()
			}, func() messaging.DeadLetterSnapshot { return messaging.DeadLetterSnapshot{Depth: -1} }, 2, 6*time.Second)(ctx)
			if canceled.Reason != "database_unavailable" {
				t.Fatal(canceled)
			}
		})
	}
}

// TestCycleZones pins the scheduler's zone configuration: an unset value keeps
// the deterministic demo zone, blank entries are dropped, and a list that names
// no usable zone still schedules rather than running an empty cycle.
func TestCycleZones(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want []string
	}{
		{"", []string{"valparaiso-demo"}},
		{"   ", []string{"valparaiso-demo"}},
		{"05101", []string{"05101"}},
		{"05101, 05102 ,05103", []string{"05101", "05102", "05103"}},
		{",,", []string{"valparaiso-demo"}},
	} {
		if got := cycleZones(tc.raw); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("cycleZones(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestDLQConfiguration(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		want    time.Duration
		invalid bool
	}{
		{"", 6 * time.Second, false}, {"1.5s", 1500 * time.Millisecond, false},
		{"garbage", 0, true}, {"0s", 0, true}, {"-1s", 0, true},
	} {
		t.Run("age="+tc.raw, func(t *testing.T) {
			got, err := parseDLQReadyMaxAge(tc.raw)
			if (err != nil) != tc.invalid || got != tc.want {
				t.Fatalf("got=%v err=%v", got, err)
			}
		})
	}
	for _, raw := range []string{"invalid", "-1"} {
		if _, err := parseDLQReadyMaxDepth(raw); err == nil {
			t.Fatalf("accepted depth %q", raw)
		}
	}
	if got, err := parseDLQReadyMaxDepth(""); err != nil || got != 0 {
		t.Fatalf("depth=%d err=%v", got, err)
	}
}

// decodeStatus decodes the status body a readiness or status route returned.
func decodeStatus(t *testing.T, body []byte) handler.Status {
	t.Helper()
	var status handler.Status
	if err := json.Unmarshal(body, &status); err != nil {
		t.Fatalf("the route body is not a status body: %v (%q)", err, body)
	}
	return status
}

// TestReadinessBodyReportsDegradationWithoutFailing protects S3: readiness now
// follows the database, so the broker and dead-letter signals must stay visible
// where an operator can still act on them. They are reported as degradation in
// the body of a 200 readiness response, with their depth, age and thresholds,
// and the authenticated status route keeps answering 503 with a reason for the
// same dependency state. Nothing honest was removed, only the routing decision
// moved.
func TestReadinessBodyReportsDegradationWithoutFailing(t *testing.T) {
	instant := time.Now().UTC()
	sampledAt := instant.Add(-42 * time.Second)
	ping := func(context.Context) error { return nil }
	for _, tc := range []struct {
		name            string
		snapshot        messaging.DeadLetterSnapshot
		brokerState     string
		deadLetterState string
		reason          string
	}{
		{"broker unavailable", messaging.DeadLetterSnapshot{Ready: false, Depth: 5, AgeSeconds: 42.5, LastSuccess: sampledAt}, "degraded", "degraded", "broker_unavailable"},
		{"dead-letter backlog", messaging.DeadLetterSnapshot{Ready: true, Depth: 5, AgeSeconds: 42.5, LastSuccess: sampledAt}, "ready", "degraded", "dead_letter_backlog"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := func() messaging.DeadLetterSnapshot { return tc.snapshot }
			w := httptest.NewRecorder()
			healthHandler(statusEvaluator(ping, snapshot, 3, 6*time.Second)).ServeHTTP(w, httptest.NewRequest("GET", "/health/ready", nil))
			body := decodeStatus(t, w.Body.Bytes())
			// S2: the database ping alone routes this request, so the degraded broker
			// does not fail the probe. S3: the body still carries the full composite
			// decision, so ready is false here while the probe answers 200.
			if w.Code != 200 || body.Ready {
				t.Fatalf("a database-healthy service was not routed, or the body lost the composite decision: %d %+v", w.Code, body)
			}
			if body.Dependencies.Database.State != "ready" || body.Dependencies.Broker.State != tc.brokerState {
				t.Fatalf("the readiness body lost the dependency truth: %+v", body.Dependencies)
			}
			deadLetter := body.Dependencies.DeadLetter
			if deadLetter.State != tc.deadLetterState || deadLetter.Depth == nil || *deadLetter.Depth != 5 || deadLetter.MaxDepth == nil || *deadLetter.MaxDepth != 3 || deadLetter.MaxAgeSeconds == nil || *deadLetter.MaxAgeSeconds != 6 || deadLetter.AgeSeconds == nil || *deadLetter.AgeSeconds != 42.5 || deadLetter.SampledAt == nil || !deadLetter.SampledAt.Equal(sampledAt) {
				t.Fatalf("the readiness body lost the dead-letter depth, age or sample: %+v, want a sample at %s", deadLetter, sampledAt)
			}
			if body.Reason != tc.reason {
				t.Fatalf("the degradation is not named in the body: %q, want %q", body.Reason, tc.reason)
			}
			// The same dependency state still fails the authenticated status route:
			// the honest 503 signal is not removed, it moved to the route that
			// reports every dependency.
			if status := statusEvaluator(ping, snapshot, 3, 6*time.Second)(t.Context()); status.Ready || status.Reason != tc.reason {
				t.Fatalf("the authenticated status route lost the degraded decision: %+v", status)
			}
		})
	}
}

// TestMetricsKeepsReportingBrokerAndDeadLetterTruth protects the second surface
// S3 keeps: /metrics still exposes the broker readiness and the dead-letter
// depth and age, so a broker outage or a backlog stays visible there even though
// it no longer removes the replica from the load balancer.
func TestMetricsKeepsReportingBrokerAndDeadLetterTruth(t *testing.T) {
	var requests atomic.Uint64
	requests.Store(7)
	broker := &messaging.Broker{}
	broker.Published.Store(3)
	broker.Processed.Store(2)
	broker.Rejected.Store(1)
	// One high-risk outcome and one other level, to prove the model counters share
	// the body and report both the named total and the labeled distribution.
	outcomes := &metrics.RiskOutcomes{}
	outcomes.Observe("alto")
	outcomes.Observe("bajo")
	// One success and one server error, so the HTTP classes, the error series and
	// the latency histogram are all exercised through the real handler.
	httpMetrics := &metrics.HTTPMetrics{}
	httpMetrics.Observe(200, 15*time.Millisecond)
	httpMetrics.Observe(503, 2*time.Second)
	snapshot := func() messaging.DeadLetterSnapshot {
		return messaging.DeadLetterSnapshot{Ready: false, Depth: 4, AgeSeconds: 12.5}
	}
	w := httptest.NewRecorder()
	metricsHandler(snapshot, &requests, broker,
		outcomes.Text,
		httpMetrics.Text,
		metrics.RuntimeText,
		func() string { return metrics.PoolStats{Total: 4, Acquired: 1, Idle: 3, Max: 10}.Text() },
	).ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if got := w.Header().Get("Content-Type"); got != "text/plain; version=0.0.4" {
		t.Fatalf("metrics content type is %q", got)
	}
	body := w.Body.String()
	for _, want := range []string{
		"firewatch_http_requests_total 7",
		"firewatch_broker_ready 0",
		"firewatch_events_published_total 3",
		"firewatch_events_processed_total 2",
		"firewatch_events_rejected_total 1",
		"firewatch_dead_letter_depth 4",
		"firewatch_dead_letter_depth_age_seconds 12.5",
		"firewatch_high_risk_predictions_total 1",
		"firewatch_risk_predictions_total{level=\"alto\"} 1",
		"firewatch_risk_predictions_total{level=\"bajo\"} 1",
		"firewatch_risk_predictions_total{level=\"medio\"} 0",
		"firewatch_risk_predictions_total{level=\"desconocido\"} 0",
		"firewatch_http_responses_total{class=\"2xx\"} 1",
		"firewatch_http_responses_total{class=\"5xx\"} 1",
		"firewatch_http_errors_total 1",
		"firewatch_http_request_duration_seconds_count 2",
		"firewatch_process_goroutines ",
		"firewatch_db_pool_total_conns 4",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body %q does not contain %q", body, want)
		}
	}
}

func TestReadinessHTTPUsesEffectiveSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name     string
		snapshot messaging.DeadLetterSnapshot
		dbOK     bool
		code     int
		reason   string
	}{
		// The database decides the routing code (S2). The broker state and the
		// dead-letter depth no longer change this code; they stay reported as
		// degradation in the body's composite ready and reason and in /metrics (S3).
		{"fresh zero", messaging.DeadLetterSnapshot{Depth: 0, Ready: true}, true, 200, ""},
		{"stale unknown", messaging.DeadLetterSnapshot{Depth: -1, Ready: true}, true, 200, "dead_letter_depth_unknown"},
		{"never observed", messaging.DeadLetterSnapshot{Depth: -1}, true, 200, "broker_unavailable"},
		{"backlog", messaging.DeadLetterSnapshot{Depth: 1, Ready: true}, true, 200, "dead_letter_backlog"},
		{"database unavailable", messaging.DeadLetterSnapshot{Depth: 0, Ready: true}, false, 503, "database_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			check := statusEvaluator(func(context.Context) error {
				if tc.dbOK {
					return nil
				}
				return context.DeadlineExceeded
			},
				func() messaging.DeadLetterSnapshot { calls++; return tc.snapshot }, 0, 6*time.Second)
			w := httptest.NewRecorder()
			healthHandler(check).ServeHTTP(w, httptest.NewRequest("GET", "/health/ready", nil))
			body := decodeStatus(t, w.Body.Bytes())
			if w.Code != tc.code || calls != 1 {
				t.Fatalf("status=%d snapshots=%d body=%+v", w.Code, calls, body)
			}
			// The body's ready field is the full composite decision, so it can be
			// false while the probe answers 200 from a healthy database ping.
			if body.Ready != (tc.reason == "") {
				t.Fatalf("body ready=%v, want the composite dependency decision %v", body.Ready, tc.reason == "")
			}
			if body.Reason != tc.reason {
				t.Fatalf("body reason=%q, want %q", body.Reason, tc.reason)
			}
		})
	}
}
