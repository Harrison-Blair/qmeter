# Code review — `dev` vs `origin/main`

Effort level: high. Reviewed `origin/main..HEAD` (8 commits, 67 files, 4,620 insertions):
`--vertical`, the run-out forecast, the reset timeline (`qmeter resets` and the `t` key),
`qmeter spend` plus the dashboard balance ledger, `--fit`, and the
`Provider.Fetch` → `provider.Usage` refactor.

> Local `main` was stale at the time of review, so `git diff main...HEAD` also pulled in
> ~3,000 lines already merged through PR #8. The config and theme packages, `cmd/pace`, and
> the CI `@v7` bumps were already on `main` and are out of scope.

## Verification performed

- `gofmt -l .` clean, `go vet ./...` clean, `go test -race ./...` all pass.
- Throwaway probes (since removed) swept `layout.Render` and `layout.RenderTimeline` across
  widths 1–300 × `{vertical, fit, banner}` × 5 body heights, and `resets.Rows` / `resets.FitRows`
  across widths 60–400 with past, far-future, exactly-7d, rate-limited and zero-reset windows
  plus empty balance names. No panics, no negative `strings.Repeat` counts, and every rendered
  line was exactly the requested width.
- `fit.go`'s `share` / `spreadBlocks` / `fitSections` slot arithmetic is sound: the distributed
  shares sum to exactly `extra`, and short columns are guarded by `y < len(section)`.
- `gauge.Render`'s forecast marker cannot over- or under-run the track (`pct` is clamped and the
  write is gated on `marker < needle`).
- `resets.fittedCell`'s `bits.Div64` cannot panic — `hi` can never reach the 7-day horizon.

No high- or medium-severity correctness bugs were found. Three low-severity issues follow.

## Findings

### 1. `spend.enabled` / `extra_usage.is_enabled` are ignored — a disabled account reports spendable money

`internal/provider/claude/claude.go:213`

`balancesFrom` keys only off "are there usable money values". The only live capture,
`testdata/usage_ledger_synthetic.json`, has `spend.enabled: false`,
`disabled_reason: "out_of_credits"` and `extra_usage.is_enabled: false`, yet
`used.amount_minor: 0` / `limit.amount_minor: 100`. So `qmeter spend` prints

```
claude  extra usage  $1.00  $1.00  ▰▰▰…
```

and the dashboard ledger shows `extra usage  $1.00 of $1.00`, telling a user who is out of
credits that a dollar is available. `internal/provider/claude/balances_test.go:26` locks this in.

Note the inconsistency: `cursor.balances` (`internal/provider/cursor/cursor.go:209,216`) gates both
of its rows on `enabled`, and `docs/specs/gauge-additions.md:353` says to omit cursor's disabled
`onDemand` row. Claude gets no equivalent gate.

### 2. Non-USD amounts print at full float64 precision

`internal/spend/render.go:35`

`strconv.FormatFloat(*value, 'f', -1, 64)` is used for every unit except `usd`. A cursor
`individualUsage.plan.remaining` of `33.333333333333336` renders literally as
`33.333333333333336` in the `LEFT` and `OF` columns of `qmeter spend`, and the same string flows
into `layout.ledgerRow`, where `truncTail` eats it against the meter block width. Current captures
happen to hold integers, so this is latent rather than live.

### 3. The `RunsDry` ✕ is suppressed in exactly the `empty` case it documents

`internal/dash/gauge/gauge.go:119`

`layout.windowBlock` maps `pace.Forecast`'s `"empty"` state to `gauge.RunsDry`, which sets
`marker = 0`. But an empty window has `RemainingPercent == 0`, so `needle == 0` and the guard
`marker >= 0 && marker < needle` is false. Since `internal/provider/codex/codex.go:483` is the only
place `RateLimited` is set (`used_percent >= 100`, i.e. remaining 0), the `"empty"` branch can never
draw its marker — a 0.0% window renders `┴▲▱▱▱…┴` with no ✕, contradicting the constant's comment
("RunsDry marks exhaustion at cell zero"). Only the `"dry"` state (remaining > 0) actually shows it.

The adjacent `empty` text note still conveys the state, and there is no free cell at needle 0, so
this may be acceptable as-is — but the constant and its comment promise behaviour the code cannot
deliver.

## Resolution (2026-09-20)

All three findings are closed. Each code change was written test-first: the new cases
were run and seen to fail for the stated reason before the fix went in.

1. **Fixed.** `balancesFrom` (`internal/provider/claude/claude.go`) now gates the money
   row on `spend.enabled` and the percent row on `extra_usage.is_enabled`, through a
   shared `sectionEnabled` helper. Only an explicit `false` switches a section off, so
   responses that omit the flag behave exactly as before. A disabled `spend` still falls
   through to an enabled `extra_usage`. The synthetic live capture has both switched
   off and now reports no balance at all instead of an available dollar.
   `internal/provider/claude/balances_test.go` covers enabled money, disabled money
   falling back to percent, both disabled, and a disabled `extra_usage` alone.
   `docs/specs/gauge-additions.md` gains rule 7 for the same behaviour.
2. **Fixed.** `amount` (`internal/spend/render.go`) formats non-USD values at two
   decimals and trims the trailing zeroes, so `33.333333333333336` renders as `33.33`
   while `10` stays `10` and `62.5%` stays `62.5%`. Existing `TestAmounts` expectations
   are unchanged; four repeating and rounding cases were added.
3. **Documented, not changed** — the owner's call. The `RunsDry` comment in
   `internal/dash/gauge/gauge.go` now says the ✕ draws only while the window still
   holds some allowance, since at 0% the needle itself occupies cell zero and the
   adjacent `empty` note carries the state. No render change, so the layout goldens
   stay byte-identical.

Verified after the changes: `gofmt -l .` clean, `go vet ./...` clean,
`go test -race ./...` all 30 packages pass.
