package scorer

import (
	"errors"
	"math"
	"testing"

	"firewatch.local/inference/internal/artifact"
)

func load(t *testing.T, name string) *artifact.Artifact {
	t.Helper()
	loaded, err := artifact.Load("../../testdata/" + name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return loaded
}

// sample builds a Sample from name/value pairs in the given order. Building it
// by hand (rather than through JSON) is what lets the order tests below control
// the declaration order exactly. Values may be any Go numeric type so a test can
// write 9 instead of 9.0; NaN is a float64 and means "declared but unobserved".
func sample(t *testing.T, pairs ...any) Sample {
	built := make(Sample, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		switch value := pairs[i+1].(type) {
		case float64:
			built = append(built, FeatureValue{Name: pairs[i].(string), Value: value})
		case float32:
			built = append(built, FeatureValue{Name: pairs[i].(string), Value: float64(value)})
		case int:
			built = append(built, FeatureValue{Name: pairs[i].(string), Value: float64(value)})
		default:
			t.Fatalf("sample value %v is not numeric", pairs[i+1])
		}
	}
	return built
}

// logisticRow is one realistic Valparaíso sample used by several tests.
func logisticRow(t *testing.T) Sample {
	return sample(t,
		"temp_max", 34.2,
		"temp_min", 18.4,
		"humidity_rel", 22.0,
		"wind_speed", 9.5,
		"wind_dir", 195.0,
		"precip_sum", 0.0,
		"radiation_sol", 1680.0,
	)
}

// TestLogisticScoreMatchesHandComputedValue pins the formula against arithmetic
// written out here from the artifact's declared numbers. Nothing in this test
// calls the preprocessing or the model, so a changed order, a transposed
// coefficient or a swapped sign all fail it.
func TestLogisticScoreMatchesHandComputedValue(t *testing.T) {
	model := load(t, "logistic.json")
	scorer := New(model)

	// z = intercept + sum(coef[i] * (x[i] - center[i]) / scale[i]), with every
	// number transcribed from testdata/logistic.json.
	z := -2.25 +
		0.62*((34.2-18.5)/6.5) +
		0.18*((18.4-8.0)/4.5) +
		-0.44*((22.0-70.0)/12.0) +
		0.51*((9.5-4.0)/2.0) +
		0.07*((195.0-200.0)/60.0) +
		0.35*((0.0-1.0)/3.0) +
		0.29*((1680.0-380.0)/180.0)
	want := 1 / (1 + math.Exp(-z))

	result, err := scorer.Score(logisticRow(t))
	if err != nil {
		t.Fatalf("score: %v", err)
	}
	if math.Abs(result.Probability-want) > 1e-12 {
		t.Fatalf("probability %v, hand-computed %v (z=%v)", result.Probability, want, z)
	}
	if result.Version != "valparaiso-logistic-v1" {
		t.Errorf("version %q", result.Version)
	}
	// The artifact declares its own thresholds (0.35/0.65) and this sample is a
	// hot, dry, windy day, so it must land high. The level is read from the
	// artifact, which is the single owner of the cut points.
	if level := model.Level(result.Probability); level != artifact.LevelHigh {
		t.Errorf("level %q want high", level)
	}
}

// TestForestScoreWalksToCorrectLeaf uses the two-tree fixture, whose leaves are
// readable by hand:
//
//	tree 1: temp_max <= 30 -> 0.10, otherwise wind_speed <= 8 -> 0.55, else 0.80
//	tree 2: temp_min <= 10 -> 0.20, otherwise 0.60
//
// The fixture declares identity preprocessing (center 0, scale 1) so thresholds
// are in the features' own units and the expected leaves are checkable without
// arithmetic.
func TestForestScoreWalksToCorrectLeaf(t *testing.T) {
	model := load(t, "forest.json")
	scorer := New(model)
	for _, tc := range []struct {
		name   string
		sample Sample
		leaves [2]float64
		want   float64
		level  artifact.Level
	}{
		{
			name:   "hot windy day",
			sample: forestRow(t, 35.0, 14.0, 12.0),
			leaves: [2]float64{0.80, 0.60},
			want:   0.70,
			level:  artifact.LevelHigh,
		},
		{
			name:   "mild calm day",
			sample: forestRow(t, 20.0, 5.0, 3.0),
			leaves: [2]float64{0.10, 0.20},
			want:   0.15,
			level:  artifact.LevelLow,
		},
		{
			name:   "above the temperature split with light wind",
			sample: forestRow(t, 32.0, 12.0, 5.0),
			leaves: [2]float64{0.55, 0.60},
			want:   0.575,
			level:  artifact.LevelMedium,
		},
		{
			name:   "threshold is inclusive on the left branch",
			sample: forestRow(t, 30.0, 10.0, 8.0),
			leaves: [2]float64{0.10, 0.20},
			want:   0.15,
			level:  artifact.LevelLow,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := scorer.Score(tc.sample)
			if err != nil {
				t.Fatalf("score: %v", err)
			}
			if math.Abs(result.Probability-tc.want) > 1e-12 {
				t.Fatalf("probability %v want %v (leaves %v)", result.Probability, tc.want, tc.leaves)
			}
			if level := model.Level(result.Probability); level != tc.level {
				t.Errorf("level %q want %q", level, tc.level)
			}
		})
	}
}

