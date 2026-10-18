package classify

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The tests below are pure unit tests: no clock, no database, no external state.
// They pin the fingerprinting behaviour and the frozen parameter set of
// academic-rule-v1.

func TestParamsHashIsCanonicalAndOrderIndependent(t *testing.T) {
	first := map[string]string{}
	first["a"] = "1"
	first["b"] = "2"

	second := map[string]string{}
	second["b"] = "2"
	second["a"] = "1"

	if ParamsHash(first) != ParamsHash(second) {
		t.Fatalf("hash depends on insertion order: %q vs %q", ParamsHash(first), ParamsHash(second))
	}
	if ParamsHash(first) != ParamsHash(first) {
		t.Fatalf("hash is not stable across calls on the same map")
	}

	// The ambiguity guard: naive concatenation would render both maps as the
	// same bytes "a=1\nb=2\n"; the length-prefixed encoding must keep them apart.
	ambiguous := map[string]string{"a": "1", "b": "2"}
	colliding := map[string]string{"a": "1\nb=2"}
	if ParamsHash(ambiguous) == ParamsHash(colliding) {
		t.Fatalf("length prefixes failed: %q and %q hash the same", ambiguous, colliding)
	}
}

func TestParamsHashDistinguishesParameters(t *testing.T) {
	base := map[string]string{"a": "1", "b": "2"}
	changed := map[string]string{"a": "1", "b": "3"}
	if ParamsHash(base) == ParamsHash(changed) {
		t.Fatalf("changing a single value left the hash unchanged")
	}

	empty := ParamsHash(map[string]string{})
	if empty == "" {
		t.Fatalf("empty map produced an empty hash")
	}
	if empty != ParamsHash(map[string]string{}) {
		t.Fatalf("empty map hash is not stable")
	}
	if empty != ParamsHash(nil) {
		t.Fatalf("nil and empty maps should hash identically")
	}
}

// TestParamsDerivationCoversEveryBehaviorFieldExactly proves the canonical map is
// the field set, not a hand-written list: every field is exported and tagged, the
// derived key set is exactly the tag set, and the derivation refuses the malformed
// shapes (missing tag, duplicate tag, non-numeric field) that would otherwise let
// a behaviour number escape the fingerprint.
func TestParamsDerivationCoversEveryBehaviorFieldExactly(t *testing.T) {
	typ := reflect.TypeOf(Params{})
	derived, err := DefaultAcademicRuleV1Params().Parameters()
	if err != nil {
		t.Fatalf("DefaultAcademicRuleV1Params().Parameters() failed: %v", err)
	}
	if typ.NumField() != len(derived) {
		t.Fatalf("Params has %d fields but the derivation produced %d keys", typ.NumField(), len(derived))
	}

	tags := map[string]string{}
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		if field.PkgPath != "" {
			t.Errorf("field %s is not exported", field.Name)
			continue
		}
		tag := field.Tag.Get("param")
		if tag == "" {
			t.Errorf("field %s has no param tag", field.Name)
			continue
		}
		if previous, duplicate := tags[tag]; duplicate {
			t.Errorf("tag %q is shared by %s and %s", tag, previous, field.Name)
			continue
		}
		tags[tag] = field.Name
		if _, present := derived[tag]; !present {
			t.Errorf("the derivation is missing key %q for field %s", tag, field.Name)
		}
	}
	if len(tags) != len(derived) {
		t.Fatalf("the tag set has %d entries but the derivation produced %d keys", len(tags), len(derived))
	}
	for tag := range derived {
		if _, present := tags[tag]; !present {
			t.Errorf("the derivation produced key %q that no field declares", tag)
		}
	}

	// The derivation must reject malformed shapes rather than silently dropping
	// them; these are the guards that make "derived from the fields" trustworthy.
	type duplicateTags struct {
		First  float64 `param:"same"`
		Second float64 `param:"same"`
	}
	if _, err := deriveParameters(reflect.ValueOf(duplicateTags{})); err == nil {
		t.Error("the derivation accepted two fields with the same param tag")
	}
	type missingTag struct {
		Untagged float64
	}
	if _, err := deriveParameters(reflect.ValueOf(missingTag{})); err == nil {
		t.Error("the derivation accepted a field with no param tag")
	}
	type unsupportedKind struct {
		Text string `param:"text"`
	}
	if _, err := deriveParameters(reflect.ValueOf(unsupportedKind{})); err == nil {
		t.Error("the derivation accepted a field that is neither int64 nor float64")
	}
}

