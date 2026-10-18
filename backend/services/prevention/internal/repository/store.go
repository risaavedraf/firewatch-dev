// Package repository owns the prevention persistence: the two tables of the
// frozen contract and the one guarded transaction that advances an action.
package repository

import (
	"context"
	"errors"
	"time"

	"firewatch.local/prevention/internal/state"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// Detail is an action with its full attributable history, oldest first.
type Detail struct {
	Action
	History []History `json:"history"`
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

// Get returns one action with its history in chronological order. History is
// ordered by the identity sequence, which is the order the transitions were
// accepted in, not by a timestamp that could tie within one transaction.
func (s Store) Get(ctx context.Context, id string) (Detail, error) {
	action, err := scanAction(s.DB.QueryRow(ctx, `SELECT `+actionColumns+` FROM prevention.actions WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, ErrActionNotFound
	}
	if err != nil {
		return Detail{}, err
	}
	rows, err := s.DB.Query(ctx, `SELECT id,action_id::text,from_status,to_status,actor_sub,note,created_at
		FROM prevention.action_history WHERE action_id=$1 ORDER BY id`, id)
	if err != nil {
		return Detail{}, err
	}
	defer rows.Close()
	detail := Detail{Action: action, History: make([]History, 0)}
	for rows.Next() {
		var h History
		if err := rows.Scan(&h.ID, &h.ActionID, &h.FromStatus, &h.ToStatus, &h.ActorSub, &h.Note, &h.CreatedAt); err != nil {
			return Detail{}, err
		}
		detail.History = append(detail.History, h)
	}
	if err := rows.Err(); err != nil {
		return Detail{}, err
	}
	return detail, nil
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
