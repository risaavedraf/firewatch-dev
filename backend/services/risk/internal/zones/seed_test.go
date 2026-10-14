package zones

import (
	"encoding/json"
	"testing"

	"firewatch.local/contracts/geo"
)

func TestEmbeddedComunasParse(t *testing.T) {
	parsed, err := ParseComunas(geo.ComunasGeoJSON)
	if err != nil {
		t.Fatalf("ParseComunas(embedded): %v", err)
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
		if len(zone.Geometry) == 0 {
			t.Errorf("zone %s has no geometry", zone.Code)
			continue
		}
		// Every geometry must be parseable JSON with a declared type. It is not
		// re-parsed by hand into rings: the asset is the product of M1's converter,
		// and PostGIS validates it on load.
		var geometry struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(zone.Geometry, &geometry); err != nil {
			t.Errorf("zone %s geometry is not valid JSON: %v", zone.Code, err)
			continue
		}
		if geometry.Type == "" {
			t.Errorf("zone %s geometry has no type", zone.Code)
		}
	}
	// Isla de Pascua and Juan Fernandez are far from the continent but they are
	// part of the region: their absence would silently shrink the map.
	for _, code := range []string{"05201", "05104"} {
		if _, ok := byCode[code]; !ok {
			t.Errorf("embedded asset is missing zone %s", code)
		}
	}

	// Cross-check names and provinces against the raw document properties, so the
	// mapping is proven independently of ParseComunas' own decoding.
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

func TestParseComunasRefusesBadInput(t *testing.T) {
	// One feature that on its own is valid, so each negative case fails for the
	// stated reason and not for an unrelated one.
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
		if _, err := ParseComunas([]byte(tc.data)); err == nil {
			t.Errorf("%s: ParseComunas accepted the input, want an error", tc.name)
		}
	}
}
