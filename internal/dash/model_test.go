package dash

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/Harrison-Blair/qmeter/internal/dash/theme"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/provider/providertest"
	"github.com/Harrison-Blair/qmeter/internal/usage"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

var now = time.Date(2026, 9, 16, 14, 22, 7, 0, time.UTC)

func sample() usage.Result {
	in := func(d time.Duration) time.Time { return now.Add(d) }
	return usage.Result{
		Windows: []provider.Window{
			{Provider: "claude", Name: "5h", Plan: "max", RemainingPercent: 68, ResetsAt: in(3*time.Hour + 38*time.Minute)},
			{Provider: "claude", Name: "weekly", Plan: "max", RemainingPercent: 65, ResetsAt: in(4*24*time.Hour + 17*time.Hour)},
			{Provider: "claude", Name: "fable weekly", Plan: "max", RemainingPercent: 65, ResetsAt: in(4*24*time.Hour + 17*time.Hour)},
			{Provider: "codex", Name: "weekly", Plan: "prolite", RemainingPercent: 99, ResetsAt: in(6*24*time.Hour + 9*time.Hour)},
			{Provider: "codex", Name: "GPT-5.3-Codex-Spark primary", Plan: "prolite", RemainingPercent: 100, ResetsAt: in(4*time.Hour + 59*time.Minute)},
			{Provider: "codex", Name: "GPT-5.3-Codex-Spark secondary", Plan: "prolite", RemainingPercent: 12, ResetsAt: in(6*24*time.Hour + 23*time.Hour), RateLimited: true},
			{Provider: "cursor", Name: "total", Plan: "free", RemainingPercent: 95.5, ResetsAt: in(6*24*time.Hour + 21*time.Hour)},
			{Provider: "cursor", Name: "auto", Plan: "free", RemainingPercent: 91},
		},
		Errors:     []usage.ProviderError{{Provider: "cursor", Message: "token expired, open cursor-agent to refresh"}},
		Undetected: []usage.ProviderError{{Provider: "opencode-go", Message: "not logged in, run opencode to log in"}},
	}
}

func step(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want dash.Model", next)
	}
	return got, cmd
}

func resize(t *testing.T, m Model, width, height int) Model {
	t.Helper()
	m, _ = step(t, m, tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

func press(t *testing.T, m Model, key string) (Model, tea.Cmd) {
	t.Helper()
	return step(t, m, keyMsg(key))
}

func keyMsg(key string) tea.KeyMsg {
	switch key {
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "pgup":
		return tea.KeyMsg{Type: tea.KeyPgUp}
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
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
}

func shown(t *testing.T, result usage.Result, width, height int) Model {
	t.Helper()
	m := New(Options{Now: func() time.Time { return now }})
	m = resize(t, m, width, height)
	m, _ = step(t, m, resultMsg{res: result})
	return m
}

func TestNewUsesThemeAndRefreshDefaults(t *testing.T) {
	th := theme.Default()
	th.Claude.Light = "#010203"
	m := New(Options{Theme: th, RefreshInterval: 17 * time.Second})
	if m.theme != th {
		t.Errorf("theme = %#v, want %#v", m.theme, th)
	}
	if m.refreshInterval != 17*time.Second {
		t.Errorf("refresh interval = %s, want 17s", m.refreshInterval)
	}
	if got := New(Options{}); got.refreshInterval != DefaultRefreshInterval || got.theme != theme.Default() {
		t.Errorf("zero options defaults = (%s, %#v)", got.refreshInterval, got.theme)
	}
}

func TestInitFetchesEveryProvider(t *testing.T) {
	claude := providertest.Succeeding("claude", []provider.Window{{Provider: "claude", Name: "5h"}})
	cursor := providertest.Succeeding("cursor", []provider.Window{{Provider: "cursor", Name: "total"}})
	m := New(Options{Providers: []provider.Provider{claude, cursor}})
	msg := fetchResult(t, m.Init())
	if got := windowIDs(msg.res); got != "claude,cursor" {
		t.Fatalf("Init fetched %q, want claude,cursor", got)
	}
	if claude.Fetches() != 1 || cursor.Fetches() != 1 {
		t.Fatalf("fetch counts = %d/%d, want one each", claude.Fetches(), cursor.Fetches())
	}
}

func TestRefreshKeyStartsOneFetchAndIgnoresRepeatedRefresh(t *testing.T) {
	fake := providertest.Succeeding("claude", []provider.Window{{Provider: "claude", Name: "5h"}})
	m := shown(t, sample(), 80, 24)
	m.providers = []provider.Provider{fake}

	m, cmd := press(t, m, "r")
	if cmd == nil || !m.loading {
		t.Fatal("r did not start a fetch")
	}
	if view := m.View(); !strings.Contains(view, "codex") || !strings.Contains(view, "refreshing · q quit") {
		t.Fatalf("refresh frame did not keep the previous result and show loading: %q", view)
	}
	if got := fetchResult(t, cmd); len(got.res.Windows) != 1 || fake.Fetches() != 1 {
		t.Fatalf("fetch result = %#v, fetch count %d", got.res, fake.Fetches())
	}
	if _, again := press(t, m, "r"); again != nil {
		t.Fatal("r during a fetch started another fetch")
	}
	m, _ = step(t, m, resultMsg{res: usage.Result{}})
	if m.loading {
		t.Fatal("model remained loading after the fetch result")
	}
}

func TestCombinedKeyRunesDispatchDashboardKeys(t *testing.T) {
	t.Run("quit", func(t *testing.T) {
		m := shown(t, sample(), 140, 45)
		_, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("rq")})
		if cmd == nil {
			t.Fatal("combined r and q input returned no command")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("combined r and q input returned %T, want tea.QuitMsg", cmd())
		}
	})

	t.Run("refresh", func(t *testing.T) {
		m := shown(t, sample(), 140, 45)
		updated, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ar")})
		if cmd == nil || !updated.loading {
			t.Fatal("r in combined input did not start a refresh")
		}
		if !strings.Contains(updated.View(), "refreshing · q quit") {
			t.Fatal("r in combined input did not show the refresh state")
		}
	})

	t.Run("paste does not activate keys", func(t *testing.T) {
		m := shown(t, sample(), 140, 45)
		updated, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("rq"), Paste: true})
		if cmd != nil || updated.loading {
			t.Fatal("pasted text activated a dashboard key")
		}
	})
}

