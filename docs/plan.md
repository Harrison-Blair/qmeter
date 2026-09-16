# qmeter usage-limit providers: implementation plan

## How to use this document

This is a working checklist, not permanent documentation. It lives on `dev`, tracks
progress with checkboxes, and gets **deleted once every milestone-1 unit below has
landed; the backlog section moves to a GitHub issue**. Each unit is one feature branch
off `dev`, merged to `dev` by PR; check the box then. The milestone ships to `main` as
one `dev` -> `main` PR. (Owner decision, 2026-09-16: a prior milestone 2 — token
refresh with write-back — was planned and then removed entirely before any unit
landed; read-only credentials are the final behavior. See "Command surface and
output.")

## Command surface and output (owner decisions, do not reopen)

- `qmeter usage` shows all detected providers. `qmeter usage --provider
  claude|codex|cursor|opencode-go` shows one. Root command with no args is reserved for
  a future TUI — out of scope here.
- Text output: one line per window — provider, window name, plan name (if available),
  percent used, time until reset. `--json` emits the normalized structure instead.
  RULING: `--json` is a persistent flag on the root command
  (`root.PersistentFlags().Bool("json", false, ...)` in `cmd/root.go`), read by
  `cmd/usage`; `--provider` stays a local flag on `usage`.
- RULING (exact text output, golden-tested in U12; this is the real output of
  `tabwriter.NewWriter(out, 0, 8, 2, ' ', 0)`, verified against the actual
  `text/tabwriter` algorithm, not hand-aligned):
  ```
  PROVIDER     WINDOW   PLAN  USED    RESETS
  claude       5h       max   42.0%   in 2h13m
  claude       weekly   max   18.5%   in 3d4h
  opencode-go  monthly  go    100.0%  in 12d  (rate limited)
  cursor       total    free  4.5%    in 7d22h
  codex        error: token expired, open codex to refresh
  ```
  Rules: renderer config is `tabwriter.NewWriter(out, 0, 8, 2, ' ', 0)` (minwidth 0,
  tabwidth 8, padding 2, pad with spaces, no flags); the header row is printed on
  every run except when nothing is detected; data rows are written as
  `provider\twindow\tplan\tused\tresets\n` (the RESETS cell is not tab-terminated, so
  it is never padded, and the `(rate limited)` suffix is appended to that same cell,
  separated from the RESETS value by two literal spaces); plan column is `-` when
  empty; percent is one decimal place with `%`; RESETS is `in <d>d<h>h` / `<h>h<m>m` /
  `<m>m` (largest two non-zero units), or `-` when `ResetsAt` is zero; failure and
  not-detected lines are written as `provider\tmessage\n` — two cells, no trailing
  tab — where `message` is `error: <text>` or `not detected: <reason>`; a
  non-tab-terminated last cell is excluded from `text/tabwriter`'s column-width
  computation, which is exactly why the long `codex` message in the block above does
  not widen the WINDOW column, while the `codex` row's own PROVIDER cell is still
  padded for real, to the widest provider name printed that run, like every other
  row; providers print in registry order and windows in provider-declared order; when
  nothing is detected, print only `no providers detected` — no header row — and exit
  0; exit 1 only when `--provider` names an unknown provider. Final failure-line
  wording (owner decision, no refresh path exists; the examples below are shown
  unpadded, outside full-block context — see the golden block for real padding):
  `ErrTokenExpired` renders as `<provider>  error: token expired, open <tool> to
  refresh` (e.g. `claude  error: token expired, open claude to refresh`, shown
  unpadded); `ErrNotLoggedIn` renders as `<provider>  error: not logged in, run
  <tool> to log in` (e.g. `codex  error: not logged in, run codex to log in`, shown
  unpadded).
- RULING (undetected providers): undetected providers are silently omitted from the
  default (`qmeter usage`, no `--provider`) listing — they never produce a line and
  never fail the run. When `--provider` names a provider that exists but was not
  detected, print `<provider>  not detected: <reason>` (a two-cell row, padded the
  same way as a failure line) and exit 0 — this is not an error. Exit 1 is reserved
  for `--provider` naming a provider that doesn't exist at all.
- RULING (JSON shape, U1 + U12): `Window` defines `func (w Window) MarshalJSON()
  ([]byte, error)` — not struct tags, since `time.Time`/`time.Duration` don't
  serialize the way tags would imply — emitting `provider`, `name`, `plan` (omitted
  when empty), `used_percent`, `resets_at` (RFC3339, omitted when
  `ResetsAt.IsZero()`), `period_seconds` (`int64(w.Period / time.Second)`, omitted
  when zero), `rate_limited`. `--json` emits an envelope with three always-present
  keys, arrays never null: `{"windows":[...],"errors":[{"provider":"codex",
  "message":"token expired, open codex to refresh"}],"undetected":[{"provider":
  "codex","reason":"not logged in, run codex to log in"}]}` — `errors` items have
  `provider`/`message`, `undetected` items have `provider`/`reason` (undetected items
  serialize `ProviderError.Message` under the key `reason`; errors items under the
  key `message`).
- Each provider fails independently: one failure line per provider, the rest still
  print. Providers are fetched concurrently with a per-provider timeout.
- Credentials are read-only, permanently. Order: per-provider env var override, then
  the vendor's local store. Missing/expired credentials produce one clear failure line
  (see the exact wording ruling above) and qmeter never writes to a vendor store.
  (Owner decision, 2026-09-16, overrides earlier plans: a milestone 2 — refresh with
  atomic write-back for Claude, Codex, and Cursor, behind an opt-in `--refresh` flag —
  was scoped and then removed before implementation started. There is no refresh
  code, no `Refresher` interface, no `--refresh` flag, and no write-back anywhere in
  this plan. If refresh is ever wanted again, it re-enters as new units appended to
  this document.)
- Platforms for v1: Linux, Windows, macOS. macOS Claude credentials come from the
  Keychain via a subprocess abstraction (fake in tests); this path is marked untested on
  real hardware until someone runs it there.
- Dependencies stay stdlib + cobra. `modernc.org/sqlite` (pure Go, no cgo) is added only
  when the Cursor desktop SQLite fallback unit is implemented.
- All tests are TDD, table-driven, served from `httptest` fixtures. Tests never call
  live endpoints. Credential readers take an injectable base dir / file path so tests
  use temp dirs instead of the real home directory.
- The internal API stays TUI-friendly: providers return structs, rendering is a
  separate concern from fetching.

## Normalized data model and provider interface

Lives in `internal/provider` (its own package, imported by every provider package and
by the orchestrator — this avoids an import cycle between the orchestrator and the
provider packages).

```go
type Window struct {
    Provider    string        // wire form: "provider" (see MarshalJSON below, not struct tags)
    Name        string        // "name"; e.g. "5h", "weekly", "monthly", "sonnet weekly"
    Plan        string        // "plan", omitted when empty
    UsedPercent float64       // "used_percent"
    ResetsAt    time.Time     // "resets_at" (RFC3339), omitted when zero
    Period      time.Duration // "period_seconds" (whole seconds), omitted when zero
    RateLimited bool          // "rate_limited"
}

// MarshalJSON is Window's wire-form encoder (not struct tags — time.Time and
// time.Duration don't serialize the way tags would imply). See the JSON ruling
// in "Command surface and output" for the exact field list and omission rules.
func (w Window) MarshalJSON() ([]byte, error)

type Provider interface {
    ID() string
    Detect(ctx context.Context) (bool, string) // ok, reason
    Fetch(ctx context.Context) ([]Window, error)
}
```

`ID()` returns exactly one of `"claude"`, `"codex"`, `"cursor"`, `"opencode-go"`.
`Detect` returns `true` when a credential is resolvable without network I/O — env
override set, or the vendor store exists and parses; its `string` is the user-facing
reason shown when `false`, and must NOT include the provider name as a prefix (the
caller already knows which provider it asked) — e.g. `"not logged in, run claude to
log in"`, not `"claude: not logged in, run claude to log in"`. The `<tool>` names used
in every hint are: `claude`, `codex`, `cursor-agent`, `opencode`.

Typed errors, all defined in `internal/provider` (U1): `ErrNotLoggedIn` and
`ErrTokenExpired` carry the "open <tool>" hint text; `ErrRateLimited` carries a
`RetryAfter time.Duration`. These are what credential lookup and HTTP 401/403/429
handling return; `internal/usage` maps them to the rendered failure-line text (see
"Command surface and output"). `internal/provider` is a leaf with respect to its own
subpackages: it imports nothing under `internal/provider/*` and nothing from
`internal/usage`; the four-provider registry lives in `internal/usage`, never here.

## Package layout (per AGENTS.md Layout rule; owner layout decision applied)

`cmd/` holds wiring only and calls into `internal/`; `internal/` mirrors `cmd/`
one-to-one plus folders for shared logic. Owner decision: each provider nests under
`internal/provider/<name>`, and shared credential/HTTP helpers nest under
`internal/lib/<name>`. These shared-logic folders are grouped two directories deep
(`internal/provider/*`, `internal/lib/*`) rather than one — this stays within the
Layout rule's "plus folders for shared logic," it just organizes those folders by
what they share (a provider contract, or a stdlib/HTTP helper) instead of leaving
them flat under `internal/`:

```
cmd/
  main.go, root.go            (existing; root.go gains the persistent --json flag)
  version/                    (existing)
  usage/
    usage.go                  cobra command: flag --provider; wiring only; reads the
                              root-level --json flag
    usage_test.go

internal/
  version/                    (existing)
  provider/                   normalized model, Provider interface, typed errors
    claude/                   Claude provider (package claude)
    codex/                    Codex provider (package codex)
    opencodego/               OpenCode Go provider (package opencodego)
    cursor/                   Cursor provider (package cursor)
  lib/
    credstore/                env-var-override -> file-loader -> typed-error-with-hint (package credstore)
    subprocess/               fake-able exec.Command wrapper, macOS Keychain shell-out (package subprocess)
    httpx/                    context-aware GET/POST + JSON decode + status mapping (package httpx)
  usage/                      orchestrator: registry, concurrent fetch, text/json render
    usage.go
    registry.go               concrete provider registry (order: claude, codex, opencode-go, cursor)
    render.go
    usage_test.go
```

Package graph: `cmd/usage` -> `internal/usage` -> `internal/provider/*` -> `internal/provider`,
`internal/lib/*`; plus the direct edge `internal/usage` -> `internal/provider`.
`internal/usage` imports `internal/provider` (for `Window`, `Provider`, the typed
errors) and the four `internal/provider/<name>` subpackages (to build the registry in
U12) — and nothing under `internal/lib/*`; only the provider packages import
`internal/lib/*`. `internal/provider` and every `internal/lib/*` package import
nothing from `internal/usage` or from any `internal/provider/<name>` subpackage, so
there is no import cycle.

## Credential lookup order

For every provider, in this order:

1. Per-provider env var override (`QMETER_CLAUDE_TOKEN`, `QMETER_CODEX_TOKEN`,
   `QMETER_CURSOR_TOKEN`, `QMETER_OPENCODE_GO_KEY`) — used verbatim as the bearer
   token/key, skips the vendor store entirely.
2. The vendor's local store (file, or Keychain on macOS for Claude).
3. If neither yields a usable, non-expired credential: a typed error
   (`ErrNotLoggedIn` or `ErrTokenExpired`), rendered per the failure-line wording
   ruling above. qmeter never writes to a vendor store — permanently, not just for
   now (see "Command surface and output").

`internal/lib/credstore` implements steps 1 and 3 generically; each provider package
supplies the step-2 file/Keychain loader and its own JSON shape.

## Per-provider reference

Facts below are copied from the verified fact sheet (measured 2026-09-15); endpoints,
headers, and field names are not paraphrased.

### Claude

| Aspect | Detail |
|---|---|
| Store (Linux/Windows/file) | `~/.claude/.credentials.json` -> `claudeAiOauth.{accessToken, refreshToken, expiresAt (epoch ms), subscriptionType}` |
| Store (macOS) | Same JSON in Keychain service `"Claude Code-credentials"`, read via `/usr/bin/security find-generic-password -s "Claude Code-credentials" -w` behind the subprocess abstraction; the file may also exist |
| Measured | `subscriptionType` "max"; access token lifetime 8.0h |
| Request | `GET https://api.anthropic.com/api/oauth/usage`; headers `Authorization: Bearer <accessToken>`, `anthropic-beta: oauth-2025-04-20` |
| Response | Top-level `five_hour`, `seven_day`, `seven_day_sonnet`, `seven_day_opus`, each `{utilization: <percent 0-100>, resets_at: ISO-8601 or epoch}`; newer responses may add a list of scoped per-model limits with `percent`, `resets_at`, `group` — parse tolerantly, skip unknown windows |
| Errors | 429 carries a `retry-after` header (plan changes cause ~25 min cooldowns) |
| Refresh | Not implemented by qmeter (read-only only, owner decision 2026-09-16); the vendor endpoint exists (`POST https://platform.claude.com/v1/oauth/token`) but is out of scope |
| Plan name | `subscriptionType` from the credentials file |

### Codex

| Aspect | Detail |
|---|---|
| Store | `~/.codex/auth.json` -> `auth_mode ("chatgpt"|"apikey")`, `OPENAI_API_KEY`, `last_refresh`, `tokens.{access_token, refresh_token, id_token, account_id}`. Newer Codex may store tokens in an OS keyring/encrypted store instead — out of scope; detect "no readable file" and explain |
| Measured | access token lifetime 240h (10 days); `id_token` lifetime 1h (its `exp` is irrelevant for usage; its claims carry `chatgpt_plan_type` and `chatgpt_account_id` and remain decodable when expired) |
| Request | `GET https://chatgpt.com/backend-api/wham/usage`; headers `Authorization: Bearer <access_token>`, `ChatGPT-Account-Id: <account_id>` (when present), `Accept: application/json` |
| Response | `plan_type`; `rate_limit.{primary_window, secondary_window}` each `{used_percent, resets_at \| resets_in_seconds, window_minutes \| window_seconds}`; `additional_rate_limits[]` each with `name` + primary/secondary windows; `credits{has_credits, unlimited, balance}`. Field names have drifted between snake_case and camelCase — accept both |
| Refresh | Not implemented by qmeter (read-only only, owner decision 2026-09-16); the vendor endpoint exists (`POST https://auth.openai.com/oauth/token`, Codex refreshes 5 min before expiry) but is out of scope |
| Plan name | `plan_type` in the response, else the `chatgpt_plan_type` claim in `id_token` |

### OpenCode Go

| Aspect | Detail |
|---|---|
| Store | `~/.local/share/opencode/auth.json` -> `"opencode-go": {type: "api", key}`. Not installed on the dev machine — rely on fixtures |
| Request | `GET https://opencode.ai/zen/go/v1/usage`; headers `Authorization: Bearer <key>`, `Accept: application/json` |
| Response | Live shape, differs from the PR draft: `{usage: {rolling\|weekly\|monthly: {status: "ok"\|"rate-limited", percent: int 0-100, resetsAt: RFC3339}}}`. `"rate-limited"` means 100% regardless of `percent` |
| Period lengths | Monthly derived from `resetsAt` (28-31 days); rolling = 5h; weekly = 7d |
| Plan name | `"go"`. Documented caps $12/5h, $30/week, $60/month are informational only |

### Cursor

| Aspect | Detail |
|---|---|
| Primary store (CLI `cursor-agent`, what the owner uses) | `~/.config/cursor/auth.json` -> `{accessToken, refreshToken}`, both JWTs. Measured lifetime ~60 days. JWT `sub` looks like `"google-oauth2\|<id>"` or `"auth0\|<id>"`; user id = the part after the last `"\|"` |
| Fallback store (desktop app, later unit) | SQLite `state.vscdb`, table `ItemTable`, keys `cursorAuth/accessToken`, `cursorAuth/refreshToken` (values sometimes JSON-quoted). Paths: Linux `~/.config/Cursor/User/globalStorage/state.vscdb`; macOS `~/Library/Application Support/Cursor/User/globalStorage/state.vscdb`; Windows `%APPDATA%\Cursor\User\globalStorage\state.vscdb`. Copy the file before reading — the app holds it open. Needs `modernc.org/sqlite` |
| Route used in milestone 1 (Request A, REST) | `GET https://cursor.com/api/usage-summary`; header `Cookie: WorkosCursorSessionToken=<userId>%3A%3A<accessToken>`. **Resolved by a live, read-only spike on 2026-09-15: the CLI token is accepted (HTTP 200).** Observed live shape (free plan): `billingCycleStart`/`billingCycleEnd` ISO; `membershipType` `"free"`; `limitType` `"user"`; `isUnlimited` bool; `individualUsage.plan.{enabled, used, limit, remaining, breakdown{...}, autoPercentUsed, apiPercentUsed, totalPercentUsed}`; `individualUsage.onDemand.{enabled, used, limit\|null, remaining\|null}`; `teamUsage` is present as an empty object `{}` on non-team accounts (not absent) — parse tolerantly, skip unknown `breakdown` subfields |
| Documented fallback route (Request B, Connect-RPC — not a milestone 1 work unit) | `POST https://api2.cursor.sh/aiserver.v1.DashboardService/GetCurrentPeriodUsage`; headers `Authorization: Bearer <token>`, `Content-Type: application/json`, `Connect-Protocol-Version: 1`, body `"{}"`. Also accepted by the CLI token per the same spike (HTTP 200), and 401/403 would mean token invalid. Response shape differs from route A: `billingCycleStart`/`billingCycleEnd` as epoch-ms **strings**; `planUsage.{totalSpend, bonusSpend, remainingBonus, autoPercentUsed, apiPercentUsed, totalPercentUsed}`; `spendLimitUsage.{...}`; `displayMessage`. Kept only as a documented fallback if route A ever breaks; not implemented unless needed |
| Legacy (not used) | `GET https://cursor.com/api/usage?user=<userId>` (`numRequests`, `maxRequestUsage`, `startOfMonth`) and `GET https://cursor.com/api/auth/stripe` (`membershipType`) — older request-based plans only, superseded by route A. Also accepted by the CLI token per the spike, but not used |
| Plan name | `membershipType` |
| Windows | Two windows per fetch, both from route A: `"total"` (`UsedPercent = totalPercentUsed`, primary) and `"auto"` (`UsedPercent = autoPercentUsed`); both share `ResetsAt = billingCycleEnd` and `Period = billingCycleEnd - billingCycleStart` |

## Testing strategy

- TDD throughout: write a failing test, confirm it fails for the right reason, write
  the minimal code to pass, refactor. Before any unit is declared done, run
  `gofmt -l .`, `go vet ./...`, and `go test -race ./...` and report the output.
- All HTTP-facing tests use `httptest.NewServer` with fixture JSON bodies (valid,
  malformed, edge-case per the tables above); no test calls a live vendor endpoint.
- Credential-store tests use `t.TempDir()`; every credential reader accepts an
  injectable base dir / file path rather than hardcoding the home directory.
- The macOS Keychain shell-out goes through `internal/lib/subprocess`, which is a small
  interface (`Run(ctx, name string, args ...string) (stdout []byte, err error)`) with a
  real `exec.Command` implementation and a fake used by every non-macOS test and by
  macOS-path tests run on any OS.
- Orchestrator tests use the fake `provider.Provider` implementations from
  `internal/provider/providertest` (slow, erroring, succeeding) to verify
  concurrency, the per-provider timeout, and that one provider's failure never
  blocks another's output. `DefaultProviderTimeout = 10 * time.Second` is owned by
  `internal/usage`; `internal/lib/httpx` takes its deadline from the context it's
  passed and sets no timeout of its own.
- Rendering tests are golden-output style: given a fixed `[]Window`, assert the exact
  text lines and the exact JSON encoding.

## Work unit stages (parallelizable groups)

Units within a stage have no dependency on each other and can be built in parallel
(separate worktrees where noted); each stage depends only on units in earlier stages.

| Stage | Units |
|---|---|
| 0 | U1, U2 (parallel) |
| 1 | U3, U4, U11 (parallel) |
| 2 | U5, U7, U8, U10 (parallel, separate worktrees) |
| 3 | U6, U12 |
| 4 | U13 |

U9 (the Cursor spike) is already resolved and blocks nothing. The backlog unit
(Cursor desktop SQLite fallback) is not staged — it follows U10 whenever picked up.

## Milestone 1: read-only providers (all four, all platforms)

**Definition of done for every unit:** `gofmt -l .` is empty, `go vet ./...` passes,
and `go test -race ./...` passes (see Testing strategy above).

- [ ] **U1 — `internal/provider`: normalized model, interfaces, and fixture allowlist**
  - Goal: define `Window` and its `MarshalJSON` (wire-form encoder, see the JSON
    ruling in "Command surface and output"), the `Provider` interface,
    `ErrNotLoggedIn` / `ErrTokenExpired` (each carrying an "open <tool>" hint
    string), `ErrRateLimited` (carrying `RetryAfter time.Duration`), the `ID()` /
    `Detect()` contracts stated above, and the fixture allowlist every later unit's
    `testdata/` relies on. Top-level files directly under `internal/provider/`
    belong exclusively to this unit — no later unit adds to them. (No `Refresher`
    interface — there is no refresh path.)
  - Files: create `internal/provider/provider.go`, `internal/provider/provider_test.go`,
    `internal/provider/providertest/providertest.go` (shared fake `Provider` for U11
    and every provider unit — imports only `internal/provider`, so no other package
    needs its own fake); modify `.gitignore` to add `!**/testdata/**` under a
    `# Test fixtures` comment (the allowlist-style `.gitignore` otherwise blocks
    every fixture this plan relies on, including the backlog unit's `.vscdb`
    fixtures).
  - Tests first: `TestErrTokenExpired_Error_IncludesHint`,
    `TestErrNotLoggedIn_Error_IncludesHint`, `TestErrRateLimited_CarriesRetryAfter`,
    `TestErrors_WrapForErrorsIs`, `TestWindowJSON_OmitsZeroResetsAt`,
    `TestWindowJSON_PeriodIsWholeSeconds`, `TestWindowJSON_OmitsEmptyPlan`.
  - Acceptance: package compiles standalone with no dependency on any provider or on
    `internal/usage`; errors round-trip through `errors.Is`/`errors.As`; `go list
    -deps ./internal/provider` lists no `internal/provider/...` subpackage and no
    `internal/usage`; `git check-ignore internal/provider/claude/testdata/x.json`
    exits 1.
  - Depends on: none.

- [ ] **U2 — `internal/lib/subprocess`: fake-able exec wrapper**
  - Goal: a minimal `Runner` interface, a real `exec.Command`-backed implementation, and a scriptable fake, for the macOS Keychain shell-out.
  - Files: create `internal/lib/subprocess/subprocess.go`, `internal/lib/subprocess/subprocess_test.go`.
  - Tests first: `TestFakeRunner_ReturnsScriptedOutput`, `TestFakeRunner_ReturnsScriptedError`, `TestRealRunner_RunsEcho` (skipped unless `echo` is present).
  - Acceptance: `Runner` is narrow enough that `internal/provider/claude` needs no other OS-exec dependency.
  - Depends on: none.

- [ ] **U3 — `internal/lib/httpx`: shared HTTP fetch helper**
  - Goal: context-aware `Get`/`Post` helpers that decode a JSON body and map non-2xx
    responses to typed errors: 401/403 to `provider.ErrTokenExpired`, 429 to
    `provider.ErrRateLimited` (defined in U1, not here) carrying the parsed
    `retry-after` duration, everything else to a plain error carrying status and
    body. `httpx` sets no timeout of its own — it takes its deadline entirely from
    the caller's context (owned by `internal/usage`'s `DefaultProviderTimeout`).
  - Files: create `internal/lib/httpx/httpx.go`, `internal/lib/httpx/httpx_test.go`.
  - Tests first: `TestGet_DecodesJSON`, `TestGet_401MapsToTokenExpired`, `TestGet_429MapsToErrRateLimitedWithRetryAfter`, `TestGet_RespectsCallerContextDeadline`, `TestPost_SendsBodyAndHeaders`.
  - Acceptance: every call is driven against `httptest.NewServer`; no networking outside loopback; no call sets its own timeout independent of the passed context.
  - Depends on: U1 (for the typed error).

