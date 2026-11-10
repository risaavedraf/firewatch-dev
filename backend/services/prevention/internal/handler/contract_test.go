package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"firewatch.local/prevention/internal/directory"
	"firewatch.local/prevention/internal/repository"
	"github.com/getkin/kin-openapi/openapi3"
)

// The published contract is the single source of truth for this API surface, so
// these tests validate real responses of the users route against it instead of
// restating the documented shape by hand. They follow the gateway's contract
// test: the YAML is loaded from disk and consumed here, so a response that stops
// matching it fails this test rather than drifting silently.
//
// The 200 case is the one that matters most: an empty display_name, an entry
// beside the documented two fields, or a list that is not an array all fail
// against the schema instead of reaching a caller.

// contractPath locates the published contract from this package's working dir.
var contractPath = filepath.Join("..", "..", "..", "..", "contracts", "openapi", "firewatch-v1.yaml")

// loadContract loads and validates the published contract.
func loadContract(t *testing.T) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	contract, err := loader.LoadFromFile(contractPath)
	if err != nil {
		t.Fatalf("load the published contract: %v", err)
	}
	if err := contract.Validate(context.Background()); err != nil {
		t.Fatalf("the published contract is not valid: %v", err)
	}
	return contract
}

// assertResponseMatchesContract fails when the real response is not the one the
// contract documents: the status must be declared for the operation, every
// header the contract declares must be present, and the JSON body must validate
// against the declared schema.
func assertResponseMatchesContract(t *testing.T, contract *openapi3.T, target, method string, response *httptest.ResponseRecorder) {
	t.Helper()
	item := contract.Paths.Find(target)
	if item == nil {
		t.Fatalf("contract documents no path %q", target)
	}
	operation := item.GetOperation(method)
	if operation == nil {
		t.Fatalf("contract documents no %s operation for %q", method, target)
	}
	documented := operation.Responses.Status(response.Code)
	if documented == nil || documented.Value == nil {
		t.Fatalf("contract documents no %d response for %s %s", response.Code, method, target)
	}
	for name := range documented.Value.Headers {
		if response.Header().Get(name) == "" {
			t.Errorf("%s %s %d is missing the documented header %q", method, target, response.Code, name)
		}
	}
	media := documented.Value.Content.Get("application/json")
	if media == nil || media.Schema == nil || media.Schema.Value == nil {
		t.Fatalf("contract documents no application/json body for %s %s %d", method, target, response.Code)
	}
	var body any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body is not JSON: %v: %s", err, response.Body.String())
	}
	if err := media.Schema.Value.VisitJSON(body); err != nil {
		t.Fatalf("response body does not match the contract for %s %s %d: %v\nbody: %s", method, target, response.Code, err, response.Body.String())
	}
}

// TestDirectoryUserSchemaRejectsAnUndocumentedField honours this file's own
// claim about the 200 body: an entry beside the documented two fields must fail
// against the schema. DirectoryUser is the schema every item of the users list is
// validated against, so an undocumented property surviving validation would mean
// the route could start returning a field the contract does not describe.
func TestDirectoryUserSchemaRejectsAnUndocumentedField(t *testing.T) {
	contract := loadContract(t)
	schema := contract.Components.Schemas["DirectoryUser"]
	if schema == nil || schema.Value == nil {
		t.Fatal("the contract documents no DirectoryUser schema")
	}

	documented := map[string]any{"id": "u1", "display_name": "Demo Admin"}
	if err := schema.Value.VisitJSON(documented); err != nil {
		t.Fatalf("the documented two-field entry was refused: %v", err)
	}

	undocumented := map[string]any{"id": "u1", "display_name": "Demo Admin", "email": "demo@example.invalid"}
	if err := schema.Value.VisitJSON(undocumented); err == nil {
		t.Fatal("an entry with an undocumented field validated against DirectoryUser, want the schema to refuse it")
	}
}

