# FireWatch frontend migration plan

Migrate the local product reference to React + TypeScript + Vite without changing its visual identity or claiming new backend capabilities. **This document is the only authorized deliverable.** All implementation tasks below are pending proposals; no dependencies, source changes, moves, deployment or Git operations are authorized by this plan.

> **Superseded in part.** This plan migrates the local *mockup* that used to live in `frontend/`. That mockup was **retired** by `odd/tasks/architecture/gateway-frontend-separation.md`: the real page now occupies `frontend/`, and the mockup's `src/**`, `styles/**`, `tests/state.test.mjs` and `scripts/check.mjs` no longer exist. One nuance: `frontend/scripts/smoke.mjs` does exist again, but it was rewritten to check the current page, not the mockup, so it is not the file this plan refers to. The source-evidence links below to `frontend/src/core/router.js`, `frontend/src/main.js` and `frontend/tests/state.test.mjs` are dead. A byte-identical copy of the retired mockup is kept at `tmp/gateway-frontend-separation-1791858059/frontend/`. This note records the supersession without rewriting the plan's history; whether a React migration is still wanted for the current page is a separate, undecided question.

## Quick path

1. Start with FW-R01: capture the existing behavioral and visual baseline and obtain implementation scope approval.
2. Complete phase A against fixtures, retaining the original reference until parity is verified.
3. Start phase B only after phase A parity and API/auth/deployment prerequisites are accepted. Real API connectivity still produces synthetic, unevaluated results.

## Objective and non-goals

Preserve eight Spanish-language views, responsive layout, local CSS tokens, keyboard behavior, scenario boundaries and deterministic domain rules while replacing manual DOM composition with typed React components.

Not included: native mobile, offline persistence, backend/gateway rewrites, real inference, ingestion/training/scheduling, real source/model governance, preventive-operation persistence, geospatial providers, or automatic commits/PRs. Future product features remain labeled fixtures or explicit unsupported states, never invented API endpoints.

## Authority and proposed decisions

| Status | Boundary or recommendation |
| --- | --- |
| Approved now | Write this plan in `odd/tasks/frontend-react-vite-migration.md` only; preserve every existing file. |
| Existing baseline | Dependency-free vanilla modules in `frontend/`; pure reducer/selectors, in-memory scenarios and roles; gateway UI remains independent and unchanged. |
| Proposed phase A workspace | Build an isolated candidate in `frontend/react/`; keep the existing `frontend/index.html`, modules, scripts and tests runnable. This location requires future implementation authorization. |
| Proposed framework | React client root, TypeScript strict checking, Vite React tooling; select compatible supported versions and lockfile during FW-R02, not from this document. |
| Proposed state/routing | Pragmatic feature slices; pure domain plus reducer/context initially. React Router with hash navigation preserves `#overview` and the other seven links; add no heavier store without measured need. |
| Proposed verification tools | Vitest + Testing Library/user-event for domain and component interactions; Playwright for browser journeys. These are future dependencies, not installed tools. |
| Deferred reference/cutover choice | Keep the original reference in place through parity. If the candidate later becomes the primary frontend, separately approve an exact reference snapshot such as `frontend/reference/` before any move/delete. No snapshot or cutover is authorized now. |

## Baseline and traceability

Source evidence: [frontend README](../../../frontend/README.md), [package scripts](../../../frontend/package.json), [completed mockup plan](frontend-product-mockup.md), `router` (`../../../frontend/src/core/router.js`), `DOM composition` (`../../../frontend/src/main.js`), `domain tests` (`../../../frontend/tests/state.test.mjs`), [API contract](../../../backend/contracts/openapi/firewatch-v1.yaml), and [backend browser login](../../../backend/README.md).

The prior mockup plan records 25 files, eight routes, 13 passing domain tests, 20 module syntax checks, 24 rendered combinations, loopback HTTP smoke and nine focused browser checks. These are **previously recorded results**, not checks rerun for this planning task. Full visual and exhaustive keyboard audits were not established by that evidence.

### Suggested target tree — not implemented

