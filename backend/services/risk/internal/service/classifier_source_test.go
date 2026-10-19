// This file covers the seam that makes an activation decide inference: how the
// publication path resolves the classifier it runs from the shared models
// registry, and what it guarantees once it has resolved one.
//
// The bug this closes: the registry recorded an activation (active, approved_at,
// metrics.activation) while the publication path kept the classifier selected at
// startup from MODEL_ID forever, so v1 -> v2 -> v1 changed the registry and
// nothing else. The tests below drive the resolution rules directly and then the
// publication path through the real registry, so "the active version judges new
// batches" is asserted instead of described.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	v1 "firewatch.local/contracts/events/v1"
	"firewatch.local/risk/internal/classify"
	"firewatch.local/risk/internal/repository"
	"firewatch.local/risk/internal/source"
)

// registryWithBothRules registers the two real frozen rule versions exactly as
// the service does at startup, so the resolution rules are exercised against the
// real identifiers and fingerprints rather than against stubs.
func registryWithBothRules(t *testing.T) *classify.Registry {
	t.Helper()
	registry := classify.NewRegistry()
	if err := registry.Register(classify.NewAcademicRuleV1()); err != nil {
		t.Fatalf("register academic-rule-v1: %v", err)
	}
	if err := registry.Register(classify.NewFirewatchRuleV2()); err != nil {
		t.Fatalf("register firewatch-rule-v2: %v", err)
	}
	return registry
}

// activatedRow is the registry row of a version an operator activated: active,
// with the "activation" object ActivateModel writes into metrics.
func activatedRow(version, hash string) repository.ModelVersion {
	return repository.ModelVersion{
		Version:    version,
		Kind:       "rule",
		ParamsHash: &hash,
		Active:     true,
		Metrics:    json.RawMessage(`{"official":false,"activation":{"at":"2026-10-18T12:00:00Z"}}`),
	}
}

// seededRow is the registry row a migration seeds: active, but with no record of
// an operator's activation decision. It expresses the registry's lifecycle, not
// a decision about which classifier must run.
func seededRow(version, hash string) repository.ModelVersion {
	return repository.ModelVersion{
		Version:    version,
		Kind:       "rule",
		ParamsHash: &hash,
		Active:     true,
		Metrics:    json.RawMessage(`{"official":false,"calibrated_against_observed_fires":false}`),
	}
}

// fixedActiveModel answers the registry read with one fixed row, or with a read
// failure, or with "no active row" when the row is empty.
type fixedActiveModel struct {
	model repository.ModelVersion
	err   error
}

func (f fixedActiveModel) ActiveModel(context.Context) (repository.ModelVersion, error) {
	if f.err != nil {
		return repository.ModelVersion{}, f.err
	}
	if f.model.Version == "" {
		return repository.ModelVersion{}, repository.ErrNoActiveModel
	}
	return f.model, nil
}

// switchingActiveModel is the same read with the state a concurrent activation
// changes, so the publication path can be driven while the active version is
// being flipped.
type switchingActiveModel struct {
	mu    sync.Mutex
	model repository.ModelVersion
	err   error
}

func (s *switchingActiveModel) ActiveModel(context.Context) (repository.ModelVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return repository.ModelVersion{}, s.err
	}
	if s.model.Version == "" {
		return repository.ModelVersion{}, repository.ErrNoActiveModel
	}
	return s.model, nil
}

func (s *switchingActiveModel) activate(model repository.ModelVersion) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.model = model
}

// stubTrainedModel is a non-frozen classifier with its own identity, level and
// policy fingerprint, so a persisted batch can be attributed to exactly the
// classifier instance that produced it and to no other version.
type stubTrainedModel struct {
	id    string
	hash  string
	level classify.Level
}

func (s stubTrainedModel) ID() string { return s.id }

func (s stubTrainedModel) Kind() classify.Kind { return classify.KindTrained }

func (s stubTrainedModel) Classify(context.Context, classify.Features) (classify.Assessment, error) {
	return classify.Assessment{
		Level:     s.level,
		Rationale: "trained stub",
		Policy:    classify.PolicyRef{PolicyID: s.id, ParamsHash: s.hash, Parameters: map[string]string{"stub": "true"}},
	}, nil
}

