# Type the token-expiry error and cache the last good response

| Item | Value |
| --- | --- |
| Wave | W2 cache |
| Tier | strong |
| Depends on | `127-dash-spec` (landed; the feature spec is on `dev`). Feature `qmeter-dash-clock-cache`. Base: `dev` after the spec lands |
| Grant | goal `g7-dash-clock-cache`, user chat c131-c141 2026-10-22: land on qmeter `dev` |

## Overview

Two connected changes, taken from decisions 2, 3, 4 and 5 of the feature spec
`.hive/spec/qmeter-dash-clock-cache/README.md` and manager decisions M1 to M5:

1. A typed error kind on `usage.ProviderError`, so the dashboard can tell an
   expired token from any other failure.
2. The dashboard keeps the last good response of each provider in memory. When
   a provider fails, its card shows the kept bars, dimmed, with the line
   `◌ cached · fetched HH:MM:SS · <reason>`.

The header clock (decision 1 and 6) is the next task. Do not change the header
in this task beyond keeping it compiling.

## Behaviour

Read `.hive/spec/qmeter-dash-clock-cache/README.md` first. These points settle
the detail.

- **Typed kind.** `usage.ProviderError` gains `Kind ErrorKind` (a string type).
  The zero value means any other failure. Add one constant,
  `usage.KindTokenExpired`, and set it where the fetch error matches
  `provider.ErrTokenExpired` through `errors.As` — the same test the current
  `message` mapping already makes. Do not add other kinds. The `Undetected`
  entries keep the zero value.
- **One-shot output does not change.** `internal/usage/render.go`,
  `internal/spend`, `internal/resets` and `cmd/**` stay byte-identical. The
  JSON envelope already rebuilds its own structs, so a new field on
  `ProviderError` must not reach it; prove it with a test.
- **The cache lives in `internal/dash`.** `usage.Result` keeps its meaning: the
  fresh output of one run. The model holds the cache and passes it to
  `layout.Screen` through `layout.ScreenOptions`. Nothing merges the cache into
  `usage.Result`.
- **Cache content and rules.** For each provider, the cache holds that
  provider's windows, its balances, and the time of the fetch that produced
  them. A success replaces its entry at once. A failure whose kind is not
  `KindTokenExpired` keeps the previous entry. A `KindTokenExpired` failure
  drops the entry. The cache is in memory only and is lost on restart.
- **Where the cache shows.** The provider card and the compact window rows use
  the cached windows and balances when that provider has a failure in
  `result.Errors`. The R1 resets list and the balances panel keep using
  `result` and therefore show fresh data only (manager decision M2). A provider
  with no cached entry shows its error line exactly as today.
- **The cached line.** Inside the card and in the compact rows, the provider's
  error message becomes exactly
  `◌ cached · fetched HH:MM:SS · <reason>` — the time of the kept fetch in
  local time to the second, and `<reason>` the same `Message` text the error
  line shows today. No `error: ` prefix. The line keeps the error colour. The
  cached provider's bars, percentage and countdown render faint (dimmed). The
  `◌` glyph and the word `cached` carry the state, so the screen still tells
  the truth with no colour.
- **Order and fit.** In the compact layout, the window rows keep the
  provider order of `presentProviders`, with a cached provider's windows in its
  own slot; the message rows keep the current order (window rows, then
  `result.Errors`, then `result.Undetected`). `compactNeed` uses the same row
  set, so the too-small message stays exact.
- **No cache at the first failure.** When the first fetch of a session fails,
  the card shows only the error line, as today.

## Design

