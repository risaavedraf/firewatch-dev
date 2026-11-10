# Observability measurement — isolated Compose stack

This document records one **measurement** of the FireWatch service signals: RPS,
p50/p95 latency, process memory, goroutines, CPU, response classes and 5xx
errors, scraped by Prometheus from the services' own `/metrics` endpoints. It
also records the `promtool` validation of the alert rules.

The figures here are **measurements of one run**, not objectives, not SLOs and
not a capacity baseline. The agreed thresholds live in
`deploy/local/alerts.yml` and stay objectives until the load benchmark of front 6
runs.

## Environment

| Component | Version / identity |
|---|---|
| Docker engine | Docker Desktop, server `29.5.3` (linux/amd64) |
| Compose project | `firewatch`, network `firewatch_backend` |
| Prometheus | `prom/prometheus:v3.2.1` |
| promtool | `3.2.1` (revision `804c49d58f3f3784c77c9c8ec17c9062092cae27`) |
| Grafana | `grafana/grafana:11.5.2` |
| Scraped targets | `gateway:8080`, `jobs:8080`, `risk:8080`, `prevention:8084`, `inference:8080` |
| Database | `postgis/postgis:17-3.5` |
| Identity provider | `quay.io/keycloak/keycloak:26.5.7` |
| Host | one developer machine, single Docker engine, no orchestrator |
| Date (UTC) | 2026-11-09 |
| Build basis | Images built about three weeks before the measurement. They predate migrations `012_readings.sql` and `013_ml_fixture_v1.sql`, so that stack's ledger stopped at 11 (see `schema-inventory.md`); every metric family measured here was already present in those builds, and a stack rebuilt from `HEAD` exposes the same families. |

Scrape interval is `15s` (`deploy/local/prometheus.yml`). Every query below was
evaluated over a `5m` window, which is the window the alert rules use.

## How to reproduce

From `backend/`:

```bash
export PATH="/c/Program Files/Docker/Docker/resources/bin:$PATH"   # or your docker in PATH
docker compose -f docker-compose.yml --profile observability up -d
curl -sf http://127.0.0.1:9090/-/ready          # blocks until Prometheus answers
curl -s http://127.0.0.1:9090/api/v1/query --get --data-urlencode 'query=up{job=~"jobs|risk|gateway|prevention|inference"}'
```

`up == 1` for all five jobs is the precondition: Prometheus is reaching every
service, so a missing series downstream is the service's fault, not the scrape's.

The rules were validated inside the running container, against the exact file
Prometheus loads:

```bash
docker compose exec -T prometheus promtool check rules  /etc/prometheus/alerts.yml
docker compose exec -T prometheus promtool check config /etc/prometheus/prometheus.yml
```

Both commands ran under `MSYS_NO_PATHCONV=1` on this host; without it, Git Bash
rewrites `/etc/prometheus/...` into a Windows path and `promtool` reports the file
as missing.

Traffic was generated from the Compose network itself, so no service port had to
be published:

```bash
docker run --rm --network firewatch_backend curlimages/curl:8.12.1 sh -c '
end=$(( $(date +%s) + 120 )); i=0
while [ $(date +%s) -lt $end ]; do
  i=$((i+1))
  curl -s -o /dev/null --max-time 3 http://gateway:8080/health/live
  curl -s -o /dev/null --max-time 3 http://gateway:8080/v1/jobs
  curl -s -o /dev/null --max-time 3 http://jobs:8080/health/live
  curl -s -o /dev/null --max-time 3 http://jobs:8080/v1/jobs
  curl -s -o /dev/null --max-time 3 http://risk:8080/v1/risk/status
  curl -s -o /dev/null --max-time 3 http://prevention:8084/v1/prevention/actions
  curl -s -o /dev/null --max-time 3 -X POST -H "Content-Type: application/json" \
    -d "{}" http://inference:8080/predict
  sleep 0.1
done'
```

## Rule validation

```
Checking /etc/prometheus/alerts.yml
  SUCCESS: 7 rules found

Checking /etc/prometheus/prometheus.yml
  SUCCESS: 1 rule files found
 SUCCESS: /etc/prometheus/prometheus.yml is valid prometheus config file syntax

Checking /etc/prometheus/alerts.yml
  SUCCESS: 7 rules found
```

The seven rules are `FireWatchHighLatencyP95`, `FireWatchHighErrorRate`,
`FireWatchDeadLetterBacklog`, `FireWatchStuckJobs`,
`FireWatchBrokerUnavailable`, `FireWatchDatabasePoolSaturated` and
`FireWatchTargetDown`. `promtool` proves they parse and are semantically valid
PromQL against the rule schema; it does not prove they fire.

## Load driven

| Run | Request pattern | Requests issued |
|---|---|---|
| 1 | Gateway only: `/healthz`, `/v1/jobs`, `/v1/risk/status`, `/v1/prevention/actions`, `/v1/no-existe`, `/v1/models` | 2376 (396 cycles) |
| 2 | All five services: `gateway/health/live`, `gateway/v1/jobs`, `jobs/health/live`, `jobs/v1/jobs`, `risk/v1/risk/status`, `prevention/v1/prevention/actions`, `POST inference/predict` | 6006 (858 cycles) |

