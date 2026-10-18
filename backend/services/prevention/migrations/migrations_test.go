package migrations

import (
	"regexp"
	"strings"
	"testing"

	"firewatch.local/prevention/internal/state"
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

// TestAllHasTheTwoContractMigrations pins the migration identity: version 9 for
// the frozen action tables and version 11 for the S2 evidence tables. Version 10
// belongs to risk's model activation in the shared schema_migrations ledger, so
// claiming it here would make one of the two migrations skip silently.
func TestAllHasTheTwoContractMigrations(t *testing.T) {
	all := All()
	if len(all) != 2 {
		t.Fatalf("the service defines two migrations, All() returned %d", len(all))
	}
	if all[0].Version != 9 || all[1].Version != 11 {
		t.Errorf("versions are %d and %d, want 9 and 11", all[0].Version, all[1].Version)
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