```text
frontend/react/
  index.html
  package.json                 # isolated candidate scripts/dependencies
  vite.config.ts
  tsconfig.json
  src/
    main.tsx
    app/{App.tsx,router.tsx,DemoProvider.tsx}
    domain/{types.ts,reducer.ts,selectors.ts,route-policy.ts}
    data/{fixtures.ts,ports.ts,fixture-adapter.ts,api-adapter.ts}
    features/
      overview/ history/ sources/ models/
      jobs/ prevention/ audit/ access/
    shared/ui/{Shell.tsx,TerrainMap.tsx,StatusFeedback.tsx}
    shared/styles/{tokens.css,app.css}
    auth/                      # phase B only: OIDC session boundary
  tests/{domain/,components/,adapters/,e2e/}
```

Each feature owns its route component and interaction tests. Shared UI owns presentation, not product transitions; domain imports neither React nor browser APIs. Data adapters translate contracts into presentation models rather than exposing transport objects to every screen.

### Source → suggested target

| Existing source under `frontend/` | Candidate under `frontend/react/` | Migration responsibility |
| --- | --- | --- |
| `index.html`, `src/main.js` | `index.html`, `src/main.tsx`, `src/app/` | React root, controlled forms, effects/cleanup, route focus and announcements; remove candidate event delegation, not the reference. |
| `styles/tokens.css`, `styles/app.css` | `src/shared/styles/` | Preserve colors, spacing, typography, breakpoints, scrollable tables and focus styles before any redesign. |
| `src/core/state.js`, `selectors.js` | `src/domain/` | Reuse pure rules; typed state/actions and deterministic clock. No behavior rewrite bundled with framework migration. |
| `src/core/router.js` | `src/app/router.tsx`, `src/domain/route-policy.ts` | Eight hash routes, own-route allowlist, viewer redirects and role-change normalization. |
| `src/data/fixtures.js` | `src/data/fixtures.ts`, `fixture-adapter.ts` | Fresh resettable fixtures, frozen clock, no storage or external requests. |
| `src/components/shell.js`, `ui.js`, `terrain-map.js` | `src/shared/ui/` | Semantic JSX and local schematic SVG; user input rendered as text, not HTML. |
| `src/views/overview.js`, `history.js` | `src/features/overview/`, `history/` | Sector selection, separate observed fires, scenario-scoped filtering and traceable detail. |
| `src/views/sources.js`, `models.js` | `src/features/sources/`, `models/` | Labeled simulated quality/governance and model/history invariance. |
| `src/views/jobs.js`, `prevention.js` | `src/features/jobs/`, `prevention/` | Manual fixture publication and explicit evidence sufficiency, without automatic risk reduction. |
| `src/views/audit.js`, `access.js` | `src/features/audit/`, `access/` | Bounded illustrative audit, permission and implemented/proposed matrix. |
| `tests/state.test.mjs` | `tests/domain/` | Port all 13 behavioral contracts; retain reference Node tests for comparison. |
| `scripts/serve.mjs`, `check.mjs`, `smoke.mjs` | Candidate Vite scripts and `tests/e2e/` | New candidate tooling; retain reference checks. Recreate CSP/asset/routing regressions rather than treating Vite as the old server. |

## Data and security contracts

- Model fixture modes distinctly: `fixture-current` and `fixture-future`, separate from `api-demo`. The live API returns a computed canonical level (`bajo`/`medio`/`alto`/`desconocido`) from the non-official academic rule wired into the risk service; the `fixture-current` scenario preserves the former `not_evaluated` placeholder for the internal scenario API only and must not be presented as the live contract. `fixture-future` classifications are high/medium/low but must remain explicitly fictional. Do not silently substitute fixtures on API failure.
- Keep freshness, risk and service health independent. Fixture expiry uses the frozen scenario clock; API freshness follows the contract. A `404` latest result is insufficient data, not low risk; request errors are not a classification.
- Separate observed-fire references from risk estimates. The schematic is not georeferenced and cannot pretend to represent API Point geometry as real sector coverage.
- Preserve provenance, emission/expiry, period and model version in history. Approvals affect subsequent simulated publications only; previous state and batches remain unchanged.
- Evidence references are bounded text, not uploads or active URLs. Explicit sufficiency needs outcome and justification; new evidence invalidates review. Action closure, evidence review and reevaluation never automatically decrease risk.
- Phase A demo role controls are not authentication. In phase B the server authorizes every operation; hiding routes is only UX. API mode must not use the demo role selector to grant privileges.
- No secrets in `VITE_*` variables or bundled code. Only public configuration may reach the client. No passwords, client secrets or access/ID tokens in browser storage, URLs to APIs, logs or fixture data. ID-token logout hints follow the existing documented OIDC flow, not API authorization.