- [ ] **U4 — `internal/lib/credstore`: env-override + hint helper**
  - Goal: implement lookup-order step 1 (env var override) and step 3 (typed error with "open <tool>" hint) generically; each provider supplies its own step-2 loader.
  - Files: create `internal/lib/credstore/credstore.go`, `internal/lib/credstore/credstore_test.go`.
  - Tests first: `TestResolve_EnvVarTakesPrecedence`, `TestResolve_FallsBackToLoader`, `TestResolve_LoaderNotFoundReturnsHintedError`.
  - Acceptance: works for any credential type via a caller-supplied loader function; no provider-specific parsing lives here.
  - Depends on: U1.

- [ ] **U5 — `internal/provider/claude`: read-only file-store provider**
  - Goal: `claude.New()` implementing `provider.Provider`; reads `~/.claude/.credentials.json` (injectable path via an explicit `defaultCredentialPath()` seam), applies `QMETER_CLAUDE_TOKEN`, fetches and parses the usage endpoint. `credentials.go` also exposes a distinct keychain-loader hook (a no-op on this unit) so U6 can add Keychain support and U13 can add Windows path handling without either touching the same function as the other.
  - Files: create `internal/provider/claude/claude.go`, `internal/provider/claude/credentials.go`, `internal/provider/claude/claude_test.go`, `internal/provider/claude/testdata/*.json`.
  - Tests first: `TestFetch_ParsesFourWindows`, `TestFetch_ParsesScopedPerModelLimits_SkipsUnknown`, `TestFetch_429RendersAsErrRateLimited`, `TestFetch_EnvOverrideLeavesPlanEmpty`, `TestCredentials_MissingFileReturnsNotLoggedIn`, `TestCredentials_ExpiredReturnsTokenExpired`, `TestCredentials_EnvOverrideWins`, `TestDetect_EnvOverride`, `TestDetect_StorePresent`, `TestDetect_StoreMissingReturnsReason`, `TestID_MatchesCLIProviderName`.
  - Acceptance: no macOS/Keychain code yet — cross-platform file path only; all four windows plus plan name (`subscriptionType`) round-trip correctly; with `QMETER_CLAUDE_TOKEN` set, `Plan` is empty (there is no local file to source a plan name from); `ID()` returns `"claude"`.
  - Depends on: U1, U3, U4.

