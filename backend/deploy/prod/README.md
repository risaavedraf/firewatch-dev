# Production deployment: one VM, Compose, Caddy edge

This is the deployment form recorded in S4 of `odd/tasks/plan-produccion/aws-ml-delivery.md`: the whole stack on one VM, driven by `docker-compose.prod.yml`, with Caddy as the only public edge. Kubernetes stays a documented target (`backend/deploy/k8s/README.md`), not the chosen path; nothing here deploys to ECS.

What runs where: `caddy` publishes **80** and **443** and nothing else does. The gateway, the identity provider, PostgreSQL, RabbitMQ, Prometheus and Grafana are reachable only on the internal `backend` network.

## 1. Before the first start

1. **VM.** EC2 with Ubuntu 22.04 or later: 8 GB of RAM (4 GB is tight with Keycloak, PostGIS, the four Go services and monitoring), 30 GB of disk, Docker Engine plus the Compose plugin (`docker compose version` must answer).
2. **Elastic IP** attached to the instance, so the address survives a stop and start.
3. **Security group.** Inbound 443 and 80 from `0.0.0.0/0` (80 is only for the ACME challenge and the redirect), 22 from the team's address only. No other inbound rule: every other port is served through Caddy or an SSH tunnel.
4. **Two DNS names pointing at the Elastic IP.** A free subdomain (for example Duck DNS) is enough; a registrar domain works the same:
   - `FIREWATCH_DOMAIN` — the page **and** the API. They share one origin on purpose, so the browser makes no cross-origin API call.
   - `FIREWATCH_AUTH_DOMAIN` (typically `auth.<the first name>`) — the identity provider, which the browser must reach to obtain a token.

   Both names must resolve **before** the first start: Caddy obtains a certificate per name over HTTP-01 and will not issue one while DNS disagrees. These are two explicit names, not a wildcard.

## 2. Variables

Compose reads `backend/.env` on the VM (write it there, `chmod 600`, never commit it). `python scripts/init_env.py` generates the random credentials and refuses to overwrite an existing file; the five variables marked *manual* below are not generated, so add them yourself.

| Variable | Example | Meaning |
|---|---|---|
| `FIREWATCH_DOMAIN` | `firewatch.example.com` | Public page and API origin. Feeds the gateway's `WEB_ORIGIN` and Caddy's first site. |
| `FIREWATCH_AUTH_DOMAIN` | `auth.firewatch.example.com` | Public identity provider origin. Feeds `KC_HOSTNAME` and every `OIDC_ISSUER`. |
| `ACME_EMAIL` | `ops@example.com` | Certificate expiry notices. Manual. |
| `MODEL_ID` | `firewatch-rule-v2` | Startup classifier, as in the development stack. Manual. |
| `KC_ADMIN_USERNAME` | `firewatch-admin` | Keycloak bootstrap administrator. Manual. |
| `POSTGRES_PASSWORD` | 48 hex chars | PostGIS superuser and migration owner. |
| `JOBS_DB_PASSWORD` | 48 hex chars | `jobs_app` role. |
| `RISK_DB_PASSWORD` | 48 hex chars | `risk_app` role (risk and prevention schemas). |
| `KEYCLOAK_DB_PASSWORD` | 48 hex chars | `identity_app` role. |
| `RABBITMQ_PASSWORD` | 48 hex chars | Broker administrator `firewatch`. It provisions the topology and it is the only identity that may use the management API; no service connects as it. |
| `RABBITMQ_JOBS_PASSWORD` | 48 hex chars | Broker identity `firewatch-jobs`, used by the `jobs` service. Manual, and distinct from the risk one. |
| `RABBITMQ_RISK_PASSWORD` | 48 hex chars | Broker identity `firewatch-risk`, used by the `risk` service. Manual, and distinct from the jobs one. |
| `KEYCLOAK_ADMIN_PASSWORD` | strong password | Bootstrap administrator password. |
| `DEMO_ADMIN_PASSWORD`, `DEMO_VIEWER_PASSWORD` | strong password | Realm users imported from the development realm. |
| `GRAFANA_ADMIN_PASSWORD` | strong password | Grafana `admin`. |

Every one of them is declared with `:?`, so a missing value stops the parse instead of starting a stack with a development credential:

```
$ docker compose -f docker-compose.prod.yml config
error while interpolating services.gateway.environment.WEB_ORIGIN:
required variable FIREWATCH_DOMAIN is missing a value: required
```

## 3. The three places that must agree on the browser origin

Login works only when the identity provider, the gateway and the built bundle name the same origin. Each of the following is one string, and they are not interchangeable:

