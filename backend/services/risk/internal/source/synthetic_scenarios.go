package source

import (
	"context"
	"fmt"
	"strings"
	"time"

	observations "firewatch.local/contracts/observations/v1"
)

// SyntheticScenario describes a demonstration, not a scientific risk estimate.
// Profiles specify related environmental inputs rather than arbitrary random
// values. Classification is left to the configured classifier, never this label.
type SyntheticScenario struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

// SyntheticScenarios returns a stable catalog copy. Missing observations and an
// unfinished retrieval are separate cases: only the latter declares incomplete
// provenance and must be refused before an assessment is stored.
func SyntheticScenarios() []SyntheticScenario {
	return []SyntheticScenario{
		{"mild", "Mild temperature, humid air and light wind; synthetic demonstration only."},
		{"mixed", "Moderate temperature, humidity and wind; synthetic demonstration only."},
		{"dry-windy", "Hot, dry and windy conditions; synthetic demonstration only."},
		{"missing-humidity", "Humidity absent; do not interpret missing input as low danger."},
		{"stale", "Observations seven hours older than the evaluation instant."},
		{"partial-retrieval", "An unfinished retrieval that must not be published as a completed assessment."},
	}
}

// SyntheticProvider generates one coherent profile for one demonstration zone.
// Its immutable configuration contains an explicit evaluation instant: neither
// the system clock nor random state can change a replay. Publisher selects it
// for scenario-bearing requests; legacy requests keep the configured provider.
type SyntheticProvider struct {
	scenarioID  string
	zone        string
	evaluatedAt time.Time
}

// NewSyntheticProvider rejects unknown profiles and absent scope/time before
// producing data. Zone identifiers are demo labels, not spatial measurements;
// transport must still apply its own supported-zone allowlist.
func NewSyntheticProvider(scenarioID, zone string, evaluatedAt time.Time) (SyntheticProvider, error) {
	known := false
	for _, scenario := range SyntheticScenarios() {
		if scenario.ID == scenarioID {
			known = true
			break
		}
	}
	if !known {
		return SyntheticProvider{}, fmt.Errorf("unknown synthetic scenario %q", scenarioID)
	}
	if strings.TrimSpace(zone) == "" {
		return SyntheticProvider{}, fmt.Errorf("synthetic scenario requires a zone")
	}
	if evaluatedAt.IsZero() {
		return SyntheticProvider{}, fmt.Errorf("synthetic scenario requires an evaluation instant")
	}
	return SyntheticProvider{scenarioID: scenarioID, zone: zone, evaluatedAt: evaluatedAt.UTC()}, nil
}

// Describe marks this as private demonstration content, never an official
// observational feed. Synthetic=true remains the authority for the demo flag.
func (p SyntheticProvider) Describe() observations.SourceDescriptor {
	return observations.SourceDescriptor{
		ID:               "synthetic-" + p.scenarioID,
		DisplayName:      "Synthetic scenario: " + p.scenarioID,
		Kind:             observations.SourceKindFile,
		Origin:           observations.OriginClientPrivate,
		Endpoint:         "builtin://synthetic-scenarios/v1/" + p.scenarioID,
		License:          "N/A (synthetic demonstration data)",
		Attribution:      "FireWatch synthetic demonstration; no real observations or official source.",
		Variables:        observations.VariableNames(),
		GeoCoverage:      "Synthetic inputs labeled for demo zone " + p.zone + "; no measured spatial coverage",
		TemporalCoverage: "Explicit evaluation instant; stale profile uses a seven-hour lag",
		Cadence:          "manual",
		Purpose:          "Reproducible demonstration and software regression testing, not scientific validation",
		Priority:         100,
		Synthetic:        true,
	}
}

// Fetch emits new records on every call, honoring the same closed-window
// semantics as FixtureProvider. Missing humidity is a complete acquisition of
// an incomplete observation; partial-retrieval represents acquisition failure.
func (p SyntheticProvider) Fetch(ctx context.Context, window observations.Window) (observations.ObservationSet, error) {
	if err := ctx.Err(); err != nil {
		return observations.ObservationSet{}, err
	}
	if _, err := NewSyntheticProvider(p.scenarioID, p.zone, p.evaluatedAt); err != nil {
		return observations.ObservationSet{}, err
	}
	// Values are deliberately explicit and related, not calibrated or fitted.
	// Order: temperature C, humidity %, wind km/h, vegetation %, historical count.
	values := [5]float64{32, 18, 45, 65, 4}
	switch p.scenarioID {
	case "mild":
		values = [5]float64{16, 75, 5, 20, 0}
	case "mixed":
		values = [5]float64{25, 40, 30, 40, 2}
	}
	observedAt := p.evaluatedAt
	if p.scenarioID == "stale" {
		observedAt = observedAt.Add(-7 * time.Hour)
	}
	descriptor := p.Describe()
	variables := observations.VariableNames()
	units := [...]string{"C", "%", "km/h", "%", "count"}
	records := make([]observations.Observation, 0, len(variables))
	for index, variable := range variables {
		if p.scenarioID == "missing-humidity" && variable == observations.VariableHumidity {
			continue
		}
		if !window.From.IsZero() && observedAt.Before(window.From) {
			continue
		}
		if !window.To.IsZero() && observedAt.After(window.To) {
			continue
		}
		records = append(records, observations.Observation{
			SourceID: descriptor.ID, Variable: variable, ZoneID: p.zone, ObservedAt: observedAt,
			Value: values[index], Unit: units[index], Quality: observations.QualityEstimated,
		})
	}
	digest, err := RecordsHash(records)
	if err != nil {
		return observations.ObservationSet{}, err
	}
	provenance := ProvenanceFrom(descriptor)
	provenance.QueryFingerprint = descriptor.Endpoint + "?zone=" + p.zone + "&evaluated_at=" + p.evaluatedAt.Format(time.RFC3339Nano)
	provenance.RetrievedAt = p.evaluatedAt
	provenance.CutoffAt = observedAt
	provenance.Format = "builtin"
	provenance.Records = len(records)
	provenance.NormalizedSHA256 = digest
	provenance.Complete = p.scenarioID != "partial-retrieval"
	return observations.ObservationSet{Window: window, Records: records, Provenance: provenance}, nil
}
