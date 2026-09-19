# Spec: run-out forecast, reset timeline, money ledger

Status: 1–2 implemented and reviewed on `dev`; 3 not started, awaiting live captures
(agent credentials unavailable). Source: the qmeter design bench shortlist.
Develop on `dev` or a branch off it; never commit to `main`.

All three read account-wide vendor data, so they behave the same on a server and on a
personal machine. None of them stores anything. Features that need storage are in
[`docs/BACKLOG.md`](../BACKLOG.md).

Rules that apply to every feature here (from `AGENTS.md`):

- Test first. Each "Tests" list below is the order to write failing tests in.
- Command files hold wiring only: `cmd/<name>/` calls into `internal/<name>/`.
- Done means `gofmt -l .`, `go vet ./...` and `go test -race ./...` are clean.
- Existing JSON fields never change meaning. New fields are additive.

---

## 1. Run-out forecast (tier A, small)

**Question it answers:** at the current burn, does this window run dry before it
resets, and if not, how much is left when it does?

**Settled decisions** (2026-09-19):

1. No forecast until 5% of the window's period has elapsed, unless already dry.
2. Dashboard: the track marker always shows, subject to the needle guards (`◇` only
   strictly left of the needle, `✕` only when the needle isn't in cell 0); the
   title-line note shows only when it fits, and the window name keeps priority over
   the note.
3. A rate-limited or empty window is treated as already dry: text `empty`, exhaustion
   at `now`, lands at 0%. Rule 1 below still wins first: invalid timing means no
   forecast, so the existing layout goldens stay byte-identical.
4. Forecast JSON fields appear only in `qmeter pace --json`; `usage --json` is
   unchanged.

### Maths

Lives in `internal/pace`, next to `Calculate`, as a pure function of
`(provider.Window, now)`:

```go
func Forecast(w provider.Window, now time.Time) Projection

type Projection struct {
	State            string     // "none", "survives", "dry", "empty"
	ExhaustionAt     *time.Time // nil unless State is "dry" or "empty"
	RemainingAtReset *float64   // nil unless State is "survives", "dry" or "empty"
}
```

`State` is not serialized; the JSON envelope carries only the two fields named under
`qmeter pace` below. `RenderText` prints `empty` by switching on `State`, never by
comparing `ExhaustionAt` to `now`. `layout` maps `State` to the gauge's `forecast`
parameter (see Dashboard below): `"none"` → `gauge.NoForecast`, `"survives"` →
`*RemainingAtReset`, `"dry"` and `"empty"` → `gauge.RunsDry`.

```
start     = ResetsAt - Period
elapsed   = now - start
left      = ResetsAt - now
used      = 100 - RemainingPercent
```

The rules are checked in this order; each one short-circuits the ones after it, and
sets `State` as named:

1. **Invalid timing** → `State = "none"`, always: `ResetsAt` is zero, `Period <= 0`, or
   `now` is outside `[start, ResetsAt)`. Same conditions under which `Calculate`
   returns `n/a`.
2. **Already dry** → `State = "empty"`: `RemainingPercent <= 0` or `RateLimited`.
   `ExhaustionAt = &now`, `RemainingAtReset = &0`. A rate-limited window with
   `Period == 0` matches both rule 1 and this rule — rule 1 wins, so it still gets
   `State = "none"`. That exact window is `sample()`'s rate-limited window in
   `internal/dash/layout/layout_test.go:49` ("GPT-5.3-Codex-Spark secondary",
   `Period` unset); if "empty" won instead, its track would gain a `✕` and every
   golden in `internal/dash/layout/testdata/*.txt` would change. The existing layout
   goldens must stay byte-identical.
3. **Elapsed floor** → `State = "none"`: `elapsed < 5%` of `Period`. A few minutes of
   data gives a wild burn rate.
4. **Survivor by construction** → `State = "survives"`: `used <= 0` — no exhaustion;
   `RemainingAtReset = &RemainingPercent`. This must be checked before any division by
   `used`.
5. **General case**, decided in float64 so a tiny `used` never divides toward
   overflow: the window runs dry (`State = "dry"`) iff
   `RemainingPercent * elapsed < used * left`. Only on that branch is a
   `time.Duration` built — `dryIn = time.Duration(RemainingPercent * float64(elapsed) /
   used)` — and it is `< left` by construction, so it cannot approach the ~292-year
   `time.Duration` ceiling or wrap negative; `ExhaustionAt = &(now + dryIn)`,
   `RemainingAtReset = &0`. Otherwise, `State = "survives"`:
   `RemainingAtReset = &max(0, RemainingPercent - used*left/elapsed)`, `ExhaustionAt`
   nil. The tie `RemainingPercent * elapsed == used * left` (`dryIn == left`) falls
   here: a survivor landing at exactly 0%.

### `qmeter pace`

- Text: one new column, `RUNS OUT`, after `EXPECTED`. Values: the `in 2h10m` table
  form `ResetsCell` already builds for `RESETS` (`internal/usage/render.go:61-69`)
  when `State == "dry"`, `empty` when `State == "empty"`, `-` for `"survives"` and
  `"none"`. (The dashboard's title-line note uses the bare `2h10m` form instead — see
  Dashboard below. The two are different strings, not the same format.)
- JSON: each window gains `projected_exhaustion_at` (RFC 3339 or `null`) and
  `projected_remaining_at_reset` (number or `null`). Both are always present, and only
  in `qmeter pace --json` — `usage --json` is unchanged.

### Dashboard

- `internal/dash/gauge`: `Render`'s signature — today `Render(pct, width, rateLimited,
  pace float64)` (`Render` in `gauge.go:82`) — gains a fifth `forecast float64`
  parameter: `Render(pct float64, width int, rateLimited bool, pace, forecast
  float64)`. Two exported constants alongside the existing `NoPace = -1`
  (`gauge.go:42`): `NoForecast = -1` draws no marker; `RunsDry = -2` draws `✕`. Any
  other value is `remaining_at_reset` in `[0, 100]` and draws `◇`. Existing gauge
  tests change mechanically only: the extra argument.
  - `◇`: at track cell `needleIndex(forecast, n)`, drawn only when that index is
    strictly less than the needle's own index.
  - `✕`: in track cell 0, in the red band colour, drawn only when the needle's index
    is not 0.
  - `forecast == NoForecast`: track is unchanged.
  - A survivor landing at exactly 0% (the Maths rule-5 tie) passes `forecast = 0`, so
    it draws a `◇` at cell 0 — distinct from a dry window's `✕`, which is a different
    parameter value (`RunsDry`) even though both occupy cell 0.
  - The `◇` colour is a new exported constant, `display.Forecast`, ANSI colour `"13"`
    (bright magenta) — defined beside the existing colour constants
    (`Countdown`/`Error`/`Neutral`/`RateLimited` in `internal/display/display.go:17-20`);
    `"13"` is not used by any existing constant there. It is not one of the health
    band colours or the pace cyan. `gauge.Plain` must strip it like the other styles.
  - No collision with the marks already there: the pace `▼` lives in the bezel row,
    not the track (`bezelRow`), so it never shares a cell with `◇`/`✕`; the two guards
    above are what keep `◇`/`✕` from colliding with the needle itself.
- `internal/dash/layout`: `windowBlock` calls `pace.Forecast` and maps its `State` to
  `gauge.Render`'s `forecast` parameter (see Maths above). The track marker is always
  drawn, subject to the same needle guards as above (`◇` strictly left of the needle,
  `✕` only when the needle isn't in cell 0); the window title line additionally gains
  a note two spaces after the pace badge, present only when it fits and only for
  three `State`s:
  - `"dry"`: `dry in 2h10m` — the countdown formatted with the same bare-form
    formatter the dashboard's own countdown already uses (`countdown`/`formatResets`
    in `layout.go`), not `ResetsCell`'s `in …` form.
  - `"survives"`: `lands at 12%` — the integer percent, rounded half up with
    `math.Round`.
  - `"empty"`: `empty` — matching the `qmeter pace` text for the same state.
  - `"none"`: no note.
  - "Fits" means the whole title line, name included, stays within the block width:
    `2 + width(name after truncMid at today's nameWidth) + 1 + width(badge) + 2 +
    width(note) <= blockw`. The name's width budget (`nameWidth`) is unchanged by the
    note's presence.
  - Style: `dry in …` and `empty` render in the red health-band colour (the same
    colour `gauge.Band`'s bottom band and `display.RateLimited` use); `lands at …`
    renders in `dimStyle` (`layout.go:111`, `lipgloss.NewStyle().Foreground(display.Neutral)`),
    the dim style `layout` already uses for secondary text.
  - The window name keeps priority for its existing width budget (`nameWidth` in
    `layout.go:433`); the note is what drops first when space is short, not the name.
    Either way, the marker and `qmeter pace` still carry the information.

### Tests

1. `pace.Forecast` table test: mid-window survivor, mid-window dry, `used == 0`,
   already empty, rate limited, zero `ResetsAt`, zero `Period`, `now` outside the
   window, under 5% elapsed, rate limited with `Period == 0` (rule 1 beats rule 2 —
   `State == "none"`), tiny `used` (still `State == "survives"` with
   `RemainingAtReset` ≈ `RemainingPercent` and no `Duration` built — the case that
   overflowed under a dryIn-first formula), and the `dryIn == left` tie
   (`State == "survives"` landing at exactly 0%).
2. `pace.RenderText`: the `RUNS OUT` column for dry, survivor, n/a and already-dry
   (`empty`) rows. Like the package's existing render test, this asserts on
   substrings, not a file golden (`internal/pace/pace_test.go:74`).
3. `pace.RenderJSON`: both fields present, `null` when no forecast, RFC 3339
   otherwise; an already-dry window's `projected_exhaustion_at` equals `now`
   formatted as RFC 3339 (sub-second precision is dropped).
4. `gauge.Render`: `◇` position, `◇` suppressed when not left of the needle, `✕` at
   cell 0, `✕` suppressed when the needle is itself in cell 0, no marker for
   `NoForecast`, `Plain` output has no styling — and `◇`/`✕` are asserted against the
   styled `Track` string too, not only `Plain`.
5. `layout`: all three note forms — `dry in 2h10m`, `lands at 12%`, `empty` — present
   when they fit, and no note for `"none"`. Force omission with a long window name or
   a long pace badge, not the minimum 22-cell gauge: at that minimum `blockw` is 36
   cells (`blockw` in `layout.go:416`) and all three example notes fit there. Also
   cover the note on a rate-limited window, alongside its `[RL]` bezel tag.

### Note

- The sample log (see backlog), once it exists, could replace the whole-window burn
  average with a recent-rate estimate; the 5% floor may not be needed once it does.

---

## 2. Reset timeline (tier A, small–medium)

**Question it answers:** across every provider, what frees up next?

### `qmeter resets`

New `cmd/resets/` and `internal/resets/`. Takes `--filter` (a string-slice flag,
resolved via `usage.Select` then `usage.Run(ctx, selected, "")` — the same pattern
`pace` (`cmd/pace/pace.go:63`) and the root dashboard (`cmd/dash/dash.go:40`) use) and
the root's persistent `--json`. `usage` itself takes a different, single-string
`--provider` flag instead (`cmd/usage/usage.go:86`); `resets` does not follow that one.

- Order: windows sorted by `ResetsAt` ascending; windows with no `ResetsAt` last, in
  their original order. The sort is stable.
- Text: a shared linear axis from `now` to `+7d`, one row per window:
  provider glyph, window name, remaining percent in its band colour, then a dotted
  run to a marker at the reset time, then the countdown. The glyph table moves from
  its current home — the unexported `order` table in `internal/dash/layout`
  (`layout.go:100-105`) — into `internal/display`, since `ProviderCell` has no glyph
  today (`ProviderCell` in `internal/display/display.go:112`); both `internal/resets`
  and `layout` then read the same table, and `layout` importing `internal/resets`
  creates no cycle (`layout` already imports `internal/pace`).
  - The marker is the provider glyph coloured by the window's health band, so a red
    marker near `now` reads as "relief is close".
  - A rate-limited window's label uses `↑RL` to say the reset lifts the rate limit,
    leaving more of the name at narrow widths. The README explains the abbreviation.
  - A window resetting beyond 7 days gets a full dotted row, a `▸`, and the countdown.
  - A window with no `ResetsAt` gets its name and percent and no marker.
  - The minimum is the available width of whatever hosts the view — the terminal for
    `qmeter resets`, the page width for the dashboard's `t` page — computed from the
    row's own columns, in order: glyph + space (2), a name minimum (10), a gutter (1),
    remaining percent (6), a gutter (1), the axis (43 cells indexed 0–42, one cell per
    4 hours), a gutter (1), countdown (6); 70 cells total. The axis cell is
    `floor(max(0, reset - now) / 4h)`: exactly `+7d` puts the provider glyph in cell
    42; anything later puts `▸` in cell 42 after 42 dots. Below the minimum width,
    fall back to a plain sorted table
    (`PROVIDER WINDOW REMAINING RESETS`).
  - Not a TTY: same plain sorted table, no axis.
- JSON: the `usage` envelope (`windows`, `errors`, `undetected`) with windows in the
  sorted order and one added field per window, `resets_in_seconds` (integer or `null`).
  It is the signed whole-second duration to the reset, truncated toward zero, or
  `null` when the reset time is unknown.

### Dashboard

- A second page in `internal/dash`, toggled with `t`, drawn by a new function in
  `internal/dash/layout` that calls the same row builder as `internal/resets`.
- `t` is added to the footer hints. `r` (refresh) and scrolling keep working on both
  pages. The page choice is not persisted.

### Tests

1. `resets.Sort`: ascending, zero `ResetsAt` last, stable for equal times.
2. Row builder: marker cell for a known `(now, ResetsAt, width)`, beyond-7-days row,
   no-reset row, rate-limited label, marker never overlaps the label.
3. Narrow width and non-TTY fall back to the table.
4. `RenderJSON`: sorted order and `resets_in_seconds`, `null` when unknown.
5. `dash.Model`: `t` toggles the page and back; `r` still refreshes on the timeline.

### Open question

- Rolling 5h windows cluster at the left edge of a linear 7-day axis. Ship linear
  first; try a log-scaled axis only if the cluster is unreadable in real use.

---

## 3. Money ledger (tier B, medium)

**Question it answers:** how much real money or credit is left, where a vendor reports
it, instead of only a percentage.

### Prerequisite: capture live responses

The field shapes below come from test fixtures, and a fixture can outlive the API it
copied. Before writing any decoder, capture one live response per vendor (keys and
value types are enough) and add it as a new fixture. One known discrepancy already:
the Codex fixture has `credits.balance` as a number (`3.5`), but Codex's own session
logs show it as a string (`"0"`). The decoder must accept both.

| Provider | Fields | Today |
|---|---|---|
| cursor | `individualUsage.plan {used, limit, remaining}` and `onDemand {enabled, used, limit, remaining}`; unit unconfirmed until the live capture (the fixture value 14.25 of 20 fits either dollars or cents, and `cursor.go` has no unit comment) — `limit` and `remaining` are `null` without on-demand spending | parsed, not surfaced |
| codex | `credits {has_credits, unlimited, balance}`, `spend_control {enabled}` | ignored by design in `usageResponse` |
| claude | `extra_section.utilization` — inferred to be a percentage from sibling fields, unconfirmed until the live capture (the fixture value is 0), no dollar amount | not decoded |
| opencode-go | `spend` / `limit` seen only in the unknown-fields fixture, as strings (e.g. `"4.80"`); that fixture also has a `caps` object (e.g. `"$12"`) — note both for when this is seen live | out of scope until seen live |

### Data model

A new normalized type beside `provider.Window`:

```go
type Balance struct {
    Provider  string   // "cursor"
    Name      string   // "included", "on-demand", "credits", "extra usage"
    Unit      string   // "usd", "credits", "percent"
    Used      *float64 // nil when the vendor does not report it
    Limit     *float64
    Remaining *float64
    Unlimited bool
}
```

Wire form follows `Window`: snake_case, explicit `MarshalJSON`, absent values are
`null`, never `0`.

Cursor's decoder fields are plain `float64` today (`individualUsage.plan`'s
`Used`/`Limit`/`Remaining` at `internal/provider/cursor/cursor.go:178-180`, and the
`OnDemand` equivalents at `cursor.go:196-198`), so a JSON `null` decodes as `0` —
indistinguishable from a genuine zero. They must become `*float64` to honour
"null, never 0" above. The whole `onDemand` object, and `plan`'s
`used`/`limit`/`remaining`, can also be `null`
(`internal/provider/cursor/testdata/usage_summary_nulls.json`) — Tests item 1 below
covers both.

**Settled decisions** (2026-09-19):

1. `Fetch` returns a struct, `provider.Usage{Windows, Balances}`. The balances come
   from the same HTTP response as the windows, so this costs no extra request.
   This is more than a mechanical change to the four providers and `providertest`:
   the `Fetch` method is declared on the `provider.Provider` interface
   (`internal/provider/provider.go:76`), so it also touches `usage.Result` (needs a
   new `Balances` field, `internal/usage/usage.go:32-36`), `usage.Run`, the `blocker`
   fake in `internal/dash/run_test.go`, `compileCheckProvider` in
   `internal/provider/provider_test.go`, and every `.Fetch(` call site across the
   provider tests — 68 of them today
   (`grep -rn '\.Fetch(' --include=*_test.go . | wc -l`).

### `qmeter spend`

New `cmd/spend/` and `internal/spend/`.

- Text: one table, `PROVIDER NAME LEFT OF BAR`. Dollars as `$5.75`, credits as a plain
  number, percent as `%`. The bar is 20 cells in the health band colour of the
  remaining fraction; no bar when there is no limit. `unlimited` replaces the amount
  when `Unlimited` is set. Providers that report nothing are omitted, not shown empty.
- JSON: `{ "balances": [...], "errors": [...], "undetected": [...] }`.

### Dashboard

- Short ledger rows under each provider's windows, inside the provider's existing
  section in `internal/dash/layout`. A provider with no balances gets no rows, so the
  page is unchanged for accounts without any.

### Tests

1. Per provider, decoder tests against the new live-shape fixture and the existing
   ones: cursor with and without on-demand (`null` limit), cursor with the whole
   `onDemand` object `null` and with `plan.{used,limit,remaining}` all `null`
   (`usage_summary_nulls.json`), codex balance as number and as string, codex
   `unlimited`, codex `hasCredits` camelCase (`usage_camel_case.json`) and
   `credits: {}` (`usage_missing_windows.json`), claude utilization, absent sections
   yield no balance.
2. `Balance.MarshalJSON`: snake_case, `null` for unknowns.
3. `usage.Run`: balances are collected in provider order; a provider error still drops
   only that provider.
4. `spend.RenderText`: an exact-match text test in the style of `usage.RenderText`'s
   inline `goldenBlock` (`internal/usage/render_test.go:22`) — there is no file-based
   golden here, unlike `layout`'s `testdata/*.txt` goldens — and `spend.RenderJSON`.
5. `layout`: ledger rows present with balances, page identical to today without them.
6. Every existing provider and `usage`/`pace` test still passes unchanged in meaning.

### Open questions

- Should `qmeter usage --json` also gain `balances`? Default: no, only `spend` does,
  so existing consumers see no change.
- Currency: assume USD and say so in `Unit`. Revisit only if a vendor reports another.
