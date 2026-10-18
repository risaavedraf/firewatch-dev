# CI migration readiness

Why the `Backend integration` job stops inside
`docker compose --profile test run --build --rm jobs-test`, what the readiness
rule behind it is, and which checks keep it fixed.

## Symptom

GitHub run `39161170125` (workflow `Backend integration`, `main`
`3caaa715df923a71d315602803d122f695156d48`) fails in the first test-profile step:

```text
Container firewatch-postgres-1  Healthy
Container firewatch-jobs-test-migrate-1  Started
service "jobs-test-migrate" didn't complete successfully: exit 1
##[error]Process completed with exit code 1.
```

The failed-step log carries the image build output and that one line, and then
nothing: `docker compose run` streams only the service it was asked to run, so
the failing migrator's own stderr — the actual reason — never reaches the log.
The migrator container itself is not removed by `--rm`, so its log survives on
the runner for the duration of the job.

The same application code ran green earlier on the same day
(`39157802502`, `0725a1b2fa6ca64e38ddde39f1caf55b09b96f05`). The only difference
between that commit and the failing one is documentation, so this is a timing
race, not a defect introduced by the commit that triggered it.

## Root cause

The PostgreSQL image runs `docker-entrypoint-initdb.d` against a *temporary*
server started with `listen_addresses=''`: that server answers on the local Unix
socket only, and the real server starts listening on TCP once the init scripts
have finished. `pg_isready` without `-h` probes that same Unix socket, so a
socket-only healthcheck reports the container healthy while the real server is
still not listening and the databases do not exist yet.

Measured on this repository's `postgis/postgis:17-3.5` on a cold volume, with a
TCP probe on the project network and the same commands CI runs:

| event | pre-fix |
| --- | --- |
| temporary server accepts (Unix socket) | `20:35:00.147` |
| Compose reports `postgres` healthy | `20:35:02.126` |
| `jobs-test-migrate` / `risk-test-migrate` start | `20:35:04.5` |
| migrators exit | `1`, `service_stopped: migration connection failed` |
| real server accepts TCP | `20:35:11.962` |

Between those two instants every dependent dials a port nobody is listening on,
which is exactly what the job above hit. On a runner the window is wide enough
to hit regularly; on a faster local host the first healthcheck tick can fall
just after the temporary server stops, which is why the same race reports as an
intermittent red rather than a permanent one.

## The invariant the healthcheck must express

```text
compose says postgres healthy
  => the real server accepts a TCP connection
  => docker-entrypoint-initdb.d has finished
  => the databases and roles created by deploy/local/postgres-init.sh exist
```

The last step holds because `postgres-init.sh` is mounted into
`/docker-entrypoint-initdb.d/`, and the entrypoint runs that directory before
starting the real server. That is why a TCP readiness probe is sufficient for
the migrators, and why the mount and the `CREATE DATABASE` statements are part
of the regression tests.

## Fix

`postgres.healthcheck` now probes TCP explicitly:

```yaml
test:
  - CMD-SHELL
  - pg_isready -h 127.0.0.1 -p 5432 -U postgres -d postgres
```

This is the same readiness rule `scripts/verify_synthetic.py` already applies
when it waits for its disposable PostgreSQL. Nothing else changed: no
`continue-on-error`, no skipped step, no removed test, no relaxed authentication,
and no retry wrapped around the migrators. A readiness failure still fails the
job.

## Checks that keep it fixed

