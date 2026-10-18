# U4 checklist evidence registry

Scope: unit **U4** (MLOps infrastructure, monitoring and optimisation) of
`Referencias/tareas_verificacion_implementacion_IA.md`, criterion sections **6** (performance/SLA,
drift, scalability and recovery), **7** (hardware/resource decisions, containers and orchestration)
and **8** (logging, project metrics, alerts and response). That document is the authoritative
criterion source; this file references its IDs and does not replace or restate them.

**No ID in this file is closed and no checklist box is marked.** A passing test and a marked box are
two different objects: a passing test observes one bounded behaviour that runs today, while a marked
box requires the ID's closing criterion — reproducible evidence, thresholds agreed with the team, a
named responsible person and, where the criterion says so, real data. A skipped or unavailable check
is not a pass.

Sub-section **3.3** (`OBS-I1`, `OBS-V1`) belongs to unit U1 and is registered in
`backend/docs/checklist-evidence-u1.md`; those two IDs are **not** re-registered here. That file was not
yet present in this worktree when this registry was written, because the unit's own registry was being
produced concurrently; it now exists in the same directory and registers both IDs under its section 3.3,
as the unit split requires.

States used below are the ones the tracking document requires (implemented, partial, proposed,
justified N/A, pending); only **Partial** and **Blocked** are assigned in this unit:

| State | Meaning here |
|---|---|
| **Partial** | An executable in-repository artifact speaks to the ID. It is not the ID's closing criterion. |
| **Blocked** | Closing requires an input this repository does not contain: real data, an agreed threshold, a load run against a live stack, ground truth, or a named owner. |

Pointers below are anchors at the declaration being cited, not whole-file references.

## Standing limit on all synthetic evidence

The synthetic profiles, the built-in fixture and `academic-rule-v1` prove **code behaviour on invented
inputs**. They cannot prove real-data quality, real input distributions, drift on real inputs, model
accuracy/recall/F1, service latency or throughput under real load, availability, recovery time,
representativeness or the absence of leakage. No executed load test, no drift measurement and no ground truth
exists in this repository; alert rules exist and one was observed firing (L27), but no threshold is
measured. Every row below inherits that limit.

## 6.1 System performance (SLA)

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `SLA-I1` | Partial | `backend/docs/operations.md` §1 records the agreed map-query p95 objective: < 500 ms under load, measured end to end at the gateway. | A measured run and responsible-person acceptance; this map-query objective is not a separate trained-model inference benchmark. |
| `SLA-I2` | Partial | `backend/docs/operations.md` §1 and `backend/deploy/load/k6-risk.js` record 4 RPS, with 20 pre-allocated VUs, as the agreed load scenario. | Measured throughput and responsible-person acceptance; the gateway rate limit is a protective cap, not this objective. |
| `SLA-I3` | Partial | `backend/deploy/load/k6-risk.js` selects k6 and implements the agreed scenario; the script has not been executed. | A recorded load run with its measured results; the tool and script exist, the execution does not. |
| `SLA-I4` | Partial | `backend/docs/operations.md` explicitly labels p95, throughput, availability and RTO as objectives, not measurements. | Measured results alongside the objectives; no load or recovery measurement is recorded. |
| `SLA-V1` | Blocked | None. | A live stack and a load-generator run with concurrent users; not executed here. |
| `SLA-V2` | Blocked | Closest artifact: `backend/services/risk/cmd/server/main.go:217` exposes a latency histogram and response classes in all four services, but no load run reads them. The p95/p99/RPS/error figures still come only from the k6 benchmark, which has not been executed. | The load run above, which is what produces p95/p99, RPS and error figures. |
| `SLA-V3` | Blocked | None. | Measured p95/p99, RPS and errors plus the agreed objectives, then a recorded pass/fail verdict. |

## 6.2 Drift monitoring

