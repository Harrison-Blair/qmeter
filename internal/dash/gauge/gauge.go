// Package gauge draws the dashboard's fuel gauge: three rows of the same
// width — a bezel with five tick marks, a track whose needle sits at the
// remaining percentage, and a 0/50/100 scale under it.
//
//	╭┬────┬─────┬────┬─────┬╮
//	┴▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▲▱▱▱▱▱▱▱┴
//	 0         50        100
//
// The package is pure: it takes a percentage and a width and returns
// strings. Colour comes from lipgloss, so a caller that sets the renderer's
// profile to termenv.Ascii (which is also what NO_COLOR does) gets the bare
// text above.
package gauge

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// MinWidth is the narrowest gauge the design allows, caps included: twenty
// coloured track cells plus the two ┴ caps. The layout gives up a column
// before it gives up track cells, so Render refuses anything narrower
// rather than drawing a gauge nobody can read.
const MinWidth = 22

// Block is one rendered gauge: three rows, each exactly the requested width
// in terminal cells (before styling; the escape sequences add no cells).
type Block struct {
	Bezel string
	Track string
	Scale string
}

// The palette. Fill colour is per band (see Band); everything else is fixed.
var (
	frameStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))  // bright black
	needleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15")) // bright white
	spentStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Faint(true)
)

// Band returns the colour for a window with pct remaining: bright green
// from 75 up, yellow from 50, orange from 25, red below that. A rate-limited
// window is red whatever is left in it, because the number is no longer the
// thing standing between the user and the wall.
//
// The whole fill takes one colour — the bar says how much is left, the
// colour says how worried to be — and the layout colours the percentage
// with the same call so the two never disagree.
func Band(pct float64, rateLimited bool) lipgloss.Color {
	switch {
	case rateLimited:
		return lipgloss.Color("1") // red
	case pct >= 75:
		return lipgloss.Color("10") // bright green
	case pct >= 50:
		return lipgloss.Color("3") // yellow
	case pct >= 25:
		return lipgloss.Color("208") // orange
	default:
		return lipgloss.Color("1") // red
	}
}

// Render draws a gauge width cells wide (the ┴ caps included) for a window
// with pct remaining. pct is clamped to [0, 100]. It returns an error, and
// the zero Block, for a width under MinWidth.
func Render(pct float64, width int, rateLimited bool) (Block, error) {
	if width < MinWidth {
		return Block{}, fmt.Errorf("gauge: width %d is under the %d-cell minimum (a %d-cell track plus two caps)",
			width, MinWidth, MinWidth-2)
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}

	n := width - 2 // track cells
	needle := needleIndex(pct, n)

	fill := lipgloss.NewStyle().Foreground(Band(pct, rateLimited))
	capCell := frameStyle.Render("┴")

	var track strings.Builder
	track.WriteString(capCell)
	if needle > 0 {
		track.WriteString(fill.Render(strings.Repeat("▰", needle)))
	}
	track.WriteString(needleStyle.Render("▲"))
	if rest := n - needle - 1; rest > 0 {
		track.WriteString(spentStyle.Render(strings.Repeat("▱", rest)))
	}
	track.WriteString(capCell)

	return Block{
		Bezel: frameStyle.Render("╭" + bezelBody(n) + "╮"),
		Track: track.String(),
		Scale: frameStyle.Render(scale(n)),
	}, nil
}

// Plain returns b with every escape sequence removed, for callers that want
// the geometry without the colour whatever the renderer's profile is.
func Plain(b Block) Block {
	return Block{Bezel: strip(b.Bezel), Track: strip(b.Track), Scale: strip(b.Scale)}
}

// needleIndex is the track cell the needle occupies: cell 0 at 0%, the last
// cell at 100%, rounded to the nearest cell in between.
func needleIndex(pct float64, n int) int {
	return int(pct/100*float64(n-1) + 0.5)
}

// bezelBody is the n-cell run between ╭ and ╮: a ┬ at each quarter of the
// track (the two ends included), the rest ─. The quarter positions are
// truncated, not rounded, so they never collide at small n.
func bezelBody(n int) string {
	tick := make([]bool, n)
	for i := 0; i < 5; i++ {
		tick[i*(n-1)/4] = true
	}
	body := make([]rune, n)
	for i := range body {
		if tick[i] {
			body[i] = '┬'
		} else {
			body[i] = '─'
		}
	}
	return string(body)
}

// scale is the label row, the full width of the gauge: 0 under the first
// track cell, 100 ending under the last one, 50 centred between them. The
// row starts and ends with the cell under a ┴ cap, so it lines up with the
// track above it.
func scale(n int) string {
	buf := []rune(strings.Repeat(" ", n+2))
	put := func(s string, col int) {
		for i, r := range s {
			if col+i >= 0 && col+i < len(buf) {
				buf[col+i] = r
			}
		}
	}
	put("0", 1)
	put("50", 1+(n-1)/2-1)
	put("100", 1+n-3)
	return string(buf)
}

// strip removes ANSI escape sequences (CSI ... final byte) from s. The
// gauge emits nothing else — no OSC, no single-character escapes.
func strip(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			if j < len(s) {
				j++ // the final byte
			}
			i = j
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}
