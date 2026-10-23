package layout_test

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/dash/layout"
	"github.com/Harrison-Blair/qmeter/internal/dash/theme"
	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

func screenOptions() layout.ScreenOptions {
	return layout.ScreenOptions{
		Now:       now,
		Theme:     theme.Default(),
		FetchedAt: time.Date(2026, 9, 16, 21, 23, 41, 0, time.Local),
	}
}

func boardScreenFixture() usage.Result {
	in := func(d time.Duration) time.Time { return now.Add(d) }
	return usage.Result{
		Windows: []provider.Window{
			{Provider: "claude", Name: "5h", Plan: "max", RemainingPercent: 81, ResetsAt: in(16 * time.Minute)},
			{Provider: "claude", Name: "weekly", Plan: "max", RemainingPercent: 91, ResetsAt: in(6*24*time.Hour + 8*time.Hour)},
			{Provider: "claude", Name: "fable weekly", Plan: "max", RemainingPercent: 96, ResetsAt: in(6*24*time.Hour + 8*time.Hour)},
			{Provider: "codex", Name: "weekly", Plan: "prolite", RemainingPercent: 28, ResetsAt: in(4*24*time.Hour + 19*time.Hour)},
			{Provider: "opencode-go", Name: "5h", Plan: "go", RemainingPercent: 100, ResetsAt: in(4*time.Hour + 59*time.Minute)},
			{Provider: "opencode-go", Name: "weekly", Plan: "go", RemainingPercent: 95, ResetsAt: in(5*24*time.Hour + 22*time.Hour)},
			{Provider: "opencode-go", Name: "monthly", Plan: "go", RemainingPercent: 54, ResetsAt: in(9*24*time.Hour + time.Hour)},
			{Provider: "cursor", Name: "total", Plan: "free", RemainingPercent: 49.5, ResetsAt: in(3*24*time.Hour + 12*time.Hour)},
			{Provider: "cursor", Name: "auto", Plan: "free", RemainingPercent: 0, ResetsAt: in(3*24*time.Hour + 12*time.Hour), RateLimited: true},
		},
		Balances: []provider.Balance{
			{Provider: "codex", Name: "credits", Unit: "credits", Remaining: screenNumber(0)},
			{Provider: "cursor", Name: "included", Unit: "unconfirmed", Remaining: screenNumber(0), Limit: screenNumber(0)},
		},
	}
}

func screenNumber(n float64) *float64 { return &n }

func TestScreenGoldenFrames(t *testing.T) {
	cases := []struct {
		name          string
		width, height int
	}{
		{"140x45", 140, 45},
		{"100x40", 100, 40},
		{"93x39", 93, 39},
		{"93x30", 93, 30},
		{"93x26", 93, 26},
		{"80x24", 80, 24},
		{"79x23", 79, 23},
		{"79x45", 79, 45},
		{"140x23", 140, 23},
		{"60x40", 60, 40},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := layout.Screen(boardScreenFixture(), tc.width, tc.height, screenOptions())
			want := goldenLines(t, "screen-"+tc.name+".txt")
			if len(got) != len(want) {
				t.Fatalf("got %d lines, want %d", len(got), len(want))
			}
			for i := range got {
				gotLine := strings.TrimRight(ansi.Strip(got[i]), " ")
				if gotLine != want[i] {
					t.Errorf("line %d:\ngot  %q\nwant %q", i+1, gotLine, want[i])
				}
			}
		})
	}
}

