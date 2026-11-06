// Package scorer executes a validated artifact over one sample.
//
// The order of operations is part of the model, not an implementation detail:
// for every feature, a NaN becomes Impute[i], then the value is standardized as
// (x - Center[i]) / Scale[i], and only then is the model evaluated. Standardize
// first and the imputation lands in a different place; impute after centering
// and the missing reading is centered twice. The tests that pin this order are
// the reason the two steps are separate functions rather than one expression.
//
// The request surface is key-addressed ({"temp_max": 31.5, ...}) while the model
// is positional. The conversion between the two happens here and only here, in
// the artifact's declared order, so two requests that differ only in JSON key
// order score identically.
package scorer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"firewatch.local/inference/internal/artifact"
)

// The stable machine-readable codes the request surface returns. They are part
// of the contract: a caller matches on the code, never on the message.
const (
	// CodeInvalidBody means the body is not the documented object shape.
	CodeInvalidBody = "invalid_body"
	// CodeDuplicateFeature means one feature name was declared twice. It is
	// rejected instead of taking the last value, because "last one wins" turns a
	// client bug into a silently different prediction.
	CodeDuplicateFeature = "duplicate_feature"
	// CodeMissingFeature means a feature the model needs was not declared.
	CodeMissingFeature = "missing_feature"
	// CodeUnknownFeature means the body named a feature the model does not take.
	CodeUnknownFeature = "unknown_feature"
	// CodeOutOfRange means a value fell outside the feature's declared guard.
	// The value is rejected, never clamped: clamping would answer confidently
	// about a reading the model was never trained on.
	CodeOutOfRange = "out_of_range"
)

// ErrNonFiniteProbability is returned when the arithmetic produces a probability
// that is not a finite number in [0, 1]: a NaN or an infinity from an
// overflowing sum, or a standardized value that overflowed on the way in. It is
// never a response: a non-finite probability serialized as JSON null would be
// read downstream as "no risk" by anything that tests for a falsy value, which
// is the opposite of what an overflowing score means.
var ErrNonFiniteProbability = errors.New("model produced a probability that is not a finite number in [0, 1]")

// InvalidSampleError is a refused request: the code is stable and the detail is
// for a human reading logs. It is deliberately not wrapped into a 500: nothing
// here is an internal failure, and calling it one would hide a client bug.
type InvalidSampleError struct {
	Code   string
	Detail string
}

func (e *InvalidSampleError) Error() string { return e.Detail }

// FeatureValue is one declared feature of one sample.
type FeatureValue struct {
	Name  string
	Value float64
}

// Sample is a prediction input as the caller declared it: the features in body
// order, with duplicates preserved. Duplicates are preserved on purpose - a
// decoded map would have collapsed them before the check that rejects them.
//
// A JSON null declares the feature but not its value: it becomes NaN and is
// imputed from the artifact. The key being present is what makes it a declared
// observation rather than a missing one; an absent key is still an error.
type Sample []FeatureValue

// UnmarshalJSON reads the documented object shape. It walks tokens instead of
// decoding into a map so the declared order and any repeated key survive long
// enough to be validated.
func (s *Sample) UnmarshalJSON(document []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	opening, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("features is not a JSON object: %w", err)
	}
	if delimiter, ok := opening.(json.Delim); !ok || delimiter != '{' {
		return fmt.Errorf("features must be a JSON object of name to value, got %v", opening)
	}
	parsed := make(Sample, 0, 8)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		name, ok := token.(string)
		if !ok {
			return fmt.Errorf("a feature name must be a string, got %v", token)
		}
		value, err := decoder.Token()
		if err != nil {
			return err
		}
		var number float64
		switch literal := value.(type) {
		case json.Number:
			number, err = literal.Float64()
			if err != nil {
				return fmt.Errorf("feature %q is not a number: %w", name, err)
			}
		case nil:
			number = math.NaN()
		default:
			return fmt.Errorf("feature %q must be a number or null, got %v", name, value)
		}
		parsed = append(parsed, FeatureValue{Name: name, Value: number})
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("features is not a closed JSON object: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("features carries trailing content")
	}
	*s = parsed
	return nil
}

// Scorer is one loaded artifact with its name-to-position index precomputed.
// It is immutable after New, so it is safe for concurrent use.
type Scorer struct {
	artifact *artifact.Artifact
	index    map[string]int
}

// New builds a scorer over a validated artifact.
func New(a *artifact.Artifact) *Scorer {
	index := make(map[string]int, len(a.Features))
	for i, feature := range a.Features {
		index[feature.Name] = i
	}
	return &Scorer{artifact: a, index: index}
}

// Artifact returns the artifact this scorer executes.
func (s *Scorer) Artifact() *artifact.Artifact { return s.artifact }

// Result is one answer. The level is deliberately not here: the artifact owns
// the cut points and the request surface applies them, so a caller can never see
// a level that disagrees with the thresholds the service reports as in force.
type Result struct {
	Probability float64
	Version     string
}

