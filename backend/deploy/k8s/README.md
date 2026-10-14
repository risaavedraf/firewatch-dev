# Kubernetes deployment base

This is a staging deployment template for the **synthetic backend slice**, not a provisioned production environment. Compose remains the local development path. The manifests provide two replicas per service, probes, resource limits, restricted containers, disruption budgets, TLS ingress and network policies.

## Configure before applying

1. Select the target cluster, registry, ingress controller and real DNS names. Replace all `replace-before-deploy` image tags with reviewed immutable digests using a Kustomize overlay.
2. Provision PostgreSQL/PostGIS and RabbitMQ with persistence, backups and TLS. This implementation uses PostgreSQL-specific SQL. Oracle production compatibility is **not** implemented; confirm the production database decision separately.
3. Configure an OIDC realm with RS256 access tokens, `firewatch-api` audience and `realm_access.roles` containing `viewer`/`admin`. Use Authorization Code + PKCE for a future browser client. The local password-grant client and demo accounts are not production identity configuration.
4. Create the namespace and provision secrets through the target secret manager:

| Secret | Keys |
|---|---|
| `jobs-runtime` | `DATABASE_URL` (jobs_app, TLS `sslmode=verify-full`), `AMQP_URL` (`amqps://`, service-specific user) |
| `risk-runtime` | `DATABASE_URL` (risk_app, TLS `sslmode=verify-full`), `AMQP_URL` (`amqps://`, service-specific user) |
| `firewatch-tls` | Ingress certificate and private key |

5. Create `jobs_app` and `risk_app` roles and separate databases. Run each corresponding application image with argument `migrate`, a migration-owner `DATABASE_URL`, and a bounded Kubernetes Job **before** deploying replicas. SQL migrations are transactional and serialized by advisory lock. PostGIS extension installation needs suitable DBA permissions. Never pass migration-owner credentials to application deployments.
6. Adapt `network.yaml` to your DNS, monitoring, ingress and infrastructure namespace labels; managed external databases require explicit egress IP rules. The initial port-443 egress rule is intentionally broad until the identity provider is selected. An enforcing CNI is required. Provision service-specific RabbitMQ permissions for topology declaration, publish and consumption; the local shared broker credential is not a production policy.
7. Render with `kubectl kustomize deploy/k8s/base`, then validate your overlay against the target cluster before applying it.

## Operational boundaries

- OIDC issuer and JWKS endpoints must use HTTPS outside `APP_ENV=development`.
- Internal HTTP traffic currently uses network isolation and bearer-token verification. Add transport encryption (for example, mesh mTLS) in the target environment before carrying production credentials across nodes.
- Gateway readiness reports its own availability; service readiness checks PostgreSQL. Broker loss deliberately does not remove read APIs from service: existing batches remain available and commands use the outbox.
- Quorum queues on a single local RabbitMQ node are durable, not highly available. Production needs an appropriately sized multi-node broker or a managed offering.
- A dead-lettered job remains `queued` in this first slice. Operators must inspect DLQs; automatic terminal failure propagation/replay tooling is a subsequent work unit.
- Deployment replicas are not evidence of a measured availability SLA. Cluster rollout, failover, restoration, mTLS, external secret integration and load targets still require environment-specific validation.
