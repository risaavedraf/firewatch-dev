package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"firewatch.local/contracts/geo"
)

func TestEmbeddedComunasParseIntoTheJobsCatalogue(t *testing.T) {
	parsed, err := ParseZones(geo.ComunasGeoJSON)
	if err != nil {
		t.Fatalf("ParseZones(embedded): %v", err)
	}
	// The expected count is pinned HERE and not in production code: a legitimate
	// change to the source shape must force a deliberate test update instead of
	// looking like a runtime defect.
	if len(parsed) != 38 {
		t.Fatalf("embedded asset yields %d zones, want 38", len(parsed))
	}
	for index := 1; index < len(parsed); index++ {
		if parsed[index-1].Code >= parsed[index].Code {
			t.Fatalf("zones are not sorted by code: %q then %q", parsed[index-1].Code, parsed[index].Code)
		}
	}
	if first, last := parsed[0].Code, parsed[len(parsed)-1].Code; first != "05101" || last != "05804" {
		t.Fatalf("codes span %s..%s, want 05101..05804", first, last)
	}
	byCode := make(map[string]Zone, len(parsed))
	for _, zone := range parsed {
		byCode[zone.Code] = zone
		if zone.Kind != "comuna" {
			t.Errorf("zone %s has kind %q, want comuna", zone.Code, zone.Kind)
		}
	}
	// Isla de Pascua and Juan Fernandez are far from the continent but they are
	// part of the region: their absence would silently shrink the map.
	for _, code := range []string{"05201", "05104"} {
		if _, ok := byCode[code]; !ok {
			t.Errorf("embedded asset is missing zone %s", code)
		}
	}
	if zone := byCode["05201"]; zone.Name != "Isla de Pascua" || zone.Province != "Isla de Pascua" {
		t.Errorf("zone 05201 is %q/%q, want Isla de Pascua/Isla de Pascua", zone.Name, zone.Province)
	}
	if zone := byCode["05104"]; zone.Name != "Juan Fernández" || zone.Province != "Valparaíso" {
		t.Errorf("zone 05104 is %q/%q, want Juan Fernández/Valparaíso", zone.Name, zone.Province)
	}

	// Cross-check names and provinces against the raw document properties, so the
	// mapping is proven independently of ParseZones' own decoding.
	var raw struct {
		Features []struct {
			Properties struct {
				CutCom    string `json:"CUT_COM"`
				Comuna    string `json:"COMUNA"`
				Provincia string `json:"PROVINCIA"`
			} `json:"properties"`
		} `json:"features"`
	}
	if err := json.Unmarshal(geo.ComunasGeoJSON, &raw); err != nil {
		t.Fatalf("decode raw asset properties: %v", err)
	}
	if len(raw.Features) != len(parsed) {
		t.Fatalf("raw asset has %d features, parsed into %d zones", len(raw.Features), len(parsed))
	}
	for _, current := range raw.Features {
		zone, ok := byCode[current.Properties.CutCom]
		if !ok {
			t.Errorf("parsed zones are missing %s", current.Properties.CutCom)
			continue
		}
		if zone.Name != current.Properties.Comuna {
			t.Errorf("zone %s name is %q, want the property %q", zone.Code, zone.Name, current.Properties.Comuna)
		}
		if zone.Province != current.Properties.Provincia {
			t.Errorf("zone %s province is %q, want the property %q", zone.Code, zone.Province, current.Properties.Provincia)
		}
	}
}

func TestParseZonesRefusesBadInput(t *testing.T) {
	// One feature that on its own is valid, so each negative case fails for the
	// stated reason and not for an unrelated one. The geometry is present but
	// must be ignored by the projection.
	const validFeature = `{"type":"Feature","properties":{"CUT_COM":"05101","COMUNA":"Valparaiso","PROVINCIA":"Valparaiso"},"geometry":{"type":"Point","coordinates":[0,0]}}`
	cases := []struct {
		name string
		data string
	}{
		{name: "empty input", data: ``},
		{name: "not JSON", data: `{`},
		{name: "empty collection", data: `{"type":"FeatureCollection","features":[]}`},
		{name: "wrong type", data: `{"type":"Feature","features":[` + validFeature + `]}`},
		{name: "missing CUT_COM", data: `{"type":"FeatureCollection","features":[{"type":"Feature","properties":{"COMUNA":"Valparaiso","PROVINCIA":"Valparaiso"},"geometry":null}]}`},
		{name: "missing COMUNA", data: `{"type":"FeatureCollection","features":[{"type":"Feature","properties":{"CUT_COM":"05101","PROVINCIA":"Valparaiso"},"geometry":null}]}`},
		{name: "duplicate code", data: `{"type":"FeatureCollection","features":[` + validFeature + `,` + validFeature + `]}`},
	}
	for _, tc := range cases {
		if _, err := ParseZones([]byte(tc.data)); err == nil {
			t.Errorf("%s: ParseZones accepted the input, want an error", tc.name)
		}
	}
}

