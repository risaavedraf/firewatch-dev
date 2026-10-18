# U5 checklist evidence registry

Scope: unit **U5** of `Referencias/tareas_verificacion_implementacion_IA.md`, sections 9, 10, 11 and 12
(test technology stack, Continuous Testing in MLOps, final Go/No-Go approval, and the session 4.2
practical activity). That document is the authoritative criterion source; this file references its IDs and
does not replace or restate them.

**No ID in this file is closed and no checklist box is marked — 0 checkboxes marked.** A passing test, a
green workflow run and a recorded check are three different objects from a marked box: a recorded check
observes one bounded behaviour that ran at one moment, while a marked box requires the ID's closing
criterion — reproducible evidence, thresholds agreed with the team, a named responsible person and, where
the criterion says so, real data. The source's five approval boxes (`GNG-V1` to `GNG-V5`) stay literally
unmarked below for that reason.

States used below are the ones the tracking document requires (implemented, partial, proposed, justified
N/A, pending):

| State | Meaning here |
|---|---|
| **Partial** | An executable in-repository artifact speaks to the ID. It is not the ID's closing criterion. |
| **Proposed** | An answer is written down but no responsible person has accepted it. |
| **Pending** | The ID is a defined, self-contained work item that is not started and is not waiting on an external input. |
| **Blocked** | Closing requires an input this repository does not contain: real data, an agreed threshold, an available runner/daemon, a tool that is not wired up, or a named owner. |

Pointers below are anchors at the declaration being cited, not whole-file references. A green run and a
skipped run are both reported as what the repository says, never promoted into a closure.

## Standing limit on all synthetic evidence

The synthetic profiles, the fixed fixture and `academic-rule-v1` prove **code behaviour on invented
inputs**. They cannot prove real-data quality, source reliability, geographic or class representativeness,
statistical bias, absence of leakage, model accuracy or robustness against real perturbations — and they
cannot prove anything about the two-stage gates this unit asks for. A workflow that runs `go test` on
synthetic fixtures is **not** a Continuous Testing pipeline in the sense of the source: it does not
validate real data, it does not benchmark a model, and it does not measure load. Every row below inherits
that limit. No workflow, test, scanner or container was executed to produce this registry; every anchor
is a file read. The single command executed afterwards, during parent review of this file, is the
ancestry check recorded in section 10 (`git merge-base --is-ancestor`); no other command was run. Where a
check is unavailable (for example a Docker-dependent end-to-end run) that is stated instead of claimed as
passing.

## 9 — Test technology stack

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `STK-I1` | Partial | The tools actually used are documented but not in the required per-area table: `backend/README.md:76-92` lists `go test`, `go vet`, the Compose `test` profile and the `Makefile` targets; `backend/docs/verification.md:15-21` records the toolchain actually exercised (`govulncheck@v1.9.0`, Trivy 0.69.3, Actionlint 1.7.7, Kustomize/Kubeconform 0.6.7, kin-openapi 0.133.0, a Prometheus `up` query); `backend/deploy/local/prometheus.yml:1-11` is the only observability-scrape configuration. | The source's per-area table completed with a real selection **and** a technical justification for each of its eight areas. Five areas have no tool at all and are not even marked "undecided": data validation, model QA, drift monitoring, load, and workflow orchestration. The table itself does not exist in this repository. |
| `STK-I2` | Blocked | Closest artifact only: `backend/docs/architecture.md:48-57` documents security and operations boundaries (identity, rate limits, CORS, container hardening, `/metrics`, CI scope). That is architecture documentation, not an evaluation against the five pillars. | An evaluation of the architecture against the five **AWS Well-Architected ML Lens** pillars, and a named owner to accept that evaluation. No pillar assessment exists anywhere in the repository. |
| `STK-V1` | Partial | `backend/docs/verification.md:15-21` ties each recorded tool to the check it covers (vulnerability scan, image scan, workflow syntax, manifest validity, HTTP contract, scrape target); `go test` covers the classifier's robustness cases of section 5 (`backend/services/risk/internal/classify/directional_metamorphic_test.go`, `extreme_inputs_test.go`). | The coverage confirmation the ID asks for: a mapping from **each chosen tool to at least one test defined in sections 4 to 6**. The section 4 data-validation, section 5 model-benchmark and section 6 load tests do not exist, so no tool can be confirmed against them. This row depends on `STK-I1`'s table existing. |

## 10 — Continuous Testing (CT) in MLOps

