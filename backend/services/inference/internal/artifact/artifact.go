// Package artifact parses and validates the versioned model artifact that the
// inference service executes.
//
// The artifact is data, not code: Go reads a JSON document that declares its own
// feature order, its own preprocessing and its own parameters, and refuses to
// start when that document does not satisfy the contract below. Nothing here
// trains, fits or repairs a model, and nothing here falls back to a default
// model: a document that needs repair is a document that must be rebuilt
// upstream, because a service that silently substitutes zeros answers every
// request with a plausible-looking number nobody trained.
//
// There is no onnxruntime and no cgo. The four other services build with
// CGO_ENABLED=0 on gcr.io/distroless/static-debian12:nonroot, and a JSON
// document interpreted by hand keeps that property (S2).
package artifact

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"time"
)

// Format is the only artifact format this service executes. The value is checked
// literally at startup: a document written for a different reader is rejected
// instead of being interpreted with the assumptions of this one.
const Format = "firewatch-model/v1"

// The two model kinds the format defines.
const (
	KindLogistic = "logistic"
	KindForest   = "forest"
)

// The documented default level thresholds, used only when the artifact does not
// declare its own. They are fixed constants in code rather than tuned values: a
// deployment that forgets to freeze its thresholds gets an answer that is stated
// here, in the contract, and identical in every replica.
const (
	DefaultMediumThreshold = 0.40
	DefaultHighThreshold   = 0.70
)

// Level is the canonical risk level vocabulary the inference surface returns. It
// matches the levels risk persists, so the caller does not translate.
type Level string

const (
	LevelLow    Level = "low"
	LevelMedium Level = "medium"
	LevelHigh   Level = "high"
)

// Feature is one model input, in the model's input order. The order of the
// Features slice is the input order: every other array in the artifact
// (impute, center, scale, coef) is indexed by that position, not by name.
//
// Min and Max are validation guards, not decoration. The real source mixes units
// (radiation_sol reaches 1758 W/m2, pressure_atm drops to 593 hPa with no unit
// declared), so the guard is what keeps a value expressed in the wrong unit from
// being scored as if it were in the right one. A request outside the guard is
// rejected; it is never clamped.
type Feature struct {
	Name string  `json:"name"`
	Unit string  `json:"unit"`
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
}

// Thresholds are the level cut points. A probability below Medium is low, below
// High is medium, and anything else is high.
type Thresholds struct {
	Medium float64 `json:"medium"`
	High   float64 `json:"high"`
}

// Node is one node of one tree. A node with Value set is a leaf and must carry
// no split fields; every other node must carry Feature, Threshold, Left and
// Right. Left and Right are indexes into the same tree's Nodes slice, and
// Feature is an index into the artifact's Features slice, so a tree can only be
// walked in the space the model was trained in.
type Node struct {
	Feature   *int     `json:"feature,omitempty"`
	Threshold *float64 `json:"threshold,omitempty"`
	Left      *int     `json:"left,omitempty"`
	Right     *int     `json:"right,omitempty"`
	Value     *float64 `json:"value,omitempty"`
}

// Tree is one decision tree of a forest.
type Tree struct {
	Nodes []Node `json:"nodes"`
}

// Timestamp is trained_at. It is a named type so a malformed instant fails with
// the field that is wrong instead of a bare time-parsing error, and so /health
// renders the instant in UTC regardless of the offset the artifact used.
type Timestamp struct{ time.Time }

// UnmarshalJSON reads one RFC3339 instant.
func (t *Timestamp) UnmarshalJSON(document []byte) error {
	var raw string
	if err := json.Unmarshal(document, &raw); err != nil {
		return errors.New("trained_at must be an RFC3339 string")
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return fmt.Errorf("trained_at: %w", err)
	}
	t.Time = parsed.UTC()
	return nil
}

// MarshalJSON renders the instant in UTC, so every reader sees one form.
func (t Timestamp) MarshalJSON() ([]byte, error) { return json.Marshal(t.UTC()) }

