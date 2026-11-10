// Package directory reads the realm's users from the Keycloak Admin API.
//
// This is the only place the prevention service holds an application credential.
// The client authenticates with the OAuth client_credentials flow, keeps the
// access token in memory until shortly before it expires, and never writes the
// secret or the token anywhere but the two requests that need them: neither
// appears in an error, in a log line, or in anything this package returns.
//
// Users is one page: the admin API is asked for usersPageSize users, and with
// more than that in the realm the answer is the identity provider's first page,
// not a globally ordered first usersPageSize. The transport sorts what it
// received; it does not page through the remainder. User reads one identifier
// directly, so it is not bounded by that page.
package directory

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrNotConfigured reports an incomplete configuration: without an admin URL and
// the client credentials there is nothing to read the directory with. Callers
// must fail closed on it instead of issuing an anonymous admin call or answering
// an empty list the identity provider never confirmed.
var ErrNotConfigured = errors.New("directory is not configured")

// ErrInsecureURL reports an admin URL that would send the client secret in clear
// text. It is wrapped together with ErrNotConfigured, so a caller that only knows
// the configuration sentinel still sees a failure this service owns rather than an
// upstream one.
var ErrInsecureURL = errors.New("directory admin URL must use https outside development")

// ErrUserNotFound reports that the identity provider answered and has no user
// with the requested identifier. It is not an upstream failure: callers map it to
// their documented "not found" refusal instead of an unavailability answer.
var ErrUserNotFound = errors.New("directory user not found")

// errUserList rejects every users body that is not exactly one JSON array of user
// objects. The text is stable and carries no credential.
var errUserList = errors.New("directory users response is not a user list")

// errUserBody rejects every single-user body that is not exactly one JSON user
// object with an identifier. The text is stable and carries no credential.
var errUserBody = errors.New("directory user response is not a user object")

const (
	// defaultRealm and defaultClientID are the deployment defaults of
	// KEYCLOAK_ADMIN_REALM and DIRECTORY_CLIENT_ID, applied here so an
	// installation that names only the credential still addresses the same client
	// the realm file declares.
	defaultRealm    = "firewatch"
	defaultClientID = "firewatch-directory"

	// tokenSafetyMargin is how long before its stated expiry a cached token stops
	// being used. Renewing inside the margin means a token that is about to die is
	// never sent, and the renewal happens before another request needs it.
	tokenSafetyMargin = 30 * time.Second

	// usersPageSize is the slice of the realm this read asks for. The route caps
	// its answer at the same number, so a larger realm is read as its first page
	// rather than paged through behind a request that has a fixed cap anyway.
	usersPageSize = 200

	// requestTimeout bounds one call to the identity provider. It is explicit on
	// the single shared client, so a slow identity provider cannot hold a request
	// open indefinitely.
	requestTimeout = 5 * time.Second

	// Body bounds. All three responses are small documents; anything larger is
	// refused rather than buffered.
	maxTokenBody = 8192
	maxUsersBody = 1 << 20
	maxUserBody  = 64 << 10

	// tlsCAFileEnv names the internal CA of the deployment
	// (odd/tasks/kubernetes-deployment.md, L10). It is the same variable every
	// service of the deployment reads, and the only trust anchor this client uses
	// when it is set.
	tlsCAFileEnv = "TLS_CA_FILE"
)

// Config is the directory configuration, one field per environment variable:
// KEYCLOAK_ADMIN_URL, KEYCLOAK_ADMIN_REALM, DIRECTORY_CLIENT_ID and
// DIRECTORY_CLIENT_SECRET.
type Config struct {
	// BaseURL is the identity provider's base address, e.g. http://identity:8080.
	// It is the admin URL, not a browser-facing URL.
	BaseURL string
	// Realm is the realm to read. Empty means defaultRealm.
	Realm string
	// ClientID is the confidential client the directory acts as. Empty means
	// defaultClientID.
	ClientID string
	// ClientSecret is that client's secret. Empty means the directory is
	// unconfigured and every read fails with ErrNotConfigured.
	ClientSecret string
	// Environment is the deployment mode, APP_ENV. It is the same value the
	// service's startup reads to decide whether the OIDC endpoints may use plain
	// http, and it decides the same for the admin URL: https is always accepted,
	// http only when it is development. Empty means the process's APP_ENV decides.
	Environment string
}

