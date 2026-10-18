# U1 checklist evidence registry

Scope: unit **U1** (governance, security and model/data protection) of
`Referencias/tareas_verificacion_implementacion_IA.md`, security-plan sections 1, 2 and 3. That document
is the authoritative criterion source; this file references its IDs and does not replace or restate them.

**No ID in this file is closed and no checklist box is marked — 0 checkboxes marked.** A passing test, a
documented control and a marked box are three different objects: a passing test observes one bounded
behaviour that runs today, while a marked box requires the ID's closing criterion — reproducible
evidence, thresholds agreed with the team, a named responsible person and, where the criterion says so,
real data.

Sections 1–3 are, with few exceptions, organisational and security-plan criteria: an internal
AI-risk-classification policy, a named AI Security Officer, a signed pre-production approval, a threat
model, an incident-response plan, a rollback drill. For those, the expected result of inspecting an
academic demonstration repository is an **absent artifact**, and that is recorded below as `Blocked` or
`Pending` with the exact missing input named, never padded with unrelated code behaviour. The clauses
that do carry real in-repository artifacts are the access-control, provenance, input-validation and
containerisation controls; even there, a documented control is not the ID's closing criterion.

States used below are the ones the tracking document requires (implemented, partial, proposed, justified
N/A, pending):

| State | Meaning here |
|---|---|
| **Partial** | An executable in-repository artifact speaks to the ID. It is not the ID's closing criterion. |
| **Pending** | The ID is a defined, self-contained work item that is not started and is not waiting on an external input. |
| **Blocked** | Closing requires an input this repository does not contain: real data, an agreed threshold, a named owner, a signature or an organisational decision. |

The tracking document also defines **implemented**, **proposed** and **justified N/A**. No row uses
implemented — no criterion here is closed — and no justified N/A is asserted, because every N/A this
unit could argue (for example prompt injection against a non-generative model) still needs a named owner
to accept it, and is therefore registered below as blocked, not as justified.

Pointers below are anchors at the declaration being cited, not whole-file references. No test, build,
scanner, migration or workflow was executed to produce this registry; every anchor is a file read. The
only command executed afterwards, during parent review of this file, is the ancestry check recorded in the
`PRV-V2` row. Where a check is unavailable, that is stated instead of claimed as passing.

## Standing limit on all synthetic evidence

The synthetic profiles, the fixed fixture and `academic-rule-v1` prove **code behaviour on invented
inputs**. They cannot prove real-data quality, source authenticity, source reliability, geographic or
class representativeness, statistical bias, absence of leakage in a trained model, or robustness against
real perturbations. `academic-rule-v1` is a documented own rule that is **not** calibrated against
observed fires; no accuracy, F1 or recall figure exists anywhere in this repository. In this unit the
limit extends to security: passing guardrail tests over synthetic requests cannot prove that real
adversarial input is blocked, that a container is isolated on a hostile network, or that real third-party
artifacts are free of backdoors. Every row below inherits that limit.

## 1.1 Governance and risk classification

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `GOB-I1` | Pending | None. No internal AI risk-classification policy exists. The only classification in the repository is a *wildfire* severity level (`bajo`/`medio`/`alto`/`desconocido`) produced by the own rule at `backend/services/risk/internal/classify/classifier.go:33`, which is not an AI-risk scale. | The policy document itself, with the low/limited/high/unacceptable scale written down and mapped to the chosen regulation. A named approver is required for its adoption, but the missing input at this point is the drafted policy. |
| `GOB-I2` | Blocked | None. No classification of the fire-risk system inside an AI-risk scale exists; `backend/docs/architecture.md:63` lists model approval as a future requirement, not a category. | The scale from `GOB-I1` plus a justified category for this system, decided and accepted by the responsible owner. This is an organisational classification decision, not repository code. |
| `GOB-I3` | Blocked | None. No role, no name and no appointment record exist; the ODD unit itself leaves the responsible person to be confirmed (`odd/urgent/01-seguridad-gobierno.md`). | The name of the designated AI Security Officer (or equivalent) and the appointment record. A named individual is the exact missing input. |
| `GOB-I4` | Blocked | Closest artifact only: `backend/docs/architecture.md:63` states that model approval remains a requirement for a subsequent slice. No approval procedure or signer is defined anywhere. | The pre-production model-approval procedure and the name of who signs it, together with the evidence required. It depends on a named responsible person and an organisational decision. |
| `GOB-V1` | Blocked | None. No EU AI Act (or other regulation) consistency assessment exists. | The classification from `GOB-I1`/`GOB-I2` plus a regulatory consistency review accepted by the responsible owner. It cannot be produced before those decisions. |
| `GOB-V2` | Blocked | None. No production promotion path exists to gate: `backend/docs/architecture.md:57` records that no workflow has run and that registry promotion and automated rollout are later work, and `backend/docs/synthetic-scenarios.md:47` states no security approval is claimed. | An approval gate in a real production pipeline plus a named owner who verifies that no model reaches production without their approval. Both are outside this repository. |

