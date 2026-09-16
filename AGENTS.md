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

## Releases

Merging to `main` cuts a release automatically: `.github/workflows/release.yml` re-runs
the format, build, and test checks and then publishes a tag, a GitHub release with
generated notes, and cross-compiled archives. The version comes from the git tags alone —
the first release is `v0.1.0`, and every later merge bumps the minor and resets the patch
(`v1.4.7` → `v1.5.0`), as implemented by `.github/scripts/next-version.sh`. No version is
stored in the repo and nothing is ever committed back to `main`; if the merged commit is
already tagged, the release is skipped. The version rule is covered by
`bash .github/scripts/next-version_test.sh`, which `.github/workflows/test.yml` runs
alongside the Go tests, so it gates both pull requests and releases.

## Test-driven development

All Go changes are test-first. Write a failing test, run it and confirm it fails for the
right reason, write the minimal code to pass, refactor, repeat. Before declaring a task
done, run `gofmt -l .`, `go vet ./...`, and `go test -race ./...` and report the output.
Never weaken or skip a test to get green.

## Layout

The module root's `main.go` is the installable entrypoint and only calls
`cmd.Execute()`, so `go install .` produces a binary named `qmeter`. `cmd/` is a
library package holding the root command and the per-subcommand wiring. Every
subcommand gets its own folder under `cmd/<name>/` with at least one file that
defines the cobra command and calls into `internal/`. `internal/` mirrors that:
one folder per subcommand under `internal/<name>/`, plus folders for shared
logic. Command files hold wiring only; logic and its tests live in `internal/`.

## Instructions

<!-- Add agent instructions below. -->
