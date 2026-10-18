// This file covers the publication path without a database: Publisher.Publish
// end to end through fakes, and the private adapter featuresFrom directly.
//
// The two seams are tested separately on purpose. Publish is the ordering and
// wiring contract (guard collaborators, fetch, adapt, classify, validate,
// persist), while featuresFrom is the adapter that decides what the classifier
// is allowed to see. A defect in one is not a defect in the other, and only a
// unit test can pin instants and record shapes exactly.
package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	v1 "firewatch.local/contracts/events/v1"
	observations "firewatch.local/contracts/observations/v1"
	"firewatch.local/risk/internal/classify"
	"firewatch.local/risk/internal/repository"
	"firewatch.local/risk/internal/source"
	"github.com/google/uuid"
)

// stubProvider is a source.Provider built from a descriptor and a set supplied
// by the test, so the publication path can be driven with an exact retrieval
// instead of the built-in fixture. The context is honored like a real provider,
// so a cancelled request fails instead of silently returning data.
type stubProvider struct {
	descriptor observations.SourceDescriptor
	set        observations.ObservationSet
}

func (s stubProvider) Describe() observations.SourceDescriptor { return s.descriptor }

func (s stubProvider) Fetch(ctx context.Context, window observations.Window) (observations.ObservationSet, error) {
	if err := ctx.Err(); err != nil {
		return observations.ObservationSet{}, err
	}
	return s.set, nil
}

// stubWriter is a BatchWriter that records what the publication path tried to
// persist and can be made to fail, so persistence is asserted without a
// database and a store failure is proven to surface through Publish. The two
// persistence seams are counted separately: Publish is the explicit-job
// completion seam and PublishScheduled is the automatic-cycle seam, so a test
// can prove the publication path chose the one its origin requires.
type stubWriter struct {
	calls      int
	scheduled  int
	request    v1.Event
	assessment repository.Assessment
	// scheduledRequest and scheduledAssessment record the automatic-cycle seam
	// separately from the explicit-job one, so a test can assert both what was
	// persisted and which seam persisted it.
	scheduledRequest    v1.Event
	scheduledAssessment repository.Assessment
	err                 error
}

func (w *stubWriter) Publish(ctx context.Context, request v1.Event, assessment repository.Assessment) error {
	w.calls++
	w.request = request
	w.assessment = assessment
	return w.err
}

func (w *stubWriter) PublishScheduled(ctx context.Context, request v1.Event, assessment repository.Assessment) error {
	w.scheduled++
	w.scheduledRequest = request
	w.scheduledAssessment = assessment
	return w.err
}

// jobOnlyWriter implements ONLY the explicit-job persistence seam. It models a
// store that was never given a way to persist a scheduled publication, so a
// scheduled request against it must fail closed instead of quietly settling a
// job that does not exist.
type jobOnlyWriter struct{ calls int }

func (w *jobOnlyWriter) Publish(_ context.Context, _ v1.Event, _ repository.Assessment) error {
	w.calls++
	return nil
}

// stubDescriptor returns a plausible descriptor for a test provider. Synthetic
// is the single input that must decide the persisted demo flag.
func stubDescriptor(synthetic bool) observations.SourceDescriptor {
	return observations.SourceDescriptor{
		ID:               "stub-provider",
		DisplayName:      "Stub provider",
		Kind:             observations.SourceKindFile,
		Origin:           observations.OriginPublicOfficial,
		Endpoint:         "stub://provider",
		License:          "N/A (test)",
		Attribution:      "FireWatch tests",
		Variables:        observations.VariableNames(),
		GeoCoverage:      "test",
		TemporalCoverage: "test",
		Cadence:          "manual",
		Purpose:          "Expose the publication path to unit tests",
		Priority:         100,
		Synthetic:        synthetic,
	}
}

// sampleRequest is one valid job request for the publication path. Its zone
// label is the request's own; the adapter does not filter records by it.
func sampleRequest() v1.Event {
	return v1.Event{
		ID:          uuid.NewString(),
		Type:        v1.JobRequestedType,
		OccurredAt:  source.SampleInstant,
		TraceID:     uuid.NewString(),
		JobID:       uuid.NewString(),
		ActorID:     "unit-test",
		ZoneID:      "valparaiso-demo",
		RequestedAt: source.SampleInstant,
		Demo:        true,
	}
}

// unitRecord builds one canonical observation for the adapter tests.
func unitRecord(variable, zone string, instant time.Time, value float64) observations.Observation {
	return observations.Observation{
		SourceID:   "stub-provider",
		Variable:   variable,
		ZoneID:     zone,
		ObservedAt: instant,
		Value:      value,
		Unit:       "unit",
		Quality:    observations.QualityOK,
	}
}

