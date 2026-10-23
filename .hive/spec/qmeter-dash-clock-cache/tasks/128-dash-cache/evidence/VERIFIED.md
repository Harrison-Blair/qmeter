# Independent verification: 128-dash-cache

- Verifier: `t128-verifier-2`.
- Date: 2026-10-23 UTC.
- Checked commit: `f372c6eb76848932c4eee0a86d5d595315e8569a`.
- Base `dev`: `3d2c2574d03ac40c68ad8b6cbfed55e233e7e2f7`.
- Branch: `task/128-dash-cache`.
- Verdict: **pass** for V1 to V9.

I read the amended brief, the feature spec, the approved plan, and `AGENTS.md`.
I ran each check independently from the worktree root.
I changed no source file, worker evidence file, or tracker.
The previous failed report remains in Git at `2f7ed93`.
This report replaces that verdict after the empty-success fix.

## Results

| Item | Result | Observation |
| --- | --- | --- |
| V1 | pass | The base has no `Kind`, so the new tests fail to compile. A zero-kind mutation fails for wrapped token expiry. The branch passes token, timeout, cancellation, rate-limit, and undetected cases. |
| V2 | pass | The exact token-expiry JSON passes on the base and branch. Adding `kind` to the envelope fails the test. Existing renderer tests pass. The required source diff is empty. |
| V3 | pass | Success replaces the entry and timestamp. Empty success removes it. Timeout preserves it. Token expiry removes it. First failure has no entry. Model tests cover the amended provider-set requirement. Independent checks also cover balances and restart. |
| V4 | pass | At 140x45, the Codex card retains its window and exact cached line. The resets and balances panels show fresh data only. The cached golden and all ten existing screen goldens pass. |
| V5 | pass | At 80x24, cached Codex windows occupy the Codex slot. Messages follow the window rows. The fit test reports `need 126x33`, fits all rows there, and rejects 126x32. |
| V6 | pass | The Ascii frame contains no ANSI escapes. `◌ cached` and `error: offline` distinguish the states by text. The cached card retains its bar. Separate ANSI tests check faint styling. |
| V7 | pass | The diff contains only permitted source paths, the permitted new cached golden, and task records. Prohibited source paths and existing goldens have no changes. |
| V8 | pass | Format, module tidy, vet, race tests, release tests, and new-issue lint pass. Full lint reports 26 findings on the branch and 27 on `dev`. An uncapped comparison finds no new findings. Details follow. |
| V9 | pass | Both required run tests pass. During blocked work, `q`, `ctrl+c`, and combined `rq` each exit in 0.11s in the fresh race run. Ignoring keys makes both tests fail. |

## V1 and V2: typed errors and unchanged one-shot output

I ran the documented command and a fresh verbose run:

```sh
go test ./internal/usage/ -run 'ErrorKind|ZeroKind|TokenExpiredErrorKeeps'
go test -count=1 -v ./internal/usage/ -run 'ErrorKind|ZeroKind|TokenExpiredErrorKeeps|RenderJSON'
go test -count=1 -v ./internal/usage/ -run 'RenderText|RenderJSON|ColoredCards'
git diff dev -- internal/usage/render.go internal/spend internal/resets cmd
```

All test commands exit 0. The diff prints nothing.
The fresh kind test passes all four cases:

```text
--- PASS: TestRun_ErrorKindMarksOnlyAnExpiredToken/token_expired_wrapped
--- PASS: TestRun_ErrorKindMarksOnlyAnExpiredToken/timeout
--- PASS: TestRun_ErrorKindMarksOnlyAnExpiredToken/cancellation
--- PASS: TestRun_ErrorKindMarksOnlyAnExpiredToken/rate_limit
--- PASS: TestRun_UndetectedKeepsTheZeroKind
--- PASS: TestRenderJSON_TokenExpiredErrorKeepsItsWireForm
```

The expected JSON bytes are identical on the base and branch:

```json
{"windows":[],"errors":[{"provider":"codex","message":"token expired, open codex to refresh"}],"undetected":[]}
```

For the base comparison, an overlay supplies `dev`'s `usage.go`.
The overlay retains the wire-form test and omits only the two tests that require the new `Kind` API.
The checked-out tests do not change.
The existing renderer tests also run unchanged.

## V3: cache rules and the previous failure

Command:

