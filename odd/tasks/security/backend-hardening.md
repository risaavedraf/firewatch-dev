# Backend hardening follow-up

Documentation only: these tasks record the two reviews; no implementation, installation, dependency upgrade, deployment operation, or commit is authorized. All implementation and verification checks remain pending.

## Evidence boundary

Static findings below do not prove externally exploitable bugs. Production hardening depends on the actual environment and threat prerequisites. The review's handler tests were blocked by the toolchain mismatch, not green; no fresh vulnerability scan or clean security claim exists. Prior checks for other work do not establish this baseline.

Evidence references are review-provided, relative to `backend/` unless a filename alone is given; line numbers are snapshot locators, not newly investigated findings. Never include credentials or secret values in task evidence.

## Tasks and status

Priority denotes suggested sequencing, not a confirmed vulnerability rating: P1 establishes evidence or protects core boundaries/recovery; P2 strengthens contracts and regression coverage; P3 remains deferred.

| Pending check | ID | Task | Priority | Status | Dependencies |
|---|---|---|---|---|---|
| [ ] | BH-01 | [Compatible toolchain and baseline](backend-hardening/01-toolchain-baseline.md) | P1 | pending | None |
| [ ] | BH-02 | [Fresh vulnerability and image scans](backend-hardening/02-vulnerability-scans.md) | P1 | pending | BH-01 for Go scanning |
| [ ] | BH-03 | [Gateway quota isolation](backend-hardening/03-gateway-rate-limits.md) | P1 | pending | BH-01 |
| [ ] | BH-04 | [Broker identities and least privilege](backend-hardening/04-broker-least-privilege.md) | P1 | pending | Target-environment authorization |
| [ ] | BH-05 | [Transport security enforcement](backend-hardening/05-transport-security.md) | P1 | pending | BH-01; coordinate BH-04 |
| [ ] | BH-06 | [Kubernetes egress restriction](backend-hardening/06-kubernetes-egress.md) | P1 | pending | Environment routing/plugin decision |
| [ ] | BH-07 | [Consumer retry and job recovery](backend-hardening/07-job-recovery.md) | P1 | pending | BH-01; recovery-policy decision |
| [ ] | BH-08 | [OpenAPI error parity](backend-hardening/08-openapi-errors.md) | P2 | pending | BH-01; coordinate BH-03 |
| [ ] | BH-09 | [Authentication negative tests](backend-hardening/09-authentication-tests.md) | P2 | pending | BH-01; token-policy agreement |
| [ ] | BH-10 | [Concurrent idempotency tests](backend-hardening/10-concurrent-idempotency.md) | P2 | pending | BH-01; isolated test infrastructure |
| [ ] | BH-11 | [Deployment security validation](backend-hardening/11-deployment-security-verification.md) | P1 | pending | BH-02, BH-04, BH-05, BH-06 |
| [ ] | BH-12 | [Centralized error handling](backend-hardening/12-centralized-error-handling.md) | P3 | deferred | Explicit activation; BH-08 |

## Recommended execution order

1. Authorize BH-01, then establish fresh BH-02 evidence before prioritizing any dependency changes.
2. Prepare BH-04–BH-06 environment decisions; implement BH-03 and BH-07 only after their policies and validation are authorized.
3. Add BH-09/BH-10 regression coverage alongside affected behavior; complete BH-08 without waiting for BH-12.
4. Validate the assembled deployment through BH-11. Leave BH-12 deferred until explicitly activated.

Proposed implementation route is delegated for scoped reading/preparation and coordinated multi-file work; this index does not itself authorize execution. No implementation size or deadline is asserted.

## Next action

Approve a bounded BH-01 execution scope and validation commands. Preserve all unchecked task status until observed evidence supports completion.
