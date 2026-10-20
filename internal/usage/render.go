package usage

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

// RenderText writes r as A5 provider cards with countdowns from time.Now().
// Color follows the output destination and NO_COLOR; redirected output is plain.
func RenderText(w io.Writer, r Result) error {
	return renderText(w, r, time.Now())
}

// renderText uses one time for every countdown and a writer-bound renderer.
func renderText(w io.Writer, r Result, now time.Time) error {
	return renderTextStyled(w, r, now, display.Renderer(w))
}

func renderTextStyled(w io.Writer, r Result, now time.Time, renderer *lipgloss.Renderer) error {
	width := 80
	if f, ok := w.(interface{ Fd() uintptr }); ok && term.IsTerminal(int(f.Fd())) {
		if cols, _, err := term.GetSize(int(f.Fd())); err == nil && cols > 0 {
			width = cols
		}
	}
	return renderTextAtWidth(w, r, now, width, renderer)
}

func renderTextAtWidth(w io.Writer, r Result, now time.Time, width int, renderer *lipgloss.Renderer) error {
	if len(r.Windows) == 0 && len(r.Balances) == 0 && len(r.Errors) == 0 && len(r.Undetected) == 0 {
		_, err := fmt.Fprintln(w, "no providers detected")
		return err
	}
	cards := make(map[string]*providerCard)
	var seen []string
	add := func(id string) *providerCard {
		if id == "" {
			return nil
		}
		card := cards[id]
		if card == nil {
			card = &providerCard{plan: "-"}
			cards[id] = card
			seen = append(seen, id)
		}
		return card
	}
	for _, win := range r.Windows {
		card := add(win.Provider)
		if card == nil {
			continue
		}
		if len(card.windows) == 0 {
			if win.Plan != "" {
				card.plan = win.Plan
			}
		}
		card.windows = append(card.windows, win)
	}
	for _, balance := range r.Balances {
		card := add(balance.Provider)
		if card == nil {
			continue
		}
		left, _ := display.BalanceAmounts(balance)
		card.balances = append(card.balances, display.Balance{Name: balance.Name, Amount: left})
	}
	for _, failure := range r.Errors {
		if card := add(failure.Provider); card != nil {
			card.messages = append(card.messages, display.CardMessage{Text: "error: " + failure.Message, Error: true})
		}
	}
	for _, missing := range r.Undetected {
		if card := add(missing.Provider); card != nil {
			card.messages = append(card.messages, display.CardMessage{Text: "not detected: " + missing.Message})
		}
	}

	ordered := make([]string, 0, len(cards))
	for _, id := range display.ProviderOrder() {
		if cards[id] != nil {
			ordered = append(ordered, id)
		}
	}
	known := make(map[string]bool, len(ordered))
	for _, id := range ordered {
		known[id] = true
	}
	for _, id := range seen {
		if !known[id] {
			ordered = append(ordered, id)
			known[id] = true
		}
	}

	columns, cardWidth, gutter := 1, width, 0
	if (width-2)/2 >= 40 {
		columns, cardWidth, gutter = 2, (width-2)/2, 2
	}
	cardWidth = max(cardWidth, 30)
	for i := 0; i < len(ordered); i += columns {
		rowIDs := ordered[i:min(i+columns, len(ordered))]
		height := 0
		for _, id := range rowIDs {
			natural := display.CardHeight(cardWidth, cards[id].windows, cards[id].balances, cards[id].messages)
			height = max(height, natural)
		}
		rowCards := make([][]string, len(rowIDs))
		for j, id := range rowIDs {
			lines, ok := display.ProviderCard(id, cards[id].plan, cards[id].windows, cards[id].balances, cards[id].messages, cardWidth, height, now, renderer, display.Default())
			if ok {
				rowCards[j] = lines
			}
		}
		for y := 0; y < height; y++ {
			for j, lines := range rowCards {
				if j > 0 {
					if _, err := io.WriteString(w, strings.Repeat(" ", gutter)); err != nil {
						return err
					}
				}
				if y < len(lines) {
					line := lines[y]
					if cardWidth > width {
						line = ansi.Truncate(line, width, "")
					}
					if _, err := io.WriteString(w, line); err != nil {
						return err
					}
				}
			}
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
	}
	return nil
}

type providerCard struct {
	plan     string
	windows  []provider.Window
	balances []display.Balance
	messages []display.CardMessage
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
		cell = "in " + display.FormatCountdown(w.ResetsAt.Sub(now))
	}
	if w.RateLimited {
		cell += "  (rate limited)"
	}
	return cell
}

func formatResets(d time.Duration) string { return display.FormatCountdown(d) }

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