No canvas board. The user chose no new board for this goal ("No canvas boards.
The tests check the header and row text"). The older boards under
`/home/penguin/source/hive/.hive/tmp/coordinator/designs/qmeter/` belong to the
`qmeter-simplify` layout goal; do not compare against them and do not capture a
screen for this task. The design of record is
`/home/penguin/source/hive/plans/qmeter-dash-clock-cache.md` plus the exact
strings above; the tests assert the card and row text.

## Source files

| Path | Purpose |
| --- | --- |
| `internal/usage/usage.go`, `internal/usage/usage_test.go` | the `Kind` field and its mapping |
| `internal/dash/cache.go`, `internal/dash/cache_test.go` | new: the cache type and its update rules |
| `internal/dash/model.go`, `internal/dash/model_test.go` | hold the cache, update it on each result, pass it to `View` |
| `internal/dash/layout/screen.go`, `internal/dash/layout/providers.go`, `internal/dash/layout/screen_test.go` | `ScreenOptions.Cached`, cached cards, cached compact rows, `compactNeed` |
| `internal/display/card.go`, `internal/display/bar.go` and their tests | the dim flag for bars and cards. Keep `display.ProviderCard`'s current signature and behaviour and add a dim-aware sibling (or a thin wrapper) that only the dashboard calls, so `internal/usage/render.go` needs no edit at all. `display.Bar` and `display.CompactBar` have no one-shot caller, so a parameter there is fine |
| `README.md` | one short paragraph in `## Dashboard` on the cache and the cached line |
| `.hive/spec/qmeter-dash-clock-cache/tasks/<id>/brief.md`, `.../evidence/` | this brief and the evidence |

No other file. In particular: no change to `internal/usage/render.go`,
`internal/spend/`, `internal/resets/`, `cmd/`, `internal/provider/`, or
`internal/dash/run.go`. Do not change an existing golden under
`internal/dash/layout/testdata/` for the no-cache fixture: the fixture has no
cache, so its frames do not change. When you need a golden for the cached
state, add a new file such as `screen-cached-93x39.txt`.

## Verification

| ID | Check | Expected result |
| --- | --- | --- |
| V1 | Kind mapping test, red then green | A test shows `Kind == KindTokenExpired` for a wrapped `provider.ErrTokenExpired`, and the zero kind for a timeout, a cancellation and a rate limit. It fails on the base (no `Kind`) and passes on the branch. |
| V2 | One-shot output unchanged | A test proves the JSON of a token-expired error has the same keys and values as before the change, and `git diff dev -- internal/usage/render.go internal/spend internal/resets cmd` is empty. The existing render tests pass unchanged. |
| V3 | Cache rules test, red then green | A success replaces the entry; a timeout keeps the previous entry; a token expiry drops it; a first failure with no entry yields no cached line; the entry keeps the fetch time of the fetch that made it. |
| V4 | Full-layout text | At 140x45 with one provider failing and a cache entry, the card keeps that provider's windows and shows exactly `◌ cached · fetched HH:MM:SS · <reason>` in plain text (ANSI stripped). The R1 resets panel does not list that provider's windows, and the balances panel does not list its balances. |
| V5 | Compact text and fit | In the compact layout the cached windows render in the provider's slot with the same cached line, and the too-small frame reports a need that fits the cached rows. |
| V6 | No colour needed | With the Ascii profile the cached line and an ordinary error line differ by text alone (`◌ cached` against `error: `), and the cached card still draws its bars. State it in the evidence with the stripped frame. |
| V7 | `git diff dev --stat` | Only the paths in the source table, plus the task records. |
| V8 | The gate | `gofmt -l .` prints nothing; `go vet ./...`, `go test -race ./...`, `bash .github/scripts/next-version_test.sh` and `golangci-lint run --new-from-rev=dev ./...` pass. `golangci-lint run ./...` shows the 27 findings that already exist on `dev`; record them, do not fix them. |
| V9 | The TUI input rule | `TestRun_QuittingCancelsTheFetchInFlight` and `TestRun_QuitsOnQ` still pass: a quit key during a slow fetch exits at once. |

The 27 baseline lint findings are known on `dev` and are out of scope.

## Finish

Commit with a subject such as `feat(dash): show the last good response when a
provider fails`. Push the task branch. Write `evidence/V1.md` to `V9.md`. Run
`hive task complete`. Send the manager the SHA, the cached-line text from V4,
and the V1 to V9 result.

## Common rules

- Read `/home/penguin/source/hive/plans/qmeter-dash-clock-cache.md` and
  `.hive/spec/qmeter-dash-clock-cache/README.md`. The decisions are final. When
  the code does not agree with this brief, stop and report it with
  `hive inbox send --to manager-qmeter TEXT --agent NAME`.
- No canvas board for this goal. The tests check the header and row text.
- The user is colourblind: each state needs a glyph or a word. Colour can add to
  a state. Colour must not be the only signal.
- Read `AGENTS.md` in the worktree. Work test-first: write each test, run it,
  confirm it fails for the right reason, then write the code. Never weaken or
  skip an existing test to get green; when a test checks removed behaviour,
  delete or rewrite it and say which in the evidence.
- Keep colour bound to the destination with `display.Renderer`. Pipes and
  `NO_COLOR` give plain text.
- Gate, from the worktree root: `gofmt -l .` prints nothing; `go vet ./...`,
  `go test -race ./...` and `bash .github/scripts/next-version_test.sh` exit 0;
  `golangci-lint run --new-from-rev=dev ./...` reports 0 issues.
- Evidence goes to `.hive/spec/qmeter-dash-clock-cache/tasks/<id>/evidence/` in
  the worktree. Commit it on the task branch.
- Commit on the task branch only. Never touch `main`. No release, no tag.
- Verifier: commit and push `VERIFIED.md` in that evidence folder, then run
  `hive task verify`.
