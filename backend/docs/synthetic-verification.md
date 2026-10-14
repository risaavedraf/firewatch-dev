# Isolated synthetic verification harness

`backend/scripts/verify_synthetic.py` is a self-contained, reusable check of the
FireWatch synthetic scenario workflow. It is deliberately **not** `smoke.py`:

| | `scripts/smoke.py` | `scripts/verify_synthetic.py` |
|---|---|---|
| Targets | the operational Compose stack | a disposable stack it creates itself |
| Reads `.env` | yes | **never** |
| Ports | fixed (`8080`, `8081`, `15672`) | free loopback ports it allocates |
| Credentials | the project's `.env` | generated in memory, never printed |
| Resource names | the `firewatch` Compose project | unique `firewatch-synthetic-verify-<run-id>` |
| Cleanup | `docker compose stop/start` | removes only its own containers/network/images |

Use the harness to prove the **real** authenticated HTTP + RabbitMQ + PostGIS
path against **synthetic** inputs. It makes no calibrated-model, trained-metric,
scientific or real-observation claim.

## Requirements

- Docker Desktop's Linux engine running and reachable (`docker info` succeeds).
- These runtime images already present locally; the harness refuses to start
  otherwise, so the run does not depend on a registry pull for them:
  - `postgis/postgis:17-3.5`
  - `rabbitmq:4.1.8-management`
  - `quay.io/keycloak/keycloak:26.5.2`
- Python 3.11+ (standard library only; nothing to install).
- The repository's `backend` build context, whose `.dockerignore` excludes
  `.env`. The harness verifies that exclusion and **refuses to build** if the
  `.env` pattern is missing.
- Network access while building: `docker build` may fetch the Go/distroless base
  images and each service image runs `go mod download`, exactly as the Compose
  build does. The "already present locally" guarantee applies to the three
  runtime images above, not to the build.

The harness mounts `deploy/local/postgres-init.sh` and
`deploy/local/firewatch-realm.json` read-only, and reuses the exact images,
database roles and migration entrypoints from the Compose stack. It does not
edit, regenerate or depend on `.env`, and it does not touch any existing
`firewatch` container, network, volume, database or port.

## Exact command

From the repository root:

```sh
python backend/scripts/verify_synthetic.py
```

The public help is the authoritative option list:

```sh
python backend/scripts/verify_synthetic.py --help
```

```
usage: verify_synthetic.py [-h] [--timeout SECONDS] [--ready-timeout SECONDS]
                           [--poll-interval SECONDS] [--zone ZONE]
                           [--check-partial-retrieval] [--keep]

  --timeout SECONDS     overall deadline for the whole run (default: 900)
  --ready-timeout SECONDS
                        per-service readiness deadline (default: 240)
  --poll-interval SECONDS
                        seconds between readiness probes (default: 1)
  --zone ZONE           catalogue zone used by the workflow (default: valparaiso-demo)
  --check-partial-retrieval
                        wait a bounded time and confirm the partial-retrieval job
                        is never published (off by default)
  --keep                leave the created containers, network and images in place
                        for inspection instead of cleaning up
```

The help phrase "never published" for `--check-partial-retrieval` describes the
bounded no-publication check below, not a guarantee that the job can never
publish.

Exit codes: `0` all checks passed; `1` a check, readiness deadline, build or
migration failed; `2` invalid arguments. An invalid `--timeout`/`--ready-timeout`
/`--poll-interval` is rejected as `argument --timeout: must be a positive whole
number of seconds` before any Docker call. A missing Docker CLI or missing local
image fails with one actionable `FAIL:` line and no traceback.

`stdout` carries only `PASS:`/`SKIP:`/`NOTE:` summaries; failures go to `stderr`
and exit non-zero. No token, password, connection URL or `.env` content is ever
printed, and Docker/build output is redacted before it is shown.

## Observed result

One parent-run invocation of `python backend/scripts/verify_synthetic.py`
(default options, disposable stack, no `--keep`) exited `0` after about 65
seconds. Against real isolated builds, ephemeral admin forward migrations and the
public authenticated API it observed: unauthenticated `401`, the viewer/admin
six-profile catalogue, the GeoJSON zone catalogue, viewer `403` on job creation,
`400 invalid_demo_request` for `evaluated_at`, unknown, `null`, empty and
non-string `scenario_id` with unchanged public state, the five publishable
profile levels with five factors, rationale and frozen fingerprint, the omission
legacy fixture, and the idempotent-retry identity. The optional
`--check-partial-retrieval` probe was **not** enabled, so that runtime case
reported `SKIP:`; provider/publisher refusal is proved separately by unit tests.
The known limits below remain in force, and this run used a disposable stack, not
the existing local Compose stack.

