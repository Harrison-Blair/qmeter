package layout

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/resets"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

func TestRenderTimeline(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	result := usage.Result{Windows: []provider.Window{{Provider: "codex", Name: "5h", RemainingPercent: 80, ResetsAt: now.Add(time.Hour)}}}
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.Ascii)
	for _, width := range []int{20, 40, 69, 70, 100} {
		page := RenderTimeline(result, width, Options{Now: now})
		for _, line := range page {
			if runewidth.StringWidth(line) != width {
				t.Fatalf("width %d: %q", width, line)
			}
		}
		joined := strings.Join(page, "\n")
		if width >= 70 && !strings.Contains(joined, resets.Row(result.Windows[0], now, width, r)) {
			t.Fatalf("shared row missing: %q", joined)
		}
		if width < 70 && !strings.Contains(joined, "PROVIDER") {
			t.Fatalf("narrow table missing: %q", joined)
		}
	}
}
