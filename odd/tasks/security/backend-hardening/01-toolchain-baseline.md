# BH-01 — Establish a compatible toolchain and baseline

**Status:** baseline executed — workflow choice awaiting human confirmation | **Priority:** P1 | **Dependencies:** None.

## Evidence and scope

The original review observed local Go 1.26.1 while `go.work` required a newer toolchain, and its handler tests never executed. Re-verified on 2026-10-16: local `go` is 1.27.1 with `GOTOOLCHAIN=auto`, and `backend/go.work` (plus every module `go.mod`) declares `go 1.27.2`. Auto toolchain resolution closes that gap, so the original mismatch no longer reproduces: the local workflow now runs the same Go 1.27.2 the CI uses (`.github/workflows/_go-service.yml` pins `go-version: '1.27.2'`).

No tool was installed, upgraded, or removed to reach this state, and `go.work` was not changed.

## Acceptance checklist

- [x] Record the chosen workflow, Go version, workspace requirements, and exact approved commands.
  - **Toolchain:** local Go 1.27.1 + `GOTOOLCHAIN=auto` → effective `go1.27.2` (matches CI). Nothing installed.
  - **Workspace requirement:** `backend/go.work` and all module `go.mod` files declare `go 1.27.2`.
  - **Commands (mirror CI, run from each module dir with `GOWORK=off`):** `gofmt -l .`; `go mod verify`; `go vet ./...`; `go test -count=1 ./...`. CI additionally runs `go test -race` and `govulncheck`.
- [x] Execute relevant handler tests and agreed baseline checks; record passes, failures, and unexecuted checks separately.
  - **Passed (2026-10-16, `GOWORK=off`, `go1.27.2`):**
    - gateway: `go test ./...` → `ok firewatch.local/gateway/internal/handler`; gofmt clean; `go mod verify` ok; `go vet` clean.
    - jobs: `go test ./...` → all packages ok (`cmd/server`, `internal/handler`, `internal/messaging`, `internal/repository`, `migrations`); gofmt/mod/vet clean.
    - risk: `go test ./...` → 9 packages ok; gofmt/mod/vet clean.
    - contracts/gen: `go test ./...` → `events/v1`, `observations/v1` ok; `geo` has no test files; gofmt/mod/vet clean.
  - **Not yet executed:** `go test -race` and `govulncheck` (CI-only in this local pass).
  - **Failures:** none.
- [x] Separate environment blockers from behavior failures without claiming either category is green.
  - **Environment blockers:** none remain for the local baseline — the toolchain mismatch does not reproduce.
  - **Behavior failures:** none observed in the packages executed above.

**Note:** "passed" covers only the commands and packages listed. The CI-only `-race` and `govulncheck` steps were not run locally, and no check beyond this document was executed.

## Verification — pending human confirmation of the chosen workflow

**Checks status:** partly run. The local workflow satisfies `go.work` and the listed baseline commands executed. `go test -race` and `govulncheck` still need to be run locally or deliberately deferred to CI. No new installation or container operation was performed or authorized.

## Execution route and next action

**Route:** inline evidence capture (read-only commands; no source, workspace, or toolchain mutation).

**Next action:** confirm the local `GOTOOLCHAIN=auto` workflow (mirroring CI with `GOWORK=off`) as the approved BH-01 baseline, or select the container workflow instead.

## Log

- **2026-10-16** — Re-verified the toolchain blocker: local `go` 1.27.1 + `GOTOOLCHAIN=auto` resolves to `go1.27.2`, matching CI, so the original 1.26.1 / "handler tests never ran" finding is stale. Ran the baseline commands for all four modules with `GOWORK=off`: every executed package passed, gofmt clean, modules verified. `-race` and `govulncheck` not run locally. No files or workspace requirements changed.