// User is one entry of the realm's user directory: the identifier the identity
// provider assigns and the name to present for it. DisplayName is never empty.
type User struct {
	ID          string
	DisplayName string
}

// Client reads the realm's users. It is safe for concurrent use.
type Client struct {
	config     Config
	httpClient *http.Client
	now        func() time.Time
	// allowInsecureHTTP is the resolved deployment mode: a plaintext admin URL is
	// only accepted when the service runs in development, because the client
	// secret travels to that address on every token acquisition.
	allowInsecureHTTP bool

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// RootCAsFromFile loads the PEM bundle at path into a new pool. An unreadable or
// certificate-free bundle is refused instead of silently becoming the system
// roots: the deployment only ever talks to services this CA vouches for, so a
// missing mount has to be visible at startup and not as a handshake failure later.
// The service's startup refuses on the same error, which is why this is exported:
// the OIDC key-set client of cmd/server verifies with the same bundle.
func RootCAsFromFile(path string) (*x509.CertPool, error) {
	bundle, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(bundle) {
		return nil, fmt.Errorf("%s holds no PEM certificate", path)
	}
	return pool, nil
}

// New builds a client. It never fails: a configuration that cannot address the
// directory is reported by Users and User as ErrNotConfigured, so a deployment
// without the credential starts and answers 503 instead of refusing to boot.
//
// The scheme rule needs the deployment mode, which the process reports through
// APP_ENV, exactly as the service's own startup does for the OIDC endpoints: an
// unset Environment falls back to the process's APP_ENV.
func New(config Config) *Client {
	if config.Realm == "" {
		config.Realm = defaultRealm
	}
	if config.ClientID == "" {
		config.ClientID = defaultClientID
	}
	if config.Environment == "" {
		config.Environment = os.Getenv("APP_ENV")
	}
	client := &Client{
		config:            config,
		httpClient:        &http.Client{Timeout: requestTimeout},
		now:               time.Now,
		allowInsecureHTTP: config.Environment == "development",
	}
	// With TLS_CA_FILE the admin API is reached through the internal CA and no
	// other trust anchor; no variable keeps the bare client with the system roots
	// this client has always used. A bundle that cannot be loaded stays a startup
	// refusal - run() reads the same path before this client is built - and a client
	// built anyway keeps the bare one, so the handshake fails closed rather than
	// trusting something nobody configured.
	if path := os.Getenv(tlsCAFileEnv); path != "" {
		if pool, err := RootCAsFromFile(path); err == nil {
			client.httpClient.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}
		}
	}
	return client
}

// Users reads the realm's users. It returns ErrNotConfigured when the admin URL
// or the client credentials are missing, an error wrapping ErrInsecureURL when a
// plaintext admin URL is configured outside development, and an error for any
// answer of the identity provider that is not exactly one JSON array of user
// objects. It never returns an empty slice for a failed read: an empty answer
// means the realm really has no users.
//
// The result is the identity provider's first page of at most usersPageSize
// entries, so it is the whole directory only for a realm that fits in one page.
func (c *Client) Users(ctx context.Context) ([]User, error) {
	base, err := c.configured()
	if err != nil {
		return nil, err
	}
	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint(base, c.adminPath("/users"), url.Values{
		"first":               {"0"},
		"max":                 {strconv.Itoa(usersPageSize)},
		"briefRepresentation": {"true"},
	}), nil)
	if err != nil {
		return nil, errors.New("directory request could not be built")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("directory users read failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		// 401 and 403 are the identity provider refusing the bearer itself, so the
		// cached token is dead before its stated expiry and the next call acquires a
		// fresh one instead of failing until then. Any other status says nothing
		// about the token, and a transport error never reaches this branch.
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			c.discardToken(token)
		}
		return nil, fmt.Errorf("directory users read answered %d", response.StatusCode)
	}
	list, err := decodeUserList(io.LimitReader(response.Body, maxUsersBody))
	if err != nil {
		return nil, err
	}
	users := make([]User, 0, len(list))
	for _, entry := range list {
		// An entry without an identifier addresses nothing, and its display name
		// would have nothing left to fall back to. It is dropped rather than
		// returned blank.
		if entry.ID == "" {
			continue
		}
		users = append(users, User{ID: entry.ID, DisplayName: displayName(entry)})
	}
	return users, nil
}

