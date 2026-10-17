package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"firewatch.local/jobs/migrations"
)

// A structural guard is appropriate here because a real migration invocation
// requires PostgreSQL. It prevents an embedded migration being silently omitted
// by the CLI runner; it does not prove that the SQL executes successfully.
func TestMigrateRegistersAllForwardMigrations(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	registered := map[int]string{}
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		version := 0
		name := ""
		for _, element := range literal.Elts {
			field, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := field.Key.(*ast.Ident)
			if !ok {
				continue
			}
			switch key.Name {
			case "version":
				number, ok := field.Value.(*ast.BasicLit)
				if ok {
					version, _ = strconv.Atoi(number.Value)
				}
			case "sql":
				selection, ok := field.Value.(*ast.SelectorExpr)
				if ok {
					owner, ok := selection.X.(*ast.Ident)
					if ok && owner.Name == "migrations" {
						name = selection.Sel.Name
					}
				}
			}
		}
		if version != 0 && name != "" {
			if _, duplicate := registered[version]; duplicate {
				t.Fatalf("duplicate migration version %d", version)
			}
			registered[version] = name
		}
		return true
	})
	expected := map[int]string{1: "Initial", 2: "Zones", 3: "SyntheticScenario", 4: "JobFailureState"}
	if len(registered) != len(expected) {
		t.Fatalf("registered migrations = %v, want %v", registered, expected)
	}
	for version, name := range expected {
		if registered[version] != name {
			t.Fatalf("version %d SQL = %q, want %q", version, registered[version], name)
		}
	}
	if migrations.Initial == "" || migrations.Zones == "" || migrations.SyntheticScenario == "" || migrations.JobFailureState == "" {
		t.Fatal("registered SQL is empty")
	}
}

// TestJobFailureMigrationIsGuardedAndForwardOnly pins the shape of the terminal
// failure migration: the new columns are added IF NOT EXISTS, the frozen state
// CHECK is dropped before it is (re)added with the 'failed' value, and nothing
// here destroys data. It is a structural test: it cannot prove execution against
// a real database.
func TestJobFailureMigrationIsGuardedAndForwardOnly(t *testing.T) {
	sql := migrations.JobFailureState
	for _, fragment := range []string{
		"ADD COLUMN IF NOT EXISTS attempts integer NOT NULL DEFAULT 0",
		"ADD COLUMN IF NOT EXISTS failure_reason text",
		"ADD COLUMN IF NOT EXISTS failed_at timestamptz",
		"DROP CONSTRAINT IF EXISTS jobs_state_check",
		"ADD CONSTRAINT jobs_state_check CHECK (state IN ('queued', 'published', 'failed'))",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("004_job_failure_state.sql does not contain %q", fragment)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "TRUNCATE", "DELETE FROM", "DROP COLUMN", "UPDATE "} {
		if strings.Contains(sql, forbidden) {
			t.Errorf("004_job_failure_state.sql contains %q; migrations here never destroy data", forbidden)
		}
	}
}