### Phase B supported API boundary

| Operation | Contract and UI adaptation |
| --- | --- |
| `GET /v1/risk/latest?zone_id=valparaiso-demo` | Viewer/admin; one Point zone, synthetic `demo: true`, `academic-rule-v1`, computed `bajo`/`medio`/`alto`/`desconocido` with factors and rationale; preserve current/stale and insufficient-data states. |
| `GET /v1/risk/history?zone_id=valparaiso-demo` | Viewer/admin; bounded 100 batches, descending request-time order; no fictitious server-side filters/pagination. |
| `POST /v1/jobs` | Admin; fixed zone, JSON body, UUID `Idempotency-Key`; preserve key for retries of one request. |
| `GET /v1/jobs/{id}` | Admin; queued/published. Publication is asynchronous server work, not an available manual publish endpoint. Poll with cancellation/backoff; do not invent terminal failure states. |
| `GET /v1/audit` | Admin; last 100 records. Do not replace real actor/time with fixture metadata. |
| Sources/models/prevention/observed-fire services | No supporting endpoints in this slice. In API mode show explicit unavailable states or an isolated labeled simulation, with no apparent server writes. Future integration is blocked until approved contracts exist. |

## Pending work units

All 12 tasks are pending. Each is a coherent future implementation/review boundary, not a promise of commits. Acceptance is task-specific; completion must record observed evidence without checking unrelated tasks.

### Phase A — framework migration with fixture parity (8 tasks)

- [ ] **FW-R01 — Baseline and implementation scope** (depends: none).
  Deliverable: route/action/role/scenario parity matrix and baseline captures for the existing reference; confirm the proposed `frontend/react/` boundary and runtime/package compatibility before source work.
  Acceptance: eight routes and all 13 domain contracts mapped; desktop 1440×1000/mobile 390×844 and keyboard baseline recorded; original entry still runnable. No moves, installs or source edits before separate authorization.
- [ ] **FW-R02 — Isolated typed application/tooling** (depends: FW-R01).
  Deliverable: React/Vite entry, strict TS configuration, local CSS tokens, candidate scripts and future test harnesses in the approved candidate workspace.
  Acceptance: root mounts, public base path resolves local assets, favicon/CSP compatibility checked, standalone type check and build pass; supported versions verified and reproducible dependency resolution documented. Reference and gateway unchanged.
- [ ] **FW-R03 — Domain parity and fixture ports** (depends: FW-R02).
  Deliverable: typed pure reducer/selectors/actions, fixture adapter and data port; reducer/context provider with explicit reset and mode boundaries.
  Acceptance: all 13 existing contracts pass conceptually equivalent tests; independent fixtures/frozen clock, immutable history/model provenance, invalid-input rejection and bounded audit preserved; no persistence or external calls in fixture mode. API DTO types remain separate from future fixture models.
- [ ] **FW-R04 — Accessible shell and route parity** (depends: FW-R03).
  Deliverable: shell/navigation, hash router, status feedback and local schematic map with token-based responsive styling.
  Acceptance: eight routes and direct refresh/back/forward work under root and subpath; unknown/inherited names (`toString`, `__proto__`, `constructor`) normalize safely; viewer redirects and role-change normalization match. Skip link focuses content without changing the route; mobile Escape/close restores focus, hidden links are not tabbable, input caret and feedback remain usable.
- [ ] **FW-R05 — Territory and history slices** (depends: FW-R04).
  Deliverable: overview and history screens, filters and batch detail.
  Acceptance: sector selection, search/freshness/scenario filters and valid detail selection match the reference; observed fires remain separate; stale does not mean high or low; technical demo never presents a live classification. Empty/unknown states are explicit.