// TestRegistryClassifierSourceSelectsTheExplicitlyActivatedVersion protects the
// core rule: once an operator has activated a version, the next publication must
// run that version, and not whatever MODEL_ID selected at startup.
func TestRegistryClassifierSourceSelectsTheExplicitlyActivatedVersion(t *testing.T) {
	source := RegistryClassifierSource{
		Models:    fixedActiveModel{model: activatedRow(classify.FirewatchRuleV2ID, classify.FirewatchRuleV2ParamsHash)},
		Registry:  registryWithBothRules(t),
		Bootstrap: classify.NewAcademicRuleV1(),
	}
	classifier, err := source.ActiveClassifier(context.Background())
	if err != nil {
		t.Fatalf("resolve the explicitly activated version: %v", err)
	}
	if classifier.ID() != classify.FirewatchRuleV2ID {
		t.Fatalf("resolved classifier is %q, want the explicitly activated %q", classifier.ID(), classify.FirewatchRuleV2ID)
	}
}

// TestRegistryClassifierSourceKeepsTheStartupSelectionWhileTheActiveRowIsOnlyASeed
// pins the documented precedence in the other direction: the row a migration
// seeds is active but records no activation decision, so the startup selection
// (MODEL_ID) still decides. Without this, the seeded academic-rule-v1 row would
// silently override a deployment configured for v2 before anyone activated
// anything.
func TestRegistryClassifierSourceKeepsTheStartupSelectionWhileTheActiveRowIsOnlyASeed(t *testing.T) {
	source := RegistryClassifierSource{
		Models:    fixedActiveModel{model: seededRow(classify.AcademicRuleV1ID, classify.AcademicRuleV1ParamsHash)},
		Registry:  registryWithBothRules(t),
		Bootstrap: classify.NewFirewatchRuleV2(),
	}
	classifier, err := source.ActiveClassifier(context.Background())
	if err != nil {
		t.Fatalf("resolve while only a seeded active row exists: %v", err)
	}
	if classifier.ID() != classify.FirewatchRuleV2ID {
		t.Fatalf("resolved classifier is %q, want the startup selection %q: a seeded active row is not an activation", classifier.ID(), classify.FirewatchRuleV2ID)
	}
}

// TestRegistryClassifierSourceUsesTheBootstrapWhenTheRegistryHasNoActiveRow
// protects the bootstrap half of the precedence: with nothing active at all, the
// startup selection still serves publications.
func TestRegistryClassifierSourceUsesTheBootstrapWhenTheRegistryHasNoActiveRow(t *testing.T) {
	source := RegistryClassifierSource{
		Models:    fixedActiveModel{},
		Registry:  registryWithBothRules(t),
		Bootstrap: classify.NewAcademicRuleV1(),
	}
	classifier, err := source.ActiveClassifier(context.Background())
	if err != nil {
		t.Fatalf("resolve without an active row: %v", err)
	}
	if classifier.ID() != classify.AcademicRuleV1ID {
		t.Fatalf("resolved classifier is %q, want the bootstrap %q", classifier.ID(), classify.AcademicRuleV1ID)
	}
}

// TestRegistryClassifierSourceFailsClosedOnAnUnknownActiveVersion protects the
// first fail-closed rule: an activation that names a version the runtime cannot
// execute must be refused, never silently replaced by the startup selection,
// because publishing under a version nobody ran is exactly the attribution this
// unit exists to keep honest.
func TestRegistryClassifierSourceFailsClosedOnAnUnknownActiveVersion(t *testing.T) {
	const version = "trained-model-v9"
	source := RegistryClassifierSource{
		Models:    fixedActiveModel{model: activatedRow(version, "any-hash")},
		Registry:  registryWithBothRules(t),
		Bootstrap: classify.NewAcademicRuleV1(),
	}
	classifier, err := source.ActiveClassifier(context.Background())
	if err == nil {
		t.Fatal("an active version with no registered classifier was resolved")
	}
	if classifier != nil {
		t.Fatalf("a failed resolution returned a classifier: %q", classifier.ID())
	}
	if !errors.Is(err, ErrClassifierUnresolvable) {
		t.Fatalf("err = %v, want it to declare ErrClassifierUnresolvable", err)
	}
	if !strings.Contains(err.Error(), version) {
		t.Fatalf("the refusal %q does not name the unresolvable version %q", err, version)
	}
}

