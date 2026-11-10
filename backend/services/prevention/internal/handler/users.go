package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"firewatch.local/prevention/internal/directory"
)

// Directory is the user directory this transport reads. It is declared here, not
// returned by the directory client, so the transport depends on the operation of
// the contract and nothing else.
type Directory interface {
	// Users returns the realm's users. ErrNotConfigured means the directory cannot
	// be addressed at all; any other error means the identity provider failed.
	Users(ctx context.Context) ([]directory.User, error)
	// User reads one user by identifier, directly rather than out of the users
	// list. directory.ErrUserNotFound means the identity provider answered and
	// knows no such user; ErrNotConfigured means the directory cannot be addressed;
	// any other error is an upstream failure.
	User(ctx context.Context, id string) (directory.User, error)
}

// maxDirectoryUsers is the bound every list route of this service shares. The
// directory read asks the identity provider for the same number, so the route
// never truncates a page it did not bound.
const maxDirectoryUsers = 200

// userEntry is one item of the users list: the identifier the identity provider
// assigns and the name to present for it.
type userEntry struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// listUsers serves GET /v1/users: admin only.
//
// The three failure modes are deliberately distinct and none of them is an empty
// list. An unconfigured directory is this service's own misconfiguration (503
// directory_unavailable); an identity provider that answers an error or cannot be
// reached is an upstream failure (502 directory_upstream_failed). 200 [] is
// reserved for a realm that really has no users, so a caller can never mistake an
// outage for an empty directory.
func listUsers(dir Directory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAdmin(r.Context()) {
			fail(w, 403, "forbidden")
			return
		}
		// A route mounted without a directory is configured with none: it must fail
		// closed rather than answer an empty array.
		if dir == nil {
			slog.Error("directory_not_configured", "request_id", r.Header.Get("X-Request-ID"))
			fail(w, 503, "directory_unavailable")
			return
		}
		users, err := dir.Users(r.Context())
		if errors.Is(err, directory.ErrNotConfigured) {
			slog.Error("directory_not_configured", "request_id", r.Header.Get("X-Request-ID"))
			fail(w, 503, "directory_unavailable")
			return
		}
		if err != nil {
			// The error is deliberately not logged: it can carry a request URL, and
			// this is the one route whose dependency holds a credential. The
			// directory client already keeps the secret and the token out of it, and
			// the correlation id is enough to find the identity provider's log.
			slog.Error("directory_read_failed", "request_id", r.Header.Get("X-Request-ID"))
			fail(w, 502, "directory_upstream_failed")
			return
		}
		entries := make([]userEntry, 0, min(len(users), maxDirectoryUsers))
		for _, user := range users {
			display := user.DisplayName
			if display == "" {
				display = user.ID
			}
			// An entry with neither a name nor an identifier can be neither
			// presented nor addressed, and returning it would break the documented
			// "display_name is never empty" rule.
			if display == "" {
				continue
			}
			entries = append(entries, userEntry{ID: user.ID, DisplayName: display})
		}
		// Total order: display name, and the identifier breaks a tie. Without the
		// second key two users with the same name could swap places between calls.
		slices.SortFunc(entries, func(a, b userEntry) int {
			if order := strings.Compare(a.DisplayName, b.DisplayName); order != 0 {
				return order
			}
			return strings.Compare(a.ID, b.ID)
		})
		if len(entries) > maxDirectoryUsers {
			entries = entries[:maxDirectoryUsers]
		}
		respond(w, 200, entries)
	}
}
