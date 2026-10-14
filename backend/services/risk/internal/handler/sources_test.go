package handler

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"testing"

	"firewatch.local/contracts/geo"
	observations "firewatch.local/contracts/observations/v1"
	"firewatch.local/risk/internal/source"
)

func TestEmbeddedProvenanceMovePreservedBytes(t *testing.T) {
	if len(geo.ComunasProvenanceJSON) != 2023 || fmt.Sprintf("%x", sha256.Sum256(geo.ComunasProvenanceJSON)) != "c9446f57d47a0631ede0715de815443442e038ec6d47468453659a4586f29303" {
		t.Fatal("original provenance bytes changed")
	}
}

func TestSourcesActualFixtureDescriptor(t *testing.T) {
	provider := source.NewFixtureProvider()
	w := httptest.NewRecorder()
	sources(provider).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	var got sourcesResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || !reflect.DeepEqual(got.RiskInput, provider.Describe()) {
		t.Fatal("fixture descriptor changed")
	}
}

func TestSourcesRequiresExplicitDescriptor(t *testing.T) {
	h, token := authorizedServer(t, "viewer", nil)
	w := getZones(t, h, token, "/v1/risk/sources")
	if w.Code != 503 {
		t.Fatalf("got %d want 503: %s", w.Code, w.Body.String())
	}
}

type describedSource struct{ descriptor observations.SourceDescriptor }

func (p describedSource) Describe() observations.SourceDescriptor { return p.descriptor }

func TestSourcesActiveProviderAndAuthorization(t *testing.T) {
	descriptor := source.NewFixtureProvider().Describe()
	descriptor.ID = "active-not-default"
	for _, tc := range []struct {
		name, role string
		want       int
	}{
		{"viewer", "viewer", 200}, {"admin", "admin", 200}, {"unrelated", "other", 403}, {"missing", "viewer", 401}, {"invalid", "viewer", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{}
			h, token := authorizedServer(t, tc.role, store, describedSource{descriptor})
			if tc.name == "missing" {
				token = ""
			}
			if tc.name == "invalid" {
				token = "invalid"
			}
			w := getZones(t, h, token, "/v1/risk/sources")
			if w.Code != tc.want {
				t.Fatalf("got %d want %d: %s", w.Code, tc.want, w.Body.String())
			}
			if store.calls+store.zoneCalls+store.listZonesCall != 0 {
				t.Fatal("repository called")
			}
			if tc.want != 200 {
				return
			}
			var got sourcesResponse
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			var original map[string]any
			if err := json.Unmarshal(geo.ComunasProvenanceJSON, &original); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Geography.Provenance, original) {
				t.Fatal("provenance changed")
			}
			if !reflect.DeepEqual(got.RiskInput, descriptor) || !got.RiskInput.Synthetic {
				t.Fatal("active descriptor lost")
			}
			expected := "02ef8e8ff49ab2ca37e5f52c3666d3993fe53730bfd7d7920c933fcec9b5da76"
			if fmt.Sprintf("%x", sha256.Sum256(geo.ComunasGeoJSON)) != expected || got.Geography.Integrity.ComputedSHA256 != expected || got.Geography.Integrity.RecordedSHA256 != expected || !got.Geography.Integrity.Matches {
				t.Fatal("hash mismatch")
			}
			if got.Geography.SourceCutoffAt != nil || got.Geography.CutoffSemantics == "" {
				t.Fatal("invented actual cutoff")
			}
			var body map[string]any
			_ = json.Unmarshal(w.Body.Bytes(), &body)
			if value, ok := body["geography"].(map[string]any)["source_cutoff_at"]; !ok || value != nil {
				t.Fatal("unknown cutoff must be explicit null")
			}
		})
	}
}

func TestSourcesFailClosed(t *testing.T) {
	descriptor := describedSource{source.NewFixtureProvider().Describe()}
	for _, tc := range []struct {
		name                 string
		artifact, boundaries []byte
	}{
		{"corrupt provenance", []byte("{"), geo.ComunasGeoJSON},
		{"empty provenance", []byte("{}"), geo.ComunasGeoJSON},
		{"corrupt geography", geo.ComunasProvenanceJSON, []byte("{")},
		{"hash mismatch", geo.ComunasProvenanceJSON, append(append([]byte{}, geo.ComunasGeoJSON...), ' ')},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			sourcesFrom(descriptor, tc.artifact, tc.boundaries).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			if w.Code != 503 {
				t.Fatalf("got %d", w.Code)
			}
		})
	}
}

func TestSourcesIndependentOfWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	w := httptest.NewRecorder()
	sources(describedSource{source.NewFixtureProvider().Describe()}).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
}

func TestSourcesMissingAuthentication(t *testing.T) {
	w := httptest.NewRecorder()
	New(nil, nil).ServeHTTP(w, httptest.NewRequest("GET", "/v1/risk/sources", nil))
	if w.Code != 401 {
		t.Fatalf("got %d", w.Code)
	}
}