```sh
go test -count=1 -v ./internal/dash/ -run 'TestCache|TestViewMatchesScreenAtEveryResize|TestViewShowsTheCachedResponseUntilATokenExpiry|TestViewDropsTheCacheAfterAnEmptySuccess'
```

Output, exit 0:

```text
--- PASS: TestCacheSuccessReplacesTheEntry
--- PASS: TestCacheEmptySuccessRemovesTheEntry
--- PASS: TestCacheOrdinaryFailureKeepsThePreviousEntryAndItsFetchTime
--- PASS: TestCacheTokenExpiryDropsTheEntry
--- PASS: TestCacheFirstFailureHasNoEntry
--- PASS: TestCacheUpdateLeavesTheEarlierCacheUnchanged
--- PASS: TestViewMatchesScreenAtEveryResize
--- PASS: TestViewShowsTheCachedResponseUntilATokenExpiry
--- PASS: TestViewDropsTheCacheAfterAnEmptySuccess
ok github.com/Harrison-Blair/qmeter/internal/dash 0.004s
```

The prior failure sequence now passes:

1. Codex succeeds at 14:22:07 with 28.0% remaining.
2. Codex succeeds at 14:23:08 with no windows or balances.
3. Codex fails at 14:24:09.

The last frame shows only the ordinary Codex error line.
It contains no `◌ cached`, `28.0%`, or old timestamp.
Removing the `asked` deletion loop makes both new regression tests fail.
Passing `nil` instead of `m.providerIDs()` makes the model regression test fail.
These red runs check both the cache rule and the model wiring.

### Additional independent checks

Temporary test overlays add two tests to `internal/dash/model_test.go`.
They call the real `m.fetch()`, `usage.Run`, `Update`, and `View`.
Only external providers use `providertest` fakes.

`TestVerifierRealFetchCacheLifecycle` checks this sequence:

- Codex returns 28.0% and Claude returns 81.0%.
- Codex times out while Claude returns 65.0%. The card retains Codex's 28.0% and first timestamp.
- The fresh `m.res.Windows` contains Claude only. It contains no cached Codex window.
- Codex returns `provider.ErrTokenExpired`. Its card shows the ordinary token-expiry error and no cached data.
- A later ordinary failure cannot restore the discarded response.
- Codex recovers with 17.0%. An empty success follows. A later timeout cannot restore 17.0%.

`TestVerifierBalancesOnlyCacheAndRestart` checks these independent expectations:

- A balance-only success returns 12.5 credits.
- A second success returns 4.25 credits at 14:23:08.
- A later timeout shows 4.25 and `◌ cached · fetched 14:23:08 · timed out after 10s`.
- The frame contains neither 12.5 nor the old timestamp.
- The fresh result and balances panel exclude cached balances.
- A new model has no cached balance when the first request fails.
- An empty success removes the balance-only entry before a later timeout.

Both tests pass. The final balance test also passes with `-race`.
Each independent test fails under a relevant broken implementation, then passes without that mutation.
The lifecycle test fails when empty success retains the entry.
The balance test fails when success retains the old balance or empty success retains the entry.
Disabling layout cache lookup also makes the original balance test fail.

Excerpt from the real model after the final empty-success sequence:

```text
qmeter 2 providers · 1 window · 0 balances                                                                                     fetched 14:24
╭─ ◆ claude ──────────────────────────────────────────────────── - ─╮  ╭─ ● codex ───────────────────────────────────────────────────── - ─╮
│ ▸ 5h                                             65.0% · resets - │  │ error: timed out after 10s                                        │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━─────────────────────── │  │                                                                   │
```

## Red runs and green runs

All mutations use Go overlays under the hive home's `.hive/tmp/t128-verifier-2/`.
No mutation changes a checked-out file.
Each red run uses this command form:

```sh
go test -overlay="$S/NAME.json" -count=1 -v PACKAGE -run PATTERN
```

`$S` denotes that scratch directory. The final balance checks also use `-race`.
Every red command below exits 1 for the stated reason.

