# U2/U3 checklist evidence registry

Scope: units **U2** (data quality, bias and leakage) and **U3** (model evaluation, robustness and
regression) of `Referencias/tareas_verificacion_implementacion_IA.md`, sections 4 and 5. That document is
the authoritative criterion source; this file references its IDs and does not replace or restate them.

**No ID in this file is closed and no checklist box is marked.** A passing test and a marked box are two
different objects: a passing test observes one bounded behaviour that runs today, while a marked box
requires the ID's closing criterion — reproducible evidence, thresholds agreed with the team, a named
responsible person and, where the criterion says so, real data.

States used below are the ones the tracking document requires (implemented, partial, proposed, justified
N/A, pending):

| State | Meaning here |
|---|---|
| **Partial** | An executable in-repository artifact speaks to the ID. It is not the ID's closing criterion. |
| **Proposed** | An answer is written down but no responsible person has accepted it. |
| **Blocked** | Closing requires an input this repository does not contain: real data, an agreed threshold, or a named owner. |

Pointers below are anchors at the declaration being cited, not whole-file references.

## Standing limit on all synthetic evidence

The synthetic profiles, the built-in fixture and `academic-rule-v1` prove **code behaviour on invented
inputs**. They cannot prove real-data quality, source reliability, geographic or class representativeness,
statistical bias, absence of leakage in a trained model, or robustness against real perturbations.
`academic-rule-v1` is a documented own rule that is **not** calibrated against observed fires; no accuracy,
F1 or recall figure exists anywhere in this repository. Every row below inherits that limit.

## U2 — data quality, bias and leakage

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `DAT-I1` | Partial | `backend/services/risk/internal/source/fixture.go:30` declares the only registered risk-input source, marks it `Synthetic: true` and states in its attribution that it carries no real observations; `backend/contracts/gen/geo/valparaiso-comunas.provenance.json:2` records the real geographic asset (MOP/IDE Chile `mop-idechile-dpa-comunas`, endpoint, query sha256, 38 records, completeness flags, DIFROL restriction); `backend/services/risk/internal/handler/sources.go:100` with `backend/services/risk/internal/handler/sources_test.go:22`, `:32`, `:44` serve that inventory at `GET /v1/risk/sources` and refuse `503` — `backend/services/risk/internal/handler/sources.go:104` for a geography integrity failure, `backend/services/risk/internal/handler/sources_test.go:32` for a missing descriptor. | An inventory of the **production** observation sources. No real weather, vegetation or fire observation source exists, so the served list is one demo source plus one geographic asset. Also: who owns the production source list. |
| `DAT-I2` | Partial | `backend/services/risk/internal/source/synthetic_scenarios_test.go:15` asserts canonical variables, one unit per variable, finite values, percentage domains and non-negative wind/counts; `backend/contracts/gen/observations/v1/observations.go:158` fixes the canonical variable vocabulary shared by providers and the classifier. | The production validation rules and the physically valid ranges agreed for real inputs. The synthetic demonstration domains are not approved production ranges. |
| `DAT-I3` | Partial | `backend/services/risk/internal/service/synthetic_scenarios_test.go:195` refuses an incomplete retrieval before anything is stored and keeps the refusal transient; `:227` refuses invalid scenario metadata permanently before any acquisition. | The agreed action per failure class on **real** ingestion: reject the batch, alert someone, or impute — and who is alerted, through which channel. There is no real ingestion path to fail. |
| `DAT-V1` | Partial | `backend/services/risk/internal/source/synthetic_scenarios_test.go:15` verifies the expected fields exist for every complete profile and that the incomplete profile declares `Complete: false`; the refusal to publish that profile is proved separately at `backend/services/risk/internal/service/synthetic_scenarios_test.go:195`. | The real dataset's schema and a validation report against it. |
| `DAT-V2` | Partial | `internal/source/synthetic_scenarios_test.go:15` verifies variable/unit correspondence; `backend/contracts/gen/observations/v1/observations.go:61` defines the record fields. | Types and ranges of the real source payloads, and their normalization into the canonical vocabulary. |
| `DAT-V3` | Partial | `backend/services/risk/internal/classify/academic_rule_test.go:105` proves an absent input never degrades to low risk; `internal/source/synthetic_scenarios_test.go:15` proves a missing humidity record is not silently replaced. | Which fields are critical (non-nullable) in the real sources, and those sources' actual null policy. |
| `DAT-V4` | Partial | `internal/source/synthetic_scenarios_test.go:15` verifies finiteness, percentages inside 0–100 and non-negative wind/fire counts. | The production domain rules per variable, agreed with the team, plus real data checked against them. |
| `DAT-V5` | Blocked | Closest artifact only: `backend/contracts/gen/geo/valparaiso-comunas.provenance.json:2` records identity, hashes, completeness and restriction for the geographic asset. That is traceability, not a reliability verdict for a risk input. | A real risk-input source and a named reviewer confirming its reliability (coverage, cadence, uptime, issuing authority). |

## U2 — bias testing

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `SES-I1` | Blocked | None. | An owner to define the groups to protect or watch. Geographic zones and risk classes are available in principle; human demographic groups would require data this project does not hold. |
| `SES-I2` | Blocked | None. | An **agreed** parity metric and its threshold. The source document's `Disparate Impact Ratio > 0.8` is a template example, not an agreed value. |
| `SES-I3` | Blocked | None. | An agreed mitigation strategy, and the dataset to apply it to. |
| `SES-V1` | Blocked | None. | A labelled dataset with zone and class columns, then a coverage report by zone and by class. |
| `SES-V2` | Blocked | None. | Depends on `SES-I1`, `SES-I2` and `SES-V1`. |
| `SES-V3` | Proposed | The rule consumes only physical variables and a zone label: `backend/services/risk/internal/classify/classifier.go:84` has no human demographic field, and `:33` makes "unknown" a first-class level instead of a low-risk default. | The responsible person's acceptance of a justified N/A for human-group fairness, **and** a decision on whether geographic-zone and risk-class parity still applies — `SES-I1` lists both as candidate groups, so the N/A is not automatically complete. |

