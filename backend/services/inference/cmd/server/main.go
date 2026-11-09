// Command inference serves the production risk model over the internal network.
//
// The model is a firewatch-model/v1 JSON artifact (see
// internal/artifact) executed by Go. There is no onnxruntime and no cgo, which
// is what keeps this image identical in shape to the other four services:
// CGO_ENABLED=0 into gcr.io/distroless/static-debian12:nonroot.
//
// Startup fails closed. A missing, unparsable or contract-violating artifact
// stops the process before the listener binds; the service never serves with a
// zero model, because a zero model answers 0.5 for every input and looks
// healthy from the outside.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"firewatch.local/inference/internal/artifact"
	"firewatch.local/inference/internal/handler"
	"firewatch.local/inference/internal/metrics"
	"firewatch.local/inference/internal/scorer"
	"github.com/google/uuid"
)

// modelPathEnv is the only required configuration: where the artifact lives.
// There is deliberately no default. A default path would make a deployment that
// forgot to mount the artifact start on whatever happened to be in the image,
// which is the failure this service is written to prevent.
const modelPathEnv = "FIREWATCH_MODEL_PATH"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(os.Args[1:]); err != nil {
		slog.Error("service_stopped", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	// validate is the deploy-time check: parse and validate the artifact, print
	// its identity and exit. It is the same code path startup uses, so a
	// pipeline that runs it cannot pass a document the service would reject.
	flags := flag.NewFlagSet("inference", flag.ContinueOnError)
	validate := flags.Bool("validate", false, "validate the configured artifact and exit")
	if err := flags.Parse(args); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	path, err := modelPath(os.Getenv(modelPathEnv))
	if err != nil {
		return err
	}
	loaded, err := loadArtifact(path)
	if err != nil {
		return err
	}
	if *validate {
		fmt.Printf("artifact ok: %s %s %s\n", loaded.Format, loaded.Version, loaded.DataHash)
		return nil
	}
	counters := &metrics.Inference{}
	httpMetrics := &metrics.HTTPMetrics{}
	mux := http.NewServeMux()
	mux.Handle("/", handler.New(loaded, scorer.New(loaded), counters))
	mux.Handle("GET /metrics", metricsHandler(counters, httpMetrics, loaded))
	server := &http.Server{
		Addr:              ":8080",
		Handler:           middleware(mux, httpMetrics),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16384,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	slog.Info("inference_ready", "model_version", loaded.Version, "data_hash", loaded.DataHash, "kind", loaded.Kind, "features", len(loaded.Features))
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// modelPath validates the one required setting. A blank value is a configuration
// error, not a reason to look somewhere else.
func modelPath(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("%s is required: the service has no model without it", modelPathEnv)
	}
	return raw, nil
}

// loadArtifact is the startup gate. Every error here aborts the process; there
// is no degraded mode, because a service that answers from an unchecked model is
// worse than a service that is down.
func loadArtifact(path string) (*artifact.Artifact, error) {
	loaded, err := artifact.Load(path)
	if err != nil {
		return nil, err
	}
	return loaded, nil
}

// metricsHandler renders the Prometheus text body: the model layer, the active
// model identity, the HTTP layer and the process layer. It is pinned here rather
// than in the metrics package so the package stays free of the artifact type.
func metricsHandler(counters *metrics.Inference, httpMetrics *metrics.HTTPMetrics, model *artifact.Artifact) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprint(w, counters.Text())
		fmt.Fprint(w, metrics.ModelText(model.Version, model.DataHash))
		fmt.Fprint(w, httpMetrics.Text())
		fmt.Fprint(w, metrics.RuntimeText())
	})
}

// middleware gives every request a correlation id, a bounded deadline and an
// observed duration, so no handler has to think about any of the three. It
// mirrors the middleware the other services run.
func middleware(next http.Handler, httpMetrics *metrics.HTTPMetrics) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.Header.Get("X-Request-ID"))
		if err != nil {
			id = uuid.New()
		}
		w.Header().Set("X-Request-ID", id.String())
		r.Header.Set("X-Request-ID", id.String())
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r.WithContext(ctx))
		// A handler that wrote without calling WriteHeader reports 200, which is
		// the code net/http itself would send.
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		httpMetrics.Observe(status, time.Since(started))
	})
}

// statusRecorder captures the response status so the middleware can count
// responses and time the request without changing any handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(body)
}
