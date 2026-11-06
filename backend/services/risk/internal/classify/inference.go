package classify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// This file is the trained-model path: a classifier that does not score in this
// binary at all. It sends the feature map the publication path already builds to
// the inference service (POST /predict, see
// backend/contracts/openapi/inference-v1.yaml) and maps the scored answer back
// into the SAME Assessment the rules return, so the publication path, the stored
// factors and the audit trail are unchanged by switching method.
//
// The service is not reachable from outside the internal network (S3): risk is
// its only caller, there is no bearer token and no CORS, and the network is the
// boundary. There is no retry loop here either, and deliberately: retrying a
// delivery is the broker's job, per unit, so a failure inside one prediction stays
// one attributable call instead of a private queue of its own.

// MLFixtureV1ID is the registry version of the model path served by the
// inference service while no trained artifact exists.
//
// It is deliberately not called ml-v1. ml-v1 is the trained model, it can only
// reach the registry together with the artifact that produced it, and it is only
// ever the serving version through POST /v1/models/{version}/activate. A row
// named ml-v1 pointing at the committed logistic fixture would claim a training
// run nobody performed, which is the exact dishonesty this name prevents.
const MLFixtureV1ID = "ml-fixture-v1"

// inferenceVersions lists the registry versions this binary executes over the
// inference service instead of in code. It exists so startup can tell "MODEL_ID
// names a version that needs INFERENCE_URL" from "MODEL_ID names nothing this
// binary knows", and it is the only list of its kind: when a trained ml-v1
// artifact exists, it joins this list together with its own parameter
// description (MLFixtureV1Params is the shape to copy).
var inferenceVersions = []string{MLFixtureV1ID}

// ServedByInference reports whether a registry version is executed by the
// inference service rather than by a classifier compiled into this binary.
func ServedByInference(version string) bool {
	for _, candidate := range inferenceVersions {
		if version == candidate {
			return true
		}
	}
	return false
}

// MLFixtureV1Params describes the artifact the fixture version executes, so that
// its params_hash is a fingerprint of a real artifact instead of a claim about a
// model somebody trained.
//
// The values mirror services/inference/testdata/logistic.json: its format, its
// kind, the artifact's own version string and data hash, and ITS feature list in
// ITS declared order. trained_on_observed_fires is the honest answer to the
// question params_hash exists to answer - which parameters produced a stored
// level - and for this artifact the answer is "none: it is a fixture". Changing
// any value changes MLFixtureV1ParamsHash and breaks the migration test, which
// is the intended response: a different artifact is a new version, never an edit
// of this one.
func MLFixtureV1Params() map[string]string {
	return map[string]string{
		"artifact_data_hash":        "9f2c41d7b0e35a6c18d4f7e2a9b03c5d6e8f1a2b4c7d9e0f3a5b6c8d1e2f4a7b",
		"artifact_features":         "temp_max,temp_min,humidity_rel,wind_speed,wind_dir,precip_sum,radiation_sol",
		"artifact_format":           "firewatch-model/v1",
		"artifact_kind":             "logistic",
		"artifact_path":             "services/inference/testdata/logistic.json",
		"artifact_version":          "valparaiso-logistic-v1",
		"role":                      "fixture: exercises the publication path before a trained artifact exists",
		"trained_on_observed_fires": "false",
	}
}

// MLFixtureV1ParamsHash is the fingerprint of MLFixtureV1Params. It is computed
// from the description rather than written twice, so the frozen literal in
// migration 011 can only agree with the description by describing the same
// artifact.
var MLFixtureV1ParamsHash = ParamsHash(MLFixtureV1Params())

// ErrModelRefused marks a refusal: the inference service answered 4xx, meaning
// it declined to score this sample. The service is up and its model is loaded;
// what failed is the input. codes are the contract's stable literals
// (out_of_range, unknown_feature, missing_feature, invalid_body, ...), so a
// caller can tell a bad reading apart from a broken call.
var ErrModelRefused = errors.New("inference service refused the sample")

