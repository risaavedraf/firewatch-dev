package classify

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// stubTrainedModel stands in for a future trained model. It exists to prove the
// fallback is a drop-in: the academic rule and a trained model satisfy the same
// interface and are selected by identifier, so switching between them is a
// configuration change rather than a refactor.
type stubTrainedModel struct{}

func (stubTrainedModel) ID() string { return "trained-model-v1" }

func (stubTrainedModel) Kind() Kind { return KindTrained }

func (stubTrainedModel) Classify(context.Context, Features) (Assessment, error) {
	return Assessment{Level: LevelMedio, Rationale: "stub"}, nil
}

func TestRegistryHoldsRuleAndTrainedModelInterchangeably(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(NewAcademicRuleV1()); err != nil {
		t.Fatalf("registering the rule failed: %v", err)
	}
	if err := registry.Register(stubTrainedModel{}); err != nil {
		t.Fatalf("registering the trained model failed: %v", err)
	}

	rule, err := registry.Select(AcademicRuleV1ID)
	if err != nil {
		t.Fatalf("selecting the rule failed: %v", err)
	}
	if rule.Kind() != KindRule {
		t.Fatalf("rule kind is %q, want %q", rule.Kind(), KindRule)
	}

	model, err := registry.Select("trained-model-v1")
	if err != nil {
		t.Fatalf("selecting the trained model failed: %v", err)
	}
	if model.Kind() != KindTrained {
		t.Fatalf("model kind is %q, want %q", model.Kind(), KindTrained)
	}

	if got, want := registry.IDs(), []string{AcademicRuleV1ID, "trained-model-v1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("registry ids are %v, want %v", got, want)
	}
}

func TestRegistryRejectsDuplicatesEmptyAndUnknown(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(NewAcademicRuleV1()); err != nil {
		t.Fatalf("registering failed: %v", err)
	}
	if err := registry.Register(NewAcademicRuleV1()); err == nil {
		t.Fatal("duplicate classifier was accepted")
	}
	if err := registry.Register(nil); err == nil {
		t.Fatal("nil classifier was accepted")
	}
	if err := NewRegistry().Register(stubTrainedModel{}); err != nil {
		t.Fatalf("registering failed: %v", err)
	}
	if _, err := registry.Select("no-existe"); !errors.Is(err, ErrUnknownClassifier) {
		t.Fatalf("Select on an unknown id returned %v, want ErrUnknownClassifier", err)
	}
}

// impersonatingRule claims the frozen academic-rule-v1 identifier but reports a
// different policy fingerprint: the exact shape of an impersonation, one name
// over a different behaviour.
type impersonatingRule struct{}

func (impersonatingRule) ID() string { return AcademicRuleV1ID }

func (impersonatingRule) Kind() Kind { return KindRule }

func (impersonatingRule) Classify(context.Context, Features) (Assessment, error) {
	return Assessment{Level: LevelBajo, Policy: PolicyRef{PolicyID: AcademicRuleV1ID, ParamsHash: "0000"}}, nil
}

func (impersonatingRule) PolicyFingerprint() (string, error) { return "0000", nil }

// silentRule claims the frozen identifier but cannot report a policy at all.
type silentRule struct{}

func (silentRule) ID() string { return AcademicRuleV1ID }

func (silentRule) Kind() Kind { return KindRule }

func (silentRule) Classify(context.Context, Features) (Assessment, error) {
	return Assessment{}, nil
}

// A frozen identifier is a promise about the behaviour behind it, so the registry
// refuses any classifier that would attach it to a different policy. A new
// identifier is not constrained.
func TestRegistryRefusesAnImpersonatingFrozenIdentifier(t *testing.T) {
	if err := NewRegistry().Register(AcademicRuleV1{}); err == nil {
		t.Fatal("the zero-value rule was registered under the frozen identifier")
	}
	if err := NewRegistry().Register(impersonatingRule{}); err == nil {
		t.Fatal("a classifier reporting a different fingerprint was registered under the frozen identifier")
	}
	if err := NewRegistry().Register(silentRule{}); err == nil {
		t.Fatal("a classifier that cannot report a policy was registered under the frozen identifier")
	}

	// The real default rule is the control: it is accepted.
	if err := NewRegistry().Register(NewAcademicRuleV1()); err != nil {
		t.Fatalf("the real rule was refused: %v", err)
	}

	// An injected, non-frozen parameter set cannot claim the frozen identifier
	// either, because its fingerprint differs from the frozen one.
	injected := DefaultAcademicRuleV1Params()
	injected.MaxStalenessSeconds = 60
	injectedRule, err := NewAcademicRuleV1WithParams(injected)
	if err != nil {
		t.Fatalf("build the injected rule: %v", err)
	}
	if err := NewRegistry().Register(injectedRule); err == nil {
		t.Fatal("a rule with a non-frozen parameter set was registered under the frozen identifier")
	}

	// A future trained model under a new identifier is still accepted, with or
	// without a reported policy.
	if err := NewRegistry().Register(stubTrainedModel{}); err != nil {
		t.Fatalf("a classifier under a new identifier was refused: %v", err)
	}
}

// The frozen expectation is exported as a copy so the identity check and the
// frozen-params test share one source without any caller being able to mutate it.
func TestFrozenPolicyFingerprintsMatchTheFrozenParamsHash(t *testing.T) {
	frozen := FrozenPolicyFingerprints()
	if got, want := frozen[AcademicRuleV1ID], AcademicRuleV1ParamsHash; got != want {
		t.Fatalf("frozen fingerprint for %q is %q, want %q", AcademicRuleV1ID, got, want)
	}
	frozen[AcademicRuleV1ID] = "mutated"
	if got := FrozenPolicyFingerprints()[AcademicRuleV1ID]; got != AcademicRuleV1ParamsHash {
		t.Fatalf("mutating the returned map changed the frozen expectation to %q", got)
	}
}

