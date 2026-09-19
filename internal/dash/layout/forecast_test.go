package layout_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/dash/gauge"
	"github.com/Harrison-Blair/qmeter/internal/dash/layout"
	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

func TestForecastNotes(t *testing.T) {
	for _, tc := range []struct {
		name            string
		remaining       float64
		elapsed, period time.Duration
		limited         bool
		note, marker    string
	}{
		{"dry", 20, 520 * time.Minute, 1200 * time.Minute, false, "dry in 2h10m", "✕"},
		{"survives", 56, 50 * time.Minute, 100 * time.Minute, false, "lands at 12%", "◇"},
		{"round half up", 56.25, 50 * time.Minute, 100 * time.Minute, false, "lands at 13%", "◇"},
		{"empty", 0, 50 * time.Minute, 100 * time.Minute, false, "empty", ""},
		{"rate limited", 70, time.Minute, 100 * time.Minute, true, "empty", "✕"},
		{"none", 70, 0, 0, false, "", ""},
		{"rate limited invalid timing", 70, 0, 0, true, "", ""},
		{"floor", 70, time.Minute, 100 * time.Minute, false, "", ""},
		{"tie", 50, 50 * time.Minute, 100 * time.Minute, false, "lands at 0%", "◇"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, long := range []bool{false, true} {
				name := "5h"
				if long {
					name = "a very long window name that must retain its width budget"
				}
				w := provider.Window{Provider: "claude", Name: name, RemainingPercent: tc.remaining, Period: tc.period, ResetsAt: now.Add(tc.period - tc.elapsed), RateLimited: tc.limited}
				r := usage.Result{Windows: []provider.Window{w}}
				lines := layout.Render(r, 64, layout.Options{Now: now})
				title, bezel, track := lines[2], lines[3], lines[4]
				if tc.note != "" && !long {
					if !strings.Contains(title, "]  "+tc.note) {
						t.Errorf("missing note %q: %q", tc.note, title)
					}
				} else if strings.Contains(title, "dry in") || strings.Contains(title, "lands at") || strings.Contains(title, "empty") {
					t.Errorf("unexpected note: %q", title)
				}
				if long {
					// A forecast must not change the name's existing truncation budget.
					w.Period = 0
					baseline := layout.Render(usage.Result{Windows: []provider.Window{w}}, 64, layout.Options{Now: now})[2]
					// n/a has a different badge width; check both ends survive, not identical truncation.
					if !strings.Contains(title, "a very long window") || !strings.Contains(title, "width budget") || !strings.Contains(baseline, "width budget") {
						t.Errorf("name lost priority: %q", title)
					}
				}
				if tc.marker != "" && !strings.Contains(track, tc.marker) {
					t.Errorf("missing track marker %q: %q", tc.marker, track)
				}
				if tc.marker == "" && strings.ContainsAny(track, "◇✕") {
					t.Errorf("unexpected track marker: %q", track)
				}
				if tc.limited && !strings.Contains(bezel, "[RL]") {
					t.Errorf("missing RL: %q", bezel)
				}
				for _, line := range lines {
					if runewidth.StringWidth(line) != 64 {
						t.Errorf("wrong width: %q", line)
					}
				}
			}
			lipgloss.SetColorProfile(termenv.ANSI256)
			defer lipgloss.SetColorProfile(termenv.Ascii)
			w := provider.Window{Provider: "claude", Name: "5h", RemainingPercent: tc.remaining, Period: tc.period, ResetsAt: now.Add(tc.period - tc.elapsed), RateLimited: tc.limited}
			title := layout.Render(usage.Result{Windows: []provider.Window{w}}, 64, layout.Options{Now: now})[2]
			if tc.note != "" {
				color := display.RateLimited
				if strings.HasPrefix(tc.note, "lands at") {
					color = display.Neutral
				}
				if !strings.Contains(title, lipgloss.NewStyle().Foreground(color).Render(tc.note)) {
					t.Errorf("missing styled note %q: %q", tc.note, title)
				}
			}
		})
	}
}

func TestForecastNoteFitBoundary(t *testing.T) {
	// Fit includes the arrow, name, badge, two spaces, and note.
	// At the minimum block width 36, an 11-cell name fits exactly; 12 does not.
	for _, n := range []int{11, 12} {
		w := provider.Window{Provider: "claude", Name: strings.Repeat("x", n), RemainingPercent: 56, Period: 100 * time.Minute, ResetsAt: now.Add(50 * time.Minute)}
		lines := layout.Render(usage.Result{Windows: []provider.Window{w}}, 36, layout.Options{Now: now, MeterWidth: gauge.MinWidth})
		if got := strings.Contains(lines[2], "lands at 12%"); got != (n == 11) {
			t.Errorf("name width %d: note present %v, title %q", n, got, lines[2])
		}
		if !strings.Contains(lines[2], w.Name) {
			t.Errorf("name truncated for note: %q", lines[2])
		}
		if !strings.Contains(lines[4], "◇") {
			t.Errorf("missing marker: %q", lines[4])
		}
	}
}
