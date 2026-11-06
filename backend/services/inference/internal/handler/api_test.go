package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"firewatch.local/inference/internal/artifact"
	"firewatch.local/inference/internal/scorer"
)

// fakeCounter records what the handler reported, so the tests can prove the
// observability seam is wired and not merely present.
type fakeCounter struct {
	levels []artifact.Level
	codes  []string
}

func (f *fakeCounter) Prediction(level artifact.Level) { f.levels = append(f.levels, level) }
func (f *fakeCounter) Refused(code string)             { f.codes = append(f.codes, code) }

// fakePredictor stands in for the model when a test needs an answer the real one
// cannot produce, such as a non-finite probability.
type fakePredictor struct {
	result scorer.Result
	err    error
	calls  int
}

func (f *fakePredictor) Score(scorer.Sample) (scorer.Result, error) {
	f.calls++
	return f.result, f.err
}

const allFeatures = `"temp_max": 34.2, "temp_min": 18.4, "humidity_rel": 22, "wind_speed": 9.5, "wind_dir": 195, "precip_sum": 0, "radiation_sol": 1680`

func loadModel(t *testing.T, name string) *artifact.Artifact {
	t.Helper()
	loaded, err := artifact.Load("../../testdata/" + name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return loaded
}

// server builds the real handler over a fixture with the real scorer and a
// recording counter.
func server(t *testing.T, fixture string) (http.Handler, *fakeCounter) {
	t.Helper()
	counters := &fakeCounter{}
	return New(loadModel(t, fixture), nil, counters), counters
}

func post(t *testing.T, app http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/predict", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", "6f1d0b0e-6a3f-4b9a-9a3d-2f3c1d4e5f60")
	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, request)
	return recorder
}

func get(app http.Handler, target string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

func decode[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()
	var body T
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v: %s", err, recorder.Body.String())
	}
	return body
}

