package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"
)

// allowedOrigin is the single exact origin the gateway is configured with.
const allowedOrigin = "http://localhost:4173"

func newTestGateway(t *testing.T, webOrigin string) http.Handler {
	t.Helper()
	jobs, _ := url.Parse("http://jobs.invalid")
	risk, _ := url.Parse("http://risk.invalid")
	return New(verifier{}, jobs, risk, webOrigin)
}

type verifier struct{}

func (verifier) Verify(_ context.Context, s string) (*oidc.IDToken, error) {
	if s != "valid" {
		return nil, errors.New("invalid")
	}
	return &oidc.IDToken{Subject: "admin"}, nil
}

func TestGatewayFiltersIdentityHeadersAndInternalRoutes(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-User-ID") != "" || r.Header.Get("X-User-Role") != "" || r.Header.Get("X-Forwarded-For") != "" {
			t.Error("untrusted identity forwarded")
		}
		if r.Header.Get("Authorization") != "Bearer valid" {
			t.Error("bearer token lost")
		}
		if r.Header.Get("X-Request-ID") == "spoofed" {
			t.Error("request ID trusted")
		}
		w.WriteHeader(202)
	}))
	defer upstream.Close()
	u, _ := url.Parse(upstream.URL)
	for _, tc := range []struct {
		path, token string
		want        int
	}{{"/v1/jobs", "valid", 202}, {"/v1/jobs", "invalid", 401}, {"/metrics", "valid", 404}, {"/health/live", "", 204}, {"/health/ready", "", 204}} {
		r := httptest.NewRequest("GET", tc.path, nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		r.Header.Set("X-User-ID", "root")
		r.Header.Set("X-User-Role", "admin")
		r.Header.Set("X-Forwarded-For", "127.0.0.1")
		r.Header.Set("X-Request-ID", "spoofed")
		w := httptest.NewRecorder()
		New(verifier{}, u, u, "").ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s got %d want %d", tc.path, w.Code, tc.want)
		}
	}
}

// S10: the preflight must be answered before the rate limiter and before the
// bearer check, with the single configured origin and the agreed methods.
func TestGatewayAnswersPreflightFromConfiguredOrigin(t *testing.T) {
	app := newTestGateway(t, allowedOrigin)

	request := httptest.NewRequest(http.MethodOptions, "/v1/risk/latest?zone_id=valparaiso-demo", nil)
	request.Header.Set("Origin", allowedOrigin)
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)
	request.Header.Set("Access-Control-Request-Headers", "authorization")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("preflight got %d want %d", response.Code, http.StatusNoContent)
	}
	for header, want := range map[string]string{
		"Access-Control-Allow-Origin":  allowedOrigin,
		"Access-Control-Allow-Methods": "GET, POST, OPTIONS",
		"Access-Control-Allow-Headers": "Authorization, Content-Type, Idempotency-Key",
		"Access-Control-Max-Age":       "600",
		"Vary":                         "Origin",
	} {
		if got := response.Header().Get(header); got != want {
			t.Errorf("preflight %s got %q want %q", header, got, want)
		}
	}
}

// headerAllowed reports whether an explicit comma-separated allow-list
// carries the named header. CORS compares header names case-insensitively, so
// the lookup does too.
func headerAllowed(list, name string) bool {
	for _, item := range strings.Split(list, ",") {
		if strings.EqualFold(strings.TrimSpace(item), name) {
			return true
		}
	}
	return false
}

// S1: the page sends Idempotency-Key on the job POST and the jobs service
// answers 400 invalid_idempotency_key without it, so the preflight must
// permit it. The list stays an explicit allow-list: no wildcard and no
// reflection of the headers the caller asked for.
func TestGatewayAllowsIdempotencyKeyOnPreflight(t *testing.T) {
	app := newTestGateway(t, allowedOrigin)

	request := httptest.NewRequest(http.MethodOptions, "/v1/jobs", nil)
	request.Header.Set("Origin", allowedOrigin)
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "authorization, content-type, idempotency-key")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("preflight got %d want %d", response.Code, http.StatusNoContent)
	}
	allowed := response.Header().Get("Access-Control-Allow-Headers")
	if !headerAllowed(allowed, "Idempotency-Key") {
		t.Errorf("preflight Access-Control-Allow-Headers got %q want it to permit Idempotency-Key", allowed)
	}
}

