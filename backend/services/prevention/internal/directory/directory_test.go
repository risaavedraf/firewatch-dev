package directory

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// The Keycloak admin client is the only place this service holds an application
// credential, so these tests pin the three properties that matter operationally:
// one token serves many calls, a token is renewed BEFORE it expires, and a
// failure is an error rather than a silent empty answer.
//
// The fake below stands in for the two endpoints the client uses. It counts the
// calls, which is how caching is observed from outside the client.

// fakeSecret is a local placeholder, not a credential: no real secret is stored
// anywhere in this repository or in this test.
const fakeSecret = "local-test-only-placeholder"

// fakeKeycloak serves the realm's client_credentials token endpoint and its admin
// users read.
type fakeKeycloak struct {
	server *httptest.Server

	mu         sync.Mutex
	tokenCalls int
	usersCalls int

	tokenValue  string
	expiresIn   int
	tokenStatus int
	usersStatus int
	usersBody   string
	userStatus  int
	userBody    string
	userBodySet bool

	userCalls int

	lastUsersPath   string
	lastUsersQuery  url.Values
	lastUsersBearer string
	lastUserPath    string
	lastUserBearer  string
}

func newFakeKeycloak(t *testing.T) *fakeKeycloak {
	t.Helper()
	return newFakeKeycloakServer(t, false)
}

// newTLSFakeKeycloak serves the same two endpoints over https, which is how the
// scheme rule is observed: the URL and the certificate are the only difference.
func newTLSFakeKeycloak(t *testing.T) *fakeKeycloak {
	t.Helper()
	return newFakeKeycloakServer(t, true)
}

