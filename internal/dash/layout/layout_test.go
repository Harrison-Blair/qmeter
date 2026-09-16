package layout_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/dash/layout"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

// TestMain pins the colour profile to Ascii, so every test in this package
// compares bare text against the design. lipgloss keeps the profile in a
// process-global renderer, so no test here may run in parallel.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

// now is the instant every countdown in the sample result is measured from.
var now = time.Date(2026, 9, 16, 14, 22, 7, 0, time.UTC)

func opts(banner bool) layout.Options {
	return layout.Options{Banner: banner, Now: now}
}

// sample is the run the approved mockups were drawn from: eight windows
// across three providers, one of them rate limited, plus one failed
// provider and one that was never detected.
func sample() usage.Result {
	in := func(d time.Duration) time.Time { return now.Add(d) }
	return usage.Result{
		Windows: []provider.Window{
			{Provider: "claude", Name: "5h", Plan: "max", RemainingPercent: 68.0, ResetsAt: in(3*time.Hour + 38*time.Minute)},
			{Provider: "claude", Name: "weekly", Plan: "max", RemainingPercent: 65.0, ResetsAt: in(4*24*time.Hour + 17*time.Hour)},
			{Provider: "claude", Name: "fable weekly", Plan: "max", RemainingPercent: 65.0, ResetsAt: in(4*24*time.Hour + 17*time.Hour)},
			{Provider: "codex", Name: "weekly", Plan: "prolite", RemainingPercent: 99.0, ResetsAt: in(6*24*time.Hour + 9*time.Hour)},
			{Provider: "codex", Name: "GPT-5.3-Codex-Spark primary", Plan: "prolite", RemainingPercent: 100.0, ResetsAt: in(4*time.Hour + 59*time.Minute)},
			{Provider: "codex", Name: "GPT-5.3-Codex-Spark secondary", Plan: "prolite", RemainingPercent: 12.0, ResetsAt: in(6*24*time.Hour + 23*time.Hour), RateLimited: true},
			{Provider: "cursor", Name: "total", Plan: "free", RemainingPercent: 95.5, ResetsAt: in(6*24*time.Hour + 21*time.Hour)},
			{Provider: "cursor", Name: "auto", Plan: "free", RemainingPercent: 91.0},
		},
		Errors:     []usage.ProviderError{{Provider: "cursor", Message: "token expired, open cursor-agent to refresh"}},
		Undetected: []usage.ProviderError{{Provider: "opencode-go", Message: "not logged in, run opencode to log in"}},
	}
}

func goldenLines(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading golden: %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// TestRenderMatchesTheGoldenPages is the acceptance test: the five approved
// pages, rendered from the sample run.
//
// The goldens are right-trimmed, which is how the design mockups are
// stored, so they stay diffable against them; the padding to the full width
// that Render adds is checked separately, by
// TestEveryLineIsExactlyWidthCells.
func TestRenderMatchesTheGoldenPages(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		banner bool
		golden string
	}{
		{"80 wide with the banner", 80, true, "80-banner.txt"},
		{"80 wide without the banner", 80, false, "80-nobanner.txt"},
		{"100 wide", 100, true, "100.txt"},
		{"120 wide", 120, true, "120.txt"},
		{"40 wide, one column", 40, true, "40.txt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := layout.Render(sample(), tc.width, opts(tc.banner))
			want := goldenLines(t, tc.golden)
			for i := range got {
				got[i] = strings.TrimRight(got[i], " ")
			}
			if len(got) != len(want) {
				t.Errorf("rendered %d lines, want %d", len(got), len(want))
			}
			for i := 0; i < len(got) && i < len(want); i++ {
				if got[i] != want[i] {
					t.Errorf("line %d:\ngot  %q\nwant %q", i+1, got[i], want[i])
				}
			}
			for i := len(want); i < len(got); i++ {
				t.Errorf("extra line %d: %q", i+1, got[i])
			}
			for i := len(got); i < len(want); i++ {
				t.Errorf("missing line %d: %q", i+1, want[i])
			}
		})
	}
}

func TestEveryLineIsExactlyWidthCells(t *testing.T) {
	results := map[string]usage.Result{"the sample run": sample(), "an empty run": {}}
	for name, res := range results {
		for width := 30; width <= 130; width++ {
			for _, banner := range []bool{true, false} {
				for i, line := range layout.Render(res, width, opts(banner)) {
					if w := runewidth.StringWidth(line); w != width {
						t.Fatalf("%s at width %d (banner=%v): line %d is %d cells: %q",
							name, width, banner, i+1, w, line)
					}
				}
			}
		}
	}
}

