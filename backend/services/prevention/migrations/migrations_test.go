package migrations

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"firewatch.local/prevention/internal/state"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// statusCheckLiteral matches a single-quoted lower-case literal, which is how the
// status vocabulary appears in the CHECK constraint.
var statusCheckLiteral = regexp.MustCompile(`'([a-z_]+)'`)

// sqlFor returns the SQL of one migration version, failing when it is absent.
func sqlFor(t *testing.T, version int) string {
	t.Helper()
	for _, migration := range All() {
		if migration.Version == version {
			return migration.SQL
		}
	}
	t.Fatalf("migration version %d is not registered", version)
	return ""
}

// TestAllHasTheThreeContractMigrations pins the migration identity: version 9
// for the frozen action tables, version 11 for the S2 evidence tables and version
// 14 for the directory projection and the assignment reference. Version 10
// belongs to risk's model activation and 12 and 13 to risk's readings and model
// fixture in the shared schema_migrations ledger, so claiming any of them here
// would make one of the two migrations skip silently.
func TestAllHasTheThreeContractMigrations(t *testing.T) {
	all := All()
	if len(all) != 3 {
		t.Fatalf("the service defines three migrations, All() returned %d", len(all))
	}
	if all[0].Version != 9 || all[1].Version != 11 || all[2].Version != 14 {
		t.Errorf("versions are %d, %d and %d, want 9, 11 and 14", all[0].Version, all[1].Version, all[2].Version)
	}
	for _, migration := range all {
		if migration.Name == "" {
			t.Errorf("migration %d has an empty Name; a failure must be attributable", migration.Version)
		}
		if strings.TrimSpace(migration.SQL) == "" {
			t.Errorf("migration %d has empty SQL", migration.Version)
		}
	}
}

// TestActionMigrationDefinesExactlyTheFrozenTables keeps the two tables the
// original contract names from silently growing a third or losing one.
func TestActionMigrationDefinesExactlyTheFrozenTables(t *testing.T) {
	sql := sqlFor(t, 9)
	for _, table := range []string{
		"CREATE TABLE IF NOT EXISTS prevention.actions (",
		"CREATE TABLE IF NOT EXISTS prevention.action_history (",
	} {
		if !strings.Contains(sql, table) {
			t.Errorf("migration 009 does not create %q", table)
		}
	}
	if n := strings.Count(sql, "CREATE TABLE"); n != 2 {
		t.Errorf("migration 009 creates %d tables, the frozen contract names exactly 2", n)
	}
	if !strings.Contains(sql, "CREATE SCHEMA IF NOT EXISTS prevention") {
		t.Error("migration 009 does not create the prevention schema")
	}
}

// TestEvidenceMigrationAddsOnlyTheThreeAppendOnlyTables pins the additive shape of
// the S2 migration: three new tables, never an alteration of the frozen ones.
func TestEvidenceMigrationAddsOnlyTheThreeAppendOnlyTables(t *testing.T) {
	sql := sqlFor(t, 11)
	for _, table := range []string{
		"CREATE TABLE IF NOT EXISTS prevention.action_evidence (",
		"CREATE TABLE IF NOT EXISTS prevention.action_evidence_review (",
		"CREATE TABLE IF NOT EXISTS prevention.action_reassessment (",
	} {
		if !strings.Contains(sql, table) {
			t.Errorf("migration 011 does not create %q", table)
		}
	}
	if n := strings.Count(sql, "CREATE TABLE"); n != 3 {
		t.Errorf("migration 011 creates %d tables, the S2 contract names exactly 3", n)
	}
	for _, forbidden := range []string{"ALTER TABLE", "DROP ", "DELETE FROM", "TRUNCATE"} {
		if strings.Contains(sql, forbidden) {
			t.Errorf("migration 011 is not additive: it contains %q", forbidden)
		}
	}
	// No FK to batches: the prevention migration runner is independent of the
	// risk one in compose, so a foreign key would add a startup ordering
	// dependency. The service validates the real batch before inserting.
	if strings.Contains(sql, "REFERENCES batches") {
		t.Error("migration 011 must not couple its startup order to the risk schema")
	}
}

// TestMigrationStatusVocabularyMatchesTheStateMachine is the guard against the
// one duplication this design cannot remove: the state machine is Go code and the
// status column is a SQL CHECK. The database must accept exactly the states the
// service can produce, no more and no fewer.
func TestMigrationStatusVocabularyMatchesTheStateMachine(t *testing.T) {
	sql := sqlFor(t, 9)
	marker := "status IN"
	index := strings.Index(sql, marker)
	if index < 0 {
		t.Fatalf("migration has no %q constraint", marker)
	}
	end := strings.Index(sql[index:], ")")
	if end < 0 {
		t.Fatal("status CHECK constraint is not terminated")
	}
	declared := map[string]bool{}
	for _, match := range statusCheckLiteral.FindAllStringSubmatch(sql[index:index+end], -1) {
		declared[match[1]] = true
	}
	want := map[string]bool{}
	for _, s := range state.States() {
		want[string(s)] = true
	}
	for value := range want {
		if !declared[value] {
			t.Errorf("prevention.actions.status does not accept %q, but the state machine can set it", value)
		}
	}
	for value := range declared {
		if !want[value] {
			t.Errorf("prevention.actions.status accepts %q, which is not a state of the contract", value)
		}
	}
}

