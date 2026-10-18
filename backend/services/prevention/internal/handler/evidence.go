package handler

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"firewatch.local/prevention/internal/evidence"
	"firewatch.local/prevention/internal/repository"
	"github.com/google/uuid"
)

// Body bounds. The evidence body is the largest of the three because it carries
// up to ten bounded references plus a criterion; the review and reassessment
// bodies are two fields and one field.
const (
	maxEvidenceBody     = 32768
	maxReviewBody       = 2048
	maxReassessmentBody = 1024
)

// registerEvidenceRoutes mounts the S2 write routes below the same prevention
// prefix the gateway already proxies. They are registered next to the frozen
// action routes so one mux carries the whole contract, and the admin-only check
// stays inside each write handler exactly like the create and transition routes.
func registerEvidenceRoutes(mux *http.ServeMux, store Repository) {
	mux.HandleFunc("POST /v1/prevention/actions/{id}/evidence", addEvidence(store))
	mux.HandleFunc("POST /v1/prevention/actions/{id}/evidence/{evidence_id}/reviews", reviewEvidence(store))
	mux.HandleFunc("POST /v1/prevention/actions/{id}/reassessments", linkReassessment(store))
}

// evidenceRequest is the append-only evidence body. A pointer criterion tells an
// omitted field from a present empty one; references is the bounded list of
// http(s) citations. Nothing in this body is fetched.
type evidenceRequest struct {
	Criterion  *string  `json:"criterion"`
	References []string `json:"references"`
}

func addEvidence(store Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAdmin(r.Context()) {
			fail(w, 403, "forbidden")
			return
		}
		if !jsonContentType(r) {
			fail(w, 415, "json_required")
			return
		}
		id, ok := actionID(r)
		if !ok {
			fail(w, 400, "invalid_action_id")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxEvidenceBody))
		if err != nil {
			fail(w, 422, "invalid_evidence_request")
			return
		}
		var request evidenceRequest
		if !decodeStrict(body, &request) {
			fail(w, 422, "invalid_evidence_request")
			return
		}
		if request.Criterion == nil || !evidence.ValidCriterion(*request.Criterion) {
			fail(w, 422, "invalid_evidence_request")
			return
		}
		if len(request.References) == 0 || len(request.References) > evidence.MaxReferences {
			fail(w, 422, "invalid_evidence_request")
			return
		}
		for _, reference := range request.References {
			if !evidence.ValidReference(reference) {
				fail(w, 422, "invalid_evidence_request")
				return
			}
		}
		added, err := store.AddEvidence(r.Context(), id, repository.NewEvidence{
			Criterion:  *request.Criterion,
			References: request.References,
			CreatedBy:  subjectOf(r.Context()),
		})
		if errors.Is(err, repository.ErrActionNotFound) {
			fail(w, 404, "action_not_found")
			return
		}
		if errors.Is(err, repository.ErrEvidenceState) {
			fail(w, 409, "invalid_evidence_state")
			return
		}
		if err != nil {
			slog.Error("add_evidence_failed", "request_id", r.Header.Get("X-Request-ID"))
			fail(w, 503, "storage_unavailable")
			return
		}
		slog.Info("audit",
			"actor_id", added.CreatedBy,
			"action", "prevention.evidence.recorded",
			"action_id", added.ActionID,
			"evidence_id", added.ID,
			"trace_id", r.Header.Get("X-Request-ID"),
			"occurred_at", added.CreatedAt)
		respond(w, 201, added)
	}
}

// reviewRequest is the human decision body. The reviewer identity and the review
// instant are never accepted from the client: they come from the verified token
// subject and the server clock.
type reviewRequest struct {
	Decision *string `json:"decision"`
	Note     *string `json:"note"`
}