- [ ] **U6 — `internal/provider/claude`: macOS Keychain credential source**
  - Goal: add the Keychain lookup (`/usr/bin/security find-generic-password -s "Claude Code-credentials" -w`, absolute path) via `internal/lib/subprocess`, tried before the file on `runtime.GOOS == "darwin"`, falling back to the file on Keychain error. This unit only fills in U5's keychain-loader hook — it does not touch `defaultCredentialPath()`, which stays U13's alone.
  - Files: modify `internal/provider/claude/credentials.go`; add `internal/provider/claude/credentials_darwin_test.go` (uses the fake runner so it runs on any OS).
  - Tests first: `TestCredentials_DarwinPrefersKeychain`, `TestCredentials_DarwinFallsBackToFileOnKeychainError`, `TestCredentials_NonDarwinSkipsKeychain`.
  - Acceptance: behavior gated on `runtime.GOOS`, fully testable off-macOS via the fake; marked "untested on real macOS hardware" until someone runs it there.
  - Depends on: U2, U5.

- [ ] **U7 — `internal/provider/codex`: read-only provider**
  - Goal: `codex.New()` reading `~/.codex/auth.json`, applying `QMETER_CODEX_TOKEN` as
    the bearer token (account id omitted unless `QMETER_CODEX_ACCOUNT_ID` is also set;
    plan name comes from the response only, since there's no local `id_token` to read
    in that case), decoding `id_token` claims as a plan-name/account-id fallback when
    reading from the file store, and parsing the usage endpoint tolerant of
    snake_case/camelCase. `auth_mode "apikey"` is detected and reported as unsupported
    for usage lookup, not treated as a parse failure. Its `id_token` decoder must not
    import or be imported by U10's Cursor JWT decoder — if the two converge, the
    shared logic moves to a new `internal/lib/jwt` package rather than one provider
    package importing the other.
  - Files: create `internal/provider/codex/codex.go`, `internal/provider/codex/credentials.go`, `internal/provider/codex/idtoken.go`, `internal/provider/codex/codex_test.go`, fixtures.
  - Tests first: `TestFetch_ParsesSnakeCaseFields`, `TestFetch_ParsesCamelCaseFields`, `TestFetch_ParsesAdditionalRateLimits`, `TestIDToken_DecodesPlanTypeAndAccountID`, `TestIDToken_DecodableWhenExpired`, `TestCredentials_UnreadableStoreExplainsKeyringCase`, `TestCredentials_APIKeyModeUnsupported`, `TestCredentials_EnvOverrideOmitsAccountIDUnlessSet`, `TestDetect_EnvOverride`, `TestDetect_StorePresent`, `TestDetect_StoreMissingReturnsReason`, `TestID_MatchesCLIProviderName`.
  - Acceptance: plan name resolves from `plan_type` first, `id_token` claim second, and
    is empty when only the env override is set; an unparseable store points at the
    possible OS-keyring case rather than a generic parse error; `auth_mode "apikey"`
    produces a clear "unsupported" message, not a crash; `ID()` returns `"codex"`.
  - Depends on: U1, U3, U4.

