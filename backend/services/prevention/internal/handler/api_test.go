package handler

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"firewatch.local/prevention/internal/repository"
	"firewatch.local/prevention/internal/state"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
)

// fixedActionID is a valid action id for path tests; the fake store never needs a
// real row for it.
var fixedActionID = "6f1e7b0a-2f4f-4a3f-9a5b-1d3c7e9f2a44"

// matrixStore is a stand-in for the real store that enforces the SAME frozen rule
// the SQL transition does: it consults internal/state and only mutates the row
// when the move is allowed. That is what makes the route-level table below a test
// of the contract rather than a test of a mock's opinion: a refused transition
// must leave the status untouched and append no history, and a 409 body must be
// the shared error shape.
type matrixStore struct {
	status  state.State
	history []repository.History
	note    string

	action       repository.Action
	detail       repository.Detail
	list         []repository.Action
	err          error
	getErr       error
	createErr    error
	notFound     bool
	lastList     [2]string
	lastCreate   repository.NewAction
	createCalls  int
	listCalls    int
	getCalls     int
	transitions  int
	lastTransID  string
	lastTransTo  string
	lastActorSub string
	traceSeen    string

	// evidenceSufficient mirrors the guard the real store evaluates inside the
	// row-locked transaction: accepting an action is refused unless the current
	// evidence snapshot carries a sufficient human review.
	evidenceSufficient bool

	addEvidenceCalls     int
	reviewCalls          int
	linkCalls            int
	lastEvidenceInput    repository.NewEvidence
	lastReviewInput      repository.NewReview
	lastReviewEvidenceID int64
	lastLinkJobID        string
	lastLinkLinkedBy     string
}

func (s *matrixStore) List(_ context.Context, communityCode, status string) ([]repository.Action, error) {
	s.listCalls++
	s.lastList = [2]string{communityCode, status}
	if s.err != nil {
		return nil, s.err
	}
	return s.list, nil
}

func (s *matrixStore) Create(_ context.Context, input repository.NewAction) (repository.Action, error) {
	s.createCalls++
	s.lastCreate = input
	if s.createErr != nil {
		return repository.Action{}, s.createErr
	}
	return s.action, nil
}

func (s *matrixStore) Get(_ context.Context, id string) (repository.Detail, error) {
	s.getCalls++
	s.lastTransID = id
	if s.getErr != nil {
		return repository.Detail{}, s.getErr
	}
	if s.notFound {
		return repository.Detail{}, repository.ErrActionNotFound
	}
	return s.detail, nil
}

// Transition mirrors the store's guarded transaction: refuse without writing, or
// advance and append exactly one history row. Accepting an action additionally
// consults the same sufficiency fact the real store reads, so the route table
// below exercises the guard instead of a mock's optimism.
func (s *matrixStore) Transition(_ context.Context, id, to, actorSub, note string) (repository.Action, error) {
	s.transitions++
	s.lastTransID, s.lastTransTo, s.lastActorSub = id, to, actorSub
	s.note = note
	if s.err != nil {
		return repository.Action{}, s.err
	}
	if !state.CanTransition(s.status, state.State(to)) {
		return repository.Action{}, repository.ErrInvalidTransition
	}
	if state.State(to) == state.Aceptada && !s.evidenceSufficient {
		return repository.Action{}, repository.ErrEvidenceInsufficient
	}
	from := s.status
	s.status = state.State(to)
	s.history = append(s.history, repository.History{
		ID: int64(len(s.history) + 1), ActionID: id, FromStatus: string(from), ToStatus: to,
		ActorSub: actorSub, Note: note, CreatedAt: time.Now().UTC(),
	})
	s.action.ID, s.action.Status = id, to
	return s.action, nil
}