// Artifact is the parsed document. It is read-only once parsed: the service
// loads exactly one artifact at startup and never mutates it, so a second
// request can never observe a half-loaded model.
type Artifact struct {
	Format    string    `json:"format"`
	Version   string    `json:"version"`
	TrainedAt Timestamp `json:"trained_at"`
	DataHash  string    `json:"data_hash"`

	// Features is the model's input order.
	Features []Feature `json:"features"`

	// Impute, Center and Scale are indexed by feature position. The
	// preprocessing order per feature is: NaN -> Impute[i], then
	// (x - Center[i]) / Scale[i], then the model.
	Impute []float64 `json:"impute"`
	Center []float64 `json:"center"`
	Scale  []float64 `json:"scale"`

	Kind      string    `json:"kind"`
	Intercept *float64  `json:"intercept,omitempty"`
	Coef      []float64 `json:"coef,omitempty"`
	Trees     []Tree    `json:"trees,omitempty"`

	// Metrics and Baseline are opaque provenance objects. They are carried so
	// the numbers a model was chosen on stay attached to the parameters that
	// produce it, and they are exposed by /v1/model. They are never read while
	// scoring: a metrics key cannot change an answer.
	Metrics  json.RawMessage `json:"metrics,omitempty"`
	Baseline json.RawMessage `json:"baseline,omitempty"`

	// Thresholds is optional. When absent the documented defaults apply.
	Thresholds *Thresholds `json:"thresholds,omitempty"`
}

// Load reads and validates an artifact from disk. Every failure is fatal for the
// caller: the service has no valid model without it.
func Load(path string) (*Artifact, error) {
	if path == "" {
		return nil, errors.New("artifact path is empty")
	}
	document, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read model artifact %q: %w", path, err)
	}
	parsed, err := Parse(document)
	if err != nil {
		return nil, fmt.Errorf("model artifact %q: %w", path, err)
	}
	return parsed, nil
}

// Parse decodes and validates one artifact document. Unknown fields are an
// error: a misspelled field would otherwise be silently dropped and the model
// would run with a default nobody declared.
func Parse(document []byte) (*Artifact, error) {
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.DisallowUnknownFields()
	var parsed Artifact
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode artifact: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return nil, err
	}
	if err := parsed.Validate(); err != nil {
		return nil, err
	}
	return &parsed, nil
}

// ensureEOF rejects trailing content, so a truncated file concatenated with
// anything else cannot be half-read as an artifact.
func ensureEOF(decoder *json.Decoder) error {
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("artifact contains more than one JSON document")
	}
	return nil
}

// Validate enforces the whole contract. It is exported so a caller may check a
// document it built itself before serving it, and so every violation has one
// name and one message.
func (a *Artifact) Validate() error {
	if err := a.validateHeader(); err != nil {
		return err
	}
	if err := a.validateFeatures(); err != nil {
		return err
	}
	if err := a.validatePreprocessing(); err != nil {
		return err
	}
	if err := a.validateThresholds(); err != nil {
		return err
	}
	if err := a.validateProvenance(); err != nil {
		return err
	}
	return a.validateModel()
}

func (a *Artifact) validateHeader() error {
	if a.Format != Format {
		return fmt.Errorf("format %q is not %q", a.Format, Format)
	}
	if a.Version == "" {
		return errors.New("version is required")
	}
	if a.TrainedAt.IsZero() {
		return errors.New("trained_at is required")
	}
	if a.DataHash == "" {
		return errors.New("data_hash is required")
	}
	return nil
}

func (a *Artifact) validateFeatures() error {
	if len(a.Features) == 0 {
		return errors.New("features is empty: the model declares no input")
	}
	declared := make(map[string]bool, len(a.Features))
	for i, feature := range a.Features {
		if feature.Name == "" {
			return fmt.Errorf("features[%d] has no name", i)
		}
		if _, duplicate := declared[feature.Name]; duplicate {
			return fmt.Errorf("feature %q is declared twice", feature.Name)
		}
		declared[feature.Name] = true
		if feature.Unit == "" {
			return fmt.Errorf("feature %q has no unit: a guard without a unit cannot be checked", feature.Name)
		}
		if !finite(feature.Min) || !finite(feature.Max) {
			return fmt.Errorf("feature %q has a non-finite min/max", feature.Name)
		}
		if feature.Min >= feature.Max {
			return fmt.Errorf("feature %q has min %v and max %v: min must be below max", feature.Name, feature.Min, feature.Max)
		}
	}
	return nil
}

