package layout_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/dash/banner"
	"github.com/Harrison-Blair/qmeter/internal/dash/gauge"
	"github.com/Harrison-Blair/qmeter/internal/dash/layout"
	"github.com/Harrison-Blair/qmeter/internal/dash/theme"
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
	// Default 50-cell meters pack two columns only once each can retain its
	// 40-cell responsive floor.
	wide := layout.Render(sample(), 110, opts(false))
	if !hasLineWith(wide, "◆ claude", "● codex") {
		t.Error("at width 110 claude and codex do not share a header line")
	}
	narrow := layout.Render(sample(), 109, opts(false))
	if hasLineWith(narrow, "◆ claude", "● codex") {
		t.Error("at width 109 claude and codex share a header line, want one column")
	}
	if !hasLineWith(narrow, "◆ claude") || !hasLineWith(narrow, "● codex") {
		t.Error("at width 109 a section is missing")
	}
}

func TestResponsiveMeterWidthBoundaries(t *testing.T) {
	tests := []struct {
		width      int
		columns    int
		gaugeWidth int
	}{
		{35, 0, 0},
		{36, 1, 22},
		{53, 1, 39},
		{54, 1, 40},
		{64, 1, 50},
		{65, 1, 50},
		{109, 1, 50},
		{110, 2, 40},
		{119, 2, 44},
		{120, 2, 44},
		{131, 2, 49},
		{132, 2, 50},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("width_%d", tc.width), func(t *testing.T) {
			got := layout.Render(sample(), tc.width, opts(false))
			if tc.columns == 0 {
				if line := strings.TrimSpace(got[0]); line != "terminal too narrow" {
					t.Fatalf("line = %q, want terminal too narrow", line)
				}
				return
			}
			first := findLine(t, got, "◆ claude")
			hasTwo := strings.Contains(first, "● codex")
			if (tc.columns == 2) != hasTwo {
				t.Errorf("columns at width %d = %d, want %d", tc.width, map[bool]int{false: 1, true: 2}[hasTwo], tc.columns)
			}
			widths := renderedGaugeWidths(got)
			if len(widths) == 0 || widths[0] != tc.gaugeWidth {
				t.Fatalf("gauge widths = %v, want first width %d", widths, tc.gaugeWidth)
			}
			if tc.columns == 2 {
				for _, width := range widths {
					if width < 40 {
						t.Fatalf("packed gauge width = %d, want at least 40", width)
					}
				}
			}
		})
	}
}

func TestOneProviderAlwaysUsesTheFullColumn(t *testing.T) {
	res := usage.Result{Windows: []provider.Window{
		{Provider: "claude", Name: "5h", Plan: "max", RemainingPercent: 68},
	}}
	for _, width := range []int{80, 110} {
		got := renderedGaugeWidths(layout.Render(res, width, opts(false)))
		if len(got) != 1 || got[0] != 50 {
			t.Errorf("width %d: gauge widths = %v, want [50]", width, got)
		}
	}
}

func TestCustomMeterTargetsUseTheirResponsivePackedFloor(t *testing.T) {
	tests := []struct {
		name      string
		target    int
		pageWidth int
		wantGauge int
	}{
		{"minimum endpoint", 22, 74, 22},
		{"forty", 40, 110, 40},
		{"seventy five growing", 75, 152, 60},
		{"seventy five reached", 75, 182, 75},
		{"maximum endpoint growing", 200, 352, 160},
		{"maximum endpoint reached", 200, 432, 200},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := opts(false)
			o.MeterWidth = tc.target
			got := renderedGaugeWidths(layout.Render(sample(), tc.pageWidth, o))
			if len(got) == 0 || got[0] != tc.wantGauge {
				t.Fatalf("gauge widths = %v, want first %d", got, tc.wantGauge)
			}
		})
	}
}