CI toolchain evidence carried from the previous hand-off, registered here because it is CT evidence. Both
items below were **resolved on `main` by a separate, separately authorized CI work unit** whose own task
document is `odd/tasks/security/ci-go-1.27.2-green.md`; this registry neither executed nor fixed anything
(`.github/workflows/**` and the CI environment stay outside its surface), and it records the resolution
together with the line that had still contradicted it before this batch corrected it:

- `govulncheck@v1.1.4` **panicked under Go 1.27.2**: `backend/docs/verification.md:34` records the pin change
  `v1.1.4` -> `v1.9.0` for exactly that reason, and `:44` records the panic in
  `golang.org/x/tools@v0.29.0/go/ssa/builder.go` (`unexpected expr: *ast.KeyValueExpr`), reproduced on a
  trivial module. **Resolved in the tree read here:** the reusable workflow pins `v1.9.0` at
  `.github/workflows/_go-service.yml:35-36`, confirmed by reading that file; `odd/tasks/security/ci-go-1.27.2-green.md:53`
  (T13) records the bump as done, verified by T5. No `v1.1.4` reference remains in `.github/workflows/**`.
- The steps `Build container with provenance and SBOM` and `Scan runtime image`
  (`.github/workflows/_go-service.yml:49`, `:56`): `backend/docs/verification.md:68` said they "have
  never executed on the runner" until this batch corrected that line, and `odd/tasks/security/ci-go-1.27.2-green.md:68` records them as SKIPPED on
  `main@0ed07e2`. That was true of `0ed07e2` and is **stale now**: `:52` (T12) and `:131` (L11) record push
  `c45508e`, where all three service runs completed with `Enable containerd image store`, `Build container
  with provenance and SBOM` and `Scan runtime image` = success (run ids 38590909040, 38590908871,
  38590908921 recorded there with their URLs). `c45508e` is an ancestor of `main` and of this branch's merge
  base, verified here with `git merge-base --is-ancestor`; the runs themselves were **not** re-queried from
  the runner by this registry, so the resolution is recorded as that document's first-hand report, not as
  this file's own observation. What remains was a **documentation defect, not an open ID input**: the stale
  sentence at `backend/docs/verification.md:68`, which this batch corrected.

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `CT-I1` | Partial | Triggers are defined, path-filtered, on `push` and `pull_request`: `.github/workflows/risk.yml:2-18`, `gateway.yml:2-18` and `jobs.yml:2-18` (each on `backend/services/<service>/**`, `backend/contracts/**`, `backend/go.work`, `backend/go.work.sum`, `backend/.dockerignore`, `.github/workflows/**`); `.github/workflows/integration.yml:2-6` (on `backend/**`, `.github/workflows/**`); `.github/workflows/_go-service.yml:2-7` is `workflow_call` only, so it has no trigger of its own. Consequence of those filters, **inferred here rather than observed**: the commits pushed to `main` today change only `.gitattributes` (`d38fdc5`) and `odd/urgent/**` (`e0cc335`), which match none of them, so those paths are covered by nothing and no run was triggered — the runner was not queried to confirm the absence, and `frontend/**` has no workflow directory at all. | A re-training trigger (the criterion's second option) — there is no training pipeline to trigger; a scheduled or manual trigger; **and** path coverage for the uncovered surfaces (`odd/**`, the root `.gitattributes`, `frontend/**`). Today a change to any of those paths can reach `main` with no workflow evaluating it. |
| `CT-I2` | Blocked | No automated data stage exists. The reusable workflow's steps are format, module integrity, tests, vet and vulnerability scan (`.github/workflows/_go-service.yml:27-36`), and the integration workflow runs Compose and the smoke script (`.github/workflows/integration.yml:20-25`); none validates a dataset. Data-shape assertions live only inside Go unit tests over synthesized profiles (`backend/services/risk/internal/source/synthetic_scenarios_test.go`). | A data stage to automate: a real ingestion path and an agreed data-validation tool/rule set. Illustrative tools from the source (Great Expectations, TF Data Validation) are not chosen, and there is no real data for them to validate. |
| `CT-I3` | Partial | The rule's robustness cases do execute in CI: `.github/workflows/risk.yml:22-24` calls the reusable workflow whose `Tests (go test -race)` step (`.github/workflows/_go-service.yml:31-32`) runs the risk suite, including `backend/services/risk/internal/classify/directional_metamorphic_test.go` and `extreme_inputs_test.go`. | A model stage with a **benchmark** and metrics. There is no trained model, no accuracy/F1/recall figure anywhere, and no model-QA tool selected (Deepchecks/Giskard in the source are not wired); running deterministic rule tests is not a benchmark. |
| `CT-I4` | Partial | The infrastructure recovery path is automated: `.github/workflows/integration.yml:22-25` builds the test profile, brings the stack up and runs `python3 scripts/smoke.py --resilience`, which stops and restarts the broker and verifies recovery (`backend/scripts/smoke.py:117-126`). | The **load** half of the stage: no load generator exists (no JMeter, k6 or Locust configuration anywhere in the repository) and no agreed load target or SLA to judge it against. |
| `CT-V1` | Blocked | Not demonstrated. The two workflows that run tests can fail (`.github/workflows/_go-service.yml:31-36`), but nothing in the repository makes such a failure a gate: no required-status-check or branch-protection configuration is present, there is no production promotion path to block, and the trigger gap in `CT-I1` means a change to `odd/**`, `.gitattributes` or `frontend/**` runs no check at all. | The blocking configuration itself, none of which lives in this repository: required status checks / branch protection, a deployment or promotion gate, and full trigger path coverage. It also depends on `CT-I2`, `CT-I3` and `CT-I4`, whose degrading-data, degrading-model and degrading-load stages are not automated. |

## 11 — Final Go / No-Go approval

The five verification rows below are the source's **approval checkboxes**. None of them is marked, and the
literal rule holds throughout: **0 checkboxes marked.** They are registered as `Blocked` or `Pending`, never
as approved.

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `GNG-I1` | Pending | None. There is no Go/No-Go document in the repository; the closest statements are negative, not a version history: `backend/docs/synthetic-scenarios.md:47` states that no Go/No-Go signature is claimed (the demo handoff that repeated it was retired from the repository on 2026-10-17). | The Go/No-Go document itself with its version history (date, version, author, description). The source's table is an unfilled template. |
| `GNG-I2` | Partial | A check/result table that can seed the matrix exists: `backend/docs/verification.md:3-21` lists the checks performed and their results, and `odd/tasks/security/ci-go-1.27.2-green.md:52` records a CI run outcome. | The matrix's **criterion** column agreed with the team, and a Go/No-Go result per row. `verification.md` carries no criterion column and has no latency or model-regression rows, so the source's three exemplar rows cannot all be filled. |
| `GNG-V1` | Blocked | Unmarked. The only data-validation evidence is synthetic (see `backend/docs/checklist-evidence-u2-u3.md`, rows `DAT-*`). | A **real-data validation report** with fields-present results, and a named approver for the Data Validation approval. Without real data the box cannot legitimately be marked. |
| `GNG-V2` | Blocked | Unmarked. There is no trained model and no evaluation report; `backend/docs/synthetic-scenarios.md:47` states the absence of a benchmark and of model metrics. | A model-evaluation report with measured metrics and a named approver for the Model Evaluation approval. Depends on the blocked `REG-*`/`ROB-*` rows of U3. |
| `GNG-V3` | Blocked | Unmarked. Partial infrastructure evidence exists — `.github/workflows/integration.yml:20-25` runs the recovery smoke and `backend/docs/verification.md:3-21` records infrastructure checks — but it is not an approval. | Agreed SLA/load results and a named approver for the Infrastructure Tests approval. No load measurement exists (see `CT-I4`). |
| `GNG-V4` | Blocked | Unmarked. No deployment decision is recorded; `backend/docs/architecture.md:57` states that registry promotion and automated rollout are later work. | The deployment decision itself (approved for production / rejected) and a named decision-maker. There is no production promotion path to decide on. |
| `GNG-V5` | Blocked | Unmarked. No name of a responsible person appears anywhere in the repository in connection with this approval. | The **signature of the responsible person** — a named individual who owns the Go/No-Go decision. This is the exact missing input; no document can substitute for it. |

## 12 — Session 4.2 practical activity

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `ACT-I1` | Partial | Compose starts the API: `backend/README.md:11` and `README.md:13` document `docker compose up -d --build`; Prometheus is defined under the `observability` profile at `backend/docker-compose.yml:136-142` with `backend/deploy/local/prometheus.yml:1-11` and README's `docker compose --profile observability up -d` (`backend/README.md:34`). | **Grafana**, which the ID requires and which is absent from this repository: `backend/docker-compose.yml` defines no Grafana service and no dashboard provisioning exists anywhere. The activity's Grafana half cannot be started from this repository. |
| `ACT-I2` | Pending | The `/metrics` surface the counter would live on exists and is fully enumerated: `backend/services/risk/internal/messaging/broker.go:71-85` (`MetricsText`) emits `firewatch_http_requests_total`, `firewatch_broker_ready`, `firewatch_events_published_total`, `firewatch_events_processed_total`, `firewatch_events_rejected_total`, `firewatch_dead_letter_depth`, `firewatch_dead_letter_depth_age_seconds`, served at `backend/services/risk/cmd/server/main.go:189` and `:295-299`. `high_risk_predictions_total` does not appear anywhere in the repository. | The counter itself and the observation point at the classification that increments it when the level is **Alto**. Neither is implemented; the current metrics are broker/event counters, not risk-level counters. |
| `ACT-I3` | Partial | Normal authenticated requests can be sent: `backend/scripts/smoke.py:80-112` performs login, health, authorization checks, job creation, idempotent retry, publication wait, latest/history and audit reads against the local stack. | The **controlled artificial latency** injection (no delay, fault-injection or slow-mode setting exists anywhere in the repository) and a load generator to produce the request volume. |
| `ACT-V1` | Blocked | Only one observability reading is recorded, and it is not one of the required signals: `backend/docs/verification.md:21` records a Prometheus `up` query returning `1`. No RPS, p95 latency, memory, error-rate or risk-distribution reading exists. | A running Prometheus **and Grafana** stack (Grafana absent, see `ACT-I1`), a load generator (`ACT-I3`), and the recorded metric series to interpret. The interpretation cannot be produced from this repository as it stands. |
| `ACT-V2` | Blocked | No written decision exists. The repository states the opposite: `backend/docs/synthetic-scenarios.md:47` records that no Go/No-Go or production approval is claimed (the demo handoff that repeated it was retired on 2026-10-17). | The observation from `ACT-V1` to reason over, plus a named person to own the written decision (scale, investigate code/PostGIS, or re-train). The activity is didactic and does not simulate a real emergency, so the written "what would you do" answer is still required and is absent. |

## Summary

- 20 IDs registered: **0 closed**, **0 checkboxes marked**.
- 8 IDs hold partial in-repository evidence: `STK-I1`, `STK-V1`, `CT-I1`, `CT-I3`, `CT-I4`, `GNG-I2`,
  `ACT-I1`, `ACT-I3`.
- 2 IDs are pending, self-contained and not started: `GNG-I1`, `ACT-I2`.
- 10 IDs are blocked on an input this repository cannot supply: `STK-I2`, `CT-I2`, `CT-V1`, `GNG-V1`,
  `GNG-V2`, `GNG-V3`, `GNG-V4`, `GNG-V5`, `ACT-V1`, `ACT-V2`.
- The five approval checkboxes (`GNG-V1`–`GNG-V5`) remain unmarked, and the signature row `GNG-V5` is
  blocked on a named responsible person.

### Grouped missing inputs

1. **Agreed tools, definitions and one architecture review** — the completed per-area tool table with
   justification (`STK-I1`), the tool-to-section-4–6 coverage confirmation (`STK-V1`), and an evaluation
   against the five AWS Well-Architected ML Lens pillars with an owner (`STK-I2`).
2. **Automation stages that do not exist** — a data-validation stage (`CT-I2`), a model benchmark stage
   with measured metrics (`CT-I3`), and a load stage with an agreed target (`CT-I4`).
3. **Repository and deployment gates outside the code** — required status checks / branch protection, a
   production promotion path, and full trigger path coverage (`CT-I1`, `CT-V1`). The CI trigger gap is
   structural, not hypothetical: today's `.gitattributes` and `odd/**` commits to `main` match no workflow
   path filter, so no run was triggered — inferred from the filters and the recorded changed-file sets, with
   the runner not queried.
4. **Reports and approvals** — a real-data validation report (`GNG-V1`), a trained-model evaluation
   (`GNG-V2`), approved infrastructure/SLA results (`GNG-V3`), a deployment decision (`GNG-V4`), the
   Go/No-Go document with its version history (`GNG-I1`), the agreed matrix criteria (`GNG-I2`), and the
   **name and signature of the responsible person** (`GNG-V5`).
5. **Observability stack and measurement for the activity** — Grafana (`ACT-I1`, `ACT-V1`), a load
   generator with controlled artificial latency (`ACT-I3`), a recorded RPS/p95/memory/error/risk-
   distribution series (`ACT-V1`), and a named owner for the written decision (`ACT-V2`).
6. **The CI toolchain items carried from the previous hand-off are resolved in-tree, with their one stale
documentation line corrected in this batch** — the `govulncheck@v1.1.4` panic under Go 1.27.2 was fixed by pinning `v1.9.0`
   (`.github/workflows/_go-service.yml:35-36`), and the provenance/SBOM and image-scan steps succeeded on
   push `c45508e` (`odd/tasks/security/ci-go-1.27.2-green.md:52`, `:131`). `backend/docs/verification.md:68`
   stated the opposite and was corrected in this batch. Neither item is an input this registry could
   supply, and neither is fixed here: `.github/workflows/**` is outside this file's surface.
