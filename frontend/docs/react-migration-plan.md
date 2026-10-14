# FireWatch frontend — React migration plan

**Recommendation: React + TypeScript + Vite, using the available Node/npm toolchain; ship static assets without a Node or Bun production server.**

**Status: planning documentation only. The migration and PWA proposals are not implemented or approved.** No dependency was installed, no source file was edited, no API was called, no commit was created, and no deployment surface was touched to produce this document. Every "proposed" item requires a separate decision before it becomes work.

Companion tracking document: [`odd/tasks/frontend/react-frontend-plan.md`](../../odd/tasks/frontend/react-frontend-plan.md) (specs, tasks, evidence log, in Spanish).

Superseded material: [`odd/tasks/frontend-react-vite-migration.md`](../../odd/tasks/frontend/frontend-react-vite-migration.md) planned the **retired mockup**. Its source evidence points at files that no longer exist (`frontend/src/core/router.js`, `frontend/src/main.js`, `frontend/tests/state.test.mjs`). That file is left untouched as history; this document is established from the **real current page**. Do not merge the two.

---

## 0. Scope and reader contract

| This document is | This document is not |
| --- | --- |
| An evidence-backed inventory of what the current page actually does | A claim that any of it was re-verified in a browser |
| A reasoned build-tool and runtime decision | An installation, a lockfile, or a version pin |
| A proposed React architecture and incremental migration path | Frontend implementation, or authorization to write any |
| A capability-parity and requirement/test matrix | A statement that checklist requirements are satisfied |
| A delimited PWA proposal | An authorization to add a service worker or a manifest |
| A list of proposed backend contract changes, labeled as proposals | An API contract change, or a claim that those payloads exist |

Rules applied throughout: a claim without `path:line` evidence is marked **unverified**; anything the current code does not do is marked **not implemented**; anything that would require a backend or deployment change is marked **PROPOSED** and needs explicit agreement with the backend session before it is frozen.

---

## 1. Verified current state (what must not be lost)

### 1.1 Runtime, serving and security surface

The page is one self-contained browser application with **no framework, no bundler, no dependencies and no build step** (`frontend/README.md`). `frontend/package.json:1-9` declares only three Node scripts — `serve`, `test`, `smoke` — and **no dependencies or devDependencies**.

The development server is the de-facto contract for how the page is served:

| Property | Value | Evidence |
| --- | --- | --- |
| Asset allowlist | exactly `index.html`, `proposal.css`, `proposal.js` | `frontend/scripts/serve.mjs:9-23` |
| Methods | `GET` and `HEAD` only; anything else `405` with `Allow: GET, HEAD` | `frontend/scripts/serve.mjs:25-38` |
| Directory listing / traversal | none; traversal rejected | `frontend/scripts/serve.mjs:25-38` |
| SPA fallback | **none** — unknown paths 404 | `frontend/scripts/serve.mjs:25-28` |
| Response headers | `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer` | `frontend/scripts/serve.mjs:48-50` |
| CSP | `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src http://localhost:8080 http://localhost:8081; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'` | `frontend/scripts/serve.mjs:48-50` |
| Binding | loopback only, no cross-host reach | `frontend/README.md` |

Three constants define every external call: `gatewayOrigin = http://localhost:8080`, `issuer = http://localhost:8081/realms/firewatch`, `redirectURI = http://localhost:4173/` (`frontend/proposal.js:16-24`). All gateway calls are absolute; `/token`, `/auth` and `/logout` resolve against the issuer, never the gateway. The origin spelling `localhost` is load-bearing: `127.0.0.1:4173` fails login and cross-origin calls (`frontend/README.md`).

**Migration consequence:** a bundler replaces the "no build step" property, but not the serving semantics. Any React build must reproduce — or explicitly and deliberately change — the allowlist behaviour, the absence of SPA fallback, `no-store`, `nosniff`, `no-referrer`, and the exact CSP. A Vite dev server does **not** reproduce them; only a real static server check can.

### 1.2 Identity and session rules

| Capability | Behaviour | Evidence |
| --- | --- | --- |
| Login | OIDC Authorization Code + PKCE `S256`: state and verifier generated, SHA-256 challenge, redirect to Keycloak | `frontend/proposal.js:559-621` |
| PKCE transaction storage | `sessionStorage`, keys for `state` + `verifier`, written once, read and removed at callback, removed at logout | `frontend/proposal.js:567`, `:592-593`, `:1267` |
| Callback | validates returned `state`; clears the query string while preserving pathname and hash | `frontend/proposal.js:621` |
| Tokens | access and ID tokens live **in page memory only**; no cookie, no `localStorage`; nothing is persisted | `frontend/proposal.js:493-558`, `frontend/index.html:64-65` |
| API authorization | `Authorization: Bearer`, `credentials: "omit"`, `cache: "no-store"` | `frontend/proposal.js:493-558` |
| `401` | resets the private view and the session state | `frontend/proposal.js:643-675` |
| Logout | Keycloak RP-Initiated Logout with the ID token as `id_token_hint` | `frontend/proposal.js:1260-1281` |

