package layout_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/dash/layout"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
)

func ledgerNumber(v float64) *float64 { return &v }

func ledgerLineIndex(t *testing.T, lines []string, text string) int {
	t.Helper()
	for i, line := range lines {
		if strings.Contains(line, text) {
			return i
		}
	}
	t.Fatalf("missing %q in:\n%s", text, strings.Join(lines, "\n"))
	return -1
}

func TestLedgerRowsFollowWindowsAndBalanceOrder(t *testing.T) {
	r := usage.Result{
		Windows: []provider.Window{
			{Provider: "claude", Name: "5h", RemainingPercent: 50},
			{Provider: "cursor", Name: "total", RemainingPercent: 80},
		},
		Balances: []provider.Balance{
			{Provider: "claude", Name: "extra usage", Unit: "usd", Remaining: ledgerNumber(5.75), Limit: ledgerNumber(20)},
			{Provider: "claude", Name: "backup", Unit: "percent", Remaining: ledgerNumber(60), Limit: ledgerNumber(100)},
			{Provider: "codex", Name: "credits", Unit: "credits", Remaining: ledgerNumber(3.5)},
			{Provider: "cursor", Name: "included", Unit: "unconfirmed", Remaining: ledgerNumber(0), Limit: ledgerNumber(0)},
			{Provider: "cursor", Name: "on-demand", Unit: "unconfirmed", Unlimited: true},
		},
	}
	for _, width := range []int{layout.MinWidth, 80, 120, 300} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			lines := layout.Render(r, width, opts(false))
			claude := ledgerLineIndex(t, lines, "◆ claude")
			first := ledgerLineIndex(t, lines, "extra usage  $5.75 of $20.00")
			second := ledgerLineIndex(t, lines, "backup  60% of 100%")
			if first != claude+5 || second != first+1 {
				t.Errorf("ledger order: header=%d first=%d second=%d", claude, first, second)
			}
			codex := ledgerLineIndex(t, lines, "● codex")
			credits := ledgerLineIndex(t, lines, "credits  3.5 of -")
			if credits != codex+1 {
				t.Errorf("balance-only section: header=%d balance=%d", codex, credits)
			}
			cursor := ledgerLineIndex(t, lines, "cursor")
			included := ledgerLineIndex(t, lines, "included  0 of 0")
			demand := ledgerLineIndex(t, lines, "on-demand  unlimited of -")
			if included != cursor+5 || demand != included+1 {
				t.Errorf("Cursor rows are misplaced: header=%d included=%d on-demand=%d", cursor, included, demand)
			}
			if strings.Contains(lines[included], "$") || strings.Contains(lines[demand], "$") {
				t.Fatal("Cursor was labeled as currency")
			}
			for _, line := range lines {
				if lipgloss.Width(line) != width {
					t.Fatalf("line width %d, want %d: %q", lipgloss.Width(line), width, line)
				}
			}
		})
	}
}

func TestLedgerOnlyUnknownProviders(t *testing.T) {
	r := usage.Result{Balances: []provider.Balance{
		{Provider: "new-vendor", Name: "credits", Unit: "credits"},
		{Provider: "another-vendor", Name: "included", Unit: "unconfirmed", Remaining: ledgerNumber(0)},
	}}
	lines := layout.Render(r, layout.MinWidth, opts(false))
	first := ledgerLineIndex(t, lines, "new-vendor")
	second := ledgerLineIndex(t, lines, "another-vendor")
	if first >= second {
		t.Fatal("unknown providers lost first-seen order")
	}
	ledgerLineIndex(t, lines, "credits  - of -")
	ledgerLineIndex(t, lines, "included  0 of -")
	if strings.Contains(strings.Join(lines, "\n"), "no providers detected") {
		t.Fatal("balance-only providers disappeared")
	}
}

func TestLedgerGeometry(t *testing.T) {
	r := sample()
	for _, id := range []string{"claude", "codex", "cursor", "opencode-go"} {
		r.Balances = append(r.Balances,
			provider.Balance{Provider: id, Name: "ledger " + strings.Repeat("界", 30) + " reserve", Unit: "usd", Remaining: ledgerNumber(1e80), Limit: ledgerNumber(2e80)},
			provider.Balance{Provider: id, Name: "on-demand", Unit: "unconfirmed", Unlimited: true},
		)
	}
	for width := layout.MinWidth; width <= 300; width++ {
		for _, target := range []int{22, 50, 200} {
			for _, vertical := range []bool{false, true} {
				o := opts(true)
				o.MeterWidth, o.Vertical = target, vertical
				lines := layout.Render(r, width, o)
				for i, line := range lines {
					if lipgloss.Width(line) != width {
						t.Fatalf("width=%d target=%d vertical=%v line %d is %d cells: %q", width, target, vertical, i, lipgloss.Width(line), line)
					}
				}
				if width == layout.MinWidth {
					seen := 0
					for _, line := range lines {
						if strings.Contains(line, "ledge") && strings.Contains(line, "erve") {
							seen++
						}
					}
					if seen != 4 {
						t.Fatalf("ledger names lost at minimum width: found %d, want 4\n%s", seen, strings.Join(lines, "\n"))
					}
				}
			}
		}
	}
}

func TestLedgerLeavesTimelineUnchanged(t *testing.T) {
	r := sample()
	withBalances := r
	withBalances.Balances = []provider.Balance{{Provider: "claude", Name: "extra usage", Unit: "usd", Remaining: ledgerNumber(1)}}
	for _, width := range []int{layout.MinWidth, 80, 120, 300} {
		if got, want := layout.RenderTimeline(withBalances, width, opts(false)), layout.RenderTimeline(r, width, opts(false)); !reflect.DeepEqual(got, want) {
			t.Errorf("ledger changed timeline at width %d", width)
		}
	}
}
