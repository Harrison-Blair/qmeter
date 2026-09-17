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
is left of the window, and how long until it resets.

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