func newFakeKeycloakServer(t *testing.T, useTLS bool) *fakeKeycloak {
	t.Helper()
	fake := &fakeKeycloak{
		tokenValue:  "first-access-token",
		expiresIn:   300,
		tokenStatus: http.StatusOK,
		usersStatus: http.StatusOK,
		userStatus:  http.StatusOK,
		usersBody: `[
			{"id":"0f3f1f0e-0000-4000-8000-000000000001","username":"demo-admin","firstName":"Demo","lastName":"Admin"},
			{"id":"0f3f1f0e-0000-4000-8000-000000000002","username":"demo-viewer","firstName":"Demo","lastName":"Viewer"}
		]`,
	}
	if useTLS {
		fake.server = httptest.NewTLSServer(http.HandlerFunc(fake.serve))
	} else {
		fake.server = httptest.NewServer(http.HandlerFunc(fake.serve))
	}
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeKeycloak) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
		f.mu.Lock()
		f.tokenCalls++
		status, value, expiresIn := f.tokenStatus, f.tokenValue, f.expiresIn
		f.mu.Unlock()
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil || r.PostFormValue("grant_type") != "client_credentials" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": value,
			"expires_in":   expiresIn,
			"token_type":   "Bearer",
		})
	case strings.HasPrefix(r.URL.Path, "/admin/realms/firewatch/users/"):
		f.mu.Lock()
		f.userCalls++
		f.lastUserPath = r.URL.Path
		f.lastUserBearer = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		status, body := f.userStatus, f.userBody
		bodySet := f.userBodySet
		f.mu.Unlock()
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		if !bodySet {
			// Answer the user the path names, so a test that is not exercising the
			// user shape does not have to restate it.
			id := strings.TrimPrefix(r.URL.Path, "/admin/realms/firewatch/users/")
			body = `{"id":"` + id + `","username":"demo-operator","firstName":"Demo","lastName":"Operator"}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	case strings.HasSuffix(r.URL.Path, "/users"):
		f.mu.Lock()
		f.usersCalls++
		f.lastUsersPath, f.lastUsersQuery = r.URL.Path, r.URL.Query()
		f.lastUsersBearer = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		status, body := f.usersStatus, f.usersBody
		f.mu.Unlock()
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeKeycloak) counts() (tokens, users int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tokenCalls, f.usersCalls
}

func (f *fakeKeycloak) lastRead() (path string, query url.Values, bearer string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastUsersPath, f.lastUsersQuery, f.lastUsersBearer
}

func (f *fakeKeycloak) userReads() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.userCalls
}

func (f *fakeKeycloak) lastUserRead() (path, bearer string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastUserPath, f.lastUserBearer
}

// client returns a client pointed at the fake in development, which is the
// deployment mode that permits the fake's plaintext address. The clock is
// replaceable so the expiry rule can be observed without sleeping.
func (f *fakeKeycloak) client() *Client { return f.clientIn("development") }

// clientIn returns a client in the given APP_ENV, which is the deployment mode
// the admin URL's scheme rule is decided by. The empty string leaves the field
// unset, so the process's own APP_ENV decides.
func (f *fakeKeycloak) clientIn(environment string) *Client {
	return New(Config{
		BaseURL:      f.server.URL,
		Realm:        "firewatch",
		ClientID:     "firewatch-directory",
		ClientSecret: fakeSecret,
		Environment:  environment,
	})
}

// TestOneTokenServesTwoCalls pins the cache: two consecutive reads must cost one
// token acquisition, not two. Fetching a token per call would multiply the
// identity provider's credential work by the number of directory reads.
func TestOneTokenServesTwoCalls(t *testing.T) {
	fake := newFakeKeycloak(t)
	client := fake.client()

	for call := 0; call < 2; call++ {
		if _, err := client.Users(context.Background()); err != nil {
			t.Fatalf("users call %d: %v", call, err)
		}
	}
	tokens, users := fake.counts()
	if tokens != 1 {
		t.Fatalf("two directory reads acquired %d tokens, want 1: a valid token must be reused", tokens)
	}
	if users != 2 {
		t.Fatalf("two directory reads performed %d users reads, want 2", users)
	}
}

// TestTokenIsRefreshedBeforeItExpires pins the safety margin: the cache must stop
// trusting a token while it is still technically valid, so a token that is about
// to expire never travels to the identity provider.
func TestTokenIsRefreshedBeforeItExpires(t *testing.T) {
	fake := newFakeKeycloak(t)
	fake.expiresIn = 60
	client := fake.client()
	clock := time.Now()
	client.now = func() time.Time { return clock }

	if _, err := client.Users(context.Background()); err != nil {
		t.Fatalf("first read: %v", err)
	}
	// Still comfortably inside the token's life: the cached token is reused.
	if _, err := client.Users(context.Background()); err != nil {
		t.Fatalf("second read: %v", err)
	}
	if tokens, _ := fake.counts(); tokens != 1 {
		t.Fatalf("a token with 60s of life was refetched inside its valid window: %d acquisitions, want 1", tokens)
	}

	// Past the safety margin but BEFORE the 60s expiry: the cached token must no
	// longer be trusted.
	clock = clock.Add(45 * time.Second)
	fake.mu.Lock()
	fake.tokenValue = "second-access-token"
	fake.mu.Unlock()
	if _, err := client.Users(context.Background()); err != nil {
		t.Fatalf("third read: %v", err)
	}
	if tokens, _ := fake.counts(); tokens != 2 {
		t.Fatalf("a token 15s from expiry was still reused: %d acquisitions, want 2", tokens)
	}
	if _, _, bearer := fake.lastRead(); bearer != "second-access-token" {
		t.Fatalf("the users read carried %q, want the refreshed token", bearer)
	}
}

// TestUsersAreReadFromTheRealmAdminAPI pins the endpoint the read targets and the
// bearer it carries, so the client cannot quietly fall back to an unauthenticated
// or differently-shaped call.
func TestUsersAreReadFromTheRealmAdminAPI(t *testing.T) {
	fake := newFakeKeycloak(t)

	users, err := fake.client().Users(context.Background())
	if err != nil {
		t.Fatalf("read the realm users: %v", err)
	}
	path, query, bearer := fake.lastRead()
	if want := "/admin/realms/firewatch/users"; path != want {
		t.Fatalf("read %q, want %q", path, want)
	}
	if got := query.Get("max"); got != "200" {
		t.Fatalf("requested max=%q, want the documented bound 200", got)
	}
	if bearer != "first-access-token" {
		t.Fatalf("the users read carried %q, want the acquired access token", bearer)
	}
	if len(users) != 2 {
		t.Fatalf("decoded %d users, want 2", len(users))
	}
	if users[0].ID != "0f3f1f0e-0000-4000-8000-000000000001" || users[0].DisplayName != "Demo Admin" {
		t.Fatalf("first user decoded as %+v, want the admin account with names joined", users[0])
	}
}

// TestDisplayNameFallsBackInTheDocumentedOrder is the display-name rule itself:
// names first, then the username, then the identifier, and never an empty string.
func TestDisplayNameFallsBackInTheDocumentedOrder(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "both names", raw: `{"id":"a","username":"u1","firstName":"Demo","lastName":"Admin"}`, want: "Demo Admin"},
		{name: "first name only", raw: `{"id":"a","username":"u1","firstName":"Demo"}`, want: "Demo"},
		{name: "last name only", raw: `{"id":"a","username":"u1","lastName":"Admin"}`, want: "Admin"},
		{name: "no names, username", raw: `{"id":"a","username":"demo-admin"}`, want: "demo-admin"},
		{name: "no names, preferred username", raw: `{"id":"a","preferred_username":"preferred-admin"}`, want: "preferred-admin"},
		{name: "nothing but the identifier", raw: `{"id":"a"}`, want: "a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeKeycloak(t)
			fake.mu.Lock()
			fake.usersBody = "[" + tc.raw + "]"
			fake.mu.Unlock()
			users, err := fake.client().Users(context.Background())
			if err != nil {
				t.Fatalf("read the realm users: %v", err)
			}
			if len(users) != 1 {
				t.Fatalf("decoded %d users, want 1", len(users))
			}
			if users[0].DisplayName != tc.want {
				t.Fatalf("display name %q, want %q", users[0].DisplayName, tc.want)
			}
			if users[0].DisplayName == "" {
				t.Fatal("an empty display name reached the caller")
			}
		})
	}
}

// TestAUserWithoutAnIdentifierIsNotReturned keeps "never an empty display name"
// true for a malformed entry: a user the identity provider reports without an id
// cannot be addressed, so it is dropped instead of being returned blank.
func TestAUserWithoutAnIdentifierIsNotReturned(t *testing.T) {
	fake := newFakeKeycloak(t)
	fake.mu.Lock()
	fake.usersBody = `[{"username":"ghost"},{"id":"b","username":"real"}]`
	fake.mu.Unlock()

	users, err := fake.client().Users(context.Background())
	if err != nil {
		t.Fatalf("read the realm users: %v", err)
	}
	if len(users) != 1 || users[0].ID != "b" {
		t.Fatalf("decoded %+v, want only the addressable user", users)
	}
}

// TestAMissingSecretIsAnErrorAndIssuesNoRequest is the fail-closed rule at the
// client boundary: without the application credential the client refuses to make
// an anonymous admin call, and it makes no request at all.
func TestAMissingSecretIsAnErrorAndIssuesNoRequest(t *testing.T) {
	fake := newFakeKeycloak(t)
	for _, tc := range []struct {
		name   string
		config Config
	}{
		{name: "no secret", config: Config{BaseURL: fake.server.URL, Realm: "firewatch", ClientID: "firewatch-directory"}},
		{name: "no admin url", config: Config{Realm: "firewatch", ClientID: "firewatch-directory", ClientSecret: fakeSecret}},
		{name: "malformed admin url", config: Config{BaseURL: "not-a-url", Realm: "firewatch", ClientID: "firewatch-directory", ClientSecret: fakeSecret}},
		{name: "non-http admin url", config: Config{BaseURL: "ftp://identity:8080", Realm: "firewatch", ClientID: "firewatch-directory", ClientSecret: fakeSecret}},
		{name: "zero config", config: Config{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			users, err := New(tc.config).Users(context.Background())
			if !errors.Is(err, ErrNotConfigured) {
				t.Fatalf("error %v, want ErrNotConfigured: an unconfigured directory must fail closed", err)
			}
			if users != nil {
				t.Fatalf("returned %+v alongside the configuration error", users)
			}
			if tokens, reads := fake.counts(); tokens != 0 || reads != 0 {
				t.Fatalf("an unconfigured directory made %d token and %d users requests, want none", tokens, reads)
			}
		})
	}
}

// TestUpstreamFailuresAreErrors pins the propagation rule: a refused token, a
// failing users read, an unreachable identity provider and a malformed body are
// errors, never an empty list and never a panic.
func TestUpstreamFailuresAreErrors(t *testing.T) {
	t.Run("token endpoint refuses", func(t *testing.T) {
		fake := newFakeKeycloak(t)
		fake.tokenStatus = http.StatusUnauthorized
		users, err := fake.client().Users(context.Background())
		if err == nil {
			t.Fatalf("a refused token returned %+v, want an error", users)
		}
		assertNoCredentialInError(t, err)
	})

	t.Run("users endpoint fails", func(t *testing.T) {
		fake := newFakeKeycloak(t)
		fake.usersStatus = http.StatusInternalServerError
		users, err := fake.client().Users(context.Background())
		if err == nil {
			t.Fatalf("a failing users read returned %+v, want an error", users)
		}
		assertNoCredentialInError(t, err)
	})

	t.Run("unreachable", func(t *testing.T) {
		client := New(Config{BaseURL: "http://127.0.0.1:1", Realm: "firewatch", ClientID: "firewatch-directory", ClientSecret: fakeSecret})
		if _, err := client.Users(context.Background()); err == nil {
			t.Fatal("an unreachable identity provider returned no error")
		}
	})

	t.Run("malformed body", func(t *testing.T) {
		fake := newFakeKeycloak(t)
		fake.mu.Lock()
		fake.usersBody = `{"not":"a list"}`
		fake.mu.Unlock()
		if _, err := fake.client().Users(context.Background()); err == nil {
			t.Fatal("a body that is not a user list returned no error")
		}
	})
}

// TestUsersRejectsANonListBody pins the shape of the answer this read accepts:
// exactly one JSON array of user objects. `null` and an array followed by
// trailing bytes both decode into a slice without an error, so a body the
// identity provider never meant as a list would arrive as "the realm has no
// users"; both are refused here instead.
func TestUsersRejectsANonListBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "null", body: `null`},
		{name: "an object", body: `{"not":"a list"}`},
		{name: "one user object", body: `{"id":"u1","username":"demo-admin"}`},
		{name: "an array with trailing bytes", body: `[] trailing`},
		{name: "an array with a second document", body: `[]{}{}`},
		{name: "an empty body", body: ``},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeKeycloak(t)
			fake.mu.Lock()
			fake.usersBody = tc.body
			fake.mu.Unlock()

			users, err := fake.client().Users(context.Background())
			if err == nil {
				t.Fatalf("a body of %q returned %+v, want an error", tc.body, users)
			}
			if users != nil {
				t.Fatalf("returned %+v alongside the error", users)
			}
			if want := "directory users response is not a user list"; err.Error() != want {
				t.Fatalf("error %q, want the stable text %q", err, want)
			}
			assertNoCredentialInError(t, err)
		})
	}
}

// TestAnEmptyArrayIsAnEmptyRealm is the one body that still means "no users":
// the stricter shape rule must not turn a fresh realm into a failure.
func TestAnEmptyArrayIsAnEmptyRealm(t *testing.T) {
	fake := newFakeKeycloak(t)
	fake.mu.Lock()
	fake.usersBody = "[]"
	fake.mu.Unlock()

	users, err := fake.client().Users(context.Background())
	if err != nil {
		t.Fatalf("an empty array returned %v, want an empty result", err)
	}
	if len(users) != 0 {
		t.Fatalf("an empty array returned %+v, want no users", users)
	}
}

// TestHTTPSIsAcceptedOutsideDevelopment pins the half of the transport rule that
// holds everywhere: TLS to the admin API needs no deployment mode. The fake
// serves https and this client trusts only the fake's certificate.
func TestHTTPSIsAcceptedOutsideDevelopment(t *testing.T) {
	fake := newTLSFakeKeycloak(t)
	client := fake.clientIn("production")
	client.httpClient = fake.server.Client()

	users, err := client.Users(context.Background())
	if err != nil {
		t.Fatalf("an https admin URL in production returned %v, want the read to proceed", err)
	}
	if len(users) != 2 {
		t.Fatalf("decoded %d users, want 2", len(users))
	}
}

// TestHTTPIsAcceptedInDevelopment keeps the local stack's own default valid: the
// compose file points this read at http://identity:8080 with APP_ENV=development.
func TestHTTPIsAcceptedInDevelopment(t *testing.T) {
	fake := newFakeKeycloak(t)
	if _, err := fake.clientIn("development").Users(context.Background()); err != nil {
		t.Fatalf("a plain http admin URL in development returned %v, want it accepted", err)
	}
}

// TestHTTPIsRefusedOutsideDevelopment is the other half: the client secret
// travels to this address on every token acquisition, so a plaintext admin URL
// outside development would send it in clear text. It is refused before any
// request is made, as a configuration failure this service owns rather than an
// upstream failure.
func TestHTTPIsRefusedOutsideDevelopment(t *testing.T) {
	for _, environment := range []string{"", "production", "staging"} {
		t.Run("APP_ENV="+environment, func(t *testing.T) {
			fake := newFakeKeycloak(t)

			users, err := fake.clientIn(environment).Users(context.Background())
			if err == nil {
				t.Fatalf("a plain http admin URL was accepted with APP_ENV=%q and returned %+v", environment, users)
			}
			if users != nil {
				t.Fatalf("returned %+v alongside the error", users)
			}
			if !errors.Is(err, ErrNotConfigured) {
				t.Fatalf("error %v, want ErrNotConfigured so the route keeps answering its documented 503", err)
			}
			if !errors.Is(err, ErrInsecureURL) {
				t.Fatalf("error %v, want ErrInsecureURL to name the refused scheme", err)
			}
			if !strings.Contains(err.Error(), "https") {
				t.Fatalf("error %q does not tell the operator that https is required", err)
			}
			assertNoCredentialInError(t, err)
			if tokens, reads := fake.counts(); tokens != 0 || reads != 0 {
				t.Fatalf("a refused admin URL made %d token and %d users requests, want none", tokens, reads)
			}
		})
	}
}

// TestTheProcessAPPENVDecidesTheSchemeRule pins the wiring the service startup
// relies on: it passes the four directory variables and nothing about the mode,
// so an unset Environment must fall back to the same APP_ENV the startup reads.
func TestTheProcessAPPENVDecidesTheSchemeRule(t *testing.T) {
	fake := newFakeKeycloak(t)

	t.Setenv("APP_ENV", "production")
	if _, err := fake.clientIn("").Users(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("APP_ENV=production with a plain http admin URL returned %v, want ErrNotConfigured", err)
	}

	t.Setenv("APP_ENV", "development")
	if _, err := fake.clientIn("").Users(context.Background()); err != nil {
		t.Fatalf("APP_ENV=development with a plain http admin URL returned %v, want it accepted", err)
	}
}

// TestARevokedTokenIsNotReused is the recovery rule: when the admin API refuses
// the bearer (401 or 403), the token is dead before its stated expiry, so the
// next call must acquire a fresh one instead of failing until that expiry.
func TestARevokedTokenIsNotReused(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			fake := newFakeKeycloak(t)
			fake.mu.Lock()
			fake.usersStatus = status
			fake.mu.Unlock()
			client := fake.client()

			if _, err := client.Users(context.Background()); err == nil {
				t.Fatalf("a users read answered %d and returned no error", status)
			}

			fake.mu.Lock()
			fake.usersStatus = http.StatusOK
			fake.tokenValue = "second-access-token"
			fake.mu.Unlock()
			if _, err := client.Users(context.Background()); err != nil {
				t.Fatalf("the read after a token refused with %d: %v", status, err)
			}
			if tokens, _ := fake.counts(); tokens != 2 {
				t.Fatalf("a token refused with %d was reused: %d acquisitions, want a fresh one", status, tokens)
			}
			if _, _, bearer := fake.lastRead(); bearer != "second-access-token" {
				t.Fatalf("the read after a refusal carried %q, want the fresh token", bearer)
			}
		})
	}
}

// TestAFailingUsersReadKeepsTheToken is the other half: a 5xx says nothing about
// the bearer, so the cached token is reused and a recovered identity provider is
// reached without a second acquisition.
func TestAFailingUsersReadKeepsTheToken(t *testing.T) {
	fake := newFakeKeycloak(t)
	fake.mu.Lock()
	fake.usersStatus = http.StatusInternalServerError
	fake.mu.Unlock()
	client := fake.client()

	if _, err := client.Users(context.Background()); err == nil {
		t.Fatal("a failing users read returned no error")
	}

	fake.mu.Lock()
	fake.usersStatus = http.StatusOK
	fake.mu.Unlock()
	if _, err := client.Users(context.Background()); err != nil {
		t.Fatalf("the read after a 5xx: %v", err)
	}
	if tokens, _ := fake.counts(); tokens != 1 {
		t.Fatalf("a 5xx discarded a healthy token: %d acquisitions, want the cached one reused", tokens)
	}
}

// assertNoCredentialInError keeps the credential out of the error path: errors
// travel into logs, so neither the secret nor an access token may be in them.
// TestDirectoryReadsOneUserByIdentifier pins the single-user read: one identifier
// is read directly at the admin API's user path, carrying the cached bearer, and
// the answer is the same User shape the users route presents. It must not cost a
// list read, which is the whole point of the operation.
func TestDirectoryReadsOneUserByIdentifier(t *testing.T) {
	const id = "0f3f1f0e-0000-4000-8000-000000000009"
	fake := newFakeKeycloak(t)
	fake.mu.Lock()
	fake.userBody = `{"id":"` + id + `","username":"demo-operator","firstName":"Brigada","lastName":"Dos"}`
	fake.userBodySet = true
	fake.mu.Unlock()

	user, err := fake.client().User(context.Background(), id)
	if err != nil {
		t.Fatalf("read the user by identifier: %v", err)
	}
	if user.ID != id || user.DisplayName != "Brigada Dos" {
		t.Fatalf("decoded %+v, want the requested identifier and its names", user)
	}
	path, bearer := fake.lastUserRead()
	if want := "/admin/realms/firewatch/users/" + id; path != want {
		t.Fatalf("read %q, want %q", path, want)
	}
	if bearer != "first-access-token" {
		t.Fatalf("the user read carried %q, want the acquired access token", bearer)
	}
	if calls := fake.userReads(); calls != 1 {
		t.Fatalf("performed %d by-identifier reads, want 1", calls)
	}
	if _, listReads := fake.counts(); listReads != 0 {
		t.Fatalf("the by-identifier read also performed %d list reads, want 0", listReads)
	}
}

// TestDirectoryUserNotFoundIsASentinel pins the one answer that is not an
// upstream failure: a 404 means the identity provider is healthy and has no such
// user, so it is a distinct sentinel callers map to their documented 422. It
// also says nothing about the bearer, so the cached token stays.
func TestDirectoryUserNotFoundIsASentinel(t *testing.T) {
	const id = "0f3f1f0e-0000-4000-8000-000000000404"
	fake := newFakeKeycloak(t)
	fake.mu.Lock()
	fake.userStatus = http.StatusNotFound
	fake.mu.Unlock()
	client := fake.client()

	user, err := client.User(context.Background(), id)
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("error %v, want ErrUserNotFound: a 404 is a distinct answer", err)
	}
	if errors.Is(err, ErrNotConfigured) {
		t.Fatalf("error %v also reads as a configuration failure", err)
	}
	if user != (User{}) {
		t.Fatalf("returned %+v alongside the not-found error", user)
	}
	assertNoCredentialInError(t, err)

	if _, err := client.User(context.Background(), id); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("second read returned %v, want ErrUserNotFound", err)
	}
	if tokens, _ := fake.counts(); tokens != 1 {
		t.Fatalf("a 404 discarded a healthy token: %d acquisitions, want the cached one reused", tokens)
	}
}

// TestDirectoryRefusedBearerIsNotReused is the by-identifier half of the token
// recovery rule: a 401 or 403 says the bearer is dead before its stated expiry,
// so the token is dropped and the next read acquires a fresh one.
func TestDirectoryRefusedBearerIsNotReused(t *testing.T) {
	const id = "0f3f1f0e-0000-4000-8000-000000000005"
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			fake := newFakeKeycloak(t)
			fake.mu.Lock()
			fake.userStatus = status
			fake.mu.Unlock()
			client := fake.client()

			_, err := client.User(context.Background(), id)
			if err == nil {
				t.Fatalf("a user read answered %d and returned no error", status)
			}
			if errors.Is(err, ErrUserNotFound) {
				t.Fatalf("a bearer refused with %d reported the user as missing: %v", status, err)
			}
			assertNoCredentialInError(t, err)

			fake.mu.Lock()
			fake.userStatus = http.StatusOK
			fake.tokenValue = "second-access-token"
			fake.mu.Unlock()
			user, err := client.User(context.Background(), id)
			if err != nil {
				t.Fatalf("the read after a token refused with %d: %v", status, err)
			}
			if user.ID != id {
				t.Fatalf("the read after a refusal decoded %+v, want the requested user", user)
			}
			if tokens, _ := fake.counts(); tokens != 2 {
				t.Fatalf("a token refused with %d was reused: %d acquisitions, want a fresh one", status, tokens)
			}
			if _, bearer := fake.lastUserRead(); bearer != "second-access-token" {
				t.Fatalf("the read after a refusal carried %q, want the fresh token", bearer)
			}
		})
	}
}

// TestDirectoryUserReadUpstreamFailuresAreErrors pins the propagation rule for
// the single-user read: a failing endpoint and an unreachable identity provider
// are upstream errors, never a not-found answer and never a zero user.
func TestDirectoryUserReadUpstreamFailuresAreErrors(t *testing.T) {
	const id = "0f3f1f0e-0000-4000-8000-000000000500"

	t.Run("5xx", func(t *testing.T) {
		fake := newFakeKeycloak(t)
		fake.mu.Lock()
		fake.userStatus = http.StatusInternalServerError
		fake.mu.Unlock()

		user, err := fake.client().User(context.Background(), id)
		if err == nil {
			t.Fatalf("a failing user read returned %+v, want an error", user)
		}
		if errors.Is(err, ErrUserNotFound) {
			t.Fatalf("a 5xx was reported as a missing user: %v", err)
		}
		assertNoCredentialInError(t, err)
	})

	t.Run("unreachable", func(t *testing.T) {
		client := New(Config{BaseURL: "http://127.0.0.1:1", Realm: "firewatch", ClientID: "firewatch-directory", ClientSecret: fakeSecret})

		user, err := client.User(context.Background(), id)
		if err == nil {
			t.Fatalf("an unreachable identity provider returned %+v, want an error", user)
		}
		if errors.Is(err, ErrUserNotFound) {
			t.Fatalf("a transport failure was reported as a missing user: %v", err)
		}
		assertNoCredentialInError(t, err)
	})
}

// TestDirectoryUserReadRejectsANonUserBody pins the shape of the answer: exactly
// one JSON user object with an identifier. `null` decodes into a zero struct and
// trailing bytes are never read, so a body the identity provider never meant as
// a user would arrive as "the directory knows nobody"; both are refused here,
// even on a 200, instead of being trusted.
func TestDirectoryUserReadRejectsANonUserBody(t *testing.T) {
	const id = "0f3f1f0e-0000-4000-8000-000000000200"
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "null", body: `null`},
		{name: "an empty array", body: `[]`},
		{name: "an array of one user", body: `[{"id":"u1"}]`},
		{name: "garbage", body: `<html>not json</html>`},
		{name: "an empty body", body: ``},
		{name: "two documents", body: `{}{}`},
		{name: "an object without an identifier", body: `{"username":"ghost"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeKeycloak(t)
			fake.mu.Lock()
			fake.userBody = tc.body
			fake.userBodySet = true
			fake.mu.Unlock()

			user, err := fake.client().User(context.Background(), id)
			if err == nil {
				t.Fatalf("a body of %q returned %+v, want an error", tc.body, user)
			}
			if user != (User{}) {
				t.Fatalf("returned %+v alongside the error", user)
			}
			if errors.Is(err, ErrUserNotFound) {
				t.Fatalf("a malformed body was reported as a missing user: %v", err)
			}
			if want := "directory user response is not a user object"; err.Error() != want {
				t.Fatalf("error %q, want the stable text %q", err, want)
			}
			assertNoCredentialInError(t, err)
		})
	}
}

