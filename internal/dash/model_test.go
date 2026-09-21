package dash

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"

	"github.com/Harrison-Blair/qmeter/internal/dash/banner"
	"github.com/Harrison-Blair/qmeter/internal/dash/layout"
	"github.com/Harrison-Blair/qmeter/internal/dash/theme"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/provider/providertest"
	"github.com/Harrison-Blair/qmeter/internal/usage"
)

// TestMain pins the colour profile to Ascii, so every test in this package
// compares bare text against the design. lipgloss keeps the profile in a
// process-global renderer, so no test here may run in parallel.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

// now is the instant every countdown in the sample result is measured from.
var now = time.Date(2026, 9, 16, 14, 22, 7, 0, time.UTC)

// sample is the same run internal/dash/layout's tests draw: eight windows
// across three providers, one of them rate limited, plus one failed
// provider and one that was never detected. At 80 cells the responsive
// 50-cell meters use one column, so with the banner it is a 44-row page: 6
// banner rows over 38 rows of body.
func sample() usage.Result {
	in := func(d time.Duration) time.Time { return now.Add(d) }
	return usage.Result{
		Windows: []provider.Window{
			{Provider: "claude", Name: "5h", Plan: "max", RemainingPercent: 68.0, ResetsAt: in(3*time.Hour + 38*time.Minute)},
			{Provider: "claude", Name: "weekly", Plan: "max", RemainingPercent: 65.0, ResetsAt: in(4*24*time.Hour + 17*time.Hour)},
			{Provider: "claude", Name: "fable weekly", Plan: "max", RemainingPercent: 65.0, ResetsAt: in(4*24*time.Hour + 17*time.Hour)},
			{Provider: "codex", Name: "weekly", Plan: "prolite", RemainingPercent: 99.0, ResetsAt: in(6*24*time.Hour + 9*time.Hour)},
			{Provider: "codex", Name: "GPT-5.3-Codex-Spark primary", Plan: "prolite", RemainingPercent: 100.0, ResetsAt: in(4*time.Hour + 59*time.Minute)},
			{Provider: "codex", Name: "GPT-5.3-Codex-Spark secondary", Plan: "prolite", RemainingPercent: 12.0, ResetsAt: in(6*24*time.Hour + 23*time.Hour), RateLimited: true},
			{Provider: "cursor", Name: "total", Plan: "free", RemainingPercent: 95.5, ResetsAt: in(6*24*time.Hour + 21*time.Hour)},
			{Provider: "cursor", Name: "auto", Plan: "free", RemainingPercent: 91.0},
		},
		Errors:     []usage.ProviderError{{Provider: "cursor", Message: "token expired, open cursor-agent to refresh"}},
		Undetected: []usage.ProviderError{{Provider: "opencode-go", Message: "not logged in, run opencode to log in"}},
	}
}

func newModel(t *testing.T, showBanner bool, providers ...provider.Provider) Model {
	t.Helper()
	return New(Options{
		Providers: providers,
		Banner:    showBanner,
		Now:       func() time.Time { return now },
	})
}

func TestNewStoresDashboardAppearance(t *testing.T) {
	th := theme.Default()
	th.Claude.Light = "#010203"
	m := New(Options{Theme: th, MeterWidth: 75, RefreshInterval: 17 * time.Second})
	if m.theme != th {
		t.Errorf("model theme = %#v, want %#v", m.theme, th)
	}
	if m.meterWidth != 75 {
		t.Errorf("model meter width = %d, want 75", m.meterWidth)
	}
	if m.refreshInterval != 17*time.Second {
		t.Errorf("model refresh interval = %s, want 17s", m.refreshInterval)
	}
}

func TestNewDefaultsDashboardAppearance(t *testing.T) {
	m := New(Options{})
	if m.theme != theme.Default() {
		t.Errorf("zero Options theme = %#v, want defaults %#v", m.theme, theme.Default())
	}
	if m.meterWidth != layout.DefaultMeterWidth {
		t.Errorf("zero Options meter width = %d, want %d", m.meterWidth, layout.DefaultMeterWidth)
	}
	if m.refreshInterval != DefaultRefreshInterval {
		t.Errorf("zero Options refresh interval = %s, want %s", m.refreshInterval, DefaultRefreshInterval)
	}
}

// step drives one message through the model and returns the new model and
// whatever command it asked for.
func step(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want dash.Model", next)
	}
	return got, cmd
}