## What it does

1. Verifies Docker, the `backend` layout, the `.dockerignore` `.env` exclusion,
   the three Dockerfiles and the three local images.
2. Builds uniquely tagged `gateway`, `jobs` and `risk` images from the backend
   context (so `.env` is excluded by `.dockerignore`).
3. Creates one private network and starts PostgreSQL/PostGIS, RabbitMQ and
   Keycloak under unique names and labels, on loopback-only gateway, identity
   and RabbitMQ-management ports. Keycloak's hostname is the assigned external
   issuer; services reach the others through the `postgres`/`rabbitmq`/`identity`
   network aliases. Keycloak readiness is the **realm discovery document on the
   assigned application port**
   (`GET /realms/firewatch/.well-known/openid-configuration`), accepted only when
   it returns HTTP 200 and its `issuer` equals
   `http://localhost:<identity-port>/realms/firewatch`. Keycloak 26 serves the
   management health endpoint on a separate port that this harness neither
   publishes nor probes.
4. Runs the forward migrations as the ephemeral PostgreSQL administrator through
   each service image's default `migrate` entrypoint, then starts `jobs`, `risk`
   and `gateway`.
5. Authenticates `demo-admin` and `demo-viewer` with ephemeral credentials and
   runs the checks below through the public HTTP API.

## Checks

- The gateway returns `401` to unauthenticated requests.
- The authenticated scenario catalogue returns the six known profile IDs, in
  contract order, identically for viewer and admin.
- The authenticated PostGIS zone catalogue returns an RFC 7946
  `FeatureCollection` with real geometries.
- A viewer receives `403` when creating a job.
- Requests carrying `evaluated_at`, an unknown `scenario_id`, `scenario_id: null`,
  `scenario_id: ""` or a non-string `scenario_id` return `400` with the
  `invalid_demo_request` code; the public audit, latest and history responses
  are unchanged under decoded-JSON equality and no job is created indirectly.
- Each publishable profile (`mild`, `mixed`, `dry-windy`, `missing-humidity`,
  `stale`) publishes the documented level (`bajo`/`medio`/`alto`/`desconocido`),
  `demo=true`, five factors, a non-empty rationale, the frozen
  `academic-rule-v1` policy fingerprint and a PostGIS `Point` geometry. The
  accepted job, the completed job, the batch and the history entry are compared
  for the same `scenario_id` and the same `evaluated_at` instant; the latest read
  is compared for `id` and `scenario_id` only, not `evaluated_at`.
- `partial-retrieval` is accepted by the transport but is not waited on by
  default; the run then reports an explicit `SKIP:` for that publication. With
  `--check-partial-retrieval` the harness waits a bounded time and checks that no
  publication was observed during that window; it does not guarantee the job can
  never publish (see the limitations below).
- A request without `scenario_id` keeps the legacy fixture: `202`, and no
  synthetic metadata on the completed job or batch.
- An idempotent retry that reuses the same actor and `Idempotency-Key` with a
  different valid scenario returns the original job, scenario and instant, and
  produces exactly one batch and the expected `job.requested`/`batch.published`
  audit pair.

RabbitMQ is exercised implicitly: the asynchronous `POST` → poll →
latest/history path only completes when the broker carried the request and the
result. PostGIS is exercised by the zone catalogue and by the geometry stored
with and returned from each batch.

## Expected effects and cleanup

A normal run creates, then removes, only its own resources:

- an image tag per service, e.g. `firewatch-synthetic-verify-<id>-gateway`,
- one network `firewatch-synthetic-verify-<id>-net`,
- containers `...-postgres`, `...-rabbitmq`, `...-identity`, `...-jobs`,
  `...-risk`, `...-gateway` (plus two short-lived `migrate` containers),
- everything labelled `firewatch.verify=1` and `firewatch.verify.run=<id>`.

Cleanup runs in a `finally` block and records ownership by **container ID, not
name**: the network is marked owned only after `docker network create` succeeds;
each service container is started with `docker run --detach --name ...` and the
ID docker returns is recorded; each migration container is created with
`docker create --name ...`, its returned ID is recorded *before*
`docker start --attach <id>` runs, and the start and remove commands use that
owned ID. A failed network create therefore never triggers a `docker network rm`,
a failed `docker create` is never tracked or deleted, a malformed or empty create
response fails closed without falling back to the name, and a migration that
fails or is cut off by the overall timeout is still removed by ID. Cleanup issues
`docker rm -f <id>` on exactly the owned container IDs, `docker network rm` on
the owned network, and `docker rmi` on the owned unique tags. It never runs
`prune`, `down`, or any global delete and never touches a resource it did not
create. A cleanup command that fails is reported and makes the run exit non-zero
instead of being silently ignored. No named volumes are created, so PostgreSQL
and RabbitMQ data is anonymous and discarded.