// TestPublishPersistsARealAssessmentFromTheFixture protects the wiring of the
// publication path against the built-in fixture: a real fetch, the real academic
// rule and a writer must agree on one complete, persisted assessment.
func TestPublishPersistsARealAssessmentFromTheFixture(t *testing.T) {
	writer := &stubWriter{}
	publisher := Publisher{
		Provider:   source.NewFixtureProvider(),
		Classifier: classify.NewAcademicRuleV1(),
		Store:      writer,
	}
	if err := publisher.Publish(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("publish the built-in fixture: %v", err)
	}
	if writer.calls != 1 {
		t.Fatalf("writer was called %d times, want 1", writer.calls)
	}
	assessment := writer.assessment
	if assessment.ModelVersion != classify.AcademicRuleV1ID {
		t.Fatalf("model version is %q, want %q", assessment.ModelVersion, classify.AcademicRuleV1ID)
	}
	// The fixture scores 0.895 on the academic rule (temperature 0.85,
	// humidity 1.0, wind 0.875, vegetation 0.9167, historical fires 0.8 after
	// weighting), which is inside the high band. This is asserted, not assumed.
	if assessment.RiskLevel != string(classify.LevelAlto) {
		t.Fatalf("risk level is %q, want %q", assessment.RiskLevel, classify.LevelAlto)
	}
	if len(assessment.Factors) != 5 {
		t.Fatalf("assessment carries %d factors, want the 5 canonical ones", len(assessment.Factors))
	}
	if assessment.Rationale == "" {
		t.Fatal("assessment was persisted without a rationale")
	}
	if !assessment.Demo {
		t.Fatal("the synthetic fixture was persisted as real data")
	}
}

// S1 regression: an automatic cycle publishes through the scheduled seam, so its
// batch is persisted but no job completion is emitted. Jobs never stored the
// job id a cycle mints, so a risk.batch.published.v1 naming it is refused as an
// unknown job and dead-lettered permanently, once per zone per cycle. The
// scheduled path must still run the whole pipeline - scenario resolution, fetch,
// validation, classification, policy identity - and hand the store the same
// assessment the explicit path would.
func TestPublishScheduledUsesTheScheduledSeamAndNeverTheCompletionSeam(t *testing.T) {
	writer := &stubWriter{}
	publisher := Publisher{
		Provider:   source.NewFixtureProvider(),
		Classifier: classify.NewAcademicRuleV1(),
		Store:      writer,
	}
	request := sampleRequest()
	if err := publisher.PublishScheduled(context.Background(), request); err != nil {
		t.Fatalf("publish the fixture as an automatic cycle: %v", err)
	}
	if writer.scheduled != 1 || writer.calls != 0 {
		t.Fatalf("scheduled=%d completion=%d, want the scheduled seam exactly once and the completion seam never", writer.scheduled, writer.calls)
	}
	if writer.scheduledRequest != request {
		t.Fatalf("the scheduled request was not preserved: %+v", writer.scheduledRequest)
	}
	assessment := writer.scheduledAssessment
	if assessment.ModelVersion != classify.AcademicRuleV1ID || assessment.RiskLevel != string(classify.LevelAlto) {
		t.Fatalf("scheduled assessment = %+v, want the real fixture assessment", assessment)
	}
	if len(assessment.Factors) != 5 || assessment.Rationale == "" || !assessment.Demo {
		t.Fatalf("the scheduled path skipped part of the shared pipeline: %+v", assessment)
	}
	if assessment.Policy == nil || assessment.Policy.PolicyID != classify.AcademicRuleV1ID || assessment.Policy.ParamsHash != classify.AcademicRuleV1ParamsHash {
		t.Fatalf("the scheduled path dropped the policy reference: %+v", assessment.Policy)
	}
}

// The explicit-job path keeps its own seam: one delivery of a requested job
// settles exactly one completion, and never reaches the scheduled seam.
func TestPublishUsesTheCompletionSeamExactlyOnce(t *testing.T) {
	writer := &stubWriter{}
	publisher := Publisher{
		Provider:   source.NewFixtureProvider(),
		Classifier: classify.NewAcademicRuleV1(),
		Store:      writer,
	}
	if err := publisher.Publish(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("publish a requested job: %v", err)
	}
	if writer.calls != 1 || writer.scheduled != 0 {
		t.Fatalf("completion=%d scheduled=%d, want the completion seam exactly once and the scheduled seam never", writer.calls, writer.scheduled)
	}
}

// A store that offers no scheduled persistence must fail closed instead of
// falling back to the completion seam: falling back would recreate the defect
// this seam exists to prevent, silently dead-lettering a completion for a job
// that was never requested.
func TestPublishScheduledFailsClosedWhenTheStoreCannotPersistScheduledPublications(t *testing.T) {
	writer := &jobOnlyWriter{}
	err := (Publisher{
		Provider:   source.NewFixtureProvider(),
		Classifier: classify.NewAcademicRuleV1(),
		Store:      writer,
	}).PublishScheduled(context.Background(), sampleRequest())
	if err == nil {
		t.Fatal("a store without the scheduled seam accepted an automatic-cycle publication")
	}
	if !strings.Contains(err.Error(), "scheduled") {
		t.Fatalf("the refusal does not name the missing seam: %v", err)
	}
	if writer.calls != 0 {
		t.Fatalf("the completion seam was reached %d times for a scheduled publication", writer.calls)
	}
}