func resize(t *testing.T, m Model, w, h int) Model {
	t.Helper()
	m, _ = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

func press(t *testing.T, m Model, keys ...string) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		m, cmd = step(t, m, keyMsg(k))
	}
	return m, cmd
}

// keyMsg builds the tea.KeyMsg whose String() is k.
func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "pgup":
		return tea.KeyMsg{Type: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
}

// shown is the model showing the sample result at w by h cells.
func shown(t *testing.T, showBanner bool, res usage.Result, w, h int) Model {
	t.Helper()
	m := newModel(t, showBanner)
	m = resize(t, m, w, h)
	m, _ = step(t, m, resultMsg{res: res})
	return m
}

func viewLines(t *testing.T, m Model) []string {
	t.Helper()
	return strings.Split(m.View(), "\n")
}

// windowIDs is the providers a result carries windows for, in order and
// without repeats, as one comparable string.
func windowIDs(res usage.Result) string {
	var ids []string
	for _, w := range res.Windows {
		if len(ids) == 0 || ids[len(ids)-1] != w.Provider {
			ids = append(ids, w.Provider)
		}
	}
	return strings.Join(ids, ",")
}

func pageOf(t *testing.T, res usage.Result, w int, showBanner bool) []string {
	t.Helper()
	return layout.Render(res, w, layout.Options{Banner: showBanner, Now: now})
}

func TestView_FillsTheTerminalExactly(t *testing.T) {
	for _, tc := range []struct{ w, h int }{{80, 24}, {100, 30}, {120, 40}, {40, 12}} {
		m := shown(t, true, sample(), tc.w, tc.h)
		got := viewLines(t, m)

		if len(got) != tc.h {
			t.Errorf("%dx%d: view is %d lines, want %d", tc.w, tc.h, len(got), tc.h)
			continue
		}
		for i, line := range got {
			if w := runewidth.StringWidth(line); w != tc.w {
				t.Errorf("%dx%d: line %d is %d cells, want %d: %q", tc.w, tc.h, i, w, tc.w, line)
			}
		}
		for i, art := range banner.Rows() {
			if strings.TrimRight(got[i], " ") != strings.TrimRight(art, " ") {
				t.Errorf("%dx%d: line %d = %q, want banner row %q", tc.w, tc.h, i, got[i], art)
			}
		}
		if last := got[len(got)-1]; !strings.Contains(last, "q quit") {
			t.Errorf("%dx%d: last line = %q, want the footer", tc.w, tc.h, last)
		}
	}
}

func TestView_BannerStaysPinnedWhileTheBodyScrolls(t *testing.T) {
	const w, h = 80, 24
	m := shown(t, true, sample(), w, h)
	body := pageOf(t, sample(), w, true)[banner.Height:]
	const visible = h - banner.Height - 1 // header and one footer row

	got := viewLines(t, m)
	for i := 0; i < visible; i++ {
		if got[banner.Height+i] != body[i] {
			t.Fatalf("unscrolled line %d = %q, want %q", banner.Height+i, got[banner.Height+i], body[i])
		}
	}

	m, _ = press(t, m, "j", "j", "j")
	got = viewLines(t, m)
	for i, art := range banner.Rows() {
		if strings.TrimRight(got[i], " ") != strings.TrimRight(art, " ") {
			t.Fatalf("after scrolling, line %d = %q, want banner row %q", i, got[i], art)
		}
	}
	for i := 0; i < visible; i++ {
		if got[banner.Height+i] != body[3+i] {
			t.Fatalf("scrolled line %d = %q, want %q", banner.Height+i, got[banner.Height+i], body[3+i])
		}
	}
}

func TestView_ScrollClampsAtBothEnds(t *testing.T) {
	const w, h = 80, 24
	body := pageOf(t, sample(), w, true)[banner.Height:]
	const visible = h - banner.Height - 1
	bottom := len(body) - visible

	m := shown(t, true, sample(), w, h)
	m, _ = press(t, m, "k", "up", "pgup")
	if m.offset != 0 {
		t.Errorf("scrolling up from the top left offset %d, want 0", m.offset)
	}

	m, _ = press(t, m, "end")
	if m.offset != bottom {
		t.Errorf("end left offset %d, want %d", m.offset, bottom)
	}
	m, _ = press(t, m, "j", "down", "pgdown", " ")
	if m.offset != bottom {
		t.Errorf("scrolling past the bottom left offset %d, want %d", m.offset, bottom)
	}

	m, _ = press(t, m, "g")
	if m.offset != 0 {
		t.Errorf("g left offset %d, want 0", m.offset)
	}
	m, _ = press(t, m, "G")
	if m.offset != bottom {
		t.Errorf("G left offset %d, want %d", m.offset, bottom)
	}
}

