# Independent verification: 128-dash-cache

- Verifier: `t128-verifier-1`.
- Date: 2026-10-23 UTC.
- Checked commit: `5b545dc2bd51ef5cdc1ffd664cf2f43087d4e0c3`.
- Base `dev`: `3d2c2574d03ac40c68ad8b6cbfed55e233e7e2f7`.
- Branch: `task/128-dash-cache`.
- Verdict: **fail**. V3 fails for an empty successful response.

I read the brief, the feature spec, the approved plan, and `AGENTS.md`.
I ran the checks independently. I did not edit source files or worker evidence.

## Results

| Item | Result | Observation |
| --- | --- | --- |
| V1 | pass | The base lacks `Kind`, so the new tests fail to compile. A zero-kind mutation also fails the wrapped-token test. The branch passes the token, timeout, cancellation, rate-limit, and undetected tests. |
| V2 | pass | The exact token-expiry JSON passes on the base and the branch. A mutation that adds `kind` to the envelope fails. Existing render tests pass. The required one-shot source diff is empty. |
| V3 | fail | Existing cache and model tests pass. Independent reproduction shows that an empty success does not replace the earlier response. A later failure restores obsolete windows and their earlier timestamp. |
| V4 | pass | The 140x45 fixture retains the cached Codex window and the exact cached line. The resets and balances panels contain fresh data only. The no-cache control and all existing screen goldens pass. |
| V5 | pass | The 80x24 compact fixture places Codex between Claude and OpenCode Go. Its message follows the window rows. The fit test reports `need 126x33`, renders all cached rows there, and rejects 126x32. |
| V6 | pass | The Ascii frame has no escape codes. `◌ cached` distinguishes cached data from `error: offline`. The card retains its bar. ANSI faint-style tests pass for cards and compact rows. |
| V7 | pass | The diff contains only permitted source paths, the permitted new cached golden, and task records. Existing goldens and prohibited source paths have no changes. |
| V8 | pass | Format, module tidy, vet, race tests, release-script tests, and new-issue lint pass. Full lint reports 26 existing findings on the branch and 27 on the base. The typed field removes one obsolete S1016 finding without a renderer change. |
| V9 | pass | Both required run tests pass. The blocked-fetch cases quit with `q`, `ctrl+c`, and combined `rq`. An ignore-key mutation makes these tests fail. |

## V3 failure: an empty success retains obsolete data

Requirement: a success replaces the provider entry at once, including its windows,
balances, and successful response time.

Observed sequence through the real dashboard model:

1. Codex succeeds at 14:22:07 with a weekly window at 28.0%.
2. Codex succeeds at 14:23:08 with no windows or balances.
3. Codex fails at 14:24:09 with `offline`.

The third frame restores the first response:

```text
qmeter 1 provider · 0 windows · 0 balances                                                                                     fetched 14:24
╭─ ● codex ────────────────────────────────────────────────────────────────────────────────────────────────────────────────────── prolite ─╮
│ ▸ weekly                                                                                                                28.0% · resets - │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━────────────────────────────────────────────────────────────────────────────────────────────────── │
│ ◌ cached · fetched 14:22:07 · offline                                                                                                    │
```

Expected: the first response must not survive the empty success.
The old window and the 14:22:07 timestamp must not reappear.

Cause: `internal/dash/cache.go:19-20` copies every previous entry.
Lines 30-46 replace entries only for providers with windows or balances.
An empty successful response never enters `fresh`, so the old entry survives.

This case is permitted by a real provider. The existing
`TestFetch_EmptyRateLimitObjectIsNotAnError` in
`internal/provider/codex/codex_test.go:944` accepts
`{"plan_type":"plus","rate_limit":{}}` with no windows and no error.
I ran that test again. It passed.

I added the following test through a temporary Go overlay on
`internal/dash/model_test.go`. The test uses the existing test helpers and imports.
Only the external provider response is synthetic. The cache, model, and renderer
remain the branch implementations.

