package classify

import (
	"context"
	"reflect"
	"testing"
)

// T5: the second rule version is a real, deterministic Classifier with its own
// identity and versioned parameters. It is registered next to academic-rule-v1,
// and selecting one never substitutes the other. The rule itself lives in
// firewatch_rule.go.
func TestFirewatchRuleV2IsRegisteredAlongsideAcademicRuleV1(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(NewAcademicRuleV1()); err != nil {
		t.Fatalf("register academic-rule-v1: %v", err)
	}
	if err := registry.Register(NewFirewatchRuleV2()); err != nil {
		t.Fatalf("register firewatch-rule-v2: %v", err)
	}
	if got, want := registry.IDs(), []string{AcademicRuleV1ID, FirewatchRuleV2ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("registered ids = %v, want %v", got, want)
	}
	v1, err := registry.Select(AcademicRuleV1ID)
	if err != nil || v1.ID() != AcademicRuleV1ID {
		t.Fatalf("select academic-rule-v1 = (%v, %v)", v1, err)
	}
	v2, err := registry.Select(FirewatchRuleV2ID)
	if err != nil || v2.ID() != FirewatchRuleV2ID {
		t.Fatalf("select firewatch-rule-v2 = (%v, %v)", v2, err)
	}
	if v1.ID() == v2.ID() {
		t.Fatal("the two versions share an identity")
	}
}

// T5: the frozen golden guard: any change to the v2 parameters must break the
// build instead of silently redefining the behaviour behind the version name.
func TestFirewatchRuleV2ParametersAreFrozen(t *testing.T) {
	hash, err := DefaultFirewatchRuleV2Params().Hash()
	if err != nil {
		t.Fatalf("hash the v2 parameters: %v", err)
	}
	if hash != FirewatchRuleV2ParamsHash {
		t.Fatalf("firewatch-rule-v2 parameters hash = %q, want frozen %q", hash, FirewatchRuleV2ParamsHash)
	}
	if hash == AcademicRuleV1ParamsHash {
		t.Fatal("firewatch-rule-v2 reuses the academic-rule-v1 fingerprint, so the versions are not distinct")
	}
}

// T5: deterministic and self-describing. The same Features always produce the
// same Assessment, and the assessment carries the v2 policy reference.
func TestFirewatchRuleV2IsDeterministic(t *testing.T) {
	rule := NewFirewatchRuleV2()
	features := midRiskFeatures()
	first, err := rule.Classify(context.Background(), features)
	if err != nil {
		t.Fatalf("first classify: %v", err)
	}
	second, err := rule.Classify(context.Background(), features)
	if err != nil {
		t.Fatalf("second classify: %v", err)
	}
	if first.Level != second.Level || first.Rationale != second.Rationale || !reflect.DeepEqual(first.Factors, second.Factors) {
		t.Fatalf("v2 is not deterministic: %+v vs %+v", first, second)
	}
	if first.Policy.PolicyID != FirewatchRuleV2ID || first.Policy.ParamsHash != FirewatchRuleV2ParamsHash {
		t.Fatalf("v2 policy reference = %+v", first.Policy)
	}
	if err := first.Validate(); err != nil {
		t.Fatalf("v2 assessment is not publishable: %v", err)
	}
}

// T5: the v2 rule keeps the same safety contract as v1: an absent input is
// undetermined, never low risk.
func TestFirewatchRuleV2AbstainsOnMissingInput(t *testing.T) {
	features := midRiskFeatures()
	features.RelativeHumidityPct = Unknown()
	assessment, err := NewFirewatchRuleV2().Classify(context.Background(), features)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if assessment.Level != LevelDesconocido {
		t.Fatalf("an absent humidity produced %q, want %q", assessment.Level, LevelDesconocido)
	}
}

// T5: the version reports the fingerprint it actually stamps, which is what the
// registry's frozen identity check consumes.
func TestFirewatchRuleV2PolicyFingerprintMatchesFrozen(t *testing.T) {
	hash, err := NewFirewatchRuleV2().PolicyFingerprint()
	if err != nil {
		t.Fatalf("policy fingerprint: %v", err)
	}
	if hash != FirewatchRuleV2ParamsHash {
		t.Fatalf("v2 fingerprint = %q, want frozen %q", hash, FirewatchRuleV2ParamsHash)
	}
	if _, err := (FirewatchRuleV2{}).PolicyFingerprint(); err == nil {
		t.Fatal("the zero-value v2 reported a policy fingerprint")
	}
}

// T5: "una no reemplaza a la otra por defecto". The two versions are reachable
// under their own identifiers, and the registry refuses a second registration of
// the same identifier, so a selection cannot be silently remapped.
func TestFirewatchRuleV2DoesNotReplaceAcademicRuleV1(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(NewAcademicRuleV1()); err != nil {
		t.Fatalf("register v1: %v", err)
	}
	v2, err := NewFirewatchRuleV2WithParams(DefaultFirewatchRuleV2Params())
	if err != nil {
		t.Fatalf("build v2: %v", err)
	}
	if err := registry.Register(v2); err != nil {
		t.Fatalf("register v2: %v", err)
	}
	if err := registry.Register(NewFirewatchRuleV2()); err == nil {
		t.Fatal("a duplicate v2 identifier was accepted")
	}
	v1, err := registry.Select(AcademicRuleV1ID)
	if err != nil {
		t.Fatalf("v1 disappeared after registering v2: %v", err)
	}
	if v1.ID() != AcademicRuleV1ID {
		t.Fatalf("the v1 slot now holds %q", v1.ID())
	}
}
