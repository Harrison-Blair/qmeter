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
	lines := finish(pageHeader(result, width, o), width)
	draw := resets.Rows
	if o.Fit {
		draw = resets.FitRows
	}
	body := draw(result, now, width, lipgloss.DefaultRenderer(), o.Theme)
	if o.Fit {
		if len(result.Windows)+len(result.Errors)+len(result.Undetected) > 0 {
			rows := fitTimeline(body, o.BodyHeight)
			body = make([]string, len(rows))
			for i, row := range rows {
				body[i] = row.text
			}
		}
	}
	lines = append(lines, body...)
	for i, line := range lines {
		line = ansi.Truncate(line, width, "")
		lines[i] = line + blanks(width-lipgloss.Width(line))
	}
	return lines
}
