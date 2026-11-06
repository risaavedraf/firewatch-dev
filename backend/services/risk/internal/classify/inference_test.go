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

// fixtureModelDocument is the model document the test stand-in serves by default:
// the seven features services/inference/testdata/logistic.json declares, in its
// declared order and with its declared units.
const fixtureModelDocument = `{
  "format": "firewatch-model/v1",
  "version": "valparaiso-logistic-v1",
  "kind": "logistic",
  "features": [
    {"name": "temp_max", "unit": "degC", "min": -5, "max": 45},
    {"name": "temp_min", "unit": "degC", "min": -10, "max": 30},
    {"name": "humidity_rel", "unit": "%", "min": 0, "max": 100},
    {"name": "wind_speed", "unit": "m/s", "min": 0, "max": 40},
    {"name": "wind_dir", "unit": "deg", "min": 0, "max": 360},
    {"name": "precip_sum", "unit": "mm", "min": 0, "max": 200},
    {"name": "radiation_sol", "unit": "W/m2", "min": 0, "max": 1758}
  ]
}`

// servePredictor starts an inference stand-in and returns it with the classifier
// configured to call it. GET /v1/model answers the fixture artifact's own declared
// feature list, and every other request is handed to handle, which is what the
// prediction tests were written against.
func servePredictor(t *testing.T, timeout time.Duration, handle http.HandlerFunc) (InferenceClassifier, *httptest.Server) {
	t.Helper()
	return serveInference(t, timeout, fixtureModelDocument, handle)
}

// serveInference is servePredictor with the model document left to the caller: the
// stand-in's descriptor is the input of the mapping, so a test that changes it is
// a test about which vector gets built.
func serveInference(t *testing.T, timeout time.Duration, document string, handle http.HandlerFunc) (InferenceClassifier, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(modelDocumentPath, modelDocumentHandler(document))
	mux.HandleFunc("/", handle)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	classifier, err := NewMLFixtureV1(context.Background(), server.URL, timeout)
	if err != nil {
		t.Fatalf("configure the fixture classifier: %v", err)
	}
	return classifier, server
}

// modelDocumentHandler answers GET /v1/model with one document verbatim, so a test
// can serve a coherent descriptor, an incoherent one, or something that is not a
// descriptor at all.
func modelDocumentHandler(document string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, document)
	}
}

// serveModelDocument starts a stand-in that serves only the model document, for the
// tests that are about construction rather than about prediction.
func serveModelDocument(t *testing.T, document string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(modelDocumentHandler(document))
	t.Cleanup(server.Close)
	return server
}

// inputNames reports the resolved plan's feature names in order, which is the order
// the artifact declared and the order the vector is built in.
func inputNames(classifier InferenceClassifier) []string {
	names := make([]string, 0, len(classifier.inputs))
	for _, input := range classifier.inputs {
		names = append(names, input.Feature)
	}
	return names
}

