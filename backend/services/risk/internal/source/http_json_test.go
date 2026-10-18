package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	observations "firewatch.local/contracts/observations/v1"
)

// httpSample is one raw record as a configured HTTP/JSON source returns it. The
// keys mirror the canonical observation tags so a source can serve the contract
// shape directly. The provider itself lives in http_json.go.
func httpSamplePayload() []byte {
	body, _ := json.Marshal(map[string]any{
		"records": []map[string]any{
			{"source_id": "remote-official", "variable": observations.VariableTemperature, "zone_id": "05101", "observed_at": "2026-10-01T12:00:00Z", "value": 31.5, "unit": "C", "quality": "ok"},
			{"source_id": "remote-official", "variable": observations.VariableHumidity, "zone_id": "05101", "observed_at": "2026-10-01T12:00:00Z", "value": 22.0, "unit": "%", "quality": "ok"},
			{"source_id": "remote-official", "variable": observations.VariableWind, "zone_id": "05101", "observed_at": "2026-10-01T12:00:00Z", "value": 40.0, "unit": "km/h", "quality": "estimated"},
			{"source_id": "remote-official", "variable": observations.VariableVegetation, "zone_id": "05101", "observed_at": "2026-10-01T12:00:00Z", "value": 60.0, "unit": "%", "quality": "ok"},
			{"source_id": "remote-official", "variable": observations.VariableHistoricalFires, "zone_id": "05101", "observed_at": "2026-10-01T12:00:00Z", "value": 3.0, "unit": "count", "quality": "ok"},
		},
		"retrieved_at": "2026-10-01T12:05:00Z",
		"cutoff_at":    "2026-10-01T12:00:00Z",
		"complete":     true,
	})
	return body
}

// T1: FIREWATCH_SOURCE_URL is the single switch. Unset (or "fixture") keeps the
// deterministic built-in sample and makes no network call; a URL selects the
// HTTP/JSON provider.
func TestProviderFromEnvDefaultsToTheDeterministicFixture(t *testing.T) {
	for _, raw := range []string{"", "   ", "fixture", "FIXTURE"} {
		provider, err := ProviderFromEnv(raw)
		if err != nil {
			t.Fatalf("ProviderFromEnv(%q) failed: %v", raw, err)
		}
		if _, ok := provider.(FixtureProvider); !ok {
			t.Fatalf("ProviderFromEnv(%q) = %T, want FixtureProvider", raw, provider)
		}
		// The fixture default must be usable without any network. Fetch only
		// touches the built-in sample.
		set, err := provider.Fetch(context.Background(), observations.Window{})
		if err != nil || len(set.Records) == 0 {
			t.Fatalf("fixture fetch failed: records=%d err=%v", len(set.Records), err)
		}
	}
	provider, err := ProviderFromEnv("http://example.invalid/observations")
	if err != nil {
		t.Fatalf("ProviderFromEnv(http url) failed: %v", err)
	}
	if _, ok := provider.(HTTPProvider); !ok {
		t.Fatalf("ProviderFromEnv(http url) = %T, want HTTPProvider", provider)
	}
}

func TestProviderFromEnvRejectsAMalformedURL(t *testing.T) {
	if _, err := ProviderFromEnv("://missing-scheme"); err == nil {
		t.Fatal("a malformed FIREWATCH_SOURCE_URL was accepted")
	}
}

