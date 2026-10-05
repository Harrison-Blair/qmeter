# Spec: closed gauge

Status: implemented on `dev`, pending independent review. Design agreed with the owner on
2026-10-05.
Develop on `dev` or a branch off it; never commit to `main`.

**Why:** the owner wants the bar entirely inside the gauge. Today the frame is open at
the bottom: the lowest track row doubles as the frame's bottom edge and carries the
needle, so the fill has a notch cut out of it. This closes the frame with a second rail
under the track and moves the needle onto that rail.

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

Today (thickness 3, width 25, 68% left, pace marker shown):

```text
╭┬────┬─────┬───▼┬─────┬╮
│▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱▱│
│▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱▱│
┴▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▲▱▱▱▱▱▱▱┴
 0         50        100
```

After (same inputs; one row taller):

```text
╭┬────┬─────┬───▼┬─────┬╮
│▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱│
│▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱│
│▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱│
╰┴────┴─────┴───▲┴─────┴╯
 0         50        100
```

Names used below: the **top rail** is the first row (today's bezel), the **bottom rail**
is the new row under the track, `n` is the number of track cells (the width minus the
two frame cells), and the **needle cell** is the track cell index today's `needleIndex`
returns for the remaining percentage.

Settled decisions:

1. **The frame is closed.** A gauge is the top rail, `N` track rows, the bottom rail and
   the scale, where `N` is the thickness. Every row is exactly the requested width.
2. **The top rail and the scale are unchanged**, pace marker included.
3. **The bottom rail mirrors the top rail**: `╰` and `╯` corners, a `┴` in every column
   where the top rail (without its pace marker) has a `┬`, and `─` everywhere else.
4. **The needle is on the bottom rail.** The white `▲` replaces the bottom-rail cell
   under the needle cell, whether that cell was `─` or a `┴` tick. This is the same
   column the needle is in today, so a needle left of the pace marker still means the
   window is being spent faster than even pace.
5. **Every track row is the same**: a `│` cap, `▰` fill, `▱` spent, a `│` cap. No track
   row contains `▲` or `┴`.
6. **The fill runs up to and including the needle cell.** For a remaining percentage
   above 0 (after clamping to 0–100), cells `[0, needle]` are `▰` and the rest are `▱`.
   At 100%, and for any input above 100, the whole track is filled. A small positive
   percentage that rounds to the first cell fills exactly that one cell.
7. **An empty window shows no fill.** This overrides decision 6 at exactly 0: when the
   clamped percentage is 0 (which includes any negative input), every track cell is
   `▱`. The needle is still drawn, under the first cell.
8. **Colours are unchanged.** Fill takes the band colour, spent cells keep the faint
   red, the needle stays white and the pace marker stays cyan. The bottom rail and its
   ticks take the frame style like the top rail, so a rate-limited window turns both
   rails, the caps and the scale faint red while the needle stays white.
9. **`gauge.MinWidth` stays 22** and the width arithmetic does not change.
10. **No forecast glyphs.** `◇` and `✕` stay out of every row, as today.

`gauge.Block` carries the rows; its exact shape is the implementer's call. `gauge.Plain`
strips every row, the bottom rail included.

The pictures in this file leave out trailing padding. Every row is padded with spaces to
the requested width, so the scale row of a 25-cell gauge is 25 cells with one trailing
space; exact-text tests include that padding.

At the ends (width 22, `n` = 20):

```text
100%   │▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰│      0%   │▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱│
       ╰┴───┴────┴────┴────▲╯           ╰▲───┴────┴────┴────┴╯
```

## Thickness

Thickness keeps its meaning, its range and its default: the number of track rows, 1
through 9, default 3. The `--thickness` flag, the `meter_thickness` config key, their
validation and their precedence do not change, and neither does anything in
`internal/dash/config`, `cmd/dash`, or the model wiring.

What changes:

1. **A window is `N + 4` rows**: name, top rail, `N` track rows, bottom rail, scale. At
   the default that is seven rows, one more than today.
2. **The percentage and the countdown stay on track row `(N-1)/2`** (integer division,
   counting from 0 at the first track row). They are never on a rail or the scale.
3. **`[RL]` stays on the top rail row.**

Thickness 1:

```text
▸ 5h [on pace]  dry in 2h54m
       ╭┬────┬─────┬───▼┬─────┬╮   [RL]
 68.0% │▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱│ 3h38m
       ╰┴────┴─────┴───▲┴─────┴╯
        0         50        100
```

Thickness 3 (the default):

```text
▸ 5h [on pace]  dry in 2h54m
       ╭┬────┬─────┬───▼┬─────┬╮   [RL]
       │▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱│
 68.0% │▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱│ 3h38m
       │▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱│
       ╰┴────┴─────┴───▲┴─────┴╯
        0         50        100
```

## The page

Everything that sizes itself from a section's row count (card heights, spare-row
sharing, scrolling) must come out right with the taller block. The unframed sections
drawn from 36 to 39 columns keep their layout and include the new rail; under 36 the
page still says the terminal is too narrow. Nothing else about the page changes: columns, card frames, banner, summary
header, forecast note, ledger rows, status rows, timeline, calendar and footer stay as
they are. Piped and `--json` output do not change.

The fallback in `windowBlock` that draws blank rows when `gauge.Render` returns an error
must produce the same number of rows as a drawn gauge. That branch cannot be reached
today, so it is checked by reading the code, not by a test.

## Inputs and trust