## 1.2 AI threat modelling

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `AME-I1` | Pending | None. No MITRE ATLAS exercise, threat catalogue or data-flow diagram for the AI design exists. | The threat-modelling exercise itself and its output. It is self-contained analytical work; no real data or external signature is the blocker at this point. |
| `AME-I2` | Pending | None. No document enumerates inversion, extraction, prompt injection or other ATLAS vectors with an assigned mitigation. | The vector-by-vector register with one assigned mitigation per vector. It depends on `AME-I1`, whose output does not exist. |
| `AME-V1` | Blocked | None. There is no threat model to cross-check, and the section 2/3 controls it would map onto are themselves partial or missing (see the rows below). | The completed register from `AME-I2` and the section 2/3 controls it must map onto, plus a named owner to accept the mapping. |
| `AME-V2` | Blocked | The repository substantiates a *candidate* N/A: the classifier input contract carries only two instants, a zone label and five physical variables, with no generative text surface (`backend/services/risk/internal/classify/classifier.go:84`, `:87`), so prompt injection is not obviously applicable. No written N/A justification exists. | The written N/A justification for each non-applicable vector and **a named owner who accepts it**. The code supports the claim but cannot sign it. |

## 1.3 Zero trust architecture

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `ZT-I1` | Partial | The inference pipeline is segmented: Compose puts the applications on an `internal: true` network with no host ports (`backend/docker-compose.yml:256`), and Kubernetes applies `default-deny` plus the `gateway-entry` and `service-entry-and-data` `NetworkPolicy` objects (`backend/deploy/k8s/base/network.yaml:3`, `:27`, `:45`). No model or training-data store exists to segment. | Segmentation of (a) model artifacts and (b) training data — neither exists — and an owner's acceptance that the applied policies are the intended segmentation. |
| `ZT-I2` | Partial | API authentication exists on both services (`backend/services/risk/internal/handler/security.go:33`, `backend/services/jobs/internal/handler/security.go:18`, wrapped at `backend/services/risk/internal/handler/api.go:106`), and the database uses per-service credentials (`backend/deploy/local/postgres-init.sh:6`, `:7`, `:19`). Prometheus is defined without authentication and bound to loopback only (`backend/docker-compose.yml:161-168`), and Grafana exists with anonymous access disabled and a required admin password (`:176-189`). | Authentication for Prometheus, and the transport/secret decisions still pending at `odd/tasks/security/backend-hardening/05-transport-security.md:7`. The bearer checks are application-level, not a zero-trust service mesh. |
| `ZT-I3` | Partial | Least privilege is partly applied: distinct `jobs_app`/`risk_app` DB roles without DDL (`backend/deploy/local/postgres-init.sh:6`, `:7`), non-root/no-capability/read-only containers (`backend/services/risk/Dockerfile:15`; `backend/docker-compose.yml:10`, `:11`, `:13`; `backend/deploy/k8s/base/workloads.yaml:16`, `:34`). But both applications share one broker identity (`backend/docker-compose.yml:45`, `:108`, `:119`). | The per-service broker identities and route permissions of BH-04 (`odd/tasks/security/backend-hardening.md:20`), and a named owner to accept the resulting permission matrix. |
| `ZT-V1` | Partial | The current access posture is documented and partly tested: every `/v1/*` request goes through `Authorize`, and the role matrix is pinned by `backend/services/risk/internal/handler/api_test.go:66` and `backend/services/risk/internal/handler/sources_test.go:44`, `:132`. The same repository records a **known implicit-access gap**: the shared broker credential above. | A completed access review that resolves the shared-credential finding and is accepted by a named owner. Documenting a gap is not the verification the ID asks for. |
| `ZT-V2` | Blocked | None. No permission-audit schedule, no audit record and no owner exist. | A periodic permission-audit procedure, its schedule, and a named person who runs and signs it. This is an ongoing organisational commitment, not a code artifact. |

