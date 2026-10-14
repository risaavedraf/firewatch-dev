package v1_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	v1 "firewatch.local/contracts/events/v1"
)

func TestLegacyEventOmitsSyntheticScenarioMetadata(t *testing.T) {
	legacy := v1.Event{ID: "event", Type: v1.JobRequestedType, JobID: "job", ZoneID: "valparaiso-demo", Demo: true}
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"scenario_id", "evaluated_at"} {
		if _, ok := fields[key]; ok {
			t.Fatalf("legacy event unexpectedly contains %s: %s", key, payload)
		}
	}
	var decoded v1.Event
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ScenarioID != "" || decoded.EvaluatedAt != nil {
		t.Fatal("legacy decode invented scenario metadata")
	}
	if !reflect.DeepEqual(legacy, decoded) {
		t.Fatal("legacy round trip changed event")
	}
}

func TestScenarioEventRoundTrip(t *testing.T) {
	instant := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	for _, kind := range []string{v1.JobRequestedType, v1.BatchPublishedType} {
		t.Run(kind, func(t *testing.T) {
			original := v1.Event{ID: "event", Type: kind, JobID: "job", ZoneID: "05101", Demo: true, ScenarioID: "mild", EvaluatedAt: &instant}
			payload, err := json.Marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(payload, &fields); err != nil {
				t.Fatal(err)
			}
			if string(fields["scenario_id"]) != `"mild"` || string(fields["evaluated_at"]) != `"2026-10-13T12:00:00Z"` {
				t.Fatalf("incorrect scenario wire shape: %s", payload)
			}
			var decoded v1.Event
			if err := json.Unmarshal(payload, &decoded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(original, decoded) {
				t.Fatalf("scenario round trip changed event: %+v", decoded)
			}
		})
	}
}

func TestSyntheticScenarioWireEnumIsStableAndIndependent(t *testing.T) {
	want := []string{"mild", "mixed", "dry-windy", "missing-humidity", "stale", "partial-retrieval"}
	got := v1.SyntheticScenarioIDs()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scenario IDs = %v, want %v", got, want)
	}
	got[0] = "corrupted"
	if !reflect.DeepEqual(v1.SyntheticScenarioIDs(), want) {
		t.Fatal("wire enum mutated by caller")
	}
}
