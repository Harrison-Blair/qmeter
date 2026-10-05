# Spec: triple-track gauge

Status: implemented and independently reviewed on `dev`. Design agreed with the owner on
2026-10-04.
Develop on `dev` or a branch off it; never commit to `main`.

**Why:** the dashboard is hard to read from across a room. The track is one row of thin
`▰▱` glyphs, so the fill, the part that says how much is left, is the smallest thing on
the page. This makes the track three rows tall and takes the forecast glyphs out of it.

This is a rendering change only. It reads no new data and stores nothing, so it behaves
the same on a server and on a personal machine.

Rules that apply (from `AGENTS.md`):

- Test first. Write each failing test, see it fail for the right reason, then implement.
- Command files hold wiring only; this change lives in `internal/dash/gauge` and
  `internal/dash/layout`.
- Done means `gofmt -l .`, `go vet ./...` and `go test -race ./...` are clean.
- Never weaken or skip a test to get green. Tests that assert behaviour this spec
  removes are rewritten to assert the new behaviour, not deleted without a replacement.

## The gauge

Today (three rows):

```text
╭┬────┬─────┬───▼┬─────┬╮
┴▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▲▱▱▱▱▱▱▱┴
 0         50        100
```

After (five rows, same width):

```text
╭┬────┬─────┬───▼┬─────┬╮
│▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱▱│
│▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱▱│
┴▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▲▱▱▱▱▱▱▱┴
 0         50        100
```

Settled decisions:

1. **Three track rows.** The bezel row and the scale row are unchanged, pace marker
   included.
2. **The needle is on the bottom track row only.** That row is today's track row: `┴`
   caps, `▰` fill in cells `[0, needle)`, the white `▲` at `needle`, `▱` spent after it.
3. **The fill ends at the same column on every row.** The two upper rows have `▰` in
   cells `[0, needle)` and `▱` in cells `[needle, n)`. The cell above the needle is
   spent, never filled. At 0% the upper rows are all `▱`; at 100% they end in one `▱`
   above the needle.
4. **Upper rows are capped with `│`** in the frame style, where the bottom row has `┴`,
   so the bezel's `╭ ╮` corners run down the sides to the `┴` feet.
5. **Colours are unchanged.** Fill takes the band colour on all three rows, spent cells
   keep the faint red, the frame keeps bright black, and a rate-limited window still
   turns its frame (the `│` caps included) faint red. The pace marker stays cyan and the
   needle stays white.
6. **No forecast glyphs in the track.** `◇` and `✕` are removed. `gauge.Render` loses
   its `forecast` parameter, and `gauge.NoForecast`, `gauge.RunsDry` and
   `display.Forecast` go with it if nothing else uses them. This supersedes settled
   decision 2 of the run-out forecast in [`gauge-additions.md`](gauge-additions.md) as
   far as the track marker goes.
7. **The forecast note stays.** `lands at N%`, `dry in …` and `empty` on the window's
   name row keep their text, colours and fit rules exactly as they are. `qmeter pace`
   and its JSON are untouched.
8. **Three rows is the default, not the only choice.** The number of track rows is the
   gauge's thickness, set as described under "Thickness" below. There is still one
   layout.
9. **`gauge.MinWidth` stays 22** and the width arithmetic does not change; every row of
   a block is still exactly the requested width in cells.

`gauge.Block` carries one row per unit of thickness; its exact shape is the
implementer's call. `gauge.Plain` strips every row.

## Thickness

Settled decisions (2026-10-04):

1. **Thickness is the number of track rows**, from 1 through 9. The default is 3.
2. **`--thickness N`** on the root command sets it for one run. A value outside 1–9 is
   an error reported before anything is fetched, the same way an unknown `--filter`
   provider is: one line on stderr, non-zero exit, no dashboard.
3. **`meter_thickness = N`** in `config.toml` sets it persistently, next to
   `meter_width`. A value outside 1–9 is a config error like any other: a warning, then
   the built-in defaults.
4. **The flag wins over the config, but only when it was passed.** Without the flag the
   config value applies; without either, 3.
5. **Piped or `--json` output ignores it**, as it ignores `--vertical`. An out-of-range
   `--thickness` is still an error there.
6. **At any thickness the needle is on the bottom track row only**, which is drawn
   exactly as decision 2 of "The gauge" says. Every row above it is an upper row as in
   decisions 3 and 4. Thickness 1 has no upper rows.
7. **The percentage and the countdown sit on track row `(N-1)/2`** (integer division,
   counting from 0 at the top): the only row at 1, the top row at 2, the middle row at
   3, the second row at 4, the middle row at 5. `[RL]` is always on the bezel row.
8. **A window is `N + 3` rows**: name, bezel, `N` track rows, scale. At thickness 1 that
   is the four-row block the dashboard had before this spec, without forecast glyphs.

Thickness 1:

