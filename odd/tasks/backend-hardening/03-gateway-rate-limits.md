# BH-03 — Isolate gateway quotas without removing emergency bounds

**Status:** pending | **Priority:** P1 | **Dependencies:** BH-01; authenticated quota-key and anonymous-control policy.

## Evidence and scope

The reviewed `gateway.go:41-85` uses a shared pre-authentication limiter with burst 40 and refill 20/second. This establishes shared capacity, not a demonstrated production denial of service.

Preserve an emergency aggregate bound while introducing fair authenticated quotas and anonymous controls. Select trustworthy identity sources; never blindly trust forwarded IP headers.

## Acceptance checklist

- [ ] Specify aggregate, authenticated, and anonymous limits, trusted proxy handling, and bounded quota-state lifecycle.
- [ ] Prevent one authenticated actor from consuming another actor's dedicated quota.
- [ ] Preserve emergency protection and define health-route behavior explicitly.
- [ ] Keep rejection responses consistent with BH-08 and avoid races under concurrency.

## Verification — future, not run

**Checks status:** pending. Test abuse versus legitimate traffic, actor isolation, anonymous bursts, spoofed forwarded headers, health routes, and concurrent requests using deterministic clocks where feasible.

## Execution route and next action

**Proposed route:** delegated for limiter-policy preparation and coordinated gateway/test changes.

**Next action:** agree quota identities and emergency/health behavior before writing tests and implementation.
