// Package migrations carries the schema migrations of the risk service.
//
// Each SQL file is embedded verbatim, so the migration a deployed binary applies
// is exactly the migration in the repository: there is no external file that can
// drift or be forgotten at deploy time. Migrations are forward-only and guarded
// so that re-applying one is a no-op.
package migrations

import _ "embed"

//go:embed 001_initial.sql
var initial string

//go:embed 002_models.sql
var models string

//go:embed 003_policy_ref.sql
var policyRef string

//go:embed 004_zones.sql
var zones string

//go:embed 005_zone_reference.sql
var zoneReference string

//go:embed 006_models_integrity.sql
var modelsIntegrity string

//go:embed 007_synthetic_evaluation.sql
var syntheticEvaluation string

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
// Adding a migration means adding one file under this directory plus one entry
// here, with the next version number and a short name. The runner applies
// whatever is still pending in schema_migrations and does nothing on a database
// that is already up to date.
func All() []Migration {
	return []Migration{
		{Version: 1, Name: "initial", SQL: initial},
		{Version: 2, Name: "models", SQL: models},
		{Version: 3, Name: "policy_ref", SQL: policyRef},
		{Version: 4, Name: "zones", SQL: zones},
		{Version: 5, Name: "zone_reference", SQL: zoneReference},
		{Version: 6, Name: "models_integrity", SQL: modelsIntegrity},
		{Version: 7, Name: "synthetic_evaluation", SQL: syntheticEvaluation},
	}
}
