package gauge_test

import (
	"strings"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/dash/gauge"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestTopRailAndScaleRetainTheirBytesAtEveryWidth(t *testing.T) {
	defer lipgloss.SetColorProfile(termenv.Ascii)
	for _, profile := range []termenv.Profile{termenv.Ascii, termenv.ANSI256} {
		lipgloss.SetColorProfile(profile)
		for width := gauge.MinWidth; width <= 60; width++ {
			for _, rl := range []bool{false, true} {
				for _, pace := range []float64{gauge.NoPace, 0.68, 1.5} {
					frame := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
					if rl {
						frame = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Faint(true)
					}
					cells := []rune("╭" + strings.Repeat("─", width-2) + "╮")
					for i := 0; i <= 4; i++ {
						cells[1+i*(width-3)/4] = '┬'
					}
					top := frame.Render(string(cells))
					if pace >= 0 {
						clamped := pace
						if clamped > 1 {
							clamped = 1
						}
						at := 1 + int(clamped*float64(width-3)+0.5)
						marker := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
						top = frame.Render(string(cells[:at])) + marker.Render("▼") + frame.Render(string(cells[at+1:]))
					}
					scale := []rune(strings.Repeat(" ", width))
					scale[1] = '0'
					copy(scale[(width-3)/2:], []rune("50"))
					copy(scale[width-4:], []rune("100"))
					b := mustRenderPace(t, 68, width, rl, pace)
					if b.Bezel != top {
						t.Fatalf("width %d profile %v RL %v pace %v: top rail bytes changed: got %q want %q", width, profile, rl, pace, b.Bezel, top)
					}
					if b.Scale != frame.Render(string(scale)) {
						t.Fatalf("width %d profile %v RL %v pace %v: scale bytes changed: got %q want %q", width, profile, rl, pace, b.Scale, frame.Render(string(scale)))
					}
				}
			}
		}
	}
}

func TestNoForecastGlyphInAnyClosedGaugeRow(t *testing.T) {
	for width := gauge.MinWidth; width <= 60; width++ {
		for thickness := 1; thickness <= 9; thickness++ {
			for _, rl := range []bool{false, true} {
				b, err := gauge.Render(68, width, rl, 0.68, thickness)
				if err != nil {
					t.Fatal(err)
				}
				rows := append(append([]string{b.Bezel}, b.Tracks...), b.Bottom, b.Scale)
				for _, row := range rows {
					if strings.ContainsAny(row, "◇✕") {
						t.Fatalf("forecast glyph in gauge row %q", row)
					}
				}
			}
		}
	}
}
