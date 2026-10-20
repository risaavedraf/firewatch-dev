# Operations: objectives, alerts, drift, load and recovery

This document records the operational agreement for the production version and
the artefacts that implement it. It is the runbook for fronts 5 (observability)
and 6 (autoscaling, load, p95/SLO).

**Status of every number here is OBJECTIVE, not measurement.** The values come
from `Referencias/Actividad Dimensionamiento_FireWatch_produccion.md` and from the
team's agreed decisions; a measurement requires running the benchmark and the
recovery test in an isolated environment, which has not been done yet. Anything
measured will be labelled *measured* and dated.

## 1. SLO / SLA objectives

| Dimension | Objective | Notes |
|---|---|---|
| Map query latency (p95) | **< 500 ms** under load | measured end-to-end at the gateway by the k6 benchmark |
| Map query maximum | **2 s** in normal conditions | the k6 `max` threshold |
| Throughput | **4 RPS** peak | scenario of 20 concurrent users, one query every 5 s |
| Availability | **99.5 %** | agreed; not yet measured against a real outage budget |
| Data freshness | **15 min** cycle | plus an update test |
| Recovery time (RTO) | **≤ 15 min** | see §6 |

## 2. Signals exposed by `/metrics`

The `jobs` and `risk` services expose the delivery counters
(`firewatch_http_requests_total`, `firewatch_broker_ready`,
`firewatch_events_{published,processed,rejected}_total`,
`firewatch_dead_letter_depth`, `firewatch_dead_letter_depth_age_seconds`).
The `jobs` service adds `firewatch_queued_job_age_seconds`: how long the oldest
`queued` job has waited, or `-1` when there is no queued job or the bounded read
failed. A job with no answer is surfaced by that signal and the alert in §3
instead of being marked failed by a clock.
The `risk` service adds:

| Layer | Metric | What it answers |
|---|---|---|
| Model | `firewatch_high_risk_predictions_total` | how many assessments reached **Alto** |
| Model | `firewatch_risk_predictions_total{level="bajo\|medio\|alto\|desconocido"}` | the risk distribution |
| Service | `firewatch_http_responses_total{class="2xx\|3xx\|4xx\|5xx"}` | responses by class |
| Service | `firewatch_http_errors_total` | the 5xx series the error alert is written against |
| Service | `firewatch_http_request_duration_seconds_{bucket,sum,count}` | p50/p95 via `histogram_quantile` |
| Infrastructure | `firewatch_process_{goroutines,cpu_seconds_total,memory_bytes}` | process CPU and RAM (no GPU: the model is tabular) |
| Data access | `firewatch_db_pool_{total,acquired,idle,max}_conns` | PostgreSQL/PostGIS pool health |

**Scope (updated 2026-10-18):** the HTTP latency, error and runtime signals are implemented for all four services — `gateway`, `jobs`, `risk` and `prevention` — with the same `internal/metrics` pattern. Their series are collected by Prometheus; what remains unmeasured is the end-to-end p95 under load, which still comes from the k6 benchmark (front 6). `jobs` also exposes the queued-age signal above, which is an availability signal, not a latency one.

## 3. Alerts: threshold and action

Rules live in `deploy/local/alerts.yml` and are loaded by Prometheus
(`rule_files` in `deploy/local/prometheus.yml`). Each rule names a concrete
action, so an alert is never a bare number (`ALR-V3`).

| Alert | Condition | Action |
|---|---|---|
| `FireWatchHighLatencyP95` | p95 > 500 ms for 5 min | investigate code, PostGIS, network, memory **before** scaling |
| `FireWatchHighErrorRate` | 5xx rate > 1 % for 5 min | investigate the route; tell dependency failure from code defect |
| `FireWatchDeadLetterBacklog` | `dead_letter_depth > 0` for 1 min | inspect the DLQ, match the `job_id` to the `failed` row and the log line, then re-drive or replay |
| `FireWatchStuckJobs` | `queued_job_age_seconds > 1200` for 5 min | check the consumer and both DLQs; re-drive or replay; never force terminal |
| `FireWatchBrokerUnavailable` | `broker_ready == 0` for 5 min | check RabbitMQ; degraded, not down |
| `FireWatchDatabasePoolSaturated` | pool acquired ≥ max for 5 min | find the slow query before raising the limit |
| `FireWatchTargetDown` | `up == 0` for 2 min | check the pod and its readiness; covers `gateway`, `jobs`, `risk` and `prevention` |