**Correction to the current README (verified during this investigation):** `frontend/README.md` states that "nothing is written to cookies, `localStorage` or `sessionStorage`". That is true for **tokens**, but the PKCE transaction (`state`, `verifier`) *is* written to `sessionStorage` and removed at callback and logout. The in-page Spanish note is the accurate one — "El token de acceso permanece solo en memoria mientras esta página está abierta. No se guardan tokens ni se envían cookies." (`frontend/index.html:65`). The React migration must preserve the accurate behaviour and any README rewrite must state it precisely; a security reviewer should not be handed a claim the code contradicts.

### 1.3 Consumed API surface and contract status

Nine routes are consumed. Every `/v1/...` route is consumed as an absolute URL on the gateway origin, and `/health/ready` is gateway-only — the issuer is never used for API calls.

| Method | Route | Evidence | In OpenAPI contract |
| --- | --- | --- | --- |
| GET | `/health/ready` | `frontend/proposal.js:1168` | **no** (unauthenticated probe) |
| GET | `/v1/risk/zones` | `frontend/proposal.js:455` | yes (`?bbox` optional, **page sends no bbox**; 0 occurrences of `bbox` in `proposal.js`) |
| GET | `/v1/risk/latest?zone_id={encoded}` | `frontend/proposal.js:1064` | yes |
| GET | `/v1/risk/history?zone_id={encoded}` | `frontend/proposal.js:1114` | yes |
| GET | `/v1/risk/sources` | `frontend/proposal.js:741` | yes |
| GET | `/v1/risk/status` | `frontend/proposal.js:844` | yes |
| GET | `/v1/jobs/status` | `frontend/proposal.js:845` | yes |
| POST | `/v1/jobs` — JSON `zone_id`, header `Idempotency-Key` | `frontend/proposal.js:1305-1309` | yes |
| GET | `/v1/jobs/{encoded-id}` | `frontend/proposal.js:1199` | yes |

Declared in the contract but **not consumed** by the page: `GET /v1/audit` (0 occurrences of `v1/audit` in `proposal.js`) and the optional `bbox` filter on zones. All contract paths require the global `bearerAuth` scheme (`backend/contracts/openapi/firewatch-v1.yaml:8-9,284`).

**Contract update received after the baseline inventory:** OpenAPI now declares `GET /v1/risk/synthetic-scenarios` and optional `scenario_id` on `POST /v1/jobs`. The current page consumes neither. Backend implementation is still in progress; contract declaration is not evidence of a working API (see §9).

### 1.4 Rendering and interaction capabilities

| Capability | Detail | Evidence |
| --- | --- | --- |
| Shell / UI | Spanish monitoring dashboard: overview, history, jobs hash links, gateway readiness card, session state, risk / map / history / source / pipeline / job panels, synthetic-data warnings, attribution | `frontend/index.html:13-95` |
| Zones catalogue | loads GeoJSON features, sorted selector, live feature count | `frontend/proposal.js:145-196` |
| Territorial map | schematic SVG paths, equirectangular/cosine projection, separate insets for geometries west of longitude −75 | `frontend/proposal.js:215-350`, `:429-489` |
| Selection | `<select>`, SVG click, Enter or Space; the selected outline is independent of risk validity | `frontend/proposal.js:253-278` |
| Map accessibility | SVG paths are buttons with `tabindex`, labels and `aria-pressed`; live regions; visible focus; reduced-motion honoured | `frontend/proposal.js:1332-1354`, `frontend/index.html:18-40`, `frontend/proposal.css:2-6,10-12` |
| Risk levels | `bajo`/`medio`/`alto`/`desconocido` → displayed as `BAJO`/`MEDIO`/`ALTO`/`INFORMACIÓN INSUFICIENTE` | `frontend/proposal.js:916-1043` |
| Fail-closed rendering | a valid display requires canonical level, boolean `demo`, valid factors, rationale and policy; otherwise the state is explicitly invalid | `frontend/proposal.js:1044-1155` |
| Insufficient data | `404` on latest means "no published batch", never low risk | `frontend/proposal.js:916-1043` |
| Freshness / validity | expiry driven by `published_at` / `valid_until`, expiring **1000 ms early**, timer delay capped at **3600000 ms** | `frontend/proposal.js:965-1000` |
| Source provenance | source panel requires HTTP 200, provenance fields and a matching integrity hash; malformed, `503` or mismatch degrades to unavailable | `frontend/proposal.js:707-751` |
| Operational readiness | pipeline state requires a fresh `checked_at`, ready state, consistent dependencies, known dead-letter depth within bound; fallback max age 30 s; refresh every 30 s with a 10 s deadline; `403` reads as "not authorized", not "down" | `frontend/proposal.js:757-914` |
| Jobs | admin creates a synthetic job with an idempotency key; publication is correlated across zone, job and batch via latest/history | `frontend/proposal.js:58`, `:1290-1329` |
| Polling | every 2 s, maximum 30 attempts, 65 s deadline, manual refresh available | `frontend/proposal.js:111-137`, `:1183-1234` |
| Race handling | per-request sequence numbers, session fences, abort controllers and deadlines discard superseded responses | `frontend/proposal.js:25-48`, `:625-700` |
| Explicit states | loading, forbidden, unauthorized, malformed, empty and HTTP-error states are all rendered | `frontend/proposal.js:625-700`, `:1044-1140` |
| Routing | `/`, `#overview`, `#history`, `#jobs`; callback query removed, hash retained; server has no SPA fallback | `frontend/index.html:16-21`, `frontend/proposal.js:621`, `frontend/scripts/serve.mjs:25-28` |
| Responsive | breakpoints at 1000 / 720 / 420 px collapse grids, sidebar and navigation; reduced-motion honoured | `frontend/proposal.css:3-6` |

