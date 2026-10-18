# FireWatch backend — first executable slice

Run an authenticated asynchronous workflow with four independently built Go services, PostgreSQL/PostGIS, RabbitMQ and Keycloak. The sample is explicitly synthetic: `demo: true`, classified by the version selected with `MODEL_ID` (the local compose default is `firewatch-rule-v2`; `academic-rule-v1` remains the code default when nothing is configured), with the level, its factors and its rationale stored in each batch.

## Quick start

Requirements: Docker Desktop running Linux containers and Python 3. Go 1.27.2+ is needed only for native development; Go's automatic toolchain selection can download it. Run these commands from `backend/`:

```sh
python scripts/init_env.py
docker compose up -d --build
python scripts/smoke.py
```

`init_env.py` creates random local credentials and refuses to overwrite `.env`. On a previously initialized checkout, skip that command. The smoke script waits for identity startup, verifies permissions and input validation, submits a job, checks idempotency, waits for publication, queries history/audit and checks dead-letter routing. It reads local credentials without printing them.

**`docker compose up` brings up the API only.** The gateway no longer serves a web page: `GET /` returns `404`, and the former page assets were removed. The browser application lives in [`frontend/`](../frontend/README.md), is served on its own port and started separately. See [Browser login](#browser-login-local-demo).

| Local endpoint | Purpose |
|---|---|
| `http://localhost:8080` | API gateway. **API only**: `GET /` returns `404`. The page is served elsewhere |
| `http://localhost:4173` | Browser page, once [`frontend/`](../frontend/README.md) is running |
| `http://localhost:8081` | Keycloak (`local-admin`; password in `.env`) |
| `http://localhost:15672` | RabbitMQ management (`firewatch`; password in `.env`) |
| `http://localhost:9090` | Prometheus, when the observability profile is enabled |
| `http://localhost:3000` | Grafana, when the observability profile is enabled (`admin`; password in `.env`) |

The endpoints bind only to host loopback. HTTP/password grants and the shared RabbitMQ development credential are local-only choices. PostgreSQL and the service APIs have no published host ports.

### Optional classifier selection

`MODEL_ID` selects the classifier version the `risk` service runs. The compose stack passes it through and defaults it to `firewatch-rule-v2`, so the local stack classifies with v2 unless `.env` sets `MODEL_ID` to another registered version; the code default, when nothing is configured at all, stays `academic-rule-v1`. The selected version must have a row in the `models` table that the migrations create and seed, or the service fails closed at startup. Loading a different version there does not change the classifier in a running container: the version is fixed when the service starts.

### Cycle and freshness

`risk` runs the ingestion cycle at startup and every 15 minutes. The startup run publishes a batch for the configured demo zone, so a restart can legitimately make `GET /v1/risk/latest` return a newer batch than the one a client read before; `latest` is the most recently persisted batch, not a durable observation contract. `GET /v1/risk/sources` reports the last run and the expected renewal (`last_run_at` + 900 s), which is a renewal deadline, not necessarily the next scheduler tick.

### Observability profile

`docker compose --profile observability up -d` adds Prometheus and Grafana to the stack. Prometheus scrapes `gateway`, `jobs`, `risk`, `prevention` and the RabbitMQ exporter, and evaluates the rules in [`deploy/local/alerts.yml`](deploy/local/alerts.yml); Grafana comes up with the Prometheus datasource and the dashboard in [`deploy/local/grafana/`](deploy/local/grafana/) already provisioned, so the panels are files under review rather than something edited in the browser.

**Grafana needs a password before it can start.** `docker compose` interpolates the whole file before it applies a profile, so the variable is required by every command in this directory, not only by the observability one: a missing value fails the parse with `required variable GRAFANA_ADMIN_PASSWORD is missing a value`. `scripts/init_env.py` writes it on a fresh checkout; on an existing `.env`, add the line yourself:

```sh
printf 'GRAFANA_ADMIN_PASSWORD=%s\n' "$(python -c 'import secrets;print(secrets.token_hex(24))')" >> .env
```

The password is required rather than defaulted on purpose: a shipped default would be a working credential nobody chose, and anonymous access would expose the panels to anyone with access to the host. Two things this profile still does **not** do, declared rather than implied: there is no Alertmanager, so a firing alert is visible in the Prometheus UI and as the `ALERTS` series but is not routed anywhere; and `/metrics` on each service is unauthenticated, the same posture it had before, which is a network or ingress concern for a real deployment.

```sh
docker compose --profile observability up -d
python scripts/smoke.py --resilience
docker compose logs -f jobs risk gateway
docker compose down
```

The resilience check temporarily stops RabbitMQ, verifies that a new job is durably accepted while the previous batch stays readable, restarts the broker, waits for recovery and restarts the application services to verify persistence. It also injects a malformed message for DLQ inspection. `down` preserves data. Deleting volumes erases demo jobs, identity users and broker messages; changing `.env` does not rotate credentials already stored in those volumes.

## API walkthrough

Use the demo script or an API client to obtain a token from Keycloak's `/realms/firewatch/protocol/openid-connect/token` endpoint, with form fields `client_id=firewatch-demo`, `grant_type=password`, `username=demo-admin` (or `demo-viewer`), and the matching generated password. This password-grant client exists only for the local demonstration.

| Method and route | Role | Result |
|---|---|---|
| `POST /v1/jobs` | admin | `202`, durable job ID; requires UUID `Idempotency-Key` and JSON `{"zone_id":"valparaiso-demo"}`; optional `scenario_id` selects a synthetic profile |
| `GET /v1/risk/synthetic-scenarios` | viewer/admin | Ordered synthetic profile catalogue; no database/broker access |
| `GET /v1/jobs/{id}` | admin | `queued` or `published`, with batch ID when published |
| `GET /v1/risk/latest?zone_id=valparaiso-demo` | viewer/admin | Latest batch, GeoJSON point, emission/validity dates and freshness |
| `GET /v1/risk/history?zone_id=valparaiso-demo` | viewer/admin | Last 100 batches |
| `GET /v1/models` | viewer/admin | Registered classifier versions and the single `active` row |
| `POST /v1/models/{version}/activate` | admin | Activates a registered version; `MODEL_ID` still selects the running classifier |
| `POST /v1/risk/cycle/run` | admin | Runs the ingestion cycle immediately and reports completion |
| `GET /v1/prevention/actions?community_code=&status=` | viewer/admin | Preventive actions, at most 200 rows |
| `GET /v1/prevention/actions/{id}` | viewer/admin | Action detail with its history |
| `POST /v1/prevention/actions` | admin | Creates a preventive action in `propuesta` |
| `POST /v1/prevention/actions/{id}/transitions` | admin | Applies a valid state transition and appends history |
| `GET /v1/audit` | admin | Last 100 administrative/publication records |

Send `Authorization: Bearer <access_token>`. No cookies are used in this backend slice. Unknown or invalid JWTs produce `401`; insufficient roles produce `403`. Full definitions: [`contracts/openapi/firewatch-v1.yaml`](contracts/openapi/firewatch-v1.yaml).

### Reproducible synthetic scenarios

Six profiles cover mild, mixed, dry/windy, missing humidity, stale observations, and incomplete retrieval. Submit `{"zone_id":"05101","scenario_id":"mixed"}` with a fresh UUID idempotency key to select one; omit the scenario to preserve the fixed legacy fixture. The server assigns `evaluated_at` once and preserves it on retries; clients cannot supply that field. Job/batch history retains optional metadata without changing historical rows.

See [scenario contract, expected demonstration outcomes and proof limits](docs/synthetic-scenarios.md) and the [isolated verification harness](docs/synthetic-verification.md). All affected local services must rebuild together, with forward migrations jobs 003 and risk 007. In isolated disposable databases, fresh forward migrations and the full jobs and risk database suites plus `go vet` ran green under the `jobs_app`/`risk_app` service credentials, with both catalogues cross-referenced; earlier skipped or failed fixture runs remain recorded and are not retroactive passes. The isolated authenticated harness also ran green once (exit 0) against its own disposable Keycloak/RabbitMQ/PostGIS stack; its optional partial-retrieval probe was **SKIPPED** by default and the known advisories remain in [`docs/synthetic-verification.md`](docs/synthetic-verification.md). That disposable stack was separate from the existing local Compose stack, which was **not** rebuilt or upgraded **during that verification**; the operator stack was later recreated with `docker compose up -d --build` in L23. The React page has a scenario selector in the job submission view. Do not present these synthetic outputs as calibrated probabilities or model accuracy.

## Browser login (local demo)

The page is served from `http://localhost:4173`; see [`frontend/`](../frontend/README.md) to start it. Open it as `localhost`, never as `127.0.0.1`: to a browser those are different origins. Choose **Iniciar sesión con Keycloak**. Keycloak hosts the login page; FireWatch does not collect passwords. The browser uses the public `firewatch-demo` client with Authorization Code + PKCE S256. Access and ID tokens stay only in page memory. Logout clears both tokens and the private view, then sends the ID token only as the RP-Initiated Logout `id_token_hint` to end the Keycloak SSO session; the ID token is never sent to an API.

Because the page and the API are different origins, two settings must agree with each other:

- **`WEB_ORIGIN` on the gateway** must be exactly `http://localhost:4173`. It is the only origin the gateway sends CORS headers to. The match is exact, and with the variable unset the gateway sends no CORS headers at all, never a wildcard. `docker-compose.yml` sets it for the local demo, so the demo needs no `.env` change. Cross-origin responses from that origin also declare `Access-Control-Expose-Headers: X-Request-ID`. The header must be named explicitly because `X-Request-ID` is not a CORS-safelisted response header: without that declaration the browser hides it from page JavaScript, so an API error would keep the correlation id out of reach of the operator who has to quote it. `X-Request-ID` is the only non-safelisted response header the page reads.
- **The `firewatch-demo` client** must accept `http://localhost:4173/` as a redirect URI, `http://localhost:4173` as a web origin, and `http://localhost:4173/` as a post-logout redirect URI. The shipped realm import declares all three alongside the older `:8080` values, which are retained rather than replaced; nothing in this repository requires those older values any more.

A fresh realm import applies those values automatically. An already-initialized realm does not: Keycloak imports only when the realm does not already exist, so the running client keeps its previous URIs until they are changed by hand. The one-time, non-destructive Admin Console steps are in [`docs/keycloak-realm-setup.md`](docs/keycloak-realm-setup.md). Do not reimport or reset the realm, and do not alter its audience mapper or existing roles.

After a successful end-session, Keycloak's SSO cookie is ended and a new authorization grant is required. A subsequent login may still select the same account automatically if the browser or identity-provider session behavior chooses it; logout does not promise an account-switch screen.

## Development and tests

```sh
# From backend/: explicit module paths are needed in a multi-module workspace.
go test ./services/gateway/... ./services/jobs/... ./services/risk/... ./services/prevention/...
go vet ./services/gateway/... ./services/jobs/... ./services/risk/... ./services/prevention/...

# Real PostgreSQL/PostGIS tests, isolated from application databases.
docker compose --profile test run --build --rm jobs-test
docker compose --profile test run --build --rm risk-test
docker compose --profile test run --build --rm prevention-test
```

Repository integration tests skip unless `TEST_DATABASE_URL` points to the corresponding migrated test database. Compose provides separate `jobs_test` and `risk_test` databases; do not point these tests at operational data. Use the integration tests before the smoke demo to keep the demonstration history readable.

For independent builds, run `GOWORK=off go test ./...` from a service directory (PowerShell: `$env:GOWORK='off'; go test ./...`). Service-local `replace` directives resolve only the shared wire-contract module. They are also available inside each Docker build; `go.work` is excluded from images.

`Makefile` exposes `test`, `build SERVICE=jobs`, `up`, `down`, `logs SERVICE=risk`, `integration` and `demo`; its loop-based test target requires a POSIX shell. The direct commands above work on PowerShell.

## What is implemented and what comes next

Implemented: OIDC/JWT verification in gateway and services, role checks, bounded HTTP handling, SQL parameterization, per-service database credentials, transactional outbox/inbox, confirmed publishing, quorum queues, bounded retry and DLQ, terminal job-failure events and operator replay, reconnect, synthetic atomic batches, stale-state calculation, audit and JSON logs. The presentation slice adds a configurable HTTP/JSON ingestion provider with per-record validation, the 15-minute cycle with per-source freshness, a second deterministic classifier (`firewatch-rule-v2`) with a transactional model registry (activation and reversal), the `prevention` service with its browser view, and Prometheus metrics with latency, error-class and process signals in all four services, seven alert rules and a provisioned Grafana dashboard.

Next work units: the real observation source and its production validation ranges; the trained model and its scientific evaluation; distributed tracing; Alertmanager routing and monitoring in the Kubernetes manifests; image signing, registry promotion and an executed rollout. The 15-minute cycle and the preventive-action flow are implemented, not future work. Kubernetes manifests remain a configurable staging base, not an executed production rollout. See [architecture](docs/architecture.md), [verification evidence](docs/verification.md) and [deployment prerequisites](deploy/k8s/README.md).
