package classify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// This file covers the trained-model path: the classifier that delegates scoring
// to the inference service over HTTP.
//
// The failure tests are the point of the file, not the happy path. A model that
// cannot answer must fail the unit closed and say why; it must never fall back to
// a rule, because a fallback would persist a level under the model's version that
// the model never produced.

// fixtureFeatures builds one classifier input. The instant pair is pinned so the
// model path is never exercised with a clock read.
func fixtureFeatures(t *testing.T, present ...func(*Features)) Features {
	t.Helper()
	features := Features{
		ObservedAt:  time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
		EvaluatedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
		ZoneID:      "valparaiso-demo",
	}
	for _, apply := range present {
		apply(&features)
	}
	return features
}

// servePredictor starts an inference stand-in and returns it with the classifier
// configured to call it.
func servePredictor(t *testing.T, timeout time.Duration, handle http.HandlerFunc) (InferenceClassifier, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handle)
	t.Cleanup(server.Close)
	classifier, err := NewMLFixtureV1(server.URL, timeout)
	if err != nil {
		t.Fatalf("configure the fixture classifier: %v", err)
	}
	return classifier, server
}

// predictionBody renders the documented success body.
func predictionBody(level string, probability float64) string {
	document, err := json.Marshal(map[string]any{
		"level":         level,
		"probability":   probability,
		"model_version": "valparaiso-logistic-v1",
	})
	if err != nil {
		panic(err)
	}
	return string(document)
}

// TestInferenceClassifierMapsTheServedProbabilityToItsLevel proves the success
// path: the artifact's own level and probability are mapped onto the levels this
// service persists, and the assessment carries a policy reference a stored level
// can be reproduced from.
func TestInferenceClassifierMapsTheServedProbabilityToItsLevel(t *testing.T) {
	for _, tc := range []struct {
		served string
		want   Level
	}{
		{"low", LevelBajo},
		{"medium", LevelMedio},
		{"high", LevelAlto},
	} {
		t.Run(tc.served, func(t *testing.T) {
			classifier, _ := servePredictor(t, time.Second, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, predictionBody(tc.served, 0.42))
			})
			assessment, err := classifier.Classify(context.Background(), fixtureFeatures(t, func(f *Features) {
				f.RelativeHumidityPct = Known(41)
				f.WindSpeedKmh = Known(18)
			}))
			if err != nil {
				t.Fatalf("classify with the model path: %v", err)
			}
			if assessment.Level != tc.want {
				t.Fatalf("served level %q mapped to %q, want %q", tc.served, assessment.Level, tc.want)
			}
			if err := assessment.Validate(); err != nil {
				t.Fatalf("the assessment is not publishable: %v", err)
			}
			if assessment.Policy.PolicyID != MLFixtureV1ID || assessment.Policy.ParamsHash != MLFixtureV1ParamsHash {
				t.Fatalf("policy reference = %+v, want the fixture version and its frozen fingerprint", assessment.Policy)
			}
			if !strings.Contains(assessment.Rationale, "0.4200") || !strings.Contains(assessment.Rationale, "valparaiso-logistic-v1") {
				t.Fatalf("the rationale does not record the answer it came from: %q", assessment.Rationale)
			}
			if len(assessment.Factors) != len(inferenceInputs) {
				t.Fatalf("assessment carries %d factors, want one per model input (%d)", len(assessment.Factors), len(inferenceInputs))
			}
			// The model does not report per-variable contributions. Recording them
			// as anything but zero would invent evidence.
			for _, factor := range assessment.Factors {
				if factor.Contribution != 0 {
					t.Fatalf("factor %q reports contribution %v, but the inference service reports none", factor.Name, factor.Contribution)
				}
			}
		})
	}
}

