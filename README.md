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

Run `qmeter` in a terminal to open the full-screen dashboard. The screen fills
the terminal and does not scroll. It has a one-line header with provider and
window counts and the fetch time, provider cards in a two-column grid, the R1
resets list beside the balances panel, and a one-line key bar. Each card has
one thick A5 bar for every usage window. The bar shows the remaining percentage
in its band colour, with the unused track dimmed. Its title shows the window,
percentage and reset countdown. Rate-limited windows use the rate-limit colour
and show `rate limited`.

The dashboard measures the available space and fits its parts in this order:

1. Full layout with provider cards, resets and balances.
2. Drop the resets list.
3. Drop the balances panel.
4. Use the compact layout, with one row per window. The bars grow as the
   terminal gets wider.
5. If the terminal is below 80 columns or 24 rows, show only a too-small
   message with the required size.

The full layout uses two columns when the terminal can fit them. Otherwise it
uses one column. At the 80x24 floor, the dashboard keeps the provider cards and
drops the panels if they do not fit. It uses the compact layout when the cards
alone do not fit. The `r` key refreshes all providers. The `q` key quits.
`--filter claude,codex` limits the dashboard to named providers; names can also
be repeated as separate flags.

When stdout is not a terminal, bare `qmeter` prints the same text as
`qmeter usage`. `qmeter --json` prints the usage JSON. A terminal that supports
colour gets coloured bars and status messages. Pipes and a nonempty `NO_COLOR`
variable produce plain text.

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

`refresh_interval` is the number of seconds between automatic refreshes. It
defaults to 60 and accepts values from 1 through 86400. The interval begins
after each fetch completes, so fetches never overlap. A manual `r` refresh
restarts the interval when that fetch completes.

If an older config contains a setting that qmeter removed, qmeter warns and
ignores that setting. Other valid settings still apply. A malformed file or an
unknown or invalid setting produces a warning; qmeter then ignores the file
and uses the defaults.

## qmeter usage

`qmeter usage` prints A5 provider cards with the same bar style as the
dashboard. It uses the terminal width, or 80 columns when output is piped.
One-shot output has no minimum-size rule. Use `--provider` to select one
provider. Without it, undetected providers are omitted.

```sh
qmeter usage
qmeter usage --provider claude
qmeter usage --json
```

The JSON output keeps the `windows`, `errors` and `undetected` arrays and every
existing window field, including `period_seconds`. Pipes and a nonempty
`NO_COLOR` variable produce plain text. Configured dashboard colours do not
change one-shot text.

## qmeter resets

`qmeter resets` prints the R1 list. It has one row per window, sorted by reset
time, soonest first. Each row shows the countdown, provider, window name, reset
date and time, and current percentage. Unknown reset times appear last.
Output uses the terminal width, or 80 columns when piped. One-shot output has
no minimum-size rule.

```sh
qmeter resets
qmeter resets --filter claude,codex
qmeter resets --json
```

The JSON output preserves the usage envelope and every existing window field,
including `period_seconds`. It adds `resets_in_seconds`, the whole seconds to
the reset, or `null` when the reset time is unknown.

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

`--filter` accepts comma-separated or repeated provider names, like `resets`;
undetected providers are omitted. USD amounts use `$` and two decimal
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