func (a *Artifact) validatePreprocessing() error {
	width := len(a.Features)
	for _, array := range []struct {
		name   string
		values []float64
	}{{"impute", a.Impute}, {"center", a.Center}, {"scale", a.Scale}} {
		if len(array.values) != width {
			return fmt.Errorf("%s has %d values for %d features: preprocessing must be positional and complete", array.name, len(array.values), width)
		}
		for i, value := range array.values {
			if !finite(value) {
				return fmt.Errorf("%s[%d] (%s) is not finite", array.name, i, a.Features[i].Name)
			}
		}
	}
	for i, scale := range a.Scale {
		if scale <= 0 {
			return fmt.Errorf("scale[%d] (%s) is %v: a scale must be positive", i, a.Features[i].Name, scale)
		}
	}
	// The imputed value stands in for a missing observation, so the guard that
	// governs every supplied value governs it too. A substitution outside the
	// declared range would score a reading the data never supports.
	for i, value := range a.Impute {
		feature := a.Features[i]
		if value < feature.Min || value > feature.Max {
			return fmt.Errorf("impute[%d] (%s) is %v, outside [%v, %v]", i, feature.Name, value, feature.Min, feature.Max)
		}
	}
	return nil
}

func (a *Artifact) validateThresholds() error {
	if a.Thresholds == nil {
		return nil
	}
	medium, high := a.Thresholds.Medium, a.Thresholds.High
	if !finite(medium) || !finite(high) {
		return errors.New("thresholds must be finite")
	}
	if !(0 < medium && medium < high && high < 1) {
		return fmt.Errorf("thresholds medium %v and high %v must satisfy 0 < medium < high < 1", medium, high)
	}
	return nil
}

func (a *Artifact) validateProvenance() error {
	for name, raw := range map[string]json.RawMessage{"metrics": a.Metrics, "baseline": a.Baseline} {
		if len(raw) == 0 {
			continue
		}
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			return fmt.Errorf("%s must be a JSON object", name)
		}
	}
	return nil
}

func (a *Artifact) validateModel() error {
	width := len(a.Features)
	switch a.Kind {
	case KindLogistic:
		if len(a.Trees) != 0 {
			return errors.New("kind logistic carries trees: one artifact must describe one model")
		}
		if a.Intercept == nil {
			return errors.New("kind logistic requires intercept")
		}
		if !finite(*a.Intercept) {
			return errors.New("intercept is not finite")
		}
		if len(a.Coef) != width {
			return fmt.Errorf("coef has %d values for %d features", len(a.Coef), width)
		}
		for i, coefficient := range a.Coef {
			if !finite(coefficient) {
				return fmt.Errorf("coef[%d] (%s) is not finite", i, a.Features[i].Name)
			}
		}
		return nil
	case KindForest:
		if a.Intercept != nil || len(a.Coef) != 0 {
			return errors.New("kind forest carries intercept or coef: one artifact must describe one model")
		}
		if len(a.Trees) == 0 {
			return errors.New("kind forest requires at least one tree")
		}
		for i, tree := range a.Trees {
			if err := validateTree(i, tree, width); err != nil {
				return err
			}
		}
		return nil
	case "":
		return errors.New("kind is required")
	default:
		return fmt.Errorf("kind %q is neither %q nor %q", a.Kind, KindLogistic, KindForest)
	}
}

