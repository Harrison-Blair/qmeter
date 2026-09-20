package layout_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/dash/layout"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/mattn/go-runewidth"
)

func TestFitWidthAndColumnSelection(t *testing.T) {
	for _, width := range []int{36, 80, 110, 120, 201, 400} {
		for _, target := range []int{0, 22, 75, 200} {
			for _, vertical := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/%d/%t", width, target, vertical), func(t *testing.T) {
					o := opts(false)
					o.MeterWidth = target
					o.Vertical = vertical
					normal := layout.Render(sample(), width, o)
					two := hasLineWith(normal, "◆ claude", "● codex")
					colw := width
					if two {
						gutter := 2
						if width >= 120 {
							gutter = 4
						}
						colw = (width - gutter) / 2
					}
					o.Fit = true
					o.BodyHeight = 100
					got := layout.Render(sample(), width, o)
					if hasLineWith(got, "◆ claude", "● codex") != two {
						t.Fatal("fit changed column selection")
					}
					for _, w := range renderedGaugeWidths(got) {
						if w != colw-14 {
							t.Fatalf("meter width=%d want %d", w, colw-14)
						}
					}
					for _, line := range got {
						if runewidth.StringWidth(line) != width {
							t.Fatalf("wrong cell width: %q", line)
						}
					}
					if len(got) != 101 {
						t.Fatalf("height=%d want 101", len(got))
					}
				})
			}
		}
	}
}

func fitResult() usage.Result {
	return usage.Result{Windows: []provider.Window{
		{Provider: "claude", Name: "first", RemainingPercent: 50},
		{Provider: "claude", Name: "second", RemainingPercent: 50},
		{Provider: "codex", Name: "paired", RemainingPercent: 50},
		{Provider: "cursor", Name: "last", RemainingPercent: 50},
	}, Errors: []usage.ProviderError{{Provider: "codex", Message: "offline"}}}
}

func TestFitSemanticGapsAndUnequalColumns(t *testing.T) {
	o := opts(false)
	o.Fit = true
	o.BodyHeight = 21
	rows := layout.Render(fitResult(), 120, o)[1:]
	// Compact rows: 9 + 5. Seven spare rows over internal gap and row gap: 4,3.
	for text, want := range map[string]int{"◆ claude": 0, "▸ first": 1, "▸ paired": 1, "▸ second": 9, "error: offline": 9, "▲ cursor": 16, "▸ last": 17} {
		if got := indexOfLineWith(rows, text); got != want {
			t.Errorf("%q row=%d want %d\n%s", text, got, want, strings.Join(rows, "\n"))
		}
	}
	for _, text := range []string{"▸ first", "▸ second", "▸ paired", "▸ last"} {
		start := indexOfLineWith(rows, text)
		if start < 0 || !strings.Contains(rows[start+1], "╭") || !strings.Contains(rows[start+2], "┴") || !strings.Contains(rows[start+3], "100") {
			t.Errorf("split window %s", text)
		}
	}
}

func TestFitCompactOverflowAndSingleton(t *testing.T) {
	o := opts(true)
	o.Fit = true
	o.BodyHeight = 3
	got := layout.Render(fitResult(), 120, o)
	if len(got) != 6+14 {
		t.Fatalf("compact page has %d rows, want 20", len(got))
	}
	if !strings.Contains(got[6], "◆ claude") || !strings.Contains(got[15], "▲ cursor") {
		t.Fatal("decorative gaps survived compact overflow")
	}
	one := usage.Result{Windows: fitResult().Windows[:1]}
	o.Banner = false
	o.BodyHeight = 12
	got = layout.Render(one, 120, o)
	if len(got) != 13 || indexOfLineWith(got, "◆ claude") != 4 {
		t.Fatalf("single block not centered: %q", got)
	}
	for _, tc := range []struct {
		result  usage.Result
		width   int
		message string
	}{{usage.Result{}, 120, "no providers detected"}, {one, 20, "terminal too narrow"}} {
		got = layout.Render(tc.result, tc.width, o)
		want := 0
		if tc.width >= layout.MinWidth {
			want = 1
		}
		if indexOfLineWith(got, tc.message) != want || len(got) != want+1 {
			t.Fatalf("status must remain at top: %q", got)
		}
	}
}

func TestFitBalancesAndStatusesAreAtomic(t *testing.T) {
	r := usage.Result{Balances: []provider.Balance{{Provider: "codex", Name: "credits"}, {Provider: "codex", Name: "bonus"}}, Errors: []usage.ProviderError{{Provider: "codex", Message: "offline"}}, Undetected: []usage.ProviderError{{Provider: "codex", Message: "missing"}}}
	o := opts(false)
	o.Fit = true
	o.BodyHeight = 13
	got := layout.Render(r, 80, o)[1:]
	for text, want := range map[string]int{"● codex": 0, "credits": 1, "bonus": 5, "error: offline": 9, "not detected: missing": 12} {
		if at := indexOfLineWith(got, text); at != want {
			t.Errorf("%s row=%d want %d", text, at, want)
		}
	}
}

func TestFitPairedSectionsShareExpansionAndExactCapacity(t *testing.T) {
	r := usage.Result{Windows: []provider.Window{
		{Provider: "claude", Name: "left-first"}, {Provider: "claude", Name: "left-second"}, {Provider: "claude", Name: "left-third"},
		{Provider: "codex", Name: "right-first"}, {Provider: "codex", Name: "right-second"},
	}}
	for _, rightWindows := range []int{1, 2} {
		r.Windows = r.Windows[:3+rightWindows]
		for _, capacity := range []int{0, 12, 13, 14, 19, 20} {
			o := opts(true)
			o.Fit = true
			o.BodyHeight = capacity
			got := layout.Render(r, 120, o)[6:]
			extra := max(0, capacity-13)
			if len(got) != max(13, capacity) {
				t.Fatalf("capacity %d body=%d", capacity, len(got))
			}
			for text, want := range map[string]int{"left-first": 1, "left-second": 5 + (extra+1)/2, "left-third": 9 + extra, "right-first": 1} {
				if at := indexOfLineWith(got, text); at != want {
					t.Errorf("capacity %d %s row=%d want %d", capacity, text, at, want)
				}
			}
			if rightWindows == 2 && indexOfLineWith(got, "right-second") != 5+extra {
				t.Fatal("short section must distribute its full row expansion over its own gap")
			}
		}
		// Restore the second right-hand window after the no-gap case.
		if rightWindows == 1 {
			r.Windows = append(r.Windows, provider.Window{Provider: "codex", Name: "right-second"})
		}
	}
}
