package classify

import (
	"math"
	"testing"
)

// TestDirectionalMetamorphicMonotonicity is the directional metamorphic test of
// academic-rule-v1. It is the in-repo evidence behind the MET-I2 and MET-V2
// entries of backend/docs/synthetic-scenarios.md (unit U3).
//
// WHY IT EXISTS. The rule is a documented, deterministic scorer, not a trained
// model: its own source states that rising(value, low, high) means "higher input
// means higher contribution", that falling is 1 - linear(...) with the opposite
// meaning, and every frozen default weight is positive. A directional
// metamorphic relation follows directly from those two facts. Holding the other
// four inputs fixed, raising temperature, wind speed, vegetation cover or
// historical fire count can never lower the weighted score, and raising relative
// humidity can never raise it. Because levelFor is non-decreasing in the score,
// the reported level must move in the same direction. This test pins that
// relation against the frozen default parameter set instead of trusting the
// prose, and it does so without reading any behaviour number that is not already
// part of the frozen params.
//
// WHAT IT PINS. For each of the five canonical variables it sweeps that variable
// over a strictly increasing sequence that begins below the documented low bound,
// passes through the range, and ends above the documented high bound, with the
// other four inputs held fixed at values chosen per case. At every step it
// asserts (a) the swept factor's own Contribution is non-decreasing for the four
// rising variables and non-increasing for humidity, and (b) the Assessment
// level's severity, measured by its index in Levels(), moves in the same
// direction. The strict monotonicity of the sweep itself is asserted first, so a
// malformed sequence cannot hollow out the relation.
//
// NON-VACUITY. A monotonicity assertion is trivially true when nothing ever
// changes, so the temperature case fixes the other four inputs specifically to
// cross a band boundary, and the test asserts that its level genuinely changes at
// least once. It also asserts a strict, opposite-signed asymmetry: the
// temperature contribution strictly increases from the first sweep point to the
// last, while the humidity contribution strictly decreases over its own sweep. An
// expectation inverted by a future regression therefore fails observably instead
// of passing by accident. Outside that pinned temperature change the level is
// never asserted to be strictly monotone, only non-decreasing or non-increasing.
//
// WHAT IT DOES NOT PROVE. This is structural monotonicity of one frozen
// deterministic rule version. It is not a property of a trained model, it is not
// a benchmark against observed fires, and it says nothing about the real-world
// directionality of wildfire risk or about the quality, completeness or
// calibration of the data the rule is fed. Passing here means the code still
// agrees with its own documented semantics; it is not evidence about fires, and
// it contributes only partial evidence toward MET-I2 and MET-V2 rather than
// closing those checklist items.
func TestDirectionalMetamorphicMonotonicity(t *testing.T) {
	midBase := func() Features {
		return Features{
			ObservedAt:          referenceInstant,
			EvaluatedAt:         referenceInstant,
			ZoneID:              "05101",
			TemperatureC:        Known(25),
			RelativeHumidityPct: Known(40),
			WindSpeedKmh:        Known(30),
			VegetationCoverPct:  Known(40),
			HistoricalFires:     Known(2.5),
		}
	}

	// temperatureBase fixes the other four inputs low enough that the temperature
	// sweep crosses the bajo/medio boundary: their weighted contribution is 0.18,
	// so the sweep covers 0.18 through 0.43 and changes level exactly once. The
	// temperature field here is only a placeholder; every sweep point overwrites
	// it.
	temperatureBase := func() Features {
		return Features{
			ObservedAt:          referenceInstant,
			EvaluatedAt:         referenceInstant,
			ZoneID:              "05101",
			TemperatureC:        Known(25),
			RelativeHumidityPct: Known(50),
			WindSpeedKmh:        Known(20),
			VegetationCoverPct:  Known(25),
			HistoricalFires:     Known(1),
		}
	}

	params := DefaultAcademicRuleV1Params()

	// sign is +1 for a variable the rule documents as rising (higher input means
	// higher contribution) and -1 for the falling one.
	type sweepCase struct {
		factor string
		low    float64
		high   float64
		sign   int
		set    func(*Features, float64)
		base   func() Features
	}

	sweeps := []sweepCase{
		{
			factor: "temperatura",
			low:    params.TemperatureLowC,
			high:   params.TemperatureHighC,
			sign:   +1,
			set:    func(f *Features, value float64) { f.TemperatureC = Known(value) },
			base:   temperatureBase,
		},
		{
			factor: "humedad_relativa",
			low:    params.HumidityDryPct,
			high:   params.HumidityWetPct,
			sign:   -1,
			set:    func(f *Features, value float64) { f.RelativeHumidityPct = Known(value) },
			base:   midBase,
		},
		{
			factor: "viento",
			low:    params.WindLowKmh,
			high:   params.WindHighKmh,
			sign:   +1,
			set:    func(f *Features, value float64) { f.WindSpeedKmh = Known(value) },
			base:   midBase,
		},
		{
			factor: "cobertura_vegetal",
			low:    params.VegetationSparsePct,
			high:   params.VegetationDensePct,
			sign:   +1,
			set:    func(f *Features, value float64) { f.VegetationCoverPct = Known(value) },
			base:   midBase,
		},
		{
			factor: "incendios_historicos",
			low:    params.HistoricalFiresLow,
			high:   params.HistoricalFiresHigh,
			sign:   +1,
			set:    func(f *Features, value float64) { f.HistoricalFires = Known(value) },
			base:   midBase,
		},
	}

	// observation captures what the final asymmetry and non-vacuity controls need
	// from each subtest. Subtests run sequentially, so a plain map is safe.
	type observation struct {
		firstContribution float64
		lastContribution  float64
		levelChanged      bool
	}
	observations := make(map[string]observation, len(sweeps))

	for _, sweep := range sweeps {
		t.Run(sweep.factor, func(t *testing.T) {
			points := directionalSweepPoints(sweep.low, sweep.high)
			for index := 1; index < len(points); index++ {
				if !(points[index] > points[index-1]) {
					t.Fatalf("sweep points are not strictly increasing at %d: %v is not above %v", index, points[index], points[index-1])
				}
			}

			var (
				seen                 bool
				previousContribution float64
				previousSeverity     int
				firstContribution    float64
				levelChanged         bool
			)

			for _, value := range points {
				features := sweep.base()
				sweep.set(&features, value)
				assessment := classifyWith(t, features)

				contribution := factorContribution(t, assessment.Factors, sweep.factor)
				severity := severityRank(t, assessment.Level)

				if !seen {
					firstContribution = contribution
					seen = true
				} else {
					if sweep.sign > 0 && contribution < previousContribution {
						t.Fatalf("rising variable %q: contribution fell from %v to %v at input %v, but the rule documents higher input as higher contribution", sweep.factor, previousContribution, contribution, value)
					}
					if sweep.sign < 0 && contribution > previousContribution {
						t.Fatalf("falling variable %q: contribution rose from %v to %v at input %v, but the rule documents higher input as lower contribution", sweep.factor, previousContribution, contribution, value)
					}
					if sweep.sign > 0 && severity < previousSeverity {
						t.Fatalf("rising variable %q: level severity fell from rank %d to rank %d at input %v, but the score is non-decreasing", sweep.factor, previousSeverity, severity, value)
					}
					if sweep.sign < 0 && severity > previousSeverity {
						t.Fatalf("falling variable %q: level severity rose from rank %d to rank %d at input %v, but the score is non-increasing", sweep.factor, previousSeverity, severity, value)
					}
					if severity != previousSeverity {
						levelChanged = true
					}
				}

				previousContribution = contribution
				previousSeverity = severity
			}

			observations[sweep.factor] = observation{
				firstContribution: firstContribution,
				lastContribution:  previousContribution,
				levelChanged:      levelChanged,
			}
		})
	}

	// Non-vacuity control: the monotonicity assertions above would also pass on a
	// constant function, so the temperature sweep must be observed to change the
	// level at least once. This is the only place the level's strictness is
	// asserted.
	temperature, ok := observations["temperatura"]
	if !ok {
		t.Fatal("the temperature sweep did not run, so the non-vacuity control has no observation")
	}
	if !temperature.levelChanged {
		t.Fatal("the temperature sweep never changed the level: the fixed inputs do not cross a band boundary, so the monotonicity assertions are vacuous")
	}

	// Asymmetry control: a regression that inverted rising and falling would keep
	// each individual monotonicity assertion internally consistent but flip the
	// two signs. Pinning both ends against each other makes that inversion
	// observably false.
	if !(temperature.lastContribution > temperature.firstContribution) {
		t.Fatalf("asymmetry control: temperature contribution went from %v to %v across the sweep, want a strict increase", temperature.firstContribution, temperature.lastContribution)
	}
	humidity, ok := observations["humedad_relativa"]
	if !ok {
		t.Fatal("the humidity sweep did not run, so the asymmetry control has no observation")
	}
	if !(humidity.lastContribution < humidity.firstContribution) {
		t.Fatalf("asymmetry control: humidity contribution went from %v to %v across the sweep, want a strict decrease", humidity.firstContribution, humidity.lastContribution)
	}
}