This change adds no input. The gauge still takes a percentage, a width, a pace fraction
and a thickness from the caller. The percentage comes from a provider's response and is
already clamped to 0–100 inside `gauge.Render`; the thickness is already validated to
1–9 before any fetch. Nothing here reads a file, a path, the environment or the network,
so there is no new threat to model. A verifier finding outside rendering correctness is
a note, not a failure.

## Tests

Behaviour this spec changes gets a test that is seen failing on the current code for the
right reason, then passing. Behaviour this spec keeps may already pass; for those, show
the test is sensitive by breaking the behaviour in a throwaway copy and seeing it fail.
The implementer's report maps every item below to a test function, or says it is a
golden review or a code reading.

`internal/dash/gauge`:

1. Shape: for every thickness 1–9 and every width from `MinWidth` to 60, a block is a
   top rail, that many track rows, a bottom rail and a scale, each exactly the requested
   width.
2. Exact plain text at width 25 and 68% with pace 0.68: the six rows drawn under
   "After" above, padded to 25 cells.
3. Track rows: the `N` track rows of a block are identical; each starts and ends with
   `│`; none contains `▲` or `┴`.
4. Needle column, independently: for a spread of percentages and widths the `▲` is in
   bottom-rail body cell `int(pct/100*float64(width-3) + 0.5)`, computed in the test
   and not by calling the package's own helper.
5. Fill rule: at 1%, 50%, 68%, 99% and 100% the track cell above the needle is `▰` and
   the cell after it, when there is one, is `▱`. At 100% and at 150% no track cell is
   `▱`, and every row at 150% equals the same row at 100%.
6. Empty window: at 0% and at -5% no track cell is `▰` and the needle is in the first
   bottom-rail body cell. At 0.5% exactly one cell is filled.
7. Bottom rail: take the top rail drawn without a pace marker, swap `╭ ┬ ╮` for
   `╰ ┴ ╯`, and put `▲` in the needle column; the bottom rail equals that, cell for
   cell. Check it at 0%, 50% and 100% (where the needle replaces a tick) and at a
   percentage where it replaces a `─`. The ticks are in body cells `i*(width-3)/4` for
   `i` in 0–4, computed in the test.
8. Pace and needle: a gauge drawn with pace equal to the remaining fraction has its `▼`
   and its `▲` in the same column, each in its own style.
9. Styling: the bottom rail and the track caps use the frame style, the needle the
   needle style, the fill the band colour, spent cells the spent style, the pace marker
   its own style. For a rate-limited window both rails, the caps and the scale are faint
   red and the needle is still white.
10. `Plain` removes every escape sequence from every row.
11. No row of any block contains `◇` or `✕`, rails and scale included, rate-limited or
    not.
12. Kept from today and still passing: `MinWidth` is 22 and width 21 is refused; the top
    rail and the scale are byte-identical to today's at every width, with a pace marker,
    with `NoPace`, and with pace above 1 clamped.
13. `internal/dash/gauge/testdata/fixed-three.json` holds the old open gauge byte for
    byte. Regenerate it for the closed gauge and keep the test that reads it, so a full
    styled and plain block is still pinned.

`internal/dash/layout`:

14. For every thickness 1–9 a window is `N + 4` rows in the order above, each padded to
    the column width.
15. The percentage and the countdown are on track row `(N-1)/2` and nowhere else; the
    bottom rail row and the scale row have neither.
16. `[RL]` is on the top rail row for a rate-limited window.
17. Zero thickness in `Options` renders the same page as 3.
18. A two-window card and a one-window card in the same row get the same height, with
    the shorter content centred.
19. The seven goldens in `internal/dash/layout/testdata/` are regenerated and reviewed by
    eye: every window in every one must show the closed frame, with the needle on the
    bottom rail and no `▲` in a track row. (A golden review, not a test function.)
20. Test helpers that read the old shape are corrected, not worked around: for example
    `renderedGaugeWidths` in `layout_test.go` measures a gauge by pairing `┴` glyphs,
    which the bottom rail's ticks break. The existing fit, scroll, compact-layout,
    forecast-note, ledger, status, timeline and calendar tests stay and pass.

`internal/dash` (the model):

21. `TestThicknessModelResize` in `internal/dash/thickness_test.go` asserts the old shape
    (a `┴` and the `▲` on the last track row, the scale `N + 2` rows below the name).
    Rewrite it for the new shape: for each thickness the view drawn from `RunOptions`
    has `N` identical capped track rows, the bottom rail holding the `▲` at `N + 2` rows
    below the name, and the scale at `N + 3`, including after a resize. Any other model
    test that turns out to encode the old shape or the old window height is updated the
    same way and listed in the implementer's report. Model production code does not
    change.

Tests in `internal/dash/config` and `cmd/dash` need no change and must still pass
untouched.

## Docs

- `README.md`, Dashboard section and the `meter_thickness` paragraph: the track sits
  inside a closed frame and the needle is on the bottom rail, not the bottom track row.
- Every comment in `internal/dash/gauge/gauge.go` and `internal/dash/layout/layout.go`
  that draws the open gauge, names `┴` caps or counts a window's rows, the package
  comments and the comment on `windowBlock` included.
- `docs/specs/triple-track.md`: a notice at the top that its gauge pictures, window
  pictures and tests are historical, and marks on decisions 2, 3 and 4 of "The gauge"
  and decisions 6 and 8 of "Thickness" saying this spec supersedes them.
- `docs/screenshots/*.png` show an older gauge. Do not regenerate them; report that
  they are stale.

## Out of scope

A new flag or config key, a different default thickness, a different scale row, labels
inside a rail, regenerating screenshots, and any change to `qmeter pace`, the forecast
note, or piped and JSON output.