func TestScreenBelowFloorShowsOnlyTooSmallMessage(t *testing.T) {
	options := screenOptions()
	loading := options
	loading.FetchedAt = time.Time{}
	loading.Loading = true
	for _, content := range []struct {
		name    string
		result  usage.Result
		options layout.ScreenOptions
	}{
		{name: "result", result: boardScreenFixture(), options: options},
		{name: "loading", options: loading},
		{name: "empty", options: options},
	} {
		for _, size := range []struct{ width, height int }{{79, 23}, {79, 45}, {140, 23}, {60, 40}} {
			t.Run(fmt.Sprintf("%s/%dx%d", content.name, size.width, size.height), func(t *testing.T) {
				frame := layout.Screen(content.result, size.width, size.height, content.options)
				if len(frame) != size.height {
					t.Fatalf("got %d lines, want %d", len(frame), size.height)
				}
				message := fmt.Sprintf("terminal too small: %dx%d, need 80x24", size.width, size.height)
				for i, line := range frame {
					plain := strings.TrimSpace(ansi.Strip(line))
					if i == size.height/2 {
						if plain != message {
							t.Errorf("center line = %q, want %q", plain, message)
						}
					} else if plain != "" {
						t.Errorf("line %d = %q, want blank", i+1, plain)
					}
				}
			})
		}
	}
}

func TestScreenFrameFitsEveryDimension(t *testing.T) {
	result := usage.Result{
		Windows: []provider.Window{
			{Provider: "claude", Name: "界 weekly", RemainingPercent: 67, ResetsAt: now.Add(time.Hour)},
			{Provider: "codex", Name: "5h", RemainingPercent: 0},
		},
		Balances: []provider.Balance{{Provider: "codex", Name: "credits", Unit: "credits", Remaining: screenNumber(3.5)}},
		Errors:   []usage.ProviderError{{Provider: "cursor", Message: "offline"}},
	}
	for width := 0; width <= 160; width++ {
		for height := 0; height <= 60; height++ {
			got := layout.Screen(result, width, height, screenOptions())
			if len(got) != height {
				t.Fatalf("%dx%d: got %d lines, want %d", width, height, len(got), height)
			}
			for i, line := range got {
				plain := ansi.Strip(line)
				if cells := runewidth.StringWidth(plain); cells != width {
					t.Fatalf("%dx%d line %d: got %d cells (%q)", width, height, i+1, cells, plain)
				}
				if strings.ContainsAny(plain, "\r\n") {
					t.Fatalf("%dx%d line %d contains a line break: %q", width, height, i+1, plain)
				}
			}
		}
	}
}

func TestScreenDropsPartsInMeasuredOrder(t *testing.T) {
	var windows []provider.Window
	for _, providerID := range []string{"claude", "codex"} {
		for i := 0; i < 8; i++ {
			windows = append(windows, provider.Window{
				Provider: providerID, Name: fmt.Sprintf("window-%d", i+1),
				Plan: "pro", RemainingPercent: float64(80 - i), ResetsAt: now.Add(time.Duration(i+1) * time.Hour),
			})
		}
	}
	result := usage.Result{
		Windows:  windows,
		Balances: []provider.Balance{{Provider: "codex", Name: "credits", Unit: "credits", Remaining: screenNumber(4)}},
	}
	for height := 46; height >= 23; height-- {
		text := strings.Join(layout.Screen(result, 100, height, screenOptions()), "\n")
		wantResets := height == 46
		wantBalances := height >= 31
		wantCards := height >= 27
		wantCompact := height >= 24 && height <= 26
		wantTooSmall := height < 24
		if got := strings.Contains(text, "│ resets"); got != wantResets {
			t.Errorf("height %d: resets panel = %v, want %v", height, got, wantResets)
		}
		if got := strings.Contains(text, "│ balances"); got != wantBalances {
			t.Errorf("height %d: balances panel = %v, want %v", height, got, wantBalances)
		}
		if got := strings.Contains(text, "terminal too small:"); got != wantTooSmall {
			t.Errorf("height %d: too-small message = %v, want %v", height, got, wantTooSmall)
		}
		if got := strings.Contains(text, "╭─"); got != wantCards {
			t.Errorf("height %d: cards = %v, want %v", height, got, wantCards)
		}
		if got := strings.Contains(text, "codex window-1"); got != wantCompact {
			t.Errorf("height %d: compact rows = %v, want %v", height, got, wantCompact)
		}
	}
}

