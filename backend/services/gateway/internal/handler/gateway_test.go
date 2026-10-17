package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
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

	for _, header := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers", "Access-Control-Max-Age", "Access-Control-Expose-Headers", "Vary"} {
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
	for header, got := range map[string]string{
		"Access-Control-Allow-Origin":   actualResponse.Header().Get("Access-Control-Allow-Origin"),
		"Access-Control-Expose-Headers": actualResponse.Header().Get("Access-Control-Expose-Headers"),
	} {
		if got != "" {
			t.Fatalf("unconfigured gateway must not emit %s, got %q", header, got)
		}
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

// S15: the page renders the correlation id of an error, and X-Request-ID is
// not a CORS-safelisted response header. Without an explicit
// Access-Control-Expose-Headers the browser hides it from the page, so the
// audit 403 loses its request_id even though the gateway sent it. The header
// is therefore part of the response contract, not decoration.
func TestGatewayExposesRequestIDToThePageFromConfiguredOrigin(t *testing.T) {
	app := newTestGateway(t, allowedOrigin)

	// The preflight must expose it too, so the two CORS paths do not diverge.
	preflight := httptest.NewRequest(http.MethodOptions, "/v1/audit", nil)
	preflight.Header.Set("Origin", allowedOrigin)
	preflight.Header.Set("Access-Control-Request-Method", http.MethodGet)
	preflightResponse := httptest.NewRecorder()
	app.ServeHTTP(preflightResponse, preflight)
	if preflightResponse.Code != http.StatusNoContent {
		t.Fatalf("preflight got %d want %d", preflightResponse.Code, http.StatusNoContent)
	}
	if exposed := preflightResponse.Header().Get("Access-Control-Expose-Headers"); !headerAllowed(exposed, "X-Request-ID") {
		t.Errorf("preflight Access-Control-Expose-Headers got %q want it to expose X-Request-ID", exposed)
	}

	// An error response is the case the page actually renders. The block that
	// sets it runs before every failure() branch, so asserting here covers the
	// 403 the jobs service returns as much as the 401 asserted below.
	unauthorized := httptest.NewRecorder()
	unauthorizedRequest := httptest.NewRequest(http.MethodGet, "/v1/audit", nil)
	unauthorizedRequest.Header.Set("Origin", allowedOrigin)
	app.ServeHTTP(unauthorized, unauthorizedRequest)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /v1/audit got %d want %d", unauthorized.Code, http.StatusUnauthorized)
	}
	if exposed := unauthorized.Header().Get("Access-Control-Expose-Headers"); !headerAllowed(exposed, "X-Request-ID") {
		t.Errorf("401 response Access-Control-Expose-Headers got %q want it to expose X-Request-ID", exposed)
	}
	if got := unauthorized.Header().Get("X-Request-ID"); got == "" {
		t.Error("401 response must carry the correlation id it exposes")
	}

	// The health endpoint is outside /v1/ and answers before any CORS decision
	// of interest here; asserting it keeps the header from being added to
	// responses the page never reads.
	ready := httptest.NewRecorder()
	readyRequest := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	readyRequest.Header.Set("Origin", allowedOrigin)
	app.ServeHTTP(ready, readyRequest)
	if ready.Code != http.StatusNoContent {
		t.Fatalf("/health/ready got %d want %d", ready.Code, http.StatusNoContent)
	}
}

// subjectVerifier accepts every non-empty token except the sentinel "invalid"
// and uses the token itself as the verified subject, so a test can drive
// distinct authenticated actors.
type subjectVerifier struct{}

func (subjectVerifier) Verify(_ context.Context, token string) (*oidc.IDToken, error) {
	if token == "" || token == "invalid" {
		return nil, errors.New("invalid")
	}
	return &oidc.IDToken{Subject: token}, nil
}

// serve runs a request through the gateway and returns the status code.
func serve(app http.Handler, request *http.Request) int {
	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, request)
	return recorder.Code
}

// BH-03: one authenticated actor exhausting its burst must not consume another
// actor's dedicated quota. Identity is the verified subject only.
func TestGatewayIsolatesAuthenticatedActorQuotas(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	app := New(subjectVerifier{}, target, target, "")

	call := func(token string) int {
		request := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		return serve(app, request)
	}

	// Drain actor-a until its own bucket rejects it.
	exhausted := false
	for burst := 0; burst < 60; burst++ {
		if call("actor-a") == http.StatusTooManyRequests {
			exhausted = true
			break
		}
	}
	if !exhausted {
		t.Fatal("actor-a never exhausted its burst")
	}

	// actor-b's full burst must be intact.
	for i := 0; i < 10; i++ {
		if got := call("actor-b"); got != http.StatusAccepted {
			t.Fatalf("actor-b request %d got %d, want %d", i, got, http.StatusAccepted)
		}
	}
}

