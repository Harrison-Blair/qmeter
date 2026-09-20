// Package spend renders the balances collected alongside provider usage windows.
package spend

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
)

// Amounts formats the remaining allowance and limit for both the table and
// dashboard. Unknown amounts stay unknown; unconfirmed units stay bare.
func Amounts(b provider.Balance) (left, limit string) {
	left, limit = amount(b.Remaining, b.Unit), amount(b.Limit, b.Unit)
	if b.Unlimited {
		left = "unlimited"
	}
	return left, limit
}

func amount(value *float64, unit string) string {
	if value == nil {
		return "-"
	}
	if unit == "usd" {
		return fmt.Sprintf("$%.2f", *value)
	}
	text := strconv.FormatFloat(*value, 'f', -1, 64)
	if unit == "percent" {
		text += "%"
	}
	return text
}

// RenderText writes one balance table, styled for the output destination.
func RenderText(w io.Writer, r usage.Result) error {
	return renderTextStyled(w, r, display.Renderer(w))
}

func renderTextStyled(w io.Writer, r usage.Result, renderer *lipgloss.Renderer) error {
	if len(r.Balances) == 0 && len(r.Errors) == 0 && len(r.Undetected) == 0 {
		_, err := fmt.Fprintln(w, "no balances reported")
		return err
	}
	rows := [][]display.Cell{display.Header(renderer, "PROVIDER", "NAME", "LEFT", "OF", "BAR")}
	for _, b := range r.Balances {
		left, limit := Amounts(b)
		bar := display.Cell{Text: "-"}
		if b.Limit != nil && *b.Limit > 0 && b.Remaining != nil {
			pct := max(0, min(100, *b.Remaining / *b.Limit * 100))
			filled := int(math.Round(pct / 5))
			bar = display.Cell{
				Text:  strings.Repeat("▰", filled) + strings.Repeat("▱", 20-filled),
				Style: renderer.NewStyle().Foreground(display.Band(pct, false)),
			}
		}
		rows = append(rows, []display.Cell{display.ProviderCell(renderer, b.Provider), {Text: b.Name}, {Text: left}, {Text: limit}, bar})
	}
	rows = append(rows, usage.MessageRows(renderer, r)...)
	return display.Table(w, rows)
}

// RenderJSON writes only balances and provider status, with all three arrays
// present and non-null. Balance.MarshalJSON owns each balance's wire fields.
func RenderJSON(w io.Writer, r usage.Result) error {
	type failure struct {
		Provider string `json:"provider"`
		Message  string `json:"message"`
	}
	type undetected struct {
		Provider string `json:"provider"`
		Reason   string `json:"reason"`
	}
	env := struct {
		Balances   []provider.Balance `json:"balances"`
		Errors     []failure          `json:"errors"`
		Undetected []undetected       `json:"undetected"`
	}{
		Balances:   r.Balances,
		Errors:     make([]failure, 0, len(r.Errors)),
		Undetected: make([]undetected, 0, len(r.Undetected)),
	}
	if env.Balances == nil {
		env.Balances = []provider.Balance{}
	}
	for _, e := range r.Errors {
		env.Errors = append(env.Errors, failure{Provider: e.Provider, Message: e.Message})
	}
	for _, u := range r.Undetected {
		env.Undetected = append(env.Undetected, undetected{Provider: u.Provider, Reason: u.Message})
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(env)
}
