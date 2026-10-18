package repository

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// fakeRow is a pgx.Row that writes one canned models row into the scan
// destinations. The destination types are checked in order, so this also pins the
// shared projection order that ListModels, ActiveModel and ActivateModel agree on:
// changing the column order without changing the scan fails here instead of in a
// database-only test that cannot run without a server.
type fakeRow struct {
	version    string
	kind       string
	paramsHash *string
	approvedAt *time.Time
	metrics    []byte
	active     bool
}

func assignTo[T any](dest any, value T) error {
	target, ok := dest.(*T)
	if !ok {
		return fmt.Errorf("a models scan destination of type %T cannot receive %T", dest, value)
	}
	*target = value
	return nil
}

func (r fakeRow) Scan(dest ...any) error {
	if len(dest) != 6 {
		return fmt.Errorf("models scan wants 6 destinations, got %d", len(dest))
	}
	for _, step := range []func() error{
		func() error { return assignTo(dest[0], r.version) },
		func() error { return assignTo(dest[1], r.kind) },
		func() error { return assignTo(dest[2], r.paramsHash) },
		func() error { return assignTo(dest[3], r.approvedAt) },
		func() error { return assignTo(dest[4], r.metrics) },
		func() error { return assignTo(dest[5], r.active) },
	} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// TestScanModelNormalisesMetricsAndApprovedAt protects the shared read that the
// public registry list and the activation write both go through. Two columns need
// normalising, and both were normalised by hand before this read existed: SQL NULL
// metrics must be served as an empty object rather than as JSON null, and
// approved_at must be exposed in UTC like every other instant this API serves.
func TestScanModelNormalisesMetricsAndApprovedAt(t *testing.T) {
	hash := "e17aaa5600eb67c9a91340e90057aec100f4e80ef0895662f373cffee66564c7"
	// A non-UTC location proves the scan converts instead of only relabelling.
	approved := time.Date(2026, 10, 1, 9, 0, 0, 0, time.FixedZone("UTC-3", -3*3600))
	model, err := scanModel(fakeRow{version: "academic-rule-v1", kind: "rule", paramsHash: &hash, approvedAt: &approved, metrics: nil, active: true})
	if err != nil {
		t.Fatalf("scan a row with SQL NULL metrics: %v", err)
	}
	if model.Version != "academic-rule-v1" || model.Kind != "rule" || !model.Active {
		t.Fatalf("scanned row = %+v, want the canned version, kind and active flag", model)
	}
	if model.ParamsHash == nil || *model.ParamsHash != hash {
		t.Fatalf("params hash = %v, want %q", model.ParamsHash, hash)
	}
	if string(model.Metrics) != "{}" {
		t.Fatalf("SQL NULL metrics were served as %q, want an empty JSON object", model.Metrics)
	}
	if model.ApprovedAt == nil || model.ApprovedAt.Location() != time.UTC || !model.ApprovedAt.Equal(approved) {
		t.Fatalf("approved_at = %v, want the same instant in UTC (%v)", model.ApprovedAt, approved.UTC())
	}

	// Control: a row that carries metrics and no approval instant is served
	// unchanged, so the normalisation above is not overwriting real values.
	stored := json.RawMessage(`{"activation":{"at":"2026-10-18T12:00:00Z"}}`)
	other, err := scanModel(fakeRow{version: "firewatch-rule-v2", kind: "rule", metrics: []byte(stored), active: false})
	if err != nil {
		t.Fatalf("scan a row with stored metrics: %v", err)
	}
	if string(other.Metrics) != string(stored) {
		t.Fatalf("stored metrics = %q, want them preserved as %q", other.Metrics, stored)
	}
	if other.ApprovedAt != nil {
		t.Fatalf("a row without an approval instant was given %v", other.ApprovedAt)
	}
	if !other.ExplicitlyActivated() {
		t.Fatal("the activation record was lost by the shared scan, so activation could not decide inference")
	}
}

// TestExplicitlyActivatedFollowsTheActivationRecord pins the one fact the
// publication path uses to tell an operator's activation from the row a
// migration seeds: ActivateModel writes an "activation" object into metrics, and
// the seeded row carries none. Getting this wrong in either direction is a wrong
// inference decision, so the whole table is pinned, including the unreadable
// metrics case, which must be read as "no activation recorded" rather than as an
// activation.
func TestExplicitlyActivatedFollowsTheActivationRecord(t *testing.T) {
	for _, tc := range []struct {
		name    string
		metrics string
		want    bool
	}{
		{"activation recorded", `{"official":false,"activation":{"at":"2026-10-18T12:00:00Z"}}`, true},
		{"activation with an actor", `{"activation":{"at":"2026-10-18T12:00:00Z","actor":"someone"}}`, true},
		{"seed row without activation", `{"official":false,"calibrated_against_observed_fires":false}`, false},
		{"empty object", `{}`, false},
		{"no metrics at all", ``, false},
		{"explicit null activation", `{"activation":null}`, false},
		{"unreadable metrics", `not json`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := ModelVersion{Version: "academic-rule-v1", Kind: "rule", Metrics: json.RawMessage(tc.metrics)}
			if got := model.ExplicitlyActivated(); got != tc.want {
				t.Fatalf("ExplicitlyActivated() = %v, want %v for metrics %s", got, tc.want, tc.metrics)
			}
		})
	}
}
