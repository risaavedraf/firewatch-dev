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
| `govulncheck@v1.1.4` | No vulnerabilities found in the three Go modules after upgrades |
| Trivy 0.69.3 runtime image scan | No HIGH/CRITICAL findings in gateway, jobs or risk application images |
| Gateway image build with `--sbom=true --provenance=true` | Passed locally |
| Actionlint 1.7.7 | Workflow syntax passed; optional ShellCheck/Pyflakes integrations disabled |
| Kustomize + Kubeconform 0.6.7 | 17 resources valid, 0 invalid/errors/skipped |
| Kin-openapi validator 0.133.0 | HTTP specification valid |
| Prometheus `up` query | jobs, risk and RabbitMQ all returned 1 |

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
- Remote GitHub execution, registry publication, signature verification or deployment promotion. Workflows are prepared locally; the workspace is a Git repository now, but they have never run, because nothing has been pushed to a remote.

Security scan results describe the advisory databases available during verification, not a guarantee against undiscovered vulnerabilities.
