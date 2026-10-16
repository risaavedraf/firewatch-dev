# BH-03 — Isolate gateway quotas without removing emergency bounds

**Status:** pending | **Priority:** P1 | **Dependencies:** BH-01; authenticated quota-key and anonymous-control policy.

## Evidence and scope

The reviewed `gateway.go:41-85` uses a shared pre-authentication limiter with burst 40 and refill 20/second. This establishes shared capacity, not a demonstrated production denial of service.

Preserve an emergency aggregate bound while introducing fair authenticated quotas and anonymous controls. Select trustworthy identity sources; never blindly trust forwarded IP headers.

## Agreed quota policy (2026-10-16)

Decided before implementation, so these numbers are a specification rather than a post-hoc description of whatever the code happened to do.

| Layer | Where it applies | Limit | Key |
|---|---|---|---|
| Aggregate (unchanged) | Every request, before authentication | refill 20/s, burst 40 **per replica** | none — one bucket |
| Authenticated | Only after a successful `Verify`, where a real `token.Subject` exists | refill 5/s, burst 10 | `token.Subject` |
| Anonymous | A request that reaches the limiter without a valid bearer token | refill 1/s, burst 5 | none — one shared bucket |

- **Identity source: the verified JWT subject only.** No `X-Forwarded-For`, no `RemoteAddr`, and no trusted-proxy configuration is introduced. A shared anonymous bucket is deliberate and coarse: it bounds the `401` answers produced by unauthenticated noise, not individual clients. Claiming a per-client identity that cannot be verified behind an ingress would be worse than admitting the bucket is shared.
- **Layer 2 cannot exist before authentication.** The identity only exists after `Verify`, so the aggregate remains the only pre-auth bound. An unauthenticated flood still costs one verification per request; what it cannot do is consume a specific actor's quota.
- **Bounded state lifecycle is part of the specification, not an optimization.** Idle actor buckets are evicted and the map has a hard cap, so a stream of distinct subjects cannot grow memory without bound.
- **Health routes and preflight stay exempt.** `/health/live`, `/health/ready` and the allowed-origin `OPTIONS` are answered before the limiter today and keep that behavior. The exemption is recorded here so it is a decision rather than an accident of ordering.
- **Rejection is unchanged:** `429` with `Retry-After: 1` and body code `rate_limited`, which is exactly what BH-08 pins.

## Acceptance checklist

- [ ] Specify aggregate, authenticated, and anonymous limits, trusted proxy handling, and bounded quota-state lifecycle.
- [ ] Prevent one authenticated actor from consuming another actor's dedicated quota.
- [ ] Preserve emergency protection and define health-route behavior explicitly.
- [ ] Keep rejection responses consistent with BH-08 and avoid races under concurrency.

## Verification — future, not run

**Checks status:** pending. Test abuse versus legitimate traffic, actor isolation, anonymous bursts, spoofed forwarded headers, health routes, and concurrent requests using deterministic clocks where feasible.

## Execution route and next action

**Proposed route:** delegated for limiter-policy preparation and coordinated gateway/test changes.

**Next action:** implement the three layers of the agreed policy with tests that fail first — actor isolation (one subject exhausting its burst must not consume another's), the shared anonymous bucket, spoofed `X-Forwarded-For` changing nothing, health and preflight staying exempt, eviction of idle buckets, and the `429` body and `Retry-After` parity with BH-08. The policy is agreed; no further decision blocks the work.
