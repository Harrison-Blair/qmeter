package display

import (
	"strings"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Balance is one already-formatted provider balance row.
type Balance struct {
	Name   string
	Amount string
}

// CardMessage is a final provider message. Error selects the error colour;
// other messages use the dim colour.
type CardMessage struct {
	Text  string
	Error bool
}

// CardHeight reports the natural line count for a provider card at width.
// Widths below the card minimum cannot be drawn and return zero.
func CardHeight(width int, windows []provider.Window, balances []Balance, messages []CardMessage) int {
	if width < 30 {
		return 0
	}
	height := 2 + 2*len(windows) + len(balances) + len(messages)
	if len(windows) > 1 {
		height += len(windows) - 1
	}
	return height
}

// ProviderCard draws one provider card. It returns false when width is below
// 30 cells. A requested height above the natural height adds blank body rows.
func ProviderCard(providerID, plan string, windows []provider.Window, balances []Balance, messages []CardMessage, width, height int, now time.Time, r *lipgloss.Renderer, theme Theme) ([]string, bool) {
	return providerCard(providerID, plan, windows, balances, messages, width, height, now, false, r, theme)
}

// CachedProviderCard draws a card like ProviderCard for windows kept from an
// earlier fetch: the bars, percentages and countdowns render faint.
func CachedProviderCard(providerID, plan string, windows []provider.Window, balances []Balance, messages []CardMessage, width, height int, now time.Time, r *lipgloss.Renderer, theme Theme) ([]string, bool) {
	return providerCard(providerID, plan, windows, balances, messages, width, height, now, true, r, theme)
}

func providerCard(providerID, plan string, windows []provider.Window, balances []Balance, messages []CardMessage, width, height int, now time.Time, dim bool, r *lipgloss.Renderer, theme Theme) ([]string, bool) {
	if width < 30 {
		return nil, false
	}
	natural := CardHeight(width, windows, balances, messages)
	if height < natural {
		height = natural
	}
	innerWidth := width - 4
	frameStyle := r.NewStyle().Foreground(theme.Accent(providerID)).Faint(true)
	lines := make([]string, 0, height)
	lines = append(lines, cardTop(providerID, plan, width, r, theme))

	body := make([]string, 0, natural-2)
	for i, window := range windows {
		if i > 0 {
			body = append(body, strings.Repeat(" ", innerWidth))
		}
		body = append(body, bar(window, now, innerWidth, dim, r)...)
	}
	for _, balance := range balances {
		parts := []styledPart{{balance.Name, r.NewStyle()}}
		if balance.Amount != "" {
			if balance.Name != "" {
				parts = append(parts, styledPart{" ", r.NewStyle()})
			}
			parts = append(parts, styledPart{balance.Amount, r.NewStyle().Foreground(Neutral).Faint(true)})
		}
		body = append(body, fitParts(parts, innerWidth))
	}
	for _, message := range messages {
		color := Neutral
		style := r.NewStyle().Foreground(color).Faint(true)
		if message.Error {
			style = r.NewStyle().Foreground(Error)
		}
		body = append(body, fitParts([]styledPart{{message.Text, style}}, innerWidth))
	}
	for len(body) < height-2 {
		body = append(body, strings.Repeat(" ", innerWidth))
	}
	for _, line := range body {
		lines = append(lines, frameStyle.Render("│")+" "+line+" "+frameStyle.Render("│"))
	}
	lines = append(lines, frameStyle.Render("╰"+strings.Repeat("─", width-2)+"╯"))
	return lines, true
}

func cardTop(providerID, plan string, width int, r *lipgloss.Renderer, theme Theme) string {
	frame := r.NewStyle().Foreground(theme.Accent(providerID)).Faint(true)
	identityStyle := r.NewStyle().Foreground(theme.Accent(providerID)).Bold(true)
	insideWidth := width - 2
	room := max(0, insideWidth-6)
	identity := ProviderGlyph(providerID) + " " + providerID
	identity = fitText(identity, room)
	identityWidth := runewidth.StringWidth(identity)
	plan = fitText(plan, max(0, room-identityWidth))
	planWidth := runewidth.StringWidth(plan)
	middle := strings.Repeat("─", max(0, room-identityWidth-planWidth))
	return frame.Render("╭─ ") + identityStyle.Render(identity) + frame.Render(" "+middle+" "+plan+" ─╮")
}

func fitParts(parts []styledPart, width int) string {
	content := renderParts(parts, width)
	used := min(partsWidth(parts), max(0, width))
	return content + strings.Repeat(" ", max(0, width-used))
}
