package repository

import (
	"context"
	"time"

	observations "firewatch.local/contracts/observations/v1"
)

// This file is the readings history: the storage the trained model's daily
// aggregates are computed from.
//
// The aggregates themselves are deliberately NOT rolled up into a table here.
// At the volume this service publishes - roughly 38 communes x a handful of
// variables x 96 readings a day - a rollup would add a second source of truth, a
// backfill job and a rebuild path, and would still have to answer "what if a late
// reading arrives". Computing on read keeps one source of truth (the readings)
// and is cheap at this size; see internal/features, which is the only reader.

// StoredReading is one row of the readings history: one value of one variable, in
// one zone, at one instant.
//
// ObservedAt is the instant the reading describes, not the instant it was stored:
// the ingest time is kept in the row but never takes part in an aggregate, so
// re-ingesting a retrieval cannot move a reading into another civil day.
type StoredReading struct {
	ZoneID     string
	Variable   string
	ObservedAt time.Time
	Value      float64
	Unit       string
}

// AppendReadings stores one retrieval's readings.
//
// Re-ingestion is idempotent by the table's own identity (zone, variable,
// instant): a redelivered publication, a restart mid-cycle or a second replica
// publishing the same retrieval inserts nothing and leaves every aggregate
// unchanged. The conflict is resolved with DO NOTHING rather than DO UPDATE: this
// is an append-only history of what a source said at an instant, so the first
// stored value stands and a correction has to arrive as its own reading. An
// update would also make "re-ingesting the same readings does not change
// aggregates" true only when the values happen to agree.
//
// The statements run one reading at a time inside one transaction. A publication
// carries a handful of readings, the round trips are local, and an explicit
// per-row statement stays readable where an assembled multi-row VALUES list would
// have to build placeholders.
func (s Store) AppendReadings(ctx context.Context, readings []observations.Observation) error {
	if len(readings) == 0 {
		return nil
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, reading := range readings {
		// The instant is normalized to UTC before it is stored. timestamptz keeps
		// the instant either way, and normalizing means the identity used for
		// idempotency is the instant itself rather than the way a provider happened
		// to render it.
		if _, err := tx.Exec(ctx, `INSERT INTO readings(zone_id,variable,observed_at,value,unit,source_id,quality)
			VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (zone_id,variable,observed_at) DO NOTHING`,
			reading.ZoneID, reading.Variable, reading.ObservedAt.UTC(), reading.Value, reading.Unit, reading.SourceID, string(reading.Quality)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ReadingsWindow returns the stored readings of one variable in one zone inside
// the half-open interval [from, to), oldest first.
//
// The half-open interval is what makes a day boundary unambiguous: a reading at
// exactly midnight belongs to the day that starts there and to no other, so two
// adjacent windows never both contain it. Ordering by instant means a caller
// that sums or takes extremes gets the same answer regardless of storage order,
// and a tie between two rows is impossible because the identity is unique.
func (s Store) ReadingsWindow(ctx context.Context, zoneID, variable string, from, to time.Time) ([]StoredReading, error) {
	rows, err := s.DB.Query(ctx, `SELECT zone_id,variable,observed_at,value,unit FROM readings
		WHERE zone_id=$1 AND variable=$2 AND observed_at >= $3 AND observed_at < $4
		ORDER BY observed_at`, zoneID, variable, from.UTC(), to.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	readings := make([]StoredReading, 0)
	for rows.Next() {
		var reading StoredReading
		if err := rows.Scan(&reading.ZoneID, &reading.Variable, &reading.ObservedAt, &reading.Value, &reading.Unit); err != nil {
			return nil, err
		}
		readings = append(readings, reading)
	}
	return readings, rows.Err()
}
