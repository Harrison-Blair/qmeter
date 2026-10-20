# qmeter full-screen simplify

## Decisions

1. The dashboard fills the terminal. It never scrolls.
2. The screen has these parts, top to bottom:
   - A one-line header: the name, the fetch time, the next refresh. No banner wordmark.
   - The provider cards in a 2-column grid.
   - The R1 resets list and the balances panel, side by side.
   - A one-line key bar.
3. Bar style A5: one thick bar for each window that shows the remaining percentage in the band colour. The rest of the track is dim. Above the bar: the window name, the percentage, and "resets in X". A rate-limited window uses the rate-limited colour and shows "rate limited".
4. R1 resets list: one row for each window, soonest first. Columns: the countdown, the provider, the window, the reset date and time, the current percentage.
5. Remove the pace feature completely:
   - the `qmeter pace` command (`cmd/pace/`)
   - `internal/pace/`, including the forecast ("dry in")
   - the pace marker in the gauge
   - the pace columns in `qmeter usage` and in the ledger
   - the pace fields in the `--json` output
6. Remove the flags `--thickness`, `--vertical`, `--width`, and `--no-banner`, and the config keys that only they use. The banner goes away, so `--no-banner` has no use. Keep `--filter` and `--json`.
7. Small terminals:
   - The full layout needs about 93x39.
   - Below that, drop the R1 list first, then the balances panel.
   - Then use the terse compact layout: one row for each window, for example `fable weekly ━━━━━━━━ 96% 6d8h`. The bars get longer when the terminal is wider. The floor is about 80x24.
   - Below the floor, show one message with the needed size, as btop does.
8. `qmeter usage`, piped `qmeter`, and `qmeter resets` print text with the same renderer as the TUI: A5 bars and the R1 list.
9. Keep the `qmeter resets` and `qmeter spend` commands.
10. Fit by measurement at run time, not by fixed sizes. The order: the full layout, then drop R1, then drop balances, then the compact layout, then the "too small" message. When the terminal is too narrow for 2 columns and tall enough, use 1 column.
11. R1 has one row for each window. The groups on the board are only an example.
12. `qmeter usage` prints the A5 cards. `qmeter resets` prints the R1 list. The width is the terminal width, or 80 when the output is a pipe. One-shot output has no minimum-size rule.
13. Remove the timeline view and the calendar view. R1 replaces them. Keys: `r` refresh, `q` quit. No filter key. `--filter` stays.
14. An old config key (`meter_width`, `meter_thickness`, or another removed key) gives a warning and is ignored. The other settings stay in use.
15. The header shows "fetched HH:MM". No countdown to the next refresh.
16. The rate-limited Cursor `auto` window on the board is example data. No provider change.
17. Keep the provider and JSON semantics, except the pace removal. Do not remove other window metadata.
18. Use a hard 80x24 floor for every dashboard tier. Below 80 columns or below 24 rows, show only the too-small message with the needed size. Fit by measurement applies at 80x24 and above.

## User input

The user can quit at any time. `q` and `ctrl+c` work in every state, including during a fetch, a refresh, or a slow operation. A key press gets a visible response at once. The input loop never waits for I/O. Do slow work in a background command. Each TUI has a test that sends a quit key while a slow operation runs and checks that the program exits.

## Tasks

Tasks 49 to 53 landed and were released as v0.14.0. The hang repair is the next task.
