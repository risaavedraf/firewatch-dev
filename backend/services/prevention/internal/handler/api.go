// Package handler is the HTTP transport of the prevention service. It validates
// the request, resolves the verified subject, and maps repository outcomes to the
// status codes and the shared error body the frozen contract fixes.
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"firewatch.local/prevention/internal/repository"
	"firewatch.local/prevention/internal/state"
	"github.com/google/uuid"
)

// Repository is the persistence this transport needs. It is declared here, not
// returned by the store, so the transport depends on the four operations of the
// contract and nothing else.
type Repository interface {
	// List returns the actions matching the optional filters, newest first. An
	// empty filter narrows nothing.
	List(ctx context.Context, communityCode, status string) ([]repository.Action, error)
	// Create inserts one action in the initial propuesta state, attributed to
	// input.CreatedBy.
	Create(ctx context.Context, input repository.NewAction) (repository.Action, error)
	// Get returns one action with its attributable history.
	Get(ctx context.Context, id string) (repository.Detail, error)
	// Transition advances one action in a single guarded transaction, or refuses
	// with repository.ErrInvalidTransition leaving nothing written.
	Transition(ctx context.Context, id, to, actorSub, note string) (repository.Action, error)
}

// body limits. The create body is a small, flat document; the transition body is
// two fields. Both are read through http.MaxBytesReader so an oversized request
// is refused instead of buffered.
const (
	maxCreateBody     = 8192
	maxTransitionBody = 1024
)

// New builds the authenticated handler for the four contract routes.
func New(store Repository, verifier Verifier) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/prevention/actions", list(store))
	mux.HandleFunc("POST /v1/prevention/actions", create(store))
	mux.HandleFunc("GET /v1/prevention/actions/{id}", detail(store))
	mux.HandleFunc("POST /v1/prevention/actions/{id}/transitions", transition(store))
	return Authorize(verifier, mux)
}

// list serves GET /v1/prevention/actions. Both query parameters are optional
// filters: an omitted one narrows nothing, which is what the contract's
// "?community_code=&status=" form means. A parameter that is repeated, unknown, or
// not a state of the vocabulary is malformed input and is refused before storage
// is touched.
func list(store Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		communityCode, status, ok := listFilters(r)
		if !ok {
			fail(w, 422, "invalid_query")
			return
		}
		actions, err := store.List(r.Context(), communityCode, status)
		if err != nil {
			slog.Error("list_actions_failed", "request_id", r.Header.Get("X-Request-ID"))
			fail(w, 503, "storage_unavailable")
			return
		}
		// A nil slice would marshal as null, which is not the documented list. An
		// empty result is a valid answer, so it is normalised to [] here instead
		// of trusting every repository implementation to do it.
		if actions == nil {
			actions = []repository.Action{}
		}
		respond(w, 200, actions)
	}
}

// listFilters validates the list query and returns the two filter values.
func listFilters(r *http.Request) (communityCode, status string, ok bool) {
	query := r.URL.Query()
	for key := range query {
		if key != "community_code" && key != "status" {
			return "", "", false
		}
	}
	for _, key := range []string{"community_code", "status"} {
		if len(query[key]) > 1 {
			return "", "", false
		}
	}
	status = query.Get("status")
	// An unknown status would silently return an empty list, which reads as "no
	// such actions" instead of "you asked for a state that does not exist".
	if status != "" && !state.Valid(state.State(status)) {
		return "", "", false
	}
	return query.Get("community_code"), status, true
}

// createRequest is the create body. Every field is a pointer so an omitted field
// is distinguishable from a present empty one, and unknown fields are refused, so
// a client cannot set the status (the contract fixes propuesta) or the id.
type createRequest struct {
	CommunityCode *string `json:"community_code"`
	Title         *string `json:"title"`
	Description   *string `json:"description"`
	Priority      *string `json:"priority"`
	Assignee      *string `json:"assignee"`
	DueDate       *string `json:"due_date"`
}

// create serves POST /v1/prevention/actions: admin only, and the created action is
// always propuesta.
func create(store Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAdmin(r.Context()) {
			fail(w, 403, "forbidden")
			return
		}
		if !jsonContentType(r) {
			fail(w, 415, "json_required")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCreateBody))
		if err != nil {
			fail(w, 422, "invalid_action_request")
			return
		}
		var request createRequest
		if !decodeStrict(body, &request) {
			fail(w, 422, "invalid_action_request")
			return
		}
		input, ok := newActionFrom(request, subjectOf(r.Context()))
		if !ok {
			fail(w, 422, "invalid_action_request")
			return
		}
		action, err := store.Create(r.Context(), input)
		if err != nil {
			slog.Error("create_action_failed", "request_id", r.Header.Get("X-Request-ID"))
			fail(w, 503, "storage_unavailable")
			return
		}
		// Durable audit: the action row plus this line. The sibling services
		// record an audit entry for every state-changing command, keyed by the
		// verified actor and the request trace.
		slog.Info("audit",
			"actor_id", action.CreatedBy,
			"action", "prevention.action.created",
			"action_id", action.ID,
			"trace_id", r.Header.Get("X-Request-ID"),
			"occurred_at", action.CreatedAt)
		w.Header().Set("Location", "/v1/prevention/actions/"+action.ID)
		respond(w, 201, action)
	}
}