func TestExpectedPolicyHashReportsTheFrozenHashAndUnknownIDs(t *testing.T) {
	// The startup path reads the frozen expectation through this lookup instead of
	// duplicating the constant, so it must report the same value the registry
	// identity check uses, and must not invent one for an identifier that has no
	// frozen promise.
	if got, ok := ExpectedPolicyHash(AcademicRuleV1ID); !ok || got != AcademicRuleV1ParamsHash {
		t.Fatalf("ExpectedPolicyHash(%q) = (%q, %v), want (%q, true)", AcademicRuleV1ID, got, ok, AcademicRuleV1ParamsHash)
	}
	if got, ok := ExpectedPolicyHash("trained-model-v1"); ok || got != "" {
		t.Fatalf("ExpectedPolicyHash on an unknown id = (%q, %v), want (\"\", false)", got, ok)
	}
}

// VerifyRegisteredModel is the pure startup check. It must refuse a selected
// version that has no registry row at all, accept a registered version whose
// stored hash matches the frozen one, and refuse a stored hash that diverges
// while naming both hashes.
func TestVerifyRegisteredModelFailsClosed(t *testing.T) {
	if err := VerifyRegisteredModel(AcademicRuleV1ID, false, ""); err == nil {
		t.Fatal("a selected version absent from the models registry was accepted")
	}
	if err := VerifyRegisteredModel(AcademicRuleV1ID, true, AcademicRuleV1ParamsHash); err != nil {
		t.Fatalf("a registered version with the frozen hash was refused: %v", err)
	}

	divergent := "0000000000000000000000000000000000000000000000000000000000000000"
	err := VerifyRegisteredModel(AcademicRuleV1ID, true, divergent)
	if err == nil {
		t.Fatal("a registered version whose stored hash diverges was accepted")
	}
	message := err.Error()
	if !strings.Contains(message, AcademicRuleV1ParamsHash) || !strings.Contains(message, divergent) {
		t.Fatalf("divergence error %q must name both hashes (%q and %q)", message, AcademicRuleV1ParamsHash, divergent)
	}

	// A version with no frozen expectation is accepted once registered: a future
	// trained model carries metrics, not a frozen rule fingerprint.
	if err := VerifyRegisteredModel("trained-model-v1", true, "whatever"); err != nil {
		t.Fatalf("a registered, non-frozen version was refused: %v", err)
	}
}

func TestLevelsAreCanonical(t *testing.T) {
	want := []Level{LevelBajo, LevelMedio, LevelAlto, LevelDesconocido}
	if got := Levels(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Levels() = %v, want %v", got, want)
	}
	for _, level := range want {
		if !level.Canonical() {
			t.Errorf("level %q is not reported as canonical", level)
		}
	}
	for _, level := range []Level{"", "critico", "bajo ", "BAJO"} {
		if level.Canonical() {
			t.Errorf("level %q is wrongly reported as canonical", level)
		}
	}
}

func TestAssessmentValidateRefusesUndefinedLevel(t *testing.T) {
	policy := PolicyRef{PolicyID: AcademicRuleV1ID, ParamsHash: AcademicRuleV1ParamsHash}
	if err := (Assessment{Level: LevelAlto, Policy: policy}).Validate(); err != nil {
		t.Fatalf("valid assessment rejected: %v", err)
	}
	if err := (Assessment{Level: "", Policy: policy}).Validate(); err == nil {
		t.Fatal("assessment with an empty level was accepted")
	}
	if err := (Assessment{Level: "critico", Policy: policy}).Validate(); err == nil {
		t.Fatal("assessment with an undefined level was accepted")
	}
}

// A stored level is only reproducible if it carries the policy that produced it,
// so an assessment without a policy id or without a parameter hash is refused
// even when its level is canonical.
func TestAssessmentValidateRefusesMissingPolicyProvenance(t *testing.T) {
	if err := (Assessment{Level: LevelAlto, Policy: PolicyRef{ParamsHash: AcademicRuleV1ParamsHash}}).Validate(); err == nil {
		t.Fatal("assessment with an empty policy id was accepted")
	}
	if err := (Assessment{Level: LevelAlto, Policy: PolicyRef{PolicyID: AcademicRuleV1ID}}).Validate(); err == nil {
		t.Fatal("assessment with an empty params hash was accepted")
	}
	if err := (Assessment{Level: LevelAlto}).Validate(); err == nil {
		t.Fatal("assessment with no policy at all was accepted")
	}
}

// An absent reading and a reading of zero are different facts, so absence is
// carried explicitly rather than as a zero value.
func TestAbsenceIsDistinctFromZero(t *testing.T) {
	zero := Known(0)
	absent := Unknown()
	if !zero.Present {
		t.Fatal("a known zero is reported as absent")
	}
	if absent.Present {
		t.Fatal("an absent feature is reported as present")
	}
	if zero.Value != 0 || absent.Value != 0 {
		t.Fatal("fixture assumption about zero values no longer holds")
	}
}

func TestFeaturesReportMissingNamesInStableOrder(t *testing.T) {
	features := Features{}
	want := []string{"temperatura", "humedad_relativa", "viento", "cobertura_vegetal", "incendios_historicos"}
	if got := features.Missing(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Missing() = %v, want %v", got, want)
	}
	features.TemperatureC = Known(20)
	if got := features.Missing(); len(got) != 4 || got[0] != "humedad_relativa" {
		t.Fatalf("Missing() = %v after supplying temperature", got)
	}
}
