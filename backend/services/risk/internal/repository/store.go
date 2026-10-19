package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	v1 "firewatch.local/contracts/events/v1"
	"firewatch.local/risk/internal/classify"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ DB *pgxpool.Pool }

// BBox is a WGS84 bounding box, the envelope a caller may constrain the zone
// catalogue to. It is a plain value: the handler validates it and the store
// only turns it into SQL, so the coordinate policy lives in one place.
type BBox struct {
	MinLon, MinLat, MaxLon, MaxLat float64
}

// Zone is one row of the zone catalogue.
//
// Geometry carries a GeoJSON geometry object exactly as the database rendered
// it. A zone without geometry is a declared state, so such a row is excluded
// from ListZones instead of being handed back with an invented geometry.
type Zone struct {
	Code     string
	Name     string
	Province string
	Kind     string
	Geometry json.RawMessage
}

type Batch struct {
	ID           string    `json:"id"`
	JobID        string    `json:"job_id"`
	ZoneID       string    `json:"zone_id"`
	RequestedAt  time.Time `json:"requested_at"`
	PublishedAt  time.Time `json:"published_at"`
	ValidUntil   time.Time `json:"valid_until"`
	ModelVersion string    `json:"model_version"`
	Demo         bool      `json:"demo"`
	// Geometry is current catalogue territory, not an observed location or a
	// frozen historical boundary. Only valparaiso-demo uses an illustrative point.
	// The aggregate read (LatestPerZone) deliberately leaves it empty, and an
	// empty value is omitted from the response instead of being sent as null: that
	// answer carries no geometry at all, because the zones route already serves it.
	Geometry   json.RawMessage `json:"geometry,omitempty"`
	DataStatus string          `json:"data_status"`
	RiskLevel  string          `json:"risk_level"`
	Factors    json.RawMessage `json:"factors"`
	Rationale  string          `json:"rationale"`
	Notice     string          `json:"notice"`
	// Policy is the effective policy reference recorded with the assessment. A
	// nil pointer means no reference was recorded (a row written before
	// 003_policy_ref.sql), and it is served as SQL NULL; the store never invents
	// one.
	Policy *classify.PolicyRef `json:"policy"`
	// ScenarioID and EvaluatedAt are the optional synthetic demonstration
	// metadata of the originating job. Both are empty for a legacy row (and are
	// then omitted from the response), and both are preserved exactly when the
	// job carried a scenario. They are job identity, never classifier input.
	ScenarioID  string     `json:"scenario_id,omitempty"`
	EvaluatedAt *time.Time `json:"evaluated_at,omitempty"`
}

// Assessment is the persisted result of classifying one job request.
//
// Level, factors and rationale travel together because they are properties of a
// single evaluation, so they live on the batch and not on the model.
type Assessment struct {
	ModelVersion string
	RiskLevel    string
	Factors      []classify.Factor
	Rationale    string
	// Demo means the input was synthetic or demonstrative. The caller derives it
	// from the provenance of the provider that produced the data.
	Demo bool
	// Policy is the reference to the parameters that actually produced the level.
	// It is a pointer on purpose: nil means "no reference was recorded" and is
	// stored as SQL NULL, because the store never invents a reference it was not
	// given.
	Policy *classify.PolicyRef
}

// errMissingReference means a foreign key refused a batch because the referenced
// row does not exist: the zone is absent from the catalogue, or the model version
// is absent from the registry. Both are committed data, so no retry can make the
// referenced row appear by itself; the error declares itself permanent and the
// consumer dead-letters the event immediately instead of spending the whole retry
// budget on it and, with prefetch 1, blocking every message queued behind it.
//
// The constraint name travels in the message so a dead-letter is attributable
// from the log alone.
type errMissingReference struct{ constraint string }

func (e errMissingReference) Error() string {
	return "referenced row does not exist: constraint " + e.constraint
}
func (e errMissingReference) Permanent() bool { return true }

// classifyPublishError tags a foreign-key violation as permanent. SQLSTATE 23503
// is the only code treated this way, and it is checked explicitly instead of by
// matching a message: the referenced row is data that this write can never create
// on its own. Every other driver failure - connection, timeout, serialization,
// deadlock - stays transient, because a later attempt can succeed.
func classifyPublishError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return errMissingReference{constraint: pgErr.ConstraintName}
	}
	return err
}

