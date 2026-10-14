# FireWatch

Two parts, deliberately separate:

- [`backend/`](backend/README.md) — the executable slice: authenticated job submission, asynchronous processing through RabbitMQ, synthetic PostGIS batch publication, history and audit.
- [`frontend/`](frontend/README.md) — the browser page, served on its own origin and calling the backend API.

## Running the local demonstration

```sh
cd backend
python scripts/init_env.py
docker compose up -d --build
python scripts/smoke.py
```

That starts the API on `http://localhost:8080`. It does **not** open a web page: the gateway is API only and answers `404` on `/`. Start the page separately, from the repository root, and open `http://localhost:4173`:

```sh
node frontend/scripts/serve.mjs
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