func TestCappedWindowBlockIsCenteredAsAUnit(t *testing.T) {
	res := usage.Result{Windows: []provider.Window{
		{Provider: "claude", Name: "5h", Plan: "max", RemainingPercent: 68, RateLimited: true},
	}}
	got := layout.Render(res, 100, opts(false))
	title := findLine(t, got, "▸ 5h")
	bezel := findLine(t, got, "[RL]")
	track := findLine(t, got, "68.0%")
	scale := findLine(t, got, "100")

	// A 50-cell gauge plus 14 cells of furniture is a 64-cell block. The
	// 36 spare cells split evenly inside this 100-cell provider column.
	if col := runeColumn(title, "▸"); col != 18 {
		t.Errorf("title starts at column %d, want 18", col)
	}
	if col := runeColumn(bezel, "╭"); col != 25 {
		t.Errorf("bezel starts at column %d, want 25", col)
	}
	if col := runeColumn(track, "6"); col != 19 {
		t.Errorf("percentage starts at column %d, want 19", col)
	}
	if col := runeColumn(scale, "0"); col != 26 {
		t.Errorf("scale starts at column %d, want 26", col)
	}
	if col := runeColumn(bezel, "[RL]"); col != 78 {
		t.Errorf("badge starts at column %d, want 78", col)
	}

	odd := layout.Render(res, 101, opts(false))
	oddTitle := findLine(t, odd, "▸ 5h")
	oddBadge := findLine(t, odd, "[RL]")
	if col := runeColumn(oddTitle, "▸"); col != 18 {
		t.Errorf("odd-slack title starts at column %d, want 18", col)
	}
	if trailing := len([]rune(oddBadge)) - len([]rune(strings.TrimRight(oddBadge, " "))); trailing != 19 {
		t.Errorf("odd-slack row has %d trailing cells, want 19 (the extra cell on the right)", trailing)
	}
}

func TestOddFinalProviderKeepsTheGridColumnGeometry(t *testing.T) {
	res := usage.Result{Windows: []provider.Window{
		{Provider: "claude", Name: "5h", RemainingPercent: 50},
		{Provider: "codex", Name: "weekly", RemainingPercent: 50},
		{Provider: "cursor", Name: "monthly", RemainingPercent: 50},
	}}
	got := renderedGaugeWidths(layout.Render(res, 110, opts(false)))
	if len(got) != 3 {
		t.Fatalf("gauge widths = %v, want three", got)
	}
	for i, width := range got {
		if width != 40 {
			t.Errorf("gauge %d width = %d, want 40", i, width)
		}
	}
}

func TestProviderThemeColorsIdentityPlanAndBannerButNotHealth(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	th := theme.Theme{
		Claude:     lipgloss.AdaptiveColor{Light: "#101011", Dark: "#202021"},
		Codex:      lipgloss.AdaptiveColor{Light: "#303031", Dark: "#404041"},
		OpenCodeGo: lipgloss.AdaptiveColor{Light: "#505051", Dark: "#606061"},
		Cursor:     lipgloss.AdaptiveColor{Light: "#707071", Dark: "#808081"},
	}
	o := opts(true)
	o.Theme = th

	for _, tc := range []struct {
		name string
		dark bool
	}{
		{"light background", false},
		{"dark background", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lipgloss.SetHasDarkBackground(tc.dark)
			got := layout.Render(sample(), 132, o)
			for i, art := range banner.Rows() {
				want := expectedBannerRow(art, th)
				if strings.TrimRight(got[i], " ") != want {
					t.Errorf("banner row %d:\ngot  %q\nwant %q", i, strings.TrimRight(got[i], " "), want)
				}
			}

			page := strings.Join(got, "\n")
			accent := th.Accent("claude")
			for _, want := range []string{
				lipgloss.NewStyle().Foreground(accent).Faint(true).Render("─ "),
				lipgloss.NewStyle().Foreground(accent).Bold(true).Render("◆ "),
				lipgloss.NewStyle().Foreground(accent).Bold(true).Render("claude"),
				lipgloss.NewStyle().Foreground(accent).Bold(true).Render("▸ "),
				lipgloss.NewStyle().Foreground(accent).Render("max"),
				lipgloss.NewStyle().Foreground(gauge.Band(68, false)).Bold(true).Render("68.0%"),
			} {
				if !strings.Contains(page, want) {
					t.Errorf("page does not contain styled fragment %q", want)
				}
			}
		})
	}
}

func TestUnknownProviderUsesNeutralAdaptiveIdentity(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(false)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	res := usage.Result{Windows: []provider.Window{
		{Provider: "future", Name: "monthly", Plan: "pro", RemainingPercent: 50},
	}}
	o := opts(false)
	o.Theme = theme.Theme{OpenCodeGo: lipgloss.AdaptiveColor{Light: "#010203", Dark: "#040506"}}
	page := strings.Join(layout.Render(res, 80, o), "\n")
	neutral := lipgloss.AdaptiveColor{Light: "#656363", Dark: "#B7B1B1"}
	for _, want := range []string{
		lipgloss.NewStyle().Foreground(neutral).Bold(true).Render("• "),
		lipgloss.NewStyle().Foreground(neutral).Render("pro"),
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page does not contain neutral fragment %q", want)
		}
	}
}

