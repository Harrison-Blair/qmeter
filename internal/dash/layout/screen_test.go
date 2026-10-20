package layout_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/dash/layout"
	"github.com/Harrison-Blair/qmeter/internal/dash/theme"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
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
		{"60x40", 60, 40},
		{"79x23", 79, 23},
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
	result := usage.Result{
		Windows: []provider.Window{
			{Provider: "claude", Name: "5h", Plan: "max", RemainingPercent: 80, ResetsAt: now.Add(time.Hour)},
			{Provider: "codex", Name: "weekly", Plan: "pro", RemainingPercent: 60, ResetsAt: now.Add(2 * time.Hour)},
		},
		Balances: []provider.Balance{{Provider: "codex", Name: "credits", Unit: "credits", Remaining: screenNumber(4)}},
	}
	for _, tc := range []struct {
		height               int
		resets, money, cards bool
		tooSmall             bool
	}{
		{height: 11, resets: true, money: true, cards: true},
		{height: 10, money: true, cards: true},
		{height: 9, cards: true},
		{height: 8, cards: true},
		{height: 7, cards: true},
		{height: 6, cards: true},
		{height: 5, tooSmall: true},
	} {
		frame := layout.Screen(result, 100, tc.height, screenOptions())
		text := strings.Join(frame, "\n")
		if got := strings.Contains(text, "│ resets"); got != tc.resets {
			t.Errorf("height %d: resets panel = %v, want %v", tc.height, got, tc.resets)
		}
		if got := strings.Contains(text, "│ balances"); got != tc.money {
			t.Errorf("height %d: balances panel = %v, want %v", tc.height, got, tc.money)
		}
		if got := strings.Contains(text, "terminal too small:"); got != tc.tooSmall {
			t.Errorf("height %d: too-small message = %v, want %v", tc.height, got, tc.tooSmall)
		}
		if got := strings.Contains(text, "╭─"); got != tc.cards {
			t.Errorf("height %d: cards = %v, want %v", tc.height, got, tc.cards)
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
