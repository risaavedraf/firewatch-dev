package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"firewatch.local/inference/internal/scorer"
	"github.com/getkin/kin-openapi/openapi3"
)

// contractPath locates the published document from this package's working
// directory. The path is the same inside the Docker test stage, which copies
// contracts/openapi to /src/contracts/openapi.
var contractPath = filepath.Join("..", "..", "..", "..", "contracts", "openapi", "inference-v1.yaml")

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

// assertResponseMatchesContract fails when a real response is not the one the
// document describes: the status must be declared for the operation, every
// declared header must be present, and a JSON body must validate against the
// declared schema. This is what keeps the document from drifting from what the
// handlers emit.
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
		// /metrics is documented as Prometheus text; there is no JSON body to
		// validate and the status/header checks above are the whole contract.
		if documented.Value.Content.Get("text/plain") == nil {
			t.Fatalf("contract documents no body for %s %s %d", method, target, response.Code)
		}
		return
	}
	var body any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body is not JSON: %v: %s", err, response.Body.String())
	}
	if err := media.Schema.Value.VisitJSON(body); err != nil {
		t.Fatalf("response body does not match the contract for %s %s %d: %v\nbody: %s", method, target, response.Code, err, response.Body.String())
	}
}

// TestPredictContract covers both documented /predict outcomes: the answer and
// every refusal code.
func TestPredictContract(t *testing.T) {
	contract := loadContract(t)
	app, _ := server(t, "logistic.json")

	response := post(t, app, `{"features": {`+allFeatures+`}}`)
	if response.Code != http.StatusOK {
		t.Fatalf("got %d: %s", response.Code, response.Body.String())
	}
	assertResponseMatchesContract(t, contract, "/predict", http.MethodPost, response)

	for _, body := range []string{
		`{"features": {"temp_max": 30}}`,
		`{"features": {` + allFeatures + `, "pressure_atm": 1013}}`,
		`{"features": {"temp_max": 999, "temp_min": 18.4, "humidity_rel": 22, "wind_speed": 9.5, "wind_dir": 195, "precip_sum": 0, "radiation_sol": 1680}}`,
		`{"features": {"temp_max": 34.2, "temp_max": 30, "temp_min": 18.4, "humidity_rel": 22, "wind_speed": 9.5, "wind_dir": 195, "precip_sum": 0, "radiation_sol": 1680}}`,
	} {
		response := post(t, app, body)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body %s got %d: %s", body, response.Code, response.Body.String())
		}
		assertResponseMatchesContract(t, contract, "/predict", http.MethodPost, response)
	}

	// The one 500 the document declares, reached through a fake model: a real
	// artifact cannot produce it, and it must still be declared.
	predictor := &fakePredictor{err: fmt.Errorf("%w: got NaN", scorer.ErrNonFiniteProbability)}
	failing := New(loadModel(t, "logistic.json"), predictor, &fakeCounter{})
	response = post(t, failing, `{"features": {`+allFeatures+`}}`)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("got %d: %s", response.Code, response.Body.String())
	}
	assertResponseMatchesContract(t, contract, "/predict", http.MethodPost, response)
}

// TestReadContractSurfaces covers the documented GET routes.
func TestReadContractSurfaces(t *testing.T) {
	contract := loadContract(t)
	app, _ := server(t, "logistic.json")
	for _, target := range []string{"/health", "/v1/model"} {
		response := get(app, target)
		if response.Code != http.StatusOK {
			t.Fatalf("%s got %d: %s", target, response.Code, response.Body.String())
		}
		assertResponseMatchesContract(t, contract, target, http.MethodGet, response)
	}
}

// TestEveryDocumentedRefusalCodeIsReachable keeps the enum in the document honest:
// each documented code must be producible by this handler, so the contract cannot
// list a code nothing emits or omit one that is emitted.
func TestEveryDocumentedRefusalCodeIsReachable(t *testing.T) {
	contract := loadContract(t)
	schema := contract.Components.Schemas["ErrorCode"]
	if schema == nil || schema.Value == nil || len(schema.Value.Enum) == 0 {
		t.Fatal("the contract declares no ErrorCode enum")
	}
	documented := make(map[string]bool, len(schema.Value.Enum))
	for _, value := range schema.Value.Enum {
		documented[value.(string)] = true
	}
	emitted := map[string]bool{
		"invalid_body":           true,
		"duplicate_feature":      true,
		"missing_feature":        true,
		"unknown_feature":        true,
		"out_of_range":           true,
		"non_finite_probability": true,
		"scoring_failed":         true,
	}
	for code := range emitted {
		if !documented[code] {
			t.Errorf("the handler emits %q, the contract does not document it", code)
		}
	}
	for code := range documented {
		if !emitted[code] {
			t.Errorf("the contract documents %q, the handler cannot emit it", code)
		}
	}
}
