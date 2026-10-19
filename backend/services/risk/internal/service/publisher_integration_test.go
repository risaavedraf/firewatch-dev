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
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	v1 "firewatch.local/contracts/events/v1"
	"firewatch.local/risk/internal/classify"
	"firewatch.local/risk/internal/cycle"
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
// operator re-dispatch publishes a second completion that names the same stored
// batch, and a legacy publication keeps both columns NULL and omits both keys from
// its event payload.
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
	// An operator re-dispatch arrives with a fresh event id, so the inbox gate
	// cannot recognise it: the batch already exists, and the store must emit a
	// second completion that still names that stored batch while keeping the
	// original scenario and instant. A silent commit here would leave the job
	// unable to reach its terminal state.
	retry := request
	retry.ID = uuid.NewString()
	if err := publisher.Publish(ctx, retry); err != nil {
		t.Fatalf("re-dispatch the same scenario job: %v", err)
	}
	var outboxRows, distinctBatchIDs int
	if err := db.QueryRow(ctx, `SELECT count(*), count(DISTINCT payload->>'batch_id') FROM outbox WHERE payload->>'job_id'=$1`, request.JobID).Scan(&outboxRows, &distinctBatchIDs); err != nil || outboxRows != 2 || distinctBatchIDs != 1 {
		t.Fatalf("re-dispatch wrote outbox rows=%d naming %d batches: %v", outboxRows, distinctBatchIDs, err)
	}
	var storedBatchID string
	if err := db.QueryRow(ctx, `SELECT id::text FROM batches WHERE job_id=$1`, request.JobID).Scan(&storedBatchID); err != nil {
		t.Fatal(err)
	}
	var namedStored int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE payload->>'job_id'=$1 AND payload->>'batch_id'=$2`, request.JobID, storedBatchID).Scan(&namedStored); err != nil || namedStored != 2 {
		t.Fatalf("completions naming the stored batch=%d err=%v", namedStored, err)
	}
	// The read-back selects the completion for the stored batch instead of an
	// arbitrary single row: with a second completion present, only matching the
	// stored batch id pins the payload to this job's batch.
	var payload []byte
	if err := db.QueryRow(ctx, `SELECT payload FROM outbox WHERE payload->>'job_id'=$1 AND payload->>'batch_id'=$2`, request.JobID, storedBatchID).Scan(&payload); err != nil {
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

// S1 integration: an automatic cycle persists one batch per zone through the real
// store and leaves no completion event behind. Jobs never stored the id the cycle
// mints, so every completion would be refused as an unknown job and dead-lettered
// permanently, once per zone per cycle. The explicit job keeps its own
// completion, so closing the scheduled path did not weaken the lifecycle.
func TestAutomaticCyclePersistsBatchesWithoutACompletionEvent(t *testing.T) {
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
	publisher := Publisher{Provider: source.NewFixtureProvider(), Classifier: classify.NewAcademicRuleV1(), Store: store}
	instant := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	ids := make([]string, 0, 3)
	runner := cycle.New(source.NewFixtureProvider(), &publisher, cycle.Options{
		Zones:    []string{"valparaiso-demo"},
		Interval: cycle.DefaultInterval,
		Now:      func() time.Time { return instant },
		NewID:    func() string { id := uuid.NewString(); ids = append(ids, id); return id },
	})
	if result := runner.RunOnce(ctx); !result.Complete {
		t.Fatalf("cycle result = %+v", result)
	}
	// The batch is stored under the job id the cycle minted...
	var jobID string
	if err = db.QueryRow(ctx, `SELECT job_id::text FROM batches WHERE job_id::text = ANY($1::text[])`, ids).Scan(&jobID); err != nil {
		t.Fatalf("the cycle persisted no batch under any id it minted: %v", err)
	}
	// ...and no completion event names any id the cycle minted, so the jobs
	// consumer has nothing to refuse and dead-letter.
	var completions int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE routing_key=$1 AND payload->>'job_id' = ANY($2::text[])`, v1.BatchPublishedType, ids).Scan(&completions); err != nil || completions != 0 {
		t.Fatalf("the automatic cycle emitted %d completion events, want 0: %v", completions, err)
	}
	explicit := v1.Event{ID: uuid.NewString(), Type: v1.JobRequestedType, OccurredAt: instant, TraceID: uuid.NewString(), JobID: uuid.NewString(), ActorID: "integration-test", ZoneID: "valparaiso-demo", RequestedAt: instant, Demo: true}
	if err = publisher.Publish(ctx, explicit); err != nil {
		t.Fatalf("publish the requested job: %v", err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE routing_key=$1 AND payload->>'job_id'=$2`, v1.BatchPublishedType, explicit.JobID).Scan(&completions); err != nil || completions != 1 {
		t.Fatalf("the requested job emitted %d completions, want exactly 1: %v", completions, err)
	}
}

// TestPublisherFollowsActivationAndReversalThroughTheRealStore is the acceptance
// proof of this unit end to end: with the real transactional activation and the
// real storage contract, activating v2 and then reverting to v1 must change the
// version, the policy identifier and the policy fingerprint of the next persisted
// batch. Before this unit the publication path never read the active row, so the
// batch after an activation still carried the startup classifier.
//
// It runs against the disposable migrated database like the other integration
// tests, so it skips without TEST_DATABASE_URL and runs in `make integration`.
func TestPublisherFollowsActivationAndReversalThroughTheRealStore(t *testing.T) {
	if testing.Short() || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires migrated disposable risk database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := repository.Store{DB: db}

	registry := classify.NewRegistry()
	if err := registry.Register(classify.NewAcademicRuleV1()); err != nil {
		t.Fatalf("register academic-rule-v1: %v", err)
	}
	if err := registry.Register(classify.NewFirewatchRuleV2()); err != nil {
		t.Fatalf("register firewatch-rule-v2: %v", err)
	}
	previous, err := store.ActiveModel(ctx)
	if err == nil {
		defer func() {
			if _, err := store.ActivateModel(context.Background(), previous.Version); err != nil {
				t.Errorf("could not restore the previously active version %q: %v", previous.Version, err)
			}
		}()
	}

	publisher := Publisher{
		Provider: source.NewFixtureProvider(),
		// No static Classifier on purpose: every publication must resolve the
		// classifier from the registry, which is what the activation changes.
		Classifiers: RegistryClassifierSource{Models: store, Registry: registry, Bootstrap: classify.NewAcademicRuleV1()},
		Store:       store,
	}
	// publishAndReadBack activates nothing: it publishes the fixture and returns the
	// batch this request persisted, matched by job id because the demo zone is
	// shared with the other integration tests.
	publishAndReadBack := func(t *testing.T) repository.Batch {
		t.Helper()
		now := time.Now().UTC()
		request := v1.Event{
			ID: uuid.NewString(), Type: v1.JobRequestedType, OccurredAt: now, TraceID: uuid.NewString(),
			JobID: uuid.NewString(), ActorID: "integration-test", ZoneID: "valparaiso-demo",
			RequestedAt: now.Add(-time.Minute), Demo: true,
		}
		if err := publisher.Publish(ctx, request); err != nil {
			t.Fatalf("publish the fixture through the real store: %v", err)
		}
		batches, err := store.List(ctx, request.ZoneID, 100)
		if err != nil {
			t.Fatalf("list the persisted batches: %v", err)
		}
		for index := range batches {
			if batches[index].JobID == request.JobID {
				return batches[index]
			}
		}
		t.Fatalf("the published job %s was not read back", request.JobID)
		return repository.Batch{}
	}

	if _, err := store.ActivateModel(ctx, classify.AcademicRuleV1ID); err != nil {
		t.Fatalf("activate academic-rule-v1: %v", err)
	}
	first := publishAndReadBack(t)
	if first.ModelVersion != classify.AcademicRuleV1ID || first.Policy == nil || first.Policy.PolicyID != classify.AcademicRuleV1ID || first.Policy.ParamsHash != classify.AcademicRuleV1ParamsHash {
		t.Fatalf("with v1 activated the batch carries %q policy %+v, want v1 with its own fingerprint", first.ModelVersion, first.Policy)
	}

	if _, err := store.ActivateModel(ctx, classify.FirewatchRuleV2ID); err != nil {
		if errors.Is(err, repository.ErrModelNotFound) {
			t.Skipf("the registry has no firewatch-rule-v2 row; this proof needs 008_firewatch_rule_v2 applied: %v", err)
		}
		t.Fatalf("activate firewatch-rule-v2: %v", err)
	}
	second := publishAndReadBack(t)
	if second.ModelVersion != classify.FirewatchRuleV2ID || second.Policy == nil || second.Policy.PolicyID != classify.FirewatchRuleV2ID || second.Policy.ParamsHash != classify.FirewatchRuleV2ParamsHash {
		t.Fatalf("after activating v2 the batch carries %q policy %+v, want v2 with its own fingerprint", second.ModelVersion, second.Policy)
	}
	if second.ModelVersion == first.ModelVersion || second.Policy.ParamsHash == first.Policy.ParamsHash {
		t.Fatalf("activating v2 did not change the published batch: %q then %q", first.ModelVersion, second.ModelVersion)
	}

	if _, err := store.ActivateModel(ctx, classify.AcademicRuleV1ID); err != nil {
		t.Fatalf("revert to academic-rule-v1: %v", err)
	}
	reverted := publishAndReadBack(t)
	if reverted.ModelVersion != first.ModelVersion || reverted.Policy == nil || reverted.Policy.ParamsHash != first.Policy.ParamsHash {
		t.Fatalf("after the reversal the batch carries %q policy %+v, want the original %q with its fingerprint", reverted.ModelVersion, reverted.Policy, first.ModelVersion)
	}
}
