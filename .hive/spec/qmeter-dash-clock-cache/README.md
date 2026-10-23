# qmeter dash clock and last-response cache

Source: `plans/qmeter-dash-clock-cache.md` in the hive home, confirmed by the user on 2026-10-22 (chat c131-c141).

## Decisions

1. The header shows `fetched HH:MM:SS · now HH:MM:SS` at its right edge, both local time to the second. The dash redraws once each second for the clock.
2. The dash keeps the last good response of each provider in memory. It is lost on restart. The one-shot commands (`usage`, `resets`, `spend`, piped `qmeter`, `--json`) do not change.
3. When a provider fails, its card keeps its bars, dimmed, with the line `◌ cached · fetched HH:MM:SS · <error reason>`. The `◌` and the word `cached` carry the state. Colour can add to it.
4. An expired token (`provider.ErrTokenExpired`) does not use the cache. That provider shows only its error line. This needs a typed error kind on `usage.ProviderError`, which today holds only a message.
5. The header `fetched` time is the time of the last refresh, success or failure.
6. A narrow header drops ` · now HH:MM:SS` first, then the whole right side.

## Assumptions

- The cache covers the windows and the balances of a provider.
- When the first fetch of a session fails, no cache exists, so the error line shows as today.
- A success replaces the cached data at once.
- No canvas boards. The tests check the header and row text.

## Manager decisions

- M1. The cache lives in `internal/dash`. `usage.Result` keeps its meaning: one run's fresh output. Only the dashboard uses the cache.
- M2. The provider cards and the compact window rows show cached data. The R1 resets list and the balances panel show fresh data only.
- M3. The cached line is exactly `◌ cached · fetched HH:MM:SS · <reason>`, with no `error: ` prefix. The reason is the same text the error line shows today (`usage.ProviderError.Message`). The line keeps the error colour. The cached provider's bars, percentage and countdown render faint.
- M4. `usage.ProviderError` gains a `Kind` field with a typed set. `KindTokenExpired` means the fetch failed on an expired token. The zero value means another error. One-shot text and JSON output do not change.
- M5. A success replaces a provider's cached windows and balances at once, with that provider's fetch time. A failure other than a token expiry keeps the previous entry. A token expiry drops it.
- M6. The clock is one `tea.Tick` per second. It starts in `Init`, re-arms on each tick, and only redraws. It never blocks, never starts a fetch, and never delays a key. `q` and `ctrl+c` quit while it runs.
- M7. Before the first result the header right side shows `fetching…`. After it, the right side is `fetched HH:MM:SS · now HH:MM:SS` until the width forces a drop per decision 6.

Decision 15 of `.hive/spec/qmeter-simplify/README.md` (`fetched HH:MM`, no clock) is superseded by decision 1 here, and decisions 1 to 18 of `qmeter-simplify` stay in force for everything else.

## User input

`q` and `ctrl+c` work in every state, including while the clock ticks and during a fetch or a refresh. A key press gets a visible response at once. The input loop never waits for I/O.

## Task plan

Two implementation tasks follow this spec:

1. The typed error kind on `usage.ProviderError` with the last-response cache.
2. The header clock to the second.
