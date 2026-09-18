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
the window's period, a `▴` under the scale marks where the needle would sit if
the window were being spent evenly: a needle left of the marker is being spent
faster than even pace, one to its right slower. A rate-limited window is drawn
as a wall: its whole frame and its countdown turn red alongside the `[RL]`
badge, since the wait is then the only number that matters.

| Key | Does |
| --- | --- |
| `j` / `↓`, `k` / `↑` | scroll a line |
| `Space` / `PgDn`, `b` / `PgUp` | scroll a page |
| `g` / `Home`, `G` / `End` | jump to the top or the bottom |
| `r` | fetch every provider again |
| `q`, `Esc`, `Ctrl-C` | quit |

Two flags:

- `--filter claude,codex` shows only the providers named — comma-separated or
  repeated, out of `claude`, `codex`, `opencode-go` and `cursor`. Without it
  every provider is shown.
- `--no-banner` replaces the wordmark with the one-line summary header.

The dashboard needs a terminal. Piped or redirected, `qmeter` prints the same
table as `qmeter usage`, and `qmeter --json` prints the same JSON envelope;
both still honour `--filter`. Colour follows [`NO_COLOR`](https://no-color.org).

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
dashboard usable.

`refresh_interval` is the number of seconds between automatic refreshes. It
defaults to 60 and accepts values from 1 through 86400. The interval begins
after each fetch completes, so fetches never overlap; a manual `r` refresh
restarts the interval when that fetch completes.

The configuration is used only by the interactive dashboard, not by JSON or
piped output. If an existing file cannot be read or contains malformed,
unknown, or invalid settings, qmeter prints one warning naming the file, ignores
the whole file, and continues with all defaults.

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
