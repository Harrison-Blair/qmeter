package layout_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/dash/layout"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/mattn/go-runewidth"
)

func tripleWindowPage(width int, limited bool) []string {
	w := provider.Window{Provider: "claude", Name: "5h", RemainingPercent: 68, ResetsAt: now.Add(3*time.Hour + 38*time.Minute), RateLimited: limited}
	return layout.Render(usage.Result{Windows: []provider.Window{w}}, width, opts(false))
}

func TestTripleWindowShapeAndOrder(t *testing.T) {
	for _, width := range []int{36, 39, 40, 64, 120} {
		lines := tripleWindowPage(width, false)
		at := indexOfLineWith(lines, "▸ 5h")
		if len(lines)-at != 6+boolInt(width >= 40) {
			t.Fatalf("width %d: window has %d rows, want six plus frame", width, len(lines)-at)
		}
		for i, mark := range []string{"▸ 5h", "╭", "│▰", "│▰", "┴", "100"} {
			if !strings.Contains(lines[at+i], mark) {
				t.Errorf("width %d row %d: want %q: %q", width, i, mark, lines[at+i])
			}
			if got := runewidth.StringWidth(lines[at+i]); got != width {
				t.Errorf("row %d width %d want %d", i, got, width)
			}
		}
	}
}
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestTripleWindowMiddleReadouts(t *testing.T) {
	for _, width := range []int{36, 40, 120} {
		lines := tripleWindowPage(width, false)
		at := indexOfLineWith(lines, "▸ 5h")
		for y, line := range lines {
			for _, readout := range []string{"68.0%", "3h38m"} {
				if got := strings.Contains(line, readout); got != (y == at+3) {
					t.Errorf("width %d row %d readout %s present %v, middle row %d", width, y, readout, got, at+3)
				}
			}
		}
	}
}

func TestTripleWindowRateLimitBezel(t *testing.T) {
	lines := tripleWindowPage(64, true)
	at := indexOfLineWith(lines, "▸ 5h")
	if len(lines)-at < 6 {
		t.Fatalf("rate-limited window has %d rows, want six", len(lines)-at)
	}
	// All six window rows must exist; only the bezel carries the badge.
	for i, mark := range []string{"▸ 5h", "╭", "│▰", "│▰", "┴", "100"} {
		if !strings.Contains(lines[at+i], mark) {
			t.Errorf("row %d lacks %q: %q", i, mark, lines[at+i])
		}
		if got := strings.Contains(lines[at+i], "[RL]"); got != (i == 1) {
			t.Errorf("row %d RL present %v", i, got)
		}
	}
}

func TestTripleCardsShareHeightAndCenter(t *testing.T) {
	r := usage.Result{Windows: []provider.Window{
		{Provider: "claude", Name: "first", RemainingPercent: 50},
		{Provider: "claude", Name: "second", RemainingPercent: 50},
		{Provider: "codex", Name: "paired", RemainingPercent: 50},
	}}
	lines := layout.Render(r, 120, opts(false))[1:]
	if len(lines) != 14 {
		t.Fatalf("paired card height %d want 14", len(lines))
	}
	for text, want := range map[string]int{"▸ first": 1, "▸ second": 7, "▸ paired": 4} {
		if got := indexOfLineWith(lines, text); got != want {
			t.Errorf("%s row %d want %d", text, got, want)
		}
	}
	if lines[13] != "╰"+strings.Repeat("─", 56)+"╯    ╰"+strings.Repeat("─", 56)+"╯" {
		t.Errorf("cards do not end together: %q", lines[13])
	}
	for y := 1; y < 13; y++ {
		right := string([]rune(lines[y])[62:])
		if y < 4 || y > 9 {
			if right != "│"+strings.Repeat(" ", 56)+"│" {
				t.Errorf("short card row %d not blank: %q", y, right)
			}
		}
	}
}
