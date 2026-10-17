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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTransactionalIdempotencyAndAudit(t *testing.T) {
	if testing.Short() || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires migrated disposable jobs database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := Store{DB: db}
	actor, key, trace := uuid.NewString(), uuid.NewString(), uuid.NewString()
	a, err := s.Create(ctx, actor, key, "valparaiso-demo", trace)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Create(ctx, actor, key, "valparaiso-demo", trace)
	if err != nil || a.ID != b.ID {
		t.Fatalf("idempotency: %v", err)
	}
	var count int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE payload->>'job_id'=$1`, a.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("outbox count=%d err=%v", count, err)
	}
	e := v1.Event{ID: uuid.NewString(), JobID: a.ID, BatchID: uuid.NewString(), ActorID: actor, ZoneID: "valparaiso-demo", TraceID: "bad-trace"}
	if err = s.Complete(ctx, e); err == nil {
		t.Fatal("invalid audit must roll back completion")
	}
	j, err := s.Get(ctx, a.ID)
	if err != nil || j.State != "queued" {
		t.Fatalf("rollback failed: %+v %v", j, err)
	}
	e.TraceID = trace
	if err = s.Complete(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err = s.Complete(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM audit WHERE job_id=$1`, a.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("audit count=%d err=%v", count, err)
	}
}

func TestCompleteEnforcesStoredZoneRelationship(t *testing.T) {
	if testing.Short() || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires migrated disposable jobs database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := Store{DB: db}
	actor, key, trace := uuid.NewString(), uuid.NewString(), uuid.NewString()
	job, err := s.Create(ctx, actor, key, "valparaiso-demo", trace)
	if err != nil {
		t.Fatal(err)
	}
	// Demo=false on purpose: the origin flag must not block completion.
	foreign := v1.Event{ID: uuid.NewString(), Type: v1.BatchPublishedType, JobID: job.ID, BatchID: uuid.NewString(), ActorID: actor, TraceID: trace, ZoneID: "otra-zona", RequestedAt: time.Now(), OccurredAt: time.Now(), Demo: false}
	err = s.Complete(ctx, foreign)
	if err == nil {
		t.Fatal("a batch from another zone must not complete the job")
	}
	var permanent interface{ Permanent() bool }
	if !errors.As(err, &permanent) || !permanent.Permanent() {
		t.Fatalf("err=%v must declare itself permanent", err)
	}
	stored, err := s.Get(ctx, job.ID)
	if err != nil || stored.State != "queued" {
		t.Fatalf("job state=%+v err=%v, want queued", stored, err)
	}
	var publishedAudits int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM audit WHERE job_id=$1 AND action='batch.published'`, job.ID).Scan(&publishedAudits); err != nil {
		t.Fatal(err)
	}
	if publishedAudits != 0 {
		t.Fatalf("published audits=%d want=0", publishedAudits)
	}
	// The job's own zone completes it, even with Demo=false.
	own := foreign
	own.ID = uuid.NewString()
	own.ZoneID = job.ZoneID
	if err = s.Complete(ctx, own); err != nil {
		t.Fatal(err)
	}
	stored, err = s.Get(ctx, job.ID)
	if err != nil || stored.State != "published" {
		t.Fatalf("job state=%+v err=%v, want published", stored, err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM audit WHERE job_id=$1 AND action='batch.published'`, job.ID).Scan(&publishedAudits); err != nil {
		t.Fatal(err)
	}
	if publishedAudits != 1 {
		t.Fatalf("published audits=%d want=1", publishedAudits)
	}
}

