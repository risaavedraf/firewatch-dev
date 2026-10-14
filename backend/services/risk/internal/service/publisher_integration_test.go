// This file is the composition proof for this slice: the real fixture provider,
// the real academic rule and the real Postgres store agree on one persisted
// assessment, which is what task 8 of the feature document asks for.
//
// The unit tests in publisher_test.go prove each seam against fakes. This test
// proves the seams hold together against the actual storage contract - the
// migrations, the SQL, the JSON encoding of factors and the read-back path - so
// a change to any of them that breaks publication is caught here and nowhere
// else. It runs only against a migrated disposable database, following the same
// skip guard as the repository integration test.
package service

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	v1 "firewatch.local/contracts/events/v1"
	"firewatch.local/risk/internal/classify"
	"firewatch.local/risk/internal/repository"
	"firewatch.local/risk/internal/source"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPublisherPersistsAndReadsBackTheFixtureAssessment(t *testing.T) {
	if testing.Short() || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires migrated disposable risk database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := repository.Store{DB: db}
	publisher := Publisher{
		Provider:   source.NewFixtureProvider(),
		Classifier: classify.NewAcademicRuleV1(),
		Store:      store,
	}
	zones, err := store.ListZones(ctx, nil)
	if err != nil || len(zones) != 38 {
		t.Fatalf("want 38 catalogue communes, got %d: %v", len(zones), err)
	}
	codes := []string{"valparaiso-demo"}
	for _, zone := range zones {
		codes = append(codes, zone.Code)
	}
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			// Backdate to avoid superseding repository tests in the shared demo zone.
			now := time.Now().UTC()
			request := v1.Event{
				ID:          uuid.NewString(),
				Type:        v1.JobRequestedType,
				OccurredAt:  now,
				TraceID:     uuid.NewString(),
				JobID:       uuid.NewString(),
				ActorID:     "integration-test",
				ZoneID:      code,
				RequestedAt: now.Add(-time.Minute),
			}
			if err := publisher.Publish(ctx, request); err != nil {
				t.Fatalf("publish the fixture through the real store: %v", err)
			}

			// The read-back uses a bounded limit larger than one, then selects this
			// request's own job id. The demo zone is shared with the repository
			// integration test, and Go runs package test binaries in parallel, so the
			// single newest row is not guaranteed to be this test's row. Matching the
			// job id proves this request's round trip regardless of what else was
			// persisted concurrently.
			batches, err := store.List(ctx, code, 100)
			if err != nil {
				t.Fatalf("list the persisted batches: %v", err)
			}
			var persisted *repository.Batch
			for index := range batches {
				if batches[index].JobID == request.JobID {
					persisted = &batches[index]
					break
				}
			}
			if persisted == nil {
				t.Fatalf("the published job %s was not read back", request.JobID)
			}
			if persisted.RiskLevel != string(classify.LevelAlto) {
				t.Fatalf("persisted risk level is %q, want %q", persisted.RiskLevel, classify.LevelAlto)
			}
			if persisted.ModelVersion != classify.AcademicRuleV1ID {
				t.Fatalf("persisted model version is %q, want %q", persisted.ModelVersion, classify.AcademicRuleV1ID)
			}
			if !persisted.Demo {
				t.Fatal("the synthetic fixture was persisted as real data")
			}
			if !strings.Contains(string(persisted.Factors), "temperatura") {
				t.Fatalf("persisted factors %s do not mention temperatura", persisted.Factors)
			}
			if persisted.Rationale == "" {
				t.Fatal("persisted assessment has no rationale")
			}
			// The policy reference must make the stored level reproducible: it travels
			// with the assessment through storage and back with the effective parameters
			// and their frozen fingerprint, not just the model name.
			if persisted.Policy == nil {
				t.Fatal("persisted assessment has no policy reference")
			}
			if persisted.Policy.PolicyID != classify.AcademicRuleV1ID {
				t.Fatalf("persisted policy id is %q, want %q", persisted.Policy.PolicyID, classify.AcademicRuleV1ID)
			}
			if persisted.Policy.ParamsHash != classify.AcademicRuleV1ParamsHash {
				t.Fatalf("persisted params hash is %q, want the frozen %q", persisted.Policy.ParamsHash, classify.AcademicRuleV1ParamsHash)
			}
			if got := persisted.Policy.Parameters["max_staleness_seconds"]; got != "21600" {
				t.Fatalf("persisted max_staleness_seconds is %q, want %q", got, "21600")
			}
			var geometry string
			if code == "valparaiso-demo" {
				err = db.QueryRow(ctx, `SELECT ST_AsGeoJSON(location) FROM batches WHERE job_id=$1`, request.JobID).Scan(&geometry)
			} else {
				err = db.QueryRow(ctx, `SELECT ST_AsGeoJSON(geometry) FROM zones WHERE code=$1`, code).Scan(&geometry)
			}
			if err != nil || string(persisted.Geometry) != geometry {
				t.Fatalf("readback geometry differs from catalogue/demo location: %s != %s: %v", persisted.Geometry, geometry, err)
			}
		})
	}
}

