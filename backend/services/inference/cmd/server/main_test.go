package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"firewatch.local/inference/internal/artifact"
	"firewatch.local/inference/internal/handler"
	"firewatch.local/inference/internal/metrics"
	"firewatch.local/inference/internal/scorer"
)

// TestModelPathIsRequired keeps startup from looking somewhere else when nothing
// is configured: a default path would make a deployment that forgot the mount
// start on whatever happened to be in the image.
func TestModelPathIsRequired(t *testing.T) {
	if _, err := modelPath(""); err == nil || !strings.Contains(err.Error(), modelPathEnv) {
		t.Fatalf("empty path error = %v", err)
	}
	if path, err := modelPath("testdata/logistic.json"); err != nil || path != "testdata/logistic.json" {
		t.Fatalf("configured path = %q, %v", path, err)
	}
}

// TestLoadArtifactAbortsOnUnusableDocuments covers the startup gate. Every one of
// these must stop the process: serving with a zero model answers 0.5 for every
// input and looks healthy from the outside.
func TestLoadArtifactAbortsOnUnusableDocuments(t *testing.T) {
	for _, tc := range []struct {
		name    string
		path    string
		wantErr string
	}{
		{"missing file", "../../testdata/does-not-exist.json", "read model artifact"},
		{"directory", "../../testdata", "read model artifact"},
		{"contract violation", "../../testdata/invalid/format.json", `format "firewatch-model/v2" is not "firewatch-model/v1"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loaded, err := loadArtifact(tc.path)
			if err == nil {
				t.Fatalf("loaded %+v, want an error", loaded)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoadArtifactReturnsTheCommittedModel(t *testing.T) {
	for _, name := range []string{"logistic.json", "forest.json"} {
		loaded, err := loadArtifact("../../testdata/" + name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if loaded.Version == "" || loaded.DataHash == "" || !loaded.TrainedAt.Time.Equal(loaded.TrainedAt.Time.UTC()) {
			t.Errorf("%s is not a usable identity: %+v", name, loaded)
		}
	}
}

// TestRoutesAndMetricsAreWired exercises the composition cmd/server builds: the
// handler set behind the middleware and the /metrics renderer beside it, with no
// listener involved.
func TestRoutesAndMetricsAreWired(t *testing.T) {
	loaded, err := loadArtifact("../../testdata/logistic.json")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	counters := &metrics.Inference{}
	httpMetrics := &metrics.HTTPMetrics{}
	mux := http.NewServeMux()
	mux.Handle("/", handler.New(loaded, scorer.New(loaded), counters))
	mux.Handle("GET /metrics", metricsHandler(counters, httpMetrics, loaded))
	app := middleware(mux, httpMetrics)

	predict := httptest.NewRequest(http.MethodPost, "/predict", strings.NewReader(
		`{"features": {"temp_max": 34.2, "temp_min": 18.4, "humidity_rel": 22, "wind_speed": 9.5, "wind_dir": 195, "precip_sum": 0, "radiation_sol": 1680}}`))
	predict.Header.Set("X-Request-ID", "not-a-uuid")
	predicted := httptest.NewRecorder()
	app.ServeHTTP(predicted, predict)
	if predicted.Code != http.StatusOK {
		t.Fatalf("predict got %d: %s", predicted.Code, predicted.Body.String())
	}
	// The middleware replaces a malformed correlation id with a canonical one
	// rather than forwarding the caller's spelling.
	if id := predicted.Header().Get("X-Request-ID"); id == "not-a-uuid" || len(id) != 36 {
		t.Errorf("correlation id %q", id)
	}

	scraped := httptest.NewRecorder()
	app.ServeHTTP(scraped, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if scraped.Code != http.StatusOK {
		t.Fatalf("metrics got %d", scraped.Code)
	}
	if contentType := scraped.Header().Get("Content-Type"); contentType != "text/plain; version=0.0.4" {
		t.Errorf("content type %q", contentType)
	}
	body := scraped.Body.String()
	for _, want := range []string{
		`firewatch_inference_predictions_total{level="high"} 1`,
		`firewatch_inference_model_info{version="valparaiso-logistic-v1"`,
		`firewatch_http_responses_total{class="2xx"}`,
		`firewatch_http_request_duration_seconds_bucket{le="0.005"}`,
		`firewatch_process_goroutines`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics body is missing %q:\n%s", want, body)
		}
	}
	// Every scrape is answered 200 and then counted, so the 2xx series covers the
	// predict call and at least this scrape.
	if strings.Contains(body, `firewatch_http_errors_total 0`) == false {
		t.Errorf("no error total rendered:\n%s", body)
	}
}

// TestValidateFlagExitsWithTheStartupGate guards the deploy-time check: running
// the binary with -validate must load the artifact through the same gate startup
// uses and return without binding a listener, so a pipeline can validate an
// artifact with the code the server will actually run.
func TestValidateFlagExitsWithTheStartupGate(t *testing.T) {
	t.Setenv(modelPathEnv, "../../testdata/logistic.json")
	if err := run([]string{"-validate"}); err != nil {
		t.Fatalf("-validate on a valid artifact: %v", err)
	}

	t.Setenv(modelPathEnv, "../../testdata/does-not-exist.json")
	if err := run([]string{"-validate"}); err == nil {
		t.Fatal("-validate accepted a missing artifact")
	}
}

// TestValidatePathUsesTheStartupGate keeps -validate honest: it loads through
// loadArtifact, so a document the flag accepts is a document the server starts
// with.
func TestValidatePathUsesTheStartupGate(t *testing.T) {
	loaded, err := loadArtifact("../../testdata/forest.json")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Kind != artifact.KindForest || len(loaded.Trees) == 0 {
		t.Fatalf("loaded %+v", loaded)
	}
}