// The scheduled path shares the publication path's permanent refusals: an
// unknown scenario is refused before any acquisition or storage and stays
// permanent, because neither the catalogue nor the request can change on a
// retry. A scheduled publication must not become a way to bypass that
// classification and spend the retry budget on a deterministic failure.
func TestPublishScheduledKeepsTheSharedPermanentRefusals(t *testing.T) {
	writer := &stubWriter{}
	err := (Publisher{
		Provider:   source.NewFixtureProvider(),
		Classifier: classify.NewAcademicRuleV1(),
		Store:      writer,
	}).PublishScheduled(context.Background(), scenarioRequest("no-such-scenario", scenarioInstant))
	var permanent interface{ Permanent() bool }
	if !errors.As(err, &permanent) || !permanent.Permanent() {
		t.Fatalf("the scheduled refusal is not permanent: %v", err)
	}
	if writer.scheduled != 0 || writer.calls != 0 {
		t.Fatalf("an unknown scenario reached persistence: scheduled=%d completion=%d", writer.scheduled, writer.calls)
	}
}

// The misconfiguration guard is shared: a scheduled publication with no
// classifier or store must return an error instead of panicking or persisting a
// partial assessment.
func TestPublishScheduledRefusesAnUnconfiguredCollaborator(t *testing.T) {
	for _, tc := range []struct {
		name      string
		publisher Publisher
	}{
		{"missing classifier", Publisher{Provider: source.NewFixtureProvider(), Store: &stubWriter{}}},
		{"missing store", Publisher{Provider: source.NewFixtureProvider(), Classifier: classify.NewAcademicRuleV1()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.publisher.PublishScheduled(context.Background(), sampleRequest()); err == nil {
				t.Fatal("a misconfigured publisher published a scheduled batch anyway")
			}
		})
	}
}

// TestPublisherPassesTheEffectivePolicyReference proves the persisted reference
// pins the parameters behind the answer, and that the identity lock holds: an
// injected instance whose fingerprint differs cannot be published under the
// frozen identifier, so the frozen name can never describe a different behaviour.
func TestPublisherPassesTheEffectivePolicyReference(t *testing.T) {
	frozen := &stubWriter{}
	if err := (Publisher{
		Provider:   source.NewFixtureProvider(),
		Classifier: classify.NewAcademicRuleV1(),
		Store:      frozen,
	}).Publish(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("publish the frozen default rule: %v", err)
	}
	if frozen.assessment.Policy == nil {
		t.Fatal("the assessment was persisted without a policy reference")
	}
	if frozen.assessment.Policy.PolicyID != classify.AcademicRuleV1ID {
		t.Fatalf("policy id is %q, want %q", frozen.assessment.Policy.PolicyID, classify.AcademicRuleV1ID)
	}
	if frozen.assessment.Policy.ParamsHash != classify.AcademicRuleV1ParamsHash {
		t.Fatalf("params hash is %q, want the frozen %q", frozen.assessment.Policy.ParamsHash, classify.AcademicRuleV1ParamsHash)
	}
	if got := frozen.assessment.Policy.Parameters["max_staleness_seconds"]; got != "21600" {
		t.Fatalf("max_staleness_seconds is %q, want the frozen %q", got, "21600")
	}

	// The injected instance is the sharpest probe of the identity lock: its
	// fingerprint differs from the frozen one, so it must NOT be persisted under
	// the frozen identifier. The rule itself still reports its own hash (see
	// TestTheHashDescribesTheInstanceActuallyUsed); the publication path is what
	// refuses to attach the frozen name to the different behaviour.
	injected := classify.DefaultAcademicRuleV1Params()
	injected.MaxStalenessSeconds = 60
	rule, err := classify.NewAcademicRuleV1WithParams(injected)
	if err != nil {
		t.Fatalf("build a rule with injected parameters: %v", err)
	}
	changed := &stubWriter{}
	err = (Publisher{
		Provider:   source.NewFixtureProvider(),
		Classifier: rule,
		Store:      changed,
	}).Publish(context.Background(), sampleRequest())
	if err == nil {
		t.Fatal("an injected parameter set was persisted under the frozen identifier")
	}
	if changed.calls != 0 {
		t.Fatalf("the writer was reached %d times for an impersonated frozen identifier", changed.calls)
	}
}

// TestDemoIsDerivedFromProvenanceNotFromAConstant protects the demo flag: it
// must follow the contributing provider's provenance, so registering a real
// source clears it without touching this code. The retrieval is complete and
// its records carry the requested zone, because those are the two guards a
// non-synthetic retrieval must now pass before the demo flag is reached (the
// mislabelled and incomplete non-synthetic retrievals are pinned separately).
func TestDemoIsDerivedFromProvenanceNotFromAConstant(t *testing.T) {
	descriptor := stubDescriptor(false)
	request := sampleRequest()
	records := source.FixtureRecords()
	for index := range records {
		records[index].ZoneID = request.ZoneID
	}
	set := observations.ObservationSet{
		Records:    records,
		Provenance: source.ProvenanceFrom(descriptor),
	}
	set.Provenance.RetrievedAt = source.SampleInstant
	// ProvenanceFrom leaves Complete at its zero value, and a retrieval that
	// declares itself incomplete is now refused before classification (S5). This
	// test is about the demo flag, so the retrieval declares itself whole.
	set.Provenance.Complete = true
	writer := &stubWriter{}
	publisher := Publisher{
		Provider:   stubProvider{descriptor: descriptor, set: set},
		Classifier: classify.NewAcademicRuleV1(),
		Store:      writer,
	}
	if err := publisher.Publish(context.Background(), request); err != nil {
		t.Fatalf("publish from a non-synthetic provider: %v", err)
	}
	// Exactly one assessment must have been written. Without this, a silent
	// successful no-write would leave writer.assessment at its zero value and the
	// demo assertion below would still pass.
	if writer.calls != 1 {
		t.Fatalf("the writer was called %d times, want exactly 1 persisted assessment", writer.calls)
	}
	if writer.assessment.Demo {
		t.Fatal("a non-synthetic provenance was persisted as a demonstration")
	}
}

