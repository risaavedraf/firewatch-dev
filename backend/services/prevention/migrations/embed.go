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

//go:embed 014_action_assignee.sql
var actionAssignee string

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
// The entries are numbers 9, 11 and 14, not 1, 2 and 3: this service's
// migrations share the risk database's schema_migrations ledger, where risk
// claims 1-8, 10, 12 and 13. Claiming any of those numbers here would make one
// of the two services' migrations skip silently on the shared ledger, so
// prevention's migrations take the next free numbers across both services: 9, 11
// and 14. The rule is in backend/docs/migration-ledger.md. The runner applies whatever is
// still pending in schema_migrations and does nothing on a database that is
// already up to date.
func All() []Migration {
	return []Migration{
		{Version: 9, Name: "preventive_actions", SQL: preventiveActions},
		{Version: 11, Name: "preventive_evidence", SQL: preventiveEvidence},
		{Version: 14, Name: "action_assignee", SQL: actionAssignee},
	}
}