| Overlay | Implementation change | Test or pattern | Observed failure |
| --- | --- | --- | --- |
| `v1-base-red` | Use `dev`'s `usage.go`. | `./internal/usage/`, `ErrorKind\|ZeroKind\|TokenExpiredErrorKeeps` | Undefined `ErrorKind`, `KindTokenExpired`, and `.Kind`. |
| `v1-zero-kind-red` | Return the zero kind for token expiry. | `./internal/usage/`, `ErrorKind\|ZeroKind` | Wrapped token: `Kind = "", want "token-expired"`. |
| `v2-json-leak-red` | Add `kind` to `jsonError` and populate it. | `./internal/usage/`, `TokenExpiredErrorKeeps` | Exact JSON contains an unexpected key. |
| `v3-empty-success-red` | Remove the deletion loop over `asked`. | Cache and model empty-success tests | Both restore obsolete data after empty success. |
| `v3-provider-set-red` | Pass `nil` instead of `m.providerIDs()`. | Model empty-success test | Old 28.0%, cached line, and timestamp reappear. |
| `v3-no-replace-red` | Keep an old entry instead of replacing it on success. | `TestCacheSuccessReplacesTheEntry` | Entry keeps 40% and 14:22:07 instead of 30% and 14:23:08. |
| `v3-timeout-drops-red` | Delete the entry for every failure. | Ordinary-failure cache test | The expected entry is absent. |
| `v3-expiry-keeps-red` | Remove the token-expiry deletion. | Token-expiry cache and model tests | Expired-token failure still shows cached data. |
| `v3-retime-red` | Set the entry timestamp to each failed request's time. | Ordinary-failure cache test | The timestamp becomes 14:24:09 instead of 14:22:07. |
| `v4-v6-no-cache-red` | Make `cachedFor` report no entry. | Cached-card, compact-order, compact-fit, and plain-text tests | All four tests fail. |
| `v5-need-ignores-cache-red` | Set `cached = nil` inside `compactNeed`. | Compact-fit test | Reports `need 103x24` instead of `need 126x33`. |
| `v6-layout-not-faint-red` | Use ordinary cards and non-dim compact bars. | `TestScreenCachedBarsRenderFaint` | Both cards and compact percentages lack faint styling. |
| `v6-card-not-faint-red` | Disable dim styling in `CachedProviderCard`. | Cached-card display test | The faint percentage and bar are absent. |
| `v6-compact-not-faint-red` | Force the compact dim flag to false. | Compact-bar dim test | The faint bar, percentage, and countdown are absent. |
| `v9-ignore-quit-red` | Return without handling key messages. | Both required run tests | Blocked-work cases exceed 1s. The ordinary `q` case reaches its 10s deadline. |
| `independent-lifecycle-red` | Remove the deletion loop over `asked`. | Independent lifecycle test | The final failure restores the obsolete 17.0% response. |
| `independent-balances-red` | Disable layout cache lookup. | Independent balance test | The cached balance and line are absent. |
| `balance-replacement-red` | Retain an old entry after success. | Extended independent balance test | Shows 12.5 and 14:22:07 instead of 4.25 and 14:23:08. |
| `balance-empty-success-red` | Remove the deletion loop over `asked`. | Extended independent balance test | The final failure restores the obsolete 4.25 balance. |

After the relevant red runs, these unmodified-branch commands all exit 0:

```sh
go test -count=1 -v ./internal/dash/layout/ -run 'TestScreenGoldenFrames|TestScreenCompact|TestScreenCached|TestScreenFailureWithoutCache'
go test -count=1 -v ./internal/display/ -run 'TestCachedProviderCard|TestCompactBarDim'
go test -v ./internal/dash/ -run 'TestRun_QuittingCancelsTheFetchInFlight|TestRun_QuitsOnQ'
go test -race -count=1 -v ./internal/dash/ -run 'TestRun_QuittingCancelsTheFetchInFlight|TestRun_QuitsOnQ'
```

The V1 to V3 green commands appear above.
The independent lifecycle tests pass again with their test overlays and no implementation mutation.

## V4 to V6: text design checks

The brief's Design section explicitly excludes screenshots and canvas boards.
The design of record is `/home/penguin/source/hive/plans/qmeter-dash-clock-cache.md` and the brief's exact strings.
I did not compare the older layout boards.
I rendered the task fixtures again through `layout.Screen` and checked their text.

