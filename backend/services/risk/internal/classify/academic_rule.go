package classify

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

// AcademicRuleV1ID is the identifier recorded against every assessment this rule
// produces.
const AcademicRuleV1ID = "academic-rule-v1"

// AcademicRuleMaxStaleness is the frozen default staleness threshold, derived
// from DefaultAcademicRuleV1Params. It is not a second source of truth.
var AcademicRuleMaxStaleness = time.Duration(DefaultAcademicRuleV1Params().MaxStalenessSeconds) * time.Second

// AcademicRuleV1 is a documented academic scoring rule, offered as our own
// non-official rule.
//
// It carries its parameter set as data and reads its behaviour numbers from that
// instance, so an assessment's level and its reported fingerprint describe the
// same object. What IS guaranteed: the default parameters are frozen by
// AcademicRuleV1ParamsHash, and (once the frozen behaviour table lands) the
// produced levels are frozen by a golden table. What is NOT guaranteed: that
// every behaviour number is unreachable from outside the Params object. A
// literal outside it - or an operator change that uses no literal - can still
// move a level, which is why the structural guard in params_test.go only reduces
// the attack surface; it is not the lock.
type AcademicRuleV1 struct {
	params Params

	// configured records that the rule came through newAcademicRuleV1, the only
	// path that also supplies a parameter set. The zero value stays unconfigured
	// on purpose, so AcademicRuleV1{} fails closed in Classify instead of
	// producing a level from an all-zero parameter set. The flag lives here and
	// NEVER in Params: deriveParameters walks every Params field, so a field
	// there would change the frozen hash.
	configured bool
}

// newAcademicRuleV1 is the single construction path for the rule. It sets the
// construction flag that distinguishes a real instance from the zero value.
func newAcademicRuleV1(params Params) AcademicRuleV1 {
	return AcademicRuleV1{params: params, configured: true}
}

// NewAcademicRuleV1 returns the rule with the frozen default parameters.
func NewAcademicRuleV1() AcademicRuleV1 { return newAcademicRuleV1(DefaultAcademicRuleV1Params()) }

// NewAcademicRuleV1WithParams returns a rule with an explicit parameter set. It is
// the testing seam; production uses the frozen default.
//
// It rejects only parameter sets for which classification is undefined: a
// non-finite float, or a negative staleness threshold. Every other value is taken
// as given, so a test can exercise unusual but well-defined rules.
func NewAcademicRuleV1WithParams(params Params) (AcademicRuleV1, error) {
	if err := params.validate(); err != nil {
		return AcademicRuleV1{}, err
	}
	return newAcademicRuleV1(params), nil
}

// ID implements Classifier.
func (AcademicRuleV1) ID() string { return AcademicRuleV1ID }

// Kind implements Classifier.
func (AcademicRuleV1) Kind() Kind { return KindRule }

// PolicyFingerprint implements PolicyFingerprinter: it reports the exact
// parameter fingerprint this instance stamps on its assessments. The registry
// uses it to refuse a classifier that claims a frozen identifier while reporting
// a different policy. It reports nothing for the zero value, whose parameter set
// is unset.
func (r AcademicRuleV1) PolicyFingerprint() (string, error) {
	if !r.configured {
		return "", fmt.Errorf("classify: %s was not constructed with NewAcademicRuleV1 or NewAcademicRuleV1WithParams: it cannot report a policy", r.ID())
	}
	return r.params.Hash()
}