// User reads exactly one realm user by identifier, directly at the admin API's
// single-user path. Unlike Users it is not bounded by a list page, so an
// identifier in a realm larger than usersPageSize is still resolved.
//
// A 404 is the identity provider's answer that no such user exists, and is
// returned as ErrUserNotFound; callers map it to their documented 422. An
// unconfigured client still returns ErrNotConfigured, and every other answer or
// failure is an upstream error. A 401/403 discards the cached token exactly as
// Users does, because the identity provider refused the bearer rather than the
// read being unanswerable. The returned user is never empty.
func (c *Client) User(ctx context.Context, id string) (User, error) {
	base, err := c.configured()
	if err != nil {
		return User{}, err
	}
	// An empty identifier addresses no user, and asking for the collection would
	// get the list body this read does not accept. It is reported as not found
	// without a request instead.
	if strings.TrimSpace(id) == "" {
		return User{}, ErrUserNotFound
	}
	token, err := c.accessToken(ctx)
	if err != nil {
		return User{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.endpoint(base, c.adminPath("/users/"+url.PathEscape(id)), nil), nil)
	if err != nil {
		return User{}, errors.New("directory request could not be built")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return User{}, fmt.Errorf("directory user read failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		// The identity provider is healthy and knows no such user. The bearer was
		// accepted, so the cached token stays.
		return User{}, ErrUserNotFound
	}
	if response.StatusCode != http.StatusOK {
		// Same recovery rule as the list read: 401 and 403 mean the bearer itself
		// was refused, so the cached token is dropped and the next call acquires a
		// fresh one. Any other status says nothing about the token.
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			c.discardToken(token)
		}
		return User{}, fmt.Errorf("directory user read answered %d", response.StatusCode)
	}
	entry, err := decodeUser(io.LimitReader(response.Body, maxUserBody))
	if err != nil {
		return User{}, err
	}
	return User{ID: entry.ID, DisplayName: displayName(entry)}, nil
}

// discardToken forgets a cached token the identity provider refused, so the next
// call acquires a fresh one. The token is dropped only while it is still the
// cached one: a concurrent call may already have replaced it, and that newer
// token is not known to be bad.
func (c *Client) discardToken(refused string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token == refused {
		c.token = ""
		c.expiresAt = time.Time{}
	}
}

// decodeUserList reads exactly one JSON array of user objects from body.
//
// Decoding straight into a []keycloakUser is not enough: `null` decodes into an
// empty slice with no error, and bytes after the array are never read, so a body
// the identity provider never meant as a list would arrive as "the realm has no
// users". Both are refused here, and so is an empty body, while `[]` stays the
// valid empty realm.
func decodeUserList(body io.Reader) ([]keycloakUser, error) {
	decoder := json.NewDecoder(body)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return nil, errUserList
	}
	// Decode consumes the first value only; the array must also be the whole
	// document, which is what refuses trailing bytes of any shape.
	if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		return nil, errUserList
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, errUserList
	}
	var entries []keycloakUser
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, errUserList
	}
	return entries, nil
}

// decodeUser reads exactly one JSON user object with an identifier from body.
//
// Decoding straight into a keycloakUser is not enough: `null` decodes into a zero
// struct with no error, and bytes after the object are never read, so a body the
// identity provider never meant as a user would arrive as an empty one. Both are
// refused here, and so is an object without an identifier: a user nothing can
// address is not one this read can return.
func decodeUser(body io.Reader) (keycloakUser, error) {
	decoder := json.NewDecoder(body)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return keycloakUser{}, errUserBody
	}
	// Decode consumes the first value only; the object must also be the whole
	// document, which is what refuses trailing bytes of any shape.
	if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		return keycloakUser{}, errUserBody
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		return keycloakUser{}, errUserBody
	}
	var entry keycloakUser
	if err := json.Unmarshal(raw, &entry); err != nil {
		return keycloakUser{}, errUserBody
	}
	if entry.ID == "" {
		return keycloakUser{}, errUserBody
	}
	return entry, nil
}

