# Feature: green CI on `main` — Go 1.27.2 toolchain and x/text bump

**Objective:** make the Jobs, Risk and Gateway workflows pass end to end on `main` by removing every govulncheck finding (9 Go stdlib advisories + GO-2026-6629) and by compiling the scanned runtime images from a fixed toolchain.
**Branch:** `main` (linked worktree `C:/Users/rikar/orca/workspaces/FireWatch/coney-main`), based on `0ed07e2`. **Delivery strategy:** `ask-on-risk` (forecast ~60 authored changed lines, far below the ~400 heuristic). **Test runner:** `go test -race -count=1 ./...`, `go vet ./...`, `govulncheck@v1.9.0` (S16 amended the handoff's `v1.1.4`), `docker build --sbom=true --provenance=true`.

## Specs

- **S1 — pin the workflow toolchain.** `.github/workflows/_go-service.yml:25`: `go-version: '1.26.6'` -> `go-version: '1.27.2'`. Quote: "Pin **Go 1.27.2**. Do NOT use 1.27.1 (it is inside the affected range)."
- **S2 — split the mega-step (optional, sanctioned).** Quote: "Optional in the same file: split the mega-step `Format, integrity, tests and static checks` into named steps (gofmt / go mod verify / go test / go vet / govulncheck) so failures are attributable; pass/fail semantics unchanged."
- **S3 — builder image.** Replace `FROM golang:1.26.6-bookworm@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36 AS build` (line 1) in `backend/services/jobs/Dockerfile`, `backend/services/risk/Dockerfile` and `backend/services/gateway/Dockerfile` with the verified digest: `golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61`. Quote the reason: "the binary scanned by trivy in `Scan runtime image` (`--scanners vuln --severity HIGH,CRITICAL --exit-code 1`) is compiled from this image; without the bump that step fails on the same stdlib vulns."
- **S4 — go directive.** Raise the `go` directive to `1.27.2` in `backend/go.work:1`, `backend/contracts/gen/go.mod:3`, `backend/services/gateway/go.mod:3`, `backend/services/jobs/go.mod:3`, `backend/services/risk/go.mod:3`; then `cd backend && go work sync`; commit `backend/go.work.sum` if it changes.
- **S5 — x/text.** In `backend/services/jobs` and `backend/services/risk`: `GOWORK=off go get golang.org/x/text@v0.41.0 && GOWORK=off go mod tidy` (updates go.mod + go.sum; MVS keeps 0.41.0 over what pgx/oauth2 request). Closes GO-2026-6629. Gateway: no change.
- **S6 — local verification (verbatim commands, Go must report 1.27.2):**
  ```
  cd backend
  for s in jobs risk gateway; do
    ( cd services/$s \
      && test -z "$(gofmt -l .)" \
      && GOWORK=off go mod verify \
      && GOWORK=off go test -race -count=1 ./... \
      && GOWORK=off go vet ./... \
      && GOWORK=off go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./... ) || exit 1
  done
  docker build --sbom=true --provenance=true -f services/<svc>/Dockerfile -t firewatch-<svc>:ci .   # from backend/, per service
  ```
- **S7 — docs/evidence.** `backend/docs/verification.md` currently claims "govulncheck: No vulnerabilities found" and "Trivy: No HIGH/CRITICAL"; refresh those rows with the new evidence or mark them historical. Update `1.26.6` -> `1.27.2` in `backend/README.md:7` and `backend/docs/code-explanation/07-deployment.md:46,47,49,56`.
- **S8 — trigger paths (recommended).** Add `'backend/go.work'` and `'backend/go.work.sum'` to `paths` in `.github/workflows/jobs.yml`, `risk.yml`, `gateway.yml`: "today a toolchain change in go.work does not re-trigger them."
- **S9 — acceptance.** Quote: "Acceptance = the three runs green end to end, including the two steps that are skipped today." Backend integration run is already green and stays green.
- **S10 — commit plan (3 work units, Conventional Commits):** `fix(ci): pin Go 1.27.2 in service workflow and builder images` (S1-S4); `fix(deps): bump golang.org/x/text to v0.41.0 in jobs and risk` (S5); `docs(backend): refresh toolchain version and vulnerability evidence` (S7, S8).
- **S11 — boundary: no unilateral push.** Quote: "Stop after local checks are green and the commits exist on `main`. Ask the human for push authorization (pushing triggers the CI that is the acceptance criterion). Do not push unilaterally."
- **S12 — boundary: scope.** Quote: "Do not touch any other branch, and do not touch the frontend/rediseño-pagina work. Scope = only the files listed below."
- **S13 — boundary: Contingency A is the human's decision.** If `Build container with provenance and SBOM` fails with `Attestation is not supported for the docker driver`, "This is the human's decision: report it, do not decide it."
- **S14 — boundary: Contingency B.** "if `Scan runtime image` fails, read what trivy reports. From the pinned `distroless/static-debian12` base -> bump its digest; from the app binary -> check the build used the T2 image."
- **S15 — risk tier.** Quote: "Risk tier of this change: Task Size item 5 (delivery/environment: toolchain, dependencies, CI), so it needs independent verification on top of its own checks."
- **S16 — bump the pinned scanner (human decision, L5).** Same workflow: `go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...` -> `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...`, because `v1.1.4` panics under Go 1.27.2 (L4), so S1 and S9 cannot both hold with the original pin. Selected verbatim by the human: "Go 1.27.2 + govulncheck v1.9.0 (última)" (`value: A27_v19`). Alternatives offered and declined: `govulncheck@v1.2.0` (minimal bump, also verified green) and Go 1.26.9 with `v1.1.4` unchanged.

## Tasks

| ID | Specs | Route | Status | Evidence |
|---|---|---|---|---|
| T1 pin workflow toolchain + split mega-step | S1, S2 | inline | done | `go-version: '1.27.2'`; mega-step split into 5 named steps |
| T2 builder digest in 3 Dockerfiles | S3 | inline | done | `golang:1.27.2-bookworm@sha256:5cf287a7…`; image inspected: `go1.27.2 linux/amd64` |
| T3 go directive 1.27.2 + `go work sync` | S4 | inline | done | sync clean; `go.work.sum` unchanged |
| T4 x/text v0.41.0 (jobs, risk) | S5 | inline | done | diff limited to `x/text v0.41.0` and `x/sync v0.22.0` (MVS) |
| T5 local verification block | S6 | inline | done | gofmt + `go mod verify` + `go test -race` + `go vet` PASSED ×3 on CI-faithful bytes (job-14); `govulncheck@v1.9.0` No vulnerabilities found ×3 (job-15, rc=0) |
| T6 docker builds with SBOM/provenance | S6 | inline | done | 3 builds rc=0; SBOM and provenance attestation manifests emitted |
| T7 trivy scan of the rebuilt images | S6 | inline | done | 0 HIGH/CRITICAL in the Debian layer and the `server` Go binary ×3 |
| T8 docs and workflow trigger paths | S7, S8 | inline | done | verification.md refreshed; README and 07-deployment at 1.27.2; `backend/go.work{,.sum}` added to the three `paths` |
| T9 independent verification (high risk) | S15 | delegated | pending | |
| T10 three work-unit commits on `main` | S10, S11 | inline | pending | |
| T11 native RDD assess/review | S15 | inline | pending | |
| T12 push + observe CI to green | S9 | blocked | pending | needs human push authorization (S11) |
| T13 scanner pin bump after the L4 finding | S16 | inline | done | `@v1.9.0` in the workflow; verified by T5 |
| T14 report to the requesting session | — | inline | pending | |

## Log

**L1 — original request (verbatim, delivered by intercom from session `01a13ac0-c9e0-75fa`, human-authorized handoff):**

```
HANDOFF — FireWatch: fix failing CI on `main` (Jobs, Risk, Gateway)

From session 01a13ac0-c9e0-75fa (read-only analysis, no writes). The human authorized this handoff and asked that the implementation land on branch `main`.

## Verified context (evidence, do not re-derive)
Repo: `risaavedraf/firewatch-dev` (private). `main` = `0ed07e2` = `origin/main`.
Failing runs (push on main@0ed07e2): Jobs 38573687332, Risk 38573687396, Gateway 38573687462. Backend integration 38573686661 is green.
All three fail in the SAME step of the shared reusable workflow `.github/workflows/_go-service.yml`: `go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...`. Earlier commands in that step pass (`gofmt`, `go mod verify`, `go test -race`, `go vet`); the step exits 1 with `##[error]Process completed with exit code 1`. Steps `Build container with provenance and SBOM` and `Scan runtime image` were SKIPPED, i.e. never validated.
Reachable vulnerabilities found: 9 Go stdlib advisories — GO-2026-6617, 6613, 6612, 6611, 6610, 6608, 6607, 6605, 6603 (OSV ranges: introduced `1.27.0-0`, fixed `1.26.9` and `1.27.2`) — plus GO-2026-6629 in `golang.org/x/text v0.39.0` (fixed `v0.41.0`), present only in jobs and risk. Gateway has no x/text.
Node.js 20 deprecation warnings on checkout v4 / setup-go v5 are warnings, NOT the cause.

## Toolchain decision (already made)
Pin **Go 1.27.2**. Do NOT use 1.27.1 (it is inside the affected range). Alternative, if the human prefers the minimal branch change: 1.26.9.
Verified digests (Docker Hub API `results[].digest`, manifest list):
- `golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61`
- `golang:1.26.9-bookworm@sha256:bcef992b77b1e2031aaaa51da75cebc32da8e9c182e12e633ea79023c79d9eff`

## Before writing anything — verify your own context
In your worktree (`C:\Users\rikar\orca\workspaces\FireWatch\coney`):
`git rev-parse --show-toplevel`, `git rev-parse --git-common-dir`, `git branch --show-current`, `git status --short`.
Work must land on branch `main`, based on `0ed07e2`. If `main` is already checked out in another worktree, create a linked worktree instead of forcing a checkout. Do not touch any other branch, and do not touch the frontend/rediseño-pagina work. Scope = only the files listed below.

## Tasks (exact)
T1 `.github/workflows/_go-service.yml:25`: `go-version: '1.26.6'` -> `go-version: '1.27.2'`. Optional in the same file: split the mega-step `Format, integrity, tests and static checks` into named steps (gofmt / go mod verify / go test / go vet / govulncheck) so failures are attributable; pass/fail semantics unchanged.
T2 (mandatory) Replace the builder image in all three Dockerfiles — `backend/services/jobs/Dockerfile:1`, `backend/services/risk/Dockerfile:1`, `backend/services/gateway/Dockerfile:1` — with the 1.27.2 digest above. Reason: the binary scanned by trivy in `Scan runtime image` (`--scanners vuln --severity HIGH,CRITICAL --exit-code 1`) is compiled from this image; without the bump that step fails on the same stdlib vulns.
T3 Raise the `go` directive to `1.27.2` in `backend/go.work:1`, `backend/contracts/gen/go.mod:3`, `backend/services/gateway/go.mod:3`, `backend/services/jobs/go.mod:3`, `backend/services/risk/go.mod:3`; then `cd backend && go work sync`; commit `backend/go.work.sum` if it changes.
T4 In `backend/services/jobs` and `backend/services/risk`:
`GOWORK=off go get golang.org/x/text@v0.41.0 && GOWORK=off go mod tidy` (updates go.mod + go.sum; MVS keeps 0.41.0 over what pgx/oauth2 request). Closes GO-2026-6629. Gateway: no change.
T5 Local verification with Go 1.27.2 installed (`go version` must report 1.27.2):
[command block omitted here — reproduced verbatim in S6]
T6 Docs/evidence: `backend/docs/verification.md` currently claims "govulncheck: No vulnerabilities found" and "Trivy: No HIGH/CRITICAL"; refresh those rows with the new evidence or mark them historical. Update `1.26.6` -> `1.27.2` in `backend/README.md:7` and `backend/docs/code-explanation/07-deployment.md:46,47,49,56`.
T7 (recommended) Add `'backend/go.work'` and `'backend/go.work.sum'` to `paths` in `.github/workflows/jobs.yml`, `risk.yml`, `gateway.yml` — today a toolchain change in go.work does not re-trigger them. Optional: bump checkout/setup-go to Node 24 majors with new SHAs.
T8 Push and observe. Acceptance = the three runs green end to end, including the two steps that are skipped today. `.github/workflows/**` is in the `paths` filters, so the fix push re-triggers all three workflows.

## Commit plan (work units, Conventional Commits)
1. `fix(ci): pin Go 1.27.2 in service workflow and builder images` (T1, T2, T3)
2. `fix(deps): bump golang.org/x/text to v0.41.0 in jobs and risk` (T4)
3. `docs(backend): refresh toolchain version and vulnerability evidence` (T6, T7)

## Boundaries and open decisions
- Stop after local checks are green and the commits exist on `main`. Ask the human for push authorization (pushing triggers the CI that is the acceptance criterion). Do not push unilaterally.
- Contingency A: `Build container with provenance and SBOM` may fail with `Attestation is not supported for the docker driver` — the ubuntu-24.04 runner (Docker 28.0.4) does not enable the containerd image store. Options: enable the snapshotter in the job, or drop `--sbom/--provenance` (degrades the supply-chain evidence the repo declares). This is the human's decision: report it, do not decide it.
- Contingency B: if `Scan runtime image` fails, read what trivy reports. From the pinned `distroless/static-debian12` base -> bump its digest; from the app binary -> check the build used the T2 image.
- Risk tier of this change: Task Size item 5 (delivery/environment: toolchain, dependencies, CI), so it needs independent verification on top of its own checks.
- Related pre-existing ODD notes: `odd/tasks/security/backend-hardening/01-toolchain-baseline.md` and `02-vulnerability-scans.md`.

## Report back
Reply to session `01a13ac0-c9e0-75fa` with: branch/commit SHAs, local check results, the CI run URLs and their conclusions, plus anything you could not verify.
```

**L2 — context verification (observed, 2026-10-14):** `git rev-parse --git-common-dir` = `E:/Duoc/Arquitectura Sistemas IA/FireWatch/.git`; `main` is not checked out in any worktree, so a linked worktree `coney-main` was created from `main` at `0ed07e2` (the sibling worktree `coney` carries the intercom-managed scratch branch `p4p1r1cky/intercom-message-above`, untouched). `git rev-parse main origin/main` both `0ed07e2`. Runs re-confirmed via `gh`: Jobs 38573687332, Risk 38573687396, Gateway 38573687462 all `completed/failure` on `main@0ed07e2`; step list shows step 4 `Format, integrity, tests and static checks` = `failure`, steps 5 `Build container with provenance and SBOM` and 6 `Scan runtime image` = `skipped`. Failing log confirms `Vulnerability #1: GO-2026-6629` (`golang.org/x/text`) and `Vulnerability #2: GO-2026-6617` (`net/http@go1.26.6`, `Fixed in: net/http@go1.26.9`). Read access required switching the active `gh` account to `risaavedraf`; the SSH remote needs no change.

**L3 — environment note:** local Go was 1.27.1 (inside the affected range); `GOTOOLCHAIN=go1.27.2 go version` downloads and reports `go version go1.27.2 windows/amd64`, so T5 runs with the exact pinned toolchain. Local Docker is 29.5.3 with the containerd snapshotter (`io.containerd.snapshotter.v1`), so local `--sbom/--provenance` builds are expected to work; the GitHub runner's capability is Contingency A (S13) and remains the human's call if the runner rejects the attestations.

**L4 — BLOCKING FINDING: `govulncheck@v1.1.4` cannot run under Go 1.27.2 (observed, 2026-10-14).** After the T1-T4 edits, the T5 container run passed gofmt, `go mod verify`, `go test -race -count=1 ./...` (every package `ok`) and `go vet` for jobs, but `go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...` first hit a transient checksum-DB error (`reading https://sum.golang.org/tile/8/1/x001/052.p/209: stream error: stream ID 9; INTERNAL_ERROR`) and then, on re-run (job-6, 3 bounded attempts per service), **panicked**: `panic: unexpected expr: *ast.KeyValueExpr` inside `golang.org/x/tools@v0.29.0/go/ssa/builder.go:958` (`compLit` -> `assign` -> `expr0`). Outcome: jobs 3/3 attempts panic, gateway 3/3 panic, risk 1/3 panic then 1 pass — i.e. flaky. Isolation probe (job-7): the same panic reproduces on a **trivial hello-world module** declaring `go 1.27.2` in the very same pinned image, so it is a scanner/toolchain incompatibility (x/tools v0.29.0 SSA vs the Go 1.27.2 standard library), not anything in FireWatch code. Consequence: **T1 as prescribed cannot make CI green**; with Go 1.27.2 the pinned scanner version must move, or the toolchain must be 1.26.9 (the handoff's own alternative) with v1.1.4 kept. Evidence for the decision: `golang.org/x/text v0.41.0` declares `go 1.25.0`, so the x/text fix is compatible with BOTH go 1.26.9 and 1.27.2 (job-8). Path A probe (job-9): `govulncheck@v1.9.0` under Go 1.27.2 reports `No vulnerabilities found.` for all three services, rc=0. Path B probe (job-10, pending): Go 1.26.9 + `govulncheck@v1.1.4` full block in a scratch copy (`.scratch-go1269`, deleted afterwards, never committed). Task T13 opened; the toolchain/scan-pin choice is a human decision (the handoff reserved the 1.27.2-vs-1.26.9 choice and never authorized changing the scanner pin).

**L5 — human decision on the L4 blocker (verbatim relay, 2026-10-14).** Question asked: "govulncheck@v1.1.4 no puede analizar con Go 1.27.2 (panic en x/tools v0.29.0, reproducido incluso en un módulo trivial), así que T1 tal como está prescrito no puede dejar la CI verde. ¿Qué camino tomo?" Options offered: "Go 1.27.2 + govulncheck v1.2.0 (bump mínimo)" (`A27_v12`), "Go 1.27.2 + govulncheck v1.9.0 (última)" (`A27_v19`), "Go 1.26.9 + govulncheck v1.1.4 (sin cambios)" (`B269`). **Human answer, verbatim: "Go 1.27.2 + govulncheck v1.9.0 (última)" (`value: A27_v19`).** Applied: `.github/workflows/_go-service.yml` now runs `govulncheck@v1.9.0`; recorded as S16 while S1 stays unchanged. The human also asked what is given up by not using v1.9.0; the relayed answer is that only scanner freshness is at stake (newer `x/tools` reachability analysis and seven minor releases of fixes), because `v1.2.0`, `v1.3.0` and `v1.9.0` all report `No vulnerabilities found.` under Go 1.27.2.

**L6 — CRLF false blocker cleared (2026-10-14).** The risk `internal/handler` failures seen in the Path B probe came from the Windows checkout, not from the code: `core.autocrlf=true` rewrites `backend/contracts/gen/geo/valparaiso-comunas.geojson` because `.gitattributes` covers `*.json` but not `*.geojson`, while the committed blob (LF) hashes to `02ef8e8ff49ab2ca…` — exactly the `derived_sha256` that `parseGeography` validates at runtime. Local verification therefore runs on a CI-faithful scratch copy (`.scratch-ci`, untracked, to be deleted before the commits) whose geojson bytes are the LF blob; the real worktree was restored so `git status` shows only the intended changes. Advisory, not implemented here because it is outside the authorized file scope: add `*.geojson text eol=lf` (or `-text`) to `.gitattributes`.

**L7 — local verification results (observed).** `job-14` and `job-15` on the CI-faithful copy, container `golang:1.27.2-bookworm@sha256:5cf287a7…`, `GOWORK=off`, geojson sha256 verified as `02ef8e8ff49ab2ca…` inside the container: gofmt clean, `go mod verify`, `go test -race -count=1 ./...` and `go vet ./...` passed for jobs, risk and gateway; `govulncheck@v1.9.0` reported `No vulnerabilities found.` for all three (rc=0). `job-4`: three `docker build --sbom=true --provenance=true` rc=0 with attestation manifests. `job-5`: trivy 0.69.3 `--scanners vuln --severity HIGH,CRITICAL --exit-code 1` → 0 in the Debian layer and 0 in the `server` Go binary for all three images (rc=0).

**L8 — work-unit commits (2026-10-14).** `74ca571` `fix(ci): pin Go 1.27.2 in service workflow and builder images` (S1-S4, S16); `1683a72` `fix(deps): bump golang.org/x/text to v0.41.0 in jobs and risk` (S5); `9dc3d1d` `docs(backend): refresh toolchain version and vulnerability evidence` (S7, S8). Each commit is a separate work unit and the go.mod hunks were staged per hunk so the toolchain pin and the dependency bump do not mix. Branch `main` sits on `9dc3d1d` over `0ed07e2`; **nothing has been pushed** (S11).