// capturePrediction builds a prediction handler that records the features object it
// received and answers one documented success body. The capture is handed over
// under a mutex so the assertion after Classify returns is ordered after the
// handler that produced it (the suite runs under -race in CI).
func capturePrediction(t *testing.T, captured *map[string]map[string]*float64, mu *sync.Mutex) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read the request body: %v", err)
		}
		body := map[string]map[string]*float64{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("the request body is not the documented object: %v (%q)", err, raw)
		}
		mu.Lock()
		*captured = body
		mu.Unlock()
		io.WriteString(w, predictionBody("medium", 0.5))
	}
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
			if len(assessment.Factors) != len(classifier.inputs) {
				t.Fatalf("assessment carries %d factors, want one per model input (%d)", len(assessment.Factors), len(classifier.inputs))
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
// mapping: the artifact's seven feature names, the unit conversion for the one
// canonical reading expressed in a different unit, and the values the model input
// vector supplies.
//
// This test USED TO assert the old two-feature behaviour - two values from the
// rules and five hardcoded nulls - because that is all the publication path could
// answer before the readings history existed. It now asserts the opposite: seven
// populated values, which is the behaviour this slice delivers. The null case is
// pinned separately, by
// TestInferenceClassifierDeclaresAnUncoveredWindowAsNullWithItsCoverage.
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
	// The vector is what internal/features builds at publication time; here it is
	// supplied directly, in the canonical units this service stores, so the unit
	// conversion below is observable.
	assessment, err := classifier.Classify(context.Background(), fixtureFeatures(t, func(f *Features) {
		f.Model = ModelInput{
			TempMaxC:         Measured{Value: Known(31.5), Reason: "máximo del día civil"},
			TempMinC:         Measured{Value: Known(12.5), Reason: "mínimo del día civil"},
			HumidityRelPct:   Measured{Value: Known(22), Reason: "lectura instantánea de humedad_relativa"},
			WindSpeedKmh:     Measured{Value: Known(45), Reason: "lectura instantánea de viento"},
			WindDirectionDeg: Measured{Value: Known(195), Reason: "lectura instantánea de direccion_viento"},
			PrecipitationMM:  Measured{Value: Known(16), Reason: "suma de siete días locales", Coverage: Coverage{Covered: 7, Required: 7, Unit: "días locales"}},
			RadiationWPerM2:  Measured{Value: Known(1680), Reason: "lectura instantánea de radiacion_solar"},
		}
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
	// Every feature of the artifact must be declared, and every one the vector
	// measured must travel with its value: a null here would be the two-feature
	// behaviour this slice replaces.
	want := map[string]float64{
		"temp_max":      31.5,
		"temp_min":      12.5,
		"humidity_rel":  22,
		"wind_dir":      195,
		"precip_sum":    16,
		"radiation_sol": 1680,
	}
	if len(features) != len(want)+1 {
		t.Fatalf("the request declares %d features, want the artifact's %d: %v", len(features), len(want)+1, features)
	}
	for name, expected := range want {
		value, declared := features[name]
		if !declared || value == nil {
			t.Fatalf("%s is not declared with a value: %v", name, features)
		}
		if math.Abs(*value-expected) > 1e-12 {
			t.Fatalf("%s = %v, want %v", name, *value, expected)
		}
	}
	// 45 km/h is 12.5 m/s: the artifact declares wind_speed in m/s, so sending
	// km/h would be a unit error its guard cannot catch (both are inside [0, 40]).
	if value := features["wind_speed"]; value == nil || math.Abs(*value-12.5) > 1e-12 {
		t.Fatalf("wind_speed = %v, want 12.5 m/s (45 km/h converted, not sent raw)", value)
	}
	// The factors carry the canonical, unconverted reading, so an operator can join
	// a persisted factor to the history row it came from.
	wantFactors := map[string]float64{
		"temperatura_maxima_diaria": 31.5,
		"temperatura_minima_diaria": 12.5,
		"humedad_relativa":          22,
		"viento":                    45,
		"direccion_viento":          195,
		"precipitacion_acumulada":   16,
		"radiacion_solar":           1680,
	}
	recorded := make(map[string]Factor, len(assessment.Factors))
	for _, factor := range assessment.Factors {
		recorded[factor.Name] = factor
	}
	if len(recorded) != len(wantFactors) {
		t.Fatalf("the assessment carries %d factors, want %d: %+v", len(recorded), len(wantFactors), assessment.Factors)
	}
	for name, expected := range wantFactors {
		factor, ok := recorded[name]
		if !ok {
			t.Fatalf("no factor %q: %+v", name, assessment.Factors)
		}
		if !factor.Known || factor.Value != expected {
			t.Fatalf("factor %q = %+v, want the canonical reading %v", name, factor, expected)
		}
		// The model does not report per-variable contributions. Recording them as
		// anything but zero would invent evidence.
		if factor.Contribution != 0 {
			t.Fatalf("factor %q reports contribution %v, but the inference service reports none", name, factor.Contribution)
		}
	}
}

// TestInferenceClassifierDeclaresAnUncoveredWindowAsNullWithItsCoverage is the
// other half of the mapping: when the history cannot cover a window, the feature
// travels as an explicit null - never as zero - and the persisted factor keeps the
// reason, including how much of the window was covered.
func TestInferenceClassifierDeclaresAnUncoveredWindowAsNullWithItsCoverage(t *testing.T) {
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
		mu.Lock()
		captured = body
		mu.Unlock()
		io.WriteString(w, predictionBody("low", 0.1))
	})
	assessment, err := classifier.Classify(context.Background(), fixtureFeatures(t, func(f *Features) {
		f.Model = ModelInput{
			TempMaxC: Measured{Value: Known(30), Reason: "máximo del día civil"},
			PrecipitationMM: Measured{
				Value:    Unknown(),
				Reason:   "ventana incompleta: 0 de 7 días locales con lectura de precipitacion; se declara nula y el artefacto la imputa",
				Coverage: Coverage{Covered: 0, Required: 7, Unit: "días locales"},
			},
		}
	}))
	if err != nil {
		t.Fatalf("classify with the model path: %v", err)
	}
	mu.Lock()
	features := captured["features"]
	mu.Unlock()
	value, declared := features["precip_sum"]
	if !declared || value != nil {
		t.Fatalf("precip_sum = %v (declared=%v), want an explicit null on an uncovered window", value, declared)
	}
	if value := features["temp_max"]; value == nil || *value != 30 {
		t.Fatalf("temp_max = %v, want 30: one absent window must not suppress a present feature", value)
	}
	for _, factor := range assessment.Factors {
		if factor.Name != "precipitacion_acumulada" {
			continue
		}
		if factor.Known {
			t.Fatalf("the precipitation factor claims a value the history could not supply: %+v", factor)
		}
		if !strings.Contains(factor.Note, "0 de 7 días locales") {
			t.Fatalf("the factor note %q does not keep the coverage reason", factor.Note)
		}
		return
	}
	t.Fatal("the assessment carries no precipitation factor")
}

// TestInferenceClassifierSendsExactlyTheSubsetTheArtifactDeclares is the change
// this slice makes, stated as a test: the vector is the artifact's declared list,
// not this package's table. An artifact that takes two of the readings this service
// can build gets two values and nothing else, and the plan keeps the artifact's
// order.
func TestInferenceClassifierSendsExactlyTheSubsetTheArtifactDeclares(t *testing.T) {
	document := `{"features":[
		{"name":"humidity_rel","unit":"%","min":0,"max":100},
		{"name":"wind_dir","unit":"deg","min":0,"max":360}
	]}`
	var (
		captured map[string]map[string]*float64
		mu       sync.Mutex
	)
	classifier, _ := serveInference(t, time.Second, document, capturePrediction(t, &captured, &mu))
	// The input vector is fully populated, including readings the artifact does not
	// declare: which of them travel is the artifact's decision, and this test is
	// about what actually does.
	assessment, err := classifier.Classify(context.Background(), fixtureFeatures(t, func(f *Features) {
		f.Model = ModelInput{
			TempMaxC:         Measured{Value: Known(31.5), Reason: "máximo del día civil"},
			HumidityRelPct:   Measured{Value: Known(22), Reason: "lectura instantánea de humedad_relativa"},
			WindDirectionDeg: Measured{Value: Known(195), Reason: "lectura instantánea de direccion_viento"},
		}
	}))
	if err != nil {
		t.Fatalf("classify with the model path: %v", err)
	}
	mu.Lock()
	features := captured["features"]
	mu.Unlock()
	if len(features) != 2 {
		t.Fatalf("the request declares %d features, want exactly the artifact's 2: %v", len(features), features)
	}
	for name, expected := range map[string]float64{"humidity_rel": 22, "wind_dir": 195} {
		value, declared := features[name]
		if !declared || value == nil || *value != expected {
			t.Fatalf("%s = %v (declared=%v), want %v: %v", name, value, declared, expected, features)
		}
	}
	// The subset is also the plan's order, and the factors describe the same two
	// inputs: a factor for a feature that was not sent would be invented evidence.
	if got := strings.Join(inputNames(classifier), ","); got != "humidity_rel,wind_dir" {
		t.Fatalf("the resolved plan is %q, want the artifact's declared order", got)
	}
	if len(assessment.Factors) != 2 {
		t.Fatalf("the assessment carries %d factors, want one per declared feature: %+v", len(assessment.Factors), assessment.Factors)
	}
}

// TestInferenceClassifierDeclaresAnUnmappedFeatureAsNullNamingIt is the other half
// of decoding the artifact's list: a feature it declares and this service cannot
// build must appear in the vector as an explicit null that names itself, never as a
// silently omitted key. An omission would make the body a different request than
// the one the endpoint documents, and the service would refuse it as a missing
// feature - or, worse, a model with an imputation would never learn it was guessed.
func TestInferenceClassifierDeclaresAnUnmappedFeatureAsNullNamingIt(t *testing.T) {
	document := `{"features":[
		{"name":"temp_max","unit":"degC","min":-5,"max":45},
		{"name":"soil_moisture","unit":"%","min":0,"max":100},
		{"name":"wind_speed","unit":"m/s","min":0,"max":40}
	]}`
	var (
		captured map[string]map[string]*float64
		mu       sync.Mutex
	)
	classifier, _ := serveInference(t, time.Second, document, capturePrediction(t, &captured, &mu))
	assessment, err := classifier.Classify(context.Background(), fixtureFeatures(t, func(f *Features) {
		f.Model = ModelInput{
			TempMaxC:     Measured{Value: Known(30), Reason: "máximo del día civil"},
			WindSpeedKmh: Measured{Value: Known(36), Reason: "lectura instantánea de viento"},
		}
	}))
	if err != nil {
		t.Fatalf("classify with the model path: %v", err)
	}
	mu.Lock()
	features := captured["features"]
	mu.Unlock()
	// The vector covers every declared feature, in order, including the one nobody
	// here can build.
	if got := strings.Join(inputNames(classifier), ","); got != "temp_max,soil_moisture,wind_speed" {
		t.Fatalf("the resolved plan is %q, want the artifact's declared order", got)
	}
	if len(features) != 3 {
		t.Fatalf("the request declares %d features, want the artifact's 3: %v", len(features), features)
	}
	if value, declared := features["soil_moisture"]; !declared || value != nil {
		t.Fatalf("soil_moisture = %v (declared=%v), want an explicit null", value, declared)
	}
	if value := features["wind_speed"]; value == nil || math.Abs(*value-10) > 1e-12 {
		t.Fatalf("wind_speed = %v, want 10 m/s (36 km/h converted): one unmapped feature must not disturb the mapped ones", value)
	}
	// The null travels with a reason that names the artifact's feature, so a stored
	// level says which input this deployment could not supply.
	for _, factor := range assessment.Factors {
		if factor.Name != "soil_moisture" {
			continue
		}
		if factor.Known {
			t.Fatalf("the unmapped feature claims a value it never had: %+v", factor)
		}
		if !strings.Contains(factor.Note, "soil_moisture") {
			t.Fatalf("the factor note %q does not name the artifact feature it could not build", factor.Note)
		}
		return
	}
	t.Fatalf("the assessment carries no factor for the unmapped feature: %+v", assessment.Factors)
}

// TestInferenceClassifierRefusesAnIncoherentModelDocument makes the descriptor's
// coherence a startup failure. Each of these would otherwise produce a vector that
// is wrong in a way only a prediction could reveal: a duplicate has no single
// position, an empty list sends nothing, and a unit that contradicts the mapping
// sends one unit where the artifact reads another - inside its own min/max guard, so
// nothing downstream can catch it.
func TestInferenceClassifierRefusesAnIncoherentModelDocument(t *testing.T) {
	for _, tc := range []struct {
		name     string
		document string
		want     []string
	}{
		{
			name:     "duplicate feature",
			document: `{"features":[{"name":"humidity_rel","unit":"%"},{"name":"humidity_rel","unit":"%"}]}`,
			want:     []string{"humidity_rel", "more than once"},
		},
		{
			name:     "empty feature list",
			document: `{"features":[]}`,
			want:     []string{"empty feature list"},
		},
		{
			name:     "no feature list",
			document: `{"format":"firewatch-model/v1","version":"valparaiso-logistic-v1"}`,
			want:     []string{"no feature list"},
		},
		{
			name:     "unit that contradicts the mapping",
			document: `{"features":[{"name":"wind_speed","unit":"km/h","min":0,"max":144}]}`,
			want:     []string{"wind_speed", `"km/h"`, `"m/s"`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := serveModelDocument(t, tc.document)
			classifier, err := NewMLFixtureV1(context.Background(), server.URL, time.Second)
			if err == nil {
				t.Fatalf("the descriptor was accepted and produced %v", inputNames(classifier))
			}
			// The failure has to name the inference service and the problem: an
			// operator reading a stopped container needs both.
			if !strings.Contains(err.Error(), "the inference service") {
				t.Fatalf("the startup error does not name the inference service: %v", err)
			}
			for _, fragment := range tc.want {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("the startup error does not name %q: %v", fragment, err)
				}
			}
		})
	}
}

