package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	v1 "firewatch.local/contracts/events/v1"
	observations "firewatch.local/contracts/observations/v1"
	"firewatch.local/risk/internal/classify"
	"firewatch.local/risk/internal/repository"
	"firewatch.local/risk/internal/source"
)

// BatchWriter persists one assessed batch. It exists so the publication path can
// be tested without a database.
type BatchWriter interface {
	Publish(ctx context.Context, request v1.Event, assessment repository.Assessment) error
}

// RiskObserver records the outcome of one published assessment. It is optional:
// a nil observer disables recording, so the publication path has no hard
// dependency on the metrics surface and its unit tests need no counter.
type RiskObserver interface {
	Observe(level classify.Level)
}

// permanentError declares that an error can never succeed on a retry. The
// publication path is the place that knows the cause, so it decides here instead
// of forcing the consumer to inspect an error string. The consumer only asks
// whether an error declares itself permanent and, when it does, dead-letters the
// event immediately instead of spending the whole retry budget on work that is
// known to fail.
//
// The classification is deliberately narrow, because discarding work that a later
// attempt could complete is worse than a slow dead-letter. Only the two
// deterministic publication failures are wrapped: a non-canonical level from
// Validate, and a policy identity mismatch. A provider fetch failure, an
// incomplete retrieval, an adaptation refusal (mixed zones, missing retrieval
// instant) and a storage failure stay TRANSIENT even when they look obviously
// wrong: the provider is re-fetched on every attempt, so a transient state can
// produce them once and not again. When in doubt, retry.
type permanentError struct{ err error }

func (e permanentError) Error() string   { return e.err.Error() }
func (e permanentError) Unwrap() error   { return e.err }
func (e permanentError) Permanent() bool { return true }

// Publisher turns one job request into one persisted, classified batch.
//
// It is the only place where data acquisition, classification and persistence
// meet, which is what keeps a provider and a classifier interchangeable.
type Publisher struct {
	Provider   source.Provider
	Classifier classify.Classifier
	Store      BatchWriter
	// Outcomes records the level of every persisted assessment. It is optional:
	// a nil observer means the outcome is logged but not counted.
	Outcomes RiskObserver
}

// selectedProvider returns the provider that must serve one request.
//
// A request without scenario metadata keeps the configured provider EXACTLY:
// that is the legacy fixture path and it is never re-routed. A request that names
// a scenario is a demonstration request: it selects one built-in synthetic
// provider pinned to the job's server-assigned evaluation instant, so the same
// job is replayable and the configured provider is not consulted at all.
//
// The check is deliberately separate from the fetch so a malformed scenario
// request is refused before any acquisition or persistence happens. It returns a
// plain error and the caller makes it permanent, because the catalogue and the
// request are both fixed: no retry can turn an unknown scenario into a known one
// or invent the instant the job never carried.
func (p Publisher) selectedProvider(request v1.Event) (source.Provider, error) {
	switch {
	case request.ScenarioID == "" && request.EvaluatedAt == nil:
		return p.Provider, nil
	case request.ScenarioID == "":
		return nil, errors.New("event carries an evaluation instant without a scenario")
	case request.EvaluatedAt == nil || request.EvaluatedAt.IsZero():
		return nil, fmt.Errorf("synthetic scenario %q requires the job's evaluation instant", request.ScenarioID)
	}
	provider, err := source.NewSyntheticProvider(request.ScenarioID, request.ZoneID, *request.EvaluatedAt)
	if err != nil {
		return nil, err
	}
	return provider, nil
}

