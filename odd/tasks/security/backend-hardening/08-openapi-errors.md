# BH-08 — Align OpenAPI errors with observable responses

**Status:** implemented — verified locally and re-verified inline on 2026-10-16 | **Priority:** P2 | **Dependencies:** BH-01 (met); coordinate BH-03; BH-12 is not required.

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

## Re-verification (2026-10-16, session A closure)

The independent verification was lost with a session restart, so it was repeated. The runtime refused the `gentle-ai-verify` launch because the delegated task declared no Task Size high-risk item, and the mechanism's instruction for that admission result is to run the focused checks inline; no retry was made, so this is a writer-side re-verification, not an independent one.

- **RED reproduced in isolation.** `750cd80` was exported to a scratch directory outside the repository and its contract replaced by the pre-change `02911d5` revision. `GOWORK=off go test -run Contract -count=1 ./internal/handler/` then failed in both modules, for the shape the tests name: gateway `contract documents no 429 response for GET /v1/risk/latest`; risk `contract documents no 503 response for GET /v1/risk/latest` and `contract documents no application/json body for GET /v1/risk/latest 404`. The tests genuinely constrain the contract rather than restating it.
- **GREEN on the current tree.** Both modules report `ok` for the same command, and their full suites are green.
- **Matrix spot-checked against the handlers, not the prose.** gateway `internal/handler/gateway.go:53,74-77,154,160-161,172` answers `502 upstream_unavailable`, `429 rate_limited` with `Retry-After: 1`, and `401 unauthorized`, each with `X-Request-ID` and `Cache-Control: no-store`; risk `internal/handler/sources.go:90-99` answers `geography_integrity_unavailable` and `risk_source_unavailable` with 503; risk `internal/handler/api.go:71` answers the `/v1/risk/latest` 404 as `{code: no_published_batch, data_status: insufficient}` with no `request_id`, which is the shape the document gives it; jobs `internal/handler/api.go:90` answers the missing content type with `415 json_required`. Every documented status, code and header checked corresponds to a reachable path, and no reachable error was found undocumented.
- **Validator.** `go run github.com/getkin/kin-openapi/cmd/validate@v0.133.0 contracts/openapi/firewatch-v1.yaml` exits 0 (same validator as CI).
- **Finding, closed.** `components/schemas/ErrorResponse.code` claimed *"The set is documented on each response; it is not a free-form message"*, which is false for every response that reuses a shared error response: those document the status and headers and enumerate no code. The description was corrected in its own commit so the document stops promising a per-response code list it does not carry.
- **Not verified here.** The `-race` pass (no C compiler on this host) and the database-side rules; CI covers both. No independent verifier ran against this candidate.

## Execution route and next action

**Route:** inline (contract edit, contract tests, local verification).

**Next action:** none for this unit. The contract diff is re-verified as above; the only open item it had left (the independent verification that a session restart lost) is replaced by that re-verification, with its limits stated rather than implied.

## Log

- **2026-10-16** — Mapped the operation-to-error matrix from the handler code, aligned `firewatch-v1.yaml` to it, and added contract tests that validate real gateway/service error paths against the contract. RED observed with the pre-change spec; both module suites green; `kin-openapi validate` exits 0.
- **2026-10-16 (session A closure)** — Re-verified inline: RED reproduced in an isolated export, GREEN on the current tree, the matrix spot-checked against the handler code, and the validator at exit 0. One over-promise was found in `ErrorResponse.code` and corrected in its own commit. The independent-verifier route was refused by runtime admission for a candidate that declares no high-risk item; that limit is recorded instead of being glossed over.
