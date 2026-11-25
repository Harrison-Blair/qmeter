package display

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

func TestProviderCardDrawsFixedWidthAndNaturalOrTallerHeight(t *testing.T) {
	now := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
	windows := []provider.Window{{Provider: "claude", Name: "weekly", RemainingPercent: 96, ResetsAt: now.Add(6*24*time.Hour + 8*time.Hour)}}
	balances := []Balance{{Name: "credits", Amount: "$12"}}
	messages := []CardMessage{{Text: "error: failed", Error: true}, {Text: "not detected: absent"}}
	if got := CardHeight(40, windows, balances, messages); got != 7 {
		t.Fatalf("natural card height = %d, want 7", got)
	}
	lines, ok := ProviderCard("claude", "max", windows, balances, messages, 40, 9, now, plainRenderer(), Default())
	if !ok || len(lines) != 9 {
		t.Fatalf("ProviderCard returned %d lines, ok=%t", len(lines), ok)
	}
	for i, line := range lines {
		if width := runewidth.StringWidth(line); width != 40 {
			t.Errorf("line %d width = %d, want 40: %q", i, width, line)
		}
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"◆ claude", "max", "▸ weekly", "━━━━━━━━", "credits", "$12", "error: failed", "not detected: absent"} {
		if !strings.Contains(joined, want) {
			t.Errorf("card lacks %q: %s", want, joined)
		}
	}
	if strings.Trim(lines[len(lines)-2], " │") != "" {
		t.Errorf("extra row is not blank inside the frame: %q", lines[len(lines)-2])
	}
}

func TestProviderCardAppliesProviderAndMessageColours(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.ANSI)
	theme := Default()
	lines, ok := ProviderCard("claude", "max", nil, []Balance{{Name: "credits", Amount: "$12"}}, []CardMessage{{Text: "error: failed", Error: true}, {Text: "not detected: absent"}}, 40, 0, barNow, r, theme)
	if !ok {
		t.Fatal("card did not draw")
	}
	identity := r.NewStyle().Foreground(theme.Accent("claude")).Bold(true).Render("◆ claude")
	errorText := r.NewStyle().Foreground(lipgloss.Color("9")).Render("error: failed")
	dimText := r.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Render("not detected: absent")
	amount := r.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Render("$12")
	for _, want := range []string{identity, errorText, dimText, amount} {
		if !strings.Contains(strings.Join(lines, "\n"), want) {
			t.Errorf("card lacks styled text %q: %#v", want, lines)
		}
	}
}

func TestProviderCardMinimumWidthAndWindowSeparation(t *testing.T) {
	now := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
	windows := []provider.Window{{Name: "one", RemainingPercent: 0}, {Name: "two", RemainingPercent: 100}}
	if _, ok := ProviderCard("codex", "", windows, nil, nil, 29, 0, now, plainRenderer(), Default()); ok {
		t.Fatal("card drew below the 30-cell minimum")
	}
	lines, ok := ProviderCard("codex", "", windows, nil, nil, 30, 0, now, plainRenderer(), Default())
	if !ok || len(lines) != CardHeight(30, windows, nil, nil) {
		t.Fatalf("natural card: %d lines, ok=%t", len(lines), ok)
	}
	if strings.Trim(lines[3], " │") != "" {
		t.Fatalf("two window blocks lack one blank row: %#v", lines)
	}
}

func TestCachedProviderCardDimsBarsAndKeepsTheText(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.ANSI)
	windows := []provider.Window{{Provider: "claude", Name: "5h", RemainingPercent: 100, ResetsAt: barNow.Add(16 * time.Minute)}}
	messages := []CardMessage{{Text: "◌ cached · fetched 14:22:07 · offline", Error: true}}
	fresh, ok := ProviderCard("claude", "max", windows, nil, messages, 50, 0, barNow, r, Default())
	if !ok {
		t.Fatal("fresh card did not draw")
	}
	cached, ok := CachedProviderCard("claude", "max", windows, nil, messages, 50, 0, barNow, r, Default())
	if !ok {
		t.Fatal("cached card did not draw")
	}

	if got, want := ansi.Strip(strings.Join(cached, "\n")), ansi.Strip(strings.Join(fresh, "\n")); got != want {
		t.Fatalf("cached card text differs from the fresh card:\n%s\nwant\n%s", got, want)
	}
	faintPercent := r.NewStyle().Foreground(lipgloss.Color("10")).Faint(true).Render("100.0%")
	faintBar := r.NewStyle().Foreground(lipgloss.Color("10")).Faint(true).Render(strings.Repeat("━", 46))
	errorLine := r.NewStyle().Foreground(lipgloss.Color("9")).Render("◌ cached · fetched 14:22:07 · offline")
	for _, want := range []string{faintPercent, faintBar, errorLine} {
		if !strings.Contains(strings.Join(cached, "\n"), want) {
			t.Errorf("cached card lacks styled %q", ansi.Strip(want))
		}
	}
	for _, unwanted := range []string{faintPercent, faintBar} {
		if strings.Contains(strings.Join(fresh, "\n"), unwanted) {
			t.Errorf("fresh card has faint %q", ansi.Strip(unwanted))
		}
	}
}