**This is the definition of "capabilities to preserve".** Each row is either ported with parity evidence or explicitly dropped by decision — never silently lost.

### 1.5 Automated coverage, and what it does not establish

`frontend/tests/proposal-readback.test.cjs` runs `proposal.js` inside a `node:vm` against a stubbed `fetch` and a DOM harness, with no network access and no backend.

Observed during this investigation, first-hand:

```text
$ cd frontend && npm test
ℹ tests 40   ℹ pass 40   ℹ fail 0   ℹ skipped 0
```

**Correction to the current README:** `frontend/README.md` documents "37 behavioural tests". The runner reports **40** passing tests. The documented number is stale; the code is the authority (`frontend/tests/proposal-readback.test.cjs:45-668`).

Covered groups: job creation/correlation, polling deadlines and races (`:45-124`); map projection, catalogue count, click/keyboard selection, logout clearing (`:125-233`, `:320`, `:622-667`); risk validation, labels, history, synthetic warnings, validity timers (`:135-233`, `:328-413`); provenance integrity and pipeline freshness/dependency fail-closed behaviour (`:288-616`); absolute gateway URLs, issuer separation, gateway-only readiness (`:523-529`, `:668-683`).

**Not covered — and this must survive the migration as an honest gap:** real browser Keycloak login and logout, visual correctness, formal accessibility conformance, and real network integration. The suite is a VM harness, not a browser. `odd/urgent/README.md:14-16` records the same gap independently.

---

## 2. Build tool and runtime decision

### 2.1 The question, separated into three questions

The request was "react with vite or bun, whichever is faster and cheaper to keep running". Those options are not the same kind of thing, and conflating them is the main decision error available here:

| Layer | What it decides | Candidates |
| --- | --- | --- |
| **Build tool** | how `.tsx`/CSS become browser-ready assets, and how fast the edit loop is | Vite, Bun's bundler, webpack, esbuild, Parcel |
| **Package manager** | how dependencies are resolved, locked and installed | npm, Bun, pnpm, yarn |
| **Runtime that runs the app** | what executes the app in production | **the browser** — this is a static client-side bundle |

The third row is the decisive one for "economic to keep running": a React build is static files. The built client needs no Node/Bun application process. It still needs HTTP static hosting, whose configuration, headers, caching and cost depend on the chosen deployment; this plan does not measure those costs. Node-vs-Bun is a **build-machine and developer-loop** choice, not a production-runtime cost.

### 2.2 Local evidence measured before deciding

| Fact | Value | Source |
| --- | --- | --- |
| Node | `v24.11.1` | `node --version` |
| npm | `11.21.0` | `npm --version` |
| Bun, pnpm, yarn, Deno | **not found on PATH** | `command -v` for each |
| Vite | not installed | `command -v vite` |
| Installed dependencies | none — no `node_modules` at root or in `frontend/` | directory check |
| Registry reachability | `npm ping` → PONG in 232 ms | `npm ping` |
| Existing test runner | `node --test`, zero dependencies, 40 green | §1.5 |
| CI workflows present | `_go-service.yml`, `gateway.yml`, `integration.yml`, `jobs.yml`, `risk.yml` — **no frontend workflow exists** | `.github/workflows/` |

### 2.3 Decision

**Vite is the proposed build tool. npm is the proposed package manager. Bun is neither required nor assumed.**

Reasoning, stated as consequences rather than preferences:

1. **React + TypeScript needs a transform step.** JSX/TSX and CSS imports are not natively executable by a browser; the current page gets away with zero tooling only because it is hand-written ES modules. Something must bundle. The choice is *which*, not *whether*.
2. **Vite minimizes the maintenance surface.** It is a build tool with a small configuration surface and a fast dev loop, and its production output is plain static assets suitable for HTTP static hosting. The existing server cannot serve them unchanged: its fixed three-file allowlist must be adapted to the generated hashed assets under separately authorized implementation, preserving confinement and headers. That is the "cheap to keep running" property in this context: no additional application runtime to operate; an HTTP static server or hosting service remains necessary.
3. **Bun is not installed here.** Making Bun the reference path would make the project depend on a tool absent from the environment where the app is built today, while Node 24 and npm 11 are already present and already run the existing test suite.
4. **Bun's genuine advantages do not apply to this project's shape.** Bun is a runtime, a package manager, a bundler and a test runner. No Bun speed advantage was measured here. The reference currently has no dependency-heavy install, no server-side application runtime, and its existing 40-test suite completed in under a second; future React build performance remains unmeasured. There is no measured problem for Bun to solve.
5. **A "fastest" claim would need evidence I do not have.** No install, no build and no benchmark was run; the work was not authorized to install anything. Any tooling performance claim here would be invented, and this document does not make one.

### 2.4 Bun as an optional accelerator — and the conditions attached

Bun may be adopted later for local install/script speed **without** becoming a build requirement, but only after all of the following are observed rather than assumed:

