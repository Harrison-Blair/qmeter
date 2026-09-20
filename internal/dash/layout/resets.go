package layout

import (
	"time"

	"github.com/Harrison-Blair/qmeter/internal/resets"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// RenderTimeline shares reset rows with the CLI beneath the dashboard header.
func RenderTimeline(result usage.Result, width int, o Options) []string {
	if width < 1 {
		return nil
	}
	now := o.Now
	if now.IsZero() {
		now = time.Now()
	}
	lines := finish(header(result, width, o.Banner, o.Theme), width)
	lines = append(lines, resets.Rows(result, now, width, lipgloss.DefaultRenderer(), o.Theme)...)
	for i, line := range lines {
		line = ansi.Truncate(line, width, "")
		lines[i] = line + blanks(width-lipgloss.Width(line))
	}
	return lines
}
