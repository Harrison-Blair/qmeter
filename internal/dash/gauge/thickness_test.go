package gauge_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/dash/gauge"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

func TestThicknessShape(t *testing.T) {
	for n := 1; n <= 9; n++ {
		for width := gauge.MinWidth; width <= 60; width++ {
			for _, pct := range []float64{0, 1, 50, 68, 99, 100} {
				b, err := gauge.Render(pct, width, false, 0.5, n)
				if err != nil {
					t.Fatal(err)
				}
				b = gauge.Plain(b)
				if len(b.Tracks) != n {
					t.Fatalf("thickness %d: %d track rows", n, len(b.Tracks))
				}
				needle := int(pct/100*float64(width-3) + 0.5)
				filled := needle + 1
				if pct == 0 {
					filled = 0
				}
				want := "│" + strings.Repeat("▰", filled) + strings.Repeat("▱", width-2-filled) + "│"
				for _, row := range b.Tracks[:len(b.Tracks)-1] {
					if row != want {
						t.Fatalf("upper %q want %q", row, want)
					}
				}
				if b.Tracks[len(b.Tracks)-1] != want || !strings.HasPrefix(b.Bottom, "╰") || !strings.HasSuffix(b.Bottom, "╯") || strings.Count(b.Bottom, "▲") != 1 {
					t.Fatalf("bottom %q", b.Tracks[len(b.Tracks)-1])
				}
				for _, row := range append(append([]string{b.Bezel}, b.Tracks...), b.Bottom, b.Scale) {
					if runewidth.StringWidth(row) != width {
						t.Fatalf("row width: %q", row)
					}
				}
			}
		}
	}
}

func TestThicknessCompatibility(t *testing.T) {
	data, err := os.ReadFile("testdata/fixed-three.json")
	if err != nil {
		t.Fatal(err)
	}
	var baseline [][]string
	if err := json.Unmarshal(data, &baseline); err != nil {
		t.Fatal(err)
	}
	defer lipgloss.SetColorProfile(termenv.Ascii)
	at := 0
	for _, p := range []termenv.Profile{termenv.Ascii, termenv.ANSI256} {
		lipgloss.SetColorProfile(p)
		for _, rl := range []bool{false, true} {
			for _, pct := range []float64{0, 1, 50, 68, 99, 100} {
				b, err := gauge.Render(pct, 25, rl, 0.5, 3)
				if err != nil {
					t.Fatal(err)
				}
				got := append(append([]string{b.Bezel}, b.Tracks...), b.Bottom, b.Scale)
				if !reflect.DeepEqual(got, baseline[at]) {
					t.Fatalf("baseline %d changed", at)
				}
				one, err := gauge.Render(pct, 25, rl, 0.5, 1)
				if err != nil {
					t.Fatal(err)
				}
				if len(one.Tracks) != 1 || one.Bezel != b.Bezel || one.Tracks[len(one.Tracks)-1] != b.Tracks[len(b.Tracks)-1] || one.Scale != b.Scale || one.Bottom != b.Bottom {
					t.Fatal("thickness 1 differs from bezel, bottom, scale")
				}
				at++
			}
		}
	}
}
