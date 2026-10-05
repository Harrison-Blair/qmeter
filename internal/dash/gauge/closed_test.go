package gauge_test

import (
	"github.com/Harrison-Blair/qmeter/internal/dash/gauge"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"strings"
	"testing"
)

func TestClosedGaugeExact(t *testing.T) {
	b := mustRenderPace(t, 68, 25, false, 0.68)
	want := []string{"╭┬────┬─────┬───▼┬─────┬╮", "│▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱│", "│▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱│", "│▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱│", "╰┴────┴─────┴───▲┴─────┴╯", " 0         50        100 "}
	for i, row := range sixRows(t, b) {
		if row != want[i] {
			t.Errorf("row %d got %q want %q", i, row, want[i])
		}
	}
}

func TestBottomRailMirrorsTicksAndNeedleColumn(t *testing.T) {
	for width := gauge.MinWidth; width <= 60; width++ {
		for _, pct := range []float64{0, 0.5, 1, 12, 33, 50, 68, 99, 100} {
			b := mustRenderPace(t, pct, width, false, pct/100)
			at := 1 + int(pct/100*float64(width-3)+0.5)
			cells := []rune(strings.Repeat("─", width))
			cells[0] = '╰'
			cells[width-1] = '╯'
			for i := 0; i < 5; i++ {
				cells[1+i*(width-3)/4] = '┴'
			}
			cells[at] = '▲'
			if b.Bottom != string(cells) {
				t.Errorf("width %d pct %v bottom got %q want %q", width, pct, b.Bottom, string(cells))
			}
			if []rune(b.Bezel)[at] != '▼' {
				t.Errorf("pace and needle differ at %d", at)
			}
		}
	}
}

func TestTrackFillIncludesNeedleAndEmptyHasNone(t *testing.T) {
	for _, pct := range []float64{-5, 0, 0.5, 1, 50, 68, 99, 100, 150} {
		b := mustRender(t, pct, 22, false)
		clamped := pct
		if clamped < 0 {
			clamped = 0
		}
		if clamped > 100 {
			clamped = 100
		}
		at := int(clamped/100*19 + 0.5)
		filled := at + 1
		if clamped == 0 {
			filled = 0
		}
		want := "│" + strings.Repeat("▰", filled) + strings.Repeat("▱", 20-filled) + "│"
		for _, row := range b.Tracks {
			if row != want {
				t.Errorf("pct %v track got %q want %q", pct, row, want)
			}
		}
		if len([]rune(b.Bottom)) != 22 || []rune(b.Bottom)[at+1] != '▲' {
			t.Errorf("pct %v needle missing at %d: %q", pct, at, b.Bottom)
		}
	}
}

func TestBottomRailStyles(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	for _, rl := range []bool{false, true} {
		b := mustRenderPace(t, 68, 25, rl, 0.68)
		frame := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
		if rl {
			frame = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Faint(true)
		}
		needle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
		want := frame.Render("╰┴────┴─────┴───") + needle.Render("▲") + frame.Render("┴─────┴╯")
		if b.Bottom != want {
			t.Errorf("rateLimited %v bottom got %q want %q", rl, b.Bottom, want)
		}
	}
}
