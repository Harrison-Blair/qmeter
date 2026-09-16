# Manual QA checklist

Everything qmeter does is unit-tested, but three things cannot be: what a real
vendor store looks like on disk, what macOS's `security(1)` actually prints and
exits with, and how paths and processes behave on real Windows. This checklist
is for a human to run once per platform, on hardware, before milestone 1 is
called done.

How to use it: work down your platform's section, tick each box, and record the
actual output next to anything that does not match. A mismatch is a bug report,
not something to fix in place — note it and open an issue.

Status:

- **Linux — runnable today.** CI runs on Linux, so this section is a smoke test
  of the real binary against real stores rather than a hunt for surprises.
- **macOS — UNTESTED ON HARDWARE.** The Keychain source (`internal/provider/claude`)
  is driven entirely through a fake subprocess runner in tests. Every assumption
  below is taken from `security(1)`'s documentation and stays unverified until
  someone runs it on a Mac.
- **Windows — UNTESTED ON HARDWARE.** The default credential paths are asserted
  by string comparison from a Linux runner (`paths_test.go` in each provider
  package). No part of qmeter has been run on Windows.

Build the binary first — `go build -o qmeter .` — and run the checklist
against that binary, not `go run`, so the `.exe`/permissions questions on
Windows are actually exercised.

## Linux

Run these with every `QMETER_*` override unset (`env -u QMETER_CLAUDE_TOKEN -u
QMETER_CODEX_TOKEN -u QMETER_CODEX_ACCOUNT_ID -u QMETER_CURSOR_TOKEN -u
QMETER_OPENCODE_GO_KEY ...`), so the local stores are what is being tested.
`QMETER_CODEX_ACCOUNT_ID` is only read alongside `QMETER_CODEX_TOKEN`, but a
stale value left in the shell would send a mismatched `ChatGPT-Account-Id`, so
clear it too.

- [ ] **No credentials at all.** With none of the four stores present (move them
  aside; do not delete them), `qmeter usage` prints exactly `no providers
  detected` — one line, no header row — and exits 0. Check with `echo $?`.
- [ ] **Empty JSON envelope.** In the same state, `qmeter usage --json` prints an
  envelope whose three keys are all present and all empty arrays, never `null`:
  `{"windows":[],"errors":[],"undetected":[]}`. Exit 0.
- [ ] **Unknown provider name.** `qmeter usage --provider clade` (a typo, not a
  real provider) prints a single line to **stderr** — nothing on stdout — and
  exits 1. Confirm the stream with `qmeter usage --provider clade 2>/dev/null`
  (no output) and `... 1>/dev/null` (the one line). A provider that exists but
  is merely not detected is exit 0, not this case.
- [ ] **A known provider, not logged in.** Move `~/.claude/.credentials.json`
  aside and run `qmeter usage --provider claude`: it prints the header row and a
  two-cell padded line whose cells are `claude` and `not detected: not logged in,
  run claude to log in` (the PROVIDER cell is padded to the widest cell
  in its column, the PROVIDER header included, so with claude the only row
  printed, claude is followed by four spaces), and exits **0** — not detected is
  not an error. `qmeter usage --json --provider claude` puts the same provider
  under `undetected`, with the text under a `reason` key (not `message`, which is
  what the `errors` array uses):
  `{"windows":[],"errors":[],"undetected":[{"provider":"claude","reason":"not
  logged in, run claude to log in"}]}`. Plain `qmeter usage` with no
  `--provider` must **not** print that line at all — undetected providers are
  silently omitted unless asked for by name.
