# Preventive evidence, human review and synthetic reassessment

This document describes the S2 half of the preventive-action contract: how an
administrator records evidence references and a sufficiency criterion, how an
attributable human decides whether that evidence is sufficient, how the
acceptance of an action depends on that decision, and how a later synthetic
reassessment is linked to a real published batch.

It exists so the rules and their limits are stated once, next to the code, rather
than being reconstructed from the routes. The frozen action contract itself
(the eight states and their edges) is unchanged and still documented in
`odd/tasks/presentation-client-ready.md` §«Contrato congelado de prevention».

## What this feature is — and is not

- It is an **MVP with references and human review**. No file is uploaded and no
  URL is fetched. A reference is a bounded citation; the administrator records
  it and a human judges it.
- Sufficiency is a **recorded human decision**, not an automatic rule. The
  service stores who decided and when, from the verified token subject and the
  server clock; neither is client-supplied.
- The reassessment is **synthetic and independent**. It is never presented as
  causal proof that the action reduced risk.
- It adds **no institutional authority**: the service does not verify that a
  cited document exists, and a `suficiente` decision is a person's judgement,
  not a certification.

## Routes

All routes sit under the existing `/v1/prevention/` gateway prefix, so no gateway
wildcard was added. Reads are viewer/admin; every write is admin-only, enforced
in the handler (not the client).

| Route | Who | Purpose |
| --- | --- | --- |
| `POST /v1/prevention/actions/{id}/evidence` | admin | Append one evidence snapshot: a criterion plus 1–10 http(s) references |
| `POST /v1/prevention/actions/{id}/evidence/{evidence_id}/reviews` | admin | Append one human decision (`suficiente`/`insuficiente`) about the current snapshot |
| `POST /v1/prevention/actions/{id}/reassessments` | admin | Link one real published synthetic batch by its server-produced `job_id` |
| `GET /v1/prevention/actions/{id}` | viewer/admin | Existing detail, now also carrying `evidence`, `evidence_reviews`, `reassessments` and `evidence_sufficient` |

The published OpenAPI document (`backend/contracts/openapi/firewatch-v1.yaml`)
is the wire source of truth; this document explains the rules behind it.

## Rules

### Evidence references

A reference is accepted only when it is `http` or `https`, has a host, embeds no
credentials (no `user:password@`) and is at most 2048 bytes. A snapshot needs
1–10 references and a non-blank criterion of at most 2000 characters. Nothing is
fetched, so a reference cannot be used to make the service issue a request.

### Append-only history and stale reviews

Evidence snapshots and reviews are append-only rows. A review always names the
exact snapshot it judged, which is what makes "new evidence invalidates the
earlier approval" a fact and not a convention:

- The detail's `evidence_sufficient` is true only when the **current** (newest)
  snapshot carries a review of `suficiente`.
- A review that names an older snapshot is refused `409 stale_evidence` instead
  of being recorded and silently ignored.
- Adding a snapshot never edits or deletes an earlier one; it only makes an
  earlier approval stop counting.

A terminal action (`aceptada`, `cancelada`) refuses new evidence and reviews with
`409 invalid_evidence_state`: its evidence set is frozen.

### The acceptance guard

`POST /v1/prevention/actions/{id}/transitions` with `to: "aceptada"` is the only
edge whose precondition is not the state machine alone. Inside the same
row-locked transaction that evaluates the edge, the service checks that the
current snapshot carries a sufficient review. Otherwise it answers
`409 {"code":"insufficient_evidence"}` and writes nothing: no status change, no
`updated_at` change, no history row. Every other edge keeps its frozen behaviour.

### Reassessment: only a real, later, same-commune, synthetic batch

The client names **only** the server-produced `job_id`. The service reads the
batch from the shared risk database and derives everything else, so a client can
never fabricate a batch, version or risk level, and can never link another
commune's batch:

| Condition | Answer |
| --- | --- |
| No batch exists for the job yet (not published) | `409 reassessment_unavailable` (retryable) |
| `batches.zone_id` differs from the action's `community_code` | `409 reassessment_commune_mismatch` |
| `batches.demo` is false (real, not synthetic data) | `409 reassessment_not_synthetic` |
| The batch predates the cutoff (below) | `409 reassessment_not_new` |
| The job is already linked | `409 reassessment_already_linked` |

The **cutoff** is the latest of the action's creation, the newest evidence
snapshot and the newest human review. A batch is accepted only when its
`requested_at`, `published_at` and (when present) `evaluated_at` are all at or
after that cutoff, so a batch produced before the review cannot be relabelled as
the reassessment. The `job_id` column is unique, so one real publication can
never be linked twice or under a second action.

The stored `synthetic` value is copied from `batches.demo`; it is never stamped
unconditionally. The stored `causal_proof` is always `false`, and the wire schema
pins it: a synthetic classification is not evidence that the action reduced risk.

### Browser-side originating-action fence

The page binds its reassessment cycle to the action that started it. The cycle
state carries the originating `actionId`, and every late callback (job creation,
polling, link) is fenced by that identity plus the existing generation number and
session; selecting another action clears the local cycle and aborts the in-flight
poll. A retained job can therefore never be sent to the action that replaced it,
and a late success for the first action is never rendered on the second.

This is **local observation only**. It never cancels or deletes the server job,
never changes the endpoint contract and never weakens the backend validation. A
link already requested for the originating action persists on the server, and
returning to that action simply re-reads the server detail; the page does not
fabricate an association or claim a completion it did not observe.

### No-write rejection semantics

Every refusal above is decided before the status update and the history insert,
inside the transaction that owns the action row. An invalid or unauthorized
request therefore changes no state and appends no history. The existing
`409 invalid_transition` / `422 invalid_transition_request` split is unchanged.

## Schema

Migration `011_preventive_evidence.sql` adds three append-only tables in the
`prevention` schema and is registered as version **11** in the shared
`schema_migrations` ledger: risk owns `001..008` and `010`, this service owns
`009`, and `011` is the next free version (version `10` belongs to risk's
`010_models_activation_grant.sql`). Migration `009` is not edited.

| Table | Contents |
| --- | --- |
| `prevention.action_evidence` | One snapshot per row: criterion, `text[]` references, author, instant |
| `prevention.action_evidence_review` | One human decision per row, naming its snapshot, reviewer and instant |
| `prevention.action_reassessment` | One link per row: job, batch, version, risk level, zone, synthetic flag, publication instant |

The application role (`risk_app`) has `SELECT, INSERT` on the three tables and
usage on their sequences. There is no `UPDATE` and no `DELETE`, so the audit
trail cannot be rewritten by the service. There is deliberately **no foreign key
to `batches`**: the prevention and risk migration runners are independent in
compose, so a foreign key would add a startup ordering dependency. The service
validates the real batch before inserting.

## Proof boundaries

- The repository integration tests (`internal/repository/store_integration_test.go`)
  run only when `TEST_DATABASE_URL` is set, as before. They cover the guard, the
  stale-snapshot rule and the four reassessment validations against a real
  migrated database.
- The `reassessment_not_synthetic` branch reads the real `batches.demo` column,
  but the current risk schema constrains `demo` to true, so the branch cannot be
  produced by a normal risk publication today. It is defense-in-depth for a
  future non-synthetic source; the handler test pins the response mapping.
- No browser was executed by the authoring work unit for this feature; jsdom is
  not browser proof. Where a real browser is required, it is run separately.
