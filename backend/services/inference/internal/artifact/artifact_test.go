package artifact

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixture loads one committed artifact and returns its decoded form, so a test
// can mutate exactly one field and prove that field is what the contract refuses.
// Editing the bytes as a map keeps every rejection local to the rule it tests.
func fixture(t *testing.T, name string) map[string]any {
	t.Helper()
	document, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatalf("decode fixture %s: %v", name, err)
	}
	return decoded
}

func encode(t *testing.T, document map[string]any) []byte {
	t.Helper()
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode artifact: %v", err)
	}
	return encoded
}

// feature returns the mutable feature object at position i.
func feature(t *testing.T, document map[string]any, i int) map[string]any {
	t.Helper()
	features, ok := document["features"].([]any)
	if !ok || i >= len(features) {
		t.Fatalf("fixture has no features[%d]", i)
	}
	return features[i].(map[string]any)
}

func node(t *testing.T, document map[string]any, tree, index int) map[string]any {
	t.Helper()
	trees := document["trees"].([]any)
	nodes := trees[tree].(map[string]any)["nodes"].([]any)
	return nodes[index].(map[string]any)
}

func TestParseAcceptsBothCommittedFixtures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		kind     string
		features int
	}{
		{"logistic.json", KindLogistic, 7},
		{"forest.json", KindForest, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := Load(filepath.Join("..", "..", "testdata", tc.name))
			if err != nil {
				t.Fatalf("committed fixture must load: %v", err)
			}
			if parsed.Kind != tc.kind {
				t.Errorf("kind %q want %q", parsed.Kind, tc.kind)
			}
			if len(parsed.Features) != tc.features {
				t.Errorf("features %d want %d", len(parsed.Features), tc.features)
			}
			if parsed.Format != Format {
				t.Errorf("format %q want %q", parsed.Format, Format)
			}
			// Every guard must be a real interval with a unit attached: the
			// source mixes units, so a guard without one cannot be checked.
			for i, f := range parsed.Features {
				if f.Name == "" || f.Unit == "" || !(f.Min < f.Max) {
					t.Errorf("features[%d] is not a usable guard: %+v", i, f)
				}
				if len(parsed.Impute) != len(parsed.Features) || len(parsed.Center) != len(parsed.Features) || len(parsed.Scale) != len(parsed.Features) {
					t.Fatalf("preprocessing is not positional at feature %d", i)
				}
			}
		})
	}
}

