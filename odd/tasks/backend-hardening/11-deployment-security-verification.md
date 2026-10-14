# BH-11 — Validate deployed controls with repeatable evidence

**Status:** pending | **Priority:** P1 | **Dependencies:** BH-02, BH-04, BH-05, BH-06; explicit target-access authorization.

## Evidence and scope

Static configuration alone does not prove effective broker ACLs, verified TLS, or egress enforcement. Validate the selected deployment and record bounded evidence; this is not certification.

Include CI evidence and image/runtime controls: non-root execution, read-only filesystem, dropped capabilities, and no service-account token mounting unless explicitly justified. Prepare sanitized secret-exposure scanning of approved current files and history only if available; the workspace root is not Git. Do not read `.env` or expose secret values.

## Acceptance checklist

- [ ] Verify effective BH-04–BH-06 controls and retain sanitized results tied to deployment/image versions.
- [ ] Check runtime hardening and record justified exceptions and unavailable checks explicitly.
- [ ] Provide repeatable safe CI checks and a runbook, including a sanitized secret-scan report without reading excluded sensitive locations.
- [ ] Require explicit destination, operation, and credential-use authorization for staging access; no stress tests or attacks without approval.

## Verification — future, not run

**Checks status:** pending. Run approved bounded allow/deny, TLS identity, runtime-policy, and secret-exposure checks; report skipped history scanning if no authorized Git history exists. Redact findings without copying credential material into logs or reports.

## Execution route and next action

**Proposed route:** delegated for validation-plan preparation and coordinated CI/runbook work; target access remains separately gated.

**Next action:** approve the validation matrix, non-sensitive scan scope, and exact environment operations before execution.
