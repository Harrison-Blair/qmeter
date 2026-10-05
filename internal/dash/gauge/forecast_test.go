package gauge_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/dash/gauge"
)

// Forecasts belong in the window note, never in any gauge row.
func TestForecastMarkersAbsent(t *testing.T) {
	for _, rl := range []bool{false, true} {
		for _, pct := range []float64{0, 1, 50, 70, 99, 100} {
			b, err := gauge.Render(pct, 22, rl, 0.5, 3)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range sixRows(t, b) {
				if strings.ContainsAny(row, "◇✕") {
					t.Errorf("pct %v limited %v: glyph in %q", pct, rl, row)
				}
			}
		}
	}
}

func TestRenderHasNoForecastParameter(t *testing.T) {
	if got := reflect.TypeOf(gauge.Render).NumIn(); got != 5 {
		t.Fatalf("Render has %d parameters, want percentage, width, rate limit, pace and thickness", got)
	}
}
