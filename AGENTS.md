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

## Test-driven development

All Go changes are test-first. Write a failing test, run it and confirm it fails for the
right reason, write the minimal code to pass, refactor, repeat. Before declaring a task
done, run `gofmt -l .`, `go vet ./...`, and `go test -race ./...` and report the output.
Never weaken or skip a test to get green.

## Instructions

<!-- Add agent instructions below. -->
