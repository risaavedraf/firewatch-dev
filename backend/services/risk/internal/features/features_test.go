package features

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	observations "firewatch.local/contracts/observations/v1"
	"firewatch.local/risk/internal/classify"
	"firewatch.local/risk/internal/repository"
	"firewatch.local/risk/internal/source"
)

// The tests in this file are the deliverable of the aggregates slice. They pin the
// three decisions that can be wrong in a way nothing downstream notices: where the
// day boundary falls, what an incomplete window yields, and what happens to a
// reading whose unit contradicts its variable.
//
// They run against an in-memory history instead of Postgres so they run everywhere;
// the SQL contract itself (idempotent append, half-open window query) is pinned
// separately, against a real database, in internal/repository.

// fakeHistory is an in-memory readings history.
//
// It mirrors the storage contract deliberately rather than loosely: identity is
// (zone, variable, instant) so re-ingesting the same reading is a no-op, the window
// is half-open so a reading at exactly the boundary instant belongs to exactly one
// window, and reads come back oldest first. A fake that was more permissive than
// Postgres would let these tests pass on behaviour the real store does not have.
type fakeHistory struct {
	readings  []repository.StoredReading
	appended  int
	appendErr error
	readErr   error
}

func (h *fakeHistory) AppendReadings(_ context.Context, records []observations.Observation) error {
	if h.appendErr != nil {
		return h.appendErr
	}
	h.appended += len(records)
	for _, record := range records {
		duplicate := false
		for _, existing := range h.readings {
			if existing.ZoneID == record.ZoneID && existing.Variable == record.Variable && existing.ObservedAt.Equal(record.ObservedAt) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		h.readings = append(h.readings, repository.StoredReading{
			ZoneID:     record.ZoneID,
			Variable:   record.Variable,
			ObservedAt: record.ObservedAt.UTC(),
			Value:      record.Value,
			Unit:       record.Unit,
		})
	}
	return nil
}

func (h *fakeHistory) ReadingsWindow(_ context.Context, zoneID, variable string, from, to time.Time) ([]repository.StoredReading, error) {
	if h.readErr != nil {
		return nil, h.readErr
	}
	window := make([]repository.StoredReading, 0, len(h.readings))
	for _, reading := range h.readings {
		if reading.ZoneID != zoneID || reading.Variable != variable {
			continue
		}
		if reading.ObservedAt.Before(from) || !reading.ObservedAt.Before(to) {
			continue
		}
		window = append(window, reading)
	}
	sort.Slice(window, func(i, j int) bool { return window[i].ObservedAt.Before(window[j].ObservedAt) })
	return window, nil
}

// zone is the zone every test publishes for.
const zone = "05101"

// newSource builds the source the way production does, so the tests exercise the
// same time zone load and cannot pass with a location the code never uses.
func newSource(t *testing.T, history History) *Source {
	t.Helper()
	built, err := New(history)
	if err != nil {
		t.Fatalf("build the feature source: %v", err)
	}
	return built
}

// ubicacion carga la zona civil de los agregados desde el código, no desde una
// constante del test, para que un cambio de Location rompa estas pruebas.
func ubicacion(t *testing.T) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(Location)
	if err != nil {
		t.Fatalf("load %s: %v", Location, err)
	}
	return location
}

// civil returns the instant of a civil date-time in the aggregate's zone. Every
// instant these tests use is written this way: writing UTC in a test about a local
// day boundary would hide the very mistake the tests exist to catch.
func civil(t *testing.T, year int, month time.Month, day, hour, minute int) time.Time {
	t.Helper()
	return time.Date(year, month, day, hour, minute, 0, 0, ubicacion(t))
}

// readingOf builds one reading of a variable in the unit that variable is stored
// in, so a test only has to state the value.
func readingOf(t *testing.T, variable string, at time.Time, value float64) observations.Observation {
	t.Helper()
	unit, declared := source.ExpectedUnit(variable)
	if !declared {
		t.Fatalf("variable %s has no expected unit", variable)
	}
	return observations.Observation{
		SourceID:   "test-source",
		Variable:   variable,
		ZoneID:     "provider-label",
		ObservedAt: at,
		Value:      value,
		Unit:       unit,
		Quality:    observations.QualityOK,
	}
}