- Bun is installed and its version recorded.
- `bun install` produces a dependency tree that resolves identically to the npm path (lockfile comparison, not an impression).
- `bun run build` produces assets byte-comparable in behaviour to the npm path — served and smoke-checked with the CSP, not merely "it opened".
- The dev-server contract in §2.5 is satisfied by Bun's dev server, or Bun's dev server is explicitly not used for verification.
- The dependency inventory obligation in §7 (`PRV-I3`) covers whichever package manager is introduced.

If those checks are not run, the honest answer is npm. Bun should be presented as an option the team may measure, never as a settled choice.

### 2.5 What the build tooling must reproduce or deliberately change

This section is the part most likely to be lost in a "just add Vite" migration. Each row is a real behavioural difference between the current server and a default Vite setup.

| Current behaviour | Vite default | Required decision |
| --- | --- | --- |
| No SPA fallback; unknown paths 404 | dev server serves `index.html` for unknown paths | **keeps or changes?** The app uses hash routes (`#overview`, `#history`, `#jobs`), so no fallback is needed today. Preserving "no fallback" is the conservative choice; adding one is a behaviour change to be approved, not a convenience to adopt silently. |
| Exactly three allowlisted assets | hashed asset filenames in a build output directory | the production server must serve the build directory, and the smoke check must assert the real asset set and MIME types. |
| CSP sent by the server; `script-src 'self'` with no `'unsafe-inline'` | no CSP headers; dev server may inject inline scripts/HMR | **the CSP is a real constraint on the target architecture.** A production build must be validated against the CSP; the dev server cannot be the CSP witness. |
| `style-src 'self' 'unsafe-inline'` | CSS is emitted as files or inlined | inlining styles is compatible with this policy, but the policy must be re-checked rather than assumed. |
| `connect-src` names exactly two origins | unchanged | the built app must still call exactly the gateway and the issuer; a new asset or telemetry origin would be refused by the browser. |
| `Cache-Control: no-store` on every asset | hashed, usually long-lived caching | **decision required:** hashed assets are normally cacheable. `no-store` was chosen for the current three files. This is a deliberate policy decision, not a bundler default to accept. |
| `Referrer-Policy: no-referrer`, `nosniff` | none | must be sent by the production server. |
| Environment configuration | `VITE_*` variables are **public and inlined into the bundle** | no secret may ever appear in a `VITE_*` variable or in bundled code. The three constants in §1.1 are public configuration and may be externalized; tokens and client secrets never may. |
| Base path | configurable `base` | the page is served from `/` on `http://localhost:4173/`. A deployed base path changes `redirectURI` and the Keycloak valid redirect URI, which is an identity-configuration change, not a bundler flag. |

**Not verified in this document:** whether any of these behaviours can be preserved without changing the server. That is P1–P4 implementation work with its own evidence, not a claim of this plan.

---

## 3. Proposed React architecture

Status: **proposed**. No file, directory or dependency is authorized by this section.

### 3.1 Guiding constraints

- The migration replaces **composition**, not product behaviour. Any behaviour change bundled into the migration is a defect, because parity can no longer be attributed.
- Domain logic stays framework-free and unit-testable without a DOM, mirroring the current structure where rules are separable from rendering.
- Two independent implementations coexist until parity is proven: the current page keeps being served (§5.2). This matters more than usual because **`master` has no commits and all content is untracked** (observed: `git log` fails, `git status` shows everything untracked) — there is no version-control safety net. Nothing may be moved or deleted to "make room" for the new implementation.

### 3.2 Proposed shape

```text
frontend/                       # UNCHANGED — the reference keeps working
  index.html  proposal.css  proposal.js  scripts/  tests/
frontend/react/                 # proposed candidate workspace (additive only)
  index.html
  package.json
  vite.config.ts
  tsconfig.json
  src/
    main.tsx
    app/            App.tsx, router.ts, SessionProvider.tsx, QueryProvider.tsx
    domain/         risk.ts, validity.ts, provenance.ts, readiness.ts, jobs.ts, types.ts
    data/           gateway.ts (transport), ports.ts, adapters/*.ts, config.ts
    auth/           pkce.ts, session.ts, storage.ts
    features/       map/ risk/ history/ sources/ pipeline/ jobs/
    shared/ui/      Shell.tsx, StatusFeedback.tsx, LiveRegion.tsx
    styles/         tokens.css, app.css
  tests/            domain/ adapters/ components/ (vm-equivalent) + e2e/
```

Mapping from the current single file, by responsibility rather than by line:

| Current responsibility | Proposed home | Parity obligation |
| --- | --- | --- |
| Constants, origins | `data/config.ts` | identical values and absolute-URL behaviour (`proposal.js:16-24`, `:493-558`) |
| Transport, sequence fences, abort, deadlines | `data/gateway.ts` | superseded responses still discarded (`:25-48`, `:625-700`) |
| PKCE + session lifecycle | `auth/*` | same storage keys and the same removal points (`:567`, `:592-593`, `:1267`) |
| Validity, freshness, provenance, readiness rules | `domain/*` | pure functions, ported with the 1000 ms early expiry and 3600000 ms cap (`:965-1000`) |
| Map projection and insets | `features/map/` | same projection output for the same GeoJSON, including the west-of-−75 insets (`:215-350`) |
| Panels and their explicit states | `features/*` | every loading/forbidden/unauthorized/malformed/empty/error state still reachable (`:625-700`, `:1044-1140`) |
| Polling and job correlation | `features/jobs/` | 2 s cadence, 30 attempts, 65 s deadline; reference generates a new key per creation (`:1308`). Reusing a key on transport retries is a proposed improvement, not existing parity (`:111-137`, `:1290-1329`) |
| Accessibility behaviour | `shared/ui/`, `features/map/` | button semantics, `aria-pressed`, live regions, focus visibility, reduced motion (`:1332-1354`, `proposal.css:2-6`) |

