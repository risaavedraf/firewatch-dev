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

Local environment notes: `go test -race` needs a Linux toolchain, so the checks above ran inside the pinned builder image rather than on the Windows host, which has no C compiler. Two local artefacts were ruled out instead of being fixed here: a transient checksum-database error while downloading the scanner, and the `*.geojson` line-ending conversion that `.gitattributes` does not cover (with `core.autocrlf=true` a Windows checkout rewrites `backend/contracts/gen/geo/valparaiso-comunas.geojson`, whose `derived_sha256` the risk service validates at runtime; the committed blob is LF, so CI is unaffected).

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
- Registry publication, signature verification or deployment promotion. The workflows do run on GitHub: on `main` at `0ed07e2` the `Backend integration` run is green and the three service runs stop at the vulnerability-scan command. Their `Build container with provenance and SBOM` and `Scan runtime image` steps have never executed on the runner (they were skipped); the toolchain refresh above validated both locally.

Security scan results describe the advisory databases available during verification, not a guarantee against undiscovered vulnerabilities.
