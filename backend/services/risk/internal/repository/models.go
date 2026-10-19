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

// ErrNoActiveModel means the registry holds no active row. It is a deterministic
// answer about the registry's state, not a storage failure, so the caller can
// tell "nothing is active" from "the registry could not be read".
var ErrNoActiveModel = errors.New("no active model version")

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

// activationMetricsKey is the metrics key ActivateModel writes when an operator
// activates a version. It is the record of a decision, and it is the only signal
// that distinguishes an operator's activation from the row a migration seeds.
const activationMetricsKey = "activation"

// ExplicitlyActivated reports whether this row records an operator's activation.
//
// ActivateModel writes an "activation" object into the row's metrics, so a row
// that carries it is the result of an explicit activation. The rows the
// migrations seed are active without that key: their active flag is registry
// lifecycle data, not a decision about which classifier must run, and the
// publication path treats the two differently on purpose.
//
// Metrics that cannot be read are reported as NOT explicitly activated: an
// unreadable record cannot be evidence of a decision, and guessing "yes" would
// hand the publication path a decision nobody made.
func (m ModelVersion) ExplicitlyActivated() bool {
	if len(m.Metrics) == 0 {
		return false
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(m.Metrics, &probe); err != nil {
		return false
	}
	activation, recorded := probe[activationMetricsKey]
	return recorded && len(activation) > 0 && string(activation) != "null"
}

// modelColumns is the registry projection shared by every read of a models row,
// so the column list and the scan cannot drift apart between the list route, the
// activation write and the publication path's read.
const modelColumns = `version, kind, params_hash, approved_at, metrics, active`

// scanModel reads one registry row in the modelColumns order and normalises the
// two columns that need it: metrics is never served as SQL NULL, and approved_at
// is exposed in UTC.
func scanModel(row pgx.Row) (ModelVersion, error) {
	var model ModelVersion
	var metrics []byte
	if err := row.Scan(&model.Version, &model.Kind, &model.ParamsHash, &model.ApprovedAt, &metrics, &model.Active); err != nil {
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
	return model, nil
}

// ActiveModel returns the single registry row marked active.
//
// It is the read the publication path uses on every publication, which is what
// makes an activation decide inference in every replica without a restart: the
// row is read from shared storage, not kept in the process. A registry with no
// active row answers ErrNoActiveModel, a deterministic answer the caller turns
// into its documented bootstrap behaviour; any other error is a storage failure
// and stays retryable.
//
// At most one row can be active (the partial unique index from migration 006),
// so this read cannot be ambiguous.
func (s Store) ActiveModel(ctx context.Context) (ModelVersion, error) {
	model, err := scanModel(s.DB.QueryRow(ctx, `SELECT `+modelColumns+` FROM models WHERE active`))
	if errors.Is(err, pgx.ErrNoRows) {
		return ModelVersion{}, ErrNoActiveModel
	}
	if err != nil {
		return ModelVersion{}, err
	}
	return model, nil
}

// ListModels returns the registry in a stable order: active first, then by
// version. The order is deterministic so the UI does not reshuffle between polls.
func (s Store) ListModels(ctx context.Context) ([]ModelVersion, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+modelColumns+`
		FROM models ORDER BY active DESC, version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	models := make([]ModelVersion, 0)
	for rows.Next() {
		model, err := scanModel(rows)
		if err != nil {
			return nil, err
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
//
// The activation record written here -- the "activation" object in metrics -- is
// what makes this row authoritative for inference: the publication path resolves
// an explicitly activated version and keeps the startup selection only as the
// bootstrap for a registry that records no activation. See
// backend/docs/model-activation.md.
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
	model, err := scanModel(tx.QueryRow(ctx, `SELECT `+modelColumns+` FROM models WHERE version=$1`, version))
	if err != nil {
		return ModelVersion{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ModelVersion{}, err
	}
	return model, nil
}
