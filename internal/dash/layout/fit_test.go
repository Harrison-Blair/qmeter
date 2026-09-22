package layout_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/dash/layout"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

func TestFitWidthAndColumnSelection(t *testing.T) {
	for _, width := range []int{36, 39, 40, 81, 82, 109, 110, 117, 118, 119, 120, 121, 201, 400} {
		for _, target := range []int{0, 22, 75, 200} {
			for _, vertical := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/%d/%t", width, target, vertical), func(t *testing.T) {
					o := opts(false)
					o.MeterWidth = target
					o.Vertical = vertical
					o.BodyHeight = 100
					preference := target
					if preference == 0 {
						preference = 50
					}
					packed := max(22, (preference*4+4)/5)
					gutter := 2
					if width >= 120 {
						gutter = 4
					}
					two := !vertical && width >= 2*(packed+18)+gutter
					colw := width
					if two {
						colw = (width - gutter) / 2
					}
					got := layout.Render(sample(), width, o)
					if hasLineWith(got, "◆ claude", "● codex") != two {
						t.Error("wrong framed column selection")
					}
					if two {
						head := []rune(findLine(t, got, "◆ claude"))
						if head[colw-1] != '╮' || string(head[colw:colw+gutter]) != strings.Repeat(" ", gutter) || head[colw+gutter] != '╭' {
							t.Errorf("card widths or gutter: %q", string(head))
						}
					}
					inner := colw
					if colw >= 40 {
						inner -= 4
					}
					for _, w := range renderedGaugeWidths(got) {
						if w != inner-14 {
							t.Errorf("meter width=%d want %d", w, inner-14)
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

func TestFitCardsPackedCenteredAndTiled(t *testing.T) {
	for _, capacity := range []int{0, 3, 16, 17, 21, 22, 45} {
		o := opts(false)
		o.BodyHeight = capacity
		got := layout.Render(fitResult(), 120, o)[1:]
		extra := max(0, capacity-16)
		h := 10 + (extra+1)/2
		lastH := 6 + extra/2
		if len(got) != h+lastH {
			t.Fatalf("capacity %d height %d", capacity, len(got))
		}
		for text, want := range map[string]int{"◆ claude": 0, "● codex": 0, "▸ first": 1 + (h-10)/2, "▸ second": 5 + (h-10)/2, "▸ paired": 1 + (h-7)/2, "error: offline": 5 + (h-7)/2, "▲ cursor": h, "▸ last": h + 1 + (lastH-6)/2} {
			if at := indexOfLineWith(got, text); at != want {
				t.Errorf("capacity %d %s row=%d want %d", capacity, text, at, want)
			}
		}
		for y, line := range got {
			cells := []rune(line)
			if y < h {
				switch y {
				case 0:
					if !strings.HasPrefix(line, "╭─ ◆ claude ") || !strings.Contains(line, " ─╮    ╭─ ● codex ") {
						t.Errorf("top: %q", line)
					}
				case h - 1:
					if line != "╰"+strings.Repeat("─", 56)+"╯    ╰"+strings.Repeat("─", 56)+"╯" {
						t.Errorf("bottom: %q", line)
					}
				default:
					if string(cells[:2]) != "│ " || string(cells[56:64]) != " │    │ " || string(cells[118:]) != " │" {
						t.Errorf("sides: %q", line)
					}
				}
			} else if y == h {
				if !strings.HasPrefix(line, "╭─ ▲ cursor ") || cells[119] != '╮' {
					t.Errorf("lone top: %q", line)
				}
			} else if y == len(got)-1 {
				if line != "╰"+strings.Repeat("─", 118)+"╯" {
					t.Errorf("lone bottom: %q", line)
				}
			} else if !strings.HasPrefix(line, "│ ") || !strings.HasSuffix(line, " │") {
				t.Errorf("lone sides: %q", line)
			}
		}
		widths := renderedGaugeWidths(got[h:])
		if len(widths) != 1 || widths[0] != 102 {
			t.Errorf("lone gauge: %v", widths)
		}
	}
}

func TestFitBalancesAndStatusesArePacked(t *testing.T) {
	r := usage.Result{Windows: fitResult().Windows[:2], Balances: []provider.Balance{{Provider: "claude", Name: "credits"}, {Provider: "claude", Name: "bonus"}}, Errors: []usage.ProviderError{{Provider: "claude", Message: "offline"}}, Undetected: []usage.ProviderError{{Provider: "codex", Message: "missing"}}}
	o := opts(false)
	o.BodyHeight = 18
	got := layout.Render(r, 120, o)[1:]
	for text, want := range map[string]int{"▸ first": 3, "▸ second": 7, "credits": 11, "bonus": 12, "error: offline": 13, "not detected: missing": 8} {
		if at := indexOfLineWith(got, text); at != want {
			t.Errorf("%s row=%d want %d", text, at, want)
		}
	}
}

func TestFitNarrowFallbackOuterSlots(t *testing.T) {
	for _, width := range []int{36, 39} {
		for _, spare := range []int{0, 1, 2, 3, 7, 8} {
			o := opts(false)
			o.MeterWidth = width - 14
			compact := layout.Render(fitResult(), width, o)[1:]
			o.BodyHeight = len(compact) + spare
			got := layout.Render(fitResult(), width, o)[1:]
			var want []string
			offset := 0
			for i, n := range []int{9, 6, 5, 0} {
				gap := spare / 4
				if i < spare%4 {
					gap++
				}
				for j := 0; j < gap; j++ {
					want = append(want, strings.Repeat(" ", width))
				}
				want = append(want, compact[offset:offset+n]...)
				offset += n
			}
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("width %d spare %d: fallback differs", width, spare)
			}
		}
	}
}

func TestFitBannerAndSingleton(t *testing.T) {
	o := opts(true)
	o.BodyHeight = 12
	got := layout.Render(usage.Result{Windows: fitResult().Windows[:1]}, 120, o)
	if len(got) != 18 || !strings.HasPrefix(got[6], "╭─ ◆ claude") || indexOfLineWith(got, "▸ first") != 10 {
		t.Fatalf("banner or singleton geometry: %q", got)
	}
	for _, tc := range []struct {
		r       usage.Result
		width   int
		message string
	}{{usage.Result{}, 120, "no providers detected"}, {fitResult(), 20, "terminal too narrow"}} {
		o.Banner = false
		got = layout.Render(tc.r, tc.width, o)
		want := 1
		if tc.width < 36 {
			want = 0
		}
		if len(got) != want+1 || indexOfLineWith(got, tc.message) != want {
			t.Fatalf("status moved: %q", got)
		}
	}
}

func TestFitCardProviderStyles(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	o := opts(false)
	r := usage.Result{Windows: []provider.Window{{Provider: "claude", Name: "5h", Plan: "max"}}}
	got := layout.Render(r, 40, o)[1:]
	rule := lipgloss.NewStyle().Foreground(o.Theme.Accent("claude")).Faint(true)
	bold := lipgloss.NewStyle().Foreground(o.Theme.Accent("claude")).Bold(true)
	plan := lipgloss.NewStyle().Foreground(o.Theme.Accent("claude"))
	for _, styled := range []string{rule.Render("╭"), rule.Render("─ "), bold.Render("◆ "), bold.Render("claude"), plan.Render("max"), rule.Render(" ─"), rule.Render("╮")} {
		if !strings.Contains(got[0], styled) {
			t.Errorf("top lacks style %q", styled)
		}
	}
	if !strings.HasPrefix(got[1], rule.Render("│ ")) || !strings.HasSuffix(got[1], rule.Render(" │")) || got[len(got)-1] != rule.Render("╰"+strings.Repeat("─", 38)+"╯") {
		t.Error("side/bottom border styles missing")
	}
	for _, line := range got {
		if ansi.StringWidth(line) != 40 {
			t.Errorf("styled width: %q", line)
		}
	}
	if head := ansi.Strip(got[0]); head != "╭─ ◆ claude "+strings.Repeat("─", 21)+" max ─╮" {
		t.Errorf("heading: %q", head)
	}
}

func TestFitNarrowFallbackOverflow(t *testing.T) {
	for _, width := range []int{36, 39} {
		for _, capacity := range []int{0, 1, 19} {
			t.Run(fmt.Sprintf("width%d/capacity%d", width, capacity), func(t *testing.T) {
				o := opts(false)
				o.MeterWidth = width - 14
				want := layout.Render(fitResult(), width, o)
				o.BodyHeight = capacity
				got := layout.Render(fitResult(), width, o)
				if strings.Join(got, "\n") != strings.Join(want, "\n") {
					t.Fatalf("overflow must preserve compact unframed sections:\n%s", strings.Join(got, "\n"))
				}
			})
		}
	}
}

func TestFitCardPlanMatchesFirstWindowHeading(t *testing.T) {
	for _, first := range []string{"max", ""} {
		t.Run("first="+first, func(t *testing.T) {
			r := usage.Result{Windows: []provider.Window{
				{Provider: "claude", Name: "first", Plan: first},
				{Provider: "claude", Name: "second", Plan: "pro"},
			}}
			plan := first
			if plan == "" {
				plan = "-"
			}
			border := layout.Render(r, 80, opts(false))[1]
			if !strings.HasPrefix(border, "╭─ ◆ claude ") || !strings.HasSuffix(border, " "+plan+" ─╮") {
				t.Fatalf("card heading must use the first window's plan: %q", border)
			}
		})
	}
}

func TestFitCardsWithoutWindows(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		result        usage.Result
	}{
		{"balance", "credits", usage.Result{Balances: []provider.Balance{{Provider: "codex", Name: "credits"}}}},
		{"error", "error: offline", usage.Result{Errors: []usage.ProviderError{{Provider: "codex", Message: "offline"}}}},
		{"undetected", "not detected: missing", usage.Result{Undetected: []usage.ProviderError{{Provider: "codex", Message: "missing"}}}},
	} {
		for _, capacity := range []int{0, 3, 8, 9} {
			t.Run(fmt.Sprintf("%s/capacity%d", tc.name, capacity), func(t *testing.T) {
				o := opts(false)
				o.BodyHeight = capacity
				got := layout.Render(tc.result, 80, o)[1:]
				height := max(3, capacity)
				if len(got) != height {
					t.Fatalf("card height = %d, want %d", len(got), height)
				}
				if !strings.HasPrefix(got[0], "╭─ ● codex ") || !strings.HasSuffix(got[0], " - ─╮") || got[height-1] != "╰"+strings.Repeat("─", 78)+"╯" {
					t.Fatalf("missing card borders: %q", got)
				}
				at := 1 + (height-3)/2
				if indexOfLineWith(got, tc.content) != at {
					t.Fatalf("content must be centred at row %d: %q", at, got)
				}
				for y := 1; y < height-1; y++ {
					if runewidth.StringWidth(got[y]) != 80 || !strings.HasPrefix(got[y], "│ ") || !strings.HasSuffix(got[y], " │") {
						t.Errorf("invalid card row: %q", got[y])
					}
					if y != at && got[y] != "│"+strings.Repeat(" ", 78)+"│" {
						t.Errorf("row %d should be blank inside the card: %q", y, got[y])
					}
				}
			})
		}
	}
}
