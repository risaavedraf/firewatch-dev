# FireWatch product reference

A dependency-free, local Spanish-language product reference centered on territorial prevention. It is **a local mockup, not a service**: it is not served by the gateway, calls no backend, ingests no real records and loads no trained model. Every value is illustrative in-memory data. The browser opens the representative intended-product fixture, with six connected views and product-facing copy.

> **The official dashboard is served by the gateway**, and it is the supported product surface for real provenance, pipeline status and territorial data. This folder is a separate local interface reference used to preserve the intended presentation and to keep the presentation checks runnable without any infrastructure.

## Retired mock surfaces (L4 decision)

The following mock surfaces were retired because they had **no backend support** and **no overlap** with the supported product, being superseded by the official gateway dashboard:

| Retired | Why | What was removed |
| --- | --- | --- |
| Model comparison (`#models`) | No version registry, no metrics endpoint and no approval workflow exist; presenting candidate precision/recall/AUC invented model governance | View and route, its reducer transitions, its metrics/sample fixtures, its CSS |
| Preventive actions (`#prevention`) | No action, responsibility, deadline or evidence service exists; the evidence flow implied a persistence and review process that does not exist | View and route, its state transitions, its action/evidence fixtures, its CSS |
| Observed fires panel (in `#overview`) | No observed-fire source is ingested; the panel read as observed risk | Panel, its fixtures and its CSS |
| Fictitious source metrics (in `#sources`) | Record counts, missing/duplicate totals, coverage percentages and timestamps had no provenance and could be mistaken for ingested data | Counts and numeric quality fields; sources are now declarative descriptors only |

After retirement this folder keeps **six** views: `#overview`, `#history`, `#sources`, `#jobs`, `#audit` and `#access`. Sources, jobs, audit and access remain **documented mockups**: they describe the intended workflow with in-memory state and no backend. No route was rewired to the backend, no real data was ingested and no model was loaded.

Retired routes are not merely hidden: they are absent from the route table, so a direct hash navigation to `#models` or `#prevention` normalizes to `#overview` for every profile, and the removed module files no longer exist. `tests/state.test.mjs` asserts this, and `scripts/check.mjs` renders every remaining route for each role and scenario.

## Quick start

From the repository root, with Node 24:

```sh
node frontend/scripts/serve.mjs
```

Open `http://127.0.0.1:4173`. Stop with Ctrl+C. No installation is needed. Native modules require HTTP; opening `index.html` as a file is not supported. From `frontend/`, `npm run serve`, `npm test`, `npm run check`, and `npm run smoke` are equivalent conveniences; there are no dependencies or lockfiles.

1. Start in **Territorio** with the **Consulta** profile. Select sectors, inspect the map detail and read the publication history.
2. Select **Coordinación** under **Perfil**. Review the declared sources, or request and manually publish a process in **Actualización de información**.
3. Use **Restablecer filtros** to restore the representative fixtures and the Consulta profile. Reloading also discards every local change.

The browser explicitly selects the internal `future` scenario on startup and reset. There is no end-user scenario switch. The domain factory still defaults to `current`, which models the former placeholder contract for the internal scenario API and tests; the **live API contract changed in the data-contract slice and now returns a computed canonical level** (`bajo`/`medio`/`alto`/`desconocido`) from the non-official academic rule, so `current` must not be read as the live contract. Consulta/Coordinación map to `viewer`/`admin`; this selector does not authenticate anyone. Presentation aliases replace technical model/batch IDs and the synthetic audit actor without rewriting those identifiers or historical classifications.

## What is real, represented, or proposed?

| Concern | Existing backend contract | This local mockup |
| --- | --- | --- |
| Technical risk | `valparaiso-demo`, Point, synthetic input (`demo: true`), `academic-rule-v1`, a computed canonical level (`bajo`/`medio`/`alto`/`desconocido`) | Local fixtures only; the `current` scenario still shows the former `not_evaluated` placeholder and makes no API call |
| Product risk | Real inference not implemented | Fictitious high/medium/low in the representative internal future fixture; no development disclaimers in UI |
| Sources | Authenticated provenance endpoint served by the gateway; the official dashboard is the supported consumer | Declarative source descriptors with a local review state; **no counts, timestamps, coverage or quality totals**, and no ingestion |
| Model governance | Not implemented | Retired here; history rows keep a reference label, but no approval, reversal or comparison flow exists |
| Jobs | Queued/published asynchronous workflow | In-memory queue and explicit manual publication |
| Scheduler | Proposed 15-minute ingestion/inference cadence | No timers for ingestion; training remains a separate proposed workflow |
| Identity | Existing gateway uses OIDC/JWT | Role selector only; no login, credentials, tokens or security guarantee |
| Audit | Admin endpoint | Maximum 100 local illustrative events with fixed actor/time |
| Geography | Existing technical demo uses one point | Fictional sectors, contours and roads; schematic, not georeferenced |
| Freshness | Emission/expiry are independent of risk | Frozen scenario clock: 2026-01-15 15:00 UTC; not wall-clock freshness |
| Service health | Not queried | No connectivity badge; the official dashboard reports pipeline status, this mockup does not |

Source review records a local review state, never proof of clean data: importing marks a descriptor as pending review again and never fabricates record totals. Publishing a process appends a local history row; it never lowers or raises risk. History rows keep reference labels for traceability, but no model version is approved, compared or activated in this folder.

## Module map

