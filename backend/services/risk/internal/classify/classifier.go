// Package classify defines the classification contract and its implementations.
//
// Classification is deliberately a separate contract from data acquisition. A
// provider answers "what data exists"; a classifier answers "what it means".
// They change for different reasons - a new source versus a new rule or model -
// and they carry different evidence, provenance against validation metrics.
// Keeping them apart is what allows an academic rule and a trained model to be
// interchangeable without touching the ingestion side.
package classify

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"time"
)

// Level is the risk level produced by a classifier.
//
// LevelDesconocido is a first-class value, not an absence of one. It records
// that the inputs were insufficient to decide. The contract deliberately
// separates it from LevelBajo: an absent or stale input must never be published
// as low risk, because to a reader "bajo" and "no sabemos" are opposite
// operational facts.
type Level string

const (
	LevelBajo        Level = "bajo"
	LevelMedio       Level = "medio"
	LevelAlto        Level = "alto"
	LevelDesconocido Level = "desconocido"
)

// Levels returns the canonical levels in ascending severity. LevelDesconocido is
// listed last and is not a severity: it means undetermined.
func Levels() []Level {
	return []Level{LevelBajo, LevelMedio, LevelAlto, LevelDesconocido}
}

// Canonical reports whether the level is one of the four defined values.
func (l Level) Canonical() bool {
	for _, candidate := range Levels() {
		if l == candidate {
			return true
		}
	}
	return false
}

// Kind distinguishes a documented rule from a trained model. Both satisfy the
// same interface on purpose, so the fallback is a configuration change rather
// than a refactor.
type Kind string

const (
	KindRule    Kind = "rule"
	KindTrained Kind = "trained"
)

// Feature is one input value that may be absent. Absence is modelled explicitly
// rather than as a zero value, because a missing humidity reading and a humidity
// reading of zero are entirely different facts.
type Feature struct {
	Present bool
	Value   float64
}

// Known returns a present feature.
func Known(value float64) Feature { return Feature{Present: true, Value: value} }

// Unknown returns an absent feature.
func Unknown() Feature { return Feature{} }

// Features is the classification input contract.
//
// Both instants are supplied by the caller and the rule never reads the system
// clock. That is what makes a classification reproducible: the same Features
// always produce the same Assessment, and a test can pin both instants.
//
//   - ObservedAt is the instant the data describes.
//   - EvaluatedAt is the instant the assessment is made, used for staleness.
type Features struct {
	ObservedAt  time.Time
	EvaluatedAt time.Time
	ZoneID      string

	TemperatureC        Feature
	RelativeHumidityPct Feature
	WindSpeedKmh        Feature
	VegetationCoverPct  Feature
	HistoricalFires     Feature
}

// named pairs a canonical variable name with its feature, so missing inputs can
// be reported by name without a second, drifting list.
func (f Features) named() []struct {
	Name  string
	Value Feature
} {
	return []struct {
		Name  string
		Value Feature
	}{
		{"temperatura", f.TemperatureC},
		{"humedad_relativa", f.RelativeHumidityPct},
		{"viento", f.WindSpeedKmh},
		{"cobertura_vegetal", f.VegetationCoverPct},
		{"incendios_historicos", f.HistoricalFires},
	}
}

// Missing returns the names of the absent inputs, in a stable order.
func (f Features) Missing() []string {
	var missing []string
	for _, entry := range f.named() {
		if !entry.Value.Present {
			missing = append(missing, entry.Name)
		}
	}
	return missing
}

// Factor is one input of an assessment, with what it contributed.
//
// Factors are part of the graded deliverable: the contest requires a dashboard
// with indicators and factors. Returning them from the classifier means a rule
// and a trained model satisfy that requirement identically, and it forces the
// rule to be explainable by construction rather than by afterthought.
//
// Factors are persisted as JSON in the batches.factors column and served by the
// REST API, so the fields carry snake_case json tags like every other payload in
// this service.
type Factor struct {
	Name         string  `json:"name"`
	Known        bool    `json:"known"`
	Value        float64 `json:"value"`
	Contribution float64 `json:"contribution"`
	Note         string  `json:"note"`
}

