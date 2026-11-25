# AGENTS.md

> **Note:** This file is the single source of truth for all agent instructions in this
> repository. `CLAUDE.md` contains only an `@AGENTS.md` import. Do not add instructions
> to `CLAUDE.md`. Edit this file.

## Project

qmeter is a Go CLI that shows AI subscription usage limits for Claude, Codex, OpenCode Go,
and Cursor. The bare `qmeter` command opens a full-screen dashboard.

## Commands

- Full check: `gofmt -l .`, `go vet ./...`, and `go test -race ./...`.
- Version-rule test: `bash .github/scripts/next-version_test.sh`.
- Build and install: `go install .`. This command makes a binary with the name `qmeter`.

## Workflow

- Make each branch from `main`.
- Changes go into `main` only through a pull request. The owner approves the merge.
- Write each Go change test-first:
  1. Write a test.
  2. Run the test. Make sure that it fails for the correct reason.
  3. Write the minimum code that makes the test pass.
  4. Refactor.
- Before you report a task as complete, run the full check and report the output.
- Do not weaken or skip a test to make the check pass.
- A merge to `main` that changes a `.go` file, `go.mod`, or `go.sum` makes a release
  through `.github/workflows/release.yml`. Other merges do not make a release.
- The release version comes only from git tags. The first release is `v0.1.0`. Each
  subsequent release increases the minor number and sets the patch number to zero.
  `.github/scripts/next-version.sh` contains this rule.
- Do not store a version in the repository. The release never makes a commit to `main`.
- If a tag already points to the merge commit, the workflow does not make a release.

## Layout

- `main.go` only calls `cmd.Execute()`.
- `cmd/` contains only command wiring. Each subcommand has its own folder `cmd/<name>/`.
- Logic and its tests go in `internal/`. Each subcommand has its own folder
  `internal/<name>/`.
- The root command opens the dashboard. Its wiring is in `cmd/dash/`. Its logic is in
  `internal/dash/`.
- Each provider implementation has its own folder `internal/provider/<name>/`.
- Each shared helper package has its own folder `internal/lib/<name>/`.
- Put related shared packages below one parent folder. Add a package directly below
  `internal/` only for a subcommand.

## Safety

Do not run the `claude`, `codex`, `pi`, or `cursor-agent` CLIs. Do not read, copy, or
change `~/.claude`, `~/.codex`, `~/.pi`, or a different vendor credential store. The vendors
rotate refresh tokens. A refresh in a copy of a store makes the token in the real store
unusable, and the user must log in again. This occurred two times on 2026-11-25.

- Tests use `subprocess.Fake` for subprocesses and temporary directories for credential
  stores.
- A provider package that runs a vendor CLI sets an empty `PATH` in `TestMain`.
- A live check with real credentials needs approval from the user before you start it.
- qmeter code never writes to a vendor credential store.

## TUI rules

- `q` and `ctrl+c` quit in every state.
- Each key press gets a visible response immediately.
- The input loop never waits for I/O. Run slow work in a background command.
- Each TUI has a test that sends a quit key while slow work runs. The test checks that
  the program exits.

## .gitignore policy

`.gitignore` uses an allowlist. It ignores all files (`*`). Then it allows Go source,
`go.mod`, `go.sum`, Markdown files, `LICENSE`, `.gitignore`, `.github/`, `install.sh`,
`install.ps1`, `**/testdata/**`, and `docs/screenshots/*.png`. Do not remove the first
`*` rule. Add a `!` rule only
when the project needs a file that `.gitignore` ignores. Use the narrowest pattern
that includes the file.