| Surface | Responsibility | React/Vite migration reference |
| --- | --- | --- |
| `index.html`, `styles/tokens.css`, `styles/app.css` | Local entry, design tokens, responsive layout | Vite entry + global theme; retain tokens |
| `src/data/fixtures.js` | Fresh in-memory fixtures and frozen clock; declarative source descriptors without counts | Development fixtures; replace with typed API adapters |
| `src/core/state.js` | Pure reducer, validation, simulated write permissions/audit | `useReducer` or domain store; server remains actual authorization authority |
| `src/core/selectors.js` | Scenario risk, data freshness, filters, management eligibility | Reusable pure selectors |
| `src/core/router.js` | Declarative route metadata and mock route permissions | Router configuration and navigation model |
| `src/components/shell.js`, `ui.js`, `terrain-map.js` | Shared shell, escaped presentation, local SVG | Layout + UI components; replace schematic only when valid geospatial data exists |
| `src/views/*.js` | Retained feature screens (overview, history, sources, jobs, audit, access) | Feature-level route components |
| `src/main.js` | DOM composition, delegated events, hash navigation, status feedback | React root, controlled forms, router, accessible toast |
| `scripts/serve.mjs`, `check.mjs`, `smoke.mjs` | Local-only serving and structural/HTTP verification | Replace development server with Vite; keep domain/security regression tests |
| `tests/state.test.mjs` | Behavior-level deterministic domain tests | Preserve reducer tests independent of UI framework |

The domain core imports no DOM or browser APIs. Rendering escapes every user-derived text value. References are rendered as text, never active URLs or raw HTML. Shared UI helpers accept trusted internal markup only; do not feed user content into markup parameters. State is not stored in cookies, browser storage, files or a database.

## Views and requirements

| Route | Product behavior | RF coverage |
| --- | --- | --- |
| `#overview` | Sector selection, risk/freshness/context, coverage and territorial factors | 04, 05, 10, 13 |
| `#history` | Scenario-scoped search, freshness filters, batch detail/source/period/reference | 05, 08, 10 |
| `#sources` | Declarative source catalogue, review criteria and local review/import state | 01, 02, 06, 13 |
| `#jobs` | Request/advance a synthetic process, publish a traceable batch | 06, 08, 09 |
| `#audit` | Actor/action/detail/time for simulated administrative writes | 09 |
| `#access` | Profile responsibilities and human-review workflow | Cross-cutting / RNF-01, 02 |

Coverage previously attributed to the retired views (model-version review and preventive-action/evidence lifecycle) is intentionally uncovered here; those requirements belong to the supported product and its backend services.

Viewer may consult territory, history and access. Administrative routes are hidden and direct hash navigation redirects to territory. Changing to viewer also normalizes the current route; the reducer independently rejects administrative writes. Source management requires the internal future scenario; synthetic jobs can be exercised in either scenario. Reset and profile selection are presentation controls available to both roles, not authenticated administrative operations. The removed scenario control remains available only through the internal reducer API.

## Verification

Run from the repository root:

```sh
node --test frontend/tests/state.test.mjs
node frontend/scripts/check.mjs
node frontend/scripts/smoke.mjs
```

Tests protect scenario separation, frozen-clock expiry, filters, routes/roles, retired-route normalization, self-publish lifecycle, source review state without fabricated counts, validation, reset and audit, and the absence of the retired state and fixtures. `scripts/check.mjs` syntax-checks every module, resolves local imports/assets, renders every remaining route/role/scenario combination and history detail, detects duplicate IDs and unsafe resources/handlers, and asserts text escaping against an adversarial injection string. Its presentation regression checks visible text, SVG descriptions, accessible labels, metadata, and reducer feedback after supported actions for development vocabulary; internal data attributes are not visible copy. HTTP smoke runs an ephemeral loopback server and closes it in `finally`.

CSS has no meaningful deterministic behavioral RED. Automated structural rendering and HTTP checks do **not** establish visual correctness, accessibility compliance, or working browser interaction. If no browser harness is already installed, browser verification remains pending; do not install dependencies just for this mockup.

### Browser / screenshot checklist

Use an already available browser. Capture screenshots through the parent review harness, not into the implementation tree.

- [ ] Desktop 1440×1000: representative territory overview; verify terrain map and detail hierarchy, selected-sector focus, no page-wide overflow or empty grid gaps.
- [ ] Desktop: admin sources (descriptor catalogue, no numeric columns) and jobs; review dense tables and manual publication.
- [ ] Mobile 390×844: open/close menu (Escape also closes); hidden navigation is not tabbable; single-column layout and horizontally scrollable tables.
- [ ] Keyboard: skip link, menu, profile, sector buttons, history search and detail, source review buttons, status feedback and visible focus.
- [ ] Viewer: direct `#models` or `#prevention` redirects to territory; changing admin to viewer from any management route returns to territory.
- [ ] Create a job, publish once, inspect the new history row and audit event. No automatic interval publication.
- [ ] Reset and reload: confirm fixtures return and local changes disappear.

## Hosting and production limitations

The development server binds only to `127.0.0.1:4173`, uses an explicit public asset allowlist, permits GET/HEAD only, rejects raw/encoded traversal and slash/backslash ambiguity, and checks canonical paths to refuse symlink escape. It never exposes source tests, scripts, credentials, repository directories or directory listings. It is **not production hosting**, HTTPS, an authorization server, or a performance benchmark. Do not expose it publicly.

For a future React/Vite implementation, retain feature/domain boundaries but replace fixtures with contract-aware adapters, implement real OIDC and server-side authorization, introduce validated geospatial data, calibrated models and evidence lifecycle persistence. Reassess RNF security, reliability, observability and load objectives independently; this reference does not claim those guarantees.