| Board element | state without colour | Result | Cause or decision |
| --- | --- | --- | --- |
| Board capture | Exact rendered text | overridden | The approved Design section requires text checks and no screen capture for this task. |
| Cached line | `◌ cached · fetched 14:20:05 · timed out after 10s` | matches | Seconds appear. The reason is unchanged. There is no `error: ` prefix. |
| Ordinary error | `error: offline` | matches | Gemini has no cache entry. |
| Cached card | `weekly`, `28.0%`, `resets in 4d19h`, and a drawn bar | matches | The Codex window remains visible. ANSI checks also show faint styling. |
| Resets panel | Fresh provider names only | matches | No cached Codex window appears. |
| Balances panel | `cursor included  0 / 0` | matches | No cached Codex balance appears. |
| Compact provider slot | `codex weekly` between Claude and OpenCode Go | matches | The compact fixture retains provider order. |
| Compact message | `codex ◌ cached · fetched 14:20:05 · timed out after 10s` | matches | The message follows the window rows. |
| Compact fit | `need 126x33` | matches | All 30 cached windows and the complete message fit. One fewer row is too small. |
| Empty-success result | `error: timed out after 10s` | matches | Old windows and balances remain absent after a later failure. |

### V4 full frame

The fresh 140x45 render matches `screen-cached-140x45.txt` line by line.
Its cached card contains:

```text
╭─ ● codex ─────────────────────────────────────────────── prolite ─╮
│ ▸ weekly                                  28.0% · resets in 4d19h │
│ ━━━━━━━━━━━━━━━━━━─────────────────────────────────────────────── │
│ ◌ cached · fetched 14:20:05 · timed out after 10s                 │
```

The resets panel lists Claude, OpenCode Go, and Cursor only.
The balances panel lists `cursor included  0 / 0` only.
The header continues to count fresh windows and balances.
This task makes no header-clock change.

### V5 compact frame excerpt

The fresh 80x24 render contains these rows in this order:

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

This frame comes from the fresh `TestScreenCachedLineAndErrorLineDifferInPlainText` run.
The test also checks that the original frame contains no ANSI escape codes.
The text and glyph distinguish cached data from the ordinary error without colour.

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

## V7: file scope

Before this report, `git diff dev --stat` reports 26 files, 2058 insertions, and 41 deletions.
All changed source paths appear in the brief's source table.
The additional paths are the task brief, worker evidence, this report, and the permitted new cached golden.
No existing golden changes.

These commands print nothing and exit 0:

```sh
git diff dev -- internal/usage/render.go internal/spend internal/resets cmd
git diff dev -- internal/provider internal/dash/run.go
git diff dev -- internal/dash/layout/testdata ':!internal/dash/layout/testdata/screen-cached-140x45.txt'
git diff --check
```

The checkout is clean before I write this report.
There is no `VERIFIED.md` at the checkout root.

## V8: gate

Environment: Go `go1.27.2-X:nodwarf5`, golangci-lint `2.15.0`.
All task gate commands run from the worktree root.

| Command | Output | Exit |
| --- | --- | --- |
| `gofmt -l .` | No output. | 0 |
| `go mod tidy -diff` | No output. | 0 |
| `go vet ./...` | No output. | 0 |
| `go test -race ./...` | All packages pass. | 0 |
| `go test -race -count=1 ./...` | All packages pass in a fresh run. | 0 |
| `bash .github/scripts/next-version_test.sh` | Ten cases pass. `all tests passed`. | 0 |
| `golangci-lint run --new-from-rev=dev ./...` | `0 issues.` | 0 |
| `golangci-lint run ./...` | 26 existing findings. | 1 |

Fresh race-test output:

```text
?    github.com/Harrison-Blair/qmeter [no test files]
ok   github.com/Harrison-Blair/qmeter/cmd 1.011s
ok   github.com/Harrison-Blair/qmeter/cmd/dash 1.012s
ok   github.com/Harrison-Blair/qmeter/cmd/resets 1.009s
ok   github.com/Harrison-Blair/qmeter/cmd/spend 1.020s
ok   github.com/Harrison-Blair/qmeter/cmd/update 1.014s
ok   github.com/Harrison-Blair/qmeter/cmd/usage 1.023s
ok   github.com/Harrison-Blair/qmeter/cmd/version 1.012s
ok   github.com/Harrison-Blair/qmeter/internal/dash 1.434s
ok   github.com/Harrison-Blair/qmeter/internal/dash/config 1.020s
ok   github.com/Harrison-Blair/qmeter/internal/dash/layout 12.905s
ok   github.com/Harrison-Blair/qmeter/internal/dash/theme 1.021s
ok   github.com/Harrison-Blair/qmeter/internal/display 1.034s
ok   github.com/Harrison-Blair/qmeter/internal/lib/credstore 1.011s
ok   github.com/Harrison-Blair/qmeter/internal/lib/httpx 1.064s
ok   github.com/Harrison-Blair/qmeter/internal/lib/piauth 1.018s
ok   github.com/Harrison-Blair/qmeter/internal/lib/subprocess 1.055s
ok   github.com/Harrison-Blair/qmeter/internal/provider 1.005s
ok   github.com/Harrison-Blair/qmeter/internal/provider/claude 1.065s
ok   github.com/Harrison-Blair/qmeter/internal/provider/codex 1.091s
ok   github.com/Harrison-Blair/qmeter/internal/provider/cursor 1.045s
ok   github.com/Harrison-Blair/qmeter/internal/provider/opencodego 1.136s
ok   github.com/Harrison-Blair/qmeter/internal/provider/providertest 1.038s
ok   github.com/Harrison-Blair/qmeter/internal/resets 1.013s
ok   github.com/Harrison-Blair/qmeter/internal/spend 1.015s
ok   github.com/Harrison-Blair/qmeter/internal/update 1.080s
ok   github.com/Harrison-Blair/qmeter/internal/usage 1.446s
ok   github.com/Harrison-Blair/qmeter/internal/version 1.004s
```

