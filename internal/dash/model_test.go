package dash

import (
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
// provider and one that was never detected. At 80 cells with the banner it
// is a 29-row page: 6 banner rows over 23 rows of body.
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

func TestView_FooterCountsTheRowsOutOfSight(t *testing.T) {
	const w, h = 80, 24
	m := shown(t, true, sample(), w, h)

	footer := func(m Model) string {
		lines := viewLines(t, m)
		return lines[len(lines)-1]
	}

	// 6 banner rows + 23 body rows, 17 of them on screen.
	top := footer(m)
	if !strings.Contains(top, "↓ 6 more") {
		t.Errorf("footer at the top = %q, want it to count 6 rows below", top)
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
	if !strings.Contains(mid, "↑ 2 more") || !strings.Contains(mid, "↓ 4 more") {
		t.Errorf("footer after two lines = %q, want ↑ 2 more and ↓ 4 more", mid)
	}

	m, _ = press(t, m, "G")
	bottom := footer(m)
	if !strings.Contains(bottom, "↑ 6 more") {
		t.Errorf("footer at the bottom = %q, want ↑ 6 more", bottom)
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
	fake := providertest.Succeeding("claude", []provider.Window{
		{Provider: "claude", Name: "5h", Plan: "max", RemainingPercent: 50},
	})
	m := newModel(t, true, fake)
	m = resize(t, m, 80, 24)
	m, _ = step(t, m, resultMsg{res: sample()})

	m, cmd := press(t, m, "r")
	if cmd == nil {
		t.Fatal("r returned no command, want a fetch")
	}
	lines := viewLines(t, m)
	if got := lines[len(lines)-1]; !strings.Contains(got, "refreshing…") {
		t.Errorf("footer while refreshing = %q, want it to say refreshing…", got)
	}
	// The old page is still on screen while the new one is fetched.
	body := pageOf(t, sample(), 80, true)[banner.Height:]
	if lines[banner.Height] != body[0] {
		t.Errorf("first body line = %q, want the previous result's %q", lines[banner.Height], body[0])
	}

	msg, ok := cmd().(resultMsg)
	if !ok {
		t.Fatalf("the fetch produced %T, want a resultMsg", cmd())
	}
	if len(msg.res.Windows) != 1 || msg.res.Windows[0].RemainingPercent != 50 {
		t.Fatalf("the fetch returned %+v, want the fake's one window", msg.res.Windows)
	}
	if fake.Fetches() != 1 {
		t.Errorf("the provider was fetched %d times, want 1", fake.Fetches())
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
	if !strings.Contains(footer, "fetching…") || !strings.Contains(footer, "q quit") {
		t.Errorf("footer during the first fetch = %q, want fetching… and q quit", footer)
	}
	if strings.Contains(footer, "scroll") || strings.Contains(footer, "refresh") {
		t.Errorf("footer during the first fetch = %q, want no keys that do nothing yet", footer)
	}
}

func TestInit_StartsAFetch(t *testing.T) {
	fake := providertest.Succeeding("claude", []provider.Window{
		{Provider: "claude", Name: "5h", RemainingPercent: 10},
	})
	m := newModel(t, true, fake)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init returned no command, want a fetch")
	}
	if _, ok := cmd().(resultMsg); !ok {
		t.Fatalf("Init's command produced %T, want a resultMsg", cmd())
	}
	if fake.Fetches() != 1 {
		t.Errorf("the provider was fetched %d times, want 1", fake.Fetches())
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