- [ ] **U8 — `internal/provider/opencodego`: read-only provider**
  - Goal: `opencodego.New()` reading `~/.local/share/opencode/auth.json` (`opencode-go.key`), applying `QMETER_OPENCODE_GO_KEY`, fetching and parsing the usage endpoint including the `"rate-limited"` status and period derivation.
  - Files: create `internal/provider/opencodego/opencodego.go`, `internal/provider/opencodego/credentials.go`, `internal/provider/opencodego/opencodego_test.go`, fixtures.
  - Tests first: `TestFetch_OKStatusUsesPercent`, `TestFetch_RateLimitedStatusForces100Percent`, `TestFetch_MonthlyPeriodDerivedFromResetsAt`, `TestFetch_RollingAndWeeklyPeriodConstants`, `TestDetect_EnvOverride`, `TestDetect_StorePresent`, `TestDetect_StoreMissingReturnsReason`, `TestID_MatchesCLIProviderName`.
  - Acceptance: works entirely from fixtures since OpenCode Go isn't installed on the dev machine; plan name is the constant `"go"`; `ID()` returns `"opencode-go"`.
  - Depends on: U1, U3, U4.

- [x] **U9 — Cursor route spike (resolved 2026-09-15, precedes the Cursor provider)**
  - Resolved via a live, read-only GET with the real CLI token: all three candidate routes accepted it (HTTP 200) — route A (`usage-summary` cookie route), route B (Connect-RPC bearer route), and the legacy `api/usage` cookie route.
  - Decision: route A (`GET https://cursor.com/api/usage-summary`) is the only Cursor route implemented in milestone 1 — it has ISO billing-cycle bounds, `membershipType`, and both plan percentages. Route B stays a documented fallback (see the reference table) but is not built unless route A breaks. The legacy route is not used.
  - No further work needed for this unit; U10 proceeds directly against route A.
  - Depends on: none.

