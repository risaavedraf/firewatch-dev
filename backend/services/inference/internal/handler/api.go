// Package handler is the HTTP surface of the inference service: POST /predict,
// GET /health, GET /v1/model. /metrics is mounted by cmd/server beside this
// handler, exactly as the other services do.
//
// The service is not published through the gateway (S3): risk calls it on the
// internal network. Every response therefore carries no CORS, no bearer token
// and no rate limit of its own; the boundary that keeps arbitrary callers off
// the model is the network, and the contract here is written for that caller.
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"firewatch.local/inference/internal/artifact"
	"firewatch.local/inference/internal/scorer"
	"github.com/google/uuid"
)

// Stable response codes for failures that are not sample violations. The sample
// codes live in the scorer package, where the rejection happens.
const (
	CodeInvalidBody                = "invalid_body"
	CodeNonFiniteProbability       = "non_finite_probability"
	CodeScoringFailed              = "scoring_failed"
	CodeMethodNotAllowed           = "method_not_allowed"
	CodeNotFound                   = "not_found"
	maxRequestBytes          int64 = 64 << 10
)

// Predictor is the scoring seam. Production uses the real scorer over the loaded
// artifact; tests use a fake to pin how a non-finite probability or an internal
// failure is reported, which cannot be provoked through a validated artifact.
type Predictor interface {
	Score(scorer.Sample) (scorer.Result, error)
}

// Counter is the observability seam: the handler reports what it answered and
// what it refused and never reads it back. Production passes
// *metrics.Inference; keeping the handler on this interface means the HTTP layer
// has no opinion about how the signal is rendered.
type Counter interface {
	Prediction(artifact.Level)
	Refused(code string)
}

// API is the handler set over one loaded artifact.
type API struct {
	artifact  *artifact.Artifact
	predictor Predictor
	counters  Counter
}

