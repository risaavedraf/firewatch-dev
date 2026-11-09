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
// What it sends is the artifact's decision, not this package's: GET /v1/model is
// read once at construction (modelInputPlan) and the request covers exactly the
// features that list declares, in its order. The mapping table below is a lookup
// keyed by those names, so a model with four features and one with nine both work
// without an edit here.
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
// migration 013 can only agree with the description by describing the same
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
	// maxModelDocumentBytes bounds what this classifier will read from the model
	// document. It is the same order of magnitude as the prediction bound for the
	// same reason: the documented document is a feature list and some provenance,
	// so a body larger than this is not one.
	maxModelDocumentBytes int64 = 64 << 10
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

// modelDocumentPath is the documented provenance route. Its body carries the
// artifact's declared feature list in its declared order, which is what fixes the
// vector this classifier sends: the list is the artifact's, never this package's.
const modelDocumentPath = "/v1/model"

// InferenceConfig is one classification method executed by the inference service
// over HTTP instead of in this binary.
type InferenceConfig struct {
	// Version is the registry version this classifier stamps on every
	// assessment. It must have a row in the models table, because
	// batches.model_version is a foreign key into it.
	Version string
	// BaseURL is the internal address of the inference service. It has no
	// default on purpose: a default would silently point a deployment at
	// whatever happened to be written here. It is also where construction reads
	// the artifact's model document (GET /v1/model) from.
	BaseURL string
	// Timeout bounds one prediction, and the single model-document read that
	// construction performs. A prediction that takes longer than this is a
	// failure, not a slow success.
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
	// inputs is the artifact's declared input list, resolved against the mapping
	// table once at construction and kept in the artifact's declared order.
	//
	// It IS the cache: GET /v1/model is called exactly once per process. The
	// inference service loads and validates its artifact before its listener starts
	// and never reloads it, so a descriptor that changed under a running process
	// could only describe a model that process is not scoring with. The consequence
	// is deliberate and is a deployment rule rather than a protocol: rotating the
	// artifact requires restarting the inference service AND this service. A
	// refresh loop was rejected because it would have to answer what happens to a
	// prediction in flight while the vector changes shape underneath it, and an
	// unanswered question there is a wrong number, not a log line.
	inputs []modelInput
}

// Compile-time proof that the model path satisfies the same contract as the
// rules, so selecting it is a configuration change and not a refactor.
var _ Classifier = InferenceClassifier{}

// NewInferenceClassifier validates one model-path configuration and reads the
// artifact's declared feature list.
//
// Every failure here is a startup failure: a classifier that cannot describe its
// artifact, its address or its timeout must not be registered and then fail per
// prediction. That includes the model document - a classifier that cannot read the
// vector its artifact expects has no honest way to build one, and starting with an
// empty or guessed feature set is exactly the silent mismatch this call replaces.
//
// The fetch is bounded by the classifier's own timeout and happens before any
// prediction, so a service that is down or serving something else stops startup
// instead of stopping the queue later.
func NewInferenceClassifier(ctx context.Context, config InferenceConfig) (InferenceClassifier, error) {
	if strings.TrimSpace(config.Version) == "" {
		return InferenceClassifier{}, errors.New("classify: the inference classifier needs the registry version it stamps on assessments")
	}
	if config.Timeout <= 0 {
		return InferenceClassifier{}, fmt.Errorf("classify: the inference classifier for %q needs a positive timeout, got %s", config.Version, config.Timeout)
	}
	base, err := serviceBaseURL(config.BaseURL)
	if err != nil {
		return InferenceClassifier{}, fmt.Errorf("classify: inference classifier for %q: %w", config.Version, err)
	}
	if len(config.Parameters) == 0 {
		return InferenceClassifier{}, fmt.Errorf("classify: the inference classifier for %q needs the parameter description of its artifact: without it no stored level can be reproduced", config.Version)
	}
	parameters := maps.Clone(config.Parameters)
	// The client's timeout and the per-request context carry the same bound. The
	// context is what expresses the bound to the transport; the client timeout is
	// the backstop that also covers a body that stalls after the headers arrived.
	client := &http.Client{Timeout: config.Timeout}
	document, err := fetchModelDocument(ctx, client, base+modelDocumentPath, config.Timeout)
	if err != nil {
		return InferenceClassifier{}, fmt.Errorf("classify: the inference classifier for %q: %w", config.Version, err)
	}
	inputs, err := modelInputPlan(document)
	if err != nil {
		return InferenceClassifier{}, fmt.Errorf("classify: the inference classifier for %q cannot serve the model document the inference service at %s declares: %w", config.Version, base+modelDocumentPath, err)
	}
	return InferenceClassifier{
		version:  config.Version,
		endpoint: base + predictionPath,
		timeout:  config.Timeout,
		// The policy carries its own copy, so no caller can mutate the map behind
		// a fingerprint that was already computed from it.
		parameters: parameters,
		policy: PolicyRef{
			PolicyID:   config.Version,
			Parameters: maps.Clone(parameters),
			ParamsHash: ParamsHash(parameters),
		},
		client:     client,
		configured: true,
		inputs:     inputs,
	}, nil
}

