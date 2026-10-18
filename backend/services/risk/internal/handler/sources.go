package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"firewatch.local/contracts/geo"
	observations "firewatch.local/contracts/observations/v1"
	"firewatch.local/risk/internal/cycle"
)

type sourcesResponse struct {
	Geography geographySource               `json:"geography"`
	RiskInput observations.SourceDescriptor `json:"risk_input"`
	// Sources is the per-source cycle state and freshness. It is always present
	// and never null: with no runner configured it is an empty list, so the UI
	// renders "no sources" without a special case.
	Sources []cycle.SourceStatus `json:"sources"`
}

type geographySource struct {
	Provenance      map[string]any  `json:"provenance"`
	SourceCutoffAt  *string         `json:"source_cutoff_at"`
	CutoffSemantics string          `json:"cutoff_semantics"`
	Integrity       sourceIntegrity `json:"integrity"`
}

type sourceIntegrity struct {
	Algorithm      string `json:"algorithm"`
	RecordedSHA256 string `json:"recorded_sha256"`
	ComputedSHA256 string `json:"computed_sha256"`
	Matches        bool   `json:"matches"`
}

// parseGeography validates locally embedded artifacts, never paths or source URLs.
// The original bytes, including whitespace, are the integrity input.
func parseGeography(artifact, boundaries []byte) (geographySource, error) {
	var p map[string]any
	if err := json.Unmarshal(artifact, &p); err != nil || p == nil {
		return geographySource{}, fmt.Errorf("invalid provenance")
	}
	for _, key := range []string{"source_id", "provider", "endpoint", "query_sha256", "source_format", "derived_format", "retrieved_at", "cutoff_at", "raw_path", "raw_sha256", "derived_path", "derived_sha256", "attribution", "restriction", "restriction_source", "notes"} {
		value, ok := p[key].(string)
		if !ok || value == "" {
			return geographySource{}, fmt.Errorf("invalid %s", key)
		}
	}
	for _, key := range []string{"retrieved_at", "cutoff_at"} {
		if _, err := time.Parse(time.RFC3339, p[key].(string)); err != nil {
			return geographySource{}, err
		}
	}
	if p["cutoff_at"] != p["retrieved_at"] {
		return geographySource{}, fmt.Errorf("unsupported cutoff semantics")
	}
	for _, key := range []string{"requested_srid", "source_wkid", "record_count", "expected_record_count"} {
		value, ok := p[key].(float64)
		if !ok || value <= 0 || value != float64(int64(value)) {
			return geographySource{}, fmt.Errorf("invalid %s", key)
		}
	}
	for _, key := range []string{"record_count_matches_expected", "complete", "exceeded_transfer_limit"} {
		if _, ok := p[key].(bool); !ok {
			return geographySource{}, fmt.Errorf("invalid %s", key)
		}
	}
	if p["complete"] != true || p["record_count_matches_expected"] != true || p["exceeded_transfer_limit"] != false || p["record_count"] != p["expected_record_count"] {
		return geographySource{}, fmt.Errorf("incomplete artifact")
	}
	recorded := p["derived_sha256"].(string)
	sum := sha256.Sum256(boundaries)
	computed := hex.EncodeToString(sum[:])
	var collection struct {
		Type     string            `json:"type"`
		Features []json.RawMessage `json:"features"`
	}
	if json.Unmarshal(boundaries, &collection) != nil || collection.Type != "FeatureCollection" || float64(len(collection.Features)) != p["record_count"] || recorded != computed {
		return geographySource{}, fmt.Errorf("invalid geography integrity")
	}
	return geographySource{Provenance: p, CutoffSemantics: "retrieval_substitution: stored cutoff_at equals retrieved_at; actual source cutoff is unknown", Integrity: sourceIntegrity{Algorithm: "sha256", RecordedSHA256: recorded, ComputedSHA256: computed, Matches: true}}, nil
}

func sources(provider DescriptorProvider) http.HandlerFunc {
	return sourcesWithCycle(provider, nil)
}

// sourcesWithCycle serves the descriptor and the per-source cycle freshness.
func sourcesWithCycle(provider DescriptorProvider, runner CycleRunner) http.HandlerFunc {
	return sourcesFromWithCycle(provider, runner, geo.ComunasProvenanceJSON, geo.ComunasGeoJSON)
}

func sourcesFrom(provider DescriptorProvider, artifact, boundaries []byte) http.HandlerFunc {
	return sourcesFromWithCycle(provider, nil, artifact, boundaries)
}

func sourcesFromWithCycle(provider DescriptorProvider, runner CycleRunner, artifact, boundaries []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		geography, err := parseGeography(artifact, boundaries)
		if err != nil {
			fail(w, http.StatusServiceUnavailable, "geography_integrity_unavailable")
			return
		}
		if provider == nil {
			fail(w, http.StatusServiceUnavailable, "risk_source_unavailable")
			return
		}
		descriptor := provider.Describe()
		if descriptor.ID == "" {
			fail(w, http.StatusServiceUnavailable, "risk_source_unavailable")
			return
		}
		statuses := []cycle.SourceStatus{}
		if runner != nil {
			if reported := runner.Statuses(); reported != nil {
				statuses = reported
			}
		}
		respond(w, http.StatusOK, sourcesResponse{Geography: geography, RiskInput: descriptor, Sources: statuses})
	}
}
