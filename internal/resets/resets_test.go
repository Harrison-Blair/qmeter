package resets

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

var now = time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)

func TestSort(t *testing.T) {
	windows := []provider.Window{
		{Name: "unknown first"}, {Name: "late", ResetsAt: now.Add(time.Hour)},
		{Name: "equal first", ResetsAt: now}, {Name: "unknown last"}, {Name: "equal last", ResetsAt: now},
	}
	before := append([]provider.Window(nil), windows...)
	got := Sort(windows)
	var names []string
	for _, w := range got {
		names = append(names, w.Name)
	}
	want := []string{"equal first", "equal last", "late", "unknown first", "unknown last"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("sort = %v, want %v", names, want)
	}
	if !reflect.DeepEqual(windows, before) {
		t.Fatal("Sort mutated its input")
	}
}

func TestRow(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.Ascii)
	for _, tc := range []struct {
		name    string
		reset   time.Time
		limited bool
		marker  int
		glyph   string
	}{
		{"known", now.Add(24 * time.Hour), false, 6, "◆"},
		{"beyond", now.Add(8 * 24 * time.Hour), false, 42, "▸"},
		{"unknown", time.Time{}, false, -1, ""},
		{"limited", now.Add(4 * time.Hour), true, 1, "◆"},
		{"due", now.Add(-time.Hour), false, 0, "◆"},
		{"seven days", now.Add(7 * 24 * time.Hour), false, 42, "◆"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, width := range []int{70, 80, 120} {
				w := provider.Window{Provider: "claude", Name: strings.Repeat("界", 60), RemainingPercent: 12, ResetsAt: tc.reset, RateLimited: tc.limited}
				got := Row(w, now, width, r)
				if runewidth.StringWidth(got) != width {
					t.Fatalf("row width = %d, want %d: %q", runewidth.StringWidth(got), width, got)
				}
				nameWidth := width - 60
				label := strings.TrimPrefix(runewidth.Truncate(got, 2+nameWidth, ""), runewidth.Truncate(got, 2, ""))
				if strings.ContainsAny(label, "◆▸") {
					t.Fatalf("marker overlaps name cells [2,%d): %q", 2+nameWidth, label)
				}
				// The final 50 cells hold the 43-cell axis, gutter and countdown.
				prefix := runewidth.Truncate(got, width-50, "")
				axis := strings.TrimPrefix(got, prefix)
				axis = string([]rune(axis)[:43])
				if tc.marker < 0 {
					if strings.TrimSpace(axis) != "" {
						t.Fatalf("unknown reset axis = %q", axis)
					}
				} else {
					want := strings.Repeat("·", tc.marker) + tc.glyph + strings.Repeat(" ", 42-tc.marker)
					if axis != want {
						t.Fatalf("axis = %q, want %q", axis, want)
					}
				}
				if tc.limited && !strings.Contains(label, " ↑RL") {
					t.Fatalf("missing rate-limit relief label: %q", got)
				}
				if strings.Count(got, "◆") != 1+boolInt(tc.glyph == "◆") {
					t.Fatalf("marker overlaps label: %q", got)
				}
			}
		})
	}
	r.SetColorProfile(termenv.ANSI)
	got := Row(provider.Window{Provider: "codex", Name: "5h", RemainingPercent: 80, ResetsAt: now.Add(time.Hour), RateLimited: true}, now, 80, r)
	if !strings.Contains(got, "\x1b[31m●\x1b[0m") || !strings.Contains(got, "\x1b[31m 80.0%\x1b[0m") {
		t.Fatalf("marker and percent lack health band: %q", got)
	}
}
func TestRowLimitedNameAtMinWidth(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.Ascii)
	w := provider.Window{Provider: "codex", Name: "weekly", RemainingPercent: 12, ResetsAt: now.Add(4 * time.Hour), RateLimited: true}
	got := Row(w, now, MinWidth, r)
	t.Logf("%d-cell row: %q", runewidth.StringWidth(got), got)
	if runewidth.StringWidth(got) != MinWidth {
		t.Fatalf("row width = %d, want %d", runewidth.StringWidth(got), MinWidth)
	}
	label := strings.TrimPrefix(runewidth.Truncate(got, 12, ""), "● ")
	if label != "weekly ↑RL" {
		t.Fatalf("name and label at MinWidth = %q, want %q", label, "weekly ↑RL")
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestTextFallback(t *testing.T) {
	result := usage.Result{Windows: []provider.Window{
		{Provider: "codex", Name: "unknown", RemainingPercent: 80},
		{Provider: "claude", Name: "next", RemainingPercent: 12, ResetsAt: now.Add(time.Hour)},
	}}
	for _, tc := range []struct {
		name  string
		width int
		tty   bool
		table bool
	}{
		{"narrow", 69, true, true}, {"pipe", 120, false, true}, {"timeline", 70, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := renderText(&b, result, now, tc.width, tc.tty, display.Renderer(&b)); err != nil {
				t.Fatal(err)
			}
			got := b.String()
			if tc.table {
				for _, header := range []string{"PROVIDER", "WINDOW", "REMAINING", "RESETS"} {
					if !strings.Contains(got, header) {
						t.Fatalf("missing %s: %q", header, got)
					}
				}
				if strings.ContainsAny(got, "◆●·") {
					t.Fatalf("axis in fallback: %q", got)
				}
			} else if !strings.Contains(got, "now") || !strings.Contains(got, "+7d") || !strings.Contains(got, "◆") {
				t.Fatalf("missing timeline: %q", got)
			}
			if strings.Index(got, "next") > strings.Index(got, "unknown") {
				t.Fatalf("unsorted: %q", got)
			}
		})
	}
	var b bytes.Buffer
	if err := RenderText(&b, result, now); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "PROVIDER") {
		t.Fatal("buffer must be non-TTY")
	}
}