func TestView_PageScrollMovesExactlyOneScreenful(t *testing.T) {
	// 40x12 with the banner: 6 header rows and one footer row leave 5 body
	// rows on screen, so a page is 5 — not half of one, and not one row of
	// overlap.
	const w, h = 40, 12
	const visible = h - banner.Height - 1
	body := pageOf(t, sample(), w, true)[banner.Height:]
	if len(body) <= 2*visible {
		t.Fatalf("the sample body is %d rows at %dx%d, too short to page through", len(body), w, h)
	}

	for _, key := range []string{"pgdown", " "} {
		m := shown(t, true, sample(), w, h)
		m, _ = press(t, m, key)
		if m.offset != visible {
			t.Errorf("%q from the top left offset %d, want %d", key, m.offset, visible)
		}
		m, _ = press(t, m, key)
		if m.offset != 2*visible {
			t.Errorf("%q twice left offset %d, want %d", key, m.offset, 2*visible)
		}
	}

	for _, key := range []string{"pgup", "b"} {
		m := shown(t, true, sample(), w, h)
		m, _ = press(t, m, "pgdown", "pgdown")
		m, _ = press(t, m, key)
		if m.offset != visible {
			t.Errorf("%q left offset %d, want %d", key, m.offset, visible)
		}
		m, _ = press(t, m, key)
		if m.offset != 0 {
			t.Errorf("%q back to the top left offset %d, want 0", key, m.offset)
		}
	}
}

func TestView_FooterCountsTheRowsOutOfSight(t *testing.T) {
	const w, h = 80, 24
	m := shown(t, true, sample(), w, h)

	footer := func(m Model) string {
		lines := viewLines(t, m)
		return lines[len(lines)-1]
	}

	// 6 banner rows + 38 body rows, 17 of them on screen.
	top := footer(m)
	if !strings.Contains(top, "↓ 21 more") {
		t.Errorf("footer at the top = %q, want it to count 21 rows below", top)
	}
	if n := strings.Count(top, "more"); n != 1 {
		t.Errorf("footer at the top = %q, want exactly one hidden-row count", top)
	}
	for _, want := range []string{"r refresh", "q quit"} {
		if !strings.Contains(top, want) {
			t.Errorf("footer = %q, want it to mention %q", top, want)
		}
	}

	m, _ = press(t, m, "j", "j")
	mid := footer(m)
	if !strings.Contains(mid, "↑ 2 more") || !strings.Contains(mid, "↓ 19 more") {
		t.Errorf("footer after two lines = %q, want ↑ 2 more and ↓ 19 more", mid)
	}

	m, _ = press(t, m, "G")
	bottom := footer(m)
	if !strings.Contains(bottom, "↑ 21 more") {
		t.Errorf("footer at the bottom = %q, want ↑ 21 more", bottom)
	}
	if n := strings.Count(bottom, "more"); n != 1 {
		t.Errorf("footer at the bottom = %q, want exactly one hidden-row count", bottom)
	}
}

func TestView_NarrowFooterKeepsTheKeysOverTheCounts(t *testing.T) {
	m := shown(t, true, sample(), 40, 12)
	lines := viewLines(t, m)
	got := lines[len(lines)-1]

	if strings.Contains(got, "more") {
		t.Errorf("footer at 40 cells = %q, want the counts dropped", got)
	}
	for _, want := range []string{"↑↓ scroll", "r refresh", "q quit"} {
		if !strings.Contains(got, want) {
			t.Errorf("footer at 40 cells = %q, want it to keep %q", got, want)
		}
	}
}

func TestView_ShortPageNeitherScrollsNorCounts(t *testing.T) {
	m := shown(t, true, usage.Result{}, 80, 40)
	lines := viewLines(t, m)
	if got := lines[len(lines)-1]; strings.Contains(got, "more") {
		t.Errorf("footer = %q, want no hidden-row counts", got)
	}
	m, _ = press(t, m, "j", "pgdown", "G")
	if m.offset != 0 {
		t.Errorf("a page that fits scrolled to %d, want 0", m.offset)
	}
}

