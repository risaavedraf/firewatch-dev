# BH-01 — Establish a compatible toolchain and baseline

**Status:** pending | **Priority:** P1 | **Dependencies:** None.

## Evidence and scope

The review observed local Go 1.26.1 while `go.work` requires Go >=1.26.6. Its handler tests never executed. Establish a reproducible baseline, not an inferred pass.

Choose a compatible local toolchain or an existing container workflow after authorization. Do not install tools now or change the workspace requirement merely to bypass the mismatch.

## Acceptance checklist

- [ ] Record the chosen workflow, Go version, workspace requirements, and exact approved commands.
- [ ] Execute relevant handler tests and agreed baseline checks; record passes, failures, and unexecuted checks separately.
- [ ] Separate environment blockers from behavior failures without claiming either category is green.

## Verification — future, not run

**Checks status:** pending. Verify the selected toolchain satisfies `go.work`, then capture reproducible baseline output including handler tests. Any installation or new container operation needs separate authorization.

## Execution route and next action

**Proposed route:** delegated for bounded workflow reading, preparation, and evidence capture; no execution authorized here.

**Next action:** approve either a compatible local workflow or the existing container workflow and its exact baseline commands.