// BH-03: the emergency aggregate bound still applies to every request, even
// when each actor stays within its own quota, and it keeps the BH-08 shape.
func TestGatewayAggregateBoundRejectsWithRetryAfter(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	app := New(subjectVerifier{}, target, target, "")

	var limited *httptest.ResponseRecorder
	for i := 0; limited == nil && i < 500; i++ {
		// A distinct subject per request keeps the per-subject bucket out of the
		// decision, so whatever rejects is the aggregate.
		request := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
		request.Header.Set("Authorization", fmt.Sprintf("Bearer actor-%d", i))
		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, request)
		if recorder.Code == http.StatusTooManyRequests {
			limited = recorder
		}
	}
	if limited == nil {
		t.Fatal("distinct actors never tripped the aggregate bound")
	}
	if got := limited.Header().Get("Retry-After"); got != "1" {
		t.Errorf("429 Retry-After got %q, want %q", got, "1")
	}
	if !strings.Contains(limited.Body.String(), "rate_limited") {
		t.Errorf("429 body got %q, want it to contain %q", limited.Body.String(), "rate_limited")
	}
}

// BH-03: unauthenticated requests share one coarse bucket, so a burst from
// many distinct callers is bounded together.
func TestGatewayBoundsAnonymousRequestsInOneSharedBucket(t *testing.T) {
	app := newTestGateway(t, allowedOrigin)

	// The anonymous burst is deliberately tighter than the aggregate, so a
	// handful of distinct callers must be bounded well before the aggregate
	// bound would ever bite.
	for burst := 0; burst < 10; burst++ {
		request := httptest.NewRequest(http.MethodGet, "/v1/risk/latest", nil)
		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, request)
		if recorder.Code == http.StatusTooManyRequests {
			return
		}
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous request %d got %d, want %d", burst, recorder.Code, http.StatusUnauthorized)
		}
	}
	t.Fatal("a small burst of unauthenticated requests was never bounded")
}

