# Fix the dashboard hang: q and r stop working

| Item | Value |
| --- | --- |
| Wave | repair after the release |
| Tier | strong |
| Depends on | `69-qmeter-simplify-spec`. Feature `qmeter-simplify`. Code base: the same as `main` `eb9553b`, release v0.14.0 |
| Grant | `grants.md 2026-10-19 qmeter simplify tasks` |

## Overview

User bug report on v0.14.0 (`~/go/bin/qmeter`, built after the PR 18 merge):
in the full-screen dashboard, pressing `q` or `r` often does nothing. The
screen then hangs and accepts no input. Find the root cause, reproduce it with
a test that fails, fix it, and make the user's TUI input rule the acceptance
bar.

## User rule (acceptance bar)

From `/home/penguin/source/hive/overrides/tui-input-first-class.md`:

- The user can quit at any time. `q` and `ctrl+c` work in every state,
  including during a fetch, a refresh, or a slow operation.
- A key press gets a visible response at once.
- The input loop never waits for I/O. Do slow work in a background command.
- Each TUI has a test that sends a quit key while a slow operation runs and
  checks that the program exits.

## Known facts (not yet the cause)

- `internal/dash/model.go` `Update` handles `q`/`esc`/`ctrl+c` with
  `tea.Quit` and `r` with a new fetch; `fetch` runs `usage.Run` in a
  `tea.Cmd`. `View` calls `layout.Screen` on every frame, and the spinner
  re-renders every 80 ms while loading.
- `layout.Screen` measures several layout tiers on each call
  (`internal/dash/layout/screen.go`). A slow `View` would starve the input
  loop; this is one hypothesis, not a finding.
- Other places to check: `internal/dash/run.go` (program options, context,
  input), the refresh timer (`scheduleTick`), `cmd/dash/dash.go` (the update
  hint after `run` returns), and anything a `Cmd` can block on.

## Source files

| Path | Purpose |
| --- | --- |
| `internal/dash/**` + tests | R1–R3 |
| `internal/dash/layout/**` | only if the cause is there |
| `cmd/dash/**` + tests | only if the cause is there |
| `AGENTS.md` | R4: the rule, in the `## Instructions` section |

Do not edit other files.

## Requirements

### R1 Reproduce

Reproduce the hang first, in a real terminal (tmux or herdr) with the built
binary and real or fake providers, and record what you saw. Then write a test
that fails for the same reason on the base. Name the root cause in one or two
sentences, with file and line.

### R2 Fix

Fix the root cause. Do not hide it with a timeout only. After the fix, `q`,
`esc` and `ctrl+c` quit within 200 ms in every state: before the first
result, during a fetch, during a manual refresh, during an automatic refresh,
with a fetch that never returns, and while resizing. `r` shows the refresh
state at once.

### R3 Tests (rule)

Add a program-level test that runs the real Bubble Tea program (`Run` with
test `Input`/`Output`) with a provider whose fetch blocks until its context
ends. Send `q` (and in another case `ctrl+c`) while the fetch runs; the
program must exit within 1 s. Add a test that a frame is drawn within a set
bound for the board fixture at 140x45 and 200x60, if `View` cost is part of
the cause. Show each new test fails on the base.

### R4 Project instruction

Add the user rule to `AGENTS.md` under `## Instructions`, in short plain
sentences: q and ctrl+c quit in every state; a key press gets a visible
response at once; the input loop never waits for I/O; each TUI has a test
that sends quit while slow work runs and checks that the program exits.

## Verification

| ID | Check | Expected result |
| --- | --- | --- |
| V1 | New tests on base vs branch | Fail on base (or hang and time out), pass on branch. |
| V2 | Built binary in tmux at 140x45 with real providers: press `r` then `q` at once; repeat 10 times; also `q` before the first result | Each run exits within 1 s; the terminal is restored. |
| V3 | Same with a fake slow provider if one is available, or the test of R3 | Quit works during the fetch. |
| V4 | Read the root-cause statement | It names the cause with file and line, and V1 shows it. |
| V5 | `AGENTS.md` | The rule is there. |
| V6 | `git diff dev --stat` | Only the files of the source table. |
| V7 | The gate | Passes. |

## Finish

Commit with a subject such as `fix(dash): keep quit and refresh responsive`.
Push. Write the evidence. Run `hive task complete`. Send the SHA, the root
cause and the V1–V7 results to the manager with `hive inbox send`.

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
- Evidence and `VERIFIED.md` go to
  `.hive/spec/qmeter-simplify/tasks/<task-id>/evidence/` in the worktree, as
  the hive worker and verifier skills say. Commit them on the task branch.
- Commit on the task branch only. Never touch `main`. No release, no tag.
- Verifier: commit and push `VERIFIED.md` in that evidence folder, then run
  `hive task verify`.
