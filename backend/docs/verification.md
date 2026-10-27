# Verification evidence — 2026-10-06

The local vertical slice was executed on Windows with Docker Desktop Linux containers. This report distinguishes executed checks from future deployment requirements.

| Check | Result |
|---|---|
| Independent Go module tests (`GOWORK=off go test ./...`) | Passed for gateway, jobs and risk |
| `go vet` and formatting | Passed |
| Linux `go test -race -count=1 ./...` | Passed for all three services; jobs/risk included database integration tests |
| PostgreSQL integration | Passed: HTTP-command idempotency, one outbox event, rollback when audit insertion fails, deduplicated completion |
| PostGIS integration | Passed: duplicate event/job suppression and out-of-order batch publication |
| `docker compose config --quiet` and image builds | Passed; all application migrations and services started |
| `python scripts/smoke.py --resilience` | Passed: real Keycloak authentication, RBAC, validation, async work, history/audit, broker outage recovery, last-valid reads, persisted data after service restart |
| Malformed-event check | Passed; verifies an increase in DLQ messages and an unchanged published batch |
| `govulncheck@v1.9.0` (Go 1.27.2) | No vulnerabilities found in the three Go modules; see *Toolchain refresh* below. Historical: `govulncheck@v1.1.4` under Go 1.26.6 reported nine standard-library advisories plus GO-2026-6629 |
| Trivy 0.69.3 runtime image scan | No HIGH/CRITICAL findings in gateway, jobs or risk application images; re-verified 2026-10-14 on the images rebuilt with Go 1.27.2 (*Toolchain refresh*) |
| Gateway image build with `--sbom=true --provenance=true` | Passed locally |
| Actionlint 1.7.7 | Workflow syntax passed; optional ShellCheck/Pyflakes integrations disabled |
| Kustomize + Kubeconform 0.6.7 | 17 resources valid, 0 invalid/errors/skipped |
| Kin-openapi validator 0.133.0 | HTTP specification valid |
| Prometheus `up` query | jobs, risk and RabbitMQ all returned 1 |

## Toolchain refresh — 2026-10-14

The `Jobs`, `Risk` and `Gateway` workflows on `main` stopped at `go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...` under Go 1.26.6: nine Go standard-library advisories (GO-2026-6603, 6605, 6607, 6608, 6610, 6611, 6612, 6613, 6617) plus GO-2026-6629 in `golang.org/x/text v0.39.0`. The rows above that describe that toolchain remain as the earlier evidence.

Candidate changes, verified locally on 2026-10-14 and not yet pushed:

| Change | Scope |
|---|---|
| Go 1.27.2 | `.github/workflows/_go-service.yml` and the three builder image digests |
| `go 1.27.2` | `go.work` and the four module files |
| `golang.org/x/text v0.41.0` | jobs and risk (closes GO-2026-6629); `golang.org/x/sync v0.22.0` follows from module selection |
| `govulncheck@v1.1.4` -> `v1.9.0` | workflow pin: `v1.1.4` panics under Go 1.27.2 (see below) |
| `backend/go.work`, `backend/go.work.sum` added to `paths` | jobs, risk and gateway workflows |

| Check (Go 1.27.2, linux/amd64, pinned builder image) | Result |
|---|---|
| `test -z "$(gofmt -l .)"`, `go mod verify`, `go test -race -count=1 ./...`, `go vet ./...` | Passed for gateway, jobs and risk |
| `govulncheck@v1.9.0` | No vulnerabilities found in the three modules |
| `docker build --sbom=true --provenance=true` | Passed for the three images; SBOM and provenance attestation manifests emitted |
| Trivy 0.69.3 `--scanners vuln --severity HIGH,CRITICAL --exit-code 1` | 0 findings in the Debian layer and in the `server` Go binary of all three images |

`govulncheck@v1.1.4` cannot analyse this code under Go 1.27.2: it panics in `golang.org/x/tools@v0.29.0/go/ssa/builder.go` (`unexpected expr: *ast.KeyValueExpr`). The panic reproduces on a trivial module that only declares `go 1.27.2`, so it is a scanner/toolchain incompatibility, not a property of this code.

