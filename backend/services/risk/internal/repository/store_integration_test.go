package repository

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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBatchDeduplicationAndOutOfOrderPublication(t *testing.T) {
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
	s := Store{DB: db}
	// A real assessment, not the old placeholder: the level, the factor and the
	// rationale must survive the round trip through storage and List.
	assessment := Assessment{
		ModelVersion: "academic-rule-v1",
		RiskLevel:    "alto",
		Factors: []classify.Factor{{
			Name:         "temperatura",
			Known:        true,
			Value:        32,
			Contribution: 0.85,
			Note:         "lectura de alta temperatura",
		}},
		Rationale: "Puntaje ponderado alto: temperatura y viento sostienen el nivel alto.",
		Demo:      false,
	}
	newer := v1.Event{ID: uuid.NewString(), JobID: uuid.NewString(), TraceID: uuid.NewString(), ActorID: "test", Type: v1.JobRequestedType, ZoneID: "valparaiso-demo", Demo: true, RequestedAt: time.Now().UTC()}
	if err = s.Publish(ctx, newer, assessment); err != nil {
		t.Fatal(err)
	}
	if err = s.Publish(ctx, newer, assessment); err != nil {
		t.Fatal(err)
	}
	// A broker redelivery of the same event id is stopped by the inbox gate
	// before the batch insert, so it must add nothing at all.
	var count int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE payload->>'job_id'=$1`, newer.JobID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("same-id retry outbox count=%d err=%v", count, err)
	}
	// An operator re-dispatch arrives with a fresh event id, so the inbox gate
	// cannot recognise it. The batch already exists, and the re-dispatch must
	// still emit a completion naming that stored batch so the job can reach its
	// terminal state; a silent commit would repair nothing.
	duplicate := newer
	duplicate.ID = uuid.NewString()
	if err = s.Publish(ctx, duplicate, assessment); err != nil {
		t.Fatal(err)
	}
	older := newer
	older.ID = uuid.NewString()
	older.JobID = uuid.NewString()
	older.RequestedAt = newer.RequestedAt.Add(-time.Hour)
	if err = s.Publish(ctx, older, assessment); err != nil {
		t.Fatal(err)
	}
	batches, err := s.List(ctx, "valparaiso-demo", 1)
	if err != nil || len(batches) != 1 || batches[0].JobID != newer.JobID {
		t.Fatalf("old batch superseded newest: %+v %v", batches, err)
	}
	// A re-dispatched delivery must not be swallowed: it writes a second
	// completion, and both completions name the one stored batch. An invented
	// batch id or a second batches row would report a batch that does not exist.
	var outboxRows, distinctBatchIDs, batchesRows int
	if err = db.QueryRow(ctx, `SELECT count(*), count(DISTINCT payload->>'batch_id') FROM outbox WHERE payload->>'job_id'=$1`, newer.JobID).Scan(&outboxRows, &distinctBatchIDs); err != nil {
		t.Fatal(err)
	}
	if outboxRows != 2 || distinctBatchIDs != 1 {
		t.Fatalf("outbox rows=%d distinct batch ids=%d, want 2 rows naming one batch", outboxRows, distinctBatchIDs)
	}
	var storedBatchID string
	if err = db.QueryRow(ctx, `SELECT id::text FROM batches WHERE job_id=$1`, newer.JobID).Scan(&storedBatchID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE payload->>'job_id'=$1 AND payload->>'batch_id'=$2`, newer.JobID, storedBatchID).Scan(&outboxRows); err != nil || outboxRows != 2 {
		t.Fatalf("completions naming the stored batch=%d err=%v", outboxRows, err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM batches WHERE job_id=$1`, newer.JobID).Scan(&batchesRows); err != nil || batchesRows != 1 {
		t.Fatalf("batches rows=%d err=%v", batchesRows, err)
	}
	if batches[0].RiskLevel != "alto" {
		t.Fatalf("risk level is %q, want the persisted %q", batches[0].RiskLevel, "alto")
	}
	if !strings.Contains(string(batches[0].Factors), "temperatura") {
		t.Fatalf("factors %s do not contain the persisted factor", batches[0].Factors)
	}
	if batches[0].Rationale != assessment.Rationale {
		t.Fatalf("rationale is %q, want the persisted %q", batches[0].Rationale, assessment.Rationale)
	}
	if batches[0].Demo {
		t.Fatal("a real assessment was persisted as a demonstration")
	}
	// This assessment was built without a policy reference, so none must be
	// invented on the way back: nil is "no reference was recorded".
	if batches[0].Policy != nil {
		t.Fatalf("an assessment without a policy reference came back with %+v", batches[0].Policy)
	}
}

// TestPublishRefusesAMissingReferencePermanently protects the live-demo failure
// mode: a well-formed request for a zone that is absent from the catalogue is
// refused by batches_zone_id_fkey, and an unregistered model version is refused
// by batches_model_version_fkey. Neither referenced row can appear by retrying,
// so the error must declare itself permanent and the risk consumer must
// dead-letter it after ONE attempt instead of blocking the queue for the whole
// retry budget.
func TestPublishRefusesAMissingReferencePermanently(t *testing.T) {
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
	s := Store{DB: db}
	assessment := Assessment{
		ModelVersion: "academic-rule-v1",
		RiskLevel:    "alto",
		Factors:      []classify.Factor{},
		Rationale:    "referencia inexistente",
	}
	// The second case keeps the zone valid (the legacy row exists in zones) so the
	// model version is the only missing reference.
	cases := []struct {
		name         string
		zone         string
		modelVersion string
	}{
		{"unknown zone", "zona-inexistente-permanente", assessment.ModelVersion},
		{"unregistered model version", "valparaiso-demo", "modelo-inexistente-permanente"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reference := assessment
			reference.ModelVersion = tc.modelVersion
			event := v1.Event{ID: uuid.NewString(), JobID: uuid.NewString(), TraceID: uuid.NewString(), ActorID: "test", Type: v1.JobRequestedType, ZoneID: tc.zone, Demo: true, RequestedAt: time.Now().UTC()}
			err := s.Publish(ctx, event, reference)
			if err == nil {
				t.Fatal("a batch referencing a row that does not exist was published")
			}
			// The broker classifies permanence with exactly this interface
			// (messaging.isPermanent asks errors.As for Permanent() bool), so this
			// error takes the single-attempt path of handleWithRetries instead of
			// the five-attempt one; broker_test.go pins that path.
			var permanent interface{ Permanent() bool }
			if !errors.As(err, &permanent) || !permanent.Permanent() {
				t.Fatalf("err=%v must declare itself permanent", err)
			}
			var stored int
			if err = db.QueryRow(ctx, `SELECT count(*) FROM batches WHERE job_id=$1`, event.JobID).Scan(&stored); err != nil {
				t.Fatal(err)
			}
			if stored != 0 {
				t.Fatalf("batches stored=%d want=0 for a refused reference", stored)
			}
		})
	}
}

// TestJobFailureEventBuildsTheOutboxRecord pins the cross-service
// terminal-failure report a give-up produces: the new wire type, the job and
// trace ids, the classified reason and the attempt count. The generated id is the
// event id, exactly as the batch publication event does it. The test is pure, so
// it runs without a database.
func TestJobFailureEventBuildsTheOutboxRecord(t *testing.T) {
	delivery := v1.Event{JobID: uuid.NewString(), TraceID: uuid.NewString()}
	report := jobFailureEvent(delivery, "retries_exhausted", 5)
	if report.Type != v1.JobFailedType || report.JobID != delivery.JobID || report.TraceID != delivery.TraceID || report.FailureReason != "retries_exhausted" || report.Attempts != 5 {
		t.Fatalf("report=%+v", report)
	}
	if id, err := uuid.Parse(report.ID); err != nil || id == uuid.Nil {
		t.Fatalf("report id %q is not a UUID: %v", report.ID, err)
	}
	payload, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["type"]) != `"risk.job.failed.v1"` || string(fields["failure_reason"]) != `"retries_exhausted"` || string(fields["attempts"]) != `5` {
		t.Fatalf("report wire shape: %s", payload)
	}
}

// TestRecordJobFailureWritesTheReportIntoTheOutbox protects the durable half of a
// give-up: the report is inserted into this service's own outbox and picked up by
// the existing relay, never published from the consumer hot path. It needs a
// migrated disposable risk database.
func TestRecordJobFailureWritesTheReportIntoTheOutbox(t *testing.T) {
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
	s := Store{DB: db}
	delivery := v1.Event{JobID: uuid.NewString(), TraceID: uuid.NewString()}
	if err := s.RecordJobFailure(ctx, delivery, "permanent_error", 1); err != nil {
		t.Fatal(err)
	}
	var routingKey string
	var payload []byte
	if err := db.QueryRow(ctx, `SELECT routing_key,payload FROM outbox WHERE payload->>'job_id'=$1`, delivery.JobID).Scan(&routingKey, &payload); err != nil {
		t.Fatal(err)
	}
	if routingKey != v1.JobFailedType {
		t.Fatalf("routing_key=%q want %q", routingKey, v1.JobFailedType)
	}
	var report v1.Event
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatal(err)
	}
	if report.Type != v1.JobFailedType || report.JobID != delivery.JobID || report.TraceID != delivery.TraceID || report.FailureReason != "permanent_error" || report.Attempts != 1 {
		t.Fatalf("report=%+v", report)
	}
}

func TestListZonesCatalogueAndBBox(t *testing.T) {
	if testing.Short() || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires migrated disposable risk database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := Store{DB: db}

	// The full catalogue holds every zone that has a geometry, ordered by code.
	all, err := s.ListZones(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) == 0 {
		t.Fatal("the migrated catalogue returned no zone with geometry")
	}
	for index, zone := range all {
		if zone.Code == "" || zone.Name == "" || zone.Kind == "" {
			t.Fatalf("zone %d is missing code, name or kind: %+v", index, zone)
		}
		if index > 0 && all[index-1].Code >= zone.Code {
			t.Fatalf("catalogue is not ordered by code: %s then %s", all[index-1].Code, zone.Code)
		}
		var geometry map[string]any
		if err := json.Unmarshal(zone.Geometry, &geometry); err != nil {
			t.Fatalf("zone %s carries invalid geometry %s: %v", zone.Code, zone.Geometry, err)
		}
		if geometry["type"] == nil {
			t.Fatalf("zone %s geometry has no type: %s", zone.Code, zone.Geometry)
		}
	}
	// A zone without geometry is a declared state; the legacy demo row is one,
	// and it must never be served.
	for _, zone := range all {
		if zone.Code == "valparaiso-demo" {
			t.Fatal("a zone without geometry was returned")
		}
	}
	// Isla de Pascua is a real commune in the catalogue; its presence is what
	// makes the bbox exclusion below meaningful.
	foundEaster := false
	for _, zone := range all {
		if zone.Code == "05201" {
			foundEaster = true
		}
	}
	if !foundEaster {
		t.Fatal("the catalogue does not contain 05201, so the bbox exclusion cannot be proven")
	}

	// A mainland bbox returns a non-empty strict subset and excludes the island
	// communes whose geometry lies far outside it.
	mainland := &BBox{MinLon: -72, MinLat: -34, MaxLon: -70, MaxLat: -32}
	subset, err := s.ListZones(ctx, mainland)
	if err != nil {
		t.Fatal(err)
	}
	if len(subset) == 0 || len(subset) >= len(all) {
		t.Fatalf("mainland bbox returned %d of %d zones, want a non-empty strict subset", len(subset), len(all))
	}
	known := make(map[string]bool, len(all))
	for _, zone := range all {
		known[zone.Code] = true
	}
	for _, zone := range subset {
		if zone.Code == "05201" {
			t.Fatal("a mainland bbox returned Isla de Pascua")
		}
		if !known[zone.Code] {
			t.Fatalf("bbox returned zone %s, which the full catalogue does not hold", zone.Code)
		}
	}

	// A bbox far from the region matches nothing. Empty is a valid answer, not
	// an error, and it must not fall back to the whole catalogue.
	far := &BBox{MinLon: 10, MinLat: 10, MaxLon: 20, MaxLat: 20}
	empty, err := s.ListZones(ctx, far)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("a bbox far away returned %d zones", len(empty))
	}
}

// TestModelParamsHashReadsTheFrozenRegistryHash protects the startup check's read
// side on a migrated database: the version the service runs must exist with the
// frozen fingerprint, and a version the registry does not hold must be reported
// as absent rather than as an empty hash.
func TestModelParamsHashReadsTheFrozenRegistryHash(t *testing.T) {
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
	s := Store{DB: db}

	hash, found, err := s.ModelParamsHash(ctx, classify.AcademicRuleV1ID)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("%s is absent from the migrated registry", classify.AcademicRuleV1ID)
	}
	if hash != classify.AcademicRuleV1ParamsHash {
		t.Fatalf("registered params_hash is %q, want the frozen %q", hash, classify.AcademicRuleV1ParamsHash)
	}

	if _, found, err = s.ModelParamsHash(ctx, "modelo-inexistente-verificacion"); err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("an unregistered version was reported as present")
	}
}

func TestListUsesCatalogueTerritoryWithoutChangingPersistedLocation(t *testing.T) {
	if testing.Short() || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires migrated disposable risk database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := Store{DB: db}
	zones, err := s.ListZones(ctx, nil)
	if err != nil || len(zones) != 38 {
		t.Fatalf("catalogue count=%d err=%v", len(zones), err)
	}
	zones = append(zones, Zone{Code: "valparaiso-demo"})
	for _, zone := range zones {
		t.Run(zone.Code, func(t *testing.T) {
			event := v1.Event{ID: uuid.NewString(), JobID: uuid.NewString(), TraceID: uuid.NewString(), ActorID: "test", Type: v1.JobRequestedType, ZoneID: zone.Code, RequestedAt: time.Now().UTC().Add(-2 * time.Minute)}
			if err := s.Publish(ctx, event, Assessment{ModelVersion: classify.AcademicRuleV1ID, RiskLevel: "alto", Demo: true}); err != nil {
				t.Fatal(err)
			}
			var location string
			if err := db.QueryRow(ctx, `SELECT ST_AsGeoJSON(location) FROM batches WHERE job_id=$1`, event.JobID).Scan(&location); err != nil {
				t.Fatal(err)
			}
			var point struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal([]byte(location), &point); err != nil || point.Type != "Point" {
				t.Fatalf("persisted location changed: %s: %v", location, err)
			}
			want := string(zone.Geometry)
			if zone.Code == "valparaiso-demo" {
				want = location
			}
			for _, limit := range []int{1, 100} {
				batches, err := s.List(ctx, zone.Code, limit)
				if err != nil || len(batches) == 0 {
					t.Fatalf("List(%d): %v", limit, err)
				}
				for _, batch := range batches {
					if string(batch.Geometry) != want {
						t.Fatalf("List(%d) geometry=%s want=%s", limit, batch.Geometry, want)
					}
				}
			}
		})
	}
}

func TestListRefusesMissingCommuneGeometry(t *testing.T) {
	if testing.Short() || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires migrated disposable risk database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	// One connection keeps this session-local catalogue shadow isolated. The
	// committed catalogue and its geometry are never altered by this test.
	config.MaxConns = 1
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(ctx, `CREATE TEMP TABLE zones AS SELECT code,name,province,kind,NULL::geometry AS geometry FROM public.zones`); err != nil {
		t.Fatal(err)
	}
	s := Store{DB: db}
	event := v1.Event{ID: uuid.NewString(), JobID: uuid.NewString(), TraceID: uuid.NewString(), ActorID: "test", Type: v1.JobRequestedType, ZoneID: "05101", RequestedAt: time.Now().UTC().Add(-3 * time.Minute)}
	if err := s.Publish(ctx, event, Assessment{ModelVersion: classify.AcademicRuleV1ID, RiskLevel: "alto", Demo: true}); err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{1, 100} {
		batches, err := s.List(ctx, event.ZoneID, limit)
		if err == nil || !strings.Contains(err.Error(), "no catalogue geometry") || batches != nil {
			t.Fatalf("List(%d) invented missing territory: batches=%v err=%v", limit, batches, err)
		}
	}
}

// S1: a scheduled publication persists its batch through the same transaction as
// an explicitly requested one, but writes no completion event. Jobs never stored
// the job id a cycle mints, so a completion naming it is refused as an unknown
// job and dead-lettered permanently, once per zone per cycle; the risk API still
// reads the stored batch. The explicit job keeps exactly one completion, so
// closing the scheduled path did not weaken the requested-job lifecycle.
func TestScheduledPublicationPersistsWithoutACompletionEvent(t *testing.T) {
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
	s := Store{DB: db}
	assessment := Assessment{ModelVersion: classify.AcademicRuleV1ID, RiskLevel: "alto", Factors: []classify.Factor{}, Rationale: "scheduled publication", Demo: true}
	now := time.Now().UTC().Add(-time.Minute)
	scheduled := v1.Event{ID: uuid.NewString(), JobID: uuid.NewString(), TraceID: uuid.NewString(), ActorID: "cycle-scheduler", Type: v1.JobRequestedType, ZoneID: "valparaiso-demo", RequestedAt: now}
	if err = s.PublishScheduled(ctx, scheduled, assessment); err != nil {
		t.Fatalf("persist the scheduled batch: %v", err)
	}
	// A redelivery with the same event id is still stopped by the inbox gate, so
	// it adds nothing and still emits nothing.
	if err = s.PublishScheduled(ctx, scheduled, assessment); err != nil {
		t.Fatalf("redeliver the scheduled batch: %v", err)
	}
	var batches, completions int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM batches WHERE job_id=$1`, scheduled.JobID).Scan(&batches); err != nil || batches != 1 {
		t.Fatalf("scheduled batches=%d err=%v, want exactly 1", batches, err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE routing_key=$1 AND payload->>'job_id'=$2`, v1.BatchPublishedType, scheduled.JobID).Scan(&completions); err != nil || completions != 0 {
		t.Fatalf("scheduled completions=%d err=%v, want 0", completions, err)
	}
	explicit := scheduled
	explicit.ID = uuid.NewString()
	explicit.JobID = uuid.NewString()
	explicit.ActorID = "operator"
	if err = s.Publish(ctx, explicit, assessment); err != nil {
		t.Fatalf("persist the requested batch: %v", err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE routing_key=$1 AND payload->>'job_id'=$2`, v1.BatchPublishedType, explicit.JobID).Scan(&completions); err != nil || completions != 1 {
		t.Fatalf("requested completions=%d err=%v, want exactly 1", completions, err)
	}
}