// The three evidence methods record what the route passed and answer a fixture.
// The route tests assert the transport half; the SQL rules are covered by the
// repository integration tests when a disposable database is present.
func (s *matrixStore) AddEvidence(_ context.Context, id string, input repository.NewEvidence) (repository.Evidence, error) {
	s.addEvidenceCalls++
	s.lastEvidenceInput = input
	if s.err != nil {
		return repository.Evidence{}, s.err
	}
	return repository.Evidence{ID: 1, ActionID: id, Criterion: input.Criterion, References: input.References, CreatedBy: input.CreatedBy}, nil
}

func (s *matrixStore) ReviewEvidence(_ context.Context, id string, evidenceID int64, input repository.NewReview) (repository.EvidenceReview, error) {
	s.reviewCalls++
	s.lastReviewEvidenceID = evidenceID
	s.lastReviewInput = input
	if s.err != nil {
		return repository.EvidenceReview{}, s.err
	}
	return repository.EvidenceReview{ID: 1, ActionID: id, EvidenceID: evidenceID, Decision: input.Decision, ReviewerSub: input.ReviewerSub, Note: input.Note}, nil
}

func (s *matrixStore) LinkReassessment(_ context.Context, id, jobID, linkedBy string) (repository.Reassessment, error) {
	s.linkCalls++
	s.lastLinkJobID, s.lastLinkLinkedBy = jobID, linkedBy
	if s.err != nil {
		return repository.Reassessment{}, s.err
	}
	return repository.Reassessment{ID: 1, ActionID: id, JobID: jobID, BatchID: "batch-1", Version: "academic-rule-v1", RiskLevel: "medio", ZoneID: "valparaiso-demo", Synthetic: true, LinkedBy: linkedBy}, nil
}

// signingFixture publishes one RS256 key, exactly as the production verifier does.
func signingFixture(t *testing.T) (Verifier, jose.Signer) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	verifier := oidc.NewVerifier("https://issuer.test", &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}, &oidc.Config{ClientID: "firewatch-api", SupportedSigningAlgs: []string{"RS256"}})
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return verifier, signer
}

// token mints a valid access token for one role, reproducing the production
// claim policy the Authorize middleware enforces.
func token(t *testing.T, signer jose.Signer, role, subject string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"iss": "https://issuer.test", "aud": "firewatch-api", "sub": subject,
		"exp": time.Now().Add(time.Hour).Unix(), "typ": "Bearer",
		"realm_access": map[string]any{"roles": []string{role}},
	})
	if err != nil {
		t.Fatal(err)
	}
	signed, err := signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := signed.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// withCorrelationID mirrors the production middleware in cmd/server/main.go: the
// request id is canonicalised into the response header and forwarded on the
// request, so the handler error body echoes one literal. It is reproduced here
// because that middleware lives in package main and cannot be imported.
func withCorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.Header.Get("X-Request-ID"))
		if err != nil {
			id = uuid.New()
		}
		w.Header().Set("X-Request-ID", id.String())
		r.Header.Set("X-Request-ID", id.String())
		next.ServeHTTP(w, r)
	})
}

// app builds the full authenticated handler chain under test.
func app(t *testing.T, store Repository) (http.Handler, jose.Signer) {
	t.Helper()
	verifier, signer := signingFixture(t)
	return withCorrelationID(New(store, verifier)), signer
}

