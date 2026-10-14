package handler

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1 "firewatch.local/contracts/events/v1"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
)

// scenarioHandler mints an authenticated admin token and returns the transport
// under test. Authentication is reused from the create path on purpose: the
// scenario rule must not open a second, weaker entry point.
func scenarioHandler(t *testing.T, store *fakeStore) (http.Handler, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, nil)
	if err != nil {
		t.Fatal(err)
	}
	verifier := oidc.NewVerifier("https://issuer.test", &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}, &oidc.Config{ClientID: "firewatch-api", SupportedSigningAlgs: []string{"RS256"}})
	payload, _ := json.Marshal(map[string]any{"iss": "https://issuer.test", "aud": "firewatch-api", "sub": "admin-subject", "exp": time.Now().Add(time.Minute).Unix(), "typ": "Bearer", "realm_access": map[string]any{"roles": []string{"admin"}}})
	signed, err := signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	token, err := signed.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return New(store, verifier), token
}

func scenarioRequest(t *testing.T, handler http.Handler, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Idempotency-Key", uuid.NewString())
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

// TestCreateRejectsExplicitlyEmptyOrNullScenario pins the fail-closed request
// contract: an explicit scenario_id of null or "" is a client error rejected
// before any catalogue lookup or persistence, while an omitted field remains
// the legacy scenario-free request. Distinguishing an absent field from an
// explicit empty value is the reason the field cannot be a plain string.
func TestCreateRejectsExplicitlyEmptyOrNullScenario(t *testing.T) {
	store := &fakeStore{zones: map[string]bool{"valparaiso-demo": true}}
	handler, token := scenarioHandler(t, store)

	// Control: the omitted field is still the legacy fixture request, proving
	// the rejections below are not a side effect of a globally stricter path.
	t.Run("omitted field stays legacy", func(t *testing.T) {
		store.calls, store.scenarios, store.zoneCalls = 0, nil, 0
		w := scenarioRequest(t, handler, token, `{"zone_id":"valparaiso-demo"}`)
		if w.Code != 202 {
			t.Fatalf("code=%d body=%s", w.Code, w.Body)
		}
		if store.calls != 1 || store.zoneCalls != 1 || len(store.scenarios) != 1 || store.scenarios[0] != "" {
			t.Fatalf("legacy request changed: calls=%d zoneCalls=%d scenarios=%v", store.calls, store.zoneCalls, store.scenarios)
		}
	})

	for _, tc := range []struct{ name, body string }{
		{"explicit null", `{"zone_id":"valparaiso-demo","scenario_id":null}`},
		{"explicit empty string", `{"zone_id":"valparaiso-demo","scenario_id":""}`},
	} {
		t.Run("rejected "+tc.name, func(t *testing.T) {
			store.calls, store.scenarios, store.zoneCalls = 0, nil, 0
			w := scenarioRequest(t, handler, token, tc.body)
			if w.Code != 400 {
				t.Fatalf("code=%d body=%s", w.Code, w.Body)
			}
			var got map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got["code"] != "invalid_demo_request" {
				t.Fatalf("code=%q body=%s", got["code"], w.Body)
			}
			if store.zoneCalls != 0 || store.calls != 0 || len(store.scenarios) != 0 {
				t.Fatalf("rejected request reached lookup/persistence: calls=%d zoneCalls=%d scenarios=%v", store.calls, store.zoneCalls, store.scenarios)
			}
		})
	}
}

// TestCreateAcceptsOnlyTheWireScenarioEnum pins the selection rule: an optional
// scenario_id is accepted only when it is one of the shared wire vocabulary
// values, the exact value is forwarded to persistence, the client cannot supply
// evaluated_at at all, and a rejected request never reaches persistence.
func TestCreateAcceptsOnlyTheWireScenarioEnum(t *testing.T) {
	store := &fakeStore{zones: map[string]bool{"valparaiso-demo": true}}
	handler, token := scenarioHandler(t, store)
	accepted := []struct{ name, scenario string }{
		// An absent scenario keeps the original fixture path.
		{"legacy request keeps the fixture", ""},
	}
	for _, scenario := range v1.SyntheticScenarioIDs() {
		accepted = append(accepted, struct{ name, scenario string }{"known profile " + scenario, scenario})
	}
	for _, tc := range accepted {
		t.Run("accepted "+tc.name, func(t *testing.T) {
			store.calls, store.scenarios = 0, nil
			body := `{"zone_id":"valparaiso-demo"}`
			if tc.scenario != "" {
				body = `{"zone_id":"valparaiso-demo","scenario_id":"` + tc.scenario + `"}`
			}
			w := scenarioRequest(t, handler, token, body)
			if w.Code != 202 {
				t.Fatalf("code=%d body=%s", w.Code, w.Body)
			}
			if store.calls != 1 || len(store.scenarios) != 1 || store.scenarios[0] != tc.scenario {
				t.Fatalf("persistence received %v after %d calls, want exactly %q", store.scenarios, store.calls, tc.scenario)
			}
		})
	}
	for _, tc := range []struct {
		name string
		body string
	}{
		{"unknown scenario", `{"zone_id":"valparaiso-demo","scenario_id":"volcano"}`},
		{"scenario is not free text", `{"zone_id":"valparaiso-demo","scenario_id":"dry-windy "}`},
		{"scenario id wrong type", `{"zone_id":"valparaiso-demo","scenario_id":5}`},
		{"client evaluated_at is not accepted", `{"zone_id":"valparaiso-demo","scenario_id":"mild","evaluated_at":"2030-01-01T00:00:00Z"}`},
		{"client evaluated_at without scenario is not accepted", `{"zone_id":"valparaiso-demo","evaluated_at":"2030-01-01T00:00:00Z"}`},
	} {
		t.Run("rejected "+tc.name, func(t *testing.T) {
			store.calls, store.scenarios = 0, nil
			w := scenarioRequest(t, handler, token, tc.body)
			if w.Code != 400 {
				t.Fatalf("code=%d body=%s", w.Code, w.Body)
			}
			if store.calls != 0 || len(store.scenarios) != 0 {
				t.Fatal("a rejected request reached persistence")
			}
		})
	}
}