// jobSettlement declares whether a persisted batch reports a job completion.
//
// It is a named value instead of a bare bool so the call sites state the
// contract: an explicitly requested job is SETTLED by its batch, while an
// automatic cycle's batch is only PERSISTED. The decision belongs to the method
// that owns the origin (Publish or PublishScheduled), never to a request field
// or an actor string, so a real delivery whose actor happens to read
// "cycle-scheduler" still settles its job and an automatic cycle never can.
type jobSettlement bool

const (
	// settleJob is the explicitly requested job origin: the batch completes the
	// job the consumer is holding, so exactly one completion event is emitted.
	settleJob jobSettlement = true
	// persistOnly is the internal automatic cycle origin: the batch is stored for
	// the risk API and the freshness panel, but no completion is emitted, because
	// Jobs never stored the job id the cycle minted.
	persistOnly jobSettlement = false
)

// Publish persists one assessed batch and its outbox event for an explicitly
// requested job.
//
// The transaction shape is deliberate: the inbox row is the deduplication gate,
// the batches insert is idempotent on job_id, and the outbox event is written in
// the same transaction as the batch so a published event can never name a batch
// that was not stored. When the batch already exists the delivery is not
// swallowed: it emits a completion for the stored batch (see
// redispatchCompletion) so an operator re-dispatch can still settle the job.
func (s Store) Publish(ctx context.Context, request v1.Event, assessment Assessment) error {
	return s.persist(ctx, request, assessment, settleJob)
}

// PublishScheduled persists one assessed batch produced by the internal
// automatic cycle, WITHOUT the job completion Publish emits.
//
// An automatic cycle is not a job: Jobs never stored a row for the id the cycle
// mints, so a risk.batch.published.v1 naming it is refused by the jobs consumer
// as an unknown job and dead-lettered permanently - one dead letter per zone per
// cycle, forever, which is the defect this method closes. The batch itself is
// persisted through the same transaction as an explicit publication, with the
// same inbox deduplication, the same idempotent insert on job_id and the same
// scenario metadata, so nothing but the completion changes.
//
// Suppression is scoped to this method, not to the consumer: Jobs keeps
// rejecting an unknown job id, and Store.Complete's unknown-job protection is
// untouched. A missing or malformed completion is still a defect; an automatic
// cycle simply never produces one.
func (s Store) PublishScheduled(ctx context.Context, request v1.Event, assessment Assessment) error {
	return s.persist(ctx, request, assessment, persistOnly)
}

