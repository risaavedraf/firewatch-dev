package zones

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestEmbeddedComunasArePersistedInPostGIS(t *testing.T) {
	if testing.Short() || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires migrated disposable risk database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var total, comunas, demos, nullCommunes int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM zones`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM zones WHERE kind='comuna'`).Scan(&comunas); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM zones WHERE kind='demo'`).Scan(&demos); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM zones WHERE kind='comuna' AND geometry IS NULL`).Scan(&nullCommunes); err != nil {
		t.Fatal(err)
	}
	if total != 39 || comunas != 38 || demos != 1 {
		t.Fatalf("zones holds total=%d comunas=%d demos=%d, want 39/38/1", total, comunas, demos)
	}
	if nullCommunes != 0 {
		t.Errorf("%d communes have NULL geometry, want 0", nullCommunes)
	}

	// The legacy row keeps its declared state: it predates the asset and stays
	// without geometry and marked demo.
	var demoCode string
	if err = db.QueryRow(ctx, `SELECT code FROM zones WHERE kind='demo' AND geometry IS NULL`).Scan(&demoCode); err != nil {
		t.Errorf("the legacy demo row with NULL geometry is missing: %v", err)
	} else if demoCode != "valparaiso-demo" {
		t.Errorf("legacy demo row is %q, want valparaiso-demo", demoCode)
	}

	var first, last string
	if err = db.QueryRow(ctx, `SELECT min(code), max(code) FROM zones WHERE kind='comuna'`).Scan(&first, &last); err != nil {
		t.Fatal(err)
	}
	if first != "05101" || last != "05804" {
		t.Errorf("commune codes span %s..%s, want 05101..05804", first, last)
	}

	// Report invalid or mis-projected geometries instead of repairing them: a
	// broken asset is a defect to surface, not a value to rewrite silently.
	invalid, err := db.Query(ctx, `SELECT code FROM zones WHERE kind='comuna' AND (geometry IS NULL OR NOT ST_IsValid(geometry)) ORDER BY code`)
	if err != nil {
		t.Fatal(err)
	}
	for invalid.Next() {
		var code string
		if err = invalid.Scan(&code); err != nil {
			invalid.Close()
			t.Fatal(err)
		}
		t.Errorf("commune %s has an invalid or missing geometry", code)
	}
	invalid.Close()
	if err = invalid.Err(); err != nil {
		t.Fatal(err)
	}
	projected, err := db.Query(ctx, `SELECT code, ST_SRID(geometry) FROM zones WHERE kind='comuna' AND ST_SRID(geometry) <> 4326 ORDER BY code`)
	if err != nil {
		t.Fatal(err)
	}
	for projected.Next() {
		var code string
		var srid int
		if err = projected.Scan(&code, &srid); err != nil {
			projected.Close()
			t.Fatal(err)
		}
		t.Errorf("commune %s has SRID %d, want 4326", code, srid)
	}
	projected.Close()
	if err = projected.Err(); err != nil {
		t.Fatal(err)
	}

	// Idempotency. Writing zones requires the migrate owner's rights; the
	// application role only has SELECT, so this half of the contract is declared
	// here and exercised when TEST_DATABASE_URL carries a writable role.
	var canWrite bool
	if err = db.QueryRow(ctx, `SELECT has_table_privilege('zones','INSERT')`).Scan(&canWrite); err != nil {
		t.Fatal(err)
	}
	if !canWrite {
		t.Log("TEST_DATABASE_URL role cannot write zones; re-seed idempotency is verified by the migrate owner run")
		return
	}
	before := zonesSnapshot(t, ctx, db)
	written, err := SeedEmbedded(ctx, db)
	if err != nil {
		t.Fatalf("re-run SeedEmbedded: %v", err)
	}
	if written != 38 {
		t.Errorf("re-run SeedEmbedded wrote %d zones, want 38", written)
	}
	if after := zonesSnapshot(t, ctx, db); after != before {
		t.Errorf("re-running the seed changed the zones table:\nbefore %s\nafter  %s", before, after)
	}
}

// zonesSnapshot returns a deterministic digest of the whole table so two states
// can be compared without depending on row order.
func zonesSnapshot(t *testing.T, ctx context.Context, db *pgxpool.Pool) string {
	t.Helper()
	var digest string
	err := db.QueryRow(ctx, `SELECT md5(string_agg(code||'|'||name||'|'||coalesce(province,'')||'|'||kind||'|'||coalesce(ST_AsEWKB(geometry)::text,'NULL'), ',' ORDER BY code)) FROM zones`).Scan(&digest)
	if err != nil {
		t.Fatalf("snapshot zones: %v", err)
	}
	return digest
}