// compareCatalogueCodes is the pure detector behind the cross-database guard of
// the geospatial feature. It holds no database access on purpose: the
// integration test feeds it the two REAL catalogues, and the unit tests below
// feed it synthetic ones. Keeping it pure means the detection logic runs in
// every environment instead of hiding behind a t.Skip.
//
// A divergence is reported as the exact code and its direction. The size check
// is additive, not an early return: when a set is truncated the caller still
// learns which codes went missing. Together they cannot be satisfied by an
// empty or one-sided read.
func compareCatalogueCodes(jobsCodes, referenceCodes []string) error {
	inReference := make(map[string]bool, len(referenceCodes))
	for _, code := range referenceCodes {
		inReference[code] = true
	}
	inJobs := make(map[string]bool, len(jobsCodes))
	for _, code := range jobsCodes {
		inJobs[code] = true
	}
	var divergences []string
	for _, code := range jobsCodes {
		if !inReference[code] {
			divergences = append(divergences, fmt.Sprintf("code %s is present in jobs.zones but missing from the risk reference", code))
		}
	}
	for _, code := range referenceCodes {
		if !inJobs[code] {
			divergences = append(divergences, fmt.Sprintf("code %s is present in the risk reference but missing from jobs.zones", code))
		}
	}
	if len(jobsCodes) != len(referenceCodes) {
		divergences = append(divergences, fmt.Sprintf("catalogue size mismatch: jobs.zones holds %d codes but the risk reference holds %d", len(jobsCodes), len(referenceCodes)))
	}
	if len(divergences) == 0 {
		return nil
	}
	return errors.New(strings.Join(divergences, "; "))
}

// TestCompareCatalogueCodesDetectsDivergentCode is the negative half of the
// cross-database guard. It plants a deliberately divergent code on the jobs side
// and requires the detector to name it and state the direction. It needs no
// database and no DSN, so it runs unconditionally: the guard can never again be
// latent just because the integration databases are absent.
func TestCompareCatalogueCodesDetectsDivergentCode(t *testing.T) {
	jobs := []string{"05101", "05102", "ZZZ-DIVERGENCE-PROBE"}
	reference := []string{"05101", "05102", "05103"}

	err := compareCatalogueCodes(jobs, reference)
	if err == nil {
		t.Fatal("a divergent code was not detected")
	}
	if !strings.Contains(err.Error(), "ZZZ-DIVERGENCE-PROBE") {
		t.Errorf("the error does not name the divergent code: %v", err)
	}
	if !strings.Contains(err.Error(), "present in jobs.zones but missing from the risk reference") {
		t.Errorf("the error does not state the direction: %v", err)
	}
}

// TestCompareCatalogueCodesDetectsReferenceOnlyCode covers the opposite
// direction: a code the reference holds and jobs lost.
func TestCompareCatalogueCodesDetectsReferenceOnlyCode(t *testing.T) {
	jobs := []string{"05101", "05102", "05103"}
	reference := []string{"05101", "05102", "ZZZ-DIVERGENCE-PROBE"}

	err := compareCatalogueCodes(jobs, reference)
	if err == nil {
		t.Fatal("a reference-only code was not detected")
	}
	if !strings.Contains(err.Error(), "ZZZ-DIVERGENCE-PROBE") {
		t.Errorf("the error does not name the divergent code: %v", err)
	}
	if !strings.Contains(err.Error(), "present in the risk reference but missing from jobs.zones") {
		t.Errorf("the error does not state the reverse direction: %v", err)
	}
}

// TestCompareCatalogueCodesDetectsSizeMismatch keeps the shared prefix
// identical and adds one extra code: a truncation must not pass by matching the
// prefix it happens to share with the reference.
func TestCompareCatalogueCodesDetectsSizeMismatch(t *testing.T) {
	jobs := []string{"05101", "05102", "05103"}
	reference := []string{"05101", "05102"}

	err := compareCatalogueCodes(jobs, reference)
	if err == nil {
		t.Fatal("a catalogue size mismatch was not detected")
	}
	if !strings.Contains(err.Error(), "size mismatch") {
		t.Errorf("the error does not name the size mismatch: %v", err)
	}
	if !strings.Contains(err.Error(), "05103") {
		t.Errorf("the error does not name the truncated code: %v", err)
	}
}

// TestCompareCatalogueCodesAcceptsMatchingSets anchors the negatives: the same
// codes in a different order are NOT a divergence, so the tests above fail for
// the planted reason and not because the detector rejects everything.
func TestCompareCatalogueCodesAcceptsMatchingSets(t *testing.T) {
	jobs := []string{"05102", "05101", "05103"}
	reference := []string{"05101", "05102", "05103"}
	if err := compareCatalogueCodes(jobs, reference); err != nil {
		t.Fatalf("matching catalogues were reported as divergent: %v", err)
	}
}

// TestCompareCatalogueCodesDetectsOneSidedEmptiness proves a one-sided read (the
// exact truncation an empty-vs-empty guard would not catch) is reported. Two
// empty sets are not a divergence at this level; refusing a trivial comparison
// outright is the integration test's job.
func TestCompareCatalogueCodesDetectsOneSidedEmptiness(t *testing.T) {
	cases := []struct {
		name      string
		jobs      []string
		reference []string
	}{
		{name: "jobs empty", jobs: nil, reference: []string{"05101", "05102"}},
		{name: "reference empty", jobs: []string{"05101", "05102"}, reference: nil},
	}
	for _, tc := range cases {
		if err := compareCatalogueCodes(tc.jobs, tc.reference); err == nil {
			t.Errorf("%s: the detector accepted a one-sided catalogue", tc.name)
		}
	}
}