// persist is the single transaction both publication origins share. Only the
// completion step depends on the settlement: the inbox gate, the idempotent
// batches insert and the metadata are identical for a requested job and an
// automatic cycle.
func (s Store) persist(ctx context.Context, request v1.Event, assessment Assessment, settlement jobSettlement) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `INSERT INTO inbox(id) VALUES($1) ON CONFLICT DO NOTHING`, request.ID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}
	// Factors are marshalled explicitly so the jsonb parameter is unambiguous. A
	// nil slice would marshal to the JSON literal null, which is not a list of
	// factors; the column is read back as an assessment's factors payload, so an
	// empty list is stored instead.
	factors := assessment.Factors
	if factors == nil {
		factors = []classify.Factor{}
	}
	encoded, err := json.Marshal(factors)
	if err != nil {
		return fmt.Errorf("marshal factors: %w", err)
	}
	// The policy reference is marshalled only when present. A nil reference stays
	// a SQL NULL rather than being replaced by a default: inventing one would
	// claim the level was produced by parameters nobody recorded.
	var policy any
	if assessment.Policy != nil {
		encodedPolicy, err := json.Marshal(assessment.Policy)
		if err != nil {
			return fmt.Errorf("marshal policy ref: %w", err)
		}
		policy = string(encodedPolicy)
	}
	// The scenario metadata is inserted only when the job carried it. An empty
	// scenario id or a nil instant stays SQL NULL, because the store never invents
	// a scenario or an evaluation instant it was not given, and migration 007's
	// pairing constraint refuses a half-recorded pair.
	var scenarioID, evaluatedAt any
	if request.ScenarioID != "" {
		scenarioID = request.ScenarioID
	}
	if request.EvaluatedAt != nil {
		evaluatedAt = *request.EvaluatedAt
	}
	batchID := uuid.NewString()
	result, err = tx.Exec(ctx, `INSERT INTO batches(id,job_id,zone_id,requested_at,valid_until,model_version,demo,risk_level,factors,rationale,policy_ref,scenario_id,evaluated_at,location)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,ST_SetSRID(ST_MakePoint(-71.6127,-33.0472),4326)) ON CONFLICT(job_id) DO NOTHING`, batchID, request.JobID, request.ZoneID, request.RequestedAt, request.RequestedAt.Add(15*time.Minute), assessment.ModelVersion, assessment.Demo, assessment.RiskLevel, string(encoded), assessment.Rationale, policy, scenarioID, evaluatedAt)
	if err != nil {
		// An unknown zone (batches_zone_id_fkey) or an unregistered model version
		// (batches_model_version_fkey) is refused here. Both are deterministic
		// reference failures, so the error is classified instead of returned raw.
		return classifyPublishError(err)
	}
	if result.RowsAffected() == 0 {
		// The batch is already stored for this job, so this delivery is an operator
		// re-dispatch: it carries a fresh event id and the inbox gate above could not
		// recognise it. Completing the job depends on the completion that names the
		// stored batch, and a silent commit would repair nothing while the job stays
		// stuck. The id is read rather than invented - a completion must never name a
		// batch that was not stored - and a read that sees no row is returned as an
		// error so the delivery is retried instead of committing an empty result.
		//
		// An automatic cycle never reaches that repair: it mints a fresh job id per
		// zone, so a conflict here is a repeated scheduled delivery and there is no
		// job to settle. The inbox row above is committed so the duplicate stays a
		// no-op instead of dead-lettering a completion nobody requested.
		if settlement == persistOnly {
			return tx.Commit(ctx)
		}
		var existingBatchID string
		if err = tx.QueryRow(ctx, `SELECT id::text FROM batches WHERE job_id=$1`, request.JobID).Scan(&existingBatchID); err != nil {
			return err
		}
		event := redispatchCompletion(request, existingBatchID)
		body, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO outbox(id,routing_key,payload) VALUES($1,$2,$3)`, event.ID, event.Type, body); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if settlement == persistOnly {
		// The batch is stored, and that is all an automatic cycle reports. Emitting
		// the completion would hand the jobs consumer an event naming a job row it
		// never created, which it refuses and dead-letters permanently.
		return tx.Commit(ctx)
	}
	event := request
	event.ID = uuid.NewString()
	event.Type = v1.BatchPublishedType
	event.BatchID = batchID
	event.OccurredAt = time.Now().UTC()
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO outbox(id,routing_key,payload) VALUES($1,$2,$3)`, event.ID, event.Type, body); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// redispatchCompletion builds the risk.batch.published.v1 completion for a job
// whose batch is already stored. It exists because an operator replay (S6)
// republishes jobs.requested.v1 under a fresh event id: the inbox gate cannot
// recognise it, the batch insert then affects zero rows, and without this event
// the completion that settles the job is never sent. The completion names the
// batch that already exists - minting a new id would report a batch that was
// never stored - and copies the request envelope verbatim apart from the fresh
// event id, the type, the batch id and the occurrence instant. It is pure and
// split out so that shape is testable without a database.
func redispatchCompletion(request v1.Event, existingBatchID string) v1.Event {
	event := request
	event.ID = uuid.NewString()
	event.Type = v1.BatchPublishedType
	event.BatchID = existingBatchID
	event.OccurredAt = time.Now().UTC()
	return event
}

// jobFailureEvent builds the risk.job.failed.v1 report for a give-up from the
// delivery that failed. It carries the job id, the trace id, the classified
// reason and the attempt count the consumer spent. It is pure and split out so the
// record's shape is testable without a database; the generated id is used as both
// the event id and the outbox row id, exactly as the batch publication event does
// it.
func jobFailureEvent(delivery v1.Event, reason string, attempts int) v1.Event {
	return v1.Event{
		ID:            uuid.NewString(),
		Type:          v1.JobFailedType,
		OccurredAt:    time.Now().UTC(),
		TraceID:       delivery.TraceID,
		JobID:         delivery.JobID,
		FailureReason: reason,
		Attempts:      attempts,
	}
}

