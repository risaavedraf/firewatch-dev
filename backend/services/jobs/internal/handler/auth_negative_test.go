package handler

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
)

// The production verifier accepts RS256 only and resolves keys from a remote
// JWKS (services/jobs/cmd/server/main.go). These tests reproduce that policy
// with controlled keys and assert the outcome on the real Authorize path, not on
// a gateway mock, so a forged or policy-violating token is proven to be rejected
// by the code that runs in production.

// policyReadyCheck makes /v1/jobs/status answer 200 so a valid token can be
// distinguished from a rejected one by status code alone.
func policyReadyCheck(context.Context) Status {
	now := time.Now().UTC()
	depth, age := int64(0), float64(0)
	return Status{
		CheckedAt: now,
		Ready:     true,
		Dependencies: Dependencies{
			Database:   Dependency{State: "ready"},
			Broker:     Dependency{State: "ready"},
			DeadLetter: DeadLetterStatus{State: "ready", Depth: &depth, SampledAt: &now, AgeSeconds: &age, MaxAgeSeconds: &age, MaxDepth: &depth},
		},
	}
}

// policyClaims is a token that satisfies the production policy: the configured
// issuer and audience, a live expiry, the Bearer type and the admin role.
func policyClaims() map[string]any {
	return map[string]any{
		"iss": "https://issuer.test", "aud": "firewatch-api", "sub": "admin-subject",
		"exp": time.Now().Add(time.Minute).Unix(), "typ": "Bearer",
		"realm_access": map[string]any{"roles": []string{"admin"}},
	}
}

func signTokenWith(t *testing.T, key *rsa.PrivateKey, algorithm jose.SignatureAlgorithm, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: algorithm, Key: key}, nil)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	signed, err := signer.Sign(payload)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	raw, err := signed.CompactSerialize()
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	return raw
}

func authorizedStatus(t *testing.T, verifier Verifier, token string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/status", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	New(&fakeStore{}, verifier, policyReadyCheck).ServeHTTP(response, request)
	return response
}

// S2/S3: a token that violates the configured policy must be rejected with 401
// before any handler runs. The valid-token control at the end proves the cases
// are not passing because every token is refused.
func TestAuthorizeRejectsTokensThatViolateTheConfiguredPolicy(t *testing.T) {
	trusted, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	untrusted, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	// Only the trusted key is published, as a real JWKS publishes only the
	// tenant's own keys.
	verifier := oidc.NewVerifier(
		"https://issuer.test",
		&oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&trusted.PublicKey}},
		&oidc.Config{ClientID: "firewatch-api", SupportedSigningAlgs: []string{"RS256"}},
	)

	for _, tc := range []struct {
		name      string
		key       *rsa.PrivateKey
		algorithm jose.SignatureAlgorithm
		mutate    func(map[string]any)
	}{
		{name: "signature from a key outside the set", key: untrusted, algorithm: jose.RS256},
		{name: "algorithm outside the allowed set", key: trusted, algorithm: jose.RS512},
		{name: "missing exp", key: trusted, algorithm: jose.RS256, mutate: func(c map[string]any) { delete(c, "exp") }},
		{name: "empty sub", key: trusted, algorithm: jose.RS256, mutate: func(c map[string]any) { c["sub"] = "" }},
		{name: "wrong audience", key: trusted, algorithm: jose.RS256, mutate: func(c map[string]any) { c["aud"] = "other" }},
		{name: "wrong issuer", key: trusted, algorithm: jose.RS256, mutate: func(c map[string]any) { c["iss"] = "https://other.test" }},
		{name: "expired", key: trusted, algorithm: jose.RS256, mutate: func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() }},
		{name: "not a bearer token", key: trusted, algorithm: jose.RS256, mutate: func(c map[string]any) { c["typ"] = "ID" }},
		{name: "not yet valid", key: trusted, algorithm: jose.RS256, mutate: func(c map[string]any) { c["nbf"] = time.Now().Add(time.Hour).Unix() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := policyClaims()
			if tc.mutate != nil {
				tc.mutate(claims)
			}
			token := signTokenWith(t, tc.key, tc.algorithm, claims)
			response := authorizedStatus(t, verifier, token)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("got %d want %d: %s", response.Code, http.StatusUnauthorized, response.Body.String())
			}
		})
	}

	t.Run("valid token control", func(t *testing.T) {
		token := signTokenWith(t, trusted, jose.RS256, policyClaims())
		response := authorizedStatus(t, verifier, token)
		if response.Code != http.StatusOK {
			t.Fatalf("the control token must be accepted, got %d: %s", response.Code, response.Body.String())
		}
	})
}

// S3: a JWKS that cannot be fetched must fail closed. A well-formed token is
// still refused, because no key was resolved to verify it against.
func TestAuthorizeFailsClosedWhenTheKeySetIsUnavailable(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	token := signTokenWith(t, key, jose.RS256, policyClaims())

	keys := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	jwksURL := keys.URL
	keys.Close() // the key set is now unreachable

	verifier := oidc.NewVerifier(
		"https://issuer.test",
		oidc.NewRemoteKeySet(context.Background(), jwksURL),
		&oidc.Config{ClientID: "firewatch-api", SupportedSigningAlgs: []string{"RS256"}},
	)

	response := authorizedStatus(t, verifier, token)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("an unreachable key set must fail closed with 401, got %d: %s", response.Code, response.Body.String())
	}
}
