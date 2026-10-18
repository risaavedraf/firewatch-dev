package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// Verifier is the subset of the OIDC verifier this service needs. It is an
// interface so the transport can be exercised without a live identity provider.
type Verifier interface {
	Verify(context.Context, string) (*oidc.IDToken, error)
}

// subjectKey carries the verified token subject, which is the only actor identity
// any write may attribute: a request-supplied identity is never trusted.
type subjectKey struct{}

// adminKey carries whether the verified token holds the admin role, so a write
// route can require it without re-reading the token.
type adminKey struct{}

// subjectOf returns the verified token subject, or "" when there is none. Every
// route that reaches a handler has passed Authorize, so this is non-empty there.
func subjectOf(ctx context.Context) string {
	subject, _ := ctx.Value(subjectKey{}).(string)
	return subject
}

// isAdmin reports whether the verified token held the admin role.
func isAdmin(ctx context.Context) bool {
	admin, _ := ctx.Value(adminKey{}).(bool)
	return admin
}

// Authorize is the authentication middleware of this service, the same policy the
// sibling services apply: the Authorization header must carry a Bearer token that
// verifies against the configured issuer and audience, must be a bearer token
// with a live not-before, and must hold one of the roles this service accepts.
//
// Reads are open to viewer and admin; admin-only enforcement for writes lives in
// the write handlers, so a viewer gets 403 (authorized but not permitted) instead
// of 401 on a route they may read.
func Authorize(verifier Verifier, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			fail(w, 401, "unauthorized")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		token, err := verifier.Verify(ctx, parts[1])
		if err != nil || token.Subject == "" {
			fail(w, 401, "unauthorized")
			return
		}
		var claims struct {
			Type      string `json:"typ"`
			NotBefore int64  `json:"nbf"`
			Realm     struct {
				Roles []string `json:"roles"`
			} `json:"realm_access"`
		}
		if token.Claims(&claims) != nil || claims.Type != "Bearer" || claims.NotBefore > time.Now().Unix() {
			fail(w, 401, "unauthorized")
			return
		}
		for _, role := range claims.Realm.Roles {
			if role != "viewer" && role != "admin" {
				continue
			}
			authorized := context.WithValue(r.Context(), subjectKey{}, token.Subject)
			authorized = context.WithValue(authorized, adminKey{}, role == "admin")
			next.ServeHTTP(w, r.WithContext(authorized))
			return
		}
		fail(w, 403, "forbidden")
	})
}

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// fail writes the shared error body of every service in this repository: a stable
// machine-readable code and the request's correlation id, which is the same
// literal the X-Request-ID header carries.
func fail(w http.ResponseWriter, status int, code string) {
	respond(w, status, map[string]string{"code": code, "request_id": w.Header().Get("X-Request-ID")})
}
