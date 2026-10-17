package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"firewatch.local/jobs/internal/handler"
	"firewatch.local/jobs/internal/messaging"
	"github.com/jackc/pgx/v5"
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
			check := statusEvaluator(func(ctx context.Context) error {
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > time.Second {
					t.Fatal("unbounded ping")
				}
				pinged = true
				if !tc.db {
					return context.Canceled
				}
				return nil
			}, func() messaging.DeadLetterSnapshot {
				if !pinged {
					t.Fatal("snapshot before ping")
				}
				calls++
				sample := start.Add(-time.Duration(tc.age * float64(time.Second)))
				if tc.age == -1 {
					sample = time.Time{}
				}
				return messaging.DeadLetterSnapshot{Ready: tc.broker, Depth: tc.depth, AgeSeconds: tc.age, LastSuccess: sample}
			}, 2, 6*time.Second)
			s := check(context.Background())
			d := s.Dependencies.DeadLetter
			if calls != 1 || s.Reason != tc.reason || s.Ready != (tc.reason == "") || d.State != tc.state || s.CheckedAt.Before(start) || s.CheckedAt.Location() != time.UTC || *d.MaxDepth != 2 || *d.MaxAgeSeconds != 6 || (d.Depth == nil) != (tc.depth < 0) || (d.SampledAt == nil) != (tc.age < 0) || (d.AgeSeconds == nil) != (tc.age < 0) {
				t.Fatalf("status=%+v dead_letter=%+v calls=%d", s, d, calls)
			}
			w := httptest.NewRecorder()
			healthHandler(check).ServeHTTP(w, httptest.NewRequest("GET", "/health/ready", nil))
			// S2: the readiness probe is decided only by the bounded database ping.
			// Broker and dead-letter state travel in the body but never change this
			// status code, so a database-only decision can answer 200 while the body
			// still names a degraded dependency.
			want := 503
			if tc.db {
				want = 200
			}
			var reported handler.Status
			if json.Unmarshal(w.Body.Bytes(), &reported) != nil {
				t.Fatalf("health body is not the status object: %q", w.Body.String())
			}
			if w.Code != want || !strings.Contains(w.Body.String(), s.Reason) || calls != 2 {
				t.Fatalf("health disagrees: %d %s", w.Code, w.Body)
			}
			if reported.Reason != s.Reason || reported.Ready != s.Ready ||
				reported.Dependencies.Database != s.Dependencies.Database ||
				reported.Dependencies.Broker != s.Dependencies.Broker ||
				reported.Dependencies.DeadLetter.State != s.Dependencies.DeadLetter.State {
				t.Fatalf("health body lost the operational report: %s", w.Body)
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

// stubRow is one scan result, so the queued-age read is testable without a
// database.
type stubRow struct {
	waited *float64
	err    error
}

func (r stubRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*(dest[0].(**float64)) = r.waited
	return nil
}

// stubQuerier records the query and context it was asked for and returns a
// canned row.
type stubQuerier struct {
	sql string
	ctx context.Context
	row stubRow
}

func (q *stubQuerier) QueryRow(ctx context.Context, sql string, _ ...any) pgx.Row {
	q.sql, q.ctx = sql, ctx
	return q.row
}

// TestQueuedAgeSecondsIsBoundedAndHonest pins the stuck-job signal: a readable
// wait is reported in seconds, no queued job and a failed read both report
// QueuedAgeUnknown, and the query is bounded to a single queued row under a
// deadline. It never marks a job failed.
func TestQueuedAgeSecondsIsBoundedAndHonest(t *testing.T) {
	value := 91.5
	for _, tc := range []struct {
		name   string
		waited *float64
		err    error
		want   float64
	}{
		{"a queued job's wait is reported", &value, nil, 91.5},
		{"no queued job is unknown", nil, nil, messaging.QueuedAgeUnknown},
		{"a failed read is unknown", nil, errors.New("read failed"), messaging.QueuedAgeUnknown},
		{"a driver no-rows is unknown", nil, pgx.ErrNoRows, messaging.QueuedAgeUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := &stubQuerier{row: stubRow{waited: tc.waited, err: tc.err}}
			if got := queuedAgeSeconds(context.Background(), q); got != tc.want {
				t.Fatalf("queuedAgeSeconds=%v want %v", got, tc.want)
			}
			if !strings.Contains(q.sql, "state='queued'") || !strings.Contains(q.sql, "LIMIT 1") {
				t.Fatalf("the queued-age query is not bounded to a single queued row: %q", q.sql)
			}
			if _, ok := q.ctx.Deadline(); !ok {
				t.Fatal("the queued-age query is not bounded by a deadline")
			}
		})
	}
}

// TestReadinessHTTPUsesEffectiveSnapshot pins S2 and S3 together: the HTTP code
// follows the database alone, while the body keeps the broker and dead-letter
// truth that the authenticated status route and /metrics also expose.
func TestReadinessHTTPUsesEffectiveSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name       string
		snapshot   messaging.DeadLetterSnapshot
		dbOK       bool
		code       int
		reason     string
		broker     string
		deadLetter string
	}{
		{"fresh zero", messaging.DeadLetterSnapshot{Depth: 0, Ready: true}, true, 200, "", "ready", "ready"},
		{"stale unknown", messaging.DeadLetterSnapshot{Depth: -1, Ready: true}, true, 200, "dead_letter_depth_unknown", "ready", "unknown"},
		{"never observed", messaging.DeadLetterSnapshot{Depth: -1}, true, 200, "broker_unavailable", "degraded", "unknown"},
		{"backlog", messaging.DeadLetterSnapshot{Depth: 1, Ready: true}, true, 200, "dead_letter_backlog", "ready", "degraded"},
		{"database unavailable", messaging.DeadLetterSnapshot{Depth: 0, Ready: true}, false, 503, "database_unavailable", "ready", "ready"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			h := readinessHandler(func(context.Context) error {
				if tc.dbOK {
					return nil
				}
				return context.DeadlineExceeded
			},
				func() messaging.DeadLetterSnapshot { calls++; return tc.snapshot }, 0)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/health/ready", nil))
			var body handler.Status
			if json.Unmarshal(w.Body.Bytes(), &body) != nil {
				t.Fatalf("body is not the status object: %q", w.Body.String())
			}
			if w.Code != tc.code || calls != 1 || body.Reason != tc.reason ||
				body.Dependencies.Broker.State != tc.broker || body.Dependencies.DeadLetter.State != tc.deadLetter {
				t.Fatalf("status=%d body=%q snapshots=%d", w.Code, w.Body.String(), calls)
			}
		})
	}
}