// TestMigrationGrantsToTheApplicationRole pins the grant that makes the deployed
// service able to work at all: without USAGE on the schema the API fails every
// query, and the absence of UPDATE/DELETE on the new tables is what keeps the
// evidence trail append-only.
func TestMigrationGrantsToTheApplicationRole(t *testing.T) {
	actions := sqlFor(t, 9)
	for _, grant := range []string{
		"GRANT USAGE ON SCHEMA prevention TO risk_app;",
		"GRANT SELECT, INSERT, UPDATE ON prevention.actions TO risk_app;",
		"GRANT SELECT, INSERT ON prevention.action_history TO risk_app;",
	} {
		if !strings.Contains(actions, grant) {
			t.Errorf("migration 009 is missing %q", grant)
		}
	}
	if strings.Contains(actions, "DELETE ON prevention.action_history") {
		t.Error("history must stay append-only; the migration grants DELETE")
	}

	evidence := sqlFor(t, 11)
	for _, grant := range []string{
		"GRANT SELECT, INSERT ON prevention.action_evidence TO risk_app;",
		"GRANT SELECT, INSERT ON prevention.action_evidence_review TO risk_app;",
		"GRANT SELECT, INSERT ON prevention.action_reassessment TO risk_app;",
		"GRANT USAGE, SELECT ON SEQUENCE prevention.action_evidence_id_seq TO risk_app;",
		"GRANT USAGE, SELECT ON SEQUENCE prevention.action_evidence_review_id_seq TO risk_app;",
		"GRANT USAGE, SELECT ON SEQUENCE prevention.action_reassessment_id_seq TO risk_app;",
	} {
		if !strings.Contains(evidence, grant) {
			t.Errorf("migration 011 is missing %q", grant)
		}
	}
	for _, forbidden := range []string{"UPDATE ON prevention.action_evidence", "DELETE ON prevention.action_evidence", "UPDATE ON prevention.action_reassessment", "DELETE ON prevention.action_reassessment"} {
		if strings.Contains(evidence, forbidden) {
			t.Errorf("the evidence trail must stay append-only; migration grants %q", forbidden)
		}
	}
}

// TestAssigneeMigrationIsAdditiveAndGuarded pins the shape of version 14: it
// creates the directory projection, adds a nullable foreign-key reference and
// grants the application role exactly what the upsert and the read need. Nothing
// in it drops, rewrites or backfills a stored value, which is what keeps every
// existing assignee byte-identical.
func TestAssigneeMigrationIsAdditiveAndGuarded(t *testing.T) {
	sql := sqlFor(t, 14)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS prevention.directory_users (",
		"id uuid PRIMARY KEY",
		"display_name text NOT NULL CHECK (btrim(display_name) <> '')",
		"synced_at timestamptz NOT NULL DEFAULT now()",
		"ALTER TABLE prevention.actions",
		"ADD COLUMN IF NOT EXISTS assignee_id uuid",
		"REFERENCES prevention.directory_users(id) ON DELETE RESTRICT",
		"CREATE INDEX IF NOT EXISTS actions_assignee",
		"GRANT SELECT, INSERT, UPDATE ON prevention.directory_users TO risk_app;",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("migration 014 does not contain %q", fragment)
		}
	}
	for _, forbidden := range []string{"DROP COLUMN", "DROP TABLE", "DELETE FROM", "TRUNCATE", "UPDATE prevention.actions", "SET assignee"} {
		if strings.Contains(sql, forbidden) {
			t.Errorf("migration 014 is not additive: it contains %q", forbidden)
		}
	}
}

