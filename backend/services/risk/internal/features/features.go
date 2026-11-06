// Package features builds the trained model's input vector at publication time.
//
// L9 found the mismatch this package closes: the artifact declares seven features,
// but a provider of instantaneous conditions can only answer two of them. The
// history this service now keeps is what makes the other five answerable, and this
// package is the only place that turns that history into the seven values:
//
//   - instantaneous readings of the current retrieval: relative humidity, wind
//     speed, wind direction and solar radiation;
//   - aggregates over the readings history: the daily maximum and minimum
//     temperature of the current civil day, and the precipitation accumulated over
//     the last seven civil days.
//
// Three rules are load-bearing and are the reason this is a component instead of
// a few lines inside the publisher:
//
//  1. The day boundary is a NAMED civil time zone (America/Santiago), loaded once
//     at startup. A "daily maximum" without a named zone is meaningless: cut in
//     UTC, the day boundary would fall at 20:00 or 21:00 local time, so two
//     readings four minutes apart on either side of local midnight would land in
//     the same "day". A host without that zone's data is a startup failure, never
//     a silent UTC fallback.
//  2. An incomplete window is NULL, never zero. Seven days of history are needed
//     for a seven day sum and one covered day for a current-day extreme; if the
//     history cannot cover the window the feature is declared absent, with the
//     coverage count ("0 de 7 días locales") recorded as the reason. A zero
//     precipitation sum from missing data is a lie that changes the prediction.
//  3. A reading in a unit that contradicts its variable is refused, not converted.
//     The unit check happens on the way in (source.Validate, and Record below) and
//     again on the way out of storage, because an aggregate is where a
//     misinterpreted unit becomes a wrong number the model cannot see.
//
// The aggregates are computed on read rather than from a rollup table: at roughly
// 38 communes x a handful of variables x 96 readings a day, a rollup would add a
// second source of truth, a backfill and a rebuild path, and would still have to
// answer what a late reading does to an already computed total.
package features

import (
	"context"
	"errors"
	"fmt"
	"time"

	observations "firewatch.local/contracts/observations/v1"
	"firewatch.local/risk/internal/classify"
	"firewatch.local/risk/internal/repository"
	"firewatch.local/risk/internal/source"
)

// Location is the civil time zone every daily aggregate is cut in.
//
// It is the zone the communes of the Valparaíso catalogue live in, and it is
// spelled here rather than left to the host's TZ because a host configured in UTC
// would otherwise silently answer a different question. Chile observes DST, so a
// day is not always 24 hours long; the windows below are built from civil
// midnights, which is what "a day" means to the people reading the level.
const Location = "America/Santiago"

// precipitationWindowDays is how many civil days precip_sum accumulates.
const precipitationWindowDays = 7

// currentDayCoverage is how many civil days a current-day extreme needs: the day
// in progress, and only that one.
const currentDayCoverage = 1

// coverageUnitDays names what a window coverage counts. It is part of the
// recorded reason, so it is a constant instead of being repeated in prose.
const coverageUnitDays = "días locales"

// History is the readings storage this component needs. repository.Store
// implements it; the interface is declared here so the aggregate rules can be
// tested without a database, and so this package depends on two methods instead of
// on the whole storage surface.
type History interface {
	// AppendReadings stores one retrieval's readings, idempotently by
	// (zone, variable, observed_at).
	AppendReadings(ctx context.Context, readings []observations.Observation) error
	// ReadingsWindow returns the stored readings of one variable in one zone
	// inside [from, to).
	ReadingsWindow(ctx context.Context, zoneID, variable string, from, to time.Time) ([]repository.StoredReading, error)
}

// Source records one retrieval's readings and builds the model input vector.
//
// It is safe for concurrent use: it holds no mutable state beyond the loaded time
// zone and the storage it was built with.
type Source struct {
	history  History
	location *time.Location
}

