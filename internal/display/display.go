// Package display shares terminal palettes and table presentation.
package display

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

const (
	Countdown   lipgloss.Color = "6"
	Forecast    lipgloss.Color = "13"
	Error       lipgloss.Color = "9"
	Neutral     lipgloss.Color = "8"
	RateLimited lipgloss.Color = "1"
)

// PaceColor is shared by the CLI status and dashboard badge.
func PaceColor(status string) lipgloss.Color {
	switch status {
	case "behind":
		return "208"
	case "on pace":
		return "10"
	case "ahead":
		return "11"
	default:
		return Neutral
	}
}

// Band reports remaining-allowance health; rate limiting overrides the band.
func Band(pct float64, limited bool) lipgloss.Color {
	switch {
	case limited:
		return RateLimited
	case pct >= 75:
		return "10"
	case pct >= 50:
		return "3"
	case pct >= 25:
		return "208"
	default:
		return "1"
	}
}

var isTerminal = func(w io.Writer) bool {
	f, ok := w.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(f.Fd()))
}
var colorProfile = func(w io.Writer) termenv.Profile { return termenv.NewOutput(w).ColorProfile() }

// Renderer binds styles to the destination, never the global renderer. Pipes
// remain plain even when force-color variables are set; NO_COLOR wins everywhere.
func Renderer(w io.Writer) *lipgloss.Renderer {
	r := lipgloss.NewRenderer(w)
	profile := termenv.Ascii
	if os.Getenv("NO_COLOR") == "" && isTerminal(w) {
		profile = colorProfile(w)
	}
	r.SetColorProfile(profile)
	return r
}

// Cell keeps visible text separate from styling so ANSI never affects padding.
type Cell struct {
	Text  string
	Style lipgloss.Style
}

// Table aligns all non-final cells with two spaces of padding. A row's final
// cell never contributes to column width, including two-cell error messages.
func Table(w io.Writer, rows [][]Cell) error {
	var widths []int
	for _, row := range rows {
		for i, c := range row {
			if i == len(row)-1 {
				break
			}
			for len(widths) <= i {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], runewidth.StringWidth(c.Text))
		}
	}
	for _, row := range rows {
		for i, c := range row {
			if _, err := io.WriteString(w, c.Style.Render(c.Text)); err != nil {
				return err
			}
			if i < len(row)-1 {
				if _, err := io.WriteString(w, strings.Repeat(" ", widths[i]-runewidth.StringWidth(c.Text)+2)); err != nil {
					return err
				}
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}

// ProviderCell uses the built-in provider palette; CLI output does not load
// interactive dashboard configuration.
func ProviderCell(r *lipgloss.Renderer, id string) Cell {
	return Cell{id, r.NewStyle().Foreground(Default().Accent(id))}
}
func RemainingCell(r *lipgloss.Renderer, pct float64, limited bool) Cell {
	return Cell{fmt.Sprintf("%.1f%%", pct), r.NewStyle().Foreground(Band(pct, limited))}
}
func ResetCell(r *lipgloss.Renderer, text string, limited bool) Cell {
	color := Countdown
	if limited {
		color = RateLimited
	}
	return Cell{text, r.NewStyle().Foreground(color)}
}
func Header(r *lipgloss.Renderer, names ...string) []Cell {
	cells := make([]Cell, len(names))
	for i, name := range names {
		cells[i] = Cell{name, r.NewStyle().Bold(true)}
	}
	return cells
}