// TestInferenceClassifierSendsTheFixtureFeatureMap pins the cross-contract
// mapping: the artifact's feature names, the explicit null for a feature this
// service does not publish, and the unit conversion for the one it publishes in a
// different unit. A silent change to any of the three would be a silent change to
// what the model is asked about.
func TestInferenceClassifierSendsTheFixtureFeatureMap(t *testing.T) {
	var (
		captured map[string]map[string]*float64
		mu       sync.Mutex
	)
	classifier, _ := servePredictor(t, time.Second, func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read the request body: %v", err)
		}
		body := map[string]map[string]*float64{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("the request body is not the documented object: %v (%q)", err, raw)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		// The captured body is handed over under the mutex rather than written to a
		// bare variable, so the assertion after Classify returns is ordered after
		// the handler that produced it (the suite runs under -race in CI).
		mu.Lock()
		captured = body
		mu.Unlock()
		io.WriteString(w, predictionBody("medium", 0.5))
	})
	// 45 km/h is 12.5 m/s: the artifact declares wind_speed in m/s, so sending
	// km/h would be a unit error its guard cannot catch (both are inside
	// [0, 40]).
	assessment, err := classifier.Classify(context.Background(), fixtureFeatures(t, func(f *Features) {
		f.TemperatureC = Known(31.5)
		f.RelativeHumidityPct = Known(22)
		f.WindSpeedKmh = Known(45)
		f.VegetationCoverPct = Known(80)
		f.HistoricalFires = Known(3)
	}))
	if err != nil {
		t.Fatalf("classify with the model path: %v", err)
	}
	mu.Lock()
	features, ok := captured["features"]
	mu.Unlock()
	if !ok {
		t.Fatalf("the request carries no features object: %v", captured)
	}
	want := []string{"temp_max", "temp_min", "humidity_rel", "wind_speed", "wind_dir", "precip_sum", "radiation_sol"}
	if len(features) != len(want) {
		t.Fatalf("the request declares %d features, want the artifact's %d: %v", len(features), len(want), features)
	}
	for _, name := range want {
		if _, declared := features[name]; !declared {
			t.Fatalf("the request does not declare the artifact's feature %q: %v", name, features)
		}
	}
	if value := features["humidity_rel"]; value == nil || *value != 22 {
		t.Fatalf("humidity_rel = %v, want the 22%% reading", value)
	}
	if value := features["wind_speed"]; value == nil || math.Abs(*value-12.5) > 1e-12 {
		t.Fatalf("wind_speed = %v, want 12.5 m/s (45 km/h converted, not sent raw)", value)
	}
	// The canonical temperature is an instantaneous reading; the artifact asks for
	// a daily maximum. Sending it as temp_max would hand the model a value it was
	// never trained to interpret.
	for _, absent := range []string{"temp_max", "temp_min", "wind_dir", "precip_sum", "radiation_sol"} {
		if value := features[absent]; value != nil {
			t.Fatalf("%s = %v, want null: this service publishes no reading that feature can honestly be built from", absent, *value)
		}
	}
	for _, factor := range assessment.Factors {
		if factor.Name == "humedad_relativa" && (!factor.Known || factor.Value != 22) {
			t.Fatalf("the humidity factor does not record the reading it sent: %+v", factor)
		}
		if factor.Name == "temperatura_maxima_diaria" && factor.Known {
			t.Fatalf("the temperature factor claims a reading the artifact never received: %+v", factor)
		}
	}
}

// TestInferenceClassifierFailsClosedOnTimeout protects the bound that makes a
// stalled model a failed unit instead of a stalled queue.
func TestInferenceClassifierFailsClosedOnTimeout(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	classifier, _ := servePredictor(t, 20*time.Millisecond, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	assessment, err := classifier.Classify(context.Background(), fixtureFeatures(t))
	if err == nil {
		t.Fatal("a prediction that outlived its timeout was reported as a level")
	}
	if !errors.Is(err, ErrModelUnavailable) {
		t.Fatalf("timeout error = %v, want it classified as the model being unavailable", err)
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("the recorded reason does not name the timeout: %v", err)
	}
	assertNoFallbackAssessment(t, assessment)
}

// TestInferenceClassifierFailsClosedOnServerError protects a 5xx: the service is
// reachable and did not produce a level.
func TestInferenceClassifierFailsClosedOnServerError(t *testing.T) {
	classifier, _ := servePredictor(t, time.Second, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"code":"scoring_failed","request_id":"req-1","detail":"boom"}`)
	})
	assessment, err := classifier.Classify(context.Background(), fixtureFeatures(t))
	if err == nil {
		t.Fatal("a 500 from the inference service was reported as a level")
	}
	if !errors.Is(err, ErrModelUnavailable) {
		t.Fatalf("5xx error = %v, want it classified as the model being unavailable", err)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("the recorded reason does not name the status: %v", err)
	}
	assertNoFallbackAssessment(t, assessment)
}

// TestInferenceClassifierFailsClosedOnAMalformedBody covers every shape of "the
// service answered something that is not the documented prediction": invalid
// JSON, an empty body, an absent member, a probability outside [0, 1] and an
// unknown level. None of them may be read as a level.
func TestInferenceClassifierFailsClosedOnAMalformedBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"not json", `{"level":"medium","probability":`},
		{"not an object", `["medium",0.5]`},
		{"empty", ``},
		{"no probability", `{"level":"medium","model_version":"valparaiso-logistic-v1"}`},
		{"no level", `{"probability":0.5,"model_version":"valparaiso-logistic-v1"}`},
		{"no model version", `{"level":"medium","probability":0.5}`},
		{"probability above one", `{"level":"medium","probability":1.4,"model_version":"valparaiso-logistic-v1"}`},
		{"probability below zero", `{"level":"low","probability":-0.1,"model_version":"valparaiso-logistic-v1"}`},
		{"unknown level", `{"level":"bajo","probability":0.5,"model_version":"valparaiso-logistic-v1"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			classifier, _ := servePredictor(t, time.Second, func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, tc.body)
			})
			assessment, err := classifier.Classify(context.Background(), fixtureFeatures(t))
			if err == nil {
				t.Fatalf("the body %q was accepted as a prediction", tc.body)
			}
			if !errors.Is(err, ErrModelUnavailable) {
				t.Fatalf("malformed body error = %v, want it classified as the model being unavailable", err)
			}
			assertNoFallbackAssessment(t, assessment)
		})
	}
}

