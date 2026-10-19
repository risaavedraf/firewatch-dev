package repository

import (
	"context"
	"os"
	"testing"
	"time"

	v1 "firewatch.local/contracts/events/v1"
	"firewatch.local/risk/internal/classify"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestLatestPerZoneAnswersTheNewestBatchOfEveryZone pins the aggregate read's
// selection rule against a real migrated database by comparing its answer with
// what the database itself reports, so the proof holds in any database state.
//
// Every element must BE the newest stored batch of its zone - the selection rule,
// asked of storage with the same ORDER BY the read claims to implement - and the
// answer must hold exactly one element per catalogue zone that has a published
// batch. That second equality carries the absence rule: a zone with a published
// batch cannot be missing, and a catalogue zone with no published batch cannot
// appear, so neither case needs a zone in any particular state. The answer is then
// checked to be ordered by zone_id, without geometry, because the frozen design
// serves geometry from the zones route.
//
// The rules cannot be proven with a fake: DISTINCT ON, the tie-break on
// requested_at and the ordering are properties of the SQL, so the fake store in
// the handler suite would only restate whatever the test handed it. The zone is
// read from the catalogue for the same reason the read path asks ZoneExists
// instead of comparing literals: membership is data.
//
// It deliberately does NOT require a catalogue zone that has never published. That
// state is not reachable in the project's own integration database - its other
// suites publish for the seeded zones - so a test that demands it can only skip,
// and a skip is not a pass.
//
// It needs the disposable migrated risk database the other integration tests use,
// so it skips without TEST_DATABASE_URL and runs in `make integration`. It writes
// rows and does not clean them, exactly as its neighbours do.
func TestLatestPerZoneAnswersTheNewestBatchOfEveryZone(t *testing.T) {
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

	// The zone that will hold several batches. Its code comes from zones, so a
	// database whose seed differs still exercises the rule.
	var batched string
	if err := db.QueryRow(ctx, `SELECT code FROM zones ORDER BY code LIMIT 1`).Scan(&batched); err != nil {
		t.Fatalf("the zone catalogue is empty, so there is no zone to publish for: %v", err)
	}

	// The three batches must be the newest ones this zone has, whatever the
	// database already holds from earlier runs, so the reference instant is the
	// newest requested_at already stored for the zone rather than the wall clock
	// alone. Without this the "newest of the three" claim would depend on state
	// the test does not own.
	//
	// It is truncated to microseconds because that is the resolution of a
	// PostgreSQL timestamptz: a nanosecond-precision time.Time comes back from
	// storage truncated, and an exact instant comparison against the untruncated
	// value would fail on a difference the read cannot preserve.
	newestRequestedAt := time.Now().UTC().Truncate(time.Microsecond)
	var storedNewest *time.Time
	if err := db.QueryRow(ctx, `SELECT max(requested_at) FROM batches WHERE zone_id = $1`, batched).Scan(&storedNewest); err != nil {
		t.Fatalf("read the newest stored requested_at of zone %q: %v", batched, err)
	}
	if storedNewest != nil && !storedNewest.Before(newestRequestedAt) {
		newestRequestedAt = storedNewest.Add(time.Minute).UTC()
	}

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
	}
	// Three batches for one zone, inserted in an order that is not the answer:
	// the newest requested_at is written in the middle, so "the newest batch" and
	// "the last row this test wrote" cannot be conflated.
	newestJobID := uuid.NewString()
	for _, requestedAt := range []time.Time{
		newestRequestedAt.Add(-time.Minute),
		newestRequestedAt,
		newestRequestedAt.Add(-2 * time.Hour),
	} {
		event := v1.Event{
			ID:          uuid.NewString(),
			JobID:       uuid.NewString(),
			TraceID:     uuid.NewString(),
			ActorID:     "test",
			Type:        v1.JobRequestedType,
			ZoneID:      batched,
			Demo:        true,
			RequestedAt: requestedAt,
		}
		if requestedAt.Equal(newestRequestedAt) {
			newestJobID = event.JobID
		}
		if err := s.Publish(ctx, event, assessment); err != nil {
			t.Fatalf("publish a batch requested at %s: %v", requestedAt, err)
		}
	}

	batches, err := s.LatestPerZone(ctx)
	if err != nil {
		t.Fatalf("LatestPerZone: %v", err)
	}

	// The selection rule, element by element: the aggregate answered what the
	// database itself calls the newest batch of that zone. Asking storage the same
	// question the read claims to answer holds for every zone in the database, so
	// it needs no zone without a batch and no knowledge of what the seed holds.
	for _, batch := range batches {
		var storedNewestID string
		if err := db.QueryRow(ctx, `SELECT id::text FROM batches WHERE zone_id = $1 ORDER BY requested_at DESC, id DESC LIMIT 1`, batch.ZoneID).Scan(&storedNewestID); err != nil {
			t.Fatalf("read the newest stored batch of zone %q: %v", batch.ZoneID, err)
		}
		if batch.ID != storedNewestID {
			t.Fatalf("zone %q answered batch %q, but its newest stored batch is %q", batch.ZoneID, batch.ID, storedNewestID)
		}
	}

	// The absence rule, as an equality: one element per catalogue zone that has a
	// published batch. The expected number is computed by the database, so a zone
	// with a published batch cannot be missing from the answer and a catalogue
	// zone without one cannot appear in it.
	var zonesWithBatches int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM (SELECT DISTINCT zone_id FROM batches WHERE zone_id IN (SELECT code FROM zones)) t`).Scan(&zonesWithBatches); err != nil {
		t.Fatalf("count the catalogue zones that have a published batch: %v", err)
	}
	if len(batches) != zonesWithBatches {
		t.Fatalf("got %d elements for the %d catalogue zones that have a published batch", len(batches), zonesWithBatches)
	}

	// The shape of the answer: ordered by zone_id, one element per zone, and no
	// geometry - the map reads territory from the zones route.
	seen := make(map[string]int, len(batches))
	for index, batch := range batches {
		seen[batch.ZoneID]++
		if seen[batch.ZoneID] > 1 {
			t.Fatalf("zone %q appears more than once, want exactly one element per zone", batch.ZoneID)
		}
		if len(batch.Geometry) != 0 {
			t.Fatalf("zone %q answered %s, but the aggregate carries no geometry", batch.ZoneID, batch.Geometry)
		}
		if index > 0 && batches[index-1].ZoneID >= batch.ZoneID {
			t.Fatalf("elements are not ordered by zone_id: %q before %q", batches[index-1].ZoneID, batch.ZoneID)
		}
	}

	// The case the three inserts set up, named explicitly: the zone under test
	// answered the newest of its three batches, not the oldest and not the row
	// written last.
	var element *Batch
	for index := range batches {
		if batches[index].ZoneID == batched {
			element = &batches[index]
			break
		}
	}
	if element == nil {
		t.Fatalf("zone %q has a published batch but the aggregate did not answer it", batched)
	}
	if element.JobID != newestJobID || !element.RequestedAt.Equal(newestRequestedAt) {
		t.Fatalf("zone %q answered job %q requested at %s, want the newest of the three: job %q requested at %s",
			batched, element.JobID, element.RequestedAt, newestJobID, newestRequestedAt)
	}
	// The read derives the same presentation members the per-zone read does, so an
	// aggregate element is usable by the same fail-closed client check.
	if element.RiskLevel != "alto" || len(element.Factors) == 0 || element.Rationale == "" {
		t.Fatalf("the aggregate element lost its assessment members: %+v", element)
	}
	if element.DataStatus != "current" && element.DataStatus != "stale" {
		t.Fatalf("the aggregate element carries data_status %q", element.DataStatus)
	}
}