// TestDirectoryUserReadDoesNotUseTheUserList is the regression this read exists
// for: a realm whose users list is unavailable, or whose first page does not
// hold the requested identifier, must still resolve that identifier. A list
// read here would fail, so a successful answer with zero list reads proves the
// by-identifier path neither requires nor uses the 200-entry page.
func TestDirectoryUserReadDoesNotUseTheUserList(t *testing.T) {
	const id = "0f3f1f0e-0000-4000-8000-000000000777"
	fake := newFakeKeycloak(t)
	fake.mu.Lock()
	fake.usersStatus = http.StatusInternalServerError
	fake.usersBody = `[]`
	fake.mu.Unlock()

	user, err := fake.client().User(context.Background(), id)
	if err != nil {
		t.Fatalf("the by-identifier read depended on the users list: %v", err)
	}
	if user.ID != id {
		t.Fatalf("decoded %+v, want the requested identifier", user)
	}
	if _, listReads := fake.counts(); listReads != 0 {
		t.Fatalf("the by-identifier read performed %d list reads, want 0", listReads)
	}
}

// tlsCABundle writes the certificate a test server presents as the PEM bundle
// TLS_CA_FILE names, inside a fresh temp dir.
func tlsCABundle(t *testing.T, server *httptest.Server) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.crt")
	bundle := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(path, bundle, 0o600); err != nil {
		t.Fatalf("write the CA bundle: %v", err)
	}
	return path
}