func TestUpdate_ResizeRedrawsAtTheNewWidth(t *testing.T) {
	m := shown(t, true, sample(), 80, 24)
	m = resize(t, m, 120, 40)

	got := viewLines(t, m)
	if len(got) != 40 {
		t.Fatalf("view is %d lines, want 40", len(got))
	}
	for i, line := range got {
		if w := runewidth.StringWidth(line); w != 120 {
			t.Fatalf("line %d is %d cells, want 120: %q", i, w, line)
		}
	}
}

func TestUpdate_ResizeClampsTheOffset(t *testing.T) {
	m := shown(t, true, sample(), 80, 24)
	m, _ = press(t, m, "G")
	if m.offset == 0 {
		t.Fatal("the sample page must be scrollable at 80x24")
	}

	// A taller terminal shows the whole page, so there is nothing left to
	// scroll past.
	m = resize(t, m, 80, 60)
	if m.offset != 0 {
		t.Errorf("offset = %d after growing the terminal, want 0", m.offset)
	}
}

func TestUpdate_NewResultClampsTheOffset(t *testing.T) {
	m := shown(t, true, sample(), 80, 24)
	m, _ = press(t, m, "G")
	if m.offset == 0 {
		t.Fatal("the sample page must be scrollable at 80x24")
	}

	m, _ = step(t, m, resultMsg{res: usage.Result{}})
	if m.offset != 0 {
		t.Errorf("offset = %d after a shorter result, want 0", m.offset)
	}
}

func TestUpdate_QuitKeys(t *testing.T) {
	for _, key := range []string{"q", "esc", "ctrl+c"} {
		m := shown(t, true, sample(), 80, 24)
		_, cmd := press(t, m, key)
		if cmd == nil {
			t.Errorf("%q returned no command, want tea.Quit", key)
			continue
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%q returned %T, want tea.QuitMsg", key, cmd())
		}
	}
}

func TestUpdate_RRefetches(t *testing.T) {
	// Two providers, with different IDs: a refresh fetches every provider
	// the model was given, never one singled out by name.
	fake := providertest.Succeeding("claude", []provider.Window{
		{Provider: "claude", Name: "5h", Plan: "max", RemainingPercent: 50},
	})
	other := providertest.Succeeding("codex", []provider.Window{
		{Provider: "codex", Name: "weekly", Plan: "pro", RemainingPercent: 30},
	})
	m := newModel(t, true, fake, other)
	m = resize(t, m, 80, 24)
	m, _ = step(t, m, resultMsg{res: sample()})

	m, cmd := press(t, m, "r")
	if cmd == nil {
		t.Fatal("r returned no command, want a fetch")
	}
	lines := viewLines(t, m)
	if got := lines[len(lines)-1]; !strings.Contains(got, "⠋ refreshing") || strings.Contains(got, "…") {
		t.Errorf("footer while refreshing = %q, want a spinner before refreshing and no ellipsis", got)
	}
	// The old page is still on screen while the new one is fetched.
	body := pageOf(t, sample(), 80, true)[banner.Height:]
	if lines[banner.Height] != body[0] {
		t.Errorf("first body line = %q, want the previous result's %q", lines[banner.Height], body[0])
	}

	msg := fetchResult(t, cmd)
	if got := windowIDs(msg.res); got != "claude,codex" {
		t.Fatalf("the fetch returned windows for %q, want claude,codex", got)
	}
	if fake.Fetches() != 1 || other.Fetches() != 1 {
		t.Errorf("providers were fetched %d and %d times, want 1 each", fake.Fetches(), other.Fetches())
	}

	// A second r while the first is still in flight is ignored.
	m, again := press(t, m, "r")
	if again != nil {
		t.Error("r while already refreshing started a second fetch")
	}
	m, _ = step(t, m, msg)
	if m.loading {
		t.Error("the model is still loading after its result arrived")
	}
}