Cross-reference: criterion sub-section **3.3** (`OBS-I1`, `OBS-V1`) belongs to unit U1 and is
registered in `backend/docs/checklist-evidence-u1.md`; those IDs are not registered here.

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `DRF-I1` | Partial | `backend/docs/operations.md` §4 selects Evidently AI for data versus concept drift. | An implemented evaluation with reference data and responsible-person acceptance; selecting the tool is not running it. |
| `DRF-I2` | Blocked | None. | An agreed statistical test (e.g. KS, Jensen-Shannon) and its threshold for data drift. |
| `DRF-I3` | Partial | `backend/docs/operations.md` §4 specifies evaluation per batch, at least daily, and retraining only with evidence of quality loss. | Ground truth and an executed concept-drift evaluation; the cadence is documented, not observed. |
| `DRF-I4` | Partial | Batch rows persist the produced assessment: `backend/services/risk/migrations/002_models.sql:16` adds `risk_level`, `:17` `factors`, `:18` `rationale`, written at `backend/services/risk/internal/repository/store.go:174`; `backend/services/risk/migrations/007_synthetic_evaluation.sql:21` adds `scenario_id` and `:22` `evaluated_at`; `backend/services/risk/internal/messaging/broker.go:198` logs `event_processed` with event/job/trace ids. | Ground truth and a retention policy; the five inputs are logged via `risk_assessed` (`backend/services/risk/internal/service/publisher.go:230-246`), but the observed outcomes are not. |
| `DRF-V1` | Blocked | None. `backend/services/risk/internal/source/synthetic_scenarios.go:23` defines only synthetic profiles. | Real input distributions to compare against the development distributions. |
| `DRF-V2` | Blocked | None. | Ground truth plus Accuracy/Recall/F1 recomputation; no labelled real data or metric exists. |

## 6.3 Scalability and recovery

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `ESC-I1` | Partial | `backend/deploy/k8s/base/autoscaling.yaml` declares one `HorizontalPodAutoscaler` per service (2→6 replicas, CPU 70 %) and is listed in the kustomization. | Verification in a cluster and a named owner to accept the trigger threshold; the manifest is written, not observed scaling. |
| `ESC-I2` | Partial | `backend/docs/operations.md` §1/§6 records the agreed RTO objective of ≤ 15 min. | A recovery drill measuring actual RTO and responsible-person acceptance; the objective is not a result. |
| `ESC-V1` | Blocked | Closest artifact: `backend/deploy/k8s/base/workloads.yaml:6` and `:92` declare two replicas per service and `:156` a PodDisruptionBudget `minAvailable: 1`, but no cluster run or peak test exists. | A peak-load test against a live cluster demonstrating horizontal scaling; `backend/deploy/k8s/README.md:29` already warns replicas are not measured-availability evidence. |
| `ESC-V2` | Blocked | None. `backend/docs/verification.md:64` records failover and recovery from backups as not established. | The agreed RTO from `ESC-I2` and a live failure drill measuring real recovery time. |

## 7.1 Hardware and resource decisions

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `OPT-I1` | Blocked | Closest artifact: `backend/docs/architecture.md:63` records performance targets as later requirements, without choosing between training-time, inference, scalability or FinOps. | A named owner to state which optimisation purpose applies. |
| `OPT-I2` | Partial | The active classifier is a deterministic rule with no GPU dependency (`backend/services/risk/internal/classify/classifier.go:84` consumes scalar features) and the deployment requests CPU only (`backend/deploy/k8s/base/workloads.yaml:124`). | A documented CPU-versus-GPU hardware decision tied to a measured workload profile, accepted by a named owner. |
| `OPT-I3` | Partial | The application image is a static CPU Go binary on distroless (`backend/services/risk/Dockerfile:3`, `:8`, `:13`); no CUDA/TensorRT/PyTorch/TensorFlow dependency is declared. | An owner's acceptance that no optimised runtime is warranted for this stack, informed by measured load. |
| `OPT-V1` | Blocked | None. | Profiling or measurement of RAM, storage and network to identify bottlenecks; no such run exists. |
| `OPT-V2` | Blocked | None. | Instrumentation or measurement that would show processor idle time waiting on data; none exists. |
| `OPT-V3` | Blocked | Closest artifact: modest resource caps are declared (`backend/deploy/k8s/base/workloads.yaml:124` and `:125`; `backend/docker-compose.yml:16` and `:17`), but utilisation was never measured. | Measured resource utilisation confirming right-sizing and a named owner's sign-off that no costlier hardware is used unnecessarily. |