// probeFeatures builds one probe set from the five canonical values, evaluated the
// given age after the fixed reference instant.
func probeFeatures(temperature, humidity, wind, vegetation, fires float64, age time.Duration) Features {
	return Features{
		ObservedAt:          referenceInstant,
		EvaluatedAt:         referenceInstant.Add(age),
		ZoneID:              "valparaiso",
		TemperatureC:        Known(temperature),
		RelativeHumidityPct: Known(humidity),
		WindSpeedKmh:        Known(wind),
		VegetationCoverPct:  Known(vegetation),
		HistoricalFires:     Known(fires),
	}
}

// behaviorProbes are the fixed inputs shared by the parameter-mutation and
// score-vector tests. They straddle every threshold: the all-mid point, the two
// interiors just next to the low and high extremes, the staleness boundary, and
// the exact edge where every rising contribution floors at 0.
func behaviorProbes() []struct {
	name     string
	features Features
} {
	return []struct {
		name     string
		features Features
	}{
		{"all-mid", probeFeatures(25, 40, 30, 40, 2.5, 0)},
		{"near-low", probeFeatures(16, 59, 11, 11, 0.1, 0)},
		{"near-high", probeFeatures(34.9, 20.1, 49.9, 69.9, 4.9, 0)},
		{"stale", probeFeatures(25, 40, 30, 40, 2.5, 22200*time.Second)},
		{"edge", probeFeatures(15, 60, 10, 10, 0, 0)},
	}
}

