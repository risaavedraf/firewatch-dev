package repository

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	v1 "firewatch.local/contracts/events/v1"
)

// These tests only exercise pure seams: the stored job's wire shape and the
// request event built from it. They need no database, so they run everywhere.
// The database-side rules (assign the instant once, reuse the stored metadata on
// an idempotency conflict, never write a second outbox row) live in the
// integration test and require an isolated TEST_DATABASE_URL.

func stringPtr(value string) *string { return &value }

func TestStoredJobKeepsLegacyMetadataAbsent(t *testing.T) {
	legacy := Job{ID: "job", ZoneID: "valparaiso-demo", State: "queued", RequestedAt: time.Unix(0, 0).UTC()}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"scenario_id", "evaluated_at"} {
		if strings.Contains(string(raw), `"`+field+`"`) {
			t.Fatalf("legacy job wire must omit %s: %s", field, raw)
		}
	}
}

func TestStoredJobExposesItsScenarioMetadata(t *testing.T) {
	instant := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	job := Job{ID: "job", ZoneID: "valparaiso-demo", State: "queued", RequestedAt: instant, ScenarioID: stringPtr("dry-windy"), EvaluatedAt: &instant}
	raw, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		ScenarioID  string     `json:"scenario_id"`
		EvaluatedAt *time.Time `json:"evaluated_at"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ScenarioID != "dry-windy" || decoded.EvaluatedAt == nil || !decoded.EvaluatedAt.Equal(instant) {
		t.Fatalf("scenario metadata not preserved: %s", raw)
	}
}

// TestRequestEventCarriesMatchingScenarioMetadata pins that the event emitted
// with the job is built FROM the stored job: a scenario job carries the exact
// persisted scenario and instant, and a legacy job omits both fields.
func TestRequestEventCarriesMatchingScenarioMetadata(t *testing.T) {
	instant := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	scenario := Job{ID: "job", ZoneID: "valparaiso-demo", State: "queued", RequestedAt: instant, ScenarioID: stringPtr("stale"), EvaluatedAt: &instant}
	event := requestEvent(scenario, "actor", "valparaiso-demo", "trace")
	if event.Type != v1.JobRequestedType || event.JobID != "job" || event.ActorID != "actor" || event.ZoneID != "valparaiso-demo" || event.TraceID != "trace" {
		t.Fatalf("event lost its identity: %+v", event)
	}
	if event.ScenarioID != "stale" || event.EvaluatedAt == nil || !event.EvaluatedAt.Equal(instant) {
		t.Fatalf("event scenario metadata=%q %v, want the stored scenario/instant", event.ScenarioID, event.EvaluatedAt)
	}
	legacy := Job{ID: "job", ZoneID: "valparaiso-demo", State: "queued", RequestedAt: instant}
	legacyEvent := requestEvent(legacy, "actor", "valparaiso-demo", "trace")
	if legacyEvent.ScenarioID != "" || legacyEvent.EvaluatedAt != nil {
		t.Fatalf("legacy event must omit metadata: %+v", legacyEvent)
	}
	raw, err := json.Marshal(legacyEvent)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "scenario_id") || strings.Contains(string(raw), "evaluated_at") {
		t.Fatalf("legacy event JSON must omit metadata: %s", raw)
	}
}
