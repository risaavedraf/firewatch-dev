package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestCatalogueMatchesRiskReference is the cross-database guard of the
// geospatial feature: jobs and risk live in separate databases, so a foreign key
// cannot hold the two catalogues together. Both are materialised from the same
// embedded asset (contracts/gen/geo) plus the one legacy demo row, and this test
// is the only place that proves the single source landed identically in both.
//
// It needs TWO databases and skips unless both are set:
//   - TEST_DATABASE_URL: the migrated jobs database (the local catalogue, no
//     geometry).
//   - TEST_ZONES_REFERENCE_DATABASE_URL: the migrated risk database (the
//     reference catalogue, with geometry). The two may differ only in the
//     geometry column; their sets of codes must be EXACTLY equal.
func TestCatalogueMatchesRiskReference(t *testing.T) {
	jobsDSN := os.Getenv("TEST_DATABASE_URL")
	riskDSN := os.Getenv("TEST_ZONES_REFERENCE_DATABASE_URL")
	if testing.Short() || jobsDSN == "" || riskDSN == "" {
		t.Skip("requires BOTH TEST_DATABASE_URL (jobs) and TEST_ZONES_REFERENCE_DATABASE_URL (risk)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	jobs, err := pgxpool.New(ctx, jobsDSN)
	if err != nil {
		t.Fatalf("connect jobs catalogue: %v", err)
	}
	defer jobs.Close()
	risk, err := pgxpool.New(ctx, riskDSN)
	if err != nil {
		t.Fatalf("connect risk reference catalogue: %v", err)
	}
	defer risk.Close()

	// Prove the guard opened TWO distinct connections with the two expected
	// application roles. Postgres grants do not cross databases, so a run that
	// read the same database twice, or read the reference with the jobs role,
	// cannot satisfy these assertions. This also preserves the grant proof: the
	// application role itself is what reads the reference catalogue.
	assertCatalogueConnection(t, ctx, jobs, "jobs", "jobs_app", "jobs_test")
	assertCatalogueConnection(t, ctx, risk, "risk", "risk_app", "risk_test")

	jobsCodes := readCatalogueCodes(t, ctx, jobs, "jobs")
	riskCodes := readCatalogueCodes(t, ctx, risk, "risk")
	t.Logf("jobs catalogue holds %d codes; risk catalogue holds %d codes", len(jobsCodes), len(riskCodes))

	// An empty-vs-empty comparison would satisfy the detector trivially, so the
	// guard refuses to compare trivial catalogues: each side must hold more than
	// one row before equality means anything.
	if len(jobsCodes) < 2 || len(riskCodes) < 2 {
		t.Fatalf("catalogues must both be non-trivial before comparing: jobs=%d risk=%d", len(jobsCodes), len(riskCodes))
	}

	// The two code sets come straight from the two real databases and the pure
	// detector decides. This is the positive case: the real catalogues must agree.
	// The detector carries no database access, so its negative case (a planted
	// divergence) runs unconditionally in zones_test.go, where no Postgres is
	// needed. Neither half can substitute for the other: this one proves the real
	// data agrees, and the pure one proves a disagreement is actually detected.
	if err := compareCatalogueCodes(jobsCodes, riskCodes); err != nil {
		t.Errorf("the jobs catalogue and the risk reference catalogue diverged: %v", err)
	}
}

// readCatalogueCodes returns the codes of a zones table in ascending order.
//
// The table name is a fixed literal chosen by the caller, never user input, so
// interpolating it is safe; a parameter placeholder cannot carry an identifier.
func readCatalogueCodes(t *testing.T, ctx context.Context, db *pgxpool.Pool, catalogue string) []string {
	t.Helper()
	rows, err := db.Query(ctx, `SELECT code FROM zones ORDER BY code`)
	if err != nil {
		t.Fatalf("read %s.zones: %v", catalogue, err)
	}
	defer rows.Close()
	codes := make([]string, 0)
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			t.Fatalf("scan %s.zones code: %v", catalogue, err)
		}
		codes = append(codes, code)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate %s.zones: %v", catalogue, err)
	}
	return codes
}

// assertCatalogueConnection proves which role and database a pool actually
// reached. It is the composition guarantee of the cross-database guard: two
// distinct connections, two distinct application roles, two distinct databases.
func assertCatalogueConnection(t *testing.T, ctx context.Context, db *pgxpool.Pool, catalogue, wantRole, wantDatabase string) {
	t.Helper()
	var role, database string
	if err := db.QueryRow(ctx, `SELECT current_user, current_database()`).Scan(&role, &database); err != nil {
		t.Fatalf("identify %s connection: %v", catalogue, err)
	}
	t.Logf("%s catalogue connection: current_user=%s current_database=%s", catalogue, role, database)
	if role != wantRole {
		t.Errorf("%s connection user is %q, want %q", catalogue, role, wantRole)
	}
	if database != wantDatabase {
		t.Errorf("%s connection database is %q, want %q", catalogue, database, wantDatabase)
	}
}