// TestInferenceClassifierRefusesToStartWithoutItsModelDocument is the startup
// guard: a service that is unreachable, or that answers something which is not a
// model document, must not become a classifier with an empty or guessed feature
// set. Guessing is what this replaces - the seven names used to be hardcoded, so a
// service that was down produced a plausible-looking vector instead of a failure.
func TestInferenceClassifierRefusesToStartWithoutItsModelDocument(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"server error", http.StatusInternalServerError, `{"code":"scoring_failed","detail":"boom"}`, "500"},
		{"not found", http.StatusNotFound, `{"code":"not_found"}`, "404"},
		{"not json", http.StatusOK, `<html>model down</html>`, "not the documented model document"},
		{"empty body", http.StatusOK, ``, "not the documented model document"},
		{"features is not a list", http.StatusOK, `{"features":{"temp_max":1}}`, "not the documented model document"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			t.Cleanup(server.Close)
			classifier, err := NewMLFixtureV1(context.Background(), server.URL, time.Second)
			if err == nil {
				t.Fatalf("the service started on a body it cannot read, with %v", inputNames(classifier))
			}
			if !strings.Contains(err.Error(), "the inference service") || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("the startup error does not name the inference service and %q: %v", tc.want, err)
			}
		})
	}

	// A closed listener is the other class: nothing answered at all.
	closed := httptest.NewServer(modelDocumentHandler(fixtureModelDocument))
	closed.Close()
	classifier, err := NewMLFixtureV1(context.Background(), closed.URL, 200*time.Millisecond)
	if err == nil {
		t.Fatalf("an unreachable inference service produced a classifier with %v", inputNames(classifier))
	}
	if !strings.Contains(err.Error(), "the inference service") || !strings.Contains(err.Error(), modelDocumentPath) {
		t.Fatalf("the startup error does not name the inference service and the route it could not read: %v", err)
	}
}