// Publish fetches the provider selected for the request, classifies what it
// returned and persists the assessment with the request that caused it.
//
// The order is fixed and each step guards the next:
//
//  1. the two invariants - classifier and store - must be present, and the
//     provider selected below must not be absent, so a misconfigured process
//     fails explicitly instead of panicking;
//  2. the scenario metadata is resolved before anything else, so an unknown
//     scenario, a missing or zero instant, or an instant without a scenario is
//     refused permanently before any acquisition or storage;
//  3. the selected provider is fetched once with a zero window, because the
//     provider contract carries no zone parameter and the provider's own coverage
//     is the scope;
//  4. the retrieval must declare itself complete, because a partial retrieval
//     must not be able to produce a determined assessment by accident;
//  5. the retrieval is adapted into classifier features, which refuses a set
//     that mixes zones and, for a non-synthetic retrieval, a set whose own zone
//     labels do not identify the requested zone;
//  6. the classifier is asked for a level, its factors and its rationale;
//  7. the assessment is validated before storage, so a non-canonical level can
//     never reach the database;
//  8. the assessment is persisted with the provenance-derived demo flag, and the
//     request's scenario metadata travels untouched into the outbox event because
//     the store copies the request.
func (p Publisher) Publish(ctx context.Context, request v1.Event) error {
	if p.Classifier == nil || p.Store == nil {
		return errors.New("publisher is misconfigured: classifier and store are required")
	}
	provider, err := p.selectedProvider(request)
	if err != nil {
		// The catalogue and the request metadata are fixed, so this can never
		// succeed on a retry: permanent keeps the consumer from spending the whole
		// retry budget on it and, with prefetch 1, blocking the queue behind it.
		return fmt.Errorf("select provider for job %s: %w", request.JobID, permanentError{err: err})
	}
	if provider == nil {
		return errors.New("publisher is misconfigured: a provider is required when the request selects no scenario")
	}
	descriptor := provider.Describe()
	set, err := provider.Fetch(ctx, observations.Window{})
	if err != nil {
		return fmt.Errorf("fetch from provider %s: %w", descriptor.ID, err)
	}
	// A retrieval that declares itself incomplete must not produce a determined
	// assessment: persisting one would present partial data as a finished answer.
	// This is the same rule Merge already enforces ("a partial retrieval must not
	// be able to satisfy a key by accident"), applied before classification so no
	// assessment can be built from it. The refusal stays TRANSIENT, because the
	// provider is re-fetched on every attempt and pagination can complete later;
	// the reason names the incompleteness so an operator can tell it apart from a
	// fetch failure.
	if !set.Provenance.Complete {
		return fmt.Errorf("provider %s declared an incomplete retrieval (provenance.complete=false): a partial retrieval must not produce a determined assessment", descriptor.ID)
	}
	// Record-level validation runs before classification, so an invalid record can
	// never be interpreted as a reading. The rejection stays TRANSIENT: the
	// provider is re-fetched on every attempt and a source can repair a bad batch,
	// so discarding the job permanently would be the wrong call. Every rejection is
	// logged with its cause and index so the invalid entry is attributable, and the
	// error carries the same reasons for the dead-letter record.
	validation := source.Validate(set)
	if !validation.Complete() {
		for _, rejection := range validation.Rejected {
			slog.Warn("source_record_rejected",
				"job_id", request.JobID,
				"provider_id", descriptor.ID,
				"index", rejection.Index,
				"cause", string(rejection.Cause),
				"detail", rejection.Detail,
			)
		}
		return fmt.Errorf("provider %s returned %d invalid records: %s", descriptor.ID, len(validation.Rejected), validation.Reasons())
	}
	features, err := featuresFrom(set, request.ZoneID)
	if err != nil {
		return fmt.Errorf("build features from provider %s: %w", descriptor.ID, err)
	}
	assessment, err := p.Classifier.Classify(ctx, features)
	if err != nil {
		return fmt.Errorf("classify with %s: %w", p.Classifier.ID(), err)
	}
	if err := assessment.Validate(); err != nil {
		// A non-canonical level is deterministic: the classifier would return the
		// same assessment on every attempt. It is tagged permanent so the consumer
		// dead-letters it instead of spending the whole retry budget on it.
		return fmt.Errorf("classify with %s: %w", p.Classifier.ID(), permanentError{err: err})
	}
	// Identity check before persistence: the assessment must not claim a frozen
	// policy identifier while reporting a different fingerprint. Validate above
	// proved the policy id and hash are present; this proves they agree with the
	// frozen expectation for that id, so "publishable under the frozen id" is
	// closed. An unknown id - a future trained model - is accepted.
	if err := verifyAssessmentPolicy(assessment.Policy); err != nil {
		// A fingerprint mismatch is deterministic too: the classifier instance
		// carries a fixed parameter set, so no retry can make it agree with the
		// frozen fingerprint. It is permanent for the same reason as above.
		return fmt.Errorf("classify with %s: %w", p.Classifier.ID(), permanentError{err: err})
	}
	// The reference identifies the parameters actually used, not the default: it
	// comes from the classifier instance that produced this assessment, and
	// Validate above already proved its policy id and params hash are present. The
	// address is taken of a local copy, never of a field of a temporary.
	policy := assessment.Policy
	if err := p.Store.Publish(ctx, request, repository.Assessment{
		ModelVersion: p.Classifier.ID(),
		RiskLevel:    string(assessment.Level),
		Factors:      assessment.Factors,
		Rationale:    assessment.Rationale,
		Policy:       &policy,
		// Demo means "synthetic or demonstrative input". It is derived from the
		// provenance of the provider that contributed the data, never from a
		// constant, so registering a real provider clears it automatically.
		Demo: set.Provenance.Synthetic,
	}); err != nil {
		return err
	}
	// Logged and counted only AFTER persistence: a failed publication is retried
	// whole, so recording before the store would report a level that publishing
	// then discarded. Log, counter and the stored row therefore always describe
	// the same set of persisted assessments.
	slog.Info("risk_assessed", assessmentLog(descriptor, request, features, assessment)...)
	if p.Outcomes != nil {
		p.Outcomes.Observe(assessment.Level)
	}
	return nil
}

