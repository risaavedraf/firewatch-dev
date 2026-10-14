# BH-08 — Align OpenAPI errors with observable responses

**Status:** pending | **Priority:** P2 | **Dependencies:** BH-01 for runnable tests; coordinate BH-03; BH-12 is not required.

## Evidence and scope

`contracts/openapi/firewatch-v1.yaml` omits reviewed GET 503 and gateway 502/429 responses. This is a documented contract gap; do not infer that every operation emits every status.

Map applicable operations to actual response status, body, headers, and request identifiers. Reuse shared response schemas and add real contract tests. Fix current parity now; account for BH-12 only if later activated.

## Acceptance checklist

- [ ] Document GET 503 and gateway 502/429 where actually applicable.
- [ ] Use shared error schemas and describe observable request-ID/header behavior without inventing guarantees.
- [ ] Exercise representative real handler/gateway error paths against the contract, not only schema syntax.
- [ ] Keep BH-12 deferred without making centralized handling a prerequisite for parity.

## Verification — future, not run

**Checks status:** pending. Validate the specification and run contract assertions for dependency-unavailable, upstream-failure, and rate-limit responses, including body shape and relevant headers.

## Execution route and next action

**Proposed route:** delegated for response inventory and coordinated specification/contract-test changes.

**Next action:** prepare an operation-to-error matrix from the approved current behavior.