`--keep` skips cleanup and prints the retained resource names to `stderr` for
inspection; remove them manually afterwards.

## Tests for the harness itself

Argument handling and the resource-lifecycle rules are covered without ever
starting real Docker:

```sh
python -m unittest discover -s backend/scripts -p test_verify_synthetic.py -v
```

- Help/argument tests run the script as a subprocess with a `PATH` that contains
  no Docker binary, proving that `--help` and argument rejection need no Docker
  and leave an isolated working directory untouched.
- Keycloak readiness tests run a local HTTP server and assert the probe targets
  the discovery document on the application port and requires the matching
  `issuer`.
- Isolation gate: before any lifecycle test launches the harness it asserts,
  via `shutil.which("docker", path=<fake PATH>)`, that the only resolvable docker
  is the owned fake executable (including the Windows `.cmd`). The fake `PATH`
  contains only the fake bin directory, the harness is launched with the absolute
  `sys.executable`, and the gate has its own tests proving it rejects a missing,
  foreign or host-fallback docker without ever executing it.
- Lifecycle tests give the harness that isolated **fake** Docker CLI, which
  records every invocation, returns deterministic container IDs, and simulates
  failure: a failed network create must never be removed, a created migration
  container must be started and cleaned up **by its returned ID**, a failed
  `docker create` (name already in use) must never be started or deleted, an
  attached `start` that outruns the bounded subprocess timeout must still be
  cleaned up by ID with a non-zero exit, and a failing cleanup must surface as a
  non-zero run rather than a silent `PASS`.

A parent runner executed this suite once: 17 tests passed in about 26.6 seconds
(exit 0). An independent reviewer separately passed the source-only isolation,
returned-container-ID ownership and attached-start timeout gates; that source
review is attributed separately from the parent execution.

## Known limits of these checks

These are recorded rather than silently "fixed" or overstated:

- Instants are compared **after parsing**: `_parse_instant` normalizes a naive
  value to UTC and compares instants, so the harness does **not** by itself prove
  the server serialized `evaluated_at` in UTC on the HTTP wire. The independent
  backend database/UTC tests separately prove that the metadata returned from the
  jobs/risk DB boundaries is in UTC (Location `UTC`) with equal instants; that is
  a database-return proof, not an HTTP-serialization proof. This harness only
  proves instant equality across the accepted job, completed job, batch and
  history.
- The `latest` assertion checks `id` and `scenario_id` but not `evaluated_at` on
  the latest row; `evaluated_at` equality is asserted on the batch and history
  entries.
- The unauthenticated `401` check covers `GET /v1/jobs` and
  `GET /v1/risk/latest`; the other routes are not separately probed without a
  token.
- A `429` from the gateway is retried a bounded number of times (two, one second
  apart); this is transport retry, not proof of the rate-limiter limits.
- `--check-partial-retrieval` can report `PASS` after a bounded wait in which no
  publication was observed. It does not prove the job can never publish: the
  documented terminal-failure event does not exist yet, so a partial-retrieval
  job may simply remain `queued`.
- `--timeout` bounds each Docker command by the remaining overall time; it is a
  per-command bound, not a strict wall-clock ceiling on the whole process.
- A `docker create` that exits 0 but prints no valid container ID makes the run
  fail closed (non-zero) without tracking the container and without falling back
  to its name. A successful create whose ID is lost or malformed could still
  leave an untracked owned container behind; an operator may then need to inspect
  owned resources by label and dispose of them manually, never deleting
  containers the harness did not create.
- The timeout regression uses the overall `--timeout` as the bound on the
  attached `docker start`; it proves cleanup-of-ID on timeout, not any specific
  production migration deadline.
- Because `docker build` may fetch base images and Go modules, a cold cache needs
  network access and may exceed the default `--timeout`.

## Not covered

- Browser/PWA login, logout, PKCE, the React/Vite frontend or any UI scenario
  selector. Only the public HTTP API is exercised.
- Calibrated or trained models and their metrics. The checks pin
  `academic-rule-v1`, an explicitly non-official rule; no probability,
  calibration or accuracy is claimed.
- Real observations. Every record is synthetic demonstration input.
- Kubernetes, production availability, backups, scaling, SLA or security
  approval.
- Scientific validity of the risk levels. The expected levels pin the current
  rule's behaviour, not the behaviour of real fires.
