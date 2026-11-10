package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"firewatch.local/prevention/internal/directory"
	"github.com/go-jose/go-jose/v4"
)

// The users directory read is admin-only and fails closed. These tests drive it
// through the same authenticated chain the other handler tests use, so the role
// gate, the correlation id and the shared error body are the production ones.

// stubDirectory stands in for the directory client where the client's own
// behaviour is not what is under test. It counts calls, so a request that was
// refused before the directory was consulted is visible, and it separates the
// list read from the by-identifier read, so a route that still scanned the list
// is visible too.
type stubDirectory struct {
	users []directory.User
	err   error

	user       directory.User
	userErr    error
	lastUserID string

	calls     int
	userCalls int
}

func (s *stubDirectory) Users(context.Context) ([]directory.User, error) {
	s.calls++
	return s.users, s.err
}

func (s *stubDirectory) User(_ context.Context, id string) (directory.User, error) {
	s.userCalls++
	s.lastUserID = id
	return s.user, s.userErr
}

// usersApp builds the authenticated handler chain with a directory attached.
func usersApp(t *testing.T, dir Directory) (http.Handler, jose.Signer) {
	t.Helper()
	verifier, signer := signingFixture(t)
	return withCorrelationID(New(&matrixStore{}, verifier, dir)), signer
}

// userEntryBody is the response item as the contract documents it.
type userEntryBody struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// usersOf decodes a 200 body strictly: it must be a JSON array of the documented
// two-field entries, never null and never an error body.
func usersOf(t *testing.T, w *httptest.ResponseRecorder) []userEntryBody {
	t.Helper()
	if contentType := w.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("content type %q, want application/json", contentType)
	}
	if trimmed := strings.TrimSpace(w.Body.String()); trimmed == "" || trimmed[0] != '[' {
		t.Fatalf("body is not a JSON array: %s", w.Body.String())
	}
	var entries []userEntryBody
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("body is not a JSON array of users: %v: %s", err, w.Body.String())
	}
	for _, entry := range entries {
		if entry.ID == "" || entry.DisplayName == "" {
			t.Fatalf("entry %+v is missing an id or a display name", entry)
		}
	}
	return entries
}

