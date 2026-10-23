# Independent verification: 127-dash-spec

Result: pass.

- Verifier: `t127-verifier-1`.
- Date: 2026-10-23 UTC.
- Reviewed HEAD: `eb62adaf150f3f56bd05a9cf7a6aa495219fd602`.
- Base `dev`: `67ff84abdc2ca66ecee186fe02f03603931320c8`.
- Commands ran from the task worktree root.
- The verifier changed only this evidence file.

## Verification items

| Item | Result | Independent observation |
| --- | --- | --- |
| V1 | pass | `git ls-files .hive/spec/qmeter-dash-clock-cache` exited 0. It listed the README, the task brief and worker evidence V1 to V6. |
| V2 | pass | The README has all required sections in order. It preserves decisions 1 to 6, all four assumptions, M1 to M7, the supersession sentence and the user-input rule. See the content check below. |
| V3 | pass | `git check-ignore -v .hive/spec/qmeter-dash-clock-cache/README.md` printed nothing and exited 1. `git status --porcelain` printed nothing before this evidence file was created. The README is tracked. |
| V4 | pass | `git diff dev --stat` listed eight files: the README, the brief and V1 to V6. No Go file changed. `git diff --exit-code dev -- '*.go' go.mod go.sum .github` printed nothing and exited 0. This verifier adds only `evidence/VERIFIED.md`. |
| V5 | pass | All task gate commands passed. The additional `go mod tidy -diff` check passed. An uncached race test run also passed. See the command table. |
| V6 | pass | `golangci-lint run --new-from-rev=dev ./...` printed `0 issues.` and exited 0. Full lint exited 1 with the expected 27 baseline findings. The brief permits these findings. See the list below. |

## Content check

The verifier read these sources independently:

- `/home/penguin/source/hive/plans/qmeter-dash-clock-cache.md`.
- `.hive/spec/qmeter-simplify/README.md`.
- The task brief and the new spec README.

The title and source line match the brief. The source line names the approved plan, date and chat references.

The sections occur in this order: Decisions, Assumptions, Manager decisions, User input, Task plan.
The supersession sentence occurs after M7 and before User input, as required.

| Content | Observation |
| --- | --- |
| Decisions 1 to 6 | The header uses `fetched HH:MM:SS · now HH:MM:SS` in local time. The clock redraws each second. The cache is per provider and lasts only for the session. One-shot commands stay unchanged. |
| Cached state | Decision 3 specifies `◌ cached · fetched HH:MM:SS · <error reason>`. Both `◌` and `cached` identify the state without colour. |
| Error and width rules | Token expiry bypasses the cache. Header fetch time includes failed refreshes. A narrow header drops the clock first, then the right side. |
| Four assumptions | The cache covers windows and balances. A first-fetch failure has no cache. Success replaces cached data immediately. No canvas boards are required. |
| M1 and M2 | The cache belongs to `internal/dash`. `usage.Result` stays fresh-only. Cards and compact rows show cached data. R1 and the balances panel stay fresh-only. |
| M3 | The exact cached line uses `<reason>` with no `error: ` prefix. It uses `usage.ProviderError.Message`. The line keeps its error colour. Bars, percentage and countdown render faint. |
| M4 and M5 | `KindTokenExpired` identifies expiry. The zero kind means another error. Success replaces the provider entry and its fetch time. Other failures retain it. Expiry removes it. One-shot text and JSON stay unchanged. |
| M6 and M7 | One `tea.Tick` starts in `Init` and re-arms each second. It only redraws and does not block input or start a fetch. The initial header shows `fetching…`. |
| Supersession | Decision 1 replaces simplify decision 15, including its `fetched HH:MM` format and no-clock rule. Other simplify requirements remain in force. |
| User input | `q` and `ctrl+c` work in every state. Keys get an immediate visible response. The input loop never waits for I/O. |
| Task plan | The typed error kind and cache task precedes the header clock task. The spec has no task history. |

This task changes documents only. It adds no behavior test or Go test, so no new red/green test pair applies.
The verifier did not change code to force an existing test to fail.
The brief excludes screen captures and comparisons with older boards. No UI implementation changes in this task.

## Gate commands

| Command | Exit | Output |
| --- | --- | --- |
| `gofmt -l .` | 0 | Empty. |
| `gofmt -l $(git ls-files -co --exclude-standard '*.go')` | 0 | Empty. |
| `go mod tidy -diff` | 0 | Empty. |
| `go vet ./...` | 0 | Empty. |
| `go test -race ./...` | 0 | All test packages passed. Most results used the Go test cache. |
| `go test -race -count=1 ./...` | 0 | All 27 test packages passed without cached results. The module root has no test files. |
| `bash .github/scripts/next-version_test.sh` | 0 | All 10 checks passed. Final line: `all tests passed`. |
| `golangci-lint run --new-from-rev=dev ./...` | 0 | `0 issues.` |
| `golangci-lint run ./...` | 1 | 27 baseline issues: 21 errcheck, 2 ineffassign, 4 staticcheck. Accepted by V6. |

### Uncached race test output