## 2.1 Data provenance and supply chain

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `PRV-I1` | Partial | Lineage is recorded for the geographic asset: `backend/contracts/gen/geo/valparaiso-comunas.provenance.json:2` records source id, provider, endpoint, query hash, source and derived formats, record counts and attribution, and the served inventory exposes it (`backend/services/risk/internal/handler/sources.go:79`). The synthetic provider also carries provenance with a per-fetch payload hash (`backend/services/risk/internal/source/synthetic_scenarios.go:140`) and `Synthetic: true` (`backend/services/risk/internal/source/fixture.go:54`). | Lineage for a **real risk-input dataset** (origin, transformations, version) — none exists — and for any training dataset. The geographic asset's lineage is real and complete, but it is a map, not a risk input. |
| `PRV-I2` | Partial | Cryptographic validation is implemented for the geographic boundaries: the SHA-256 of the embedded GeoJSON is compared with the recorded `derived_sha256` (`backend/contracts/gen/geo/valparaiso-comunas.provenance.json:20`) at `backend/services/risk/internal/handler/sources.go:74`, `:81`, failing closed with `503` (`:104`); the corrupt-artifact case is exercised at `backend/services/risk/internal/handler/sources_test.go:102`. | Cryptographic validation of **real risk-input sources before ingestion** (there is no ingestion path) and signature verification as distinct from content hashing. The existing check covers one bundled geographic file. |
| `PRV-I3` | Partial | Third-party dependencies are pinned and checked: `go mod verify` and a `govulncheck` step (`.github/workflows/_go-service.yml:30`, `:36`), an SBOM/provenance build and a Trivy image scan (`:54`, `:62`), with historical results recorded at `backend/docs/verification.md:15`, `:16`, `:17`. | A maintained dependency/model **inventory** with owners, and any pre-trained-model inventory (none is used). The hardening review also states the fresh scan baseline is still pending (`odd/tasks/security/backend-hardening/02-vulnerability-scans.md:7`). |
| `PRV-V1` | Partial | Integrity is verified before the geographic artifact is served — the closest in-repository analogue of an ingest check — and fails closed on mismatch (`backend/services/risk/internal/handler/sources.go:81`, `:104`). | Verification of a **real input source before each ingestion**. No ingestion path and no external risk source exist, so there is nothing to verify at ingest time. |
| `PRV-V2` | Partial | A vulnerability/backdoor review is partly recorded: `govulncheck` found no vulnerabilities in the three modules and Trivy found no HIGH/CRITICAL in the three application images (`backend/docs/verification.md:15`, `:16`), and CI is configured to run both. | A **fresh, authoritative scan** against the actual deployment inventory, accepted by a named owner; the review states there is no current clean scan claim (`odd/tasks/security/backend-hardening/02-vulnerability-scans.md:7`), the sentence at `backend/docs/verification.md:68` saying the SBOM/scan steps "have never executed on the runner" described `main@0ed07e2` before the toolchain refresh and has since been corrected in this branch; the steps now succeed, after `c45508e` enables the containerd image store (`odd/tasks/security/ci-go-1.27.2-green.md:52`, `:131`; `c45508e` is an ancestor of `main` and of this branch, checked during parent review with `git merge-base --is-ancestor`), and no pre-trained-model audit exists. |

## 2.2 Privacy by design and anonymisation

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `PRI-I1` | Blocked | The classifier input contract has no personal-data field: `backend/services/risk/internal/classify/classifier.go:84`, `:87` carry two instants, a zone label and five physical variables only, and the canonical vocabulary is closed (`backend/contracts/gen/observations/v1/observations.go:152`). That describes the synthetic inputs, not the real dataset's sensitivity. | A real dataset to inspect and a data owner's written determination of whether it contains personal or sensitive data. The absence of personal fields in the synthetic contract cannot stand in for that determination. |
| `PRI-I2` | Blocked | None. No differential-privacy or anonymisation mechanism is applied anywhere. | The `PRI-I1` determination and, if personal data is present, an agreed anonymisation/differential-privacy mechanism and its parameters. Both are external inputs. |
| `PRI-I3` | Blocked | None. No homomorphic-encryption or SMPC evaluation exists. | The `PRI-I1` determination plus a written N/A justification (or a mechanism) accepted by a named owner. |
| `PRI-V1` | Blocked | None. No training data and no trained model exist, so there is nothing to reidentify; `backend/docs/architecture.md:63` lists model work as a future slice. | A real training dataset, a trained model, and a reidentification analysis accepted by a named owner. |

