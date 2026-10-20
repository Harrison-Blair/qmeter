# Verified: 51-simplify-screen

Task commits: `56e2948`, repair `6b15f67`. Base: `103b84d` (merge base with `dev`). Verifier re-ran each item on 2026-10-19 after the repair.

Verdict: **pass**.

| ID | Result | Observation |
| --- | --- | --- |
| V1 | pass | `screen_test.go` and the `testdata/screen-*.txt` frames, copied onto a worktree at `103b84d`, fail to compile (`undefined: layout.Screen`). Copied onto the pre-repair commit `56e2948`, `TestScreenGoldenFrames` and `TestScreenBelowFloorShowsOnlyTooSmallMessage` fail. On `6b15f67`, all 9 `TestScreen*` tests pass. |
| V2 | pass (note) | `screen-140x45.txt`: header with `fetched 21:23`, 2-column cards, `resets` panel (9 rows) beside `balances` (2 rows), key bar. `screen-93x30.txt`: cards, R1 dropped, balances at full width. `screen-80x24.txt`: cards only, both panels dropped. `screen-79x23.txt`: only the centred `terminal too small: 79x23, need 80x24`. `79x45`, `140x23` and `60x40` also show only the too-small message. Measured with East Asian width: every golden line is at most the frame width, and the full-content lines are exactly the frame width (goldens are right-trimmed; the property test checks exact width). No ANSI escapes, no wrapped lines. Note (carried from the first run): at 80x24 the rate-limited cursor `auto` row shows no window name (`▸  0.0% · resets in 3d12h · rate l…`), and at 93x30 it shows `a…`. |
| V3 | pass | `TestScreenDropsPartsInMeasuredOrder` passes. At 100 columns, height 46 → 23: resets only at 46; balances at 31 and up; cards at 27 and up; compact rows at 24–26; too-small at 23. That is the R3 order with the 80x24 floor. |
| V4 | pass | `git diff 103b84d` shows no change to `layout.go` or to any pre-existing `_test.go` in `internal/dash/layout`. 70 non-`TestScreen` test results in the package pass. |
| V5 | pass | `git diff 103b84d..HEAD --stat` lists `screen.go`, `screen_test.go`, ten `testdata/screen-*.txt` files, and this verification record. A literal `git diff dev --stat` also shows the one-shot task, because `dev` moved to `70d9f8f` after this branch was cut. Those files are not changed by this branch. |
| V6 | pass | `gofmt -l .` prints nothing. `go vet ./...`, `go test -race ./...` and `bash .github/scripts/next-version_test.sh` exit 0. |