```go
func TestVerifierEmptySuccessReplacesOldCachedData(t *testing.T) {
    clock := firstFetch
    m := New(Options{Now: func() time.Time { return clock }})
    m = resize(t, m, 140, 45)
    m, _ = step(t, m, resultMsg{res: codexSuccess(28)})
    if !strings.Contains(m.View(), "28.0%") {
        t.Fatal("initial successful response did not show 28.0%")
    }

    empty := usage.Run(context.Background(), []provider.Provider{
        providertest.Succeeding("codex", nil),
    }, "")
    if len(empty.Windows) != 0 || len(empty.Balances) != 0 || len(empty.Errors) != 0 {
        t.Fatalf("empty successful response was not empty: %#v", empty)
    }
    clock = secondFetch
    m, _ = step(t, m, resultMsg{res: empty})
    if strings.Contains(m.View(), "28.0%") {
        t.Fatal("empty successful response still shows the earlier windows")
    }

    clock = thirdFetch
    m, _ = step(t, m, resultMsg{res: codexFailure("")})
    view := m.View()
    t.Logf("frame after success, empty success, then timeout:\n%s", view)
    for _, stale := range []string{"28.0%", "fetched 14:22:07"} {
        if strings.Contains(view, stale) {
            t.Errorf("earlier response survived the empty success: %q", stale)
        }
    }
}
```

Command, from the worktree root:

```sh
go test -overlay="$S/edge.json" -count=1 -v ./internal/dash/ -run '^TestVerifierEmptySuccessReplacesOldCachedData$'
```

`$S` was the temporary verification directory in the hive home's `.hive/tmp/`.
The overlay replaced only `internal/dash/model_test.go` with its original contents
plus the test above. The source checkout did not change.

```text
=== RUN   TestVerifierEmptySuccessReplacesOldCachedData
    model_test.go:518: earlier response survived the empty success: "28.0%"
    model_test.go:518: earlier response survived the empty success: "fetched 14:22:07"
--- FAIL: TestVerifierEmptySuccessReplacesOldCachedData (0.00s)
FAIL github.com/Harrison-Blair/qmeter/internal/dash 0.002s
exit=1
```

I sent this finding to `manager-qmeter` as inbox record 54 before writing this report.
No fix is part of this verification.

## Red runs and green runs

All red runs used Go overlays. They did not change checked-out source files.
Each red command used this form:

```sh
go test -overlay="$S/NAME.json" -count=1 -v PACKAGE -run PATTERN
```

| Overlay name | Change | Package and pattern | Observed result |
| --- | --- | --- | --- |
| `v1-base-red` | Use `dev`'s `usage.go`. | `./internal/usage/`, `ErrorKind\|ZeroKind\|TokenExpiredErrorKeeps` | exit 1. Undefined `ErrorKind`, `KindTokenExpired`, and `ProviderError.Kind`. |
| `v1-kind-zero-red` | Return the zero kind for token expiry. | `./internal/usage/`, `ErrorKind\|ZeroKind` | exit 1. Wrapped token: `Kind = "", want "token-expired"`. Other cases pass. |
| `v2-kind-leak-red` | Add `kind` to the JSON error envelope. | `./internal/usage/`, `TokenExpiredErrorKeeps` | exit 1. Exact JSON differs. |
| `v3-base-red` | Replace `cache.go` with only its package declaration. | `./internal/dash/`, `TestCache\|TestViewShowsTheCachedResponseUntilATokenExpiry` | exit 1. Undefined `cache`. |
| `v3-no-replace-red` | Keep an existing entry instead of replacing it. | `./internal/dash/`, `TestCacheSuccessReplacesTheEntry` | exit 1. Entry keeps 40% and the first timestamp instead of 30% and the second timestamp. |
| `v3-timeout-drops-red` | Drop the entry for every failure. | `./internal/dash/`, `TestCacheOrdinaryFailureKeepsThePreviousEntryAndItsFetchTime` | exit 1. The expected entry is absent. |
| `v3-expiry-keeps-red` | Remove the token-expiry deletion. | `./internal/dash/`, `TestCacheTokenExpiryDropsTheEntry\|TestViewShowsTheCachedResponseUntilATokenExpiry` | exit 1. Both tests fail. |
| `v3-time-changes-red` | Change the cached timestamp on each failure. | `./internal/dash/`, `TestCacheOrdinaryFailureKeepsThePreviousEntryAndItsFetchTime` | exit 1. Entry reports 14:24:09 instead of 14:22:07. |
| `v3-model-disconnected-red` | Remove the model's cache update. | `./internal/dash/`, `TestViewShowsTheCachedResponseUntilATokenExpiry` | exit 1. Cached line and percentage are absent. |
| `v4-v6-no-cache-red` | Make `cachedFor` return no entry. | `./internal/dash/layout/`, the cached-card, compact-order, compact-fit, and Ascii-state tests | exit 1. All four tests fail. |
| `v5-need-ignores-cache-red` | Set the cache to nil inside `compactNeed`. | `./internal/dash/layout/`, `TestScreenCompactNeedCountsCachedRows` | exit 1. Reported need does not include cached rows. |
| `v6-layout-not-faint-red` | Use the ordinary card and a non-dim compact bar. | `./internal/dash/layout/`, `TestScreenCachedBarsRenderFaint` | exit 1. Both layout subtests fail. |
| `v6-card-not-faint-red` | Disable dim styling in `CachedProviderCard`. | `./internal/display/`, `TestCachedProviderCardDimsBarsAndKeepsTheText` | exit 1. Faint bar and percentage are absent. |
| `v6-compact-not-faint-red` | Force the compact dim flag to false. | `./internal/display/`, `TestCompactBarDimRendersBarPercentAndCountdownFaint` | exit 1. Faint bar, percentage, and countdown are absent. |
| `v9-ignore-quit-red` | Return without handling any key message. | `./internal/dash/`, `TestRun_QuittingCancelsTheFetchInFlight\|TestRun_QuitsOnQ` | exit 1. Each blocked-fetch case exceeds 1s. The ordinary `q` case reaches its 10s deadline. |