### 3.3 State, data and error policy

- **Server authorization stays authoritative.** The client never decides a permission; it renders what the server returns, including `403` ("not authorized, not unavailable") and `401` (session reset).
- **No silent fallback.** An API failure never substitutes fixture data. This is an existing property of the page and a requirement of the migration.
- **Fail-closed rendering is preserved verbatim in meaning:** invalid or unverifiable input renders as unavailable/insufficient, never as a risk level.
- **Synthetic data stays labelled.** `demo: true`, the synthetic warnings and the attribution remain visible in every re-render path.
- **Cancellation is a first-class concern,** not an afterthought: every request carries a sequence fence and an abort controller, and deadlines are explicit.

### 3.4 Testing strategy for the migration

1. **Port the 40 existing tests first, against the candidate.** A port that passes is parity evidence for the behaviours the suite pins. A port that fails is a real behavioural difference, not a test problem.
2. Add component-level tests only where the current DOM harness cannot express the behaviour (real event wiring, focus).
3. Add a browser-level suite for the gaps the VM harness cannot reach — above all the **real login/logout round trip**, which `frontend/README.md` and `odd/urgent/README.md:14-16` both record as never executed.
4. Keep the existing Node checks runnable throughout, and keep the smoke check asserting CSP, methods, MIME types and traversal confinement for whichever server is actually serving.

---

## 4. PWA proposal (delimited)

Status: **conditional proposal, not approved, not part of the migration.** The prior PWA recommendation was conditional and must stay conditional.

### 4.1 What is proposed

- A web app manifest plus icons, so the page can be installed to a home screen or desktop.
- An app-shell service worker caching **only** the built application assets (HTML/CSS/JS/icons).
- Offline behaviour limited to: the shell loads, and it states plainly that no risk data is available.

### 4.2 What is explicitly not proposed, and why

- **No offline risk level, ever.** Cached `risk_level`, `data_status` or freshness values would present a stale hazard assessment as current. This is a safety-relevant surface: the page's own fail-closed validity logic exists precisely to prevent that, and a cache that outlives `valid_until` would defeat it. Cached API responses for `/v1/*` are out of scope.
- **No background sync, no push, no periodic background fetch, no background refresh.** Browser support is uneven and the guarantees are not reliable. Nothing in the plan may promise "the data updates in the background".
- **No offline writes.** Job creation, submission or any state change while offline is out of scope.
- **No promise of freshness.** The offline view must not look like a live dashboard.

### 4.3 Cost, and the conditions attached

A service worker is not a free feature: it introduces an update/activate lifecycle, cache-version invalidation, a second origin-scoped security surface, and CSP directives that must be reviewed (`worker-src`, `manifest-src`, and whether the existing `default-src 'self'` already covers them). It also invalidates the simple `no-store` caching story in §2.5.

Conditions before this becomes work: P4 parity is complete; the caching policy is reviewed as a security decision by whoever owns the frontend security surface; the offline view is explicitly designed and tested to show no risk level; and installability is verified on a real browser rather than inferred. If those conditions are not met, dropping PWA entirely is the correct outcome.

---

## 5. Incremental migration

### 5.1 Phases

| Phase | Goal | Exit evidence | Gate before starting |
| --- | --- | --- | --- |
| **P0 — baseline capture** | freeze what "parity" means before writing the candidate | capability inventory from §1 accepted; the 40 tests recorded green with the exact command; screenshots and keyboard notes captured at the two viewports | separate implementation authorization; baseline capture is the recommended first future step |
| **P1 — candidate skeleton** | React + TS + Vite mounting in an isolated candidate workspace | root mounts; local assets resolve; build and type check pass; the reference still runs unchanged | decision on §2.5 items and explicit implementation authorization |
| **P2 — domain and transport parity** | port validity/freshness/provenance/readiness/jobs logic and the request layer | ported domain tests green; superseded-response and deadline behaviour demonstrated | P1 |
| **P3 — UI parity** | shell, map, panels, states, accessibility | every row of §1.4 reachable; keyboard and focus journeys; reduced motion | P2 |
| **P4 — parity gate** | prove equivalence, then decide | all 40 ported tests green; browser checks; explicit diff of behaviours that changed; reference still runnable | P3 |
| **P5 — real session and live API** | close the never-executed login/logout round trip against a running stack | recorded browser login/logout plus viewer/admin behaviour against the real gateway | requires a running stack (Docker Linux engine), which is currently unavailable |
| **P6 — optional PWA** | only if §4 conditions are met | installability and the no-risk-offline behaviour verified in a browser | P4 |

### 5.2 Cutover, rollback and the missing Git safety net

Because the repository has no commits and everything is untracked:

- The migration is **additive**: the candidate lives in a new directory; no existing file is moved, renamed or deleted to make space.
- "Rollback" means *stop serving the candidate and keep serving the unchanged reference*. That only works if the reference was never modified. It is not a hypothetical: without commits, `git diff` cannot prove a file was unchanged.
- Any eventual promotion of the candidate to the primary entry point, and any snapshot of the retired reference, is a **separate approval** with an explicit snapshot path and checksum manifest.

