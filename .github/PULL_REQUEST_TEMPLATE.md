<!--
FireWatch pull request.

Authoring follows the skill flow this repository adopted (skills: branch-pr /
gentle-ai-branch-pr). Read `odd/PR-CONVENTIONS.md` before opening a PR: it records
the issue-first policy this template enforces, and why it was adopted.

The body requires six sections: Linked Issue, PR Type, Summary, Changes, Test Plan,
Contributor Checklist. Every PR links an issue that already carries
`status:approved`. Check exactly ONE box under "PR Type" and apply the matching
`type:*` label. Never mark a checklist item that no evidence supports: leave it
unchecked and say so in "Test Plan".
-->

## Linked Issue

<!--
`Closes #N` / `Fixes #N` / `Resolves #N` when this PR completes the issue.
`Refs #N` when it does not: that does not close the issue.
The issue must carry `status:approved` before this PR is opened.
-->

## PR Type

<!-- Check exactly one. Apply the label that matches it. -->

- [ ] Bug fix — `type:bug`
- [ ] New feature — `type:feature`
- [ ] Documentation only — `type:docs`
- [ ] Code refactoring — `type:refactor`
- [ ] Maintenance/tooling — `type:chore`
- [ ] Breaking change — `type:breaking-change`

## Summary

<!--
One to three bullet points: what changes and why it had to. Name the behaviour that
differs after this PR, not the files you touched; the diff already shows the files.
-->

## Changes

<!-- One row per changed file or logical group of files. State the change, not the diff. -->

| File | Change |
| --- | --- |
|  |  |

## Test Plan

<!--
The exact commands you ran and their observed outcomes, including the exit code
where one exists. The authoritative commands are the ones in the affected service's
workflow under `.github/workflows/`; quote what you actually ran, not what the
workflow would have run. Mark an inapplicable check as inapplicable instead of
leaving it blank or inventing a run.
-->

## Contributor Checklist

- [ ] The linked issue carries `status:approved`
- [ ] Exactly one `type:*` label is applied
- [ ] Applicable tests and checks were run, and their commands and outcomes are recorded above
- [ ] Documentation that describes the changed behaviour is updated
- [ ] Commits follow Conventional Commits and carry no `Co-Authored-By` trailer
- [ ] Any check that could not be run is named above as not run, not implied to have passed
