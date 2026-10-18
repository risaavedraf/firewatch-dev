package service

import (
	"context"
	"errors"
	"fmt"

	"firewatch.local/risk/internal/classify"
	"firewatch.local/risk/internal/repository"
)

// ActiveModelReader reads the model the registry currently marks active. It is an
// interface so the resolution rules can be exercised without a database; the
// repository store is the production implementation.
type ActiveModelReader interface {
	// ActiveModel returns the active row, or repository.ErrNoActiveModel when the
	// registry holds none. Any other error is a storage failure.
	ActiveModel(ctx context.Context) (repository.ModelVersion, error)
}

// ClassifierLookup resolves a registered classifier by version identifier. It is
// satisfied by *classify.Registry, the same registry the startup path validates
// against, so no second registry is invented here.
type ClassifierLookup interface {
	Select(id string) (classify.Classifier, error)
}

// ClassifierSource resolves the classifier that must serve one publication.
//
// It is the seam between the registry (data) and the running classifiers (code),
// and the place where "which version judged this batch" is decided once per
// publication instead of once per process.
type ClassifierSource interface {
	ActiveClassifier(ctx context.Context) (classify.Classifier, error)
}

// ErrClassifierUnresolvable marks a resolution failure that no retry can fix: the
// registry names a version the running binary cannot execute, or it records an
// activation the service has no way to honour. The publication path turns it into
// a permanent failure so the job is dead-lettered instead of blocking the queue
// through the whole retry budget. A storage failure is NOT wrapped with it: the
// database can come back, so that error stays retryable.
var ErrClassifierUnresolvable = errors.New("active classifier cannot be resolved")

// RegistryClassifierSource resolves the classifier from the shared models
// registry on every publication.
//
// It is the fix for the defect this unit is about: the registry could record an
// activation (active, approved_at, metrics.activation) while the publication path
// kept the classifier selected at startup forever, so activating v2 changed the
// registry panel and nothing else. Resolution now reads shared storage, so an
// activation decides inference in every replica without a restart.
//
// The precedence is deliberate and documented in backend/docs/model-activation.md:
//
//  1. an explicitly activated version wins. "Explicitly activated" means the row
//     records the activation object ActivateModel writes; that version must be
//     registered in the runtime registry and, when its identifier is frozen, its
//     stored fingerprint must match the frozen one, or resolution fails closed.
//  2. otherwise the bootstrap classifier -- the version MODEL_ID selected and the
//     startup check verified -- serves the publication. A registry with no active
//     row and a row that only carries the migration's seed flag are both this
//     case, which is what keeps a deployment configured for v2 running v2 until
//     somebody actually activates a version.
//
// The struct is immutable once built and every call reads the registry afresh, so
// it holds no state a concurrent activation could race: a publication that has
// resolved its classifier is unaffected by an activation that lands while it is
// running, and the next publication simply sees the new version.
type RegistryClassifierSource struct {
	// Models reads the active row from shared storage. Required.
	Models ActiveModelReader
	// Registry resolves the active version to a classifier that is registered in
	// this binary. Required once an explicit activation exists.
	Registry ClassifierLookup
	// Bootstrap is the classifier the startup selection (MODEL_ID) produced. It is
	// used when the registry records no explicit activation, and it must be nil
	// only in a deployment that is expected to fail closed.
	Bootstrap classify.Classifier
}

// ActiveClassifier returns the classifier that must serve one publication.
func (s RegistryClassifierSource) ActiveClassifier(ctx context.Context) (classify.Classifier, error) {
	if s.Models == nil {
		return nil, fmt.Errorf("%w: no models registry reader is configured", ErrClassifierUnresolvable)
	}
	model, err := s.Models.ActiveModel(ctx)
	switch {
	case errors.Is(err, repository.ErrNoActiveModel):
		return s.bootstrap()
	case err != nil:
		// A registry that cannot be read is not a decision about a version: it is
		// a dependency failure, so it stays retryable.
		return nil, fmt.Errorf("read the active model from the models registry: %w", err)
	}
	if !model.ExplicitlyActivated() {
		// The seeded active row expresses the registry's lifecycle, not an
		// operator's decision, so it does not override the startup selection.
		return s.bootstrap()
	}
	if s.Registry == nil {
		return nil, fmt.Errorf("%w: model %q is explicitly active but no classifier registry is configured", ErrClassifierUnresolvable, model.Version)
	}
	classifier, err := s.Registry.Select(model.Version)
	if err != nil {
		// Fail closed: the version nobody can execute must not be silently replaced
		// by the startup selection, because the batch would then be attributed to a
		// version that did not judge it.
		return nil, fmt.Errorf("%w: the registry marks %q active but this service has no such classifier: %v", ErrClassifierUnresolvable, model.Version, err)
	}
	storedHash := ""
	if model.ParamsHash != nil {
		storedHash = *model.ParamsHash
	}
	// The same check the startup path runs, applied to the version that will
	// actually serve: a frozen identifier must not be served by a classifier whose
	// behaviour the registry contradicts.
	if err := classify.VerifyRegisteredModel(model.Version, true, storedHash); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrClassifierUnresolvable, err)
	}
	return classifier, nil
}

// bootstrap returns the startup-selected classifier, or fails closed when a
// deployment has no classifier it may fall back to.
func (s RegistryClassifierSource) bootstrap() (classify.Classifier, error) {
	if s.Bootstrap == nil {
		return nil, fmt.Errorf("%w: the registry records no explicit activation and no startup classifier is configured", ErrClassifierUnresolvable)
	}
	return s.Bootstrap, nil
}