func TestParseRejectsContractViolations(t *testing.T) {
	type mutate func(t *testing.T, document map[string]any)

	for _, tc := range []struct {
		name    string
		base    string
		mutate  mutate
		wantErr string
	}{
		{
			name:    "bad format",
			base:    "logistic.json",
			mutate:  func(_ *testing.T, d map[string]any) { d["format"] = "firewatch-model/v2" },
			wantErr: `format "firewatch-model/v2" is not "firewatch-model/v1"`,
		},
		{
			name:    "missing version",
			base:    "logistic.json",
			mutate:  func(_ *testing.T, d map[string]any) { delete(d, "version") },
			wantErr: "version is required",
		},
		{
			name:    "missing data hash",
			base:    "logistic.json",
			mutate:  func(_ *testing.T, d map[string]any) { delete(d, "data_hash") },
			wantErr: "data_hash is required",
		},
		{
			name:    "unparsable trained_at",
			base:    "logistic.json",
			mutate:  func(_ *testing.T, d map[string]any) { d["trained_at"] = "2026-11-01" },
			wantErr: "trained_at",
		},
		{
			name:    "misspelled field",
			base:    "logistic.json",
			mutate:  func(_ *testing.T, d map[string]any) { d["coeff"] = d["coef"]; delete(d, "coef") },
			wantErr: "unknown field",
		},
		{
			name:    "no features",
			base:    "logistic.json",
			mutate:  func(_ *testing.T, d map[string]any) { d["features"] = []any{} },
			wantErr: "features is empty",
		},
		{
			name:    "feature without a unit",
			base:    "logistic.json",
			mutate:  func(t *testing.T, d map[string]any) { delete(feature(t, d, 0), "unit") },
			wantErr: `feature "temp_max" has no unit`,
		},
		{
			name:    "duplicate feature",
			base:    "logistic.json",
			mutate:  func(t *testing.T, d map[string]any) { feature(t, d, 1)["name"] = "temp_max" },
			wantErr: `feature "temp_max" is declared twice`,
		},
		{
			name:    "min above max",
			base:    "logistic.json",
			mutate:  func(t *testing.T, d map[string]any) { feature(t, d, 0)["min"] = 90 },
			wantErr: `feature "temp_max" has min 90 and max 45`,
		},
		{
			name:    "min equal to max",
			base:    "logistic.json",
			mutate:  func(t *testing.T, d map[string]any) { feature(t, d, 0)["max"] = -5 },
			wantErr: "min must be below max",
		},
		{
			name:    "impute array too short",
			base:    "logistic.json",
			mutate:  func(_ *testing.T, d map[string]any) { d["impute"] = []float64{1} },
			wantErr: "impute has 1 values for 7 features",
		},
		{
			name:    "scale array missing",
			base:    "logistic.json",
			mutate:  func(_ *testing.T, d map[string]any) { delete(d, "scale") },
			wantErr: "scale has 0 values for 7 features",
		},
		{
			name:    "zero scale",
			base:    "logistic.json",
			mutate:  func(_ *testing.T, d map[string]any) { d["scale"] = []float64{6.5, 4.5, 12, 2, 60, 0, 180} },
			wantErr: "a scale must be positive",
		},
		{
			name:    "impute outside the guard",
			base:    "logistic.json",
			mutate:  func(_ *testing.T, d map[string]any) { d["impute"] = []float64{400, 12, 62, 3.5, 210, 0, 420} },
			wantErr: "outside [-5, 45]",
		},
		{
			name:    "coefficient length mismatch",
			base:    "logistic.json",
			mutate:  func(_ *testing.T, d map[string]any) { d["coef"] = []float64{0.62, 0.18} },
			wantErr: "coef has 2 values for 7 features",
		},
		{
			name:    "logistic without intercept",
			base:    "logistic.json",
			mutate:  func(_ *testing.T, d map[string]any) { delete(d, "intercept") },
			wantErr: "kind logistic requires intercept",
		},
		{
			name:    "logistic carrying trees",
			base:    "logistic.json",
			mutate:  func(t *testing.T, d map[string]any) { d["trees"] = fixture(t, "forest.json")["trees"] },
			wantErr: "kind logistic carries trees",
		},
		{
			name:    "forest carrying coef",
			base:    "forest.json",
			mutate:  func(_ *testing.T, d map[string]any) { d["coef"] = []float64{0.62} },
			wantErr: "kind forest carries intercept or coef",
		},
		{
			name:    "forest without trees",
			base:    "forest.json",
			mutate:  func(_ *testing.T, d map[string]any) { d["trees"] = []any{} },
			wantErr: "kind forest requires at least one tree",
		},
		{
			name:    "unknown kind",
			base:    "logistic.json",
			mutate:  func(_ *testing.T, d map[string]any) { d["kind"] = "neural" },
			wantErr: `kind "neural"`,
		},
		{
			name:    "forest node index out of bounds",
			base:    "forest.json",
			mutate:  func(t *testing.T, d map[string]any) { node(t, d, 0, 0)["right"] = 99 },
			wantErr: "right index 99 is outside [0, 5)",
		},
		{
			name:    "forest feature index out of bounds",
			base:    "forest.json",
			mutate:  func(t *testing.T, d map[string]any) { node(t, d, 0, 0)["feature"] = 7 },
			wantErr: "feature index 7 is outside [0, 7)",
		},
		{
			name:    "forest node pointing at itself",
			base:    "forest.json",
			mutate:  func(t *testing.T, d map[string]any) { node(t, d, 0, 2)["left"] = 2 },
			wantErr: "left points at itself",
		},
		{
			name: "forest cycle",
			base: "forest.json",
			mutate: func(t *testing.T, d map[string]any) {
				// 2.left -> 0 and 0.right -> 2 closes a loop while every index
				// stays in bounds, which is exactly the case an index check alone
				// cannot see.
				node(t, d, 0, 2)["left"] = 0
			},
			wantErr: "the tree has a cycle",
		},
		{
			name: "forest unreachable node",
			base: "forest.json",
			mutate: func(t *testing.T, d map[string]any) {
				trees := d["trees"].([]any)
				tree := trees[0].(map[string]any)
				tree["nodes"] = append(tree["nodes"].([]any), map[string]any{"value": 0.4})
			},
			wantErr: "is unreachable from the root",
		},
		{
			name:    "forest leaf with split fields",
			base:    "forest.json",
			mutate:  func(t *testing.T, d map[string]any) { node(t, d, 0, 1)["feature"] = 0 },
			wantErr: "is a leaf and also carries split fields",
		},
		{
			name:    "forest leaf value outside the probability range",
			base:    "forest.json",
			mutate:  func(t *testing.T, d map[string]any) { node(t, d, 0, 1)["value"] = 1.4 },
			wantErr: "is not a probability in [0, 1]",
		},
		{
			name:    "forest split node missing a child",
			base:    "forest.json",
			mutate:  func(t *testing.T, d map[string]any) { delete(node(t, d, 0, 2), "right") },
			wantErr: "is missing feature, threshold, left or right",
		},
		{
			name:    "thresholds out of order",
			base:    "logistic.json",
			mutate:  func(_ *testing.T, d map[string]any) { d["thresholds"] = map[string]any{"medium": 0.8, "high": 0.4} },
			wantErr: "must satisfy 0 < medium < high < 1",
		},
		{
			name:    "metrics not an object",
			base:    "logistic.json",
			mutate:  func(_ *testing.T, d map[string]any) { d["metrics"] = "roc_auc=0.78" },
			wantErr: "metrics must be a JSON object",
		},
		{
			name:    "two documents in one file",
			base:    "logistic.json",
			mutate:  func(_ *testing.T, d map[string]any) { d["_second"] = map[string]any{} },
			wantErr: "unknown field",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			document := fixture(t, tc.base)
			tc.mutate(t, document)
			_, err := Parse(encode(t, document))
			if err == nil {
				t.Fatalf("expected the contract to reject this artifact")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestParseRejectsTrailingDocument(t *testing.T) {
	document := encode(t, fixture(t, "logistic.json"))
	if _, err := Parse(append(document, []byte("\n{}")...)); err == nil || !strings.Contains(err.Error(), "more than one JSON document") {
		t.Fatalf("trailing document must be rejected, got %v", err)
	}
}

func TestParseRejectsTruncatedDocument(t *testing.T) {
	document := encode(t, fixture(t, "logistic.json"))
	if _, err := Parse(document[:len(document)/2]); err == nil {
		t.Fatal("a truncated artifact must not parse")
	}
}

// validLogistic is a hand-built artifact that satisfies every rule, so the
// non-finite cases below can isolate one field each. A JSON fixture cannot carry
// NaN or an infinity, which is why these are built as structs.
func validLogistic() *Artifact {
	intercept := -1.5
	return &Artifact{
		Format:    Format,
		Version:   "hand-built-v1",
		TrainedAt: Timestamp{mustTime()},
		DataHash:  strings.Repeat("a", 64),
		Features: []Feature{
			{Name: "temp_max", Unit: "degC", Min: -5, Max: 45},
			{Name: "wind_speed", Unit: "m/s", Min: 0, Max: 40},
		},
		Impute:    []float64{20, 4},
		Center:    []float64{18, 4},
		Scale:     []float64{5, 2},
		Kind:      KindLogistic,
		Intercept: &intercept,
		Coef:      []float64{0.5, -0.25},
	}
}

func TestValidateRejectsNonFiniteNumbers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(*Artifact)
		wantErr string
	}{
		{"feature min", func(a *Artifact) { a.Features[0].Min = math.NaN() }, "non-finite min/max"},
		{"feature max", func(a *Artifact) { a.Features[1].Max = math.Inf(1) }, "non-finite min/max"},
		{"impute", func(a *Artifact) { a.Impute[0] = math.NaN() }, "impute[0] (temp_max) is not finite"},
		{"center", func(a *Artifact) { a.Center[1] = math.Inf(-1) }, "center[1] (wind_speed) is not finite"},
		{"scale", func(a *Artifact) { a.Scale[0] = math.Inf(1) }, "scale[0] (temp_max) is not finite"},
		{"intercept", func(a *Artifact) { value := math.Inf(1); a.Intercept = &value }, "intercept is not finite"},
		{"coef", func(a *Artifact) { a.Coef[1] = math.NaN() }, "coef[1] (wind_speed) is not finite"},
		{"threshold medium", func(a *Artifact) { a.Thresholds = &Thresholds{Medium: math.NaN(), High: 0.7} }, "thresholds must be finite"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := validLogistic()
			tc.mutate(a)
			err := a.Validate()
			if err == nil {
				t.Fatal("expected the contract to reject this artifact")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateRejectsNonFiniteForestNumbers(t *testing.T) {
	zero, nan := 0, math.NaN()
	for _, tc := range []struct {
		name    string
		nodes   []Node
		wantErr string
	}{
		{
			name:    "leaf value",
			nodes:   []Node{{Feature: &zero, Threshold: ptrFloat(1), Left: intPtr(1), Right: intPtr(2)}, {Value: &nan}, {Value: ptrFloat(0.2)}},
			wantErr: "is not a probability in [0, 1]",
		},
		{
			name:    "threshold",
			nodes:   []Node{{Feature: &zero, Threshold: &nan, Left: intPtr(1), Right: intPtr(2)}, {Value: ptrFloat(0.1)}, {Value: ptrFloat(0.2)}},
			wantErr: "threshold is not finite",
		},
		{
			name: "every node is an internal node",
			nodes: []Node{
				{Feature: &zero, Threshold: ptrFloat(1), Left: intPtr(1), Right: intPtr(1)},
				{Feature: &zero, Threshold: ptrFloat(1), Left: intPtr(1), Right: intPtr(1)},
			},
			wantErr: "left points at itself",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := validLogistic()
			a.Kind = KindForest
			a.Intercept = nil
			a.Coef = nil
			a.Trees = []Tree{{Nodes: tc.nodes}}
			err := a.Validate()
			if err == nil {
				t.Fatal("expected the contract to reject this forest")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestLevelUsesArtifactThresholdsOrDocumentedDefaults(t *testing.T) {
	withThresholds := validLogistic()
	withThresholds.Thresholds = &Thresholds{Medium: 0.2, High: 0.5}
	withoutThresholds := validLogistic()

	for _, tc := range []struct {
		name        string
		artifact    *Artifact
		probability float64
		want        Level
	}{
		{"below medium", withThresholds, 0.19, LevelLow},
		{"at medium", withThresholds, 0.20, LevelMedium},
		{"below high", withThresholds, 0.499, LevelMedium},
		{"at high", withThresholds, 0.50, LevelHigh},
		{"default low", withoutThresholds, 0.399, LevelLow},
		{"default medium", withoutThresholds, 0.40, LevelMedium},
		{"default medium upper", withoutThresholds, 0.699, LevelMedium},
		{"default high", withoutThresholds, 0.70, LevelHigh},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.artifact.Level(tc.probability); got != tc.want {
				t.Errorf("Level(%v) = %q want %q", tc.probability, got, tc.want)
			}
		})
	}

	medium, high := withoutThresholds.LevelThresholds()
	if medium != DefaultMediumThreshold || high != DefaultHighThreshold {
		t.Errorf("defaults are %v/%v, want the documented %v/%v", medium, high, DefaultMediumThreshold, DefaultHighThreshold)
	}
}

func TestFeatureNamesAndIndexFollowOrder(t *testing.T) {
	a := validLogistic()
	if got := a.FeatureNames(); len(got) != 2 || got[0] != "temp_max" || got[1] != "wind_speed" {
		t.Fatalf("FeatureNames() = %v", got)
	}
	if index, ok := a.Index("wind_speed"); !ok || index != 1 {
		t.Errorf("Index(wind_speed) = %d, %v", index, ok)
	}
	if _, ok := a.Index("humidity_rel"); ok {
		t.Error("Index reported a feature the artifact does not declare")
	}
}

func mustTime() time.Time { return time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC) }

func intPtr(value int) *int { return &value }

func ptrFloat(value float64) *float64 { return &value }
