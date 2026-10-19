package handler

import (
	"encoding/json"
	"net/http"
	"testing"

	"firewatch.local/prevention/internal/repository"
	"firewatch.local/prevention/internal/state"
)

// The evidence, human-review and synthetic-reassessment routes are the S2 half of
// the preventive contract: an admin records bounded, credential-free references
// and a sufficiency criterion, an attributable human decides sufficiency, and the
// acceptance of an action now depends on that decision. These tests drive the
// routes through the same authenticated chain the other handler tests use.

func TestNewEvidenceRouteIsAdminOnly(t *testing.T) {
	for _, tc := range []struct {
		role string
		want int
	}{
		{role: "viewer", want: http.StatusForbidden},
		{role: "operator", want: http.StatusForbidden},
		{role: "admin", want: http.StatusCreated},
	} {
		t.Run(tc.role, func(t *testing.T) {
			store := &matrixStore{status: state.Enviada, action: repository.Action{ID: fixedActionID}}
			h, signer := app(t, store)
			w := requestWith(t, h, http.MethodPost, "/v1/prevention/actions/"+fixedActionID+"/evidence",
				bearerFor(t, signer, tc.role),
				`{"criterion":"Informe de la brigada y acta de la junta","references":["https://example.org/informe-2026"]}`)
			if w.Code != tc.want {
				t.Fatalf("answered %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestEvidenceRouteRejectsUnusableReferences(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{name: "empty criterion", body: `{"criterion":"  ","references":["https://example.org/a"]}`, want: http.StatusUnprocessableEntity},
		{name: "no references", body: `{"criterion":"acta","references":[]}`, want: http.StatusUnprocessableEntity},
		{name: "missing references", body: `{"criterion":"acta"}`, want: http.StatusUnprocessableEntity},
		{name: "not http", body: `{"criterion":"acta","references":["ftp://example.org/a"]}`, want: http.StatusUnprocessableEntity},
		{name: "no host", body: `{"criterion":"acta","references":["https:///a"]}`, want: http.StatusUnprocessableEntity},
		{name: "credentials in url", body: `{"criterion":"acta","references":["https://user:secret@example.org/a"]}`, want: http.StatusUnprocessableEntity},
		{name: "unknown field", body: `{"criterion":"acta","references":["https://example.org/a"],"status":"aceptada"}`, want: http.StatusUnprocessableEntity},
		{name: "not json", body: `criterion=acta`, want: http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &matrixStore{status: state.Enviada}
			h, signer := app(t, store)
			w := requestWith(t, h, http.MethodPost, "/v1/prevention/actions/"+fixedActionID+"/evidence",
				token(t, signer, "admin", "admin-1"), tc.body)
			if w.Code != tc.want {
				t.Fatalf("answered %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
			if code := errorBody(t, w)["code"]; code != "invalid_evidence_request" {
				t.Errorf("code %q, want invalid_evidence_request", code)
			}
		})
	}
}

func TestEvidenceReviewRouteRequiresAdminAndValidDecision(t *testing.T) {
	for _, tc := range []struct {
		name, role, body string
		want             int
	}{
		{name: "viewer", role: "viewer", body: `{"decision":"suficiente"}`, want: http.StatusForbidden},
		{name: "operator", role: "operator", body: `{"decision":"suficiente"}`, want: http.StatusForbidden},
		{name: "unknown decision", role: "admin", body: `{"decision":"quizas"}`, want: http.StatusUnprocessableEntity},
		{name: "missing decision", role: "admin", body: `{}`, want: http.StatusUnprocessableEntity},
		{name: "admin", role: "admin", body: `{"decision":"suficiente","note":"acta revisada"}`, want: http.StatusCreated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &matrixStore{status: state.Enviada}
			h, signer := app(t, store)
			w := requestWith(t, h, http.MethodPost,
				"/v1/prevention/actions/"+fixedActionID+"/evidence/7/reviews",
				bearerFor(t, signer, tc.role), tc.body)
			if w.Code != tc.want {
				t.Fatalf("answered %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestReassessmentRouteRejectsAFabricatedPayload(t *testing.T) {
	store := &matrixStore{status: state.Enviada}
	h, signer := app(t, store)
	// The client may name only the server-produced job. A batch, version or risk
	// level in the body is an attempt to fabricate the association and is refused
	// before storage is touched.
	for _, body := range []string{
		`{"job_id":"not-a-uuid"}`,
		`{"job_id":"6f1e7b0a-2f4f-4a3f-9a5b-1d3c7e9f2a44","batch_id":"6f1e7b0a-2f4f-4a3f-9a5b-1d3c7e9f2a45"}`,
		`{"job_id":"6f1e7b0a-2f4f-4a3f-9a5b-1d3c7e9f2a44","version":"academic-rule-v1"}`,
		`{}`,
	} {
		w := requestWith(t, h, http.MethodPost, "/v1/prevention/actions/"+fixedActionID+"/reassessments",
			token(t, signer, "admin", "admin-1"), body)
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("answered %d, want 422 for %s: %s", w.Code, body, w.Body.String())
		}
		if code := errorBody(t, w)["code"]; code != "invalid_reassessment_request" {
			t.Errorf("code %q, want invalid_reassessment_request", code)
		}
	}
}

func TestAcceptanceWithoutSufficientEvidenceIsRefused(t *testing.T) {
	store := &matrixStore{status: state.Enviada, action: repository.Action{ID: fixedActionID, Status: string(state.Enviada)}}
	store.evidenceSufficient = false
	h, signer := app(t, store)
	w := requestWith(t, h, http.MethodPost, "/v1/prevention/actions/"+fixedActionID+"/transitions",
		token(t, signer, "admin", "admin-1"), `{"to":"aceptada","note":"cierre"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("answered %d, want 409: %s", w.Code, w.Body.String())
	}
	if code := errorBody(t, w)["code"]; code != "insufficient_evidence" {
		t.Errorf("code %q, want insufficient_evidence", code)
	}
	if store.status != state.Enviada {
		t.Errorf("status changed to %q after a refused acceptance", store.status)
	}
	if len(store.history) != 0 {
		t.Errorf("a refused acceptance appended history: %+v", store.history)
	}
}

func TestAcceptanceWithSufficientEvidenceSucceeds(t *testing.T) {
	store := &matrixStore{status: state.Enviada, action: repository.Action{ID: fixedActionID, Status: string(state.Enviada)}}
	store.evidenceSufficient = true
	h, signer := app(t, store)
	w := requestWith(t, h, http.MethodPost, "/v1/prevention/actions/"+fixedActionID+"/transitions",
		token(t, signer, "admin", "admin-1"), `{"to":"aceptada"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("answered %d, want 200: %s", w.Code, w.Body.String())
	}
	if store.status != state.Aceptada {
		t.Errorf("status %q, want aceptada", store.status)
	}
}

func TestEvidenceRouteMapsStorageRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, target, body string
		err                error
		want               int
		code               string
	}{
		{
			name: "terminal action", target: "/v1/prevention/actions/" + fixedActionID + "/evidence",
			body: `{"criterion":"acta","references":["https://example.org/a"]}`,
			err:  repository.ErrEvidenceState, want: http.StatusConflict, code: "invalid_evidence_state",
		},
		{
			name: "stale snapshot", target: "/v1/prevention/actions/" + fixedActionID + "/evidence/2/reviews",
			body: `{"decision":"suficiente"}`,
			err:  repository.ErrStaleEvidence, want: http.StatusConflict, code: "stale_evidence",
		},
		{
			name: "missing evidence", target: "/v1/prevention/actions/" + fixedActionID + "/evidence/2/reviews",
			body: `{"decision":"suficiente"}`,
			err:  repository.ErrEvidenceNotFound, want: http.StatusNotFound, code: "evidence_not_found",
		},
		{
			name: "reassessment not published", target: "/v1/prevention/actions/" + fixedActionID + "/reassessments",
			body: `{"job_id":"6f1e7b0a-2f4f-4a3f-9a5b-1d3c7e9f2a44"}`,
			err:  repository.ErrReassessmentUnavailable, want: http.StatusConflict, code: "reassessment_unavailable",
		},
		{
			name: "reassessment not synthetic", target: "/v1/prevention/actions/" + fixedActionID + "/reassessments",
			body: `{"job_id":"6f1e7b0a-2f4f-4a3f-9a5b-1d3c7e9f2a44"}`,
			err:  repository.ErrReassessmentNotSynthetic, want: http.StatusConflict, code: "reassessment_not_synthetic",
		},
		{
			name: "reassessment other commune", target: "/v1/prevention/actions/" + fixedActionID + "/reassessments",
			body: `{"job_id":"6f1e7b0a-2f4f-4a3f-9a5b-1d3c7e9f2a44"}`,
			err:  repository.ErrReassessmentCommuneMismatch, want: http.StatusConflict, code: "reassessment_commune_mismatch",
		},
		{
			name: "reassessment not new", target: "/v1/prevention/actions/" + fixedActionID + "/reassessments",
			body: `{"job_id":"6f1e7b0a-2f4f-4a3f-9a5b-1d3c7e9f2a44"}`,
			err:  repository.ErrReassessmentNotNew, want: http.StatusConflict, code: "reassessment_not_new",
		},
		{
			name: "reassessment already linked", target: "/v1/prevention/actions/" + fixedActionID + "/reassessments",
			body: `{"job_id":"6f1e7b0a-2f4f-4a3f-9a5b-1d3c7e9f2a44"}`,
			err:  repository.ErrReassessmentAlreadyLinked, want: http.StatusConflict, code: "reassessment_already_linked",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &matrixStore{err: tc.err}
			h, signer := app(t, store)
			w := requestWith(t, h, http.MethodPost, tc.target, token(t, signer, "admin", "admin-1"), tc.body)
			if w.Code != tc.want {
				t.Fatalf("answered %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
			if code := errorBody(t, w)["code"]; code != tc.code {
				t.Errorf("code %q, want %q", code, tc.code)
			}
		})
	}
}

func TestEvidenceWritesAttributeTheVerifiedSubject(t *testing.T) {
	store := &matrixStore{status: state.Enviada}
	h, signer := app(t, store)

	evidenceBody := `{"criterion":"Acta de la junta","references":["https://example.org/acta","http://example.org/foto"]}`
	if w := requestWith(t, h, http.MethodPost, "/v1/prevention/actions/"+fixedActionID+"/evidence",
		token(t, signer, "admin", "admin-7"), evidenceBody); w.Code != http.StatusCreated {
		t.Fatalf("evidence answered %d: %s", w.Code, w.Body.String())
	}
	if store.lastEvidenceInput.CreatedBy != "admin-7" {
		t.Errorf("created_by %q, want the verified token subject", store.lastEvidenceInput.CreatedBy)
	}
	if store.lastEvidenceInput.Criterion != "Acta de la junta" || len(store.lastEvidenceInput.References) != 2 {
		t.Errorf("evidence command %+v did not carry the body", store.lastEvidenceInput)
	}

	if w := requestWith(t, h, http.MethodPost, "/v1/prevention/actions/"+fixedActionID+"/evidence/3/reviews",
		token(t, signer, "admin", "reviewer-9"), `{"decision":"suficiente","note":"revisado"}`); w.Code != http.StatusCreated {
		t.Fatalf("review answered %d: %s", w.Code, w.Body.String())
	}
	if store.lastReviewInput.ReviewerSub != "reviewer-9" {
		t.Errorf("reviewer_sub %q, want the verified token subject", store.lastReviewInput.ReviewerSub)
	}
	if store.lastReviewEvidenceID != 3 || store.lastReviewInput.Decision != "suficiente" {
		t.Errorf("review command %+v / evidence %d did not carry the body", store.lastReviewInput, store.lastReviewEvidenceID)
	}

	jobID := "6f1e7b0a-2f4f-4a3f-9a5b-1d3c7e9f2a44"
	if w := requestWith(t, h, http.MethodPost, "/v1/prevention/actions/"+fixedActionID+"/reassessments",
		token(t, signer, "admin", "admin-7"), `{"job_id":"`+jobID+`"}`); w.Code != http.StatusCreated {
		t.Fatalf("reassessment answered %d: %s", w.Code, w.Body.String())
	}
	if store.lastLinkJobID != jobID || store.lastLinkLinkedBy != "admin-7" {
		t.Errorf("link command job %q by %q did not carry the verified values", store.lastLinkJobID, store.lastLinkLinkedBy)
	}
}

func TestReviewRouteRejectsANonNumericEvidenceID(t *testing.T) {
	store := &matrixStore{}
	h, signer := app(t, store)
	w := requestWith(t, h, http.MethodPost, "/v1/prevention/actions/"+fixedActionID+"/evidence/not-a-number/reviews",
		token(t, signer, "admin", "admin-1"), `{"decision":"suficiente"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("answered %d, want 400: %s", w.Code, w.Body.String())
	}
	if code := errorBody(t, w)["code"]; code != "invalid_evidence_id" {
		t.Errorf("code %q, want invalid_evidence_id", code)
	}
	if store.reviewCalls != 0 {
		t.Error("a malformed evidence id reached storage")
	}
}

func TestDetailCarriesTheEvidenceCollections(t *testing.T) {
	store := &matrixStore{detail: repository.Detail{
		Action:             repository.Action{ID: fixedActionID, Status: string(state.Enviada)},
		History:            []repository.History{},
		Evidence:           []repository.Evidence{},
		EvidenceReviews:    []repository.EvidenceReview{},
		Reassessments:      []repository.Reassessment{},
		EvidenceSufficient: false,
	}}
	h, signer := app(t, store)
	w := requestWith(t, h, http.MethodGet, "/v1/prevention/actions/"+fixedActionID, token(t, signer, "viewer", "viewer-1"), "")
	if w.Code != http.StatusOK {
		t.Fatalf("answered %d, want 200: %s", w.Code, w.Body.String())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"evidence", "evidence_reviews", "reassessments", "evidence_sufficient"} {
		if _, ok := body[key]; !ok {
			t.Errorf("detail body is missing %q: %s", key, w.Body.String())
		}
	}
}
