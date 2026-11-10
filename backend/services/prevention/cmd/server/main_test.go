package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"firewatch.local/prevention/internal/metrics"
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

// TestMetricsHandlerRendersTheServiceSignals pins the /metrics surface (T26):
// the handler answers the Prometheus text format with the families the latency
// and error alerts are written against, plus the process signals. A scrape
// cannot obtain a token, so this route is answered before authentication, the
// same posture health has.
func TestMetricsHandlerRendersTheServiceSignals(t *testing.T) {
	httpMetrics := &metrics.HTTPMetrics{}
	httpMetrics.Observe(http.StatusCreated, 10*time.Millisecond)
	httpMetrics.Observe(http.StatusServiceUnavailable, 20*time.Millisecond)
	w := httptest.NewRecorder()
	metricsHandler(httpMetrics.Text, metrics.RuntimeText).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("answered %d, want 200", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "text/plain; version=0.0.4" {
		t.Errorf("content type %q, want the Prometheus text format", got)
	}
	body := w.Body.String()
	for _, want := range []string{
		`firewatch_http_responses_total{class="2xx"} 1`,
		`firewatch_http_responses_total{class="5xx"} 1`,
		"firewatch_http_errors_total 1",
		"firewatch_http_request_duration_seconds_bucket",
		"firewatch_process_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body omits %q:\n%s", want, body)
		}
	}
}

// TestRunRefusesMissingListenerTLSOutsideDevelopment pins the process-level
// wiring of T7.4: outside development a start with no key pair stops before the
// listener binds, and the message names the variable, so the refusal is not
// something an operator has to infer from a plaintext listener.
func TestRunRefusesMissingListenerTLSOutsideDevelopment(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://user:pass@127.0.0.1:1/x?sslmode=verify-full")
	t.Setenv(tlsCertFileEnv, "")
	t.Setenv(tlsKeyFileEnv, "")
	err := run()
	if err == nil || !strings.Contains(err.Error(), tlsCertFileEnv) {
		t.Fatalf("run() = %v, want a refusal naming %s", err, tlsCertFileEnv)
	}
}

// TestRunMigrateDoesNotRequireListenerTLS keeps the migration job of T5 working:
// it holds only DATABASE_URL, never binds a port and never reads a certificate, so
// it must not be asked for the key pair even outside development. The database is
// a closed loopback port, so the run fails on the connection instead of reaching a
// real server, and the failure has to be the migration one: an error about the key
// pair would mean the transport decision moved in front of the migrate branch.
func TestRunMigrateDoesNotRequireListenerTLS(t *testing.T) {
	args := os.Args
	t.Cleanup(func() { os.Args = args })
	os.Args = []string{"server", "migrate"}
	t.Setenv("APP_ENV", "staging")
	t.Setenv("DATABASE_URL", "postgres://user:pass@127.0.0.1:1/x?sslmode=verify-full")
	t.Setenv(tlsCertFileEnv, "")
	t.Setenv(tlsKeyFileEnv, "")
	err := run()
	if err == nil || !strings.Contains(err.Error(), "migration") || strings.Contains(err.Error(), tlsCertFileEnv) {
		t.Fatalf("run(migrate) = %v, want the migration connection failure and no key-pair requirement", err)
	}
}

// TestRunRefusesAnUnusableTLSCAFile pins the other startup decision of T7.4: a
// TLS_CA_FILE that holds no certificate stops the process, naming the variable,
// before the OIDC endpoints are even read. Nothing is bound and no client is
// built with trust the operator did not configure.
func TestRunRefusesAnUnusableTLSCAFile(t *testing.T) {
	blank := filepath.Join(t.TempDir(), "blank.crt")
	if err := os.WriteFile(blank, []byte("not a certificate"), 0o600); err != nil {
		t.Fatalf("write the unusable bundle: %v", err)
	}
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://user:pass@127.0.0.1:1/x?sslmode=verify-full")
	t.Setenv(tlsCertFileEnv, listenerCertPath)
	t.Setenv(tlsKeyFileEnv, listenerKeyPath)
	t.Setenv(tlsCAFileEnv, blank)
	err := run()
	if err == nil || !strings.Contains(err.Error(), tlsCAFileEnv) {
		t.Fatalf("run() = %v, want a refusal naming %s", err, tlsCAFileEnv)
	}
}

// TestStatusRecorderKeepsTheAnsweredStatus pins the one thing the middleware
// needs from the writer: the status the client was answered with. A handler that
// writes without calling WriteHeader is answered 200 by net/http, and the
// recorder has to report that same code rather than zero.
func TestStatusRecorderKeepsTheAnsweredStatus(t *testing.T) {
	explicit := &statusRecorder{ResponseWriter: httptest.NewRecorder()}
	explicit.WriteHeader(http.StatusConflict)
	if explicit.status != http.StatusConflict {
		t.Errorf("recorded %d, want 409", explicit.status)
	}
	implicit := &statusRecorder{ResponseWriter: httptest.NewRecorder()}
	if _, err := implicit.Write([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	if implicit.status != http.StatusOK {
		t.Errorf("recorded %d, want 200 for a write without WriteHeader", implicit.status)
	}
}