func TestRenderJSON(t *testing.T) {
	result := usage.Result{
		Windows: []provider.Window{
			{Provider: "cursor", Name: "unknown", RemainingPercent: 30},
			{Provider: "codex", Name: "later", ResetsAt: now.Add(90*time.Second + 500*time.Millisecond), Period: time.Hour, Plan: "pro"},
			{Provider: "claude", Name: "first", ResetsAt: now.Add(time.Second)},
		},
		Errors:     []usage.ProviderError{{Provider: "failed", Message: "a & b"}},
		Undetected: []usage.ProviderError{{Provider: "missing", Message: "why"}},
	}
	var b bytes.Buffer
	if err := RenderJSON(&b, result, now); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Windows    []map[string]any
		Errors     []map[string]string
		Undetected []map[string]string
	}
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"first", "later", "unknown"} {
		if got.Windows[i]["name"] != name {
			t.Fatalf("order = %v", got.Windows)
		}
		value, ok := got.Windows[i]["resets_in_seconds"]
		want := []any{float64(1), float64(90), nil}[i]
		if !ok || value != want {
			t.Errorf("%s seconds = %v (present %t), want %v", name, value, ok, want)
		}
	}
	if got.Windows[1]["plan"] != "pro" || got.Windows[1]["period_seconds"] != float64(3600) || got.Windows[1]["resets_at"] != now.Add(90*time.Second).Format(time.RFC3339) {
		t.Fatal(got.Windows[1])
	}
	if got.Errors[0]["message"] != "a & b" || got.Undetected[0]["reason"] != "why" || strings.Contains(b.String(), `\u0026`) {
		t.Fatal(b.String())
	}
	b.Reset()
	if err := RenderJSON(&b, usage.Result{}, now); err != nil {
		t.Fatal(err)
	}
	var empty map[string]any
	if err := json.Unmarshal(b.Bytes(), &empty); err != nil {
		t.Fatal(err)
	}
	if len(empty) != 3 {
		t.Fatal(empty)
	}
	for _, key := range []string{"windows", "errors", "undetected"} {
		if arr, ok := empty[key].([]any); !ok || len(arr) != 0 {
			t.Fatalf("%s = %v", key, empty[key])
		}
	}
}
