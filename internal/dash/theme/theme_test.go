package theme_test

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/Harrison-Blair/qmeter/internal/dash/theme"
)

func TestDefaultProviderColors(t *testing.T) {
	tests := []struct {
		provider string
		want     lipgloss.AdaptiveColor
	}{
		{"claude", lipgloss.AdaptiveColor{Light: "#A64526", Dark: "#D97757"}},
		{"codex", lipgloss.AdaptiveColor{Light: "#0B6F57", Dark: "#10A37F"}},
		{"opencode-go", lipgloss.AdaptiveColor{Light: "#656363", Dark: "#B7B1B1"}},
		{"cursor", lipgloss.AdaptiveColor{Light: "#26251E", Dark: "#EDECEC"}},
	}
	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			if got := (theme.Theme{}).Accent(tc.provider); got != tc.want {
				t.Errorf("zero Theme accent = %#v, want %#v", got, tc.want)
			}
			if got := theme.Default().Accent(tc.provider); got != tc.want {
				t.Errorf("Default accent = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestPartialColorFallsBackLeafByLeaf(t *testing.T) {
	th := theme.Theme{Claude: lipgloss.AdaptiveColor{Light: "#112233"}}
	want := lipgloss.AdaptiveColor{Light: "#112233", Dark: "#D97757"}
	if got := th.Accent("claude"); got != want {
		t.Fatalf("Accent = %#v, want %#v", got, want)
	}
}

func TestUnknownProviderUsesAdaptiveNeutral(t *testing.T) {
	want := lipgloss.AdaptiveColor{Light: "#656363", Dark: "#B7B1B1"}
	if got := theme.Default().Accent("future-provider"); got != want {
		t.Fatalf("unknown accent = %#v, want %#v", got, want)
	}
}

func TestAccentAdaptsToTheTerminalBackground(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	accent := theme.Default().Accent("claude")
	for _, tc := range []struct {
		name string
		dark bool
		want lipgloss.Color
	}{
		{"light background", false, lipgloss.Color("#A64526")},
		{"dark background", true, lipgloss.Color("#D97757")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lipgloss.SetHasDarkBackground(tc.dark)
			got := lipgloss.NewStyle().Foreground(accent).Render("x")
			want := lipgloss.NewStyle().Foreground(tc.want).Render("x")
			if got != want {
				t.Fatalf("rendered accent = %q, want %q", got, want)
			}
		})
	}
}
