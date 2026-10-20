# Find and fix the remaining dashboard hang

| Item | Value |
| --- | --- |
| Wave | repair 2 |
| Tier | strong |
| Depends on | none. Feature `qmeter-simplify`. Base: `dev` at `978f2ba` or later (released as v0.15.0) |
| Grant | `grants.md 2026-10-19 qmeter simplify tasks` |

## Overview

Task 71 fixed one cause (Bubble Tea groups adjacent runes, so `rq` was
ignored). The user still sees the hang with a build of `978f2ba`
(`~/go/bin/qmeter`, vcs.revision 978f2ba48f14): in the full-screen
dashboard, `q` or `r` often does nothing, and then the screen accepts no
input. So the user's cause is a different one. Find it in a real terminal,
report it to the manager, wait for the manager's go, then fix it.

## Phase 1: investigate (no fix yet)

Run the built binary of `978f2ba` with real providers, in a herdr pane or a
pty (tmux is fine), WITHOUT `QMETER_NO_UPDATE_CHECK`. Also run the user's own
`~/go/bin/qmeter` read-only once to compare. Send `q`, `r`, `ctrl+c` at
different times, one key per input:

- during the first fetch;
- during a manual refresh and an automatic refresh (use a short
  `refresh_interval` in a scratch config under a scratch `XDG_CONFIG_HOME`);
- during and right after the update check;
- after a resize, and during a resize;
- after the dashboard has been open for some minutes;
- with a slow or failing provider (for example no network for one).

Record exactly when keys stop working, what the screen shows, and whether the
process is still alive (CPU, goroutine dump with `kill -QUIT` to a copy built
with `GOTRACEBACK=all`).

Check the code for: a blocking call in `Update`, `Init` or `View`; a channel
or `tea.Cmd` that never returns; `tea.Exec`; a stdin reader that competes
with Bubble Tea (`internal/update` hint or prompt, anything that reads
`os.Stdin`); a second `tea.Program`; terminal mode left wrong (raw mode,
alt screen); signal handling; the refresh timer (`scheduleTick`) and the
spinner tick; context cancellation in `Run`.

Then send the manager one message (`hive inbox send --to manager-qmeter`)
with: how to reproduce it, the root cause with file and line, the goroutine
dump excerpt or trace that proves it, and the fix you propose. Write the
same as `evidence/V1.md` and commit it. Then STOP and wait for the manager's
reply. Do not change Go code before the reply.

If you cannot reproduce it, say so in the message, list what you tried, and
wait.

## Phase 2: fix (after the manager's go)

Fix the root cause the manager approves. Add a test that fails on the base
for the same reason, preferably at program level (`Run` with test input and
output, or a pty test). Keep the TUI input rule in `AGENTS.md`.

## Source files

| Path | Purpose |
| --- | --- |
| `internal/dash/**`, `cmd/dash/**`, `internal/update/**` + tests | phase 2, only where the cause is |
| other Go files | only with the manager's go in phase 2 |

## Verification

| ID | Check | Expected result |
| --- | --- | --- |
| V1 | Phase 1 report | Reproduction steps, root cause with file and line, proof. |
| V2 | New test on base vs branch | Fails (or times out) on base, passes on branch. |
| V3 | The phase 1 reproduction, run again on the branch binary in a real terminal, 10 times | Keys work each time; `q` exits within 1 s; the terminal is restored. |
| V4 | `git diff dev --stat` | Only files the cause needs, plus the task records. |
| V5 | The gate | Passes. |

## Finish

Commit with a subject such as `fix(dash): ...`. Push. Write the evidence. Run
`hive task complete`. Send the SHA, the root cause and V1–V5 to the manager.

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
