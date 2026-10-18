package classify

import (
	"context"
	"fmt"
	"time"
)

// FirewatchRuleV2ID is the identifier recorded against every assessment the
// second rule produces. It is a distinct identity from academic-rule-v1, so a
// stored level states which rule version judged it.
const FirewatchRuleV2ID = "firewatch-rule-v2"

// FirewatchRuleV2 is the second deterministic rule implementation. It shares the
// Params struct and the frozen-identity machinery with academic-rule-v1, but it
// is a genuinely different rule: the level is driven by the single largest
// weighted contribution (the dominant factor) rather than by the weighted sum.
//
// A versioned rule exists so a behaviour change is a new, immutable identity
// instead of an edit of a frozen version. It carries its own parameter set, its
// own fingerprint and its own identifier; the registry registers both, and
// selecting one never substitutes the other.
type FirewatchRuleV2 struct {
	params     Params
	configured bool
}

// newFirewatchRuleV2 is the single construction path. The zero value stays
// unconfigured on purpose, so FirewatchRuleV2{} fails closed in Classify instead
// of producing a level from an all-zero parameter set.
func newFirewatchRuleV2(params Params) FirewatchRuleV2 {
	return FirewatchRuleV2{params: params, configured: true}
}

// NewFirewatchRuleV2 returns the rule with the frozen v2 parameters.
func NewFirewatchRuleV2() FirewatchRuleV2 { return newFirewatchRuleV2(DefaultFirewatchRuleV2Params()) }

// NewFirewatchRuleV2WithParams returns the rule with an explicit parameter set.
// It is the testing seam; production uses the frozen default.
func NewFirewatchRuleV2WithParams(params Params) (FirewatchRuleV2, error) {
	if err := params.validate(); err != nil {
		return FirewatchRuleV2{}, err
	}
	return newFirewatchRuleV2(params), nil
}

// ID implements Classifier.
func (FirewatchRuleV2) ID() string { return FirewatchRuleV2ID }

// Kind implements Classifier.
func (FirewatchRuleV2) Kind() Kind { return KindRule }

// PolicyFingerprint implements PolicyFingerprinter, so the registry can refuse a
// classifier that claims the v2 identifier while reporting a different policy.
func (r FirewatchRuleV2) PolicyFingerprint() (string, error) {
	if !r.configured {
		return "", fmt.Errorf("classify: %s was not constructed with NewFirewatchRuleV2 or NewFirewatchRuleV2WithParams: it cannot report a policy", r.ID())
	}
	return r.params.Hash()
}

// Classify implements Classifier.
//
// It applies the same fail-closed safety contract as academic-rule-v1 - absent
// input, missing instants and stale data are all undetermined, never low risk -
// but computes the score from the dominant weighted factor. Every non-error path
// carries the v2 PolicyRef of the instance actually used.
func (r FirewatchRuleV2) Classify(ctx context.Context, features Features) (Assessment, error) {
	if !r.configured {
		return Assessment{}, fmt.Errorf("classify: %s was not constructed with NewFirewatchRuleV2 or NewFirewatchRuleV2WithParams: its parameter set is unset", r.ID())
	}
	if err := ctx.Err(); err != nil {
		return Assessment{}, err
	}
	policy, err := r.params.PolicyRefFor(r.ID())
	if err != nil {
		return Assessment{}, err
	}
	if nonFinite := features.nonFinite(); len(nonFinite) > 0 {
		return Assessment{
			Level:     LevelDesconocido,
			Factors:   r.params.factorsOf(features),
			Rationale: nonFiniteRationale(nonFinite),
			Policy:    policy,
		}, nil
	}
	if features.ObservedAt.IsZero() || features.EvaluatedAt.IsZero() {
		return Assessment{
			Level:     LevelDesconocido,
			Factors:   r.params.factorsOf(features),
			Rationale: "No se puede clasificar sin instante de observación y de evaluación: el dato está incompleto.",
			Policy:    policy,
		}, nil
	}
	if missing := features.Missing(); len(missing) > 0 {
		return Assessment{
			Level:     LevelDesconocido,
			Factors:   r.params.factorsOf(features),
			Rationale: missingRationale(missing),
			Policy:    policy,
		}, nil
	}
	age := features.EvaluatedAt.Sub(features.ObservedAt)
	maxStaleness := r.params.maxStaleness()
	if age > maxStaleness {
		return Assessment{
			Level:   LevelDesconocido,
			Factors: r.params.factorsOf(features),
			Rationale: fmt.Sprintf(
				"El dato tiene %s de antigüedad y supera el máximo de %s: un dato vencido no se clasifica. Valor desconocido, nunca riesgo bajo.",
				age.Round(time.Minute), maxStaleness,
			),
			Policy: policy,
		}, nil
	}
	if age < 0 {
		return Assessment{
			Level:     LevelDesconocido,
			Factors:   r.params.factorsOf(features),
			Rationale: "El instante de observación es posterior al de evaluación: el dato es inconsistente y no se clasifica.",
			Policy:    policy,
		}, nil
	}
	score := r.params.scoreV2(features)
	level := r.params.levelFor(score)
	return Assessment{
		Level:     level,
		Factors:   r.params.factorsOf(features),
		Rationale: levelRationale(score, level),
		Policy:    policy,
	}, nil
}

// scoreV2 is the dominant-factor combination: the level is driven by the single
// largest weighted contribution rather than by their sum. It uses only the
// structural literals the package allows, so it cannot hide a behaviour number
// outside the frozen parameter set.
func (p Params) scoreV2(features Features) float64 {
	var score float64
	contributions := []float64{
		p.WeightTemperature * rising(features.TemperatureC.Value, p.TemperatureLowC, p.TemperatureHighC),
		p.WeightHumidity * falling(features.RelativeHumidityPct.Value, p.HumidityDryPct, p.HumidityWetPct),
		p.WeightWind * rising(features.WindSpeedKmh.Value, p.WindLowKmh, p.WindHighKmh),
		p.WeightVegetation * rising(features.VegetationCoverPct.Value, p.VegetationSparsePct, p.VegetationDensePct),
		p.WeightHistorical * rising(features.HistoricalFires.Value, p.HistoricalFiresLow, p.HistoricalFiresHigh),
	}
	for _, contribution := range contributions {
		if contribution > score {
			score = contribution
		}
	}
	return score
}
