package classify

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"
)

// referenceInstant is the fixed instant every rule test uses. The rule must be
// independent of the system clock, so both instants are pinned here.
var referenceInstant = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func lowRiskFeatures() Features {
	return Features{
		ObservedAt:          referenceInstant,
		EvaluatedAt:         referenceInstant,
		ZoneID:              "valparaiso",
		TemperatureC:        Known(10),
		RelativeHumidityPct: Known(90),
		WindSpeedKmh:        Known(5),
		VegetationCoverPct:  Known(5),
		HistoricalFires:     Known(0),
	}
}

func midRiskFeatures() Features {
	return Features{
		ObservedAt:          referenceInstant,
		EvaluatedAt:         referenceInstant,
		ZoneID:              "valparaiso",
		TemperatureC:        Known(25),
		RelativeHumidityPct: Known(40),
		WindSpeedKmh:        Known(30),
		VegetationCoverPct:  Known(40),
		HistoricalFires:     Known(2.5),
	}
}

func highRiskFeatures() Features {
	return Features{
		ObservedAt:          referenceInstant,
		EvaluatedAt:         referenceInstant,
		ZoneID:              "valparaiso",
		TemperatureC:        Known(40),
		RelativeHumidityPct: Known(5),
		WindSpeedKmh:        Known(60),
		VegetationCoverPct:  Known(90),
		HistoricalFires:     Known(9),
	}
}

func classifyWith(t *testing.T, features Features) Assessment {
	t.Helper()
	assessment, err := NewAcademicRuleV1().Classify(t.Context(), features)
	if err != nil {
		t.Fatalf("Classify failed: %v", err)
	}
	if err := assessment.Validate(); err != nil {
		t.Fatalf("assessment is not publishable: %v", err)
	}
	return assessment
}

func TestWeightsSumToOne(t *testing.T) {
	params := DefaultAcademicRuleV1Params()
	total := params.WeightTemperature + params.WeightHumidity + params.WeightWind + params.WeightVegetation + params.WeightHistorical
	if math.Abs(total-1) > 1e-9 {
		t.Fatalf("weights sum to %v, want 1", total)
	}
}

// The bands must stay strictly ascending, otherwise a score could fall outside
// every band and be silently mapped to the default branch.
func TestBandsAreOrdered(t *testing.T) {
	params := DefaultAcademicRuleV1Params()
	if !(params.BandBajoMax > 0 && params.BandBajoMax < params.BandMedioMax && params.BandMedioMax < 1) {
		t.Fatalf("bands are not ordered: bajo<%v, medio<%v", params.BandBajoMax, params.BandMedioMax)
	}
}

func TestRuleLevelsForKnownInputs(t *testing.T) {
	cases := []struct {
		name     string
		features Features
		want     Level
	}{
		{"condiciones benignas", lowRiskFeatures(), LevelBajo},
		{"condiciones intermedias", midRiskFeatures(), LevelMedio},
		{"condiciones extremas", highRiskFeatures(), LevelAlto},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := classifyWith(t, testCase.features).Level; got != testCase.want {
				t.Fatalf("Level = %q, want %q", got, testCase.want)
			}
		})
	}
}

// The core safety property: an absent input must never be published as low risk,
// for any one of the five variables. The base features are otherwise benign, so
// silently treating a missing value as zero would produce LevelBajo.
func TestMissingInputNeverDegradesToLowRisk(t *testing.T) {
	cases := []struct {
		name    string
		omit    func(*Features)
		omitted string
	}{
		{"sin temperatura", func(f *Features) { f.TemperatureC = Unknown() }, "temperatura"},
		{"sin humedad", func(f *Features) { f.RelativeHumidityPct = Unknown() }, "humedad_relativa"},
		{"sin viento", func(f *Features) { f.WindSpeedKmh = Unknown() }, "viento"},
		{"sin cobertura vegetal", func(f *Features) { f.VegetationCoverPct = Unknown() }, "cobertura_vegetal"},
		{"sin incendios historicos", func(f *Features) { f.HistoricalFires = Unknown() }, "incendios_historicos"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			features := lowRiskFeatures()
			testCase.omit(&features)

			assessment := classifyWith(t, features)
			if assessment.Level == LevelBajo {
				t.Fatalf("a missing %s produced LevelBajo: absence was treated as a benign value", testCase.omitted)
			}
			if assessment.Level != LevelDesconocido {
				t.Fatalf("Level = %q, want %q when %s is absent", assessment.Level, LevelDesconocido, testCase.omitted)
			}
			if !strings.Contains(assessment.Rationale, testCase.omitted) {
				t.Fatalf("rationale does not name the missing variable %q: %s", testCase.omitted, assessment.Rationale)
			}
		})
	}
}