// ErrModelUnavailable marks the model that did not answer: a transport failure,
// a timeout, a 5xx, or a 200 whose body is not the documented prediction. None of
// these produce a level, and none of them are a statement about the input.
//
// The distinction is carried in the error value and in the recorded reason, not
// in the publication path's permanent/transient taxonomy, which is deliberately
// left as narrow as it is: the provider is re-fetched on every attempt, so even
// a 4xx refusal is a statement about data that may differ next time, and the
// publisher's own rule is that when in doubt it retries.
var ErrModelUnavailable = errors.New("inference service unavailable")

// The transport's numbers. They are package-level named constants rather than
// inline literals because this package is guarded by
// TestNoBehaviorNumbersOutsideTheParamsObject: a number written inside a function
// body can change a level without touching the frozen parameters, so the model
// path is allowed to name its mechanics and nothing else. They are not rule
// thresholds and none of them enters an assessment.
const (
	// maxPredictionBytes bounds what this classifier will read from a prediction
	// response. The documented answer is three fields; anything larger is not one.
	maxPredictionBytes int64 = 64 << 10
	// maxReasonBytes bounds how much of a failing body travels in an error reason,
	// so a refusal stays a log line instead of an unbounded document.
	maxReasonBytes = 200
	// minProbability and maxProbability are the bounds the contract already
	// promises. A value outside them is refused, never clamped.
	minProbability = 0.0
	maxProbability = 1.0
	// metresPerSecondPerKmh converts the canonical wind reading (km/h) into the
	// unit the fixture artifact declares (m/s). It is a physical unit conversion,
	// not a model parameter: sending km/h as m/s would be a unit error the
	// artifact's guard cannot catch, because both values sit inside [0, 40].
	metresPerSecondPerKmh = 1.0 / 3.6
	// unchangedUnit is the identity conversion, named so a factor of 1 here reads
	// as "the units agree" rather than as a scoring constant.
	unchangedUnit = 1.0
)

// predictionPath is the documented scoring route.
const predictionPath = "/predict"

// InferenceConfig is one classification method executed by the inference service
// over HTTP instead of in this binary.
type InferenceConfig struct {
	// Version is the registry version this classifier stamps on every
	// assessment. It must have a row in the models table, because
	// batches.model_version is a foreign key into it.
	Version string
	// BaseURL is the internal address of the inference service. It has no
	// default on purpose: a default would silently point a deployment at
	// whatever happened to be written here.
	BaseURL string
	// Timeout bounds one prediction. A prediction that takes longer than this is
	// a failure, not a slow success.
	Timeout time.Duration
	// Parameters describes the artifact this version executes. It is what the
	// params hash fingerprints, so it must be a description of the artifact and
	// not of the deployment: the URL and the timeout are configuration and
	// deliberately absent.
	Parameters map[string]string
}

// InferenceClassifier delegates scoring to the inference service.
//
// Every failure returns an error and NO assessment. That is the whole point of
// this type: there is no local rule to fall back to, because a fallback would
// persist a level under ml-fixture-v1 that the model never produced, and the
// audit trail the batch already guarantees would be a lie.
type InferenceClassifier struct {
	version    string
	endpoint   string
	timeout    time.Duration
	parameters map[string]string
	policy     PolicyRef
	client     *http.Client
	configured bool
}

// Compile-time proof that the model path satisfies the same contract as the
// rules, so selecting it is a configuration change and not a refactor.
var _ Classifier = InferenceClassifier{}