// TestTLSCAFileFeedsTheAdminClient pins T7.4's trust side for the administrative
// client: with TLS_CA_FILE the admin API is verified against that pool only,
// which is what lets KEYCLOAK_ADMIN_URL be https inside a deployment where the
// identity provider presents a certificate no public root vouches for.
func TestTLSCAFileFeedsTheAdminClient(t *testing.T) {
	fake := newTLSFakeKeycloak(t)
	t.Setenv(tlsCAFileEnv, tlsCABundle(t, fake.server))
	client := fake.clientIn("production")
	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok || transport.TLSClientConfig == nil || transport.TLSClientConfig.RootCAs == nil {
		t.Fatalf("the admin client does not carry the configured CA: %#v", client.httpClient.Transport)
	}
	// The read is the proof, not the field: the https fake is reachable only
	// through the pool the bundle built, because the system roots never vouch for
	// the certificate it presents.
	users, err := client.Users(context.Background())
	if err != nil || len(users) != 2 {
		t.Fatalf("https admin read with the internal CA = %+v, %v", users, err)
	}
}

// TestNoTLSCAFileKeepsTheBareAdminClient pins the behaviour that must not
// change: without TLS_CA_FILE the admin client keeps no Transport at all, so the
// system roots and the 5s bound it always had stay in effect, and the plaintext
// local stack keeps reading the directory.
func TestNoTLSCAFileKeepsTheBareAdminClient(t *testing.T) {
	fake := newFakeKeycloak(t)
	t.Setenv(tlsCAFileEnv, "")
	client := fake.clientIn("development")
	if client.httpClient.Transport != nil {
		t.Fatalf("without TLS_CA_FILE the admin client must keep no Transport, got %#v", client.httpClient.Transport)
	}
	if client.httpClient.Timeout != requestTimeout {
		t.Fatalf("timeout %v, want the %v bound it always had", client.httpClient.Timeout, requestTimeout)
	}
	if _, err := client.Users(context.Background()); err != nil {
		t.Fatalf("plaintext development read = %v, want it accepted", err)
	}
}

