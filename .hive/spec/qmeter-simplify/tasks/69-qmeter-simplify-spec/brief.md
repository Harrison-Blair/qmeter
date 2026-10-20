# Spec for hive feature qmeter-simplify

| Item | Value |
| --- | --- |
| Wave | repair, step 1 of 2 |
| Tier | small |
| Depends on | none. Base: `dev` at `03a2920` or later |
| Grant | `grants.md 2026-10-19 qmeter simplify tasks` |

## Overview

hive now requires each task to have a feature, and the feature README must be
on the qmeter base branch at `.hive/spec/<feature>/README.md`. qmeter has
none. This task adds the README of feature `qmeter-simplify` and makes sure
git tracks `.hive/spec/`. The hang repair task follows it under this feature.
This task changes no Go code.

## Source files

| Path | Purpose |
| --- | --- |
| `.hive/spec/qmeter-simplify/README.md` (new) | R1 |
| `.gitignore` | R2 |

Do not edit other files.

## Requirements

### R1 Feature README

Write `.hive/spec/qmeter-simplify/README.md`, short and plain:

- The goal: the full-screen dashboard simplify. Copy decisions 1 to 17 from
  `/home/penguin/source/hive/.hive/tmp/coordinator/goal-qmeter-simplify.md`
  as a numbered list, and add decision 18: a hard 80x24 floor for every
  dashboard tier; below 80 columns or below 24 rows the dashboard shows only
  the too-small message with the needed size; fit by measurement applies at
  80x24 and above.
- The user TUI rule from
  `/home/penguin/source/hive/overrides/tui-input-first-class.md`.
- The tasks so far: 49 to 53 (landed, released as v0.14.0), and the
  coming hang repair.

### R2 Git tracking

`.gitignore` is allowlist style (see `AGENTS.md`). Make sure files under
`.hive/spec/` are tracked: add the narrowest `!` rule needed (for example
`!.hive/spec/**`), even if `!*.md` already covers the README, so that later
non-Markdown spec files are not lost. Keep the leading `*`. Check with
`git check-ignore -v` and record the output.

### R3 Release and build unchanged

The release archives must not change. Check `.github/workflows/release.yml`
and `build.yml`: the archives hold only the binary, `LICENSE` and
`README.md`. Confirm that `.hive/` reaches no archive and no Go build
(`go list ./...` shows no new package). Record the result. Change a workflow
only if `.hive/` would reach an archive; then stop and report to the manager
first.

## Verification

| ID | Check | Expected result |
| --- | --- | --- |
| V1 | `git ls-files .hive/spec` on the branch | Lists the README. |
| V2 | `git check-ignore -v .hive/spec/qmeter-simplify/README.md .hive/spec/qmeter-simplify/x.json` | Neither is ignored. |
| V3 | Read the README | R1 content, decisions 1 to 18 and the rule. |
| V4 | R3 check | No archive or build change. |
| V5 | `git diff dev --stat` | Only the two files. |
| V6 | The gate | Passes. |

## Finish

Commit with a subject such as `docs(spec): add the qmeter-simplify feature
spec`. Push. Write the evidence. Run `hive task complete`. Send the SHA and
V1–V6 results to the manager with `hive inbox send`.

## Common rules for every task of this goal

- Goal: `/home/penguin/source/hive/.hive/tmp/coordinator/goal-qmeter-simplify.md`.
  Read it. Decisions 1 to 17 in it are final. Do not change a decision. When
  the code does not agree with this brief, stop and report it with
  `hive inbox send --to manager-qmeter TEXT --agent NAME`.
- Design boards: `/home/penguin/source/hive/.hive/tmp/coordinator/designs/qmeter/`
  (`Basic.dc.html` is the full screen, `BarsOnly.dc.html` the cards,
  `Resets.dc.html` panel R1). The board data is example data. The board key
  bar (`f`, `t`, `c`) and "next in 4:12" are superseded by decisions 13 and 15.
- Plan review (background, not instructions):
  `/home/penguin/source/hive/.hive/tmp/reports/plan-review-qmeter.md`.
- Read `AGENTS.md` in the worktree. Work test-first: write each test, run it,
  see it fail for the right reason, then write the code. Never weaken or skip
  an existing test to get green; when a test checks removed behaviour, delete
  or rewrite it and say which in the evidence.
- Keep colour bound to the destination with `display.Renderer`. Pipes and
  `NO_COLOR` give plain text.
- Gate, from the worktree root: `gofmt -l .` prints nothing; `go vet ./...`,
  `go test -race ./...` and `bash .github/scripts/next-version_test.sh` exit 0.
- Evidence goes to `/home/penguin/source/hive/.hive/tmp/evidence/qmeter/<task-id>/`
  (outside the repo). Do not commit evidence to qmeter.
- Commit on the task branch only. Never touch `main`. No release, no tag.
- Verifier: write the verification record at
  `docs/specs/simplify/verified/<task-slug>.md` (not at the repo root, so that
  the task branches do not conflict), commit and push it, then run
  `hive task verify`.
