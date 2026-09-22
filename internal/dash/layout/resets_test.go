package layout

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/resets"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

var viewNow = time.Date(2026, 9, 21, 12, 0, 0, 0, time.FixedZone("IST", 19800))

func plainViews(t *testing.T) {
	t.Helper()
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
}
func viewResult() usage.Result {
	return usage.Result{Windows: []provider.Window{
		{Provider: "codex", Name: "codex-window", Plan: "pro", RemainingPercent: 80, ResetsAt: viewNow.Add(time.Hour)},
		{Provider: "claude", Name: "late", Plan: "max", RemainingPercent: 65, ResetsAt: viewNow.Add(6*24*time.Hour + 23*time.Hour)},
		{Provider: "claude", Name: "early", Plan: "other", RemainingPercent: 12, ResetsAt: viewNow.Add(12 * time.Hour), RateLimited: true},
		{Provider: "claude", Name: "unknown", RemainingPercent: 50},
	}, Errors: []usage.ProviderError{{Provider: "claude", Message: "offline"}}, Undetected: []usage.ProviderError{{Provider: "cursor", Message: "missing"}}}
}
func viewLine(t *testing.T, lines []string, text string) string {
	t.Helper()
	for _, line := range lines {
		if strings.Contains(ansi.Strip(line), text) {
			return line
		}
	}
	t.Fatalf("missing %q:\n%s", text, strings.Join(lines, "\n"))
	return ""
}
func TestTimelineCardsOrderAndFit(t *testing.T) {
	plainViews(t)
	for _, capacity := range []int{0, 3, 20, 21, 35} {
		o := Options{Now: viewNow, BodyHeight: capacity}
		r := viewResult()
		before := append([]provider.Window(nil), r.Windows...)
		body := RenderTimeline(r, 120, o)[1:]
		// Four claude rows, one codex row and one cursor row: card heights 6,3,3.
		extra := max(0, capacity-14)
		if len(body) != 14+extra {
			t.Fatalf("capacity %d height %d want %d", capacity, len(body), 14+extra)
		}
		offset := 2
		for i, tc := range []struct {
			id, plan string
			content  []string
		}{{"claude", "max", []string{"early", "late", "unknown", "error: offline"}}, {"codex", "pro", []string{"codex-window"}}, {"cursor", "-", []string{"not detected: missing"}}} {
			h := len(tc.content) + 2 + extra/3
			if i < extra%3 {
				h++
			}
			if !strings.HasPrefix(body[offset], "╭─ ") || !strings.Contains(body[offset], tc.id) || !strings.HasSuffix(body[offset], " "+tc.plan+" ─╮") {
				t.Fatalf("wrong card top %q", body[offset])
			}
			if body[offset+h-1] != "╰"+strings.Repeat("─", 118)+"╯" {
				t.Fatal("wrong bottom")
			}
			start := offset + 1 + (h-2-len(tc.content))/2
			for j, text := range tc.content {
				if !strings.Contains(body[start+j], text) {
					t.Fatalf("packed/centred row %d lacks %s", start+j, text)
				}
			}
			for y := offset + 1; y < offset+h-1; y++ {
				if !strings.HasPrefix(body[y], "│ ") || !strings.HasSuffix(body[y], " │") {
					t.Fatal("missing sides")
				}
			}
			offset += h
		}
		if !reflect.DeepEqual(r.Windows, before) {
			t.Fatal("mutated windows")
		}
	}
}
func TestTimelineAxisAndLocalRuler(t *testing.T) {
	plainViews(t)
	for _, width := range []int{70, 120, 121, 240} {
		ax := width - 39
		for _, tc := range []struct {
			name   string
			d      time.Duration
			marker string
			cell   int
		}{{"due", -time.Hour, "◆", 0}, {"round", 12 * time.Hour, "◆", int(float64(ax-1)/14 + 0.5)}, {"seven", 7 * 24 * time.Hour, "◆", ax - 1}, {"beyond", 8 * 24 * time.Hour, "▸", ax - 1}} {
			r := usage.Result{Windows: []provider.Window{{Provider: "claude", Name: tc.name, RemainingPercent: 68, ResetsAt: viewNow.Add(tc.d)}}}
			body := RenderTimeline(r, width, Options{Now: viewNow, BodyHeight: 9})[1:]
			line := []rune(viewLine(t, body, "▸ "+tc.name))
			axis := string(line[37 : 37+ax])
			cells := []rune(axis)
			if string(cells[tc.cell]) != tc.marker {
				t.Errorf("%d %s marker cell %d: %q", width, tc.name, tc.cell, axis)
			}
			label := formatResets(tc.d)
			at := tc.cell + 2
			if at+len(label) > ax {
				at = tc.cell - len(label) - 1
			}
			if string(cells[at:at+len(label)]) != label {
				t.Errorf("countdown placement: %q", axis)
			}
			for i := 0; i < tc.cell; i++ {
				if i >= at-1 && i < at+len(label)+1 {
					continue
				}
				if cells[i] != '━' {
					t.Errorf("bar cell %d: %q", i, axis)
				}
			}
			rule := []rune(body[1])
			if rule[37] != '├' || rule[37+ax-1] != '┤' {
				t.Fatal("ruler endpoints")
			}
			if string([]rune(body[0])[37:40]) != "now" {
				t.Fatal("now label")
			}
			// Local midnight is twelve hours away, not UTC midnight.
			for day := 0; day < 7; day++ {
				midnight := time.Date(2026, 9, 22+day, 0, 0, 0, 0, viewNow.Location())
				cell := int(float64(midnight.Sub(viewNow))/float64(7*24*time.Hour)*float64(ax-1) + 0.5)
				if rule[37+cell] != '┼' {
					t.Errorf("missing midnight tick at %d", cell)
				}
				if cell >= 3 && cell+3 <= ax && string([]rune(body[0])[37+cell:40+cell]) != midnight.Format("Mon") {
					t.Errorf("missing local weekday at %d", cell)
				}
				// First interior row is blank due to centring and must carry guides.
				if []rune(body[3])[37+cell] != '┊' {
					t.Errorf("blank row missing guide %d", cell)
				}
				if cell > tc.cell && !(cell >= at && cell < at+len(label)) && cells[cell] != '┊' {
					t.Errorf("window missing guide %d: %q", cell, axis)
				}
			}
		}
	}
	r := usage.Result{Windows: []provider.Window{{Provider: "codex", Name: strings.Repeat("界", 30) + "tail", RateLimited: true}}}
	body := RenderTimeline(r, 120, Options{Now: viewNow})[1:]
	line := viewLine(t, body, "↑RL")
	if !strings.Contains(line, "…") || !strings.Contains(line, "tail ↑RL") || !strings.Contains(line, "no reset time reported") {
		t.Fatal(line)
	}
	if ansi.StringWidth(line) != 120 {
		t.Fatal("wide label overflow")
	}
}
func TestTimelineFallbackAndEmpty(t *testing.T) {
	plainViews(t)
	for _, width := range []int{1, 20, 36, 69} {
		r := viewResult()
		got := RenderTimeline(r, width, Options{Now: viewNow, BodyHeight: 35})[1:]
		want := resets.Rows(r, viewNow, width, lipgloss.DefaultRenderer(), display.Default())
		if len(got) != len(want) {
			t.Fatalf("fallback row count %d != %d", len(got), len(want))
		}
		for i, line := range want {
			line = ansi.Truncate(line, width, "")
			line += strings.Repeat(" ", width-ansi.StringWidth(line))
			if got[i] != line {
				t.Errorf("fallback changed width %d", width)
			}
		}
	}
	for _, width := range []int{69, 70, 120} {
		r := usage.Result{Balances: []provider.Balance{{Provider: "claude", Name: "credits"}}}
		got := RenderTimeline(r, width, Options{Now: viewNow, BodyHeight: 35})
		if len(got) != 2 || strings.TrimSpace(got[1]) != "no providers detected" {
			t.Fatalf("empty/balance-only: %q", got)
		}
	}
}
func TestTimelineStylesAndWidths(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	o := Options{Now: viewNow, BodyHeight: 35}
	r := viewResult()
	for width := 70; width <= 150; width++ {
		lines := RenderTimeline(r, width, o)
		for _, line := range lines {
			if ansi.StringWidth(line) != width {
				t.Fatalf("width %d: %q", width, line)
			}
		}
	}
	page := strings.Join(RenderTimeline(r, 120, o), "\n")
	colour := o.Theme.Accent("claude")
	for _, s := range []string{lipgloss.NewStyle().Foreground(colour).Bold(true).Render("◆"), lipgloss.NewStyle().Foreground(colour).Faint(true).Render("━"), rlCdStyle.Render("12h"), lipgloss.NewStyle().Foreground(display.Band(12, true)).Bold(true).Render(" 12.0%"), lipgloss.NewStyle().Foreground(colour).Faint(true).Render("╭"), dimStyle.Render("┊")} {
		if !strings.Contains(page, s) {
			t.Errorf("missing style %q", s)
		}
	}
	// The main card and timeline use byte-identical styled borders.
	main := Render(r, 120, Options{Now: viewNow, Vertical: true})
	if viewLine(t, main, "◆ claude") != viewLine(t, strings.Split(page, "\n"), "◆ claude") {
		t.Fatal("frame styles diverged")
	}
}

