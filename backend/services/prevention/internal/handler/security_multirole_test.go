package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"firewatch.local/prevention/internal/repository"
	"firewatch.local/prevention/internal/state"
	"github.com/go-jose/go-jose/v4"
)

// multiRoleToken mints a valid access token carrying several realm roles in the
// exact order given. The sibling helper in api_test.go mints one role, which is
// why the defect below was invisible to it.
func multiRoleToken(t *testing.T, signer jose.Signer, roles []string, subject string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"iss": "https://issuer.test", "aud": "firewatch-api", "sub": subject,
		"exp": time.Now().Add(time.Hour).Unix(), "typ": "Bearer",
		"realm_access": map[string]any{"roles": roles},
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

const createActionBody = `{"community_code":"05101","title":"accion de prueba"}`

// TestAnAdminTokenThatAlsoListsViewerKeepsItsAdminRights pins the multi-role case
// the isolated end-to-end verification found: the realm grants its admin account
// BOTH `viewer` and `admin`, and the order inside realm_access is the identity
// provider's decision, not this service's.
//
// The first implementation stored the decision of the FIRST recognised role, so a
// token listing `viewer` before `admin` lost its admin rights: reads answered 200
// and every write answered 403. The same token was admitted by `risk`, which scans
// the whole role list, so the difference was visible only when the two services
// were exercised with one token — something no single-role test could produce.
func TestAnAdminTokenThatAlsoListsViewerKeepsItsAdminRights(t *testing.T) {
	for _, roles := range [][]string{
		{"viewer", "admin"}, // the order that broke it
		{"admin", "viewer"},
		{"viewer", "offline_access", "admin"},
	} {
		t.Run(strings.Join(roles, "+"), func(t *testing.T) {
			store := &matrixStore{status: state.Propuesta, action: repository.Action{ID: fixedActionID}}
			h, signer := app(t, store)
			bearer := multiRoleToken(t, signer, roles, "demo-admin")
			rec := requestWith(t, h, http.MethodPost, "/v1/prevention/actions", bearer, createActionBody)
			if rec.Code != http.StatusCreated {
				t.Fatalf("POST /v1/prevention/actions with roles %v answered %d (%s); a token that holds admin must be admitted wherever the role appears in the list",
					roles, rec.Code, rec.Body.String())
			}
		})
	}
}

// TestAViewerOnlyTokenStillCannotWrite is the other half of the same rule: the fix
// must not hand a viewer the write routes. Without this case, a change that stopped
// checking roles at all would satisfy the test above.
func TestAViewerOnlyTokenStillCannotWrite(t *testing.T) {
	store := &matrixStore{status: state.Propuesta, action: repository.Action{ID: fixedActionID}}
	h, signer := app(t, store)
	bearer := token(t, signer, "viewer", "demo-viewer")
	rec := requestWith(t, h, http.MethodPost, "/v1/prevention/actions", bearer, createActionBody)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a viewer-only token reached the write route: got %d (%s), want 403", rec.Code, rec.Body.String())
	}
}
