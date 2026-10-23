# Spec for the dash clock and last-response cache

| Item | Value |
| --- | --- |
| Wave | W1 spec |
| Tier | small |
| Depends on | none. Feature `qmeter-dash-clock-cache`. Base: `dev` at `67ff84a` |
| Grant | goal `g7-dash-clock-cache`, user chat c131-c141 2026-10-22: land on qmeter `dev` |

## Overview

Write the feature spec of the new goal "dash clock to the second and
last-response cache". The spec is one file, `.hive/spec/qmeter-dash-clock-cache/README.md`,
and nothing else. The approved source is
`/home/penguin/source/hive/plans/qmeter-dash-clock-cache.md` in the home. Copy
its 6 decisions and its assumptions into the spec, keep the manager decisions
below, and add the user-input rule. No Go file changes in this task.

## Content

The file must hold these sections, in this order, with this text.

1. Title `# qmeter dash clock and last-response cache`, and one line naming
   the source: `plans/qmeter-dash-clock-cache.md` in the hive home, confirmed
   by the user on 2026-10-22 (chat c131-c141).
2. `## Decisions`, numbered 1 to 6, taken from the plan source and not
   paraphrased into a different meaning:
   1. The header shows `fetched HH:MM:SS · now HH:MM:SS` at its right edge,
      both local time to the second. The dash redraws once each second for the
      clock.
   2. The dash keeps the last good response of each provider in memory. It is
      lost on restart. The one-shot commands (`usage`, `resets`, `spend`,
      piped `qmeter`, `--json`) do not change.
   3. When a provider fails, its card keeps its bars, dimmed, with the line
      `◌ cached · fetched HH:MM:SS · <error reason>`. The `◌` and the word
      `cached` carry the state. Colour can add to it.
   4. An expired token (`provider.ErrTokenExpired`) does not use the cache.
      That provider shows only its error line. This needs a typed error kind
      on `usage.ProviderError`, which today holds only a message.
   5. The header `fetched` time is the time of the last refresh, success or
      failure.
   6. A narrow header drops ` · now HH:MM:SS` first, then the whole right
      side.
3. `## Assumptions`, the 4 accepted assumptions from the plan: the cache
   covers the windows and the balances of a provider; when the first fetch of
   a session fails no cache exists, so the error line shows as today; a success
   replaces the cached data at once; no canvas boards, the tests check the
   header and row text.
4. `## Manager decisions`, labelled M1 to M7:
   - M1. The cache lives in `internal/dash`. `usage.Result` keeps its meaning:
     one run's fresh output. Only the dashboard uses the cache.
   - M2. The provider cards and the compact window rows show cached data. The
     R1 resets list and the balances panel show fresh data only.
   - M3. The cached line is exactly `◌ cached · fetched HH:MM:SS · <reason>`,
     with no `error: ` prefix. The reason is the same text the error line shows
     today (`usage.ProviderError.Message`). The line keeps the error colour.
     The cached provider's bars, percentage and countdown render faint.
   - M4. `usage.ProviderError` gains a `Kind` field with a typed set.
     `KindTokenExpired` means the fetch failed on an expired token. The zero
     value means another error. One-shot text and JSON output do not change.
   - M5. A success replaces a provider's cached windows and balances at once,
     with that provider's fetch time. A failure other than a token expiry
     keeps the previous entry. A token expiry drops it.
   - M6. The clock is one `tea.Tick` per second. It starts in `Init`, re-arms
     on each tick, and only redraws. It never blocks, never starts a fetch, and
     never delays a key. `q` and `ctrl+c` quit while it runs.
   - M7. Before the first result the header right side shows `fetching…`.
     After it, the right side is `fetched HH:MM:SS · now HH:MM:SS` until the
     width forces a drop per decision 6.
5. One sentence: decision 15 of `.hive/spec/qmeter-simplify/README.md`
   (`fetched HH:MM`, no clock) is superseded by decision 1 here, and decisions
   1 to 18 of `qmeter-simplify` stay in force for everything else.