// TestUsersContractSuccessResponse pins the documented 200: an array of the two
// documented fields, with a display name that is never empty even when the
// directory reports none.
func TestUsersContractSuccessResponse(t *testing.T) {
	contract := loadContract(t)
	h, signer := usersApp(t, &stubDirectory{users: []directory.User{
		{ID: "u1", DisplayName: "Demo Admin"},
		{ID: "u2"},
	}})

	w := requestWith(t, h, http.MethodGet, "/v1/users", token(t, signer, "admin", "demo-admin"), "")
	if w.Code != http.StatusOK {
		t.Fatalf("got %d want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	assertResponseMatchesContract(t, contract, "/v1/users", http.MethodGet, w)
}

// TestUsersContractUnauthorizedResponse pins the documented 401 body.
func TestUsersContractUnauthorizedResponse(t *testing.T) {
	contract := loadContract(t)
	h, _ := usersApp(t, &stubDirectory{})

	w := requestWith(t, h, http.MethodGet, "/v1/users", "", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d want %d: %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
	assertResponseMatchesContract(t, contract, "/v1/users", http.MethodGet, w)
}

// TestUsersContractForbiddenResponse pins the documented 403 body a viewer gets.
func TestUsersContractForbiddenResponse(t *testing.T) {
	contract := loadContract(t)
	h, signer := usersApp(t, &stubDirectory{})

	w := requestWith(t, h, http.MethodGet, "/v1/users", token(t, signer, "viewer", "demo-viewer"), "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("got %d want %d: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertResponseMatchesContract(t, contract, "/v1/users", http.MethodGet, w)
}

// TestUsersContractDirectoryUnavailableResponse pins the documented fail-closed
// 503: an unconfigured directory answers it, never a 200 with an empty list.
func TestUsersContractDirectoryUnavailableResponse(t *testing.T) {
	contract := loadContract(t)
	h, signer := usersApp(t, &stubDirectory{err: directory.ErrNotConfigured})

	w := requestWith(t, h, http.MethodGet, "/v1/users", token(t, signer, "admin", "demo-admin"), "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d want %d: %s", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}
	assertResponseMatchesContract(t, contract, "/v1/users", http.MethodGet, w)
}

// TestUsersContractDirectoryUpstreamFailedResponse pins the documented 502 an
// identity provider failure produces.
func TestUsersContractDirectoryUpstreamFailedResponse(t *testing.T) {
	contract := loadContract(t)
	h, signer := usersApp(t, &stubDirectory{err: errors.New("identity provider refused the read")})

	w := requestWith(t, h, http.MethodGet, "/v1/users", token(t, signer, "admin", "demo-admin"), "")
	if w.Code != http.StatusBadGateway {
		t.Fatalf("got %d want %d: %s", w.Code, http.StatusBadGateway, w.Body.String())
	}
	assertResponseMatchesContract(t, contract, "/v1/users", http.MethodGet, w)
}

// TestCreateActionContractAssignmentResponses binds the assignment answers to the
// published contract instead of restating their shape: a 201 that carries the
// reference and a 201 that is unassigned must validate against the documented
// action, and the 422 for an identifier the directory does not return must
// validate against the documented refusal body.
func TestCreateActionContractAssignmentResponses(t *testing.T) {
	const directoryID = "b1f2c3d4-0000-4000-8000-000000000001"
	contract := loadContract(t)

	t.Run("assigned", func(t *testing.T) {
		store := &matrixStore{action: repository.Action{
			ID: fixedActionID, CommunityCode: "valparaiso", Title: "Cortafuegos",
			Status: "propuesta", AssigneeID: strPtr(directoryID), CreatedBy: "operator-1",
		}}
		h, signer := appWith(t, store, &stubDirectory{user: directory.User{
			ID: directoryID, DisplayName: "Brigada 2",
		}})
		w := requestWith(t, h, http.MethodPost, "/v1/prevention/actions",
			token(t, signer, "admin", "operator-1"),
			`{"community_code":"valparaiso","title":"Cortafuegos","assignee_id":"`+directoryID+`"}`)
		if w.Code != http.StatusCreated {
			t.Fatalf("got %d want %d: %s", w.Code, http.StatusCreated, w.Body.String())
		}
		assertResponseMatchesContract(t, contract, "/v1/prevention/actions", http.MethodPost, w)
	})

	t.Run("unassigned", func(t *testing.T) {
		store := &matrixStore{action: repository.Action{
			ID: fixedActionID, CommunityCode: "valparaiso", Title: "Cortafuegos",
			Status: "propuesta", CreatedBy: "operator-1",
		}}
		h, signer := app(t, store)
		w := requestWith(t, h, http.MethodPost, "/v1/prevention/actions",
			token(t, signer, "admin", "operator-1"),
			`{"community_code":"valparaiso","title":"Cortafuegos"}`)
		if w.Code != http.StatusCreated {
			t.Fatalf("got %d want %d: %s", w.Code, http.StatusCreated, w.Body.String())
		}
		assertResponseMatchesContract(t, contract, "/v1/prevention/actions", http.MethodPost, w)
	})

	t.Run("unknown assignee", func(t *testing.T) {
		store := &matrixStore{}
		h, signer := appWith(t, store, &stubDirectory{userErr: directory.ErrUserNotFound})
		w := requestWith(t, h, http.MethodPost, "/v1/prevention/actions",
			token(t, signer, "admin", "operator-1"),
			`{"community_code":"valparaiso","title":"Cortafuegos","assignee_id":"`+directoryID+`"}`)
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("got %d want %d: %s", w.Code, http.StatusUnprocessableEntity, w.Body.String())
		}
		if code := errorBody(t, w)["code"]; code != "assignee_not_found" {
			t.Fatalf("error code %q, want assignee_not_found", code)
		}
		assertResponseMatchesContract(t, contract, "/v1/prevention/actions", http.MethodPost, w)
	})
}

// TestPreventiveActionSchemaDocumentsBothAssignments keeps the contract from
// describing only one half of the assignment: assignee is the legacy free text a
// reader still receives, assignee_id is the documented reference new assignments
// use, and the create body must accept the reference.
func TestPreventiveActionSchemaDocumentsBothAssignments(t *testing.T) {
	contract := loadContract(t)
	action := contract.Components.Schemas["PreventiveAction"]
	if action == nil || action.Value == nil {
		t.Fatal("the contract documents no PreventiveAction schema")
	}
	required := map[string]bool{}
	for _, name := range action.Value.Required {
		required[name] = true
	}
	for _, name := range []string{"assignee", "assignee_id"} {
		if !required[name] {
			t.Errorf("PreventiveAction does not require %q; every read returns both fields", name)
		}
	}
	assignee := action.Value.Properties["assignee"]
	if assignee == nil || assignee.Value == nil || !strings.Contains(strings.ToLower(assignee.Value.Description), "legacy") {
		t.Error("assignee is not documented as the legacy free text kept for rows created before the reference existed")
	}
	reference := action.Value.Properties["assignee_id"]
	if reference == nil || reference.Value == nil {
		t.Fatal("PreventiveAction does not document assignee_id")
	}
	if !reference.Value.Nullable {
		t.Error("assignee_id must be nullable: an action created without it stays unassigned")
	}

	create := contract.Components.Schemas["PreventiveActionCreate"]
	if create == nil || create.Value == nil {
		t.Fatal("the contract documents no PreventiveActionCreate schema")
	}
	if create.Value.Properties["assignee_id"] == nil {
		t.Error("PreventiveActionCreate does not accept assignee_id")
	}
}
