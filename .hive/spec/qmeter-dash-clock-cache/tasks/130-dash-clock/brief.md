# Show the fetched and current time to the second

| Item | Value |
| --- | --- |
| Wave | W3 clock |
| Tier | decent |
| Depends on | the cache task of this feature (it lands first; both edit `internal/dash/model.go` and `internal/dash/layout/screen.go`). Feature `qmeter-dash-clock-cache`. Base: `dev` after that landing |
| Grant | goal `g7-dash-clock-cache`, user chat c131-c141 2026-10-22: land on qmeter `dev` |

## Overview

Decisions 1, 5 and 6 of the feature spec `.hive/spec/qmeter-dash-clock-cache/README.md`,
and manager decisions M6 and M7:

- The header's right side becomes `fetched HH:MM:SS · now HH:MM:SS`, both in
  local time to the second.
- The dashboard redraws once each second for the clock.
- The header `fetched` time stays the time of the last refresh, success or
  failure.
- A narrow header drops ` · now HH:MM:SS` first, then the whole right side.

Today the right side is `fetched HH:MM` (`internal/dash/layout/screen.go`,
`headerLine`), and the frame only redraws on a fetch result, a spinner tick, a
key or a resize.

## Behaviour

Read `.hive/spec/qmeter-dash-clock-cache/README.md` first. These points settle
the detail.

- **Header text.** When the right side fits, it is exactly
  `fetched HH:MM:SS · now HH:MM:SS` (local time, 24-hour clock). `fetched`
  comes from `ScreenOptions.FetchedAt`, `now` from `ScreenOptions.Now`; both are
  already passed by the model. The left side and its layout do not change.
- **Drop order.** When the left side and the full right side do not fit on one
  line, the right side becomes `fetched HH:MM:SS` alone. When that does not
  fit either, the line holds the left side only, as it does today. A right side
  that is dropped leaves no trailing spaces: the line still has exactly the
  terminal width in cells.
