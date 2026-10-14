package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	v1 "firewatch.local/contracts/events/v1"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ DB *pgxpool.Pool }

// canonicalEvaluatedAt re-expresses the optional synthetic evaluation instant in
// UTC. pgx may decode a timestamptz into the connection's location, so the scan
// result is canonicalised without shifting the instant; every other timestamp
// (requested_at and historical columns) is returned exactly as stored.
func canonicalEvaluatedAt(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}

// jobColumns is the single projection shared by Create and Get so the stored
// metadata can never be read in one place and forgotten in the other.
const jobColumns = `id::text,zone_id,state,batch_id::text,requested_at,scenario_id,evaluated_at`

type Job struct {
	ID          string    `json:"id"`
	ZoneID      string    `json:"zone_id"`
	State       string    `json:"state"`
	BatchID     *string   `json:"batch_id"`
	RequestedAt time.Time `json:"requested_at"`
	// Optional synthetic metadata. Both are absent on historical jobs and are
	// omitted from the wire so a legacy job keeps exactly its previous shape.
	ScenarioID  *string    `json:"scenario_id,omitempty"`
	EvaluatedAt *time.Time `json:"evaluated_at,omitempty"`
}
type Audit struct {
	ActorID    string    `json:"actor_id"`
	Action     string    `json:"action"`
	JobID      string    `json:"job_id"`
	TraceID    string    `json:"trace_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

// requestEvent builds the jobs.requested.v1 envelope FROM the stored row, so the
// event and the job can never disagree about the scenario or its instant. A
// legacy job carries neither optional field.
func requestEvent(job Job, actor, zone, trace string) v1.Event {
	event := v1.Event{ID: uuid.NewString(), Type: v1.JobRequestedType, OccurredAt: job.RequestedAt, RequestedAt: job.RequestedAt, TraceID: trace, JobID: job.ID, ActorID: actor, ZoneID: zone, Demo: true}
	if job.ScenarioID != nil {
		event.ScenarioID = *job.ScenarioID
	}
	if job.EvaluatedAt != nil {
		instant := *job.EvaluatedAt
		event.EvaluatedAt = &instant
	}
	return event
}

// Create is idempotent per (actor, idempotency_key). The optional scenario is
// variadic so legacy callers keep compiling. A scenario-bearing job gets one
// server-assigned UTC instant; on a conflict the STORED row is returned with its
// original scenario and instant, so a retry with a different scenario neither
// replaces the job nor writes a second outbox row.
func (s Store) Create(ctx context.Context, actor, key, zone, trace string, scenarioID ...string) (Job, error) {
	scenario := ""
	if len(scenarioID) > 0 {
		scenario = scenarioID[0]
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(ctx)
	id := uuid.NewString()
	var job Job
	// Typed pointers make the legacy NULLs explicit to the driver instead of
	// relying on INSERT target-column inference for untyped nil.
	var scenarioArg *string
	var evaluatedArg *time.Time
	if scenario != "" {
		instant := time.Now().UTC()
		scenarioArg, evaluatedArg = &scenario, &instant
	}
	err = tx.QueryRow(ctx, `INSERT INTO jobs(id, actor_id, idempotency_key, zone_id, state, scenario_id, evaluated_at)
		VALUES($1,$2,$3,$4,'queued',$5,$6) ON CONFLICT(actor_id,idempotency_key) DO NOTHING
		RETURNING `+jobColumns, id, actor, key, zone, scenarioArg, evaluatedArg).Scan(&job.ID, &job.ZoneID, &job.State, &job.BatchID, &job.RequestedAt, &job.ScenarioID, &job.EvaluatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs WHERE actor_id=$1 AND idempotency_key=$2`, actor, key).Scan(&job.ID, &job.ZoneID, &job.State, &job.BatchID, &job.RequestedAt, &job.ScenarioID, &job.EvaluatedAt)
		if err != nil {
			return Job{}, err
		}
		job.EvaluatedAt = canonicalEvaluatedAt(job.EvaluatedAt)
		return job, tx.Commit(ctx)
	}
	if err != nil {
		return Job{}, err
	}
	job.EvaluatedAt = canonicalEvaluatedAt(job.EvaluatedAt)
	event := requestEvent(job, actor, zone, trace)
	body, err := json.Marshal(event)
	if err != nil {
		return Job{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO outbox(id,routing_key,payload) VALUES($1,$2,$3)`, event.ID, event.Type, body); err != nil {
		return Job{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit(actor_id,action,job_id,trace_id) VALUES($1,'job.requested',$2,$3)`, actor, job.ID, trace); err != nil {
		return Job{}, err
	}
	return job, tx.Commit(ctx)
}

func (s Store) Get(ctx context.Context, id string) (Job, error) {
	var job Job
	err := s.DB.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id=$1`, id).Scan(&job.ID, &job.ZoneID, &job.State, &job.BatchID, &job.RequestedAt, &job.ScenarioID, &job.EvaluatedAt)
	if err != nil {
		return job, err
	}
	job.EvaluatedAt = canonicalEvaluatedAt(job.EvaluatedAt)
	return job, nil
}

func (s Store) Audit(ctx context.Context) ([]Audit, error) {
	rows, err := s.DB.Query(ctx, `SELECT actor_id,action,job_id::text,trace_id::text,occurred_at FROM audit ORDER BY id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Audit, 0)
	for rows.Next() {
		var a Audit
		if err := rows.Scan(&a.ActorID, &a.Action, &a.JobID, &a.TraceID, &a.OccurredAt); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

// errNoMatchingJob means the published batch does not match the stored job: a
// different zone, actor or batch id, or an unknown job. The job row is committed
// before its event is published, so no retry can make it match; the error
// declares itself permanent and the consumer dead-letters it immediately. The
// job id travels in the message so a dead-letter is attributable from the log
// alone.
type errNoMatchingJob struct{ jobID string }

func (e errNoMatchingJob) Error() string   { return "result does not match a known job: " + e.jobID }
func (e errNoMatchingJob) Permanent() bool { return true }

func (s Store) Complete(ctx context.Context, event v1.Event) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `INSERT INTO inbox(id) VALUES($1) ON CONFLICT DO NOTHING`, event.ID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}
	result, err = tx.Exec(ctx, `UPDATE jobs SET state='published',batch_id=$2 WHERE id=$1 AND actor_id=$3 AND zone_id=$4 AND state='queued'`, event.JobID, event.BatchID, event.ActorID, event.ZoneID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 1 {
		_, err = tx.Exec(ctx, `INSERT INTO audit(actor_id,action,job_id,trace_id) VALUES($1,'batch.published',$2,$3)`, event.ActorID, event.JobID, event.TraceID)
		if err != nil {
			return err
		}
	} else {
		var matches bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE id=$1 AND batch_id=$2 AND actor_id=$3 AND zone_id=$4 AND state='published')`, event.JobID, event.BatchID, event.ActorID, event.ZoneID).Scan(&matches)
		if err != nil {
			return err
		}
		if !matches {
			return errNoMatchingJob{jobID: event.JobID}
		}
	}
	return tx.Commit(ctx)
}