// behaviorFingerprint is the observable behaviour of a rule across every probe:
// the level, the rationale and every factor's name, presence and contribution.
func behaviorFingerprint(t *testing.T, rule AcademicRuleV1) string {
	t.Helper()
	var builder strings.Builder
	for _, probe := range behaviorProbes() {
		assessment, err := rule.Classify(t.Context(), probe.features)
		if err != nil {
			t.Fatalf("probe %s: Classify failed: %v", probe.name, err)
		}
		fmt.Fprintf(&builder, "%s|level=%s|rationale=%s", probe.name, assessment.Level, assessment.Rationale)
		for _, factor := range assessment.Factors {
			fmt.Fprintf(&builder, "|%s,%t,%v", factor.Name, factor.Known, factor.Contribution)
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

// TestEveryBehaviorFieldChangesTheHashAndTheBehavior is the core invariant of this
// task: the behaviour is read only from the Params object. Mutating any single
// field must change both the frozen fingerprint and what the rule actually does.
// If a field is not detected, the correct response is a new probe that exercises
// it - never to relax the assertion.
func TestEveryBehaviorFieldChangesTheHashAndTheBehavior(t *testing.T) {
	base := DefaultAcademicRuleV1Params()
	baseHash, err := base.Hash()
	if err != nil {
		t.Fatalf("default Hash() failed: %v", err)
	}
	baseFingerprint := behaviorFingerprint(t, newAcademicRuleV1(base))

	typ := reflect.TypeOf(Params{})
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		mutated := base
		mutatedField := reflect.ValueOf(&mutated).Elem().Field(index)
		switch mutatedField.Kind() {
		case reflect.Int64:
			mutatedField.SetInt(mutatedField.Int() + 3600)
		case reflect.Float64:
			mutatedField.SetFloat(mutatedField.Float()*2 + 1)
		default:
			t.Fatalf("field %s has unsupported kind %s", field.Name, mutatedField.Kind())
		}

		mutatedHash, err := mutated.Hash()
		if err != nil {
			t.Fatalf("mutating %s broke Hash(): %v", field.Name, err)
		}
		if mutatedHash == baseHash {
			t.Errorf("mutating %s left the hash unchanged", field.Name)
		}

		mutatedFingerprint := behaviorFingerprint(t, newAcademicRuleV1(mutated))
		if mutatedFingerprint == baseFingerprint {
			t.Errorf("mutating %s left every probe's behaviour unchanged; add a probe that exercises it", field.Name)
		}
	}
}

func TestAcademicRuleV1ParametersAreFrozen(t *testing.T) {
	// This literal map is the golden value. Duplicating it here, by hand, is the
	// point: if someone edits DefaultAcademicRuleV1Params this test fails and the
	// correct fix is a new version, not a silent update of this map.
	want := map[string]string{
		"max_staleness_seconds": "21600",
		"temperature_low_c":     "15",
		"temperature_high_c":    "35",
		"humidity_dry_pct":      "20",
		"humidity_wet_pct":      "60",
		"wind_low_kmh":          "10",
		"wind_high_kmh":         "50",
		"vegetation_sparse_pct": "10",
		"vegetation_dense_pct":  "70",
		"historical_fires_low":  "0",
		"historical_fires_high": "5",
		"weight_temperature":    "0.25",
		"weight_humidity":       "0.25",
		"weight_wind":           "0.2",
		"weight_vegetation":     "0.15",
		"weight_historical":     "0.15",
		"band_bajo_max":         "0.34",
		"band_medio_max":        "0.67",
	}

	got, err := DefaultAcademicRuleV1Params().Parameters()
	if err != nil {
		t.Fatalf("DefaultAcademicRuleV1Params().Parameters() failed: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("academic-rule-v1 parameters changed:\n got: %#v\nwant: %#v", got, want)
	}

	hash, err := DefaultAcademicRuleV1Params().Hash()
	if err != nil {
		t.Fatalf("DefaultAcademicRuleV1Params().Hash() failed: %v", err)
	}
	if hash != AcademicRuleV1ParamsHash {
		t.Fatalf("academic-rule-v1 parameters hash = %q, want frozen %q", hash, AcademicRuleV1ParamsHash)
	}
	t.Logf("computed default parameters hash = %s", hash)
}

// TestAcademicRuleV1ScoreVectorIsFrozen pins the score of the probe sets with
// numbers computed by hand, independently of the code, so a behaviour number
// added outside Params cannot slip through:
//
//	all-mid   (25/40/30/40/2.5): 0.25*0.5 + 0.25*0.5 + 0.20*0.5 + 0.15*0.5 + 0.15*0.5 = 0.5
//	near-low  (16/59/11/11/0.1): 0.25*0.05 + 0.25*0.025 + 0.20*0.025 + 0.15*(1/60) + 0.15*0.02
//	                             = 0.0125 + 0.00625 + 0.005 + 0.0025 + 0.003 = 0.02925
//	near-high (34.9/20.1/49.9/69.9/4.9): 0.25*0.995 + 0.25*0.9975 + 0.20*0.9975
//	                             + 0.15*(59.9/60) + 0.15*0.98
//	                             = 0.24875 + 0.249375 + 0.1995 + 0.14975 + 0.147 = 0.994375
//	edge      (15/60/10/10/0):   every contribution floors at 0, score 0
//
// The stale probe reuses the all-mid values, so score() still yields 0.5; only
// Classify returns desconocido for it, because of the staleness gate.
func TestAcademicRuleV1ScoreVectorIsFrozen(t *testing.T) {
	params := DefaultAcademicRuleV1Params()

	probes := map[string]Features{}
	for _, probe := range behaviorProbes() {
		probes[probe.name] = probe.features
	}

	cases := []struct {
		probe string
		score float64
		level Level
	}{
		{"all-mid", 0.5, LevelMedio},
		{"near-low", 0.02925, LevelBajo},
		{"near-high", 0.994375, LevelAlto},
		{"edge", 0, LevelBajo},
	}
	for _, testCase := range cases {
		t.Run(testCase.probe, func(t *testing.T) {
			features := probes[testCase.probe]
			score := params.score(features)
			if math.Abs(score-testCase.score) > 1e-12 {
				t.Fatalf("score = %.15g, want %.15g", score, testCase.score)
			}
			if level := params.levelFor(score); level != testCase.level {
				t.Fatalf("levelFor(%.15g) = %q, want %q", score, level, testCase.level)
			}
		})
	}

	stale := probes["stale"]
	if score := params.score(stale); math.Abs(score-0.5) > 1e-12 {
		t.Fatalf("stale probe score = %.15g, want 0.5", score)
	}
	assessment, err := newAcademicRuleV1(params).Classify(t.Context(), stale)
	if err != nil {
		t.Fatalf("Classify on the stale probe failed: %v", err)
	}
	if assessment.Level != LevelDesconocido {
		t.Fatalf("stale probe classified as %q, want %q", assessment.Level, LevelDesconocido)
	}
}

// TestTheHashDescribesTheInstanceActuallyUsed is the end-to-end proof: an injected
// staleness threshold changes the decision and is reported on the assessment, and
// the reported hash is that of the injected instance, not the frozen default.
func TestTheHashDescribesTheInstanceActuallyUsed(t *testing.T) {
	injected := DefaultAcademicRuleV1Params()
	injected.MaxStalenessSeconds = 60

	rule, err := NewAcademicRuleV1WithParams(injected)
	if err != nil {
		t.Fatalf("NewAcademicRuleV1WithParams failed: %v", err)
	}

	features := midRiskFeatures()
	features.EvaluatedAt = features.ObservedAt.Add(5 * time.Minute)

	assessment, err := rule.Classify(t.Context(), features)
	if err != nil {
		t.Fatalf("Classify with the injected rule failed: %v", err)
	}
	if assessment.Level != LevelDesconocido {
		t.Fatalf("a 5m old observation under a 60s threshold produced %q, want %q", assessment.Level, LevelDesconocido)
	}

	defaultAssessment, err := NewAcademicRuleV1().Classify(t.Context(), features)
	if err != nil {
		t.Fatalf("Classify with the default rule failed: %v", err)
	}
	if defaultAssessment.Level != LevelMedio {
		t.Fatalf("the default rule produced %q for the same 5m old observation, want %q", defaultAssessment.Level, LevelMedio)
	}

	injectedHash, err := injected.Hash()
	if err != nil {
		t.Fatalf("hashing the injected parameters failed: %v", err)
	}
	if assessment.Policy.ParamsHash != injectedHash {
		t.Fatalf("Policy.ParamsHash = %q, want the injected hash %q", assessment.Policy.ParamsHash, injectedHash)
	}
	if assessment.Policy.ParamsHash == AcademicRuleV1ParamsHash {
		t.Fatal("the injected instance reported the frozen default hash")
	}
	if got := assessment.Policy.Parameters["max_staleness_seconds"]; got != "60" {
		t.Fatalf("Policy.Parameters[max_staleness_seconds] = %q, want %q", got, "60")
	}
}

func TestCanonicalFormRoundTrips(t *testing.T) {
	params := DefaultAcademicRuleV1Params()

	derived, err := params.Parameters()
	if err != nil {
		t.Fatalf("Parameters() failed: %v", err)
	}
	rederived, err := params.Parameters()
	if err != nil {
		t.Fatalf("re-deriving Parameters() failed: %v", err)
	}
	if ParamsHash(derived) != ParamsHash(rederived) {
		t.Fatalf("re-deriving the same parameters changed the hash")
	}

	hash, err := params.Hash()
	if err != nil {
		t.Fatalf("Hash() failed: %v", err)
	}
	if hash != ParamsHash(derived) {
		t.Fatalf("Hash() = %q, want ParamsHash(Parameters()) = %q", hash, ParamsHash(derived))
	}

	if _, err := strconv.ParseInt(derived["max_staleness_seconds"], 10, 64); err != nil {
		t.Fatalf("max_staleness_seconds is not a base-10 integer: %v", err)
	}
	if got := derived["band_bajo_max"]; got != strconv.FormatFloat(0.34, 'g', -1, 64) {
		t.Fatalf("band_bajo_max = %q, want %q", got, strconv.FormatFloat(0.34, 'g', -1, 64))
	}
}

// numericViolation is one numeric literal that escaped the Params object.
type numericViolation struct {
	position token.Position
	literal  string
	reason   string
}

func (v numericViolation) String() string {
	return fmt.Sprintf("%s: numeric literal %q: %s", v.position, v.literal, v.reason)
}

// sourceRange is a half-open [start, end) position range inside one file.
type sourceRange struct {
	start token.Pos
	end   token.Pos
}

// structuralViolations parses one non-test source file and returns every numeric
// literal that the structural guard forbids.
//
// What it DOES guarantee: no behaviour number escapes the Params object as a
// literal. The rules are scoped by file, because "structural" is not the same
// everywhere:
//
//   - No package-level const or var may carry a numeric value. 0 and 1 are the
//     only exemptions: they are zero checks, loop bounds and unit steps, not rule
//     thresholds.
//   - Behaviour files (academic_rule.go, classifier.go, and any future non-test
//     file other than params.go) may contain only 0 and 1. Every other integer or
//     float is a behaviour number and belongs in Params, so an inline threshold
//     such as `value > 100` can no longer hide from the frozen hash.
//   - The literal 1 is allowed only in a structural position: a return result, an
//     index or slice bound, a call argument, a loop clause or an assignment
//     right-hand side. Anywhere else it is behaviour. In particular it must not
//     be an operand of a comparison (`if value == 1` can change a level without
//     touching the frozen parameters) nor of arithmetic (`score + 1`). The
//     literal 0 stays allowed as a zero check, including `len(x) > 0` and
//     `age < 0`, because those compare against emptiness or a sign, not a rule
//     threshold; forbidding them would also flag the frozen params.go code.
//   - params.go is the one file allowed to define behaviour numbers. Its
//     DefaultAcademicRuleV1Params body must be EXACTLY one return statement whose
//     single result is one composite literal; any other statement - an if, an
//     assignment, a nested const, a second return - is a violation. Only the
//     literals INSIDE that composite literal are exempt, because those values ARE
//     the hashed parameters that TestAcademicRuleV1ParametersAreFrozen already
//     pins by hand. Inside deriveParameters, only the two encoding constants 10
//     and 64 are allowed, because that is exactly what strconv.FormatInt (base
//     10) and strconv.FormatFloat (64-bit float size) need to encode the
//     canonical map; they are encoding, never rule thresholds. Outside those two
//     functions params.go may still use 0 and 1.
//
// The one documented exception is the literal 1 in falling's `1 - linear(...)`
// complement. The only literal-free reformulation changes the floating-point
// expression and would break the bit-exact behaviour preservation the A.1
// refactor achieved, so a narrow exemption is the honest trade. The frozen
// behaviour table, not this guard, is what actually locks those helpers.
//
// What it does NOT guarantee: it can NEVER see an operator change that uses no
// literal. Rewriting `score < p.BandBajoMax` as `score <= p.BandBajoMax` changes a
// level and introduces no number the parser can see. This guard reduces the
// attack surface; it is NOT the lock.
//
// It takes the source bytes rather than reading from disk so the same logic can
// be exercised over synthetic sources in the positive controls below.
func structuralViolations(name string, src []byte) ([]numericViolation, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		return nil, err
	}

	parents := parentMap(file)
	falling := functionRanges(file, "falling")
	defaults, shapeViolations := defaultsCompositeRanges(file, fset)
	encoding := functionRanges(file, "deriveParameters")
	seen := map[token.Pos]bool{}
	violations := append([]numericViolation(nil), shapeViolations...)
	record := func(literal *ast.BasicLit, reason string) {
		if seen[literal.Pos()] {
			return
		}
		seen[literal.Pos()] = true
		violations = append(violations, numericViolation{
			position: fset.Position(literal.Pos()),
			literal:  literal.Value,
			reason:   reason,
		})
	}

	// Rule 1: package-level const/var may not carry numeric values.
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || (genDecl.Tok != token.CONST && genDecl.Tok != token.VAR) {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, value := range valueSpec.Values {
				ast.Inspect(value, func(node ast.Node) bool {
					literal, ok := node.(*ast.BasicLit)
					if !ok || !isNumericLiteral(literal) || isStructuralLiteral(literal) {
						return true
					}
					record(literal, fmt.Sprintf("a package-level %s must not carry a numeric value", genDecl.Tok))
					return true
				})
			}
		}
	}

	// Rule 2: the numeric literals of the file.
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || !isNumericLiteral(literal) {
			return true
		}
		if name == "params.go" {
			if insideAnyRange(literal.Pos(), defaults) {
				return true
			}
			if insideAnyRange(literal.Pos(), encoding) && isEncodingConstant(literal) {
				return true
			}
		}
		if literal.Kind == token.INT && literal.Value == "1" {
			if isFallingComplement(literal, parents, falling) {
				return true
			}
			reason, structural := oneIsStructural(literal, parents)
			if !structural {
				record(literal, reason)
			}
			return true
		}
		if isStructuralLiteral(literal) {
			return true
		}
		record(literal, "only 0 and 1 are structural; every other behaviour number must live in defaultAcademicRuleV1Params")
		return true
	})

	return violations, nil
}