// TestInferenceClassifierDistinguishesARefusalFromAnUnreachableModel is the
// taxonomy test: a 4xx is the model refusing this sample (it is up), while a
// transport failure and a 5xx are the model not answering. The distinction is
// carried by the error value and by the recorded reason, so an operator can tell
// a bad reading from a broken call.
func TestInferenceClassifierDistinguishesARefusalFromAnUnreachableModel(t *testing.T) {
	refusing, _ := servePredictor(t, time.Second, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"code":"unknown_feature","request_id":"req-7","detail":"feature \"temperatura\" is not one this model takes"}`)
	})
	assessment, err := refusing.Classify(context.Background(), fixtureFeatures(t))
	if err == nil {
		t.Fatal("a 400 from the inference service was reported as a level")
	}
	if !errors.Is(err, ErrModelRefused) {
		t.Fatalf("refusal error = %v, want it classified as the model refusing the sample", err)
	}
	if errors.Is(err, ErrModelUnavailable) {
		t.Fatalf("a refusal was also classified as the model being unavailable: %v", err)
	}
	var refusal RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("the refusal is not inspectable: %v", err)
	}
	// The stable code and the request id are what join this failure to the
	// inference service's own record of it.
	if refusal.Code != "unknown_feature" || refusal.RequestID != "req-7" || refusal.Status != http.StatusBadRequest {
		t.Fatalf("refusal = %+v, want the contract's code, request id and status", refusal)
	}
	if !strings.Contains(err.Error(), "unknown_feature") {
		t.Fatalf("the recorded reason does not name the refusal code: %v", err)
	}
	assertNoFallbackAssessment(t, assessment)

	// A closed listener is the other class: nothing answered at all.
	closed, server := servePredictor(t, time.Second, func(w http.ResponseWriter, r *http.Request) {})
	server.Close()
	assessment, err = closed.Classify(context.Background(), fixtureFeatures(t))
	if err == nil {
		t.Fatal("an unreachable inference service was reported as a level")
	}
	if !errors.Is(err, ErrModelUnavailable) || errors.Is(err, ErrModelRefused) {
		t.Fatalf("transport error = %v, want it classified as the model being unavailable and not as a refusal", err)
	}
	assertNoFallbackAssessment(t, assessment)
}

// TestInferenceClassifierNeverProducesAnAssessmentOnFailure is the anti-fallback
// guard stated as one property over every failure mode: an error always comes
// with the zero Assessment, and the zero Assessment cannot be published because
// its level is not canonical. Even a caller that ignored the error therefore
// cannot store a level under this classifier.
func TestInferenceClassifierNeverProducesAnAssessmentOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		body string
	}{
		{"refusal", http.StatusBadRequest, `{"code":"out_of_range","detail":"temp_max 46 is outside [-5, 45]"}`},
		{"server error", http.StatusInternalServerError, `{"code":"scoring_failed"}`},
		{"malformed", http.StatusOK, `not a prediction`},
		{"unexpected status", http.StatusNoContent, ``},
	} {
		t.Run(tc.name, func(t *testing.T) {
			classifier, _ := servePredictor(t, time.Second, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.code)
				io.WriteString(w, tc.body)
			})
			assessment, err := classifier.Classify(context.Background(), fixtureFeatures(t))
			if err == nil {
				t.Fatal("the failure was reported as a level")
			}
			assertNoFallbackAssessment(t, assessment)
		})
	}
	if (InferenceClassifier{}).Kind() != KindTrained {
		t.Fatal("the model path does not declare itself a trained-model path")
	}
	if _, err := (InferenceClassifier{}).Classify(context.Background(), Features{}); err == nil {
		t.Fatal("the zero-value classifier produced an assessment instead of failing closed")
	}
}

// TestInferenceClassifierHonoursACancelledContext keeps the publication path's
// own cancellation authoritative: a cancelled job must not be scored anyway.
func TestInferenceClassifierHonoursACancelledContext(t *testing.T) {
	var calls atomic.Int64
	classifier, _ := servePredictor(t, time.Second, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.WriteString(w, predictionBody("medium", 0.5))
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assessment, err := classifier.Classify(ctx, fixtureFeatures(t))
	if err == nil {
		t.Fatal("a cancelled publication was scored anyway")
	}
	if calls.Load() != 0 {
		t.Fatalf("a cancelled publication reached the inference service %d times", calls.Load())
	}
	assertNoFallbackAssessment(t, assessment)
}

// TestInferenceClassifierConfigurationIsValidatedAtStartup protects the rule
// that a misconfigured model path never becomes a classifier that fails later.
func TestInferenceClassifierConfigurationIsValidatedAtStartup(t *testing.T) {
	valid := InferenceConfig{Version: "ml-test-v1", BaseURL: "http://inference:8080", Timeout: time.Second, Parameters: map[string]string{"artifact": "test"}}
	if _, err := NewInferenceClassifier(valid); err != nil {
		t.Fatalf("a complete configuration was refused: %v", err)
	}
	for _, tc := range []struct {
		name   string
		change func(*InferenceConfig)
	}{
		{"no version", func(c *InferenceConfig) { c.Version = "" }},
		{"no url", func(c *InferenceConfig) { c.BaseURL = "" }},
		{"blank url", func(c *InferenceConfig) { c.BaseURL = "   " }},
		{"relative url", func(c *InferenceConfig) { c.BaseURL = "inference:8080/predict" }},
		{"no host", func(c *InferenceConfig) { c.BaseURL = "http://" }},
		{"wrong scheme", func(c *InferenceConfig) { c.BaseURL = "ftp://inference" }},
		{"zero timeout", func(c *InferenceConfig) { c.Timeout = 0 }},
		{"negative timeout", func(c *InferenceConfig) { c.Timeout = -time.Second }},
		{"no parameters", func(c *InferenceConfig) { c.Parameters = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := valid
			tc.change(&config)
			if _, err := NewInferenceClassifier(config); err == nil {
				t.Fatalf("the configuration was accepted: %+v", config)
			}
		})
	}
	if _, err := NewMLFixtureV1("", time.Second); err == nil {
		t.Fatal("the fixture classifier was built without an address")
	}
}

// TestMLFixtureV1DescribesAnUntrainedArtifact keeps the fixture's provenance
// honest: its params hash must describe the committed fixture, and it must say
// that no training run happened.
func TestMLFixtureV1DescribesAnUntrainedArtifact(t *testing.T) {
	params := MLFixtureV1Params()
	if params["trained_on_observed_fires"] != "false" {
		t.Fatalf("the fixture claims a training run: %q", params["trained_on_observed_fires"])
	}
	if params["artifact_path"] != "services/inference/testdata/logistic.json" {
		t.Fatalf("the fixture description does not name its artifact: %q", params["artifact_path"])
	}
	if got := ParamsHash(params); got != MLFixtureV1ParamsHash {
		t.Fatalf("the frozen fingerprint %q does not describe the current fixture description (%q)", MLFixtureV1ParamsHash, got)
	}
	classifier, err := NewMLFixtureV1("http://inference:8080", time.Second)
	if err != nil {
		t.Fatalf("configure the fixture classifier: %v", err)
	}
	reported, err := classifier.PolicyFingerprint()
	if err != nil {
		t.Fatalf("the fixture classifier cannot report its policy: %v", err)
	}
	if reported != MLFixtureV1ParamsHash {
		t.Fatalf("the classifier reports %q, want the frozen %q", reported, MLFixtureV1ParamsHash)
	}
	if classifier.ID() != MLFixtureV1ID || !ServedByInference(MLFixtureV1ID) {
		t.Fatalf("the fixture identifier %q is not the one startup resolves through INFERENCE_URL", classifier.ID())
	}
	if ServedByInference("firewatch-rule-v2") || ServedByInference("ml-v1") {
		t.Fatal("a version this binary does not execute over HTTP is reported as one it does")
	}
	if got := classifier.endpoint; got != "http://inference:8080/predict" {
		t.Fatalf("endpoint = %q, want the documented scoring route", got)
	}
}

// assertNoFallbackAssessment refuses any assessment that came with an error, at
// the interface level rather than per test: an empty level is not canonical, so
// Assessment.Validate rejects it and the publication path cannot persist it.
func assertNoFallbackAssessment(t *testing.T, assessment Assessment) {
	t.Helper()
	if assessment.Level != "" || len(assessment.Factors) != 0 || assessment.Rationale != "" || assessment.Policy.PolicyID != "" {
		t.Fatalf("a failed prediction produced an assessment: %+v", assessment)
	}
	if err := assessment.Validate(); err == nil {
		t.Fatal("a failed prediction produced a publishable assessment")
	}
}
