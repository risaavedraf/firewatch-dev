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

- T1 — S1, S4; route: inline; status: pending; commit: none. Integrate the 24 unique `fix/backend-hardening-bh` commits into `main` and verify backend/contract changes.
- T2 — S1, S4; route: inline; status: pending; commit: none. Integrate the remaining unique backend/frontend fixes and demo-selector commits in dependency order.
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
