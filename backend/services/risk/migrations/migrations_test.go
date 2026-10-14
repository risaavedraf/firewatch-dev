package migrations

import (
	"regexp"
	"strings"
	"testing"

	"firewatch.local/risk/internal/classify"
)

// quotedSHA256 matches a single-quoted lowercase sha256 literal in SQL.
var quotedSHA256 = regexp.MustCompile(`'([0-9a-f]{64})'`)

// migrationFor returns the migration with the given version, failing the test
// when it is absent.
func migrationFor(t *testing.T, version int) Migration {
	t.Helper()
	for _, migration := range All() {
		if migration.Version == version {
			return migration
		}
	}
	t.Fatalf("no migration with version %d", version)
	return Migration{}
}

func TestMigrationsAreOrderedCompleteAndNonEmpty(t *testing.T) {
	all := All()
	if len(all) == 0 {
		t.Fatal("All() returned no migrations")
	}
	for index, migration := range all {
		want := index + 1
		if migration.Version != want {
			t.Fatalf("migration at index %d has version %d, want %d; versions must start at 1 and increase by exactly 1 with no gaps or duplicates", index, migration.Version, want)
		}
		if migration.Name == "" {
			t.Errorf("migration %d has an empty Name; a failure must be attributable", migration.Version)
		}
		if strings.TrimSpace(migration.SQL) == "" {
			t.Errorf("migration %d (%s) has empty SQL", migration.Version, migration.Name)
		}
	}
}

func TestInitialMigrationIsNeverRewritten(t *testing.T) {
	// 001_initial.sql is frozen. Version 1 is already recorded in
	// schema_migrations on every existing database, so an edit to that file would
	// never run again and would be silently ineffective.
	sql := migrationFor(t, 1).SQL
	for _, fragment := range []string{
		"model_version = 'demo-fixture-v1'",
		"CHECK (demo)",
		"CHECK (zone_id = 'valparaiso-demo')",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("001_initial.sql does not contain %q; that file is frozen and must not be rewritten", fragment)
		}
	}
}

func TestModelsMigrationIsGuardedAndIdempotent(t *testing.T) {
	// Every statement is guarded so that applying the file twice is a no-op, and
	// schema migrations here never destroy data.
	sql := migrationFor(t, 2).SQL
	for _, fragment := range []string{
		"DROP CONSTRAINT IF EXISTS batches_model_version_check",
		"DROP CONSTRAINT IF EXISTS batches_demo_check",
		"CREATE TABLE IF NOT EXISTS models",
		"ADD COLUMN IF NOT EXISTS risk_level",
		"ON CONFLICT (version) DO NOTHING",
		"FOREIGN KEY (model_version) REFERENCES models(version)",
		"GRANT SELECT ON models TO risk_app",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("002_models.sql does not contain %q", fragment)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "TRUNCATE", "DELETE FROM"} {
		if strings.Contains(sql, forbidden) {
			t.Errorf("002_models.sql contains %q; schema migrations here never destroy data", forbidden)
		}
	}
}

func TestModelsMigrationCarriesTheFrozenParamsHash(t *testing.T) {
	// Second half of the golden guard: the migration records the fingerprint of
	// academic-rule-v1's effective parameters, so a behaviour change cannot be
	// shipped while the recorded version keeps its old identity.
	//
	// The literal is extracted and counted rather than merely searched for, so
	// mentioning the hash in a comment cannot satisfy this test while the value
	// actually stored falls behind.
	sql := migrationFor(t, 2).SQL
	recorded := quotedSHA256.FindAllStringSubmatch(sql, -1)
	if len(recorded) != 1 {
		t.Fatalf("002_models.sql records %d quoted sha256 literals, want exactly 1", len(recorded))
	}
	if got := recorded[0][1]; got != classify.AcademicRuleV1ParamsHash {
		t.Errorf("002_models.sql records params hash %q, want the frozen %q", got, classify.AcademicRuleV1ParamsHash)
	}
}

func TestPolicyRefMigrationIsGuardedAndIdempotent(t *testing.T) {
	// The column and its constraint are added so that re-applying the file is a
	// no-op: the column is created IF NOT EXISTS and the constraint is dropped
	// before it is added. schema migrations here never destroy data.
	sql := migrationFor(t, 3).SQL
	for _, fragment := range []string{
		"ADD COLUMN IF NOT EXISTS policy_ref",
		"DROP CONSTRAINT IF EXISTS batches_policy_ref_check",
		"ADD CONSTRAINT batches_policy_ref_check",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("003_policy_ref.sql does not contain %q", fragment)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "TRUNCATE", "DELETE FROM", "UPDATE "} {
		if strings.Contains(sql, forbidden) {
			t.Errorf("003_policy_ref.sql contains %q; schema migrations here never destroy data", forbidden)
		}
	}
}