func TestScreenCompactCodexWithEightWindows(t *testing.T) {
	windows := make([]provider.Window, 8)
	for i := range windows {
		windows[i] = provider.Window{
			Provider: "codex", Name: fmt.Sprintf("window-%d", i+1),
			RemainingPercent: float64(90 - i), ResetsAt: now.Add(time.Duration(i+1) * time.Hour),
		}
	}
	frame := layout.Screen(usage.Result{Windows: windows}, 80, 24, screenOptions())
	text := ansi.Strip(strings.Join(frame, "\n"))
	for i := 1; i <= 8; i++ {
		if !strings.Contains(text, fmt.Sprintf("codex window-%d", i)) {
			t.Errorf("compact frame omits window-%d:\n%s", i, text)
		}
	}
	if strings.Contains(text, "╭") {
		t.Fatalf("compact frame still has provider cards:\n%s", text)
	}
}

func TestScreenTruncatesLongUnicodeNames(t *testing.T) {
	result := usage.Result{Windows: []provider.Window{{Provider: "codex", Name: strings.Repeat("界", 60), RemainingPercent: 75}}}
	frame := layout.Screen(result, 100, 40, screenOptions())
	text := ansi.Strip(strings.Join(frame, "\n"))
	if !strings.Contains(text, "…") {
		t.Fatalf("long Unicode window name was not truncated:\n%s", text)
	}
	for i, line := range frame {
		if cells := runewidth.StringWidth(ansi.Strip(line)); cells != 100 {
			t.Fatalf("line %d has %d cells, want 100", i+1, cells)
		}
	}
}

func TestScreenSpecialProvidersAndUnicode(t *testing.T) {
	result := usage.Result{
		Windows:    []provider.Window{{Provider: "claude", Name: "界界 long window name", RemainingPercent: 52, ResetsAt: time.Time{}}},
		Balances:   []provider.Balance{{Provider: "gemini", Name: "credits", Unit: "credits", Remaining: screenNumber(2)}},
		Errors:     []usage.ProviderError{{Provider: "cursor", Message: "token expired"}},
		Undetected: []usage.ProviderError{{Provider: "opencode-go", Message: "not logged in"}},
	}
	frame := layout.Screen(result, 100, 40, screenOptions())
	text := ansi.Strip(strings.Join(frame, "\n"))
	for _, want := range []string{"界界 long window name", "error: token expired", "not detected: not logged in", "gemini", "credits", "╭─ • gemini"} {
		if !strings.Contains(text, want) {
			t.Errorf("screen omits %q:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "resets -") && !strings.Contains(text, "│ -") {
		t.Errorf("screen does not show the unknown reset time:\n%s", text)
	}
}

func TestScreenLoadingAndNoProviders(t *testing.T) {
	options := screenOptions()
	options.FetchedAt = time.Time{}
	options.Loading = true
	options.SpinnerFrame = 2
	frame := layout.Screen(usage.Result{}, 80, 24, options)
	text := ansi.Strip(strings.Join(frame, "\n"))
	for _, want := range []string{"qmeter", "fetching…", "⠹ refreshing · q quit"} {
		if !strings.Contains(text, want) {
			t.Errorf("loading screen omits %q:\n%s", want, text)
		}
	}
	options.FetchedAt = now
	options.Loading = false
	frame = layout.Screen(usage.Result{}, 80, 24, options)
	if !strings.Contains(ansi.Strip(strings.Join(frame, "\n")), "no providers detected") {
		t.Fatalf("empty result has no body message: %q", frame)
	}
}

func TestScreenZeroDimensions(t *testing.T) {
	for _, tc := range []struct{ width, height int }{{0, 0}, {0, 4}, {4, 0}} {
		frame := layout.Screen(boardScreenFixture(), tc.width, tc.height, screenOptions())
		if len(frame) != tc.height {
			t.Fatalf("%dx%d: got %d lines", tc.width, tc.height, len(frame))
		}
		for _, line := range frame {
			if runewidth.StringWidth(ansi.Strip(line)) != tc.width {
				t.Fatalf("%dx%d: invalid line %q", tc.width, tc.height, line)
			}
		}
	}
}

const cachedLine = "◌ cached · fetched 14:20:05 · timed out after 10s"

// cachedScreenFixture is the board fixture with codex failing on a timeout,
// and the options that hold codex's last good response.
func cachedScreenFixture() (usage.Result, layout.ScreenOptions) {
	board := boardScreenFixture()
	var result usage.Result
	entry := layout.CachedProvider{FetchedAt: time.Date(2026, 9, 16, 14, 20, 5, 0, time.Local)}
	for _, w := range board.Windows {
		if w.Provider == "codex" {
			entry.Windows = append(entry.Windows, w)
		} else {
			result.Windows = append(result.Windows, w)
		}
	}
	for _, b := range board.Balances {
		if b.Provider == "codex" {
			entry.Balances = append(entry.Balances, b)
		} else {
			result.Balances = append(result.Balances, b)
		}
	}
	result.Errors = []usage.ProviderError{{Provider: "codex", Message: "timed out after 10s"}}
	options := screenOptions()
	options.Cached = map[string]layout.CachedProvider{"codex": entry}
	return result, options
}

func plainFrame(frame []string) string {
	lines := make([]string, len(frame))
	for i, line := range frame {
		lines[i] = strings.TrimRight(ansi.Strip(line), " ")
	}
	return strings.Join(lines, "\n")
}

func TestScreenCachedCardKeepsWindowsWithTheCachedLine(t *testing.T) {
	result, options := cachedScreenFixture()
	text := plainFrame(layout.Screen(result, 140, 45, options))

	for _, want := range []string{
		"╭─ ● codex ───",
		"─ prolite ─╮",
		"│ ▸ weekly                                  28.0% · resets in 4d19h │",
		"│ " + cachedLine + " ",
		"│ resets",
		"│ balances",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("frame omits %q:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"error: timed out", "codex        weekly", "codex credits"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("frame shows %q:\n%s", unwanted, text)
		}
	}
	if got, want := strings.Split(text, "\n"), goldenLines(t, "screen-cached-140x45.txt"); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("frame differs from the cached golden:\n%s", text)
	}
}

func TestScreenFailureWithoutCacheShowsOnlyTheErrorLine(t *testing.T) {
	result, options := cachedScreenFixture()
	options.Cached = nil
	text := plainFrame(layout.Screen(result, 140, 45, options))

	if !strings.Contains(text, "│ error: timed out after 10s ") {
		t.Errorf("frame omits the error line:\n%s", text)
	}
	for _, unwanted := range []string{"◌ cached", "28.0%"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("frame shows %q with no cache:\n%s", unwanted, text)
		}
	}
}