I then ran these commands without overlays. Each command exited 0:

```sh
go test ./internal/usage/ -run 'ErrorKind|ZeroKind|TokenExpiredErrorKeeps'
go test -v ./internal/usage/ -run 'ErrorKind|ZeroKind|TokenExpiredErrorKeeps|RenderJSON'
go test -count=1 -v ./internal/usage/ -run 'RenderText|RenderJSON|ColoredCards'
go test -count=1 -v ./internal/dash/ -run 'TestCache|TestViewMatchesScreenAtEveryResize|TestViewShowsTheCachedResponseUntilATokenExpiry'
go test -count=1 -v ./internal/dash/layout/ -run 'TestScreenGoldenFrames|TestScreenCompact|TestScreenCached|TestScreenFailureWithoutCache'
go test -count=1 -v ./internal/display/ -run 'TestCachedProviderCard|TestCompactBarDim'
go test -v ./internal/dash/ -run 'TestRun_QuittingCancelsTheFetchInFlight|TestRun_QuitsOnQ'
go test -count=1 -v ./internal/dash/ -run 'TestRun_QuittingCancelsTheFetchInFlight|TestRun_QuitsOnQ'
go test -count=1 -v ./internal/provider/codex/ -run '^TestFetch_EmptyRateLimitObjectIsNotAnError$'
```

The uncached run of the quit tests completed in 0.243s.
The blocked-fetch subtests completed in 0.11s, 0.06s, and 0.06s.

For V2, I also ran the wire-form test with `dev`'s `usage.go` and without the
Kind-dependent tests. It passed with the same expected bytes:

```json
{"windows":[],"errors":[{"provider":"codex","message":"token expired, open codex to refresh"}],"undetected":[]}
```

## V4-V6 text design checks

The brief's Design section explicitly excludes screenshots and canvas boards.
The approved plan is
`/home/penguin/source/hive/plans/qmeter-dash-clock-cache.md`.
I checked the exact text requirements. I did not compare older boards.

| Board element | state without colour | Result | Cause or decision |
| --- | --- | --- | --- |
| Board capture | Exact strings and rendered text | overridden | The approved Design section requires text tests and prohibits screen captures for this task. |
| Cached status line | `◌ cached · fetched 14:20:05 · timed out after 10s` | matches | No `error: ` prefix. Timestamp includes seconds. |
| Ordinary error | `error: offline` | matches | Gemini has no cache entry. |
| Cached window | `weekly`, `28.0%`, `resets in 4d19h`, and a drawn bar | matches | Cached Codex data remains visible. ANSI tests separately check faint styling. |
| R1 resets panel | Fresh provider names only | matches | No Codex window appears in the panel. |
| Balances panel | `cursor included  0 / 0` | matches | No cached Codex balance appears. |
| Compact provider slot | `codex weekly` between Claude and OpenCode Go | matches | Provider order is preserved. |
| Compact message | `codex ◌ cached · fetched 14:20:05 · timed out after 10s` | matches | The message follows all window rows. |
| Compact fit | `need 126x33` | matches | All 30 cached windows and the full cached message fit at the reported size. |
| Cache after an empty success | Old `28.0%` and `fetched 14:22:07` reappear | differs | V3 failure described above. |

