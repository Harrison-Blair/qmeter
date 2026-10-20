# VERIFIED — 78-simplify-hang-fix-2

Verifier re-ran each item on the task branch at `8422409` (base `dev` = `978f2ba`).

| ID | Result | What I ran and saw |
| --- | --- | --- |
| V1 | pass | `evidence/V1.md` has reproduction steps (real providers, `refresh_interval = 1`, no `QMETER_NO_UPDATE_CHECK`), the root cause with file and line (`internal/dash/layout/screen.go:64` made a new `display.Renderer(os.Stdout)` per `View`, so Lipgloss `HasDarkBackground` ran termenv's terminal status query on the event loop, up to 5 s, and read the same terminal as Bubble Tea's input), and a `SIGQUIT` goroutine dump showing goroutine 1 in `termenv.(*Output).waitForData` under `Model.View`. The fix in the diff addresses exactly that path. |
| V2 | pass | Red: in a scratch worktree of the branch I put the runtime back to base behaviour (`Screen` ignores `options.Renderer` and builds a renderer per call; `Run`/`New` prepare no renderer) and kept the new tests. `go test ./internal/dash ./internal/dash/layout -run 'Test(ScreenReusesProvidedRenderer\|RunPreparesOneDestinationRendererBeforeReadingInput\|NewDashboardRendererKeepsDetectedBackground)$'` failed: `renderer factory calls = 0, want 1`, `background query calls = 0, want 1`, `renderer did not keep the detected light/dark choice`, `Bubble Tea read input before background detection`, `Screen created 3 renderers despite a provided renderer`. Green: same command on the branch, all three tests PASS. |
| V3 | pass | Built the branch binary, ran it 10 times in a real tmux terminal (120x40) with real providers, scratch `XDG_CONFIG_HOME` (`refresh_interval = 1`) and `XDG_CACHE_HOME`, `QMETER_NO_UPDATE_CHECK` unset. Table below. Every `q` exited in ≤ 22 ms; after each exit the pane foreground was `bash`, `alternate_on = 0`, and `stty -a` showed `icanon echo`. Extra run: `ctrl+c` 4 s after the first frame exited in 21 ms, terminal restored. The screen showed 4 providers; Claude showed `error: rate limited`, so a failing provider was also covered. |
| V4 | pass | `git diff dev --stat`: source changes only in `internal/dash/layout/screen.go`, `internal/dash/layout/screen_renderer_test.go`, `internal/dash/model.go`, `internal/dash/run.go`, `internal/dash/run_test.go`; the rest are the task brief and evidence files. All inside the brief's allowed `internal/dash/**`. |
| V5 | pass | `gofmt -l .` printed nothing; `go vet ./...` exit 0; `go test -race -count=1 ./...` exit 0, every package `ok`; `bash .github/scripts/next-version_test.sh` exit 0. |

## V3 runs

| Run | When `q` was sent | `r` → refreshing frame | Resize | `q` → exit |
| --- | --- | ---: | --- | ---: |
| 1 | during the first fetch | — | no | 22 ms |
| 2 | 3 s after first fetch | — | no | 21 ms |
| 3 | 8 s after, during refresh | 11 ms | no | 21 ms |
| 4 | 15 s after | — | no | 21 ms |
| 5 | 20 s after, during refresh | 34 ms | no | 18 ms |
| 6 | 5 s after, during refresh | 23 ms | before `r` and during refresh | 20 ms |
| 7 | 30 s after | — | no | 20 ms |
| 8 | 45 s after, during refresh | 23 ms | no | 21 ms |
| 9 | 12 s after | — | yes, before `q` | 21 ms |
| 10 | 60 s after, during refresh | 26 ms | no | 19 ms |

The automatic refresh ran every second during every wait. The first fetched frame appeared 315–533 ms after launch.

Limits: the longest session was about 61 s; I did not hold the dashboard open for several minutes. The update check ran in each launch (not disabled), but I did not time a key to land exactly inside it.
