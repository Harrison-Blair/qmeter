// Package pace compares usage with a continuous, evenly spent allowance.
package pace

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
)

// Assessment is the comparison for one window. A nil expectation means its
// timing is insufficient to calculate pace reliably.
type Assessment struct {
	Pace                     string   `json:"pace"`
	RemainingPercent         float64  `json:"remaining_percent"`
	ExpectedRemainingPercent *float64 `json:"expected_remaining_percent"`
}

// Calculate compares remaining allowance with remaining time using an inclusive
// five percentage point tolerance. Ahead means less allowance remains than expected.
func Calculate(w provider.Window, now time.Time) Assessment {
	a := Assessment{Pace: "n/a", RemainingPercent: w.RemainingPercent}
	if w.ResetsAt.IsZero() || w.Period <= 0 {
		return a
	}
	start := w.ResetsAt.Add(-w.Period)
	if now.Before(start) || !now.Before(w.ResetsAt) {
		return a
	}
	expected := 100 * float64(w.ResetsAt.Sub(now)) / float64(w.Period)
	a.ExpectedRemainingPercent = &expected
	switch {
	case a.RemainingPercent > expected+5:
		a.Pace = "behind"
	case a.RemainingPercent < expected-5:
		a.Pace = "ahead"
	default:
		a.Pace = "on pace"
	}
	return a
}

// RenderText writes every window and provider failure, using the same time
// for each assessment and reset countdown.
func RenderText(w io.Writer, r usage.Result, now time.Time) error {
	return renderTextStyled(w, r, now, display.Renderer(w))
}

func renderTextStyled(w io.Writer, r usage.Result, now time.Time, renderer *lipgloss.Renderer) error {
	if len(r.Windows) == 0 && len(r.Errors) == 0 && len(r.Undetected) == 0 {
		_, err := fmt.Fprintln(w, "no providers detected")
		return err
	}
	rows := [][]display.Cell{display.Header(renderer, "PROVIDER", "WINDOW", "PACE", "REMAINING", "EXPECTED", "RUNS OUT", "RESETS")}
	for _, win := range r.Windows {
		a := Calculate(win, now)
		projection := Forecast(win, now)
		runsOut := "-"
		switch projection.State {
		case "dry":
			runsOut = usage.ResetsCell(provider.Window{ResetsAt: *projection.ExhaustionAt}, now)
		case "empty":
			runsOut = "empty"
		}
		expected := "-"
		if a.ExpectedRemainingPercent != nil {
			expected = fmt.Sprintf("%.1f%%", *a.ExpectedRemainingPercent)
		}
		rows = append(rows, []display.Cell{display.ProviderCell(renderer, win.Provider), {Text: win.Name}, {Text: a.Pace, Style: renderer.NewStyle().Foreground(display.PaceColor(a.Pace)).Bold(true)}, display.RemainingCell(renderer, a.RemainingPercent, win.RateLimited), {Text: expected}, {Text: runsOut}, display.ResetCell(renderer, usage.ResetsCell(win, now), win.RateLimited)})
	}
	rows = append(rows, usage.MessageRows(renderer, r)...)
	return display.Table(w, rows)
}

// assessedWindow explicitly merges the provider's wire form with the pace
// fields, preserving its optional fields without inheriting its marshaler.
type assessedWindow struct {
	window     provider.Window
	assessment Assessment
	projection Projection
}

func (w assessedWindow) MarshalJSON() ([]byte, error) {
	raw, err := json.Marshal(w.window)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	var exhaustion *string
	if w.projection.ExhaustionAt != nil {
		formatted := w.projection.ExhaustionAt.Format(time.RFC3339)
		exhaustion = &formatted
	}
	assessment, err := json.Marshal(struct {
		Assessment
		Exhaustion *string  `json:"projected_exhaustion_at"`
		Remaining  *float64 `json:"projected_remaining_at_reset"`
	}{w.assessment, exhaustion, w.projection.RemainingAtReset})
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(assessment, &fields); err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

// RenderJSON preserves usage's envelope and window fields, and adds the pace
// assessment. Empty arrays are always emitted, and n/a expectations are null.
func RenderJSON(w io.Writer, r usage.Result, now time.Time) error {
	type failure struct {
		Provider string `json:"provider"`
		Message  string `json:"message"`
	}
	type undetected struct {
		Provider string `json:"provider"`
		Reason   string `json:"reason"`
	}
	env := struct {
		Windows    []assessedWindow `json:"windows"`
		Errors     []failure        `json:"errors"`
		Undetected []undetected     `json:"undetected"`
	}{make([]assessedWindow, 0, len(r.Windows)), make([]failure, 0, len(r.Errors)), make([]undetected, 0, len(r.Undetected))}
	for _, win := range r.Windows {
		env.Windows = append(env.Windows, assessedWindow{win, Calculate(win, now), Forecast(win, now)})
	}
	for _, e := range r.Errors {
		env.Errors = append(env.Errors, failure{e.Provider, e.Message})
	}
	for _, u := range r.Undetected {
		env.Undetected = append(env.Undetected, undetected{u.Provider, u.Message})
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(env)
}
