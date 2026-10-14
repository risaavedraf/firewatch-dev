# Client Proposal Page

## Objective
Create one polished Spanish, product-like FireWatch dashboard served by the existing gateway, with controls and views backed by the implemented API; use prior prototypes only as visual references.

## Problem and why
The earlier page was an editorial proposal/API panel rather than a realistic system interface. The user clarified that the prototypes in `web/prototipos/` are visual references only; the deliverable must be a single interface that consumes the real API, not links or routes displaying the prototype files.

## Scope and constraints
- Keep the gateway as host; use the prior prototypes only as design references. Do not expose them as selectable pages/routes or show their static mock values.
- Build a cohesive dashboard view whose data and actions come only from existing endpoints: readiness, risk latest/history, and job submission/status where supported.
- Preserve gateway authentication and authorization. Keep user-provided access tokens transient in page memory only; do not persist them or send cookies.
- Add hosted Keycloak Authorization Code + PKCE login (S256) for the browser; never collect user passwords in FireWatch. Preserve the existing local-only password grant for the PowerShell demo unless a separate decision changes it.
- Logout must clear the dashboard's in-memory tokens and end the Keycloak SSO session using OIDC RP-Initiated Logout; do not persist the ID token used as `id_token_hint`.
- Do not destroy or overwrite the persistent local Keycloak realm. Document any one-time client settings needed in an already-initialized realm.
- Label backend risk fixtures as synthetic/not evaluated; do not invent operational metrics, model predictions, or actions that the API does not return.
- Do not read secrets or change ignored credentials.
- `git rev-parse --show-toplevel` failed in the initial workspace check. Native review preflight currently treats the candidate as an empty-base workspace with an untracked inventory and requires an intended-file selection before START.

## Tasks
- [ ] **CP-1 — Build an API-backed FireWatch dashboard** (reopened after user correction; route: delegated direct; trigger: multi-file UI/API/auth implementation; TDD: focused gateway and API-adapter tests RED → GREEN where applicable).
  - Acceptance: root renders one coherent operational dashboard inspired by (but not linking to/rendering) the existing prototypes; real API responses populate supported risk/job views; authentication remains enforced; token is volatile and cleared on logout/401; all synthetic/not-evaluated values are visibly identified; unsupported dashboard metrics/actions are not fabricated.
  - Checks: inspect current API contracts; test root assets and unchanged API auth; validate JavaScript syntax; run `go test ./services/gateway/...`; browser check only if an existing local browser is available.
- [ ] **CP-2 — Add Keycloak browser login with PKCE** (authorized by user; route: delegated direct; trigger: auth UI, callback/token flow, CSP and realm configuration changes; TDD: focused deterministic auth-state/config checks RED → GREEN where feasible).
  - Acceptance: dashboard redirects to Keycloak-hosted login, validates OAuth state and uses PKCE S256; callback exchanges the code with the public `firewatch-demo` client; logout clears memory and redirects through Keycloak's logout endpoint using the volatile ID token; no password, access token, refresh token, or ID token is persisted; API authorization remains server-enforced; existing persistent realms are not reset or overwritten.
  - Checks: Go gateway tests and JavaScript syntax/state checks where a local runner is available; inspect exact login/logout redirect, CSP, and client settings. Live sign-in/out requires the initialized Keycloak client's standard flow, exact login redirect URI, web origin, PKCE S256, and valid post-logout redirect URI; provide safe one-time Admin Console instructions for an existing realm.

## Delivery strategy
`ask-on-risk` (default); no PR or remote operation is authorized.

## Progress and evidence
- Exploration found no existing frontend; backend has authenticated jobs/risk/audit API routes and synthetic risk fixtures. See `backend/README.md` and `backend/contracts/openapi/firewatch-v1.yaml`.
- RDD mode status observed: on (global). Selectorless STATUS returned `collect` for `intended_untracked_selection`; its candidate inventory includes workspace files, and the runtime offers no capture interface for that external selection. No START/review was run. The current Git root resolves, but `git status --short` reports all workspace paths as untracked; no work-unit commit exists.
- Previous implementation: gateway served an editorial page plus a live readiness/risk panel. Go and Node tests passed, but this did not satisfy the user's presentation intent.
- A worker briefly added prototype routes and links; a follow-up implementation removed the routes/links and narrowed embedded assets to the actual dashboard files. Prototype HTML remains unserved and unchanged.
- User correction: the page is now one product-style dashboard, using the prototypes only as design references. It calls public readiness and protected latest/history/job endpoints; risk outputs remain synthetic/not evaluated.
- Verification: writer and parent ran `go test ./services/gateway/...` successfully; writer ran `node --check services/gateway/internal/handler/web/proposal.js` successfully. No browser rendering or live request with a valid access token was exercised.
- Native review remains unresolved: previous selectorless STATUS requires `intended_untracked_selection`; no START/review or approval was performed. The user’s approval of the nine implementation paths did not complete this separate native review collection.
- User selected hosted Keycloak login with Authorization Code + PKCE. Safe mapping found `firewatch-demo` is public, has direct grants enabled, standard flow disabled, and an audience mapper for `firewatch-api`; Compose persists Keycloak state in the PostgreSQL volume. Do not reset/reimport that volume to apply client changes.
- CP-2 implementation: dashboard now redirects to Keycloak using state and PKCE S256, exchanges the callback code, and keeps the access token in memory only. Fresh realm-import config enables the exact loopback callback/client settings; `backend/README.md` documents how to apply those settings to an existing realm without reset/reimport.
- Logout follow-up: added OIDC RP-Initiated Logout; the dashboard now clears access/ID tokens and private data, then redirects to Keycloak using `id_token_hint` and the registered post-logout URI. Fresh realm config and README include that URI; the existing persistent realm needs the Admin Console setting.
- Verification: PKCE and logout handler tests observed RED → GREEN; `go test ./services/gateway/...` passed in writer and parent spot check; `node --check services/gateway/internal/handler/web/proposal.js` passed. No live browser/Keycloak sign-in or sign-out was run; the persistent client still needs its post-logout redirect setting.

## Next step
Apply the documented non-destructive login and post-logout redirect settings to the persistent Keycloak realm, rebuild the gateway and run a live sign-in/out/API check if available; resolve native intended-untracked selection before claiming review/ODD closure.
