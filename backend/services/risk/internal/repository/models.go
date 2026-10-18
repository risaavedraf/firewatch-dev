package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrModelNotFound means the requested model version has no row in the registry.
// It is a deterministic answer, not a storage failure.
var ErrModelNotFound = errors.New("model version not found")

// ErrModelNotActivatable means the row exists but the schema forbids it from ever
// being active: a placeholder that no classifier produced. Activating it would
// claim a behaviour nobody can reproduce.
var ErrModelNotActivatable = errors.New("model version cannot be activated")

// ModelVersion is one row of the models registry as served by the API.
//
// ParamsHash is a pointer on purpose: a placeholder genuinely has no
// reconstructible parameter set, and the store serves SQL NULL rather than
// inventing a fingerprint. Metrics is raw JSON so a future model can carry its
// own shape without this service re-encoding it.
type ModelVersion struct {
	Version    string          `json:"version"`
	Kind       string          `json:"kind"`
	ParamsHash *string         `json:"params_hash"`
	ApprovedAt *time.Time      `json:"approved_at"`
	Metrics    json.RawMessage `json:"metrics"`
	Active     bool            `json:"active"`
}

// ListModels returns the registry in a stable order: active first, then by
// version. The order is deterministic so the UI does not reshuffle between polls.
func (s Store) ListModels(ctx context.Context) ([]ModelVersion, error) {
	rows, err := s.DB.Query(ctx, `SELECT version, kind, params_hash, approved_at, metrics, active
		FROM models ORDER BY active DESC, version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	models := make([]ModelVersion, 0)
	for rows.Next() {
		var model ModelVersion
		var metrics []byte
		if err := rows.Scan(&model.Version, &model.Kind, &model.ParamsHash, &model.ApprovedAt, &metrics, &model.Active); err != nil {
			return nil, err
		}
		if metrics == nil {
			metrics = []byte("{}")
		}
		model.Metrics = json.RawMessage(metrics)
		if model.ApprovedAt != nil {
			utc := model.ApprovedAt.UTC()
			model.ApprovedAt = &utc
		}
		models = append(models, model)
	}
	return models, rows.Err()
}

// ActivateModel makes one version active and records the approval atomically.
//
// The transaction shape is deliberate. At most one model may be active (the
// partial unique index from migration 006), so the previously active row is
// cleared before the target is set; doing that in one transaction means a reader
// never observes two active versions or none. approved_at is set only when it was
// null, so re-activating a version does not rewrite the date it was first
// approved, and metrics records the activation without discarding the model's
// existing metrics. A placeholder is refused before any write, and a missing row
// is a deterministic not-found rather than an error.
func (s Store) ActivateModel(ctx context.Context, version string) (ModelVersion, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return ModelVersion{}, err
	}
	defer tx.Rollback(ctx)

	var kind string
	if err := tx.QueryRow(ctx, `SELECT kind FROM models WHERE version=$1 FOR UPDATE`, version).Scan(&kind); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ModelVersion{}, ErrModelNotFound
		}
		return ModelVersion{}, err
	}
	if kind == "placeholder" {
		return ModelVersion{}, ErrModelNotActivatable
	}
	activatedAt := time.Now().UTC()
	if _, err := tx.Exec(ctx, `UPDATE models SET active=false WHERE active AND version<>$1`, version); err != nil {
		return ModelVersion{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE models
		SET active=true,
		    approved_at=COALESCE(approved_at,$2),
		    metrics=metrics || jsonb_build_object('activation', jsonb_build_object('at',$2))
		WHERE version=$1`, version, activatedAt); err != nil {
		return ModelVersion{}, err
	}
	var model ModelVersion
	var metrics []byte
	err = tx.QueryRow(ctx, `SELECT version, kind, params_hash, approved_at, metrics, active FROM models WHERE version=$1`, version).
		Scan(&model.Version, &model.Kind, &model.ParamsHash, &model.ApprovedAt, &metrics, &model.Active)
	if err != nil {
		return ModelVersion{}, err
	}
	if metrics == nil {
		metrics = []byte("{}")
	}
	model.Metrics = json.RawMessage(metrics)
	if model.ApprovedAt != nil {
		utc := model.ApprovedAt.UTC()
		model.ApprovedAt = &utc
	}
	if err := tx.Commit(ctx); err != nil {
		return ModelVersion{}, err
	}
	return model, nil
}