// defaultsCompositeRanges returns the range of the single composite literal that
// each Default*Params function is allowed to return, plus a violation for every
// occurrence whose body is not exactly that. The composite literal is the ONLY
// place a params file may spell behaviour numbers, because those values ARE the
// hashed parameters. Every rule version gets its own Default*Params function, so
// adding a version does not widen the exemption to any other function.
func defaultsCompositeRanges(file *ast.File, fset *token.FileSet) ([]sourceRange, []numericViolation) {
	var ranges []sourceRange
	var violations []numericViolation
	for _, decl := range file.Decls {
		function, ok := decl.(*ast.FuncDecl)
		if !ok || function.Body == nil || !isDefaultParamsFunction(function.Name.Name) {
			continue
		}
		composite, ok := singleReturnComposite(function.Body)
		if !ok {
			violations = append(violations, numericViolation{
				position: fset.Position(function.Body.Pos()),
				literal:  "(body)",
				reason:   function.Name.Name + " must be exactly one return statement whose single result is one composite literal: any other statement is a place to hide behaviour outside the hashed parameters",
			})
			continue
		}
		ranges = append(ranges, sourceRange{start: composite.Pos(), end: composite.End()})
	}
	return ranges, violations
}

// isDefaultParamsFunction reports whether a function is one of the versioned
// parameter-set constructors whose single composite return is the only place
// behaviour numbers may be spelled. It matches DefaultAcademicRuleV1Params and
// DefaultFirewatchRuleV2Params alike, and nothing else.
func isDefaultParamsFunction(name string) bool {
	return strings.HasPrefix(name, "Default") && strings.HasSuffix(name, "Params")
}