// NewInferenceClassifier validates one model-path configuration. Every failure
// here is a startup failure: a classifier that cannot describe its artifact, its
// address or its timeout must not be registered and then fail per prediction.
func NewInferenceClassifier(config InferenceConfig) (InferenceClassifier, error) {
	if strings.TrimSpace(config.Version) == "" {
		return InferenceClassifier{}, errors.New("classify: the inference classifier needs the registry version it stamps on assessments")
	}
	if config.Timeout <= 0 {
		return InferenceClassifier{}, fmt.Errorf("classify: the inference classifier for %q needs a positive timeout, got %s", config.Version, config.Timeout)
	}
	endpoint, err := predictionEndpoint(config.BaseURL)
	if err != nil {
		return InferenceClassifier{}, fmt.Errorf("classify: inference classifier for %q: %w", config.Version, err)
	}
	if len(config.Parameters) == 0 {
		return InferenceClassifier{}, fmt.Errorf("classify: the inference classifier for %q needs the parameter description of its artifact: without it no stored level can be reproduced", config.Version)
	}
	parameters := maps.Clone(config.Parameters)
	return InferenceClassifier{
		version:  config.Version,
		endpoint: endpoint,
		timeout:  config.Timeout,
		// The policy carries its own copy, so no caller can mutate the map behind
		// a fingerprint that was already computed from it.
		parameters: parameters,
		policy: PolicyRef{
			PolicyID:   config.Version,
			Parameters: maps.Clone(parameters),
			ParamsHash: ParamsHash(parameters),
		},
		// The client's timeout and the per-request context carry the same bound.
		// The context is what expresses the bound to the transport; the client
		// timeout is the backstop that also covers a body that stalls after the
		// headers arrived.
		client:     &http.Client{Timeout: config.Timeout},
		configured: true,
	}, nil
}

// NewMLFixtureV1 returns the fixture-backed model path: the registry version
// that runs the committed logistic artifact while no trained model exists.
func NewMLFixtureV1(baseURL string, timeout time.Duration) (InferenceClassifier, error) {
	return NewInferenceClassifier(InferenceConfig{
		Version:    MLFixtureV1ID,
		BaseURL:    baseURL,
		Timeout:    timeout,
		Parameters: MLFixtureV1Params(),
	})
}