// TestAnUnloadableTLSCAFileDoesNotBecomeATrustAnchor pins the refusal: a bundle
// that cannot be read, or holds no certificate, never becomes an empty pool or a
// silent fall back presented as the internal CA. Startup reads the same path and
// stops there; a client built anyway keeps the bare client, so the handshake
// fails closed instead of trusting something nobody configured.
func TestAnUnloadableTLSCAFileDoesNotBecomeATrustAnchor(t *testing.T) {
	dir := t.TempDir()
	blank := filepath.Join(dir, "blank.crt")
	if err := os.WriteFile(blank, []byte("not a certificate"), 0o600); err != nil {
		t.Fatalf("write the unusable bundle: %v", err)
	}
	for _, path := range []string{filepath.Join(dir, "absent.crt"), blank} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Setenv(tlsCAFileEnv, path)
			if client := newFakeKeycloak(t).client(); client.httpClient.Transport != nil {
				t.Fatalf("TLS_CA_FILE=%s built a trust anchor: %#v", path, client.httpClient.Transport)
			}
		})
	}
}

func assertNoCredentialInError(t *testing.T, err error) {
	t.Helper()
	for _, credential := range []string{fakeSecret, "first-access-token", "second-access-token"} {
		if strings.Contains(err.Error(), credential) {
			t.Fatalf("the error carries a credential: %v", err)
		}
	}
}