// requestWith issues one request with an optional bearer token. An empty bearer
// token means the header is absent entirely, which is what an unauthenticated
// caller sends.
func requestWith(t *testing.T, h http.Handler, method, target, bearer, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	r := httptest.NewRequest(method, target, reader)
	r.Header.Set("X-Request-ID", uuid.NewString())
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// errorBody decodes the shared error shape and fails when it carries anything
// other than the two documented keys.
func errorBody(t *testing.T, w *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("error body is not JSON: %v: %s", err, w.Body.String())
	}
	if len(raw) != 2 {
		t.Fatalf("error body must carry exactly code and request_id, got %v", raw)
	}
	var body struct {
		Code      string `json:"code"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.RequestID != w.Header().Get("X-Request-ID") || body.RequestID == "" {
		t.Fatalf("body request_id %q and header %q must be the same non-empty literal", body.RequestID, w.Header().Get("X-Request-ID"))
	}
	return map[string]string{"code": body.Code, "request_id": body.RequestID}
}

// TestTransitionRouteMatchesTheFrozenContract drives every pair of the contract's
// state matrix through POST /v1/prevention/actions/{id}/transitions. A pair the
// contract allows must answer 200 with the new version and append exactly one
// history row; every other pair must answer 409 invalid_transition and leave the
// action exactly as it was, with no history row.
func TestTransitionRouteMatchesTheFrozenContract(t *testing.T) {
	states := state.States()
	allowedCount, refusedCount := 0, 0
	for _, from := range states {
		for _, to := range states {
			name := fmt.Sprintf("%s_to_%s", from, to)
			t.Run(name, func(t *testing.T) {
				store := &matrixStore{status: from, action: repository.Action{ID: fixedActionID, Status: string(from)}, evidenceSufficient: true}
				h, signer := app(t, store)
				w := requestWith(t, h, http.MethodPost,
					"/v1/prevention/actions/"+fixedActionID+"/transitions",
					token(t, signer, "admin", "operator-1"),
					fmt.Sprintf(`{"to":%q,"note":"paso de prueba"}`, to))

				if store.transitions != 1 {
					t.Fatalf("the route must call the store's single transition exactly once, got %d", store.transitions)
				}
				if store.lastActorSub != "operator-1" {
					t.Errorf("actor_sub %q must be the verified token subject", store.lastActorSub)
				}
				if store.note != "paso de prueba" {
					t.Errorf("note %q must reach storage", store.note)
				}

				if state.CanTransition(from, to) {
					allowedCount++
					if w.Code != http.StatusOK {
						t.Fatalf("allowed transition %s -> %s answered %d: %s", from, to, w.Code, w.Body.String())
					}
					if store.status != to {
						t.Errorf("status is %q after the transition, want %q", store.status, to)
					}
					if len(store.history) != 1 || store.history[0].FromStatus != string(from) || store.history[0].ToStatus != string(to) {
						t.Errorf("history %+v must record exactly %s -> %s", store.history, from, to)
					}
					var got struct {
						Status string `json:"status"`
					}
					if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
						t.Fatal(err)
					}
					if got.Status != string(to) {
						t.Errorf("response status %q, want %q", got.Status, to)
					}
					return
				}

				refusedCount++
				if w.Code != http.StatusConflict {
					t.Fatalf("refused transition %s -> %s answered %d, want 409: %s", from, to, w.Code, w.Body.String())
				}
				if code := errorBody(t, w)["code"]; code != "invalid_transition" {
					t.Errorf("409 code %q, want invalid_transition", code)
				}
				if store.status != from {
					t.Errorf("a refused transition changed the status to %q, want %q", store.status, from)
				}
				if len(store.history) != 0 {
					t.Errorf("a refused transition appended history: %+v", store.history)
				}
			})
		}
	}
	if allowedCount != 14 || refusedCount != 50 {
		t.Fatalf("the contract's matrix is 14 allowed and 50 refused pairs, walked %d and %d", allowedCount, refusedCount)
	}
}

func TestTransitionRouteRejectsUnknownTargetStateWithoutCallingTheStore(t *testing.T) {
	for _, target := range []string{"", "PENDIENTE", "pending", "en curso", "asignada "} {
		t.Run(target, func(t *testing.T) {
			store := &matrixStore{status: state.Propuesta}
			h, signer := app(t, store)
			w := requestWith(t, h, http.MethodPost, "/v1/prevention/actions/"+fixedActionID+"/transitions",
				token(t, signer, "admin", "operator-1"), fmt.Sprintf(`{"to":%q}`, target))
			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("unknown target %q answered %d, want 422: %s", target, w.Code, w.Body.String())
			}
			if code := errorBody(t, w)["code"]; code != "invalid_transition_request" {
				t.Errorf("422 code %q, want invalid_transition_request", code)
			}
			if store.transitions != 0 {
				t.Errorf("an unprocessable target reached storage %d times", store.transitions)
			}
		})
	}
}

func TestTransitionRouteInputValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body, contentType string
		want                    int
	}{
		{name: "missing to", body: `{"note":"x"}`, contentType: "application/json", want: http.StatusUnprocessableEntity},
		{name: "null to", body: `{"to":null}`, contentType: "application/json", want: http.StatusUnprocessableEntity},
		{name: "unknown field", body: `{"to":"asignada","status":"aceptada"}`, contentType: "application/json", want: http.StatusUnprocessableEntity},
		{name: "trailing document", body: `{"to":"asignada"}{"to":"en_curso"}`, contentType: "application/json", want: http.StatusUnprocessableEntity},
		{name: "not json", body: `to=asignada`, contentType: "application/json", want: http.StatusUnprocessableEntity},
		{name: "wrong content type", body: `{"to":"asignada"}`, contentType: "text/plain", want: http.StatusUnsupportedMediaType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &matrixStore{status: state.Propuesta}
			h, signer := app(t, store)
			r := httptest.NewRequest(http.MethodPost, "/v1/prevention/actions/"+fixedActionID+"/transitions", strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+token(t, signer, "admin", "operator-1"))
			r.Header.Set("X-Request-ID", uuid.NewString())
			r.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("answered %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
			errorBody(t, w)
			if store.transitions != 0 {
				t.Errorf("an invalid request reached storage %d times", store.transitions)
			}
		})
	}
}

func TestActionRouteAuthorization(t *testing.T) {
	for _, tc := range []struct {
		method, target, role string
		want                 int
	}{
		{method: http.MethodGet, target: "/v1/prevention/actions", role: "", want: http.StatusUnauthorized},
		{method: http.MethodGet, target: "/v1/prevention/actions", role: "operator", want: http.StatusForbidden},
		{method: http.MethodGet, target: "/v1/prevention/actions", role: "viewer", want: http.StatusOK},
		{method: http.MethodGet, target: "/v1/prevention/actions", role: "admin", want: http.StatusOK},
		{method: http.MethodGet, target: "/v1/prevention/actions/" + fixedActionID, role: "viewer", want: http.StatusOK},
		{method: http.MethodGet, target: "/v1/prevention/actions/" + fixedActionID, role: "operator", want: http.StatusForbidden},
		{method: http.MethodPost, target: "/v1/prevention/actions", role: "viewer", want: http.StatusForbidden},
		{method: http.MethodPost, target: "/v1/prevention/actions", role: "operator", want: http.StatusForbidden},
		{method: http.MethodPost, target: "/v1/prevention/actions", role: "admin", want: http.StatusCreated},
		{method: http.MethodPost, target: "/v1/prevention/actions/" + fixedActionID + "/transitions", role: "viewer", want: http.StatusForbidden},
		{method: http.MethodPost, target: "/v1/prevention/actions/" + fixedActionID + "/transitions", role: "operator", want: http.StatusForbidden},
		{method: http.MethodPost, target: "/v1/prevention/actions/" + fixedActionID + "/transitions", role: "admin", want: http.StatusOK},
	} {
		name := fmt.Sprintf("%s_%s_%s", tc.method, tc.target, tc.role)
		t.Run(name, func(t *testing.T) {
			store := &matrixStore{status: state.Propuesta, action: repository.Action{ID: fixedActionID, Status: "propuesta"}}
			h, signer := app(t, store)
			body := ""
			switch tc.method {
			case http.MethodPost:
				if strings.HasSuffix(tc.target, "/transitions") {
					body = `{"to":"asignada"}`
				} else {
					body = `{"community_code":"valparaiso","title":"Cortafuegos"}`
				}
			}
			w := requestWith(t, h, tc.method, tc.target, bearerFor(t, signer, tc.role), body)
			if w.Code != tc.want {
				t.Fatalf("answered %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
			if tc.want == http.StatusForbidden || tc.want == http.StatusUnauthorized {
				if store.createCalls+store.transitions != 0 {
					t.Errorf("a refused request reached storage")
				}
				errorBody(t, w)
			}
		})
	}
}

// bearerFor mints a token for a role, and no token at all for the empty role:
// 401 and 403 are different answers and the table above checks both.
func bearerFor(t *testing.T, signer jose.Signer, role string) string {
	t.Helper()
	if role == "" {
		return ""
	}
	return token(t, signer, role, "operator-1")
}

func TestCreateActionReturnsTheCreatedVersion(t *testing.T) {
	store := &matrixStore{action: repository.Action{
		ID: fixedActionID, CommunityCode: "valparaiso", Title: "Cortafuegos",
		Status: string(state.Propuesta), CreatedBy: "operator-1",
	}}
	h, signer := app(t, store)
	w := requestWith(t, h, http.MethodPost, "/v1/prevention/actions",
		token(t, signer, "admin", "operator-1"),
		`{"community_code":"valparaiso","title":"Cortafuegos","description":"Faja de 30 m","priority":"alta","assignee":"brigada-2","due_date":"2026-11-01"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("answered %d, want 201: %s", w.Code, w.Body.String())
	}
	if store.createCalls != 1 {
		t.Fatalf("create reached storage %d times", store.createCalls)
	}
	// The creator is the verified subject and the initial status is propuesta,
	// never a client-supplied value.
	if store.lastCreate.CreatedBy != "operator-1" {
		t.Errorf("created_by %q, want the token subject", store.lastCreate.CreatedBy)
	}
	if store.lastCreate.CommunityCode != "valparaiso" || store.lastCreate.Title != "Cortafuegos" ||
		store.lastCreate.Description != "Faja de 30 m" || store.lastCreate.Priority != "alta" {
		t.Errorf("create command %+v did not carry the body fields", store.lastCreate)
	}
	if store.lastCreate.Assignee == nil || *store.lastCreate.Assignee != "brigada-2" {
		t.Errorf("assignee %v did not reach storage", store.lastCreate.Assignee)
	}
	if store.lastCreate.DueDate == nil || *store.lastCreate.DueDate != "2026-11-01" {
		t.Errorf("due_date %v did not reach storage", store.lastCreate.DueDate)
	}
	if loc := w.Header().Get("Location"); loc != "/v1/prevention/actions/"+fixedActionID {
		t.Errorf("Location %q, want the created resource", loc)
	}
}

func TestCreateActionInputValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{name: "missing community", body: `{"title":"x"}`, want: http.StatusUnprocessableEntity},
		{name: "whitespace community", body: `{"community_code":"   ","title":"x"}`, want: http.StatusUnprocessableEntity},
		{name: "missing title", body: `{"community_code":"valparaiso"}`, want: http.StatusUnprocessableEntity},
		{name: "blank title", body: `{"community_code":"valparaiso","title":""}`, want: http.StatusUnprocessableEntity},
		{name: "client chosen status", body: `{"community_code":"valparaiso","title":"x","status":"aceptada"}`, want: http.StatusUnprocessableEntity},
		{name: "client chosen id", body: `{"community_code":"valparaiso","title":"x","id":"1"}`, want: http.StatusUnprocessableEntity},
		{name: "due date with time", body: `{"community_code":"valparaiso","title":"x","due_date":"2026-11-01T10:00:00Z"}`, want: http.StatusUnprocessableEntity},
		{name: "impossible due date", body: `{"community_code":"valparaiso","title":"x","due_date":"2026-02-31"}`, want: http.StatusUnprocessableEntity},
		{name: "wrong type", body: `{"community_code":7,"title":"x"}`, want: http.StatusUnprocessableEntity},
		{name: "trailing document", body: `{"community_code":"v","title":"x"}{}`, want: http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &matrixStore{}
			h, signer := app(t, store)
			w := requestWith(t, h, http.MethodPost, "/v1/prevention/actions", token(t, signer, "admin", "operator-1"), tc.body)
			if w.Code != tc.want {
				t.Fatalf("answered %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
			if store.createCalls != 0 {
				t.Errorf("an invalid create reached storage %d times", store.createCalls)
			}
			if w.Code != http.StatusUnsupportedMediaType {
				if code := errorBody(t, w)["code"]; code != "invalid_action_request" {
					t.Errorf("code %q, want invalid_action_request", code)
				}
			}
		})
	}
}