- [ ] **U10 — `internal/provider/cursor`: read-only CLI-store provider**
  - Goal: `cursor.New()` reading `~/.config/cursor/auth.json`, applying
    `QMETER_CURSOR_TOKEN` (must itself be a JWT so `sub` can supply the user id; a
    non-JWT override is a clear error, not a silent failure), parsing the JWT `sub`
    for the user id, fetching via route A (`WorkosCursorSessionToken` cookie), and
    producing two windows: `"total"` (from `totalPercentUsed`) and `"auto"` (from
    `autoPercentUsed`). Its JWT decoder must not import or be imported by U7's Codex
    `id_token` decoder — if the two converge, the shared logic moves to a new
    `internal/lib/jwt` package rather than one provider package importing the other.
  - Files: create `internal/provider/cursor/cursor.go`, `internal/provider/cursor/credentials.go`, `internal/provider/cursor/jwt.go`, `internal/provider/cursor/cursor_test.go`, fixtures (including one with `teamUsage: {}`).
  - Tests first: `TestJWT_ParsesGoogleOAuth2AndAuth0Subjects`, `TestFetch_ParsesRouteAResponse`, `TestFetch_EmitsTotalAndAutoWindows`, `TestFetch_ToleratesEmptyTeamUsageObject`, `TestFetch_401Or403ReturnsTokenExpired`, `TestCredentials_EnvOverrideNonJWTErrors`, `TestDetect_EnvOverride`, `TestDetect_StorePresent`, `TestDetect_StoreMissingReturnsReason`, `TestID_MatchesCLIProviderName`.
  - Acceptance: only route A is implemented (no route B code in this unit); both windows share `ResetsAt`/`Period`; plan name is `membershipType`; `ID()` returns `"cursor"`.
  - Depends on: U9 (resolved), U1, U3, U4.