// TestPreprocessingOrderIsImputeThenCenterThenScale is the order test. It uses a
// one-feature logistic model with coef 1 and intercept 0, so the standardized
// value is exactly the model input, and asserts the value directly instead of
// reverse-engineering it out of a probability.
//
// impute 11, center 3, scale 2 means NaN -> 11 -> 4. Any other order gives a
// different number: standardize-then-impute gives 11, scale-then-impute-then-
// center gives 2.5, and centre-before-impute gives 8/2 = 4 only by coincidence of
// these numbers, which is why the second case uses a different raw value (9)
// that must produce 3.
func TestPreprocessingOrderIsImputeThenCenterThenScale(t *testing.T) {
	intercept := 0.0
	model := &artifact.Artifact{
		Format:    artifact.Format,
		Version:   "order-fixture",
		TrainedAt: artifact.Timestamp{},
		DataHash:  "order",
		Features:  []artifact.Feature{{Name: "temp_max", Unit: "degC", Min: 0, Max: 50}},
		Impute:    []float64{11},
		Center:    []float64{3},
		Scale:     []float64{2},
		Kind:      artifact.KindLogistic,
		Intercept: &intercept,
		Coef:      []float64{1},
	}
	scorer := New(model)

	standardized, err := scorer.standardize(sample(t, "temp_max", math.NaN()))
	if err != nil {
		t.Fatalf("standardize imputed value: %v", err)
	}
	if len(standardized) != 1 || math.Abs(standardized[0]-4) > 1e-12 {
		t.Fatalf("NaN standardized to %v, want (11-3)/2 = 4", standardized)
	}

	standardized, err = scorer.standardize(sample(t, "temp_max", 9))
	if err != nil {
		t.Fatalf("standardize observed value: %v", err)
	}
	if len(standardized) != 1 || math.Abs(standardized[0]-3) > 1e-12 {
		t.Fatalf("9 standardized to %v, want (9-3)/2 = 3", standardized)
	}

	// The composition through Score must agree with the two steps above.
	result, err := scorer.Score(sample(t, "temp_max", 9))
	if err != nil {
		t.Fatalf("score: %v", err)
	}
	want := 1 / (1 + math.Exp(-3))
	if math.Abs(result.Probability-want) > 1e-12 {
		t.Fatalf("probability %v want sigmoid(3)=%v", result.Probability, want)
	}
}

// TestFeatureOrderIsEnforcedByPosition proves the model is positional: the same
// values declared in a different key order must score identically, because the
// scorer places each value at the artifact's index rather than at its position
// in the request.
func TestFeatureOrderIsEnforcedByPosition(t *testing.T) {
	scorer := New(load(t, "logistic.json"))
	forward, err := scorer.Score(logisticRow(t))
	if err != nil {
		t.Fatalf("score forward: %v", err)
	}
	reversed := sample(t,
		"radiation_sol", 1680.0,
		"precip_sum", 0.0,
		"wind_dir", 195.0,
		"wind_speed", 9.5,
		"humidity_rel", 22.0,
		"temp_min", 18.4,
		"temp_max", 34.2,
	)
	backward, err := scorer.Score(reversed)
	if err != nil {
		t.Fatalf("score reversed: %v", err)
	}
	if forward.Probability != backward.Probability {
		t.Fatalf("declaration order changed the answer: %v vs %v", forward.Probability, backward.Probability)
	}
	// A positional implementation that ignored names would still pass the two
	// calls above only if the values were palindromic; swapping two values must
	// move the answer.
	swapped := sample(t,
		"temp_max", 22.0,
		"temp_min", 18.4,
		"humidity_rel", 34.2,
		"wind_speed", 9.5,
		"wind_dir", 195.0,
		"precip_sum", 0.0,
		"radiation_sol", 1680.0,
	)
	different, err := scorer.Score(swapped)
	if err != nil {
		t.Fatalf("score swapped: %v", err)
	}
	if different.Probability == forward.Probability {
		t.Fatal("swapping two feature values did not change the answer")
	}
}