I captured the real full and compact renderer output with a temporary test overlay
that called `layout.Screen` on the existing fixtures. The full 140x45 output also
matched `screen-cached-140x45.txt` line by line.

The compact 80x24 output included these rows in this order:

```text
claude extra-5      ━━━━━━━━━━━━━━━━━━━━━━━━━━────────────────────────── 50.0% -
codex weekly        ━━━━━━━━━━━━━─────────────────────────────────── 28.0% 4d19h
opencode-go 5h      ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 100.0% 4h59m
opencode-go weekly  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━── 95.0% 5d22h
opencode-go monthly ━━━━━━━━━━━━━━━━━━━━━━━━━━─────────────────────── 54.0% 9d1h
cursor total        ━━━━━━━━━━━━━━━━━━━━━━━━──────────────────────── 49.5% 3d12h
cursor auto         ───────────────────────────────────────────────── 0.0% 3d12h
codex ◌ cached · fetched 14:20:05 · timed out after 10s
```

### V6 stripped Ascii frame, 140x45

This frame is from my fresh run of
`TestScreenCachedLineAndErrorLineDifferInPlainText`.
The test checks that the original frame contains no ANSI escape codes.

```text
qmeter 5 providers · 8 windows · 1 balance                                                                                     fetched 21:23
╭─ ◆ claude ────────────────────────────────────────────────── max ─╮  ╭─ ● codex ─────────────────────────────────────────────── prolite ─╮
│ ▸ 5h                                        81.0% · resets in 16m │  │ ▸ weekly                                  28.0% · resets in 4d19h │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━──────────── │  │ ━━━━━━━━━━━━━━━━━━─────────────────────────────────────────────── │
│                                                                   │  │ ◌ cached · fetched 14:20:05 · timed out after 10s                 │
│ ▸ weekly                                   91.0% · resets in 6d8h │  │                                                                   │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━────── │  │                                                                   │
│                                                                   │  │                                                                   │
│ ▸ fable weekly                             96.0% · resets in 6d8h │  │                                                                   │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━─── │  │                                                                   │
│                                                                   │  │                                                                   │
│                                                                   │  │                                                                   │
│                                                                   │  │                                                                   │
╰───────────────────────────────────────────────────────────────────╯  ╰───────────────────────────────────────────────────────────────────╯
╭─ ○ opencode-go ────────────────────────────────────────────── go ─╮  ╭─ ▲ cursor ───────────────────────────────────────────────── free ─╮
│ ▸ 5h                                     100.0% · resets in 4h59m │  │ ▸ total                                   49.5% · resets in 3d12h │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ │  │ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━───────────────────────────────── │
│                                                                   │  │                                                                   │
│ ▸ weekly                                  95.0% · resets in 5d22h │  │ ▸ auto                      0.0% · resets in 3d12h · rate limited │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━─── │  │ ───────────────────────────────────────────────────────────────── │
│                                                                   │  │                                                                   │
│ ▸ monthly                                  54.0% · resets in 9d1h │  │                                                                   │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━────────────────────────────── │  │                                                                   │
│                                                                   │  │                                                                   │
│                                                                   │  │                                                                   │
│                                                                   │  │                                                                   │
╰───────────────────────────────────────────────────────────────────╯  ╰───────────────────────────────────────────────────────────────────╯
╭─ • gemini ─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────── - ─╮
│ error: offline                                                                                                                           │
│                                                                                                                                          │
│                                                                                                                                          │
│                                                                                                                                          │
╰──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────────────────────────────────────────╮  ╭────────────────────────────────────────────╮
│ resets                                                                                   │  │ balances                                   │
│    16m  claude       5h            Wed 16 14:38   now 81.0%                              │  │ cursor included  0 / 0                     │
│  4h59m  opencode-go  5h            Wed 16 19:21   now 100.0%                             │  │                                            │
│  3d12h  cursor       total         Sun 20 02:22   now 49.5%                              │  │                                            │
│  3d12h  cursor       auto          Sun 20 02:22   now 0.0% rate limited                  │  │                                            │
│  5d22h  opencode-go  weekly        Tue 22 12:22   now 95.0%                              │  │                                            │
│   6d8h  claude       weekly        Tue 22 22:22   now 91.0%                              │  │                                            │
│   6d8h  claude       fable weekly  Tue 22 22:22   now 96.0%                              │  │                                            │
│   9d1h  opencode-go  monthly       Fri 25 15:22   now 54.0%                              │  │                                            │
╰──────────────────────────────────────────────────────────────────────────────────────────╯  ╰────────────────────────────────────────────╯
r refresh · q quit
```

