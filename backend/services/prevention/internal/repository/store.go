// Package repository owns the prevention persistence: the frozen action tables,
// the append-only evidence/review/reassessment tables, and the guarded
// transactions that advance an action or append to its evidence trail.
package repository

import (
	"context"
	"errors"
	"time"

	evidenceRules "firewatch.local/prevention/internal/evidence"
	"firewatch.local/prevention/internal/state"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ DB *pgxpool.Pool }

// ErrActionNotFound means no action holds the requested id. The transport maps it
// to 404 and nothing is written.
var ErrActionNotFound = errors.New("preventive action not found")

// ErrInvalidTransition means the action exists but the frozen state machine does
// not allow the requested move from its current status. It is decided inside the
// transaction that holds the row, so the transport maps it to 409 without the
// race a read-then-write check would have, and the transaction rolls back with
// no status change and no history row.
var ErrInvalidTransition = errors.New("invalid preventive action transition")

// ErrEvidenceInsufficient means the target state is reachable but the current
// evidence snapshot does not carry a sufficient human review. It is decided in
// the same row-locked transaction as the transition, so a refusal writes nothing.
var ErrEvidenceInsufficient = errors.New("preventive action evidence is insufficient")

// ErrEvidenceState means the action is terminal (aceptada or cancelada), so its
// evidence set is frozen and no further snapshot or review may be appended.
var ErrEvidenceState = errors.New("preventive action evidence state is terminal")

// ErrEvidenceNotFound means no evidence snapshot holds the requested id for the
// addressed action.
var ErrEvidenceNotFound = errors.New("preventive action evidence not found")

// ErrStaleEvidence means a review named an evidence snapshot that is no longer
// the current one. Recording new evidence invalidates earlier approvals, so a
// review of a replaced snapshot is refused instead of counted.
var ErrStaleEvidence = errors.New("preventive action evidence snapshot is stale")

// ErrReassessmentUnavailable means no published batch exists for the named job
// yet, which is a retryable condition rather than a fabricated association.
var ErrReassessmentUnavailable = errors.New("preventive reassessment batch is not published")

// ErrReassessmentNotSynthetic means the batch is real data, not a synthetic
// classification, and S2 only authorizes synthetic reassessments.
var ErrReassessmentNotSynthetic = errors.New("preventive reassessment batch is not synthetic")

// ErrReassessmentCommuneMismatch means the batch belongs to another commune and
// must never be linked to this action.
var ErrReassessmentCommuneMismatch = errors.New("preventive reassessment batch belongs to another commune")

// ErrReassessmentNotNew means the batch was produced before the current evidence
// review, so linking it would relabel an older classification as a reassessment.
var ErrReassessmentNotNew = errors.New("preventive reassessment batch is not later than the reviewed evidence")

// ErrReassessmentAlreadyLinked means the job is already linked to an action. A
// job is one real publication and can never be relabelled under another action.
var ErrReassessmentAlreadyLinked = errors.New("preventive reassessment job is already linked")

