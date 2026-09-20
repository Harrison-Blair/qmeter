package layout

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/resets"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

func TestRenderTimelineProviderTheme(t *testing.T) {
	r := lipgloss.DefaultRenderer()
	profile, dark := r.ColorProfile(), r.HasDarkBackground()
	t.Cleanup(func() {
		r.SetColorProfile(profile)
		r.SetHasDarkBackground(dark)
	})
	r.SetColorProfile(termenv.TrueColor)
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	th := display.Theme{Codex: lipgloss.AdaptiveColor{Light: "#123456", Dark: "#abcdef"}}
	result := usage.Result{Windows: []provider.Window{{Provider: "codex", Name: "5h", RemainingPercent: 80, ResetsAt: now.Add(time.Hour)}}}
	for _, dark := range []bool{false, true} {
		r.SetHasDarkBackground(dark)
		for _, width := range []int{69, 70} {
			page := RenderTimeline(result, width, Options{Now: now, Theme: th})
			body := strings.Join(page[1:], "\n")
			identity := "codex"
			if width >= resets.MinWidth {
				identity = "●"
				marker := r.NewStyle().Foreground(display.Band(80, false)).Render("●")
				if !strings.Contains(body, marker) {
					t.Errorf("width %d: reset marker lost health color: %q", width, body)
				}
			}
			want := r.NewStyle().Foreground(th.Accent("codex")).Render(identity)
			if !strings.Contains(body, want) {
				t.Errorf("width %d, dark %t: missing configured identity %q in %q", width, dark, want, body)
			}
		}
	}
}

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
		if width >= 70 && !strings.Contains(joined, resets.Row(result.Windows[0], now, width, r, display.Default())) {
			t.Fatalf("shared row missing: %q", joined)
		}
		if width < 70 && !strings.Contains(joined, "PROVIDER") {
			t.Fatalf("narrow table missing: %q", joined)
		}
	}
}