func TestQuitKeysWorkAcrossDashboardStates(t *testing.T) {
	states := []struct {
		name  string
		model func() Model
	}{
		{
			name: "first fetch",
			model: func() Model {
				return resize(t, New(Options{}), 140, 45)
			},
		},
		{
			name: "manual refresh",
			model: func() Model {
				m, _ := press(t, shown(t, sample(), 140, 45), "r")
				return m
			},
		},
		{
			name: "automatic refresh",
			model: func() Model {
				m := shown(t, sample(), 140, 45)
				m, _ = step(t, m, refreshMsg{generation: m.refreshGeneration})
				return m
			},
		},
		{
			name: "resized",
			model: func() Model {
				return resize(t, shown(t, sample(), 140, 45), 200, 60)
			},
		},
	}

	for _, state := range states {
		for _, key := range []struct {
			name string
			msg  tea.KeyMsg
		}{
			{name: "combined rq", msg: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("rq")}},
			{name: "esc", msg: keyMsg("esc")},
			{name: "ctrl+c", msg: keyMsg("ctrl+c")},
		} {
			t.Run(state.name+"/"+key.name, func(t *testing.T) {
				_, cmd := step(t, state.model(), key.msg)
				if cmd == nil {
					t.Fatalf("%q returned no quit command", key.name)
				}
				msg := cmd()
				if _, ok := msg.(tea.QuitMsg); !ok {
					t.Fatalf("%q returned %T, want tea.QuitMsg", key.name, msg)
				}
			})
		}
	}
}

func TestQuitKeysReturnQuitAndCancelRefreshTimer(t *testing.T) {
	for _, key := range []string{"q", "esc", "ctrl+c"} {
		t.Run(key, func(t *testing.T) {
			m, done := startPendingRefresh(t, New(Options{}))
			_, cmd := press(t, m, key)
			if cmd == nil {
				t.Fatalf("%q returned no command", key)
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatalf("%q returned %T, want tea.QuitMsg", key, cmd())
			}
			requireTimerReleased(t, done)
		})
	}
}

func TestAutoRefreshStartsAfterAResultAndUsesTheConfiguredInterval(t *testing.T) {
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
		t.Fatalf("Init scheduled timers before a result: %v", intervals)
	}
	m, timer := step(t, m, resultMsg{res: sample()})
	if timer == nil || len(intervals) != 1 || intervals[0] != 17*time.Second {
		t.Fatalf("timer=%v intervals=%v, want one 17s timer", timer != nil, intervals)
	}
	if _, ok := timer().(refreshMsg); !ok || m.loading {
		t.Fatalf("timer result or loading state invalid: %T, loading=%v", timer(), m.loading)
	}
}