// assessmentLog renders one persisted assessment as structured log attributes:
// the inputs the classifier actually saw, the level it produced, and the
// identity needed to attribute them. It is the inputs/outputs half of the model
// observability logging; ground truth (LOG-I3) is deliberately absent, because
// the real observations are deferred and a placeholder would be a fabricated
// label. When ground truth arrives it joins to these records by job_id.
//
// An absent input is logged as the string "unknown", never as 0: a missing
// humidity reading and a humidity of zero are different facts, and the
// substitution the classifier refuses must not be reintroduced by the log.
func assessmentLog(descriptor observations.SourceDescriptor, request v1.Event, features classify.Features, assessment classify.Assessment) []any {
	return []any{
		"job_id", request.JobID,
		"zone_id", request.ZoneID,
		"provider_id", descriptor.ID,
		"synthetic", descriptor.Synthetic,
		"model_version", assessment.Policy.PolicyID,
		"params_hash", assessment.Policy.ParamsHash,
		"risk_level", string(assessment.Level),
		"observed_at", features.ObservedAt,
		"evaluated_at", features.EvaluatedAt,
		"temperatura_c", featureLogValue(features.TemperatureC),
		"humedad_relativa_pct", featureLogValue(features.RelativeHumidityPct),
		"viento_kmh", featureLogValue(features.WindSpeedKmh),
		"cobertura_vegetal_pct", featureLogValue(features.VegetationCoverPct),
		"incendios_historicos", featureLogValue(features.HistoricalFires),
	}
}

// featureLogValue renders a present feature as its value and an absent one as
// "unknown", so the log never invents a reading the classifier never received.
func featureLogValue(feature classify.Feature) any {
	if !feature.Present {
		return "unknown"
	}
	return feature.Value
}

// verifyAssessmentPolicy refuses a policy that claims a frozen identifier while
// reporting a different fingerprint. An identifier that is not frozen is accepted
// unchanged, so a future trained model is not constrained by this check.
//
// It is the publication-side half of the registry check: the registry refuses an
// impersonating classifier at registration, and this refuses an impersonating
// assessment before it is persisted, so a policy can never be stored under a
// frozen identifier it does not match.
func verifyAssessmentPolicy(policy classify.PolicyRef) error {
	expected, frozen := classify.FrozenPolicyFingerprints()[policy.PolicyID]
	if !frozen {
		return nil
	}
	if policy.ParamsHash != expected {
		return fmt.Errorf("policy %q would be persisted with params hash %q, want the frozen %q", policy.PolicyID, policy.ParamsHash, expected)
	}
	return nil
}