// TestRegistryClassifierSourceFailsClosedOnAFrozenFingerprintMismatch protects
// the second fail-closed rule: the row the activation points at claims a frozen
// identifier with a fingerprint the running classifier does not produce, so the
// service would stamp a provenance the registry contradicts. The refusal must
// name both hashes.
func TestRegistryClassifierSourceFailsClosedOnAFrozenFingerprintMismatch(t *testing.T) {
	const divergent = "0000000000000000000000000000000000000000000000000000000000000000"
	source := RegistryClassifierSource{
		Models:    fixedActiveModel{model: activatedRow(classify.AcademicRuleV1ID, divergent)},
		Registry:  registryWithBothRules(t),
		Bootstrap: classify.NewAcademicRuleV1(),
	}
	_, err := source.ActiveClassifier(context.Background())
	if err == nil {
		t.Fatal("an activated version with a divergent frozen fingerprint was resolved")
	}
	if !errors.Is(err, ErrClassifierUnresolvable) {
		t.Fatalf("err = %v, want it to declare ErrClassifierUnresolvable", err)
	}
	if !strings.Contains(err.Error(), divergent) || !strings.Contains(err.Error(), classify.AcademicRuleV1ParamsHash) {
		t.Fatalf("the refusal %q does not name both the stored %q and the frozen %q", err, divergent, classify.AcademicRuleV1ParamsHash)
	}
}

// TestRegistryClassifierSourceFailsClosedWithoutAnExecutableDefault protects the
// "absent" half of the fail-closed rule: a registry that names nothing active and
// a deployment that configured no startup classifier must refuse to publish
// rather than let an arbitrary classifier judge the batch. The other two cases
// pin the two collaborators that must be present for an activation to be
// executable.
func TestRegistryClassifierSourceFailsClosedWithoutAnExecutableDefault(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source RegistryClassifierSource
	}{
		{"no active row and no startup classifier", RegistryClassifierSource{Models: fixedActiveModel{}, Registry: registryWithBothRules(t)}},
		{"no registry reader", RegistryClassifierSource{Registry: registryWithBothRules(t), Bootstrap: classify.NewAcademicRuleV1()}},
		{"activation without a classifier registry", RegistryClassifierSource{Models: fixedActiveModel{model: activatedRow(classify.FirewatchRuleV2ID, classify.FirewatchRuleV2ParamsHash)}, Bootstrap: classify.NewAcademicRuleV1()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.source.ActiveClassifier(context.Background())
			if err == nil {
				t.Fatal("an unexecutable classifier configuration was resolved")
			}
			if !errors.Is(err, ErrClassifierUnresolvable) {
				t.Fatalf("err = %v, want it to declare ErrClassifierUnresolvable", err)
			}
		})
	}
}

// TestRegistryClassifierSourceSurfacesARegistryReadFailure is the control for the
// fail-closed rules: an unreadable registry is a transient dependency failure, so
// it must stay retryable instead of being declared unresolvable. Declaring it
// unresolvable would dead-letter every job while the database was merely down.
func TestRegistryClassifierSourceSurfacesARegistryReadFailure(t *testing.T) {
	failure := errors.New("registry unavailable")
	source := RegistryClassifierSource{
		Models:    fixedActiveModel{err: failure},
		Registry:  registryWithBothRules(t),
		Bootstrap: classify.NewAcademicRuleV1(),
	}
	_, err := source.ActiveClassifier(context.Background())
	if !errors.Is(err, failure) {
		t.Fatalf("err = %v, want the registry failure %v", err, failure)
	}
	if errors.Is(err, ErrClassifierUnresolvable) {
		t.Fatalf("a transient registry read failure was declared unresolvable: %v", err)
	}
}

// assessmentTuple is what one persisted batch says about the classifier that
// produced it. Mixing any two of these values would mean one batch carried two
// versions at once.
type assessmentTuple struct {
	version    string
	policyID   string
	paramsHash string
	level      string
}

func tupleOf(assessment repository.Assessment) assessmentTuple {
	tuple := assessmentTuple{version: assessment.ModelVersion, level: assessment.RiskLevel}
	if assessment.Policy != nil {
		tuple.policyID = assessment.Policy.PolicyID
		tuple.paramsHash = assessment.Policy.ParamsHash
	}
	return tuple
}

// publishTuple publishes the built-in fixture once through the given classifier
// source and reports what was persisted.
func publishTuple(t *testing.T, classifiers ClassifierSource) assessmentTuple {
	t.Helper()
	writer := &stubWriter{}
	if err := (Publisher{Provider: source.NewFixtureProvider(), Classifiers: classifiers, Store: writer}).Publish(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("publish through the classifier source: %v", err)
	}
	if writer.calls != 1 {
		t.Fatalf("the writer was called %d times, want exactly 1", writer.calls)
	}
	return tupleOf(writer.assessment)
}

