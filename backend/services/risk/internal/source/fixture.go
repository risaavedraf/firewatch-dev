package source

import (
	"context"
	"time"

	observations "firewatch.local/contracts/observations/v1"
)

// SampleInstant is the fixed instant of the built-in sample. Holding it as a
// constant is what lets every fixture-based test be reproducible without a
// clock, including the classifier tests that must prove a missing input is never
// reported as low risk.
var SampleInstant = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// FixtureProvider serves a small synthetic sample.
//
// It exists so the whole contract can be exercised before any real data is
// downloaded: the provider registry, the traceability table, the precedence
// rule and the classifier all work against it. It is deliberately declared as a
// public official source of synthetic content so that the shape of a real
// descriptor is exercised, but its licence and attribution state plainly that it
// carries no real observations.
type FixtureProvider struct{}

// NewFixtureProvider returns the built-in sample provider.
func NewFixtureProvider() FixtureProvider { return FixtureProvider{} }

// Describe implements Provider.
func (FixtureProvider) Describe() observations.SourceDescriptor {
	return observations.SourceDescriptor{
		ID:          "fixture-sample",
		DisplayName: "Muestra sintética de demostración",
		Kind:        observations.SourceKindFile,
		Origin:      observations.OriginPublicOfficial,
		Endpoint:    "builtin://fixture-sample/sample-v1",
		License:     "N/A (datos sintéticos de demostración)",
		Attribution: "FireWatch (2026). Muestra sintética de demostración. No contiene observaciones reales.",
		Variables:   observations.VariableNames(),
		// The coverage fields describe the synthetic sample honestly rather than
		// borrowing the coverage of a real dataset it does not contain.
		GeoCoverage:      "Región de Valparaíso (muestra sintética)",
		TemporalCoverage: "instante fijo de muestra",
		Cadence:          "manual",
		Purpose:          "Ejercitar el contrato de datos y el clasificador sin datos reales",
		// A high priority keeps the sample from ever outranking a real source
		// inside the same origin.
		Priority: 100,
		// The built-in sample contains no real observations: every value is
		// generated for demonstration. Declaring that on the descriptor is what
		// lets the publication path derive the persisted demo flag from the
		// provider instead of hardcoding it, and it flips to false the moment a
		// real source is registered here.
		Synthetic: true,
	}
}

// Fetch implements Provider. When the window is zero-valued the whole sample is
// returned; otherwise only records inside the window are returned. A window that
// contains no sample record is a successful, complete, empty retrieval - not an
// error - so callers must treat "no records" and "fetch failed" differently.
func (FixtureProvider) Fetch(ctx context.Context, window observations.Window) (observations.ObservationSet, error) {
	if err := ctx.Err(); err != nil {
		return observations.ObservationSet{}, err
	}
	records := FixtureRecords()
	if !window.From.IsZero() || !window.To.IsZero() {
		filtered := make([]observations.Observation, 0, len(records))
		for _, record := range records {
			if !window.From.IsZero() && record.ObservedAt.Before(window.From) {
				continue
			}
			if !window.To.IsZero() && record.ObservedAt.After(window.To) {
				continue
			}
			filtered = append(filtered, record)
		}
		records = filtered
	}

	digest, err := RecordsHash(records)
	if err != nil {
		return observations.ObservationSet{}, err
	}
	provenance := ProvenanceFrom(FixtureProvider{}.Describe())
	provenance.QueryFingerprint = "builtin:sample-v1"
	provenance.RetrievedAt = SampleInstant
	provenance.CutoffAt = SampleInstant
	provenance.Format = "builtin"
	provenance.Records = len(records)
	provenance.NormalizedSHA256 = digest
	// No payload was transferred, so PayloadSHA256 stays empty and HTTPStatus is
	// not applicable; both are documented on the contract rather than faked.
	provenance.Complete = true

	return observations.ObservationSet{
		Window:     window,
		Records:    records,
		Provenance: provenance,
	}, nil
}

// FixtureRecords returns the synthetic sample, one observation per canonical
// variable. Values are fixed so a test can assert an exact classification.
func FixtureRecords() []observations.Observation {
	values := []struct {
		variable string
		value    float64
		unit     string
	}{
		{observations.VariableTemperature, 32.0, "C"},
		{observations.VariableHumidity, 18.0, "%"},
		{observations.VariableWind, 45.0, "km/h"},
		{observations.VariableVegetation, 65.0, "%"},
		{observations.VariableHistoricalFires, 4.0, "count"},
	}
	records := make([]observations.Observation, 0, len(values))
	for _, entry := range values {
		records = append(records, observations.Observation{
			SourceID:   "fixture-sample",
			Variable:   entry.variable,
			ZoneID:     "valparaiso",
			ObservedAt: SampleInstant,
			Value:      entry.value,
			Unit:       entry.unit,
			Quality:    observations.QualityOK,
		})
	}
	return records
}
