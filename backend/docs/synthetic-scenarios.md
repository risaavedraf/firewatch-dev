# Reproducible synthetic scenarios

The backend implements six coherent synthetic profiles, optional scenario selection in the job API, and scenario metadata in job/event/batch history. Generation uses no Faker, random state, external data, or system-clock dependency. **Unit tests and compilation pass. Fresh forward migrations and the full jobs/risk database suites ran green in isolated disposable PostgreSQL databases under service credentials, and the isolated authenticated harness ran green once end to end (exit 0) against its own disposable Keycloak/RabbitMQ/PostGIS stack. That disposable stack is separate from the existing local Compose stack, which was not rebuilt or upgraded. The current frontend still has no scenario selector.** Requests without a scenario retain the original configured fixture.

## Verify the available profiles

From `backend/services/risk/`:

```sh
GOWORK=off go test ./internal/source ./internal/service -run 'TestSynthetic' -count=1
```

The tests call the public `Provider.Fetch` and `Publisher.Publish` boundaries with a fixed evaluation instant. Publication uses the real academic classifier and a recording test writer, not PostgreSQL.

| ID | Temperature °C | Humidity % | Wind km/h | Vegetation % | Historical count | Expected demonstration outcome |
|---|---:|---:|---:|---:|---:|---|
| `mild` | 16 | 75 | 5 | 20 | 0 | `bajo` |
| `mixed` | 25 | 40 | 30 | 40 | 2 | `medio` |
| `dry-windy` | 32 | 18 | 45 | 65 | 4 | `alto` |
| `missing-humidity` | 32 | absent | 45 | 65 | 4 | `desconocido`, not low |
| `stale` | 32 | 18 | 45 | 65 | 4 | `desconocido`; observations are seven hours old |
| `partial-retrieval` | 32 | 18 | 45 | 65 | 4 | Publication rejected; writer not called |

These levels pin the behavior of `academic-rule-v1`. They are not labels obtained from actual fires, calibrated probabilities, operational thresholds, or evidence of model accuracy.

## Provider contract

Within the risk service, construct `source.NewSyntheticProvider(scenarioID, zoneID, evaluatedAt)` and use it through the existing `source.Provider` interface. Unknown scenario IDs, empty/whitespace-only zone IDs, and a zero evaluation instant are rejected. Scenario IDs are exact and case-sensitive. Zone labels do not prove spatial coverage; the API must continue to validate supported zones.

`source.SyntheticScenarios()` returns an independent, ordered catalog copy. Fetches return independent observation slices; callers cannot mutate subsequent retrievals. All records are declared `estimated`, and both the descriptor and provenance declare `synthetic=true`. They are private demonstration content, not an official observational source.

Every fetch computes `normalized_sha256` from its actual returned records. No HTTP status or payload hash is invented because nothing was downloaded. The observation cutoff and evaluation/retrieval instant remain distinct for the stale profile. Closed time windows filter records and recompute record count and hash, including an empty successful retrieval.

A missing observation differs from an unfinished acquisition:

- `missing-humidity` has `complete=true` but no humidity record; the classifier abstains.
- `partial-retrieval` has `complete=false`; the existing publication guard refuses it before persistence.

## Checklist evidence and limits

This slice contributes **partial synthetic-scope evidence**, not full checklist closure:

- `DAT-I2`, `DAT-I3`, `DAT-V1`–`DAT-V4`: canonical variables, units, finite values, demonstration domains, explicit absence, and incomplete-acquisition rejection are exercised. Production data ranges and source reliability remain pending.
- `MET-I1`, `MET-V1`: changing only the synthetic request's zone label leaves classification and rationale unchanged. This does not say real zones have identical conditions.
- `MET-I2`, `MET-V2`: with the other four inputs held fixed, the `academic-rule-v1` level is non-decreasing in temperature, wind speed, vegetation cover and historical fire count, and non-increasing in relative humidity, exactly as the rule's own `rising`/`falling` semantics and positive fixed weights document. `backend/services/risk/internal/classify/directional_metamorphic_test.go` pins this structural monotonicity of one frozen deterministic rule version; it is not a property of a trained model and is not evidence about real wildfire risk.
- `ROB-I1`: missing, stale, and partial-input scenarios are available, and extreme out-of-range finite values are pinned to saturate at the documented bounds without abstention by `backend/services/risk/internal/classify/extreme_inputs_test.go`. Scientific robustness metrics and team-approved tolerances remain pending; the tolerance threshold (`ROB-I2`) and measured degradation (`ROB-V1`, `ROB-V2`) remain open.
- No trained-model benchmark, fairness metric, drift measurement, load/SLA result, security approval, or Go/No-Go signature is claimed.

## API workflow and compatibility

After rebuilding all affected local services together and applying forward migrations (jobs 003, risk 007), an authorized API client can:

1. Read `GET /v1/risk/synthetic-scenarios` as viewer/admin. The response is the ordered array of `{id, description}` from the provider catalog, without database/broker access.
2. Submit `POST /v1/jobs` as admin with a UUID `Idempotency-Key` and `{"zone_id":"05101","scenario_id":"mixed"}`. Omit `scenario_id` to use the legacy fixture. Unknown profile IDs and client-supplied `evaluated_at` are rejected before persistence.
3. Poll the returned job, then read latest/history for the selected zone. Scenario-bearing jobs and batches expose `scenario_id` and `evaluated_at`; legacy responses omit them.

The server assigns the UTC evaluation instant once. Repeating the same actor/key returns the original job, scenario, and instant, even if the new valid body selects another profile; it does not create another outbox row. This is backend idempotency, not a claim that the current frontend reuses keys for transport retries.

Historical metadata stays NULL without backfill. The evaluation instant is not real observation freshness or a forecast horizon. Invalid request-event metadata is refused permanently before fetch/store. Incomplete acquisition remains a transient refusal, never a stored low-risk answer; `partial-retrieval` may leave the existing job in `queued` after retries/dead-letter handling because terminal failure events are not implemented.

These instructions describe implemented code and the declared contract. PostgreSQL persistence and the database suites are observed under isolated disposable databases, and the authenticated end-to-end path is observed once through the isolated harness; its skipped optional probe and known advisories are listed in [`synthetic-verification.md`](synthetic-verification.md). Never regenerate existing credentials or delete volumes to apply this change. Mixed old/new consumers are unsupported for scenario-bearing messages; local services must upgrade together.

## Required next proof

Forward migrations and the jobs/risk database suites run against disposable jobs/risk databases under the service credentials, and the isolated harness has now exercised the authenticated workflow through Keycloak, RabbitMQ and PostGIS (exit 0). Remaining gaps: the optional `--check-partial-retrieval` probe was skipped by default (provider/publisher refusal is proved separately in unit tests) and the harness advisories listed in [`synthetic-verification.md`](synthetic-verification.md) remain. Full browser login/logout and the future React selector require separate browser evidence. The harness ran a disposable stack; it did not rebuild or upgrade the existing local user stack.

See [`../../odd/tasks/backend/urgent-synthetic-mvp.md`](../../odd/tasks/backend/urgent-synthetic-mvp.md) for task evidence and [`../../Referencias/tareas_verificacion_implementacion_IA.md`](../../Referencias/tareas_verificacion_implementacion_IA.md) for authoritative checklist criteria.
