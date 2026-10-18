package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

type Verifier interface {
	Verify(context.Context, string) (*oidc.IDToken, error)
}

// rolesContextKey carries the verified realm roles from Authorize to a
// per-route guard, so a route can require admin without verifying the token a
// second time.
type rolesContextKey struct{}

// rolesFrom returns the verified roles placed on the request context by
// Authorize. An unauthenticated request carries none.
func rolesFrom(ctx context.Context) []string {
	roles, _ := ctx.Value(rolesContextKey{}).([]string)
	return roles
}

// Authorize authenticates the bearer token and admits the viewer and admin roles,
// the same read-or-admin gate every route has always used. It stores the
// verified roles on the request context so an admin-only route wraps itself with
// requireAdmin instead of re-verifying the token.
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
			if role == "viewer" || role == "admin" {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), rolesContextKey{}, claims.Realm.Roles)))
				return
			}
		}
		fail(w, 403, "forbidden")
	})
}

// requireAdmin admits only the admin role. It is applied inside the router, after
// Authorize has already verified the token and stored the roles, so it neither
// re-verifies nor lets an unauthenticated request through: Authorize's 401 runs
// first.
func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for _, role := range rolesFrom(r.Context()) {
			if role == "admin" {
				next(w, r)
				return
			}
		}
		fail(w, 403, "forbidden")
	}
}

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, code string) {
	respond(w, status, map[string]string{"code": code, "request_id": w.Header().Get("X-Request-ID")})
}