// Action is one row of prevention.actions as it appears on the wire.
type Action struct {
	ID string `json:"id"`
	// community_code is the community the action belongs to; the contract does
	// not model a separate communities table.
	CommunityCode string `json:"community_code"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	// Priority is free text: the contract names the column and defines no
	// vocabulary for it. Empty means "not stated".
	Priority string `json:"priority"`
	// Status is one of the eight frozen states.
	Status string `json:"status"`
	// Assignee and DueDate are absent until someone sets them; DueDate is
	// rendered as a plain calendar date (YYYY-MM-DD) because it is a date, not
	// an instant, and a time of day would be a fabricated precision.
	Assignee  *string   `json:"assignee"`
	DueDate   *string   `json:"due_date"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// History is one row of prevention.action_history: who moved an action from one
// status to another, when, and why. It is the durable audit record of the
// service and is never updated or deleted.
type History struct {
	ID         int64     `json:"id"`
	ActionID   string    `json:"action_id"`
	FromStatus string    `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	ActorSub   string    `json:"actor_sub"`
	Note       string    `json:"note"`
	CreatedAt  time.Time `json:"created_at"`
}

// Evidence is one admin-recorded snapshot of the references a reviewer is asked
// to judge, plus the sufficiency criterion that snapshot was recorded against.
// Rows are append-only: a later snapshot never edits an earlier one, so a review
// always names exactly the evidence it saw.
type Evidence struct {
	ID         int64     `json:"id"`
	ActionID   string    `json:"action_id"`
	Criterion  string    `json:"criterion"`
	References []string  `json:"references"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
}

// EvidenceReview is one attributable human decision about one evidence snapshot.
// The reviewer and the date are taken from the verified token subject and the
// server clock; neither is client-supplied.
type EvidenceReview struct {
	ID          int64     `json:"id"`
	ActionID    string    `json:"action_id"`
	EvidenceID  int64     `json:"evidence_id"`
	Decision    string    `json:"decision"`
	ReviewerSub string    `json:"reviewer_sub"`
	Note        string    `json:"note"`
	ReviewedAt  time.Time `json:"reviewed_at"`
}

// Reassessment is the link between an action and one REAL synthetic batch that a
// later job published. Every classification field is derived server-side from the
// shared risk database; the client only names the job.
type Reassessment struct {
	ID               int64     `json:"id"`
	ActionID         string    `json:"action_id"`
	JobID            string    `json:"job_id"`
	BatchID          string    `json:"batch_id"`
	Version          string    `json:"version"`
	RiskLevel        string    `json:"risk_level"`
	ZoneID           string    `json:"zone_id"`
	Synthetic        bool      `json:"synthetic"`
	CausalProof      bool      `json:"causal_proof"`
	BatchPublishedAt time.Time `json:"batch_published_at"`
	LinkedBy         string    `json:"linked_by"`
	LinkedAt         time.Time `json:"linked_at"`
}

// Detail is an action with its full attributable history and the append-only
// evidence, review and reassessment records that the detail route renders.
type Detail struct {
	Action
	History            []History        `json:"history"`
	Evidence           []Evidence       `json:"evidence"`
	EvidenceReviews    []EvidenceReview `json:"evidence_reviews"`
	Reassessments      []Reassessment   `json:"reassessments"`
	EvidenceSufficient bool             `json:"evidence_sufficient"`
}

// NewAction is the validated create command. Status is deliberately absent: the
// contract fixes a new action at propuesta, so a client cannot choose it.
type NewAction struct {
	CommunityCode string
	Title         string
	Description   string
	Priority      string
	Assignee      *string
	DueDate       *string
	CreatedBy     string
}

// NewEvidence is the validated evidence snapshot to append.
type NewEvidence struct {
	Criterion  string
	References []string
	CreatedBy  string
}

// NewReview is the validated human decision to append.
type NewReview struct {
	Decision    string
	ReviewerSub string
	Note        string
}

// actionColumns is the single projection every read shares, so a column cannot be
// selected in one query and forgotten in another. due_date is rendered in SQL so
// the wire format is a calendar date regardless of the driver's time zone.
const actionColumns = `id::text,community_code,title,description,priority,status,assignee,to_char(due_date,'YYYY-MM-DD'),created_by,created_at,updated_at`

// scanAction reads the shared projection in the shared ordered list.
func scanAction(row pgx.Row) (Action, error) {
	var a Action
	err := row.Scan(&a.ID, &a.CommunityCode, &a.Title, &a.Description, &a.Priority, &a.Status, &a.Assignee, &a.DueDate, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

// Create inserts one action in the initial propuesta state. The id is generated
// here, so it is known before the row exists and cannot collide with a
// client-supplied value. Creation writes no history row: history records
// transitions, and a creation is not a transition from another status.
func (s Store) Create(ctx context.Context, input NewAction) (Action, error) {
	action, err := scanAction(s.DB.QueryRow(ctx, `INSERT INTO prevention.actions
		(id,community_code,title,description,priority,status,assignee,due_date,created_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING `+actionColumns,
		uuid.NewString(), input.CommunityCode, input.Title, input.Description, input.Priority,
		string(state.Propuesta), input.Assignee, input.DueDate, input.CreatedBy))
	if err != nil {
		return Action{}, err
	}
	return action, nil
}

// defaultListLimit bounds a list read. The contract documents no page size, so
// the bound is stated here rather than assumed by the caller.
const defaultListLimit = 200

// List returns the actions of one community, newest first. An empty
// communityCode is not a filter, an empty status is not a filter: the contract
// writes both query parameters as optional filters, so an omitted one narrows
// nothing instead of matching nothing. The result slice is never nil, so an empty
// result marshals as [] rather than null.
func (s Store) List(ctx context.Context, communityCode, status string) ([]Action, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+actionColumns+` FROM prevention.actions
		WHERE ($1='' OR community_code=$1) AND ($2='' OR status=$2)
		ORDER BY created_at DESC, id DESC LIMIT $3`, communityCode, status, defaultListLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	actions := make([]Action, 0)
	for rows.Next() {
		action, err := scanAction(rows)
		if err != nil {
			return nil, err
		}
		actions = append(actions, action)
	}
	return actions, rows.Err()
}

// Get returns one action with its history, evidence snapshots, human reviews and
// linked reassessments in chronological order. History is ordered by the identity
// sequence, which is the order the transitions were accepted in, not by a
// timestamp that could tie within one transaction.
func (s Store) Get(ctx context.Context, id string) (Detail, error) {
	action, err := scanAction(s.DB.QueryRow(ctx, `SELECT `+actionColumns+` FROM prevention.actions WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, ErrActionNotFound
	}
	if err != nil {
		return Detail{}, err
	}
	detail := Detail{
		Action:          action,
		History:         make([]History, 0),
		Evidence:        make([]Evidence, 0),
		EvidenceReviews: make([]EvidenceReview, 0),
		Reassessments:   make([]Reassessment, 0),
	}
	rows, err := s.DB.Query(ctx, `SELECT id,action_id::text,from_status,to_status,actor_sub,note,created_at
		FROM prevention.action_history WHERE action_id=$1 ORDER BY id`, id)
	if err != nil {
		return Detail{}, err
	}
	for rows.Next() {
		var h History
		if err := rows.Scan(&h.ID, &h.ActionID, &h.FromStatus, &h.ToStatus, &h.ActorSub, &h.Note, &h.CreatedAt); err != nil {
			rows.Close()
			return Detail{}, err
		}
		detail.History = append(detail.History, h)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Detail{}, err
	}
	rows.Close()

	if detail.Evidence, err = s.listEvidence(ctx, id); err != nil {
		return Detail{}, err
	}
	if detail.EvidenceReviews, err = s.listReviews(ctx, id); err != nil {
		return Detail{}, err
	}
	if detail.Reassessments, err = s.listReassessments(ctx, id); err != nil {
		return Detail{}, err
	}
	detail.EvidenceSufficient = currentSufficiency(detail.Evidence, detail.EvidenceReviews)
	return detail, nil
}

// currentSufficiency is the single rule the detail read and the acceptance guard
// share: the newest evidence snapshot must carry a sufficient review of exactly
// that snapshot. A review of an older snapshot is stale and never counts.
func currentSufficiency(evidence []Evidence, reviews []EvidenceReview) bool {
	latestEvidenceID := int64(0)
	if len(evidence) > 0 {
		latestEvidenceID = evidence[len(evidence)-1].ID
	}
	var latest *evidenceRules.Review
	if len(reviews) > 0 {
		newest := reviews[len(reviews)-1]
		latest = &evidenceRules.Review{EvidenceID: newest.EvidenceID, Decision: evidenceRules.Decision(newest.Decision)}
	}
	return evidenceRules.Sufficient(latestEvidenceID, latest)
}

// Transition advances one action to `to`, recording who did it and why.
//
// The whole decision happens in one transaction that first locks the action row
// FOR UPDATE and then asks the frozen state machine whether the move is allowed.
// Two properties follow from that order, and both are part of the contract:
//
//   - The answer cannot be stale. A concurrent transition either waits for the
//     lock and then sees the new status, or has committed first and is seen here.
//   - An invalid transition writes nothing. The status update and the history
//     row are only reached after the move is accepted, and the deferred rollback
//     discards both otherwise, so a refused 409 leaves the action exactly as it
//     was and appends no history.
func (s Store) Transition(ctx context.Context, id, to, actorSub, note string) (Action, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Action{}, err
	}
	defer tx.Rollback(ctx)
	var current string
	err = tx.QueryRow(ctx, `SELECT status FROM prevention.actions WHERE id=$1 FOR UPDATE`, id).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return Action{}, ErrActionNotFound
	}
	if err != nil {
		return Action{}, err
	}
	if !state.CanTransition(state.State(current), state.State(to)) {
		return Action{}, ErrInvalidTransition
	}
	// Accepting an action is the one edge whose precondition is not the state
	// machine alone. It is evaluated here, under the same row lock, so a concurrent
	// evidence change either commits first (and is seen) or waits and cannot
	// satisfy the guard behind this decision. A refusal returns before any write,
	// so the status, updated_at and history stay untouched.
	if state.State(to) == state.Aceptada {
		sufficient, err := s.sufficientForAcceptance(ctx, tx, id)
		if err != nil {
			return Action{}, err
		}
		if !sufficient {
			return Action{}, ErrEvidenceInsufficient
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE prevention.actions SET status=$2, updated_at=now() WHERE id=$1`, id, to); err != nil {
		return Action{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO prevention.action_history(action_id,from_status,to_status,actor_sub,note) VALUES($1,$2,$3,$4,$5)`,
		id, current, to, actorSub, note); err != nil {
		return Action{}, err
	}
	// The updated row is read inside the transaction, so the response describes
	// exactly the version this transition created and never a later one.
	action, err := scanAction(tx.QueryRow(ctx, `SELECT `+actionColumns+` FROM prevention.actions WHERE id=$1`, id))
	if err != nil {
		return Action{}, err
	}
	return action, tx.Commit(ctx)
}

// queryer is the read surface shared by the pool and a transaction, so one list
// helper serves the detail read and the guarded transaction without duplicating
// SQL.
type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// evidenceColumns is the single projection every evidence read shares.
const evidenceColumns = `id,action_id::text,criterion,evidence_references,created_by,created_at`

// reviewColumns is the single projection every review read shares.
const reviewColumns = `id,action_id::text,evidence_id,decision,reviewer_sub,note,reviewed_at`

// reassessmentColumns is the single projection every link read shares.
const reassessmentColumns = `id,action_id::text,job_id::text,batch_id::text,version,risk_level,zone_id,synthetic,batch_published_at,linked_by,linked_at`

func scanEvidence(row pgx.Row) (Evidence, error) {
	var e Evidence
	err := row.Scan(&e.ID, &e.ActionID, &e.Criterion, &e.References, &e.CreatedBy, &e.CreatedAt)
	return e, err
}

func scanReview(row pgx.Row) (EvidenceReview, error) {
	var r EvidenceReview
	err := row.Scan(&r.ID, &r.ActionID, &r.EvidenceID, &r.Decision, &r.ReviewerSub, &r.Note, &r.ReviewedAt)
	return r, err
}

func scanReassessment(row pgx.Row) (Reassessment, error) {
	var r Reassessment
	err := row.Scan(&r.ID, &r.ActionID, &r.JobID, &r.BatchID, &r.Version, &r.RiskLevel, &r.ZoneID, &r.Synthetic, &r.BatchPublishedAt, &r.LinkedBy, &r.LinkedAt)
	return r, err
}

func queryEvidence(ctx context.Context, q queryer, actionID string) ([]Evidence, error) {
	rows, err := q.Query(ctx, `SELECT `+evidenceColumns+` FROM prevention.action_evidence WHERE action_id=$1 ORDER BY id`, actionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Evidence, 0)
	for rows.Next() {
		e, err := scanEvidence(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func queryReviews(ctx context.Context, q queryer, actionID string) ([]EvidenceReview, error) {
	rows, err := q.Query(ctx, `SELECT `+reviewColumns+` FROM prevention.action_evidence_review WHERE action_id=$1 ORDER BY id`, actionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]EvidenceReview, 0)
	for rows.Next() {
		r, err := scanReview(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func queryReassessments(ctx context.Context, q queryer, actionID string) ([]Reassessment, error) {
	rows, err := q.Query(ctx, `SELECT `+reassessmentColumns+` FROM prevention.action_reassessment WHERE action_id=$1 ORDER BY id`, actionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Reassessment, 0)
	for rows.Next() {
		r, err := scanReassessment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s Store) listEvidence(ctx context.Context, actionID string) ([]Evidence, error) {
	return queryEvidence(ctx, s.DB, actionID)
}

func (s Store) listReviews(ctx context.Context, actionID string) ([]EvidenceReview, error) {
	return queryReviews(ctx, s.DB, actionID)
}

func (s Store) listReassessments(ctx context.Context, actionID string) ([]Reassessment, error) {
	return queryReassessments(ctx, s.DB, actionID)
}

// currentEvidence returns the newest evidence snapshot of the action, or false
// when none exists.
func currentEvidence(ctx context.Context, q queryer, actionID string) (Evidence, bool, error) {
	e, err := scanEvidence(q.QueryRow(ctx, `SELECT `+evidenceColumns+` FROM prevention.action_evidence WHERE action_id=$1 ORDER BY id DESC LIMIT 1`, actionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Evidence{}, false, nil
	}
	if err != nil {
		return Evidence{}, false, err
	}
	return e, true, nil
}

// newestReview returns the latest human decision of the action, or nil when none
// exists.
func newestReview(ctx context.Context, q queryer, actionID string) (*EvidenceReview, error) {
	r, err := scanReview(q.QueryRow(ctx, `SELECT `+reviewColumns+` FROM prevention.action_evidence_review WHERE action_id=$1 ORDER BY id DESC LIMIT 1`, actionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// sufficientForAcceptance evaluates the acceptance guard with the row already
// locked by the caller.
func (s Store) sufficientForAcceptance(ctx context.Context, q queryer, actionID string) (bool, error) {
	current, ok, err := currentEvidence(ctx, q, actionID)
	if err != nil || !ok {
		return false, err
	}
	newest, err := newestReview(ctx, q, actionID)
	if err != nil {
		return false, err
	}
	var latest *evidenceRules.Review
	if newest != nil {
		latest = &evidenceRules.Review{EvidenceID: newest.EvidenceID, Decision: evidenceRules.Decision(newest.Decision)}
	}
	return evidenceRules.Sufficient(current.ID, latest), nil
}

// AddEvidence appends one evidence snapshot to a non-terminal action. Nothing is
// updated: earlier snapshots and reviews stay exactly as recorded, and a later
// review of the new snapshot is what makes it current.
func (s Store) AddEvidence(ctx context.Context, actionID string, input NewEvidence) (Evidence, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Evidence{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM prevention.actions WHERE id=$1 FOR UPDATE`, actionID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Evidence{}, ErrActionNotFound
	}
	if err != nil {
		return Evidence{}, err
	}
	if state.Terminal(state.State(status)) {
		return Evidence{}, ErrEvidenceState
	}
	added, err := scanEvidence(tx.QueryRow(ctx, `INSERT INTO prevention.action_evidence(action_id,criterion,evidence_references,created_by)
		VALUES($1,$2,$3,$4) RETURNING `+evidenceColumns,
		actionID, input.Criterion, input.References, input.CreatedBy))
	if err != nil {
		return Evidence{}, err
	}
	return added, tx.Commit(ctx)
}

// ReviewEvidence appends one human decision about the current evidence snapshot.
// A decision about an older snapshot is refused as stale rather than recorded and
// ignored, so the audit trail never holds a misleading approval.
func (s Store) ReviewEvidence(ctx context.Context, actionID string, evidenceID int64, input NewReview) (EvidenceReview, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return EvidenceReview{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM prevention.actions WHERE id=$1 FOR UPDATE`, actionID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return EvidenceReview{}, ErrActionNotFound
	}
	if err != nil {
		return EvidenceReview{}, err
	}
	if state.Terminal(state.State(status)) {
		return EvidenceReview{}, ErrEvidenceState
	}
	current, ok, err := currentEvidence(ctx, tx, actionID)
	if err != nil {
		return EvidenceReview{}, err
	}
	if !ok {
		return EvidenceReview{}, ErrEvidenceNotFound
	}
	if current.ID != evidenceID {
		return EvidenceReview{}, ErrStaleEvidence
	}
	review, err := scanReview(tx.QueryRow(ctx, `INSERT INTO prevention.action_evidence_review(action_id,evidence_id,decision,reviewer_sub,note)
		VALUES($1,$2,$3,$4,$5) RETURNING `+reviewColumns,
		actionID, evidenceID, input.Decision, input.ReviewerSub, input.Note))
	if err != nil {
		return EvidenceReview{}, err
	}
	return review, tx.Commit(ctx)
}

// LinkReassessment persists the link between an action and one REAL published
// synthetic batch. Every classification field is read from the shared risk
// database; the caller can only name the job. The cutoff includes the latest
// human review as well as the latest evidence snapshot, so a batch produced
// before the review can never masquerade as the later reassessment.
func (s Store) LinkReassessment(ctx context.Context, actionID, jobID, linkedBy string) (Reassessment, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Reassessment{}, err
	}
	defer tx.Rollback(ctx)
	var communityCode string
	var actionCreatedAt time.Time
	err = tx.QueryRow(ctx, `SELECT community_code,created_at FROM prevention.actions WHERE id=$1 FOR UPDATE`, actionID).Scan(&communityCode, &actionCreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Reassessment{}, ErrActionNotFound
	}
	if err != nil {
		return Reassessment{}, err
	}
	latestEvidenceAt, err := newestEvidenceInstant(ctx, tx, actionID)
	if err != nil {
		return Reassessment{}, err
	}
	latestReviewedAt, err := newestReviewInstant(ctx, tx, actionID)
	if err != nil {
		return Reassessment{}, err
	}
	cutoff := evidenceRules.Cutoff(actionCreatedAt, latestEvidenceAt, latestReviewedAt)

	var batchID, zoneID, version, riskLevel string
	var synthetic bool
	var requestedAt, publishedAt time.Time
	var evaluatedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT id::text,zone_id,model_version,risk_level,demo,requested_at,published_at,evaluated_at
		FROM batches WHERE job_id=$1`, jobID).
		Scan(&batchID, &zoneID, &version, &riskLevel, &synthetic, &requestedAt, &publishedAt, &evaluatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Reassessment{}, ErrReassessmentUnavailable
	}
	if err != nil {
		return Reassessment{}, err
	}
	if !synthetic {
		return Reassessment{}, ErrReassessmentNotSynthetic
	}
	if zoneID != communityCode {
		return Reassessment{}, ErrReassessmentCommuneMismatch
	}
	// requested_at is the job submission, evaluated_at the server-assigned
	// synthetic evaluation instant and published_at the publication. All three
	// must be at or after the cutoff; a batch that predates the reviewed evidence
	// cannot be relabelled as a later reassessment.
	if requestedAt.Before(cutoff) || publishedAt.Before(cutoff) {
		return Reassessment{}, ErrReassessmentNotNew
	}
	if evaluatedAt != nil && evaluatedAt.Before(cutoff) {
		return Reassessment{}, ErrReassessmentNotNew
	}
	link, err := scanReassessment(tx.QueryRow(ctx, `INSERT INTO prevention.action_reassessment(action_id,job_id,batch_id,version,risk_level,zone_id,synthetic,batch_published_at,linked_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+reassessmentColumns,
		actionID, jobID, batchID, version, riskLevel, zoneID, synthetic, publishedAt, linkedBy))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Reassessment{}, ErrReassessmentAlreadyLinked
		}
		return Reassessment{}, err
	}
	return link, tx.Commit(ctx)
}

func newestEvidenceInstant(ctx context.Context, q queryer, actionID string) (time.Time, error) {
	var instant time.Time
	err := q.QueryRow(ctx, `SELECT created_at FROM prevention.action_evidence WHERE action_id=$1 ORDER BY id DESC LIMIT 1`, actionID).Scan(&instant)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}
	return instant, err
}

func newestReviewInstant(ctx context.Context, q queryer, actionID string) (time.Time, error) {
	var instant time.Time
	err := q.QueryRow(ctx, `SELECT reviewed_at FROM prevention.action_evidence_review WHERE action_id=$1 ORDER BY id DESC LIMIT 1`, actionID).Scan(&instant)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}
	return instant, err
}