func reviewEvidence(store Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAdmin(r.Context()) {
			fail(w, 403, "forbidden")
			return
		}
		if !jsonContentType(r) {
			fail(w, 415, "json_required")
			return
		}
		id, ok := actionID(r)
		if !ok {
			fail(w, 400, "invalid_action_id")
			return
		}
		evidenceID, err := strconv.ParseInt(r.PathValue("evidence_id"), 10, 64)
		if err != nil || evidenceID <= 0 {
			fail(w, 400, "invalid_evidence_id")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxReviewBody))
		if err != nil {
			fail(w, 422, "invalid_review_request")
			return
		}
		var request reviewRequest
		if !decodeStrict(body, &request) {
			fail(w, 422, "invalid_review_request")
			return
		}
		if request.Decision == nil || !evidence.ValidDecision(*request.Decision) {
			fail(w, 422, "invalid_review_request")
			return
		}
		note := ""
		if request.Note != nil {
			note = *request.Note
		}
		review, err := store.ReviewEvidence(r.Context(), id, evidenceID, repository.NewReview{
			Decision:    *request.Decision,
			ReviewerSub: subjectOf(r.Context()),
			Note:        note,
		})
		if errors.Is(err, repository.ErrActionNotFound) {
			fail(w, 404, "action_not_found")
			return
		}
		if errors.Is(err, repository.ErrEvidenceNotFound) {
			fail(w, 404, "evidence_not_found")
			return
		}
		if errors.Is(err, repository.ErrStaleEvidence) {
			fail(w, 409, "stale_evidence")
			return
		}
		if errors.Is(err, repository.ErrEvidenceState) {
			fail(w, 409, "invalid_evidence_state")
			return
		}
		if err != nil {
			slog.Error("review_evidence_failed", "request_id", r.Header.Get("X-Request-ID"))
			fail(w, 503, "storage_unavailable")
			return
		}
		slog.Info("audit",
			"actor_id", review.ReviewerSub,
			"action", "prevention.evidence.reviewed",
			"action_id", review.ActionID,
			"evidence_id", review.EvidenceID,
			"decision", review.Decision,
			"trace_id", r.Header.Get("X-Request-ID"),
			"occurred_at", review.ReviewedAt)
		respond(w, 201, review)
	}
}

// reassessmentRequest names only the server-produced job. A batch, version or
// risk level in the body is unknown-field input and is refused before storage.
type reassessmentRequest struct {
	JobID *string `json:"job_id"`
}

func linkReassessment(store Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAdmin(r.Context()) {
			fail(w, 403, "forbidden")
			return
		}
		if !jsonContentType(r) {
			fail(w, 415, "json_required")
			return
		}
		id, ok := actionID(r)
		if !ok {
			fail(w, 400, "invalid_action_id")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxReassessmentBody))
		if err != nil {
			fail(w, 422, "invalid_reassessment_request")
			return
		}
		var request reassessmentRequest
		if !decodeStrict(body, &request) {
			fail(w, 422, "invalid_reassessment_request")
			return
		}
		jobID, err := uuid.Parse(derefString(request.JobID))
		if err != nil || jobID == uuid.Nil {
			fail(w, 422, "invalid_reassessment_request")
			return
		}
		link, err := store.LinkReassessment(r.Context(), id, jobID.String(), subjectOf(r.Context()))
		if errors.Is(err, repository.ErrActionNotFound) {
			fail(w, 404, "action_not_found")
			return
		}
		switch {
		case errors.Is(err, repository.ErrReassessmentUnavailable):
			fail(w, 409, "reassessment_unavailable")
			return
		case errors.Is(err, repository.ErrReassessmentNotSynthetic):
			fail(w, 409, "reassessment_not_synthetic")
			return
		case errors.Is(err, repository.ErrReassessmentCommuneMismatch):
			fail(w, 409, "reassessment_commune_mismatch")
			return
		case errors.Is(err, repository.ErrReassessmentNotNew):
			fail(w, 409, "reassessment_not_new")
			return
		case errors.Is(err, repository.ErrReassessmentAlreadyLinked):
			fail(w, 409, "reassessment_already_linked")
			return
		case err != nil:
			slog.Error("link_reassessment_failed", "request_id", r.Header.Get("X-Request-ID"))
			fail(w, 503, "storage_unavailable")
			return
		}
		slog.Info("audit",
			"actor_id", link.LinkedBy,
			"action", "prevention.reassessment.linked",
			"action_id", link.ActionID,
			"job_id", link.JobID,
			"batch_id", link.BatchID,
			"version", link.Version,
			"risk_level", link.RiskLevel,
			"trace_id", r.Header.Get("X-Request-ID"),
			"occurred_at", link.LinkedAt)
		respond(w, 201, link)
	}
}

// derefString returns the pointed-to value or the empty string, so a missing field
// is validated the same way as an empty one.
func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