// T1: the HTTP/JSON provider normalizes a remote payload into the canonical
// ObservationSet and stamps the retrieval provenance (status, payload hash,
// normalized hash, format).
func TestHTTPProviderNormalizesPayloadIntoObservationSet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(httpSamplePayload())
	}))
	defer server.Close()

	instant := time.Date(2026, 10, 1, 12, 5, 0, 0, time.UTC)
	provider, err := NewHTTPProvider(server.URL, HTTPProviderOptions{Client: server.Client(), Now: func() time.Time { return instant }})
	if err != nil {
		t.Fatalf("NewHTTPProvider failed: %v", err)
	}
	if got := provider.Describe().Kind; got != observations.SourceKindHTTPAPI {
		t.Fatalf("descriptor kind = %q, want %q", got, observations.SourceKindHTTPAPI)
	}
	set, err := provider.Fetch(context.Background(), observations.Window{})
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if got := len(set.Records); got != 5 {
		t.Fatalf("records = %d, want 5", got)
	}
	first := set.Records[0]
	if first.Variable != observations.VariableTemperature || first.ZoneID != "05101" || first.Unit != "C" || first.Quality != observations.QualityOK {
		t.Fatalf("normalized record lost fields: %+v", first)
	}
	if first.ObservedAt.Location() != time.UTC || !first.ObservedAt.Equal(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("observed_at = %s, want 2026-10-01T12:00:00Z", first.ObservedAt)
	}
	provenance := set.Provenance
	payload := httpSamplePayload()
	sum := sha256.Sum256(payload)
	if provenance.PayloadSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("payload hash = %q, want the sha256 of the served bytes", provenance.PayloadSHA256)
	}
	if provenance.HTTPStatus != http.StatusOK {
		t.Fatalf("http status = %d, want 200", provenance.HTTPStatus)
	}
	if provenance.Format != "http_json" || provenance.SourceKind != observations.SourceKindHTTPAPI {
		t.Fatalf("provenance lost the transport identity: %+v", provenance)
	}
	if !provenance.RetrievedAt.Equal(instant) || provenance.CutoffAt.IsZero() {
		t.Fatalf("provenance instants = retrieved %s, cutoff %s", provenance.RetrievedAt, provenance.CutoffAt)
	}
	if provenance.Records != 5 || provenance.NormalizedSHA256 == "" || !provenance.Complete {
		t.Fatalf("provenance incomplete: %+v", provenance)
	}
}

// T1: a source that declares an incomplete retrieval must be carried through as
// incomplete, because Merge and the publisher refuse partial sets.
func TestHTTPProviderPreservesIncompleteRetrieval(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"records":  []map[string]any{{"source_id": "remote", "variable": observations.VariableWind, "zone_id": "05101", "observed_at": "2026-10-01T12:00:00Z", "value": 10.0, "unit": "km/h", "quality": "ok"}},
		"complete": false,
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	defer server.Close()
	provider, err := NewHTTPProvider(server.URL, HTTPProviderOptions{Client: server.Client()})
	if err != nil {
		t.Fatalf("NewHTTPProvider failed: %v", err)
	}
	set, err := provider.Fetch(context.Background(), observations.Window{})
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if set.Provenance.Complete {
		t.Fatal("a source that declared complete=false was reported complete")
	}
}

// T1: transport failures are errors, never silent empty sets.
func TestHTTPProviderFailsOnBadStatusAndBody(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"server error", http.StatusInternalServerError, "{}"},
		{"not json", http.StatusOK, "<html>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			provider, err := NewHTTPProvider(server.URL, HTTPProviderOptions{Client: server.Client()})
			if err != nil {
				t.Fatalf("NewHTTPProvider failed: %v", err)
			}
			if _, err := provider.Fetch(context.Background(), observations.Window{}); err == nil {
				t.Fatal("a failed retrieval was reported as success")
			}
		})
	}
}

// T1: the window is forwarded as query parameters so the remote source can
// paginate; the normalized set keeps the requested window.
func TestHTTPProviderForwardsTheWindow(t *testing.T) {
	var gotQuery map[string][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_, _ = w.Write(httpSamplePayload())
	}))
	defer server.Close()
	provider, err := NewHTTPProvider(server.URL, HTTPProviderOptions{Client: server.Client()})
	if err != nil {
		t.Fatalf("NewHTTPProvider failed: %v", err)
	}
	window := observations.Window{From: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	set, err := provider.Fetch(context.Background(), window)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if !reflect.DeepEqual(gotQuery["from"], []string{window.From.Format(time.RFC3339)}) || !reflect.DeepEqual(gotQuery["to"], []string{window.To.Format(time.RFC3339)}) {
		t.Fatalf("window query = %v, want from/to RFC3339", gotQuery)
	}
	if !set.Window.From.Equal(window.From) || !set.Window.To.Equal(window.To) {
		t.Fatalf("set window = %+v, want %+v", set.Window, window)
	}
}
