package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"firewatch.local/risk/internal/cycle"
	"firewatch.local/risk/internal/repository"
	"firewatch.local/risk/internal/source"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
)

// These tests validate real handler responses against the published contract,
// instead of only checking that the specification parses. They cover the
// service-owned error bodies the gateway cannot reach without this service: the
// dependency-unavailable 503 and the empty-state 404.

// contractPath locates the published contract from this package's working dir.
var contractPath = filepath.Join("..", "..", "..", "..", "contracts", "openapi", "firewatch-v1.yaml")

func loadContract(t *testing.T) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	contract, err := loader.LoadFromFile(contractPath)
	if err != nil {
		t.Fatalf("load the published contract: %v", err)
	}
	if err := contract.Validate(context.Background()); err != nil {
		t.Fatalf("the published contract is not valid: %v", err)
	}
	return contract
}

// withCorrelationID mirrors the production middleware in cmd/server/main.go: it
// parses the forwarded correlation id, rewrites the response header with the
// canonical dashed form and forwards the same value on the request so the
// handler error bodies echo one literal. It is reproduced here because that
// middleware lives in package main and cannot be imported; the id's origin is
// what it fakes, and the request-to-body-to-header flow it enables is what the
// assertions below pin.
func withCorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.Header.Get("X-Request-ID"))
		if err != nil {
			id = uuid.New()
		}
		w.Header().Set("X-Request-ID", id.String())
		r.Header.Set("X-Request-ID", id.String())
		next.ServeHTTP(w, r)
	})
}

// assertResponseMatchesContract fails when the real response is not the one the
// contract documents: the status must be declared for the operation, every
// header the contract declares must be present, and the JSON body must validate
// against the declared schema.
func assertResponseMatchesContract(t *testing.T, contract *openapi3.T, target, method string, response *httptest.ResponseRecorder) {
	t.Helper()
	item := contract.Paths.Find(target)
	if item == nil {
		t.Fatalf("contract documents no path %q", target)
	}
	operation := item.GetOperation(method)
	if operation == nil {
		t.Fatalf("contract documents no %s operation for %q", method, target)
	}
	documented := operation.Responses.Status(response.Code)
	if documented == nil || documented.Value == nil {
		t.Fatalf("contract documents no %d response for %s %s", response.Code, method, target)
	}
	for name := range documented.Value.Headers {
		if response.Header().Get(name) == "" {
			t.Errorf("%s %s %d is missing the documented header %q", method, target, response.Code, name)
		}
	}
	media := documented.Value.Content.Get("application/json")
	if media == nil || media.Schema == nil || media.Schema.Value == nil {
		t.Fatalf("contract documents no application/json body for %s %s %d", method, target, response.Code)
	}
	var body any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body is not JSON: %v: %s", err, response.Body.String())
	}
	if err := media.Schema.Value.VisitJSON(body); err != nil {
		t.Fatalf("response body does not match the contract for %s %s %d: %v\nbody: %s", method, target, response.Code, err, response.Body.String())
	}
}

func request(t *testing.T, handler http.Handler, token, target string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Request-ID", uuid.NewString())
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

// TestRiskContractNewSurfacesMatchTheContract validates the responses added for
// the cycle and the model registry against the published schema, so the document
// cannot drift from what the handlers emit.
func TestRiskContractNewSurfacesMatchTheContract(t *testing.T) {
	contract := loadContract(t)
	hash := "59259c4c0c470e5a0d512fe09f6c748c38fd29f9bc5d760d49754555c66e699b"
	approved := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	age := 30.0
	last, next := approved, approved.Add(15*time.Minute)
	store := &fakeStore{models: []repository.ModelVersion{
		{Version: "academic-rule-v1", Kind: "rule", ParamsHash: &hash, ApprovedAt: &approved, Active: true, Metrics: json.RawMessage(`{}`)},
		{Version: "demo-fixture-v1", Kind: "placeholder", Active: false, Metrics: json.RawMessage(`{}`)},
	}}
	runner := &fakeCycle{
		statuses: []cycle.SourceStatus{{SourceID: "fixture-sample", LastRunAt: &last, NextRunAt: &next, Complete: true, AgeSeconds: &age}},
		result: cycle.RunResult{
			SourceID:   "fixture-sample",
			StartedAt:  approved,
			FinishedAt: approved,
			Complete:   true,
			Zones:      []cycle.ZoneResult{{ZoneID: "valparaiso-demo", Published: true}},
		},
	}
	base, token := authorizedServerWith(t, "admin", store, runner, describedSource{source.NewFixtureProvider().Describe()})
	app := withCorrelationID(base)

	for _, target := range []string{"/v1/models", "/v1/risk/sources"} {
		w := request(t, app, token, target)
		if w.Code != http.StatusOK {
			t.Fatalf("%s got %d: %s", target, w.Code, w.Body.String())
		}
		assertResponseMatchesContract(t, contract, target, http.MethodGet, w)
	}

	r := httptest.NewRequest(http.MethodPost, "/v1/risk/cycle/run", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Request-ID", uuid.NewString())
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("cycle run got %d: %s", w.Code, w.Body.String())
	}
	assertResponseMatchesContract(t, contract, "/v1/risk/cycle/run", http.MethodPost, w)
}

// S3: "dependencia no disponible" — a storage failure is answered 503 with the
// documented shared error body, and its request_id is the same literal as the
// header the page reads.
func TestRiskContractDependencyUnavailableResponse(t *testing.T) {
	contract := loadContract(t)
	base, token := authorizedServer(t, "viewer", &fakeStore{zoneErr: errors.New("storage unavailable")})
	app := withCorrelationID(base)

	w := request(t, app, token, "/v1/risk/latest?zone_id=valparaiso-demo")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d want %d: %s", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}
	assertResponseMatchesContract(t, contract, "/v1/risk/latest", http.MethodGet, w)

	var body struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.RequestID != w.Header().Get("X-Request-ID") {
		t.Errorf("body request_id %q and header %q must be the same literal", body.RequestID, w.Header().Get("X-Request-ID"))
	}
}

// S3: the empty-state 404 is deliberately not the shared error body: it carries
// code and data_status and no request_id. The contract documents that shape, and
// this test keeps the two from being conflated.
func TestRiskContractEmptyStateResponse(t *testing.T) {
	contract := loadContract(t)
	base, token := authorizedServer(t, "viewer", &fakeStore{zones: map[string]bool{"valparaiso-demo": true}})
	app := withCorrelationID(base)

	w := request(t, app, token, "/v1/risk/latest?zone_id=valparaiso-demo")
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d want %d: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
	assertResponseMatchesContract(t, contract, "/v1/risk/latest", http.MethodGet, w)
}