// validateTree checks one tree's structure and then proves it can be walked: the
// root is node 0, every split index is in bounds, no node is reachable twice,
// and no node is unreachable. A cycle would otherwise be an infinite loop at
// scoring time, which is a hang, not an error a caller can see.
func validateTree(treeIndex int, tree Tree, featureCount int) error {
	const (
		unvisited = 0
		onPath    = 1
		done      = 2
	)
	nodes := tree.Nodes
	if len(nodes) == 0 {
		return fmt.Errorf("trees[%d] has no nodes", treeIndex)
	}
	for i, node := range nodes {
		where := fmt.Sprintf("trees[%d].nodes[%d]", treeIndex, i)
		if node.Value != nil {
			if !finite(*node.Value) || *node.Value < 0 || *node.Value > 1 {
				return fmt.Errorf("%s leaf value %v is not a probability in [0, 1]", where, *node.Value)
			}
			if node.Feature != nil || node.Threshold != nil || node.Left != nil || node.Right != nil {
				return fmt.Errorf("%s is a leaf and also carries split fields", where)
			}
			continue
		}
		if node.Feature == nil || node.Threshold == nil || node.Left == nil || node.Right == nil {
			return fmt.Errorf("%s is not a leaf and is missing feature, threshold, left or right", where)
		}
		if *node.Feature < 0 || *node.Feature >= featureCount {
			return fmt.Errorf("%s feature index %d is outside [0, %d)", where, *node.Feature, featureCount)
		}
		if !finite(*node.Threshold) {
			return fmt.Errorf("%s threshold is not finite", where)
		}
		for _, child := range []struct {
			name  string
			index int
		}{{"left", *node.Left}, {"right", *node.Right}} {
			if child.index < 0 || child.index >= len(nodes) {
				return fmt.Errorf("%s %s index %d is outside [0, %d)", where, child.name, child.index, len(nodes))
			}
			if child.index == i {
				return fmt.Errorf("%s %s points at itself", where, child.name)
			}
		}
	}
	state := make([]int, len(nodes))
	var walk func(index, depth int) error
	walk = func(index, depth int) error {
		if depth > len(nodes) {
			return fmt.Errorf("trees[%d] is deeper than its %d nodes: the tree does not terminate", treeIndex, len(nodes))
		}
		switch state[index] {
		case onPath:
			return fmt.Errorf("trees[%d].nodes[%d] is reached twice on one path: the tree has a cycle", treeIndex, index)
		case done:
			return nil
		}
		state[index] = onPath
		node := nodes[index]
		if node.Value == nil {
			if err := walk(*node.Left, depth+1); err != nil {
				return err
			}
			if err := walk(*node.Right, depth+1); err != nil {
				return err
			}
		}
		state[index] = done
		return nil
	}
	if err := walk(0, 0); err != nil {
		return err
	}
	for i, s := range state {
		if s == unvisited {
			return fmt.Errorf("trees[%d].nodes[%d] is unreachable from the root", treeIndex, i)
		}
	}
	return nil
}

// FeatureNames returns the model's input order.
func (a *Artifact) FeatureNames() []string {
	names := make([]string, len(a.Features))
	for i, feature := range a.Features {
		names[i] = feature.Name
	}
	return names
}

// Index reports the position of a feature in the model's input order. The scan
// is linear on purpose: the feature list is a handful of entries, and a cached
// map would only be correct on a document that had already been validated - the
// one call a caller is most likely to make before validating.
func (a *Artifact) Index(name string) (int, bool) {
	for i, feature := range a.Features {
		if feature.Name == name {
			return i, true
		}
	}
	return 0, false
}

// LevelThresholds returns the cut points in force: the artifact's own when it
// declares them, the documented defaults otherwise.
func (a *Artifact) LevelThresholds() (medium, high float64) {
	if a.Thresholds != nil {
		return a.Thresholds.Medium, a.Thresholds.High
	}
	return DefaultMediumThreshold, DefaultHighThreshold
}

// Level maps one probability to the canonical level vocabulary.
func (a *Artifact) Level(probability float64) Level {
	medium, high := a.LevelThresholds()
	switch {
	case probability < medium:
		return LevelLow
	case probability < high:
		return LevelMedium
	default:
		return LevelHigh
	}
}

// finite reports whether v is a real number. NaN and both infinities are not:
// every one of them is a defect a caller must see, not a value to score with.
func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
