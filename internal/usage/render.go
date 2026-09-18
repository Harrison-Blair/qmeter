package usage

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/charmbracelet/lipgloss"
)

// RenderText writes r as a human-readable table with countdowns from time.Now().
// Color follows the output destination and NO_COLOR; redirected output is plain.
//
// It formats only what Run already decided: every message in r.Errors and
// r.Undetected is final text and is printed verbatim, never re-wrapped.
func RenderText(w io.Writer, r Result) error {
	return renderText(w, r, time.Now())
}

// renderText uses one time for every countdown and a writer-bound renderer.
func renderText(w io.Writer, r Result, now time.Time) error {
	return renderTextStyled(w, r, now, display.Renderer(w))
}

func renderTextStyled(w io.Writer, r Result, now time.Time, renderer *lipgloss.Renderer) error {
	if len(r.Windows) == 0 && len(r.Errors) == 0 && len(r.Undetected) == 0 {
		_, err := fmt.Fprintln(w, "no providers detected")
		return err
	}
	rows := [][]display.Cell{display.Header(renderer, "PROVIDER", "WINDOW", "PLAN", "REMAINING", "RESETS")}
	for _, win := range r.Windows {
		plan := win.Plan
		if plan == "" {
			plan = "-"
		}
		rows = append(rows, []display.Cell{display.ProviderCell(renderer, win.Provider), {Text: win.Name}, {Text: plan}, display.RemainingCell(renderer, win.RemainingPercent, win.RateLimited), display.ResetCell(renderer, ResetsCell(win, now), win.RateLimited)})
	}
	rows = append(rows, MessageRows(renderer, r)...)
	return display.Table(w, rows)
}

// MessageRows shares the two-cell provider-error and not-detected rows between
// tables without allowing the final message to widen the WINDOW column.
func MessageRows(renderer *lipgloss.Renderer, r Result) [][]display.Cell {
	var rows [][]display.Cell
	for _, e := range r.Errors {
		rows = append(rows, []display.Cell{display.ProviderCell(renderer, e.Provider), {Text: "error: " + e.Message, Style: renderer.NewStyle().Foreground(display.Error)}})
	}
	for _, u := range r.Undetected {
		rows = append(rows, []display.Cell{display.ProviderCell(renderer, u.Provider), {Text: "not detected: " + u.Message, Style: renderer.NewStyle().Foreground(display.Neutral)}})
	}
	return rows
}

// ResetsCell renders a window's whole RESETS cell: the countdown, plus the
// two-space-separated "(rate limited)" suffix when the window is rate
// limited. It is one cell so the suffix never becomes a column of its own.
func ResetsCell(w provider.Window, now time.Time) string {
	cell := "-"
	if !w.ResetsAt.IsZero() {
		cell = "in " + formatResets(w.ResetsAt.Sub(now))
	}
	if w.RateLimited {
		cell += "  (rate limited)"
	}
	return cell
}

// formatResets renders d as its largest two non-zero units: "<d>d<h>h",
// "<h>h<m>m" or "<m>m", dropping a trailing zero unit ("12d", not "12d0h").
// A duration that has already elapsed, or is shorter than a minute, is
// "0m" — the window is due, which is not the same as having no reset time
// at all (that renders as "-", see ResetsCell).
func formatResets(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	days := int64(d / (24 * time.Hour))
	hours := int64(d/time.Hour) % 24
	minutes := int64(d/time.Minute) % 60

	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("%dd%dh", days, hours)
	case days > 0:
		return fmt.Sprintf("%dd", days)
	case hours > 0 && minutes > 0:
		return fmt.Sprintf("%dh%dm", hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

// jsonEnvelope is the --json wire form: three keys, always present, never
// null. Windows marshal through provider.Window's own MarshalJSON.
type jsonEnvelope struct {
	Windows    []provider.Window `json:"windows"`
	Errors     []jsonError       `json:"errors"`
	Undetected []jsonUndetected  `json:"undetected"`
}

// jsonError is one failed provider: ProviderError.Message under "message".
type jsonError struct {
	Provider string `json:"provider"`
	Message  string `json:"message"`
}

// jsonUndetected is one undetected provider: ProviderError.Message under
// "reason", since for these the message is Detect's reason.
type jsonUndetected struct {
	Provider string `json:"provider"`
	Reason   string `json:"reason"`
}

// RenderJSON writes r as the JSON envelope this package's callers consume —
// {"windows":[...],"errors":[...],"undetected":[...]} — with all three keys
// always present, every array non-null even when Result's slices are nil,
// and one trailing newline.
func RenderJSON(w io.Writer, r Result) error {
	env := jsonEnvelope{
		Windows:    r.Windows,
		Errors:     make([]jsonError, 0, len(r.Errors)),
		Undetected: make([]jsonUndetected, 0, len(r.Undetected)),
	}
	if env.Windows == nil {
		env.Windows = []provider.Window{}
	}
	for _, e := range r.Errors {
		env.Errors = append(env.Errors, jsonError{Provider: e.Provider, Message: e.Message})
	}
	for _, u := range r.Undetected {
		env.Undetected = append(env.Undetected, jsonUndetected{Provider: u.Provider, Reason: u.Message})
	}
	enc := json.NewEncoder(w)
	// Messages carry credential paths and provider reasons, not HTML: a
	// path containing & must stay readable rather than become &.
	enc.SetEscapeHTML(false)
	return enc.Encode(env)
}