func TestAutoRefreshStartsOnlyAfterAResultAndUsesTheConfiguredInterval(t *testing.T) {
	var intervals []time.Duration
	saved := scheduleTick
	t.Cleanup(func() { scheduleTick = saved })
	scheduleTick = func(_ context.Context, d time.Duration, fn func(time.Time) tea.Msg) (tea.Cmd, context.CancelFunc) {
		intervals = append(intervals, d)
		return func() tea.Msg { return fn(now) }, func() {}
	}

	m := New(Options{RefreshInterval: 17 * time.Second})
	if cmd := m.Init(); cmd == nil {
		t.Fatal("Init returned no fetch")
	}
	if len(intervals) != 0 {
		t.Fatalf("Init scheduled %d timers before the first result", len(intervals))
	}

	m, timer := step(t, m, resultMsg{res: sample()})
	if timer == nil {
		t.Fatal("completed initial fetch scheduled no refresh timer")
	}
	if len(intervals) != 1 || intervals[0] != 17*time.Second {
		t.Fatalf("scheduled intervals = %v, want [17s]", intervals)
	}
	if _, ok := timer().(refreshMsg); !ok {
		t.Fatalf("timer produced %T, want refreshMsg", timer())
	}
	if m.loading {
		t.Error("model remained loading after the result")
	}
}

func TestValidAutoRefreshNeverOverlapsAndCompletionSchedulesTheNextTimer(t *testing.T) {
	var tokens []refreshMsg
	saved := scheduleTick
	t.Cleanup(func() { scheduleTick = saved })
	scheduleTick = func(_ context.Context, _ time.Duration, fn func(time.Time) tea.Msg) (tea.Cmd, context.CancelFunc) {
		return func() tea.Msg {
			msg := fn(now).(refreshMsg)
			tokens = append(tokens, msg)
			return msg
		}, func() {}
	}

	fake := providertest.Succeeding("claude", []provider.Window{{Name: "5h", RemainingPercent: 50}})
	m := newModel(t, true, fake)
	m, timer := step(t, m, resultMsg{res: sample()})
	timerMsg := timer().(refreshMsg)

	m, fetch := step(t, m, timerMsg)
	if fetch == nil || !m.loading {
		t.Fatal("valid timer did not start a loading fetch")
	}
	if _, overlap := step(t, m, timerMsg); overlap != nil {
		t.Error("a timer while loading started an overlapping fetch")
	}
	if fake.Fetches() != 0 {
		t.Fatal("provider fetched before the returned fetch command ran")
	}

	result := fetchResult(t, fetch)
	if fake.Fetches() != 1 {
		t.Fatalf("provider fetched %d times, want 1", fake.Fetches())
	}
	m, nextTimer := step(t, m, result)
	if nextTimer == nil || m.loading {
		t.Fatal("fetch completion did not finish loading and schedule the next timer")
	}
	nextMsg := nextTimer().(refreshMsg)
	if nextMsg.generation == timerMsg.generation {
		t.Errorf("next timer reused generation %d", nextMsg.generation)
	}
	if len(tokens) != 2 {
		t.Errorf("timer commands produced %d messages, want 2", len(tokens))
	}
}

func TestManualRefreshInvalidatesTheScheduledTimerAndRestartsAfterCompletion(t *testing.T) {
	saved := scheduleTick
	t.Cleanup(func() { scheduleTick = saved })
	scheduleTick = func(_ context.Context, _ time.Duration, fn func(time.Time) tea.Msg) (tea.Cmd, context.CancelFunc) {
		return func() tea.Msg { return fn(now) }, func() {}
	}

	fake := providertest.Succeeding("claude", []provider.Window{{Name: "5h", RemainingPercent: 50}})
	m := newModel(t, true, fake)
	m, timer := step(t, m, resultMsg{res: sample()})
	stale := timer().(refreshMsg)

	m, manualFetch := press(t, m, "r")
	if manualFetch == nil || !m.loading {
		t.Fatal("manual refresh did not start a fetch")
	}
	if _, cmd := step(t, m, stale); cmd != nil {
		t.Error("stale timer started a fetch while the manual refresh was loading")
	}

	result := fetchResult(t, manualFetch)
	m, nextTimer := step(t, m, result)
	if nextTimer == nil {
		t.Fatal("manual refresh completion scheduled no fresh timer")
	}
	if _, cmd := step(t, m, stale); cmd != nil {
		t.Error("stale timer started an early fetch after manual completion")
	}
	if m.loading {
		t.Error("stale timer changed the model to loading")
	}
	if next := nextTimer().(refreshMsg); next.generation == stale.generation {
		t.Errorf("fresh timer reused stale generation %d", stale.generation)
	}
}