### 5.3 What the migration must not carry over

- `odd/tasks/frontend/frontend-react-vite-migration.md`'s eight-route / eight-view mockup model: it describes a product that no longer exists in `frontend/`. Claiming those eight routes as current parity targets would fabricate requirements.
- Any fixture adapter that swaps in data on API failure.
- Any client-side permission decision.

---

## 6. Capability parity matrix

`Parity evidence` names what must be observed before the capability counts as preserved. `Current evidence` is what exists today.

| # | Capability | Current evidence | Parity evidence required | Status |
| --- | --- | --- | --- | --- |
| C1 | OIDC PKCE login and callback validation | code only (`proposal.js:559-621`); real browser round trip **never executed** | browser login against the running stack, state validated, query cleared, hash kept | **partial — gap** |
| C2 | Logout with `id_token_hint` | code only (`:1260-1281`); real logout never executed | browser logout returns to `http://localhost:4173/`, panels cleared | **partial — gap** |
| C3 | Tokens in memory only; PKCE transaction in `sessionStorage`, removed | code (`:567`, `:592-593`, `:1267`) | no token in any storage; transaction absent after callback and logout | implemented in reference; parity pending |
| C4 | Absolute gateway URLs; issuer separation | test (`:523-529`, `:668-683`) | ported test green | implemented |
| C5 | Zones catalogue load, count, sorted selector | test (`:125-233`) | ported test green | implemented |
| C6 | Map projection incl. island insets | test (`:125-233`); visual not verified | ported projection test green + visual check | implemented; visual gap |
| C7 | Map selection by click and keyboard | test (`:320`) | ported test + keyboard journey | implemented |
| C8 | Risk label mapping incl. `desconocido` | test (`:135-233`) | ported test green | implemented |
| C9 | Fail-closed validity and `404` ≠ low risk | test (`:135-233`, `:328-413`) | ported test green | implemented |
| C10 | Freshness timers (1000 ms early expiry, 3600000 ms cap) | test (`:328-413`) | ported test green | implemented |
| C11 | Provenance integrity incl. hash mismatch and `503` | test (`:288-616`) | ported test green | implemented |
| C12 | Readiness freshness, dependency consistency, `403` semantics | test (`:288-616`) | ported test green | implemented |
| C13 | Hash routes `/`, `#overview`, `#history`, `#jobs` | code (`index.html:16-21`) | candidate routes refresh and navigate identically | implemented |
| C14 | Superseded-response and logout races | test (`:622-667`) | ported test green | implemented |
| C15 | Job creation + idempotency key + polling bounds | test (`:45-124`) | ported test green | implemented |
| C16 | Explicit loading/forbidden/unauthorized/malformed/empty/error states | partial in tests | each state reachable in the candidate | partial |
| C17 | Accessibility: button semantics, `aria-pressed`, live regions, focus, reduced motion | code + CSS only; **no formal audit** | keyboard and focus journeys; reduced-motion check | **partial — gap** |
| C18 | Responsive behaviour at 1000/720/420 px | CSS only; no visual verification | screenshots at target viewports; no page-wide overflow | **partial — gap** |
| C19 | Serving contract: allowlist, methods, no SPA fallback, CSP, headers | smoke test (`scripts/smoke.mjs`) | the serving smoke check re-asserted for the real server | implemented for reference only |
| C20 | No silent fixture fallback | code and tests | ported test green | implemented |
| C21 | PWA installability / offline shell | **does not exist** (`no manifest, no service worker`) | nothing — proposed only | not implemented |

---

## 7. Requirements → frontend contribution matrix

Source of truth: `Referencias/tareas_verificacion_implementacion_IA.md`. Unit ownership follows `odd/urgent/README.md`. Status values: **contributes evidence** (the frontend can produce part of the evidence), **consumes** (the frontend only displays a result owned elsewhere), **not applicable to frontend**, **pending**.

The frontend cannot close any of these alone. Nothing below is claimed as satisfied, and the backend session owns the criteria that require measured performance, drift statistics, ground truth or signatures.

