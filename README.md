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

### Credential discovery

qmeter reads credentials from each provider's native store, with Pi as a
fallback for compatible subscriptions:

| qmeter provider | Pi entry in `auth.json` | Credential |
| --- | --- | --- |
| `claude` | `anthropic` | Claude subscription OAuth |
| `codex` | `openai-codex` | ChatGPT subscription OAuth |
| `opencode-go` | `opencode-go` | OpenCode Go API key |

Pi's store defaults to `~/.pi/agent/auth.json`. `PI_CODING_AGENT_DIR` overrides
the directory; qmeter appends `auth.json` and expands a leading `~` like Pi.
Stored API keys can be literals or use `$VAR` / `${VAR}` references. Nonempty
values in the credential's `env` object take precedence over the process
environment. `$$` and `$!` escape a dollar sign and exclamation mark.
Command-based keys (`!command`) are unsupported; use `QMETER_OPENCODE_GO_KEY`
with the resolved key instead.

Working native credentials are preferred. OpenCode Go checks `opencode.db`
before the older `auth.json` store, with Pi following both native stores.
If native credentials cannot be
loaded, are expired, or the usage API rejects them with `401` or `403`, qmeter
tries Pi. Rate limits, server errors, and timeouts do not trigger another
credential attempt. Each subscription appears once under its existing provider
name; when the stores belong to different accounts, the working native account
wins. Claude credentials from Pi carry no plan name, so that field remains empty.

Explicit `QMETER_CLAUDE_TOKEN`, `QMETER_CODEX_TOKEN` (with optional
`QMETER_CODEX_ACCOUNT_ID`), and `QMETER_OPENCODE_GO_KEY` overrides remain
authoritative, including when a request fails. qmeter reloads credentials on
refresh and never writes or refreshes OAuth tokens itself; expired Pi tokens
produce an `open pi to refresh` hint. Pi discovery covers this auth store and
its key references, rather than custom extensions or `models.json`. Ordinary
Anthropic/OpenAI API keys and OpenCode Zen keys are not subscription credentials
for the providers above. Cursor continues using its native credential sources.

## Dashboard

`qmeter` on its own opens a live dashboard: the wordmark pinned at the top, and
under it a fuel gauge for every usage window of every provider it detects — what
is left of the window, and how long until it resets. The track is three rows tall,
inside a closed frame, with the white `▲` needle on the bottom rail. When the provider reports
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
| `t` | toggle timeline and gauges; switch directly from calendar to timeline |
| `c` | toggle calendar and gauges; switch directly from timeline to calendar |
| `q`, `Esc`, `Ctrl-C` | quit |

Dashboard flags:

- `--filter claude,codex` shows only the providers named — comma-separated or
  repeated, out of `claude`, `codex`, `opencode-go` and `cursor`. Without it
  every provider is shown.
- `--thickness N` sets the number of track rows from 1 through 9 (default 3).
  It overrides `meter_thickness` only when passed.
- `--no-banner` replaces the wordmark with the one-line summary header.
- `--vertical` stacks providers in one full-width column instead of the
  automatic one-or-two-column grid.

The dashboard draws framed provider cards in one or two columns, with meters
filling each card's inner width, leaving 18 cells for the frame, percentages,
spacing and countdowns. Windows and status rows stay packed together and
centred vertically; spare body rows are shared equally between card rows. A
lone last card spans the full width. Below 40 columns, compact unframed
sections have spare rows shared above, between and below them. In the
timeline, spare height is shared between full-width provider cards, with
packed rows centred inside each card. In the calendar, the day grid grows so
its bottom strip stays at the bottom of the body. Text and meter thickness
stay the same; content that cannot fit remains scrollable. The banner keeps
its normal width-based fallback even in short terminals.

The dashboard needs a terminal. Piped or redirected, `qmeter` prints the same
table as `qmeter usage`, and `qmeter --json` prints the same JSON envelope;
both still honour `--filter` and ignore `--vertical` and thickness. An out-of-range `--thickness` still fails
before fetching. Colour follows [`NO_COLOR`](https://no-color.org).

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
meter_thickness = 3
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
through 200. It decides when two columns fit: the dashboard uses two columns
only when each card's inner width can hold 80% of the target, accounting for
the card frames. Meters then stretch to the cards' inner width, beyond the
configured preference if space permits, and a lone last card fills the page
width. `--vertical` ignores `meter_width` and always uses one framed column
(unframed below 40 columns). Timeline and calendar widths follow their own
layouts regardless of `meter_width` or `--vertical`.

`meter_thickness` sets the number of track rows from 1 through 9, defaulting
to 3. The track sits inside a closed frame and the needle stays on the bottom rail; the percentage and countdown sit
on row `(N-1)/2`, counting from zero. `--thickness N` overrides this setting
for one run. An invalid config value warns and uses the built-in defaults.

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
table. This CLI output is independent of the dashboard views.

In the dashboard, `t` opens a timeline of full-width provider cards under one
shared now-to-seven-days ruler. Bars end at provider-coloured reset markers,
with countdowns beside them. Weekday labels and vertical guides mark local
midnights; below 70 columns this view uses the plain reset table. Spare body
rows are shared between the cards and each provider's packed rows are centred.

The dashboard's `c` opens a calendar starting with today's local date. It shows
`min(8, (width+1)/15)` days, each at least 14 cells wide, separated by one cell;
column width is `(width+1)/days - 1` using integer division. Each entry shows the
local reset time, provider glyph, remaining allowance, window name and countdown.
Already-due windows appear in today. Resets after the visible days, unknown reset
times and provider messages appear in a bottom strip. The day columns extend
to fill the body and keep that strip at the bottom. The calendar needs at
least 36 columns.

Pressing the active view's key returns to gauges; pressing the other view's key
switches directly. Each switch returns to the top. Refresh, scrolling and the
pinned banner/footer work in all three views, and each launch starts with gauges.
Both reset views ignore balances and remain scrollable when their content is
taller than the available body.

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