func TestValidAutomaticRefreshStartsFetchAndReschedules(t *testing.T) {
	saved := scheduleTick
	t.Cleanup(func() { scheduleTick = saved })
	scheduleTick = func(_ context.Context, _ time.Duration, fn func(time.Time) tea.Msg) (tea.Cmd, context.CancelFunc) {
		return func() tea.Msg { return fn(now) }, func() {}
	}

	fake := providertest.Succeeding("claude", []provider.Window{{Provider: "claude", Name: "5h"}})
	m := New(Options{Providers: []provider.Provider{fake}})
	m, timer := step(t, m, resultMsg{res: sample()})
	first := timer().(refreshMsg)
	m, fetch := step(t, m, first)
	if fetch == nil || !m.loading {
		t.Fatal("valid timer did not start a fetch")
	}
	if _, overlap := step(t, m, first); overlap != nil {
		t.Fatal("timer while loading started an overlapping fetch")
	}
	result := fetchResult(t, fetch)
	if fake.Fetches() != 1 {
		t.Fatalf("provider fetched %d times, want one", fake.Fetches())
	}
	m, next := step(t, m, result)
	if next == nil || m.loading {
		t.Fatal("fetch result did not schedule the next timer")
	}
	if next().(refreshMsg).generation == first.generation {
		t.Fatal("next timer reused the previous generation")
	}
}

func TestAutoRefreshRejectsStaleAndOverlappingTicks(t *testing.T) {
	saved := scheduleTick
	t.Cleanup(func() { scheduleTick = saved })
	scheduleTick = func(_ context.Context, _ time.Duration, fn func(time.Time) tea.Msg) (tea.Cmd, context.CancelFunc) {
		return func() tea.Msg { return fn(now) }, func() {}
	}

	fake := providertest.Succeeding("claude", []provider.Window{{Provider: "claude", Name: "5h"}})
	m := New(Options{Providers: []provider.Provider{fake}})
	m, firstTimer := step(t, m, resultMsg{res: sample()})
	stale := firstTimer().(refreshMsg)
	m, manualFetch := press(t, m, "r")
	if manualFetch == nil || !m.loading {
		t.Fatal("manual refresh did not start")
	}
	if _, cmd := step(t, m, stale); cmd != nil {
		t.Fatal("stale timer started an overlapping fetch")
	}
	result := fetchResult(t, manualFetch)
	m, nextTimer := step(t, m, result)
	if nextTimer == nil || m.loading {
		t.Fatal("fetch completion did not schedule a new timer")
	}
	if _, cmd := step(t, m, stale); cmd != nil {
		t.Fatal("stale timer started an early fetch after completion")
	}
	if nextTimer().(refreshMsg).generation == stale.generation {
		t.Fatal("replacement timer reused the stale generation")
	}
}

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
		t.Fatal("pending timer command was not released")
	}
}

func TestManualRefreshCancelsTimerCommand(t *testing.T) {
	m, done := startPendingRefresh(t, New(Options{}))
	m, cmd := press(t, m, "r")
	if cmd == nil || !m.loading {
		t.Fatal("manual refresh did not start")
	}
	requireTimerReleased(t, done)
}

func TestReplacingTimerCancelsPendingCommand(t *testing.T) {
	m, done := startPendingRefresh(t, New(Options{}))
	m, replacement := step(t, m, resultMsg{res: sample()})
	if replacement == nil {
		t.Fatal("replacement result scheduled no timer")
	}
	requireTimerReleased(t, done)
	m.cancelRefreshTimer()
}

func TestParentCancellationReleasesTimerCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m, done := startPendingRefresh(t, New(Options{Ctx: ctx}))
	_ = m
	cancel()
	requireTimerReleased(t, done)
}

func TestSpinnerRunsOnlyWhileLoading(t *testing.T) {
	m := New(Options{Now: func() time.Time { return now }})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if !strings.Contains(m.View(), "⠋ refreshing · q quit") {
		t.Fatal("initial fetch frame has no spinner")
	}
	m, next := step(t, m, spinMsg{})
	if next == nil || !strings.Contains(m.View(), "⠙ refreshing · q quit") {
		t.Fatal("spinner did not advance while loading")
	}
	m, _ = step(t, m, resultMsg{res: sample()})
	m, late := step(t, m, spinMsg{})
	if late != nil || !strings.Contains(m.View(), "r refresh · q quit") || strings.Contains(m.View(), "refreshing") {
		t.Fatal("spinner continued after the result arrived")
	}
}

func fetchResult(t *testing.T, cmd tea.Cmd) resultMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("no fetch command")
	}
	msg := cmd()
	if result, ok := msg.(resultMsg); ok {
		return result
	}
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("fetch command produced %T, want result or batch", msg)
	}
	for _, child := range batch {
		if child == nil {
			continue
		}
		if result, ok := child().(resultMsg); ok {
			return result
		}
	}
	t.Fatal("fetch batch produced no result")
	return resultMsg{}
}

func windowIDs(result usage.Result) string {
	var ids []string
	for _, window := range result.Windows {
		if len(ids) == 0 || ids[len(ids)-1] != window.Provider {
			ids = append(ids, window.Provider)
		}
	}
	return strings.Join(ids, ",")
}
