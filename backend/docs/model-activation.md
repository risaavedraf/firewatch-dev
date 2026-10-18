# Model activation and inference

This document states which classifier version judges a newly published batch, how
an activation changes that, and what the service refuses. It is the authoritative
description of the contract implemented by `internal/service/classifier_source.go`
and `internal/repository/models.go`.

## The contract

`POST /v1/models/{version}/activate` moves the registry's lifecycle state (`active`,
`approved_at`, and an `activation` record in `metrics`) in one transaction. The
publication path resolves the classifier it runs from that shared registry **once
per publication**, so an activation decides inference in every replica without a
restart.

Before this contract, the registry could say v2 while inference kept running the
classifier selected at startup forever: the panel and the batches disagreed, and
only editing `MODEL_ID` and recreating the container changed anything.

## When an activation takes effect

Precisely, and this is the whole timing contract:

- an activation or a reversal affects **publications whose classifier resolution
  happens after the call**. Those publications run the newly activated version
  and record its identifier and policy;
- a publication that is **already in flight** and resolved its classifier before
  the call is **not** affected. It completes with the one snapshot it resolved,
  which is the same guarantee that keeps a single batch from mixing two versions;
- nothing is swapped underneath a running classification, and no restart is
  involved. The observable consequence is that the registry's `active` row and
  the `model_version` of the newest batch can differ until the next publication is
  resolved after the activation. That difference is expected, not a defect: the
  batch states the version that actually judged it.

## Precedence

There are two sources for a classifier version, and exactly one of them wins:

| Registry state | Classifier used | Why |
| --- | --- | --- |
| An **explicitly activated** version (`metrics.activation` present) | That version | An operator's decision is authoritative. It is resolved through the in-process registry, and a frozen fingerprint must match. |
| An active row **without** an activation record (the row a migration seeds) | The startup selection (`MODEL_ID`, default `academic-rule-v1`) | A seeded `active` flag is registry lifecycle metadata, not a decision. This is what keeps a deployment configured for `firewatch-rule-v2` running v2 until somebody activates a version. |
| **No active row at all** | The startup selection | Same bootstrap rule: nothing has been decided, so the verified startup selection serves. |

The precedence is deliberate: **an activation is never silently overridden by
`MODEL_ID` on restart**, and `MODEL_ID` never silently overrides a recorded
activation either. Adopting a different version is now an API call, not a
recreate. `MODEL_ID` remains the bootstrap and the startup guard: the selected
version must have a row in `models` and, when its identifier is frozen, a matching
`params_hash`, or the service fails closed at startup.

Because the check is on the *activation record* and not on the seeded flag, the
migration seed (`academic-rule-v1`, `active = true`, no `activation` key) does not
pretend to be a decision. Only `ActivateModel` writes `metrics.activation`.

Stated the other way round, because this is the reading that used to be wrong: a
registry row with `active = true` is **not evidence that the running service
selected it**. Before an explicit activation exists, the classifier in use is the
startup bootstrap (`MODEL_ID`), whatever the seeded `active` row says. Only after
an activation record exists does `active` identify the version that publications
will run.

`MODEL_ID` therefore means "startup/bootstrap only until an explicit activation":
it keeps its role as the startup guard (the version must be registered and, when
frozen, fingerprint-matched) and as the fallback while no activation is recorded.
It is not consulted again once an activation exists.

## Fail-closed rules

A publication refuses, before it fetches or persists anything, when:

- the explicitly activated version has **no registered classifier** in the running
  binary (the row names a version this service cannot execute); or
- the activated version's identifier is **frozen** and its stored `params_hash`
  does not match the frozen expectation, so the batch would record a provenance
  the registry contradicts; or
- the registry records **no activation and no usable active row** while no startup
  classifier is configured.

These are deterministic: no retry can register a classifier in a running process,
so the failure is tagged permanent and the job is dead-lettered rather than
blocking the queue through the whole retry budget.

A registry that cannot be **read** is a different thing: it is a dependency
failure. It is reported as a transient error so the consumer retries the job.

## One classifier per publication

The classifier is resolved once per publication and the same instance then serves
every step that names a version:

- `Classify` produces the level, the factors and the rationale;
- the batch's `model_version` is the **producing classifier's** identifier;
- the batch's `policy_ref` (`policy_id`, `parameters`, `params_hash`) is the
  reference the producing classifier reported.

An activation that lands while a publication is running therefore changes what the
**next** batch uses; it can never make one batch carry two versions. The published
version is never copied from the registry row onto a different classifier's
result.

## Operating it

Inspect the registry (viewer or admin):

```bash
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/v1/models
```

Activate v2, then revert to v1 (admin only):

```bash
curl -s -X POST -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/v1/models/firewatch-rule-v2/activate
curl -s -X POST -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/v1/models/academic-rule-v1/activate
```

Every publication that resolves its classifier after each successful call carries
the corresponding `model_version` and `policy_ref`; an already-resolved publication
keeps its in-flight snapshot. The 200 body of the activation is the activated row with its new
lifecycle state; a placeholder is still `409 model_not_activatable` and an unknown
version is still `404 model_not_found`.

## What this does NOT guarantee

- **No actor attribution yet.** `metrics.activation` records the instant of the
  activation. Recording *who* activated a version would need the verified admin
  subject threaded out of the authorization layer into this route; that layer is
  outside this change, so the field is absent rather than fabricated.
- **No hot swap inside a request.** A publication that has resolved its classifier
  completes with that classifier, even if an activation lands while it is running.
  The change takes effect on the next publication whose resolution happens after
  the activation, which is the smallest unit that can be attributed as one version.
  For the same reason, `GET /v1/models` (registry) and the `model_version` of the
  newest batch can legitimately disagree until that next publication.
- **No in-memory cache.** Every publication reads the active row from shared
  storage, which is what makes the registry authoritative across replicas at the
  cost of one small indexed read per publication. Caching would reintroduce the
  staleness this contract removes.
- **No change to `/v1/models/{version}/activate`'s status codes or body.** The
  route still moves registry lifecycle state; what changed is that the publication
  path now reads it.

## Where this is verified

- `internal/service/classifier_source_test.go` — the resolution rules (activation
  wins, seed row keeps the bootstrap, fail-closed cases, transient read failure),
  the v1 → v2 → v1 publication proof, the attribution proof, and the concurrent
  activation snapshot proof.
- `internal/repository/models_test.go` — the activation-record read and the shared
  row scan.
- `internal/repository/store_models_integration_test.go` — the active-model read
  against real transactional activation (disposable database only).
- `internal/service/publisher_integration_test.go` — v1 → v2 → v1 through the real
  store and the real fixture (disposable database only).

The database-backed proofs skip without `TEST_DATABASE_URL`; run them with
`make integration`.
