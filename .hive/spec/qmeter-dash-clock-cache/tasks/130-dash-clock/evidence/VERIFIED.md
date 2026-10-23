# Independent verification: 130-dash-clock

- Verifier: `t130-verifier-1`.
- Date: 2026-10-23 UTC.
- Task commit: `81e31470d7653e6cc4c25f9ec5561b0a5be7cb11`.
- Implementation commit: `6250bf8`.
- Base `dev`: `8f53aeeb9a49911757fcc98db56e054a0142d490`.
- Branch: `task/130-dash-clock`.
- Verdict: **pass**. V1–V8 pass under the brief and the manager's superseding instruction.

No functional defect appeared in these checks. The manager approved the additional test path and corrected the baseline lint count before the verdict.

## Item results

| Item | Result | Independent observation |
| --- | --- | --- |
| V1 | pass | Base implementation fails the clock tests. Base plus compile-only clock declarations fails behaviorally. Branch schedules one 1s tick in `Init` and re-arms one tick per message. Loading, refresh generation and spinner remain unchanged in first-fetch, shown and refreshing states. The clock branch returns only the next tick. |
| V2 | pass | The ANSI-stripped 140x45 header ends with `fetched 21:23:41 · now 14:22:07` and has 140 cells. The base header fails this assertion. |
| V3 | pass | All six direct header cases pass. Widths 74/80 show both times, 73/59 show fetched alone, 58 shows only the padded left side, and 30 truncates the left side. Each line has the requested cell width. The base fails all five changed cases. |
| V4 | pass | Zero `FetchedAt` shows `fetching…` without `now`. Adding a clock to this state causes the test to fail. The unmodified branch passes. |
| V5 | pass | Pending-clock model checks return `tea.QuitMsg` for `q` and `ctrl+c`. Both named run tests pass. Removing quit handling causes the pending-clock test to fail. A real clock run also exits on `q`. |
| V6 | pass | All four named layout tests pass. Seven goldens change only their header line. The base header fails the six applicable regular golden cases. All other frame content stays unchanged in the golden diff. |
| V7 | pass | `git diff dev --stat` contains only the source table paths, task records, and `internal/dash/layout/screen_renderer_test.go`. The manager explicitly approved that test file before the verdict. |
| V8 | pass | All required success commands pass, including tidy diff and new-line lint. Full lint reports the corrected baseline of 26 findings on both branch and `dev`, with no new lint finding. |

The manager received the two stale-brief differences in inbox records 65 and 66 before the verdict.
The manager then sent this superseding instruction:

1. `internal/dash/layout/screen_renderer_test.go` is in scope. Its package-internal test is the approved place to call `headerLine` below the screen floor. The rest of the source table stands.
2. V8's baseline is 26 findings on `dev`. Do not fail V8 for the stale count of 27.
3. V1–V6 and the functional checks stand as run.

The manager reported that `hive task edit` was unavailable because the task was already completed.
The verifier applied the explicit instruction without changing the brief.

## Red and green runs

All commands ran from the worktree root, except the explicitly identified `dev` lint comparison.
Go overlays supplied temporary base or broken source files without changing project source files.
`SCRATCH` denotes `/home/penguin/source/hive/.hive/tmp/t130-verifier-1.mkdCEI`.

### V1

```sh
go test -overlay="$SCRATCH/model-base.json" -count=1 ./internal/dash/ -run Clock -v
```

Exit 1: branch tests cannot compile against the base because `clockTick` and `clockMsg` do not exist.

A second overlay added only `var clockTick = tea.Tick` and `type clockMsg struct{}` to the base.
It did not add clock behavior.

```sh
go test -overlay="$SCRATCH/model-base-seam.json" -count=1 ./internal/dash/ -run 'TestInitStartsTheOneSecondClock|TestClockTickReArmsAndOnlyRedraws' -v
```

Exit 1, with these behavioral failures:

```text
Init scheduled clock ticks [], want one of 1s
tick scheduled clock ticks [], want one of 1s
```

The tick assertion failed in all three model states.

### V2, V3 and V6

```sh
go test -overlay="$SCRATCH/screen-base.json" -count=1 ./internal/dash/layout/ -run 'TestScreenHeaderShowsFetchedAndNowToTheSecond|TestHeaderLineDropsNowThenTheWholeRightSide|TestScreenGoldenFrames' -v
```

Exit 1. V2 saw `fetched 21:23`, without seconds or `now`.
V3 failed the five cases with changed output. The unchanged left-truncation case passed.
The regular goldens failed on line 1 at 140x45, 100x40, 93x39, 93x30, 93x26 and 80x24.
The four unchanged too-small golden cases passed.