// TestPublishRefusesANonSyntheticRetrievalLabelledWithAnotherZone protects zone
// attribution on the publication path. The persisted assessment is attributed to
// the zone the request names, and the adapter does not filter records by zone, so
// for a real provider a label mismatch would silently re-label another commune's
// data as the requested one. A non-synthetic retrieval therefore fails closed
// unless its own zone labels identify the requested zone, records the mismatch in
// its reason, persists nothing, and stays recoverable.
func TestPublishRefusesANonSyntheticRetrievalLabelledWithAnotherZone(t *testing.T) {
	descriptor := stubDescriptor(false)
	request := sampleRequest()
	set := observations.ObservationSet{
		// The fixture records are labelled "valparaiso"; the request names
		// "valparaiso-demo".
		Records:    source.FixtureRecords(),
		Provenance: source.ProvenanceFrom(descriptor),
	}
	set.Provenance.RetrievedAt = source.SampleInstant
	set.Provenance.Complete = true
	writer := &stubWriter{}
	err := (Publisher{
		Provider:   stubProvider{descriptor: descriptor, set: set},
		Classifier: classify.NewAcademicRuleV1(),
		Store:      writer,
	}).Publish(context.Background(), request)

	if err == nil {
		t.Fatal("a non-synthetic retrieval labelled with another zone was published")
	}
	if writer.calls != 0 {
		t.Fatalf("the writer was reached %d times for a mislabelled retrieval", writer.calls)
	}
	if !strings.Contains(err.Error(), "valparaiso\"") || !strings.Contains(err.Error(), request.ZoneID) {
		t.Fatalf("the refusal reason %q does not name both the retrieval label and the requested zone", err)
	}
	var permanent interface{ Permanent() bool }
	if errors.As(err, &permanent) && permanent.Permanent() {
		t.Fatalf("a mislabelled retrieval was classified as permanent: %v", err)
	}
}

// TestPublisherRefusesAnUnconfiguredCollaborator protects against a
// misconfigured process: a missing provider, classifier or store must return an
// error instead of panicking or persisting a partial assessment.
func TestPublisherRefusesAnUnconfiguredCollaborator(t *testing.T) {
	cases := []struct {
		name      string
		publisher Publisher
		writer    *stubWriter
	}{
		{"missing provider", Publisher{Classifier: classify.NewAcademicRuleV1(), Store: &stubWriter{}}, nil},
		{"missing classifier", Publisher{Provider: source.NewFixtureProvider(), Store: &stubWriter{}}, nil},
		{"missing store", Publisher{Provider: source.NewFixtureProvider(), Classifier: classify.NewAcademicRuleV1()}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writer, _ := tc.publisher.Store.(*stubWriter)
			if err := tc.publisher.Publish(context.Background(), sampleRequest()); err == nil {
				t.Fatal("a misconfigured publisher published anyway")
			}
			if writer != nil && writer.calls != 0 {
				t.Fatalf("the writer was reached %d times by a misconfigured publisher", writer.calls)
			}
		})
	}
}

// TestPublishRefusesAnIncompleteRetrieval protects the same rule Merge already
// enforces ("a partial retrieval must not be able to satisfy a key by
// accident") on the publication path: a provider that declares its retrieval
// incomplete must not produce a determined, persisted assessment. The refusal is
// recoverable, because the provider is re-fetched on every attempt, and its
// reason names the incompleteness so an operator can tell it apart from a fetch
// failure. The chosen provider is synthetic, so incompleteness is the only guard
// that can refuse this set.
func TestPublishRefusesAnIncompleteRetrieval(t *testing.T) {
	descriptor := stubDescriptor(true)
	set := observations.ObservationSet{
		Records: source.FixtureRecords(),
		// ProvenanceFrom stamps the descriptor and leaves Complete false, which
		// is exactly the partial-retrieval declaration under test.
		Provenance: source.ProvenanceFrom(descriptor),
	}
	set.Provenance.RetrievedAt = source.SampleInstant
	writer := &stubWriter{}
	classifier := &countingClassifier{assessment: classify.Assessment{Level: classify.LevelMedio, Rationale: "stub"}}
	err := (Publisher{
		Provider:   stubProvider{descriptor: descriptor, set: set},
		Classifier: classifier,
		Store:      writer,
	}).Publish(context.Background(), sampleRequest())

	if err == nil {
		t.Fatal("an incomplete retrieval produced an assessment")
	}
	if writer.calls != 0 {
		t.Fatalf("the writer was reached %d times for an incomplete retrieval", writer.calls)
	}
	if classifier.calls != 0 {
		t.Fatalf("the classifier was reached %d times for an incomplete retrieval", classifier.calls)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "incomplete") {
		t.Fatalf("the refusal reason %q does not name the incompleteness", err)
	}
	var permanent interface{ Permanent() bool }
	if errors.As(err, &permanent) && permanent.Permanent() {
		t.Fatalf("an incomplete retrieval was classified as permanent: %v", err)
	}
}