## 2.3 Adversarial training

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `ENT-I1` | Blocked | None. No training pipeline exists; the classifier is a frozen deterministic rule (`backend/services/risk/internal/classify/academic_rule.go:31`, whose default parameters are frozen by a hash as documented at `:24-26`), not a model trained on a dataset, and `backend/docs/synthetic-scenarios.md:47` states no trained-model benchmark is claimed. | A training pipeline and a labelled dataset, then noisy/deceptive examples added to training. The whole input class is absent. |
| `ENT-I2` | Blocked | None. There is no perturbation record because there is no training. | The `ENT-I1` pipeline plus the documented perturbation type and volume. |
| `ENT-V1` | Blocked | None. No comparison of performance with and without adversarial training exists; there is no accuracy metric anywhere. | A trained model with measured metrics under both regimes, and a named owner to accept the comparison. |

## 3.1 Input/output guardrails

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `GRD-I1` | Partial | Input validation is implemented and tested on the API path: bounded body read and strict decode (`backend/services/jobs/internal/handler/api.go:98`, `:112-114`), explicit rejection of null/empty/unknown `scenario_id` before any lookup or write (`:127-133`, `:138-140`; tests at `backend/services/jobs/internal/handler/synthetic_scenarios_test.go:80`, `:81`, `:137`), zone-shape and zone-membership guards (`backend/services/risk/internal/handler/api.go:59`, `:63`, `:69`), and fail-closed event validation (`backend/services/risk/internal/service/demo.go:33`, exercised at `backend/services/risk/internal/service/demo_test.go:36`). | A **malicious-content** layer (the criterion's jailbreak/prompt-injection case) and the agreed production ranges. The service has no generative surface, so that case may be N/A — but the N/A needs the owner decision described under `AME-V2`. |
| `GRD-I2` | Partial | Output hygiene is partly present: the shared responder sets `X-Content-Type-Options: nosniff` and `Cache-Control: no-store` (`backend/services/risk/internal/handler/security.go:86`, `:87`), and the API returns risk levels, factors, GeoJSON and error codes rather than free text. | An explicit **output content review** (data-leak and harmful-response check) and the data-classification decision that would define what "sensitive" means here. Security headers are not that review. |
| `GRD-V1` | Partial | Manipulation attempts are exercised: oversized/strict bodies, explicit null/empty `scenario_id`, unknown scenario ids and unknown roles all fail closed in the tests cited for `GRD-I1`, and negative authentication cases are pinned at `backend/services/risk/internal/handler/api_test.go:66`. | A manipulation/adversarial test plan accepted by a named owner, and the N/A decision for injection cases that may not apply. The existing tests prove specific malformed inputs, not the criterion's test plan. |
| `GRD-V2` | Blocked | None. No test or report confirms that outputs expose no sensitive information. | A completed output-sensitivity inspection plus the data-classification decision (`PRI-I1`) that determines what sensitive means. Without that classification the confirmation is not a testable statement. |

## 3.2 Sandboxing

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `SBX-I1` | Partial | The inference service is containerised: the risk image builds a static binary and runs on distroless nonroot (`backend/services/risk/Dockerfile:13`, `:15`), and Compose runs it read-only, without capabilities, as UID 65532 (`backend/docker-compose.yml:10`, `:11`, `:13`); Kubernetes repeats the hardening (`backend/deploy/k8s/base/workloads.yaml:16`, `:34`). | An owner's acceptance and a real deployment: the Kubernetes image is a `replace-before-deploy` placeholder, and `odd/tasks/security/backend-hardening/11-deployment-security-verification.md:7` states static configuration does not prove the deployed controls. |
| `SBX-I2` | Partial | Network restriction is declared, not complete: Compose marks the application network `internal: true` (`backend/docker-compose.yml:256`) and Kubernetes applies `default-deny` and service-scoped egress (`backend/deploy/k8s/base/network.yaml:3`, `:27`, `:45`). But the `dns-and-oidc` policy still allows egress TCP 443 to any IPv4 address (`:22`). | The narrowed egress of BH-06 (`odd/tasks/security/backend-hardening/06-kubernetes-egress.md:7`), an environment overlay, and confirmation that the CNI enforces the policies. Internet egress is currently **not** blocked by default. |
| `SBX-V1` | Blocked | None. No isolation test was run and no authorized environment exists; the ODD hardening index keeps deployment validation pending (`odd/tasks/security/backend-hardening.md:27`). | An authorized cluster/network in which to demonstrate that the container reaches no corporate or unauthorized external host, plus a named owner to accept the result. |

## 3.3 Drift monitoring (security view)

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `OBS-I1` | Blocked | Closest artifact only: a `/metrics` surface exists (`backend/services/risk/cmd/server/main.go:217`) exposing request/broker/event counters (`backend/services/risk/internal/messaging/broker.go:79`, `:88`), Prometheus scrapes all four services (`backend/deploy/local/prometheus.yml:8-32`), and `backend/deploy/local/alerts.yml` carries seven rules. There is still no input-distribution or model-confidence metric. | A distribution/confidence metric and the agreed thresholds that make an alert meaningful. Threshold agreement is the blocking input; the metric surface alone cannot close the ID. |
| `OBS-V1` | Blocked | None. No deviation can be simulated because no alert exists to fire. | An alert rule with a threshold (`OBS-I1`) and an authorized environment in which to trigger and observe it. |

## 3.4 Red teaming and incident response

| ID | State | Evidence in this repository | What is missing to close it |
|---|---|---|---|
| `RED-I1` | Blocked | None. No red-teaming calendar, cadence or exercise record exists. | An agreed periodic schedule and a named owner responsible for running it. This is an organisational commitment. |
| `RED-I2` | Blocked | Closest artifact only: each batch records the classifier version (`backend/services/risk/internal/repository/store.go:47`, written at `:174`), but there is no model registry, no deployment of versioned model artifacts and no rollback mechanism. | A versioned model registry plus an implemented rollback procedure. No trained model is deployed, so there is nothing to roll back to. |
| `RED-I3` | Pending | None. No incident-response plan exists. | The incident-response plan document itself, including roles, escalation and the rollback trigger. The missing input at this point is the document. |
| `RED-V1` | Blocked | None. No rollback drill was run and no duration was measured, because `RED-I2` is not implemented. | An implemented rollback (`RED-I2`) and a recorded drill with its measured duration. |
| `RED-V2` | Blocked | None. Service availability during a rollback was never observed. | The drill from `RED-V1` plus availability measurement and a named owner to accept it. |

## Summary

- 41 IDs registered: **0 closed**, **0 checkboxes marked**.
- 14 IDs hold partial in-repository evidence: `ZT-I1`, `ZT-I2`, `ZT-I3`, `ZT-V1`, `PRV-I1`, `PRV-I2`,
  `PRV-I3`, `PRV-V1`, `PRV-V2`, `GRD-I1`, `GRD-I2`, `GRD-V1`, `SBX-I1`, `SBX-I2`.
- 4 IDs are pending, self-contained and not started: `GOB-I1`, `AME-I1`, `AME-I2`, `RED-I3`.
- 23 IDs are blocked on an input this repository cannot supply.

### Grouped missing inputs

1. **Policies, owners and signatures** — the internal AI risk-classification policy (`GOB-I1`), the
   system's category and its regulatory consistency (`GOB-I2`, `GOB-V1`), the AI Security Officer's name
   (`GOB-I3`), the pre-production approval procedure and signer (`GOB-I4`, `GOB-V2`), and a named owner
   to accept the non-applicable ATLAS vectors (`AME-V2`).
2. **A threat-model artifact** — the MITRE ATLAS exercise and its vector register (`AME-I1`, `AME-I2`)
   and the control mapping onto sections 2 and 3 (`AME-V1`).
3. **Real data, a real ingestion path and a trained model** — a real risk-input dataset and its
   lineage/integrity checks (`PRV-I1`, `PRV-I2`, `PRV-V1`), a dependency/model inventory and fresh
   authoritative scan evidence (`PRV-I3`, `PRV-V2`), the dataset-sensitivity determination and any
   anonymisation or reidentification analysis (`PRI-I1`–`PRI-I3`, `PRI-V1`), and a training pipeline with
   recorded perturbations (`ENT-I1`–`ENT-V1`).
4. **Effective access control** — per-service broker identities and a completed access review
   (`ZT-I3`, `ZT-V1`), Prometheus authentication and a Grafana service (`ZT-I2`), and a periodic
   permission-audit schedule with an owner (`ZT-V2`).
5. **Authorized-environment security verification** — narrowed egress and a CNI-enforced isolation test
   (`SBX-I2`, `SBX-V1`), an output-sensitivity inspection (`GRD-V2`), and the deployment-control
   validation of BH-11 (`SBX-I1`).
6. **Agreed thresholds for monitoring** — input-distribution/model-confidence metrics and the thresholds
   that make an alert meaningful (`OBS-I1`, `OBS-V1`).
7. **Ongoing security operations** — the red-teaming calendar (`RED-I1`), the incident-response plan
   (`RED-I3`), and a versioned model with a rollback mechanism and a measured drill (`RED-I2`, `RED-V1`,
   `RED-V2`).