## 7.2 Containers, orchestration and auto-scaling

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `ORQ-I1` | Partial | Three service Dockerfiles build pinned, non-root, distroless runtime images (`backend/services/risk/Dockerfile:1`, `:13`); Compose applies a hardened baseline (`backend/docker-compose.yml:9`). | A reproducible deployment proof: image tags are placeholders (`firewatch/risk:replace-before-deploy`) and `backend/deploy/k8s/README.md:3` describes the manifests as a staging deployment template rather than a provisioned environment. |
| `ORQ-I2` | Partial | `backend/deploy/k8s/base/workloads.yaml:6` and `:92` declare two replicas per service with liveness/readiness probes (`:38`–`:42`) and PodDisruptionBudgets (`:156`, `:164`, `:172`). | An executed cluster rollout demonstrating replica maintenance and failure recovery; `backend/deploy/k8s/README.md:29` lists rollout and failover as still requiring environment-specific validation. |
| `ORQ-I3` | Partial | `backend/deploy/k8s/base/autoscaling.yaml` exists with per-service HPAs, but no cluster run exercises the scale-up. | An agreed auto-scaling policy accepted by a named owner and a cluster run (same gap as `ESC-I1`). |
| `ORQ-V1` | Partial | `backend/deploy/k8s/README.md:3` and `:29` describe the staging template and its boundaries; `backend/docs/architecture.md` records the service boundaries and consistency design. | A documented benefit-versus-cost evaluation with figures (performance, efficiency, complexity, investment, obsolescence, vendor lock-in) and a named owner's decision. |
| `ORQ-V2` | Blocked | Closest artifact: `backend/deploy/k8s/README.md:29` states replicas are not measured-availability evidence, which is the opposite of a measure-then-scale claim. | A measured baseline and a named owner confirming the scale decision was measurement-driven. |

## 8.2 Logging

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `LOG-I1` | Partial | The persisted assessment emits `risk_assessed` with the five inputs (`backend/services/risk/internal/service/publisher.go:213`, `:230-246`), adapted from observations at `:281`, and the synthetic scenario identity is persisted (`backend/services/risk/migrations/007_synthetic_evaluation.sql:21`); an absent input is logged as `"unknown"`, never `0`. | Real input variables and ground truth are still synthetic or deferred; retention and a named owner remain open. |
| `LOG-I2` | Partial | The assessment output is persisted: `risk_level`, `factors` and `rationale` columns (`backend/services/risk/migrations/002_models.sql:16`–`:18`), written at `backend/services/risk/internal/repository/store.go:174`; `backend/services/risk/internal/messaging/broker.go:198` logs a processed event with ids. | Real outputs and their linkage to observed outcomes; the stored values are synthetic-rule outputs on invented inputs, with no ground-truth key. |
| `LOG-I3` | Blocked | None. No ground-truth source exists anywhere in the repository; the closest statement is `backend/docs/synthetic-scenarios.md:47`, which lists the claims the repository does not make (no trained-model benchmark, fairness metric, drift measurement, load/SLA result, security approval or Go/No-Go signature). | A ground-truth source, its schema and a named owner. |
| `LOG-V1` | Blocked | None. | Ground truth and real inputs to confirm the records permit re-evaluation and change detection; nothing real is recorded. |

## 8.3 Project metrics

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `MET-I3` | Partial | Risk exposes `GET /metrics` (`backend/services/risk/cmd/server/main.go:217`) rendering the delivery counters (`firewatch_http_requests_total`, `firewatch_broker_ready`, `firewatch_events_{published,processed,rejected}_total`, `firewatch_dead_letter_depth{,_age_seconds}` at `backend/services/risk/internal/messaging/broker.go:79`, `:88`) plus latency, response classes, errors and process/pool signals; jobs exposes the same surface (`backend/services/jobs/cmd/server/main.go:146`); gateway and prevention expose it too; Prometheus scrapes all four (`backend/deploy/local/prometheus.yml:8-32`). | Measured p50/p95 from a load run, disk/network signals and model/data quality metrics; the series exist, the measurements do not. |
| `MET-I4` | Partial | Demand, event, HTTP-error, latency and process counters are exposed (see `MET-I3`); no GPU/VRAM metric appears, consistent with a GPU-free stack, and broker/DLQ signals are prioritised. | A documented priority decision accepted by a named owner, plus PostGIS-access and disk/network metrics. |
| `MET-V3` | Blocked | Closest artifact: the latency histogram exists (`MET-I3`); a load run showing RPS rising while latency stays stable does not. | A load run and its latency/RPS readings; the series exist, the measurement does not. |
| `MET-V4` | Blocked | Closest artifact: the p95 histogram exists (`MET-I3`); a baseline run to detect p95 rising without RPS growth does not. | A baseline load run and its p95 readings; the series exist, the measurement does not. |
| `MET-V5` | Partial | Grafana is a service in the `observability` profile with the Prometheus datasource and a versioned 8-panel dashboard under `backend/deploy/local/grafana/` (T27). | A named owner to accept the dashboard as the operational view; the panels are files, not a reviewed runbook. |