// Score validates one sample and answers it.
func (s *Scorer) Score(sample Sample) (Result, error) {
	standardized, err := s.standardize(sample)
	if err != nil {
		return Result{}, err
	}
	probability, err := s.probability(standardized)
	if err != nil {
		return Result{}, err
	}
	return Result{Probability: probability, Version: s.artifact.Version}, nil
}

// standardize is the request-validation boundary and the preprocessing pipeline.
// It rejects before it computes, so an invalid sample never reaches the model:
//
//  1. a feature name declared twice is a client bug, not a tie to break;
//  2. a feature the model needs but the body omits is missing;
//  3. a name the model does not take is unknown;
//  4. a value outside the feature's declared guard is out of range.
//
// The order matters for the message a caller gets, not for the decision: a
// six-key body reports what is missing, and a body carrying every required
// feature plus one extra reports the extra name. A body with a misspelled name
// therefore reports the misspelling as missing (the expected name is absent)
// rather than as unknown, which is the actionable half of the pair.
func (s *Scorer) standardize(sample Sample) ([]float64, error) {
	width := len(s.artifact.Features)
	seen := make(map[string]bool, len(sample))
	for _, declared := range sample {
		if seen[declared.Name] {
			return nil, &InvalidSampleError{
				Code:   CodeDuplicateFeature,
				Detail: fmt.Sprintf("feature %q is declared more than once", declared.Name),
			}
		}
		seen[declared.Name] = true
	}
	values := make([]float64, width)
	present := make([]bool, width)
	var unknown []string
	for _, declared := range sample {
		position, known := s.index[declared.Name]
		if !known {
			unknown = append(unknown, declared.Name)
			continue
		}
		values[position] = declared.Value
		present[position] = true
		if math.IsNaN(declared.Value) {
			// Declared but unobserved: imputation below, not a range violation.
			continue
		}
		feature := s.artifact.Features[position]
		if declared.Value < feature.Min || declared.Value > feature.Max {
			return nil, &InvalidSampleError{
				Code: CodeOutOfRange,
				Detail: fmt.Sprintf("feature %q value %v is outside [%v, %v] %s",
					feature.Name, declared.Value, feature.Min, feature.Max, feature.Unit),
			}
		}
	}
	var missing []string
	for i, feature := range s.artifact.Features {
		if !present[i] {
			missing = append(missing, feature.Name)
		}
	}
	if len(missing) > 0 {
		return nil, &InvalidSampleError{
			Code: CodeMissingFeature,
			Detail: fmt.Sprintf("the model takes %d features and the request declared %d: missing %s",
				width, len(sample), strings.Join(missing, ", ")),
		}
	}
	if len(unknown) > 0 {
		return nil, &InvalidSampleError{
			Code:   CodeUnknownFeature,
			Detail: fmt.Sprintf("the model takes no feature named %s", strings.Join(unknown, ", ")),
		}
	}
	standardized := make([]float64, width)
	for i, value := range values {
		if math.IsNaN(value) {
			value = s.artifact.Impute[i]
		}
		value = (value - s.artifact.Center[i]) / s.artifact.Scale[i]
		if !finite(value) {
			return nil, fmt.Errorf("%w: feature %q standardized to %v", ErrNonFiniteProbability, s.artifact.Features[i].Name, value)
		}
		standardized[i] = value
	}
	return standardized, nil
}

// probability evaluates the model over standardized values.
func (s *Scorer) probability(values []float64) (float64, error) {
	var probability float64
	switch s.artifact.Kind {
	case artifact.KindLogistic:
		z := *s.artifact.Intercept
		for i, coefficient := range s.artifact.Coef {
			z += coefficient * values[i]
		}
		probability = 1 / (1 + math.Exp(-z))
	case artifact.KindForest:
		var total float64
		for i, tree := range s.artifact.Trees {
			leaf, err := walk(tree, values)
			if err != nil {
				return 0, fmt.Errorf("trees[%d]: %w", i, err)
			}
			total += leaf
		}
		// The forest answers with the arithmetic mean of its trees' leaf
		// probabilities. It is documented here because it is a modelling choice,
		// not an arithmetic accident: averaging in logit space would give a
		// different number for the same trees.
		probability = total / float64(len(s.artifact.Trees))
	default:
		return 0, fmt.Errorf("kind %q cannot be scored", s.artifact.Kind)
	}
	if !finite(probability) || probability < 0 || probability > 1 {
		return 0, fmt.Errorf("%w: got %v", ErrNonFiniteProbability, probability)
	}
	return probability, nil
}

// walk follows one tree from its root to its first leaf. The step bound is the
// node count: a tree is validated at load, but a scorer built by hand must not be
// able to hang a request on a cycle.
func walk(tree artifact.Tree, values []float64) (float64, error) {
	index := 0
	for steps := 0; ; steps++ {
		if steps > len(tree.Nodes) {
			return 0, errors.New("tree walk did not reach a leaf")
		}
		node := tree.Nodes[index]
		if node.Value != nil {
			return *node.Value, nil
		}
		if values[*node.Feature] <= *node.Threshold {
			index = *node.Left
		} else {
			index = *node.Right
		}
	}
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
