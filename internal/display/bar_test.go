package display

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

var barNow = time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)

func plainRenderer() *lipgloss.Renderer {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.Ascii)
	return r
}

func TestBarA5FixedOutputAndCellRounding(t *testing.T) {
	w := provider.Window{Name: "weekly", RemainingPercent: 49.5, ResetsAt: barNow.Add(5 * time.Minute)}
	got := Bar(w, barNow, 40, plainRenderer())
	want := []string{
		"▸ weekly            49.5% · resets in 5m",
		"━━━━━━━━━━━━━━━━━━━━────────────────────",
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Bar = %#v, want %#v", got, want)
	}
	for _, line := range got {
		if width := runewidth.StringWidth(line); width != 40 {
			t.Errorf("line width = %d, want 40: %q", width, line)
		}
	}
}

func TestBarMatchesBoardExamplesAtWidth60(t *testing.T) {
	claude := Bar(provider.Window{Name: "5h", RemainingPercent: 81, ResetsAt: barNow.Add(16 * time.Minute)}, barNow, 60, plainRenderer())
	wantTitle := "▸ 5h" + strings.Repeat(" ", 35) + "81.0% · resets in 16m"
	wantTrack := strings.Repeat("━", 49) + strings.Repeat("─", 11)
	if claude[0] != wantTitle || claude[1] != wantTrack {
		t.Fatalf("claude board bar = %#v, want %#v", claude, []string{wantTitle, wantTrack})
	}
	cursor := Bar(provider.Window{Name: "auto", RemainingPercent: 0, ResetsAt: barNow.Add(3*24*time.Hour + 12*time.Hour), RateLimited: true}, barNow, 60, plainRenderer())
	if !strings.Contains(cursor[0], "0.0% · resets in 3d12h · rate limited") || cursor[1] != strings.Repeat("─", 60) {
		t.Fatalf("cursor board bar = %#v", cursor)
	}
}

func TestBarA5BoundariesLimitedAndUnknownReset(t *testing.T) {
	for _, tc := range []struct {
		name string
		win  provider.Window
		fill string
		text string
	}{
		{"zero", provider.Window{Name: "zero", RemainingPercent: 0}, "", "▸ zero"},
		{"full", provider.Window{Name: "full", RemainingPercent: 100}, "━━━━━━━━━━━━━━━━━━━━", "▸ full"},
		{"limited", provider.Window{Name: "auto", RemainingPercent: 0, RateLimited: true}, "", "rate limited"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Bar(tc.win, barNow, 40, plainRenderer())
			if len(got) != 2 || runewidth.StringWidth(got[0]) != 40 || runewidth.StringWidth(got[1]) != 40 {
				t.Fatalf("Bar lines = %#v", got)
			}
			if !strings.Contains(got[0], tc.text) || !strings.Contains(got[0], " · resets -") {
				t.Errorf("title = %q", got[0])
			}
			if tc.fill != "" && !strings.HasPrefix(got[1], tc.fill) {
				t.Errorf("bar = %q, want prefix %q", got[1], tc.fill)
			}
		})
	}
}

func TestBarRoundsHalfCellAndSupportsStandardWidths(t *testing.T) {
	got := Bar(provider.Window{Name: "edge", RemainingPercent: 2.5}, barNow, 20, plainRenderer())
	if got[1] != "━───────────────────" {
		t.Fatalf("half-cell fill = %q, want one filled cell", got[1])
	}
	for _, width := range []int{20, 40, 80} {
		lines := Bar(provider.Window{Name: "full", RemainingPercent: 100}, barNow, width, plainRenderer())
		for _, line := range lines {
			if gotWidth := runewidth.StringWidth(line); gotWidth != width {
				t.Errorf("width %d line has %d cells: %q", width, gotWidth, line)
			}
		}
	}
}