// startPendingRefresh runs a real, hour-long timer command and returns only
// once that command has started. The deadline is deliberately far away: these
// tests pass only when model cancellation releases the command.
func startPendingRefresh(t *testing.T, m Model) (Model, <-chan tea.Msg) {
	t.Helper()

	saved := scheduleTick
	defer func() { scheduleTick = saved }()
	started := make(chan struct{})
	scheduleTick = func(ctx context.Context, d time.Duration, fn func(time.Time) tea.Msg) (tea.Cmd, context.CancelFunc) {
		cmd, cancel := saved(ctx, d, fn)
		return func() tea.Msg {
			close(started)
			return cmd()
		}, cancel
	}
	m.refreshInterval = time.Hour
	m, timer := step(t, m, resultMsg{res: sample()})

	done := make(chan tea.Msg, 1)
	go func() { done <- timer() }()
	<-started
	select {
	case msg := <-done:
		t.Fatalf("hour-long timer returned before cancellation with %T", msg)
	default:
	}
	return m, done
}

func requireTimerReleased(t *testing.T, done <-chan tea.Msg) {
	t.Helper()
	select {
	case msg := <-done:
		if msg != nil {
			t.Fatalf("canceled timer returned %T, want nil", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("pending timer command was not released after cancellation")
	}
}

func TestManualRefreshCancelsThePendingTimerCommand(t *testing.T) {
	m, done := startPendingRefresh(t, newModel(t, true))
	m, fetch := press(t, m, "r")
	if fetch == nil || !m.loading {
		t.Fatal("manual refresh did not start a fetch")
	}
	requireTimerReleased(t, done)
}

func TestReplacingARefreshTimerCancelsThePendingCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	m, done := startPendingRefresh(t, New(Options{Ctx: ctx}))

	m, replacement := step(t, m, resultMsg{res: sample()})
	if replacement == nil {
		t.Fatal("replacement result scheduled no new timer")
	}
	requireTimerReleased(t, done)
	_, _ = press(t, m, "q")
}

func TestQuitKeysCancelThePendingTimerCommand(t *testing.T) {
	for _, key := range []string{"q", "esc", "ctrl+c"} {
		t.Run(key, func(t *testing.T) {
			m, done := startPendingRefresh(t, newModel(t, true))
			_, quit := press(t, m, key)
			if quit == nil {
				t.Fatalf("%q returned no quit command", key)
			}
			requireTimerReleased(t, done)
		})
	}
}

func TestParentContextCancellationReleasesThePendingTimerCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_, done := startPendingRefresh(t, New(Options{Ctx: ctx}))

	cancel()
	requireTimerReleased(t, done)
}

func TestView_LoadingBeforeTheFirstResult(t *testing.T) {
	m := newModel(t, true)
	m = resize(t, m, 80, 24)

	got := viewLines(t, m)
	if len(got) != 24 {
		t.Fatalf("view is %d lines, want 24", len(got))
	}
	if !strings.Contains(got[banner.Height], "fetching…") {
		t.Errorf("first body line = %q, want fetching…", got[banner.Height])
	}
	footer := got[len(got)-1]
	if !strings.Contains(footer, "⠋ fetching") || !strings.Contains(footer, "q quit") {
		t.Errorf("footer during the first fetch = %q, want a spinner, fetching and q quit", footer)
	}
	if strings.Contains(footer, "scroll") || strings.Contains(footer, "refresh") || strings.Contains(footer, "updated") {
		t.Errorf("footer during the first fetch = %q, want no keys that do nothing yet and no update time", footer)
	}
}

func TestInit_StartsAFetch(t *testing.T) {
	fake := providertest.Succeeding("claude", []provider.Window{
		{Provider: "claude", Name: "5h", RemainingPercent: 10},
	})
	other := providertest.Succeeding("cursor", []provider.Window{
		{Provider: "cursor", Name: "total", RemainingPercent: 20},
	})
	m := newModel(t, true, fake, other)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init returned no command, want a fetch")
	}
	msg := fetchResult(t, cmd)
	// Every provider, not a single one picked by name.
	if got := windowIDs(msg.res); got != "claude,cursor" {
		t.Fatalf("Init fetched windows for %q, want claude,cursor", got)
	}
	if fake.Fetches() != 1 || other.Fetches() != 1 {
		t.Errorf("providers were fetched %d and %d times, want 1 each", fake.Fetches(), other.Fetches())
	}
}

