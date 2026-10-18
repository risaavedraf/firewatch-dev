# Scheduler publication: why an automatic cycle emits no job completion

The risk service has two ways to persist a risk batch, and only one of them
belongs to a job:

| Origin | Entry point | Persists the batch | Emits `risk.batch.published.v1` |
| --- | --- | --- | --- |
| Explicitly requested job | `service.Publisher.Publish` → `repository.Store.Publish` | yes | yes, exactly one |
| Internal automatic cycle | `service.Publisher.PublishScheduled` → `repository.Store.PublishScheduled` | yes | no |

An automatic cycle is not a job. The cycle mints its own event, trace and job
identifiers, and Jobs never stored a row for the job id it mints. A
`risk.batch.published.v1` naming that id is therefore refused by the jobs
consumer as an unknown job (`Store.Complete` → `errNoMatchingJob`, permanent) and
dead-lettered immediately. Before this seam existed, every zone of every 15
minute cycle added one permanent dead letter, and with `DLQ_READY_MAX_DEPTH=0`
the service reported itself degraded for a condition no operator could settle.

## The seam

The origin is a **method**, not a request field and not an actor string:

- `cycle.Publisher` declares only `PublishScheduled`, so the cycle has no way to
  ask for a job completion.
- `service.Publisher.PublishScheduled` runs the same pipeline as `Publish`
  (scenario resolution, provider selection, retrieval validation, classification,
  policy-identity check, logging, outcome counter) and hands the finished
  assessment to the store's scheduled seam.
- `service.ScheduledBatchWriter` is a separate interface. A store that cannot
  persist a scheduled publication makes `PublishScheduled` fail closed instead of
  falling back to the completion seam.
- `repository.Store.persist` is the single transaction both origins share: the
  inbox deduplication gate, the idempotent `batches` insert on `job_id`, the
  provenance-derived `demo` flag and the scenario metadata are identical. Only
  the completion step depends on the origin.

Because the decision is the method that is called, `ActorID` stays descriptive
metadata: a real delivery whose actor happens to read `cycle-scheduler` still
settles its job through `Publish`, and a scheduled publication can never settle
one. The jobs consumer is unchanged: `Store.Complete` still rejects an unknown
job id, and nothing swallows unknown jobs globally.

## Invariants this seam preserves

- **One completion per explicitly requested job.** A fresh delivery emits one
  completion naming the stored batch; an operator re-dispatch emits one more
  naming the same stored batch (S6 replay repair).
- **Inbox deduplication.** A redelivery with the same event id commits without a
  second batch and without a second event, on both origins.
- **Idempotence.** A second publication for the same `job_id` never creates a
  second batch row; the scheduled origin commits the duplicate as a no-op.
- **Metadata.** Scenario id and evaluation instant travel untouched into the
  batch for both origins.
- **Snapshot / policy identity.** The frozen policy fingerprint check runs before
  persistence on both origins, so a scheduled batch can never be stored under an
  impersonated policy id.
- **Public wire contract unchanged.** No event type, field or routing key was
  added or renamed.

## Running the checks

Deterministic (no database):

```bash
cd backend
go test -mod=readonly -count=1 -run 'AutomaticCycle|PublishScheduled|PublicationNeverReports' ./services/risk/...
```

The composition check `TestAutomaticCycleCannotGrowTheJobsDeadLetterQueue`
(`services/risk/cmd/server`) wires the cycle exactly as `run()` does and models
the jobs consumer's terminal decision, so it proves the batch is persisted per
zone, zero completions are produced, and an explicit job still settles once.

Against a migrated disposable database, the same rule is proven on the real SQL
path (batch row present, zero `outbox` rows with `routing_key =
risk.batch.published.v1`, explicit job still exactly one):

```bash
cd backend
TEST_DATABASE_URL=postgres://... go test -mod=readonly -count=1 \
  -run 'TestAutomaticCyclePersistsBatchesWithoutACompletionEvent|TestScheduledPublicationPersistsWithoutACompletionEvent' \
  ./services/risk/internal/...
```

Both database tests skip when `TEST_DATABASE_URL` is unset; a skip is not a pass.
