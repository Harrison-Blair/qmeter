package pace

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
)

func TestForecast(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 123456789, time.UTC)
	for _, tc := range []struct {
		name             string
		remaining        float64
		elapsed, period  time.Duration
		missing, limited bool
		state            string
		lands            float64
		dryIn            time.Duration
	}{
		{"mid-window survivor", 70, 50 * time.Minute, 100 * time.Minute, false, false, "survives", 40, 0},
		{"mid-window dry", 20, 40 * time.Minute, 100 * time.Minute, false, false, "dry", 0, 10 * time.Minute},
		{"used zero", 100, 50 * time.Minute, 100 * time.Minute, false, false, "survives", 100, 0},
		{"used negative", 101, 50 * time.Minute, 100 * time.Minute, false, false, "survives", 101, 0},
		{"already empty before floor", 0, time.Minute, 100 * time.Minute, false, false, "empty", 0, 0},
		{"negative remaining", -1, 50 * time.Minute, 100 * time.Minute, false, false, "empty", 0, 0},
		{"rate limited before floor", 80, time.Minute, 100 * time.Minute, false, true, "empty", 0, 0},
		{"zero reset", 0, 50 * time.Minute, 100 * time.Minute, true, true, "none", 0, 0},
		{"zero period", 20, 0, 0, false, false, "none", 0, 0},
		{"negative period", 20, 0, -time.Hour, false, false, "none", 0, 0},
		{"before start", 0, -time.Minute, 100 * time.Minute, false, true, "none", 0, 0},
		{"at reset", 0, 100 * time.Minute, 100 * time.Minute, false, true, "none", 0, 0},
		{"after reset", 20, 101 * time.Minute, 100 * time.Minute, false, false, "none", 0, 0},
		{"under five percent", 20, 5*time.Minute - time.Nanosecond, 100 * time.Minute, false, false, "none", 0, 0},
		{"at five percent", 100, 5 * time.Minute, 100 * time.Minute, false, false, "survives", 100, 0},
		{"rate limited zero period", 20, 0, 0, false, true, "none", 0, 0},
		{"tiny used", 100 - 1e-10, 50 * time.Hour, 100 * time.Hour, false, false, "survives", 100 - 2e-10, 0},
		{"tie", 50, 50 * time.Minute, 100 * time.Minute, false, false, "survives", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := provider.Window{RemainingPercent: tc.remaining, Period: tc.period, ResetsAt: now.Add(tc.period - tc.elapsed), RateLimited: tc.limited}
			if tc.missing {
				w.ResetsAt = time.Time{}
			}
			got := Forecast(w, now)
			if got.State != tc.state {
				t.Fatalf("state = %q, want %q", got.State, tc.state)
			}
			if tc.state == "none" {
				if got.ExhaustionAt != nil || got.RemainingAtReset != nil {
					t.Fatalf("none has projection: %+v", got)
				}
				return
			}
			if got.RemainingAtReset == nil || math.Abs(*got.RemainingAtReset-tc.lands) > 1e-9 {
				t.Fatalf("remaining = %v, want %.12f", got.RemainingAtReset, tc.lands)
			}
			if tc.state == "survives" {
				if got.ExhaustionAt != nil {
					t.Fatalf("survivor exhaustion = %v", got.ExhaustionAt)
				}
			} else if got.ExhaustionAt == nil || !got.ExhaustionAt.Equal(now.Add(tc.dryIn)) {
				t.Fatalf("exhaustion = %v, want %v", got.ExhaustionAt, now.Add(tc.dryIn))
			}
		})
	}
}

func TestRenderTextForecast(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name      string
		remaining float64
		period    time.Duration
		limited   bool
		want      string
	}{
		{"dry", 20, 100 * time.Minute, false, "in 10m"},
		{"survivor", 80, 100 * time.Minute, false, "-"},
		{"n/a", 20, 0, false, "-"},
		{"already dry", 0, 100 * time.Minute, false, "empty"},
		{"rate limited", 80, 100 * time.Minute, true, "empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			w := provider.Window{Provider: "claude", Name: "window", RemainingPercent: tc.remaining, Period: tc.period, ResetsAt: now.Add(60 * time.Minute), RateLimited: tc.limited}
			if err := RenderText(&out, usage.Result{Windows: []provider.Window{w}}, now); err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(out.String(), "\n")
			start := strings.Index(lines[0], "RUNS OUT")
			end := strings.Index(lines[0], "RESETS")
			if start < 0 || start <= strings.Index(lines[0], "EXPECTED") || end <= start {
				t.Fatalf("missing RUNS OUT after EXPECTED: %s", lines[0])
			}
			if got := strings.TrimSpace(lines[1][start:end]); got != tc.want {
				t.Fatalf("RUNS OUT = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderJSONForecast(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 123456789, time.UTC)
	for _, tc := range []struct {
		name       string
		remaining  float64
		period     time.Duration
		limited    bool
		exhaustion any
		lands      any
	}{
		{"dry", 20, 100 * time.Minute, false, now.Add(10 * time.Minute).Format(time.RFC3339), float64(0)},
		{"survivor", 80, 100 * time.Minute, false, nil, float64(50)},
		{"none", 20, 0, false, nil, nil},
		{"empty", 0, 100 * time.Minute, false, now.Format(time.RFC3339), float64(0)},
		{"rate limited", 80, 100 * time.Minute, true, now.Format(time.RFC3339), float64(0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			r := usage.Result{Windows: []provider.Window{{Provider: "claude", Name: "window", RemainingPercent: tc.remaining, Period: tc.period, ResetsAt: now.Add(60 * time.Minute), RateLimited: tc.limited}}}
			if err := RenderJSON(&out, r, now); err != nil {
				t.Fatal(err)
			}
			var env struct{ Windows []map[string]any }
			if err := json.Unmarshal(out.Bytes(), &env); err != nil {
				t.Fatal(err)
			}
			for key, want := range map[string]any{"projected_exhaustion_at": tc.exhaustion, "projected_remaining_at_reset": tc.lands} {
				if got, ok := env.Windows[0][key]; !ok || got != want {
					t.Errorf("%s = %v (present %v), want %v", key, got, ok, want)
				}
			}
			if _, ok := env.Windows[0]["State"]; ok {
				t.Error("State serialized")
			}
			if _, ok := env.Windows[0]["state"]; ok {
				t.Error("state serialized")
			}
			out.Reset()
			if err := usage.RenderJSON(&out, r); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), "projected_") {
				t.Fatal("forecast leaked into usage JSON")
			}
		})
	}
}
