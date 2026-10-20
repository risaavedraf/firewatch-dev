# FireWatch

Two parts, deliberately separate:

- [`backend/`](backend/README.md) — the executable slice: authenticated job submission, asynchronous processing through RabbitMQ, a 15-minute ingestion cycle with per-record validation, synthetic PostGIS batch publication, preventive actions, model-version activation, history, audit and Prometheus metrics.
- [`frontend/`](frontend/README.md) — the browser page, served on its own origin and calling the backend API.

## Running the local demonstration

```sh
cd backend
python scripts/init_env.py
docker compose up -d --build
python scripts/smoke.py
```

That starts the API on `http://localhost:8080`. It does **not** open a web page: the gateway is API only and answers `404` on `/`. Start the page separately — it is a Vite build, so it must be built before it is served — and open `http://localhost:4173`:

```sh
cd frontend
npm install
npm run build   # serve.mjs serves dist/, so build first
npm run serve
```

Open it as `localhost`, never as `127.0.0.1`. The identity provider and the gateway both have the `localhost:4173` origin registered, and a browser treats the two spellings as different origins.

## Documentation

- [Backend: run, API walkthrough and limits](backend/README.md#quick-start)
- [Frontend: the page and how to run it](frontend/README.md)
- [Architecture and delivery boundaries](backend/docs/architecture.md)
- [HTTP contract](backend/contracts/openapi/firewatch-v1.yaml)
- [Keycloak realm setup for the separated frontend](backend/docs/keycloak-realm-setup.md)
- [Kubernetes deployment base](backend/deploy/k8s/README.md)
- [Original project references](Referencias/)

The current backend uses demonstration data only. It does not estimate real wildfire risk.

**Delivery status (2026-10-18).** The presentation delivery landed on `main` at `f509666` (PR #31, issue #30): ingestion cycle, model v2, preventive actions and browser login. The local monitoring wave followed in PRs #33–#35, so the current `main` baseline is `ef686c0` (the PR #41 merge). The trained model and real data sources are still pending. See the [documentation map](odd/README.md) for where each state lives, and the [task index](odd/tasks/README.md) for the delivered/missing state and its limits.