## V7 file scope

`git diff dev --stat` reported 25 files, 1418 insertions, and 41 deletions
before this report. All changes are permitted by the source table.
The only new golden is `internal/dash/layout/testdata/screen-cached-140x45.txt`.
This report adds one task evidence file.

Both commands below printed nothing:

```sh
git diff dev -- internal/usage/render.go internal/spend internal/resets cmd
git diff dev -- internal/dash/layout/testdata ':!internal/dash/layout/testdata/screen-cached-140x45.txt'
```

The additional prohibited paths also have an empty diff:

```sh
git diff dev --numstat -- internal/usage/render.go internal/spend internal/resets cmd internal/provider internal/dash/run.go
```

## V8 gate

Environment: Go `go1.27.2-X:nodwarf5`, golangci-lint `2.15.0`.
All task gate commands ran from the worktree root.

| Command | Output | Exit |
| --- | --- | --- |
| `gofmt -l .` | No output. | 0 |
| `go mod tidy -diff` | No output. | 0 |
| `go vet ./...` | No output. | 0 |
| `go test -race ./...` | All tested packages pass. | 0 |
| `go test -race -count=1 ./...` | All tested packages pass in a fresh run. | 0 |
| `bash .github/scripts/next-version_test.sh` | All 10 cases pass. `all tests passed`. | 0 |
| `golangci-lint run --new-from-rev=dev ./...` | `0 issues.` | 0 |
| `golangci-lint run ./...` | 26 existing findings. Details below. | 1 |

Fresh race-test output:

```text
?    github.com/Harrison-Blair/qmeter [no test files]
ok   github.com/Harrison-Blair/qmeter/cmd 1.012s
ok   github.com/Harrison-Blair/qmeter/cmd/dash 1.013s
ok   github.com/Harrison-Blair/qmeter/cmd/resets 1.008s
ok   github.com/Harrison-Blair/qmeter/cmd/spend 1.016s
ok   github.com/Harrison-Blair/qmeter/cmd/update 1.028s
ok   github.com/Harrison-Blair/qmeter/cmd/usage 1.019s
ok   github.com/Harrison-Blair/qmeter/cmd/version 1.007s
ok   github.com/Harrison-Blair/qmeter/internal/dash 1.461s
ok   github.com/Harrison-Blair/qmeter/internal/dash/config 1.024s
ok   github.com/Harrison-Blair/qmeter/internal/dash/layout 12.736s
ok   github.com/Harrison-Blair/qmeter/internal/dash/theme 1.022s
ok   github.com/Harrison-Blair/qmeter/internal/display 1.039s
ok   github.com/Harrison-Blair/qmeter/internal/lib/credstore 1.021s
ok   github.com/Harrison-Blair/qmeter/internal/lib/httpx 1.076s
ok   github.com/Harrison-Blair/qmeter/internal/lib/piauth 1.014s
ok   github.com/Harrison-Blair/qmeter/internal/lib/subprocess 1.055s
ok   github.com/Harrison-Blair/qmeter/internal/provider 1.005s
ok   github.com/Harrison-Blair/qmeter/internal/provider/claude 1.059s
ok   github.com/Harrison-Blair/qmeter/internal/provider/codex 1.082s
ok   github.com/Harrison-Blair/qmeter/internal/provider/cursor 1.040s
ok   github.com/Harrison-Blair/qmeter/internal/provider/opencodego 1.123s
ok   github.com/Harrison-Blair/qmeter/internal/provider/providertest 1.036s
ok   github.com/Harrison-Blair/qmeter/internal/resets 1.011s
ok   github.com/Harrison-Blair/qmeter/internal/spend 1.011s
ok   github.com/Harrison-Blair/qmeter/internal/update 1.073s
ok   github.com/Harrison-Blair/qmeter/internal/usage 1.441s
ok   github.com/Harrison-Blair/qmeter/internal/version 1.004s
```

