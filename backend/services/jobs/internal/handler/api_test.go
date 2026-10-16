package handler

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"firewatch.local/jobs/internal/repository"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeStore struct {
	calls     int
	zones     map[string]bool
	zoneErr   error
	zoneCalls int
	// scenarios records the optional scenario each Create received, so a test can
	// prove the transport forwarded (or withheld) the selection.
	scenarios []string
	// job and getErr let a test drive GET /v1/jobs/{id} through the Repository
	// interface without a database.
	job    repository.Job
	getErr error
	// replayJob, replayErr and the recorded arguments let a test drive
	// POST /v1/jobs/{id}/replay through the Repository interface and prove the
	// transport forwards one call to the single store transition.
	replayJob   repository.Job
	replayErr   error
	replayCalls int
	replayID    string
	replayActor string
	replayTrace string
}

func (s *fakeStore) Create(_ context.Context, actor, key, zone, trace string, scenarioID ...string) (repository.Job, error) {
	s.calls++
	scenario := ""
	if len(scenarioID) > 0 {
		scenario = scenarioID[0]
	}
	s.scenarios = append(s.scenarios, scenario)
	return repository.Job{ID: uuid.NewString(), ZoneID: zone, State: "queued"}, nil
}
func (s *fakeStore) Get(context.Context, string) (repository.Job, error) {
	return s.job, s.getErr
}
func (s *fakeStore) Audit(context.Context) ([]repository.Audit, error) { return nil, nil }

// Replay records the forwarded arguments so a test can pin that the operator
// subject and the request trace are what reach persistence, and returns the
// configured outcome.
func (s *fakeStore) Replay(_ context.Context, id, actor, trace string) (repository.Job, error) {
	s.replayCalls++
	s.replayID, s.replayActor, s.replayTrace = id, actor, trace
	return s.replayJob, s.replayErr
}

// ZoneExists stands in for the zones catalogue so the handler path that decides
// membership is exercised without a database.
func (s *fakeStore) ZoneExists(_ context.Context, code string) (bool, error) {
	s.zoneCalls++
	if s.zoneErr != nil {
		return false, s.zoneErr
	}
	return s.zones[code], nil
}

func TestStatusAuthorization(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, nil)
	verifier := oidc.NewVerifier("https://issuer.test", &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}, &oidc.Config{ClientID: "firewatch-api"})
	for _, tc := range []struct {
		role string
		want int
	}{{"", 401}, {"invalid", 401}, {"operator", 403}, {"viewer", 403}, {"admin", 200}} {
		t.Run(tc.role, func(t *testing.T) {
			calls := 0
			ready := true
			check := func(context.Context) Status {
				calls++
				now := time.Now().UTC()
				depth, age := int64(0), float64(0)
				return Status{CheckedAt: now, Ready: ready, Dependencies: Dependencies{Database: Dependency{"ready"}, Broker: Dependency{"ready"}, DeadLetter: DeadLetterStatus{State: "ready", Depth: &depth, SampledAt: &now, AgeSeconds: &age, MaxAgeSeconds: &age, MaxDepth: &depth}}}
			}
			h := New(&fakeStore{}, verifier, check)
			r := httptest.NewRequest("GET", "/v1/jobs/status", nil)
			if tc.role != "" {
				payload, _ := json.Marshal(map[string]any{"iss": "https://issuer.test", "aud": "firewatch-api", "sub": "user", "exp": time.Now().Add(time.Hour).Unix(), "typ": "Bearer", "realm_access": map[string]any{"roles": []string{tc.role}}})
				signed, _ := signer.Sign(payload)
				raw, _ := signed.CompactSerialize()
				if tc.role == "invalid" {
					raw = "invalid"
				}
				r.Header.Set("Authorization", "Bearer "+raw)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want || (calls == 1) != (tc.want == 200) {
				t.Fatalf("code=%d calls=%d body=%s", w.Code, calls, w.Body)
			}
			if tc.want == 200 {
				var got Status
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got.CheckedAt.IsZero() || got.Dependencies.DeadLetter.Depth == nil || got.Dependencies.DeadLetter.SampledAt == nil || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
					t.Fatalf("ready wire: %s", w.Body)
				}
				ready = false
				w = httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != 503 || !strings.Contains(w.Body.String(), `"ready":false`) {
					t.Fatalf("degraded wire: %s", w.Body)
				}
				w = httptest.NewRecorder()
				New(&fakeStore{}, verifier).ServeHTTP(w, r)
				var s Status
				if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
					t.Fatal(err)
				}
				for _, field := range []string{"depth", "sampled_at", "age_seconds", "max_age_seconds", "max_depth"} {
					if !strings.Contains(w.Body.String(), `"`+field+`":null`) {
						t.Fatalf("missing nullable %s: %s", field, w.Body)
					}
				}
				if w.Code != 503 || s.Reason != "status_unavailable" || s.Dependencies.Database.State != "unknown" || s.Dependencies.DeadLetter.MaxDepth != nil || s.CheckedAt.IsZero() || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
					t.Fatalf("unavailable: %s", w.Body)
				}
			}
		})
	}
}

