package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestActivateModelMovesTheSingleActiveVersionAndKeepsOneActive is the proof that
// could not be written with a fake repository: activation is a write path, and a
// fake cannot reproduce a missing privilege. Against a real migrated database an
// ungranted UPDATE on `models` makes ActivateModel fail and the API answers 503
// storage_unavailable, which is exactly how this defect escaped the unit suites.
//
// It needs the disposable migrated risk database the other integration tests use,
// so it skips without TEST_DATABASE_URL and runs in `make integration`.
func TestActivateModelMovesTheSingleActiveVersionAndKeepsOneActive(t *testing.T) {
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

	registry, err := s.ListModels(ctx)
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if len(registry) == 0 {
		t.Skip("the registry is empty; this proof needs the seeded models")
	}
	// Pick the current active version and a second, non-placeholder one. A
	// placeholder is refused by the schema on purpose, so it is not a candidate.
	var current, target string
	for _, model := range registry {
		if model.Active {
			current = model.Version
		}
		if model.Kind != "placeholder" && !model.Active && target == "" {
			target = model.Version
		}
	}
	if target == "" {
		t.Skip("the registry has no second activatable version; this proof needs 008_firewatch_rule_v2 applied")
	}
	if current != "" && current != target {
		defer func() {
			if _, err := s.ActivateModel(context.Background(), current); err != nil {
				t.Errorf("could not restore the previously active version %q: %v", current, err)
			}
		}()
	}

	// The regression: before 010 this failed with `permission denied for table
	// models`, because the role was only ever granted SELECT.
	activated, err := s.ActivateModel(ctx, target)
	if err != nil {
		t.Fatalf("ActivateModel(%q) failed: %v; a permission error here means the UPDATE grant on models is missing", target, err)
	}
	if !activated.Active {
		t.Errorf("ActivateModel(%q) returned active=false", target)
	}
	if activated.ApprovedAt == nil {
		t.Errorf("ActivateModel(%q) recorded no approved_at", target)
	}
	firstApproval := activated.ApprovedAt

	after, err := s.ListModels(ctx)
	if err != nil {
		t.Fatalf("ListModels after activation failed: %v", err)
	}
	var active []string
	for _, model := range after {
		if model.Active {
			active = append(active, model.Version)
		}
	}
	if len(active) != 1 || active[0] != target {
		t.Fatalf("after activating %q the active set is %v, want exactly [%q]; the single-active invariant is part of the schema", target, active, target)
	}

	// Re-activating keeps the date the version was first approved.
	again, err := s.ActivateModel(ctx, target)
	if err != nil {
		t.Fatalf("re-activating %q failed: %v", target, err)
	}
	if again.ApprovedAt == nil || firstApproval == nil || !again.ApprovedAt.Equal(*firstApproval) {
		t.Errorf("re-activation rewrote approved_at (%v then %v); the first approval date is the one to keep", firstApproval, again.ApprovedAt)
	}
}
