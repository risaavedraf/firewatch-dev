# BH-04 — Separate broker identities and enforce route permissions

**Status:** implemented and validated on the development Compose broker; validation against the target deployment broker remains an explicit deployment gate | **Priority:** P1 | **Dependencies:** authorized target environment; coordinate BH-05.

## Evidence and scope

The broker used to run on one shared identity (`firewatch`) that both `jobs` and `risk` used for everything, and both services declared the full topology, including each other's queue. Either service could publish the other's events and consume the other's work; nothing but convention prevented it.

That is now split. Two publish exchanges, one per producing service, and one identity per service whose authority is a narrow permission triple. Measured on the development Compose broker, 2026-11-10.

## Why the exchange had to split

The broker authorises by **resource name only, never by routing key**. On one shared topic exchange a rule like "`jobs` may publish only `jobs.requested.v1`" cannot be expressed: any identity with `write` on the exchange could publish every event type to it, including a forged batch result. Splitting the exchange turns that rule into a permission on a name, which the broker does enforce.

The same constraint explains two other decisions:

- **Cross-bindings are created by the provisioner, not by the services.** Binding a queue requires `write` on the exchange and `read` on the queue, so a consumer that binds itself to its peer's exchange would need write there — undoing the isolation the split provides.
- **No service holds `write` on `firewatch.dead`.** Dead-lettering is performed by the broker on its own authority, not under the service identity, so no service needs it; granting it would let one service forge a dead letter straight into its peer's queue.

## Required operations per service

| Service | Publishes | Consumes | May configure |
| --- | --- | --- | --- |
| `jobs` | `firewatch.jobs.events` (`jobs.requested.v1`) | `jobs.results.v1` | `firewatch.jobs.events`, `jobs.results.v1`, `jobs.results.v1.dead` |
| `risk` | `firewatch.risk.events` (`risk.batch.published.v1`, `risk.job.failed.v1`) | `risk.jobs.v1` | `firewatch.risk.events`, `risk.jobs.v1`, `risk.jobs.v1.dead` |

Effective permission triples, read back from the broker:

```
firewatch-jobs  configure ^(firewatch\.jobs\.events|jobs\.results\.v1|jobs\.results\.v1\.dead)$
                write     ^firewatch\.jobs\.events$
                read      ^jobs\.results\.v1$
firewatch-risk  configure ^(firewatch\.risk\.events|risk\.jobs\.v1|risk\.jobs\.v1\.dead)$
                write     ^firewatch\.risk\.events$
                read      ^risk\.jobs\.v1$
```

Neither service holds a management tag; their only authority is the triple above. The `firewatch` identity keeps `.* / .* / .*` and is the provisioning and operations identity — no service uses it any more.

Five bindings, all created by the provisioner: `risk.jobs.v1` ← `firewatch.jobs.events` on `jobs.requested.v1`; `jobs.results.v1` ← `firewatch.risk.events` on `risk.batch.published.v1` and on `risk.job.failed.v1`; and each `.dead` companion ← `firewatch.dead` on its own queue name.

## Provisioning and credentials

`deploy/local/rabbitmq-topology.sh` owns the exchanges, queues, bindings, identities and permissions. It is a one-shot `broker-topology` compose service that `jobs` and `risk` depend on with `service_completed_successfully`; it is idempotent and safe on every `up`.

The services no longer declare topology. They verify it passively at startup (`ExchangeDeclarePassive` on their own publish exchange, `QueueDeclarePassive` on their own queue and dead queue), so a missing or mistyped resource fails immediately, naming it, instead of failing later mid-delivery.

Credentials never enter the repository. The development file derives `RABBITMQ_PASSWORD-jobs` and `RABBITMQ_PASSWORD-risk` from the single secret it already requires, so a fresh clone needs no new variables; the production overlay requires dedicated `RABBITMQ_JOBS_PASSWORD` and `RABBITMQ_RISK_PASSWORD` instead. Rotation is one provisioning run with the new values, plus a restart of the affected service — and the provisioner re-PUTs **both** identities' passwords on every run, so rotating one requires the other's current value in the same environment. Otherwise the other service's password is silently reset and it fails at its next reconnect.

## Allow and deny evidence

Four checks against the real broker, in `services/jobs/internal/messaging/broker_permissions_test.go`. Opt-in like the migration tests: without `BH04_JOBS_URL` and `BH04_RISK_URL` they skip, so the default suite stays offline.

| Check | Operation | Result |
| --- | --- | --- |
| `TestBrokerRefusesPeerExchangePublish` | `firewatch-risk` publishes `jobs.requested.v1` into `firewatch.jobs.events` | `ACCESS_REFUSED`, names the exchange and the write access |
| `TestBrokerRefusesDeadExchangePublish` | `firewatch-jobs` publishes into `firewatch.dead` | `ACCESS_REFUSED`, names the exchange and the write access |
| `TestBrokerRefusesPeerQueueConsume` | `firewatch-jobs` consumes `risk.jobs.v1` | `ACCESS_REFUSED`, names the queue and the read access |
| `TestBrokerAllowsOwnExchangePublish` | each identity publishes on its own exchange, routing key matching no binding | publisher confirm acked; nothing enqueued |

```
--- PASS: TestBrokerRefusesPeerExchangePublish (0.01s)
--- PASS: TestBrokerRefusesDeadExchangePublish (0.00s)
--- PASS: TestBrokerRefusesPeerQueueConsume (0.00s)
--- PASS: TestBrokerAllowsOwnExchangePublish (0.01s)
ok  firewatch.local/jobs/internal/messaging
```

The allow case is the control: without it, the three denials could equally be explained by a broken identity rather than a narrow one. Its publish uses a prefixed routing key that matches no binding, so the running consumers never see a probe message.