// New returns the feature source.
//
// The civil time zone is loaded exactly once, here, and a host that cannot supply
// it is a startup error. The alternative - falling back to UTC - would produce
// aggregates that look plausible and answer a different question, which is worse
// than refusing to start.
func New(history History) (*Source, error) {
	if history == nil {
		return nil, errors.New("features: a readings history is required")
	}
	location, err := time.LoadLocation(Location)
	if err != nil {
		return nil, fmt.Errorf("features: load the civil time zone %q: %w (the daily aggregates need a named zone; this host must provide tzdata)", Location, err)
	}
	return &Source{history: history, location: location}, nil
}

// Record stores the readings of one publication.
//
// It runs on the publication path so the history accumulates from the first cycle,
// which is what makes the aggregates stop being null without a separate ingestion
// job. Only the variables the history stores are recorded: a rule-only input such
// as vegetation cover is not a weather reading and has no aggregate to feed.
//
// The zone stored is the zone the PUBLICATION is for, not the zone label the
// provider put on the record. That is the same attribution decision the batch
// makes (the publisher's own guard has already refused any non-synthetic retrieval
// whose labels do not identify the requested zone), and doing it here means a
// documented label divergence - the fixture says "valparaiso" while the demo zone
// is "valparaiso-demo" - cannot split one commune's history into two.
func (s *Source) Record(ctx context.Context, zoneID string, set observations.ObservationSet) error {
	stored := make([]observations.Observation, 0, len(set.Records))
	for _, record := range set.Records {
		if !storesReading(record.Variable) {
			continue
		}
		if !source.UnitMatches(record.Variable, record.Unit) {
			expected, _ := source.ExpectedUnit(record.Variable)
			return fmt.Errorf("features: reading of %s for zone %s declares unit %q, want %q: a reading in another unit must never enter the history an aggregate is computed from", record.Variable, zoneID, record.Unit, expected)
		}
		record.ZoneID = zoneID
		stored = append(stored, record)
	}
	if len(stored) == 0 {
		return nil
	}
	if err := s.history.AppendReadings(ctx, stored); err != nil {
		return fmt.Errorf("features: append %d readings for zone %s: %w", len(stored), zoneID, err)
	}
	return nil
}

// Build returns the seven-value input vector for one zone and one retrieval.
//
// The reference instant is the retrieval's own instant, read from the retrieval
// provenance: this component never reads the system clock, so the vector that
// describes a publication is reproducible from that publication alone.
func (s *Source) Build(ctx context.Context, zoneID string, set observations.ObservationSet) (classify.ModelInput, error) {
	at := set.Provenance.RetrievedAt
	if at.IsZero() {
		return classify.ModelInput{}, errors.New("features: the model input vector needs the retrieval instant, which fixes the civil day the daily aggregates are cut in")
	}
	latest, err := latestByVariable(set.Records)
	if err != nil {
		return classify.ModelInput{}, err
	}
	input := classify.ModelInput{
		HumidityRelPct:   instantaneous(latest, observations.VariableHumidity),
		WindSpeedKmh:     instantaneous(latest, observations.VariableWind),
		WindDirectionDeg: instantaneous(latest, source.VariableWindDirection),
		RadiationWPerM2:  instantaneous(latest, source.VariableRadiation),
	}
	maximum, minimum, err := s.dailyExtremes(ctx, zoneID, at)
	if err != nil {
		return classify.ModelInput{}, err
	}
	input.TempMaxC, input.TempMinC = maximum, minimum
	total, err := s.precipitation(ctx, zoneID, at)
	if err != nil {
		return classify.ModelInput{}, err
	}
	input.PrecipitationMM = total
	return input, nil
}