### V4

The mutation changed the initial right side to `fetching… · now HH:MM:SS`.

```sh
go test -overlay="$SCRATCH/screen-loading-broken.json" -count=1 ./internal/dash/layout/ -run TestScreenHeaderBeforeTheFirstResultHasNoClock -v
```

Exit 1: the header ended with `fetching… · now 14:22:07`, rather than ` fetching…`.

### V5

The mutation returned no command for the direct quit-key case.

```sh
go test -overlay="$SCRATCH/model-quit-broken.json" -count=1 ./internal/dash/ -run TestQuitWorksWithAClockTickPending -v
```

Exit 1: `"q" returned no command`.

### Green after the red runs

```sh
go test -count=1 -race ./internal/dash/ -run 'Clock|TestRun_QuitsOnQ|TestRun_QuittingCancelsTheFetchInFlight|TestQuit' -v
go test -count=1 ./internal/dash/layout/ -run 'TestHeaderLineDropsNowThenTheWholeRightSide|TestScreenHeader|TestScreenGoldenFrames|TestScreenFrameFitsEveryDimension|TestScreenDropsPartsInMeasuredOrder|TestScreenBelowFloorShowsOnlyTooSmallMessage' -v
```

Both commands exit 0. The dash run includes all three clock states, both pending-clock quit keys, `TestRun_QuitsOnQ`, and the blocked-provider quit cases.

The header, drop-order, regular golden and cached-card golden checks also pass under each of these time zones:

```sh
TZ=UTC go test -count=1 ./internal/dash/layout/ -run 'TestHeaderLine|TestScreenHeader|TestScreenGoldenFrames|TestScreenCachedCardKeepsWindowsWithTheCachedLine'
TZ=Asia/Kolkata go test -count=1 ./internal/dash/layout/ -run 'TestHeaderLine|TestScreenHeader|TestScreenGoldenFrames|TestScreenCachedCardKeepsWindowsWithTheCachedLine'
TZ=America/Los_Angeles go test -count=1 ./internal/dash/layout/ -run 'TestHeaderLine|TestScreenHeader|TestScreenGoldenFrames|TestScreenCachedCardKeepsWindowsWithTheCachedLine'
```

### Additional real-loop check

A verifier-only overlay added `TestVerifierLiveClockRedrawsWithoutFetching`.
It ran the real Bubble Tea program at 140x45 with a successful external provider stub.
It used the real clock and a one-hour refresh interval.
It sent `q` through a pipe after 2.3 seconds and read the program output after exit.
It required three distinct current times, one fetched time, one provider call, and exit before the six-second deadline.

The first two-time assertion also passed without a clock because `q` caused a final redraw.
The verifier strengthened the independent expectation to three distinct times and repeated both runs.

```sh
go test -overlay="$SCRATCH/live-red.json" -count=1 -race ./internal/dash/ -run TestVerifierLiveClockRedrawsWithoutFetching -v
go test -overlay="$SCRATCH/live-green.json" -count=1 -race ./internal/dash/ -run TestVerifierLiveClockRedrawsWithoutFetching -v
```

Red, using the base model plus compile-only declarations: exit 1, `real clock rendered 2 distinct current times, want at least 3`.
Green, using the task implementation: exit 0, with this observed header sequence:

```text
fetched 22:49:14 · now 22:49:14
fetched 22:49:14 · now 22:49:15
fetched 22:49:14 · now 22:49:16
```

The provider ran once. The program exited on `q` while the next clock tick was pending.
No live provider credentials or network requests were needed.

## Design and plain-text states

The brief has a Design section and explicitly waives boards and screen captures.
The design of record is `/home/penguin/source/hive/plans/qmeter-dash-clock-cache.md` plus the exact brief strings.
No older board was used. No screen capture was made.

| Board element | state without colour | Result | Cause or decision |
| --- | --- | --- | --- |
| Full header right side | `fetched 21:23:41 · now 14:22:07` | matches | Exact ANSI-stripped text and 140-cell width pass. |
| Narrow header, first drop | `fetched 21:23:41` | matches | Exact text at widths 73 and 59 passes. |
| Narrow header, second drop | `qmeter 1 provider · 0 windows · 0 balances` | matches | Only the left text remains at width 58, with frame padding. |
| Before the first result | `fetching…` | matches | The loading-header assertion passes without a clock suffix. |
| Existing cached and error states | `◌ cached` and `error: offline` | matches | `TestScreenCachedLineAndErrorLineDifferInPlainText` passes with the updated header. |