// NewMLFixtureV1 returns the fixture-backed model path: the registry version
// that runs the committed logistic artifact while no trained model exists.
func NewMLFixtureV1(ctx context.Context, baseURL string, timeout time.Duration) (InferenceClassifier, error) {
	return NewInferenceClassifier(ctx, InferenceConfig{
		Version:    MLFixtureV1ID,
		BaseURL:    baseURL,
		Timeout:    timeout,
		Parameters: MLFixtureV1Params(),
	})
}

// serviceBaseURL validates the configured address and returns it without a
// trailing slash. It refuses anything that is not an absolute http(s) URL,
// including the empty string: a missing address is a configuration error, never a
// reason to guess.
func serviceBaseURL(raw string) (string, error) {
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
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

// modelDocument is the one part of GET /v1/model this classifier reads: the
// artifact's declared feature list, in its declared order.
//
// The rest of the document - format, version, kind, thresholds, metrics, baseline
// - is provenance for a human and stays unread here. Decoding it would only give
// this package a second opinion about an artifact the inference service already
// validated, and the one thing this package must agree with it about is the input
// vector.
type modelDocument struct {
	Features []declaredFeature `json:"features"`
}

// declaredFeature is one input the artifact declares. Only the two members this
// service acts on are kept: the name is the vector's key and the unit is checked
// against the mapping that answers it. min and max are the artifact's own guard
// and are enforced where the scoring happens, not here.
type declaredFeature struct {
	Name string `json:"name"`
	Unit string `json:"unit"`
}

// fetchModelDocument reads the artifact's declared feature list, bounded by the
// same timeout as a prediction.
//
// Every failure names the inference service and the reason: unreachable, an
// undocumented status, a body that is not the documented document, or a body too
// large to be one. None of them produces a feature set, and none of them is
// retried: a service that cannot describe its model at startup is a deployment
// that must not start.
func fetchModelDocument(ctx context.Context, client *http.Client, endpoint string, timeout time.Duration) (modelDocument, error) {
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return modelDocument{}, fmt.Errorf("build the %s request to the inference service: %w", modelDocumentPath, err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return modelDocument{}, fmt.Errorf("the inference service at %s could not be reached for %s (%s): %w", endpoint, modelDocumentPath, transportCause(err), err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxModelDocumentBytes))
	if err != nil {
		return modelDocument{}, fmt.Errorf("the inference service at %s: read %s: %w", endpoint, modelDocumentPath, err)
	}
	if response.StatusCode != http.StatusOK {
		return modelDocument{}, fmt.Errorf("the inference service at %s answered %d for %s: %s", endpoint, response.StatusCode, modelDocumentPath, summarise(body))
	}
	var document modelDocument
	if err := json.Unmarshal(body, &document); err != nil {
		return modelDocument{}, fmt.Errorf("the inference service at %s answered a %s that is not the documented model document: %w", endpoint, modelDocumentPath, err)
	}
	return document, nil
}

// modelInput is one entry of the vector this classifier sends: a feature the
// artifact declares, in the artifact's declared order, and how this service
// answers it.
type modelInput struct {
	// Feature is the artifact's feature name, exactly as it declared it.
	Feature string
	// Declared is the unit the artifact declares for it.
	Declared string
	// Mapping is the reading or aggregate this service builds for the name, or nil
	// when it can build none. A nil Mapping is sent as an explicit null with a
	// reason naming the feature, never dropped from the vector.
	Mapping *inferenceMapping
}

// modelInputPlan turns the artifact's declared list into the ordered vector this
// classifier sends, and refuses a descriptor this service cannot serve.
//
// The checks are the difference between a misalignment that is a startup failure
// and one that is a wrong prediction. A duplicate name has no single position in
// the vector; an empty list is a vector nothing can be sent in; a unit that
// contradicts the mapping would send one unit where the artifact reads another,
// and the artifact's own min/max guard cannot catch that because both values sit
// inside the same interval (45 km/h and 12.5 m/s are both plausible wind speeds).
//
// A declared name this service cannot build is NOT a refusal: it becomes a null
// that names itself in the factors, because whether a model may take a reading
// this deployment lacks is the model's decision (it imputes it), while whether the
// vector covers the declared list is a client bug.
func modelInputPlan(document modelDocument) ([]modelInput, error) {
	switch {
	case document.Features == nil:
		return nil, errors.New("the artifact declares no feature list, so the vector its model takes is unknown")
	case len(document.Features) == 0:
		return nil, errors.New("the artifact declares an empty feature list, so there is no vector to send")
	}
	plan := make([]modelInput, 0, len(document.Features))
	declared := make(map[string]bool, len(document.Features))
	for _, feature := range document.Features {
		name := strings.TrimSpace(feature.Name)
		if name == "" {
			return nil, errors.New("the artifact declares a feature with no name, so it cannot be addressed in the vector")
		}
		if declared[name] {
			return nil, fmt.Errorf("the artifact declares the feature %q more than once, so it has no single position in the vector", name)
		}
		declared[name] = true
		input := modelInput{Feature: name, Declared: strings.TrimSpace(feature.Unit)}
		if mapping, mapped := inferenceMappings[name]; mapped {
			if input.Declared != mapping.Unit {
				return nil, fmt.Errorf("the artifact declares the feature %q in unit %q, while this service builds it in %q: sending one as the other would be a unit error the artifact's own guard cannot catch", name, input.Declared, mapping.Unit)
			}
			input.Mapping = &mapping
		}
		plan = append(plan, input)
	}
	return plan, nil
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
	sample, factors := inferenceSample(c.inputs, features)
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

// Measured is one value of the trained model's input vector, together with the
// reason it is what it is.
//
// It exists because "null" is not one fact. A feature can be absent because the
// retrieval did not carry the variable, because the readings history does not
// cover the window the aggregate is defined over, or because the stored value
// arrived in a unit that contradicts the variable's own. All of those send a JSON
// null, and all of them must record a different, operator-readable reason: without
// it a persisted factor would say "unknown" and nothing about which window was
// short or why. That is the difference between "we cannot know" and "we forgot".
type Measured struct {
	// Value is the reading, present or absent. An absent reading is Unknown(),
	// never Known(0): a zero precipitation sum computed from a missing window is a
	// lie that changes the prediction, and absence is the only honest encoding of
	// "the history cannot answer this".
	Value Feature
	// Reason backs Value: how it was computed when present, or which window was
	// short when absent. It is appended to the persisted factor's note, so the
	// reason survives in the audit trail instead of only in a log line.
	Reason string
	// Coverage reports how much of the feature's window the readings history
	// covers. It is the zero Coverage for an instantaneous reading, which has no
	// window to cover.
	Coverage Coverage
}

// Coverage records how much of the window a windowed feature is defined over the
// readings history actually covers.
//
// It is carried separately from the prose reason so a caller can assert the count
// instead of parsing a sentence: "0 of 7 days" is a fact about the data, and the
// number that proves a null was justified has to be checkable.
type Coverage struct {
	// Covered is how many units of the window hold at least one reading.
	Covered int
	// Required is how many units a complete window needs.
	Required int
	// Unit names what is counted, for example "días locales".
	Unit string
}

// Complete reports whether the window is fully covered. A zero Coverage - an
// instantaneous reading, or one that was never windowed - is not complete, and no
// caller treats it as if it were.
func (c Coverage) Complete() bool { return c.Required > 0 && c.Covered >= c.Required }

// String renders the coverage for a recorded reason, for example
// "0 de 7 días locales".
func (c Coverage) String() string { return fmt.Sprintf("%d de %d %s", c.Covered, c.Required, c.Unit) }

// ModelInput is the input vector internal/features builds at publication time, in
// the canonical units this service stores: temperature in degrees Celsius, relative
// humidity in percent, wind speed in km/h, wind direction in degrees, precipitation
// in millimetres, radiation in W/m2.
//
// It is the set of readings and aggregates this service CAN build, not the set a
// model takes. Which of them travel, in which order and in which unit, is the
// artifact's declaration (see modelInputPlan): this type offers the values, and the
// mapping table below converts each one into the unit the artifact declares.
// Keeping those two roles apart is what lets a model take two of these or nine
// of them without an edit here.
type ModelInput struct {
	// TempMaxC and TempMinC are the extremes of the current civil day, computed
	// over the readings history in the aggregate's named time zone.
	TempMaxC Measured
	TempMinC Measured
	// HumidityRelPct, WindSpeedKmh, WindDirectionDeg and RadiationWPerM2 are
	// instantaneous readings of the current retrieval.
	HumidityRelPct   Measured
	WindSpeedKmh     Measured
	WindDirectionDeg Measured
	RadiationWPerM2  Measured
	// PrecipitationMM is the accumulated precipitation over the last seven civil
	// days of the readings history.
	PrecipitationMM Measured
}

// inferenceMapping is one reading or aggregate this service can build, expressed in
// the canonical vocabulary of this service and addressable by the feature name an
// artifact declares.
//
// The table is the whole cross-contract decision, so it is stated rather than
// derived: a feature whose Measured is absent is declared null ("declared but
// unobserved") and the artifact imputes it, and a feature with a value carries the
// unit conversion that reaches the unit the artifact declares. What is
// deliberately absent: the instantaneous `temperatura` of the rules is not sent as
// temp_max. The daily maximum comes from the readings history, through
// internal/features, and claiming an instantaneous reading is a daily maximum would
// hand the model a value it was never trained to interpret.
type inferenceMapping struct {
	// Factor is the canonical name persisted in batches.factors.
	Factor string
	// Unit is the unit this service builds this reading in. It is compared against
	// the unit the artifact declares for the name, and a disagreement stops startup:
	// a conversion chosen for the wrong unit is silent, because both values sit
	// inside the artifact's own min/max guard.
	Unit string
	// Scale converts the canonical reading into the artifact's unit. unchangedUnit
	// means the units already agree.
	Scale float64
	// Read extracts the measured value from the model input vector.
	Read func(ModelInput) Measured
	// Note explains, for the persisted factor, what was sent and why.
	Note string
}

// inferenceMappings is every feature name this service can answer, keyed by the
// name the artifact declares.
//
// It is a LOOKUP, not the input vector. Which entries are used, and in which
// order, is decided by the artifact's own declared list (GET /v1/model): an
// artifact that takes two of these sends two, and one that takes a name nobody here
// can build sends a null that names it. The list used to be the vector itself,
// which made the artifact's feature set and this service's expectations two places
// that could disagree without any test noticing - the mismatch only showed up as a
// wrong prediction.
//
// Every value here is built by internal/features at publication time; a null
// therefore means "the history or the retrieval could not answer this", and the
// Measured reason says which, with its window coverage.
var inferenceMappings = map[string]inferenceMapping{
	"temp_max": {
		Factor: "temperatura_maxima_diaria",
		Unit:   "degC",
		Scale:  unchangedUnit,
		Read:   func(m ModelInput) Measured { return m.TempMaxC },
		Note:   "máximo del día civil en curso en la zona horaria de los agregados",
	},
	"temp_min": {
		Factor: "temperatura_minima_diaria",
		Unit:   "degC",
		Scale:  unchangedUnit,
		Read:   func(m ModelInput) Measured { return m.TempMinC },
		Note:   "mínimo del día civil en curso en la zona horaria de los agregados",
	},
	"humidity_rel": {
		Factor: "humedad_relativa",
		Unit:   "%",
		Scale:  unchangedUnit,
		Read:   func(m ModelInput) Measured { return m.HumidityRelPct },
		Note:   "misma variable y misma unidad que humedad_relativa",
	},
	"wind_speed": {
		Factor: "viento",
		Unit:   "m/s",
		Scale:  metresPerSecondPerKmh,
		Read:   func(m ModelInput) Measured { return m.WindSpeedKmh },
		Note:   "viento convertido de km/h a m/s (÷3.6), la unidad que declara el artefacto",
	},
	"wind_dir": {
		Factor: "direccion_viento",
		Unit:   "deg",
		Scale:  unchangedUnit,
		Read:   func(m ModelInput) Measured { return m.WindDirectionDeg },
		Note:   "lectura instantánea de la dirección del viento (grados, desde donde sopla)",
	},
	"precip_sum": {
		Factor: "precipitacion_acumulada",
		Unit:   "mm",
		Scale:  unchangedUnit,
		Read:   func(m ModelInput) Measured { return m.PrecipitationMM },
		// A sum is an accumulation, so the declared unit is mm over the window, not
		// mm/day. A per-day mean would be a different feature with a different name;
		// if the model ever wants that, the artifact declares it and this mapping
		// follows. The unit check refuses the mismatch rather than converting.
		Note: "acumulado de los últimos siete días locales del historial, en mm",
	},
	"radiation_sol": {
		Factor: "radiacion_solar",
		Unit:   "W/m2",
		Scale:  unchangedUnit,
		Read:   func(m ModelInput) Measured { return m.RadiationWPerM2 },
		Note:   "lectura instantánea en W/m2, tal como la publica la fuente; no se convierte a integral diaria",
	},
}

// inferenceSample builds the request body and the factors that describe it, one
// entry per feature the artifact declared, in the artifact's declared order.
//
// A feature the vector does not carry is sent as an explicit null, which the
// contract defines as "declared but unobserved" and the artifact imputes.
// Omitting the key instead would make the service refuse the sample as missing,
// which is the correct answer for a client bug and the wrong one for a feature the
// model asks for and this deployment cannot supply. That is also why a declared
// feature with no mapping is a null that names itself rather than a skipped entry:
// the body has to cover exactly what the artifact declared, or it is a different
// request than the one the endpoint documents. Absence is never reported as a zero
// reading, for the same reason the rules abstain instead of interpreting a missing
// humidity as 0%, and the reason the vector recorded travels into the factor so a
// stored level says which window was short.
func inferenceSample(inputs []modelInput, features Features) (map[string]*float64, []Factor) {
	sample := make(map[string]*float64, len(inputs))
	factors := make([]Factor, 0, len(inputs))
	for _, input := range inputs {
		if input.Mapping == nil {
			// Nothing this service builds answers this name. The artifact imputes it,
			// and the factor records that this deployment could not supply it: an
			// omitted key would look exactly like a model with fewer inputs.
			sample[input.Feature] = nil
			factors = append(factors, Factor{
				Name: input.Feature,
				Note: fmt.Sprintf("el artefacto declara la característica %q (unidad %q) y este servicio no sabe construirla: se declara nula y el artefacto la imputa", input.Feature, input.Declared),
			})
			continue
		}
		mapping := *input.Mapping
		measured := Measured{}
		if mapping.Read != nil {
			measured = mapping.Read(features.Model)
		}
		factor := Factor{Name: mapping.Factor, Note: mapping.Note + " (" + input.Declared + ")"}
		if measured.Reason != "" {
			factor.Note += ": " + measured.Reason
		}
		reading := measured.Value
		switch {
		case !reading.Present:
			factor.Note += "; sin lectura, se declara nula y el artefacto la imputa"
			sample[input.Feature] = nil
		case math.IsNaN(reading.Value) || math.IsInf(reading.Value, 0):
			// The artifact's own preprocessing maps NaN to its imputed value, so a
			// non-finite reading is declared unobserved instead of being encoded as
			// a number. It is recorded on the factor, never dropped in silence.
			factor.Note += "; el valor recibido no es un número finito, se declara nulo y el artefacto lo imputa"
			sample[input.Feature] = nil
		default:
			value := reading.Value * mapping.Scale
			factor.Known = true
			factor.Value = reading.Value
			sample[input.Feature] = &value
		}
		factors = append(factors, factor)
	}
	return sample, factors
}
