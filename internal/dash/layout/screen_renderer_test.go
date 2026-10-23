package layout

import (
	"io"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/dash/theme"
	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

func TestScreenReusesProvidedRenderer(t *testing.T) {
	originalFactory := rendererFactory
	t.Cleanup(func() { rendererFactory = originalFactory })
	calls := 0
	rendererFactory = func(io.Writer) *lipgloss.Renderer {
		calls++
		return display.Renderer(io.Discard)
	}

	renderer := lipgloss.NewRenderer(io.Discard)
	renderer.SetColorProfile(termenv.Ascii)
	renderer.SetHasDarkBackground(true)
	options := ScreenOptions{
		Now:       time.Now(),
		Theme:     theme.Default(),
		FetchedAt: time.Now(),
		Renderer:  renderer,
	}
	result := usage.Result{Windows: []provider.Window{{Provider: "claude", Name: "5h", RemainingPercent: 50}}}
	for i := 0; i < 3; i++ {
		Screen(result, 100, 40, options)
	}
	if calls != 0 {
		t.Fatalf("Screen created %d renderers despite a provided renderer", calls)
	}
}

func TestHeaderLineDropsNowThenTheWholeRightSide(t *testing.T) {
	renderer := lipgloss.NewRenderer(io.Discard)
	renderer.SetColorProfile(termenv.Ascii)
	options := ScreenOptions{
		Now:       time.Date(2026, 9, 16, 14, 22, 7, 0, time.Local),
		FetchedAt: time.Date(2026, 9, 16, 21, 23, 41, 0, time.Local),
	}
	left := "qmeter 1 provider · 0 windows · 0 balances"
	for _, tc := range []struct {
		name  string
		width int
		want  string
	}{
		{"full right side", 74, left + " fetched 21:23:41 · now 14:22:07"},
		{"full right side with padding", 80, left + "       fetched 21:23:41 · now 14:22:07"},
		{"fetched alone", 73, left + "               fetched 21:23:41"},
		{"fetched alone at its least width", 59, left + " fetched 21:23:41"},
		{"left side only", 58, left + "                "},
		{"left side cut", 30, "qmeter 1 provider · 0 windows…"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := headerLine(usage.Result{}, 1, tc.width, options, renderer)
			if got != tc.want {
				t.Fatalf("headerLine at %d cells:\ngot  %q\nwant %q", tc.width, got, tc.want)
			}
			if cells := runewidth.StringWidth(got); cells != tc.width {
				t.Fatalf("headerLine has %d cells, want %d", cells, tc.width)
			}
		})
	}
}