// accessToken returns a usable access token, acquiring one with the
// client_credentials flow when none is cached or when the cached one is inside
// the safety margin. The lock is held across the acquisition so concurrent calls
// share one token instead of stampeding the identity provider.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && c.now().Add(tokenSafetyMargin).Before(c.expiresAt) {
		return c.token, nil
	}
	base, err := c.configured()
	if err != nil {
		return "", err
	}
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.config.ClientID},
		"client_secret": {c.config.ClientSecret},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint(base, "/realms/"+url.PathEscape(c.config.Realm)+"/protocol/openid-connect/token", nil),
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", errors.New("directory token request could not be built")
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		// The credential travels in the request body, which the transport's error
		// does not carry; the wrapped error names the method and the token URL.
		return "", fmt.Errorf("directory token request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("directory token endpoint answered %d", response.StatusCode)
	}
	var grant struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxTokenBody)).Decode(&grant); err != nil {
		return "", errors.New("directory token response is not readable")
	}
	if grant.AccessToken == "" {
		return "", errors.New("directory token response carried no access token")
	}
	c.token = grant.AccessToken
	// A response without a positive lifetime is cached for no time at all, so the
	// next call acquires a fresh token instead of trusting an unknown one.
	c.expiresAt = c.now().Add(time.Duration(grant.ExpiresIn) * time.Second)
	return c.token, nil
}

// configured returns the effective base URL, or ErrNotConfigured. A URL that is
// not absolute http(s) is a configuration failure, not a runtime one: it can
// never reach an identity provider. A plaintext URL outside development is
// refused for the same reason the startup refuses a plaintext OIDC URL: the
// client secret would travel to it in clear text.
func (c *Client) configured() (*url.URL, error) {
	if strings.TrimSpace(c.config.BaseURL) == "" || strings.TrimSpace(c.config.ClientSecret) == "" {
		return nil, ErrNotConfigured
	}
	base, err := url.Parse(c.config.BaseURL)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") {
		return nil, ErrNotConfigured
	}
	if base.Scheme == "http" && !c.allowInsecureHTTP {
		return nil, fmt.Errorf("%w: %w", ErrNotConfigured, ErrInsecureURL)
	}
	return base, nil
}

// adminPath is the realm-scoped admin path of this client.
func (c *Client) adminPath(suffix string) string {
	return "/admin/realms/" + url.PathEscape(c.config.Realm) + suffix
}

// endpoint joins the base address with a path and its query.
func (c *Client) endpoint(base *url.URL, path string, query url.Values) string {
	joined := strings.TrimRight(base.String(), "/") + path
	if len(query) == 0 {
		return joined
	}
	return joined + "?" + query.Encode()
}

// keycloakUser is the subset of the admin API's user representation this read
// uses. briefRepresentation still carries the names and the username.
type keycloakUser struct {
	ID                string `json:"id"`
	Username          string `json:"username"`
	PreferredUsername string `json:"preferred_username"`
	FirstName         string `json:"firstName"`
	LastName          string `json:"lastName"`
}

// displayName resolves the name to present, in the documented order: the first
// and last name joined by a space when either is present, the username
// otherwise, and the identifier as the last resort. It is never empty for a user
// that carries an identifier.
func displayName(entry keycloakUser) string {
	names := strings.TrimSpace(strings.TrimSpace(entry.FirstName) + " " + strings.TrimSpace(entry.LastName))
	if names != "" {
		return names
	}
	// The admin API reports the account name as `username`; `preferred_username`
	// is the claim the token carries and some representations expose it too.
	// Either names the account, so both are accepted here.
	if username := strings.TrimSpace(entry.PreferredUsername); username != "" {
		return username
	}
	if username := strings.TrimSpace(entry.Username); username != "" {
		return username
	}
	return entry.ID
}