// dailyExtremes returns the maximum and minimum temperature of the civil day that
// contains at, in the aggregate's zone.
//
// Both are null when the history holds no reading for that day. They are not
// clamped, interpolated or carried over from the previous day: a level computed
// from yesterday's maximum presented as today's would be wrong in exactly the
// direction that matters.
func (s *Source) dailyExtremes(ctx context.Context, zoneID string, at time.Time) (classify.Measured, classify.Measured, error) {
	day := dayAt(at, s.location)
	readings, err := s.window(ctx, zoneID, observations.VariableTemperature, day.Start, day.End)
	if err != nil {
		return classify.Measured{}, classify.Measured{}, err
	}
	coverage := classify.Coverage{
		Covered:  daysCovered(readings, s.location, []localDay{day}),
		Required: currentDayCoverage,
		Unit:     coverageUnitDays,
	}
	if !coverage.Complete() {
		// One reason for both extremes: the window is one window, and a reader that
		// saw two different coverage sentences would look for a difference that is
		// not there.
		absent := classify.Measured{
			Value:    classify.Unknown(),
			Reason:   fmt.Sprintf("ventana incompleta: %s con lectura de %s; se declara nula y el artefacto la imputa", coverage, observations.VariableTemperature),
			Coverage: coverage,
		}
		return absent, absent, nil
	}
	maximum, minimum := extremes(readings)
	return classify.Measured{
		Value:    classify.Known(maximum),
		Reason:   fmt.Sprintf("máximo de %d lecturas del día civil %s (%s)", len(readings), day.Label, Location),
		Coverage: coverage,
	}, classify.Measured{
		Value:    classify.Known(minimum),
		Reason:   fmt.Sprintf("mínimo de %d lecturas del día civil %s (%s)", len(readings), day.Label, Location),
		Coverage: coverage,
	}, nil
}

// precipitation returns the precipitation accumulated over the last seven civil
// days, ending with the day that contains at.
//
// Every one of the seven days must hold at least one reading. Six days of a seven
// day window is not a smaller sum worth reporting: it is a different quantity, and
// reporting it as precip_sum would understate a dry spell's rain exactly when the
// missing day carried the storm.
func (s *Source) precipitation(ctx context.Context, zoneID string, at time.Time) (classify.Measured, error) {
	days := daysEndingAt(at, precipitationWindowDays, s.location)
	readings, err := s.window(ctx, zoneID, source.VariablePrecipitation, days[0].Start, days[len(days)-1].End)
	if err != nil {
		return classify.Measured{}, err
	}
	coverage := classify.Coverage{
		Covered:  daysCovered(readings, s.location, days),
		Required: precipitationWindowDays,
		Unit:     coverageUnitDays,
	}
	if !coverage.Complete() {
		return classify.Measured{
			Value:    classify.Unknown(),
			Reason:   fmt.Sprintf("ventana incompleta: %s con lectura de %s; se declara nula y el artefacto la imputa", coverage, source.VariablePrecipitation),
			Coverage: coverage,
		}, nil
	}
	total := 0.0
	for _, reading := range readings {
		total += reading.Value
	}
	return classify.Measured{
		Value:    classify.Known(total),
		Coverage: coverage,
		Reason:   fmt.Sprintf("suma de %d lecturas entre %s y %s (%s)", len(readings), days[0].Label, days[len(days)-1].Label, Location),
	}, nil
}

// window reads one variable's readings over a half-open instant range and refuses
// a stored row whose unit contradicts the variable's own.
//
// The stored-unit check is the second half of the guard that Record applies on the
// way in. It matters because the history outlives the code that wrote it: a row
// stored by an earlier version, or by another writer, must not be summed with
// today's values just because it happens to be in the table.
func (s *Source) window(ctx context.Context, zoneID, variable string, from, to time.Time) ([]repository.StoredReading, error) {
	readings, err := s.history.ReadingsWindow(ctx, zoneID, variable, from, to)
	if err != nil {
		return nil, fmt.Errorf("features: read %s for zone %s: %w", variable, zoneID, err)
	}
	for _, reading := range readings {
		if !source.UnitMatches(reading.Variable, reading.Unit) {
			expected, _ := source.ExpectedUnit(reading.Variable)
			return nil, fmt.Errorf("features: the stored %s history for zone %s carries unit %q, want %q: a reading in another unit must never enter an aggregate", reading.Variable, zoneID, reading.Unit, expected)
		}
	}
	return readings, nil
}