6. `## User input`: `q` and `ctrl+c` work in every state, a key press gets a
   visible response at once, the input loop never waits for I/O.
7. `## Task plan`: two implementation tasks follow this spec — the typed error
   kind with the last-response cache, then the header clock to the second.

Read `/home/penguin/source/hive/plans/qmeter-dash-clock-cache.md` and
`.hive/spec/qmeter-simplify/README.md` before you write. Keep the spec free of
a task history: the task records hold it.

## Design

No canvas board. The user chose no new board for this goal ("No canvas boards.
The tests check the header and row text"), and the older boards under
`/home/penguin/source/hive/.hive/tmp/coordinator/designs/qmeter/` belong to the
`qmeter-simplify` layout goal, not to this one. Do not compare against them and
do not capture a screen for this doc task. The exact text in `## Content`
above is the design of record for the two tasks that follow.

## Source files

| Path | Purpose |
| --- | --- |
| `.hive/spec/qmeter-dash-clock-cache/README.md` | the spec, create it |
| `.hive/spec/qmeter-dash-clock-cache/tasks/<id>/brief.md` | this brief, commit it |

No other file. No Go file.

## Verification

| ID | Check | Expected result |
| --- | --- | --- |
| V1 | `git ls-files .hive/spec/qmeter-dash-clock-cache` | Lists the README and the task brief. |
| V2 | The README sections | Decisions 1 to 6, the 4 assumptions, M1 to M7, the supersession sentence, and the user-input rule are present, each with its exact strings (`fetched HH:MM:SS · now HH:MM:SS`, `◌ cached · fetched HH:MM:SS · <error reason>`, `KindTokenExpired`, `tea.Tick`). |
| V3 | `git check-ignore -v .hive/spec/qmeter-dash-clock-cache/README.md` | Exits 1: the file is not ignored. `git status --porcelain` does not list it as untracked after the commit. |
| V4 | `git diff dev --stat` | Only the spec README, the task brief and the evidence files. No Go file. |
| V5 | The gate | `gofmt -l .` prints nothing; `go vet ./...`, `go test -race ./...`, `bash .github/scripts/next-version_test.sh` exit 0. |
| V6 | The hive worker gate on lint | `golangci-lint run --new-from-rev=dev ./...` reports 0 issues. `golangci-lint run ./...` has the 27 findings that already exist on `dev`; record them, do not fix them. |

The 27 baseline findings are known on `dev` (handoff of 2026-10-20) and are out
of scope for every task of this goal. V6 is the task gate for lint: no new
finding.

## Finish

Commit with a subject such as `docs(spec): add qmeter dash clock and cache
spec`. Push the task branch. Write `evidence/V1.md` to `evidence/V6.md`. Run
`hive task complete`. Send the manager the SHA and the V1 to V6 result.

## Common rules

- Read `/home/penguin/source/hive/plans/qmeter-dash-clock-cache.md` and this
  brief. The plan is final. When the code or the base disagrees with the plan,
  stop and report with `hive inbox send --to manager-qmeter TEXT --agent NAME`.
- No canvas board for this goal. The tests check the header and row text.
- The user is colourblind: each state needs a glyph or a word. Colour can add
  to a state. Colour must not be the only signal.
- Read `AGENTS.md` in the worktree. Work test-first where the task has a test.
  Never weaken or skip an existing test to get green.
- Gate, from the worktree root: `gofmt -l .` prints nothing; `go vet ./...`,
  `go test -race ./...` and `bash .github/scripts/next-version_test.sh` exit 0;
  `golangci-lint run --new-from-rev=dev ./...` reports 0 issues.
- Evidence goes to `.hive/spec/qmeter-dash-clock-cache/tasks/<id>/evidence/`
  in the worktree. Commit it on the task branch.
- Commit on the task branch only. Never touch `main`. No release, no tag.
- Verifier: commit and push `VERIFIED.md` in that evidence folder, then run
  `hive task verify`.
