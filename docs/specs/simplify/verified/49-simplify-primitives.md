# Verified: 49-simplify-primitives

Task commit: `7e61160`. Base: `dev` at `dbbc000`. Verifier re-ran each item on 2026-10-19.

| ID | Result | Observation |
| --- | --- | --- |
| V1 | pass | The four new test files copied onto a `dev` worktree fail to compile (`undefined: Bar`, …). On the task branch, all 15 new tests pass (`Bar*`, `CompactBar*`, `ProviderCard*`, `FormatCountdown*`, `ResetRows*`, `SortWindows*`). |
| V2 | pass | Width 60, plain renderer. `claude 5h 81% in 16m` gives `▸ 5h … 81.0% · resets in 16m` (60 cells) and 49 `━` + 11 `─`. `cursor auto 0% rate limited` gives `0.0% · resets in 3d12h · rate limited` and 60 `─`. R1 rows: `   16m  claude  5h     Mon 19 21:39   now 81.0%` and ` 3d12h  cursor  auto   Fri 23 09:23   now 0.0% rate limited`. Rows are sorted soonest first. Overdue gives `0m` with the date shown as is. Unknown gives `-` and `-` and sorts last. The card top is `╭─ ◆ claude ─── … max ─╮`. Note: three spaces stand before `now`. This matches `Resets.dc.html`. The brief's template shows two. |
| V3 | pass | Base and branch binaries ran with real providers and stdout piped. `usage`: 553 bytes, identical. `resets`: 463 bytes, identical. |
| V4 | pass | `git diff dev --stat` lists only `internal/display/{bar,card,countdown,resetlist}{,_test}.go`, `internal/resets/resets.go` and `internal/usage/render.go`. |
| V5 | pass | `gofmt -l .` prints nothing. `go vet ./...`, `go test -race ./...` and `bash .github/scripts/next-version_test.sh` exit 0. |