- [ ] **U11 — `internal/usage`: orchestrator (registry-decoupled)**
  - Goal: `func Run(ctx context.Context, providers []provider.Provider, only string)
    Result` (no concrete registry here — that's built in U12's `registry.go` once the
    concrete providers exist). `only` is the `--provider` filter (`""` = all).
    `Result{Windows []provider.Window; Errors []ProviderError; Undetected
    []ProviderError}` where `ProviderError{Provider, Message string}`.
    `Detect`-based inclusion (undetected providers are simply skipped and never
    appear in `Errors`; when filtered by `only`, they appear in `Undetected` instead
    (see below)), concurrent `Fetch` with a per-provider
    timeout (`DefaultProviderTimeout = 10 * time.Second`, owned by this package), and
    independent failure handling — `Errors` collects one `ProviderError` per failed
    provider, in registry order, so a slow or erroring provider never blocks
    another's result. `internal/usage` owns the error-to-message mapping (U12 only
    formats what's already in `Result`): `provider.ErrTokenExpired` ->
    `"token expired, open <tool> to refresh"`; `provider.ErrNotLoggedIn` ->
    `"not logged in, run <tool> to log in"`; `provider.ErrRateLimited` ->
    `"rate limited, retry in <dur>"`; anything else -> `err.Error()`. When `only`
    names a provider that exists but was not detected, `Result.Undetected` gets one
    `ProviderError{Provider: only, Message: <Detect's reason>}` — this only happens
    for an explicit `--provider` filter, never in the default all-providers listing.
    No text/JSON rendering yet — that's U12.
  - Files: create `internal/usage/usage.go`, `internal/usage/usage_test.go`.
  - Tests first: `TestRun_FetchesDetectedProvidersConcurrently`,
    `TestRun_SlowProviderTimesOutIndependently`,
    `TestRun_OneProviderFailureDoesNotBlockOthers`,
    `TestRun_ProviderFilterRestrictsToOne`,
    `TestRun_RateLimitedErrorProducesRetryAfterMessage`,
    `TestRun_UndetectedProviderReasonSurfacedOnlyWhenFiltered`.
  - Acceptance: uses only the fakes from `internal/provider/providertest`, no
    concrete provider package; wall-clock test time bounded by the per-provider
    timeout, not by summing providers serially; `internal/usage` is the only package
    that constructs the real four-provider registry (in U12); `Errors` is ordered by
    registry order, not completion order.
  - Depends on: U1.

