package classify

import (
	"testing"
)

// TestExtremeInputsSaturateAtDocumentedBounds pins what academic-rule-v1 does
// with finite inputs that sit outside the documented parameter range. It is the
// in-repo evidence behind the extreme-value part of the ROB-I1 bullet in
// backend/docs/synthetic-scenarios.md.
//
// WHY IT EXISTS. ROB-I1 names "valores extremos" (extreme values) as one of the
// simulated adversarial inputs, but nothing in the repository pinned the rule's
// behaviour for an out-of-range finite value. The adjacent non-finite case is
// already covered by TestNonFiniteValueIsNotClassified: NaN, +Inf and -Inf make
// the rule abstain with LevelDesconocido. A very large but finite value is a
// different input and takes a different path: it is clamped by linear, not
// rejected.
//
// WHAT IT PINS. For each of the five canonical variables the test classifies
// once with the variable exactly at its documented bound and once far outside
// that bound on the same side, holding the other four inputs pinned at fresh,
// known values. The two classifications must agree exactly: the swept factor's
// Contribution must be bit-identical and the resulting RiskLevel must be the
// same. Both the low side (at TemperatureLowC and far below it) and the high
// side (at TemperatureHighC and far above it) are exercised for every variable,
// including relative humidity, whose contribution inverts but is subject to the
// same saturation. On every classification the test calls severityRank, which
// fails on LevelDesconocido: saturating an extreme finite value must not make the
// rule abstain.
//
// EXACT EQUALITY IS DELIBERATE. At or beyond a bound, linear returns exactly 0
// (for value <= low) or exactly 1 (for value >= high), never an approximation, so
// weight*rising(...) and weight*falling(...) are the same 64-bit float at the
// bound and beyond it rather than merely close. Comparing with == therefore
// asserts the strong property that saturation is exact, not a tolerance.
//
// NON-VACUITY. Equality at the bounds would be trivially true for a constant
// function, so the test also classifies at the midpoint of each variable's range
// and asserts that its contribution is strictly between the two bound
// contributions. An interior value that stopped moving the score would fail this
// control even if every saturation assertion still passed.
//
// WHAT IT DOES NOT PROVE. This is code behaviour on invented inputs. It pins
// saturation at the bounds that the frozen default parameter set already
// declares; it says nothing about the production valid ranges, which are not
// agreed, nothing about the tolerance threshold with which out-of-range data
// should be accepted or rejected (ROB-I2), and nothing about any measured
// degradation of real classifications (ROB-V1, ROB-V2). It is not robustness
// evidence about real data quality, and it contributes partial evidence to
// ROB-I1 only.
func TestExtremeInputsSaturateAtDocumentedBounds(t *testing.T) {
	params := DefaultAcademicRuleV1Params()

	pinnedBase := func() Features {
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

	type extremeCase struct {
		factor string
		low    float64
		high   float64
		set    func(*Features, float64)
	}

	cases := []extremeCase{
		{
			factor: "temperatura",
			low:    params.TemperatureLowC,
			high:   params.TemperatureHighC,
			set:    func(f *Features, value float64) { f.TemperatureC = Known(value) },
		},
		{
			factor: "humedad_relativa",
			low:    params.HumidityDryPct,
			high:   params.HumidityWetPct,
			set:    func(f *Features, value float64) { f.RelativeHumidityPct = Known(value) },
		},
		{
			factor: "viento",
			low:    params.WindLowKmh,
			high:   params.WindHighKmh,
			set:    func(f *Features, value float64) { f.WindSpeedKmh = Known(value) },
		},
		{
			factor: "cobertura_vegetal",
			low:    params.VegetationSparsePct,
			high:   params.VegetationDensePct,
			set:    func(f *Features, value float64) { f.VegetationCoverPct = Known(value) },
		},
		{
			factor: "incendios_historicos",
			low:    params.HistoricalFiresLow,
			high:   params.HistoricalFiresHigh,
			set:    func(f *Features, value float64) { f.HistoricalFires = Known(value) },
		},
	}

	// beyond is deliberately far outside any plausible production domain. The
	// test is about saturation, so the exact magnitude does not matter as long as
	// the value stays finite.
	const beyond = 1000.0

	type boundsObservation struct {
		low  float64
		high float64
		mid  float64
	}
	observations := make(map[string]boundsObservation, len(cases))

	for _, testCase := range cases {
		t.Run(testCase.factor, func(t *testing.T) {
			classifyAt := func(value float64) Assessment {
				t.Helper()
				features := pinnedBase()
				testCase.set(&features, value)
				return classifyWith(t, features)
			}

			// The midpoint is strictly inside the range for every variable, so it
			// is the non-vacuity control against a constant contribution.
			midContribution := factorContribution(t, classifyAt((testCase.low+testCase.high)/2).Factors, testCase.factor)
			observation := boundsObservation{mid: midContribution}

			sides := []struct {
				name   string
				bound  float64
				beyond float64
			}{
				{name: "low", bound: testCase.low, beyond: testCase.low - beyond},
				{name: "high", bound: testCase.high, beyond: testCase.high + beyond},
			}

			for _, side := range sides {
				atBound := classifyAt(side.bound)
				outOfRange := classifyAt(side.beyond)

				atBoundContribution := factorContribution(t, atBound.Factors, testCase.factor)
				outOfRangeContribution := factorContribution(t, outOfRange.Factors, testCase.factor)

				// Exact equality, not an approximation: at and beyond a bound
				// linear returns exactly 0 or exactly 1, so the weighted product
				// is bit-identical rather than merely close.
				if atBoundContribution != outOfRangeContribution {
					t.Fatalf("%s %s side: contribution is %v at the bound %v but %v at %v, want exact saturation",
						testCase.factor, side.name, atBoundContribution, side.bound, outOfRangeContribution, side.beyond)
				}

				if atBound.Level != outOfRange.Level {
					t.Fatalf("%s %s side: level is %q at the bound %v but %q at %v, want identical",
						testCase.factor, side.name, atBound.Level, side.bound, outOfRange.Level, side.beyond)
				}

				// severityRank fails on LevelDesconocido, so calling it is the
				// abstention check. No redundant level comparison is added.
				severityRank(t, atBound.Level)
				severityRank(t, outOfRange.Level)

				if side.name == "low" {
					observation.low = atBoundContribution
				} else {
					observation.high = atBoundContribution
				}
			}

			observations[testCase.factor] = observation
		})
	}

	// Non-vacuity control: saturation equality would also hold for a constant
	// function, so the midpoint contribution must differ from both bounds. The
	// check runs for every variable, which is stronger than the one-variable
	// minimum the contract requires.
	for _, testCase := range cases {
		observation, ok := observations[testCase.factor]
		if !ok {
			t.Fatalf("the %s subtest recorded no observation, so the non-vacuity control cannot run", testCase.factor)
		}
		lower, upper := observation.low, observation.high
		if lower > upper {
			lower, upper = upper, lower
		}
		if !(observation.mid > lower && observation.mid < upper) {
			t.Fatalf("non-vacuity control: %s midpoint contribution %v is not strictly between the bound contributions %v and %v",
				testCase.factor, observation.mid, observation.low, observation.high)
		}
	}
}
