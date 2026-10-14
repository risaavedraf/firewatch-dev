# BH-07 — Recover jobs after consumer retry exhaustion

**Status:** pending | **Priority:** P1 | **Dependencies:** BH-01; recovery/terminal-state product decision; coordinate BH-10.

## Evidence and scope

The reviewed jobs `broker.go:96-113` exhausts failures with `Reject(false)` while jobs can remain queued. Actual dead-letter handling depends on broker configuration; no successful replay path is established by that call alone.

Define transient versus permanent failures, bounded delayed retries, dead-letter handling, idempotent replay, and reconciliation or terminal-state transitions. Avoid permanent queued jobs without an accountable recovery path.

## Acceptance checklist

- [ ] Agree retry budgets, backoff, permanent-failure handling, and product-visible terminal/recovery states.
- [ ] Implement durable recovery/reconciliation and replay without duplicate effects.
- [ ] Handle restart and duplicate/reordered delivery safely across persistence and acknowledgment boundaries.
- [ ] Provide alerts and an operator runbook for exhausted retries, dead letters, and stuck jobs.

## Verification — future, not run

**Checks status:** pending. Use isolated infrastructure to interrupt PostgreSQL during valid event consumption, exhaust retries, restore it, and restart consumers. Verify eventual completion for recoverable jobs without duplication; permanent failures reach the agreed observable state.

## Execution route and next action

**Proposed route:** delegated for recovery-policy preparation and coordinated consumer, persistence, broker, and test changes.

**Next action:** approve the retry/replay and terminal-state policy before behavior-level tests.