// RecordJobFailure writes the cross-service terminal-failure report into this
// service's outbox. risk does not own the job row, so it reports the failure to
// the jobs consumer instead of writing it. It is one INSERT into the same outbox
// the batch publication uses, so the existing relay publishes it with publisher
// confirms and mandatory routing and nothing is published from the consumer hot
// path. A failed insert is the caller's to log, and it never invents evidence: the
// dead letter stays the record.
func (s Store) RecordJobFailure(ctx context.Context, delivery v1.Event, reason string, attempts int) error {
	report := jobFailureEvent(delivery, reason, attempts)
	body, err := json.Marshal(report)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO outbox(id,routing_key,payload) VALUES($1,$2,$3)`, report.ID, report.Type, body)
	return err
}

// ZoneExists reports whether the zone catalogue holds code.
//
// Membership is decided by the data in zones, never by a literal in Go: the
// read path uses this instead of comparing against a hardcoded zone. A
// well-formed unknown code is simply absent, not an error; a driver failure is
// returned so the caller can distinguish "no such zone" from "cannot answer".
func (s Store) ZoneExists(ctx context.Context, code string) (bool, error) {
	var exists bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM zones WHERE code=$1)`, code).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// ListZones reads the zone catalogue as GeoJSON-ready rows, ordered by code so
// the response is stable across calls.
//
// Rows without geometry are excluded: the catalogue may legitimately lack a
// geometry for a zone, and the response must not invent one. A nil bbox asks
// for the whole catalogue; a non-nil bbox filters with ST_Intersects so the
// spatial index is used. An empty result is a valid answer, not an error.
func (s Store) ListZones(ctx context.Context, bbox *BBox) ([]Zone, error) {
	query := `SELECT code, name, province, kind, ST_AsGeoJSON(geometry) FROM zones WHERE geometry IS NOT NULL`
	args := make([]any, 0, 4)
	if bbox != nil {
		query += ` AND ST_Intersects(geometry, ST_MakeEnvelope($1,$2,$3,$4,4326))`
		args = append(args, bbox.MinLon, bbox.MinLat, bbox.MaxLon, bbox.MaxLat)
	}
	query += ` ORDER BY code`
	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	zones := make([]Zone, 0)
	for rows.Next() {
		var z Zone
		var geometry string
		// province is nullable in the schema even though the seeded communes
		// always carry one. A missing province is read as the empty string
		// rather than failing the whole catalogue read.
		var province *string
		if err := rows.Scan(&z.Code, &z.Name, &province, &z.Kind, &geometry); err != nil {
			return nil, err
		}
		if province != nil {
			z.Province = *province
		}
		z.Geometry = json.RawMessage(geometry)
		zones = append(zones, z)
	}
	return zones, rows.Err()
}

// ModelParamsHash reads the params_hash recorded for a model version in the
// registry, and reports whether that version exists at all.
//
// It exists so the startup path can confront the running classifier with the
// registry row it will be written against: a version that is registered in Go but
// absent from - or divergent in - the models table would otherwise start healthy
// and then discard every job. A version that exists with no recorded hash (a
// placeholder, whose parameter set is not reconstructible) is reported as
// ("", true) rather than as absent; the caller decides whether an empty hash is
// acceptable for that identifier. A missing row is ("", false, nil), not an
// error, because "no such version" is an answer and not a failure to answer.
func (s Store) ModelParamsHash(ctx context.Context, version string) (string, bool, error) {
	var hash *string
	err := s.DB.QueryRow(ctx, `SELECT params_hash FROM models WHERE version=$1`, version).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if hash == nil {
		return "", true, nil
	}
	return *hash, true, nil
}