### Baseline lint findings

I ran full lint independently on a `git archive dev` copy in the scratch directory.
The copy has no Git worktree registration and makes no task source changes.

| Linter | `dev` | Branch |
| --- | --- | --- |
| errcheck | 21 | 21 |
| ineffassign | 2 | 2 |
| staticcheck | 4 | 3 |
| Total | 27 | 26 |

The brief's 27 findings refer to the base.
The extra `Kind` field prevents conversion from `ProviderError` to the two-field `jsonError`.
Thus the old S1016 finding no longer applies. `render.go` remains byte-identical to `dev`.
This count reduction adds no finding and does not block the gate.

These are the 27 findings from my default base run.
The default branch run prints the same first 26 findings and omits S1016:

```text
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
internal/update/run.go:176:16: unchecked fmt.Fprintln (errcheck)
internal/update/run.go:226:15: unchecked fmt.Fprintln (errcheck)
internal/update/run.go:234:13: unchecked fmt.Fprintf (errcheck)
internal/usage/pi_test.go:52:16: unchecked fmt.Fprint (errcheck)
internal/usage/pi_test.go:54:16: unchecked fmt.Fprint (errcheck)
internal/usage/pi_test.go:56:16: unchecked fmt.Fprint (errcheck)
internal/lib/piauth/piauth.go:138:3: ineffectual assignment to end (ineffassign)
internal/provider/claude/pi_test.go:138:2: ineffectual assignment to p (ineffassign)
internal/lib/piauth/piauth.go:63:24: ST1005 error strings should not be capitalized (staticcheck)
internal/lib/piauth/piauth.go:67:24: ST1005 error strings should not be capitalized (staticcheck)
internal/lib/piauth/piauth.go:77:24: ST1005 error strings should not be capitalized (staticcheck)
internal/usage/render.go:206:35: S1016 should convert ProviderError to jsonError (staticcheck)
```

To exclude any effect from diagnostic limits, I also ran this command on both copies:

```sh
golangci-lint run --max-same-issues=0 --max-issues-per-linter=0 ./...
```

The uncapped totals are 35 on `dev` and 34 on the branch.
Comparing complete diagnostic lines gives zero additions and one removal: the S1016 finding above.
No lint finding was fixed during verification.

## V9: quit during slow work

The required tests pass again without cached results:

```text
--- PASS: TestRun_QuittingCancelsTheFetchInFlight (0.33s)
    --- PASS: TestRun_QuittingCancelsTheFetchInFlight/q (0.11s)
    --- PASS: TestRun_QuittingCancelsTheFetchInFlight/ctrl+c (0.11s)
    --- PASS: TestRun_QuittingCancelsTheFetchInFlight/combined_r_and_q (0.11s)
--- PASS: TestRun_QuitsOnQ (0.01s)
ok github.com/Harrison-Blair/qmeter/internal/dash 1.349s
```

The tests check prompt exit and cancellation of the blocked provider request.
The key-ignoring mutation fails all three blocked-work cases and the ordinary `q` case.

## Limits and cleanup

This verification uses real model and layout code with synthetic external provider responses.
It does not contact live subscription services.
The approved design excludes terminal screenshots.

The temporary overlays, scripts, logs, and base archive are removed after recording these results.
The only verifier deliverable is this report in the task evidence folder.