// predictionEndpoint turns the configured base address into the scoring route.
// It refuses anything that is not an absolute http(s) URL, including the empty
// string: a missing address is a configuration error, never a reason to guess.
func predictionEndpoint(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("the inference service needs a base URL")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("base URL %q is not a URL: %w", raw, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("base URL %q must be an http or https URL, not %q", raw, parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("base URL %q names no host", raw)
	}
	return strings.TrimSuffix(parsed.String(), "/") + predictionPath, nil
}

// ID implements Classifier.
func (c InferenceClassifier) ID() string { return c.version }

// Kind implements Classifier. The model path is the trained-model branch of the
// contract; whether the artifact behind it is a fixture is stated by the version
// name, the registry row's kind and its params hash, not by this flag.
func (c InferenceClassifier) Kind() Kind { return KindTrained }

// PolicyFingerprint implements PolicyFingerprinter, so the registry can refuse a
// classifier that claims a version while reporting a different artifact.
func (c InferenceClassifier) PolicyFingerprint() (string, error) {
	if !c.configured {
		return "", fmt.Errorf("classify: %q was not constructed with NewInferenceClassifier or NewMLFixtureV1: it cannot report a policy", c.version)
	}
	return c.policy.ParamsHash, nil
}

// Classify sends one feature map to the inference service and maps its answer
// into this service's assessment contract.
//
// The level is the one the artifact reports, not one recomputed here: the cut
// points belong to the model (it may declare its own), and a second
// implementation of them in this package would be a second source of truth.
func (c InferenceClassifier) Classify(ctx context.Context, features Features) (Assessment, error) {
	if !c.configured {
		return Assessment{}, fmt.Errorf("classify: %q was not constructed with NewInferenceClassifier or NewMLFixtureV1: it has no endpoint to call", c.version)
	}
	if err := ctx.Err(); err != nil {
		return Assessment{}, err
	}
	sample, factors := inferenceSample(features)
	body, err := json.Marshal(predictionRequest{Features: sample})
	if err != nil {
		return Assessment{}, fmt.Errorf("classify: encode the sample for %s: %w", c.version, err)
	}
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return Assessment{}, fmt.Errorf("classify: build the prediction request for %s: %w", c.version, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return Assessment{}, unavailableError(transportCause(err), fmt.Errorf("POST %s: %w", c.endpoint, err))
	}
	defer response.Body.Close()
	// The body is read even on a refusal, because the contract's refusal reason
	// (its stable code) is what makes the failure attributable.
	document, err := io.ReadAll(io.LimitReader(response.Body, maxPredictionBytes))
	if err != nil {
		return Assessment{}, unavailableError("body", fmt.Errorf("read the prediction response: %w", err))
	}
	switch {
	case response.StatusCode == http.StatusOK:
	case response.StatusCode >= http.StatusBadRequest && response.StatusCode < http.StatusInternalServerError:
		// The service is up and its model is loaded; it declined this sample.
		return Assessment{}, refusalError(response.StatusCode, document)
	case response.StatusCode >= http.StatusInternalServerError:
		return Assessment{}, unavailableError("status", fmt.Errorf("the inference service answered %d: %s", response.StatusCode, summarise(document)))
	default:
		return Assessment{}, unavailableError("status", fmt.Errorf("the inference service answered an undocumented status %d: %s", response.StatusCode, summarise(document)))
	}
	prediction, err := decodePrediction(document)
	if err != nil {
		return Assessment{}, err
	}
	level, known := inferenceLevel(prediction.Level)
	if !known {
		return Assessment{}, unavailableError("body", fmt.Errorf("the prediction names the undocumented level %q", prediction.Level))
	}
	probability := *prediction.Probability
	return Assessment{
		Level:   level,
		Factors: factors,
		Rationale: fmt.Sprintf(
			"El servicio de inferencia %s (artefacto %q) respondió probabilidad %.4f y el artefacto la clasifica como %q. El modelo no reporta contribución por variable: los factores registran las entradas enviadas, con contribución 0.",
			c.version, prediction.ModelVersion, probability, prediction.Level,
		),
		Policy: c.policy,
	}, nil
}

// predictionRequest is the documented body: one object of feature name to value.
// A nil value is a JSON null, which the contract defines as "declared but
// unobserved" and the artifact imputes.
type predictionRequest struct {
	Features map[string]*float64 `json:"features"`
}

// prediction is the documented success body. Probability is a pointer so an
// absent member is refused instead of silently scoring zero.
type prediction struct {
	Level        string   `json:"level"`
	Probability  *float64 `json:"probability"`
	ModelVersion string   `json:"model_version"`
}

// decodePrediction parses a 200 body. Unknown members are tolerated - a
// compatible addition to the response must not stop publication - but every
// documented member is required, and the probability must be a finite number in
// [0, 1], which is what the contract already promises.
func decodePrediction(document []byte) (prediction, error) {
	var decoded prediction
	if err := json.Unmarshal(document, &decoded); err != nil {
		return prediction{}, unavailableError("body", fmt.Errorf("the prediction body is not the documented object: %w", err))
	}
	if decoded.Probability == nil {
		return prediction{}, unavailableError("body", errors.New("the prediction body carries no probability"))
	}
	if decoded.Level == "" {
		return prediction{}, unavailableError("body", errors.New("the prediction body carries no level"))
	}
	if decoded.ModelVersion == "" {
		return prediction{}, unavailableError("body", errors.New("the prediction body carries no model_version, so the answer cannot be attributed"))
	}
	probability := *decoded.Probability
	if math.IsNaN(probability) || math.IsInf(probability, 0) || probability < minProbability || probability > maxProbability {
		return prediction{}, unavailableError("body", fmt.Errorf("the prediction carries probability %v, which is not a finite number in [0, 1]", probability))
	}
	return decoded, nil
}

// inferenceLevel maps the artifact's level vocabulary onto the levels this
// service persists. An unrecognized level is refused rather than guessed.
func inferenceLevel(served string) (Level, bool) {
	switch served {
	case "low":
		return LevelBajo, true
	case "medium":
		return LevelMedio, true
	case "high":
		return LevelAlto, true
	}
	return "", false
}

// RefusalError is one 4xx answer: the service declined to score this sample.
type RefusalError struct {
	Status    int
	Code      string
	RequestID string
	Detail    string
}

// Error renders the refusal with the contract's stable code and the request id,
// so an operator can join this log line to the inference service's own record.
func (e RefusalError) Error() string {
	code := e.Code
	if code == "" {
		code = "unparsed"
	}
	reason := fmt.Sprintf("the inference service refused the sample with status %d and code %q", e.Status, code)
	if e.RequestID != "" {
		reason += " (request_id " + e.RequestID + ")"
	}
	if e.Detail != "" {
		reason += ": " + e.Detail
	}
	return reason
}

// Unwrap makes errors.Is(err, ErrModelRefused) true for a refusal.
func (e RefusalError) Unwrap() error { return ErrModelRefused }

// UnavailableError is the other class of failure: the model did not answer. Cause
// is a stable literal (timeout, transport, status, body) so a log line can be
// grouped without parsing a message.
type UnavailableError struct {
	Cause string
	Err   error
}

// Error renders the cause class and the underlying failure.
func (e UnavailableError) Error() string { return e.Cause + ": " + e.Err.Error() }

// Unwrap makes errors.Is(err, ErrModelUnavailable) true for a service that did
// not answer. The cause class and the underlying failure stay reachable through
// the concrete type, which is where a caller that needs them looks.
func (e UnavailableError) Unwrap() error { return ErrModelUnavailable }

// refusalError reads the documented refusal body. A refusal whose body cannot be
// parsed is still a refusal: the status is the fact, the detail is only for logs.
func refusalError(status int, document []byte) error {
	var body struct {
		Code      string `json:"code"`
		RequestID string `json:"request_id"`
		Detail    string `json:"detail"`
	}
	if err := json.Unmarshal(document, &body); err != nil {
		return RefusalError{Status: status, Detail: summarise(document)}
	}
	return RefusalError{Status: status, Code: body.Code, RequestID: body.RequestID, Detail: body.Detail}
}

// unavailableError is the single construction path for a model that did not
// answer, so the cause class is always set.
func unavailableError(cause string, err error) error {
	return UnavailableError{Cause: cause, Err: err}
}

// transportCause classifies a transport failure as a timeout when the request
// ran out of time rather than being refused.
func transportCause(err error) string {
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return "timeout"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "transport"
}

// summarise bounds a body for a log line: a failure reason must stay readable
// and must not carry an unbounded document into an error string.
func summarise(document []byte) string {
	text := strings.TrimSpace(string(document))
	if text == "" {
		return "(empty body)"
	}
	runes := []rune(text)
	if len(runes) > maxReasonBytes {
		return string(runes[:maxReasonBytes]) + "..."
	}
	return text
}

// inferenceInput is one feature of the artifact's input vector, expressed in the
// canonical vocabulary of this service.
//
// The table is the whole cross-contract decision, so it is stated rather than
// derived: a feature with no Read is declared null ("declared but unobserved")
// and the artifact imputes it, and a feature with a Read carries the unit
// conversion that reaches the unit the artifact declares. What is deliberately
// absent: `temperatura` is not sent as temp_max. The canonical variable is an
// instantaneous reading with no qualifier, and claiming it is a daily maximum
// would hand the model a value it was never trained to interpret - the same
// class of error as re-labelling another commune's data as this one.
type inferenceInput struct {
	// Feature is the artifact's feature name, as firewatch-model/v1 declares it.
	Feature string
	// Factor is the canonical name persisted in batches.factors.
	Factor string
	// Unit is the unit the artifact declares for this feature.
	Unit string
	// Scale converts the canonical reading into the artifact's unit. unchangedUnit
	// means the units already agree.
	Scale float64
	// Read extracts the canonical reading. It is nil when this service publishes
	// no reading that the artifact's feature could honestly be built from.
	Read func(Features) Feature
	// Note explains, for the persisted factor, what was sent and why.
	Note string
}

// inferenceInputs is the feature map this classifier sends, in the artifact's
// declared order. The order is documentation rather than protocol - the request
// is key-addressed - but it keeps this table readable against
// services/inference/testdata/logistic.json, which is what it describes.
var inferenceInputs = []inferenceInput{
	{
		Feature: "temp_max",
		Factor:  "temperatura_maxima_diaria",
		Unit:    "degC",
		Note:    "el artefacto pide la temperatura máxima diaria y este servicio no publica esa variable",
	},
	{
		Feature: "temp_min",
		Factor:  "temperatura_minima_diaria",
		Unit:    "degC",
		Note:    "el artefacto pide la temperatura mínima diaria y este servicio no publica esa variable",
	},
	{
		Feature: "humidity_rel",
		Factor:  "humedad_relativa",
		Unit:    "%",
		Scale:   unchangedUnit,
		Read:    func(f Features) Feature { return f.RelativeHumidityPct },
		Note:    "misma variable y misma unidad que humedad_relativa",
	},
	{
		Feature: "wind_speed",
		Factor:  "viento",
		Unit:    "m/s",
		Scale:   metresPerSecondPerKmh,
		Read:    func(f Features) Feature { return f.WindSpeedKmh },
		Note:    "viento convertido de km/h a m/s (÷3.6), la unidad que declara el artefacto",
	},
	{
		Feature: "wind_dir",
		Factor:  "direccion_viento",
		Unit:    "deg",
		Note:    "el artefacto pide la dirección del viento y este servicio no publica esa variable",
	},
	{
		Feature: "precip_sum",
		Factor:  "precipitacion_acumulada",
		Unit:    "mm/day",
		Note:    "el artefacto pide la precipitación acumulada y este servicio no publica esa variable",
	},
	{
		Feature: "radiation_sol",
		Factor:  "radiacion_solar",
		Unit:    "W/m2",
		Note:    "el artefacto pide la radiación solar y este servicio no publica esa variable",
	},
}

// inferenceSample builds the request body and the factors that describe it.
//
// A reading this service does not have is sent as an explicit null, which the
// contract defines as "declared but unobserved" and the artifact imputes.
// Omitting the key instead would make the service refuse the sample as missing,
// which is the correct answer for a client bug and the wrong one for a feature
// the model asks for and this deployment cannot supply. Absence is never
// reported as a zero reading, for the same reason the rules abstain instead of
// interpreting a missing humidity as 0%.
func inferenceSample(features Features) (map[string]*float64, []Factor) {
	sample := make(map[string]*float64, len(inferenceInputs))
	factors := make([]Factor, 0, len(inferenceInputs))
	for _, input := range inferenceInputs {
		factor := Factor{Name: input.Factor, Note: input.Note + " (" + input.Unit + ")"}
		var reading Feature
		if input.Read != nil {
			reading = input.Read(features)
		}
		switch {
		case !reading.Present:
			factor.Note += ": sin lectura, se declara nula y el artefacto la imputa"
			sample[input.Feature] = nil
		case math.IsNaN(reading.Value) || math.IsInf(reading.Value, 0):
			// The artifact's own preprocessing maps NaN to its imputed value, so a
			// non-finite reading is declared unobserved instead of being encoded as
			// a number. It is recorded on the factor, never dropped in silence.
			factor.Note += ": el valor recibido no es un número finito, se declara nulo y el artefacto lo imputa"
			sample[input.Feature] = nil
		default:
			value := reading.Value * input.Scale
			factor.Known = true
			factor.Value = reading.Value
			sample[input.Feature] = &value
		}
		factors = append(factors, factor)
	}
	return sample, factors
}
