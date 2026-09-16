# AGENTS.md

> **Note:** This file is the single source of truth for all agent instructions in this
> repository. `CLAUDE.md` contains only an `@AGENTS.md` import so Claude Code picks up
> the same instructions. Do not add instructions to `CLAUDE.md` directly; edit this
> file instead.

## Project

qmeter is a Go CLI tool to see your AI subscription usage limits.

## .gitignore policy

`.gitignore` is allowlist-style: it ignores everything (`*`) and then explicitly allows
Go source, `go.mod`/`go.sum`, Markdown docs, `LICENSE`, and `.github/`. Keep it small.
Only add a new `!` allow rule when a file the project genuinely needs is being ignored,
and add the narrowest pattern that covers it. Never remove the leading `*`.

## Branching

Develop on `dev` or feature branches off `dev`. Changes reach `main` only through a pull request, which the owner approves. Never commit directly to `main`.

## Instructions

<!-- Add agent instructions below. -->
