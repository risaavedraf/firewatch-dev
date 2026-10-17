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
- T2 — S1, S4; route: inline; status: completed; commits: `cb7e9e6`, `7dd9685`, `c8b1fd4`. Merge `fix/session-copy-and-correlation-id`; record the demo-selector and frontend-review histories after verifying their patches are already represented in `main`.
  - Risk: item 3 (adds the `X-Request-ID` response header and matching error-body correlation ID consumed by API clients).
- T3 — S2; route: inline; status: completed; commits: `548a4dd`, `340d5a1`. Record the React and page-redesign histories as represented in the existing `main` tree; preserve the current page-redesign outcome without replacing newer source files with stale branch snapshots.
  - Risk: none (tree-preserving history merges; no source paths changed).
- T4 — S3, S4; route: inline; status: completed; commit: none (local metadata cleanup). Removed the clean merged `tmp/ci-gates` worktree and all safe merged local branch refs; preserved dirty worktrees and scratch files.

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
- L12 — `git cherry main` showed all 2 commits from `feat/demo-local-stack-scenario-selector` and all 2 from `fix/frontend-review-findings` were patch-equivalent in `main`. Tree-preserving merges `7dd9685` and `c8b1fd4` make both branch tips reachable without duplicating content.
- L13 — RDD assessment for T2 (`base-ref c8830b9`) returned `risk: medium`, `review_due: false`, `review_due_reason: under_budget`; retain the slice for the later feature assessment.
- L14 — T3: `git cherry` showed all 13 `frontend/rediseno-pagina` commits patch-equivalent in `main`; 13 of 18 React commits were patch-equivalent, and the five remaining outcomes were verified in the current tree: seven-view routing (`frontend/src/app/routes.ts`), session/readiness/filter-reset behavior (`frontend/src/components/Shell.tsx` and `frontend/src/app/store.ts`), legacy harness removal (`frontend/legacy` absent; `frontend/scripts/check.mjs` no longer ignores it), 114-ID inventory (`odd/urgent/rediseno-pagina/app-react-vite.md`), two-sided freshness and error-header policy (`frontend/src/domain/readiness.ts`, tests, and `frontend/scripts/serve.mjs`). Tree-preserving merges `548a4dd` and `340d5a1` make both requested branch tips reachable without replacing the newer current tree.
- L15 — T3 ancestry-only merges had empty tree diffs (`git diff-tree --stat <parent> <merge>` produced no file changes); existing T2 frontend tests, lint, typecheck, build, and smoke passed on the unchanged candidate tree.
- L16 — T4 cleanup: removed the clean `tmp/ci-gates` worktree through Orca; its local `frontend/rediseno-09-final` branch was removed with it. Deleted 12 additional local branches only after confirming each tip was reachable from `main`; the three refs Git initially refused were force-deleted only after that ancestry proof. Remote refs were not changed.
- L17 — Preserved `tmp/backend-urgent` and `tmp/rediseno`: both have the same uncommitted additions to `backend/go.work.sum` (`golang.org/x/mod v0.38.0` and `golang.org/x/tools v0.48.0`). Preserved `tmp/rediseno/.probe` with 12 untracked files. No files were stashed or discarded. Orca now lists six FireWatch worktrees: main, two Gentle AI candidate views, these two dirty worktrees, and `.probe`.
- L18 — The local main tree is clean on `main`, 78 commits ahead of `origin/main`; no push or GitHub operation was performed. Parent Go is 1.27.1 with CGO disabled and no `gcc`, below the modules' Go 1.27.2 requirement; T1 remains pending independent proof.