- [ ] **FW-R06 — Management slices** (depends: FW-R04).
  Deliverable: sources, models, jobs and audit screens.
  Acceptance: viewer cannot invoke administrative transitions even through the domain API; source review is not clean-data proof; model approval/reversal cannot rewrite history; fixture publication is manual, once-only and scenario-scoped; audit remains bounded and clearly simulated.
- [ ] **FW-R07 — Prevention and scope slices** (depends: FW-R05, FW-R06).
  Deliverable: prevention/evidence forms and access/scope matrix.
  Acceptance: bounded text/date/sector validation, explicit sufficiency with justification, review invalidation, reset/reload and reevaluation invariance match; injection payloads appear as text in forms/cards/audit without execution or active links. Proposed features remain unavailable in current mode.
- [ ] **FW-R08 — Phase A parity gate and rollback rehearsal** (depends: FW-R05, FW-R06, FW-R07).
  Deliverable: domain/component/browser results, comparative screenshots, fixture network audit and candidate usage/rollback notes.
  Acceptance: all eight routes checked at both viewports with no page-wide overflow; keyboard/focus journeys and required security regressions pass; no external fixture calls; build/type checks and root/subpath refresh pass. Original reference remains runnable. Any primary-entry cutover/reference snapshot is a separate approval; phase A can finish without it.

### Phase B — supported API/auth integration (4 tasks)

- [ ] **FW-R09 — API/OIDC/deployment contract gate** (depends: FW-R08).
  Deliverable: endpoint-to-screen matrix, runtime DTO validation strategy, error/retry policy and approved public origin/base/redirect/logout/CSP configuration.
  Acceptance: recheck OpenAPI and actual gateway/browser constraints; confirm public Keycloak client Authorization Code + PKCE S256, state/nonce checks, token expiry policy and logout behavior. New frontend origin may require separately authorized identity/hosting configuration; do not assume existing loopback allowlists permit it. No gateway/backend edits in this migration boundary.
- [ ] **FW-R10 — Real session boundary** (depends: FW-R09).
  Deliverable: OIDC session provider isolated from demo controls, real role-aware navigation and session lifecycle tests.
  Acceptance: cancellation, invalid callback/state, expiry, `401`, forbidden role and logout clear private UI and memory; access token only for API authorization, ID token never sent to API; define supported renewal or re-login instead of assuming refresh tokens. Server denial remains authoritative; reference gateway login unchanged.
- [ ] **FW-R11 — Contract-aware API adapter** (depends: FW-R09, FW-R10).
  Deliverable: supported latest/history/jobs/audit adapters and explicit unsupported future-feature states.
  Acceptance: validated DTOs, fixed zone/Point/unevaluated notice, bounded history/audit and current/stale/insufficient/error/loading states; idempotent request retries and cancellable job polling; handle `401/403/404/429/503` where applicable. No manual API publication, fabricated APIs or silent fixture fallback. Adapter tests use deterministic mocked transport; real integration evidence is separately labeled.
- [ ] **FW-R12 — Deployment/session release gate** (depends: FW-R10, FW-R11).
  Deliverable: authorized-origin integration evidence, built-asset/route smoke, production CSP review and reversible hosting proposal.
  Acceptance: login/logout/token lifecycle, viewer/admin server permissions and asynchronous job/history/audit work on the approved environment; fresh load/refresh under intended base, local favicon/assets and CSP pass without broad unsafe exceptions. Fixture mode remains offline from external services; no production-risk claim. Deployment or cutover happens only after separate approval, with rollback to preserved reference; no destructive backend or realm reset.

## Test-first and verification policy

This is passive planning documentation: meaningful behavioral RED/GREEN does not apply, and no tests/builds are required or run for authoring it. Reread the plan, verify source links/task dependencies and inspect scoped status instead. Unborn/untracked Git status cannot prove file-content equality; `git diff` alone is not evidence that untracked content is unchanged.

For future deterministic behavior work: add the smallest failing test, observe intended RED, implement GREEN, then triangulate invalid/alternate inputs before refactoring. Existing parity tests may already pass for reused domain code; record baseline/port evidence rather than inventing RED. CSS/visual-only work uses screenshot and functional checks, not a manufactured unit-test failure.