// TestInferenceClassifierReadsTheModelDocumentOnce protects the cache: the
// descriptor is read at construction and never again, because the inference service
// loads its artifact before it starts serving and never reloads it. A second read
// per prediction would be a request the service's own contract does not promise to
// keep answering, and it would make the vector's shape change under a running
// process.
func TestInferenceClassifierReadsTheModelDocumentOnce(t *testing.T) {
	var documents atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc(modelDocumentPath, func(w http.ResponseWriter, r *http.Request) {
		documents.Add(1)
		modelDocumentHandler(fixtureModelDocument)(w, r)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, predictionBody("medium", 0.5))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	classifier, err := NewMLFixtureV1(context.Background(), server.URL, time.Second)
	if err != nil {
		t.Fatalf("configure the fixture classifier: %v", err)
	}
	for range 3 {
		if _, err := classifier.Classify(context.Background(), fixtureFeatures(t)); err != nil {
			t.Fatalf("classify with the model path: %v", err)
		}
	}
	if got := documents.Load(); got != 1 {
		t.Fatalf("the model document was read %d times, want exactly once at construction", got)
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
//
// The address cases are refused before any request is made, so they need no
// stand-in; the accepted configuration points at one, because construction now also
// reads the artifact's model document.
func TestInferenceClassifierConfigurationIsValidatedAtStartup(t *testing.T) {
	server := serveModelDocument(t, fixtureModelDocument)
	valid := InferenceConfig{Version: "ml-test-v1", BaseURL: server.URL, Timeout: time.Second, Parameters: map[string]string{"artifact": "test"}}
	if _, err := NewInferenceClassifier(context.Background(), valid); err != nil {
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
			if _, err := NewInferenceClassifier(context.Background(), config); err == nil {
				t.Fatalf("the configuration was accepted: %+v", config)
			}
		})
	}
	if _, err := NewMLFixtureV1(context.Background(), "", time.Second); err == nil {
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
	server := serveModelDocument(t, fixtureModelDocument)
	classifier, err := NewMLFixtureV1(context.Background(), server.URL, time.Second)
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
	if got := classifier.endpoint; got != server.URL+predictionPath {
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
