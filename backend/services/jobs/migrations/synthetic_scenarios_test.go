package migrations

import (
	"strings"
	"testing"
)

// TestSyntheticScenarioMigrationIsGuardedAndForwardOnly pins the shape of the
// forward migration: the metadata columns are added IF NOT EXISTS, the pairing
// constraint is dropped before it is (re)added so re-applying the file is a
// no-op, and nothing here destroys data. It is a structural test: it cannot
// prove execution against a real database (see the integration test).
func TestSyntheticScenarioMigrationIsGuardedAndForwardOnly(t *testing.T) {
	sql := SyntheticScenario
	for _, fragment := range []string{
		"ADD COLUMN IF NOT EXISTS scenario_id text",
		"ADD COLUMN IF NOT EXISTS evaluated_at timestamptz",
		"DROP CONSTRAINT IF EXISTS jobs_synthetic_metadata_pairing",
		"ADD CONSTRAINT jobs_synthetic_metadata_pairing",
		"scenario_id IS NULL AND evaluated_at IS NULL",
		"scenario_id IS NOT NULL AND evaluated_at IS NOT NULL",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("003_synthetic_scenario.sql does not contain %q", fragment)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "TRUNCATE", "DELETE FROM", "UPDATE ", "DROP COLUMN", "NOT NULL DEFAULT"} {
		if strings.Contains(sql, forbidden) {
			t.Errorf("003_synthetic_scenario.sql contains %q; migrations here never destroy data and never backfill", forbidden)
		}
	}
}

// TestFrozenJobsMigrationsAreNeverRewritten pins the markers of the two applied
// files. A later version may only be added, never an edit to a version already
// recorded in schema_migrations, because such an edit would never run again.
func TestFrozenJobsMigrationsAreNeverRewritten(t *testing.T) {
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS jobs",
		"CHECK (zone_id = 'valparaiso-demo')",
		"UNIQUE(actor_id, idempotency_key)",
	} {
		if !strings.Contains(Initial, fragment) {
			t.Errorf("001_initial.sql no longer contains %q; that file is frozen", fragment)
		}
	}
	for _, fragment := range []string{
		"DROP CONSTRAINT IF EXISTS jobs_zone_id_check",
		"ADD CONSTRAINT jobs_zone_id_fkey FOREIGN KEY (zone_id) REFERENCES zones(code)",
	} {
		if !strings.Contains(Zones, fragment) {
			t.Errorf("002_zones.sql no longer contains %q; that file is frozen", fragment)
		}
	}
}