// newActionFrom validates the create body. The only required fields are the
// community and the title: the contract names the other columns without defining
// a vocabulary for them, so requiring more would reject requests the contract
// allows. due_date must be a calendar date, and the actor is the verified subject.
func newActionFrom(request createRequest, actor string) (repository.NewAction, bool) {
	if request.CommunityCode == nil || strings.TrimSpace(*request.CommunityCode) == "" {
		return repository.NewAction{}, false
	}
	if request.Title == nil || strings.TrimSpace(*request.Title) == "" {
		return repository.NewAction{}, false
	}
	input := repository.NewAction{CommunityCode: *request.CommunityCode, Title: *request.Title, CreatedBy: actor}
	if request.Description != nil {
		input.Description = *request.Description
	}
	if request.Priority != nil {
		input.Priority = *request.Priority
	}
	// An empty assignee is the same as no assignee: it would otherwise show up as
	// an assignment to nobody.
	if request.Assignee != nil && *request.Assignee != "" {
		input.Assignee = request.Assignee
	}
	if request.DueDate != nil && *request.DueDate != "" {
		parsed, err := time.Parse("2006-01-02", *request.DueDate)
		if err != nil || parsed.Format("2006-01-02") != *request.DueDate {
			return repository.NewAction{}, false
		}
		input.DueDate = request.DueDate
	}
	return input, true
}

// detail serves GET /v1/prevention/actions/{id}: the action plus its history.
func detail(store Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := actionID(r)
		if !ok {
			fail(w, 400, "invalid_action_id")
			return
		}
		result, err := store.Get(r.Context(), id)
		if errors.Is(err, repository.ErrActionNotFound) {
			fail(w, 404, "action_not_found")
			return
		}
		if err != nil {
			slog.Error("get_action_failed", "request_id", r.Header.Get("X-Request-ID"))
			fail(w, 503, "storage_unavailable")
			return
		}
		respond(w, 200, result)
	}
}

// transitionRequest is the transition body: the target state is required, the note
// is optional.
type transitionRequest struct {
	To   *string `json:"to"`
	Note *string `json:"note"`
}

// transition serves POST /v1/prevention/actions/{id}/transitions: admin only.
//
// The two failure modes are deliberately different. A target that is not a state
// of the vocabulary is invalid input (422) and never reaches storage. A target
// that is a state but that the frozen machine does not allow from the action's
// current status is a conflict (409 invalid_transition), decided inside the
// repository's transaction so the action is left exactly as it was with no
// history row.
func transition(store Repository) http.HandlerFunc {
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
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxTransitionBody))
		if err != nil {
			fail(w, 422, "invalid_transition_request")
			return
		}
		var request transitionRequest
		if !decodeStrict(body, &request) {
			fail(w, 422, "invalid_transition_request")
			return
		}
		if request.To == nil || !state.Valid(state.State(*request.To)) {
			fail(w, 422, "invalid_transition_request")
			return
		}
		note := ""
		if request.Note != nil {
			note = *request.Note
		}
		action, err := store.Transition(r.Context(), id, *request.To, subjectOf(r.Context()), note)
		if errors.Is(err, repository.ErrActionNotFound) {
			fail(w, 404, "action_not_found")
			return
		}
		if errors.Is(err, repository.ErrInvalidTransition) {
			fail(w, 409, "invalid_transition")
			return
		}
		if err != nil {
			slog.Error("transition_action_failed", "request_id", r.Header.Get("X-Request-ID"))
			fail(w, 503, "storage_unavailable")
			return
		}
		slog.Info("audit",
			"actor_id", subjectOf(r.Context()),
			"action", "prevention.action.transitioned",
			"action_id", action.ID,
			"to_status", action.Status,
			"trace_id", r.Header.Get("X-Request-ID"),
			"occurred_at", action.UpdatedAt)
		respond(w, 200, action)
	}
}

// actionID parses the {id} path value as a UUID. The path is a UUID or it is
// malformed; nothing else addresses an action.
func actionID(r *http.Request) (string, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil || id == uuid.Nil {
		return "", false
	}
	return id.String(), true
}

// jsonContentType reports whether the request declares an application/json body.
func jsonContentType(r *http.Request) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && media == "application/json"
}

// decodeStrict decodes exactly one JSON document and refuses unknown fields and a
// second document. A body that is empty, malformed, or carries a field the
// contract does not define never reaches storage.
func decodeStrict(body []byte, target any) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return false
	}
	return decoder.Decode(new(any)) == io.EOF
}