| Regression tests (deterministic, no Docker) | `python -m unittest discover -s backend/scripts -p test_ci_migration.py -v` | The healthcheck dials TCP; every database Compose connects to is created by the mounted init helper; the test-profile services still wait for completed migrators; the workflow still runs the gate, the readiness checks and the failure diagnostics; and, through a fake `docker` CLI, that every runtime command is pinned, that cleanup keeps its own budget and its own exit code, and that a foreign-labelled resource is never deleted |
| Runtime readiness check | `python -u backend/scripts/verify_ci_migration.py` | On a disposable project: `healthy` implies an in-container TCP probe succeeds, both test migrators exit 0 at that instant, and `jobs_app`/`risk_app` reach `jobs_test`/`risk_test` over TCP |
| Existing test-profile steps | `docker compose --profile test run --build --rm jobs-test` (and `risk-test`, `prevention-test`) | End-to-end behaviour; unchanged |
| Failure diagnostics (workflow) | `docker compose --profile test ps -a` and `docker compose --profile test logs --no-color --tail 200`, both `if: failure()` | Prints the migrator's own stderr before the teardown removes the project, which is what the failing run could not show |

The regression tests fail against a socket-only healthcheck and against an
unhardened runtime harness, so both halves are caught without a Docker daemon.

### Runtime harness contracts

The readiness check is safe to run from any directory because it decides its own
context rather than inheriting one:

* every command pins the manifest (`-f backend/docker-compose.yml`), the project
  directory and child cwd (`backend/`), the project name and an *empty*
  temporary `--env-file`; `COMPOSE_FILE`, `COMPOSE_PROJECT_NAME`,
  `COMPOSE_PROFILES` and every other inherited `COMPOSE_*` value are stripped, so
  the caller's shell and the developer's `backend/.env` cannot redirect it. Only
  daemon settings such as `DOCKER_HOST` pass through.
* the generated project name must be unknown to the daemon before anything
  starts. Ownership is then the set of resources the daemon reports under that
  project's `com.docker.compose.project` label; after a failed teardown, a
  resource is deleted only when its label can still be read back as this run's
  project, and a resource that cannot be confirmed is reported instead of
  removed.
* cleanup runs on its own `--cleanup-timeout` budget (default 300s), independent
  of the `--timeout` readiness budget, inside a `finally` block that always
  removes this run's temporary env file; every cleanup error is reported.
* the exit code is the result: `0` the contract holds, `1` the contract was
  violated, `2` the harness could not run the check, `3` the contract held but
  cleanup failed. A cleanup failure never replaces a failure above it. stdout
  carries the redacted progress and result lines, `argparse` usage errors go to
  stderr, and no generated credential is ever printed.

## Reproduce locally

```bash
# Isolated project, generated credentials in memory, empty env file so the
# developer's backend/.env is never loaded. The caller's directory is irrelevant:
# the harness pins the manifest, the project directory and its child cwd.
python -u backend/scripts/verify_ci_migration.py

# The failing path itself, cold, in its own project:
cd backend
printf '' > ../tmp/empty.env            # temporary, not committed
for k in POSTGRES_PASSWORD JOBS_DB_PASSWORD RISK_DB_PASSWORD \
         KEYCLOAK_DB_PASSWORD RABBITMQ_PASSWORD KEYCLOAK_ADMIN_PASSWORD \
         DEMO_ADMIN_PASSWORD DEMO_VIEWER_PASSWORD GRAFANA_ADMIN_PASSWORD; do
  export "$k=$(python -c 'import secrets;print(secrets.token_hex(24))')"
done
docker compose -p firewatch-ci-check-1 --env-file ../tmp/empty.env \
  --profile test run --build --rm jobs-test
docker compose -p firewatch-ci-check-1 --env-file ../tmp/empty.env \
  --profile test down -v --remove-orphans
```

To watch the race itself on a host that initialises faster than the 5s
healthcheck interval, constrain only the disposable database container
(`docker update --cpus 0.25 <container>` before starting it) and probe TCP from
a container on the project network. Pre-fix that reproduces `exit 1` with
`migration connection failed`; post-fix readiness and TCP become true together.

## Boundaries

This document and its checks prove the readiness rule and the failing path on
disposable state. They are not a GitHub-green claim: only an observed run of
`Backend integration` can say that, and no workflow run was triggered by this
change. The steps downstream of the test profiles (`docker compose up -d
--build`, `scripts/smoke.py --resilience`, the kustomize and OpenAPI validation)
were not re-run for this fix and are unaffected by it, because it changes one
healthcheck command.