func TestView_TooNarrow(t *testing.T) {
	m := shown(t, true, sample(), layout.MinWidth-1, 10)
	got := viewLines(t, m)

	if len(got) != 10 {
		t.Fatalf("view is %d lines, want 10", len(got))
	}
	if !strings.Contains(got[0], "terminal too narrow") {
		t.Errorf("first line = %q, want terminal too narrow", got[0])
	}
	for i, line := range got {
		if w := runewidth.StringWidth(line); w != layout.MinWidth-1 {
			t.Errorf("line %d is %d cells, want %d: %q", i, w, layout.MinWidth-1, line)
		}
	}
}

func TestView_NoProvidersDetected(t *testing.T) {
	m := shown(t, true, usage.Result{}, 80, 24)
	got := viewLines(t, m)

	if !strings.Contains(got[banner.Height], "no providers detected") {
		t.Errorf("first body line = %q, want no providers detected", got[banner.Height])
	}
	if !strings.Contains(strings.TrimRight(got[0], " "), "_") {
		t.Errorf("line 0 = %q, want the banner", got[0])
	}
}

func TestView_WithoutTheBannerTheSummaryLineIsTheHeader(t *testing.T) {
	const w, h = 80, 24
	m := shown(t, false, sample(), w, h)
	page := pageOf(t, sample(), w, false)
	got := viewLines(t, m)

	if got[0] != page[0] {
		t.Fatalf("header = %q, want the summary line %q", got[0], page[0])
	}
	if !strings.HasPrefix(got[0], "qmeter") {
		t.Fatalf("header = %q, want it to start with qmeter", got[0])
	}
	// One header row, so one more body row fits than with the banner.
	m, _ = press(t, m, "j")
	if got = viewLines(t, m); got[0] != page[0] {
		t.Fatalf("header after scrolling = %q, want it pinned", got[0])
	}
	if got[1] != page[2] {
		t.Fatalf("first body line = %q, want %q", got[1], page[2])
	}
}

func TestView_BeforeTheFirstResize(t *testing.T) {
	m := newModel(t, true)
	if got := m.View(); got != "" {
		t.Errorf("View before a WindowSizeMsg = %q, want empty", got)
	}
}

// fetchResult runs a fetch command and returns the result it produces. A
// fetch shares its command with the spinner tick, so the result may sit
// inside a batch; every other command in the batch is run too, so the
// spinner's real timer costs the test one frame.
func fetchResult(t *testing.T, cmd tea.Cmd) resultMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("no fetch command")
	}
	switch msg := cmd().(type) {
	case resultMsg:
		return msg
	case tea.BatchMsg:
		var found *resultMsg
		for _, c := range msg {
			if c == nil {
				continue
			}
			if r, ok := c().(resultMsg); ok {
				r := r
				found = &r
			}
		}
		if found != nil {
			return *found
		}
		t.Fatal("the batch produced no resultMsg")
	default:
		t.Fatalf("the fetch produced %T, want a resultMsg", msg)
	}
	return resultMsg{}
}

// footerOf is the last line of the view, right-trimmed.
func footerOf(t *testing.T, m Model) string {
	t.Helper()
	lines := viewLines(t, m)
	return strings.TrimRight(lines[len(lines)-1], " ")
}

// TestView_FooterSaysWhenTheNumbersWereLastTrue: once a result is on
// screen the footer ends with the time it arrived, right-aligned, and
// keeps it through the next refresh so the old page is dated while the
// new one is fetched.
func TestView_FooterSaysWhenTheNumbersWereLastTrue(t *testing.T) {
	m := shown(t, true, sample(), 100, 24)
	if got := footerOf(t, m); !strings.HasSuffix(got, "updated 14:22:07") {
		t.Errorf("footer = %q, want it to end with updated 14:22:07", got)
	}
	if line := viewLines(t, m)[23]; runewidth.StringWidth(line) != 100 {
		t.Errorf("footer is %d cells, want 100", runewidth.StringWidth(line))
	}
	m, _ = press(t, m, "r")
	got := footerOf(t, m)
	if !strings.HasSuffix(got, "updated 14:22:07") || !strings.Contains(got, "refreshing") {
		t.Errorf("footer while refreshing = %q, want refreshing and the previous update time", got)
	}
}