// compactCachedFixture is cachedScreenFixture with five more claude windows,
// which make the cards too tall for 80x24.
func compactCachedFixture() (usage.Result, layout.ScreenOptions) {
	result, options := cachedScreenFixture()
	var windows []provider.Window
	for _, w := range result.Windows {
		windows = append(windows, w)
		if w.Name == "fable weekly" {
			for i := 1; i <= 5; i++ {
				windows = append(windows, provider.Window{Provider: "claude", Name: fmt.Sprintf("extra-%d", i), Plan: "max", RemainingPercent: 50})
			}
		}
	}
	result.Windows = windows
	return result, options
}

func TestScreenCompactCachedRowsKeepTheProviderSlot(t *testing.T) {
	result, options := compactCachedFixture()
	var rows []string
	for _, line := range strings.Split(plainFrame(layout.Screen(result, 80, 24, options)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			rows = append(rows, line)
		}
	}
	wantPrefixes := []string{
		"qmeter",
		"claude 5h", "claude weekly", "claude fable weekly",
		"claude extra-1", "claude extra-2", "claude extra-3", "claude extra-4", "claude extra-5",
		"codex weekly",
		"opencode-go 5h", "opencode-go weekly", "opencode-go monthly",
		"cursor total", "cursor auto",
		"codex " + cachedLine,
		"r refresh",
	}
	if len(rows) != len(wantPrefixes) {
		t.Fatalf("got %d rows, want %d:\n%s", len(rows), len(wantPrefixes), strings.Join(rows, "\n"))
	}
	for i, want := range wantPrefixes {
		if !strings.HasPrefix(rows[i], want) {
			t.Errorf("row %d = %q, want prefix %q", i+1, rows[i], want)
		}
	}
	if !strings.Contains(rows[9], "28.0%") {
		t.Errorf("cached codex row = %q, want its kept percentage", rows[9])
	}
}

