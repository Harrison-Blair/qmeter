package usage

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// renderNow is the fixed "now" every renderer test injects, so the RESETS
// countdown is deterministic.
var renderNow = time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)

func TestRenderText_RendersA5ProviderCard(t *testing.T) {
	res := Result{Windows: []provider.Window{{Provider: "claude", Name: "weekly", Plan: "max", RemainingPercent: 80}}}
	var out bytes.Buffer
	if err := renderText(&out, res, renderNow); err != nil {
		t.Fatal(err)
	}
	want := "╭─ ◆ claude " + strings.Repeat("─", 61) + " max ─╮\n" +
		"│ ▸ weekly" + strings.Repeat(" ", 52) + "80.0% · resets - │\n" +
		"│ " + strings.Repeat("━", 61) + strings.Repeat("─", 15) + " │\n" +
		"╰" + strings.Repeat("─", 78) + "╯\n"
	if got := out.String(); got != want {
		t.Fatalf("card output mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderText_NarrowWidthStillPrintsCard(t *testing.T) {
	res := Result{Windows: []provider.Window{{Provider: "claude", Name: "weekly", RemainingPercent: 80}}}
	var out bytes.Buffer
	if err := renderTextAtWidth(&out, res, renderNow, 20, display.Renderer(&out)); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 4 || !strings.Contains(out.String(), "▸ weekly") {
		t.Fatalf("narrow output did not draw the card: %q", out.String())
	}
	for i, line := range lines {
		if got := ansi.StringWidth(line); got != 20 {
			t.Errorf("line %d width = %d, want 20: %q", i, got, line)
		}
	}
}

func TestRenderText_CardsGoldenWidths(t *testing.T) {
	credit := 1.5
	res := Result{
		Windows: []provider.Window{
			{Provider: "claude", Name: strings.Repeat("界", 30), Plan: "max", RemainingPercent: 80, ResetsAt: renderNow.Add(2 * time.Hour)},
			{Provider: "claude", Name: "weekly", Plan: "max", RemainingPercent: 0, RateLimited: true},
		},
		Balances:   []provider.Balance{{Provider: "opencode-go", Name: "credit pack", Unit: "usd", Remaining: &credit}},
		Errors:     []ProviderError{{Provider: "codex", Message: "offline"}},
		Undetected: []ProviderError{{Provider: "cursor", Message: "not logged in"}},
	}
	body := func(text string, width int) string {
		padding := width - 4 - lipgloss.Width(text)
		if padding < 0 {
			t.Fatalf("golden body exceeds %d cells: %q", width, text)
		}
		return "│ " + text + strings.Repeat(" ", padding) + " │"
	}
	bottom := func(width int) string { return "╰" + strings.Repeat("─", width-2) + "╯" }
	long := strings.Repeat("界", 30)
	want80 := "╭─ ◆ claude " + strings.Repeat("─", 61) + " max ─╮\n" +
		body("▸ "+long+" 80.0% · 2h", 80) + "\n" +
		body(strings.Repeat("━", 61)+strings.Repeat("─", 15), 80) + "\n" +
		body("", 80) + "\n" +
		body("▸ weekly"+strings.Repeat(" ", 38)+"0.0% · resets - · rate limited", 80) + "\n" +
		body(strings.Repeat("─", 76), 80) + "\n" + bottom(80) + "\n" +
		"╭─ ● codex " + strings.Repeat("─", 64) + " - ─╮\n" +
		body("error: offline", 80) + "\n" + bottom(80) + "\n" +
		"╭─ ○ opencode-go " + strings.Repeat("─", 58) + " - ─╮\n" +
		body("credit pack $1.50", 80) + "\n" + bottom(80) + "\n" +
		"╭─ ▲ cursor " + strings.Repeat("─", 63) + " - ─╮\n" +
		body("not detected: not logged in", 80) + "\n" + bottom(80) + "\n"
	long120 := strings.Repeat("界", 20) + "…"
	blank59 := body("", 59)
	want120 := "╭─ ◆ claude " + strings.Repeat("─", 40) + " max ─╮  " + "╭─ ● codex " + strings.Repeat("─", 43) + " - ─╮\n" +
		body("▸ "+long120+"  80.0% · 2h", 59) + "  " + body("error: offline", 59) + "\n" +
		body(strings.Repeat("━", 44)+strings.Repeat("─", 11), 59) + "  " + blank59 + "\n" +
		blank59 + "  " + blank59 + "\n" +
		body("▸ weekly"+strings.Repeat(" ", 17)+"0.0% · resets - · rate limited", 59) + "  " + blank59 + "\n" +
		body(strings.Repeat("─", 55), 59) + "  " + blank59 + "\n" +
		bottom(59) + "  " + bottom(59) + "\n" +
		"╭─ ○ opencode-go " + strings.Repeat("─", 37) + " - ─╮  " + "╭─ ▲ cursor " + strings.Repeat("─", 42) + " - ─╮\n" +
		body("credit pack $1.50", 59) + "  " + body("not detected: not logged in", 59) + "\n" +
		bottom(59) + "  " + bottom(59) + "\n"

	for _, tc := range []struct {
		width int
		want  string
	}{{80, want80}, {120, want120}} {
		t.Run(strconv.Itoa(tc.width), func(t *testing.T) {
			var out bytes.Buffer
			if err := renderTextAtWidth(&out, res, renderNow, tc.width, display.Renderer(&out)); err != nil {
				t.Fatal(err)
			}
			if got := out.String(); got != tc.want {
				t.Fatalf("width %d mismatch\n got:\n%s\nwant:\n%s", tc.width, got, tc.want)
			}
		})
	}
}

func TestRenderText_NoProvidersDetectedLine(t *testing.T) {
	var out bytes.Buffer
	if err := renderText(&out, Result{}, renderNow); err != nil {
		t.Fatalf("renderText: %v", err)
	}
	const want = "no providers detected\n"
	if got := out.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderText_UsesRealNowByDefault(t *testing.T) {
	res := Result{
		Windows: []provider.Window{
			{Provider: "claude", Name: "5h", RemainingPercent: 99, ResetsAt: time.Now().Add(2*time.Hour + 13*time.Minute)},
		},
	}

	var out bytes.Buffer
	if err := RenderText(&out, res); err != nil {
		t.Fatalf("RenderText: %v", err)
	}
	if !strings.Contains(out.String(), "in 2h1") {
		t.Fatalf("RenderText did not count down from time.Now(): %q", out.String())
	}
}

func TestFormatResets_LargestTwoNonZeroUnits(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"days and hours", 3*24*time.Hour + 4*time.Hour, "3d4h"},
		{"whole days", 12 * 24 * time.Hour, "12d"},
		{"days with zero hours but minutes", 3*24*time.Hour + 7*time.Minute, "3d"},
		{"hours and minutes", 2*time.Hour + 13*time.Minute, "2h13m"},
		{"whole hours", 5 * time.Hour, "5h"},
		{"minutes only", 45 * time.Minute, "45m"},
		{"sub-minute", 30 * time.Second, "0m"},
		{"zero", 0, "0m"},
		{"already past", -5 * time.Minute, "0m"},
		{"days hours and minutes truncates", 7*24*time.Hour + 22*time.Hour + 59*time.Minute, "7d22h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatResets(tt.d); got != tt.want {
				t.Fatalf("formatResets(%s) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestRenderJSON_MatchesEnvelopeWithAllThreeKeysAlwaysPresent(t *testing.T) {
	res := Result{
		Windows: []provider.Window{
			{
				Provider:         "claude",
				Name:             "5h",
				Plan:             "max",
				RemainingPercent: 57.5,
				ResetsAt:         time.Date(2026, time.September, 16, 14, 13, 0, 0, time.UTC),
				Period:           5 * time.Hour,
			},
		},
		Errors: []ProviderError{
			{Provider: "codex", Message: "token expired, open codex to refresh"},
		},
		Undetected: []ProviderError{
			{Provider: "cursor", Message: "not logged in, run cursor-agent to log in"},
		},
	}

	var out bytes.Buffer
	if err := RenderJSON(&out, res); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	want := `{"windows":[{"provider":"claude","name":"5h","plan":"max","remaining_percent":57.5,` +
		`"resets_at":"2026-09-16T14:13:00Z","period_seconds":18000,"rate_limited":false}],` +
		`"errors":[{"provider":"codex","message":"token expired, open codex to refresh"}],` +
		`"undetected":[{"provider":"cursor","reason":"not logged in, run cursor-agent to log in"}]}` + "\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestRenderJSON_DoesNotEscapeHTMLInMessages(t *testing.T) {
	res := Result{
		Undetected: []ProviderError{
			{Provider: "cursor", Message: `no readable /home/me/a&b/auth.json <or> other`},
		},
	}

	var out bytes.Buffer
	if err := RenderJSON(&out, res); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	// A path or reason carrying & < > must survive verbatim rather than
	// arriving as & — these are messages, not HTML.
	if !strings.Contains(out.String(), `/home/me/a&b/auth.json <or> other`) {
		t.Fatalf("message was HTML-escaped: %s", out.String())
	}
}

func TestRenderJSON_NilSlicesRenderAsEmptyArrays(t *testing.T) {
	var out bytes.Buffer
	if err := RenderJSON(&out, Result{}); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	want := `{"windows":[],"errors":[],"undetected":[]}` + "\n"
	if got := out.String(); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestColoredCardsKeepPlainAlignment(t *testing.T) {
	r := Result{Windows: []provider.Window{{Provider: "claude", Name: "日本", Plan: "max", RemainingPercent: 80, ResetsAt: renderNow.Add(time.Hour)}, {Provider: "codex", Name: "weekly", RemainingPercent: 30, RateLimited: true}}, Errors: []ProviderError{{Provider: "cursor", Message: "failed"}}, Undetected: []ProviderError{{Provider: "opencode-go", Message: "missing"}}}
	var plainOut, colored bytes.Buffer
	if err := renderText(&plainOut, r, renderNow); err != nil {
		t.Fatal(err)
	}
	renderer := lipgloss.NewRenderer(&colored)
	renderer.SetColorProfile(termenv.ANSI256)
	renderer.SetHasDarkBackground(true)
	if err := renderTextStyled(&colored, r, renderNow, renderer); err != nil {
		t.Fatal(err)
	}
	plainLines := strings.Split(strings.TrimSuffix(plainOut.String(), "\n"), "\n")
	coloredLines := strings.Split(strings.TrimSuffix(ansi.Strip(colored.String()), "\n"), "\n")
	if len(coloredLines) != len(plainLines) {
		t.Fatalf("ANSI changed line count: %d vs %d", len(coloredLines), len(plainLines))
	}
	for i := range plainLines {
		if lipgloss.Width(coloredLines[i]) != lipgloss.Width(plainLines[i]) {
			t.Errorf("ANSI changed line %d width: %d vs %d", i, lipgloss.Width(coloredLines[i]), lipgloss.Width(plainLines[i]))
		}
	}
	for _, want := range []string{
		renderer.NewStyle().Foreground(display.Default().Accent("claude")).Bold(true).Render("◆ claude"),
		renderer.NewStyle().Foreground(lipgloss.Color("10")).Bold(true).Render("80.0%"),
		renderer.NewStyle().Foreground(lipgloss.Color("1")).Bold(true).Render("30.0%"),
		renderer.NewStyle().Foreground(display.Neutral).Faint(true).Render(" · resets in 1h"),
		renderer.NewStyle().Foreground(lipgloss.Color("1")).Render(" · rate limited"),
		renderer.NewStyle().Foreground(lipgloss.Color("9")).Render("error: failed"),
		renderer.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Render("not detected: missing"),
	} {
		if !strings.Contains(colored.String(), want) {
			t.Errorf("missing %q in %q", want, colored.String())
		}
	}
	var wire bytes.Buffer
	if err := RenderJSON(&wire, r); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(wire.String(), "\\u001b") || strings.Contains(wire.String(), "\x1b") {
		t.Fatal("styled JSON")
	}
}