An Alertmanager is not configured; firing alerts are visible in the Prometheus UI
and as the `ALERTS` series. Routing them is a later integration step.

## 4. Drift: method and frequency (`S3`)

- **Tool:** Evidently AI.
- **Distinguish the two kinds.** *Data drift* is a shift in the input
  distribution (temperature, humidity, wind, vegetation, historical fires).
  *Concept drift* is a shift in the relationship between inputs and the real
  outcome. They need different responses.
- **Frequency:** per batch, and at least daily. Before ground truth exists the
  reevaluation is over input distribution only.
- **Never retrain on a distribution change alone.** Retraining requires evidence
  of quality loss against ground truth (`odd/urgent/04-operacion-monitoreo.md:21`).

The inputs and outputs needed to compute drift are logged by the publication path
(`risk_assessed`, see §7). Ground truth is not logged yet: the real observations
are deferred, so a drift *quality* comparison cannot run until that seam is
filled. The reference sets and the ground-truth join are pending.

## 5. Load benchmark (`S2`, `S5`)

`deploy/load/k6-risk.js` implements the agreed scenario (constant 4 RPS, 20
pre-allocated VUs) with pass/fail thresholds `p(95)<500`, `max<2000`,
`http_req_failed<0.01`.

```
k6 run -e TOKEN="$TOKEN" -e BASE_URL=http://localhost:8080 deploy/load/k6-risk.js
```

The token is a real bearer token from the target realm; it is passed through the
environment and never committed. The run produces the measured p95/p99/RPS/error
rate that §1 currently lacks. **Not executed yet.**

## 6. Autoscaling and recovery (`S3`, `S4`)

- **Autoscaling policy:** `deploy/k8s/base/autoscaling.yaml` adds one
  `HorizontalPodAutoscaler` per service — `gateway`, `jobs`, `risk` and
  `prevention` — 2→6 replicas, scaling on CPU utilisation
  (70 % of the request). Scale-up stabilises for 60 s, scale-down for 300 s so a
  lull does not shrink the pool the next 15-minute cycle needs.
- **Recovery (RTO ≤ 15 min):** the 2-replica deployments, the `PodDisruptionBudget`
  (`minAvailable: 1`) and the Kubernetes rollout already keep the service
  available during a single-replica loss. The recovery test — kill a replica or a
  dependency and measure the time back to healthy — has **not been run**, because
  it needs the target cluster. The objective is ≤ 15 min, aligned with the data
  cycle.

## 7. Logging for model observability (`S1`)

The publication path emits one structured line per persisted assessment:

```
risk_assessed job_id=… zone_id=… provider_id=… synthetic=… model_version=…
             params_hash=… risk_level=… observed_at=… evaluated_at=…
             temperatura_c=… humedad_relativa_pct=… viento_kmh=…
             cobertura_vegetal_pct=… incendios_historicos=…
```

An absent input is logged as `"unknown"`, never as `0` (a missing reading and a
reading of zero are different facts). **Ground truth (`LOG-I3`) is not logged**:
the real observations are deferred, so there is nothing to record and a
placeholder would be a fabricated label. When ground truth arrives it joins to
these records by `job_id`.

## 8. Capacity validation plan (`S5`)

The initial capacity (API/Web 2×8 GB, prediction replica 4 GB, worker 16 GB/4
vCPU) is an **objective** to be validated with load, not a measured result. The
validation is: run the k6 benchmark at 4 RPS and a peak above it, watch
`firewatch_process_*` and `firewatch_db_pool_*`, and confirm the objective in §1
holds with the stated replicas. Per-size justification follows the observed load
(`odd/urgent/04-operacion-monitoreo.md:28`).

## 9. Operator runbook: exhausted retries, dead letters and stuck jobs

The recovery policy this runbook follows is
`odd/tasks/security/backend-hardening/07-job-recovery.md`. A job reaches the
terminal `failed` state only on real evidence, never on a clock: either its own
consumer failed permanently or exhausted its five-attempt budget, or a delivery
tied to it reached a dead-letter queue. The three cases below are the whole
operator surface.

