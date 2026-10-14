package repository

import (
	"encoding/json"
	"testing"
	"time"
)

// TestBatchJSONKeepsLegacyAndScenarioMetadataDistinct protects the read shape
// served by latest and history: a batch written before scenario metadata existed
// must not grow invented keys, while a scenario batch must carry both fields
// exactly. Opposite mistakes are possible in each direction, so both are pinned.
func TestBatchJSONKeepsLegacyAndScenarioMetadataDistinct(t *testing.T) {
	legacy := Batch{ID: "batch", JobID: "job", ZoneID: "05101", RiskLevel: "alto"}
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"scenario_id", "evaluated_at"} {
		if _, present := fields[key]; present {
			t.Fatalf("a legacy batch serialised %q: %s", key, encoded)
		}
	}

	instant := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	scenario := legacy
	scenario.ScenarioID = "dry-windy"
	scenario.EvaluatedAt = &instant
	encoded, err = json.Marshal(scenario)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["scenario_id"]) != `"dry-windy"` || string(fields["evaluated_at"]) != `"2026-10-13T12:00:00Z"` {
		t.Fatalf("scenario metadata wire shape is %s", encoded)
	}
}