### Existing reference commands — previously recorded, not run for this plan

From repository root:

```sh
node --test frontend/tests/state.test.mjs
node frontend/scripts/check.mjs
node frontend/scripts/smoke.mjs
```

These remain reference checks and do not prove React parity. The mockup plan records 13 domain passes and 24 templates; its browser evidence is scoped, not compliance certification.

### Proposed future candidate commands — unavailable until FW-R02

Define these scripts in the future candidate package; run from repository root only after implementation/dependency authorization. Do not run them for this document.

```sh
npm --prefix frontend/react run test:domain
npm --prefix frontend/react run test:components
npm --prefix frontend/react run test:adapters
npm --prefix frontend/react run typecheck
npm --prefix frontend/react run build
npm --prefix frontend/react run test:e2e
```

Suggested script contracts: `test:domain`, `test:components`, `test:adapters` use scoped `vitest run`; `typecheck` runs `tsc --noEmit` against the candidate app configuration; `build` runs `vite build`; `test:e2e` runs Playwright against loopback candidate hosting, including built assets and root/subpath configuration. Configure browser output inside the authorized candidate workspace. Phase B live checks require an approved environment and exact runner commands defined at FW-R09; this plan does not invent a live backend test command.

Required browser matrix: skip-link route independence, inherited route names, direct protected hashes, role downgrade, mobile menu/focus/keyboard, injection-as-text, all eight routes, scenario/reset/reload, model/history invariance, evidence/risk invariance, CSP/favicon, no false live predictions, no external calls in fixture mode, responsive smoke, refresh/base path. Add login/logout/expiry/server-denial journeys for phase B. Review screenshots separately; green structural tests alone cannot establish visual correctness.

## Risks, prerequisites and rollback

| Risk or open product decision | Boundary / mitigation |
| --- | --- |
| Offline behavior or native mobile | Not required for phase A; separate product decision, no persistence/PWA/mobile work assumed. |
| Real geography/provider and licensing | Retain schematic; approve data accuracy, coverage, provider/privacy requirements separately before map replacement. |
| Prevention acceptance and evidence retention | Preserve current simulated criterion; real workflows require product approval and new server contracts. |
| Origin/CSP/Keycloak compatibility | Phase B gate; public config is not a secret, but redirect/origin permissions and production hosting must be verified. |
| Fixture/API shape mismatch | Explicit adapter/mode boundary; one real demo Point is not the fictional sector catalogue; never merge fictional history into API records. |
| Framework drift or unnecessary redesign | Verify supported versions at bootstrap; preserve tokens, pure rules and route parity before considering optimizations. |
| No Git baseline; everything untracked | Preserve reference in place and document evidence. Future ownership/snapshot/cutover approval required; status/diff alone cannot ensure recovery. |

Rollback during phase A means stop serving the candidate and run the unchanged reference. Phase B rollback disables the API candidate and returns to the preserved, clearly labeled mockup; it does not undo server jobs, audit or identity state. No deletion, reset, automatic branch, commit, PR or publication is part of these work units.

## Framework documentation consulted

Retrieved through Context7 on **2026-10-09**; recommendations are proposals, not version pins or installed dependencies.

- React: [add React to an existing project](https://react.dev/learn/add-react-to-an-existing-project), [client createRoot](https://react.dev/reference/react-dom/client/createRoot), [TypeScript](https://react.dev/learn/typescript). Supports client-root migration and typed reducer boundaries.
- Vite: [React TypeScript template](https://github.com/vitejs/vite/tree/main/packages/create-vite/template-react-ts), [features / TypeScript](https://vite.dev/guide/features.html#typescript), [public base path](https://vite.dev/guide/build.html#public-base-path), [environment variables](https://vite.dev/guide/env-and-mode.html). Template/tooling and public environment exposure informed the plan; confirm selected-version base/hosting behavior during implementation. Vite transformation/build is not a substitute for explicit type checking.

## Next action

**FW-R01 is first.** Ask for separate authorization to implement the candidate boundary, then capture the baseline before installing dependencies or changing source. Offline/mobile/map/prevention decisions do not block phase A parity work.
