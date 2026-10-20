package display

import (
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

func TestResetRowsSortsStablyAndAlignsColumns(t *testing.T) {
	now := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
	windows := []provider.Window{
		{Provider: "cursor", Name: "auto", RemainingPercent: 0, ResetsAt: now.Add(3*24*time.Hour + 12*time.Hour), RateLimited: true},
		{Provider: "claude", Name: "5h", RemainingPercent: 81, ResetsAt: now.Add(16 * time.Minute)},
		{Provider: "claude", Name: "weekly", RemainingPercent: 90, ResetsAt: now.Add(16 * time.Minute)},
		{Provider: "opencode-go", Name: "unknown", RemainingPercent: 49.5},
	}
	got := ResetRows(windows, now, 80, plainRenderer(), Default())
	want := []string{
		"   16m  claude       5h       Wed 16 12:16   now 81.0%            ",
		"   16m  claude       weekly   Wed 16 12:16   now 90.0%            ",
		" 3d12h  cursor       auto     Sun 20 00:00   now 0.0% rate limited",
		"     -  opencode-go  unknown  -              now 49.5%            ",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ResetRows = %#v, want %#v", got, want)
	}
}

func TestResetRowsMatchesBoardExamplesAtWidth60(t *testing.T) {
	now := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
	rows := ResetRows([]provider.Window{
		{Provider: "claude", Name: "5h", RemainingPercent: 81, ResetsAt: now.Add(16 * time.Minute)},
		{Provider: "cursor", Name: "auto", RemainingPercent: 0, ResetsAt: now.Add(3*24*time.Hour + 12*time.Hour), RateLimited: true},
	}, now, 60, plainRenderer(), Default())
	want := []string{
		"   16m  claude  5h    Wed 16 12:16   now 81.0%            ",
		" 3d12h  cursor  auto  Sun 20 00:00   now 0.0% rate limited",
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("board R1 rows = %#v, want %#v", rows, want)
	}
}

func TestResetRowsNeverExceedsWidthAndUsesOverdueCountdown(t *testing.T) {
	now := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
	window := provider.Window{Provider: "claude", Name: "界界界界界界界界", RemainingPercent: 100, ResetsAt: now.Add(-time.Hour)}
	for _, width := range []int{20, 40} {
		got := ResetRows([]provider.Window{window}, now, width, plainRenderer(), Default())
		if len(got) != 1 || runewidth.StringWidth(got[0]) > width {
			t.Fatalf("width %d reset row = %#v", width, got)
		}
		if !strings.Contains(got[0], "0m") {
			t.Errorf("width %d overdue row = %q", width, got[0])
		}
		if width == 40 {
			if !strings.Contains(got[0], "Wed 16 11:00") {
				t.Errorf("overdue reset date = %q", got[0])
			}
			if !strings.Contains(got[0], "…") {
				t.Errorf("wide window name was not truncated with an ellipsis: %q", got[0])
			}
		}
	}
}

func TestResetRowsStylesRateLimitedProviderAndPercent(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.ANSI)
	now := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
	rows := ResetRows([]provider.Window{{Provider: "codex", Name: "5h", RemainingPercent: 12, ResetsAt: now.Add(4 * time.Hour), RateLimited: true}}, now, 80, r, Default())
	providerText := r.NewStyle().Foreground(Default().Accent("codex")).Bold(true).Render("codex")
	percent := r.NewStyle().Foreground(lipgloss.Color("1")).Render("12.0%")
	note := r.NewStyle().Foreground(lipgloss.Color("1")).Render(" rate limited")
	for _, want := range []string{providerText, percent, note} {
		if !strings.Contains(rows[0], want) {
			t.Errorf("R1 row lacks styled text %q: %q", want, rows[0])
		}
	}
}

func TestSortWindowsKeepsTiesAndUnknownsStable(t *testing.T) {
	now := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
	input := []provider.Window{{Name: "unknown A"}, {Name: "tie A", ResetsAt: now}, {Name: "earlier", ResetsAt: now.Add(-time.Minute)}, {Name: "tie B", ResetsAt: now}, {Name: "unknown B"}}
	got := SortWindows(input)
	var names []string
	for _, w := range got {
		names = append(names, w.Name)
	}
	want := []string{"earlier", "tie A", "tie B", "unknown A", "unknown B"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("sort order = %v, want %v", names, want)
	}
	if input[0].Name != "unknown A" {
		t.Fatal("sort changed the input")
	}
}