func TestStaleObservationIsNotClassified(t *testing.T) {
	features := highRiskFeatures()
	features.EvaluatedAt = features.ObservedAt.Add(AcademicRuleMaxStaleness + time.Minute)

	assessment := classifyWith(t, features)
	if assessment.Level != LevelDesconocido {
		t.Fatalf("a stale observation produced %q, want %q", assessment.Level, LevelDesconocido)
	}
	if !strings.Contains(assessment.Rationale, "antigüedad") {
		t.Fatalf("rationale does not explain staleness: %s", assessment.Rationale)
	}
}

func TestObservationAtTheStalenessLimitIsStillClassified(t *testing.T) {
	features := highRiskFeatures()
	features.EvaluatedAt = features.ObservedAt.Add(AcademicRuleMaxStaleness)

	if got := classifyWith(t, features).Level; got != LevelAlto {
		t.Fatalf("an observation exactly at the staleness limit produced %q, want %q", got, LevelAlto)
	}
}

func TestInconsistentInstantsAreNotClassified(t *testing.T) {
	features := highRiskFeatures()
	features.EvaluatedAt = features.ObservedAt.Add(-time.Minute)

	if got := classifyWith(t, features).Level; got != LevelDesconocido {
		t.Fatalf("an observation newer than its evaluation produced %q, want %q", got, LevelDesconocido)
	}
}

func TestMissingInstantsAreNotClassified(t *testing.T) {
	for name, mutate := range map[string]func(*Features){
		"sin instante de observacion": func(f *Features) { f.ObservedAt = time.Time{} },
		"sin instante de evaluacion":  func(f *Features) { f.EvaluatedAt = time.Time{} },
		"sin ningun instante":         func(f *Features) { f.ObservedAt, f.EvaluatedAt = time.Time{}, time.Time{} },
	} {
		t.Run(name, func(t *testing.T) {
			features := highRiskFeatures()
			mutate(&features)
			if got := classifyWith(t, features).Level; got != LevelDesconocido {
				t.Fatalf("Level = %q, want %q", got, LevelDesconocido)
			}
		})
	}
}

func TestNonFiniteValueIsNotClassified(t *testing.T) {
	for name, mutate := range map[string]func(*Features){
		"nan":               func(f *Features) { f.WindSpeedKmh = Known(math.NaN()) },
		"infinito positivo": func(f *Features) { f.TemperatureC = Known(math.Inf(1)) },
		"infinito negativo": func(f *Features) { f.TemperatureC = Known(math.Inf(-1)) },
	} {
		t.Run(name, func(t *testing.T) {
			features := highRiskFeatures()
			mutate(&features)
			assessment := classifyWith(t, features)
			if assessment.Level != LevelDesconocido {
				t.Fatalf("Level = %q, want %q", assessment.Level, LevelDesconocido)
			}
		})
	}
}

// Determinism, and proof the rule does not read the system clock: the instants
// below are in the past relative to the real clock, so a rule that consulted
// time.Now would report the observation as stale. Producing a level proves the
// only clock in play is the one carried in Features.
func TestRuleIsDeterministicAndIgnoresTheSystemClock(t *testing.T) {
	features := highRiskFeatures()
	first := classifyWith(t, features)
	second := classifyWith(t, features)

	if first.Level != LevelAlto {
		t.Fatalf("Level = %q, want %q: a past instant was treated as stale, so the system clock leaked into the rule", first.Level, LevelAlto)
	}
	if first.Level != second.Level || first.Rationale != second.Rationale {
		t.Fatalf("rule is not deterministic:\nfirst  = %+v\nsecond = %+v", first, second)
	}
	if len(first.Factors) != len(second.Factors) {
		t.Fatalf("factor count differs between runs: %d vs %d", len(first.Factors), len(second.Factors))
	}
	for index := range first.Factors {
		if first.Factors[index] != second.Factors[index] {
			t.Fatalf("factor %d differs between runs: %+v vs %+v", index, first.Factors[index], second.Factors[index])
		}
	}
}