// TestPublishRefusesAProviderThatReturnedInvalidRecords protects S1's
// "validate before use": a retrieval whose records fail the per-record rules
// must not reach classification or storage, and the refusal must name the cause
// so an operator can attribute it. The chosen provider is synthetic and the
// retrieval is complete, so validation is the only guard that can refuse it.
func TestPublishRefusesAProviderThatReturnedInvalidRecords(t *testing.T) {
	descriptor := stubDescriptor(true)
	records := source.FixtureRecords()
	// One record with no unit is a missing required field; the other four stay
	// valid, so the refusal is attributable to a single record.
	records[0].Unit = ""
	set := observations.ObservationSet{
		Records:    records,
		Provenance: source.ProvenanceFrom(descriptor),
	}
	set.Provenance.RetrievedAt = source.SampleInstant
	set.Provenance.Complete = true
	writer := &stubWriter{}
	classifier := &countingClassifier{assessment: classify.Assessment{Level: classify.LevelMedio, Rationale: "stub"}}
	err := (Publisher{
		Provider:   stubProvider{descriptor: descriptor, set: set},
		Classifier: classifier,
		Store:      writer,
	}).Publish(context.Background(), sampleRequest())

	if err == nil {
		t.Fatal("a retrieval with an invalid record was published")
	}
	if writer.calls != 0 {
		t.Fatalf("the writer was reached %d times for an invalid retrieval", writer.calls)
	}
	if classifier.calls != 0 {
		t.Fatalf("the classifier was reached %d times for an invalid retrieval", classifier.calls)
	}
	if !strings.Contains(err.Error(), string(source.CauseMissing)) || !strings.Contains(err.Error(), "unit") {
		t.Fatalf("the refusal reason %q does not name the missing unit", err)
	}
	var permanent interface{ Permanent() bool }
	if errors.As(err, &permanent) && permanent.Permanent() {
		t.Fatalf("an invalid retrieval was classified as permanent: %v", err)
	}
}

// countingClassifier records whether the classifier was reached, so the ordering
// of the publication path's guards can be asserted and not only its outcome.
type countingClassifier struct {
	calls      int
	assessment classify.Assessment
}

func (c *countingClassifier) ID() string { return classify.AcademicRuleV1ID }

func (c *countingClassifier) Kind() classify.Kind { return classify.KindRule }

func (c *countingClassifier) Classify(context.Context, classify.Features) (classify.Assessment, error) {
	c.calls++
	return c.assessment, nil
}

// TestPublishSurfacesAStoreFailure protects the last step: a store that refuses
// the assessment must surface its error instead of reporting a successful
// publication. A storage failure stays TRANSIENT: the store can come back, so
// the error must not declare itself permanent and the consumer must retry it.
func TestPublishSurfacesAStoreFailure(t *testing.T) {
	failure := errors.New("store unavailable")
	writer := &stubWriter{err: failure}
	publisher := Publisher{
		Provider:   source.NewFixtureProvider(),
		Classifier: classify.NewAcademicRuleV1(),
		Store:      writer,
	}
	err := publisher.Publish(context.Background(), sampleRequest())
	if !errors.Is(err, failure) {
		t.Fatalf("store failure was reported as %v, want %v", err, failure)
	}
	if writer.calls != 1 {
		t.Fatalf("writer was called %d times, want 1", writer.calls)
	}
	var permanent interface{ Permanent() bool }
	if errors.As(err, &permanent) && permanent.Permanent() {
		t.Fatalf("a transient store failure was classified as permanent: %v", err)
	}
}

// TestPublishTagsDeterministicFailuresAsPermanent protects the classification
// the risk consumer depends on: a non-canonical level and a policy identity
// mismatch cannot succeed on a retry, so Publish must return an error that
// declares itself permanent. Anything else would spend the whole retry budget on
// work that is known to fail and, with prefetch 1, would block the queue.
func TestPublishTagsDeterministicFailuresAsPermanent(t *testing.T) {
	mismatchedFingerprint := classify.Assessment{
		Level:     classify.LevelMedio,
		Rationale: "stub",
		Policy: classify.PolicyRef{
			PolicyID:   classify.AcademicRuleV1ID,
			ParamsHash: "0000000000000000000000000000000000000000000000000000000000000000",
		},
	}
	nonCanonicalLevel := mismatchedFingerprint
	nonCanonicalLevel.Level = classify.Level("fuera-de-catalogo")
	nonCanonicalLevel.Policy = classify.PolicyRef{
		PolicyID:   classify.AcademicRuleV1ID,
		ParamsHash: classify.AcademicRuleV1ParamsHash,
	}
	cases := []struct {
		name       string
		assessment classify.Assessment
	}{
		{"non-canonical level", nonCanonicalLevel},
		{"policy identity mismatch", mismatchedFingerprint},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writer := &stubWriter{}
			err := (Publisher{
				Provider:   source.NewFixtureProvider(),
				Classifier: stubPolicyClassifier{assessment: tc.assessment},
				Store:      writer,
			}).Publish(context.Background(), sampleRequest())
			if err == nil {
				t.Fatal("a deterministic failure was published")
			}
			if writer.calls != 0 {
				t.Fatalf("the writer was reached %d times for a deterministic failure", writer.calls)
			}
			var permanent interface{ Permanent() bool }
			if !errors.As(err, &permanent) || !permanent.Permanent() {
				t.Fatalf("err=%v must declare itself permanent", err)
			}
		})
	}
}