// singleReturnComposite reports whether body is exactly `return <composite
// literal>`. A wrapped result such as `return (Params{...})` is accepted.
func singleReturnComposite(body *ast.BlockStmt) (*ast.CompositeLit, bool) {
	if len(body.List) != 1 {
		return nil, false
	}
	ret, ok := body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return nil, false
	}
	result := ret.Results[0]
	for {
		paren, ok := result.(*ast.ParenExpr)
		if !ok {
			break
		}
		result = paren.X
	}
	composite, ok := result.(*ast.CompositeLit)
	return composite, ok
}

// parentMap maps every node to its parent, so a literal can be classified by the
// position it occupies rather than by the file it lives in.
func parentMap(file *ast.File) map[ast.Node]ast.Node {
	parents := map[ast.Node]ast.Node{}
	var stack []ast.Node
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			return false
		}
		if len(stack) > 0 {
			parents[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		return true
	})
	return parents
}

// oneIsStructural reports whether the literal 1 sits in a structural position,
// and when it does not, why that is a violation.
func oneIsStructural(literal *ast.BasicLit, parents map[ast.Node]ast.Node) (string, bool) {
	// Parentheses and a leading sign are transparent: `-1` and `(1)` are
	// positioned by the expression that governs them.
	node := ast.Node(literal)
	parent := parents[node]
	for {
		if paren, ok := parent.(*ast.ParenExpr); ok {
			node = paren
			parent = parents[node]
			continue
		}
		if unary, ok := parent.(*ast.UnaryExpr); ok && (unary.Op == token.SUB || unary.Op == token.ADD) {
			node = unary
			parent = parents[node]
			continue
		}
		break
	}

	switch parentNode := parent.(type) {
	case *ast.ReturnStmt:
		return "", true
	case *ast.AssignStmt:
		for _, rhs := range parentNode.Rhs {
			if rhs == node {
				return "", true
			}
		}
	case *ast.CallExpr:
		for _, arg := range parentNode.Args {
			if arg == node {
				return "", true
			}
		}
	case *ast.IndexExpr:
		if parentNode.Index == node {
			return "", true
		}
	case *ast.SliceExpr:
		if parentNode.Low == node || parentNode.High == node || parentNode.Max == node {
			return "", true
		}
	case *ast.ForStmt:
		if parentNode.Init == node || parentNode.Cond == node || parentNode.Post == node {
			return "", true
		}
	case *ast.BinaryExpr:
		if isComparison(parentNode.Op) {
			return "the literal 1 must not be an operand of a comparison: `if value == 1` can change a level without touching the frozen parameters", false
		}
		return "the literal 1 is arithmetic here, and arithmetic can change a level without touching the frozen parameters", false
	}
	return "the literal 1 is only structural as a return result, an index or slice bound, a call argument, a loop clause or an assignment right-hand side", false
}

