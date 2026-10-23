package display

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

type styledPart struct {
	text  string
	style lipgloss.Style
}

// Bar draws the two A5 lines for one usage window. Both lines have exactly
// width visible cells.
func Bar(w provider.Window, now time.Time, width int, r *lipgloss.Renderer) []string {
	return bar(w, now, width, false, r)
}

// bar is Bar with dim, which renders the percentage and the bar faint.
func bar(w provider.Window, now time.Time, width int, dim bool, r *lipgloss.Renderer) []string {
	if width < 0 {
		width = 0
	}
	percent := fmt.Sprintf("%.1f%%", w.RemainingPercent)
	band := Band(w.RemainingPercent, w.RateLimited)
	percentStyle := bandStyle(band, dim, r)
	dimStyle := r.NewStyle().Foreground(Neutral).Faint(true)
	reset := "-"
	resetText := " · resets -"
	if !w.ResetsAt.IsZero() {
		reset = FormatCountdown(w.ResetsAt.Sub(now))
		resetText = " · resets in " + reset
	}
	rateText := " · rate limited"
	right := []styledPart{{percent, percentStyle}, {resetText, dimStyle}}
	if w.RateLimited {
		right = append(right, styledPart{rateText, r.NewStyle().Foreground(RateLimited)})
	}

	fullNameWidth := runewidth.StringWidth(w.Name)
	minNameWidth := min(fullNameWidth, 8)
	shortened := false
	if width-3-partsWidth(right) < fullNameWidth && !w.ResetsAt.IsZero() {
		resetText = " · " + reset
		right[1].text = resetText
		shortened = true
	}
	if width-3-partsWidth(right) < fullNameWidth && w.RateLimited {
		rateText = " · RL"
		right[2].text = rateText
		shortened = true
	}

	rightWidth := partsWidth(right)
	nameWidth := max(0, width-3-rightWidth)
	if shortened && nameWidth >= fullNameWidth {
		nameWidth = fullNameWidth
	} else if nameWidth < minNameWidth {
		rightWidth = min(rightWidth, max(0, width-3-minNameWidth))
		nameWidth = max(0, width-3-rightWidth)
	}
	name := fitText(w.Name, nameWidth)
	name += strings.Repeat(" ", max(0, nameWidth-runewidth.StringWidth(name)))
	left := "▸ " + name
	if width <= runewidth.StringWidth(left) {
		line := fitText(left, width)
		return []string{line + strings.Repeat(" ", width-runewidth.StringWidth(line)), renderBar(w, width, dim, r)}
	}
	rightText := renderParts(right, rightWidth)
	line := left + " " + rightText
	lineWidth := runewidth.StringWidth(left) + 1 + rightWidth
	line += strings.Repeat(" ", max(0, width-lineWidth))
	return []string{line, renderBar(w, width, dim, r)}
}

// CompactBar draws the one-line small-terminal form. nameWidth fixes the name
// column; the bar uses the remaining width, with a minimum of eight cells.
// dim renders the bar, the percentage and the countdown faint.
func CompactBar(w provider.Window, now time.Time, width, nameWidth int, dim bool, r *lipgloss.Renderer) string {
	width = max(0, width)
	nameWidth = max(0, nameWidth)
	name := fitText(w.Name, nameWidth)
	name += strings.Repeat(" ", max(0, nameWidth-runewidth.StringWidth(name)))
	percent := fmt.Sprintf("%.1f%%", w.RemainingPercent)
	countdown := "-"
	if !w.ResetsAt.IsZero() {
		countdown = FormatCountdown(w.ResetsAt.Sub(now))
	}
	barWidth := max(8, width-nameWidth-runewidth.StringWidth(percent)-runewidth.StringWidth(countdown)-3)
	if dim {
		countdown = r.NewStyle().Foreground(Neutral).Faint(true).Render(countdown)
	}
	return name + " " + renderBar(w, barWidth, dim, r) + " " + bandStyle(Band(w.RemainingPercent, w.RateLimited), dim, r).Render(percent) + " " + countdown
}

// bandStyle is the band colour in bold, or faint when dim.
func bandStyle(band lipgloss.Color, dim bool, r *lipgloss.Renderer) lipgloss.Style {
	if dim {
		return r.NewStyle().Foreground(band).Faint(true)
	}
	return r.NewStyle().Foreground(band).Bold(true)
}

func renderBar(w provider.Window, width int, dim bool, r *lipgloss.Renderer) string {
	if width <= 0 {
		return ""
	}
	filled := 0
	switch {
	case math.IsNaN(w.RemainingPercent), w.RemainingPercent <= 0:
	case w.RemainingPercent >= 100:
		filled = width
	default:
		filled = int(math.Round(w.RemainingPercent / 100 * float64(width)))
	}
	filled = min(width, max(0, filled))
	track := "━"
	if r.ColorProfile() == termenv.Ascii {
		track = "─"
	}
	return bandStyle(Band(w.RemainingPercent, w.RateLimited), dim, r).Render(strings.Repeat("━", filled)) +
		r.NewStyle().Foreground(Neutral).Faint(true).Render(strings.Repeat(track, width-filled))
}

func partsWidth(parts []styledPart) int {
	width := 0
	for _, part := range parts {
		width += runewidth.StringWidth(part.text)
	}
	return width
}

// renderParts keeps complete styled parts when possible and clips the final
// part with an ellipsis when the destination is too narrow.
func renderParts(parts []styledPart, width int) string {
	var b strings.Builder
	remaining := max(0, width)
	for _, part := range parts {
		if remaining == 0 {
			break
		}
		partWidth := runewidth.StringWidth(part.text)
		text := part.text
		if partWidth > remaining {
			text = runewidth.Truncate(text, remaining, "…")
			partWidth = runewidth.StringWidth(text)
		}
		b.WriteString(part.style.Render(text))
		remaining -= partWidth
	}
	return b.String()
}

func fitText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	return runewidth.Truncate(text, width, "…")
}