```text
?   github.com/Harrison-Blair/qmeter [no test files]
ok  github.com/Harrison-Blair/qmeter/cmd 1.009s
ok  github.com/Harrison-Blair/qmeter/cmd/dash 1.030s
ok  github.com/Harrison-Blair/qmeter/cmd/resets 1.026s
ok  github.com/Harrison-Blair/qmeter/cmd/spend 1.025s
ok  github.com/Harrison-Blair/qmeter/cmd/update 1.021s
ok  github.com/Harrison-Blair/qmeter/cmd/usage 1.026s
ok  github.com/Harrison-Blair/qmeter/cmd/version 1.015s
ok  github.com/Harrison-Blair/qmeter/internal/dash 1.431s
ok  github.com/Harrison-Blair/qmeter/internal/dash/config 1.021s
ok  github.com/Harrison-Blair/qmeter/internal/dash/layout 12.710s
ok  github.com/Harrison-Blair/qmeter/internal/dash/theme 1.019s
ok  github.com/Harrison-Blair/qmeter/internal/display 1.029s
ok  github.com/Harrison-Blair/qmeter/internal/lib/credstore 1.023s
ok  github.com/Harrison-Blair/qmeter/internal/lib/httpx 1.078s
ok  github.com/Harrison-Blair/qmeter/internal/lib/piauth 1.030s
ok  github.com/Harrison-Blair/qmeter/internal/lib/subprocess 1.054s
ok  github.com/Harrison-Blair/qmeter/internal/provider 1.005s
ok  github.com/Harrison-Blair/qmeter/internal/provider/claude 1.066s
ok  github.com/Harrison-Blair/qmeter/internal/provider/codex 1.092s
ok  github.com/Harrison-Blair/qmeter/internal/provider/cursor 1.050s
ok  github.com/Harrison-Blair/qmeter/internal/provider/opencodego 1.140s
ok  github.com/Harrison-Blair/qmeter/internal/provider/providertest 1.038s
ok  github.com/Harrison-Blair/qmeter/internal/resets 1.015s
ok  github.com/Harrison-Blair/qmeter/internal/spend 1.015s
ok  github.com/Harrison-Blair/qmeter/internal/update 1.082s
ok  github.com/Harrison-Blair/qmeter/internal/usage 1.425s
ok  github.com/Harrison-Blair/qmeter/internal/version 1.007s
```

## Full lint findings

These are the 27 findings emitted by the verifier's full lint run.
The source and configuration files are identical to `dev`. None of these findings comes from this task.

| Location | Linter | Finding |
| --- | --- | --- |
| `cmd/spend/spend.go:40:17` | errcheck | Unchecked `fmt.Fprintln` return. |
| `cmd/usage/usage.go:56:17` | errcheck | Unchecked `fmt.Fprintln` return. |
| `internal/display/display_test.go:60:18` | errcheck | Unchecked `read.Close` return. |
| `internal/display/display_test.go:61:19` | errcheck | Unchecked `write.Close` return. |
| `internal/lib/httpx/httpx.go:97:23` | errcheck | Unchecked `resp.Body.Close` return. |
| `internal/provider/claude/pi_test.go:74:16` | errcheck | Unchecked `fmt.Fprint` return. |
| `internal/provider/codex/pi_test.go:41:18` | errcheck | Unchecked `os.WriteFile` return. |
| `internal/provider/codex/pi_test.go:43:18` | errcheck | Unchecked `os.WriteFile` return. |
| `internal/provider/codex/pi_test.go:62:13` | errcheck | Unchecked `w.Write` return. |
| `internal/provider/opencodego/db.go:79:16` | errcheck | Unchecked `db.Close` return. |
| `internal/provider/opencodego/db_test.go:285:16` | errcheck | Unchecked `db.Close` return. |
| `internal/update/asset.go:72:16` | errcheck | Unchecked `gz.Close` return. |
| `internal/update/asset.go:127:17` | errcheck | Unchecked `rc.Close` return. |
| `internal/update/hint.go:138:14` | errcheck | Unchecked `fmt.Fprintln` return. |
| `internal/update/release.go:87:23` | errcheck | Unchecked `resp.Body.Close` return. |
| `internal/update/release.go:147:23` | errcheck | Unchecked `resp.Body.Close` return. |
| `internal/update/replace.go:49:12` | errcheck | Unchecked `tmp.Close` return. |
| `internal/update/run.go:151:14` | errcheck | Unchecked `fmt.Fprintf` return. |
| `internal/update/run.go:234:13` | errcheck | Unchecked `fmt.Fprintf` return. |
| `internal/usage/pi_test.go:52:16` | errcheck | Unchecked `fmt.Fprint` return. |
| `internal/usage/pi_test.go:54:16` | errcheck | Unchecked `fmt.Fprint` return. |
| `internal/lib/piauth/piauth.go:138:3` | ineffassign | Ineffectual assignment to `end`. |
| `internal/provider/claude/pi_test.go:138:2` | ineffassign | Ineffectual assignment to `p`. |
| `internal/lib/piauth/piauth.go:63:24` | staticcheck | ST1005: capitalized error string. |
| `internal/lib/piauth/piauth.go:67:24` | staticcheck | ST1005: capitalized error string. |
| `internal/lib/piauth/piauth.go:77:24` | staticcheck | ST1005: capitalized error string. |
| `internal/usage/render.go:206:35` | staticcheck | S1016: use a conversion from `ProviderError` to `jsonError`. |

The worker's V6 sample lists `cmd/resets/resets.go:42` instead of `internal/update/hint.go:138`.
Both runs report the expected totals. The new-finding gate reports zero. No Go source differs from `dev`.

## Additional observation and cleanup

`git diff --check dev` exited 2 for trailing whitespace in the worker's `evidence/V2.md`, line 37.
This is a non-blocking evidence formatting issue. It is outside V1 to V6. The verifier did not edit the worker's evidence.

The verifier created no scratch files, panes or worktrees. The worktree was clean before this report was created.