// PolicyRef records the exact policy that produced an assessment: the policy
// identifier plus the effective parameter set and its fingerprint.
//
// It exists so a stored level can be reproduced. The identifier alone is a name,
// but the parameters and their hash record which numbers were in effect, and
// carrying them on every assessment means the audit trail travels with the
// answer instead of being looked up later in code that may have changed.
//
// What that does NOT guarantee: that the recorded numbers are the only thing able
// to move a level. A literal outside the Params object, or an operator change
// that uses no literal, can still change the behaviour behind an unchanged hash,
// which is why the behaviour is additionally bound to a frozen golden table.
type PolicyRef struct {
	PolicyID   string            `json:"policy_id"`
	Parameters map[string]string `json:"parameters"`
	ParamsHash string            `json:"params_hash"`
}

// PolicyRefFor builds the policy reference for this parameter set under the given
// policy identifier.
func (p Params) PolicyRefFor(policyID string) (PolicyRef, error) {
	parameters, err := p.Parameters()
	if err != nil {
		return PolicyRef{}, err
	}
	hash, err := p.Hash()
	if err != nil {
		return PolicyRef{}, err
	}
	return PolicyRef{PolicyID: policyID, Parameters: parameters, ParamsHash: hash}, nil
}

// Assessment is the classifier's answer.
type Assessment struct {
	Level     Level
	Factors   []Factor
	Rationale string
	Policy    PolicyRef
}

// Validate checks the assessment is publishable. It refuses an undefined level,
// which is the one mistake that would silently turn an undetermined input into
// a stored value, and it refuses an assessment with no policy provenance, which
// would make a stored level unreproducible.
func (a Assessment) Validate() error {
	if !a.Level.Canonical() {
		return fmt.Errorf("non-canonical risk level %q", a.Level)
	}
	if a.Policy.PolicyID == "" {
		return errors.New("assessment has no policy id")
	}
	if a.Policy.ParamsHash == "" {
		return errors.New("assessment has no params hash")
	}
	return nil
}

// Classifier is implemented once per classification method: a documented
// academic rule, or a trained model.
type Classifier interface {
	// ID is the stable identifier recorded next to every assessment, so a level
	// can be attributed and later reproduced.
	ID() string

	// Kind distinguishes a rule from a trained model.
	Kind() Kind

	// Classify returns the level with its factors and a human-readable
	// rationale. Implementations must not read the system clock.
	Classify(ctx context.Context, features Features) (Assessment, error)
}

// FrozenPolicyFingerprints returns the identifiers whose policy fingerprint is
// frozen, each mapped to the exact fingerprint it must report.
//
// It exists because an identifier and a behaviour must not drift apart:
// "academic-rule-v1" is a promise about the numbers behind it. The map is the
// in-code statement of that promise, checked at registration and again before an
// assessment is persisted. An identifier that is not listed - a future trained
// model - is not constrained by it.
//
// What it does NOT guarantee: that a frozen behaviour is unreachable from outside
// the Params object. It freezes the parameters and their fingerprint; it cannot
// see an operator change that introduces no literal.
func FrozenPolicyFingerprints() map[string]string {
	return maps.Clone(frozenPolicyFingerprints)
}

// frozenPolicyFingerprints is the authoritative expectation behind
// FrozenPolicyFingerprints. It reuses AcademicRuleV1ParamsHash, the same constant
// the frozen-params test pins, so the identity check and the parameter guard
// cannot disagree. The accessor returns a copy, so no caller can mutate it.
var frozenPolicyFingerprints = map[string]string{
	AcademicRuleV1ID: AcademicRuleV1ParamsHash,
}

// ExpectedPolicyHash reports the frozen policy fingerprint for a classifier
// identifier, and whether the identifier is one whose fingerprint is frozen.
//
// It is the startup path's read of the same frozen expectation Register enforces,
// so a classifier version registered against a database row cannot disagree with
// the code that will run. An identifier that is not listed is not constrained:
// only the frozen ones carry a promise about their numbers.
func ExpectedPolicyHash(id string) (string, bool) {
	hash, frozen := frozenPolicyFingerprints[id]
	return hash, frozen
}

