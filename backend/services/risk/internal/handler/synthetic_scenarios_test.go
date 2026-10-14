package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	v1 "firewatch.local/contracts/events/v1"
	"firewatch.local/risk/internal/source"
)

// TestSyntheticScenariosEndpointServesTheProviderCatalogue protects the
// authenticated catalogue route: it must return the built-in profile list in
// order, generated from the provider catalogue rather than a duplicated UI list,
// and it must answer without touching storage.
func TestSyntheticScenariosEndpointServesTheProviderCatalogue(t *testing.T) {
	for _, role := range []string{"viewer", "admin"} {
		t.Run(role, func(t *testing.T) {
			store := &fakeStore{}
			handler, token := authorizedServer(t, role, store)
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/v1/risk/synthetic-scenarios", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("code=%d body=%s", w.Code, w.Body)
			}
			if w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("headers: %v", w.Header())
			}
			var got []source.SyntheticScenario
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("catalogue is not a JSON array: %v: %s", err, w.Body)
			}
			if want := source.SyntheticScenarios(); !reflect.DeepEqual(got, want) {
				t.Fatalf("catalogue=%+v want=%+v", got, want)
			}
			// The served catalogue and the shared wire enum must agree on the
			// exact ID set and order, so neither can drift from the other.
			ids := make([]string, 0, len(got))
			for _, scenario := range got {
				if scenario.ID == "" || scenario.Description == "" {
					t.Fatalf("scenario entry is incomplete: %+v", scenario)
				}
				ids = append(ids, scenario.ID)
			}
			if !reflect.DeepEqual(ids, v1.SyntheticScenarioIDs()) {
				t.Fatalf("route ids=%v want the shared enum %v", ids, v1.SyntheticScenarioIDs())
			}
			if store.calls != 0 || store.zoneCalls != 0 || store.listZonesCall != 0 {
				t.Fatalf("the catalogue route touched storage: %+v", store)
			}
		})
	}
}

// TestSyntheticScenariosEndpointRequiresReadRole protects the same authorization
// as the zones catalogue: the profile list is viewer/admin-only, and an absent
// token is unauthorized rather than forbidden.
func TestSyntheticScenariosEndpointRequiresReadRole(t *testing.T) {
	handler, token := authorizedServer(t, "operator", &fakeStore{})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/risk/synthetic-scenarios", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("role operator: code=%d want 403: %s", w.Code, w.Body)
	}

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/risk/synthetic-scenarios", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: code=%d want 401: %s", w.Code, w.Body)
	}
}
