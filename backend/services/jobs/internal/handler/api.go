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

	v1 "firewatch.local/contracts/events/v1"
	"firewatch.local/jobs/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Repository interface {
	// Create is variadic on purpose: the optional scenario is additive, so every
	// pre-existing caller and test keeps compiling with the legacy five arguments
	// and an absent scenario preserves the original fixture path.
	Create(context.Context, string, string, string, string, ...string) (repository.Job, error)
	Get(context.Context, string) (repository.Job, error)
	Audit(context.Context) ([]repository.Audit, error)
	// ZoneExists reports whether the zone catalogue holds code. Membership is a
	// property of the data, not of a literal compiled into this service, so the
	// transport asks the catalogue instead of comparing to a hardcoded zone.
	ZoneExists(ctx context.Context, code string) (bool, error)
	// Replay returns a failed job, or a queued one the operator explicitly
	// confirmed, to queued. It is the operator-driven recovery path: the single
	// guarded store transition also writes the audit record and the outgoing
	// request event, so the transport only maps its outcome to a status code.
	Replay(ctx context.Context, id, actor, trace string, confirmQueued bool) (repository.Job, error)
}

// replayConfirmToken is the only accepted value of the replay body's confirm
// field. It is an explicit token rather than a boolean on purpose: replaying a
// queued job may dispatch work that is still running, so the operator has to name
// what they are accepting instead of sending a stray true.
const replayConfirmToken = "queued_may_be_in_flight"

// replayConfirmation reads the optional replay body and reports whether the
// re-dispatch of a queued job was confirmed. An absent body carries no
// confirmation; a present one must decode strictly to the single documented
// token, so any other document is refused before the store is reached and can
// never produce a partial effect.
func replayConfirmation(r *http.Request) (confirmQueued, ok bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1024))
	if err != nil {
		return false, false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return false, true
	}
	var input struct {
		Confirm string `json:"confirm"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || input.Confirm != replayConfirmToken {
		return false, false
	}
	return true, true
}

// knownScenarioID reports whether id is part of the shared wire vocabulary. The
// transport validates membership; it never owns the list, so adding a profile is
// a contracts change, not an edit here. Exact equality is deliberate: trimming
// or case-folding would invent aliases the contract does not define.
func knownScenarioID(id string) bool {
	for _, known := range v1.SyntheticScenarioIDs() {
		if known == id {
			return true
		}
	}
	return false
}

func New(store Repository, verifier Verifier, checks ...StatusCheck) http.Handler {
	mux := http.NewServeMux()
	var check StatusCheck
	if len(checks) == 1 {
		check = checks[0]
	}
	mux.HandleFunc("GET /v1/jobs/status", statusHandler(check))
	mux.HandleFunc("POST /v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			fail(w, 415, "json_required")
			return
		}
		key, err := uuid.Parse(r.Header.Get("Idempotency-Key"))
		if err != nil || key == uuid.Nil {
			fail(w, 400, "invalid_idempotency_key")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
		if err != nil {
			fail(w, 400, "invalid_demo_request")
			return
		}
		var input struct {
			ZoneID string `json:"zone_id"`
			// Optional demonstration profile. evaluated_at is deliberately NOT a
			// field here: the server assigns the instant, and DisallowUnknownFields
			// turns a client-supplied evaluated_at into the existing invalid request
			// error. A plain string cannot tell an absent field from an explicit null
			// or "", so key presence is read from the raw document below.
			ScenarioID string `json:"scenario_id"`
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
			fail(w, 400, "invalid_demo_request")
			return
		}
		// Fail-closed scenario contract: an explicit null or "" is rejected before
		// any lookup or write, while an omitted field keeps the legacy fixture. The
		// strict decode above is unchanged; this only adds the presence check that
		// a string field cannot express.
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			fail(w, 400, "invalid_demo_request")
			return
		}
		if rawScenario, present := fields["scenario_id"]; present {
			var scenario string
			if json.Unmarshal(rawScenario, &scenario) != nil || scenario == "" {
				fail(w, 400, "invalid_demo_request")
				return
			}
			input.ScenarioID = scenario
		}
		// Unknown or malformed scenarios are rejected before any write, with the
		// existing 400 code: no job row and no outbox row can be created for a
		// profile the wire contract does not define.
		if input.ScenarioID != "" && !knownScenarioID(input.ScenarioID) {
			fail(w, 400, "invalid_demo_request")
			return
		}
		// Membership is a property of the catalogue, not of a literal compiled
		// into this service. The 400 code is unchanged for wire compatibility;
		// what changed is that the accepted set is whatever zones holds, and it
		// is no longer only the demo zone.
		exists, err := store.ZoneExists(r.Context(), input.ZoneID)
		if err != nil {
			slog.Error("zone_lookup_failed", "request_id", r.Header.Get("X-Request-ID"))
			fail(w, 503, "storage_unavailable")
			return
		}
		if !exists {
			fail(w, 400, "invalid_demo_request")
			return
		}
		trace, err := uuid.Parse(r.Header.Get("X-Request-ID"))
		if err != nil {
			trace = uuid.New()
		}
		w.Header().Set("X-Request-ID", trace.String())
		job, err := store.Create(r.Context(), r.Context().Value(subjectKey{}).(string), key.String(), input.ZoneID, trace.String(), input.ScenarioID)
		if err != nil {
			slog.Error("create_job_failed", "request_id", trace.String())
			fail(w, 503, "storage_unavailable")
			return
		}
		w.Header().Set("Location", "/v1/jobs/"+job.ID)
		respond(w, 202, job)
	})
	mux.HandleFunc("GET /v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			fail(w, 400, "invalid_job_id")
			return
		}
		job, err := store.Get(r.Context(), id.String())
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 404, "job_not_found")
			return
		}
		if err != nil {
			fail(w, 503, "storage_unavailable")
			return
		}
		respond(w, 200, job)
	})
	mux.HandleFunc("POST /v1/jobs/{id}/replay", func(w http.ResponseWriter, r *http.Request) {
		// Echo the correlation id first so every rejection below carries the same
		// request_id as its response header, exactly as the create path does.
		trace, err := uuid.Parse(r.Header.Get("X-Request-ID"))
		if err != nil {
			trace = uuid.New()
		}
		w.Header().Set("X-Request-ID", trace.String())
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			fail(w, 400, "invalid_job_id")
			return
		}
		// The confirmation is read before the store is reached: a malformed body
		// is a client defect, not a state conflict, and it must leave no row,
		// audit entry or outgoing event behind.
		confirmQueued, ok := replayConfirmation(r)
		if !ok {
			fail(w, 400, "invalid_replay_request")
			return
		}
		job, err := store.Replay(r.Context(), id.String(), r.Context().Value(subjectKey{}).(string), trace.String(), confirmQueued)
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 404, "job_not_found")
			return
		}
		if errors.Is(err, repository.ErrReplayConfirmationRequired) {
			fail(w, 409, "replay_confirmation_required")
			return
		}
		if errors.Is(err, repository.ErrJobNotReplayable) {
			fail(w, 409, "job_not_replayable")
			return
		}
		if err != nil {
			slog.Error("replay_job_failed", "request_id", trace.String())
			fail(w, 503, "storage_unavailable")
			return
		}
		respond(w, 202, job)
	})
	mux.HandleFunc("GET /v1/audit", func(w http.ResponseWriter, r *http.Request) {
		items, err := store.Audit(r.Context())
		if err != nil {
			fail(w, 503, "storage_unavailable")
			return
		}
		respond(w, 200, items)
	})
	return Authorize(verifier, mux)
}
