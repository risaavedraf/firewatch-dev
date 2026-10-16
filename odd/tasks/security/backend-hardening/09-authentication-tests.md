# BH-09 — Add policy-grounded authentication negative tests

**Status:** implemented — verified locally | **Priority:** P2 | **Dependencies:** BH-01 (met); the token policy is defined in production code (see below).

## Evidence and scope

The review inspected signed jobs JWT tests and mocked gateway tests. The gaps it named — bad signatures, wrong algorithms, missing expiration, empty subjects, and JWKS rotation/outage — were **not** covered: the existing `TestCreateAuthorizationAndValidation` covers missing token, wrong audience/issuer, expired, future `nbf` and wrong token type, but its `badKey` case is about `Idempotency-Key`, not a signing key.

## Token policy (already fixed by production code, not newly decided here)

| Aspect | Source |
|---|---|
| Algorithm: RS256 only | `services/jobs/cmd/server/main.go:116`, `services/risk/cmd/server/main.go:127`, `services/gateway/cmd/server/main.go:48` — `oidc.Config{SupportedSigningAlgs: []string{"RS256"}}` |
| Keys: remote JWKS | same lines — `oidc.NewRemoteKeySet` |
| Issuer / audience | `oidc.NewVerifier(issuer, keys, &oidc.Config{ClientID: OIDC_AUDIENCE})` |
| Token type must be `Bearer` and `sub` non-empty | `services/jobs/internal/handler/security.go` and the risk equivalent |
| Expiry / `nbf` | go-oidc default checks (no `SkipExpiryCheck`) plus the handler's `nbf` guard |

No new policy is invented: every expectation below cites one of these.

## Acceptance checklist

- [~] Agree algorithm, required-claim, key-refresh, cache, and outage policy before encoding expectations.
  - Algorithm, issuer/audience, `typ`/`sub` and expiry are already fixed by the production config above, and the tests assert exactly those. Key-refresh/cache remain go-oidc's `RemoteKeySet` defaults; the outage behaviour is asserted (fail closed). **Residual:** the team has not formally ratified this policy in writing — the tests pin code-defined behaviour.
- [x] Test bad signatures, disallowed algorithms, missing `exp`, and empty `sub` with explicit expected outcomes — `auth_negative_test.go`: a signature from a key outside the set, RS512 where RS256 is the only allowed algorithm, a token with no `exp`, and a token with `sub: ""` all must yield 401.
- [x] Cover JWKS rotation, unknown keys, and outages without silently accepting unverified tokens — an unknown key (key absent from the set) and an unreachable JWKS are both asserted to fail closed with 401. A genuine rotation (a new key appearing) is not simulated; the equivalent "key not in the set" class is.
- [x] Retain valid-token controls and ensure gateway mocks are not the sole evidence — a valid-token control asserts 200, so the rejections are not vacuous, and the tests drive the real handler `Authorize` rather than a mocked gateway.

## Verification — run locally

From `backend/services/jobs`, with `GOWORK=off`:

```
go test -run TestAuthorize -count=1 -v ./internal/handler/
```

Observed: `TestAuthorizeRejectsTokensThatViolateTheConfiguredPolicy` (9 rejections + valid-token control) and `TestAuthorizeFailsClosedWhenTheKeySetIsUnavailable` all PASS; the module suite is green.

These tests pin **existing** behaviour, so no RED was expected or produced: no production behaviour was changed. The valid-token control is the anti-vacuity guard.

## Execution route and next action

**Route:** inline (test-only addition; no production change).

**Next action:** none required to close the coverage gap; the written policy ratification in the checklist remains a human item.

## Log

- **2026-10-16** — Added `auth_negative_test.go` over the real `Authorize` path for the review's named gaps, citing the production policy for each expectation. All cases pass, including the valid-token control and the fail-closed JWKS case.