// retrievalOf builds the retrieval a publication carries: the records and the
// provenance whose instant fixes the civil day.
func retrievalOf(records []observations.Observation, at time.Time) observations.ObservationSet {
	return observations.ObservationSet{
		Records: records,
		Provenance: observations.Provenance{
			SourceID:    "test-source",
			RetrievedAt: at,
			Complete:    true,
		},
	}
}

// TestDailyExtremesSeparateReadingsAcrossLocalMidnight is the boundary test.
//
// The two readings are four minutes apart, but they sit on either side of local
// midnight and therefore belong to different civil days. Both fall inside the same
// UTC day (2026-10-01), which is what makes this test able to fail: a boundary
// silently cut in UTC would put the 23:55 reading in the same day as the 00:05 one
// and the daily maximum would come out as 35 instead of 30.
func TestDailyExtremesSeparateReadingsAcrossLocalMidnight(t *testing.T) {
	lateYesterday := civil(t, 2026, time.September, 30, 23, 55)
	justAfterMidnight := civil(t, 2026, time.October, 1, 0, 5)
	// Guard the test's own premise: the two instants share a UTC day, so only a
	// boundary cut in the named civil zone can separate them.
	if lateYesterday.UTC().Format("2006-01-02") != justAfterMidnight.UTC().Format("2006-01-02") {
		t.Fatalf("the premise is broken: %s and %s do not share a UTC day", lateYesterday.UTC(), justAfterMidnight.UTC())
	}
	history := &fakeHistory{readings: []repository.StoredReading{
		{ZoneID: zone, Variable: observations.VariableTemperature, ObservedAt: lateYesterday.UTC(), Value: 35, Unit: "C"},
		{ZoneID: zone, Variable: observations.VariableTemperature, ObservedAt: justAfterMidnight.UTC(), Value: 30, Unit: "C"},
	}}
	builder := newSource(t, history)

	input, err := builder.Build(context.Background(), zone, retrievalOf(nil, justAfterMidnight))
	if err != nil {
		t.Fatalf("build the vector: %v", err)
	}
	if !input.TempMaxC.Coverage.Complete() {
		t.Fatalf("the current civil day is not reported as covered: %+v", input.TempMaxC.Coverage)
	}
	if !input.TempMaxC.Value.Present || input.TempMaxC.Value.Value != 30 {
		t.Fatalf("temp_max = %+v, want 30: the 23:55 reading belongs to the previous civil day", input.TempMaxC.Value)
	}
	if !input.TempMinC.Value.Present || input.TempMinC.Value.Value != 30 {
		t.Fatalf("temp_min = %+v, want 30 from the only reading of the current civil day", input.TempMinC.Value)
	}

	// Later the same civil day, the previous day's 35 must still be excluded.
	noon := civil(t, 2026, time.October, 1, 12, 0)
	history.readings = append(history.readings, repository.StoredReading{
		ZoneID: zone, Variable: observations.VariableTemperature, ObservedAt: noon.UTC(), Value: 22, Unit: "C",
	})
	input, err = builder.Build(context.Background(), zone, retrievalOf(nil, noon))
	if err != nil {
		t.Fatalf("build the vector at noon: %v", err)
	}
	if input.TempMaxC.Value.Value != 30 || input.TempMinC.Value.Value != 22 {
		t.Fatalf("extremes = %v/%v, want 30/22 for the current civil day", input.TempMaxC.Value.Value, input.TempMinC.Value.Value)
	}
}

