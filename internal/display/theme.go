// Package display defines configurable provider identity
// colours. Gauge colours are deliberately not part of the theme: they report
// window health, not provider identity.
package display

import "github.com/charmbracelet/lipgloss"

// Theme is the provider palette. A missing side of an adaptive colour falls
// back to that provider's built-in default, so the zero value is the default
// theme and a partial configuration can override one leaf at a time.
type Theme struct {
	Claude     lipgloss.AdaptiveColor
	Codex      lipgloss.AdaptiveColor
	OpenCodeGo lipgloss.AdaptiveColor
	Cursor     lipgloss.AdaptiveColor
}

var defaults = Theme{
	Claude:     lipgloss.AdaptiveColor{Light: "#A64526", Dark: "#D97757"},
	Codex:      lipgloss.AdaptiveColor{Light: "#0B6F57", Dark: "#10A37F"},
	OpenCodeGo: lipgloss.AdaptiveColor{Light: "#656363", Dark: "#B7B1B1"},
	Cursor:     lipgloss.AdaptiveColor{Light: "#26251E", Dark: "#EDECEC"},
}

// The fallback for a provider introduced before the dashboard learns its
// identity. It is intentionally adaptive, unlike the old fixed ANSI gray.
var neutral = lipgloss.AdaptiveColor{Light: "#656363", Dark: "#B7B1B1"}

// Default returns the built-in provider palette.
func Default() Theme { return defaults }

// Accent returns providerID's adaptive identity colour. Unknown providers get
// a neutral accent so they remain visible without acquiring another vendor's
// identity.
func (t Theme) Accent(providerID string) lipgloss.AdaptiveColor {
	var configured, fallback lipgloss.AdaptiveColor
	switch providerID {
	case "claude":
		configured, fallback = t.Claude, defaults.Claude
	case "codex":
		configured, fallback = t.Codex, defaults.Codex
	case "opencode-go":
		configured, fallback = t.OpenCodeGo, defaults.OpenCodeGo
	case "cursor":
		configured, fallback = t.Cursor, defaults.Cursor
	default:
		return neutral
	}
	if configured.Light == "" {
		configured.Light = fallback.Light
	}
	if configured.Dark == "" {
		configured.Dark = fallback.Dark
	}
	return configured
}