// S1: the allow-list must stay explicit. A header the page never sends must
// not be admitted, and a wildcard must never stand in for the real list.
func TestGatewayKeepsExplicitHeaderAllowListOnPreflight(t *testing.T) {
	app := newTestGateway(t, allowedOrigin)

	request := httptest.NewRequest(http.MethodOptions, "/v1/jobs", nil)
	request.Header.Set("Origin", allowedOrigin)
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "x-invented-header")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("preflight got %d want %d", response.Code, http.StatusNoContent)
	}
	allowed := response.Header().Get("Access-Control-Allow-Headers")
	if strings.Contains(allowed, "*") {
		t.Errorf("preflight Access-Control-Allow-Headers got %q want no wildcard", allowed)
	}
	if headerAllowed(allowed, "x-invented-header") {
		t.Errorf("preflight Access-Control-Allow-Headers got %q want it not to reflect the requested header", allowed)
	}
}

// S9: a different origin must receive no CORS headers at all.
func TestGatewayRejectsPreflightFromUnconfiguredOrigin(t *testing.T) {
	app := newTestGateway(t, allowedOrigin)

	request := httptest.NewRequest(http.MethodOptions, "/v1/risk/latest", nil)
	request.Header.Set("Origin", "http://localhost:9999")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)

	for _, header := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers", "Access-Control-Max-Age", "Vary"} {
		if got := response.Header().Get(header); got != "" {
			t.Errorf("disallowed origin preflight must not carry %s, got %q", header, got)
		}
	}
}

// S9: without configuration CORS stays off even for a matching-looking origin.
func TestGatewayFailsClosedWithoutConfiguredOrigin(t *testing.T) {
	app := newTestGateway(t, "")

	preflight := httptest.NewRequest(http.MethodOptions, "/v1/risk/latest", nil)
	preflight.Header.Set("Origin", allowedOrigin)
	preflightResponse := httptest.NewRecorder()
	app.ServeHTTP(preflightResponse, preflight)
	if got := preflightResponse.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unconfigured gateway must not emit Access-Control-Allow-Origin, got %q", got)
	}

	actual := httptest.NewRequest(http.MethodGet, "/v1/risk/latest", nil)
	actual.Header.Set("Origin", allowedOrigin)
	actual.Header.Set("Authorization", "Bearer valid")
	actualResponse := httptest.NewRecorder()
	app.ServeHTTP(actualResponse, actual)
	if got := actualResponse.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unconfigured gateway must not emit Access-Control-Allow-Origin, got %q", got)
	}
}