// stubPolicyClassifier returns one fixed assessment, so the publication path's
// identity check can be driven with a policy that claims a frozen identifier
// while reporting a fingerprint that does not match it.
type stubPolicyClassifier struct {
	assessment classify.Assessment
}

func (s stubPolicyClassifier) ID() string { return s.assessment.Policy.PolicyID }

func (s stubPolicyClassifier) Kind() classify.Kind { return classify.KindRule }

func (s stubPolicyClassifier) Classify(context.Context, classify.Features) (classify.Assessment, error) {
	return s.assessment, nil
}

// TestPublishRefusesAPolicyThatImpersonatesAFrozenIdentifier protects the
// persistence side of the identity check: an assessment must not be stored under
// a frozen policy identifier while reporting a different fingerprint. An
// identifier that is not frozen is accepted.
func TestPublishRefusesAPolicyThatImpersonatesAFrozenIdentifier(t *testing.T) {
	mismatched := classify.Assessment{
		Level:     classify.LevelMedio,
		Rationale: "stub",
		Policy: classify.PolicyRef{
			PolicyID:   classify.AcademicRuleV1ID,
			ParamsHash: "0000000000000000000000000000000000000000000000000000000000000000",
		},
	}
	writer := &stubWriter{}
	publisher := Publisher{
		Provider:   source.NewFixtureProvider(),
		Classifier: stubPolicyClassifier{assessment: mismatched},
		Store:      writer,
	}
	if err := publisher.Publish(context.Background(), sampleRequest()); err == nil {
		t.Fatal("an assessment claiming the frozen identifier with a different fingerprint was published")
	}
	if writer.calls != 0 {
		t.Fatalf("the writer was reached %d times for an impersonated policy", writer.calls)
	}

	// Control: the same classifier under an identifier that is not frozen is
	// accepted, so the refusal above is the identity check and not the stub.
	unknown := mismatched
	unknown.Policy.PolicyID = "trained-model-v1"
	accepted := &stubWriter{}
	if err := (Publisher{
		Provider:   source.NewFixtureProvider(),
		Classifier: stubPolicyClassifier{assessment: unknown},
		Store:      accepted,
	}).Publish(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("a policy under an unknown identifier was refused: %v", err)
	}
	if accepted.calls != 1 {
		t.Fatalf("the writer was called %d times for an unknown policy, want 1", accepted.calls)
	}
}

// TestFeaturesFromRefusesAMixedZoneSet protects attribution: a set that mixes
// zones cannot be described by one zone id, so it is refused instead of being
// silently averaged across them.
func TestFeaturesFromRefusesAMixedZoneSet(t *testing.T) {
	instant := source.SampleInstant
	set := observations.ObservationSet{
		Records: []observations.Observation{
			unitRecord(observations.VariableTemperature, "zone-a", instant, 30),
			unitRecord(observations.VariableHumidity, "zone-b", instant, 20),
		},
		Provenance: observations.Provenance{RetrievedAt: instant},
	}
	features, err := featuresFrom(set, "zone-a")
	if err == nil {
		t.Fatal("a mixed-zone set was adapted instead of refused")
	}
	if features.ZoneID != "" || !features.ObservedAt.IsZero() {
		t.Fatalf("a refused set still produced features: %+v", features)
	}
}

// TestFeaturesFromRefusesANonSyntheticSetThatDoesNotIdentifyTheRequestedZone
// protects the fail-closed side of zone attribution: without an exact match
// between the retrieval's own label and the requested zone, the adapter would be
// handing the classifier a requested zone the data does not support, and the
// persisted geometry would be attributed to a commune nothing identified.
// Records with no label at all identify no zone, so they are refused too.
func TestFeaturesFromRefusesANonSyntheticSetThatDoesNotIdentifyTheRequestedZone(t *testing.T) {
	instant := source.SampleInstant
	for _, tc := range []struct {
		name    string
		records []observations.Observation
	}{
		{"another zone", []observations.Observation{unitRecord(observations.VariableTemperature, "zone-b", instant, 30)}},
		{"no label at all", []observations.Observation{unitRecord(observations.VariableTemperature, "", instant, 30)}},
		{"no records at all", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := observations.ObservationSet{
				Records:    tc.records,
				Provenance: observations.Provenance{RetrievedAt: instant, Complete: true, Synthetic: false},
			}
			features, err := featuresFrom(set, "zone-a")
			if err == nil {
				t.Fatalf("a non-synthetic retrieval with %s was adapted instead of refused", tc.name)
			}
			if features.ZoneID != "" || !features.ObservedAt.IsZero() {
				t.Fatalf("a refused set still produced features: %+v", features)
			}
		})
	}
}

