package layout_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/dash/layout"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/mattn/go-runewidth"
)

func TestThicknessWindow(t *testing.T) {
	for n := 1; n <= 5; n++ {
		for _, width := range []int{36, 39, 40, 120} {
			o := opts(false)
			o.MeterThickness = n
			r := usage.Result{Windows: []provider.Window{{Provider: "claude", Name: "5h", RemainingPercent: 68, ResetsAt: now.Add(3*time.Hour + 38*time.Minute), RateLimited: true}}}
			lines := layout.Render(r, width, o)
			at := indexOfLineWith(lines, "▸ 5h")
			if at < 0 || len(lines)-at != n+3+boolInt(width >= 40) {
				t.Fatalf("thickness %d width %d: wrong window height", n, width)
			}
			for y, line := range lines {
				for _, s := range []string{"68.0%", "3h38m"} {
					if strings.Contains(line, s) != (y == at+2+(n-1)/2) {
						t.Fatalf("thickness %d row %d misplaced %s", n, y, s)
					}
				}
				if strings.Contains(line, "[RL]") != (y == at+1) {
					t.Fatal("misplaced RL")
				}
				if runewidth.StringWidth(line) != width {
					t.Fatal("wrong width")
				}
			}
		}
	}
}

func TestThicknessDefault(t *testing.T) {
	zero := opts(true)
	zero.MeterThickness = 0
	three := zero
	three.MeterThickness = 3
	for _, width := range []int{36, 40, 80, 100, 120} {
		if !reflect.DeepEqual(layout.Render(sample(), width, zero), layout.Render(sample(), width, three)) {
			t.Fatalf("width %d: zero differs from 3", width)
		}
	}
	TestRenderMatchesTheGoldenPages(t)
}

func thicknessSample() usage.Result {
	return usage.Result{Windows: []provider.Window{{Provider: "claude", Name: "5h", Plan: "max", RemainingPercent: 68, Period: 5 * time.Hour, ResetsAt: now.Add(3*time.Hour + 38*time.Minute)}, {Provider: "claude", Name: "weekly", Plan: "max", RemainingPercent: 50, Period: 7 * 24 * time.Hour, ResetsAt: now.Add(4*24*time.Hour + 17*time.Hour)}, {Provider: "codex", Name: "weekly", Plan: "pro", RemainingPercent: 99, Period: 7 * 24 * time.Hour, ResetsAt: now.Add(6*24*time.Hour + 9*time.Hour)}}}
}
func TestThicknessGoldens(t *testing.T) {
	for _, n := range []int{1, 5} {
		o := opts(false)
		o.MeterThickness = n
		got := layout.Render(thicknessSample(), 120, o)
		want := goldenLines(t, fmt.Sprintf("120-thickness-%d.txt", n))
		if len(got) != len(want) {
			t.Fatalf("thickness %d height %d want %d", n, len(got), len(want))
		}
		for i, line := range got {
			if strings.TrimRight(line, " ") != want[i] {
				t.Fatalf("thickness %d row %d: %q want %q", n, i, line, want[i])
			}
		}
	}
}
