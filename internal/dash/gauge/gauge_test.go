package gauge_test

import (
	"os"
	"strings"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/dash/gauge"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

// TestMain pins the colour profile to Ascii so every geometry test compares
// bare text. lipgloss keeps the profile in a process-global renderer, so no
// test in this package may run in parallel with another.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

func mustRender(t *testing.T, pct float64, width int, rl bool) gauge.Block {
	t.Helper()
	b, err := gauge.Render(pct, width, rl, gauge.NoPace)
	if err != nil {
		t.Fatalf("Render(%v, %d, %v) returned error: %v", pct, width, rl, err)
	}
	return b
}

// TestRenderMatchesTheDesignGeometry pins the three rows against the
// approved mockups: the gauges lifted out of design-u21's 80x24, 100x30,
// 120x40 and 40x12 pages.
func TestRenderMatchesTheDesignGeometry(t *testing.T) {
	tests := []struct {
		name  string
		pct   float64
		width int
		rl    bool
		want  gauge.Block
	}{
		{
			name:  "the 80-wide page: a 23-cell track",
			pct:   68.0,
			width: 25,
			rl:    false,
			want: gauge.Block{
				Bezel: "╭┬────┬─────┬────┬─────┬╮",
				Track: "┴▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▲▱▱▱▱▱▱▱┴",
				Scale: " 0         50        100 ",
			},
		},
		{
			name:  "the 100-wide page: a 33-cell track",
			pct:   68.0,
			width: 35,
			rl:    false,
			want: gauge.Block{
				Bezel: "╭┬───────┬───────┬───────┬───────┬╮",
				Track: "┴▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▲▱▱▱▱▱▱▱▱▱▱┴",
				Scale: " 0              50             100 ",
			},
		},
		{
			name:  "the 120-wide page, rate limited: a 42-cell track",
			pct:   12.0,
			width: 44,
			rl:    true,
			want: gauge.Block{
				Bezel: "╭┬─────────┬─────────┬─────────┬──────────┬╮",
				Track: "┴▰▰▰▰▰▲▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱┴",
				Scale: " 0                  50                  100 ",
			},
		},
		{
			name:  "the 40-wide page: a 24-cell track",
			pct:   68.0,
			width: 26,
			rl:    false,
			want: gauge.Block{
				Bezel: "╭┬────┬─────┬─────┬─────┬╮",
				Track: "┴▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▲▱▱▱▱▱▱▱┴",
				Scale: " 0         50         100 ",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mustRender(t, tc.pct, tc.width, tc.rl)
			if got.Bezel != tc.want.Bezel {
				t.Errorf("Bezel:\ngot  %q\nwant %q", got.Bezel, tc.want.Bezel)
			}
			if got.Track != tc.want.Track {
				t.Errorf("Track:\ngot  %q\nwant %q", got.Track, tc.want.Track)
			}
			if got.Scale != tc.want.Scale {
				t.Errorf("Scale:\ngot  %q\nwant %q", got.Scale, tc.want.Scale)
			}
		})
	}
}

func TestEveryRowIsExactlyWidthCells(t *testing.T) {
	for width := gauge.MinWidth; width <= 60; width++ {
		b := mustRender(t, 43.5, width, false)
		for _, row := range []struct {
			name, text string
		}{{"bezel", b.Bezel}, {"track", b.Track}, {"scale", b.Scale}} {
			if w := runewidth.StringWidth(row.text); w != width {
				t.Errorf("width %d: %s is %d cells: %q", width, row.name, w, row.text)
			}
		}
	}
}

func TestNeedleSitsAtTheEndsAtZeroAndFull(t *testing.T) {
	const width = 25 // 23 track cells
	empty := mustRender(t, 0, width, false)
	if want := "┴▲" + strings.Repeat("▱", 22) + "┴"; empty.Track != want {
		t.Errorf("0%%:\ngot  %q\nwant %q", empty.Track, want)
	}
	full := mustRender(t, 100, width, false)
	if want := "┴" + strings.Repeat("▰", 22) + "▲┴"; full.Track != want {
		t.Errorf("100%%:\ngot  %q\nwant %q", full.Track, want)
	}
}

func TestPercentIsClampedToTheTrack(t *testing.T) {
	const width = 25
	if got, want := mustRender(t, -12.5, width, false), mustRender(t, 0, width, false); got != want {
		t.Errorf("-12.5%% rendered %q, want the 0%% track %q", got.Track, want.Track)
	}
	if got, want := mustRender(t, 150, width, false), mustRender(t, 100, width, false); got != want {
		t.Errorf("150%% rendered %q, want the 100%% track %q", got.Track, want.Track)
	}
}

func TestRenderRefusesAGaugeUnderTheTwentyCellFloor(t *testing.T) {
	if gauge.MinWidth != 22 {
		t.Errorf("MinWidth = %d, want 22 (a 20-cell track plus the two caps)", gauge.MinWidth)
	}
	if _, err := gauge.Render(50, gauge.MinWidth, false, gauge.NoPace); err != nil {
		t.Errorf("Render at MinWidth returned error: %v", err)
	}
	b, err := gauge.Render(50, gauge.MinWidth-1, false, gauge.NoPace)
	if err == nil {
		t.Fatalf("Render at %d cells returned no error, want one", gauge.MinWidth-1)
	}
	if b != (gauge.Block{}) {
		t.Errorf("Render returned %+v alongside its error, want the zero Block", b)
	}
	if !strings.Contains(err.Error(), "21") {
		t.Errorf("error %q does not name the offending width", err)
	}
}

func TestBandFollowsTheRemainingThresholds(t *testing.T) {
	const (
		brightGreen = lipgloss.Color("10")
		yellow      = lipgloss.Color("3")
		orange      = lipgloss.Color("208")
		red         = lipgloss.Color("1")
	)
	tests := []struct {
		pct  float64
		rl   bool
		want lipgloss.Color
	}{
		{100, false, brightGreen},
		{75, false, brightGreen},
		{74.9, false, yellow},
		{50, false, yellow},
		{49.9, false, orange},
		{25, false, orange},
		{24.9, false, red},
		{0, false, red},
		{90, true, red}, // a rate-limited window is red however much is left
		{100, true, red},
	}
	for _, tc := range tests {
		if got := gauge.Band(tc.pct, tc.rl); got != tc.want {
			t.Errorf("Band(%v, %v) = %q, want %q", tc.pct, tc.rl, got, tc.want)
		}
	}
}

// TestRenderColoursEveryPartOfTheTrack checks the styling against styles the
// test builds itself from the documented palette: bright black frame, one
// band colour for the whole fill, a bright white needle, dim red for what is
// spent.
func TestRenderColoursEveryPartOfTheTrack(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	const width = 25 // 23 cells; 90% puts the needle at cell 20
	b, err := gauge.Render(90, width, false, gauge.NoPace)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	frame := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	fill := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	needle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	spent := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Faint(true)

	wantTrack := frame.Render("┴") + fill.Render(strings.Repeat("▰", 20)) +
		needle.Render("▲") + spent.Render(strings.Repeat("▱", 2)) + frame.Render("┴")
	if b.Track != wantTrack {
		t.Errorf("track:\ngot  %q\nwant %q", b.Track, wantTrack)
	}
	if want := frame.Render("╭┬────┬─────┬────┬─────┬╮"); b.Bezel != want {
		t.Errorf("bezel:\ngot  %q\nwant %q", b.Bezel, want)
	}
	if want := frame.Render(" 0         50        100 "); b.Scale != want {
		t.Errorf("scale:\ngot  %q\nwant %q", b.Scale, want)
	}
}

func TestRateLimitedFillIsRed(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	b, err := gauge.Render(90, 25, true, gauge.NoPace)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	want := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(strings.Repeat("▰", 20))
	if !strings.Contains(b.Track, want) {
		t.Errorf("rate-limited track %q does not carry the red fill %q", b.Track, want)
	}
}

func TestPlainStripsTheStyling(t *testing.T) {
	bare := mustRender(t, 68, 25, false) // Ascii profile: already unstyled

	lipgloss.SetColorProfile(termenv.ANSI256)
	styled, err := gauge.Render(68, 25, false, gauge.NoPace)
	lipgloss.SetColorProfile(termenv.Ascii)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if styled == bare {
		t.Fatal("the 256-colour render carries no styling, so Plain proves nothing")
	}
	if got := gauge.Plain(styled); got != bare {
		t.Errorf("Plain:\ngot  %+v\nwant %+v", got, bare)
	}
}

// mustRenderPace is mustRender with a pace marker.
func mustRenderPace(t *testing.T, pct float64, width int, rl bool, pace float64) gauge.Block {
	t.Helper()
	b, err := gauge.Render(pct, width, rl, pace)
	if err != nil {
		t.Fatalf("Render(%v, %d, %v, %v) returned error: %v", pct, width, rl, pace, err)
	}
	return b
}

// TestPaceMarkerSitsUnderTheEvenSpendCell pins the marker against the
// design: ▴ in the scale row, under the track cell the needle would occupy
// if the window were being spent evenly, which is the fraction of the
// window still ahead. The bezel and track are untouched by it.
func TestPaceMarkerSitsUnderTheEvenSpendCell(t *testing.T) {
	const width = 25 // 23 track cells
	tests := []struct {
		name string
		pace float64
		want string
	}{
		{"3h38m left of a 5h window", 218.0 / 300, " 0         50    ▴   100 "},
		{"halfway: the marker wins over the 0 of 50", 0.5, " 0         5▴        100 "},
		{"the window is due: the marker wins over the 0", 0, " ▴         50        100 "},
		{"the window has just reset: the marker wins over the last digit", 1, " 0         50        10▴ "},
		{"over one is clamped", 1.5, " 0         50        10▴ "},
		{"NoPace draws no marker", gauge.NoPace, " 0         50        100 "},
		{"any negative pace draws no marker", -0.3, " 0         50        100 "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mustRenderPace(t, 68, width, false, tc.pace)
			if got.Scale != tc.want {
				t.Errorf("scale:\ngot  %q\nwant %q", got.Scale, tc.want)
			}
			plain := mustRender(t, 68, width, false)
			if got.Bezel != plain.Bezel || got.Track != plain.Track {
				t.Errorf("the pace marker changed the bezel or the track:\ngot  %+v\nwant %+v", got, plain)
			}
		})
	}
}

func TestPaceMarkerKeepsEveryRowExactlyWidthCells(t *testing.T) {
	for width := gauge.MinWidth; width <= 60; width++ {
		for _, pace := range []float64{0, 0.33, 0.5, 0.99, 1} {
			b := mustRenderPace(t, 43.5, width, false, pace)
			if w := runewidth.StringWidth(b.Scale); w != width {
				t.Errorf("width %d, pace %v: scale is %d cells: %q", width, pace, w, b.Scale)
			}
			if !strings.Contains(b.Scale, "▴") {
				t.Errorf("width %d, pace %v: no marker in %q", width, pace, b.Scale)
			}
		}
	}
}

// TestPaceMarkerIsCyan: the marker is a time-derived position, so it takes
// the countdown's colour, never a band colour, and the rest of the scale
// row stays in the frame colour on both sides of it.
func TestPaceMarkerIsCyan(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	b, err := gauge.Render(68, 25, false, 0.5)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	frame := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	cyan := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	want := frame.Render(" 0         5") + cyan.Render("▴") + frame.Render("        100 ")
	if b.Scale != want {
		t.Errorf("scale:\ngot  %q\nwant %q", b.Scale, want)
	}
}