// TestPublisherFollowsActivationAndReversal is the acceptance proof of this unit:
// v1 -> v2 -> v1 in the registry must change the version, the policy identifier
// and the policy fingerprint of the next published batch, and the reversal must
// restore the first tuple exactly.
func TestPublisherFollowsActivationAndReversal(t *testing.T) {
	registry := registryWithBothRules(t)
	reader := &switchingActiveModel{model: activatedRow(classify.AcademicRuleV1ID, classify.AcademicRuleV1ParamsHash)}
	publisher := Publisher{
		Provider:    source.NewFixtureProvider(),
		Classifiers: RegistryClassifierSource{Models: reader, Registry: registry, Bootstrap: classify.NewAcademicRuleV1()},
		Store:       &stubWriter{},
	}
	publish := func(t *testing.T) assessmentTuple {
		t.Helper()
		writer := &stubWriter{}
		publisher.Store = writer
		if err := publisher.Publish(context.Background(), sampleRequest()); err != nil {
			t.Fatalf("publish with the active version %q: %v", reader.model.Version, err)
		}
		return tupleOf(writer.assessment)
	}

	before := publish(t)
	if before.version != classify.AcademicRuleV1ID || before.paramsHash != classify.AcademicRuleV1ParamsHash {
		t.Fatalf("with v1 activated the batch carries %+v, want %q with the v1 fingerprint", before, classify.AcademicRuleV1ID)
	}

	reader.activate(activatedRow(classify.FirewatchRuleV2ID, classify.FirewatchRuleV2ParamsHash))
	after := publish(t)
	if after.version != classify.FirewatchRuleV2ID || after.policyID != classify.FirewatchRuleV2ID || after.paramsHash != classify.FirewatchRuleV2ParamsHash {
		t.Fatalf("after activating v2 the batch carries %+v, want v2 with its own policy and fingerprint", after)
	}
	if after == before {
		t.Fatalf("activating v2 did not change the published batch: %+v", after)
	}

	reader.activate(activatedRow(classify.AcademicRuleV1ID, classify.AcademicRuleV1ParamsHash))
	reverted := publish(t)
	if reverted != before {
		t.Fatalf("after reverting to v1 the batch carries %+v, want the original %+v", reverted, before)
	}
}

// TestPublisherNeverStampsARegistryVersionOnAnotherClassifiersResult protects the
// attribution rule: the batch records what the classifier that produced it
// reported, never the registry row's version or fingerprint. Here the row and the
// classifier disagree on purpose, and the classifier's own values must win.
func TestPublisherNeverStampsARegistryVersionOnAnotherClassifiersResult(t *testing.T) {
	const version = "trained-model-v1"
	const rowHash = "2222222222222222222222222222222222222222222222222222222222222222"
	producer := stubTrainedModel{id: version, hash: "1111111111111111111111111111111111111111111111111111111111111111", level: classify.LevelBajo}
	registry := classify.NewRegistry()
	if err := registry.Register(producer); err != nil {
		t.Fatalf("register the trained stub: %v", err)
	}
	writer := &stubWriter{}
	err := (Publisher{
		Provider:    source.NewFixtureProvider(),
		Classifiers: RegistryClassifierSource{Models: fixedActiveModel{model: activatedRow(version, rowHash)}, Registry: registry, Bootstrap: classify.NewAcademicRuleV1()},
		Store:       writer,
	}).Publish(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("publish through the activated trained model: %v", err)
	}
	if writer.assessment.ModelVersion != version {
		t.Fatalf("batch version is %q, want the producing classifier %q", writer.assessment.ModelVersion, version)
	}
	if writer.assessment.Policy == nil || writer.assessment.Policy.PolicyID != version {
		t.Fatalf("batch policy is %+v, want policy id %q from the producing classifier", writer.assessment.Policy, version)
	}
	if writer.assessment.Policy.ParamsHash != producer.hash {
		t.Fatalf("batch carries params hash %q, want the producing classifier's %q and never the registry row's %q", writer.assessment.Policy.ParamsHash, producer.hash, rowHash)
	}
	if writer.assessment.RiskLevel != string(classify.LevelBajo) {
		t.Fatalf("batch level is %q, want the level the resolved classifier produced (%q)", writer.assessment.RiskLevel, classify.LevelBajo)
	}
}