func (s Store) List(ctx context.Context, zone string, limit int) ([]Batch, error) {
	rows, err := s.DB.Query(ctx, `SELECT b.id::text,b.job_id::text,b.zone_id,b.requested_at,b.published_at,b.valid_until,b.model_version,b.demo,b.risk_level,coalesce(b.factors, '[]'::jsonb),coalesce(b.rationale, ''),coalesce(b.policy_ref, 'null'::jsonb),b.scenario_id,b.evaluated_at,ST_AsGeoJSON(CASE WHEN b.zone_id='valparaiso-demo' THEN b.location ELSE z.geometry END) FROM batches b JOIN zones z ON z.code=b.zone_id WHERE b.zone_id=$1 ORDER BY b.requested_at DESC,b.id DESC LIMIT $2`, zone, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	batches := make([]Batch, 0)
	for rows.Next() {
		var b Batch
		var factors, policy string
		var geometry *string
		var scenario *string
		if err := rows.Scan(&b.ID, &b.JobID, &b.ZoneID, &b.RequestedAt, &b.PublishedAt, &b.ValidUntil, &b.ModelVersion, &b.Demo, &b.RiskLevel, &factors, &b.Rationale, &policy, &scenario, &b.EvaluatedAt, &geometry); err != nil {
			return nil, err
		}
		if err := b.readAssessment(factors, policy, scenario); err != nil {
			return nil, err
		}
		if geometry == nil {
			return nil, fmt.Errorf("zone %s has no catalogue geometry", b.ZoneID)
		}
		b.Geometry = json.RawMessage(*geometry)
		batches = append(batches, b)
	}
	return batches, rows.Err()
}

// LatestPerZone reads the newest published batch of every zone in one query.
//
// It exists so a map of the whole territory can be painted on load without one
// request per zone. DISTINCT ON (b.zone_id) keeps exactly one row per zone; the
// ORDER BY decides which one - newest requested_at first, with the batch id as a
// deterministic tie-break - and then orders the answer by zone_id so the response
// is stable across calls. Because the ORDER BY leads with zone_id, a zone with no
// published batch simply produces no row, so it is absent from the answer instead
// of being served an invented level.
//
// The catalogue geometry is deliberately NOT read here: the aggregate response
// carries no geometry at all. The zones route already serves every commune
// polygon, and repeating them in this answer would be payload no caller needs.
// For the same reason a zone whose catalogue geometry is missing cannot make this
// read fail, which is the opposite of List - the join stays because a batch is
// only meaningful for a catalogue zone, but nothing of the zone row is selected.
//
// An empty result is a valid answer, not an error.
func (s Store) LatestPerZone(ctx context.Context) ([]Batch, error) {
	rows, err := s.DB.Query(ctx, `SELECT DISTINCT ON (b.zone_id) b.id::text,b.job_id::text,b.zone_id,b.requested_at,b.published_at,b.valid_until,b.model_version,b.demo,b.risk_level,coalesce(b.factors, '[]'::jsonb),coalesce(b.rationale, ''),coalesce(b.policy_ref, 'null'::jsonb),b.scenario_id,b.evaluated_at FROM batches b JOIN zones z ON z.code=b.zone_id ORDER BY b.zone_id,b.requested_at DESC,b.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	batches := make([]Batch, 0)
	for rows.Next() {
		var b Batch
		var factors, policy string
		var scenario *string
		if err := rows.Scan(&b.ID, &b.JobID, &b.ZoneID, &b.RequestedAt, &b.PublishedAt, &b.ValidUntil, &b.ModelVersion, &b.Demo, &b.RiskLevel, &factors, &b.Rationale, &policy, &scenario, &b.EvaluatedAt); err != nil {
			return nil, err
		}
		if err := b.readAssessment(factors, policy, scenario); err != nil {
			return nil, err
		}
		batches = append(batches, b)
	}
	return batches, rows.Err()
}

// readAssessment completes a batch from the column values both read paths share:
// the optional evaluation instant is canonicalised to UTC, the JSON columns are
// kept verbatim, the recorded policy reference is decoded, and the derived
// data_status and notice are computed from the persisted assessment instead of
// being read from storage, so they cannot drift from the level they describe.
//
// It is shared rather than copied because a second copy of these rules is a
// second definition of what a batch means: the per-zone read and the aggregate
// read must serve the same element shape or the map would validate the two
// differently.
func (b *Batch) readAssessment(factors, policy string, scenario *string) error {
	// Only the optional synthetic evaluation instant is canonicalised. pgx may
	// decode a timestamptz in the connection's location, and the read-back must
	// expose the same instant in UTC; requested_at and published_at keep their
	// stored representation untouched.
	if b.EvaluatedAt != nil {
		utc := b.EvaluatedAt.UTC()
		b.EvaluatedAt = &utc
	}
	b.Factors = json.RawMessage(factors)
	if scenario != nil {
		b.ScenarioID = *scenario
	}
	// A stored reference that cannot be read is a defect, not something to
	// swallow: the row claims a policy it can no longer reproduce. Only the
	// SQL NULL literal means "no reference was recorded".
	if policy != "null" {
		var ref classify.PolicyRef
		if err := json.Unmarshal([]byte(policy), &ref); err != nil {
			return fmt.Errorf("unmarshal policy ref: %w", err)
		}
		b.Policy = &ref
	}
	b.DataStatus = "current"
	if !time.Now().Before(b.ValidUntil) {
		b.DataStatus = "stale"
	}
	// Notice is derived from the persisted assessment rather than stored: a
	// stored notice could drift from the level it explains.
	switch {
	case b.Demo:
		b.Notice = "Synthetic demonstration input; no real observations were used."
	case b.RiskLevel == string(classify.LevelDesconocido):
		b.Notice = "Insufficient data: the risk level could not be determined."
	}
	return nil
}
