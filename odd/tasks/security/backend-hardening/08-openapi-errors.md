# BH-08 — Align OpenAPI errors with observable responses

**Status:** implemented — verified locally; awaiting independent verification (contract change is a high-risk item) | **Priority:** P2 | **Dependencies:** BH-01 (met); coordinate BH-03; BH-12 is not required.

## Evidence and scope

`contracts/openapi/firewatch-v1.yaml` omitted reviewed GET 503 and gateway 502/429 responses, and its 400/401/403 responses carried a description and no body at all, while every real error body is `{"code": ..., "request_id": ...}`. Re-verified on 2026-10-16 against the running handler code; the gaps below are derived from that behavior, not assumed.

## Operation-to-error matrix (from current behavior)

Gateway wrappers apply to **every** documented `/v1` operation, because the limiter runs before routing and the proxy error handler answers a failed upstream:

| Status | code | Applies to | Headers |
|---|---|---|---|
| 429 | `rate_limited` | every operation (global limiter) | `X-Request-ID`, `Retry-After: 1`, `Cache-Control: no-store` |
| 502 | `upstream_unavailable` | every operation (proxy transport error) | `X-Request-ID`, `Cache-Control: no-store` |
| 401 | `unauthorized` | every operation (missing/invalid bearer) | `X-Request-ID`, `Cache-Control: no-store` |

Operation-owned statuses (source: `risk/internal/handler/api.go`, `sources.go`, `jobs/internal/handler/api.go`):

| Operation | 400 | 404 | 415 | 503 | Error body |
|---|---|---|---|---|---|
| GET /v1/risk/status | — | — | — | `OperationalStatus` | not the shared error body |
| GET /v1/jobs/status | — | — | — | `OperationalStatus` | not the shared error body |
| POST /v1/jobs | `invalid_idempotency_key`, `invalid_demo_request` | — | `json_required` | `storage_unavailable` | `ErrorResponse` |
| GET /v1/jobs/{id} | `invalid_job_id` | `job_not_found` | — | `storage_unavailable` | `ErrorResponse` |
| GET /v1/risk/latest | `invalid_zone` | `no_published_batch` + `data_status` | — | `storage_unavailable` | 404 is its own shape (no `request_id`); the rest are `ErrorResponse` |
| GET /v1/risk/history | `invalid_zone` | — | — | `storage_unavailable` | `ErrorResponse` |
| GET /v1/risk/synthetic-scenarios | — | — | — | — (reads no storage) | — |
| GET /v1/risk/sources | — | — | — | `geography_integrity_unavailable` / `risk_source_unavailable` | `ErrorResponse` |
| GET /v1/risk/zones | `invalid_bbox` | — | — | `storage_unavailable` | `ErrorResponse` |
| GET /v1/audit | — | — | — | `storage_unavailable` | `ErrorResponse` |

Out of scope, and deliberately not documented: the gateway's `404 not_found` for a non-`/v1` path and its `204` health answers, because no operation in the document reaches them.

## Change summary

- Added `components/schemas/ErrorResponse` (`code` + `request_id`), `components/headers/RequestID`, and shared responses `BadRequest`, `Unauthorized`, `Forbidden`, `NotFound`, `RateLimited` (with `Retry-After`), `UpstreamUnavailable`, `StorageUnavailable`.
- Added `429` and `502` to every operation; added the missing `503` to GET `/v1/jobs/{id}`, `/v1/risk/latest`, `/v1/risk/history`, `/v1/audit`.
- Typed the previously bare `415` (POST `/v1/jobs`), `503` (`/v1/risk/sources`) and `404` (`/v1/jobs/{id}`).
- Documented the `/v1/risk/latest` `404` as its own empty-state body (`code` + `data_status`, no `request_id`) instead of forcing it into the shared error schema.
- BH-12 left deferred and untouched.

## Acceptance checklist

- [x] Document GET 503 and gateway 502/429 where actually applicable — see the matrix and the changed operations.
- [x] Use shared error schemas and describe observable request-ID/header behavior without inventing guarantees — `ErrorResponse`, `X-Request-ID`, `Retry-After`; the two non-error bodies are documented as such.
- [x] Exercise representative real handler/gateway error paths against the contract, not only schema syntax — `gateway/internal/handler/contract_test.go` (401/429/502) and `risk/internal/handler/contract_test.go` (503/404), both loading `firewatch-v1.yaml` and validating the real response against it.
- [x] Keep BH-12 deferred without making centralized handling a prerequisite for parity.

## Verification — run locally

- `go test ./...` — gateway: `ok firewatch.local/gateway/internal/handler`; risk: all packages ok.
- `go run github.com/getkin/kin-openapi/cmd/validate@v0.133.0 contracts/openapi/firewatch-v1.yaml` — exit 0.
- RED proof: with the pre-change spec (`git stash` of the yaml), both contract tests fail (`contract documents no 429/503 response`, `contract documents no application/json body for 404`), so the tests genuinely constrain the spec.
- Not run: the CI `-race` pass; the service contract tests read the spec via a relative path and are not exercised outside a checkout.

## Execution route and next action

**Route:** inline (contract edit, contract tests, local verification).

**Next action:** independent verification of the contract diff — the change touches item 3 (a published contract other consumers read).

## Log

- **2026-10-16** — Mapped the operation-to-error matrix from the handler code, aligned `firewatch-v1.yaml` to it, and added contract tests that validate real gateway/service error paths against the contract. RED observed with the pre-change spec; both module suites green; `kin-openapi validate` exits 0.