// featuresFrom adapts one provider retrieval into classifier input.
//
// It is deliberately package-private: it is the seam between two contracts, not
// a public shape, so only the publication path depends on it.
func featuresFrom(set observations.ObservationSet, zone string) (classify.Features, error) {
	features := classify.Features{
		// The evaluation instant is the retrieval instant. The classifier must
		// never read the system clock, and the publish path fetches and
		// classifies in the same request, so the moment of retrieval is the
		// moment of assessment.
		EvaluatedAt: set.Provenance.RetrievedAt,
		// The zone is the one the assessment is about, taken from the request.
		// Records are NOT filtered by zone: the fetch contract has no zone
		// parameter, so the provider's coverage is the scope. A synthetic retrieval
		// is allowed to label its records differently from the requested zone (the
		// documented fixture divergence); a non-synthetic retrieval is refused
		// below unless its own labels identify the requested zone.
		ZoneID: zone,
	}
	if features.EvaluatedAt.IsZero() {
		return classify.Features{}, errors.New("provenance has no retrieval instant")
	}

	// A set that mixes zones cannot be attributed to a single zone, so it is
	// refused rather than silently averaged across them. A set carrying one label,
	// or no label at all, passes this guard; the non-synthetic attribution guard
	// below decides whether that label identifies the requested zone. Note that
	// the fixture's own zone label ("valparaiso") differs from the frozen
	// transport literal ("valparaiso-demo"); reconciling zone names is a separate,
	// already identified task and is not done here.
	label, err := retrievalZone(set.Records)
	if err != nil {
		return classify.Features{}, err
	}
	// Zone attribution, fail-closed for a real provider. The assessment is
	// persisted under the zone the request names and records are never filtered
	// by zone, so accepting a non-synthetic retrieval whose own labels do not
	// identify the requested zone would re-label another commune's data as this
	// one, geometry included. The comparison is exact. Documented limitation:
	// until the deferred zone-name reconciliation lands, a real provider whose
	// label is not literally the requested zone - or that carries no label at all
	// - fails closed here, so every non-synthetic provider fails this guard today.
	// That is the intended outcome, not a bug to be weakened so hypothetical real
	// data passes: a mismatch means the data does not support the requested
	// commune and nothing may be attributed to it. The synthetic path keeps its
	// documented divergence below.
	if !set.Provenance.Synthetic {
		if label == "" {
			return classify.Features{}, fmt.Errorf("non-synthetic retrieval carries no zone label and cannot identify the requested zone %q", zone)
		}
		if label != zone {
			return classify.Features{}, fmt.Errorf("non-synthetic retrieval is labelled zone %q but the request is for zone %q: refusing to attribute data to a commune its own labels do not identify", label, zone)
		}
	}

	// Only the five canonical variables participate. Any other variable name is
	// ignored, so a provider cannot widen the classifier's input by inventing a
	// field. For each canonical variable the greatest ObservedAt wins; on an
	// exact tie the first record in input order is kept, because a provider is
	// expected to return one reading per variable and instant.
	kept := map[string]observations.Observation{}
	for _, record := range set.Records {
		if !canonicalVariable(record.Variable) {
			continue
		}
		current, seen := kept[record.Variable]
		if !seen || record.ObservedAt.After(current.ObservedAt) {
			kept[record.Variable] = record
		}
	}

	// ObservedAt is the EARLIEST instant among the kept readings: a data set is
	// only as fresh as its oldest input. It is left zero when nothing was kept,
	// and the rule then abstains on its own missing-instant branch instead of
	// being told a freshness it does not have.
	for _, record := range kept {
		if features.ObservedAt.IsZero() || record.ObservedAt.Before(features.ObservedAt) {
			features.ObservedAt = record.ObservedAt
		}
	}

	// A canonical variable with no reading stays an absent feature, never
	// classify.Known(0): a missing humidity reading and a humidity of zero are
	// different facts, and only the absent form lets the rule abstain.
	if record, ok := kept[observations.VariableTemperature]; ok {
		features.TemperatureC = classify.Known(record.Value)
	}
	if record, ok := kept[observations.VariableHumidity]; ok {
		features.RelativeHumidityPct = classify.Known(record.Value)
	}
	if record, ok := kept[observations.VariableWind]; ok {
		features.WindSpeedKmh = classify.Known(record.Value)
	}
	if record, ok := kept[observations.VariableVegetation]; ok {
		features.VegetationCoverPct = classify.Known(record.Value)
	}
	if record, ok := kept[observations.VariableHistoricalFires]; ok {
		features.HistoricalFires = classify.Known(record.Value)
	}
	return features, nil
}

// retrievalZone returns the single non-empty zone label a record set carries and
// "" when it carries none. It refuses a set whose non-empty labels disagree,
// because such a set cannot be attributed to a single zone.
func retrievalZone(records []observations.Observation) (string, error) {
	label := ""
	for _, record := range records {
		if record.ZoneID == "" {
			continue
		}
		if label == "" {
			label = record.ZoneID
			continue
		}
		if record.ZoneID != label {
			return "", fmt.Errorf("observation set mixes zones %q and %q and cannot be attributed to a single zone", label, record.ZoneID)
		}
	}
	return label, nil
}

// canonicalVariable reports whether a variable name belongs to the shared
// canonical vocabulary of the observations contract.
func canonicalVariable(name string) bool {
	for _, candidate := range observations.VariableNames() {
		if name == candidate {
			return true
		}
	}
	return false
}