- [ ] **U12 — `internal/usage`: registry + rendering + `cmd/usage`: cobra command**
  - Goal: `internal/usage/registry.go` builds the concrete `[]provider.Provider` in
    order claude, codex, opencode-go, cursor; a `tabwriter.NewWriter(out, 0, 8, 2,
    ' ', 0)` text renderer that only formats `Result` (message text already decided
    by U11) — matching the exact literal block in "Command surface and output"
    (header + data rows written per the tab-cell rules stated there; `-` for empty
    plan/zero `ResetsAt`, one-decimal percent, `(rate limited)` suffix on the RESETS
    cell separated by two spaces; `no providers detected` — with no header row —
    printed only when `Result.Windows`, `Result.Errors`, and `Result.Undetected` are
    all empty; two-cell padded failure lines from `Result.Errors`; and — only when
    `--provider` was given and the name is valid but undetected — a two-cell padded
    `<provider>  not detected: <reason>` line from `Result.Undetected`, exit 0); a
    JSON renderer emitting the `{"windows":[...],"errors":[...],"undetected":[...]}`
    envelope (via `Window.MarshalJSON`) with all three keys always present and
    arrays never null. `cmd/usage` wires the local `--provider` flag, reads the
    root-level persistent `--json` flag (added to `cmd/root.go` in this unit), and
    calls into `internal/usage`; exit 1 only when `--provider` names an unknown
    provider (not merely undetected).
  - Files: create `internal/usage/registry.go`, `internal/usage/render.go`, extend
    `internal/usage/usage_test.go`; create `cmd/usage/usage.go`,
    `cmd/usage/usage_test.go`; modify `cmd/root.go` to register the subcommand and add
    `root.PersistentFlags().Bool("json", false, ...)`.
  - Tests first: `TestRenderText_MatchesGoldenOutputBlock`,
    `TestRenderText_NoProvidersDetectedLine`,
    `TestRenderText_FailureLineIsTwoCellPaddedRow`,
    `TestRenderText_DashForEmptyPlanAndZeroResets`,
    `TestRenderJSON_MatchesEnvelopeWithAllThreeKeysAlwaysPresent`,
    `TestRegistry_OrdersClaudeCodexOpenCodeGoCursor`,
    `TestCmd_ProviderFlagRestrictsOutput`,
    `TestCmd_ProviderFlagOnUndetectedProviderShowsReason`,
    `TestCmd_UnknownProviderExitsNonZero`, `TestCmd_RootJSONFlagSelectsJSONRenderer`.
  - Acceptance: `cmd/usage/usage.go` is wiring only (flags, calling `internal/usage.Run`); all formatting and registry logic and their tests live in `internal/usage`; text output byte-matches the golden block for the documented fixture windows; `no providers detected` prints only when all three `Result` fields are empty; `Result.Undetected` only ever renders when `--provider` was explicitly given.
  - Depends on: U11, U5, U7, U8, U10.