// VerifyRegisteredModel checks the registry record of the classifier version the
// service selected against the frozen policy expectation.
//
// registered reports whether the version has a row in the models table (the
// registry lives in data, so this is an input rather than something this function
// can read), and storedHash is the params_hash that row records. The function is
// deliberately pure so the startup check can be exercised without a database or a
// server.
//
// It fails closed on the two ways a selected version can be a lie:
//
//   - the version has no row at all. This is the scenario that used to start
//     healthy and then discard every job, because batches.model_version is a
//     foreign key into the registry.
//   - the version has a row but its stored fingerprint differs from the frozen
//     one, so the classifier would stamp a provenance the database contradicts.
//
// A version without a frozen expectation is accepted as long as it is registered:
// a future trained model carries metrics, not a frozen rule fingerprint.
func VerifyRegisteredModel(id string, registered bool, storedHash string) error {
	if !registered {
		return fmt.Errorf("selected classifier version %q has no row in the models registry", id)
	}
	expected, frozen := ExpectedPolicyHash(id)
	if !frozen {
		return nil
	}
	if storedHash != expected {
		return fmt.Errorf("classifier %q records params_hash %q in the models registry, but the running classifier expects the frozen %q: refusing to start with a divergent policy", id, storedHash, expected)
	}
	return nil
}

// PolicyFingerprinter is the optional interface a classifier implements to
// report the policy fingerprint it stamps on its assessments. Register uses it to
// verify that a classifier claiming a frozen identifier really reports that
// identifier's frozen policy. A classifier under an identifier that is not frozen
// does not need to implement it.
type PolicyFingerprinter interface {
	// PolicyFingerprint returns the params hash this classifier records on every
	// assessment it produces, or an error when it cannot report one.
	PolicyFingerprint() (string, error)
}

// ErrUnknownClassifier is returned when an unregistered identifier is selected.
var ErrUnknownClassifier = errors.New("unknown classifier")

// Registry holds the known classifiers so the active one can be chosen by
// configuration rather than by code.
type Registry struct {
	classifiers map[string]Classifier
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{classifiers: map[string]Classifier{}}
}

// Register adds a classifier. Identifier collisions are refused because the
// identifier is written into the audit trail of every assessment it produces.
//
// When the identifier is one of the frozen ones, the classifier must also report
// that identifier's frozen policy fingerprint: an identifier and a behaviour are
// not allowed to impersonate each other. A classifier that cannot report a policy
// at all, or that reports a different one, is refused. An identifier that is not
// frozen - a future trained model - is accepted as is.
func (r *Registry) Register(classifier Classifier) error {
	if classifier == nil {
		return errors.New("nil classifier")
	}
	if classifier.ID() == "" {
		return errors.New("classifier id is empty")
	}
	if _, exists := r.classifiers[classifier.ID()]; exists {
		return fmt.Errorf("classifier %q is already registered", classifier.ID())
	}
	if expected, frozen := frozenPolicyFingerprints[classifier.ID()]; frozen {
		reporter, ok := classifier.(PolicyFingerprinter)
		if !ok {
			return fmt.Errorf("classifier %q claims the frozen identifier %q but cannot report its policy fingerprint", classifier.ID(), classifier.ID())
		}
		reported, err := reporter.PolicyFingerprint()
		if err != nil {
			return fmt.Errorf("classifier %q cannot report its frozen policy fingerprint: %w", classifier.ID(), err)
		}
		if reported != expected {
			return fmt.Errorf("classifier %q would report policy fingerprint %q, want the frozen %q: refusing to attach a frozen identifier to a different behaviour", classifier.ID(), reported, expected)
		}
	}
	r.classifiers[classifier.ID()] = classifier
	return nil
}

// Select returns the classifier registered under id.
func (r *Registry) Select(id string) (Classifier, error) {
	classifier, ok := r.classifiers[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownClassifier, id)
	}
	return classifier, nil
}

// IDs returns the registered identifiers in a stable order.
func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.classifiers))
	for id := range r.classifiers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
