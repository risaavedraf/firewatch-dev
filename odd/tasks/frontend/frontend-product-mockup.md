# FireWatch product mockup

## Objective and problem

Create a polished, modular, responsive static mockup in `frontend/` that demonstrates the complete intended FireWatch product and provides a migration reference for React and Vite. The existing gateway UI remains untouched.

## Authorized scope and constraints

- New static HTML, local CSS and JavaScript modules, fixtures, tests and usage documentation inside the currently empty `frontend/` directory.
- Neutral Spanish UI, following existing product language; English technical documentation and identifiers.
- Cover RF-01 through RF-13 with explicit current-demo versus proposed-future labels.
- No real API, authentication, prediction, ingestion, notifications or prevention operations. No external assets, CDN or dependencies.
- All data and user actions are illustrative, in memory, resettable. Never present a synthetic classification as a real prediction.
- Schematic map with fictitious sectors, not a georeferenced operational map. Observed-fire references and predictions remain separate.
- Viewer/admin selection is a demonstration, not an authorization boundary.
- Keep all pre-existing repository files intact, including untracked files. Do not stage, commit or change branches: user did not request Git mutations; repository has no initial commit.

## Work units and route

- [x] FW-M01 — Build the coherent interactive product mockup, pure-state tests and migration documentation.
  - Route: delegated direct; multi-file non-trivial implementation and preparation-reading trigger.
  - Views: territorial overview, history, source quality, models, processing, preventive actions/evidence, audit, access/scope.
  - Acceptance: local responsive shell; working navigation/filters/sector detail; meaningful simulated administrative interactions; viewer restrictions; explicit demo boundaries; freshness and uncertainty; accessible labels, keyboard actions and status feedback; no destructive or remote operations.
  - Test-first policy: Node v24.11.1 built-in runner for deterministic pure-state behavior. Observe RED before implementation, then GREEN. CSS/visual design has no meaningful automated RED; use structural and browser checks.
  - Checks: `node --test frontend/tests/state.test.mjs`; `node frontend/scripts/check.mjs`; `git diff --check -- frontend odd/tasks/frontend/frontend-product-mockup.md`; local HTTP and browser smoke checks when available.
  - Commit: not authorized, pending explicit user request. No feature branch created.
- [x] FW-M02 — Independently verify the delivered mockup and document observed results and limitations.
  - Route: delegated verification because executable UI and native assessment currently unassessable/high. The final favicon CSP adjustment was a known one-line mechanical inline correction in `frontend/scripts/serve.mjs`.
  - Acceptance: rerun required checks; assess navigation, permissions simulation, demo/future distinctions, injection safety and server path confinement; browser responsiveness when tooling is available.

## Delivery and forecast

- Local-only delivery; no PR, publishing, deployment, install or Git mutation.
- Forecast: approximately 1,400–2,200 authored lines including tests and documentation. This is one coherent mockup, not a production implementation; do not shrink or minify to meet a line heuristic.
- Running committed lines: 0; no reviewed boundary exists in this unborn repository.
- RDD mode: on, global. Initial assessment returned high/unassessable because all existing content is untracked. Recheck native status after implementation without enrolling unrelated files. Never fabricate approval.

## Progress and evidence

- Exploration confirmed `frontend/` has no files.
- Requirements spot-checked in `Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md:40–88`.
- Node v24.11.1 and npm 11.21.0 observed. No dependency install is required.
- Added 25 frontend files with eight routes, local schematic SVG, current/future scenarios, role controls and in-memory workflows.
- Writer observed behavioral RED (10 assertion failures), GREEN, then 12 passing domain tests. Parent independently reran all 12 successfully.
- Writer structural checks passed: 20 syntax checks and 24 rendered route/role/scenario combinations. Ephemeral HTTP smoke passed. `git diff --check` returned 0 but does not inspect these untracked files.
- Existing local Edge enabled dependency-free CDP verification despite no Playwright/Puppeteer package. All eight admin/future views passed desktop/mobile overflow checks; screenshots at 1440×1000 and 390×844 were inspected by verifier and parent.
- Native preflight requires intended-untracked selection with schema-bound JSON. No review has started or approved this candidate; unrelated repository files remain excluded from any proposed review.
- Independent verification found and corrected skip-link hash routing, inherited-property route acceptance, mobile menu focus restoration and favicon/CSP compatibility. Routing regression observed RED then GREEN.
- Final independent checks: 13/13 domain tests, 20 syntax checks, 24 templates, HTTP confinement smoke and 9/9 focused browser checks. Browser console errors and application exceptions: 0. Parent reran final HTTP smoke successfully.
- Role/scenario, history, sources, model approval/revert, jobs, prevention/evidence, reset and injection-as-text flows were browser exercised. Full-page visual inspection and exhaustive keyboard accessibility audit remain unperformed; no production compliance claim.
- Local server/browser processes started for verification were closed. No installs, commits or unrelated edits.
- Native review remains pending at intended-untracked schema selection; no receipt or approval claimed.

## Next step

User can run `node frontend/scripts/serve.mjs` and open `http://127.0.0.1:4173`. Collect product feedback before migrating feature boundaries to React/Vite. Formal native review needs its schema-bound untracked selection before it can start.

## Supersesion L4 (2026-10-11)

La unidad L4 de odd/tasks/firewatch-app-funcional.md retiro de esta maqueta las vistas sin respaldo de backend: src/views/models.js (comparacion de modelos), src/views/prevention.js (acciones preventivas) y el panel de fuegos observados de overview.js; ademas quito las metricas y conteos ficticios de sources. Se conservan overview/history/sources/jobs/audit/access como maqueta documentada.

Decision del usuario registrada: no recablear nada al backend, no ingerir datos ni modelos reales y no tocar los prototipos de web/prototipos/. Este documento se conserva como historia de la maqueta; el frontend oficial vigente es el servido por el gateway (internal/handler/web/). La maqueta sigue siendo local (solo frontend/scripts/serve.mjs), no la sirve el gateway y no tiene consumidores en backend, deploy, CI ni docker-compose.
