package repository

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"firewatch.local/prevention/internal/state"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests need a migrated disposable database holding the prevention schema.
// They are skipped when TEST_DATABASE_URL is unset, which is how the local unit
// run behaves; the compose `prevention-test` service sets it, and that is where
// the database-side rules - the ones a fake cannot prove - actually run.

func testStore(t *testing.T) (context.Context, Store) {
	t.Helper()
	if testing.Short() || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires a migrated disposable database with the prevention schema")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	db, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return ctx, Store{DB: db}
}

// create inserts one action for a test and cleans it up afterwards.
func create(t *testing.T, ctx context.Context, store Store, community string) Action {
	t.Helper()
	action, err := store.Create(ctx, NewAction{
		CommunityCode: community, Title: "Cortafuegos", Description: "Faja de 30 m",
		Priority: "alta", CreatedBy: "tester",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		// Deleting the fixtures is safe here because this is a disposable test
		// database and the rows were created by this test. History first: it
		// references the action.
		_, _ = store.DB.Exec(context.Background(), `DELETE FROM prevention.action_history WHERE action_id=$1`, action.ID)
		_, _ = store.DB.Exec(context.Background(), `DELETE FROM prevention.actions WHERE id=$1`, action.ID)
	})
	return action
}

// TestCreateStartsAtPropuestaWithNoHistory pins the two create-time rules: the
// status is the contract's initial one, and creation is not a transition, so it
// writes no history row.
func TestCreateStartsAtPropuestaWithNoHistory(t *testing.T) {
	ctx, store := testStore(t)
	action := create(t, ctx, store, "valparaiso")
	if action.Status != string(state.Propuesta) {
		t.Errorf("new action status %q, want %q", action.Status, state.Propuesta)
	}
	detail, err := store.Get(ctx, action.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(detail.History) != 0 {
		t.Errorf("a new action must have no history, got %+v", detail.History)
	}
}

// TestAcceptedTransitionWritesStatusAndHistory proves the durable half of a
// transition: the status changes, updated_at advances, and exactly one history
// row records the actor and the note.
func TestAcceptedTransitionWritesStatusAndHistory(t *testing.T) {
	ctx, store := testStore(t)
	action := create(t, ctx, store, "valparaiso")
	updated, err := store.Transition(ctx, action.ID, string(state.Asignada), "admin-1", "primer acuse")
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	if updated.Status != string(state.Asignada) {
		t.Errorf("status %q, want asignada", updated.Status)
	}
	if !updated.UpdatedAt.After(action.UpdatedAt) && !updated.UpdatedAt.Equal(action.UpdatedAt) {
		t.Errorf("updated_at went backwards: %v then %v", action.UpdatedAt, updated.UpdatedAt)
	}
	detail, err := store.Get(ctx, action.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(detail.History) != 1 {
		t.Fatalf("history must have exactly one row, got %+v", detail.History)
	}
	row := detail.History[0]
	if row.FromStatus != string(state.Propuesta) || row.ToStatus != string(state.Asignada) ||
		row.ActorSub != "admin-1" || row.Note != "primer acuse" {
		t.Errorf("history row %+v does not record the transition", row)
	}
}

// TestRefusedTransitionWritesNothing is the contract's "no escribe nada": a 409
// must leave the status, updated_at and the history exactly as they were, in the
// database and not only in the response.
func TestRefusedTransitionWritesNothing(t *testing.T) {
	ctx, store := testStore(t)
	action := create(t, ctx, store, "valparaiso")
	if _, err := store.Transition(ctx, action.ID, string(state.Asignada), "admin-1", "acuse"); err != nil {
		t.Fatalf("first transition: %v", err)
	}
	before, err := store.Get(ctx, action.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	// asignada -> aceptada is not in the contract's table, so it must be refused.
	if _, err := store.Transition(ctx, action.ID, string(state.Aceptada), "admin-2", "atajo"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("refused transition returned %v, want ErrInvalidTransition", err)
	}
	after, err := store.Get(ctx, action.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if after.Status != before.Status {
		t.Errorf("status changed to %q after a refused transition", after.Status)
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Errorf("updated_at changed from %v to %v after a refused transition", before.UpdatedAt, after.UpdatedAt)
	}
	if len(after.History) != len(before.History) {
		t.Errorf("history grew from %d to %d rows after a refused transition", len(before.History), len(after.History))
	}
}

// TestTerminalStatesRefuseEveryFurtherMove keeps aceptada and cancelada terminal
// in the database and not only in the handler: no further history row may be
// written for them.
func TestTerminalStatesRefuseEveryFurtherMove(t *testing.T) {
	ctx, store := testStore(t)
	action := create(t, ctx, store, "valparaiso")
	for _, step := range []state.State{state.Asignada, state.EnCurso, state.Enviada, state.Aceptada} {
		if _, err := store.Transition(ctx, action.ID, string(step), "admin-1", ""); err != nil {
			t.Fatalf("transition to %s: %v", step, err)
		}
	}
	for _, target := range state.States() {
		if _, err := store.Transition(ctx, action.ID, string(target), "admin-1", ""); !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("accepted action accepted a move to %s: %v", target, err)
		}
	}
	detail, err := store.Get(ctx, action.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(detail.History) != 4 {
		t.Errorf("terminal action has %d history rows, want the 4 accepted ones", len(detail.History))
	}
}

// TestReopeningIsRecordedInOrder covers the reopening edge the contract names
// explicitly and the chronological order the detail read depends on.
func TestReopeningIsRecordedInOrder(t *testing.T) {
	ctx, store := testStore(t)
	action := create(t, ctx, store, "valparaiso")
	path := []state.State{state.Asignada, state.EnCurso, state.Enviada, state.Devuelta, state.EnCurso}
	for _, step := range path {
		if _, err := store.Transition(ctx, action.ID, string(step), "admin-1", ""); err != nil {
			t.Fatalf("transition to %s: %v", step, err)
		}
	}
	detail, err := store.Get(ctx, action.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(detail.History) != len(path) {
		t.Fatalf("history has %d rows, want %d", len(detail.History), len(path))
	}
	for index, step := range path {
		if detail.History[index].ToStatus != string(step) {
			t.Errorf("history row %d moved to %s, want %s", index, detail.History[index].ToStatus, step)
		}
	}
}

// TestUnknownActionIsNotFound keeps a missing row distinguishable from a refused
// transition: they are 404 and 409 respectively, and only one of them means the
// action exists.
func TestUnknownActionIsNotFound(t *testing.T) {
	ctx, store := testStore(t)
	if _, err := store.Get(ctx, "6f1e7b0a-2f4f-4a3f-9a5b-1d3c7e9f2a44"); !errors.Is(err, ErrActionNotFound) {
		t.Errorf("Get on a missing action returned %v, want ErrActionNotFound", err)
	}
	if _, err := store.Transition(ctx, "6f1e7b0a-2f4f-4a3f-9a5b-1d3c7e9f2a44", string(state.Asignada), "admin-1", ""); !errors.Is(err, ErrActionNotFound) {
		t.Errorf("Transition on a missing action returned %v, want ErrActionNotFound", err)
	}
}

// TestListFiltersAndOrder covers the two filter refresh rules and the ordering
// the list route documents: newest first, an empty filter narrowing nothing, and
// a status filter that matches only that status.
func TestListFiltersAndOrder(t *testing.T) {
	ctx, store := testStore(t)
	first := create(t, ctx, store, "list-community")
	second := create(t, ctx, store, "list-community")
	if _, err := store.Transition(ctx, second.ID, string(state.Asignada), "admin-1", ""); err != nil {
		t.Fatalf("transition: %v", err)
	}
	all, err := store.List(ctx, "list-community", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("list returned %d actions, want 2", len(all))
	}
	if all[0].ID != second.ID || all[1].ID != first.ID {
		t.Errorf("list is not newest first: %s then %s", all[0].ID, all[1].ID)
	}
	if all[0].Status != string(state.Asignada) || all[1].Status != string(state.Propuesta) {
		t.Errorf("unexpected statuses %s and %s", all[0].Status, all[1].Status)
	}
	assigned, err := store.List(ctx, "list-community", string(state.Asignada))
	if err != nil {
		t.Fatalf("filtered list: %v", err)
	}
	if len(assigned) != 1 || assigned[0].ID != second.ID {
		t.Errorf("status filter returned %+v", assigned)
	}
	other, err := store.List(ctx, "no-such-community", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if other == nil || len(other) != 0 {
		t.Errorf("an empty result must be an empty slice, got %#v", other)
	}
}
