package layout

import (
	"fmt"
	"strings"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/dash/gauge"
	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/resets"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// RenderCalendar places upcoming resets in local-date columns, with other events below.
func RenderCalendar(result usage.Result, width int, o Options) []string {
	if width < 1 {
		return nil
	}
	if width < MinWidth {
		return []string{fitPlain("terminal too narrow", width)}
	}
	now := o.Now
	if now.IsZero() {
		now = time.Now()
	}
	rows := pageHeader(result, width, o)
	if len(result.Windows)+len(result.Errors)+len(result.Undetected) == 0 {
		return finish(append(rows, row{}.put(dimStyle, "no providers detected")), width)
	}
	n := min(8, (width+1)/15)
	colw := (width+1)/n - 1
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	days := make([][]row, n)
	var strip []row
	for _, w := range resets.Sort(result.Windows) {
		day := 0
		for day < n && !w.ResetsAt.Before(today.AddDate(0, 0, day+1)) {
			day++
		}
		if w.ResetsAt.IsZero() || day == n {
			label := "later"
			if w.ResetsAt.IsZero() {
				label = "no reset time"
			}
			identity := lipgloss.NewStyle().Foreground(o.Theme.Accent(w.Provider))
			line := row{}.put(dimStyle, label+"  ").put(identity.Bold(true), display.ProviderGlyph(w.Provider)).put(identity, " "+w.Provider+"  "+w.Name+"  ")
			if !w.ResetsAt.IsZero() {
				line = line.put(cdStyle, w.ResetsAt.In(now.Location()).Format("Mon 02 15:04")+"  ")
			}
			line = line.put(lipgloss.NewStyle().Foreground(gauge.Band(w.RemainingPercent, w.RateLimited)).Bold(true), fmt.Sprintf("%.1f%%", w.RemainingPercent))
			strip = append(strip, line)
			continue
		}
		days[day] = append(days[day], calendarEntry(w, now, colw, o)...)
	}
	for _, group := range []struct {
		items       []usage.ProviderError
		mark, label string
		style       lipgloss.Style
	}{{result.Errors, "!", "error: ", errStyle}, {result.Undetected, "?", "not detected: ", warnStyle}} {
		for _, e := range group.items {
			identity := lipgloss.NewStyle().Foreground(o.Theme.Accent(e.Provider))
			strip = append(strip, row{}.put(group.style, group.mark+" ").put(identity.Bold(true), display.ProviderGlyph(e.Provider)).put(identity, " "+e.Provider+"  ").put(group.style, group.label).put(dimStyle, e.Message))
		}
	}
	var head, rule row
	height := 0
	for i, day := range days {
		if i > 0 {
			head = head.put(dimStyle, "│")
			rule = rule.put(dimStyle, "┼")
		}
		style := plain
		if i == 0 {
			style = style.Bold(true)
		}
		label := row{}.put(style, today.AddDate(0, 0, i).Format("Mon 02"))
		if i == 0 {
			label = label.put(cdStyle, " today")
		}
		head = head.join(label.pad(colw))
		rule = rule.put(dimStyle, strings.Repeat("─", colw))
		height = max(height, len(day))
	}
	stripHeight := 0
	if len(strip) > 0 {
		stripHeight = len(strip) + 1
	}
	if o.Fit {
		height = max(height, o.BodyHeight-2-stripHeight)
	}
	rows = append(rows, head, rule)
	for y := 0; y < height; y++ {
		var line row
		for i, day := range days {
			if i > 0 {
				line = line.put(dimStyle, "│")
			}
			entry := row{}
			if y < len(day) {
				entry = day[y]
			}
			line = line.join(entry.pad(colw))
		}
		rows = append(rows, line)
	}
	if len(strip) > 0 {
		rows = append(rows, row{}.put(dimStyle, strings.Repeat("─", width)))
		for _, line := range strip {
			text := ansi.Truncate(line.text, width, "…")
			rows = append(rows, row{}.raw(text, ansi.StringWidth(text)))
		}
	}
	return finish(rows, width)
}

func calendarEntry(w provider.Window, now time.Time, width int, o Options) []row {
	identity := lipgloss.NewStyle().Foreground(o.Theme.Accent(w.Provider))
	health := lipgloss.NewStyle().Foreground(gauge.Band(w.RemainingPercent, w.RateLimited)).Bold(true)
	first := row{}.put(dimStyle, w.ResetsAt.In(now.Location()).Format("15:04")+" ").put(identity.Bold(true), display.ProviderGlyph(w.Provider)).put(plain, " ")
	first = rightAlign(first, health, fmt.Sprintf("%.1f%%", w.RemainingPercent), 6)
	name := row{}.put(identity, truncMid(w.Name, width))
	suffix := ""
	if w.RateLimited {
		suffix = " [RL]"
	}
	countdown := row{}.put(countdownStyle(w), truncTail("in "+formatResets(w.ResetsAt.Sub(now)), width-len(suffix)))
	if suffix != "" {
		countdown = countdown.put(rlStyle, suffix)
	}
	return []row{first, name, countdown, {}}
}