// New wires the surface. A nil counters is replaced by a fresh no-op so a test
// never has to care, and a nil predictor falls back to the real scorer over the
// artifact.
func New(a *artifact.Artifact, predictor Predictor, counters Counter) http.Handler {
	if counters == nil {
		counters = discardCounter{}
	}
	if predictor == nil {
		predictor = scorer.New(a)
	}
	api := &API{artifact: a, predictor: predictor, counters: counters}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /predict", api.predict)
	mux.HandleFunc("GET /health", api.health)
	mux.HandleFunc("GET /v1/model", api.model)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The correlation id is minted here, not only in the process middleware,
		// so this handler set is self-contained: mounted under a server that
		// normalizes the id it arrives on the request and is echoed here, and
		// mounted bare it still answers with one, which is what makes the
		// documented header part of this surface's contract.
		id, err := uuid.Parse(r.Header.Get("X-Request-ID"))
		if err != nil {
			id = uuid.New()
		}
		r.Header.Set("X-Request-ID", id.String())
		w.Header().Set("X-Request-ID", id.String())
		// A shared error body shape on every response: a caller reads code and
		// request_id, never a human message.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}

// predictionRequest is the documented body: one object of feature name to value.
// A nil Features means the member was absent, which is a malformed body rather
// than a sample missing every feature.
type predictionRequest struct {
	Features *scorer.Sample `json:"features"`
}

// prediction is the documented success body.
type prediction struct {
	Level        artifact.Level `json:"level"`
	Probability  float64        `json:"probability"`
	ModelVersion string         `json:"model_version"`
}

// errorBody is the documented failure body.
type errorBody struct {
	Code      string `json:"code"`
	RequestID string `json:"request_id"`
	Detail    string `json:"detail"`
}

func (api *API) predict(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	decoder.DisallowUnknownFields()
	var request predictionRequest
	if err := decoder.Decode(&request); err != nil {
		api.refuse(w, http.StatusBadRequest, CodeInvalidBody, fmt.Sprintf("body is not the documented object: %v", err))
		return
	}
	if request.Features == nil {
		api.refuse(w, http.StatusBadRequest, CodeInvalidBody, "body must carry a features object")
		return
	}
	if decoder.More() {
		api.refuse(w, http.StatusBadRequest, CodeInvalidBody, "body carries more than one JSON value")
		return
	}
	result, err := api.predictor.Score(*request.Features)
	if err != nil {
		var invalid *scorer.InvalidSampleError
		switch {
		case errors.As(err, &invalid):
			api.refuse(w, http.StatusBadRequest, invalid.Code, invalid.Detail)
		case errors.Is(err, scorer.ErrNonFiniteProbability):
			// Not a response and not a 400: the sample was well formed and the
			// model produced a number that cannot be answered with. It is the
			// service's failure to report, and it must be visible as one.
			api.refuse(w, http.StatusInternalServerError, CodeNonFiniteProbability, err.Error())
		default:
			api.refuse(w, http.StatusInternalServerError, CodeScoringFailed, err.Error())
		}
		return
	}
	level := api.artifact.Level(result.Probability)
	api.counters.Prediction(level)
	respond(w, http.StatusOK, prediction{Level: level, Probability: result.Probability, ModelVersion: result.Version})
}

// health answers readiness. The artifact is loaded and validated before the
// listener starts and never reloaded, so a response means the model is the one
// this process validated: there is no dependency left to probe, and claiming
// "degraded" would only be a lie about a state that cannot occur without the
// process being dead.
func (api *API) health(w http.ResponseWriter, _ *http.Request) {
	medium, high := api.artifact.LevelThresholds()
	respond(w, http.StatusOK, map[string]any{
		"status":        "ready",
		"format":        api.artifact.Format,
		"model_version": api.artifact.Version,
		"data_hash":     api.artifact.DataHash,
		"kind":          api.artifact.Kind,
		"trained_at":    api.artifact.TrainedAt,
		"feature_count": len(api.artifact.Features),
		"thresholds":    map[string]float64{"medium": medium, "high": high},
	})
}

// modelDocument is the provenance surface: what the model is and what it was
// chosen on. The parameters (impute, center, scale, coef, trees) are
// deliberately not exposed; they are in the artifact file and in the frozen
// equivalence fixture, which is where a reviewer reproduces them. What a caller
// cannot get from this endpoint is a number to reverse-engineer a decision from.
type modelDocument struct {
	Format     string             `json:"format"`
	Version    string             `json:"version"`
	TrainedAt  any                `json:"trained_at"`
	DataHash   string             `json:"data_hash"`
	Kind       string             `json:"kind"`
	Features   []artifact.Feature `json:"features"`
	Thresholds map[string]float64 `json:"thresholds"`
	Metrics    json.RawMessage    `json:"metrics"`
	Baseline   json.RawMessage    `json:"baseline"`
}

func (api *API) model(w http.ResponseWriter, _ *http.Request) {
	medium, high := api.artifact.LevelThresholds()
	document := modelDocument{
		Format:     api.artifact.Format,
		Version:    api.artifact.Version,
		TrainedAt:  api.artifact.TrainedAt,
		DataHash:   api.artifact.DataHash,
		Kind:       api.artifact.Kind,
		Features:   api.artifact.Features,
		Thresholds: map[string]float64{"medium": medium, "high": high},
		Metrics:    emptyObject(api.artifact.Metrics),
		Baseline:   emptyObject(api.artifact.Baseline),
	}
	respond(w, http.StatusOK, document)
}

// emptyObject keeps metrics and baseline always objects in the rendered
// document. An artifact without them would otherwise serialize a nil RawMessage
// as null, and a caller reading .metrics.positives would break on the shape
// instead of on an absent key.
func emptyObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

// refuse answers a failure with the documented body and counts it under its
// stable code.
func (api *API) refuse(w http.ResponseWriter, status int, code, detail string) {
	api.counters.Refused(code)
	respond(w, status, errorBody{
		Code:      code,
		RequestID: w.Header().Get("X-Request-ID"),
		Detail:    detail,
	})
}

// respond writes one JSON body. The header write is the status, so a caller
// never sees a 200 with an error body.
func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// discardCounter is the zero observability seam: it counts nothing and is used
// only when a caller wires no counter at all.
type discardCounter struct{}

func (discardCounter) Prediction(artifact.Level) {}
func (discardCounter) Refused(string)            {}
