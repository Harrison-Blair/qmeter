# Backlog

Ideas that are wanted but not scheduled. Nothing here is specced. The active spec is
[`docs/specs/gauge-additions.md`](specs/gauge-additions.md). Mockups for most of these
are on the qmeter design bench artifact.

## Storage

qmeter persists nothing today except the update-hint cache. Everything in this section
needs qmeter's first real persistent state, so the storage decisions come first.

### Decisions to make first

- **Where state lives.** `$XDG_STATE_HOME/qmeter` (falling back to
  `~/.local/state/qmeter`), with the platform equivalents on macOS and Windows. The
  status-strip cache (below) should use `$XDG_CACHE_HOME` instead, since it is
  disposable.
- **Format.** Append-only JSONL is the simplest: one line per window per fetch, no new
  dependency, easy to inspect and to merge. SQLite makes time-range queries and
  retention easier but adds a driver dependency (pure-Go drivers avoid cgo and keep
  cross-compiled releases working).
- **Retention.** A rule such as "keep 90 days" plus compaction, or rotation by month.
- **Concurrent writers.** The dashboard, a `qmeter record` timer and a status strip can
  all run at once. Needs a lock file or single-`write` appends under the pipe-buf size.
- **Gaps.** Nothing is sampled while no qmeter is running. Views must draw gaps as
  gaps, never interpolate across them.
- **Which machine records.** The samples are account-wide, so any machine's log is
  valid, but each log only covers the time that machine was running qmeter. An
  always-on server running `qmeter record` on a timer gives the most complete history.
  Open: whether personal machines should read the server's log (over SSH, a synced
  directory, or a tiny HTTP endpoint) or each keep their own.

### Sample log (tier C, foundation, large)

One timestamped sample per window per fetch, written after each fetch in
`internal/usage`. New `internal/history`. Adds `qmeter record` for a systemd timer or
cron job so the log fills while the dashboard is closed. Everything else in this
section depends on it.

### Window trace (tier C, large, with the log)

A block sparkline per window whose width is the window's full period, filled to now and
dotted out to the reset, with a line showing how past cycles ended.
`qmeter history --window claude/weekly --since 7d`.

### Habits heatmap (tier C, medium, after the log)

A 7 × 24 grid of burn per hour of the week, with the busiest stretch and how often the
short window ran dry. `qmeter habits --provider claude`. Only meaningful after a few
weeks of samples; show a "collecting" state until then.

### Sharper run-out forecast (after the log)

The forecast in the active spec uses average burn since the window opened, which is
crude after a quiet morning. With the log it can use a recent burn rate, and the 5%
elapsed floor can go.

### Status strip and its cache (tier A data, small)

`qmeter line` with `--worst`, `--bars` and `--filter`: one line for tmux, a shell
prompt or a Claude Code statusline. It needs a short-lived response cache so a prompt
does not call four APIs on every keypress. That cache would be the first state shared
between qmeter runs, so decide where it lives together with the sample log.

## Set aside: Who spent it (tokens per model and provider)

Dropped from the shortlist on 2026-09-19 because it is machine dependent, and qmeter
runs on a server as well as personal machines. Kept here because the investigation was
real work and the design is finished.

**What was found**

- The vendor endpoints qmeter calls return quota percentages, reset times and (Codex)
  request counts. No token counts.
- Documented account-side token reporting exists only for API organisations
  (Anthropic Usage and Cost Admin API) and enterprise plans (Claude Enterprise
  Analytics API, OpenAI Compliance API). Not for individual subscriptions.
- Not checked: whether the live subscription endpoints carry undocumented token
  fields. The probe needed to read stored credentials and was blocked by the
  permission guard.
- Per-request token counts by model and timestamp do exist in local CLI session logs,
  verified on one machine:
  - Claude Code: `~/.claude/projects/**/*.jsonl`, `message.usage` with `input_tokens`,
    `cache_creation_input_tokens`, `cache_read_input_tokens`, `output_tokens`, plus
    `message.model` and `timestamp`. One API message spans several lines, so dedupe by
    `message.id` or counts roughly double.
  - Codex: `~/.codex/sessions/**/*.jsonl`, `token_usage_record` lines, model from the
    preceding `turn_context`. Do not sum `token_count` events; they repeat and
    overcounted by 1.5–4%. Uncached input is `input_tokens - cached_input_tokens`.
  - opencode: `~/.local/share/opencode/opencode.db`, `message.data` JSON on assistant
    rows (`tokens.input`, `output`, `reasoning`, `cache.read`, `cache.write`,
    `providerID`, `modelID`, `cost`). SQLite, so a driver dependency.
  - Cursor: no local token source found (only table names in `~/.cursor/ai-tracking`
    were checked).
- These logs cover only CLI sessions run on that machine. Web apps, IDEs and other
  devices appear nowhere. The formats are undocumented and can change.

**Settled design, if it comes back**

- Grouped (by provider, with provider totals) and Ranked (all models by total) views on
  one shared scale and time range.
- Stacked bar of uncached input, cache write and output; categories are dynamic, so a
  model shows only what its source reports. Cache reads are about 98% of all tokens, so
  they stay out of the bar and show as a count.
- Cache hit % = cache read ÷ (input + cache write + cache read), per model, provider
  and overall, computed from summed tokens. A high hit % does not mean low cost.
- Counts visible under every model; distinct glyph and colour per category.

**Ways to bring it back**

1. Per-machine, labelled "this machine's sessions".
2. Merge machines: each exports its token records and one qmeter reads several. Shares
   the transport question with "which machine records" above.
3. Finish the live endpoint check, in case an account-side source exists after all.

## Smaller ideas

- Threshold notch on the bezel and `qmeter check --below 15`, exiting non-zero so a
  script can refuse to start a long agent run on an empty tank.
- `Enter` opens a detail pane for one window, holding its trace and forecast.
- Surface Cursor's `isUnlimited` (already parsed) as a full gauge with an `∞` badge.
- A worst-window summary line in the `--no-banner` header.
