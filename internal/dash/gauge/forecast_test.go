package gauge_test

import (
	"strings"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/dash/gauge"
	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestForecastMarkers(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	if display.Forecast != lipgloss.Color("13") {
		t.Fatal("forecast colour must be bright magenta")
	}
	for _, tc := range []struct {
		name          string
		pct, forecast float64
		at            int
		mark          string
	}{
		{"diamond position", 70, 20, 5, "◇"},
		{"diamond tie at zero", 50, 0, 1, "◇"},
		{"diamond same cell", 70, 69, -1, ""},
		{"diamond right", 70, 80, -1, ""},
		{"dry at zero", 70, gauge.RunsDry, 1, "✕"},
		{"dry needle at zero", 0, gauge.RunsDry, -1, ""},
		{"dry needle rounds to zero", 1, gauge.RunsDry, -1, ""},
		{"no forecast", 70, gauge.NoForecast, -1, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := gauge.Render(tc.pct, 22, false, 0.5, tc.forecast)
			if err != nil {
				t.Fatal(err)
			}
			base, err := gauge.Render(tc.pct, 22, false, 0.5, gauge.NoForecast)
			if err != nil {
				t.Fatal(err)
			}
			bare := gauge.Plain(b)
			want := []rune(gauge.Plain(base).Track)
			if tc.at >= 0 {
				want[tc.at] = []rune(tc.mark)[0]
			}
			if bare.Track != string(want) {
				t.Errorf("track = %q, want %q", bare.Track, string(want))
			}
			if strings.Contains(bare.Track+bare.Bezel+bare.Scale, "\x1b") {
				t.Error("Plain retained styling")
			}
			if bare.Bezel != gauge.Plain(base).Bezel || bare.Scale != gauge.Plain(base).Scale {
				t.Error("forecast changed bezel or scale")
			}
			if tc.at >= 0 {
				color := display.Forecast
				if tc.mark == "✕" {
					color = display.RateLimited
				}
				styled := lipgloss.NewStyle().Foreground(color).Render(tc.mark)
				if !strings.Contains(b.Track, styled) {
					t.Errorf("styled Track missing %q: %q", styled, b.Track)
				}
			} else if b.Track != base.Track {
				t.Error("suppressed marker changed styled Track")
			}
		})
	}
}

func TestForecastSentinelValues(t *testing.T) {
	if gauge.NoForecast != -1 || gauge.RunsDry != -2 {
		t.Errorf("NoForecast = %v, RunsDry = %v; want -1, -2", gauge.NoForecast, gauge.RunsDry)
	}
}
