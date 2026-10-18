package classify

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// ParamsHash returns a canonical, order-independent, unambiguous fingerprint of a
// parameter map, as lowercase hex sha256.
//
// It exists because a classifier version is immutable: a level stored today must
// still mean the same thing years from now. The version name alone guarantees
// nothing - the effective numbers do - so the effective parameter set is hashed
// and frozen as a golden value. An accidental edit to a parameter then breaks the
// build instead of silently redefining the behaviour behind an already stored
// level.
//
// The canonical byte string is built here, never through encoding/json, so the
// result does not depend on Go map iteration order or on JSON key ordering. Each
// parameter is written as a length-prefixed line:
//
//	<len(key)>:<key>=<len(value)>:<value>\n
//
// The length prefixes are what make the encoding unambiguous: {"a":"1\nb=2"}
// cannot collide with {"a":"1","b":"2"}. The function cannot fail.
func ParamsHash(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		value := params[key]
		fmt.Fprintf(&builder, "%d:%s=%d:%s\n", len(key), key, len(value), value)
	}

	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}

// AcademicRuleV1ParamsHash is the frozen fingerprint of the effective parameter
// set of academic-rule-v1, i.e. ParamsHash(DefaultAcademicRuleV1Params().Parameters()).
//
// It is the golden guard of this rule version's parameter set: changing any
// parameter in DefaultAcademicRuleV1Params changes this hash,
// TestAcademicRuleV1ParametersAreFrozen fails, and the correct response is a NEW
// classifier version (academic-rule-v2) with a new migration row - never an edit
// of the existing, immutable version. Reusing the version name for changed
// numbers would make every previously stored level unreproducible.
//
// It freezes the numbers, not every path to a level: a literal elsewhere, or an
// operator change with no literal, is invisible to it (see Params).
const AcademicRuleV1ParamsHash = "e17aaa5600eb67c9a91340e90057aec100f4e80ef0895662f373cffee66564c7"

// FirewatchRuleV2ParamsHash is the frozen fingerprint of the effective parameter
// set of firewatch-rule-v2, i.e. ParamsHash(DefaultFirewatchRuleV2Params().Parameters()).
//
// It is the golden guard of the second rule version. A second version exists
// precisely so a behaviour change is a new, immutable identity rather than an
// edit of the frozen academic-rule-v1: the migration records this fingerprint,
// the registry refuses to attach the identifier to any other parameter set, and
// TestFirewatchRuleV2ParametersAreFrozen fails if the numbers move.
const FirewatchRuleV2ParamsHash = "59259c4c0c470e5a0d512fe09f6c748c38fd29f9bc5d760d49754555c66e699b"

// Params is the complete, ordered set of numeric parameters that define the
// behaviour of academic-rule-v1.
//
// It exists so the frozen parameter set and its fingerprint cannot drift. What
// that guarantees: Parameters() derives the canonical map mechanically from these
// fields, so every field here is part of the hashed, frozen set, and adding a
// threshold to the rule means adding a field here, which changes the hash and
// forces a new classifier version instead of silently reusing the name for
// different numbers.
//
// What it does NOT guarantee: that the behaviour is unreachable from outside this
// object. A literal spelled elsewhere, or an operator change that uses no literal,
// can still move a level while this struct and its fingerprint stay untouched. The
// behaviour lock is therefore the frozen golden behaviour table, not this type,
// not the fingerprint, and not the structural guard, which only reduces the attack
// surface.
//
// The field order is the declaration order of the map keys. The `param` tags are
// the canonical keys; they are the only names the fingerprint knows.
type Params struct {
	MaxStalenessSeconds int64   `param:"max_staleness_seconds"`
	TemperatureLowC     float64 `param:"temperature_low_c"`
	TemperatureHighC    float64 `param:"temperature_high_c"`
	HumidityDryPct      float64 `param:"humidity_dry_pct"`
	HumidityWetPct      float64 `param:"humidity_wet_pct"`
	WindLowKmh          float64 `param:"wind_low_kmh"`
	WindHighKmh         float64 `param:"wind_high_kmh"`
	VegetationSparsePct float64 `param:"vegetation_sparse_pct"`
	VegetationDensePct  float64 `param:"vegetation_dense_pct"`
	HistoricalFiresLow  float64 `param:"historical_fires_low"`
	HistoricalFiresHigh float64 `param:"historical_fires_high"`
	WeightTemperature   float64 `param:"weight_temperature"`
	WeightHumidity      float64 `param:"weight_humidity"`
	WeightWind          float64 `param:"weight_wind"`
	WeightVegetation    float64 `param:"weight_vegetation"`
	WeightHistorical    float64 `param:"weight_historical"`
	BandBajoMax         float64 `param:"band_bajo_max"`
	BandMedioMax        float64 `param:"band_medio_max"`
}

