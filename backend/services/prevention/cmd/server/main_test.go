package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestReadyHandlerReportsTheDatabaseDecision pins the readiness probe: the code
// is the routing decision and the body reports the same decision, so a degraded
// database can never answer 200 with a body that says otherwise. The ping is
// asserted to be bounded, because an unbounded probe would hang the kubelet
// instead of degrading.
func TestReadyHandlerReportsTheDatabaseDecision(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pingErr  error
		wantCode int
		ready    bool
		state    string
		reason   string
	}{
		{name: "ready", wantCode: http.StatusOK, ready: true, state: "ready"},
		{name: "database down", pingErr: errors.New("connection refused"), wantCode: http.StatusServiceUnavailable, ready: false, state: "degraded", reason: "database_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ping := func(ctx context.Context) error {
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > time.Second {
					t.Error("the readiness ping must be bounded")
				}
				return tc.pingErr
			}
			w := httptest.NewRecorder()
			readyHandler(ping).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
			if w.Code != tc.wantCode {
				t.Fatalf("answered %d, want %d: %s", w.Code, tc.wantCode, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
				t.Errorf("unexpected headers %v", w.Header())
			}
			var body struct {
				CheckedAt    time.Time `json:"checked_at"`
				Ready        bool      `json:"ready"`
				Reason       string    `json:"reason"`
				Dependencies map[string]struct {
					State string `json:"state"`
				} `json:"dependencies"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Ready != tc.ready || body.Reason != tc.reason {
				t.Errorf("ready=%v reason=%q, want %v and %q", body.Ready, body.Reason, tc.ready, tc.reason)
			}
			if body.CheckedAt.IsZero() {
				t.Error("checked_at must be reported")
			}
			if got := body.Dependencies["database"].State; got != tc.state {
				t.Errorf("database state %q, want %q", got, tc.state)
			}
			// The service talks to its database and nothing else: a broker field
			// here would be a fabricated dependency.
			if len(body.Dependencies) != 1 {
				t.Errorf("dependencies %v must contain only database", body.Dependencies)
			}
		})
	}
}