- [ ] **Each provider, logged in.** Log in to each CLI in turn (`claude`,
  `codex`, `cursor-agent`, `opencode`) and run `qmeter usage --provider
  <claude|codex|cursor|opencode-go>`:
  - [ ] `claude` — reads `~/.claude/.credentials.json`; at least one window row,
    plan column matches your subscription.
  - [ ] `codex` — reads `~/.codex/auth.json`; rows appear and the plan name
    matches the ChatGPT plan. A store in API-key mode should say so rather than
    claim you are logged out.
  - [ ] `cursor` — reads `~/.config/cursor/auth.json` (lowercase `cursor`, the
    CLI store, not the desktop app's); exactly two windows, `total` then `auto`.
  - [ ] `opencode-go` — reads `~/.local/share/opencode/auth.json`; up to three
    windows in the order 5h, weekly, monthly, plan `go`.
  - [ ] Plain `qmeter usage` with several of them logged in prints the providers
    in registry order (claude, codex, opencode-go, cursor), windows in each
    provider's own order, and undetected providers not at all.
- [ ] **Percentages are plausible.** Cross-check one provider's number against
  the vendor's own UI. A number that is off by 100x means a percent/fraction
  mix-up that no fixture would catch.
- [ ] **`go install` produces `qmeter`.** From the repo root, `go install .`
  writes a binary named exactly `qmeter` (not `cmd`) into
  `$(go env GOPATH)/bin`; check with `ls "$(go env GOPATH)/bin/qmeter"` and run
  `qmeter version`.

## macOS

**UNTESTED ON HARDWARE.** These five items are exactly the assumptions the
Keychain credential source is built on. Run them on a Mac with Claude Code
logged in. Items 1-3 are about `security(1)` itself; run them in a terminal
before running qmeter at all.

- [ ] **1. Item-not-found is exit 44.** With no such item in the Keychain,
  `security find-generic-password -s "Claude Code-credentials" -w; echo $?`
  exits **44**, and Go sees that as an error whose message is exactly
  `exit status 44`. qmeter matches that text to decide a lookup is a miss; any
  other code is treated as a real failure.
- [ ] **2. `-w` prints raw JSON plus a trailing newline.** With the item
  present, the same command prints the credential JSON as-is followed by `\n`
  (`| xxd | tail -1` to see the final byte). The parser tolerates `\n` and
  `\r\n`. If the secret ever comes back as `security`'s hex-dump form instead,
  that is a hard parse error with no file fallback — by design — so note it.
- [ ] **3. A denied prompt is NOT 44.** Run the command and click **Deny** on
  the Keychain prompt: the exit status must be non-44 (51 expected). This is the
  load-bearing one. If Deny ever exits 44, a user who denied the prompt is told
  "not logged in, run claude to log in", which is wrong and misleading.
- [ ] **4. Two Keychain reads per run.** `qmeter usage` resolves the credential
  twice — once in Detect, once in Fetch — so expect up to **two** prompts per
  run unless you click "Always Allow". Confirm two prompts appear and decide
  whether that is acceptable; if not, file a follow-up to cache the resolve for
  the duration of a run.
- [ ] **5. Denied prompt with the file present.** With `~/.claude/.credentials.json`
  on disk, deny the Keychain prompt: qmeter silently falls back to the file and
  prints usage normally. This is intended. With the file **absent** and the
  prompt denied, the output must report the Keychain failure — not "not logged
  in".

## Windows

**UNTESTED ON HARDWARE.** Run in PowerShell with the four CLIs logged in where
possible.

- [ ] **Each store path resolves under `%USERPROFILE%`.** None of these are
  `%APPDATA%`-relative. Confirm the file qmeter reads is the file the vendor
  wrote, for each of:
  - [ ] Claude — `%USERPROFILE%\.claude\.credentials.json`
  - [ ] Codex — `%USERPROFILE%\.codex\auth.json`
  - [ ] Cursor CLI — `%USERPROFILE%\.config\cursor\auth.json`
  - [ ] OpenCode Go — `%USERPROFILE%\.local\share\opencode\auth.json`
- [ ] **`%USERPROFILE%` is what is actually read.** Point `%USERPROFILE%` at a
  scratch directory holding a copy of one store, run `qmeter usage --provider
  <that one>`, and confirm the copy is what was read.
- [ ] **The `.exe` runs.** `go build -o qmeter.exe .`; `.\qmeter.exe version`
  and `.\qmeter.exe usage` both run from PowerShell and from `cmd.exe`, with no
  missing-DLL or SmartScreen blocker worth documenting.
- [ ] **No Keychain path is taken.** The macOS Keychain source is gated on
  `runtime.GOOS == "darwin"`, so nothing spawns `security` here. Confirm no
  unexpected subprocess or prompt appears.
- [ ] **The test suite does not fail on `sleep`/`echo`.** `go test ./...` on
  Windows: the `internal/lib/subprocess` tests that need `echo` or `sleep`
  **skip** (they call `exec.LookPath` first) rather than fail. A failure there is
  a test bug, not a Windows bug — report it.
- [ ] **Path separators in messages.** Error lines that embed a store path (a
  missing or unusable store) show backslash-separated Windows paths, not forward
  slashes.