- **Before the first result.** With no `FetchedAt`, the right side stays
  `fetching…` (today's behaviour). Do not show a `now` part before the first
  result.
- **The clock tick.** One `tea.Tick` per second. It starts in `Init`, it
  re-arms on each tick, and `Update` of a tick only returns the next tick so the
  frame is drawn again. It never sets `loading`, never starts a fetch, never
  touches the refresh generation or the spinner, and never blocks. `q` and
  `ctrl+c` still quit while a tick is pending, so the user-input rule holds.
- **Test seam.** Use the seam style the model already uses for the refresh
  timer (`scheduleTick`): a package variable that returns a `tea.Cmd`, so a
  test can assert the one-second interval and drive a tick without waiting.
  Keep the real implementation `tea.Tick(time.Second, …)`.
- **Time zone.** The header formats local time. Make the layout test fixture
  build `Now` and `FetchedAt` with `time.Local` so the golden text is the same
  on every machine. Change `now` in
  `internal/dash/layout/screen_test_helpers_test.go` for that.
- **No other frame change.** The cached line, the cards, the panels and the key
  line are the cache task's work. Only the header and the redraw cadence change
  here.

## Design

No canvas board. The user chose no new board for this goal ("No canvas boards.
The tests check the header and row text"). The older boards under
`/home/penguin/source/hive/.hive/tmp/coordinator/designs/qmeter/` belong to the
`qmeter-simplify` layout goal; do not compare against them and do not capture a
screen for this task. The design of record is
`/home/penguin/source/hive/plans/qmeter-dash-clock-cache.md` plus the exact
strings above; the tests assert the header text and the drop order.

## Source files

| Path | Purpose |
| --- | --- |
| `internal/dash/model.go`, `internal/dash/model_test.go` | the one-second tick, its seam, its start in `Init`, and the quit rule |
| `internal/dash/layout/screen.go`, `internal/dash/layout/screen_test.go`, `internal/dash/layout/screen_test_helpers_test.go` | the header text and the drop order |
| `internal/dash/layout/testdata/screen-*.txt` | the goldens whose header changes |
| `README.md` | the header sentence in `## Dashboard` |
| `.hive/spec/qmeter-dash-clock-cache/tasks/<id>/brief.md`, `.../evidence/` | this brief and the evidence |

No other file. In particular: no change to `internal/display/`,
`internal/usage/`, `cmd/`, or the cached-card behaviour of the previous task.

## Verification

| ID | Check | Expected result |
| --- | --- | --- |
| V1 | Clock tick test, red then green | `Init` schedules one clock tick with a one-second interval; a tick re-arms exactly one more; a tick leaves `loading`, the refresh generation and the spinner unchanged and starts no fetch. It fails on the base and passes on the branch. |
| V2 | Header text | At 140x45 the first line ends with `fetched 21:23:41 · now 14:22:07` in plain text (ANSI stripped), and the line still has 140 cells. |
| V3 | Drop order | Three width cases: the full right side; `fetched HH:MM:SS` alone; the left side only. A test covers each, and each line is exactly as wide as the frame. |
| V4 | Before the first result | The `fetching…` right side is unchanged and carries no `now` part. |
| V5 | Quit during a pending tick | `q` with a clock tick pending returns `tea.Quit`, and `TestRun_QuitsOnQ` and `TestRun_QuittingCancelsTheFetchInFlight` still pass. |
| V6 | Goldens and fit | The header goldens show the new text; `TestScreenGoldenFrames`, `TestScreenFrameFitsEveryDimension`, `TestScreenDropsPartsInMeasuredOrder` and `TestScreenBelowFloorShowsOnlyTooSmallMessage` pass. |
| V7 | `git diff dev --stat` | Only the paths in the source table, plus the task records. |
| V8 | The gate | `gofmt -l .` prints nothing; `go vet ./...`, `go test -race ./...`, `bash .github/scripts/next-version_test.sh` and `golangci-lint run --new-from-rev=dev ./...` pass. `golangci-lint run ./...` shows the 27 findings that already exist on `dev`; record them, do not fix them. |

The 27 baseline lint findings are known on `dev` and are out of scope.

## Finish

Commit with a subject such as `feat(dash): show fetched and current time to the
second`. Push the task branch. Write `evidence/V1.md` to `V8.md`. Run
`hive task complete`. Send the manager the SHA, the header text from V2, and the
V1 to V8 result.

## Common rules

- Read `/home/penguin/source/hive/plans/qmeter-dash-clock-cache.md` and
  `.hive/spec/qmeter-dash-clock-cache/README.md`. The decisions are final. When
  the code does not agree with this brief, stop and report it with
  `hive inbox send --to manager-qmeter TEXT --agent NAME`.
- No canvas board for this goal. The tests check the header and row text.
- The user is colourblind: each state needs a glyph or a word. Colour can add to
  a state. Colour must not be the only signal.
- Read `AGENTS.md` in the worktree. Work test-first: write each test, run it,
  confirm it fails for the right reason, then write the code. Never weaken or
  skip an existing test to get green; when a test checks removed behaviour,
  delete or rewrite it and say which in the evidence.
- Keep colour bound to the destination with `display.Renderer`. Pipes and
  `NO_COLOR` give plain text.
- Gate, from the worktree root: `gofmt -l .` prints nothing; `go vet ./...`,
  `go test -race ./...` and `bash .github/scripts/next-version_test.sh` exit 0;
  `golangci-lint run --new-from-rev=dev ./...` reports 0 issues.
- Evidence goes to `.hive/spec/qmeter-dash-clock-cache/tasks/<id>/evidence/` in
  the worktree. Commit it on the task branch.
- Commit on the task branch only. Never touch `main`. No release, no tag.
- Verifier: commit and push `VERIFIED.md` in that evidence folder, then run
  `hive task verify`.