func TestBarA5ClipsWideNamesAndUsesBandColour(t *testing.T) {
	w := provider.Window{Name: "界界界界界界界界界界界界界界界界界界界界", RemainingPercent: 100, ResetsAt: barNow.Add(time.Hour)}
	got := Bar(w, barNow, 40, plainRenderer())
	if len(got) != 2 || runewidth.StringWidth(got[0]) != 40 || runewidth.StringWidth(got[1]) != 40 {
		t.Fatalf("wide title or bar has wrong cell width: %#v", got)
	}
	if !strings.Contains(got[0], "…") {
		t.Fatalf("wide name was not truncated with an ellipsis: %q", got[0])
	}

	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.ANSI)
	colored := Bar(provider.Window{Name: "5h", RemainingPercent: 100}, barNow, 20, r)
	if !strings.Contains(colored[1], "\x1b[") {
		t.Fatalf("bar has no colour styling: %#v", colored)
	}
	bandPercent := r.NewStyle().Foreground(lipgloss.Color("10")).Bold(true).Render("100.0%")
	if !strings.Contains(colored[0], bandPercent) {
		t.Fatalf("percentage does not use the band colour: %q", colored[0])
	}
	track := Bar(provider.Window{Name: "5h", RemainingPercent: 50}, barNow, 20, r)[1]
	if got := ansi.Strip(track); got != "━━━━━━━━━━━━━━━━━━━━" {
		t.Fatalf("coloured track = %q", got)
	}
	limited := Bar(provider.Window{Name: "auto", RemainingPercent: 0, RateLimited: true}, barNow, 40, r)
	limitedPercent := r.NewStyle().Foreground(lipgloss.Color("1")).Bold(true).Render("0.0%")
	limitedNote := r.NewStyle().Foreground(lipgloss.Color("1")).Render(" · rate limited")
	if !strings.Contains(limited[0], limitedPercent) || !strings.Contains(limited[0], limitedNote) {
		t.Fatalf("rate-limit note or percentage has wrong colour: %q", limited[0])
	}
}

func TestCompactBarUsesNameColumnAndMinimumBar(t *testing.T) {
	got := CompactBar(provider.Window{Name: "fable weekly", RemainingPercent: 96, ResetsAt: barNow.Add(6*24*time.Hour + 8*time.Hour)}, barNow, 40, 14, false, plainRenderer())
	want := "fable weekly   ━━━━━━━━━━━━━─ 96.0% 6d8h"
	if got != want {
		t.Fatalf("CompactBar = %q, want %q", got, want)
	}
	short := CompactBar(provider.Window{Name: "x", RemainingPercent: 0}, barNow, 20, 3, false, plainRenderer())
	if !strings.Contains(short, "x   ───────── 0.0% -") {
		t.Fatalf("compact bar below width floor = %q", short)
	}
}

func TestCompactBarDimRendersBarPercentAndCountdownFaint(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.ANSI)
	w := provider.Window{Name: "5h", RemainingPercent: 100, ResetsAt: barNow.Add(16 * time.Minute)}
	plain := CompactBar(w, barNow, 30, 2, false, r)
	dim := CompactBar(w, barNow, 30, 2, true, r)

	if ansi.Strip(dim) != ansi.Strip(plain) {
		t.Fatalf("dim text = %q, want the plain text %q", ansi.Strip(dim), ansi.Strip(plain))
	}
	faintPercent := r.NewStyle().Foreground(lipgloss.Color("10")).Faint(true).Render("100.0%")
	faintCountdown := r.NewStyle().Foreground(Neutral).Faint(true).Render("16m")
	faintBar := r.NewStyle().Foreground(lipgloss.Color("10")).Faint(true).Render("━━━━━━━━━━━━━━━━")
	for _, want := range []string{faintPercent, faintCountdown, faintBar} {
		if !strings.Contains(dim, want) {
			t.Errorf("dim compact bar lacks faint %q: %q", ansi.Strip(want), dim)
		}
		if strings.Contains(plain, want) {
			t.Errorf("plain compact bar has faint %q: %q", ansi.Strip(want), plain)
		}
	}
}