// TestGetJobExposesStateAndPersistedFailure drives GET /v1/jobs/{id} through the
// Repository interface. It proves the terminal 'failed' state and its persisted
// failure_reason/attempts/failed_at reach the wire, and that the two pre-existing
// states still answer unchanged (the nullable fields stay absent).
func TestGetJobExposesStateAndPersistedFailure(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, nil)
	if err != nil {
		t.Fatal(err)
	}
	verifier := oidc.NewVerifier("https://issuer.test", &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}, &oidc.Config{ClientID: "firewatch-api", SupportedSigningAlgs: []string{"RS256"}})
	instant := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	reason := "permanent_error"
	id := uuid.NewString()
	cases := []struct {
		name              string
		job               repository.Job
		wantState         string
		wantAttempts      int
		wantFailureReason *string
		wantFailedAt      *time.Time
		wantFieldsAbsent  bool
	}{
		{
			name:         "queued legacy job keeps its previous shape",
			job:          repository.Job{ID: id, ZoneID: "valparaiso-demo", State: "queued", RequestedAt: instant},
			wantState:    "queued",
			wantAttempts: 0, wantFieldsAbsent: true,
		},
		{
			name:         "published job keeps its previous shape",
			job:          repository.Job{ID: id, ZoneID: "valparaiso-demo", State: "published", RequestedAt: instant},
			wantState:    "published",
			wantAttempts: 0, wantFieldsAbsent: true,
		},
		{
			name:              "failed job exposes its reason, attempts and instant",
			job:               repository.Job{ID: id, ZoneID: "valparaiso-demo", State: "failed", RequestedAt: instant, Attempts: 5, FailureReason: &reason, FailedAt: &instant},
			wantState:         "failed",
			wantAttempts:      5,
			wantFailureReason: &reason,
			wantFailedAt:      &instant,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, _ := json.Marshal(map[string]any{"iss": "https://issuer.test", "aud": "firewatch-api", "sub": "admin-subject", "exp": time.Now().Add(time.Hour).Unix(), "typ": "Bearer", "realm_access": map[string]any{"roles": []string{"admin"}}})
			signed, err := signer.Sign(payload)
			if err != nil {
				t.Fatal(err)
			}
			token, err := signed.CompactSerialize()
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+id, nil)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			New(&fakeStore{job: tc.job}, verifier).ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("code=%d body=%s", w.Code, w.Body)
			}
			var got struct {
				State         string     `json:"state"`
				Attempts      int        `json:"attempts"`
				FailureReason *string    `json:"failure_reason"`
				FailedAt      *time.Time `json:"failed_at"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.State != tc.wantState || got.Attempts != tc.wantAttempts {
				t.Fatalf("state=%q attempts=%d want %q/%d: %s", got.State, got.Attempts, tc.wantState, tc.wantAttempts, w.Body)
			}
			if (got.FailureReason == nil) != (tc.wantFailureReason == nil) || (got.FailedAt == nil) != (tc.wantFailedAt == nil) {
				t.Fatalf("nullable fields mismatch: %s", w.Body)
			}
			if tc.wantFailureReason != nil && *got.FailureReason != *tc.wantFailureReason {
				t.Fatalf("failure_reason=%q want %q", *got.FailureReason, *tc.wantFailureReason)
			}
			if tc.wantFailedAt != nil && !got.FailedAt.Equal(*tc.wantFailedAt) {
				t.Fatalf("failed_at=%v want %v", got.FailedAt, tc.wantFailedAt)
			}
			if tc.wantFieldsAbsent && (strings.Contains(w.Body.String(), `"failure_reason"`) || strings.Contains(w.Body.String(), `"failed_at"`)) {
				t.Fatalf("non-failed job must omit the terminal fields: %s", w.Body)
			}
		})
	}
}

func TestCreateAuthorizationAndValidation(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, nil)
	if err != nil {
		t.Fatal(err)
	}
	verifier := oidc.NewVerifier("https://issuer.test", &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}, &oidc.Config{ClientID: "firewatch-api", SupportedSigningAlgs: []string{"RS256"}})
	cases := []struct {
		name, role, body, audience, issuer, tokenType string
		expiry, nbf                                   int64
		noToken, badKey                               bool
		zones                                         map[string]bool
		zoneErr                                       error
		want                                          int
	}{
		{name: "admin accepted", role: "admin", want: 202},
		{name: "viewer forbidden", role: "viewer", want: 403},
		{name: "unknown role forbidden", role: "operator", want: 403},
		{name: "missing token", noToken: true, want: 401},
		{name: "wrong audience", role: "admin", audience: "other", want: 401},
		{name: "wrong issuer", role: "admin", issuer: "https://other.test", want: 401},
		{name: "expired token", role: "admin", expiry: time.Now().Add(-time.Minute).Unix(), want: 401},
		{name: "future token", role: "admin", nbf: time.Now().Add(time.Hour).Unix(), want: 401},
		{name: "id token rejected", role: "admin", tokenType: "ID", want: 401},
		{name: "unknown field", role: "admin", body: `{"zone_id":"valparaiso-demo","admin":true}`, want: 400},
		{name: "multiple documents", role: "admin", body: `{"zone_id":"valparaiso-demo"}{}`, want: 400},
		// A zone absent from the catalogue is rejected with the existing 400. The
		// handler asks ZoneExists instead of comparing to a literal, so the
		// accepted set is whatever the catalogue holds.
		{name: "unknown zone", role: "admin", body: `{"zone_id":"other"}`, want: 400},
		// A real commune code (as seeded from the geographic asset) is accepted,
		// proving the hardcoded demo literal is gone and that membership is no
		// longer limited to the legacy demo zone.
		{name: "catalogue commune code accepted", role: "admin", body: `{"zone_id":"05101"}`, zones: map[string]bool{"05101": true}, want: 202},
		// A catalogue failure must surface as storage_unavailable, never as an
		// unknown zone: the two mean different things to a client.
		{name: "catalogue lookup fails", role: "admin", zoneErr: errors.New("storage unavailable"), want: 503},
		{name: "missing idempotency", role: "admin", badKey: true, want: 400},
		{name: "oversized body", role: "admin", body: `{"zone_id":"` + strings.Repeat("a", 9000) + `"}`, want: 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			aud := tc.audience
			if aud == "" {
				aud = "firewatch-api"
			}
			iss := tc.issuer
			if iss == "" {
				iss = "https://issuer.test"
			}
			exp := tc.expiry
			if exp == 0 {
				exp = time.Now().Add(time.Minute).Unix()
			}
			typ := tc.tokenType
			if typ == "" {
				typ = "Bearer"
			}
			claims := map[string]any{"iss": iss, "aud": aud, "sub": "admin-subject", "exp": exp, "nbf": tc.nbf, "typ": typ, "realm_access": map[string]any{"roles": []string{tc.role}}}
			payload, _ := json.Marshal(claims)
			signed, err := signer.Sign(payload)
			if err != nil {
				t.Fatal(err)
			}
			token, err := signed.CompactSerialize()
			if err != nil {
				t.Fatal(err)
			}
			body := tc.body
			if body == "" {
				body = `{"zone_id":"valparaiso-demo"}`
			}
			r := httptest.NewRequest(http.MethodPost, "/v1/jobs", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			if !tc.noToken {
				r.Header.Set("Authorization", "Bearer "+token)
			}
			if !tc.badKey {
				r.Header.Set("Idempotency-Key", uuid.NewString())
			}
			store := &fakeStore{zones: tc.zones, zoneErr: tc.zoneErr}
			if store.zones == nil {
				store.zones = map[string]bool{"valparaiso-demo": true}
			}
			w := httptest.NewRecorder()
			New(store, verifier).ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
			if tc.want != 202 && store.calls != 0 {
				t.Fatal("rejected request reached persistence")
			}
		})
	}
}

// replayHandler builds the transport with the production RS256 policy and a
// signer, so a replay test can mint a token of any role without a remote JWKS.
// The subject is fixed so a test can assert the operator identity the transport
// forwards to the store.
func replayHandler(t *testing.T, store *fakeStore) (http.Handler, func(roles ...string) string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	verifier := oidc.NewVerifier("https://issuer.test", &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}, &oidc.Config{ClientID: "firewatch-api", SupportedSigningAlgs: []string{"RS256"}})
	mint := func(roles ...string) string {
		claims := map[string]any{"iss": "https://issuer.test", "aud": "firewatch-api", "sub": "operator-subject", "exp": time.Now().Add(time.Minute).Unix(), "typ": "Bearer", "realm_access": map[string]any{"roles": roles}}
		return signTokenWith(t, key, jose.RS256, claims)
	}
	return New(store, verifier), mint
}

// TestReplayFailedJobReturnsQueued pins the happy path: a failed job is returned
// to queued through exactly one store call, the response is the same Job object
// as GET with the terminal fields cleared and attempts reset, and the operator
// subject is what the transport forwards.
func TestReplayFailedJobReturnsQueued(t *testing.T) {
	id := uuid.NewString()
	instant := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	store := &fakeStore{replayJob: repository.Job{ID: id, ZoneID: "valparaiso-demo", State: "queued", RequestedAt: instant}}
	handler, mint := replayHandler(t, store)
	trace := uuid.NewString()
	r := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+id+"/replay", nil)
	r.Header.Set("Authorization", "Bearer "+mint("admin"))
	r.Header.Set("X-Request-ID", trace)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("code=%d want=%d body=%s", w.Code, http.StatusAccepted, w.Body)
	}
	var got struct {
		ID            string     `json:"id"`
		State         string     `json:"state"`
		Attempts      int        `json:"attempts"`
		FailureReason *string    `json:"failure_reason"`
		FailedAt      *time.Time `json:"failed_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != id || got.State != "queued" || got.Attempts != 0 {
		t.Fatalf("body=%s", w.Body)
	}
	if got.FailureReason != nil || got.FailedAt != nil {
		t.Fatalf("terminal fields must be cleared: %s", w.Body)
	}
	if store.replayCalls != 1 || store.replayID != id || store.replayActor != "operator-subject" || store.replayTrace != trace {
		t.Fatalf("store call id=%q actor=%q trace=%q calls=%d", store.replayID, store.replayActor, store.replayTrace, store.replayCalls)
	}
	if w.Header().Get("X-Request-ID") != trace {
		t.Fatalf("X-Request-ID header=%q want %q", w.Header().Get("X-Request-ID"), trace)
	}
}

