# Backend decisions and current boundaries

The first delivery proves a complete asynchronous business path before adding real wildfire inference. Services share **wire contracts only**; they own their persistence, authorization and messaging adapters.

```text
admin -> gateway -> jobs API -> jobs DB (job + audit + outbox)
                                   |
                            confirmed publisher
                                   v
                       RabbitMQ topic exchange
                                   |
                         risk consumer (demo)
                                   v
                       risk DB (batch + inbox + outbox)
                                   |
                       risk.batch.published.v1
                                   v
                         jobs status + audit

viewer -> gateway -> risk API -> last published batch / history
```

## Boundaries

| Module | Owns | Does not own |
|---|---|---|
| gateway | Public API entry (`/v1/*` and health), request IDs, exact-origin CORS for the browser app, aggregate local rate limit, upstream timeouts | Passwords, business storage, static web assets |
| jobs | Admin commands, idempotency, job status, administrative/publication audit | Risk storage or model execution |
| risk | Demo worker, batch publication, latest/history queries, GeoJSON output | Jobs tables or identity credentials |
| contracts | Versioned JSON envelopes and HTTP specification | Shared repositories, authentication utilities or business logic |

`contracts/gen` contains handwritten wire types for now, explicitly documented as such. REST and JSON events are sufficient for this slice; protobuf/gRPC generation is not introduced without a concrete consumer need. Module paths use `firewatch.local` until a repository organization is selected.

The per-service security and messaging adapters are intentionally local rather than a shared runtime SDK. Changes to them must be tested in both consumers. Audit remains transactionally colocated with job state for this slice; a standalone audit projection and `prevention` service are future boundaries, not empty placeholders.

The gateway does not serve the browser application. The page is a separate frontend on its own origin and the gateway answers `404` on `/`; it is an API only. Serving the page from the same origin as the API was the earlier arrangement and was deliberately replaced, which is also how the Kubernetes ingress is configured: it publishes the `/v1/` prefix and never the root page.

## Delivery and consistency

1. An admin request commits the job, audit record and outgoing event in one PostgreSQL transaction. `(actor_id, idempotency_key)` enforces HTTP idempotency. Only one fixed demo request shape is currently supported.
2. Relays lock pending outbox rows with `FOR UPDATE SKIP LOCKED`, publish persistent messages with publisher confirms and mandatory routing, then mark rows published. A crash between confirmation and commit can deliver a duplicate; it cannot legitimately be described as exactly once.
3. Consumers commit inbox records and business effects together, then acknowledge. The risk database also has a unique job constraint. An invalid event is rejected to a dead-letter queue; transient failures have five bounded attempts with exponential waits. Quorum delivery limits additionally bound repeated unacknowledged deliveries across reconnections.
4. A complete synthetic batch and its publication event commit atomically. Queries sort by request time, so an older slow job cannot replace a newer requested batch merely by finishing later.
5. Broker loss leaves reads available and new jobs safely queued in PostgreSQL. Broker reconnect backs off to 30 seconds. PostgreSQL failure rejects writes with `503` instead of acknowledging an unpersisted job.

The development broker has one node. Quorum queue type alone does not provide HA. Dead-lettered jobs currently remain queued; the DLQ is the operational failure record until failure propagation and replay are implemented. Outbox/inbox/audit retention is not automated yet.

## Security and operations

- Keycloak owns passwords and issuance. Every service validates RS256 signature, issuer, audience and expiration; business services also check token type, not-before and role. JWT account revocation is bounded by the five-minute token lifetime rather than synchronous introspection.
- Clients cannot assert identities through forwarding headers. The gateway replaces request IDs, strips caller-supplied identity headers and limits total requests to a 20/s refill with burst 40 **per replica**, not a distributed per-user quota.
- The browser application is a different origin from the API, so cross-origin access is explicit and minimal. The gateway answers CORS for one exactly configured origin (`WEB_ORIGIN`) and fails closed: unset means no CORS headers at all and never a wildcard, and a non-matching `Origin` is not echoed back. Preflights are answered before rate limiting and before authentication, and error responses carry the same headers so the browser sees the real status instead of a generic network failure. This is browser-enforced hygiene only: authorization remains the bearer check below, which the gateway applies to every `/v1/*` request regardless of origin. The single exception is a preflight: an `OPTIONS` from the allowed origin is answered `204` immediately, before the rate limiter and before the bearer check, and without proxying anything, so it reaches no protected operation even though it is not itself limited or authenticated.
- Request bodies, headers, read/write time and SQL query contexts are bounded. Queries are parameterized. Application roles have no DDL permissions; audit writes are insert-only for the jobs application role.
- Service containers run as non-root, without Linux capabilities, with a read-only filesystem. Secrets are generated into ignored local configuration and excluded from Docker build contexts.
- Logs contain identifiers and outcomes rather than tokens, passwords or request bodies. Trace IDs correlate HTTP commands with events and audit records; this is not yet OpenTelemetry distributed tracing.
- Jobs/risk expose internal `/metrics` counters, broker readiness and dead-letter depth/age. `/health/ready` is the Kubernetes readiness probe. Its HTTP code is the routing decision and is decided by the bounded database ping alone: `200` with the operational status body when the ping succeeds, `503` with the same body and a non-empty `reason` when it fails. The body's `ready` field is not that routing decision: it is the full dependency decision, identical to the one the authenticated status routes return, so it can be `false` while the probe answers `200` (for example a degraded broker with a healthy database). Broker connectivity and a dead-letter backlog never change that status code; they stay visible as degradation in that body's `dependencies.broker` and `dead_letter` fields, in the authenticated `/v1/jobs/status` and `/v1/risk/status` routes (which keep answering `200`/`503` from the full dependency decision with a `reason`), and in `/metrics`. `/health/live` checks the HTTP process. Prometheus is optional locally.
- CI checks each service independently, including contract changes, tests, race detection, vet, module integrity, Go vulnerability scanning, runtime-image scanning and image builds with SBOM/provenance requests. Go build/runtime base images and Actions are digest/commit pinned. CI is configured, but no workflow has run yet: the repository was created empty and nothing has been pushed to it. Image signing, registry promotion and automated rollout are later deployment work.

## Relation to the reference documents

The user confirmed **Compose for development and Kubernetes for deployment**, superseding the old single-VM Compose decision. PostgreSQL/PostGIS is approved locally. This implementation depends on PostgreSQL transactions, locking and PostGIS and therefore does not claim Oracle compatibility; the production data-engine decision remains explicit.

The references' 15-minute ingestion cycle, real sources, model approval, inference, preventive actions, production SLA and performance targets remain requirements for subsequent slices. This demonstration proves infrastructure behavior and traceability, not scientific validity or operational readiness.