// directionalSweepPoints returns a strictly increasing sequence that starts one
// unit below low, passes through low, the quarter/half/three-quarter interior
// points, high, and ends one unit above high. The endpoints are what guarantee
// the below-low and above-high regions are exercised; the interior points are
// what guarantee a change is observable rather than only clamped.
func directionalSweepPoints(low, high float64) []float64 {
	return []float64{
		low - 1,
		low,
		math.Nextafter(low, math.Inf(1)),
		low + 0.25*(high-low),
		low + 0.5*(high-low),
		low + 0.75*(high-low),
		math.Nextafter(high, math.Inf(-1)),
		high,
		high + 1,
	}
}

// factorContribution returns the contribution the assessment reports for one
// named factor. A sweep always supplies its variable, so an absent factor means
// the assessment and the sweep disagree and the test stops.
func factorContribution(t *testing.T, factors []Factor, name string) float64 {
	t.Helper()
	for _, factor := range factors {
		if factor.Name != name {
			continue
		}
		if !factor.Known {
			t.Fatalf("factor %q is absent during a sweep that always supplies it", name)
		}
		return factor.Contribution
	}
	t.Fatalf("factor %q is missing from the assessment factors", name)
	return 0
}

// severityRank maps a level to its position in Levels(), which is ordered by
// ascending severity for the three decided levels. LevelDesconocido is listed
// last and is not a severity, so it is rejected here: every sweep supplies all
// five inputs at pinned, fresh instants and must never abstain.
func severityRank(t *testing.T, level Level) int {
	t.Helper()
	if level == LevelDesconocido {
		t.Fatalf("a sweep with all inputs present and fresh produced %q", LevelDesconocido)
	}
	for index, candidate := range Levels() {
		if candidate == level {
			return index
		}
	}
	t.Fatalf("level %q is not one of Levels()", level)
	return -1
}
