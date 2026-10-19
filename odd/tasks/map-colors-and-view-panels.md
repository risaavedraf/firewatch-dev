# Colour every comuna on load, keep the selected one, and give the flat views their cards

Objective: the territorial map must show every comuna's last published level without a click, the
zone the reader selected must survive leaving the view, and the five views that render their body
straight onto the canvas — plus the actions table's selected row — must get the cards and marks
their stylesheet already declares.

Branch: `fix/map-colors-and-panels` (feature branch; `push`, PR and merge stay the user's decisions).
Delivery: local commits only, one work unit per task.
Runners: `npm run verify` from `frontend/` and `go test ./...` from `backend/` (plus the focused
packages), with a Playwright differential under `tmp/` (Playwright is not a repository dependency).

## Specs

- **S1** — User verbatim: «hay otro error en el mapa que solo cargan los colores cuando se seleccionan
  en el mapa, si no los seleccionas antes no muestra el color, lo arreglas, la idea es que se vean los
  colores sin necesidad de seleccionar esa psrte del mapa».
- **S2** — User verbatim: «otro error esque siempre que se sale de la parte del mapa la comuna
  seleccionada queda en Valparaiso y no la que seleccioné antes de salir del apartado del mapa».
- **S3** — User verbatim: «fixea esos errores porfa», answering the two observations reported with the
  earlier CSS fix: (a) `.is-selected` has no rule, so the selected row of the actions table is not
  marked visually; (b) `HistoryView`, `AuditView`, `JobsView`, `PendingView` (and the actions list)
  render their body directly on the canvas — no card — although `app.css` already declares
  `.history-panel`, `.audit-panel`, `.jobs-panel` and `.pending-panel` padding for them.
- **S4** — User verbatim, on how the map gets the data: «y si haces un endpoint que solo se ejecute sl
  iniciar la pagina que cargue todo el mapa de apoco? y luego se usa el endpoint normal».
- **S5** — Project honesty rule, already enforced by `frontend/scripts/check.mjs` and the frontend
  suites: no invented data. A comuna whose level the page did not read and validate stays neutral; a
  level is never inferred, copied or defaulted, and a zone with no published batch is never painted as
  low risk.
- **S6** — Project test-first rule: one RED test per rule before the fix, asserting the observable
  effect through the public interface; the stylesheet half is pinned in jsdom and the geometry is
  measured in a browser.

## Design (frozen before the first write)

- **New route** `GET /v1/risk/batches` in the risk service, proxied unchanged by the gateway (it
  already proxies the `/v1/risk/` prefix). It answers the latest published batch of **every** zone in
  one request: `{"batches":[…]}` ordered by `zone_id` ascending. A zone with no published batch is
  **absent** from the array; nothing published at all answers `{"batches":[]}` with 200, never 404.
- Each element carries the same members `/v1/risk/latest` returns for that zone **except the batch
  `geometry`**: the map takes geometry from `/v1/risk/zones`, and repeating 38 polygons in an
  aggregate response is payload no caller needs. Every member the frontend's fail-closed
  `presentBatch` validates is present.
- One repository query (`DISTINCT ON (zone_id)` … `ORDER BY zone_id, requested_at DESC, id DESC`), not
  one query per zone.
- **Frontend**: one aggregate read per signed-in session, started together with the catalogue read;
  each element is validated with the existing `presentBatch` and only a valid batch may set a level
  (`rememberZoneLevel`). An element that fails validation, and a zone that is absent, leave that
  comuna neutral. The selected zone's detail keeps reading `/v1/risk/latest`, unchanged.
- **Copy correction**: the "Comunas con nivel alto" note currently states «la API responde por zona y
  no existe endpoint agregado» (false once this route exists) and the metric must describe what it
  actually counts in both the success and the failure case of the aggregate read.

## Tasks

- **T1 [S4,S5,S6]** — done: `Store.LatestPerZone` (one `DISTINCT ON (b.zone_id)` query, no geometry
  expression), the shared `(*Batch).readAssessment` derivation, `GET /v1/risk/batches` inside the
  `Authorize`-wrapped mux answering `{"batches":[…]}` ordered by `zone_id` (200 `{"batches":[]}` when
  nothing is published, 503 `storage_unavailable` on failure), handler and contract tests, the additive
  contract composition, the endpoint table row, the permissions-matrix row for the new route and the
  re-pinned citations. Commit `3ce3f91` (9 files, +605/−57).
- **T2 [S1,S2,S4,S5,S6]** — done: `batches()` transport read with whole-answer rejection; one aggregate
  effect per session seeding the map through `presentBatch`; the store claim that `clearZoneState`
  drops; the catalogue auto-select now filling only an empty selection; the corrected metric note; the
  route gate at twelve routes with the URL in `config.routes`. Commit `1df1274` (9 files, +436/−14).
- **T3 [S3,S5,S6]** — done: the five flat views wrapped in the cards their stylesheet already padded,
  the new `actions-panel` modifier in the shared padding rule, the selected-row mark after the hover
  rule, `has-rows` honoured, and the panel scan widened to template literals so a state class cannot
  hide a modifier. Commit `0af247d` (9 files, +543/−344, most of it reindentation).
- **T4 [S1..S6]** — done: evidence in this document's `## Log` (L8).

## Log

- **L1 (2026-10-19) — original request (verbatim, both messages).** «hay otro error en el mapa que solo
  cargan los colores cuando se seleccionan en el mapa, si no los seleccionas antes no muestra el color,
  lo arreglas, la idea es que se vean los colores sin necesidad de seleccionar esa psrte del mapa,
  otro error esque siempre que se sale de la parte del mapa la comuna seleccionada queda en Valparaiso
  y no la que seleccioné antes de salir del apartado del mapa». Together with the authorisation for the
  two previously reported observations: «fixea esos errores porfa».
- **L2 (2026-10-19) — the map's colour is not a style problem: the page never reads the other comunas.**
  Both causes traced before any write. The colour comes from the in-memory level map
  (`src/app/store.ts:26-27`, `:65`), which only the read of the *selected* zone fills, and the map
  paints `zoneLevels.levelFor(shape.code)` (`src/components/map/MapPanel.tsx:138`). The service exposes
  no aggregate: `/v1/risk/zones` carries no level (`backend/services/risk/internal/handler/api.go:100`,
  `newZoneCollection`) and `/v1/risk/latest` and `/v1/risk/history` are both per `zone_id`
  (`api.go:92-93`); the catalogue holds 38 comunas. The reset in S2 is a second cause in the same view:
  the catalogue effect runs on every mount of `OverviewView` and unconditionally selects the first code
  (`src/views/OverviewView.tsx`, `firstCatalogueCode` → `selectZone`), which is Valparaíso (05101).
- **L3 (2026-10-19) — user decision on how the map gets the data.** Offered the three routes (frontend
  fan-out of 38 `latest` reads; a new aggregate endpoint; or only the selection fix), the user answered
  «y si haces un endpoint que solo se ejecute sl iniciar la pagina que cargue todo el mapa de apoco? y
  luego se usa el endpoint normal» — the aggregate endpoint, read once as the page loads, with the
  per-zone endpoint kept for the selected zone afterwards. That is S4, and it is why the honesty copy
  in T2 must change: the page may no longer say the API has no aggregate endpoint.
- **L4 (2026-10-19) — T1 delegated, stalled, re-briefed, delivered and reviewed.** The first attempt
  (task `mvfc9l9g-1-j0ao`) stalled for four minutes inside a bash call and left **no** backend change.
  The cause was the command scope this document's runner line implied, not the toolchain: measured
  directly, `go build ./services/risk/...` takes 1.2 s, `go vet` 0.5 s and the handler suite 3.0 s, so
  the worker had been handed `go test ./...` across the whole workspace, which reaches the tests that
  wait on a database. The retry (task `mvfci28m-3-bgwx`) was briefed with four named commands and no
  git operations, and delivered: `Store.LatestPerZone` with exactly the frozen `DISTINCT ON` query and
  no geometry expression, the shared `(*Batch).readAssessment` extraction so the two reads cannot serve
  two definitions of a batch, `GET /v1/risk/batches` inside the `Authorize`-wrapped mux answering
  `{"batches":[…]}` ordered by `zone_id` (200 `{"batches":[]}` when nothing is published, 503
  `storage_unavailable` on failure), handler and contract tests, and the additive contract split
  (`BatchCore` + `Batch` with `geometry` still required for the per-zone reads, `ZoneBatch` for the
  aggregate). Reviewed against the tree rather than the report: query, envelope, auth path, `omitempty`
  scope (only `List` and `LatestPerZone` produce `repository.Batch`, so no per-zone wire shape moves),
  and the tests' RED evidence all hold.
- **L5 (2026-10-19) — T3 delivered, and one cross-task breakage it was right to refuse.** T3 (task
  `mvfcb37r-2-uwks`) wrapped the five flat views in the cards their stylesheet already padded, added the
  selected-row rule after `tbody tr:hover`, honoured `.history-panel:not(.has-rows)`, and widened the
  panel scan so a conditional `className` cannot hide a modifier. Its suite was red for one reason
  outside its surfaces, and that reason is this feature's own doing: `frontend/tests/views/session.test.tsx`
  verifies every `path:line` the permissions matrix cites by reading **that line** of the cited server
  file, and T1 inserted 34 lines into `backend/services/risk/internal/handler/api.go`, moving the route
  lines. Reconciled here: the seven risk citations in `SessionView.tsx` re-pinned (53→58, 92→97,
  93→98, 94→100, 95→101, 105→111, 106→112), the corresponding keys in the test's `GUARD_LINES`, and a
  new matrix row for the aggregate route the page now calls (`api.go:99`), because a page that reads a
  route must document its guard. `.pending-panel` was also added to the 720 px padding list: the view
  now actually renders that card, so it must drop to 14 px with its siblings. Frontend `npm run verify`
  green afterwards (346 tests).
- **L6 (2026-10-19) — the one rule T1 could not observe, and the user's authorisation to observe it.**
  The repository test `TestLatestPerZoneAnswersTheNewestBatchOfEveryZone` skips without
  `TEST_DATABASE_URL`, so `DISTINCT ON`, the newest-batch tie-break and the absence of a zone with no
  batch were written but never executed. Asked whether to run the repository's own documented
  integration path, the user chose «Corre la integración en `risk_test` (Docker)»: `docker compose
  --profile test run --build --rm risk-test` (`backend/README.md:116`, `Makefile:15`,
  `docker-compose.yml:213`), which migrates the disposable `risk_test` database and then runs the suite
  against it. A skip is not a pass, so this run is what decides whether that rule is verified or
  reported as missing.
- **L7 (2026-10-19) — the run was green and proved nothing, and the reason is a trap worth writing
  down.** The authorized `--build` run exited 0 with `ok firewatch.local/risk/internal/repository`,
  and that `ok` is ambiguous: a skipped test still lets the package report `ok`. Re-running the rule by
  name, verbose, inside the same container printed
  `store_zone_batches_integration_test.go:55: every catalogue zone except the one under test already
  has a published batch; this proof needs a zone without one` and `--- SKIP`. So the rewrite's own
  guard made it vacuous in the repository's own integration database, whose `batches` rows are left
  behind by every run. Two corrections follow, both in flight: the test is being rewritten to pin the
  selection rule as an equality with what the database itself reports (`SELECT id … ORDER BY
  requested_at DESC, id DESC LIMIT 1` per returned element, plus one element per zone and a count
  equality that carries the absence rule), which needs no zone without a batch; and the container must
  be **rebuilt** before it can run that rewrite at all, because the `risk-test` image bakes
  `services/risk` at build time and a `run` without `--build` executes the previously built image. The
  stale-image trap is the real lesson: a green `ok` from a container run says nothing about which
  source it ran, so the evidence that counts is `-v` output showing `--- PASS` for the named test and
  no `--- SKIP`.
- **L8 (2026-10-19) — T4: the evidence, including one measurement of mine that was wrong and the check
  that caught it.**
  - **(a) The browser differential first measured the wrong thing.** The harness navigated the round
    trip with `page.goto`, which is a full document load: it resets the in-memory store, so the map
    re-read the aggregate and the selection fell back to the first catalogue code. That result says
    nothing about S2, which is about leaving the view inside the app. The harness now navigates by hash
    and proves no reload happened (a marker set on `window` survives the round trip); the first run is
    kept as the record of the error rather than deleted.
  - **(b) S1 — colours with no click.** With three synthetic comunas intercepted (two publishing a
    batch with different levels, one with none), and **zero interaction**: `level-bajo: 1`,
    `level-alto: 1`, `zone-neutral: 1`, and exactly **one** `GET /v1/risk/batches`. The comuna without a
    published batch stayed neutral, never `bajo`. Screenshot `tmp/map-colors-and-selection/in-app/
    01-colours-on-load.png`.
  - **(c) S2 — the selection survives.** Selecting Los Andes (05301), leaving to `#history` and coming
    back: the selector, the topbar chip and the outlined shape are all 05301, `noReloadDuringRoundTrip`
    is `true`, and the aggregate was **not** re-issued (`batches` requests still 1). Screenshot
    `…/04-back-on-the-map.png`. The same run also pins the aggregate claim: leaving and returning does
    not re-read it.
  - **(d) S3 — panel geometry in a real engine**, over all eight routes with the current build: every
    panel reports `padding: 18px` and a first-row inset of `19px` — the five restored cards identical
    to the ones that already had a card — with no descendant overflowing its panel, the sidebar blank
    band at 0 px and the workspace block still boxed. Evidence `tmp/layout-probe/after-t3/`.
  - **(e) Suites.** Frontend `npm run verify` exit 0: 358 tests, 41 shipped files, twelve consumed
    routes, build and loopback smoke. Backend: the full risk suite run verbosely in the rebuilt
    container reported **zero `--- SKIP`** and zero failures, with
    `--- PASS: TestLatestPerZoneAnswersTheNewestBatchOfEveryZone (0.03s)` inside it, and the sibling
    suites (`gateway`, `jobs`, `prevention`) are green against the changed OpenAPI document.
- **L9 (2026-10-19) — the work units, their slice boundaries and the residual risks.** Commits on
  `fix/map-colors-and-panels`: `3ce3f91` (T1, 9 files, +605/−57), `0af247d` (T3, 9 files, +543/−344),
  `1df1274` (T2, 9 files, +436/−14), then this document. T1 exceeds the ~400-line slice target and the
  boundary is deliberate, not accidental: the route cannot land without its published contract (79
  lines of document) and without the permissions-matrix half (the matrix cites `path:line` and the
  frontend test reads that exact line, so committing either half alone leaves a red tree); 392 of its
  605 added lines are tests. Residual risks left standing, named rather than hidden: a valid aggregate
  element whose `zone_id` is absent from `/v1/risk/zones` would be remembered and counted, which is
  unreachable today because the backend joins `zones` but would need a catalogue cross-check if that
  ever changed; the transport rejects the **whole** aggregate answer when one element fails its wire
  guard (the discipline `/history` already used), so one unreadable element costs every colour rather
  than only its own, while the domain layer keeps per-element neutrality; and the operational stack
  still runs the previous binary, so the end-to-end path (the new route served from Postgres by the
  running service) needs that container rebuilt — what is proven here is the query against `risk_test`
  and the response-to-colour mapping in a browser.