// TestPrecipitationSumsSevenLocalDays pins the accumulation and its window: seven
// civil days, each contributing its own readings, summed.
func TestPrecipitationSumsSevenLocalDays(t *testing.T) {
	history := &fakeHistory{}
	for offset, value := range map[int]float64{6: 1, 5: 2, 4: 3, 3: 4, 2: 5, 1: 6, 0: 7} {
		instant := civil(t, 2026, time.October, 1-offset, 10, 0)
		history.readings = append(history.readings, repository.StoredReading{
			ZoneID: zone, Variable: source.VariablePrecipitation, ObservedAt: instant.UTC(), Value: value, Unit: "mm",
		})
	}
	builder := newSource(t, history)
	at := civil(t, 2026, time.October, 1, 12, 0)

	input, err := builder.Build(context.Background(), zone, retrievalOf(nil, at))
	if err != nil {
		t.Fatalf("build the vector: %v", err)
	}
	if got := input.PrecipitationMM.Coverage; got.Covered != 7 || got.Required != 7 {
		t.Fatalf("coverage = %+v, want 7 of 7 days", got)
	}
	if !input.PrecipitationMM.Value.Present {
		t.Fatalf("a fully covered window was declared absent: %+v", input.PrecipitationMM)
	}
	if got := input.PrecipitationMM.Value.Value; got != 28 {
		t.Fatalf("precip_sum = %v, want 28 mm", got)
	}
}

// TestPrecipitationRefusesAWindowCoveredBySixDays is the refusal half of the same
// rule: six days of a seven day window is a different quantity, so the feature is
// null and the reason names how much of the window existed.
func TestPrecipitationRefusesAWindowCoveredBySixDays(t *testing.T) {
	history := &fakeHistory{}
	for offset := 0; offset < 6; offset++ {
		instant := civil(t, 2026, time.October, 1-offset, 10, 0)
		history.readings = append(history.readings, repository.StoredReading{
			ZoneID: zone, Variable: source.VariablePrecipitation, ObservedAt: instant.UTC(), Value: 2, Unit: "mm",
		})
	}
	builder := newSource(t, history)
	at := civil(t, 2026, time.October, 1, 12, 0)

	input, err := builder.Build(context.Background(), zone, retrievalOf(nil, at))
	if err != nil {
		t.Fatalf("build the vector: %v", err)
	}
	measured := input.PrecipitationMM
	if measured.Value.Present {
		t.Fatalf("a window covered by six of seven days produced a value: %+v", measured.Value)
	}
	if measured.Coverage.Covered != 6 || measured.Coverage.Required != 7 {
		t.Fatalf("coverage = %+v, want 6 of 7 days", measured.Coverage)
	}
	if !strings.Contains(measured.Reason, "6 de 7 días locales") {
		t.Fatalf("the recorded reason %q does not state how much of the window was covered", measured.Reason)
	}
}

// TestMissingWindowIsNullAndNeverZero is the anti-fabrication test.
//
// An empty history must produce absence for every windowed feature, with the
// coverage count recorded, and must never produce a zero: a zero precipitation sum
// from missing data is a lie that changes the prediction, and zero is what a naive
// "sum whatever is there" implementation returns.
func TestMissingWindowIsNullAndNeverZero(t *testing.T) {
	at := civil(t, 2026, time.October, 1, 12, 0)
	set := retrievalOf([]observations.Observation{
		readingOf(t, observations.VariableHumidity, at, 22),
		readingOf(t, observations.VariableWind, at, 45),
		readingOf(t, source.VariableWindDirection, at, 195),
		readingOf(t, source.VariableRadiation, at, 1680),
	}, at)
	builder := newSource(t, &fakeHistory{})

	input, err := builder.Build(context.Background(), zone, set)
	if err != nil {
		t.Fatalf("build the vector: %v", err)
	}
	windowed := map[string]classify.Measured{
		"temp_max":   input.TempMaxC,
		"temp_min":   input.TempMinC,
		"precip_sum": input.PrecipitationMM,
	}
	for name, measured := range windowed {
		if measured.Value.Present {
			t.Fatalf("%s is present with %v on an empty history", name, measured.Value.Value)
		}
		if measured.Value.Value != 0 {
			t.Fatalf("%s carries a non-zero value while absent: %+v", name, measured.Value)
		}
		if measured.Reason == "" {
			t.Fatalf("%s is absent with no recorded reason", name)
		}
		if !strings.Contains(measured.Reason, "0 de ") {
			t.Fatalf("%s reason %q does not state that the window was empty", name, measured.Reason)
		}
	}
	if !strings.Contains(input.PrecipitationMM.Reason, "0 de 7 días locales") {
		t.Fatalf("precip_sum reason %q does not state the seven day coverage", input.PrecipitationMM.Reason)
	}
	if !strings.Contains(input.TempMaxC.Reason, "0 de 1 días locales") {
		t.Fatalf("temp_max reason %q does not state the current day coverage", input.TempMaxC.Reason)
	}
	// The instantaneous readings are the control: absence above is about the
	// window, not about the build giving up on everything.
	for name, measured := range map[string]classify.Measured{
		"humidity_rel":  input.HumidityRelPct,
		"wind_speed":    input.WindSpeedKmh,
		"wind_dir":      input.WindDirectionDeg,
		"radiation_sol": input.RadiationWPerM2,
	} {
		if !measured.Value.Present {
			t.Fatalf("%s was declared absent although the retrieval carried it: %+v", name, measured)
		}
	}
}

