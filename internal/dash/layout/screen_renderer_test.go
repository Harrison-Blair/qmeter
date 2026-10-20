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
