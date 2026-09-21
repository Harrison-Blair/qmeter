package layout

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func calendarResult() usage.Result {
	return usage.Result{Windows: []provider.Window{
		{Provider: "codex", Name: "later-today", RemainingPercent: 12, ResetsAt: viewNow.Add(2 * time.Hour), RateLimited: true},
		{Provider: "claude", Name: "overdue", RemainingPercent: 50, ResetsAt: viewNow.Add(-24 * time.Hour)},
		{Provider: "claude", Name: "first-today", RemainingPercent: 68, ResetsAt: viewNow.Add(time.Hour).UTC()},
		{Provider: "codex", Name: "tomorrow", RemainingPercent: 80, ResetsAt: viewNow.Add(24 * time.Hour)},
		{Provider: "claude", Name: "day-seven", RemainingPercent: 40, ResetsAt: viewNow.AddDate(0, 0, 7)},
		{Provider: "cursor", Name: "far-future", RemainingPercent: 30, ResetsAt: viewNow.AddDate(0, 0, 8)},
		{Provider: "cursor", Name: "auto", RemainingPercent: 91},
	}, Errors: []usage.ProviderError{{Provider: "cursor", Message: "offline"}}, Undetected: []usage.ProviderError{{Provider: "opencode-go", Message: "missing"}}}
}
func TestCalendarLocalDaysEntriesAndStrip(t *testing.T) {
	plainViews(t)
	r := calendarResult()
	before := append([]provider.Window(nil), r.Windows...)
	for _, width := range []int{36, 44, 45, 80, 119, 120, 121, 240} {
		body := RenderCalendar(r, width, Options{Now: viewNow})[1:]
		n := min(8, (width+1)/15)
		cw := (width+1)/n - 1
		if len(body) < 14 {
			t.Fatalf("calendar missing grid: %q", body)
		}
		for day := 0; day < n; day++ {
			label := viewNow.AddDate(0, 0, day).Format("Mon 02")
			if day == 0 {
				label += " today"
			}
			if !strings.HasPrefix(string([]rune(body[0])[day*(cw+1):]), label) {
				t.Errorf("width %d label %s: %q", width, label, body[0])
			}
			if day < n-1 && []rune(body[1])[(day+1)*(cw+1)-1] != '┼' {
				t.Fatal("header separator")
			}
		}
		for _, tc := range []struct {
			row  int
			text string
		}{{2, "12:00 ◆  50.0%"}, {3, "overdue"}, {4, "in 0m"}, {6, "13:00 ◆  68.0%"}, {7, "first-today"}, {8, "in 1h"}, {10, "14:00 ●  12.0%"}, {11, "later-today"}, {12, "in 2h [RL]"}} {
			if !strings.HasPrefix(body[tc.row], tc.text) {
				t.Errorf("row %d: %q lacks %q", tc.row, body[tc.row], tc.text)
			}
		}
		if !strings.HasPrefix(string([]rune(body[3])[cw+1:]), "tomorrow") {
			t.Fatal("tomorrow not in second day")
		}
		if n == 8 && !strings.HasPrefix(string([]rune(body[3])[7*(cw+1):]), "day-seven") {
			t.Fatal("last visible day misclassified")
		}
		for _, y := range []int{5, 9, 13} {
			if strings.Trim(body[y], " │") != "" {
				t.Errorf("entry spacer row %d is not blank: %q", y, body[y])
			}
		}
		strip := body[14:]
		if strip[0] != strings.Repeat("─", width) {
			t.Fatal("missing strip rule")
		}
		prefixes := []string{"later", "no reset time", "! ▲ cursor", "? ○ opencode-go"}
		if n < 8 {
			prefixes = append([]string{"later"}, prefixes...)
		}
		if len(strip) != len(prefixes)+1 {
			t.Fatalf("strip height %d, want %d", len(strip), len(prefixes)+1)
		}
		for i, prefix := range prefixes {
			if !strings.HasPrefix(strip[i+1], prefix) {
				t.Errorf("strip row %d order: %q, want %s", i+1, strip[i+1], prefix)
			}
		}
		text := strings.Join(strip, "\n")
		for _, want := range []string{"later", "far-future", "no reset time", "not detected:", "error:"} {
			if !strings.Contains(text, want) {
				t.Errorf("strip lacks %s: %s", want, text)
			}
		}
		if n < 8 && !strings.Contains(text, "day-seven") {
			t.Fatal("off-grid day missing")
		}
		if width >= 80 {
			for _, want := range []string{"later  ▲ cursor  far-future  Tue 29 12:00  30.0%", "no reset time  ▲ cursor  auto  91.0%", "! ▲ cursor  error: offline", "? ○ opencode-go  not detected: missing"} {
				if !strings.Contains(text, want) {
					t.Errorf("strip lacks %q: %s", want, text)
				}
			}
		}
	}
	if !reflect.DeepEqual(r.Windows, before) {
		t.Fatal("calendar mutated input")
	}
}
func TestCalendarFitHeightAndEmpty(t *testing.T) {
	plainViews(t)
	for _, capacity := range []int{0, 1, 18, 19, 20, 35} {
		for _, fit := range []bool{false, true} {
			got := RenderCalendar(calendarResult(), 120, Options{Now: viewNow, Fit: fit, BodyHeight: capacity})[1:]
			// Header 2, busiest day 12, strip rule + four rows = 5.
			height := 19
			if fit {
				height = max(height, capacity)
			}
			if len(got) != height {
				t.Fatalf("fit %t capacity %d height %d want %d", fit, capacity, len(got), height)
			}
			if got[height-5] != strings.Repeat("─", 120) {
				t.Fatal("strip not at bottom")
			}
			for y := 2; y < height-5; y++ {
				for col := 1; col < 8; col++ {
					if []rune(got[y])[col*15-1] != '│' {
						t.Fatalf("separator missing row %d col %d", y, col)
					}
				}
			}
		}
	}
	one := usage.Result{Windows: calendarResult().Windows[2:3]}
	for _, fit := range []bool{false, true} {
		got := RenderCalendar(one, 80, Options{Now: viewNow, Fit: fit, BodyHeight: 20})[1:]
		want := 6
		if fit {
			want = 20
		}
		if len(got) != want || strings.Contains(strings.Join(got[2:], ""), "─") {
			t.Fatal("unneeded strip or wrong grid height")
		}
	}
	for _, width := range []int{0, 1, 35, 36, 120} {
		got := RenderCalendar(usage.Result{}, width, Options{Now: viewNow, Fit: true, BodyHeight: 35})
		switch {
		case width == 0:
			if len(got) != 0 {
				t.Fatal("zero width")
			}
		case width < 36:
			if len(got) != 1 || got[0] != fitPlain("terminal too narrow", width) {
				t.Fatal("narrow fallback")
			}
		default:
			if len(got) != 2 || strings.TrimSpace(got[1]) != "no providers detected" {
				t.Fatal("empty result")
			}
		}
	}
	statuses := usage.Result{Errors: []usage.ProviderError{{Provider: "cursor", Message: "offline"}}}
	got := RenderCalendar(statuses, 80, Options{Now: viewNow, Fit: true, BodyHeight: 20})[1:]
	if len(got) != 20 || !strings.Contains(got[19], "error: offline") {
		t.Fatal("status-only strip placement")
	}
}
func TestCalendarStylesTruncationAndWidths(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	r := calendarResult()
	r.Windows[0].Name = strings.Repeat("界", 20) + "tail"
	r.Errors[0].Message = strings.Repeat("界", 100)
	for width := 36; width <= 150; width++ {
		for _, fit := range []bool{false, true} {
			for _, line := range RenderCalendar(r, width, Options{Now: viewNow, Fit: fit, BodyHeight: 35}) {
				if ansi.StringWidth(line) != width {
					t.Fatalf("width %d: %q", width, line)
				}
			}
		}
	}
	page := strings.Join(RenderCalendar(r, 120, Options{Now: viewNow}), "\n")
	for _, want := range []string{lipgloss.NewStyle().Bold(true).Render("Mon 21"), cdStyle.Render(" today"), lipgloss.NewStyle().Foreground(display.Default().Accent("codex")).Bold(true).Render("●"), lipgloss.NewStyle().Foreground(display.Band(12, true)).Bold(true).Render("12.0%"), rlCdStyle.Render("in 2h"), rlStyle.Render(" [RL]"), dimStyle.Render("│")} {
		if !strings.Contains(page, want) {
			t.Errorf("missing style %q", want)
		}
	}
	if !strings.Contains(ansi.Strip(page), "…") || !strings.Contains(ansi.Strip(page), "tail") {
		t.Fatal("middle truncation lost ends")
	}
}
func TestCalendarLocalDateAcrossDST(t *testing.T) {
	plainViews(t)
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 3, 7, 23, 30, 0, 0, loc)
	r := usage.Result{Windows: []provider.Window{{Provider: "claude", Name: "after-DST", ResetsAt: time.Date(2026, 3, 9, 0, 15, 0, 0, loc).UTC()}}}
	got := RenderCalendar(r, 120, Options{Now: now})[1:]
	if !strings.HasPrefix(string([]rune(got[0])[30:]), "Mon 09") || !strings.HasPrefix(string([]rune(got[2])[30:]), "00:15 ◆") || !strings.HasPrefix(string([]rune(got[3])[30:]), "after-DST") {
		t.Fatal(fmt.Sprint(got))
	}
}