## V7 scope detail

`git diff dev --stat` before this evidence file: 23 files changed, 636 insertions, 20 deletions.
The only path beyond the original source table is the manager-approved test file:

```text
internal/dash/layout/screen_renderer_test.go | 33 +++++
```

That file adds `TestHeaderLineDropsNowThenTheWholeRightSide` and its width-measurement import.
Its test passes. The superseding instruction approves this location because the drop states are below the 80-column screen floor.
Every changed path is now permitted.
No file in `internal/display/`, `internal/usage/` or `cmd/` changed.
`git diff --check` exits 0.

## V8 gate detail

| Exact command | Exit | Output |
| --- | --- | --- |
| `gofmt -l .` | 0 | No output. |
| `go mod tidy -diff` | 0 | No output. |
| `go vet ./...` | 0 | No output. |
| `go test -race ./...` | 0 | All packages pass. |
| `bash .github/scripts/next-version_test.sh` | 0 | All ten cases pass, followed by `all tests passed`. |
| `golangci-lint run --new-from-rev=dev ./...` | 0 | `0 issues.` |
| `golangci-lint run ./...` | 1 | `26 issues:`, comprising 21 errcheck, 2 ineffassign and 3 staticcheck. |

Versions: Go `go1.27.2-X:nodwarf5`, golangci-lint `2.15.0`.

The verifier exported `dev` with `git archive dev` into scratch and reran `golangci-lint run ./...` there.
That command also exits 1 with 26 findings and the same category counts.
The observed count agrees with the manager's corrected baseline.
The manager attributes the reduction from the older 27-finding baseline to the cache task's typed error kind.
The printed samples differ between the two runs. Every reported file is unchanged by this task.
A direct `git diff dev -- <all reported files>` prints nothing.

These are the 26 findings printed on the task branch:

| File and line | Finding |
| --- | --- |
| `cmd/resets/resets.go:42` | errcheck: `fmt.Fprintln` |
| `cmd/spend/spend.go:40` | errcheck: `fmt.Fprintln` |
| `internal/display/display_test.go:60` | errcheck: `read.Close` |
| `internal/display/display_test.go:61` | errcheck: `write.Close` |
| `internal/lib/httpx/httpx.go:97` | errcheck: `resp.Body.Close` |
| `internal/provider/codex/pi_test.go:41` | errcheck: `os.WriteFile` |
| `internal/provider/codex/pi_test.go:43` | errcheck: `os.WriteFile` |
| `internal/provider/codex/pi_test.go:62` | errcheck: `w.Write` |
| `internal/provider/opencodego/db.go:79` | errcheck: `db.Close` |
| `internal/provider/opencodego/db_test.go:285` | errcheck: `db.Close` |
| `internal/update/asset.go:72` | errcheck: `gz.Close` |
| `internal/update/asset.go:127` | errcheck: `rc.Close` |
| `internal/update/hint.go:138` | errcheck: `fmt.Fprintln` |
| `internal/update/release.go:87` | errcheck: `resp.Body.Close` |
| `internal/update/release.go:147` | errcheck: `resp.Body.Close` |
| `internal/update/replace.go:49` | errcheck: `tmp.Close` |
| `internal/update/run.go:151` | errcheck: `fmt.Fprintf` |
| `internal/update/run.go:234` | errcheck: `fmt.Fprintf` |
| `internal/usage/pi_test.go:52` | errcheck: `fmt.Fprint` |
| `internal/usage/pi_test.go:54` | errcheck: `fmt.Fprint` |
| `internal/usage/pi_test.go:56` | errcheck: `fmt.Fprint` |
| `internal/lib/piauth/piauth.go:138` | ineffassign: `end` |
| `internal/provider/claude/pi_test.go:138` | ineffassign: `p` |
| `internal/lib/piauth/piauth.go:63` | staticcheck ST1005: capitalized error string |
| `internal/lib/piauth/piauth.go:67` | staticcheck ST1005: capitalized error string |
| `internal/lib/piauth/piauth.go:77` | staticcheck ST1005: capitalized error string |

The `dev` output replaces the task's samples at `cmd/spend/spend.go:40` and `internal/usage/pi_test.go:56` with these two pre-existing samples:

- `internal/provider/claude/pi_test.go:74`: errcheck, `fmt.Fprint`.
- `internal/update/run.go:176`: errcheck, `fmt.Fprintln`.

No baseline finding was fixed. No source file or worker evidence file was changed by the verifier.
All temporary overlays, test code, logs and the exported `dev` tree are removed after this result is recorded.
