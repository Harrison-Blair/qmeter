// Package theme exposes the dashboard's configurable provider palette.
package theme

import "github.com/Harrison-Blair/qmeter/internal/display"

// Theme is the configurable provider palette, shared with CLI defaults.
type Theme = display.Theme

// Default returns the built-in provider palette.
func Default() Theme { return display.Default() }