// TestPredictAnswersWithLevelProbabilityAndVersion pins the documented success
// body and the fact that a real sample scores through the real artifact.
func TestPredictAnswersWithLevelProbabilityAndVersion(t *testing.T) {
	app, counters := server(t, "logistic.json")
	recorder := post(t, app, `{"features": {`+allFeatures+`}}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("got %d: %s", recorder.Code, recorder.Body.String())
	}
	body := decode[map[string]any](t, recorder)
	if body["level"] != "high" {
		t.Errorf("level %v want high", body["level"])
	}
	if body["model_version"] != "valparaiso-logistic-v1" {
		t.Errorf("model_version %v", body["model_version"])
	}
	probability, ok := body["probability"].(float64)
	if !ok || probability < 0 || probability > 1 {
		t.Fatalf("probability %v is not a number in [0, 1]", body["probability"])
	}
	if len(counters.levels) != 1 || counters.levels[0] != artifact.LevelHigh {
		t.Errorf("counter levels %v", counters.levels)
	}
	if recorder.Header().Get("Content-Type") != "application/json" {
		t.Errorf("content type %q", recorder.Header().Get("Content-Type"))
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("missing hardening headers: %v", recorder.Header())
	}
}

// TestPredictLevelUsesArtifactThresholdsOrDefaults proves the cut points come
// from the artifact when it declares them and from the documented defaults when
// it does not, through the HTTP surface rather than through the artifact alone.
func TestPredictLevelUsesArtifactThresholdsOrDefaults(t *testing.T) {
	for _, tc := range []struct {
		fixture     string
		probability float64
		want        string
	}{
		// logistic.json declares medium 0.35, high 0.65.
		{"logistic.json", 0.34, "low"},
		{"logistic.json", 0.35, "medium"},
		{"logistic.json", 0.649, "medium"},
		{"logistic.json", 0.65, "high"},
		// forest.json declares no thresholds, so the documented 0.40/0.70 apply.
		{"forest.json", 0.39, "low"},
		{"forest.json", 0.40, "medium"},
		{"forest.json", 0.699, "medium"},
		{"forest.json", 0.70, "high"},
	} {
		t.Run(fmt.Sprintf("%s@%v", tc.fixture, tc.probability), func(t *testing.T) {
			predictor := &fakePredictor{result: scorer.Result{Probability: tc.probability, Version: "v"}}
			app := New(loadModel(t, tc.fixture), predictor, &fakeCounter{})
			body := decode[map[string]any](t, post(t, app, `{"features": {`+allFeatures+`}}`))
			if body["level"] != tc.want {
				t.Fatalf("level %v want %q", body["level"], tc.want)
			}
			if body["probability"] != tc.probability {
				t.Errorf("probability %v want %v", body["probability"], tc.probability)
			}
		})
	}
}

// TestPredictRefusalsAreStableMachineReadableCodes covers the request shapes the
// contract refuses. The code is what a caller matches on, so each case asserts
// the exact literal, the 400 status and that the refusal was counted.
func TestPredictRefusalsAreStableMachineReadableCodes(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		code string
	}{
		{
			name: "missing required feature",
			body: `{"features": {"temp_max": 30, "temp_min": 12, "humidity_rel": 40, "wind_speed": 5, "wind_dir": 180, "precip_sum": 0}}`,
			code: "missing_feature",
		},
		{
			name: "empty features object",
			body: `{"features": {}}`,
			code: "missing_feature",
		},
		{
			name: "features member absent",
			body: `{}`,
			code: "invalid_body",
		},
		{
			name: "features is not an object",
			body: `{"features": [1, 2, 3]}`,
			code: "invalid_body",
		},
		{
			name: "malformed json",
			body: `{"features": {`,
			code: "invalid_body",
		},
		{
			name: "unknown top level member",
			body: `{"features": {` + allFeatures + `}, "explain": true}`,
			code: "invalid_body",
		},
		{
			name: "value is not a number",
			body: `{"features": {"temp_max": "34.2"}}`,
			code: "invalid_body",
		},
		{
			name: "two json documents",
			body: `{"features": {` + allFeatures + `}} {"features": {}}`,
			code: "invalid_body",
		},
		{
			name: "duplicate feature",
			body: `{"features": {"temp_max": 34.2, "temp_max": 30, "temp_min": 18.4, "humidity_rel": 22, "wind_speed": 9.5, "wind_dir": 195, "precip_sum": 0, "radiation_sol": 1680}}`,
			code: "duplicate_feature",
		},
		{
			name: "unknown feature alongside every required one",
			body: `{"features": {` + allFeatures + `, "pressure_atm": 1013}}`,
			code: "unknown_feature",
		},
		{
			name: "value above the declared guard",
			body: `{"features": {"temp_max": 46, "temp_min": 18.4, "humidity_rel": 22, "wind_speed": 9.5, "wind_dir": 195, "precip_sum": 0, "radiation_sol": 1680}}`,
			code: "out_of_range",
		},
		{
			name: "radiation in the wrong unit",
			body: `{"features": {"temp_max": 34.2, "temp_min": 18.4, "humidity_rel": 22, "wind_speed": 9.5, "wind_dir": 195, "precip_sum": 0, "radiation_sol": 2400}}`,
			code: "out_of_range",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, counters := server(t, "logistic.json")
			recorder := post(t, app, tc.body)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("got %d want 400: %s", recorder.Code, recorder.Body.String())
			}
			body := decode[map[string]any](t, recorder)
			if body["code"] != tc.code {
				t.Fatalf("code %v want %q (%v)", body["code"], tc.code, body["detail"])
			}
			// The error body echoes the correlation id the middleware minted, so a
			// caller can join its own log line to this refusal.
			if body["request_id"] != recorder.Header().Get("X-Request-ID") {
				t.Errorf("body request_id %v and header %q must be one literal", body["request_id"], recorder.Header().Get("X-Request-ID"))
			}
			if detail, ok := body["detail"].(string); !ok || detail == "" {
				t.Errorf("refusal carries no detail: %v", body["detail"])
			}
			if len(counters.codes) != 1 || counters.codes[0] != tc.code {
				t.Errorf("counter codes %v", counters.codes)
			}
			if len(counters.levels) != 0 {
				t.Errorf("a refusal must not count as an answer: %v", counters.levels)
			}
		})
	}
}

// TestPredictImputesDeclaredButUnobservedValue pins the one meaning a JSON null
// has on this surface. The key being present is what distinguishes it from a
// missing feature, which is a 400 above.
func TestPredictImputesDeclaredButUnobservedValue(t *testing.T) {
	app, _ := server(t, "logistic.json")
	recorder := post(t, app, `{"features": {"temp_max": 34.2, "temp_min": null, "humidity_rel": 22, "wind_speed": 9.5, "wind_dir": 195, "precip_sum": 0, "radiation_sol": 1680}}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("got %d want 200: %s", recorder.Code, recorder.Body.String())
	}

	explicit := post(t, app, `{"features": {"temp_max": 34.2, "temp_min": 12, "humidity_rel": 22, "wind_speed": 9.5, "wind_dir": 195, "precip_sum": 0, "radiation_sol": 1680}}`)
	if recorder.Body.String() != explicit.Body.String() {
		t.Fatalf("imputed answer %s differs from the explicit one %s", recorder.Body.String(), explicit.Body.String())
	}
}

// TestPredictNonFiniteProbabilityIsAnErrorNotAResponse pins the failure this unit
// is about: an overflowing model must be reported as a 500 with its own code, and
// the body must never carry a null or an absurd probability a caller could read
// as "no risk".
func TestPredictNonFiniteProbabilityIsAnErrorNotAResponse(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code string
	}{
		{"non-finite probability", fmt.Errorf("%w: got NaN", scorer.ErrNonFiniteProbability), "non_finite_probability"},
		{"standardization overflow", fmt.Errorf("%w: feature \"temp_max\" standardized to +Inf", scorer.ErrNonFiniteProbability), "non_finite_probability"},
		{"unexpected failure", errors.New("model exploded"), "scoring_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			predictor := &fakePredictor{err: tc.err}
			counters := &fakeCounter{}
			app := New(loadModel(t, "logistic.json"), predictor, counters)
			recorder := post(t, app, `{"features": {`+allFeatures+`}}`)
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("got %d want 500: %s", recorder.Code, recorder.Body.String())
			}
			body := decode[map[string]any](t, recorder)
			if body["code"] != tc.code {
				t.Fatalf("code %v want %q", body["code"], tc.code)
			}
			if _, present := body["probability"]; present {
				t.Fatalf("a failed score must not carry a probability: %s", recorder.Body.String())
			}
			if len(counters.levels) != 0 {
				t.Errorf("a failure must not count as an answer: %v", counters.levels)
			}
		})
	}
}