| ID | Requirement (verbatim, short) | Unit | Frontend relationship | Status |
| --- | --- | --- | --- | --- |
| `ZT-I2` | "Configurar autenticación para API, base de datos (PostGIS), Prometheus y Grafana" | U1 | consumes: the page is a client of the authenticated API and performs PKCE | pending (backend-owned) |
| `ZT-V1` | "Revisar que no existan accesos implícitos (servicios sin autenticación, credenciales compartidas)" | U1 | contributes evidence: no client secret exists in the bundle; the public client is documented | pending |
| `OBS-I1` | "Configurar alertas sobre distribución de entradas y confianza del modelo" | U1 | not applicable to frontend | pending (backend-owned) |
| `OBS-V1` | "Simular una desviación repentina y confirmar que la alerta se dispara" | U1 | not applicable to frontend | pending (backend-owned) |
| `DAT-I1`…`DAT-V5` | data schema, ranges, domain rules | U2 | not applicable to frontend (the page renders, it does not validate inputs into the model) | pending (backend-owned) |
| `SES-I1`…`SES-V3`, `LEA-*` | bias and leakage | U2 | not assessed by a frontend migration | pending |
| `PRV-I1/I2`, `PRI-*`, `ENT-*` | provenance, privacy, adversarial training | U1 | server/data/model governance; outside this deliverable | pending |
| `PRV-I3` | "Mantener un inventario de dependencias y modelos pre-entrenados de terceros" | U1 | contributes evidence: the dependency inventory of the built frontend | pending |
| `MET-I1/I2`, `MET-V1/V2`, `ROB-*`, `REG-*` | metamorphic, robustness, model regression | U3 | not applicable to frontend | pending (backend-owned) |
| `GRD-I1` | "Implementar validación de entrada (formato, rangos, contenido malicioso)" | U1 | contributes evidence: form-level validation and rendering of hostile input as text, not markup | pending |
| `GRD-V1` | "Probar intentos de manipulación de entrada y confirmar que se bloquean" | U1 | contributes evidence: an injection payload renders as literal text in forms, panels and messages | pending |
| `DRF-I1`…`DRF-V2` | drift tooling, statistics and logging | U4 | consumes: drift results are backend/model evidence; the page does not compute drift | pending (backend-owned) |
| `LOG-I1/I2/I3`, `LOG-V1` | log inputs, outputs, ground truth | U4 | not applicable to frontend (server-side logging) | pending |
| `MET-I3` | "Exponer en `/metrics`…" (demand, latency, resources, model/data) | U4 | not applicable to frontend (server metrics) | pending |
| `MET-I4` | "Priorizar CPU, RAM, latencia, RPS, errores y acceso a datos (PostGIS)" | U4 | not applicable to frontend | pending |
| `MET-V3/V4/V5` | interpret load vs latency, dashboard must raise questions | U4 | consumes: the page's own readiness and freshness display is a different surface from `/metrics` | pending |
| `ALR-I1/I2`, `ALR-V1/V2/V3` | alert thresholds, response actions | U4 | not applicable to frontend | pending |
| `STK-I1` | "Completar la tabla sin inventar herramientas que no se usarán" | U5 | contributes evidence: the frontend row of the stack table (this document §2) | pending |
| `STK-I2` | architecture against the five AWS Well-Architected ML Lens pillars | U5 | contributes partial evidence: cost and operational burden of the frontend tooling choice | pending |
| `STK-V1` | "Confirmar que cada herramienta elegida cubre al menos una prueba definida en las secciones 4 a 6" | U5 | consumes | pending |
| `CT-I1`…`CT-I4`, `CT-V1` | continuous testing triggers and automated stages | U5 | contributes evidence: a frontend check that runs on change — **note there is no frontend CI workflow today** | pending |
| `GNG-I1/I2`, `GNG-V1`…`GNG-V5` | version history, test matrix, approvals and signature | U5 | consumes | pending |
| `ACT-I1/I2/I3`, `ACT-V1/V2` | Docker Compose Prometheus/Grafana exercise | U5 | not applicable to frontend | pending |
| `SLA-I1`…`SLA-V3`, `ESC-*`, `OPT-*`, `ORQ-*` | load, scaling, hardware, orchestration | U4 | not applicable to frontend | pending (backend-owned) |

**Honest reading of this matrix:** the frontend contributes evidence to a small number of IDs — `ZT-V1`, `GRD-I1`, `GRD-V1`, `PRV-I3`, `STK-I1`, `STK-I2`, `CT-*` — and consumes or does not affect the rest. Presenting the migration as progress on the 132-ID checklist would be false. Its real value is that the demo page keeps working, stays honest about synthetic data, and stops being a single 65 KB file.

---

## 8. Limits, unsupported claims and open decisions

### 8.1 Claims this document does not make

- It does not claim any checklist requirement is satisfied.
- It does not claim the page was verified in a browser, that login works end to end, that the layout is visually correct, or that it is accessible to any conformance level.
- It does not claim Vite, React or Bun is installed, approved, version-pinned, or faster. No install, build or benchmark was run.
- It distinguishes the newly declared scenario contract from runtime availability: jobs/risk integration remains in progress and no live scenario flow was verified.
- It does not claim the backend is reachable: the Docker Linux engine is unavailable, so no live stack was exercised.
- It does not claim the current README is accurate on test count (37 vs 40 observed) or on `sessionStorage`.

### 8.2 Open decisions requiring a human or backend agreement

| # | Decision | Why it is blocked | Who decides |
| --- | --- | --- | --- |
| D1 | Accept or reject the Vite + npm tooling decision (§2.3) | it sets the shape of all later work | product/team |
| D2 | Keep "no SPA fallback", or introduce one (§2.5) | changes serving behaviour and the smoke contract | team |
| D3 | Keep `Cache-Control: no-store`, or move to hashed long-lived caching (§2.5) | a real caching-policy change | team |
| D4 | PWA: proceed, defer, or drop (§4) | installability and offline policy are product decisions with a safety dimension | product |
| D5 | Where the candidate lives and when it may be promoted | no Git baseline exists; promotion needs an explicit snapshot | team |
| D6 | Backend readiness for the coordinated scenario contract | payload is now declared; jobs/risk runtime integration and end-to-end evidence remain pending | backend session + team |
| D7 | Whether a frontend CI workflow is added | there is none today; affects `CT-*` evidence | team |
| D8 | Scope of the real login/logout acceptance check and who runs it | needs a running stack | team |

### 8.3 Risks