func TestTimelineStatusOnlyAndBalanceProviders(t *testing.T) {
	plainViews(t)
	r := usage.Result{Balances: []provider.Balance{{Provider: "claude", Name: "ignored"}}, Errors: []usage.ProviderError{{Provider: "codex", Message: "offline"}}, Undetected: []usage.ProviderError{{Provider: "cursor", Message: "missing"}}}
	got := RenderTimeline(r, 120, Options{Now: viewNow, BodyHeight: 15})[1:]
	height := 15
	second := 9
	firstAt := 5
	secondAt := 11
	if len(got) != height || !strings.Contains(got[2], "● codex") || !strings.Contains(got[second], "▲ cursor") || !strings.Contains(got[firstAt], "error: offline") || !strings.Contains(got[secondAt], "not detected: missing") {
		t.Fatalf("status-only geometry: %q", got)
	}
	if strings.Contains(strings.Join(got, "\n"), "claude") {
		t.Fatal("balance-only provider leaked into timeline")
	}
}

func TestTimelineUnknownResetAxisInset(t *testing.T) {
	plainViews(t)
	result := usage.Result{Windows: []provider.Window{{Provider: "codex", Name: "unknown"}}}
	for _, width := range []int{70, 120} {
		lines := RenderTimeline(result, width, Options{Now: viewNow})
		line := ansi.Strip(viewLine(t, lines, "no reset time reported"))
		start := strings.Index(line, "no reset time reported")
		// Two frame cells + 35 prefix cells + one axis cell of inset.
		if column := ansi.StringWidth(line[:start]); column != 38 {
			t.Errorf("width %d: unknown-reset text starts at column %d, want 38", width, column)
		}
		if []rune(line)[37] != ' ' {
			t.Errorf("width %d: axis cell zero must remain blank: %q", width, line)
		}
	}
}