```text
▸ 5h [on pace]  dry in 2h54m
       ╭┬────┬─────┬───▼┬─────┬╮   [RL]
 68.0% ┴▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▲▱▱▱▱▱▱▱┴ 3h38m
        0         50        100
```

Thickness 5:

```text
▸ 5h [on pace]  dry in 2h54m
       ╭┬────┬─────┬───▼┬─────┬╮   [RL]
       │▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱▱│
       │▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱▱│
 68.0% │▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱▱│ 3h38m
       │▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱▱│
       ┴▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▲▱▱▱▱▱▱▱┴
        0         50        100
```

The value travels the way `meter_width` does: `config.Settings` → `cmd/dash` →
`dash.RunOptions` → the model → `layout.Options` → `gauge.Render`. Zero in
`layout.Options` means the default, as it does for `MeterWidth`.

## The window block

At the default thickness a window goes from four rows to six. The percentage and the
countdown move to the middle track row; `[RL]` stays on the bezel row.

```text
▸ 5h [on pace]  dry in 2h54m
       ╭┬────┬─────┬───▼┬─────┬╮   [RL]
       │▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱▱│
 68.0% │▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱▱│ 3h38m
       ┴▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▲▱▱▱▱▱▱▱┴
        0         50        100
```

Everything that sizes itself from a section's row count (card heights, spare-row
sharing, the compact layout under 40 columns, scrolling) must come out right with the
taller block. Nothing else about the page changes: columns, card frames, banner, summary
header, ledger rows, status rows, timeline, calendar and footer stay as they are.

## Tests

Write these failing first, in this order.

`internal/dash/gauge`:

1. Shape: a block has a bezel, three track rows and a scale, each exactly the requested
   width, for every width from `MinWidth` to 60.
2. Upper rows at a known width and percentage: exact plain text, `│` caps, `▰` in
   `[0, needle)`, `▱` from the needle column to the end.
3. The cell above the needle is `▱` on both upper rows, at 0%, 1%, 50%, 99% and 100%.
4. The bottom row is byte-identical to the track row the old `Render` produced with no
   forecast, styled and plain.
5. No row of any block contains `◇` or `✕`, including for a rate-limited window.
6. Styling: the upper rows' fill uses the band colour and their caps use the frame
   style; for a rate-limited window both the caps and the frame are faint red.
7. `Plain` removes every escape sequence from all five rows.

`internal/dash/layout`:

8. A window block is six rows in the order above, each padded to the column width.
9. The percentage and the countdown are on the middle track row and nowhere else; the
   bottom track row has neither.
10. `[RL]` is on the bezel row for a rate-limited window.
11. The forecast note cases in `forecast_test.go` still pass for the note itself, and
    the same cases assert no `◇` or `✕` anywhere on the page.
12. A two-window card and a one-window card in the same row get the same height, with
    the shorter content centred.
13. The goldens in `internal/dash/layout/testdata/` are regenerated and reviewed by eye:
    every one must show the six-row block and no forecast glyph.

Thickness option (write these failing first too):

14. `gauge`: for every thickness 1–9 a block has that many track rows, each the
    requested width; only the last has `▲` and `┴` caps, the rest have `│` caps and the
    upper-row fill rule.
15. `gauge`: thickness 1 is exactly bezel, bottom row, scale; thickness 3 is
    byte-identical to what the fixed three-row implementation renders.
16. `layout`: for thicknesses 1, 2, 3, 4 and 5 a window is `N + 3` rows and the
    percentage and countdown are on track row `(N-1)/2` and nowhere else.
17. `layout`: zero thickness in `Options` renders the same page as 3. The existing
    goldens do not change.
18. `layout`: a new golden at 120 columns for thickness 1 and one for thickness 5.
19. `config`: `meter_thickness` absent gives 3; 1 and 9 are accepted; 0, 10 and a
    negative number are rejected with a message naming the key and the range.
20. `cmd/dash`: `--thickness 5` reaches `RunOptions`; without the flag the config value
    does; the flag beats the config; `--thickness 0` and `--thickness 10` fail before
    any fetch with one stderr line; piped output with a valid `--thickness` is the plain
    usage table.
21. `dash` model: the thickness given in `RunOptions` is the one the page is drawn
    with, including after a resize.

## Docs

- `README.md`, Dashboard section: say the track is three rows tall with the needle on
  the bottom row.
- The package comments in `internal/dash/gauge/gauge.go` and
  `internal/dash/layout/layout.go`, which draw the old gauge and say "four rows per
  usage window".
- `docs/specs/gauge-additions.md`: mark the track-marker half of forecast decision 2 as
  superseded by this spec.
- `README.md`: add `--thickness` to the dashboard flags list and `meter_thickness` to
  the configuration example and its explanation.
- `docs/screenshots/*.png` show the old gauge. Do not regenerate them; report that they
  are stale.

## Out of scope

Bigger text, block-digit numerals, a solid-colour fill, hiding the banner by default, and
any flag or config key other than `--thickness` and `meter_thickness`.
