# BH-05 — Enforce secure internal and data-service transport

**Status:** enforced at startup in all four services and covered by tests; `main` issues one server certificate per service, signed by the internal CA, and the runtime Secret bundles the Deployments mount, so provisioning no longer blocks the delivery. What remains is mounting those certificates and their CA into the workloads and moving the gateway upstreams to HTTPS (T7.7), then applying the base to a cluster and verifying it there (T9) | **Priority:** P1 | **Dependencies:** BH-01; coordinate BH-04 and BH-06.

## Evidence and scope

Before this unit, the OIDC endpoints were the only transport that anything refused to reach in clear. Everything else ran plaintext with no guard:

| Transport | Before |
| --- | --- |
| service → PostgreSQL | `sslmode=disable` in every compose file, nothing checked at startup |
| `jobs`/`risk` → RabbitMQ | `amqp://`, nothing checked at startup |
| gateway → `jobs`/`risk`/`prevention` | `http://`, nothing checked at startup |
| browser / IdP | already HTTPS, terminated by Caddy with ACME |
| `prevention` → Keycloak Admin API | already refused plaintext outside development |

## Transport paths and trust ownership

| Path | Terminates at | Trust source | Who owns the certificate |
| --- | --- | --- | --- |
| Browser → edge | Caddy | public CA via ACME | the deployment, renewed automatically |
| Edge → gateway | Caddy | internal | the deployment |
| Gateway → services | each service | internal CA | the deployment: `bootstrap-secrets.sh` issues one server certificate per service, naming the Service and its `<name>.<namespace>.svc` forms as SANs, and the gateway loads that CA from `TLS_CA_FILE`. **No Deployment mounts the certificate yet**, so the base manifests still serve these hops in plaintext (T7.7) |
| Services → PostgreSQL | PostgreSQL | internal CA | the deployment |
| `jobs`/`risk` → RabbitMQ (AMQP) | RabbitMQ | internal CA | the deployment |
| Services → IdP (OIDC, JWKS, Admin API) | Keycloak | public CA via the ingress | the identity deployment |

## The exception

`APP_ENV` must be exactly `development`. That is the same predicate the OIDC checks already use, so there is one way to express the exception and no second one can drift from it. An unset, empty, `Production` or `Development` value is a deployment and fails closed.

## What is enforced now

- **Database.** `requireTLSOutsideDevelopment` tests `cfg.ConnConfig.TLSConfig`, not the URL text: pgx leaves it nil for every mode that negotiates no TLS, so `sslmode=disable` cannot slip through a spelling the guard does not recognise. Error: `DATABASE_URL must require TLS outside development`.
- **Broker.** `requireSecureBrokerURL` requires the `amqps` scheme, and additionally refuses `verify=verify_none`. A TLS connection with verification off protects nothing against an active attacker, and it is what someone reaches for when a certificate check starts failing.
- **Gateway upstreams.** `requireSecureUpstreams` requires `https` for each non-empty upstream and names the offending variable. The names are sorted so two replicas of the same misconfiguration print the same message.

Each guard sits on a small seam (`startupBrokerURL`, `startupTransport`) so the refusal is testable without a database, a broker, or a listener. The broker guard runs after the migrate branch on purpose: a migration job holds only `DATABASE_URL` and never dials the broker.

Tests cover, per guard: production with the insecure form returns the exact message; production with the secure form passes; development with the insecure form passes; an empty `APP_ENV` fails; `Development` capitalised fails; `amqps` with `verify_none` fails.

## What is not done

- **The certificates are issued but not mounted.** `deploy/k8s/base/workloads.yaml` sets neither `TLS_CERT_FILE`, `TLS_KEY_FILE` nor `TLS_CA_FILE` and mounts no `*-tls` Secret, so with `APP_ENV: staging` its pods refuse to start rather than serve in clear (T7.7 in `odd/tasks/kubernetes-deployment.md`). Nothing has been applied to a cluster, so no pod has mounted one of these certificates and no handshake has run there (T9). The production compose file is fail-closed for the same reason: it will refuse to start until its URLs point at TLS endpoints with a root certificate, which is the intended behaviour of a guard and not a working deployment.
- The Kubernetes manifests carry the TLS wiring for the **data plane** only (PostgreSQL and RabbitMQ server certificates from an internal CA, `amqps` only, plaintext AMQP listener disabled). They carry nothing for the gateway's upstreams: `deploy/k8s/base/config.yaml` still points at `http://`, and no application Deployment mounts a certificate, so that base cannot start the gateway as written. **They have never been applied**; see `odd/tasks/kubernetes-deployment.md`.
- The verification cases that need a live environment remain unrun: a secure connection succeeding, an untrusted certificate rejected, an expired certificate rejected, and a hostname mismatch rejected. None can be claimed from a manifest.
- mTLS was considered and not chosen: inside a single-cluster network the client would have to be identified by a certificate the same cluster issued, and service identity is already carried by the bearer token and, for the broker, by the AMQP user whose permissions BH-04 measures. Revisit it if services ever span networks the project does not control.

## Acceptance checklist

- [x] Transport paths, termination points, trust sources and certificate ownership are documented above.
- [x] Insecure configurations are refused at startup outside development.
- [x] The development-only exception is explicit and fails closed for any other value.
- [ ] Application settings and deployment configuration are aligned: no longer blocked on provisioning the certificates. `fcbabaf`, now in `main`, emits one certificate per service and wires the runtime environment that consumes them. Still unverified: the deployment security verification is pending, because nothing has been applied to a cluster yet — see T9 in `odd/tasks/kubernetes-deployment.md`.

## Log

- **L1 (2026-11-09)** — Guards added to `jobs`, `risk`, `prevention` and the gateway with tests for the false cases. The database guard checks pgx's negotiated TLS state rather than the URL text, because a string match would accept a spelling it did not anticipate. Full suites pass: jobs 6 packages, risk 12, prevention 8, gateway 4.
