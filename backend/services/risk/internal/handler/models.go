package handler

import (
	"errors"
	"net/http"

	"firewatch.local/risk/internal/repository"
)

// modelListResponse is the GET /v1/models body. The list is wrapped rather than
// returned bare so a future field (the active version, for instance) is additive.
type modelListResponse struct {
	Models []repository.ModelVersion `json:"models"`
}

// listModels serves the registry. It never invents rows: an empty registry is an
// empty list, and a storage failure is a 503, so "nothing registered" and "cannot
// read" stay distinguishable.
func listModels(store Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		models, err := store.ListModels(r.Context())
		if err != nil {
			fail(w, http.StatusServiceUnavailable, "storage_unavailable")
			return
		}
		if models == nil {
			models = []repository.ModelVersion{}
		}
		respond(w, http.StatusOK, modelListResponse{Models: models})
	}
}

// activateModel makes one version active. It is wrapped in requireAdmin by the
// router, so reaching it already means the caller holds the admin role. The
// failure codes are distinct on purpose: a missing version is 404, a version the
// schema forbids from being active is 409, and a storage failure is 503.
func activateModel(store Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		version := r.PathValue("version")
		if version == "" {
			fail(w, http.StatusBadRequest, "invalid_version")
			return
		}
		model, err := store.ActivateModel(r.Context(), version)
		switch {
		case errors.Is(err, repository.ErrModelNotFound):
			fail(w, http.StatusNotFound, "model_not_found")
		case errors.Is(err, repository.ErrModelNotActivatable):
			fail(w, http.StatusConflict, "model_not_activatable")
		case err != nil:
			fail(w, http.StatusServiceUnavailable, "storage_unavailable")
		default:
			respond(w, http.StatusOK, model)
		}
	}
}
