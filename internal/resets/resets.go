// Package resets orders usage windows by their next reset and draws a timeline.
package resets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// Sort returns a stable copy, with unknown reset times last.
func Sort(windows []provider.Window) []provider.Window {
	out := append([]provider.Window(nil), windows...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ResetsAt.IsZero() {
			return false
		}
		return out[j].ResetsAt.IsZero() || out[i].ResetsAt.Before(out[j].ResetsAt)
	})
	return out
}

// MinWidth is the glyph, ten-cell name, percent, 43-cell axis and countdown.
const MinWidth = 70

const rateLimitSuffix = " ↑RL"

// Row draws one window on a fixed linear axis: one cell per four hours.
// It requires width >= MinWidth; callers are responsible for the table fallback below that.
// Extra width belongs to the name.
func Row(w provider.Window, now time.Time, width int, r *lipgloss.Renderer) string {
	nameWidth := width - 60
	name := w.Name
	if w.RateLimited {
		name = runewidth.Truncate(name, nameWidth-runewidth.StringWidth(rateLimitSuffix), "…") + rateLimitSuffix
	}
	name = runewidth.Truncate(name, nameWidth, "…")
	name += strings.Repeat(" ", nameWidth-runewidth.StringWidth(name))
	health := r.NewStyle().Foreground(display.Band(w.RemainingPercent, w.RateLimited))
	axis := strings.Repeat(" ", 43)
	countdown := "-"
	if !w.ResetsAt.IsZero() {
		d := w.ResetsAt.Sub(now)
		cell := int(max(0, d) / (4 * time.Hour))
		marker := display.ProviderGlyph(w.Provider)
		if d > 7*24*time.Hour {
			cell, marker = 42, "▸"
		}
		axis = strings.Repeat("·", cell) + health.Render(marker) + strings.Repeat(" ", 42-cell)
		countdown = strings.TrimPrefix(usage.ResetsCell(provider.Window{ResetsAt: w.ResetsAt}, now), "in ")
	}
	countdown = runewidth.Truncate(countdown, 6, "…")
	countdown += strings.Repeat(" ", 6-runewidth.StringWidth(countdown))
	return r.NewStyle().Foreground(display.Default().Accent(w.Provider)).Render(display.ProviderGlyph(w.Provider)) + " " + name + " " + health.Render(fmt.Sprintf("%5.1f%%", w.RemainingPercent)) + " " + axis + " " + display.ResetCell(r, countdown, w.RateLimited).Style.Render(countdown)
}

// RenderText uses the destination's terminal width, or a plain table for pipes.
func RenderText(w io.Writer, result usage.Result, now time.Time) error {
	width := 0
	tty := false
	if f, ok := w.(interface{ Fd() uintptr }); ok && term.IsTerminal(int(f.Fd())) {
		tty = true
		width, _, _ = term.GetSize(int(f.Fd()))
	}
	return renderText(w, result, now, width, tty, display.Renderer(w))
}

func renderText(w io.Writer, result usage.Result, now time.Time, width int, tty bool, r *lipgloss.Renderer) error {
	if !tty {
		width = 0
	}
	_, err := fmt.Fprintln(w, strings.Join(Rows(result, now, width, r), "\n"))
	return err
}

// Rows builds the same sorted timeline or narrow table for CLI and dashboard.
func Rows(result usage.Result, now time.Time, width int, r *lipgloss.Renderer) []string {
	if len(result.Windows) == 0 && len(result.Errors) == 0 && len(result.Undetected) == 0 {
		return []string{"no providers detected"}
	}
	var out []string
	var table [][]display.Cell
	if width < MinWidth {
		table = append(table, display.Header(r, "PROVIDER", "WINDOW", "REMAINING", "RESETS"))
	} else {
		out = append(out, strings.Repeat(" ", width-50)+"now"+strings.Repeat(" ", 37)+"+7d")
	}
	for _, w := range Sort(result.Windows) {
		if width < MinWidth {
			table = append(table, []display.Cell{display.ProviderCell(r, w.Provider), {Text: w.Name}, display.RemainingCell(r, w.RemainingPercent, w.RateLimited), display.ResetCell(r, usage.ResetsCell(w, now), w.RateLimited)})
		} else {
			out = append(out, Row(w, now, width, r))
		}
	}
	table = append(table, usage.MessageRows(r, result)...)
	if len(table) > 0 {
		var b strings.Builder
		_ = display.Table(&b, table) // strings.Builder writes cannot fail.
		out = append(out, strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")...)
	}
	return out
}

// RenderJSON extends usage's envelope without changing its window wire fields.
func RenderJSON(w io.Writer, result usage.Result, now time.Time) error {
	result.Windows = Sort(result.Windows)
	var b bytes.Buffer
	if err := usage.RenderJSON(&b, result); err != nil {
		return err
	}
	var envelope struct {
		Windows    []map[string]json.RawMessage `json:"windows"`
		Errors     json.RawMessage              `json:"errors"`
		Undetected json.RawMessage              `json:"undetected"`
	}
	if err := json.Unmarshal(b.Bytes(), &envelope); err != nil {
		return err
	}
	for i, win := range result.Windows {
		var seconds *int64
		if !win.ResetsAt.IsZero() {
			n := int64(win.ResetsAt.Sub(now) / time.Second)
			seconds = &n
		}
		raw, err := json.Marshal(seconds)
		if err != nil {
			return err
		}
		envelope.Windows[i]["resets_in_seconds"] = raw
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(envelope)
}
