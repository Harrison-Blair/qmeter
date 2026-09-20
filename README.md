```text
                         _
  __ _  _ __ ___    ___ | |_  ___  _ __
 / _` || '_ ` _ \  / _ \| __|/ _ \| '__|
| (_| || | | | | ||  __/| |_|  __/| |
 \__, ||_| |_| |_| \___| \__|\___||_|
    |_|
```

[![Version](https://img.shields.io/github/v/release/Harrison-Blair/qmeter?label=version)](https://github.com/Harrison-Blair/qmeter/releases/latest)

A CLI tool to see your AI subscription usage limits

## Supported providers

- Claude
- Codex
- OpenCode Go
- Cursor

## Dashboard

`qmeter` on its own opens a live dashboard: the wordmark pinned at the top, and
under it a fuel gauge for every usage window of every provider it detects — what
is left of the window, and how long until it resets. When the provider reports
the window's period, a bold bright-cyan `▼` on the top border points down at where
the white `▲` needle would sit if the window were being spent evenly: a needle left of the marker is being spent
faster than even pace, one to its right slower. Each limit name includes a pace
badge: orange `[behind]`, green `[on pace]`, yellow `[ahead]`, or gray `[n/a]`.
These use the same remaining-allowance calculation as `qmeter pace`. A rate-limited window is drawn
as a wall: its frame and countdown turn red alongside the `[RL]` badge;
the pace marker stays cyan and the actual needle stays white. The footer ends
with the time the numbers on screen were fetched, and shows a spinner while
the next fetch is in flight.

Reported balances appear as short ledger rows below each provider's gauges,
using the same fetch. Providers with no balances get no ledger rows.

| Key | Does |
| --- | --- |
| `j` / `↓`, `k` / `↑` | scroll a line |
| `Space` / `PgDn`, `b` / `PgUp` | scroll a page |
| `g` / `Home`, `G` / `End` | jump to the top or the bottom |
| `r` | fetch every provider again |
| `t` | toggle gauges and the reset timeline |
| `q`, `Esc`, `Ctrl-C` | quit |

Dashboard flags:

- `--filter claude,codex` shows only the providers named — comma-separated or
  repeated, out of `claude`, `codex`, `opencode-go` and `cursor`. Without it
  every provider is shown.
- `--no-banner` replaces the wordmark with the one-line summary header.
- `--vertical` stacks providers in one full-width column and stretches meters
  to the terminal width, leaving 14 cells for percentages, spacing, and countdowns.

The dashboard needs a terminal. Piped or redirected, `qmeter` prints the same
table as `qmeter usage`, and `qmeter --json` prints the same JSON envelope;
both still honour `--filter` and ignore `--vertical`. Colour follows [`NO_COLOR`](https://no-color.org).

The `usage` and `pace` text tables use provider colors, remaining-allowance bands,
cyan reset countdowns (red when rate limited), and colored status messages. Pace
labels use the badge colors without brackets. Styling is enabled only when the
output destination is a terminal that supports color. Pipes, files, JSON, and a
nonempty `NO_COLOR` environment variable produce plain output. CLI tables use
the default provider colors; dashboard color configuration stays dashboard-only.

### Configuration

The interactive dashboard reads `qmeter/config.toml` from the operating
system's user configuration directory:

- Linux and other Unix systems: `$XDG_CONFIG_HOME/qmeter/config.toml`, or
  `~/.config/qmeter/config.toml` when `XDG_CONFIG_HOME` is unset
- macOS: `~/Library/Application Support/qmeter/config.toml`
- Windows: `%AppData%\qmeter\config.toml`

qmeter does not create this file. Every setting is optional; a partial file is
merged with the built-in defaults.

```toml
meter_width = 50
refresh_interval = 60

[colors.claude]
light = "#A64526"
dark = "#D97757"

[colors.codex]
light = "#0B6F57"
dark = "#10A37F"

[colors.opencode-go]
light = "#656363"
dark = "#B7B1B1"

[colors.cursor]
light = "#26251E"
dark = "#EDECEC"
```

`light` is used on a light terminal background and `dark` on a dark terminal
background. Colours must be six-digit hex values. `NO_COLOR` takes precedence
over the configured palette and disables colour output.

`meter_width` is the preferred complete meter width in terminal cells, from 22
through 200. Meters grow toward that target, use one or two columns according
to the available terminal width, and shrink only when necessary to keep the
dashboard usable. `--vertical` overrides `meter_width`: meters fill the available
width even when it exceeds 200 cells. Without `--vertical`, the configured
preference still applies.

`refresh_interval` is the number of seconds between automatic refreshes. It
defaults to 60 and accepts values from 1 through 86400. The interval begins
after each fetch completes, so fetches never overlap; a manual `r` refresh
restarts the interval when that fetch completes.

The configuration is used only by the interactive dashboard, not by JSON or
piped output. If an existing file cannot be read or contains malformed,
unknown, or invalid settings, qmeter prints one warning naming the file, ignores
the whole file, and continues with all defaults.

## Pace

`qmeter pace` lists each detected usage limit and compares remaining allowance with
how much of its period remains. The table shows `PROVIDER`, `WINDOW`, `PACE`,
`REMAINING`, `EXPECTED`, `RUNS OUT`, and `RESETS`.
`RUNS OUT` projects average consumption: `in 2h10m` when allowance runs out before
reset, `empty` when already exhausted or rate limited, and `-` otherwise. Forecasts
require valid timing and 5% elapsed, except already-dry windows need only valid timing.

- **ahead**: remaining allowance is more than 5 percentage points below expected; allowance is being used faster.
- **on pace**: remaining allowance is within 5 percentage points of expected, including both boundaries.
- **behind**: remaining allowance is more than 5 percentage points above expected; allowance is being used slower.
- **n/a**: the provider does not report a reset and positive period, or the
  current time is outside that period. Expected remaining allowance is shown as `-`.

The baseline spreads usage evenly across continuous elapsed time, including
nights and weekends. A period begins at its reset time minus its reported
length. Expected remaining allowance is the percentage of that period left until
reset. Labels use unrounded values; the table displays one decimal place.
Exhaustion and rate limiting do not override the pace calculation. `n/a` means
there is insufficient timing information, not necessarily a nonrecurring plan.

```sh
qmeter pace
qmeter pace --filter claude,codex
qmeter pace --filter claude --filter codex --json
```

`--filter` follows the dashboard's comma-separated or repeated syntax; undetected
providers are omitted. `--json` retains the usage envelope (`windows`, `errors`,
`undetected`) and existing window fields (including `remaining_percent`), adding `pace` and
`expected_remaining_percent` to each window. The expected percentage is `null` for
`n/a`; all three envelope arrays are present even when empty.
Each pace window also includes `projected_exhaustion_at` (RFC 3339, or `null`)
and `projected_remaining_at_reset` (number, or `null`). Both are `null` without a
forecast; survivors have no exhaustion time, while dry or empty windows land at 0%.
These fields are exclusive to `pace --json`.

## Reset timeline

`qmeter resets` shows what frees up next, sorted by reset time. On terminals at
least 70 cells wide, each window appears on a shared linear axis from now to
seven days. The provider glyph marks its reset, colored by remaining allowance;
`▸` marks a reset beyond seven days. Unknown reset times sort last and have no
marker. An `↑RL` label identifies a reset that lifts a rate limit.
Narrow terminals and piped output use a plain `PROVIDER WINDOW REMAINING RESETS`
table. The dashboard's `t` key toggles the same timeline; refresh and scrolling
work on both pages, and each launch starts with gauges.

```sh
qmeter resets
qmeter resets --filter claude,codex
qmeter resets --filter claude --filter codex --json
```

`--json` preserves the usage envelope and window fields, sorts its windows, and
adds `resets_in_seconds` (whole seconds until reset, or `null` when unknown).

## Spend

`qmeter spend` shows reported balances in a `PROVIDER NAME LEFT OF BAR` table.
`LEFT` is the remaining amount, `OF` is the limit, and the 20-cell bar shows the
remaining fraction when both amounts and a positive limit are known. Unknown
amounts show `-`; unlimited credit shows `unlimited`. Providers with no balances
are omitted; fetch failures appear as status rows.

```sh
qmeter spend
qmeter spend --filter claude,codex
qmeter spend --filter claude --filter cursor --json
```

`--filter` accepts comma-separated or repeated provider names, like `pace` and
`resets`; undetected providers are omitted. USD amounts use `$` and two decimal
places, credits use plain numbers, and percentage balances use `%`. Cursor's
amount units are **unconfirmed** and appear as bare numbers, never dollars.
Pipes and `NO_COLOR` produce plain output.

`--json` emits exactly `balances`, `errors`, and `undetected`, each a non-null
array. Balance fields follow this shape; unknown amounts are `null`, while a
reported zero stays `0`:

```json
{"balances":[{"provider":"codex","name":"credits","unit":"credits","used":null,"limit":null,"remaining":0,"unlimited":false}],"errors":[],"undetected":[]}
```

Errors use `{ "provider": "...", "message": "..." }`; undetected entries use
`{ "provider": "...", "reason": "..." }`. The existing `usage --json` envelope
is unchanged.

## Install

Linux and macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/Harrison-Blair/qmeter/main/install.sh | bash
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/Harrison-Blair/qmeter/main/install.ps1 | iex
```

With Go:

```sh
go install github.com/Harrison-Blair/qmeter@latest
```

Release archives for every supported platform are on the
[Releases](https://github.com/Harrison-Blair/qmeter/releases) page, and
`qmeter update` upgrades an existing install however it was installed.