- [ ] **U13 — Milestone 1 wrap-up: default paths per OS + manual QA notes**
  - Goal: wire each provider's real default path (each package's `defaultCredentialPath()` seam) to its OS-correct default; touches only that function, distinct from U5's/U6's functions. Exact Windows paths (owner ruling — most credential stores are `%USERPROFILE%`-relative, not `%APPDATA%`): Claude `%USERPROFILE%\.claude\.credentials.json`; Codex `%USERPROFILE%\.codex\auth.json`; Cursor CLI `%USERPROFILE%\.config\cursor\auth.json`; OpenCode Go `%USERPROFILE%\.local\share\opencode\auth.json`. Only the Cursor **desktop** `state.vscdb` fallback (backlog unit) is `%APPDATA%`-relative. This unit also adds a manual smoke-test checklist for a human to run once on real Windows and macOS machines.
  - Files: create `internal/provider/<name>/paths_test.go` in each of `claude`, `codex`, `opencodego`, `cursor`; modify each provider's `credentials.go` `defaultCredentialPath()` only.
  - Tests first: `TestDefaultPath_Windows`, `TestDefaultPath_MacOS`, `TestDefaultPath_Linux` in each of the four `paths_test.go` files.
  - Acceptance: `go test -race ./...` green on the CI runner (Linux); Windows/macOS default paths are unit-tested by string assertion, not by running on those OSes, and stay flagged untested-on-hardware until a human confirms.
  - Depends on: U5, U6, U7, U8, U10, U12.

## Backlog / deferred (not committed to a milestone)

- [ ] **U14 — Cursor desktop SQLite fallback**
  - Goal: when the CLI store (`~/.config/cursor/auth.json`) is absent, fall back to
    reading `state.vscdb` (table `ItemTable`, keys `cursorAuth/accessToken` /
    `cursorAuth/refreshToken`, values sometimes JSON-quoted) from the desktop app's
    per-OS path, copying the file first since the app holds it open. Read-only, same
    as everything else in this plan — there is no write-back anywhere.
  - Files: add `modernc.org/sqlite` to `go.mod`; create
    `internal/provider/cursor/desktopstore.go`, `internal/provider/cursor/desktopstore_test.go`, fixture
    `.vscdb` files (or a small script that builds one at test time) per OS path shape.
  - Tests first: `TestDesktopStore_ReadsAccessAndRefreshTokens`, `TestDesktopStore_UnquotesJSONQuotedValues`, `TestDesktopStore_CopiesBeforeReading`, `TestDesktopStore_DefaultPathPerOS`, `TestDesktopStore_FallsBackOnlyWhenCLIStoreAbsent`.
  - Acceptance: `internal/provider/cursor` still prefers the CLI store; this path only engages when that store is missing; no cgo introduced.
  - Depends on: U10.

## Known unknowns

- The live route A response carries `isUnlimited` and a `plan.breakdown{...}` object
  not in the original fact sheet; U10 parses both tolerantly but neither is surfaced
  in the normalized `Window` yet — revisit whether `isUnlimited` should suppress or
  relabel the percentage display for unlimited plans.
- Route A's shape was observed on a single free-plan account; team-plan
  `teamUsage.{pooled, onDemand}` content (present as `{}` when unused) is not yet
  confirmed against a real team account.
- OpenCode Go is not installed on the dev machine, so U8 is built and tested entirely
  from fixtures; the live response shape has already drifted once from an earlier PR
  draft and may drift again.
- Codex may move credential storage to an OS keyring/encrypted store in newer
  versions; U7 only detects and explains this case, it does not read from a keyring.
- Codex response field casing has already drifted between snake_case and camelCase;
  U7's tolerant parsing may still miss a future rename.
- The macOS Keychain path (U6) is testable only through the fake subprocess runner in
  CI; it is marked untested on real hardware until a human confirms it on a Mac.
- Windows default credential paths (U13) are asserted by string comparison, not
  exercised on a real Windows machine, for the same reason.
- Cursor desktop SQLite JSON-quoting of stored values (U14, backlog) needs confirming
  against a real `state.vscdb` file; the exact quoting behavior is inferred from the
  fact sheet, not yet observed directly in this repo's test fixtures.
- Whether refresh (write-back) is ever wanted again is an open product question, not
  a technical one — it was scoped as milestone 2, then removed by the owner before
  any unit landed. If it returns, it needs its own new work units; nothing in the
  current plan assumes it.

## Reference implementations (background only, not authoritative for facts above)

- `github.com/ItsJazii/pane` — `src-tauri/src/providers/{claude,opencode,cursor}.rs`
  (Rust; the most complete)
- `github.com/jens-duttke/usage-monitor-for-claude` — `usage_monitor_for_claude/api.py`
- `github.com/Ganymede404/vscode-codex-usage` — `src/codexApi.ts`
- `github.com/Tendo33/cursor-usage-tracker`
- `github.com/openai/codex` — `codex-rs/login/src/{token_data.rs, auth/storage.rs, auth/manager.rs}`
