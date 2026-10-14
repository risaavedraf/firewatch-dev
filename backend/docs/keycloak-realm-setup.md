# Keycloak realm setup for the separated frontend

The FireWatch page is served as its own application from `http://localhost:4173`, while the gateway stays an API on `http://localhost:8080`. Those are different origins, so the `firewatch-demo` client must accept the new origin for login, for the token exchange, and for RP-initiated logout.

## What the shipped realm declares

`backend/deploy/local/firewatch-realm.json` declares, for the public client `firewatch-demo`:

| Setting | Value |
| --- | --- |
| `redirectUris` | `http://localhost:8080/`, `http://localhost:4173/` |
| `webOrigins` | `http://localhost:8080`, `http://localhost:4173` |
| `attributes["post.logout.redirect.uris"]` | `http://localhost:8080/##http://localhost:4173/` |
| `attributes["pkce.code.challenge.method"]` | `S256` |
| `standardFlowEnabled` / `directAccessGrantsEnabled` | `true` / `true` |

Notes on the format, because both are easy to get wrong:

- `redirectUris` and `webOrigins` are JSON arrays of separate entries. Redirect URIs keep the trailing slash; web origins do not, because an origin has no path.
- `post.logout.redirect.uris` is **not** an array. Keycloak stores it inside `attributes` as one string, and multiple values are joined with `##`. Do not use a comma or a space.
- The `:8080` entries were deliberately retained rather than replaced. **Nothing in this repository requires them any more.** The page uses `:4173`, and the password-grant helper in `backend/scripts/smoke.py` performs a server-side token request that supplies no redirect URI at all, so it does not depend on them either. They are kept because narrowing an existing public client's configuration buys nothing here and would break any already-configured consumer that still points at `:8080`.

## Why editing the JSON is not enough on an existing realm

`backend/docker-compose.yml` starts Keycloak with `command: [start-dev, --import-realm]`. Keycloak import is **create-only**: if the realm `firewatch` already exists in the identity database, the file is read and ignored, and the running client keeps its previous URIs.

Consequences:

- On a **fresh** identity database, the JSON is authoritative and nothing else is needed.
- On an **already-initialized** realm, re-import does not apply the change. Use the manual steps below.

**Do not run `docker compose down -v` to force a re-import.** That deletes the identity volume and with it every local user, session and credential. The realm is persistent local state and must be updated in place instead.

## One-time manual steps on an existing realm

Keycloak listens on `http://localhost:8081` (`KC_HOSTNAME`, `backend/docker-compose.yml:70`).

1. Open `http://localhost:8081` and sign in to the Administration Console as `local-admin`. The password is the `KEYCLOAK_ADMIN_PASSWORD` you set in your local `.env`; the username comes from `KC_BOOTSTRAP_ADMIN_USERNAME`.
2. Select the realm **firewatch**.
3. Go to **Clients** and open **firewatch-demo**.
4. On **Valid redirect URIs**, add `http://localhost:4173/`. Keep `http://localhost:8080/`.
5. On **Web origins**, add `http://localhost:4173`. Keep `http://localhost:8080`.
6. On **Valid post logout redirect URIs**, add `http://localhost:4173/`. Keep `http://localhost:8080/`.
7. **Save**.

Steps 4-6 are additive. Do not replace existing entries, and do not change the client from public to confidential: the page uses Authorization Code with PKCE `S256` and holds no client secret.

## Verification

Open **Clients → firewatch-demo → Settings** and confirm all three fields show both the `:8080` and the `:4173` value. The observable end-to-end check is signing in from the page served at `http://localhost:4173` and then logging out; logout must return to `http://localhost:4173/`.

A programmatic check was **not executed** as part of this change: it needs a running Keycloak with the local `.env` credentials, which is outside what this repository can verify on its own. Treat the sign-in and logout round trip through the browser as the real acceptance check.

## Forward risk: HTTPS enforcement for client URIs

Keycloak 26.6.0 introduces the `secure-client-uris` client policy executor, which requires HTTPS and makes **existing clients using `http` in "Post logout redirect URIs" fail validation when they are updated**. This repository pins `quay.io/keycloak/keycloak:26.5.2` (`backend/docker-compose.yml:61`), so the behavior does not apply today.

Before bumping the Keycloak version above 26.6.0, expect local logout to break for the plain-`http` entries above, and plan for either keeping the executor disabled or moving local development to TLS.
