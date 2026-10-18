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
representativeness or the absence of leakage. No load test, no drift measurement, no ground truth and
no alert has been produced in this repository. Every row below inherits that limit.

## 6.1 System performance (SLA)

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `SLA-I1` | Blocked | Closest artifact: `backend/services/gateway/internal/handler/gateway.go:118` logs one `duration_ms` per gateway request, but it is a log line, not a p95 objective or an aggregation. `backend/docs/architecture.md:63` records production performance targets as later requirements. | An **agreed** p95 inference-latency objective (a FireWatch value, not the source document's `< 200 ms` example), the service boundary it applies to, and a named owner. |
| `SLA-I2` | Blocked | None. `backend/docs/architecture.md:51` describes a 20/s-per-replica gateway limit, which is a protective cap, not a required throughput. | An agreed required throughput figure, the load profile it is measured over, and a named owner. |
| `SLA-I3` | Blocked | None. No JMeter, K6 or Locust tool, config or runner exists in the repository or in `.github/workflows/*.yml`. | A named owner to select and justify a load-testing tool. |
| `SLA-I4` | Blocked | None. | The objectives from `SLA-I1`/`SLA-I2` once agreed, each explicitly labelled objective or measured; nothing exists to label. |
| `SLA-V1` | Blocked | None. | A live stack and a load-generator run with concurrent users; not executed here. |
| `SLA-V2` | Blocked | None. `backend/services/risk/cmd/server/main.go:189` exposes counters, not a latency histogram. | The load run above, which is what produces p95/p99, RPS and error figures. |
| `SLA-V3` | Blocked | None. | Measured p95/p99, RPS and errors plus the agreed objectives, then a recorded pass/fail verdict. |

## 6.2 Drift monitoring

Cross-reference: criterion sub-section **3.3** (`OBS-I1`, `OBS-V1`) belongs to unit U1 and is
registered in `backend/docs/checklist-evidence-u1.md`; those IDs are not registered here.

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `DRF-I1` | Blocked | `backend/docs/synthetic-scenarios.md:47` explicitly states no drift measurement is claimed. | A named owner to select and justify a drift-monitoring tool; none is present. |
| `DRF-I2` | Blocked | None. | An agreed statistical test (e.g. KS, Jensen-Shannon) and its threshold for data drift. |
| `DRF-I3` | Blocked | None. | Ground truth and an agreed re-evaluation frequency for concept drift. |
| `DRF-I4` | Partial | Batch rows persist the produced assessment: `backend/services/risk/migrations/002_models.sql:16` adds `risk_level`, `:17` `factors`, `:18` `rationale`, written at `backend/services/risk/internal/repository/store.go:172`; `backend/services/risk/migrations/007_synthetic_evaluation.sql:21` adds `scenario_id` and `:22` `evaluated_at`; `backend/services/risk/internal/messaging/broker.go:191` logs `event_processed` with event/job/trace ids. | Logging of the actual **input variables** (temperature, humidity, wind, vegetation, history) and of **ground truth**, plus a retention policy; the columns carry synthetic scenario identity and the assessment, not real inputs or observed outcomes. |
| `DRF-V1` | Blocked | None. `backend/services/risk/internal/source/synthetic_scenarios.go:23` defines only synthetic profiles. | Real input distributions to compare against the development distributions. |
| `DRF-V2` | Blocked | None. | Ground truth plus Accuracy/Recall/F1 recomputation; no labelled real data or metric exists. |

## 6.3 Scalability and recovery

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `ESC-I1` | Blocked | None. No HorizontalPodAutoscaler or equivalent exists under `backend/deploy/k8s/base/`. | An agreed auto-scaling policy with its trigger metric and threshold, accepted by a named owner. |
| `ESC-I2` | Blocked | None. | An agreed RTO figure; no RTO is defined anywhere in the repository. |
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
| `ORQ-I3` | Blocked | None. No autoscaler manifest exists. | An agreed auto-scaling policy and a named owner (same gap as `ESC-I1`). |
| `ORQ-V1` | Partial | `backend/deploy/k8s/README.md:3` and `:29` describe the staging template and its boundaries; `backend/docs/architecture.md` records the service boundaries and consistency design. | A documented benefit-versus-cost evaluation with figures (performance, efficiency, complexity, investment, obsolescence, vendor lock-in) and a named owner's decision. |
| `ORQ-V2` | Blocked | Closest artifact: `backend/deploy/k8s/README.md:29` states replicas are not measured-availability evidence, which is the opposite of a measure-then-scale claim. | A measured baseline and a named owner confirming the scale decision was measurement-driven. |

## 8.2 Logging

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `LOG-I1` | Partial | Input observations are adapted to classifier features at `backend/services/risk/internal/service/publisher.go:202` and the synthetic scenario identity is persisted (`backend/services/risk/migrations/007_synthetic_evaluation.sql:21`), but the raw input variables are never logged. | Logging (or durable storage) of the actual input variables per assessment, plus retention and a named owner; only synthetic metadata exists today. |
| `LOG-I2` | Partial | The assessment output is persisted: `risk_level`, `factors` and `rationale` columns (`backend/services/risk/migrations/002_models.sql:16`–`:18`), written at `backend/services/risk/internal/repository/store.go:172`; `backend/services/risk/internal/messaging/broker.go:191` logs a processed event with ids. | Real outputs and their linkage to observed outcomes; the stored values are synthetic-rule outputs on invented inputs, with no ground-truth key. |
| `LOG-I3` | Blocked | None. No ground-truth source exists anywhere in the repository; the closest statement is `backend/docs/synthetic-scenarios.md:47`, which lists the claims the repository does not make (no trained-model benchmark, fairness metric, drift measurement, load/SLA result, security approval or Go/No-Go signature). | A ground-truth source, its schema and a named owner. |
| `LOG-V1` | Blocked | None. | Ground truth and real inputs to confirm the records permit re-evaluation and change detection; nothing real is recorded. |

## 8.3 Project metrics

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `MET-I3` | Partial | Risk exposes `GET /metrics` (`backend/services/risk/cmd/server/main.go:189`) rendering `firewatch_http_requests_total`, `firewatch_broker_ready`, `firewatch_events_{published,processed,rejected}_total` and `firewatch_dead_letter_depth{,_age_seconds}` (`backend/services/risk/internal/messaging/broker.go:68`); jobs exposes the same surface (`backend/services/jobs/cmd/server/main.go:138`); Prometheus scrapes jobs, risk and RabbitMQ (`backend/deploy/local/prometheus.yml:4`) and `backend/docs/verification.md:21` records the Prometheus `up` query returning 1. | Latency (p50/p95), CPU/RAM, disk/network and model/data signals (level counts, input distributions, drift, quality); the endpoint is demand-and-broker counters only. |
| `MET-I4` | Partial | Demand and event counters are exposed (see `MET-I3`: HTTP request totals and published/processed/rejected event counts); no HTTP-error counter is among them, no GPU/VRAM metric appears, consistent with a GPU-free stack, and broker/DLQ signals are prioritised. | CPU, RAM, latency and PostGIS-access metrics, and a documented priority decision accepted by a named owner. |
| `MET-V3` | Blocked | None. | A latency metric and a load run showing RPS rising while latency stays stable; neither exists. |
| `MET-V4` | Blocked | None. | A p95 latency metric and a baseline run to detect p95 rising without RPS growth; neither exists. |
| `MET-V5` | Blocked | None. No Grafana or dashboard artifact exists in the repository. | A dashboard and a named owner; only raw Prometheus counters exist today. |

## 8.4 Alerts and response

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `ALR-I1` | Blocked | None. No alert rules exist in the repository. | Agreed thresholds and sustained-trend durations with a named owner; the source document's example numbers are not project values. |
| `ALR-I2` | Blocked | None. `backend/docs/architecture.md:46` documents DLQ inspection as manual until tooling exists, which is not a response runbook. | A documented response runbook (investigate, escalate, fix, retrain-only-with-evidence) with a named owner. |
| `ALR-V1` | Blocked | None. | Ground truth and Accuracy/Recall/F1 evaluation; no labelled real data exists. |
| `ALR-V2` | Blocked | None. | Distribution, confidence, error, latency and usage signals — which the `MET-I3` counters do not provide today — and a review owner. |
| `ALR-V3` | Blocked | None. No alert exists. | Alert rules, a runbook and evidence that each alert drives a concrete technical action. |

## Summary

- 42 IDs registered: **0 closed**, **0 boxes marked**.
- 10 IDs hold partial synthetic/in-repository evidence: `DRF-I4`, `OPT-I2`, `OPT-I3`, `ORQ-I1`,
  `ORQ-I2`, `ORQ-V1`, `LOG-I1`, `LOG-I2`, `MET-I3`, `MET-I4`.
- 32 IDs are blocked on an input this repository cannot supply.

### Grouped missing inputs

1. **Agreed thresholds and objectives** — p95 latency and throughput (`SLA-I1`, `SLA-I2`), drift test
   and threshold (`DRF-I2`), re-evaluation cadence (`DRF-I3`), auto-scaling policy (`ESC-I1`,
   `ORQ-I3`), RTO (`ESC-I2`, `ESC-V2`), alert thresholds and durations (`ALR-I1`).
2. **A live load or failure run** — concurrent-user load test and its p95/p99/RPS/error figures
   (`SLA-V1`, `SLA-V2`, `SLA-V3`), peak-scaling proof (`ESC-V1`), recovery drill (`ESC-V2`), and the
   latency/RPS readings the metric interpretations depend on (`MET-V3`, `MET-V4`).
3. **Real data and ground truth** — real input distributions vs development (`DRF-V1`), Accuracy/
   Recall/F1 recomputation (`DRF-V2`, `ALR-V1`), ground-truth logging source (`LOG-I3`), and the
   real-input record that would make logging re-evaluable (`LOG-V1`).
4. **A named owner** — load-tool selection (`SLA-I3`), drift-tool selection (`DRF-I1`), optimisation
   purpose and hardware sign-off (`OPT-I1`, `OPT-I2`, `OPT-I3`, `OPT-V3`), container/orchestration
   tradeoff decision (`ORQ-V1`, `ORQ-V2`), metric priority (`MET-I4`), dashboard (`MET-V5`), response
   runbook (`ALR-I2`) and alert-to-action ownership (`ALR-V3`).
5. **Measurement instrumentation** — bottleneck and data-starvation profiling (`OPT-V1`, `OPT-V2`),
   missing latency/CPU/RAM/PostGIS metrics (`MET-I3`, `MET-I4`, `ALR-V2`).
6. **A monitoring/alerting surface** — drift monitoring (`DRF-I1`), dashboards (`MET-V5`) and alert
   rules (`ALR-I1`, `ALR-V3`); the repository currently has Prometheus scraping only, with no Grafana,
   dashboard or alert-rule artifact.
