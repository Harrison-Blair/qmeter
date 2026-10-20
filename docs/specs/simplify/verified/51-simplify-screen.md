# Verified: 51-simplify-screen

Task commit: `56e2948`. Base: `dev` at `103b84d`. Verifier re-ran each item on 2026-10-19.

Verdict: **fail** (V2).

| ID | Result | Observation |
| --- | --- | --- |
| V1 | pass | `screen_test.go` and the `testdata/screen-*.txt` frames, copied onto a `dev` worktree, fail to compile (`undefined: layout.Screen`). On the task branch, all 8 `TestScreen*` tests pass. |
| V2 | fail | The 140x45, 93x30 and 80x24 frames match R2/R3. The 140x45 frame has 2-column cards and R1 + balances. The 93x30 frame drops R1 and keeps balances at full width. The 80x24 frame drops both panels. The stripped text is aligned and no line wraps. **The `screen-79x23.txt` frame is not a "too small" frame.** It shows the header, the 2-column cards and the key bar (23 lines). R5 labels 79x23 "too small", and decision 7 says "Below the floor [about 80x24], show one message with the needed size". The worker's V2 evidence says the 79x23 frame "shows one centered size message". That is not what the file contains. Cause: `Screen` applies the 80x24 floor only to the compact tier and to the loading and empty frames. The card tiers are measured with no floor. Minor note: at 80x24 the rate-limited cursor `auto` window shows no name (`▸  0.0% · resets in 3d12h · rate l…`). At 93x30 it shows `a…`. |
| V3 | pass (note) | `go test -run '^TestScreenDropsPartsInMeasuredOrder$'` passes. At 100 columns: resets drop at height 10, balances at 9, cards stay through 6, too small at 5. That is the R3 order. The compact tier is never reached because its need is at least 80x24. Note: cards stay at heights 6–9, below the 24-row floor. This is the same cause as V2. |
| V4 | pass | `git diff dev` shows no change to `layout.go` or to any existing `_test.go` file. 324 non-`TestScreen` test results pass on the branch. The same 324 pass on `dev`. |
| V5 | pass | `git diff dev --stat` lists only `internal/dash/layout/screen.go`, `screen_test.go` and eight `testdata/screen-*.txt` files. These are in the source table. |
| V6 | pass | `gofmt -l .` prints nothing. `go vet ./...`, `go test -race ./...` and `bash .github/scripts/next-version_test.sh` exit 0. |

## Repair needed

Decide how the floor applies to the measured card tiers. Then make 79x23 (the board fixture) produce the "too small" frame, as R5 and decision 7 require. The brief also asks for a 60x40 1-column golden, and 60 is below the 80-column floor. The manager must resolve this conflict: either the floor applies only to height, or 60x40 is also "too small".