// TestATrueZeroSurvivesAndIsNotConfusedWithAbsence pins the other side of the same
// rule: a measured zero over a complete window is a real reading and stays present.
// Without this, "never fabricate a zero" could be implemented as "never report a
// zero", which would throw away dry days.
func TestATrueZeroSurvivesAndIsNotConfusedWithAbsence(t *testing.T) {
	history := &fakeHistory{}
	for offset := 0; offset < 7; offset++ {
		instant := civil(t, 2026, time.October, 1-offset, 10, 0)
		history.readings = append(history.readings, repository.StoredReading{
			ZoneID: zone, Variable: source.VariablePrecipitation, ObservedAt: instant.UTC(), Value: 0, Unit: "mm",
		})
	}
	builder := newSource(t, history)
	input, err := builder.Build(context.Background(), zone, retrievalOf(nil, civil(t, 2026, time.October, 1, 12, 0)))
	if err != nil {
		t.Fatalf("build the vector: %v", err)
	}
	if !input.PrecipitationMM.Value.Present || input.PrecipitationMM.Value.Value != 0 {
		t.Fatalf("a measured zero over a complete window = %+v, want a present 0", input.PrecipitationMM.Value)
	}
}

// TestReingestingTheSameRetrievalChangesNothing pins idempotency at the level the
// aggregates care about: the same publication recorded twice must not duplicate a
// reading and must not move a sum or an extreme.
func TestReingestingTheSameRetrievalChangesNothing(t *testing.T) {
	at := civil(t, 2026, time.October, 1, 12, 0)
	history := &fakeHistory{}
	// Six previous civil days of rain, so the retrieval below completes the seven
	// day window and precip_sum is a real value rather than the null an incomplete
	// window produces.
	for offset := 1; offset < 7; offset++ {
		instant := civil(t, 2026, time.October, 1-offset, 10, 0)
		history.readings = append(history.readings, repository.StoredReading{
			ZoneID: zone, Variable: source.VariablePrecipitation, ObservedAt: instant.UTC(), Value: 2, Unit: "mm",
		})
	}
	builder := newSource(t, history)
	set := retrievalOf([]observations.Observation{
		readingOf(t, observations.VariableTemperature, at, 31),
		readingOf(t, source.VariablePrecipitation, at, 4),
		readingOf(t, observations.VariableHumidity, at, 22),
	}, at)

	if err := builder.Record(context.Background(), zone, set); err != nil {
		t.Fatalf("record the retrieval: %v", err)
	}
	first, err := builder.Build(context.Background(), zone, set)
	if err != nil {
		t.Fatalf("build the vector: %v", err)
	}
	afterFirst := len(history.readings)

	if err := builder.Record(context.Background(), zone, set); err != nil {
		t.Fatalf("re-record the retrieval: %v", err)
	}
	if len(history.readings) != afterFirst {
		t.Fatalf("re-ingesting duplicated readings: %d rows, want %d", len(history.readings), afterFirst)
	}
	second, err := builder.Build(context.Background(), zone, set)
	if err != nil {
		t.Fatalf("build the vector again: %v", err)
	}
	if !sameValues(first, second) {
		t.Fatalf("re-ingesting changed the vector:\n first=%+v\nsecond=%+v", first, second)
	}
	if got := countReadings(history, source.VariablePrecipitation); got != 7 {
		t.Fatalf("precipitation rows = %d, want 7: a duplicated reading would inflate the sum", got)
	}
	if !second.PrecipitationMM.Value.Present || second.PrecipitationMM.Value.Value != 16 {
		t.Fatalf("precip_sum = %+v after re-ingesting, want a present 16 mm (6 days of 2 mm plus 4 mm)", second.PrecipitationMM.Value)
	}
}

