# Verified: 52-simplify-wiring

Task commits: `62971f2`, `119aaa1`. Base: `e72190f` (merge base with `dev`). The verifier re-ran each item on 2026-10-19. Captures are in the task evidence folder under `verifier/`.

Verdict: **pass**.

| ID | Result | Observation |
| --- | --- | --- |
| V1 | pass | The four added test files (`cmd/dash/removed_flags_test.go`, `internal/dash/config/removed_keys_test.go`, `internal/dash/model_fullscreen_test.go`, `internal/display/bar_title_test.go`) were copied onto a worktree at `e72190f`. They compile there and fail for the expected reasons: removed flags accepted (`execute error = <nil>`), `Load returned no warning`, `View` does not match `layout.Screen` at 79x23/80x24/140x45, the header has no `fetched 14:22`, keys `t c j k down up pgdown pgup g G home end space b` change the frame, and the visible window name has 0 cells. On the branch, all of these tests pass. |
| V2 | pass (note) | Built binary, run in tmux, real providers. 140x45: header with `fetched 23:01`, 2x2 cards, `resets` panel (9 rows) beside `balances`, key bar, 45 rows. 93x30: cards and full-width balances, no resets panel, 30 rows. 80x24: cards only, 24 rows. On this data, 80x24 shows cards only and does not show the compact rows, because the cards fit. The compact tier is covered by the screen task's fixture. 70x20: only `terminal too small: 70x20, need 80x24`. No scroll state and no stale rows after each resize. |
| V3 | pass | At 140x45, `t`, `c`, `j`, `Down` each left the captured pane byte-identical. `r` started a refresh: the header changed from `fetched 23:01` to `fetched 23:02`. `q` exited with status 0, and the pane returned to the shell (only `EXIT=0` visible), so the alternate screen was left. |
| V4 | pass | The config at `XDG_CONFIG_HOME` had `meter_width = 50` and `[colors.claude]` set to `#ff00ff` for light and dark. Stderr had exactly one line: `warning: <path>/qmeter/config.toml: ignoring removed key "meter_width"`. The truecolor pane capture shows the claude card border and title in `38;2;255;0;255` (19 lines). |
| V5 | pass | `go run . --width 50`, `--thickness 3`, `--vertical`, `--no-banner`: each prints `Error: unknown flag: --<name>` and exits with status 1. |
| V6 | pass | The brief's grep prints nothing (exit 1). `internal/dash/gauge` and `internal/dash/banner` are gone. `internal/dash/layout` holds only `doc.go`, `providers.go`, `screen.go` and the screen tests and goldens. |
| V7 | pass | `gofmt -l .` prints nothing. `go vet ./...`, `go test -race -count=1 ./...` and `bash .github/scripts/next-version_test.sh` exit 0. |

## Note on R5b (not a V item)

The live 80x24 frame shows `▸ fable wee… 96.0% · resets in 6d6h`. `bar.go` shortens the right side only after the name would fall below min(its width, 8) cells, so a name longer than 8 cells is truncated while `resets in` is still shown. The added test covers only the case below the floor (`automatically` at width 30). A strict reading of R5b ("shorten the right side first … only then truncate the name") would show `▸ fable weekly 96.0% · 6d6h` in this case. The manager should decide which reading was intended. The original defect, where the name was gone completely, is fixed.
