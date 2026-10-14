# BH-12 — Centralize safe error handling when explicitly activated

**Status:** deferred | **Priority:** P3 | **Dependencies:** explicit activation; BH-08 contract baseline.

## Evidence and scope

Jobs `security.go:53-61` already contains `fail`/`respond` helpers returning a generic code and `request_id`. Preserve useful existing behavior rather than treating error handling as absent.

After activation, normalize service error taxonomy, HTTP status mapping, request IDs, and safe structured logs. Consider a panic boundary only where appropriate; do not leak raw internal errors. Keep observable responses consistent with the contract.

## Acceptance checklist

- [ ] Obtain explicit activation before reading/preparing implementation or changing behavior for this task.
- [ ] Agree cross-service error categories/statuses and request-ID propagation without exposing raw errors or credentials.
- [ ] Preserve safe structured diagnostic context and define panic recovery/logging boundaries if needed.
- [ ] Update contract tests and documentation together for any authorized response changes.

## Verification — future, not run

**Checks status:** pending; task deferred. After activation, test status/body/request-ID parity, safe log output, and any panic boundary. Confirm representative internal errors never escape as raw client-facing messages.

## Execution route and next action

**Proposed route:** delegated only after activation for cross-service preparation and coordinated handler/logging/contract changes.

**Next action:** leave deferred; request explicit activation only when the higher-priority work and contract baseline justify it.