Provisioning output, sanitized — the script prints no credential:

```
provisioning: exchanges
provisioning: queues
provisioning: bindings
provisioning: identities
provisioning: permissions
provisioning complete: 2 identities, 3 exchanges, 4 queues, 5 bindings
```

## Broker-specific limitations

- **No routing-key permissions.** Enforced by resource name only; the split exchange is the workaround, and it is why publishing "the wrong event type on the right exchange" remains possible for a service against its own exchange. A service can only forge events it is already entitled to produce.
- **The management API needs a management tag.** Measured: a service identity gets `401 {"error":"not_authorised","reason":"Not management user"}` before any permission check. Operators therefore keep using the `firewatch` identity for management-API work, and that identity is never distributed to a service.
- **`configure` is not read-only authority.** The services need it for their passive checks, and it also lets a compromised service delete its own queue or exchange. Its peer's objects stay intact, but the peer's pipeline can be starved: deleting `firewatch.jobs.events` stops work reaching `risk`, and deleting `jobs.results.v1` stops `risk`'s outbox ever draining. This is availability, not authority, and it is recorded instead of denied.
- **Development credentials are not secret-independent.** Both service passwords derive from the single `RABBITMQ_PASSWORD`, and the admin password is that value without the suffix, so a compromised development service can recover it. What development proves is the permission triple, which is the mechanism this unit installs; it does not prove secrecy of the admin password. Production uses independent `RABBITMQ_JOBS_PASSWORD` and `RABBITMQ_RISK_PASSWORD`.

## Safe rollout

The outbox makes the exchange switch safe: unpublished events stay in the producer's outbox and are published after restart, to whichever exchange the new code names. Nothing is lost by quiescing first.

1. Quiesce producers: `docker compose stop jobs risk`.
2. Run provisioning: `docker compose up -d broker-topology` (creates identities, exchanges, queues, bindings, and retires the legacy exchange).
3. Deploy the new images and start: `docker compose up -d --build jobs risk`.
4. Confirm the four permission checks pass and both services report ready.

## Safe rollback

Queue names, routing keys and queue arguments never changed, so no message needs migrating and no queue is recreated.

1. `docker compose stop jobs risk`.
2. Redeploy the previous images and compose file (the one with the shared identity).
3. The previous services declare the topology themselves under the admin identity, recreating `firewatch.events` and its four bindings; the two service identities remain but nothing uses them, which is harmless.
4. Unpublished outbox rows publish to `firewatch.events` on restart, in order.

## Acceptance checklist

- [x] Each service's operations are documented, with unrelated publication and consumption denied.
- [x] Identities are separate and provisioned from the environment; no credential is committed.
- [x] Route-level behaviour is validated, and the broker's enforcement limitations are recorded above.
- [x] Allow/deny evidence and the rollout/rollback procedure are recorded here.

## Outstanding

- **A virgin-broker rehearsal was attempted on this workstation and not completed.** Four attempts with throwaway containers got an unstable node hostname and a management application that never came up, so the provisioner never reached a broker that started empty. That is a defect of the ad-hoc host tooling, not of the provisioner, and it is recorded rather than counted as a pass: the deployment run described below remains the first exercise against a broker that starts from zero.
- **The intended deployment broker has not been validated** — it does not exist yet. The acceptance clause for the target environment closes by running the same provisioning and the same four checks there; only the two URLs change. This is a deployment gate, not remaining design work.
- The service `.env.example` files still show the shared identity. They are outside this unit's surface and are tracked as documentation debt.
- `backend/.env.example` and `scripts/init_env.py` name only `RABBITMQ_PASSWORD`; production adds `RABBITMQ_JOBS_PASSWORD` and `RABBITMQ_RISK_PASSWORD` by hand until the generator learns them. Recorded in `deploy/prod/README.md`.

## Independent review

Reviewed by a separate agent with no broker access, so its probes are static: per-module `go build`, `go vet` and `go test` green; every permission in the triple matched against a specific call the code makes (passive declare → configure, publish → write, consume → read), with no grant left unused; and the document itself audited for overclaiming. Verdict: pass with findings. What that review changed:

- The blast-radius sentence said a compromised service could never affect its peer. Authority-wise true, availability-wise false, and it is now stated as such above.
- The development stack was presented as isolated without saying that both service passwords derive from the admin one. It now says so.
- Each management-API call is bounded (`--connect-timeout 5 --max-time 15`). Without a bound, a broker that accepts TCP and stalls would hold the one-shot service open and block every service that depends on it, silently.
- A declaration mismatch is documented as `400 bad_request`, which is what the API answers, rather than the 409 the first draft assumed.
- `verify_synthetic.py` preflights the provisioning image, so a host without it fails before building three images instead of after.
- The pre-existing `[ ] pending` row for BH-04 in the hardening index was corrected.

Left as declared debt, not fixed: the two service `.env.example` files still name the shared identity (the file tooling refuses those paths, and working around a refusal is not a fix), and `docs/code-explanation/source-snapshot.json` needs a `verify_structure.py --snapshot` re-baseline on a host with Python.

## Log

- **L2 (2026-11-10)** — Independent review returned pass with eight findings, none a permission defect. Six addressed: two documentation overclaims, the burst-radius wording, an unbounded curl, the wrong status code in a comment, and a missing preflight image. Two recorded as debt above.
- **L1 (2026-11-10)** — Identities and topology implemented and measured. The split exchange was forced by the broker's name-based authorisation, not chosen for tidiness; the alternative (one exchange, shared write) could not deny a forged peer event at all. Three denials and one allow pass against the real broker; the legacy `firewatch.events` exchange was retired after the new bindings existed.