// TestRecordRejectsAReadingInAUnitThatContradictsItsVariable protects the input
// gate: a temperature in Fahrenheit must never be stored as if it were Celsius.
func TestRecordRejectsAReadingInAUnitThatContradictsItsVariable(t *testing.T) {
	at := civil(t, 2026, time.October, 1, 12, 0)
	history := &fakeHistory{}
	builder := newSource(t, history)
	fahrenheit := readingOf(t, observations.VariableTemperature, at, 87.8)
	fahrenheit.Unit = "°F"

	err := builder.Record(context.Background(), zone, retrievalOf([]observations.Observation{fahrenheit}, at))
	if err == nil {
		t.Fatal("a Fahrenheit temperature was recorded in a Celsius history")
	}
	if !strings.Contains(err.Error(), "°F") || !strings.Contains(err.Error(), "C") {
		t.Fatalf("the refusal %q does not name both units", err)
	}
	if len(history.readings) != 0 {
		t.Fatalf("%d readings reached the history despite the refusal", len(history.readings))
	}
}

// TestABuildRefusesAStoredReadingInAnotherUnit protects the output gate: the
// history outlives the code that wrote it, so a row stored in another unit must
// stop the aggregate instead of being summed into it.
func TestABuildRefusesAStoredReadingInAnotherUnit(t *testing.T) {
	at := civil(t, 2026, time.October, 1, 12, 0)
	history := &fakeHistory{readings: []repository.StoredReading{
		{ZoneID: zone, Variable: observations.VariableTemperature, ObservedAt: at.UTC(), Value: 87.8, Unit: "°F"},
	}}
	builder := newSource(t, history)

	if _, err := builder.Build(context.Background(), zone, retrievalOf(nil, at)); err == nil {
		t.Fatal("a stored Fahrenheit reading entered a Celsius aggregate")
	}

	// The same rule guards the instantaneous half, which reads the retrieval rather
	// than the history.
	broken := readingOf(t, observations.VariableHumidity, at, 40)
	broken.Unit = "%RH"
	if _, err := builder.Build(context.Background(), zone, retrievalOf([]observations.Observation{broken}, at)); err == nil {
		t.Fatal("a retrieval record in an unexpected unit entered the model input")
	}
}

// TestBuildRequiresTheRetrievalInstant protects reproducibility: the civil day is
// derived from that instant, so a vector built without it would silently use
// whatever day the zero time falls on.
func TestBuildRequiresTheRetrievalInstant(t *testing.T) {
	builder := newSource(t, &fakeHistory{})
	if _, err := builder.Build(context.Background(), zone, retrievalOf(nil, time.Time{})); err == nil {
		t.Fatal("a vector was built without the retrieval instant that fixes its civil day")
	}
}

// TestInstantaneousReadingsTakeTheNewestObservation pins the precedence inside one
// retrieval, and that a variable the retrieval omits is absent with a reason.
func TestInstantaneousReadingsTakeTheNewestObservation(t *testing.T) {
	at := civil(t, 2026, time.October, 1, 12, 0)
	older := at.Add(-15 * time.Minute)
	set := retrievalOf([]observations.Observation{
		readingOf(t, observations.VariableHumidity, older, 10),
		readingOf(t, observations.VariableHumidity, at, 22),
	}, at)
	builder := newSource(t, &fakeHistory{})

	input, err := builder.Build(context.Background(), zone, set)
	if err != nil {
		t.Fatalf("build the vector: %v", err)
	}
	if got := input.HumidityRelPct.Value.Value; got != 22 {
		t.Fatalf("humidity = %v, want the newest reading 22", got)
	}
	if input.WindSpeedKmh.Value.Present {
		t.Fatalf("wind_speed is present although the retrieval carried no wind reading: %+v", input.WindSpeedKmh)
	}
	if input.WindSpeedKmh.Reason == "" {
		t.Fatal("an omitted variable was declared absent with no recorded reason")
	}
}

