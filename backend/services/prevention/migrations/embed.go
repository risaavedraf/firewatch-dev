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

//go:embed 011_preventive_evidence.sql
var preventiveEvidence string

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
// The entries are versions 9 and 11, not 1 and 2: this service's migrations share
// the risk database's schema_migrations ledger, which already holds risk's
// 001..008 and 010. Version 8 belongs to risk's 008_firewatch_rule_v2.sql and
// version 10 to risk's 010_models_activation_grant.sql, so claiming either here
// would make one of the two migrations skip silently on the shared ledger.
// Version 11 is the next free version and is deliberately not 10. The runner
// applies whatever is still pending in schema_migrations and does nothing on a
// database that is already up to date.
func All() []Migration {
	return []Migration{
		{Version: 9, Name: "preventive_actions", SQL: preventiveActions},
		{Version: 11, Name: "preventive_evidence", SQL: preventiveEvidence},
	}
}