func TestNoColorProfileLeavesAConfiguredThemeUnstyled(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	o := opts(true)
	o.Theme = theme.Theme{
		Claude: lipgloss.AdaptiveColor{Light: "#010203", Dark: "#040506"},
	}
	got := strings.Join(layout.Render(sample(), 132, o), "\n")
	if strings.Contains(got, "\x1b") {
		t.Fatalf("ASCII/NO_COLOR render contains an escape sequence: %q", got)
	}
	if !strings.Contains(got, "◆ claude") || !strings.Contains(got, "68.0%") {
		t.Fatal("unstyled render lost dashboard content")
	}
}

func TestBannerBandsDoNotDependOnPresentOrFilteredProviders(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	o := opts(true)
	one := usage.Result{Windows: []provider.Window{{Provider: "claude", Name: "5h", RemainingPercent: 50}}}
	all := layout.Render(sample(), 132, o)[:banner.Height]
	filtered := layout.Render(one, 132, o)[:banner.Height]
	empty := layout.Render(usage.Result{}, 132, o)[:banner.Height]
	for i := 0; i < banner.Height; i++ {
		if filtered[i] != all[i] || empty[i] != all[i] {
			t.Errorf("banner row %d depends on result presence", i)
		}
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

	// At the 36-cell floor the name has 28 cells after reserving the badge.
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
	name := strings.TrimSuffix(strings.TrimRight(strings.TrimPrefix(line, "▸ "), " "), " [n/a]")
	if runewidth.StringWidth(name) != 28 {
		t.Errorf("truncated name is %d cells, want 28: %q", runewidth.StringWidth(name), name)
	}
	if !strings.HasPrefix(name, long[:8]) || !strings.HasSuffix(name, long[len(long)-8:]) {
		t.Errorf("middle truncation kept the wrong ends: %q", name)
	}

	// The sample's name and full badge fit without truncation at width 40.
	whole := layout.Render(sample(), 40, opts(false))
	if !hasLineWith(whole, "▸ GPT-5.3-Codex-Spark secondary [n/a]") {
		t.Error("name was truncated despite room for its badge")
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
	got := layout.Render(sample(), 110, opts(false))
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
	got := layout.Render(res, 110, opts(false))
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
	got := layout.Render(res, 110, opts(false))
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

func renderedGaugeWidths(lines []string) []int {
	var widths []int
	for _, line := range lines {
		runes := []rune(line)
		start := -1
		for i, r := range runes {
			if r != '┴' {
				continue
			}
			if start < 0 {
				start = i
				continue
			}
			widths = append(widths, i-start+1)
			start = -1
		}
	}
	return widths
}

func runeColumn(line, sub string) int {
	plain := []rune(line)
	needle := []rune(sub)
	for i := 0; i+len(needle) <= len(plain); i++ {
		if string(plain[i:i+len(needle)]) == sub {
			return i
		}
	}
	return -1
}

func expectedBannerRow(art string, th theme.Theme) string {
	colors := []lipgloss.TerminalColor{
		th.Accent("claude"),
		th.Accent("codex"),
		th.Accent("opencode-go"),
		th.Accent("cursor"),
	}
	runes := []rune(art)
	var out strings.Builder
	for i := 0; i < len(runes); {
		band := i / 10
		space := runes[i] == ' '
		j := i + 1
		for j < len(runes) && j/10 == band && (runes[j] == ' ') == space {
			j++
		}
		text := string(runes[i:j])
		if space {
			out.WriteString(text)
		} else {
			out.WriteString(lipgloss.NewStyle().Foreground(colors[band]).Render(text))
		}
		i = j
	}
	return out.String()
}

func findLine(t *testing.T, lines []string, sub string) string {
	t.Helper()
	i := indexOfLineWith(lines, sub)
	if i < 0 {
		t.Fatalf("no line contains %q:\n%s", sub, strings.Join(lines, "\n"))
	}
	return lines[i]
}

// TestPaceMarkerFollowsTheWindowPeriod: a window that knows its period gets
// a ▼ in its bezel row at the fraction of the period still ahead; one that
// does not is drawn exactly as before. At the 36-cell floor the track is
// 20 cells, so 3h38m of a 5h window (0.727) is track cell 14, page
// column 7 + 1 + 14 = 22.
func TestPaceMarkerFollowsTheWindowPeriod(t *testing.T) {
	window := func(period time.Duration) usage.Result {
		return usage.Result{Windows: []provider.Window{{
			Provider: "claude", Name: "5h", Plan: "max", RemainingPercent: 68,
			ResetsAt: now.Add(3*time.Hour + 38*time.Minute), Period: period,
		}}}
	}
	paced := layout.Render(window(5*time.Hour), layout.MinColumn, opts(false))
	scale := findLine(t, paced, "▼")
	if !strings.Contains(scale, "╭") || !strings.Contains(scale, "╮") {
		t.Errorf("the marker is not on the bezel row: %q", scale)
	}
	if cells := []rune(scale); cells[22] != '▼' {
		t.Errorf("marker at column %d, want 22: %q", strings.IndexRune(scale, '▼'), scale)
	}
	if n := countLinesWith(paced, "▼"); n != 1 {
		t.Errorf("%d lines carry a marker, want 1", n)
	}

	unpaced := layout.Render(window(0), layout.MinColumn, opts(false))
	if hasLineWith(unpaced, "▼") {
		t.Errorf("a window without a period has a pace marker:\n%s", strings.Join(unpaced, "\n"))
	}
}

// TestPaceMarkerIsClampedToTheTrack: a reset already due sits at 0, and a
// reset further away than the period (a provider's clock skew) at 100.
func TestPaceMarkerIsClampedToTheTrack(t *testing.T) {
	at := func(resets time.Time) []string {
		return layout.Render(usage.Result{Windows: []provider.Window{{
			Provider: "claude", Name: "5h", RemainingPercent: 68, ResetsAt: resets, Period: 5 * time.Hour,
		}}}, layout.MinColumn, opts(false))
	}
	if scale := findLine(t, at(now.Add(-time.Minute)), "▼"); []rune(scale)[8] != '▼' {
		t.Errorf("a due window's marker is not above the first cell: %q", scale)
	}
	if scale := findLine(t, at(now.Add(9*time.Hour)), "▼"); []rune(scale)[27] != '▼' {
		t.Errorf("a window resetting beyond its period is not above the last cell: %q", scale)
	}
}

// TestRateLimitedCountdownIsBoldRed: once a window is rate limited the
// countdown is the only number that matters, so it leaves the time colour
// for the health one. Every other countdown keeps its cyan.
func TestRateLimitedCountdownIsBoldRed(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	page := strings.Join(layout.Render(sample(), 40, opts(false)), "\n")
	wall := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	cyan := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	if want := wall.Render("6d23h"); !strings.Contains(page, want) {
		t.Errorf("the rate-limited countdown is not bold red: page lacks %q", want)
	}
	if want := cyan.Render("3h38m"); !strings.Contains(page, want) {
		t.Errorf("an ordinary countdown lost its cyan: page lacks %q", want)
	}
	if stray := cyan.Render("6d23h"); strings.Contains(page, stray) {
		t.Errorf("the rate-limited countdown is still cyan: page has %q", stray)
	}
}

func TestWindowPaceBadges(t *testing.T) {
	for _, tc := range []struct {
		name      string
		remaining float64
		reset     time.Time
		period    time.Duration
		want      string
	}{
		{"ahead", 30, now.Add(50 * time.Minute), 100 * time.Minute, "ahead"},
		{"behind", 70, now.Add(50 * time.Minute), 100 * time.Minute, "behind"},
		{"lower boundary", 45, now.Add(50 * time.Minute), 100 * time.Minute, "on pace"},
		{"upper boundary", 55, now.Add(50 * time.Minute), 100 * time.Minute, "on pace"},
		{"below boundary", 44.999, now.Add(50 * time.Minute), 100 * time.Minute, "ahead"},
		{"above boundary", 55.001, now.Add(50 * time.Minute), 100 * time.Minute, "behind"},
		{"no period", 50, now.Add(time.Hour), 0, "n/a"},
		{"no reset", 50, time.Time{}, time.Hour, "n/a"},
		{"expired", 50, now, time.Hour, "n/a"},
		{"future start", 50, now.Add(2 * time.Hour), time.Hour, "n/a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := usage.Result{Windows: []provider.Window{{Provider: "codex", Name: "weekly", RemainingPercent: tc.remaining, ResetsAt: tc.reset, Period: tc.period}}}
			page := layout.Render(r, 40, opts(false))
			if !hasLineWith(page, "▸ weekly ["+tc.want+"]") {
				t.Fatalf("badge missing: %s", strings.Join(page, "\n"))
			}
			if strings.Contains(strings.Join(page, ""), "\x1b") {
				t.Fatal("plain badge contains ANSI")
			}
		})
	}
}

func TestPaceBadgeColorsAndRateLimit(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	for _, tc := range []struct {
		remaining    float64
		period       time.Duration
		label, color string
	}{
		{70, 100 * time.Minute, "behind", "208"}, {50, 100 * time.Minute, "on pace", "10"}, {30, 100 * time.Minute, "ahead", "11"}, {30, 0, "n/a", "8"},
	} {
		r := usage.Result{Windows: []provider.Window{{Provider: "codex", Name: "weekly", RemainingPercent: tc.remaining, Period: tc.period, ResetsAt: now.Add(50 * time.Minute), RateLimited: true}}}
		page := strings.Join(layout.Render(r, 40, opts(false)), "\n")
		want := lipgloss.NewStyle().Foreground(lipgloss.Color(tc.color)).Bold(true).Render("[" + tc.label + "]")
		if !strings.Contains(page, want) {
			t.Errorf("missing styled badge %q in %q", want, page)
		}
		if !strings.Contains(page, "[RL]") {
			t.Error("lost rate limit badge")
		}
		if tc.period > 0 && !strings.Contains(page, lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true).Render("▼")) {
			t.Error("lost cyan marker")
		}
	}
}

func TestLongWindowNamesPreserveCompletePaceBadge(t *testing.T) {
	for _, width := range []int{36, 40, 80, 100, 120} {
		for _, name := range []string{strings.Repeat("long-name", 20), strings.Repeat("日本語", 30)} {
			r := usage.Result{Windows: []provider.Window{
				{Provider: "claude", Name: name, RemainingPercent: 50, Period: time.Hour, ResetsAt: now.Add(30 * time.Minute)},
				{Provider: "codex", Name: name, RemainingPercent: 50, Period: time.Hour, ResetsAt: now.Add(30 * time.Minute)},
			}}
			lines := layout.Render(r, width, opts(false))
			count := 0
			for _, line := range lines {
				if runewidth.StringWidth(line) != width {
					t.Errorf("width %d got %d: %q", width, runewidth.StringWidth(line), line)
				}
				count += strings.Count(line, " [on pace]")
			}
			if count != 2 {
				t.Errorf("width %d complete badges %d: %s", width, count, strings.Join(lines, "\n"))
			}
		}
	}
}

func TestVerticalStretchesMetersInOneColumn(t *testing.T) {
	for _, width := range []int{35, 36, 110, 132, 260} {
		for _, target := range []int{0, 22, 75, 200} {
			for _, showBanner := range []bool{false, true} {
				t.Run(fmt.Sprintf("width_%d_target_%d_banner_%v", width, target, showBanner), func(t *testing.T) {
					o := opts(showBanner)
					o.Vertical, o.MeterWidth = true, target
					lines := layout.Render(sample(), width, o)
					for _, line := range lines {
						if runewidth.StringWidth(line) != width {
							t.Fatalf("line width != %d: %q", width, line)
						}
					}
					if width == 35 {
						if len(lines) != 1 || strings.TrimSpace(lines[0]) != "terminal too narrow" {
							t.Fatalf("narrow output = %q", lines)
						}
						return
					}
					widths := renderedGaugeWidths(lines)
					if len(widths) != 8 {
						t.Fatalf("gauge count = %d, want 8", len(widths))
					}
					for _, got := range widths {
						if got != width-14 {
							t.Fatalf("gauge width = %d, want %d", got, width-14)
						}
					}
					next := 0
					headers := []string{"◆ claude", "● codex", "○ opencode-go", "▲ cursor"}
					for _, line := range lines {
						count := 0
						for _, header := range headers {
							if strings.Contains(line, header) {
								count++
								if next >= len(headers) || header != headers[next] {
									t.Fatalf("provider order at line %q", line)
								}
								next++
							}
						}
						if count > 1 {
							t.Fatalf("providers share header: %q", line)
						}
					}
					if next != len(headers) {
						t.Fatalf("provider headers = %d, want 4", next)
					}
				})
			}
		}
	}
}
