Objective: Integrate the user's selected local FireWatch branch histories into `main` and clean only safe local worktrees/branches.
Branch: `fix/backend-hardening-bh` → `main`; delivery strategy: `stacked-to-main` (user-selected); verification runner: repository Go and frontend checks, confirmed from CI before execution.

## Specs

### S1 — Integrate local work into main
- User request: “hacer que todo vaya al main? hay un montón de worktrees abiertos en Orca y el worktree principal está en la rama fix/backend-hardening y hay como mil ramas abiertas, 1 issue y 2 pr, podemos dejar limpio el repo y que todo esté en la rama main porfa”
- User authorization: “Integrar todo localmente”
- Acceptance: preserve every authorized, unintegrated local branch change on `main`; do not perform remote/GitHub operations or push.
- Check: compare the final `main` history and tree against every in-scope local branch before cleanup.

### S2 — Resolve overlapping frontend histories
- User decision: “Integrar ambas (Recomendado)”
- Accepted resolution: include unique commits from `frontend/react-vite` and `frontend/rediseno-pagina`; where implementations conflict, keep `frontend/rediseno-pagina` as the final tree state. Do not change GitHub or PR #3.
- Check: confirm each branch's unique commits are reachable from `main` and run applicable frontend checks.

### S3 — Preserve dirty or untracked work
- Acceptance: do not remove or overwrite worktrees containing modifications or untracked files; do not discard their contents. An attempted removal of `tmp/backend-urgent` was refused because `backend/go.work.sum` is modified.
- Check: inspect Git and Orca worktree status before every removal; record retained paths and reasons.

### S4 — Local-only scope
- Acceptance: no fetch, push, PR/issue edits, or other GitHub operations. The primary checkout may be switched to `main` after a fresh terminal/worktree check.
- Check: final `git status`, `git worktree list`, branch ancestry, and Orca inventory.

## Tasks

- T1 — S1, S4; route: inline; status: pending independent verification; commit: `bd9aa16`. Integrate the 24 unique `fix/backend-hardening-bh` commits into `main` and verify backend/contract changes. Parent non-race suites passed, but independent verification was partial because its scratch environment has Go 1.27.1 while modules require 1.27.2; race proof is also unavailable without `gcc`.
  - Risk: item 1 (job failure migration), item 2 (authentication and authorization), item 3 (OpenAPI/event contracts), item 4 (broker retry/replay concurrency).
- T2 — S1, S4; route: inline; status: completed; commit: `cb7e9e6`. Merge `fix/session-copy-and-correlation-id`; verify the demo-selector and frontend-review branches are already represented by patch-equivalent changes in `main` instead of duplicating them.
  - Risk: item 3 (adds the `X-Request-ID` response header and matching error-body correlation ID consumed by API clients).
- T3 — S2; route: inline; status: pending; commit: none. Integrate the React and page-redesign histories using the approved page-redesign conflict outcome.
- T4 — S3, S4; route: inline; status: pending; commit: none. Remove only clean, integrated local worktrees and branches; preserve modified/untracked work.

## Log

- L1 — Original request: “hola bro puedes porfavor hacer que todo vaya al main? hay un montón de worktrees abiertos en Orca y el worktree principal está en la rama fix/backend-hardening y hay como mil ramas abiertas, 1 issue y 2 pr, podemos dejar limpio el repo y que todo esté en la rama main porfa”
- L2 — Authorization: “Integrar todo localmente”. This limits the work to local Git/Orca; GitHub and remote operations are not authorized.
- L3 — User confirmed the separate Pi terminal had closed and that the remaining OpenCode terminal is this session.
- L4 — User chose “Integrar ambas (Recomendado)”: integrate both frontend histories and use `frontend/rediseno-pagina` as the result for conflicts, without changing the closed PR.
- L5 — Initial inspection: local `main` is at `b0f77f9`; `fix/backend-hardening-bh` has 24 unique commits, `fix/session-copy-and-correlation-id` 9, `feat/demo-local-stack-scenario-selector` 2, `fix/frontend-review-findings` 2, `frontend/react-vite` 18, and `frontend/rediseno-pagina` 13. Other local branch tips are already ancestors of `main`.
- L6 — Orca reported `frontend/react-vite` linked to closed PR #3 and the primary checkout linked to open PR #29. No GitHub lookup was made.
- L7 — Dirty state: `tmp/backend-urgent` and `tmp/rediseno` have modified `backend/go.work.sum`; `tmp/rediseno/.probe` contains untracked scratch files. Orca refused removal of `tmp/backend-urgent` because it is modified; no worktree was removed.
- L8 — Delivery strategy: user selected “stacked-to-main (Recomendado)”, so integrate in reviewable local slices on `main`; no push or PR creation.
- L9 — T1 verification on the fast-forwarded `main`: `go test -race -count=1 ./...` failed because CGO is disabled; with CGO enabled it failed because `gcc` is absent. For gateway, jobs, and risk, `GOWORK=off go test -count=1 ./...`, `gofmt -l .`, `go mod verify`, and `go vet ./...` passed.
- L10 — Independent verifier returned partial with no candidate-caused backend defect reproduced. Its isolated scratch Go toolchain was 1.27.1, below the modules' required 1.27.2; it could not execute the behavioral probes, migration application, or race tests. The scratch directory was removed by the parent. T1 remains pending independent proof.
- L11 — T2: merged `fix/session-copy-and-correlation-id` as `cb7e9e6`; resolved shared guard citations against the merged gateway's actual lines (69, 125, 139, 145), retaining both rate-limit and request-correlation tests. The gateway Go suite passed. Frontend CI commands `npm test` (24 files/246 tests), `npm run lint`, `npm run typecheck`, `npm run build`, and `npm run smoke` all passed. `gofmt -l .`, `go mod verify`, and `go vet ./...` passed for the gateway module.
- L12 — `git cherry main` shows all 2 commits from `feat/demo-local-stack-scenario-selector` and all 2 from `fix/frontend-review-findings` are patch-equivalent in `main`; neither branch adds new content requiring a duplicate merge. The 5 unique patches remaining in `frontend/react-vite` are assigned to T3.
- L13 — RDD assessment for T2 (`base-ref c8830b9`) returned `risk: medium`, `review_due: false`, `review_due_reason: under_budget`; retain the slice for the later feature assessment.