// TestNewRefusesAMissingHistory protects the constructor's two failure modes.
func TestNewRefusesAMissingHistory(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("a source was built with no readings history")
	}
}

// TestRecordStoresThePublicationZone pins the attribution decision: the history is
// keyed by the zone the publication is for, so a documented label divergence cannot
// split one commune's history in two.
func TestRecordStoresThePublicationZone(t *testing.T) {
	at := civil(t, 2026, time.October, 1, 12, 0)
	history := &fakeHistory{}
	builder := newSource(t, history)

	record := readingOf(t, observations.VariableHumidity, at, 22)
	record.ZoneID = "provider-label"
	if err := builder.Record(context.Background(), zone, retrievalOf([]observations.Observation{record}, at)); err != nil {
		t.Fatalf("record the retrieval: %v", err)
	}
	if len(history.readings) != 1 || history.readings[0].ZoneID != zone {
		t.Fatalf("stored readings = %+v, want the publication zone %q", history.readings, zone)
	}
}

// TestReadingsWindowIsHalfOpenAtTheDayBoundary pins the interval rule the aggregates
// rely on: a reading at exactly local midnight belongs to the day that starts there.
func TestReadingsWindowIsHalfOpenAtTheDayBoundary(t *testing.T) {
	midnight := civil(t, 2026, time.October, 1, 0, 0)
	day := dayAt(midnight, ubicacion(t))
	history := &fakeHistory{readings: []repository.StoredReading{
		{ZoneID: zone, Variable: observations.VariableTemperature, ObservedAt: midnight.UTC(), Value: 20, Unit: "C"},
		{ZoneID: zone, Variable: observations.VariableTemperature, ObservedAt: day.End.UTC(), Value: 30, Unit: "C"},
	}}
	builder := newSource(t, history)
	readings, err := builder.window(context.Background(), zone, observations.VariableTemperature, day.Start, day.End)
	if err != nil {
		t.Fatalf("read the window: %v", err)
	}
	if len(readings) != 1 || readings[0].Value != 20 {
		t.Fatalf("window = %+v, want only the reading at the start instant", readings)
	}
}

// TestConfiguredZoneIsTheChileanOne pins the named zone itself: the aggregates must
// be cut in America/Santiago, not in UTC and not in the host's zone.
func TestConfiguredZoneIsTheChileanOne(t *testing.T) {
	if Location != "America/Santiago" {
		t.Fatalf("Location = %q, want America/Santiago", Location)
	}
	// A January instant in Santiago is UTC-3 and a July instant is UTC-4, so a
	// fixed offset could not satisfy both.
	january := time.Date(2026, time.January, 15, 0, 0, 0, 0, ubicacion(t)).UTC()
	july := time.Date(2026, time.July, 15, 0, 0, 0, 0, ubicacion(t)).UTC()
	if _, offset := january.In(ubicacion(t)).Zone(); offset != -3*3600 {
		t.Fatalf("January offset = %d, want -3h: the zone must be %s", offset, Location)
	}
	if _, offset := july.In(ubicacion(t)).Zone(); offset != -4*3600 {
		t.Fatalf("July offset = %d, want -4h: the zone must be %s", offset, Location)
	}
}

// sameValues compares the seven values of two vectors without comparing the prose
// reasons, which carry the reading count and would change with the row count.
func sameValues(a, b classify.ModelInput) bool {
	values := func(input classify.ModelInput) []classify.Feature {
		return []classify.Feature{
			input.TempMaxC.Value, input.TempMinC.Value, input.HumidityRelPct.Value,
			input.WindSpeedKmh.Value, input.WindDirectionDeg.Value,
			input.PrecipitationMM.Value, input.RadiationWPerM2.Value,
		}
	}
	first, second := values(a), values(b)
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}

// countReadings counts the stored rows of one variable.
func countReadings(history *fakeHistory, variable string) int {
	count := 0
	for _, reading := range history.readings {
		if reading.Variable == variable {
			count++
		}
	}
	return count
}
