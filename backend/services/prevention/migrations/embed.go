// Package migrations carries the schema migrations of the prevention service.
//
// Each SQL file is embedded verbatim, so the migration a deployed binary applies
// is exactly the migration in the repository: there is no external file that can
// drift or be forgotten at deploy time. Migrations are forward-only and guarded
// so that re-applying one is a no-op.
package migrations

import _ "embed"

//go:embed 008_preventive_actions.sql
var preventiveActions string

// Migration is one forward-only schema migration.
//
// Name exists so a failure can be attributed without reading the SQL.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// All returns the known migrations in ascending version order.
//
// The single entry is version 8, not version 1: the frozen contract fixes this
// file as 008_preventive_actions.sql and the ledger it is recorded in already
// holds the seven risk migrations. The runner applies whatever is still pending
// in schema_migrations and does nothing on a database that is already up to
// date, so the shared ledger stays consistent whichever runner executes first.
func All() []Migration {
	return []Migration{
		{Version: 8, Name: "preventive_actions", SQL: preventiveActions},
	}
}