// TestPublisherFailsClosedOnAnUnresolvableActiveClassifier protects the
// publication side of the fail-closed rule: an activation the runtime cannot
// execute must refuse the publication before anything is fetched or stored, and
// it must declare itself permanent because no retry can register a classifier in
// a running binary. The control keeps a registry read failure retryable.
func TestPublisherFailsClosedOnAnUnresolvableActiveClassifier(t *testing.T) {
	registry := registryWithBothRules(t)
	unknown := Publisher{
		Provider:    source.NewFixtureProvider(),
		Classifiers: RegistryClassifierSource{Models: fixedActiveModel{model: activatedRow("trained-model-v9", "any-hash")}, Registry: registry, Bootstrap: classify.NewAcademicRuleV1()},
		Store:       &stubWriter{},
	}
	writer := unknown.Store.(*stubWriter)
	err := unknown.Publish(context.Background(), sampleRequest())
	if err == nil {
		t.Fatal("a publication ran under an unresolvable active version")
	}
	if writer.calls != 0 {
		t.Fatalf("the writer was reached %d times for an unresolvable active version", writer.calls)
	}
	var permanent interface{ Permanent() bool }
	if !errors.As(err, &permanent) || !permanent.Permanent() {
		t.Fatalf("err = %v must declare itself permanent", err)
	}

	transient := Publisher{
		Provider:    source.NewFixtureProvider(),
		Classifiers: RegistryClassifierSource{Models: fixedActiveModel{err: errors.New("registry unavailable")}, Registry: registry, Bootstrap: classify.NewAcademicRuleV1()},
		Store:       &stubWriter{},
	}
	transientWriter := transient.Store.(*stubWriter)
	err = transient.Publish(context.Background(), sampleRequest())
	if err == nil {
		t.Fatal("a publication ran while the registry was unreadable")
	}
	if transientWriter.calls != 0 {
		t.Fatalf("the writer was reached %d times while the registry was unreadable", transientWriter.calls)
	}
	if errors.As(err, &permanent) && permanent.Permanent() {
		t.Fatalf("a transient registry failure was declared permanent: %v", err)
	}
}

// recordingWriter collects every persisted assessment so the concurrent test can
// inspect them after the race is over.
type recordingWriter struct {
	mu          sync.Mutex
	assessments []repository.Assessment
}

func (w *recordingWriter) Publish(_ context.Context, _ v1.Event, assessment repository.Assessment) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.assessments = append(w.assessments, assessment)
	return nil
}

func (w *recordingWriter) tuples() []assessmentTuple {
	w.mu.Lock()
	defer w.mu.Unlock()
	tuples := make([]assessmentTuple, 0, len(w.assessments))
	for _, assessment := range w.assessments {
		tuples = append(tuples, tupleOf(assessment))
	}
	return tuples
}

// TestPublisherTakesOneClassifierSnapshotUnderConcurrentActivation protects the
// concurrency rule: a publication resolves the classifier ONCE and uses that one
// instance for the classification, the recorded version and the policy, so an
// activation racing with a publication can change what the NEXT batch uses but
// can never produce a batch that mixes two versions. Every persisted tuple must
// equal one of the two single-version references; a tuple carrying v2's version
// with v1's fingerprint or level would fail here.
func TestPublisherTakesOneClassifierSnapshotUnderConcurrentActivation(t *testing.T) {
	registry := registryWithBothRules(t)
	v1Row := activatedRow(classify.AcademicRuleV1ID, classify.AcademicRuleV1ParamsHash)
	v2Row := activatedRow(classify.FirewatchRuleV2ID, classify.FirewatchRuleV2ParamsHash)
	referenceV1 := publishTuple(t, RegistryClassifierSource{Models: fixedActiveModel{model: v1Row}, Registry: registry, Bootstrap: classify.NewAcademicRuleV1()})
	referenceV2 := publishTuple(t, RegistryClassifierSource{Models: fixedActiveModel{model: v2Row}, Registry: registry, Bootstrap: classify.NewAcademicRuleV1()})
	if referenceV1 == referenceV2 {
		t.Fatalf("the two versions produced the same persisted tuple %+v, so this proof cannot tell them apart", referenceV1)
	}

	reader := &switchingActiveModel{model: v1Row}
	writer := &recordingWriter{}
	publisher := Publisher{
		Provider:    source.NewFixtureProvider(),
		Classifiers: RegistryClassifierSource{Models: reader, Registry: registry, Bootstrap: classify.NewAcademicRuleV1()},
		Store:       writer,
	}
	const publications = 48
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for index := 0; index < publications; index++ {
			if index%2 == 0 {
				reader.activate(v2Row)
				continue
			}
			reader.activate(v1Row)
		}
	}()
	go func() {
		defer wg.Done()
		for index := 0; index < publications; index++ {
			if err := publisher.Publish(context.Background(), sampleRequest()); err != nil {
				t.Errorf("publication %d under concurrent activation: %v", index, err)
			}
		}
	}()
	wg.Wait()

	tuples := writer.tuples()
	if len(tuples) != publications {
		t.Fatalf("%d batches were persisted, want %d", len(tuples), publications)
	}
	for _, tuple := range tuples {
		if tuple != referenceV1 && tuple != referenceV2 {
			t.Fatalf("a publication mixed metadata from two classifier versions: %+v is neither %+v nor %+v", tuple, referenceV1, referenceV2)
		}
	}
}