## 8.4 Alerts and response

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `ALR-I1` | Partial | `backend/deploy/local/alerts.yml` carries seven rules (p95, 5xx rate, DLQ backlog, stuck jobs, broker down, pool saturated, target down), each with an action-oriented description. | The thresholds are agreed objectives, not measurements, and a named owner must accept them (`backend/docs/operations.md` §1/§3). |
| `ALR-I2` | Partial | `backend/docs/operations.md` §9 is an operator runbook for exhausted retries, dead letters and stuck jobs, including the replay confirmation. | A named owner and an escalation path; the runbook exists, but routing is not configured. |
| `ALR-V1` | Blocked | None. | Ground truth and Accuracy/Recall/F1 evaluation; no labelled real data exists. |
| `ALR-V2` | Blocked | Error, latency and usage signals exist (`MET-I3`); distribution and confidence signals do not. | Distribution and confidence signals and a review owner. |
| `ALR-V3` | Partial | Seven rules exist and `FireWatchDeadLetterBacklog` was observed firing with real DLQ data (L27); each rule names a concrete action. | Evidence that each rule drives its named action end to end; the rules exist and one fired, but the operator response was not exercised as a drill. |

## Summary

- 42 IDs registered: **0 closed**, **0 boxes marked**.
- 23 IDs hold partial synthetic/in-repository evidence: `SLA-I1`, `SLA-I2`, `SLA-I3`, `SLA-I4`,
  `DRF-I1`, `DRF-I3`, `DRF-I4`, `ESC-I1`, `ESC-I2`, `OPT-I2`, `OPT-I3`,
  `ORQ-I1`, `ORQ-I2`, `ORQ-I3`, `ORQ-V1`, `LOG-I1`, `LOG-I2`, `MET-I3`, `MET-I4`, `MET-V5`,
  `ALR-I1`, `ALR-I2`, `ALR-V3`.
- 19 IDs are blocked on an input this repository cannot supply.

### Grouped missing inputs

1. **Threshold validation and acceptance** — p95, throughput, cadence, autoscaling, RTO and
   alert objectives are documented, not measured; the drift test and its threshold (`DRF-I2`)
   still need agreement.
2. **A live load or failure run** — concurrent-user load test and its p95/p99/RPS/error figures
   (`SLA-V1`, `SLA-V2`, `SLA-V3`), peak-scaling proof (`ESC-V1`), recovery drill (`ESC-V2`), and the
   latency/RPS readings the metric interpretations depend on (`MET-V3`, `MET-V4`).
3. **Real data and ground truth** — real input distributions vs development (`DRF-V1`), Accuracy/
   Recall/F1 recomputation (`DRF-V2`, `ALR-V1`), ground-truth logging source (`LOG-I3`), and the
   real-input record that would make logging re-evaluable (`LOG-V1`).
4. **A named owner** — drift-tool selection (`DRF-I1`), optimisation
   purpose and hardware sign-off (`OPT-I1`, `OPT-I2`, `OPT-I3`, `OPT-V3`), container/orchestration
   tradeoff decision (`ORQ-V1`, `ORQ-V2`), metric priority (`MET-I4`), dashboard ownership
   (`MET-V5`) and alert-to-action ownership (`ALR-V3`).
5. **Measurement instrumentation** — bottleneck and data-starvation profiling (`OPT-V1`, `OPT-V2`),
   and PostGIS/disk/network signals and measured latency (`MET-I3`, `MET-I4`, `ALR-V2`).
6. **A measured monitoring loop** — drift monitoring (`DRF-I1`), accepted thresholds (`ALR-I1`),
   dashboard ownership (`MET-V5`) and alert-to-action attestation (`ALR-V3`); Grafana, the dashboard
   and the rules exist, the measurement and owner do not.