func TestRefusalsCarryStableCodes(t *testing.T) {
	scorer := New(load(t, "logistic.json"))
	for _, tc := range []struct {
		name   string
		sample Sample
		code   string
	}{
		{
			name:   "missing required feature",
			sample: sample(t, "temp_max", 30.0, "temp_min", 12.0, "humidity_rel", 40.0, "wind_speed", 5.0, "wind_dir", 180.0, "precip_sum", 0.0),
			code:   CodeMissingFeature,
		},
		{
			name:   "empty sample",
			sample: Sample{},
			code:   CodeMissingFeature,
		},
		{
			name: "unknown feature alongside every required one",
			sample: sample(t, "temp_max", 30.0, "temp_min", 12.0, "humidity_rel", 40.0, "wind_speed", 5.0,
				"wind_dir", 180.0, "precip_sum", 0.0, "radiation_sol", 900.0, "pressure_atm", 1013.0),
			code: CodeUnknownFeature,
		},
		{
			name: "duplicate feature",
			sample: sample(t, "temp_max", 30.0, "temp_max", 31.0, "temp_min", 12.0, "humidity_rel", 40.0,
				"wind_speed", 5.0, "wind_dir", 180.0, "precip_sum", 0.0, "radiation_sol", 900.0),
			code: CodeDuplicateFeature,
		},
		{
			name: "value above the guard",
			sample: sample(t, "temp_max", 120.0, "temp_min", 12.0, "humidity_rel", 40.0, "wind_speed", 5.0,
				"wind_dir", 180.0, "precip_sum", 0.0, "radiation_sol", 900.0),
			code: CodeOutOfRange,
		},
		{
			name: "value below the guard",
			sample: sample(t, "temp_max", 30.0, "temp_min", 12.0, "humidity_rel", 40.0, "wind_speed", 5.0,
				"wind_dir", -1.0, "precip_sum", 0.0, "radiation_sol", 900.0),
			code: CodeOutOfRange,
		},
		{
			name: "radiation in the wrong unit",
			sample: sample(t, "temp_max", 30.0, "temp_min", 12.0, "humidity_rel", 40.0, "wind_speed", 5.0,
				"wind_dir", 180.0, "precip_sum", 0.0, "radiation_sol", 2400.0),
			code: CodeOutOfRange,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := scorer.Score(tc.sample)
			var invalid *InvalidSampleError
			if !errors.As(err, &invalid) {
				t.Fatalf("error %v is not an InvalidSampleError", err)
			}
			if invalid.Code != tc.code {
				t.Fatalf("code %q want %q (%s)", invalid.Code, tc.code, invalid.Detail)
			}
		})
	}
}

// TestGuardEdgesAreInclusiveAndNeverClamped keeps the guard from drifting into
// an off-by-one or into silent clamping: the declared bounds are accepted
// unchanged, and one step outside is refused with the same code.
func TestGuardEdgesAreInclusiveAndNeverClamped(t *testing.T) {
	scorer := New(load(t, "logistic.json"))
	edges := sample(t,
		"temp_max", 45.0,
		"temp_min", -10.0,
		"humidity_rel", 100.0,
		"wind_speed", 40.0,
		"wind_dir", 360.0,
		"precip_sum", 200.0,
		"radiation_sol", 1758.0,
	)
	edgeResult, err := scorer.Score(edges)
	if err != nil {
		t.Fatalf("the declared bounds must be accepted: %v", err)
	}

	// The same sample, one unit above the radiation guard, must be refused
	// instead of clamped to the edge - clamping would return the edge answer.
	clamped := sample(t,
		"temp_max", 45.0,
		"temp_min", -10.0,
		"humidity_rel", 100.0,
		"wind_speed", 40.0,
		"wind_dir", 360.0,
		"precip_sum", 200.0,
		"radiation_sol", 1759.0,
	)
	_, err = scorer.Score(clamped)
	var invalid *InvalidSampleError
	if !errors.As(err, &invalid) || invalid.Code != CodeOutOfRange {
		t.Fatalf("out-of-guard value must be refused, got %v", err)
	}
	if invalid.Detail == "" || edgeResult.Probability == 0 {
		t.Fatalf("unexpected empty detail or zero probability: %q %v", invalid.Detail, edgeResult.Probability)
	}
}

