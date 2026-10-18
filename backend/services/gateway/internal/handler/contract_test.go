package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// The published contract is the single source of truth for this API surface, so
// these tests validate real gateway responses against it instead of restating
// the expected shape by hand. They deliberately exercise the error paths only
// the gateway owns: the bearer gate, the global rate limiter and the
// upstream-failure handler. Service-owned bodies (503 storage, 400 validation)
// are validated by the service handler contract tests.
//
// The contract is generated nowhere: contracts/openapi/firewatch-v1.yaml is
// edited by hand and consumed here, so a response that stops matching it fails
// this test rather than drifting silently.

// contractPath locates the published contract from this package's working dir.
var contractPath = filepath.Join("..", "..", "..", "..", "contracts", "openapi", "firewatch-v1.yaml")

// loadContract loads and validates the published contract.
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

// S3: the bearer gate is answered by the gateway and its body must be the one
// the contract documents for the operation.
func TestGatewayContractUnauthorizedResponse(t *testing.T) {
	contract := loadContract(t)
	app := newTestGateway(t, allowedOrigin)

	request := httptest.NewRequest(http.MethodGet, "/v1/risk/latest?zone_id=valparaiso-demo", nil)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("got %d want %d", response.Code, http.StatusUnauthorized)
	}
	assertResponseMatchesContract(t, contract, "/v1/risk/latest", http.MethodGet, response)
}

// S3: "fallo upstream" — an unreachable upstream service is answered 502 by the
// gateway, and that body must be the documented UpstreamUnavailable response.
func TestGatewayContractUpstreamUnavailableResponse(t *testing.T) {
	contract := loadContract(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	target, _ := url.Parse(upstream.URL)
	upstream.Close()
	app := New(verifier{}, target, target, target, allowedOrigin)

	request := httptest.NewRequest(http.MethodGet, "/v1/risk/latest?zone_id=valparaiso-demo", nil)
	request.Header.Set("Authorization", "Bearer valid")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway {
		t.Fatalf("got %d want %d", response.Code, http.StatusBadGateway)
	}
	assertResponseMatchesContract(t, contract, "/v1/risk/latest", http.MethodGet, response)
}

// S3: "rate limit" — the global limiter answers 429 before routing. The
// documented response carries Retry-After, so the real one must too.
func TestGatewayContractRateLimitedResponse(t *testing.T) {
	contract := loadContract(t)
	app := newTestGateway(t, allowedOrigin)

	var limited *httptest.ResponseRecorder
	// No bearer is sent: the limiter runs before the bearer check, so every
	// request drains the same budget until the limiter answers 429.
	for burst := 0; burst < 200 && limited == nil; burst++ {
		request := httptest.NewRequest(http.MethodGet, "/v1/risk/latest?zone_id=valparaiso-demo", nil)
		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, request)
		if recorder.Code == http.StatusTooManyRequests {
			limited = recorder
		}
	}
	if limited == nil {
		t.Fatal("a burst of requests never reached the rate limit")
	}
	if got := limited.Header().Get("Retry-After"); got == "" {
		t.Error("429 response must carry Retry-After")
	}
	assertResponseMatchesContract(t, contract, "/v1/risk/latest", http.MethodGet, limited)
}