// The bases allow a documented rule in place of a trained model, but only if it
// is declared as our own and not as an official index. The declaration travels
// with every assessment so a consumer cannot present it as authoritative.
func TestRationaleDeclaresTheRuleIsOurOwnAndNotOfficial(t *testing.T) {
	for _, assessment := range []Assessment{
		classifyWith(t, highRiskFeatures()),
		classifyWith(t, func() Features { f := lowRiskFeatures(); f.WindSpeedKmh = Unknown(); return f }()),
	} {
		for _, fragment := range []string{"regla propia", "no oficial", "no calibrada", "CONAF"} {
			if !strings.Contains(assessment.Rationale, fragment) {
				t.Errorf("rationale is missing the declaration %q: %s", fragment, assessment.Rationale)
			}
		}
	}
}

// The dashboard the contest requires shows indicators and factors, so every
// canonical variable must appear in the factors, including the absent ones.
func TestFactorsCoverEveryVariableIncludingAbsentOnes(t *testing.T) {
	features := highRiskFeatures()
	features.TemperatureC = Unknown()
	assessment := classifyWith(t, features)

	if len(assessment.Factors) != 5 {
		t.Fatalf("got %d factors, want 5", len(assessment.Factors))
	}
	seen := map[string]Factor{}
	for _, factor := range assessment.Factors {
		seen[factor.Name] = factor
	}
	for _, name := range []string{"temperatura", "humedad_relativa", "viento", "cobertura_vegetal", "incendios_historicos"} {
		if _, ok := seen[name]; !ok {
			t.Errorf("factor %q is missing", name)
		}
	}
	if absent := seen["temperatura"]; absent.Known {
		t.Error("an absent input is reported as a known factor")
	}
	if present := seen["viento"]; !present.Known || present.Contribution <= 0 {
		t.Errorf("a present input carried no contribution: %+v", present)
	}
}

func TestClassifyHonoursContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewAcademicRuleV1().Classify(ctx, highRiskFeatures()); err == nil {
		t.Fatal("a cancelled context was ignored")
	}
}

// The zero value must fail closed. AcademicRuleV1{} carries no parameter set, so
// classifying with it would produce a level from all-zero numbers - a behaviour
// no version ever promised. ID and Kind keep working so the failure is
// attributable.
func TestZeroValueRuleFailsClosed(t *testing.T) {
	zero := AcademicRuleV1{}
	if _, err := zero.Classify(t.Context(), highRiskFeatures()); err == nil {
		t.Fatal("the zero-value rule classified with an unset parameter set")
	}
	if got := zero.ID(); got != AcademicRuleV1ID {
		t.Fatalf("the zero-value rule reports id %q, want %q", got, AcademicRuleV1ID)
	}
	if got := zero.Kind(); got != KindRule {
		t.Fatalf("the zero-value rule reports kind %q, want %q", got, KindRule)
	}
	if _, err := zero.PolicyFingerprint(); err == nil {
		t.Fatal("the zero-value rule reported a policy fingerprint")
	}

	// The constructed rule is the control: the same features classify without
	// error, so the refusal above is the missing flag and not the features.
	if _, err := NewAcademicRuleV1().Classify(t.Context(), highRiskFeatures()); err != nil {
		t.Fatalf("a constructed rule failed to classify: %v", err)
	}
}

// NewAcademicRuleV1WithParams is the testing seam. It rejects only the parameter
// sets for which classification is undefined, and takes every other value as
// given, so unusual but well-defined rules remain exercisable.
func TestNewAcademicRuleV1WithParamsRejectsOnlyUndefinedValues(t *testing.T) {
	base := DefaultAcademicRuleV1Params()

	nonFinite := base
	nonFinite.TemperatureLowC = math.NaN()
	if _, err := NewAcademicRuleV1WithParams(nonFinite); err == nil {
		t.Fatal("a non-finite parameter was accepted")
	}

	infinite := base
	infinite.WeightWind = math.Inf(1)
	if _, err := NewAcademicRuleV1WithParams(infinite); err == nil {
		t.Fatal("an infinite parameter was accepted")
	}

	negativeStaleness := base
	negativeStaleness.MaxStalenessSeconds = -1
	if _, err := NewAcademicRuleV1WithParams(negativeStaleness); err == nil {
		t.Fatal("a negative staleness threshold was accepted")
	}

	unusual := base
	unusual.BandBajoMax = 0.9 // Above BandMedioMax: well-defined, if strange.
	if _, err := NewAcademicRuleV1WithParams(unusual); err != nil {
		t.Fatalf("a well-defined but unusual parameter set was rejected: %v", err)
	}
}