// TestSyntheticScenarioMetadataIsAssignedOnceAndSurvivesRetries requires a real
// migrated database: it proves the server-assigned instant, the conflict
// semantics and the outbox contents, none of which a structural test can show.
// Without TEST_DATABASE_URL it skips, and the runnable command is documented.
func TestSyntheticScenarioMetadataIsAssignedOnceAndSurvivesRetries(t *testing.T) {
	if testing.Short() || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires migrated disposable jobs database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := Store{DB: db}
	actor, key, trace := uuid.NewString(), uuid.NewString(), uuid.NewString()
	first, err := s.Create(ctx, actor, key, "valparaiso-demo", trace, "dry-windy")
	if err != nil {
		t.Fatal(err)
	}
	if first.ScenarioID == nil || *first.ScenarioID != "dry-windy" || first.EvaluatedAt == nil || first.EvaluatedAt.IsZero() {
		t.Fatalf("scenario metadata missing on the stored job: %+v", first)
	}
	if first.EvaluatedAt.Location() != time.UTC {
		t.Fatalf("instant location=%v want UTC", first.EvaluatedAt.Location())
	}
	if first.EvaluatedAt.After(time.Now().Add(time.Minute)) {
		t.Fatalf("assigned instant %v is not a server assignment", first.EvaluatedAt)
	}
	// The emitted request event carries the exact stored metadata, not a second
	// clock read.
	var payload []byte
	if err = db.QueryRow(ctx, `SELECT payload FROM outbox WHERE payload->>'job_id'=$1`, first.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var event v1.Event
	if err = json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	if event.ScenarioID != "dry-windy" || event.EvaluatedAt == nil || !event.EvaluatedAt.Equal(*first.EvaluatedAt) {
		t.Fatalf("event metadata=%q %v, want the stored %q %v", event.ScenarioID, event.EvaluatedAt, "dry-windy", first.EvaluatedAt)
	}
	// A retry with a DIFFERENT scenario must return the original job, scenario and
	// instant, and must not add a second outbox row.
	second, err := s.Create(ctx, actor, key, "valparaiso-demo", trace, "mild")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.ScenarioID == nil || *second.ScenarioID != "dry-windy" || second.EvaluatedAt == nil || !second.EvaluatedAt.Equal(*first.EvaluatedAt) {
		t.Fatalf("conflict replaced the stored metadata: %+v", second)
	}
	// The conflict return is a second Create boundary: it too must expose the
	// stored instant in UTC without shifting it.
	if second.EvaluatedAt.Location() != time.UTC {
		t.Fatalf("retry instant location=%v want UTC", second.EvaluatedAt.Location())
	}
	// A legacy retry on the same key must also return the original scenario job.
	legacyRetry, err := s.Create(ctx, actor, key, "valparaiso-demo", trace)
	if err != nil || legacyRetry.ID != first.ID || legacyRetry.ScenarioID == nil || *legacyRetry.ScenarioID != "dry-windy" {
		t.Fatalf("legacy retry replaced the stored metadata: %+v %v", legacyRetry, err)
	}
	var outboxRows int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE payload->>'job_id'=$1`, first.ID).Scan(&outboxRows); err != nil || outboxRows != 1 {
		t.Fatalf("outbox rows=%d err=%v, want exactly 1", outboxRows, err)
	}
	// A legacy request stores no metadata and its event omits both fields.
	legacyJob, err := s.Create(ctx, uuid.NewString(), uuid.NewString(), "valparaiso-demo", trace)
	if err != nil {
		t.Fatal(err)
	}
	if legacyJob.ScenarioID != nil || legacyJob.EvaluatedAt != nil {
		t.Fatalf("legacy job stored metadata: %+v", legacyJob)
	}
	if err = db.QueryRow(ctx, `SELECT payload FROM outbox WHERE payload->>'job_id'=$1`, legacyJob.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "scenario_id") || strings.Contains(string(payload), "evaluated_at") {
		t.Fatalf("legacy event must omit metadata: %s", payload)
	}
	// Completion must not wipe the metadata, and orphan batches must not either.
	if err = s.Complete(ctx, v1.Event{ID: uuid.NewString(), Type: v1.BatchPublishedType, JobID: first.ID, BatchID: uuid.NewString(), ActorID: actor, ZoneID: "valparaiso-demo", TraceID: trace, RequestedAt: first.RequestedAt, OccurredAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Get(ctx, first.ID)
	if err != nil || stored.State != "published" || stored.ScenarioID == nil || *stored.ScenarioID != "dry-windy" || stored.EvaluatedAt == nil || !stored.EvaluatedAt.Equal(*first.EvaluatedAt) {
		t.Fatalf("completion did not preserve metadata: %+v %v", stored, err)
	}
	// Get is the read boundary: it must expose the same canonical UTC instant it
	// returned from Create, not a re-located copy.
	if stored.EvaluatedAt.Location() != time.UTC {
		t.Fatalf("Get instant location=%v want UTC", stored.EvaluatedAt.Location())
	}
	// The pairing constraint is the schema's half of the rule: a scenario without
	// its instant cannot be stored even by a direct write.
	if _, err = db.Exec(ctx, `INSERT INTO jobs(id,actor_id,idempotency_key,zone_id,state,scenario_id) VALUES($1,$2,$3,'valparaiso-demo','queued','mild')`, uuid.NewString(), uuid.NewString(), uuid.NewString()); err == nil {
		t.Fatal("pairing constraint accepted a scenario without an evaluation instant")
	}
}

// TestReplayEventUsesTheStoredIdentity is the pure seam for the replay's side
// effect: it pins that the re-dispatched request event is built from the stored
// row and the STORED actor and zone, so the operator who triggered the replay
// cannot change the identity Complete matches on. It needs no database; the
// transactional write and the persisted rows are covered below and skip without
// TEST_DATABASE_URL.
func TestReplayEventUsesTheStoredIdentity(t *testing.T) {
	instant := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	job := Job{ID: "job", ZoneID: "valparaiso-demo", State: "queued", RequestedAt: instant, ScenarioID: stringPtr("dry-windy"), EvaluatedAt: &instant}
	event := replayEvent(job, "stored-actor", "operator-trace")
	if event.Type != v1.JobRequestedType || event.JobID != job.ID || event.ActorID != "stored-actor" || event.ZoneID != job.ZoneID || event.TraceID != "operator-trace" {
		t.Fatalf("replay event identity: %+v", event)
	}
	if event.ID == "" {
		t.Fatal("replay event must mint a fresh id")
	}
	if event.ScenarioID != "dry-windy" || event.EvaluatedAt == nil || !event.EvaluatedAt.Equal(instant) {
		t.Fatalf("replay event metadata: %+v", event)
	}
}

// TestReplayRestoresFailedJobAndEnqueuesOnce requires a real migrated database:
// it proves the guarded transition, the cleared terminal fields, the new outbox
// row and its audit record, and that a second replay is refused with no further
// effect. It skips without TEST_DATABASE_URL, so the transaction, the SQL and the
// persisted rows stay unproven in an environment without PostgreSQL — exactly the
// database-side rules no pure seam can show.
func TestReplayRestoresFailedJobAndEnqueuesOnce(t *testing.T) {
	if testing.Short() || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires migrated disposable jobs database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := Store{DB: db}
	actor, key, trace := uuid.NewString(), uuid.NewString(), uuid.NewString()
	job, err := s.Create(ctx, actor, key, "valparaiso-demo", trace, "dry-windy")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Fail(ctx, job.ID, "permanent_error", 5); err != nil {
		t.Fatal(err)
	}
	failed, err := s.Get(ctx, job.ID)
	if err != nil || failed.State != "failed" || failed.Attempts != 5 || failed.FailureReason == nil || failed.FailedAt == nil {
		t.Fatalf("failed row=%+v err=%v", failed, err)
	}
	// The original request event is the only outbox row before the replay.
	before := map[string]bool{}
	rows, err := db.Query(ctx, `SELECT id::text FROM outbox WHERE payload->>'job_id'=$1`, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var outboxID string
		if err = rows.Scan(&outboxID); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		before[outboxID] = true
	}
	rows.Close()
	if len(before) != 1 {
		t.Fatalf("outbox before=%v want exactly 1", before)
	}

	operator := "operator-subject"
	replayed, err := s.Replay(ctx, job.ID, operator, trace, false)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ID != job.ID || replayed.State != "queued" || replayed.Attempts != 0 || replayed.FailureReason != nil || replayed.FailedAt != nil {
		t.Fatalf("replayed job=%+v", replayed)
	}
	stored, err := s.Get(ctx, job.ID)
	if err != nil || stored.State != "queued" || stored.Attempts != 0 || stored.FailureReason != nil || stored.FailedAt != nil {
		t.Fatalf("stored job=%+v err=%v", stored, err)
	}
	// Exactly one new outbox row: the outgoing request event rebuilt from the
	// stored row with the STORED actor and zone, so it still matches Complete.
	var newPayload []byte
	rows, err = db.Query(ctx, `SELECT id::text,payload FROM outbox WHERE payload->>'job_id'=$1`, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for rows.Next() {
		var outboxID string
		var payload []byte
		if err = rows.Scan(&outboxID, &payload); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		count++
		if !before[outboxID] {
			newPayload = payload
		}
	}
	rows.Close()
	if count != 2 || newPayload == nil {
		t.Fatalf("outbox after replay=%d new=%s", count, newPayload)
	}
	var event v1.Event
	if err = json.Unmarshal(newPayload, &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != v1.JobRequestedType || event.JobID != job.ID || event.ActorID != actor || event.ZoneID != job.ZoneID || event.ScenarioID != "dry-windy" {
		t.Fatalf("replay event=%+v", event)
	}
	// The audit names the operator and mirrors the action creation writes.
	var operatorAudits int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM audit WHERE job_id=$1 AND action='job.requested' AND actor_id=$2`, job.ID, operator).Scan(&operatorAudits); err != nil {
		t.Fatal(err)
	}
	if operatorAudits != 1 {
		t.Fatalf("operator audits=%d want 1", operatorAudits)
	}
	// A losing or concurrent second replay is refused and enqueues nothing. The
	// job is queued now, so re-dispatching it needs the operator's confirmation;
	// an unconfirmed second replay is exactly that refusal.
	if _, err = s.Replay(ctx, job.ID, operator, uuid.NewString(), false); !errors.Is(err, ErrReplayConfirmationRequired) {
		t.Fatalf("second replay err=%v want ErrReplayConfirmationRequired", err)
	}
	var after int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE payload->>'job_id'=$1`, job.ID).Scan(&after); err != nil || after != 2 {
		t.Fatalf("outbox after second replay=%d err=%v want 2", after, err)
	}
	// An unknown id is distinguishable from a non-failed one.
	if _, err = s.Replay(ctx, uuid.NewString(), operator, uuid.NewString(), false); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unknown id err=%v want pgx.ErrNoRows", err)
	}
}

// TestReplayQueuedJobRequiresConfirmation requires a real migrated database: it
// proves the confirmed queued transition, that an unconfirmed one writes nothing,
// that the observation instant is left alone, and that a published job is still
// refused. It skips without TEST_DATABASE_URL, so those database-side rules stay
// unproven in an environment without PostgreSQL.
func TestReplayQueuedJobRequiresConfirmation(t *testing.T) {
	if testing.Short() || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires migrated disposable jobs database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := Store{DB: db}
	actor, key, trace := uuid.NewString(), uuid.NewString(), uuid.NewString()
	job, err := s.Create(ctx, actor, key, "valparaiso-demo", trace, "dry-windy")
	if err != nil {
		t.Fatal(err)
	}
	operator := "operator-subject"
	// Unconfirmed: the queued job is left exactly as it was, and nothing is
	// written - no outbox row and no audit record for the operator.
	if _, err = s.Replay(ctx, job.ID, operator, uuid.NewString(), false); !errors.Is(err, ErrReplayConfirmationRequired) {
		t.Fatalf("unconfirmed queued replay err=%v want ErrReplayConfirmationRequired", err)
	}
	var outboxRows, audits int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE payload->>'job_id'=$1`, job.ID).Scan(&outboxRows); err != nil || outboxRows != 1 {
		t.Fatalf("outbox after refused replay=%d err=%v want 1", outboxRows, err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM audit WHERE job_id=$1 AND actor_id=$2`, job.ID, operator).Scan(&audits); err != nil || audits != 0 {
		t.Fatalf("audits after refused replay=%d err=%v want 0", audits, err)
	}
	// Confirmed: the row is re-dispatched. The attempt budget restarts and the
	// observation instant does NOT move, because nobody observed it again.
	replayed, err := s.Replay(ctx, job.ID, operator, uuid.NewString(), true)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.State != "queued" || replayed.Attempts != 0 || replayed.FailureReason != nil || replayed.FailedAt != nil {
		t.Fatalf("replayed queued job=%+v", replayed)
	}
	if !replayed.RequestedAt.Equal(job.RequestedAt) || replayed.ScenarioID == nil || *replayed.ScenarioID != "dry-windy" {
		t.Fatalf("replay moved the observation instant or lost the scenario: %+v want requested_at=%v", replayed, job.RequestedAt)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE payload->>'job_id'=$1`, job.ID).Scan(&outboxRows); err != nil || outboxRows != 2 {
		t.Fatalf("outbox after confirmed replay=%d err=%v want 2", outboxRows, err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM audit WHERE job_id=$1 AND actor_id=$2`, job.ID, operator).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audits after confirmed replay=%d err=%v want 1", audits, err)
	}
	// The extension opened `queued`, not `published`: a completed job keeps its
	// outcome and is still refused, with or without a confirmation.
	completion := v1.Event{ID: uuid.NewString(), Type: v1.BatchPublishedType, JobID: job.ID, BatchID: uuid.NewString(), ActorID: actor, ZoneID: job.ZoneID, TraceID: trace}
	if err = s.Complete(ctx, completion); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Replay(ctx, job.ID, operator, uuid.NewString(), true); !errors.Is(err, ErrJobNotReplayable) {
		t.Fatalf("replay of a published job err=%v want ErrJobNotReplayable", err)
	}
}