## U2 — leakage prevention

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `LEA-I1` | Blocked | None. | A training pipeline and a labelled dataset, then a documented temporal or geographic split with its justification. No model is trained in this repository. |
| `LEA-I2` | Partial | `backend/services/risk/internal/service/synthetic_scenarios_test.go:122` proves the scenario id does not enter the assessment factors (assertion at `:184`); the transport zone label is separately shown not to change the level or the rationale (`:20`, assertion at `:72`). | A complete giveaway-variable inventory, which requires the real feature list of a trained model. One demonstrated non-leak is not that inventory. |
| `LEA-V1` | Blocked | None. | A train set and a strictly later test set. |
| `LEA-V2` | Blocked | None. | A training run and a leakage audit over it. |

## U3 — metamorphic tests

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `MET-I1` | Partial | `backend/services/risk/internal/service/synthetic_scenarios_test.go:20` changes only the request's zone label and asserts the level and rationale are unchanged (assertion at `:72`). | The same invariance stated for the **real** feature set: which identifier is provably irrelevant to a trained model. |
| `MET-V1` | Partial | Same test as `MET-I1`; the invariance assertion is executed and reported on every run. | Recorded results for the real evaluation protocol (dataset, model version, reviewer). |
| `MET-I2` | Partial | `backend/services/risk/internal/classify/directional_metamorphic_test.go:53` sweeps each of the five canonical variables across its documented bounds with the other four held fixed, and asserts the documented direction for the factor contribution and the resulting level. The relation is documented by the rule's own `rising`/`falling` semantics (`internal/classify/academic_rule.go:199`, `:210`). | A documented directional relation for a trained model. Direction here is structural for one frozen deterministic rule version and does not transfer to a model that does not guarantee monotonicity. |
| `MET-V2` | Partial | The same test carries the justification: the direction holds because every weight in the frozen parameter set is positive (`internal/classify/params.go:125`) and `levelFor` is non-decreasing in the score (`internal/classify/academic_rule.go:187`), which `score` composes through the documented `rising`/`falling` helpers (`:199`, `:210`). | An equivalent justification for each directional case of a real model, or an explicit decision not to assume monotonicity for it. |

## U3 — adversarial robustness

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `ROB-I1` | Partial | `backend/services/risk/internal/source/synthetic_scenarios.go:23` defines six coherent profiles, of which three are adversarial inputs: `missing-humidity`, `stale` (observations seven hours old) and `partial-retrieval` (unfinished acquisition, `backend/services/risk/internal/service/synthetic_scenarios_test.go:195`). Extreme out-of-range finite values are separately pinned by `backend/services/risk/internal/classify/extreme_inputs_test.go:52`, which requires exact saturation at the documented bound and no abstention. | A defined scenario for gaussian noise, and above all a real input distribution to perturb. |
| `ROB-I2` | Blocked | None. | The tolerance threshold `X` (the criterion says the team defines it) and the performance metric it bounds. |
| `ROB-V1` | Blocked | None. | A measured baseline performance figure to compare against under perturbation. There is no ground truth and no accuracy metric in this repository. |
| `ROB-V2` | Blocked | None. | `ROB-I2`'s tolerance plus an agreed definition of "catastrophic". Without both, "no catastrophic failure" is not a testable statement. |

## U3 — model regression

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `REG-I1` | Blocked | None. | A candidate model with measured Accuracy, F1 and Recall, plus the minimum acceptable value for each. The source document's table is an unfilled template. |
| `REG-V1` | Blocked | None. | A re-training run that produces a challenger comparable to a champion. |
| `REG-V2` | Blocked | None. | The comparison above plus a named person to accept or refuse the replacement decision. |

## Summary

- 29 IDs registered: **0 closed**, **0 boxes marked**.
- 13 IDs hold partial synthetic evidence: `DAT-I1`, `DAT-I2`, `DAT-I3`, `DAT-V1`, `DAT-V2`, `DAT-V3`,
  `DAT-V4`, `LEA-I2`, `MET-I1`, `MET-I2`, `MET-V1`, `MET-V2`, `ROB-I1`.
- 1 ID is proposed but unsigned: `SES-V3`.
- 15 IDs are blocked on an input this repository cannot supply.

### Grouped missing inputs

1. **Real data** — a labelled observation dataset with zone and class columns (`DAT-V1`–`DAT-V5`, `SES-V1`,
   `SES-V2`, `LEA-*`, `ROB-V1`).
2. **An agreed threshold or definition** — parity metric and threshold (`SES-I2`), adversarial tolerance
   (`ROB-I2`, `ROB-V2`), Accuracy/F1/Recall targets (`REG-I1`).
3. **A trained model and its baseline** — a champion/challenger pair with measured metrics (`REG-*`,
   `ROB-V1`, `LEA-V1`, `LEA-V2`, and `MET-I2` for a model that does not guarantee monotonicity).
4. **A named owner** — for the production source inventory (`DAT-I1`), the protected groups (`SES-I1`),
   the mitigation strategy (`SES-I3`), the N/A justification (`SES-V3`) and the replacement decision
   (`REG-V2`).
5. **A real ingestion path** — to have schema failures, alerts and null policies to define (`DAT-I3`,
   `DAT-V3`, `DAT-V5`).