// TestBandBoundariesAreExclusiveAtTheBoundary is the in-repo proof for the
// literal-free mutation that motivated the frozen behaviour table: an adversarial
// review changed `case score < p.BandBajoMax` into `case score <= p.BandBajoMax`,
// introduced no literal anywhere, left the frozen parameter hash untouched, and
// silently moved the level at exactly 0.34 from medio to bajo. levelFor compares
// with a STRICTLY less-than operator, so the boundary score belongs to the UPPER
// band and the immediate neighbour below belongs to the lower one. If someone
// flips the operator again, this test and the frozen table both move.
func TestBandBoundariesAreExclusiveAtTheBoundary(t *testing.T) {
	params := DefaultAcademicRuleV1Params()

	cases := []struct {
		name  string
		score float64
		want  Level
	}{
		// With `<`, 0.34 is medio; with `<=` it would be bajo.
		{"bajo boundary exactly", params.BandBajoMax, LevelMedio},
		{"just below the bajo boundary", math.Nextafter(params.BandBajoMax, math.Inf(-1)), LevelBajo},
		// With `<`, 0.67 is alto; with `<=` it would be medio.
		{"medio boundary exactly", params.BandMedioMax, LevelAlto},
		{"just below the medio boundary", math.Nextafter(params.BandMedioMax, math.Inf(-1)), LevelMedio},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := params.levelFor(testCase.score); got != testCase.want {
				t.Fatalf("levelFor(%s) = %q, want %q: the band comparison is not exclusive at the boundary", behaviorFloat(testCase.score), got, testCase.want)
			}
		})
	}
}

// TestLinearAndFallingLowEdgeIsInclusive pins the VALUE of the interpolators at
// their thresholds: linear(low, low, high) is exactly 0, falling(low, low, high)
// is exactly 1, linear(high, low, high) is exactly 1, and the immediate neighbour
// above the low edge is neither 0 nor 1.
//
// It is NOT a mutation guard for the symmetric operator swap, and an independent
// hostile verification proved why: changing `value <= low` into `value < low`
// (or `value >= high` into `value > high`) is BEHAVIOUR-PRESERVING for distinct
// thresholds, because the fall-through computes (low-low)/(high-low), which is
// exactly +0.0, and (high-low)/(high-low), which is exactly 1.0. The only pair
// where it would differ is a degenerate low == high, which the frozen defaults
// never produce. What actually guards an operator change is the frozen behaviour
// table at the levelFor band boundaries, checked by
// TestBandBoundariesAreExclusiveAtTheBoundary, not this test.
func TestLinearAndFallingLowEdgeIsInclusive(t *testing.T) {
	const low, high = 15.0, 35.0

	if got := linear(low, low, high); got != 0 {
		t.Fatalf("linear(low, low, high) = %v, want exactly 0", got)
	}
	if got := linear(math.Nextafter(low, math.Inf(1)), low, high); got == 0 {
		t.Fatalf("linear(Nextafter(low, +Inf), low, high) = 0: the low edge is not the only inclusive point")
	}

	if got := falling(low, low, high); got != 1 {
		t.Fatalf("falling(low, low, high) = %v, want exactly 1", got)
	}
	if got := falling(math.Nextafter(low, math.Inf(1)), low, high); got == 1 {
		t.Fatalf("falling(Nextafter(low, +Inf), low, high) = 1: the low edge is not the only inclusive point")
	}

	// The high edge is symmetric: at the high threshold the rising contribution
	// is exactly 1 and the falling one exactly 0.
	if got := linear(high, low, high); got != 1 {
		t.Fatalf("linear(high, low, high) = %v, want exactly 1", got)
	}
	if got := falling(high, low, high); got != 0 {
		t.Fatalf("falling(high, low, high) = %v, want exactly 0", got)
	}
}