// TestAssigneeMigrationAppliesAndReappliesWithoutRewritingLegacyAssignees is the
// database-side proof. It applies version 9 to an isolated schema of a disposable
// database, inserts an action whose assignee is non-null free text, then applies
// version 14 twice and checks three things the SQL alone cannot show: the legacy
// value is byte-identical, its new reference is null, and the reference is a real
// foreign key that refuses an unknown identifier and restricts the parent delete.
//
// It needs a connection that may create a schema, which the compose test
// service's unprivileged risk_app URL cannot do. Set TEST_MIGRATION_DATABASE_URL
// to a disposable database's owner or superuser and run:
//
//	TEST_MIGRATION_DATABASE_URL=postgres://postgres@127.0.0.1:5432/risk?sslmode=disable go test ./migrations -run TestAssigneeMigrationAppliesAndReapplies -count=1 -v
func TestAssigneeMigrationAppliesAndReappliesWithoutRewritingLegacyAssignees(t *testing.T) {
	ctx, db := migrationDatabase(t)
	schema := throwawaySchema(t, ctx, db)
	ident := pgx.Identifier{schema}.Sanitize()

	if _, err := db.Exec(ctx, sqlInSchema(sqlFor(t, 9), schema)); err != nil {
		t.Fatalf("apply migration 009 to the isolated schema: %v", err)
	}
	const legacyID = "6f1e7b0a-2f4f-4a3f-9a5b-1d3c7e9f2a40"
	if _, err := db.Exec(ctx, `INSERT INTO `+ident+`.actions(id,community_code,title,status,assignee,created_by)
		VALUES($1,'legacy-community','Acta','propuesta','brigada-2','tester')`, legacyID); err != nil {
		t.Fatalf("insert the legacy action: %v", err)
	}

	if _, err := db.Exec(ctx, sqlInSchema(sqlFor(t, 14), schema)); err != nil {
		t.Fatalf("apply migration 014: %v", err)
	}
	// The runner records the version and never re-executes a file, but the SQL is
	// guarded like 009 and 011, so executing it a second time is still a no-op.
	if _, err := db.Exec(ctx, sqlInSchema(sqlFor(t, 14), schema)); err != nil {
		t.Fatalf("re-apply migration 014: %v", err)
	}

	var assignee string
	var assigneeID *string
	if err := db.QueryRow(ctx, `SELECT assignee, assignee_id::text FROM `+ident+`.actions WHERE id=$1`, legacyID).
		Scan(&assignee, &assigneeID); err != nil {
		t.Fatalf("read the legacy action back: %v", err)
	}
	if assignee != "brigada-2" {
		t.Errorf("legacy assignee is %q after the migration, want the byte-identical %q", assignee, "brigada-2")
	}
	if assigneeID != nil {
		t.Errorf("legacy assignment gained the reference %q; it must stay null", *assigneeID)
	}

	// A projected user is a valid target.
	userID := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO `+ident+`.directory_users(id,display_name) VALUES($1,$2)`, userID, "Brigada 2"); err != nil {
		t.Fatalf("insert the directory projection: %v", err)
	}
	if _, err := db.Exec(ctx, `UPDATE `+ident+`.actions SET assignee_id=$1 WHERE id=$2`, userID, legacyID); err != nil {
		t.Fatalf("assign the projected user: %v", err)
	}

	// An identifier the projection never saw is refused by the database, and the
	// explicit ON DELETE RESTRICT keeps the parent row alive under an assignment.
	_, err := db.Exec(ctx, `UPDATE `+ident+`.actions SET assignee_id=$1 WHERE id=$2`, uuid.NewString(), legacyID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatalf("assigning an unprojected user returned %v, want SQLSTATE 23503", err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM `+ident+`.directory_users WHERE id=$1`, userID); err == nil {
		t.Error("deleting an assigned directory user succeeded; ON DELETE RESTRICT must refuse it")
	}

	var deleteAction string
	if err := db.QueryRow(ctx, `SELECT c.confdeltype::text
		FROM pg_constraint c
		JOIN pg_class t ON t.oid = c.conrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		WHERE n.nspname=$1 AND t.relname='actions' AND c.conname='actions_assignee_id_fkey'`, schema).Scan(&deleteAction); err != nil {
		t.Fatalf("read the foreign key's delete action: %v", err)
	}
	if deleteAction != "r" {
		t.Errorf("the assignment foreign key is ON DELETE %q, want 'r' (RESTRICT)", deleteAction)
	}

	var nullable string
	if err := db.QueryRow(ctx, `SELECT is_nullable FROM information_schema.columns
		WHERE table_schema=$1 AND table_name='actions' AND column_name='assignee_id'`, schema).Scan(&nullable); err != nil {
		t.Fatalf("read assignee_id's nullability: %v", err)
	}
	if nullable != "YES" {
		t.Errorf("assignee_id is nullable=%q, want YES so an unassigned action stays valid", nullable)
	}
}

// migrationDatabase connects to the disposable database the migration apply
// tests need. It is a separate variable from TEST_DATABASE_URL on purpose: the
// compose test service connects as risk_app, which may read and write the
// migrated schema but may not create one, so it cannot apply DDL.
func migrationDatabase(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("TEST_MIGRATION_DATABASE_URL")
	if testing.Short() || dsn == "" {
		t.Skip("requires TEST_MIGRATION_DATABASE_URL: a disposable database whose connection may create a schema")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to TEST_MIGRATION_DATABASE_URL: %v", err)
	}
	t.Cleanup(db.Close)
	if err := db.Ping(ctx); err != nil {
		t.Fatalf("ping TEST_MIGRATION_DATABASE_URL: %v", err)
	}
	return ctx, db
}

// throwawaySchema creates a schema this test owns, so applying the migrations
// never touches the database's real prevention schema.
func throwawaySchema(t *testing.T, ctx context.Context, db *pgxpool.Pool) string {
	t.Helper()
	name := "prevention_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	if _, err := db.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create the throwaway schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+pgx.Identifier{name}.Sanitize()+" CASCADE")
	})
	return name
}

// sqlInSchema rewrites the migration's schema-qualified names onto the throwaway
// schema, so the file under test is executed verbatim apart from its schema name.
func sqlInSchema(sql, schema string) string {
	return strings.ReplaceAll(sql, "prevention", schema)
}