### Known full-lint findings

I independently ran full lint on an archive of `dev` in a temporary directory.
No Git worktree or task source changed for this comparison.

| Linter | `dev` | Branch |
| --- | --- | --- |
| errcheck | 21 | 21 |
| ineffassign | 2 | 2 |
| staticcheck | 4 | 3 |
| Total | 27 | 26 |

The brief's baseline count of 27 is correct for `dev`.
The branch has 26 because `ProviderError` now has three fields.
It can no longer convert directly to the two-field `jsonError`.
Thus S1016 at `internal/usage/render.go:206:35` no longer applies.
`render.go` is byte-identical to `dev`. No new finding appears.
The 27-to-26 reduction does not block the gate.

My full branch lint printed these 26 findings:

```text
cmd/resets/resets.go:42:17: unchecked fmt.Fprintln (errcheck)
cmd/usage/usage.go:56:17: unchecked fmt.Fprintln (errcheck)
internal/display/display_test.go:60:18: unchecked read.Close (errcheck)
internal/display/display_test.go:61:19: unchecked write.Close (errcheck)
internal/lib/httpx/httpx.go:97:23: unchecked resp.Body.Close (errcheck)
internal/provider/codex/pi_test.go:41:18: unchecked os.WriteFile (errcheck)
internal/provider/codex/pi_test.go:43:18: unchecked os.WriteFile (errcheck)
internal/provider/codex/pi_test.go:62:13: unchecked w.Write (errcheck)
internal/provider/opencodego/db.go:79:16: unchecked db.Close (errcheck)
internal/provider/opencodego/db_test.go:285:16: unchecked db.Close (errcheck)
internal/update/asset.go:72:16: unchecked gz.Close (errcheck)
internal/update/asset.go:127:17: unchecked rc.Close (errcheck)
internal/update/hint.go:138:14: unchecked fmt.Fprintln (errcheck)
internal/update/release.go:87:23: unchecked resp.Body.Close (errcheck)
internal/update/release.go:147:23: unchecked resp.Body.Close (errcheck)
internal/update/replace.go:49:12: unchecked tmp.Close (errcheck)
internal/update/run.go:151:14: unchecked fmt.Fprintf (errcheck)
internal/update/run.go:234:13: unchecked fmt.Fprintf (errcheck)
internal/usage/pi_test.go:52:16: unchecked fmt.Fprint (errcheck)
internal/usage/pi_test.go:54:16: unchecked fmt.Fprint (errcheck)
internal/usage/pi_test.go:56:16: unchecked fmt.Fprint (errcheck)
internal/lib/piauth/piauth.go:138:3: ineffectual assignment to end (ineffassign)
internal/provider/claude/pi_test.go:138:2: ineffectual assignment to p (ineffassign)
internal/lib/piauth/piauth.go:63:24: ST1005 error strings should not be capitalized (staticcheck)
internal/lib/piauth/piauth.go:67:24: ST1005 error strings should not be capitalized (staticcheck)
internal/lib/piauth/piauth.go:77:24: ST1005 error strings should not be capitalized (staticcheck)
26 issues: errcheck 21, ineffassign 2, staticcheck 3
```

The base prints the same linter totals except for the additional S1016 finding:

```text
internal/usage/render.go:206:35: S1016: should convert e (type ProviderError) to jsonError instead of using struct literal (staticcheck)
27 issues: errcheck 21, ineffassign 2, staticcheck 4
```

The same-message cap changes which existing errcheck locations appear in each run.
In my base run, `cmd/spend/spend.go:40:17` replaces the branch's
`cmd/usage/usage.go:56:17`, and `internal/provider/claude/pi_test.go:74:16`
replaces `internal/usage/pi_test.go:56:16` in the printed samples.
Those files did not change. No baseline finding was fixed as part of verification.

## Limits and cleanup

This verification uses model tests, real layout rendering, and the required
in-memory TUI run tests. It does not contact live subscription services.
The approved Design section excludes terminal screenshots.

Temporary overlays, scripts, logs, and the base archive are removed after their
results are recorded here. Only this report is a verifier deliverable.