// latestByVariable keeps the newest reading of each variable the history stores.
//
// On an exact tie the first record in input order is kept, which is the same rule
// the publisher's feature adapter uses, so one retrieval cannot be interpreted two
// different ways depending on which part of the path looks at it.
func latestByVariable(records []observations.Observation) (map[string]observations.Observation, error) {
	kept := make(map[string]observations.Observation, len(records))
	for _, record := range records {
		if !storesReading(record.Variable) {
			continue
		}
		if !source.UnitMatches(record.Variable, record.Unit) {
			expected, _ := source.ExpectedUnit(record.Variable)
			return nil, fmt.Errorf("features: reading of %s declares unit %q, want %q: a reading in another unit must never enter the model input", record.Variable, record.Unit, expected)
		}
		current, seen := kept[record.Variable]
		if !seen || record.ObservedAt.After(current.ObservedAt) {
			kept[record.Variable] = record
		}
	}
	return kept, nil
}

// instantaneous renders one reading of the current retrieval.
func instantaneous(latest map[string]observations.Observation, variable string) classify.Measured {
	record, seen := latest[variable]
	if !seen {
		return classify.Measured{
			Value:  classify.Unknown(),
			Reason: fmt.Sprintf("el conjunto recuperado no trae %s; se declara nula y el artefacto la imputa", variable),
		}
	}
	return classify.Measured{
		Value:  classify.Known(record.Value),
		Reason: fmt.Sprintf("lectura instantánea de %s de las %s", variable, record.ObservedAt.UTC().Format(time.RFC3339)),
	}
}

// extremes returns the maximum and minimum of a non-empty set of readings.
func extremes(readings []repository.StoredReading) (float64, float64) {
	maximum, minimum := readings[0].Value, readings[0].Value
	for _, reading := range readings[1:] {
		if reading.Value > maximum {
			maximum = reading.Value
		}
		if reading.Value < minimum {
			minimum = reading.Value
		}
	}
	return maximum, minimum
}

// storesReading reports whether a variable belongs to the readings history.
func storesReading(variable string) bool {
	for _, candidate := range source.ReadingVariableNames() {
		if variable == candidate {
			return true
		}
	}
	return false
}

// localDay is one civil day in the aggregate's zone: the half-open interval of
// instants it covers, plus the calendar label a coverage reason prints.
type localDay struct {
	// Label is the civil date, for example "2026-10-01".
	Label string
	// Start is the first instant of the day, End the first instant of the next one.
	Start time.Time
	End   time.Time
}

// dayAt returns the civil day that contains at.
//
// The boundaries are built with time.Date on the CIVIL date, not by adding or
// subtracting a fixed duration: Chile observes DST, so the day that starts or ends
// a transition is 23 or 25 hours long, and an arithmetic day boundary would put a
// reading on the wrong side of local midnight twice a year.
func dayAt(at time.Time, location *time.Location) localDay {
	local := at.In(location)
	year, month, day := local.Date()
	start := time.Date(year, month, day, 0, 0, 0, 0, location)
	return localDay{
		Label: start.Format("2006-01-02"),
		Start: start,
		End:   time.Date(year, month, day+1, 0, 0, 0, 0, location),
	}
}

// daysEndingAt returns the last count civil days, ending with the day that
// contains at, in ascending order.
func daysEndingAt(at time.Time, count int, location *time.Location) []localDay {
	local := at.In(location)
	year, month, day := local.Date()
	days := make([]localDay, 0, count)
	for offset := count - 1; offset >= 0; offset-- {
		start := time.Date(year, month, day-offset, 0, 0, 0, 0, location)
		days = append(days, localDay{
			Label: start.Format("2006-01-02"),
			Start: start,
			End:   time.Date(year, month, day-offset+1, 0, 0, 0, 0, location),
		})
	}
	return days
}

// daysCovered counts how many of days hold at least one reading. A day with a
// single reading counts as covered, and a day with none does not: the count
// answers "can the history speak for this day at all", which is exactly the
// question the null decision turns on.
func daysCovered(readings []repository.StoredReading, location *time.Location, days []localDay) int {
	covered := 0
	for _, day := range days {
		for _, reading := range readings {
			instant := reading.ObservedAt.In(location)
			if !instant.Before(day.Start) && instant.Before(day.End) {
				covered++
				break
			}
		}
	}
	return covered
}