No request carried a token, so the authenticated routes answered `401` and the
unrouted ones `404`. That is deliberate and it bounds the measurement: what was
measured is the HTTP middleware path — routing, auth rejection, response
recording — not the handlers behind the token. See "What this does not prove".

## Results

### RPS

`sum(rate(firewatch_http_responses_total[5m])) by (job)`:

| Job | RPS |
|---|---|
| gateway | 8.95 |
| jobs | 5.84 |
| risk | 2.98 |
| prevention | 2.95 |
| inference | 2.93 |

`sum(rate(firewatch_http_requests_total[5m])) by (job)` returns `risk 3.01` and
`jobs 5.85` only: those two publish that family, while the gateway publishes the
class-labelled `firewatch_http_responses_total`. The alert rule divides by the
latter for exactly this reason, so no ratio is left empty.

### Latency

`histogram_quantile(0.95, sum(rate(firewatch_http_request_duration_seconds_bucket[5m])) by (le, job))`:

| Job | p50 | p95 |
|---|---|---|
| gateway | 2.5 ms | 4.75 ms |
| jobs | 2.5 ms | 4.75 ms |
| risk | 2.5 ms | 4.75 ms |
| prevention | 2.5 ms | 4.75 ms |
| inference | 2.5 ms | 4.75 ms |

Both figures are histogram bucket boundaries, not interpolated interior values:
the observed distribution sits in the first two non-zero buckets, so the honest
reading is `p50 ≤ 2.5 ms` and `p95 ≤ 4.75 ms` at this load.

### Process signals

| Job | Memory (`firewatch_process_memory_bytes`) | Goroutines | CPU seconds (`firewatch_process_cpu_seconds_total`) |
|---|---|---|---|
| gateway | 13 211 912 | 6 | 803.92 |
| jobs | 13 211 912 | 15 | 792.18 |
| risk | 17 406 216 | 16 | 741.95 |
| prevention | 12 613 896 | 7 | 683.33 |
| inference | 12 613 896 | 6 | 719.83 |

Memory is `/memory/classes/total:bytes` read from `runtime/metrics`
(`services/*/internal/metrics/process.go`). Two services reporting the same byte
count is the runtime's own quantization of mapped memory, not a shared or frozen
value: `risk` moved `12 622 088 → 17 406 216` and `jobs` moved
`12 015 880 → 13 211 912 → 13 211 912` across scrapes during the same session.
CPU seconds are process-lifetime totals since container start, not per window.

### Errors and response classes

`firewatch_http_errors_total` is `0` for all five jobs, and
`firewatch_http_responses_total` by class totals:

| Job | 2xx | 3xx | 4xx | 5xx |
|---|---|---|---|---|
| gateway | 860 | 0 | 3209 | 0 |
| jobs | 887 | 0 | 860 | 0 |
| risk | 28 | 0 | 860 | 0 |
| prevention | 28 | 0 | 860 | 0 |
| inference | 28 | 0 | 818 | 0 |

The `2xx` counts of `risk`, `prevention` and `inference` are their own `/metrics`
scrapes (one every 15 s, plus the readiness probes). Counters are cumulative
since container start; RPS above is the rate over the window.

### Supporting series

`up{job=~"jobs|risk|gateway|prevention|inference"} == 1` for all five targets.
`firewatch_broker_ready == 1` on `jobs` and `risk`.
`firewatch_db_pool_acquired_conns == 0` on `risk`, the one service that publishes
the pool families: the traffic never entered a handler that holds a connection.

`firewatch_high_risk_predictions_total` and
`firewatch_risk_predictions_total{level=...}` are registered and exposed, but
they stayed empty: filling them needs a real classification, which needs a token
and a job flow, not an unauthenticated HTTP probe.

## What this does not prove

- **Handler latency.** Every authenticated route answered `401` before reaching
  its handler. The measured p95 is the middleware's, not the business path's.
- **The error signal under failure.** No request produced a `5xx`, so
  `firewatch_http_errors_total` was verified as exposed and zero. The alert
  `FireWatchHighErrorRate` reaching `> 0.01` was not observed.
- **That any alert fires.** `promtool` proves the rules are valid; nothing here
  proves a threshold crossing, a `for` window elapsing or a notification
  arriving. There is no Alertmanager in this stack by design.
- **Capacity, throughput ceilings or a capacity baseline.** One machine, one
  engine, 858 sequential cycles. The `p95 < 500 ms`, `max < 2000`,
  `http_req_failed < 0.01` verdicts belong to the k6 benchmark of front 6
  (`odd/plan-produccion/06-autoscaling-carga-slo.md`), which has not run.
- **Autoscaling or recovery.** No Kubernetes, no HPA, no RTO measurement.
- **Model quality or drift.** Those need ground truth, which does not exist yet
  (`LOG-I3`, front 2).
