# BH-09 — Add policy-grounded authentication negative tests

**Status:** pending | **Priority:** P2 | **Dependencies:** BH-01; intended token-policy agreement.

## Evidence and scope

The review inspected signed jobs JWT tests and mocked gateway tests. Coverage gaps include bad signatures, wrong algorithms, missing expiration, empty subjects, and JWKS rotation/outage. Missing tests do not establish an authentication bypass.

Define expected acceptance/rejection and availability behavior from the intended token policy, then test the real validation path with controlled keys and a deterministic JWKS endpoint.

## Acceptance checklist

- [ ] Agree algorithm, required-claim, key-refresh, cache, and outage policy before encoding expectations.
- [ ] Test bad signatures, disallowed algorithms, missing `exp`, and empty `sub` with explicit expected outcomes.
- [ ] Cover JWKS rotation, unknown keys, and outages without silently accepting unverified tokens.
- [ ] Retain valid-token controls and ensure gateway mocks are not the sole evidence for authentication behavior.

## Verification — future, not run

**Checks status:** pending. Run deterministic negative and positive cases using isolated signing keys, controlled time, and JWKS responses; distinguish policy disagreement from implementation defects.

## Execution route and next action

**Proposed route:** delegated for token-policy preparation and coordinated validator/gateway test coverage.

**Next action:** confirm the token and JWKS outage policies, then authorize focused tests.
