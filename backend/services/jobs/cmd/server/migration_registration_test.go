package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
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
	expected := map[int]string{1: "Initial", 2: "Zones", 3: "SyntheticScenario"}
	if len(registered) != len(expected) {
		t.Fatalf("registered migrations = %v, want %v", registered, expected)
	}
	for version, name := range expected {
		if registered[version] != name {
			t.Fatalf("version %d SQL = %q, want %q", version, registered[version], name)
		}
	}
	if migrations.Initial == "" || migrations.Zones == "" || migrations.SyntheticScenario == "" {
		t.Fatal("registered SQL is empty")
	}
}
