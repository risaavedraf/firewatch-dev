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

- [~] Agree retry budgets, backoff, permanent-failure handling, and product-visible terminal/recovery states — the budget, backoff and permanent-failure handling are agreed and implemented, and the terminal state is persisted and visible. The **recovery** half (replay) is still pending, so this stays partial.
- [ ] Implement durable recovery/reconciliation and replay without duplicate effects — the durable terminal transition exists, but the cross-service failure report and the operator replay are later units.
- [~] Handle restart and duplicate/reordered delivery safely across persistence and acknowledgment boundaries — the terminal write only transitions a `queued` row, so a duplicate or reordered delivery cannot rewrite a terminal job, and `attempts` is durable on the row instead of living in process memory. Resuming an interrupted retry budget across a restart is deliberately **not** implemented: the policy keeps the in-line budget unchanged.
- [ ] Provide alerts and an operator runbook for exhausted retries, dead letters, and stuck jobs — later unit; the DLQ action is still manual, as `05-observabilidad` recorded.

## Verification — future, not run

**Checks status:** pending. Use isolated infrastructure to interrupt PostgreSQL during valid event consumption, exhaust retries, restore it, and restart consumers. Verify eventual completion for recoverable jobs without duplication; permanent failures reach the agreed observable state.

**Observed in this session (2026-10-16):** unit 1 only. The classification of a give-up as `permanent_error` or `retries_exhausted`, the terminal write on the job's own row with the persisted attempt count, a job with no delivery staying `queued`, and the exposed `failed` shape are covered by passing tests, with the RED captured first. `gofmt` and `go vet` are clean and the OpenAPI document validates. **Not verified:** the migration executing against PostgreSQL, the store SQL, the live AMQP path, and everything the scenario above needs — a database and a broker are required and the Docker daemon is not available.

## Execution route and next action

**Proposed route:** delegated for recovery-policy preparation and coordinated consumer, persistence, broker, and test changes.

**Next action:** implement the agreed policy with tests that fail first — a permanent consumer failure writes `failed` with its reason, a dead letter tied to a `job_id` does too, a job with no answer stays `queued`, and a replay of the same `job_id` produces no duplicate effect. The end-to-end check (interrupt PostgreSQL, exhaust retries, restore, restart consumers) stays deferred: it needs a broker and a database, and the Docker daemon is not available.