### 9.1 Exhausted retries (a job is `failed`)

- **Signal.** `GET /v1/jobs/{id}` answers `state: "failed"` with `failure_reason`
  and `attempts`. The consumer logs `event_dead_lettered` with the classified
  `reason` and the attempt count, and `firewatch_events_rejected_total` rises.
- **Read it.** `permanent_error` means a retry can never succeed (a missing
  reference, a malformed assessment); `retries_exhausted` means all five attempts
  — one, two, four and eight seconds apart — failed. Only the bounded code is
  stored on the row; the raw error is in the `event_dead_lettered` log line.
- **Act.** Find the cause from the log's `error` field, fix the data or the code,
  then reclaim the job with the admin replay action
  (`POST /v1/jobs/{id}/replay`). Nothing is replayed automatically.

### 9.2 Dead letters

- **Signal.** `FireWatchDeadLetterBacklog` fires on
  `firewatch_dead_letter_depth > 0`; readiness is already degraded. Two queues
  exist, `risk.jobs.v1.dead` and `jobs.results.v1.dead`, one per consumer.
- **Why a delivery is there.** An event exhausted its delivery budget or was
  refused as malformed. The dead letter is the durable record. For the terminal
  cases a consumer can attribute, the job's own row is marked `failed`
  independently of the queue (see 9.1); for the rest there is no row to look at,
  which is why the alert is raised.
- **Act.** Inspect the dead queue, read the `job_id` from the payload, match it to
  the `failed` row and to the `event_dead_lettered` log line, fix the cause, then
  re-drive the delivery or replay the job. Replay is operator-driven, by design.

### 9.3 Stuck jobs (a job is still `queued`)

- **Signal.** `firewatch_queued_job_age_seconds` reports how long the oldest
  `queued` job has waited; `-1` means there is no queued job or the bounded read
  failed. `FireWatchStuckJobs` fires when it exceeds 20 minutes for 5 minutes.
- **Why it happens.** No answer reached the job: a consumer is down, or its
  delivery dead-lettered without the row being updated, or the queue is draining
  slowly. The job is deliberately **not** failed by a clock, so a legitimate
  backlog is never converted into a false terminal state.
- **Act.** Check `firewatch_broker_ready`, `up` and both dead queues. Search the
  dead-letter payloads for the job's `job_id`, and re-drive a delivery you find.
- **When no delivery can be found**, the job is still `queued` and the admin
  replay recovers it: `POST /v1/jobs/{id}/replay` with the body
  `{"confirm":"queued_may_be_in_flight"}` re-dispatches it. The confirmation is
  required because a queued job may still be in flight, and sending it is the
  operator's acceptance that the same work may run twice - so check the dead
  queues above first. Without it the route answers `409
  replay_confirmation_required` and writes nothing. A `published` job is still
  refused (`409 job_not_replayable`): it already has its outcome.
- **What the re-dispatch guarantees.** The row keeps its `requested_at`, so a job
  older than the 15-minute freshness window produces a batch that is already
  `stale`. That is deliberate: nobody observed the zone again, and moving the
  instant would claim a fresh observation. If the assessment had already been
  stored but its result never reached `jobs`, `risk` now re-emits the completion
  for that stored batch instead of swallowing the re-dispatched request, so the
  job reaches `published` in either case; a repeated completion for the same
  batch is idempotent, and no second batch is ever created.

## 10. What is explicitly not done

- No measured SLO/SLA, throughput, p95 or RTO: no benchmark or recovery run yet.
- No cluster rollout or HPA-in-action verification.
- No Alertmanager routing.
- End-to-end p95/RPS under load and the RTO are not measured; the per-service latency/error/runtime series exist for all four services but no load run has used them.
- No drift evaluation, because there is no ground truth yet.
- **A `queued` job's re-dispatch is an operator attestation, not proof.** The
  replay cannot see the broker, so it cannot know whether a copy of the request is
  still in flight: it refuses (`409 replay_confirmation_required`) unless the
  operator confirms, and duplicate *effects* are prevented downstream by the
  completion re-emitted for the batch that already exists. The duplicate *work* -
  one extra assessment when the request really was in flight - is bounded but not
  prevented.
