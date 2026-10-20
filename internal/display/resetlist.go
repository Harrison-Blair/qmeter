package display

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// SortWindows returns a stable copy ordered by reset time, with unknown times
// last. Windows with equal reset times keep their input order.
func SortWindows(windows []provider.Window) []provider.Window {
	out := append([]provider.Window(nil), windows...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ResetsAt.IsZero() {
			return false
		}
		return out[j].ResetsAt.IsZero() || out[i].ResetsAt.Before(out[j].ResetsAt)
	})
	return out
}

// ResetRows draws one R1 row for each window. It sorts a copy by reset time,
// aligns columns, and truncates the window-name column first when width is
// constrained.
func ResetRows(windows []provider.Window, now time.Time, width int, r *lipgloss.Renderer, theme Theme) []string {
	windows = SortWindows(windows)
	if len(windows) == 0 {
		return nil
	}
	if width < 0 {
		width = 0
	}
	providerWidth, nameWidth, statusWidth := 0, 0, 0
	for _, w := range windows {
		providerWidth = max(providerWidth, runewidth.StringWidth(w.Provider))
		nameWidth = max(nameWidth, runewidth.StringWidth(w.Name))
		status := fmt.Sprintf("now %.1f%%", w.RemainingPercent)
		if w.RateLimited {
			status += " rate limited"
		}
		statusWidth = max(statusWidth, runewidth.StringWidth(status))
	}
	countdownWidth, dateWidth := 6, 12
	gaps := []int{2, 2, 2, 3}
	fixedWidth := func() int {
		return countdownWidth + providerWidth + nameWidth + dateWidth + statusWidth + gaps[0] + gaps[1] + gaps[2] + gaps[3]
	}
	for i := range gaps {
		if fixedWidth() <= width {
			break
		}
		if gaps[i] > 1 {
			gaps[i] = 1
		}
	}
	if fixedWidth() > width {
		nameWidth = max(0, nameWidth-(fixedWidth()-width))
	}
	for _, col := range []struct {
		width *int
		floor int
	}{{&statusWidth, 5}, {&providerWidth, 1}, {&dateWidth, 1}, {&countdownWidth, 1}} {
		if fixedWidth() <= width {
			break
		}
		*col.width = max(col.floor, *col.width-(fixedWidth()-width))
	}
	for _, col := range []*int{&statusWidth, &dateWidth, &providerWidth, &countdownWidth} {
		if fixedWidth() <= width {
			break
		}
		*col = max(0, *col-(fixedWidth()-width))
	}
	for i := range gaps {
		if fixedWidth() <= width {
			break
		}
		gaps[i] = 0
	}

	rows := make([]string, 0, len(windows))
	for _, w := range windows {
		countdown, date := "-", "-"
		if !w.ResetsAt.IsZero() {
			countdown = FormatCountdown(w.ResetsAt.Sub(now))
			date = w.ResetsAt.In(now.Location()).Format("Mon 02 15:04")
		}
		countdown = rightCell(countdown, countdownWidth)
		provider := fitCell(w.Provider, providerWidth)
		name := fitCell(w.Name, nameWidth)
		date = fitCell(date, dateWidth)
		status := resetStatus(w, statusWidth, r)
		row := r.NewStyle().Foreground(Countdown).Render(countdown) + strings.Repeat(" ", gaps[0]) +
			r.NewStyle().Foreground(theme.Accent(w.Provider)).Bold(true).Render(provider) + strings.Repeat(" ", gaps[1]) +
			name + strings.Repeat(" ", gaps[2]) +
			r.NewStyle().Foreground(Neutral).Faint(true).Render(date) + strings.Repeat(" ", gaps[3]) + status
		rows = append(rows, row)
	}
	return rows
}

func resetStatus(w provider.Window, width int, r *lipgloss.Renderer) string {
	percent := fmt.Sprintf("%.1f%%", w.RemainingPercent)
	color := Band(w.RemainingPercent, w.RateLimited)
	parts := []styledPart{{"now ", r.NewStyle()}, {percent, r.NewStyle().Foreground(color)}}
	if w.RateLimited {
		parts = append(parts, styledPart{" rate limited", r.NewStyle().Foreground(RateLimited)})
	}
	text := renderParts(parts, width)
	return text + strings.Repeat(" ", max(0, width-partsWidth(parts)))
}

func rightCell(text string, width int) string {
	text = runewidth.Truncate(text, max(0, width), "…")
	return strings.Repeat(" ", max(0, width-runewidth.StringWidth(text))) + text
}

func fitCell(text string, width int) string {
	if width <= 0 {
		return ""
	}
	return runewidth.Truncate(text, width, "…") + strings.Repeat(" ", max(0, width-runewidth.StringWidth(runewidth.Truncate(text, width, "…"))))
}
