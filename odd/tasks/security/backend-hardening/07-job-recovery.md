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

## Mechanism (2026-10-16)

The policy above is delivered by the pieces below, fixed here so both halves of the flow are built against one design instead of two guesses.

**The cross-service failure report.** A job can die in either consumer and, today, nothing reaches its row. `risk` cannot write that row — it does not own it — so it reports the terminal failure as an event:

- A new wire type `JobFailedType = "risk.job.failed.v1"` in `backend/contracts/gen/events/v1`, carried in the existing `Event` envelope with two additive optional fields: `failure_reason` and `attempts`. Additive and optional so a legacy message keeps the shape it always had.
- `risk` does **not** publish from its consumer hot path. On give-up it inserts the failure event into its own `outbox` in one statement, and the existing relay publishes it with the same publisher confirms and mandatory routing every other event already uses. If that insert fails the code still rejects the delivery and logs: the dead letter stays the record, and no evidence is invented.
- `jobs` binds its own queue to the new routing key and dispatches on the event type: `batch.published` still completes the job, `risk.job.failed.v1` writes the terminal failure through the writer this unit already introduced. An unrecognised type is rejected rather than silently accepted.
- Topology must therefore carry more than one binding per queue. Both services currently declare exactly one key per queue, so the declaration changes shape in **both** consumers — which the local-adapter decision already requires, and which is why the rule is stated on both sides.

**Operator replay.** `POST /v1/jobs/{id}/replay`, admin-only through the existing authorization, is the only way work comes back: nothing is replayed automatically.

- Only a `failed` job is replayable. The transition is one guarded update (`WHERE id=$1 AND state='failed'`), so two concurrent replays cannot both enqueue; the loser is answered `409` instead of being silently accepted.
- The replay returns the row to `queued` and clears the terminal fields, and writes the audit record and the outgoing `jobs.requested.v1` event in the same transaction, exactly as job creation does.
- Duplicate effects are prevented downstream by the inbox unique constraint that already exists, never by trusting the replay itself.

**Observability.** Dead-letter depth is already alerted. Stuck jobs gain a queued-age signal and an alert, and the runbook covers exhausted retries, dead letters and stuck jobs.

## Acceptance checklist

- [x] Agree retry budgets, backoff, permanent-failure handling, and product-visible terminal/recovery states — agreed and delivered: five attempts with the literal 1/2/4/8 schedule, permanent errors skipping every retry, a persisted `failed` state with its reason and attempt count, and an explicit operator recovery. The literal schedule is pinned against the agreed policy rather than against the implementation.
- [~] Implement durable recovery/reconciliation and replay without duplicate effects — the failures of both consumers now reach the row, and the replay is a single guarded transaction that also writes the audit record and the outgoing request event. Duplicate safety is structural: the replay updates only a `failed` row, so only one of two concurrent replays can enqueue, and downstream duplication is stopped by the existing inbox and batch uniqueness. The database-level observation of both is still pending, so this stays partial rather than satisfied.
- [~] Handle restart and duplicate/reordered delivery safely across persistence and acknowledgment boundaries — a duplicate or reordered delivery can no longer rewrite a terminal row (both the terminal write and the replay transition are guarded), and the attempt count is durable on the row. Resuming an interrupted retry budget across a restart is deliberately **not** implemented: the policy keeps the in-line budget unchanged.
- [x] Provide alerts and an operator runbook for exhausted retries, dead letters, and stuck jobs — the dead-letter backlog rule now names the propagated terminal state and the replay, a stuck-job rule alerts on the oldest queued job with a concrete action, and the operations runbook covers all three cases. The rules are not loaded by a running Prometheus here, so their semantics are unexecuted.

## Verification — future, not run

**Checks status:** pending. Use isolated infrastructure to interrupt PostgreSQL during valid event consumption, exhaust retries, restore it, and restart consumers. Verify eventual completion for recoverable jobs without duplication; permanent failures reach the agreed observable state.

**Observed in this session (2026-10-16):** unit 1 only. The classification of a give-up as `permanent_error` or `retries_exhausted`, the terminal write on the job's own row with the persisted attempt count, a job with no delivery staying `queued`, and the exposed `failed` shape are covered by passing tests, with the RED captured first. `gofmt` and `go vet` are clean and the OpenAPI document validates. **Not verified:** the migration executing against PostgreSQL, the store SQL, the live AMQP path, and everything the scenario above needs — a database and a broker are required and the Docker daemon is not available.

## Execution route and next action

**Proposed route:** delegated for recovery-policy preparation and coordinated consumer, persistence, broker, and test changes.

**Next action:** implement the agreed policy with tests that fail first — a permanent consumer failure writes `failed` with its reason, a dead letter tied to a `job_id` does too, a job with no answer stays `queued`, and a replay of the same `job_id` produces no duplicate effect. The end-to-end check (interrupt PostgreSQL, exhaust retries, restore, restart consumers) stays deferred: it needs a broker and a database, and the Docker daemon is not available.
