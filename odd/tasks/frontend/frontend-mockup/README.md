# FireWatch prevention mockup evolution

## Objective

Reframe the product reference as a prevention and territorial work-coordination system: incoming information supports an administrator's priorities, assignments and operations, followed by progress, evidence and review. The model is a background information supplier, not the user's primary workflow.

## Authorization

- Authorized now: research official references and product opportunities, remove development/fiction disclaimers from the local mockup's visible interface, and create a proposed task backlog in this directory for user review.
- Not authorized now: implement the proposed dashboard/operations features, integrate APIs, alter backend/authentication, deploy, install dependencies, commit or mutate Git history.
- Existing source/core fixture data remain local. Developer documentation retains the implementation boundary; the product UI should look like its intended finished experience without repeatedly describing itself as a mockup.
- Do not add claims of live connectivity, scientifically validated predictions, official emergency alerts or demonstrated risk reduction. Meaningful freshness, insufficient-information and uncertainty states remain part of the product.
- Spanish user-facing interface; English planning documents and identifiers.
- Preserve the existing migration plan and historical mockup plan. Any changes to product requirements are proposals for review, not silent migration approval.

## Current work

- [ ] FWP-C01 — Product-facing copy and presentation cleanup. Implementation complete; final browser visual review pending.
  - Route: delegated direct; multiple non-trivial presentation files and preparation reading.
  - Acceptance: no development/fiction wording in visible views or transient messages; clear prevention identity; representative intended-product entry; existing interactions, escaping, domain invariants and keyboard routing retained.
  - Checks: Node domain tests, syntax/template check, HTTP smoke, presentation-copy regressions and local desktop/mobile browser checks.
  - TDD: deterministic copy/rendering checks can observe RED before cleanup; preserve existing domain tests. Passive copy has no artificial scientific or integration proof.
  - Commit: not requested; none authorized.
- [x] FWP-C02 — Evidence-grounded proposed backlog for review (planning document only).
  - Route: read-only research delegation followed by bounded documentation writing.
  - Acceptance: official sources, capability comparison and explicitly labelled opportunity hypotheses; ordered proposed work units with acceptance/dependencies and open product decisions.
  - No proposed feature is implemented under this task.

## Constraints and delivery

- Local-only work, no external assets or service calls in the mockup.
- Native RDD mode is on; existing preflight is pending schema-bound intended-untracked selection in an unborn/untracked repository. No receipt or approval is implied. Functional independent verification remains required.
- Forecast: approximately 150–300 presentation/test lines plus 300–600 planning-document lines; advisory only, no minification or arbitrary task split.
- Research and interface cleanup may run concurrently because research does not read or edit frontend source.
- UI writer must be the sole source writer; subsequent planning writes are restricted to this folder.

## Evidence and next step

- User explicitly requests administrator assignment of preventive work and operations, a data dashboard, and 15-minute input visibility rather than model-centric controls.
- Copy cleanup removed product-facing development/fiction labels and reframed the overview around prevention and territorial priorities. Meaningful information-quality states remain. No new dashboard or assignment workflow has been implemented.
- After copy cleanup: `node --test frontend/tests/state.test.mjs` passed 13/13; `node frontend/scripts/check.mjs` passed syntax, 24 rendered combinations, product-copy and injection checks; `node frontend/scripts/smoke.mjs` passed HTTP confinement. Automated checks do not replace visual browser review.
- Edge-based visual verification was attempted three times, but the CDP harness failed due PowerShell quote parsing. Available prior screenshots predate the current copy update and are not evidence. Current visual/responsive browser verification remains pending.
- Official-source research and the eight proposed work units FWP01–FWP08 are documented below. All feature tasks remain pending user review; only planning item FWP-C02 is complete.
- Next: review this backlog with the user. If requested, continue browser proof or revise tasks. Do not implement FWP01+ before review.

## Proposed backlog — pending user review