// isFallingComplement reports whether the literal is the 1 of `1 - linear(...)`
// inside the falling helper: the one documented exemption.
func isFallingComplement(literal *ast.BasicLit, parents map[ast.Node]ast.Node, falling []sourceRange) bool {
	if literal.Value != "1" || !insideAnyRange(literal.Pos(), falling) {
		return false
	}
	binary, ok := parents[literal].(*ast.BinaryExpr)
	return ok && binary.Op == token.SUB && binary.X == literal
}

// isComparison reports whether op is one of the six comparison operators.
func isComparison(op token.Token) bool {
	return op == token.EQL || op == token.NEQ || op == token.LSS ||
		op == token.LEQ || op == token.GTR || op == token.GEQ
}

// functionRanges returns the body ranges of every top-level function with name.
func functionRanges(file *ast.File, name string) []sourceRange {
	var ranges []sourceRange
	for _, decl := range file.Decls {
		function, ok := decl.(*ast.FuncDecl)
		if !ok || function.Body == nil || function.Name.Name != name {
			continue
		}
		ranges = append(ranges, sourceRange{start: function.Body.Pos(), end: function.Body.End()})
	}
	return ranges
}

func insideAnyRange(position token.Pos, ranges []sourceRange) bool {
	for _, candidate := range ranges {
		if position >= candidate.start && position < candidate.end {
			return true
		}
	}
	return false
}

