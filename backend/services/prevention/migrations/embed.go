// Package migrations carries the schema migrations of the prevention service.
//
// Each SQL file is embedded verbatim, so the migration a deployed binary applies
// is exactly the migration in the repository: there is no external file that can
// drift or be forgotten at deploy time. Migrations are forward-only and guarded
// so that re-applying one is a no-op.
package migrations

import _ "embed"

//go:embed 009_preventive_actions.sql
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
// The single entry is version 9, not version 1: this service's migration shares
// the risk database's schema_migrations ledger, which already holds risk's
// 001..008. Version 8 belongs to risk's 008_firewatch_rule_v2.sql, so claiming it
// here would make one of the two migrations skip silently on the shared ledger.
// The runner applies whatever is still pending in schema_migrations and does
// nothing on a database that is already up to date.
func All() []Migration {
	return []Migration{
		{Version: 9, Name: "preventive_actions", SQL: preventiveActions},
	}
}