1. **Keycloak client** `firewatch-demo` in realm `firewatch`:
   - Valid redirect URI: `https://$FIREWATCH_DOMAIN/`
   - Web origin: `https://$FIREWATCH_DOMAIN`
   - Post-logout redirect URI: `https://$FIREWATCH_DOMAIN/`
   - `KC_HOSTNAME` (compose) is `https://$FIREWATCH_AUTH_DOMAIN`, which is exactly the issuer every service verifies.
2. **Gateway** `WEB_ORIGIN=https://$FIREWATCH_DOMAIN` (compose). CORS is fail-closed and compares the `Origin` header byte for byte, so an origin that differs by a slash or a port is refused.
3. **Frontend bundle**: `VITE_GATEWAY_ORIGIN`, `VITE_ISSUER` and `VITE_REDIRECT_URI` from `frontend/.env.production`. `VITE_REDIRECT_URI` is the registered redirect URI, and `VITE_ISSUER` is `https://$FIREWATCH_AUTH_DOMAIN/realms/firewatch`.

A browser treats `http://localhost:4173` and `http://127.0.0.1:4173` as **different origins**, and the same is true of `https://firewatch.example.com` and `https://firewatch.example.com:443` written explicitly. The identity provider, the gateway and the bundle must therefore spell the origin identically, scheme and trailing slash included.

The realm this stack mounts (`deploy/local/firewatch-realm.json`) is the **development** realm and is imported only on first boot. It registers `http://localhost:4173/`, so after the first start, before any demo, open `https://$FIREWATCH_AUTH_DOMAIN` as `KC_ADMIN_USERNAME` and move the client's redirect URIs and web origins to the production strings above (or point the volume at a corrected copy). Nothing rewrites the realm automatically.

## 4. Build the page, then start

Caddy serves the built application from `frontend/dist`, mounted read-only. Build it before starting the stack, on the VM or on a machine that produces the same files:

```sh
cd frontend
npm ci
# .env.production holds the three VITE_* production values (see .env.production.example)
npm run build
```

A production build without those variables fails loudly instead of shipping a bundle that still calls `localhost`.

```sh
cd backend
docker compose -f docker-compose.prod.yml --profile observability up -d --build
docker compose -f docker-compose.prod.yml logs -f caddy
```

The migration services run to completion before the API replicas start; Keycloak imports the realm and obtains its certificate on first boot, so the first minute can show `502` at the edge. When it settles, `https://$FIREWATCH_DOMAIN/` serves the page and `https://$FIREWATCH_AUTH_DOMAIN/realms/firewatch/.well-known/openid-configuration` answers with the issuer the services expect.

Operations that only need a shell inside the network:

```sh
docker compose -f docker-compose.prod.yml ps
docker compose -f docker-compose.prod.yml logs -f gateway risk jobs prevention
docker compose -f docker-compose.prod.yml --profile observability up -d
```

Prometheus, Grafana and the RabbitMQ management UI have no published port. Reach them through an SSH tunnel to the container address:

```sh
ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' firewatch-prod-rabbitmq-1)
ssh -i key.pem -N -L 15672:$ip:15672 ubuntu@<elastic-ip>
# then http://localhost:15672  (Grafana 3000, Prometheus 9090, same recipe)
```

The address changes whenever the container is recreated, so read it again after a `down`/`up`.

## 5. Stop, and shut the instance down

```sh
cd backend
docker compose -f docker-compose.prod.yml down       # stops the stack, keeps the volumes
docker compose -f docker-compose.prod.yml down -v    # also erases Postgres, RabbitMQ, and the certificates
```

**Shut the instance down after every demo: the course requires it.** `aws ec2 stop-instances --instance-ids <id>` (or the console) ends the instance-hour charges; the EBS volume and the public IPv4 address keep accruing a small charge, so terminate the instance when the delivery is over. `caddy-data` holds the certificates: deleting that volume forces a re-issuance, and repeated re-issuance runs into the certificate authority's rate limits.

## 6. What this stack does not do

- Images are built on the VM from the `build:` sections; there is no registry push/pull, no immutable image tag and no rollback path yet (T11).
- The inference service is not part of this file. It joins when its slice lands, on the internal network, never published (S3).
- Prometheus and Grafana reuse `deploy/local/prometheus.yml`, `alerts.yml` and the Grafana provisioning files. There is no Alertmanager, so a firing alert is visible in the UI and nowhere else.
- PostgreSQL and RabbitMQ run as single containers with a volume each: no backups, no replication, no measured availability. That is the Compose form S4 records, not a highly available deployment.
- Keycloak runs in production mode behind Caddy (`KC_PROXY_HEADERS=xforwarded`); without the proxy in front it will refuse the hostname. Its realm is the imported development one, with users whose passwords come from `DEMO_*`.
