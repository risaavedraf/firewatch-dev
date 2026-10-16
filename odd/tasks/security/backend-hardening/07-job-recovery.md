# BH-07 — Recover jobs after consumer retry exhaustion

**Status:** pending | **Priority:** P1 | **Dependencies:** BH-01; recovery/terminal-state product decision; coordinate BH-10.

## Evidence and scope

The reviewed jobs `broker.go:96-113` exhausts failures with `Reject(false)` while jobs can remain queued. Actual dead-letter handling depends on broker configuration; no successful replay path is established by that call alone.

Define transient versus permanent failures, bounded delayed retries, dead-letter handling, idempotent replay, and reconciliation or terminal-state transitions. Avoid permanent queued jobs without an accountable recovery path.

## Agreed recovery policy (2026-10-16)

Decided before implementation, so these states are a specification rather than a post-hoc description of whatever the consumer happened to do.

**Terminal state.** A job whose event exhausted its delivery budget becomes `failed`, with `failure_reason` and `attempts` persisted on the row and exposed by `GET /v1/jobs/{id}`. `attempts` moves out of process memory into the row: today a consumer restart silently resets the count, so the budget is not what the code claims it is.

**When `failed` is assigned — only on real evidence, never on a clock.**

1. The `jobs` consumer fails permanently (`Permanent() bool`) over its own row. `jobs` owns that row, so it writes the transition directly.
2. A delivery exists in a dead-letter queue tied to that `job_id`. There are two queues today — `v1.RiskQueue + ".dead"` and `v1.JobsQueue + ".dead"` — and a job can die in either without anything reaching its row.

A job that simply never receives an answer stays `queued`. It stops being invisible by observable means instead: a queued-age metric, an alert, and the replay below. A time-based sweep is rejected because it would mark legitimate backlog as failed — with prefetch 1 and a 15-minute cycle, a slow but healthy queue would produce false terminal states.

**Retry budget (unchanged).** Five attempts, `retryBackoff = {1,2,4,8}` seconds, in line. A permanent error skips every retry. The 15-second in-line budget cannot outlast a real database outage, which is precisely why replay is required rather than optional.

**Replay.** An explicit action taken by an authenticated operator, idempotent by `job_id` against the existing inbox unique constraint. Nothing is replayed automatically.

**Observability.** Dead-letter depth and queued-job age are alerted, and the runbook covers exhausted retries, dead letters and stuck jobs.

## Acceptance checklist

- [ ] Agree retry budgets, backoff, permanent-failure handling, and product-visible terminal/recovery states.
- [ ] Implement durable recovery/reconciliation and replay without duplicate effects.
- [ ] Handle restart and duplicate/reordered delivery safely across persistence and acknowledgment boundaries.
- [ ] Provide alerts and an operator runbook for exhausted retries, dead letters, and stuck jobs.

## Verification — future, not run

**Checks status:** pending. Use isolated infrastructure to interrupt PostgreSQL during valid event consumption, exhaust retries, restore it, and restart consumers. Verify eventual completion for recoverable jobs without duplication; permanent failures reach the agreed observable state.

## Execution route and next action

**Proposed route:** delegated for recovery-policy preparation and coordinated consumer, persistence, broker, and test changes.

**Next action:** implement the agreed policy with tests that fail first — a permanent consumer failure writes `failed` with its reason, a dead letter tied to a `job_id` does too, a job with no answer stays `queued`, and a replay of the same `job_id` produces no duplicate effect. The end-to-end check (interrupt PostgreSQL, exhaust retries, restore, restart consumers) stays deferred: it needs a broker and a database, and the Docker daemon is not available.
