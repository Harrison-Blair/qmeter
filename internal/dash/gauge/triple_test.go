package gauge_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/dash/gauge"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

func sixRows(t *testing.T, b gauge.Block) []string {
	t.Helper()
	if len(b.Tracks) != 3 {
		t.Fatalf("default has %d track rows, want 3", len(b.Tracks))
	}
	u := b.Tracks
	return []string{b.Bezel, u[0], u[1], b.Tracks[len(b.Tracks)-1], b.Bottom, b.Scale}
}

func TestTripleShape(t *testing.T) {
	for width := gauge.MinWidth; width <= 60; width++ {
		for _, row := range sixRows(t, mustRender(t, 43.5, width, false)) {
			if runewidth.StringWidth(row) != width {
				t.Fatalf("width %d: %q", width, row)
			}
		}
	}
}

func TestTripleUpperExact(t *testing.T) {
	b := mustRender(t, 68, 25, false)
	want := "│" + strings.Repeat("▰", 16) + strings.Repeat("▱", 7) + "│"
	for _, row := range b.Tracks[:len(b.Tracks)-1] {
		if row != want {
			t.Fatalf("got %q want %q", row, want)
		}
	}
}

func TestTripleAboveNeedleFilled(t *testing.T) {
	for _, pct := range []float64{0, 1, 50, 99, 100} {
		b := mustRender(t, pct, 25, false)
		at := strings.IndexRune(b.Bottom, '▲')
		if at < 0 {
			t.Fatalf("pct %v: bottom row missing needle: %q", pct, b.Tracks[len(b.Tracks)-1])
		}
		if len(b.Tracks) != 3 {
			t.Fatalf("pct %v: got %d track rows, want 3", pct, len(b.Tracks))
		}
		column := len([]rune(b.Bottom[:at]))
		for _, row := range b.Tracks[:len(b.Tracks)-1] {
			if column >= len([]rune(row)) {
				t.Fatalf("pct %v: upper row too short for needle column %d: %q", pct, column, row)
			}
			want := '▰'
			if pct == 0 {
				want = '▱'
			}
			if []rune(row)[column] != want {
				t.Fatalf("pct %v above needle: %q", pct, row)
			}
		}
	}
}

func TestTripleTrackStyles(t *testing.T) {
	defer lipgloss.SetColorProfile(termenv.Ascii)
	for _, profile := range []termenv.Profile{termenv.Ascii, termenv.ANSI256} {
		lipgloss.SetColorProfile(profile)
		for _, rl := range []bool{false, true} {
			for _, pct := range []float64{0, 1, 50, 68, 99, 100} {
				b := mustRender(t, pct, 25, rl)
				n := int(pct/100*22+0.5) + 1
				if pct == 0 {
					n = 0
				}
				frame := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
				if rl {
					frame = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Faint(true)
				}
				fill := lipgloss.NewStyle().Foreground(gauge.Band(pct, rl))
				spent := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Faint(true)
				want := frame.Render("│")
				if n > 0 {
					want += fill.Render(strings.Repeat("▰", n))
				}
				if n < 23 {
					want += spent.Render(strings.Repeat("▱", 23-n))
				}
				want += frame.Render("│")
				if b.Tracks[len(b.Tracks)-1] != want {
					t.Fatalf("track styling differs: %q want %q", b.Tracks[len(b.Tracks)-1], want)
				}
			}
		}
	}
	lipgloss.SetColorProfile(termenv.Ascii)
}

func TestTripleNoForecastGlyphs(t *testing.T) {
	for _, rl := range []bool{false, true} {
		for _, pct := range []float64{0, 1, 50, 99, 100} {
			b, err := gauge.Render(pct, 25, rl, 0.5, 3)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range sixRows(t, b) {
				if strings.ContainsAny(row, "◇✕") {
					t.Fatalf("forecast glyph in %q", row)
				}
			}
		}
	}
}

func TestTripleUpperStyles(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	for _, rl := range []bool{false, true} {
		for _, pct := range []float64{12, 30, 60, 90} {
			b := mustRender(t, pct, 25, rl)
			n := int(pct/100*22+0.5) + 1
			frame := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
			spent := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Faint(true)
			if rl {
				frame = spent
			}
			want := frame.Render("│") + lipgloss.NewStyle().Foreground(gauge.Band(pct, rl)).Render(strings.Repeat("▰", n)) + spent.Render(strings.Repeat("▱", 23-n)) + frame.Render("│")
			for _, row := range b.Tracks[:len(b.Tracks)-1] {
				if row != want {
					t.Fatalf("upper styling: %q want %q", row, want)
				}
			}
			if !strings.HasPrefix(b.Bezel, frame.Render("╭┬────┬─────┬────┬─────┬╮")) {
				t.Fatal("frame styling changed")
			}
		}
	}
}

func TestTriplePlainAllRows(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	b := mustRender(t, 68, 25, true)
	lipgloss.SetColorProfile(termenv.Ascii)
	bare := mustRender(t, 68, 25, true)
	rows := sixRows(t, gauge.Plain(b))
	for _, row := range rows {
		if strings.ContainsRune(row, '\x1b') {
			t.Fatalf("escape retained: %q", row)
		}
	}
	if !reflect.DeepEqual(gauge.Plain(b), bare) {
		t.Fatalf("plain mismatch")
	}
}