// TestHealthReportsModelIdentity covers the readiness body: it is ready by
// construction, and it names the artifact so a caller can tell which model it is
// talking to before it sends a sample.
func TestHealthReportsModelIdentity(t *testing.T) {
	app, _ := server(t, "logistic.json")
	recorder := get(app, "/health")
	if recorder.Code != http.StatusOK {
		t.Fatalf("got %d: %s", recorder.Code, recorder.Body.String())
	}
	body := decode[map[string]any](t, recorder)
	for key, want := range map[string]any{
		"status":        "ready",
		"format":        "firewatch-model/v1",
		"model_version": "valparaiso-logistic-v1",
		"data_hash":     "9f2c41d7b0e35a6c18d4f7e2a9b03c5d6e8f1a2b4c7d9e0f3a5b6c8d1e2f4a7b",
		"kind":          "logistic",
		"feature_count": float64(7),
	} {
		if body[key] != want {
			t.Errorf("%s = %v, want %v", key, body[key], want)
		}
	}
	thresholds, ok := body["thresholds"].(map[string]any)
	if !ok || thresholds["medium"] != 0.35 || thresholds["high"] != 0.65 {
		t.Errorf("thresholds %v", body["thresholds"])
	}
	trainedAt, ok := body["trained_at"].(string)
	if !ok {
		t.Fatalf("trained_at is %v, want a string", body["trained_at"])
	}
	if _, err := time.Parse(time.RFC3339, trainedAt); err != nil {
		t.Errorf("trained_at %q is not RFC3339: %v", trainedAt, err)
	}
}