// TestPublisherAppliesTheActivatedClassifierToScheduledPublications is the seam
// proof between the two fixes this integration carries. The automatic cycle
// publishes through the scheduled sink, so no completion is emitted for a job the
// cycle never requested; and the classifier that serves a publication is the
// version the registry activated, so an activation decides inference without a
// restart. Each fix was verified on its own unit, so this test pins their
// composition: one activation must change the version, the policy identifier and
// the policy fingerprint of the next SCHEDULED batch, the scheduled sink must be
// the only seam reached, and the completion seam must stay untouched. A shared
// publication path that resolved the classifier once satisfies the activation
// half; resolving it while publishing through the scheduled sink satisfies the
// other, and this test fails if either is dropped.
func TestPublisherAppliesTheActivatedClassifierToScheduledPublications(t *testing.T) {
	registry := registryWithBothRules(t)
	reader := &switchingActiveModel{model: activatedRow(classify.AcademicRuleV1ID, classify.AcademicRuleV1ParamsHash)}
	writer := &stubWriter{}
	publisher := Publisher{
		Provider:    source.NewFixtureProvider(),
		Classifiers: RegistryClassifierSource{Models: reader, Registry: registry, Bootstrap: classify.NewAcademicRuleV1()},
		Store:       writer,
	}

	scheduled := func(t *testing.T) assessmentTuple {
		t.Helper()
		if err := publisher.PublishScheduled(context.Background(), sampleRequest()); err != nil {
			t.Fatalf("publish the fixture through the scheduled seam: %v", err)
		}
		return tupleOf(writer.scheduledAssessment)
	}

	before := scheduled(t)
	if before.version != classify.AcademicRuleV1ID || before.policyID != classify.AcademicRuleV1ID || before.paramsHash != classify.AcademicRuleV1ParamsHash {
		t.Fatalf("with v1 activated the scheduled batch carries %+v, want v1 with its own policy and fingerprint", before)
	}

	reader.activate(activatedRow(classify.FirewatchRuleV2ID, classify.FirewatchRuleV2ParamsHash))
	after := scheduled(t)
	if after.version != classify.FirewatchRuleV2ID || after.policyID != classify.FirewatchRuleV2ID || after.paramsHash != classify.FirewatchRuleV2ParamsHash {
		t.Fatalf("after activating v2 the scheduled batch carries %+v, want v2 with its own policy and fingerprint", after)
	}
	if after == before {
		t.Fatalf("activating v2 did not change the scheduled batch: %+v", after)
	}

	reader.activate(activatedRow(classify.AcademicRuleV1ID, classify.AcademicRuleV1ParamsHash))
	reverted := scheduled(t)
	if reverted != before {
		t.Fatalf("after reverting to v1 the scheduled batch carries %+v, want the original %+v", reverted, before)
	}

	// Every publication above went through the scheduled seam, so the automatic
	// cycle emitted no completion event for a job it never requested. Asserted
	// last so the counter describes the whole sequence.
	if writer.scheduled != 3 || writer.calls != 0 {
		t.Fatalf("scheduled=%d completion=%d, want the scheduled seam exactly 3 times and the completion seam never", writer.scheduled, writer.calls)
	}
}