// keycloakStub serves the two endpoints the real directory client uses, so the
// display-name mapping is exercised through the client instead of being restated
// by a fake.
func keycloakStub(t *testing.T, usersJSON string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"stub-access-token","expires_in":300,"token_type":"Bearer"}`))
		case strings.HasSuffix(r.URL.Path, "/users"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(usersJSON))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// TestUsersRouteMapsTheThreeDisplayNameFallbacks drives the real directory client
// against a stand-in identity provider holding one user of each shape: both
// names, no names but a username, and nothing but the identifier.
func TestUsersRouteMapsTheThreeDisplayNameFallbacks(t *testing.T) {
	base := keycloakStub(t, `[
		{"id":"u1","username":"demo-admin","firstName":"Demo","lastName":"Admin"},
		{"id":"u2","username":"demo-viewer"},
		{"id":"u3"}
	]`)
	client := directory.New(directory.Config{
		BaseURL:      base,
		Realm:        "firewatch",
		ClientID:     "firewatch-directory",
		ClientSecret: "local-test-only-placeholder",
		// The stub identity provider answers plain http, which the client only
		// accepts in development. Every other mode would refuse the URL before a
		// request is made, so the mode is stated instead of inherited.
		Environment: "development",
	})
	h, signer := usersApp(t, client)

	w := requestWith(t, h, http.MethodGet, "/v1/users", token(t, signer, "admin", "demo-admin"), "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /v1/users answered %d (%s), want 200", w.Code, w.Body.String())
	}
	got := usersOf(t, w)
	want := []userEntryBody{
		{ID: "u1", DisplayName: "Demo Admin"},
		{ID: "u2", DisplayName: "demo-viewer"},
		{ID: "u3", DisplayName: "u3"},
	}
	if len(got) != len(want) {
		t.Fatalf("returned %d entries %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entry %d is %+v, want %+v (full body: %s)", i, got[i], want[i], w.Body.String())
		}
	}
}

// TestUsersRouteOrdersByDisplayNameThenID pins the order the contract documents:
// display name ascending, and the identifier breaks a tie so the order is total
// and a repeated call cannot shuffle equal names.
func TestUsersRouteOrdersByDisplayNameThenID(t *testing.T) {
	stub := &stubDirectory{users: []directory.User{
		{ID: "b", DisplayName: "Same"},
		{ID: "a", DisplayName: "Same"},
		{ID: "c", DisplayName: "Another"},
	}}
	h, signer := usersApp(t, stub)

	w := requestWith(t, h, http.MethodGet, "/v1/users", token(t, signer, "admin", "demo-admin"), "")
	if w.Code != http.StatusOK {
		t.Fatalf("answered %d (%s), want 200", w.Code, w.Body.String())
	}
	got := usersOf(t, w)
	want := []userEntryBody{
		{ID: "c", DisplayName: "Another"},
		{ID: "a", DisplayName: "Same"},
		{ID: "b", DisplayName: "Same"},
	}
	if len(got) != len(want) {
		t.Fatalf("returned %d entries %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entry %d is %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestUsersRouteCapsAt200 pins the bound every list route of this service shares:
// a realm with more users than the cap answers with the first 200 of the
// documented order, not with all of them.
func TestUsersRouteCapsAt200(t *testing.T) {
	users := make([]directory.User, 0, 205)
	for i := 0; i < 205; i++ {
		users = append(users, directory.User{ID: fmt.Sprintf("id-%03d", i), DisplayName: fmt.Sprintf("person-%03d", i)})
	}
	h, signer := usersApp(t, &stubDirectory{users: users})

	w := requestWith(t, h, http.MethodGet, "/v1/users", token(t, signer, "admin", "demo-admin"), "")
	if w.Code != http.StatusOK {
		t.Fatalf("answered %d (%s), want 200", w.Code, w.Body.String())
	}
	got := usersOf(t, w)
	if len(got) != 200 {
		t.Fatalf("returned %d entries, want the documented cap 200", len(got))
	}
	if got[0].DisplayName != "person-000" || got[199].DisplayName != "person-199" {
		t.Fatalf("the cap kept %q..%q, want the first 200 of the documented order", got[0].DisplayName, got[199].DisplayName)
	}
}

// TestUsersRouteIsAdminOnly pins the role gate: a viewer is authorized to read
// this service but not to enumerate its users, and the directory is never asked.
func TestUsersRouteIsAdminOnly(t *testing.T) {
	stub := &stubDirectory{users: []directory.User{{ID: "u1", DisplayName: "Demo Admin"}}}
	h, signer := usersApp(t, stub)

	w := requestWith(t, h, http.MethodGet, "/v1/users", token(t, signer, "viewer", "demo-viewer"), "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("a viewer answered %d (%s), want 403", w.Code, w.Body.String())
	}
	if body := errorBody(t, w); body["code"] != "forbidden" {
		t.Fatalf("error code %q, want forbidden", body["code"])
	}
	if stub.calls != 0 {
		t.Fatalf("the directory was read %d times for a refused request, want 0", stub.calls)
	}
}

// TestUsersRouteRequiresABearerToken pins the authentication gate: without a
// verifiable bearer the route answers 401 and never touches the directory.
func TestUsersRouteRequiresABearerToken(t *testing.T) {
	stub := &stubDirectory{users: []directory.User{{ID: "u1", DisplayName: "Demo Admin"}}}
	h, _ := usersApp(t, stub)

	w := requestWith(t, h, http.MethodGet, "/v1/users", "", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("an unauthenticated request answered %d (%s), want 401", w.Code, w.Body.String())
	}
	if body := errorBody(t, w); body["code"] != "unauthorized" {
		t.Fatalf("error code %q, want unauthorized", body["code"])
	}
	if stub.calls != 0 {
		t.Fatalf("the directory was read %d times for an unauthenticated request, want 0", stub.calls)
	}
}

// TestUsersRouteFailsClosedWhenTheDirectoryIsNotConfigured is the fail-closed
// rule at the route: an unconfigured directory answers 503 with the documented
// body, never 200 with an empty list that would claim the realm has no users.
func TestUsersRouteFailsClosedWhenTheDirectoryIsNotConfigured(t *testing.T) {
	for _, tc := range []struct {
		name string
		dir  Directory
	}{
		{name: "client reports unconfigured", dir: &stubDirectory{err: directory.ErrNotConfigured}},
		{name: "no directory attached", dir: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, signer := usersApp(t, tc.dir)
			w := requestWith(t, h, http.MethodGet, "/v1/users", token(t, signer, "admin", "demo-admin"), "")
			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("answered %d (%s), want 503", w.Code, w.Body.String())
			}
			if trimmed := strings.TrimSpace(w.Body.String()); trimmed != "" && trimmed[0] == '[' {
				t.Fatalf("an unconfigured directory answered with a list: %s", w.Body.String())
			}
			if body := errorBody(t, w); body["code"] != "directory_unavailable" {
				t.Fatalf("error code %q, want directory_unavailable", body["code"])
			}
		})
	}
}

// TestUsersRouteAnswers502WhenTheDirectoryFails pins the upstream rule: an
// identity provider that answers an error or cannot be reached is a 502, not an
// empty list and not a 503 that would blame this service's configuration.
func TestUsersRouteAnswers502WhenTheDirectoryFails(t *testing.T) {
	h, signer := usersApp(t, &stubDirectory{err: errors.New("identity provider refused the read")})

	w := requestWith(t, h, http.MethodGet, "/v1/users", token(t, signer, "admin", "demo-admin"), "")
	if w.Code != http.StatusBadGateway {
		t.Fatalf("answered %d (%s), want 502", w.Code, w.Body.String())
	}
	if trimmed := strings.TrimSpace(w.Body.String()); trimmed != "" && trimmed[0] == '[' {
		t.Fatalf("a failing directory answered with a list: %s", w.Body.String())
	}
	if body := errorBody(t, w); body["code"] != "directory_upstream_failed" {
		t.Fatalf("error code %q, want directory_upstream_failed", body["code"])
	}
}

// TestUsersRouteAnswers502ForANonListUpstreamBody drives the real directory
// client against a stand-in identity provider whose users answer is not a JSON
// array. `null` and an array followed by trailing bytes both decode into an
// empty slice without an error, so this is the case where a 200 [] would claim
// the realm has no users for a body nobody meant as a list; the route must
// answer the documented 502 instead.
func TestUsersRouteAnswers502ForANonListUpstreamBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "null", body: "null"},
		{name: "an object", body: `{"not":"a list"}`},
		{name: "an array with trailing bytes", body: "[] trailing"},
		{name: "an empty body", body: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := keycloakStub(t, tc.body)
			client := directory.New(directory.Config{
				BaseURL:      base,
				Realm:        "firewatch",
				ClientID:     "firewatch-directory",
				ClientSecret: "local-test-only-placeholder",
				Environment:  "development",
			})
			h, signer := usersApp(t, client)

			w := requestWith(t, h, http.MethodGet, "/v1/users", token(t, signer, "admin", "demo-admin"), "")
			if w.Code != http.StatusBadGateway {
				t.Fatalf("answered %d (%s), want 502", w.Code, w.Body.String())
			}
			if body := errorBody(t, w); body["code"] != "directory_upstream_failed" {
				t.Fatalf("error code %q, want directory_upstream_failed", body["code"])
			}
		})
	}
}

// TestUsersRouteAnswersAnEmptyArrayForAnEmptyRealm pins what the empty list
// means: only a realm that really has no users answers 200 [], and it is the
// documented array rather than null.
func TestUsersRouteAnswersAnEmptyArrayForAnEmptyRealm(t *testing.T) {
	h, signer := usersApp(t, &stubDirectory{users: nil})

	w := requestWith(t, h, http.MethodGet, "/v1/users", token(t, signer, "admin", "demo-admin"), "")
	if w.Code != http.StatusOK {
		t.Fatalf("answered %d (%s), want 200", w.Code, w.Body.String())
	}
	if got := strings.TrimSpace(w.Body.String()); got != "[]" {
		t.Fatalf("body is %q, want the documented empty array []", got)
	}
}