func TestZonesMigrationIsGuardedAndIdempotent(t *testing.T) {
	// The zones table, its spatial index and the legacy demo row are the only
	// things this file may create; the 38 communes come from the embedded asset
	// in the seed step, so no CUT or geometry is duplicated in SQL. Every
	// statement is guarded so that applying the file twice is a no-op.
	migration := migrationFor(t, 4)
	if migration.Name != "zones" {
		t.Errorf("migration 4 is named %q, want %q", migration.Name, "zones")
	}
	sql := migration.SQL
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS zones",
		"geometry geometry(Geometry,4326)",
		"CHECK (kind IN ('comuna','demo'))",
		"CREATE INDEX IF NOT EXISTS zones_geometry ON zones USING gist(geometry)",
		"ON CONFLICT (code) DO NOTHING",
		"GRANT SELECT ON zones TO risk_app",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("004_zones.sql does not contain %q", fragment)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "TRUNCATE", "DELETE FROM", "UPDATE "} {
		if strings.Contains(sql, forbidden) {
			t.Errorf("004_zones.sql contains %q; schema migrations here never destroy data", forbidden)
		}
	}
}

func TestZoneReferenceMigrationIsGuardedAndIdempotent(t *testing.T) {
	// 005 replaces the frozen CHECK (zone_id = 'valparaiso-demo') with a foreign
	// key into the zones catalogue. Version 5 must exist, both the old CHECK and
	// any prior foreign key are dropped before the foreign key is (re)added so
	// that applying the file twice is a no-op, and nothing here destroys data.
	migration := migrationFor(t, 5)
	if migration.Name != "zone_reference" {
		t.Errorf("migration 5 is named %q, want %q", migration.Name, "zone_reference")
	}
	sql := migration.SQL
	for _, fragment := range []string{
		"ALTER TABLE batches DROP CONSTRAINT IF EXISTS batches_zone_id_check",
		"ALTER TABLE batches DROP CONSTRAINT IF EXISTS batches_zone_id_fkey",
		"ADD CONSTRAINT batches_zone_id_fkey FOREIGN KEY (zone_id) REFERENCES zones(code)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("005_zone_reference.sql does not contain %q", fragment)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "TRUNCATE", "DELETE FROM", "UPDATE "} {
		if strings.Contains(sql, forbidden) {
			t.Errorf("005_zone_reference.sql contains %q; schema migrations here never destroy data", forbidden)
		}
	}
}

func TestSyntheticEvaluationMigrationIsGuardedAndIdempotent(t *testing.T) {
	// 007 adds the nullable scenario metadata to batches. Both columns are added
	// IF NOT EXISTS and the pairing constraint is dropped before it is added, so
	// applying the file twice is a no-op, and nothing here destroys or backfills
	// historical data: a legacy row keeps NULL metadata.
	migration := migrationFor(t, 7)
	if migration.Name != "synthetic_evaluation" {
		t.Errorf("migration 7 is named %q, want %q", migration.Name, "synthetic_evaluation")
	}
	sql := migration.SQL
	for _, fragment := range []string{
		"ADD COLUMN IF NOT EXISTS scenario_id",
		"ADD COLUMN IF NOT EXISTS evaluated_at",
		"DROP CONSTRAINT IF EXISTS batches_synthetic_metadata_check",
		"ADD CONSTRAINT batches_synthetic_metadata_check",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("007_synthetic_evaluation.sql does not contain %q", fragment)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "TRUNCATE", "DELETE FROM", "UPDATE "} {
		if strings.Contains(sql, forbidden) {
			t.Errorf("007_synthetic_evaluation.sql contains %q; schema migrations here never destroy data", forbidden)
		}
	}
}

func TestModelsIntegrityMigrationIsGuardedAndIdempotent(t *testing.T) {
	// 006 states the registry's structural invariants in the schema: a placeholder
	// can never be active, and at most one model may be active. The constraint is
	// dropped before it is added and the index is created IF NOT EXISTS, so
	// applying the file twice is a no-op, and nothing here destroys data.
	migration := migrationFor(t, 6)
	if migration.Name != "models_integrity" {
		t.Errorf("migration 6 is named %q, want %q", migration.Name, "models_integrity")
	}
	sql := migration.SQL
	for _, fragment := range []string{
		"DROP CONSTRAINT IF EXISTS models_placeholder_not_active_check",
		"ADD CONSTRAINT models_placeholder_not_active_check",
		"CHECK (kind <> 'placeholder' OR active = false)",
		"CREATE UNIQUE INDEX IF NOT EXISTS models_single_active ON models(active) WHERE active",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("006_models_integrity.sql does not contain %q", fragment)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "TRUNCATE", "DELETE FROM", "UPDATE "} {
		if strings.Contains(sql, forbidden) {
			t.Errorf("006_models_integrity.sql contains %q; schema migrations here never destroy data", forbidden)
		}
	}
}