func TestScreenCompactNeedCountsCachedRows(t *testing.T) {
	entry := layout.CachedProvider{FetchedAt: time.Date(2026, 9, 16, 14, 20, 5, 0, time.Local)}
	for i := 0; i < 30; i++ {
		entry.Windows = append(entry.Windows, provider.Window{Provider: "codex", Name: fmt.Sprintf("window-%d", i+1), RemainingPercent: 50})
	}
	reason := strings.Repeat("x", 90)
	result := usage.Result{Errors: []usage.ProviderError{{Provider: "codex", Message: reason}}}
	options := screenOptions()
	options.Cached = map[string]layout.CachedProvider{"codex": entry}
	line := "codex ◌ cached · fetched 14:20:05 · " + reason
	needWidth, needHeight := runewidth.StringWidth(line), 30+1+2

	small := plainFrame(layout.Screen(result, 79, 23, options))
	if want := fmt.Sprintf("need %dx%d", needWidth, needHeight); !strings.Contains(small, want) {
		t.Fatalf("too-small frame omits %q:\n%s", want, small)
	}
	fits := plainFrame(layout.Screen(result, needWidth, needHeight, options))
	for _, want := range []string{"codex window-1 ", "codex window-30 ", line} {
		if !strings.Contains(fits, want) {
			t.Errorf("frame at the reported need omits %q:\n%s", want, fits)
		}
	}
	if short := plainFrame(layout.Screen(result, needWidth, needHeight-1, options)); !strings.Contains(short, "terminal too small:") {
		t.Errorf("frame one row below the need is not too small:\n%s", short)
	}
}

func TestScreenCachedLineAndErrorLineDifferInPlainText(t *testing.T) {
	result, options := cachedScreenFixture()
	result.Errors = append(result.Errors, usage.ProviderError{Provider: "gemini", Message: "offline"})
	renderer := lipgloss.NewRenderer(io.Discard)
	renderer.SetColorProfile(termenv.Ascii)
	options.Renderer = renderer
	frame := layout.Screen(result, 140, 45, options)
	text := plainFrame(frame)
	t.Logf("Ascii frame:\n%s", text)

	if strings.Contains(strings.Join(frame, "\n"), "\x1b[") {
		t.Fatalf("Ascii frame carries escape codes")
	}
	for _, want := range []string{"│ " + cachedLine + " ", "│ error: offline ", "│ ━━━━━━━━━━━━━━━━━━─"} {
		if !strings.Contains(text, want) {
			t.Errorf("Ascii frame omits %q:\n%s", want, text)
		}
	}
}

func TestScreenCachedBarsRenderFaint(t *testing.T) {
	renderer := lipgloss.NewRenderer(io.Discard)
	renderer.SetColorProfile(termenv.ANSI)
	faint := renderer.NewStyle().Foreground(display.Band(28, false)).Faint(true).Render("28.0%")
	bold := renderer.NewStyle().Foreground(display.Band(28, false)).Bold(true).Render("28.0%")
	for _, tc := range []struct {
		name          string
		fixture       func() (usage.Result, layout.ScreenOptions)
		width, height int
	}{
		{"cards", cachedScreenFixture, 140, 45},
		{"compact", compactCachedFixture, 80, 24},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, options := tc.fixture()
			options.Renderer = renderer
			frame := strings.Join(layout.Screen(result, tc.width, tc.height, options), "\n")
			if !strings.Contains(frame, faint) {
				t.Errorf("cached percentage is not faint")
			}
			if strings.Contains(frame, bold) {
				t.Errorf("cached percentage is bold")
			}
		})
	}
}