// TestCreateActionAcceptsOmittedOptionalFields pins that a minimal body is a
// valid request: the contract names the columns but defines no required
// vocabulary for them, so the service must not demand values it cannot validate.
func TestCreateActionAcceptsOmittedOptionalFields(t *testing.T) {
	store := &matrixStore{}
	h, signer := app(t, store)
	w := requestWith(t, h, http.MethodPost, "/v1/prevention/actions", token(t, signer, "admin", "operator-1"),
		`{"community_code":"valparaiso","title":"Cortafuegos"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("answered %d, want 201: %s", w.Code, w.Body.String())
	}
	if store.lastCreate.Description != "" || store.lastCreate.Priority != "" {
		t.Errorf("omitted text fields must be empty, got %+v", store.lastCreate)
	}
	if store.lastCreate.Assignee != nil || store.lastCreate.DueDate != nil {
		t.Errorf("omitted assignee and due_date must be absent, got %+v", store.lastCreate)
	}
}

func TestListRouteForwardsFiltersAndNeverReturnsNull(t *testing.T) {
	for _, tc := range []struct {
		name, target          string
		communityCode, status string
	}{
		{name: "no filter", target: "/v1/prevention/actions", communityCode: "", status: ""},
		{name: "community", target: "/v1/prevention/actions?community_code=valparaiso", communityCode: "valparaiso", status: ""},
		{name: "status", target: "/v1/prevention/actions?status=en_curso", communityCode: "", status: "en_curso"},
		{name: "both", target: "/v1/prevention/actions?community_code=valparaiso&status=devuelta", communityCode: "valparaiso", status: "devuelta"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &matrixStore{list: nil}
			h, signer := app(t, store)
			w := requestWith(t, h, http.MethodGet, tc.target, token(t, signer, "viewer", "viewer-1"), "")
			if w.Code != http.StatusOK {
				t.Fatalf("answered %d, want 200: %s", w.Code, w.Body.String())
			}
			if strings.TrimSpace(w.Body.String()) != "[]" {
				t.Errorf("an empty list must serialise as [], got %s", w.Body.String())
			}
			if store.lastList != [2]string{tc.communityCode, tc.status} {
				t.Errorf("filters forwarded as %v, want %v", store.lastList, [2]string{tc.communityCode, tc.status})
			}
		})
	}
}

func TestListRouteRejectsMalformedFilters(t *testing.T) {
	for _, target := range []string{
		"/v1/prevention/actions?status=cancelada_por_error",
		"/v1/prevention/actions?status=EN_CURSO",
		"/v1/prevention/actions?status=en_curso&status=bloqueada",
		"/v1/prevention/actions?unknown=1",
		"/v1/prevention/actions?community_code=a&community_code=b",
	} {
		t.Run(target, func(t *testing.T) {
			store := &matrixStore{}
			h, signer := app(t, store)
			w := requestWith(t, h, http.MethodGet, target, token(t, signer, "viewer", "viewer-1"), "")
			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("answered %d, want 422: %s", w.Code, w.Body.String())
			}
			if code := errorBody(t, w)["code"]; code != "invalid_query" {
				t.Errorf("code %q, want invalid_query", code)
			}
			if store.listCalls != 0 {
				t.Errorf("a malformed filter reached storage %d times", store.listCalls)
			}
		})
	}
}

func TestDetailRouteReturnsHistoryAndMapsMissingRows(t *testing.T) {
	store := &matrixStore{detail: repository.Detail{
		Action: repository.Action{ID: fixedActionID, Status: string(state.Devuelta)},
		History: []repository.History{
			{ID: 1, FromStatus: "propuesta", ToStatus: "asignada", ActorSub: "admin-1", Note: "primer acuse"},
			{ID: 2, FromStatus: "enviada", ToStatus: "devuelta", ActorSub: "admin-2", Note: "falta adjunto"},
		},
	}}
	h, signer := app(t, store)
	w := requestWith(t, h, http.MethodGet, "/v1/prevention/actions/"+fixedActionID, token(t, signer, "viewer", "viewer-1"), "")
	if w.Code != http.StatusOK {
		t.Fatalf("answered %d, want 200: %s", w.Code, w.Body.String())
	}
	var got struct {
		Status  string               `json:"status"`
		History []repository.History `json:"history"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "devuelta" || len(got.History) != 2 || got.History[0].Note != "primer acuse" {
		t.Errorf("detail body %s did not carry the action and its history", w.Body.String())
	}

	missing := &matrixStore{notFound: true}
	h2, signer2 := app(t, missing)
	w2 := requestWith(t, h2, http.MethodGet, "/v1/prevention/actions/"+fixedActionID, token(t, signer2, "viewer", "viewer-1"), "")
	if w2.Code != http.StatusNotFound {
		t.Fatalf("missing action answered %d, want 404: %s", w2.Code, w2.Body.String())
	}
	if code := errorBody(t, w2)["code"]; code != "action_not_found" {
		t.Errorf("code %q, want action_not_found", code)
	}
}

func TestMalformedActionIDIsRejectedBeforeStorage(t *testing.T) {
	for _, target := range []string{"/v1/prevention/actions/not-a-uuid", "/v1/prevention/actions/12"} {
		t.Run(target, func(t *testing.T) {
			store := &matrixStore{}
			h, signer := app(t, store)
			w := requestWith(t, h, http.MethodGet, target, token(t, signer, "viewer", "viewer-1"), "")
			if w.Code != http.StatusBadRequest {
				t.Fatalf("answered %d, want 400: %s", w.Code, w.Body.String())
			}
			if code := errorBody(t, w)["code"]; code != "invalid_action_id" {
				t.Errorf("code %q, want invalid_action_id", code)
			}
			if store.getCalls+store.transitions != 0 {
				t.Errorf("a malformed id reached storage")
			}
		})
	}
}

func TestStorageFailureIsReportedAsUnavailable(t *testing.T) {
	store := &matrixStore{err: fmt.Errorf("connection refused")}
	h, signer := app(t, store)
	w := requestWith(t, h, http.MethodGet, "/v1/prevention/actions", token(t, signer, "viewer", "viewer-1"), "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("answered %d, want 503: %s", w.Code, w.Body.String())
	}
	if code := errorBody(t, w)["code"]; code != "storage_unavailable" {
		t.Errorf("code %q, want storage_unavailable", code)
	}
}

// TestTransitionRefusedByStorageIs409 proves the transport does not accept a
// transition the store refused, whatever the reason the store gives.
func TestTransitionRefusedByStorageIs409(t *testing.T) {
	store := &matrixStore{status: state.Aceptada}
	h, signer := app(t, store)
	w := requestWith(t, h, http.MethodPost, "/v1/prevention/actions/"+fixedActionID+"/transitions",
		token(t, signer, "admin", "operator-1"), `{"to":"en_curso"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("answered %d, want 409: %s", w.Code, w.Body.String())
	}
	if code := errorBody(t, w)["code"]; code != "invalid_transition" {
		t.Errorf("code %q, want invalid_transition", code)
	}
	if store.status != state.Aceptada {
		t.Errorf("a refused transition changed the status to %q", store.status)
	}
}