// TestView_FooterDropsTheTimestampBeforeTheCounts: on a terminal too
// narrow for everything the update time goes first, then the row counts,
// and the keys never. At 56 cells the counts and keys fit (45 cells) but
// the timestamp (16 cells plus a gap) does not; at 64 everything fits.
func TestView_FooterDropsTheTimestampBeforeTheCounts(t *testing.T) {
	narrow := footerOf(t, shown(t, true, sample(), 56, 24))
	if strings.Contains(narrow, "updated") {
		t.Errorf("footer at 56 cells = %q, want the update time dropped", narrow)
	}
	if !strings.Contains(narrow, "more") || !strings.Contains(narrow, "q quit") {
		t.Errorf("footer at 56 cells = %q, want the counts and keys kept", narrow)
	}
	wide := footerOf(t, shown(t, true, sample(), 64, 24))
	if !strings.Contains(wide, "more") || !strings.HasSuffix(wide, "updated 14:22:07") {
		t.Errorf("footer at 64 cells = %q, want the counts and the update time", wide)
	}
}

// TestSpinnerTurnsOnlyWhileAFetchIsInFlight: starting a fetch also starts
// the spinner, each tick advances it one frame and asks for the next, and
// a tick that arrives after the result is ignored so the timer dies.
func TestSpinnerTurnsOnlyWhileAFetchIsInFlight(t *testing.T) {
	fake := providertest.Succeeding("claude", []provider.Window{{Name: "5h", RemainingPercent: 50}})
	m := shown(t, true, sample(), 80, 24)
	m.providers = []provider.Provider{fake}

	m, cmd := press(t, m, "r")
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("r returned a %T, want a batch of the fetch and the spinner tick", cmd())
	}
	spun := false
	for _, c := range batch {
		if _, ok := c().(spinMsg); ok {
			spun = true
		}
	}
	if !spun {
		t.Fatal("the fetch batch carries no spinner tick")
	}
	if got := footerOf(t, m); !strings.Contains(got, "⠋ refreshing") {
		t.Errorf("footer at the start of a fetch = %q, want the first spinner frame", got)
	}

	m, next := step(t, m, spinMsg{})
	if next == nil {
		t.Fatal("a spinner tick while loading scheduled no next tick")
	}
	if got := footerOf(t, m); !strings.Contains(got, "⠙ refreshing") {
		t.Errorf("footer after one tick = %q, want the second spinner frame", got)
	}

	m, _ = step(t, m, resultMsg{res: sample()})
	m, late := step(t, m, spinMsg{})
	if late != nil {
		t.Error("a spinner tick after the result scheduled another tick")
	}
	if got := footerOf(t, m); strings.Contains(got, "refreshing") || !strings.Contains(got, "r refresh") {
		t.Errorf("footer after the result = %q, want the keys back", got)
	}
}

func TestVerticalSurvivesResizeAndScroll(t *testing.T) {
	for _, showBanner := range []bool{false, true} {
		m := New(Options{Vertical: true, Banner: showBanner, MeterWidth: 22, Now: func() time.Time { return now }})
		m, _ = step(t, m, resultMsg{res: sample()})
		t.Cleanup(func() {
			if m.cancelRefresh != nil {
				m.cancelRefresh()
			}
		})
		for _, width := range []int{110, 36, 260, 132} {
			m = resize(t, m, width, 16)
			m, _ = press(t, m, "home")
			lines := viewLines(t, m)
			foundGauge := false
			for _, line := range lines {
				start, end := strings.Index(line, "╭"), strings.Index(line, "╮")
				if start >= 0 && end > start {
					foundGauge = true
					if got := runewidth.StringWidth(line[start:end]) + 1; got != width-14 {
						t.Fatalf("width %d: gauge width = %d, want %d", width, got, width-14)
					}
				}
			}
			if !foundGauge {
				t.Fatalf("width %d: no gauge visible", width)
			}
			for _, line := range lines {
				if runewidth.StringWidth(line) != width {
					t.Fatalf("line width != %d: %q", width, line)
				}
			}
			for !strings.Contains(m.View(), "▲ cursor") {
				previous := m.offset
				m, _ = press(t, m, "j")
				if m.offset == previous {
					t.Fatalf("width %d: scrolling did not reach cursor", width)
				}
			}
			m, _ = press(t, m, "end")
			if m.offset == 0 || !strings.Contains(m.View(), "token expired") {
				t.Fatalf("width %d: end did not reach the last provider's error", width)
			}
			if viewLines(t, m)[0] != lines[0] {
				t.Fatal("scrolling moved pinned header")
			}
		}
	}
}