// DefaultAcademicRuleV1Params returns the frozen default parameter set of
// academic-rule-v1.
//
// It is the only place in the package where behaviour numbers are meant to appear.
// TestAcademicRuleV1ParametersAreFrozen pins its values against a hand-written
// golden map, so changing a number here changes the frozen hash and the correct
// fix is a new classifier version. TestNoBehaviorNumbersOutsideTheParamsObject
// also rejects obvious numbers elsewhere, but it only reduces the attack surface:
// it cannot see an operator change that uses no literal, so it is not the lock.
func DefaultAcademicRuleV1Params() Params {
	return Params{
		MaxStalenessSeconds: 21600,
		TemperatureLowC:     15,
		TemperatureHighC:    35,
		HumidityDryPct:      20,
		HumidityWetPct:      60,
		WindLowKmh:          10,
		WindHighKmh:         50,
		VegetationSparsePct: 10,
		VegetationDensePct:  70,
		HistoricalFiresLow:  0,
		HistoricalFiresHigh: 5,
		WeightTemperature:   0.25,
		WeightHumidity:      0.25,
		WeightWind:          0.20,
		WeightVegetation:    0.15,
		WeightHistorical:    0.15,
		BandBajoMax:         0.34,
		BandMedioMax:        0.67,
	}
}

// DefaultFirewatchRuleV2Params returns the frozen default parameter set of
// firewatch-rule-v2.
//
// It is a distinct, versioned parameter set on purpose: v2 is a second rule, not
// a re-tuning of v1. Its numbers live only here, so a change moves
// FirewatchRuleV2ParamsHash and forces a new version instead of silently
// redefining a stored level. It shares the Params struct so the canonical map and
// its fingerprint are derived by the same mechanical rule as v1.
func DefaultFirewatchRuleV2Params() Params {
	return Params{
		MaxStalenessSeconds: 10800,
		TemperatureLowC:     20,
		TemperatureHighC:    34,
		HumidityDryPct:      25,
		HumidityWetPct:      55,
		WindLowKmh:          15,
		WindHighKmh:         55,
		VegetationSparsePct: 15,
		VegetationDensePct:  75,
		HistoricalFiresLow:  0,
		HistoricalFiresHigh: 6,
		WeightTemperature:   0.30,
		WeightHumidity:      0.30,
		WeightWind:          0.20,
		WeightVegetation:    0.10,
		WeightHistorical:    0.10,
		BandBajoMax:         0.30,
		BandMedioMax:        0.60,
	}
}

// Parameters returns the effective numeric parameters of this rule, derived
// mechanically from the fields so the canonical map cannot drift from the struct
// fields it is derived from.
//
// It exists because a classifier version is immutable: to reproduce a stored
// level, what must be recorded is the effective parameter set, not the name. This
// map is that set, and AcademicRuleV1ParamsHash freezes it. What it does NOT
// guarantee is that the map is the only source of a level: a literal or an
// operator change outside the Params object is invisible to the fingerprint (see
// Params).
//
// The derivation walks the fields in declaration order, using the `param` tag as
// the canonical key, and encodes int64 as base-10 and float64 with the shortest
// round-trippable form. Deliberately excluded: narrative text such as
// nonOfficialNotice. It does not change a level, so it is not a parameter, and it
// is not part of the fingerprint.
func (p Params) Parameters() (map[string]string, error) {
	return deriveParameters(reflect.ValueOf(p))
}

// Hash returns the canonical fingerprint of this exact parameter set.
func (p Params) Hash() (string, error) {
	parameters, err := p.Parameters()
	if err != nil {
		return "", err
	}
	return ParamsHash(parameters), nil
}

// deriveParameters is the mechanical derivation behind Parameters. It is a
// separate function so the duplicate-tag and unsupported-kind guards can be
// exercised directly, which is how the derivation test proves they are real.
func deriveParameters(value reflect.Value) (map[string]string, error) {
	typ := value.Type()
	parameters := make(map[string]string, typ.NumField())
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		tag := field.Tag.Get("param")
		if tag == "" {
			return nil, fmt.Errorf("classify: field %s has no param tag", field.Name)
		}
		if _, exists := parameters[tag]; exists {
			return nil, fmt.Errorf("classify: duplicate param tag %q on field %s", tag, field.Name)
		}
		switch kind := value.Field(index).Kind(); kind {
		case reflect.Int64:
			parameters[tag] = strconv.FormatInt(value.Field(index).Int(), 10)
		case reflect.Float64:
			parameters[tag] = strconv.FormatFloat(value.Field(index).Float(), 'g', -1, 64)
		default:
			return nil, fmt.Errorf("classify: field %s has unsupported kind %s", field.Name, kind)
		}
	}
	return parameters, nil
}

// validate reports whether the parameter set is defined enough to classify with:
// every float must be finite and the staleness threshold must not be negative.
//
// Everything else is taken as given. A parameter set is data, and
// NewAcademicRuleV1WithParams exists as a testing seam, so the package does not
// second-guess unusual but well-defined values such as unordered bands.
func (p Params) validate() error {
	if p.MaxStalenessSeconds < 0 {
		return fmt.Errorf("classify: max_staleness_seconds must not be negative: %d", p.MaxStalenessSeconds)
	}
	value := reflect.ValueOf(p)
	typ := value.Type()
	for index := 0; index < typ.NumField(); index++ {
		if value.Field(index).Kind() != reflect.Float64 {
			continue
		}
		candidate := value.Field(index).Float()
		if math.IsNaN(candidate) || math.IsInf(candidate, 0) {
			return fmt.Errorf("classify: parameter %s is not finite: %v", typ.Field(index).Name, candidate)
		}
	}
	return nil
}
