package migrations

import (
	"regexp"
	"strings"
	"testing"
)

// commentLine strips a SQL line comment so prose cannot trip a guard.
var commentLine = regexp.MustCompile(`(?m)--.*$`)

// statementMutations lists the data-mutating statements in a migration.
//
// It scans statement by statement instead of substring-searching the whole file.
// A migration may legitimately GRANT UPDATE, and forbidding the word would force
// a worse fix than forbidding the statement; every other guard in this package
// keeps its stricter substring check because none of those files needs a grant.
// DROP is deliberately not listed: ALTER TABLE ... DROP CONSTRAINT is how these
// migrations re-apply their own CHECK constraints idempotently.
func statementMutations(sql string) []string {
	withoutComments := commentLine.ReplaceAllString(sql, "")
	var found []string
	for _, statement := range strings.Split(withoutComments, ";") {
		fields := strings.Fields(strings.TrimSpace(statement))
		if len(fields) == 0 {
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "UPDATE", "DELETE", "TRUNCATE":
			found = append(found, strings.ToUpper(fields[0]))
		}
	}
	return found
}

// TestModelsActivationGrantMigrationIsAdditiveAndGuarded pins migration 10: it
// exists to give the application role the UPDATE privilege the activation
// endpoint needs, and it must not do anything else. Without this migration the
// endpoint answers 503 storage_unavailable against a real database, which is a
// failure no fake repository can reproduce.
func TestModelsActivationGrantMigrationIsAdditiveAndGuarded(t *testing.T) {
	migration := migrationFor(t, 10)
	if migration.Name != "models_activation_grant" {
		t.Errorf("migration 10 is named %q, want %q", migration.Name, "models_activation_grant")
	}
	sql := migration.SQL
	if !strings.Contains(sql, "GRANT UPDATE ON models TO risk_app") {
		t.Error("010_models_activation_grant.sql does not grant UPDATE on models to risk_app; the activation endpoint keeps failing with a permission error")
	}
	if strings.Contains(sql, "DROP TABLE") {
		t.Error("010_models_activation_grant.sql drops a table; this migration only grants a privilege")
	}
	if mutations := statementMutations(sql); len(mutations) > 0 {
		t.Errorf("010_models_activation_grant.sql contains data-mutating statements %v; a grant is not a data change", mutations)
	}
}

// TestStatementMutationsOnlyFlagsRealDataChanges is the self-test of the helper
// above: it must accept a GRANT that names UPDATE and still catch the statements
// the package forbids, so the looser check is demonstrably not a blind spot.
func TestStatementMutationsOnlyFlagsRealDataChanges(t *testing.T) {
	for _, accepted := range []string{
		"GRANT UPDATE ON models TO risk_app;",
		"-- UPDATE models is forbidden here\nGRANT SELECT ON models TO risk_app;",
		"ALTER TABLE models DROP CONSTRAINT IF EXISTS models_placeholder_not_active_check;",
	} {
		if got := statementMutations(accepted); len(got) != 0 {
			t.Errorf("statementMutations(%q) = %v, want no findings", accepted, got)
		}
	}
	for _, rejected := range []string{
		"UPDATE models SET active = true;",
		"DELETE FROM models;",
		"TRUNCATE models;",
		"ALTER TABLE models ADD COLUMN x int;\nUPDATE models SET x = 1;",
	} {
		if got := statementMutations(rejected); len(got) == 0 {
			t.Errorf("statementMutations(%q) found nothing, want a data-mutating statement", rejected)
		}
	}
}