func TestTwoColumnsFromEightyOneColumnBelow(t *testing.T) {
	// At 80 the two top sections share a line; at the 36-cell floor they
	// cannot, so every section owns its own lines.
	wide := layout.Render(sample(), 80, opts(false))
	if !hasLineWith(wide, "◆ claude", "● codex") {
		t.Error("at width 80 claude and codex do not share a header line")
	}
	narrow := layout.Render(sample(), 36, opts(false))
	if hasLineWith(narrow, "◆ claude", "● codex") {
		t.Error("at width 36 claude and codex share a header line, want one column")
	}
	if !hasLineWith(narrow, "◆ claude") || !hasLineWith(narrow, "● codex") {
		t.Error("at width 36 a section is missing")
	}
}

func TestBelowTheFloorSaysSo(t *testing.T) {
	got := layout.Render(sample(), 35, opts(true))
	if len(got) != 1 {
		t.Fatalf("rendered %d lines at width 35, want 1: %q", len(got), got)
	}
	if strings.TrimRight(got[0], " ") != "terminal too narrow" {
		t.Errorf("got %q, want %q", got[0], "terminal too narrow")
	}
}

func TestLongWindowNameIsMiddleTruncated(t *testing.T) {
	res := sample()
	const long = "GPT-5.3-Codex-Spark secondary with an unreasonably long name"
	res.Windows[5].Name = long

	// At the 36-cell floor the name has 34 cells to live in.
	got := layout.Render(res, 36, opts(false))
	line := ""
	for _, l := range got {
		if strings.HasPrefix(l, "▸ ") && strings.Contains(l, "…") {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatalf("no name line was truncated:\n%s", strings.Join(got, "\n"))
	}
	name := strings.TrimRight(strings.TrimPrefix(line, "▸ "), " ")
	if runewidth.StringWidth(name) != 34 {
		t.Errorf("truncated name is %d cells, want 34: %q", runewidth.StringWidth(name), name)
	}
	if !strings.HasPrefix(name, long[:8]) || !strings.HasSuffix(name, long[len(long)-8:]) {
		t.Errorf("middle truncation kept the wrong ends: %q", name)
	}

	// The sample's own 29-character name still fits a 36-cell column whole.
	whole := layout.Render(sample(), 36, opts(false))
	if !hasLineWith(whole, "▸ GPT-5.3-Codex-Spark secondary") {
		t.Error("a 29-character name was truncated in a 36-cell column")
	}
}

func TestRateLimitedBadgeSitsAtTheColumnEdge(t *testing.T) {
	// One column, so the column edge is the page edge.
	got := layout.Render(sample(), 40, opts(false))
	line := findLine(t, got, "[RL]")
	if !strings.HasSuffix(line, "[RL]") {
		t.Errorf("the badge is not flush right: %q", line)
	}
	if runewidth.StringWidth(line) != 40 {
		t.Errorf("badge line is %d cells, want 40: %q", runewidth.StringWidth(line), line)
	}
	if !strings.Contains(line, "╮") {
		t.Errorf("the badge is not on the bezel row: %q", line)
	}
	if count := countLinesWith(got, "[RL]"); count != 1 {
		t.Errorf("%d lines carry a badge, want 1 (only the rate-limited window)", count)
	}
}

func TestBlankLineAfterTheBannerOnlyFromOneTwenty(t *testing.T) {
	for _, tc := range []struct {
		width int
		blank bool
	}{{80, false}, {100, false}, {119, false}, {120, true}, {140, true}} {
		got := layout.Render(sample(), tc.width, opts(true))
		if isBlank := strings.TrimSpace(got[6]) == ""; isBlank != tc.blank {
			t.Errorf("width %d: blank line after the banner = %v, want %v (line 7 is %q)",
				tc.width, isBlank, tc.blank, got[6])
		}
	}
}

func TestBlankLineBetweenSectionRowsOnlyFromOneHundred(t *testing.T) {
	for _, tc := range []struct {
		width int
		blank bool
	}{{40, false}, {80, false}, {99, false}, {100, true}, {120, true}} {
		got := layout.Render(sample(), tc.width, opts(false))
		i := indexOfLineWith(got, "○ opencode-go")
		if i <= 0 {
			t.Fatalf("width %d: no opencode-go section header", tc.width)
		}
		if isBlank := strings.TrimSpace(got[i-1]) == ""; isBlank != tc.blank {
			t.Errorf("width %d: blank line before the second section row = %v, want %v (line %d is %q)",
				tc.width, isBlank, tc.blank, i, got[i-1])
		}
	}
}

func TestSectionsFollowRegistryOrderRowMajor(t *testing.T) {
	got := layout.Render(sample(), 80, opts(false))
	first := findLine(t, got, "◆ claude")
	if !strings.Contains(first, "● codex") {
		t.Errorf("the first section row is not claude then codex: %q", first)
	}
	second := findLine(t, got, "○ opencode-go")
	if !strings.HasPrefix(strings.TrimLeft(second, " "), "─ ○ opencode-go") {
		t.Errorf("opencode-go is not the left section of the second row: %q", second)
	}
	if !strings.Contains(second, "▲ cursor") {
		t.Errorf("cursor is not the right section of the second row: %q", second)
	}
	if strings.Index(second, "○ opencode-go") > strings.Index(second, "▲ cursor") {
		t.Errorf("cursor is drawn left of opencode-go: %q", second)
	}
}

func TestSummaryLineReplacesTheBannerWhenAskedOrTooNarrow(t *testing.T) {
	const want = "qmeter · 8 windows · 1 rate limited · 1 error · 1 not detected"
	got := layout.Render(sample(), 80, opts(false))
	if line := strings.TrimRight(got[0], " "); line != want {
		t.Errorf("banner off: got %q, want %q", line, want)
	}
	// Below the banner's own width the banner is dropped, asked for or not.
	narrow := layout.Render(sample(), 39, opts(true))
	if line := strings.TrimRight(narrow[0], " "); !strings.HasPrefix(line, "qmeter · 8 windows") {
		t.Errorf("width 39 with the banner on: got %q, want the summary line", line)
	}
	if wide := layout.Render(sample(), 40, opts(true)); strings.Contains(wide[0], "qmeter ·") {
		t.Errorf("width 40 with the banner on: got %q, want the banner", wide[0])
	}
}

func TestSummaryLineOmitsZeroCounts(t *testing.T) {
	res := usage.Result{Windows: []provider.Window{
		{Provider: "claude", Name: "5h", Plan: "max", RemainingPercent: 50},
	}}
	got := layout.Render(res, 80, opts(false))
	if line := strings.TrimRight(got[0], " "); line != "qmeter · 1 window" {
		t.Errorf("got %q, want %q", line, "qmeter · 1 window")
	}
}

func TestEmptyResultSaysNoProvidersDetected(t *testing.T) {
	got := layout.Render(usage.Result{}, 80, opts(false))
	if len(got) != 2 {
		t.Fatalf("rendered %d lines, want the header and one message: %q", len(got), got)
	}
	if line := strings.TrimRight(got[0], " "); line != "qmeter" {
		t.Errorf("header = %q, want %q", line, "qmeter")
	}
	if line := strings.TrimRight(got[1], " "); line != "no providers detected" {
		t.Errorf("body = %q, want %q", line, "no providers detected")
	}
	withBanner := layout.Render(usage.Result{}, 80, opts(true))
	if len(withBanner) != 7 {
		t.Fatalf("with the banner: rendered %d lines, want 7", len(withBanner))
	}
	if line := strings.TrimRight(withBanner[6], " "); line != "no providers detected" {
		t.Errorf("with the banner: last line = %q, want %q", line, "no providers detected")
	}
}

func TestProviderMissingFromTheResultIsNotDrawn(t *testing.T) {
	res := usage.Result{Windows: []provider.Window{
		{Provider: "claude", Name: "5h", Plan: "max", RemainingPercent: 50, ResetsAt: now.Add(time.Hour)},
	}}
	got := strings.Join(layout.Render(res, 100, opts(false)), "\n")
	for _, absent := range []string{"codex", "cursor", "opencode-go"} {
		if strings.Contains(got, absent) {
			t.Errorf("%s is drawn although it is nowhere in the result:\n%s", absent, got)
		}
	}
}

func TestProviderOutsideTheRegistryIsStillDrawn(t *testing.T) {
	res := usage.Result{Windows: []provider.Window{
		{Provider: "claude", Name: "5h", Plan: "max", RemainingPercent: 50, ResetsAt: now.Add(time.Hour)},
		{Provider: "newcomer", Name: "monthly", Plan: "pro", RemainingPercent: 10, ResetsAt: now.Add(time.Hour)},
	}}
	got := layout.Render(res, 80, opts(false))
	if !hasLineWith(got, "newcomer") {
		t.Fatalf("a provider the layout has no icon for was dropped:\n%s", strings.Join(got, "\n"))
	}
	row := findLine(t, got, "claude")
	if strings.Index(row, "claude") > strings.Index(row, "newcomer") {
		t.Errorf("the unknown provider was drawn before the registry's own: %q", row)
	}
}

func TestZeroNowFallsBackToTheClock(t *testing.T) {
	res := usage.Result{Windows: []provider.Window{
		{Provider: "claude", Name: "5h", Plan: "max", RemainingPercent: 50, ResetsAt: time.Now().Add(2*time.Hour + 30*time.Minute)},
	}}
	got := layout.Render(res, 80, layout.Options{})
	if !hasLineWith(got, "2h29m") && !hasLineWith(got, "2h30m") {
		t.Errorf("a zero Options.Now did not count down from time.Now():\n%s", strings.Join(got, "\n"))
	}
}

// TestCountdownCell drives the countdown field through Render: the largest
// two non-zero units, a trailing zero unit dropped, an elapsed window that
// is due now, and no reset time at all. The field is six cells wide, so a
// countdown too long for it is cut with an ellipsis rather than pushing the
// line out of true.
func TestCountdownCell(t *testing.T) {
	tests := []struct {
		name   string
		resets time.Duration
		zero   bool
		want   string
	}{
		{"exactly a day drops the zero hours", 24 * time.Hour, false, "1d"},
		{"twelve days", 12 * 24 * time.Hour, false, "12d"},
		{"exactly three hours drops the zero minutes", 3 * time.Hour, false, "3h"},
		{"a day and two hours", 26 * time.Hour, false, "1d2h"},
		{"two hours and five minutes", 2*time.Hour + 5*time.Minute, false, "2h5m"},
		{"already elapsed", -5 * time.Minute, false, "0m"},
		{"under a minute away", 30 * time.Second, false, "0m"},
		{"no reset time at all", 0, true, "-"},
		{"longer than the field", 1000*24*time.Hour + 5*time.Hour, false, "1000d…"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resets := now.Add(tc.resets)
			if tc.zero {
				resets = time.Time{}
			}
			if got := countdownCell(t, resets); got != tc.want {
				t.Errorf("countdown = %q, want %q", got, tc.want)
			}
		})
	}
}