Local environment notes: `go test -race` needs a Linux toolchain, so the checks above ran inside the pinned builder image rather than on the Windows host, which has no C compiler. Two local artefacts were ruled out instead of being fixed here: a transient checksum-database error while downloading the scanner, and the `*.geojson` line-ending conversion that `.gitattributes` does not cover (with `core.autocrlf=true` a Windows checkout rewrites `backend/contracts/gen/geo/valparaiso-comunas.geojson`, whose `derived_sha256` the risk service validates at runtime; the committed blob is LF, so CI is unaffected). Re-observed on 2026-10-14 in a fresh `git worktree`: the checkout wrote 2,137,629 bytes (CRLF) against the committed 2,137,628-byte LF blob, and `GOWORK=off go test ./... -count=1` under `backend/services/risk` failed three handler tests — `TestSourcesActualFixtureDescriptor`, `TestSourcesActiveProviderAndAuthorization`, `TestSourcesIndependentOfWorkingDirectory` — with `503 geography_integrity_unavailable`. Normalizing the worktree file to LF restores 293 PASS / 9 SKIP / 0 FAIL, and `git diff` for that path stays empty either way, so no content change is involved. The observation is first-hand from the worktree created for this report, whose `valparaiso-comunas.geojson` is now LF and byte-identical to the committed blob; whether a given Windows checkout writes CRLF depends on that clone's `core.autocrlf` and on the extension not being marked in `.gitattributes`, which is where a durable fix belongs and which is outside this report.

## Toolchain refresh — 2026-10-27

The workspace was already pinned to Go 1.27.2 (2026-10-08), which carries that month's `net/http` batch. Go 1.27.3 (2026-10-19) closes one further item: `crypto/x509` stops trusting eight eMudhra intermediate CAs whose private keys were generated with insufficient entropy, so a client built before that release can be served forged certificates for any host. Two module advisories were closed at the same time.

| Change | Scope |
|---|---|
| Go 1.27.3 | `.github/workflows/_go-service.yml` and the four builder image digests; `golang:1.27.3-bookworm@sha256:972b4110ba0f...` was read from the registry tag, the same source as the 1.27.2 pin it replaces |
| `go 1.27.3` | `go.work` and the five module files |
| `github.com/go-jose/go-jose/v4` 4.1.4 -> 4.1.6 | gateway, jobs, prevention and risk (GHSA-mgcc-cww5-fp2q: unbounded RSA validation while parsing a crafted JWK) |
| `github.com/rabbitmq/amqp091-go` 1.13.0 -> 1.14.0 | jobs and risk (GHSA-w6r9-248c-frg8: pre-negotiation frame limit not enforced) |

| Check (Go 1.27.3, Windows host, `GOWORK=off`) | Result |
|---|---|
| `test -z "$(gofmt -l .)"`, `go mod verify`, `go vet ./...` | Passed for gateway, jobs, prevention and risk |
| `go test -count=1 ./...` | Passed: 3, 6, 7 and 11 packages, 0 failures |
| `govulncheck@v1.9.0` | No vulnerabilities found, in all four modules |

How far the go-jose bump reaches: `govulncheck@v1.9.0` reports no vulnerabilities for the gateway with either 4.1.4 or 4.1.6, because the vulnerable JWK path is not reachable from this call graph, while OSV module matching does list GHSA-mgcc-cww5-fp2q for 4.1.4. The upgrade removes a listed advisory, not a demonstrated path.

Not covered by this refresh: `go test -race`, the Docker image builds and the Trivy scan. This host has no C compiler and the Docker daemon was not running, so those checks remain the ones recorded in the 2026-10-14 section. `kin-openapi` 0.133.0 keeps its advisories and is imported by the contract tests only; `golang.org/x/mod` 0.38.0 is build tooling. Neither is changed here.

## Image refresh — 2026-10-27

