package v1

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The JSON field names below are a wire contract. Changing one silently breaks
// every consumer, so they are pinned here rather than left to inspection.
func TestObservationJSONFieldNamesAreStable(t *testing.T) {
	observed := Observation{
		SourceID:   "ide-chile",
		Variable:   "temperatura",
		ZoneID:     "valparaiso",
		ObservedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
		Value:      27.5,
		Unit:       "C",
		Quality:    QualityOK,
	}
	for _, want := range []string{
		`"source_id"`, `"variable"`, `"zone_id"`, `"observed_at"`, `"value"`, `"unit"`, `"quality"`,
	} {
		if !mustJSON(t, observed, want) {
			t.Errorf("Observation JSON is missing field %s", want)
		}
	}
}

func TestProvenanceJSONFieldNamesAreStable(t *testing.T) {
	provenance := Provenance{
		SourceID:         "ide-chile",
		SourceKind:       SourceKindFile,
		Origin:           OriginPublicOfficial,
		Priority:         10,
		Endpoint:         "https://example.invalid/dataset.csv",
		QueryFingerprint: "sha256:query",
		RetrievedAt:      time.Date(2026, 10, 1, 13, 0, 0, 0, time.UTC),
		CutoffAt:         time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
		Format:           "csv",
		Records:          3,
		PayloadSHA256:    "aaa",
		NormalizedSHA256: "bbb",
		HTTPStatus:       200,
		Complete:         true,
	}
	for _, want := range []string{
		`"source_id"`, `"source_kind"`, `"origin"`, `"priority"`, `"endpoint"`, `"query_fingerprint"`,
		`"retrieved_at"`, `"cutoff_at"`, `"format"`, `"records"`, `"payload_sha256"`,
		`"normalized_sha256"`, `"http_status"`, `"complete"`,
	} {
		if !mustJSON(t, provenance, want) {
			t.Errorf("Provenance JSON is missing field %s", want)
		}
	}
}

func TestSourceDescriptorJSONFieldNamesAreStable(t *testing.T) {
	descriptor := SourceDescriptor{
		ID:               "ide-chile",
		DisplayName:      "IDE Chile",
		Kind:             SourceKindFile,
		Origin:           OriginPublicOfficial,
		Endpoint:         "https://example.invalid/dataset.csv",
		License:          "CC BY 4.0",
		Attribution:      "IDE Chile (2026). Dataset. CC BY 4.0.",
		Variables:        []string{"temperatura"},
		GeoCoverage:      "Región de Valparaíso",
		TemporalCoverage: "2002-2020",
		Cadence:          "static",
		Purpose:          "Etiquetas históricas de incendios",
		Priority:         10,
	}
	for _, want := range []string{
		`"id"`, `"display_name"`, `"kind"`, `"origin"`, `"endpoint"`, `"license"`,
		`"attribution"`, `"variables"`, `"geo_coverage"`, `"temporal_coverage"`,
		`"cadence"`, `"purpose"`, `"priority"`,
	} {
		if !mustJSON(t, descriptor, want) {
			t.Errorf("SourceDescriptor JSON is missing field %s", want)
		}
	}
}

// The kind and origin values reach the traceability table, so their spelling is
// part of the contract too.
func TestKindAndOriginConstantsAreStable(t *testing.T) {
	kinds := map[SourceKind]string{
		SourceKindFile:     "file",
		SourceKindHTTPAPI:  "http_api",
		SourceKindDatabase: "database",
	}
	for kind, want := range kinds {
		if string(kind) != want {
			t.Errorf("SourceKind %q = %q, want %q", want, kind, want)
		}
	}

	origins := map[Origin]string{
		OriginPublicOfficial: "publico_oficial",
		OriginClientPrivate:  "privado_cliente",
	}
	for origin, want := range origins {
		if string(origin) != want {
			t.Errorf("Origin %q = %q, want %q", want, origin, want)
		}
	}
}

func mustJSON(t *testing.T, value any, want string) bool {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return strings.Contains(string(encoded), want)
}