// TestReplayRequiresAdmin pins that the replay route sits behind the existing
// authorization: no token and a non-admin role are refused before the store is
// reached, so replay cannot become a weaker recovery entry point.
func TestReplayRequiresAdmin(t *testing.T) {
	id := uuid.NewString()
	for _, tc := range []struct {
		name   string
		roles  []string
		noAuth bool
		want   int
	}{
		{"missing token", nil, true, http.StatusUnauthorized},
		{"viewer forbidden", []string{"viewer"}, false, http.StatusForbidden},
		{"unknown role forbidden", []string{"operator"}, false, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{}
			handler, mint := replayHandler(t, store)
			r := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+id+"/replay", nil)
			if !tc.noAuth {
				r.Header.Set("Authorization", "Bearer "+mint(tc.roles...))
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("code=%d want=%d body=%s", w.Code, tc.want, w.Body)
			}
			if store.replayCalls != 0 {
				t.Fatal("an unauthorised replay reached the store")
			}
		})
	}
}

// TestReplayRejections pins the failure vocabulary of the new route: an unknown
// id is 404 job_not_found, a job that is not failed is 409 job_not_replayable, a
// storage failure is 503 storage_unavailable, and a malformed id keeps the
// existing 400 invalid_job_id without ever reaching the store. The successful
// store call count proves a rejection produces no partial effect.
func TestReplayRejections(t *testing.T) {
	id := uuid.NewString()
	for _, tc := range []struct {
		name     string
		target   string
		storeErr error
		want     int
		wantCode string
		calls    int
	}{
		{"unknown job", "/v1/jobs/" + id + "/replay", pgx.ErrNoRows, http.StatusNotFound, "job_not_found", 1},
		{"not failed is a conflict", "/v1/jobs/" + id + "/replay", repository.ErrJobNotReplayable, http.StatusConflict, "job_not_replayable", 1},
		{"storage failure", "/v1/jobs/" + id + "/replay", errors.New("storage unavailable"), http.StatusServiceUnavailable, "storage_unavailable", 1},
		{"malformed id", "/v1/jobs/not-a-uuid/replay", nil, http.StatusBadRequest, "invalid_job_id", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{replayErr: tc.storeErr}
			handler, mint := replayHandler(t, store)
			trace := uuid.NewString()
			r := httptest.NewRequest(http.MethodPost, tc.target, nil)
			r.Header.Set("Authorization", "Bearer "+mint("admin"))
			r.Header.Set("X-Request-ID", trace)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("code=%d want=%d body=%s", w.Code, tc.want, w.Body)
			}
			var got struct {
				Code      string `json:"code"`
				RequestID string `json:"request_id"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Code != tc.wantCode || got.RequestID != trace {
				t.Fatalf("code=%q request_id=%q want %q/%q body=%s", got.Code, got.RequestID, tc.wantCode, trace, w.Body)
			}
			if store.replayCalls != tc.calls {
				t.Fatalf("store calls=%d want=%d", store.replayCalls, tc.calls)
			}
		})
	}
}