// TestModelDocumentExposesProvenanceAndNoParameters pins what /v1/model is for:
// the numbers a model was chosen on stay attached to it, and the parameters stay
// in the artifact file rather than on an endpoint a caller could fit against.
func TestModelDocumentExposesProvenanceAndNoParameters(t *testing.T) {
	app, _ := server(t, "logistic.json")
	recorder := get(app, "/v1/model")
	if recorder.Code != http.StatusOK {
		t.Fatalf("got %d: %s", recorder.Code, recorder.Body.String())
	}
	body := decode[map[string]any](t, recorder)
	for _, key := range []string{"metrics", "baseline"} {
		provenance, ok := body[key].(map[string]any)
		if !ok || len(provenance) == 0 {
			t.Errorf("%s is not an object: %v", key, body[key])
		}
	}
	// The two provenance objects are carried verbatim, not summarised.
	metrics := body["metrics"].(map[string]any)
	if metrics["rows"] != float64(90) || metrics["positives"] != float64(9) {
		t.Errorf("metrics were not carried verbatim: %v", metrics)
	}
	features, ok := body["features"].([]any)
	if !ok || len(features) != 7 {
		t.Fatalf("features %v", body["features"])
	}
	first := features[0].(map[string]any)
	if first["name"] != "temp_max" || first["unit"] != "degC" {
		t.Errorf("features[0] %v", first)
	}
	for _, parameter := range []string{"coef", "intercept", "impute", "center", "scale", "trees"} {
		if _, present := body[parameter]; present {
			t.Errorf("%q must not be exposed by the provenance endpoint", parameter)
		}
	}
	if recorder.Body.String() == "" || strings.Contains(recorder.Body.String(), "coef") {
		t.Errorf("body leaks a parameter: %s", recorder.Body.String())
	}
}

// TestModelDocumentRendersEmptyProvenanceAsObjects keeps the document's shape
// stable for an artifact that carries neither object.
func TestModelDocumentRendersEmptyProvenanceAsObjects(t *testing.T) {
	model := loadModel(t, "forest.json")
	model.Metrics = nil
	model.Baseline = nil
	app := New(model, nil, &fakeCounter{})
	recorder := get(app, "/v1/model")
	if recorder.Code != http.StatusOK {
		t.Fatalf("got %d: %s", recorder.Code, recorder.Body.String())
	}
	body := decode[map[string]any](t, recorder)
	for _, key := range []string{"metrics", "baseline"} {
		if _, ok := body[key].(map[string]any); !ok {
			t.Errorf("%s is %v, want an object", key, body[key])
		}
	}
}

// TestUnknownRoutesAndMethods keeps the surface small: nothing but the three
// documented routes answers, and a GET on /predict is a method error rather than
// an accidental empty prediction.
func TestUnknownRoutesAndMethods(t *testing.T) {
	app, _ := server(t, "logistic.json")
	if recorder := get(app, "/predict"); recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /predict got %d want 405", recorder.Code)
	}
	if recorder := get(app, "/v1/predict"); recorder.Code != http.StatusNotFound {
		t.Errorf("GET /v1/predict got %d want 404", recorder.Code)
	}
	if recorder := get(app, "/metrics"); recorder.Code != http.StatusNotFound {
		t.Errorf("GET /metrics is mounted by cmd/server, not here; got %d", recorder.Code)
	}
}

// TestNilPredictorFallsBackToTheRealScorer keeps New's documented behaviour: a
// caller that supplies only an artifact still gets a working surface, which is
// what cmd/server relies on.
func TestNilPredictorFallsBackToTheRealScorer(t *testing.T) {
	app := New(loadModel(t, "logistic.json"), nil, nil)
	recorder := post(t, app, `{"features": {`+allFeatures+`}}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("got %d: %s", recorder.Code, recorder.Body.String())
	}
	if probability := decode[map[string]any](t, recorder)["probability"].(float64); math.IsNaN(probability) || probability <= 0 {
		t.Fatalf("probability %v", probability)
	}
}