// TestFeaturesFromAcceptsANonSyntheticSetLabelledWithTheRequestedZone is the
// control for the guard above: an exact label match passes, so the refusal is
// the attribution rule and not the non-synthetic flag alone.
func TestFeaturesFromAcceptsANonSyntheticSetLabelledWithTheRequestedZone(t *testing.T) {
	instant := source.SampleInstant
	set := observations.ObservationSet{
		Records:    []observations.Observation{unitRecord(observations.VariableTemperature, "zone-a", instant, 30)},
		Provenance: observations.Provenance{RetrievedAt: instant, Complete: true, Synthetic: false},
	}
	features, err := featuresFrom(set, "zone-a")
	if err != nil {
		t.Fatalf("adapt a correctly labelled non-synthetic set: %v", err)
	}
	if features.ZoneID != "zone-a" || !features.TemperatureC.Present {
		t.Fatalf("the correctly labelled set produced %+v", features)
	}
}

// TestFeaturesFromRefusesAMissingRetrievalInstant protects reproducibility: the
// evaluation instant is the retrieval instant, so a set without one cannot be
// assessed and must be refused rather than stamped with the system clock.
func TestFeaturesFromRefusesAMissingRetrievalInstant(t *testing.T) {
	set := observations.ObservationSet{
		Records: []observations.Observation{
			unitRecord(observations.VariableTemperature, "zone-a", source.SampleInstant, 30),
		},
	}
	features, err := featuresFrom(set, "zone-a")
	if err == nil {
		t.Fatal("a set without a retrieval instant was adapted instead of refused")
	}
	if !features.EvaluatedAt.IsZero() || !features.ObservedAt.IsZero() {
		t.Fatalf("a refused set still produced instants: %+v", features)
	}
}

// TestFeaturesFromUsesTheRetrievalInstantAndTheEarliestObservation protects the
// freshness basis: evaluation is pinned to the retrieval instant and the
// observation instant is the EARLIEST reading, because a data set is only as
// fresh as its oldest input.
func TestFeaturesFromUsesTheRetrievalInstantAndTheEarliestObservation(t *testing.T) {
	earliest := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	latest := earliest.Add(2 * time.Hour)
	retrieved := latest.Add(time.Hour)
	set := observations.ObservationSet{
		Records: []observations.Observation{
			unitRecord(observations.VariableTemperature, "zone-a", earliest, 30),
			unitRecord(observations.VariableWind, "zone-a", latest, 20),
		},
		Provenance: observations.Provenance{RetrievedAt: retrieved},
	}
	features, err := featuresFrom(set, "zone-a")
	if err != nil {
		t.Fatalf("adapt a two-instant set: %v", err)
	}
	if !features.EvaluatedAt.Equal(retrieved) {
		t.Fatalf("evaluated at %s, want the retrieval instant %s", features.EvaluatedAt, retrieved)
	}
	if !features.ObservedAt.Equal(earliest) {
		t.Fatalf("observed at %s, want the earliest instant %s", features.ObservedAt, earliest)
	}
}

// TestFeaturesFromKeepsTheLatestReadingPerVariable protects the latest-wins
// rule: a provider that returns several readings of the same variable must have
// only its newest one reach the classifier.
func TestFeaturesFromKeepsTheLatestReadingPerVariable(t *testing.T) {
	older := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	set := observations.ObservationSet{
		Records: []observations.Observation{
			unitRecord(observations.VariableTemperature, "zone-a", older, 12),
			unitRecord(observations.VariableTemperature, "zone-a", newer, 33),
		},
		Provenance: observations.Provenance{RetrievedAt: newer},
	}
	features, err := featuresFrom(set, "zone-a")
	if err != nil {
		t.Fatalf("adapt a repeated-variable set: %v", err)
	}
	if features.TemperatureC.Value != 33 {
		t.Fatalf("temperature is %v, want the later reading 33", features.TemperatureC.Value)
	}
	if !features.ObservedAt.Equal(newer) {
		t.Fatalf("observed at %s, want the kept reading instant %s", features.ObservedAt, newer)
	}
}