// S10: error responses must carry the CORS headers, otherwise the browser hides
// the real status from the frontend.
func TestGatewayCarriesCORSOnErrorResponsesFromConfiguredOrigin(t *testing.T) {
	app := newTestGateway(t, allowedOrigin)

	unauthorized := httptest.NewRecorder()
	unauthorizedRequest := httptest.NewRequest(http.MethodGet, "/v1/risk/latest", nil)
	unauthorizedRequest.Header.Set("Origin", allowedOrigin)
	app.ServeHTTP(unauthorized, unauthorizedRequest)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request got %d want %d", unauthorized.Code, http.StatusUnauthorized)
	}
	if got := unauthorized.Header().Get("Access-Control-Allow-Origin"); got != allowedOrigin {
		t.Errorf("401 response must carry the configured origin, got %q", got)
	}
	if got := unauthorized.Header().Get("Vary"); got != "Origin" {
		t.Errorf("401 response must vary on Origin, got %q", got)
	}

	notFound := httptest.NewRecorder()
	notFoundRequest := httptest.NewRequest(http.MethodGet, "/v1/unknown", nil)
	notFoundRequest.Header.Set("Origin", allowedOrigin)
	notFoundRequest.Header.Set("Authorization", "Bearer valid")
	app.ServeHTTP(notFound, notFoundRequest)
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("unknown route got %d want %d", notFound.Code, http.StatusNotFound)
	}
	if got := notFound.Header().Get("Access-Control-Allow-Origin"); got != allowedOrigin {
		t.Errorf("404 response must carry the configured origin, got %q", got)
	}

	rateLimited := httptest.NewRecorder()
	for burst := 0; burst < 200 && rateLimited.Code != http.StatusTooManyRequests; burst++ {
		request := httptest.NewRequest(http.MethodGet, "/v1/risk/latest", nil)
		request.Header.Set("Origin", allowedOrigin)
		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, request)
		if recorder.Code == http.StatusTooManyRequests {
			rateLimited = recorder
		}
	}
	if rateLimited.Code != http.StatusTooManyRequests {
		t.Fatal("a burst of requests never reached the rate limit")
	}
	if got := rateLimited.Header().Get("Access-Control-Allow-Origin"); got != allowedOrigin {
		t.Errorf("429 response must carry the configured origin, got %q", got)
	}
}

// S10: an upstream failure must also carry the CORS headers.
func TestGatewayCarriesCORSOnUpstreamFailureFromConfiguredOrigin(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	target, _ := url.Parse(upstream.URL)
	upstream.Close()
	app := New(verifier{}, target, target, allowedOrigin)

	request := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	request.Header.Set("Origin", allowedOrigin)
	request.Header.Set("Authorization", "Bearer valid")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("unreachable upstream got %d want %d", response.Code, http.StatusBadGateway)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != allowedOrigin {
		t.Errorf("502 response must carry the configured origin, got %q", got)
	}
}

// S10: the preflight is answered before the rate limiter, so a burst of
// preflights must not consume the budget of the following real request.
func TestGatewayPreflightsDoNotConsumeTheRateLimitBudget(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	app := New(verifier{}, target, target, allowedOrigin)

	for burst := 0; burst < 200; burst++ {
		request := httptest.NewRequest(http.MethodOptions, "/v1/jobs", nil)
		request.Header.Set("Origin", allowedOrigin)
		response := httptest.NewRecorder()
		app.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("preflight %d got %d want %d", burst, response.Code, http.StatusNoContent)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	request.Header.Set("Origin", allowedOrigin)
	request.Header.Set("Authorization", "Bearer valid")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("request after a preflight burst got %d want %d", response.Code, http.StatusAccepted)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != allowedOrigin {
		t.Errorf("actual cross-origin request must carry the configured origin, got %q", got)
	}
}

// S4/S5: the gateway is API-only. The page and its assets are served by the
// frontend app, so every former page route must answer 404.
func TestGatewayDoesNotServeTheRemovedPage(t *testing.T) {
	app := newTestGateway(t, allowedOrigin)

	for _, path := range []string{
		"/",
		"/proposal.css",
		"/proposal.js",
		"/prototipos/wireframe-incendios-directo.html",
	} {
		response := httptest.NewRecorder()
		app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("page route %q got %d want %d", path, response.Code, http.StatusNotFound)
		}
		if !strings.Contains(response.Body.String(), "not_found") {
			t.Errorf("page route %q body got %q want it to contain %q", path, response.Body.String(), "not_found")
		}
	}
}

// Regression: CORS must not weaken the bearer requirement or the health checks.
func TestGatewayKeepsBearerRequiredAndHealthReady(t *testing.T) {
	app := newTestGateway(t, allowedOrigin)

	unauthorized := httptest.NewRecorder()
	app.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/risk/latest", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("/v1/risk/latest without a bearer token got %d want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	ready := httptest.NewRecorder()
	app.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if ready.Code != http.StatusNoContent {
		t.Fatalf("/health/ready got %d want %d", ready.Code, http.StatusNoContent)
	}
}