func isNumericLiteral(literal *ast.BasicLit) bool {
	return literal.Kind == token.INT || literal.Kind == token.FLOAT
}

// isStructuralLiteral reports whether a literal is one of the two values that are
// structural rather than behavioural: 0 for zero checks and empty ranges, 1 for
// unit steps and complements.
func isStructuralLiteral(literal *ast.BasicLit) bool {
	return literal.Kind == token.INT && (literal.Value == "0" || literal.Value == "1")
}

// isEncodingConstant reports whether a literal is one of the two numbers
// strconv.FormatInt/FormatFloat need to encode the canonical map: base 10 and the
// 64-bit float size.
func isEncodingConstant(literal *ast.BasicLit) bool {
	return literal.Kind == token.INT && (literal.Value == "10" || literal.Value == "64")
}

// TestNoBehaviorNumbersOutsideTheParamsObject is a structural guard. It runs
// structuralViolations over every non-test source of the package, and then proves
// the guard detects violations using synthetic positive controls: a guard that
// cannot fail is not a guard.
func TestNoBehaviorNumbersOutsideTheParamsObject(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the package directory: %v", err)
	}

	parsed := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed++
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		violations, err := structuralViolations(name, src)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, violation := range violations {
			t.Error(violation)
		}
	}
	if parsed == 0 {
		t.Fatal("the structural guard parsed no non-test files, so it is silently passing")
	}

	// Positive control: a float threshold in a behaviour file must be caught.
	plantedFloat := "package classify\n\nfunc plantedBehaviour() float64 { return 0.5 }\n"
	violations, err := structuralViolations("academic_rule.go", []byte(plantedFloat))
	if err != nil {
		t.Fatalf("parse the planted float source: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("the guard found %d violations in the planted float source, want exactly 1: %v", len(violations), violations)
	}

	// Positive control: integer thresholds must be caught too, not only floats.
	plantedInt := "package classify\n\nfunc plantedInt() bool { return 4 > 3 }\n"
	violations, err = structuralViolations("academic_rule.go", []byte(plantedInt))
	if err != nil {
		t.Fatalf("parse the planted int source: %v", err)
	}
	if len(violations) != 2 {
		t.Fatalf("the guard found %d violations in the planted int source, want 2 (the thresholds 4 and 3): %v", len(violations), violations)
	}

	// Positive control: the demonstrated attack. A comparison against 1 can move
	// a level while introducing no parameter, so it must be caught.
	plantedComparison := "package classify\n\nfunc plantedComparison(value int) bool {\n\tif value == 1 {\n\t\treturn true\n\t}\n\treturn false\n}\n"
	violations, err = structuralViolations("academic_rule.go", []byte(plantedComparison))
	if err != nil {
		t.Fatalf("parse the planted comparison source: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("the guard found %d violations in the planted `value == 1` source, want exactly 1: %v", len(violations), violations)
	}

	// Positive control: arithmetic with 1 is behaviour too and must be caught.
	plantedArithmetic := "package classify\n\nfunc plantedArithmetic(score float64) float64 { return score + 1 }\n"
	violations, err = structuralViolations("academic_rule.go", []byte(plantedArithmetic))
	if err != nil {
		t.Fatalf("parse the planted arithmetic source: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("the guard found %d violations in the planted `score + 1` source, want exactly 1: %v", len(violations), violations)
	}

	// The falling exemption is narrow: the same kind of subtraction anywhere else
	// is still a violation.
	plantedOutsideFalling := "package classify\n\nfunc elsewhere(score float64) float64 { return 1 - score }\n"
	violations, err = structuralViolations("academic_rule.go", []byte(plantedOutsideFalling))
	if err != nil {
		t.Fatalf("parse the planted subtraction source: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("the guard found %d violations outside falling, want exactly 1: %v", len(violations), violations)
	}

	// Positive control: a DefaultAcademicRuleV1Params body with any extra
	// statement is a place to hide behaviour and must be caught even though it
	// carries no literal.
	plantedBody := "package classify\n\nfunc DefaultAcademicRuleV1Params() Params {\n\tparams := Params{}\n\treturn params\n}\n"
	violations, err = structuralViolations("params.go", []byte(plantedBody))
	if err != nil {
		t.Fatalf("parse the planted defaults body: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("the guard found %d violations in a planted defaults body with an extra statement, want exactly 1: %v", len(violations), violations)
	}

	// Positive control: a nested const inside that body hides a number just as
	// well, so it is caught both as a body violation and as a stray literal.
	plantedHiddenBody := "package classify\n\nfunc DefaultAcademicRuleV1Params() Params {\n\tconst hidden = 21600\n\treturn Params{}\n}\n"
	violations, err = structuralViolations("params.go", []byte(plantedHiddenBody))
	if err != nil {
		t.Fatalf("parse the planted hidden defaults source: %v", err)
	}
	if len(violations) != 2 {
		t.Fatalf("the guard found %d violations for a nested const in the defaults body, want 2 (the body shape and the stray 21600): %v", len(violations), violations)
	}

	// Over-blocking control: the one file allowed to spell behaviour numbers
	// still may, as long as the body is the single return of one composite lit.
	allowedDefaults := "package classify\n\nfunc DefaultAcademicRuleV1Params() Params {\n\treturn Params{MaxStalenessSeconds: 21600, WeightWind: 0.20}\n}\n"
	violations, err = structuralViolations("params.go", []byte(allowedDefaults))
	if err != nil {
		t.Fatalf("parse the allowed defaults source: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("the guard rejected the single-return composite defaults body: %v", violations)
	}

	// The params.go allowance is scoped: the encoding constants are accepted only
	// inside the function that encodes the canonical map.
	allowedEncoding := "package classify\n\nfunc deriveParameters() int { return 64 }\n"
	violations, err = structuralViolations("params.go", []byte(allowedEncoding))
	if err != nil {
		t.Fatalf("parse the allowed encoding source: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("the guard rejected 64 inside deriveParameters: %v", violations)
	}
	strayEncoding := "package classify\n\nfunc other() int { return 64 }\n"
	violations, err = structuralViolations("params.go", []byte(strayEncoding))
	if err != nil {
		t.Fatalf("parse the stray encoding source: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("the guard found %d violations for 64 outside the encoding function, want exactly 1: %v", len(violations), violations)
	}
}