// TestFeaturesFromLeavesAnAbsentVariableAbsent protects the single most
// dangerous mistake in this adapter: an absent reading must stay absent, never
// become a present zero, because "no humidity reading" and "humidity is zero"
// are opposite facts. It also protects the identity of the assessed zone, and it
// pins the documented zone-name divergence of the SYNTHETIC path: the fixture's
// own label differs from the frozen transport literal, and that divergence is
// preserved for synthetic data by decision, while a non-synthetic retrieval is
// refused instead (see the guards below).
func TestFeaturesFromLeavesAnAbsentVariableAbsent(t *testing.T) {
	instant := source.SampleInstant
	zone := "assessment-zone"
	set := observations.ObservationSet{
		// The records carry a different non-empty label than the assessed zone.
		// Records are not filtered by zone, so the request's zone must win for a
		// synthetic retrieval.
		Records: []observations.Observation{
			unitRecord(observations.VariableTemperature, "provider-zone", instant, 30),
			unitRecord(observations.VariableWind, "provider-zone", instant, 20),
			unitRecord(observations.VariableVegetation, "provider-zone", instant, 55),
			unitRecord(observations.VariableHistoricalFires, "provider-zone", instant, 3),
		},
		Provenance: observations.Provenance{RetrievedAt: instant, Complete: true, Synthetic: true},
	}
	features, err := featuresFrom(set, zone)
	if err != nil {
		t.Fatalf("adapt a set with one absent variable: %v", err)
	}
	if features.RelativeHumidityPct.Present {
		t.Fatal("an absent variable was reported as present")
	}
	if features.RelativeHumidityPct == classify.Known(0) {
		t.Fatal("an absent variable was zeroed into a present zero reading")
	}
	if features.ZoneID != zone {
		t.Fatalf("features are attributed to zone %q, want the request zone %q", features.ZoneID, zone)
	}
}

// TestFeaturesFromIgnoresNonCanonicalVariables protects the closed vocabulary:
// a provider cannot widen the classifier's input by inventing a field, and a set
// carrying only unknown names is still a valid (empty) adaptation.
func TestFeaturesFromIgnoresNonCanonicalVariables(t *testing.T) {
	instant := source.SampleInstant
	set := observations.ObservationSet{
		Records: []observations.Observation{
			unitRecord(observations.VariableTemperature, "zone-a", instant, 30),
			unitRecord("presion_atmosferica", "zone-a", instant, 1013),
		},
		Provenance: observations.Provenance{RetrievedAt: instant},
	}
	features, err := featuresFrom(set, "zone-a")
	if err != nil {
		t.Fatalf("adapt a set with a non-canonical variable: %v", err)
	}
	if !features.TemperatureC.Present || features.TemperatureC.Value != 30 {
		t.Fatalf("the canonical reading was lost: %+v", features.TemperatureC)
	}
	if features.RelativeHumidityPct.Present || features.WindSpeedKmh.Present ||
		features.VegetationCoverPct.Present || features.HistoricalFires.Present {
		t.Fatal("a non-canonical variable fabricated a canonical feature")
	}

	// A set made only of unknown names adapts to no features at all; it must not
	// fail, because the vocabulary filter is a silent drop by design.
	unknownOnly := observations.ObservationSet{
		Records: []observations.Observation{
			unitRecord("presion_atmosferica", "zone-a", instant, 1013),
		},
		Provenance: observations.Provenance{RetrievedAt: instant},
	}
	features, err = featuresFrom(unknownOnly, "zone-a")
	if err != nil {
		t.Fatalf("adapt a set of only non-canonical variables: %v", err)
	}
	if len(features.Missing()) != 5 {
		t.Fatalf("expected all 5 canonical features to be absent, got %+v", features)
	}
}

// stubObserver records the levels the publication path reported, so the
// observability wiring can be asserted without the real metrics type.
type stubObserver struct{ levels []classify.Level }

func (o *stubObserver) Observe(level classify.Level) { o.levels = append(o.levels, level) }

// TestPublisherReportsTheOutcomeOnlyAfterPersistence protects the observability
// seam: the level is reported exactly once, and only once the assessment is
// persisted, so the log and the counter describe stored predictions rather than
// discarded ones.
func TestPublisherReportsTheOutcomeOnlyAfterPersistence(t *testing.T) {
	observer := &stubObserver{}
	writer := &stubWriter{}
	if err := (Publisher{
		Provider:   source.NewFixtureProvider(),
		Classifier: classify.NewAcademicRuleV1(),
		Store:      writer,
		Outcomes:   observer,
	}).Publish(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("publish the built-in fixture: %v", err)
	}
	// The fixture scores inside the high band, so the reported level is Alto,
	// which ties the counter to the same value the persisted row carries.
	if len(observer.levels) != 1 || observer.levels[0] != classify.LevelAlto {
		t.Fatalf("the observer recorded %v, want exactly one %q", observer.levels, classify.LevelAlto)
	}
}

// TestPublisherDoesNotReportAnOutcomeWhenPersistenceFails is the control for the
// rule above: a store failure means nothing was published, so the counter must
// not be advanced for an assessment that was then discarded.
func TestPublisherDoesNotReportAnOutcomeWhenPersistenceFails(t *testing.T) {
	observer := &stubObserver{}
	writer := &stubWriter{err: errors.New("store unavailable")}
	if err := (Publisher{
		Provider:   source.NewFixtureProvider(),
		Classifier: classify.NewAcademicRuleV1(),
		Store:      writer,
		Outcomes:   observer,
	}).Publish(context.Background(), sampleRequest()); err == nil {
		t.Fatal("a store failure was reported as a successful publication")
	}
	if len(observer.levels) != 0 {
		t.Fatalf("the observer recorded %v for a failed publication", observer.levels)
	}
}