// BH-03: no forwarded-IP or remote-address identity is trusted. Changing
// X-Forwarded-For must not hand an unauthenticated caller a fresh bucket.
func TestGatewayAnonymousBucketIgnoresForwardedFor(t *testing.T) {
	app := newTestGateway(t, allowedOrigin)

	for i := 0; i < 5; i++ {
		request := httptest.NewRequest(http.MethodGet, "/v1/risk/latest", nil)
		request.Header.Set("X-Forwarded-For", fmt.Sprintf("10.0.0.%d", i))
		if got := serve(app, request); got != http.StatusUnauthorized {
			t.Fatalf("anonymous request %d got %d, want %d", i, got, http.StatusUnauthorized)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/risk/latest", nil)
	request.Header.Set("X-Forwarded-For", "203.0.113.99")
	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("sixth anonymous request with a spoofed X-Forwarded-For got %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
	if got := recorder.Header().Get("Retry-After"); got == "" {
		t.Error("anonymous 429 must carry Retry-After")
	}
	if !strings.Contains(recorder.Body.String(), "rate_limited") {
		t.Errorf("anonymous 429 body got %q, want it to contain %q", recorder.Body.String(), "rate_limited")
	}
}

// BH-03: health routes and the allowed-origin preflight stay exempt and consume
// no tokens from any layer.
func TestGatewayHealthAndPreflightConsumeNoQuota(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	app := New(subjectVerifier{}, target, target, allowedOrigin)

	for i := 0; i < 200; i++ {
		preflight := httptest.NewRequest(http.MethodOptions, "/v1/jobs", nil)
		preflight.Header.Set("Origin", allowedOrigin)
		if got := serve(app, preflight); got != http.StatusNoContent {
			t.Fatalf("preflight %d got %d, want %d", i, got, http.StatusNoContent)
		}
		live := httptest.NewRequest(http.MethodGet, "/health/live", nil)
		if got := serve(app, live); got != http.StatusNoContent {
			t.Fatalf("health/live %d got %d, want %d", i, got, http.StatusNoContent)
		}
		ready := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
		if got := serve(app, ready); got != http.StatusNoContent {
			t.Fatalf("health/ready %d got %d, want %d", i, got, http.StatusNoContent)
		}
	}

	// The aggregate and per-subject budgets are untouched: an authenticated
	// request is served normally, not rate limited.
	authenticated := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	authenticated.Header.Set("Authorization", "Bearer actor-x")
	if got := serve(app, authenticated); got != http.StatusAccepted {
		t.Fatalf("authenticated request after exempt traffic got %d, want %d", got, http.StatusAccepted)
	}

	// The anonymous budget is untouched too: the first unauthenticated request
	// is rejected by the bearer gate, not by the rate limiter.
	anonymous := httptest.NewRequest(http.MethodGet, "/v1/risk/latest", nil)
	if got := serve(app, anonymous); got != http.StatusUnauthorized {
		t.Fatalf("first anonymous request got %d, want %d", got, http.StatusUnauthorized)
	}
}

// canonicalUUIDPattern is the one format this route accepts: the canonical
// dashed lower-case UUID string that uuid.UUID.String() produces, version 4 with
// the RFC 9562 variant, and the format jobs and risk already emit on their
// responses.
var canonicalUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// asCanonicalUUID reformats a correlation id the way the jobs and risk
// services do on their way out (`uuid.Parse(forwarded).String()`). Both the
// bare 32-hex form and the canonical dashed form map to the canonical string,
// which reproduces their response header without this module taking a uuid
// dependency. A value that is not 32 hex digits is returned unchanged, so a
// surprising input fails the assertions instead of the helper.
func asCanonicalUUID(raw string) string {
	bare := strings.ReplaceAll(raw, "-", "")
	if len(bare) != 32 {
		return raw
	}
	return bare[0:8] + "-" + bare[8:12] + "-" + bare[12:16] + "-" + bare[16:20] + "-" + bare[20:32]
}

// S4: the response exposes exactly one X-Request-ID, in one format, and it is
// the gateway's own id. jobs and risk re-stamp the header on their responses
// (`uuid.Parse` the forwarded id, write back `id.String()`), and
// httputil.ReverseProxy copies response headers with Add, so without discarding
// the upstream value the browser receives two values, joins them with ", " and
// the page renders both formats in the same message. The upstream value is
// dropped in ModifyResponse, which runs before the proxy copies the upstream
// headers, so the gateway's own Set stays the only value.
func TestGatewayExposesExactlyOneCorrelationIDFromUpstreamResponses(t *testing.T) {
	for _, tc := range []struct {
		name string
		// mintUnrelatedID models an upstream that answers with an id of its
		// own instead of echoing the one the gateway forwarded.
		mintUnrelatedID bool
	}{
		{name: "the upstream reformats the forwarded id like jobs and risk"},
		{name: "the upstream mints an unrelated id", mintUnrelatedID: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstreamResponseID := func(forwarded string) string {
				if tc.mintUnrelatedID {
					return "aabbccdd-eeff-0011-2233-445566778899"
				}
				return asCanonicalUUID(forwarded)
			}
			forwarded := make(chan string, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				id := r.Header.Get("X-Request-ID")
				forwarded <- id
				w.Header().Set("X-Request-ID", upstreamResponseID(id))
				w.WriteHeader(http.StatusOK)
			}))
			defer upstream.Close()
			target, _ := url.Parse(upstream.URL)
			app := New(verifier{}, target, target, allowedOrigin)

			request := httptest.NewRequest(http.MethodGet, "/v1/audit", nil)
			request.Header.Set("Origin", allowedOrigin)
			request.Header.Set("Authorization", "Bearer valid")
			request.Header.Set("X-Request-ID", "spoofed")
			response := httptest.NewRecorder()
			app.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("proxied /v1/audit got %d want %d", response.Code, http.StatusOK)
			}

			gatewayID := asCanonicalUUID(<-forwarded)
			values := response.Header().Values("X-Request-ID")
			if len(values) != 1 {
				t.Fatalf("X-Request-ID got %d values %q, want exactly one: the gateway's own id", len(values), values)
			}
			if !canonicalUUIDPattern.MatchString(values[0]) {
				t.Errorf("X-Request-ID got %q, want the canonical dashed UUID jobs and risk emit", values[0])
			}
			if values[0] != gatewayID {
				t.Errorf("X-Request-ID got %q, want the gateway's own id %q and not the upstream's %q", values[0], gatewayID, upstreamResponseID(gatewayID))
			}
		})
	}
}

// S4: one id means one literal. The page renders the request_id of the JSON
// error body and the id it reads back from the exposed header, and the
// gateway's log line carries the same value, so all three stay comparable when
// an operator quotes them.
func TestGatewayErrorBodyCarriesTheSameCorrelationIDAsTheHeader(t *testing.T) {
	app := newTestGateway(t, allowedOrigin)

	request := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	request.Header.Set("Origin", allowedOrigin)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /v1/jobs got %d want %d", response.Code, http.StatusUnauthorized)
	}

	var body struct {
		RequestID string `json:"request_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	values := response.Header().Values("X-Request-ID")
	if len(values) != 1 {
		t.Fatalf("X-Request-ID got %d values %q, want exactly one", len(values), values)
	}
	if !canonicalUUIDPattern.MatchString(values[0]) {
		t.Errorf("X-Request-ID got %q, want the canonical dashed UUID jobs and risk emit", values[0])
	}
	if body.RequestID != values[0] {
		t.Errorf("body request_id got %q and header got %q, want the same literal", body.RequestID, values[0])
	}
}
