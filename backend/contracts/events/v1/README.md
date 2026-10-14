# Event contract v1

Transport: persistent JSON messages on the durable `firewatch.events` topic exchange.
The Go wire types are maintained in `contracts/gen/events/v1/events.go` (currently handwritten).

| Routing key | Producer | Subscriber queue | Meaning |
|---|---|---|---|
| `jobs.requested.v1` | jobs outbox | `risk.jobs.v1` | An authenticated admin requested a synthetic demonstration batch. |
| `risk.batch.published.v1` | risk outbox | `jobs.results.v1` | The complete batch committed and is queryable. |

Each envelope includes UUID `id`, `job_id`, `trace_id`, subject `actor_id`, `zone_id`, UTC `occurred_at`, UTC `requested_at`, and `demo: true`. Published events also include UUID `batch_id`.

Delivery is at least once. Event IDs deduplicate deliveries transactionally; job IDs additionally prevent duplicate batches. Database writes and outgoing events share a transaction. Consumers acknowledge only after commit. Breaking changes require a new routing-key version and a migration of subscribers. Unknown or invalid events go to a dead-letter queue; they never publish a batch.

Dataset and model artifacts must be referenced, not placed in message bodies. This demo does not carry actual weather observations or model predictions.

## Optional synthetic scenario metadata

Both request and result envelopes may carry `scenario_id` and `evaluated_at`. Allowed scenario IDs are `mild`, `mixed`, `dry-windy`, `missing-humidity`, `stale`, and `partial-retrieval`; the Go wire vocabulary is `SyntheticScenarioIDs()`.

- Neither field present: preserve the original fixture behavior and omit both fields from legacy JSON.
- Scenario present: the jobs service assigns a UTC `evaluated_at` once, stores it with the job, and reuses that exact instant for idempotent retries. The client cannot supply it. The risk boundary requires a known scenario and a nonzero evaluation instant; result events preserve both fields.
- `evaluated_at` is the synthetic evaluation instant, not a measured observation timestamp, source freshness, or a forecast horizon. Scenario generation determines its own observation cutoff.
- Historical jobs/batches retain null scenario metadata; do not backfill or reinterpret them.

This extension is supported only when all affected **local services are upgraded together**, as agreed for the demo. An older consumer can ignore optional JSON fields and produce the wrong fixture, so mixed old/new consumers are not supported for scenario-bearing messages. Mixed-version deployment would require versioned routing and subscriber migration. The presence of optional fields does not waive event validation, idempotency, or synthetic labeling.
