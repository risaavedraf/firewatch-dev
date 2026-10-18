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

// TestAllHasExactlyTheContractMigration pins the migration identity: one
// migration, version 9, with a name and SQL. Version 9 rather than 8 because the
// shared schema_migrations ledger already holds risk's 008.
func TestAllHasExactlyTheContractMigration(t *testing.T) {
	all := All()
	if len(all) != 1 {
		t.Fatalf("the contract defines one migration, All() returned %d", len(all))
	}
	if all[0].Version != 9 {
		t.Errorf("migration version is %d, the frozen version is 009_preventive_actions.sql (008 belongs to risk in the shared ledger)", all[0].Version)
	}
	if all[0].Name == "" {
		t.Error("migration has an empty Name; a failure must be attributable")
	}
	if strings.TrimSpace(all[0].SQL) == "" {
		t.Error("migration SQL is empty")
	}
}

// TestMigrationDefinesExactlyTheContractTables keeps the two tables the contract
// names from silently growing a third or losing one.
func TestMigrationDefinesExactlyTheContractTables(t *testing.T) {
	sql := All()[0].SQL
	for _, table := range []string{
		"CREATE TABLE IF NOT EXISTS prevention.actions (",
		"CREATE TABLE IF NOT EXISTS prevention.action_history (",
	} {
		if !strings.Contains(sql, table) {
			t.Errorf("migration does not create %q", table)
		}
	}
	if n := strings.Count(sql, "CREATE TABLE"); n != 2 {
		t.Errorf("migration creates %d tables, the contract names exactly 2", n)
	}
	if !strings.Contains(sql, "CREATE SCHEMA IF NOT EXISTS prevention") {
		t.Error("migration does not create the prevention schema the contract names")
	}
}

// TestMigrationStatusVocabularyMatchesTheStateMachine is the guard against the
// one duplication this design cannot remove: the state machine is Go code and the
// status column is a SQL CHECK. The database must accept exactly the states the
// service can produce, no more and no fewer.
func TestMigrationStatusVocabularyMatchesTheStateMachine(t *testing.T) {
	sql := All()[0].SQL
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
// query, and the absence of DELETE is what keeps history append-only.
func TestMigrationGrantsToTheApplicationRole(t *testing.T) {
	sql := All()[0].SQL
	for _, grant := range []string{
		"GRANT USAGE ON SCHEMA prevention TO risk_app;",
		"GRANT SELECT, INSERT, UPDATE ON prevention.actions TO risk_app;",
		"GRANT SELECT, INSERT ON prevention.action_history TO risk_app;",
	} {
		if !strings.Contains(sql, grant) {
			t.Errorf("migration is missing %q", grant)
		}
	}
	if strings.Contains(sql, "DELETE ON prevention.action_history") {
		t.Error("history must stay append-only; the migration grants DELETE")
	}
}
