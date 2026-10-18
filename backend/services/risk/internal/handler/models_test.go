package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"firewatch.local/risk/internal/repository"
)

// T6: GET /v1/models is a read route (viewer or admin) that lists the registry
// rows with their lifecycle state, so the UI can show the active version.
func TestModelsListRequiresReadRoleAndReturnsTheRegistry(t *testing.T) {
	approved := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	hash := "59259c4c0c470e5a0d512fe09f6c748c38fd29f9bc5d760d49754555c66e699b"
	store := &fakeStore{models: []repository.ModelVersion{
		{Version: "academic-rule-v1", Kind: "rule", ParamsHash: &hash, Active: true, Metrics: json.RawMessage(`{}`)},
		{Version: "firewatch-rule-v2", Kind: "rule", ParamsHash: &hash, Active: false, ApprovedAt: &approved, Metrics: json.RawMessage(`{}`)},
	}}
	for _, tc := range []struct {
		name, role string
		want       int
	}{
		{"viewer", "viewer", 200},
		{"admin", "admin", 200},
		{"other", "operator", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler, token := authorizedServer(t, tc.role, store)
			w := getZones(t, handler, token, "/v1/models")
			if w.Code != tc.want {
				t.Fatalf("got %d want %d: %s", w.Code, tc.want, w.Body.String())
			}
			if tc.want != 200 {
				return
			}
			var body struct {
				Models []repository.ModelVersion `json:"models"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body.Models) != 2 || body.Models[0].Version != "academic-rule-v1" || !body.Models[0].Active {
				t.Fatalf("list body = %+v", body.Models)
			}
		})
	}
}

// T6: activation is admin-only. A viewer must be refused without touching the
// registry, and the body must be the shared error shape.
func TestModelActivationRequiresAdmin(t *testing.T) {
	store := &fakeStore{}
	handler, token := authorizedServer(t, "viewer", store)
	r := httptest.NewRequest(http.MethodPost, "/v1/models/firewatch-rule-v2/activate", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("got %d want 403: %s", w.Code, w.Body.String())
	}
	if len(store.activated) != 0 {
		t.Fatalf("a denied activation reached the registry: %v", store.activated)
	}
	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["code"] != "forbidden" {
		t.Fatalf("error body = %v", body)
	}
}

// T6: an admin activates a version transactionally. The returned row reports the
// new state, and the version named in the path is the one the repository is
// asked to activate.
func TestModelActivationSucceedsForAdminAndReactivationIsAllowed(t *testing.T) {
	store := &fakeStore{}
	handler, token := authorizedServer(t, "admin", store)
	for _, version := range []string{"firewatch-rule-v2", "academic-rule-v1"} {
		r := httptest.NewRequest(http.MethodPost, "/v1/models/"+version+"/activate", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("activate %s: got %d: %s", version, w.Code, w.Body.String())
		}
		var model repository.ModelVersion
		if err := json.Unmarshal(w.Body.Bytes(), &model); err != nil {
			t.Fatal(err)
		}
		if model.Version != version || !model.Active {
			t.Fatalf("activation response = %+v, want %s active", model, version)
		}
		if model.ApprovedAt == nil {
			t.Fatal("activation did not set approved_at")
		}
	}
	// Reversion is activating the previous version, which the second call above
	// already exercised. The repository recorded both in order.
	if strings.Join(store.activated, ",") != "firewatch-rule-v2,academic-rule-v1" {
		t.Fatalf("activated = %v, want both versions in order", store.activated)
	}
}

// T6: an unknown version is a 404 and a placeholder that the schema forbids from
// being active is a 409. A storage failure stays a 503.
func TestModelActivationFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		store   *fakeStore
		version string
		want    int
		code    string
	}{
		{"unknown version", &fakeStore{activateErr: repository.ErrModelNotFound}, "no-existe", 404, "model_not_found"},
		{"placeholder", &fakeStore{activateErr: repository.ErrModelNotActivatable}, "demo-fixture-v1", 409, "model_not_activatable"},
		{"storage failure", &fakeStore{activateErr: errors.New("db down")}, "firewatch-rule-v2", 503, "storage_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler, token := authorizedServer(t, "admin", tc.store)
			r := httptest.NewRequest(http.MethodPost, "/v1/models/"+tc.version+"/activate", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d want %d: %s", w.Code, tc.want, w.Body.String())
			}
			var body map[string]string
			_ = json.Unmarshal(w.Body.Bytes(), &body)
			if body["code"] != tc.code {
				t.Fatalf("code = %q, want %q", body["code"], tc.code)
			}
		})
	}
}

// T6: a list failure is a 503, not an empty success: an empty registry and an
// unreachable registry mean different things.
func TestModelsListSurfacesAStorageFailure(t *testing.T) {
	store := &fakeStore{modelsErr: errors.New("db down")}
	handler, token := authorizedServer(t, "viewer", store)
	w := getZones(t, handler, token, "/v1/models")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d want 503: %s", w.Code, w.Body.String())
	}
}