func TestTimelineRulerOmitsCollidingLabels(t *testing.T) {
	plainViews(t)
	for _, tc := range []struct {
		name                 string
		hour, tick, blankEnd int
		omitted              string
	}{
		{"midnight at cell one", 18, 1, 5, "Tue"},
		{"midnight at cell two", 12, 2, 6, "Tue"},
		{"weekday past axis end", 3, 29, 31, "Mon"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 9, 21, tc.hour, 0, 0, 0, viewNow.Location())
			result := usage.Result{Windows: []provider.Window{{Provider: "claude", Name: "weekly", ResetsAt: now.Add(time.Hour)}}}
			lines := RenderTimeline(result, 70, Options{Now: now})
			labels := []rune(ansi.Strip(lines[1]))[37:68]
			rule := []rune(ansi.Strip(lines[2]))[37:68]
			if string(labels[:3]) != "now" {
				t.Errorf("now was overwritten: %q", string(labels))
			}
			if strings.Contains(string(labels), tc.omitted) {
				t.Errorf("colliding weekday %s must be absent: %q", tc.omitted, string(labels))
			}
			// Also reject partial label remnants after now or at the far edge.
			start := max(3, tc.tick)
			if got := string(labels[start:tc.blankEnd]); got != strings.Repeat(" ", tc.blankEnd-start) {
				t.Errorf("omitted label left fragments: %q", string(labels))
			}
			if rule[tc.tick] != '┼' {
				t.Errorf("omitted weekday must keep its midnight tick at %d", tc.tick)
			}
			if !strings.Contains(string(labels), "Wed") {
				t.Error("non-colliding weekday label disappeared")
			}
		})
	}
}
