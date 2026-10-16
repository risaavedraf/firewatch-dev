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

Every service already exposes the delivery counters
(`firewatch_http_requests_total`, `firewatch_broker_ready`,
`firewatch_events_{published,processed,rejected}_total`,
`firewatch_dead_letter_depth`, `firewatch_dead_letter_depth_age_seconds`).
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

**Scope limit:** the HTTP latency, error and runtime signals are implemented for
the `risk` service today (`internal/metrics`). The `gateway` and `jobs` services
still expose only the delivery counters, so their latency and error rate are not
measured yet; the end-to-end p95 comes from the k6 benchmark instead.

## 3. Alerts: threshold and action

Rules live in `deploy/local/alerts.yml` and are loaded by Prometheus
(`rule_files` in `deploy/local/prometheus.yml`). Each rule names a concrete
action, so an alert is never a bare number (`ALR-V3`).

| Alert | Condition | Action |
|---|---|---|
| `FireWatchHighLatencyP95` | p95 > 500 ms for 5 min | investigate code, PostGIS, network, memory **before** scaling |
| `FireWatchHighErrorRate` | 5xx rate > 1 % for 5 min | investigate the route; tell dependency failure from code defect |
| `FireWatchDeadLetterBacklog` | `dead_letter_depth > 0` for 1 min | inspect the DLQ (replay is front 4, not implemented) |
| `FireWatchBrokerUnavailable` | `broker_ready == 0` for 5 min | check RabbitMQ; degraded, not down |
| `FireWatchDatabasePoolSaturated` | pool acquired ≥ max for 5 min | find the slow query before raising the limit |
| `FireWatchTargetDown` | `up == 0` for 2 min | check the pod and its readiness |

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
  of quality loss against ground truth (`urgent/04-operacion-monitoreo.md:21`).

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
  `HorizontalPodAutoscaler` per service, 2→6 replicas, scaling on CPU utilisation
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
(`urgent/04-operacion-monitoreo.md:28`).

## 9. What is explicitly not done

- No measured SLO/SLA, throughput, p95 or RTO: no benchmark or recovery run yet.
- No cluster rollout or HPA-in-action verification.
- No Alertmanager routing.
- Latency/error/runtime signals exist only for the `risk` service.
- No drift evaluation, because there is no ground truth yet.