`quay.io/keycloak/keycloak:26.5.2` -> `26.5.7`. The 26.5 patch line carries the fixes that matter in this stack: CVE-2026-1486 (authorization-grant bypass, 26.5.3), CVE-2026-0707 and CVE-2026-1190 (26.5.4), CVE-2026-3009 and CVE-2026-3047 (a disabled identity provider still authenticating, 26.5.5), CVE-2026-1180 and CVE-2026-1035 (26.5.6), CVE-2025-14083 (Admin REST API information disclosure, 26.5.7). Staying inside 26.5 avoids the `secure-client-uris` change that 26.6.0 introduces, which `keycloak-realm-setup.md` already flags as breaking for the local `http` logout URIs.

Four pinned images were examined and deliberately not moved:

| Image | Why it stays |
|---|---|
| `grafana/grafana:11.5.2` | The critical advisories of 2026-03-25 (CVE-2026-27876, CVSS 9.1; CVE-2026-27880) affect 11.6.0 and later, so this tag is not exposed to them. The service sits behind the opt-in `observability` profile, bound to 127.0.0.1, and requires an admin password. |
| `rabbitmq:4.1.8-management` | 4.1.8 is the last release of its line, so staying is not a fix; moving to 4.3.x crosses broker deprecations and needs a booted stack. |
| `prom/prometheus:v3.2.1` | Observability profile only, bound to 127.0.0.1, and no advisory for this tag was identified in these checks. |
| `postgis/postgis:17-3.5` | 3.6 exists, but this is the data store: changing the spatial library is its own work unit (Risk: item 1) and needs the database integration tests, not a tag edit. |

Not verified: no image was pulled, built or scanned here. The Docker daemon was not running, so `docker compose config --quiet` is the only check that ran; the Trivy scan of runtime images stays in CI.

## Reproduce the meaningful runtime checks

From `backend/`, with the local `.env` already initialized:

```sh
docker compose --profile test run --build --rm jobs-test
docker compose --profile test run --build --rm risk-test
docker compose up -d --build
python scripts/smoke.py --resilience
docker compose --profile observability up -d
```

The test profile uses dedicated test databases. The smoke script creates synthetic demonstration records and deliberately adds a malformed message to the risk DLQ. It never prints tokens or passwords.

## Not established by these checks

- Kubernetes rollout, cluster failover, recovery from backups or production availability/performance SLAs.
- Real wildfire inference, model accuracy/calibration, source-data validity or the 15-minute scheduler.
- Browser authentication/CSRF behavior, service-mesh encryption or immediate JWT revocation.
- Infrastructure-image security: the Trivy results above cover the three authored application images, not Keycloak, PostgreSQL, RabbitMQ or Prometheus.
- Registry publication, signature verification or deployment promotion. The workflows do run on GitHub: on `main` at `0ed07e2`, before the toolchain refresh, the `Backend integration` run was green while the three service runs stopped at the vulnerability-scan command, and their `Build container with provenance and SBOM` and `Scan runtime image` steps were skipped in that run. That is no longer the current state: `c45508e` enables the containerd image store in the job, after which the three service runs complete with both steps successful (recorded at `odd/tasks/security/ci-go-1.27.2-green.md:52`, `:131`), and the `Risk` run `38612773033` on `572c12e` reports steps 8 (`govulncheck`), 10 (`Build container with provenance and SBOM`) and 11 (`Scan runtime image`) as success, checked through the GitHub API by the routing session and not re-queried here. The toolchain refresh above validated both steps locally as well.

Security scan results describe the advisory databases available during verification, not a guarantee against undiscovered vulnerabilities.

## Historic baseline

**Historic baseline (2026-10-06; toolchain refresh 2026-10-14).** This report is kept as the evidence of that slice and is not rewritten. It predates the presentation delivery: the 15-minute ingestion cycle, `firewatch-rule-v2`, the `prevention` service and HTTP latency/error/process metrics in all four services were implemented later. For the current delivered/missing state see [`odd/tasks/README.md`](../../odd/tasks/README.md) and [`odd/tasks/presentation-client-ready.md`](../../odd/tasks/presentation-client-ready.md) L22–L28.
