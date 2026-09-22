package layout

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/dash/gauge"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/resets"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const timelinePrefix = 35

// RenderTimeline draws full-width provider cards under a shared local-week ruler.
func RenderTimeline(result usage.Result, width int, o Options) []string {
	if width < 1 {
		return nil
	}
	now := o.Now
	if now.IsZero() {
		now = time.Now()
	}
	rows := header(result, width, o.Banner, o.Theme)
	if width < resets.MinWidth {
		for _, line := range resets.Rows(result, now, width, lipgloss.DefaultRenderer(), o.Theme) {
			line = ansi.Truncate(line, width, "")
			rows = append(rows, row{}.raw(line, ansi.StringWidth(line)))
		}
		return finish(rows, width)
	}
	result.Balances = nil
	providers := presentProviders(result, o.Theme)
	if len(providers) == 0 {
		return finish(append(rows, row{}.put(dimStyle, "no providers detected")), width)
	}
	ax := width - 4 - timelinePrefix
	midnights := timelineMidnights(now, ax)
	labels, rule := []rune(strings.Repeat(" ", ax)), []rune(strings.Repeat("─", ax))
	for cell, day := range midnights {
		rule[cell] = '┼'
		if cell >= 3 && cell+3 <= ax {
			copy(labels[cell:], []rune(day))
		}
	}
	copy(labels, []rune("now"))
	rule[0], rule[ax-1] = '├', '┤'
	rows = append(rows, row{}.pad(2+timelinePrefix).put(dimStyle, string(labels)), row{}.pad(2+timelinePrefix).put(dimStyle, string(rule)))
	contents := make([][]row, len(providers))
	compact := 2
	windows := resets.Sort(result.Windows)
	for i, p := range providers {
		for _, w := range windows {
			if w.Provider == p.id {
				contents[i] = append(contents[i], timelineWindow(w, p, now, ax, midnights))
			}
		}
		for _, e := range result.Errors {
			if e.Provider == p.id {
				contents[i] = append(contents[i], statusRow(errStyle, "!", "error: ", e.Message, width-4))
			}
		}
		for _, u := range result.Undetected {
			if u.Provider == p.id {
				contents[i] = append(contents[i], statusRow(warnStyle, "?", "not detected: ", u.Message, width-4))
			}
		}
		compact += len(contents[i]) + 2
	}
	extra := max(0, o.BodyHeight-compact)
	for i, p := range providers {
		content := contents[i]
		height := len(content) + 2 + share(extra, len(providers), i)
		border := lipgloss.NewStyle().Foreground(p.color).Faint(true)
		rows = append(rows, row{}.put(border, "╭").join(sectionHead(p, providerPlan(result, p.id), width-2)).put(border, "╮"))
		above := (height - 2 - len(content)) / 2
		for y := 0; y < height-2; y++ {
			line := row{}.pad(timelinePrefix).join(timelineAxis(nil, p, now, ax, midnights))
			if at := y - above; at >= 0 && at < len(content) {
				line = content[at]
			}
			rows = append(rows, row{}.put(border, "│ ").join(line).pad(width-2).put(border, " │"))
		}
		rows = append(rows, row{}.put(border, "╰"+strings.Repeat("─", width-2)+"╯"))
	}
	return finish(rows, width)
}

func timelineCell(d time.Duration, width int) int {
	return int(math.Round(max(0, min(1, float64(d)/float64(7*24*time.Hour))) * float64(width-1)))
}

func timelineMidnights(now time.Time, width int) map[int]string {
	out := map[int]string{}
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	end := now.Add(7 * 24 * time.Hour)
	for day := next; !day.After(end); day = day.AddDate(0, 0, 1) {
		out[timelineCell(day.Sub(now), width)] = day.Format("Mon")
	}
	return out
}

func timelineWindow(w provider.Window, p provInfo, now time.Time, ax int, ticks map[int]string) row {
	name := truncMid(w.Name, 24)
	if w.RateLimited {
		name = truncMid(w.Name, 20) + " ↑RL"
	}
	identity := lipgloss.NewStyle().Foreground(p.color)
	pct := lipgloss.NewStyle().Foreground(gauge.Band(w.RemainingPercent, w.RateLimited)).Bold(true)
	return row{}.put(identity.Bold(true), "▸ ").put(identity, name).pad(26).put(plain, " ").
		put(pct, fmt.Sprintf("%5.1f%%", w.RemainingPercent)).put(plain, "  ").join(timelineAxis(&w, p, now, ax, ticks))
}

// Axis entries can span cells (countdowns); later entries covered by a span are skipped.
func timelineAxis(w *provider.Window, p provInfo, now time.Time, width int, ticks map[int]string) row {
	cells := make([]row, width)
	for cell := range ticks {
		cells[cell] = row{}.put(dimStyle, "┊")
	}
	if w != nil {
		if w.ResetsAt.IsZero() {
			cells[1] = row{}.put(dimStyle, "no reset time reported")
		} else {
			d := w.ResetsAt.Sub(now)
			cell := timelineCell(d, width)
			rule := lipgloss.NewStyle().Foreground(p.color).Faint(true)
			for i := 0; i < cell; i++ {
				cells[i] = row{}.put(rule, "━")
			}
			marker := p.icon
			if d > 7*24*time.Hour {
				marker = "▸"
			}
			cells[cell] = row{}.put(lipgloss.NewStyle().Foreground(p.color).Bold(true), marker)
			label := formatResets(d)
			at := cell + 2
			if at+len(label) > width {
				at = cell - len(label) - 2
				cells[at] = row{}.put(plain, " ").put(countdownStyle(*w), label).put(plain, " ")
			} else {
				cells[at] = row{}.put(countdownStyle(*w), label)
			}
		}
	}
	out := row{}
	for i := 0; i < width; {
		c := cells[i]
		if c.cells == 0 {
			c = row{}.put(plain, " ")
		}
		out = out.join(c)
		i += c.cells
	}
	return out
}
