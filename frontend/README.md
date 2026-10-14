# FireWatch frontend

The FireWatch web page: one self-contained browser application, served on its own origin and talking to the backend API on another. No framework, no bundler, no dependencies and no build step.

This directory holds the **real page**. It previously held a separate local mockup, which was retired; see [Retired mockup](#retired-mockup) below.

## Layout

| File | Responsibility |
| --- | --- |
| `index.html` | Page structure and all element IDs the script binds to |
| `proposal.css` | Styles |
| `proposal.js` | The whole application: OIDC PKCE login, API reads, the SVG territorial map, polling |
| `scripts/serve.mjs` | Development server with an explicit three-asset allowlist |
| `scripts/smoke.mjs` | Loopback HTTP check of the development server |
| `tests/proposal-readback.test.cjs` | Behaviour suite: runs `proposal.js` in a `node:vm` against a stubbed `fetch` |
| `package.json` | `serve`, `test` and `smoke` conveniences |

## Running it

The page needs the **backend running**: the gateway API on `http://localhost:8080` and Keycloak on `http://localhost:8081`. See [`backend/README.md`](../backend/README.md#quick-start).

```sh
node frontend/scripts/serve.mjs
```

Then open **`http://localhost:4173`**.

> **Open it as `localhost`, never as `127.0.0.1`.** Both the Keycloak redirect URI and the gateway's allowed origin are registered for the `localhost` spelling. To a browser those are two different origins, so `http://127.0.0.1:4173` will fail login and cross-origin requests even though the server answers. The server binds loopback only; it is not reachable from another host.

`npm run serve`, `npm test` and `npm run smoke` are equivalent conveniences from this directory.

## Where it points

The page is served from its own origin and calls two external origins directly. Three constants in `proposal.js` define that:

| Constant | Value | Purpose |
| --- | --- | --- |
| `gatewayOrigin` | `http://localhost:8080` | Prefixes every gateway call. All nine consumed routes are absolute |
| `issuer` | `http://localhost:8081/realms/firewatch` | Identity provider. `/token`, `/auth` and `/logout` resolve against this, never against the gateway |
| `redirectURI` | `http://localhost:4173/` | Must match the realm client's valid redirect URI |

Login uses the public `firewatch-demo` client with Authorization Code and PKCE `S256`. Access and ID tokens live in page memory only and are never written to cookies or to `localStorage`. Exactly one non-token value does touch browser storage: the PKCE transaction — the `state` and the `code_verifier` — is written to `sessionStorage` under `firewatch.pkce.transaction` for the redirect round trip, and removed both when the callback consumes it and again on logout. No token, and nothing else, is persisted there.

`WEB_ORIGIN` on the gateway must be exactly `http://localhost:4173`, or the browser's cross-origin calls are refused. The gateway matches that origin exactly, sends no CORS headers at all when it is unset, and never uses a wildcard. `docker-compose.yml` already sets it for the local demo.

## Content Security Policy

`scripts/serve.mjs` sends this policy on every asset:

```text
default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:;
connect-src http://localhost:8080 http://localhost:8081; object-src 'none'; base-uri 'none';
form-action 'none'; frame-ancestors 'none'
```

Read it as written, not as stricter than it is:

- `script-src` allows only the page's own scripts, with **no** `'unsafe-inline'`.
- `style-src` **does** allow `'unsafe-inline'`, so inline `style` attributes and `<style>` elements are permitted.
- `img-src` allows the page's own images **and** `data:` URIs.
- `connect-src` allows exactly two origins, the gateway and the identity provider. No wildcard, and neither is reachable for anything else.
- `default-src 'self'` covers every directive not named above.

The server also answers only `GET` and `HEAD`, from an explicit allowlist of three files, with no directory listing and no SPA fallback, and sends `Referrer-Policy: no-referrer`. These headers come from the development server: **a real deployment must send an equivalent policy**, because the page was built against this one. A deployment that tightens `style-src` or `img-src` must re-check rendering rather than assume the page avoids inline styles or `data:` images.

## Tests

```sh
npm test     # 40 behavioural tests
npm run smoke
```

The behaviour suite runs the page in a `node:vm` with a stubbed `fetch`, so it performs **no network access** and needs no backend. It covers the map projection against the real GeoJSON catalogue in `../backend/contracts/gen/geo/`, risk level mapping, staleness and fail-closed provenance handling, logout cleanup, superseded-response races, and that every consumed gateway path is absolute while `/token` still resolves against the identity provider.

The smoke check starts an ephemeral loopback server and closes it in `finally`. It asserts the three assets, their MIME types, the CSP, `HEAD`, `405` for writes, `404` for the retired mockup paths, and path-traversal confinement.

## What is not verified

Automated checks do **not** establish visual correctness or accessibility, and they do not exercise the real login path. The cross-origin round trip — signing in from `http://localhost:4173` through Keycloak and logging out back to it — has **not** been executed end to end in a browser. Treat that round trip as the real acceptance check, and see [`backend/docs/keycloak-realm-setup.md`](../backend/docs/keycloak-realm-setup.md) for the realm settings it depends on.

## Retired mockup

This directory used to hold a separate Spanish-language local mockup with six views, its own reducer and its own tests. It was never served by the gateway, called no backend, and was superseded by this page. It was retired along with its views, state module, fixtures, styles and scripts.

A byte-identical copy is kept outside version control, relative to the repository root, at `tmp/gateway-frontend-separation-1791858059/frontend/`, with a `MANIFEST.md` of checksums. The history of the retirement is in [`odd/tasks/architecture/gateway-frontend-separation.md`](../odd/tasks/architecture/gateway-frontend-separation.md).
