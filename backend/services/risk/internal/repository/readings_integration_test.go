package repository

import (
	"context"
	"os"
	"testing"
	"time"

	observations "firewatch.local/contracts/observations/v1"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This file pins the SQL contract the daily aggregates are computed from. The
// aggregate rules themselves are unit-tested in internal/features against an
// in-memory history; what only a real database can prove is that the identity
// constraint really makes re-ingestion a no-op and that the window query is
// half-open and scoped to one variable in one zone.
//
// It runs only against a migrated disposable database, following the same skip
// guard as the other integration tests in this package.

func TestAppendReadingsIsIdempotentAndWindowed(t *testing.T) {
	if testing.Short() || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires migrated disposable risk database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := Store{DB: db}

	// A zone of its own, so the test neither reads nor disturbs rows another test
	// wrote. There is deliberately no foreign key to zones (see 012_readings.sql),
	// which is what makes a synthetic zone identifier legal here.
	zone := "readings-test-" + uuid.NewString()[:8]
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	record := func(variable string, at time.Time, value float64, unit string) observations.Observation {
		return observations.Observation{
			SourceID:   "readings-integration-test",
			Variable:   variable,
			ZoneID:     zone,
			ObservedAt: at,
			Value:      value,
			Unit:       unit,
			Quality:    observations.QualityOK,
		}
	}
	records := []observations.Observation{
		record(observations.VariableTemperature, base, 31, "C"),
		record(observations.VariableTemperature, base.Add(time.Hour), 33, "C"),
		record("precipitacion", base, 4, "mm"),
		// A second zone with the same identity, to prove the uniqueness constraint is
		// scoped by zone rather than global.
		func() observations.Observation {
			other := record(observations.VariableTemperature, base, 20, "C")
			other.ZoneID = zone + "-b"
			return other
		}(),
	}

	if err := store.AppendReadings(ctx, records); err != nil {
		t.Fatalf("append readings: %v", err)
	}
	// Re-ingesting the same retrieval must be a no-op, not a second row: this is what
	// keeps a redelivered publication from inflating a precipitation sum.
	if err := store.AppendReadings(ctx, records); err != nil {
		t.Fatalf("re-append readings: %v", err)
	}
	var stored int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM readings WHERE zone_id IN ($1,$2)`, zone, zone+"-b").Scan(&stored); err != nil {
		t.Fatalf("count readings: %v", err)
	}
	if stored != len(records) {
		t.Fatalf("stored %d rows after ingesting %d readings twice, want %d", stored, len(records), len(records))
	}

	// The window is half-open: the reading at exactly `to` belongs to the next
	// window, so two adjacent windows never both contain it.
	window, err := store.ReadingsWindow(ctx, zone, observations.VariableTemperature, base, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("read the window: %v", err)
	}
	if len(window) != 1 || window[0].Value != 31 {
		t.Fatalf("window = %+v, want only the reading at the start instant", window)
	}
	if window[0].Unit != "C" {
		t.Fatalf("the stored unit is %q, want the unit the reading arrived in", window[0].Unit)
	}

	window, err = store.ReadingsWindow(ctx, zone, observations.VariableTemperature, base, base.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("read the wider window: %v", err)
	}
	if len(window) != 2 || window[0].Value != 31 || window[1].Value != 33 {
		t.Fatalf("window = %+v, want both readings oldest first", window)
	}

	// One variable in one zone: the precipitation reading of the same instant must
	// not appear in a temperature window, and the other zone's temperature must not
	// appear either.
	window, err = store.ReadingsWindow(ctx, zone, "precipitacion", base, base.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("read the precipitation window: %v", err)
	}
	if len(window) != 1 || window[0].Value != 4 {
		t.Fatalf("precipitation window = %+v, want the single 4 mm reading", window)
	}
	window, err = store.ReadingsWindow(ctx, zone+"-b", observations.VariableTemperature, base, base.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("read the other zone's window: %v", err)
	}
	if len(window) != 1 || window[0].Value != 20 {
		t.Fatalf("other zone window = %+v, want its own single reading", window)
	}
}