These are proposals to inspect and approve, not accepted requirements or implementation authorization. Review the first batch (FWP01–03) before considering later increments. IDs are stable proposal references; items remain pending until explicitly approved. No item authorizes source, API, backend, authentication, deployment, dependency, or Git changes.

### Product hypothesis and evidence boundary

**Hypothesis to validate:** FireWatch can connect territorially contextualized incoming information to transparent human prioritization, resourced preventive operations, auditable evidence and deliberate reassessment. This is a local-validation hypothesis, not a proven market gap or exclusive capability. Validate with a municipal coordinator and a prospective field-team member.

The product's primary actor is the responsible human. Information refresh is not an automatic reassignment, interruption, or risk judgment. Keep observed fire history, assessment, and action distinct; unknown or insufficient information must never be shown as low risk. The 15-minute cadence is user intent, not a delivered backend capability: the current mockup has browser-memory fixtures, while a backend refresh scheduler is not implemented. Do not claim live data, model calibration, or measured risk reduction.

### Ordered work units

#### First review batch — workflow foundation

**FWP01 — Trustworthy operations dashboard** · Priority: 1 · Status: pending approval  
**Acceptance:** Present territory, observation/source context, and separate observed facts, assessment, and actions. Show latest received time, latest successful refresh, last-good-data age, expected 15-minute cadence versus actual freshness, and explicit delayed/partial/failed/unavailable/no-data states. Preserve last-good age on failure. Provide a useful summary and recent trend/coverage without invented chart values or predictive claims.  
**Dependencies:** None.  
**Testing:** Fixed-clock cases for fresh, delayed, partial, failed, unavailable, empty and stale-last-good data; verify unknown is not rendered as low risk and no unsupported values are fabricated.

**FWP02 — Human-reviewed territory priority queue** · Priority: 2 · Status: pending approval  
**Acceptance:** Explain each proposed ordering using inspectable criteria and rationale; surface overdue/blocked prior operations and uncertainty. Allow a human to override with a recorded reason. Distinguish historical fire occurrence from current action priority. Use transparent, reviewable rules rather than an invented calibrated score or model-only prediction.  
**Dependencies:** FWP01; validated input definitions and an accountable priority owner.  
**Testing:** Known-input ordering and missing-data cases; tie/uncertainty behavior; override reason and unchanged history; confirm refresh alone does not reassign or interrupt work.

**FWP03 — Preventive operation assignment lifecycle** · Priority: 3 · Status: pending approval  
**Acceptance:** Define an operation linked to a community/sector and, where validated, a prevention-plan objective/action; assign an owner/team and due date. Represent acknowledgement, progress, blocker, submission, reviewer acceptance, return for revision, reopen, and cancellation with history. Keep assignment and review decisions attributable.  
**Dependencies:** FWP01–02; stakeholder agreement on operation vocabulary and lifecycle.  
**Testing:** Valid and invalid transitions, missing required assignment data, reviewer return/reopen, cancellation, and preserved actor/time/history. Validate the proposed data model before any implementation is separately authorized.

#### Later increments — do not start without review

**FWP04 — Roles, scopes and approval policy** · Priority: 4 · Status: pending approval  
**Acceptance:** Confirm whether viewer, coordinator/admin, team member and reviewer are distinct roles; define organization/territory scope and who may assign, submit, approve or reopen. Keep external emergency authorization separate.  
**Dependencies:** FWP03 and stakeholder role mapping.  
**Testing:** Permission matrix for allowed and denied actions across roles and scopes.

**FWP05 — Calendar and resource capacity** · Priority: 5 · Status: pending approval  
**Acceptance:** Only after validation, represent team/crew/equipment availability, travel, conflicts and estimated versus actual effort.  
**Dependencies:** FWP03–04; validated resource units and scheduling owner.  
**Testing:** Capacity/conflict cases and unknown availability; no silently assumed capacity.