| Risk | Severity | Mitigation in this plan |
| --- | --- | --- |
| **No Git baseline** — everything untracked; a bad edit is unrecoverable | high | additive candidate workspace; never move or delete the reference; explicit snapshot before any promotion |
| Behaviour silently lost during the rewrite (fail-closed semantics, race handling, timers) | high | capability parity matrix (§6) with required evidence per row; port the 40 tests **before** rewriting |
| Security regression from serving changes (CSP, fallback, caching, origin) | high | §2.5 decisions are explicit and gated; the serving smoke check is re-asserted for the real server |
| Bundled secrets via `VITE_*` | high | stated rule: public configuration only; no secret in the bundle |
| Trusting the stale superseded plan | medium | §1.5 and the header record the supersession and the dead evidence links |
| Invented tooling performance claims | medium | §2.3 declines to make them; §2.4 lists the measurements required first |
| Plan drifting into implementation without approval | medium | §0 scope table, Phase gates in §5.1, and the delivery boundary in the tracking document |
| Offline/PWA showing stale risk as current | high if PWA is built | §4.2 forbids cached API responses and offline risk levels outright |

---

## 9. Coordinated scenario contract — declared, runtime integration pending

Historical update: optional scenario v1 event fields and enum were reported implemented with passing contract checks before OpenAPI coordination. Latest update: OpenAPI/catalogue contract is now coordinated; jobs/risk runtime integration remains pending. This is not a live scenario API. At the earlier inventory snapshot, scenario selection was **not** connected to the server: the live server still uses the original fixed fixture, there is no API or UI scenario selector, and no scenario persistence. The six synthetic profiles in `backend/services/risk/internal/source/synthetic_scenarios.go` exist as a provider for tests and future integration.

The following table preserves the earlier proposal snapshot, superseded by the latest coordinated-contract bullets below; it is not the current contract-status table. internal event fields may already exist as reported above. The current page consumes the nine routes in §1.3 and nothing more. These items are recorded here only so that the frontend design and the backend design do not diverge — they are the backend session's to accept, modify or reject.

| # | Proposal | Frontend implication if accepted | Status |
| --- | --- | --- | --- |
| P1 | Optional `scenario_id` on job creation | a selector appears in the jobs panel; the value must be sent only when one is chosen | PROPOSED — needs agreement |
| P2 | Server-assigned `evaluated_at`, persisted once with the job | the UI displays a server-assigned instant instead of a client-side guess | PROPOSED |
| P3 | Nullable scenario metadata on jobs, batches, risk and audit records, without backfill | new null fields must render as "not applicable", never as an error | PROPOSED |
| P4 | `GET /v1/risk/synthetic-scenarios`, viewer/admin, array of `{id, description}` | fetch catalogue instead of hardcoding profiles; validate response | Declared in OpenAPI; runtime integration not verified |
| P5 | Compatibility rule: scenario-free requests keep the existing fixture | no UI change; preserves the current demo path | PROPOSED |
| P6 | All local services updated together (already stated by the backend session) | the frontend contract gate must re-check the OpenAPI document at that moment | PROPOSED |

### Latest coordinated contract (source: updated OpenAPI and peer message)

- `POST /v1/jobs` remains admin-only with `Idempotency-Key`; body is `{zone_id, scenario_id?}`. Never send client `evaluated_at`: the server assigns it once.
- `Job` and `Batch` metadata `scenario_id` / `evaluated_at` may be absent or null for legacy records; no backfill is promised.
- Same actor/key returns the original job, scenario and instant even when a retry body differs. Use a new key for a genuinely new scenario request; retain the key for retries.
- Scenario-free requests preserve the fixture.
- `partial-retrieval` intentionally produces no batch. Existing job handling can leave it `queued`; do not invent a terminal failure state or report successful publication. Show bounded polling expiry as unconfirmed completion, not low risk.
- Catalogue and metadata are now contract-declared, but backend jobs/risk implementation is still in progress. The table above records earlier proposals except P4; the latest bullets supersede their undecided payload wording.

Frontend implementation is not authorized by this documentation. A future selector may target this coordinated contract only after backend readiness and integration checks; no fixture may masquerade as its API.

---

## 10. Verification policy and evidence log

This is passive planning documentation: meaningful behavioral RED/GREEN does not apply. The existing frontend suite was run once to resolve the test-count discrepancy: exit 0, 40 passing, no failures or skips. No React build was run because no candidate or dependencies exist. Verification instead means:

1. Re-reading this document against the current `frontend/` sources — the code it cites — checking that every `path:line` reference still resolves to what the text says.
2. Confirming the three documented corrections (§1.2, §1.5, §2.2) are stated as observations with the command that produced them, not as assumptions.
3. Confirming no claim asserts an installed tool, a verified browser behaviour, or a satisfied checklist requirement.
4. An independent read-only verifier pass over both deliverables, focused on invented capabilities and mislabeled contracts.

Commands used for the observations in this document:

```text
node --version; npm --version; command -v bun pnpm deno yarn vite
npm config get registry; npm ping
cd frontend && npm test
grep -n "sessionStorage\|localStorage\|cookie" frontend/proposal.js frontend/index.html
grep -c "v1/audit" frontend/proposal.js            # 0
git log --oneline; git status --porcelain
```

No install, build, network call to the application, file edit outside the two authorized paths, or commit was performed.