// Classify implements Classifier.
//
// It refuses to produce a level in three situations, and reports which one:
// an input is absent, an instant is missing, or the observation is stale. In all
// three it returns LevelDesconocido. Zeroing a missing variable instead would
// turn "we do not know" into "nothing to worry about", which is the single most
// dangerous mistake this component could make.
//
// A fourth, structural refusal comes first: the zero value has no parameter set,
// so it fails closed instead of classifying with all-zero numbers. ID and Kind
// still work, so the failure is attributable.
//
// Every non-error path carries the PolicyRef of the instance actually used, so
// the parameters behind a stored level travel with the level instead of being
// assumed from the currently deployed code.
func (r AcademicRuleV1) Classify(ctx context.Context, features Features) (Assessment, error) {
	if !r.configured {
		return Assessment{}, fmt.Errorf("classify: %s was not constructed with NewAcademicRuleV1 or NewAcademicRuleV1WithParams: its parameter set is unset", r.ID())
	}
	if err := ctx.Err(); err != nil {
		return Assessment{}, err
	}

	policy, err := r.params.PolicyRefFor(r.ID())
	if err != nil {
		return Assessment{}, err
	}

	nonFinite := features.nonFinite()
	if len(nonFinite) > 0 {
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

	score := r.params.score(features)
	level := r.params.levelFor(score)
	return Assessment{
		Level:     level,
		Factors:   r.params.factorsOf(features),
		Rationale: levelRationale(score, level),
		Policy:    policy,
	}, nil
}

// maxStaleness is how old an observation may be before this rule refuses it.
func (p Params) maxStaleness() time.Duration {
	return time.Duration(p.MaxStalenessSeconds) * time.Second
}

// score combines the weighted contributions read from this parameter set. The
// result is in [0, 1].
func (p Params) score(features Features) float64 {
	return p.WeightTemperature*rising(features.TemperatureC.Value, p.TemperatureLowC, p.TemperatureHighC) +
		p.WeightHumidity*falling(features.RelativeHumidityPct.Value, p.HumidityDryPct, p.HumidityWetPct) +
		p.WeightWind*rising(features.WindSpeedKmh.Value, p.WindLowKmh, p.WindHighKmh) +
		p.WeightVegetation*rising(features.VegetationCoverPct.Value, p.VegetationSparsePct, p.VegetationDensePct) +
		p.WeightHistorical*rising(features.HistoricalFires.Value, p.HistoricalFiresLow, p.HistoricalFiresHigh)
}

// levelFor maps a score onto a level using this rule's bands.
func (p Params) levelFor(score float64) Level {
	switch {
	case score < p.BandBajoMax:
		return LevelBajo
	case score < p.BandMedioMax:
		return LevelMedio
	default:
		return LevelAlto
	}
}

// rising maps a value onto [0, 1], where higher input means higher contribution.
func rising(value, low, high float64) float64 { return linear(value, low, high) }

// falling maps a value onto [0, 1], where higher input means lower contribution.
//
// The literal 1 in `1 - linear(...)` is the one deliberate exception to the
// "behaviour numbers live in Params" rule that the structural guard tolerates.
// The only literal-free reformulation (for example (high - value) / (high - low))
// is a different floating-point expression, and it would break the bit-exact
// behaviour preservation the A.1 refactor achieved. A narrow, documented
// exemption is the honest trade: this guard was never what locked these helpers,
// the frozen behaviour table is.
func falling(value, low, high float64) float64 { return 1 - linear(value, low, high) }

func linear(value, low, high float64) float64 {
	if value <= low {
		return 0
	}
	if value >= high {
		return 1
	}
	return (value - low) / (high - low)
}

// factorsOf reports every canonical variable with its contribution under this
// parameter set. Absent inputs stay present in the list, with no contribution.
func (p Params) factorsOf(features Features) []Factor {
	factors := make([]Factor, 0, len(features.named()))
	for _, entry := range features.named() {
		factor := Factor{Name: entry.Name, Known: entry.Value.Present}
		if !entry.Value.Present {
			factor.Note = "dato ausente"
			factors = append(factors, factor)
			continue
		}
		factor.Value = entry.Value.Value
		factors = append(factors, factor)
	}
	for index := range factors {
		if !factors[index].Known {
			continue
		}
		factors[index].Contribution = p.contributionOf(factors[index].Name, features)
	}
	return factors
}

// contributionOf is the contribution of one named variable under this rule.
func (p Params) contributionOf(name string, features Features) float64 {
	switch name {
	case "temperatura":
		return p.WeightTemperature * rising(features.TemperatureC.Value, p.TemperatureLowC, p.TemperatureHighC)
	case "humedad_relativa":
		return p.WeightHumidity * falling(features.RelativeHumidityPct.Value, p.HumidityDryPct, p.HumidityWetPct)
	case "viento":
		return p.WeightWind * rising(features.WindSpeedKmh.Value, p.WindLowKmh, p.WindHighKmh)
	case "cobertura_vegetal":
		return p.WeightVegetation * rising(features.VegetationCoverPct.Value, p.VegetationSparsePct, p.VegetationDensePct)
	case "incendios_historicos":
		return p.WeightHistorical * rising(features.HistoricalFires.Value, p.HistoricalFiresLow, p.HistoricalFiresHigh)
	default:
		return 0
	}
}

func (f Features) nonFinite() []string {
	var found []string
	for _, entry := range f.named() {
		if entry.Value.Present && (math.IsNaN(entry.Value.Value) || math.IsInf(entry.Value.Value, 0)) {
			found = append(found, entry.Name)
		}
	}
	return found
}

func missingRationale(missing []string) string {
	return fmt.Sprintf(
		"Faltan %d de 5 variables (%s). Un dato ausente nunca se interpreta como riesgo bajo: el resultado es desconocido. %s",
		len(missing), strings.Join(missing, ", "), nonOfficialNotice,
	)
}

func nonFiniteRationale(names []string) string {
	return fmt.Sprintf(
		"Valor no numérico en %s: el dato no es evaluable. %s",
		strings.Join(names, ", "), nonOfficialNotice,
	)
}

func levelRationale(score float64, level Level) string {
	return fmt.Sprintf(
		"Puntaje ponderado %.3f sobre 1.000, en la banda %s. %s",
		score, level, nonOfficialNotice,
	)
}

// nonOfficialNotice is the declaration required by the bases: a documented rule
// of our own, standing in for a trained model, making no official claim.
const nonOfficialNotice = "Corresponde a una regla propia no oficial, no calibrada contra incendios observados; no constituye un índice oficial ni sustituye a CONAF, SENAPRED ni a la autoridad."