// countdownCell renders one window at the 36-cell floor — where the column
// is the whole page, so the countdown is the last six cells of the track
// row — and returns that field with its padding trimmed. It fails the test
// if the row is not exactly the page width, which is what a countdown too
// long for its field would cost.
func countdownCell(t *testing.T, resets time.Time) string {
	t.Helper()
	res := usage.Result{Windows: []provider.Window{
		{Provider: "claude", Name: "5h", Plan: "max", RemainingPercent: 68, ResetsAt: resets},
	}}
	for _, line := range layout.Render(res, layout.MinColumn, opts(false)) {
		if !strings.Contains(line, "┴") || !strings.Contains(line, "%") {
			continue
		}
		if w := runewidth.StringWidth(line); w != layout.MinColumn {
			t.Fatalf("the track row is %d cells, want %d: %q", w, layout.MinColumn, line)
		}
		cells := []rune(line)
		return strings.TrimRight(string(cells[len(cells)-6:]), " ")
	}
	t.Fatalf("no track row was rendered for a window resetting at %v", resets)
	return ""
}

// --- helpers ---------------------------------------------------------------

func hasLineWith(lines []string, subs ...string) bool {
	for _, line := range lines {
		all := true
		for _, s := range subs {
			if !strings.Contains(line, s) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

func indexOfLineWith(lines []string, sub string) int {
	for i, line := range lines {
		if strings.Contains(line, sub) {
			return i
		}
	}
	return -1
}

func countLinesWith(lines []string, sub string) int {
	n := 0
	for _, line := range lines {
		if strings.Contains(line, sub) {
			n++
		}
	}
	return n
}

func findLine(t *testing.T, lines []string, sub string) string {
	t.Helper()
	i := indexOfLineWith(lines, sub)
	if i < 0 {
		t.Fatalf("no line contains %q:\n%s", sub, strings.Join(lines, "\n"))
	}
	return lines[i]
}