// TestNaNFeatureIsImputedNotRejected pins the one thing a null means on this
// surface: the caller declared the feature but has no observation for it, which
// is exactly what the artifact's impute array is for. An absent key is still an
// error (TestRefusalsCarryStableCodes above).
func TestNaNFeatureIsImputedNotRejected(t *testing.T) {
	scorer := New(load(t, "logistic.json"))
	imputed, err := scorer.Score(sample(t,
		"temp_max", 34.2,
		"temp_min", math.NaN(),
		"humidity_rel", 22.0,
		"wind_speed", 9.5,
		"wind_dir", 195.0,
		"precip_sum", 0.0,
		"radiation_sol", 1680.0,
	))
	if err != nil {
		t.Fatalf("a null feature must be imputed, not refused: %v", err)
	}

	// The same prediction with the artifact's declared imputation supplied
	// explicitly must be identical: imputation is a substitution, not a special
	// case inside the model.
	explicit, err := scorer.Score(sample(t,
		"temp_max", 34.2,
		"temp_min", 12.0,
		"humidity_rel", 22.0,
		"wind_speed", 9.5,
		"wind_dir", 195.0,
		"precip_sum", 0.0,
		"radiation_sol", 1680.0,
	))
	if err != nil {
		t.Fatalf("score explicit: %v", err)
	}
	if imputed.Probability != explicit.Probability {
		t.Fatalf("imputed %v and explicit %v must match", imputed.Probability, explicit.Probability)
	}
}

// TestNonFiniteProbabilityIsAnError covers both ways the arithmetic can produce a
// number that must never be answered with: a standardized value that overflows
// on the way in, and a logistic sum that cancels +Inf against -Inf into NaN.
func TestNonFiniteProbabilityIsAnError(t *testing.T) {
	intercept := 0.0
	for _, tc := range []struct {
		name  string
		model *artifact.Artifact
	}{
		{
			name: "standardized value overflows",
			model: &artifact.Artifact{
				Format:  artifact.Format,
				Version: "overflow-v1",
				Features: []artifact.Feature{
					{Name: "temp_max", Unit: "degC", Min: -1e308, Max: 1e308},
				},
				Impute:    []float64{0},
				Center:    []float64{-1e308},
				Scale:     []float64{1e-308},
				Kind:      artifact.KindLogistic,
				Intercept: &intercept,
				Coef:      []float64{1},
			},
		},
		{
			name: "logistic sum is not a number",
			model: &artifact.Artifact{
				Format:  artifact.Format,
				Version: "nan-v1",
				Features: []artifact.Feature{
					{Name: "temp_max", Unit: "degC", Min: -1e308, Max: 1e308},
					{Name: "temp_min", Unit: "degC", Min: -1e308, Max: 1e308},
				},
				Impute:    []float64{0, 0},
				Center:    []float64{0, 0},
				Scale:     []float64{1, 1},
				Kind:      artifact.KindLogistic,
				Intercept: &intercept,
				Coef:      []float64{1e308, 1e308},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scorer := New(tc.model)
			values := make(Sample, 0, len(tc.model.Features))
			for i, feature := range tc.model.Features {
				value := 1e308
				if i == 1 {
					value = -1e308
				}
				values = append(values, FeatureValue{Name: feature.Name, Value: value})
			}
			result, err := scorer.Score(values)
			if !errors.Is(err, ErrNonFiniteProbability) {
				t.Fatalf("error %v is not ErrNonFiniteProbability", err)
			}
			if result.Probability != 0 || result.Version != "" {
				t.Fatalf("a failed score must not return a result: %+v", result)
			}
		})
	}
}

// TestForestWalkTerminatesOnAHandBuiltCycle proves the scoring path cannot hang
// even when a caller bypasses artifact validation and hands over a cyclic tree.
func TestForestWalkTerminatesOnAHandBuiltCycle(t *testing.T) {
	zero := 0
	loop := &artifact.Artifact{
		Format:  artifact.Format,
		Version: "cyclic-v1",
		Features: []artifact.Feature{
			{Name: "temp_max", Unit: "degC", Min: 0, Max: 50},
		},
		Impute: []float64{10},
		Center: []float64{0},
		Scale:  []float64{1},
		Kind:   artifact.KindForest,
		Trees: []artifact.Tree{{Nodes: []artifact.Node{
			{Feature: &zero, Threshold: ptrFloat(20), Left: intPtr(0), Right: intPtr(0)},
		}}},
	}
	_, err := New(loop).Score(sample(t, "temp_max", 30.0))
	if err == nil {
		t.Fatal("a cyclic tree must fail, not hang")
	}
}

// forestRow builds the seven-feature sample the forest fixture requires. Only
// temp_max, temp_min and wind_speed drive its thresholds; the other four are
// supplied with plausible values because the contract requires every declared
// feature to be present, not only the ones a tree happens to split on.
func forestRow(t *testing.T, tempMax, tempMin, windSpeed float64) Sample {
	return sample(t,
		"temp_max", tempMax,
		"temp_min", tempMin,
		"humidity_rel", 55.0,
		"wind_speed", windSpeed,
		"wind_dir", 200.0,
		"precip_sum", 0.0,
		"radiation_sol", 900.0,
	)
}

func ptrFloat(value float64) *float64 { return &value }

func intPtr(value int) *int { return &value }