**FWP06 — Field evidence and quality** · Priority: 6 · Status: pending approval  
**Acceptance:** Specify required evidence criteria and distinguish captured from submitted time, source, location confidence, correction and review. Decide offline behavior, privacy and retention before any real uploads. Vendor patterns are references, not partnership commitments.  
**Dependencies:** FWP03–04; approved privacy/retention policy and evidence ownership.  
**Testing:** Required/optional evidence, invalid or corrected submissions, provenance and review outcomes; offline synchronization only if separately approved.

**FWP07 — Prevention effort and backlog measures** · Priority: 7 · Status: pending approval  
**Acceptance:** Define denominators and terms for overdue, blocked, assigned, submitted, accepted/completed, effort by team/commune and distinct community reach. Do not claim a percentage reduction in risk without a validated causal study.  
**Dependencies:** FWP03–06 and agreed metric definitions/data quality.  
**Testing:** Hand-calculated fixtures, boundary/empty denominators, and explicit unknown values.

**FWP08 — Audit and 15-minute reassessment timeline** · Priority: 8 · Status: pending approval  
**Acceptance:** Show new information as a review flag or possible reassessment; preserve the current assigned-action snapshot until a human changes it. Record actor, time, old/new values and reason; expose stale state. No automatic reassignment, notification or interruption is implied.  
**Dependencies:** FWP01–04; refresh provenance and audit ownership.  
**Testing:** New-input-without-assignment-change, human reassessment, stale/late input, and immutable audit history cases.

### Decisions required before approval

- Who is the operational administrator, reviewer and field user, and at what organization/territory scale?
- Which sources are authoritative; how are source reliability, partial results and the actual 15-minute integration measured?
- Which prevention-plan objectives/actions and geographic data are available, licensed and maintained?
- Which assignment states, priority criteria, override authority and review thresholds are appropriate?
- Are offline field capture, maps, evidence types, privacy/retention, and integrations needed and permitted?
- Which effort units and backlog measures are useful, and who owns their definitions?

### Research basis (official/vendor capability evidence, retrieved 2026-10-09)

- [CONAF — Prevention](https://www.conaf.cl/incendios/prevencion/): distinguishes preventing human-caused ignitions from vegetation measures that mitigate spread; includes education and territorial management.
- [CONAF — Prepared Communities](https://www.conaf.cl/incendios/prevencion-y-mitigacion/comunidades-preparadas-frente-a-los-incendios-forestales/): plans, training, continuous follow-up and updated action plans. Historical totals run through 2024 and 2025 is a forecast; no 2026 total is asserted.
- [SENAPRED — Plans](https://www.senapred.cl/planes-grd/): distinguishes risk-reduction mitigation/preparedness from response coordination; plans specify actions, targets, deadlines and responsibility.
- [SENAPRED — GRD program](https://www.senapred.cl/programa-grd/): 2026 municipal-plan/professional GRD funding lines do not establish funding for FireWatch software or product demand.
- [SafetyCulture/Mitti — Actions](https://help.mitti.com/en-US/topics/actions/): documents individual/bulk/external assignments, recurring actions, priorities, due dates, evidence attachments, templates and access. Current pages use Mitti (by SafetyCulture) branding.
- [Esri — Field Maps](https://www.esri.com/en-us/arcgis/products/arcgis-field-maps/overview): advertises field task assignment, prioritization, scheduling/dispatch, forms, shared spatial information and offline use.
- [Esri — Survey123](https://www.esri.com/en-us/arcgis/products/arcgis-survey123/overview): documents required smart-form fields/rules, location, photos/audio/signature, reporting and offline use.
- [Fulcrum — Santa Barbara County Fire Department case](https://www.fulcrumapp.com/customer-stories/santa-barbara-county-fire-department/): describes defensible-space inspection, mapped coverage, backlog/workload and resource allocation in a 2019 historical case; this is not evidence of Chilean policy or performance.

These references support the relevance of planning, assignment, evidence and follow-up patterns; they do not prove competitor absence, FireWatch demand, local policy fit, or efficacy. Validate each hypothesis with local stakeholders before turning it into a requirement.