// TestPublisherPersistsScenarioMetadataThroughTheRealStore closes the scenario
// selection path against actual storage: the real Publisher selects the built-in
// provider from the request's scenario id, the real store retains the scenario and
// the server-assigned instant on both the batch row and the result event, an
// idempotent retry adds no second outbox row, and a legacy publication keeps both
// columns NULL and omits both keys from its event payload.
func TestPublisherPersistsScenarioMetadataThroughTheRealStore(t *testing.T) {
	if testing.Short() || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires migrated disposable risk database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := repository.Store{DB: db}
	publisher := Publisher{
		// The configured provider is the legacy fixture; a scenario request must
		// override it without consulting it.
		Provider:   source.NewFixtureProvider(),
		Classifier: classify.NewAcademicRuleV1(),
		Store:      store,
	}
	instant := time.Now().UTC().Truncate(time.Second)
	now := time.Now().UTC()
	request := v1.Event{
		ID: uuid.NewString(), Type: v1.JobRequestedType, OccurredAt: now, TraceID: uuid.NewString(),
		JobID: uuid.NewString(), ActorID: "integration-test", ZoneID: "valparaiso-demo",
		RequestedAt: now.Add(-time.Minute), Demo: true,
		ScenarioID: "dry-windy", EvaluatedAt: &instant,
	}
	if err := publisher.Publish(ctx, request); err != nil {
		t.Fatalf("publish the scenario through the real store: %v", err)
	}
	// An at-least-once retry with a fresh delivery id must not create a second
	// outbox row and must keep the original scenario and instant.
	retry := request
	retry.ID = uuid.NewString()
	if err := publisher.Publish(ctx, retry); err != nil {
		t.Fatalf("retry the same scenario job: %v", err)
	}
	var outboxRows int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE payload->>'job_id'=$1`, request.JobID).Scan(&outboxRows); err != nil || outboxRows != 1 {
		t.Fatalf("idempotent retry wrote outbox rows=%d err=%v", outboxRows, err)
	}
	var payload []byte
	if err := db.QueryRow(ctx, `SELECT payload FROM outbox WHERE payload->>'job_id'=$1`, request.JobID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var result v1.Event
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	if result.ScenarioID != "dry-windy" || result.EvaluatedAt == nil || !result.EvaluatedAt.Equal(instant) {
		t.Fatalf("result event did not preserve the scenario/instant: %s", payload)
	}
	batches, err := store.List(ctx, request.ZoneID, 100)
	if err != nil {
		t.Fatal(err)
	}
	var persisted *repository.Batch
	for index := range batches {
		if batches[index].JobID == request.JobID {
			persisted = &batches[index]
		}
	}
	if persisted == nil {
		t.Fatalf("the scenario job %s was not read back", request.JobID)
	}
	if persisted.ScenarioID != "dry-windy" || persisted.EvaluatedAt == nil || !persisted.EvaluatedAt.Equal(instant) {
		t.Fatalf("batch did not retain scenario metadata: %+v", persisted)
	}
	// The store read-back must return the same instant in canonical UTC; Equal
	// alone would accept any location for the same epoch.
	if persisted.EvaluatedAt.Location() != time.UTC {
		t.Fatalf("batch evaluated_at location=%v want UTC", persisted.EvaluatedAt.Location())
	}
	if persisted.RiskLevel != string(classify.LevelAlto) {
		t.Fatalf("dry-windy level is %q, want %q", persisted.RiskLevel, classify.LevelAlto)
	}
	// Legacy leg: no metadata means NULL columns and an event without the keys.
	legacy := request
	legacy.ID = uuid.NewString()
	legacy.JobID = uuid.NewString()
	legacy.ScenarioID = ""
	legacy.EvaluatedAt = nil
	if err := publisher.Publish(ctx, legacy); err != nil {
		t.Fatalf("publish the legacy fixture request: %v", err)
	}
	batches, err = store.List(ctx, legacy.ZoneID, 100)
	if err != nil {
		t.Fatal(err)
	}
	persisted = nil
	for index := range batches {
		if batches[index].JobID == legacy.JobID {
			persisted = &batches[index]
		}
	}
	if persisted == nil {
		t.Fatalf("the legacy job %s was not read back", legacy.JobID)
	}
	if persisted.ScenarioID != "" || persisted.EvaluatedAt != nil {
		t.Fatalf("a legacy batch gained scenario metadata: %+v", persisted)
	}
	if err := db.QueryRow(ctx, `SELECT payload FROM outbox WHERE payload->>'job_id'=$1`, legacy.JobID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "scenario_id") || strings.Contains(string(payload), "evaluated_at") {
		t.Fatalf("a legacy result event grew scenario keys: %s", payload)
	}
}
