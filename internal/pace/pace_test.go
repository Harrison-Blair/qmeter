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
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestCalculate(t *testing.T) {
	reset := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name             string
		used             float64
		elapsed          time.Duration
		period           time.Duration
		missing, limited bool
		want             string
		expected         float64
	}{
		{"behind", 35, 50 * time.Minute, 100 * time.Minute, false, false, "behind", 50},
		{"lower boundary", 45, 50 * time.Minute, 100 * time.Minute, false, false, "on pace", 50},
		{"upper boundary", 55, 50 * time.Minute, 100 * time.Minute, false, false, "on pace", 50},
		{"below boundary", 44.999, 50 * time.Minute, 100 * time.Minute, false, false, "behind", 50},
		{"above boundary", 55.001, 50 * time.Minute, 100 * time.Minute, false, false, "ahead", 50},
		{"ahead", 70, 50 * time.Minute, 100 * time.Minute, false, false, "ahead", 50},
		{"start", 0, 0, 100 * time.Minute, false, false, "on pace", 0},
		{"before reset", 100, 100*time.Minute - time.Nanosecond, 100 * time.Minute, false, false, "on pace", 100},
		{"reset", 100, 100 * time.Minute, 100 * time.Minute, false, false, "n/a", 0},
		{"expired", 100, 101 * time.Minute, 100 * time.Minute, false, false, "n/a", 0},
		{"future start", 0, -time.Minute, 100 * time.Minute, false, false, "n/a", 0},
		{"no period", 10, 0, 0, false, false, "n/a", 0},
		{"negative period", 10, 0, -time.Hour, false, false, "n/a", 0},
		{"no reset", 10, 50 * time.Minute, 100 * time.Minute, true, false, "n/a", 0},
		{"exhausted rate limited", 100, 97 * time.Minute, 100 * time.Minute, false, true, "on pace", 97},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := provider.Window{RemainingPercent: 100 - tc.used, ResetsAt: reset, Period: tc.period, RateLimited: tc.limited}
			if tc.missing {
				w.ResetsAt = time.Time{}
			}
			got := Calculate(w, reset.Add(-tc.period).Add(tc.elapsed))
			if got.Pace != tc.want || math.Abs(got.RemainingPercent-(100-tc.used)) > 1e-9 {
				t.Fatalf("got %+v", got)
			}
			if tc.want == "n/a" {
				if got.ExpectedRemainingPercent != nil {
					t.Fatal("expected nil")
				}
			} else if got.ExpectedRemainingPercent == nil || math.Abs(*got.ExpectedRemainingPercent-(100-tc.expected)) > 1e-8 {
				t.Fatalf("expected %v", got.ExpectedRemainingPercent)
			}
		})
	}
}

func TestRender(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	w := provider.Window{Provider: "claude", Name: "weekly", Plan: "max", RemainingPercent: 65, ResetsAt: now.Add(50 * time.Minute), Period: 100 * time.Minute, RateLimited: true}
	r := usage.Result{Windows: []provider.Window{w, {Provider: "codex", Name: "credits", RemainingPercent: 70}}, Errors: []usage.ProviderError{{Provider: "cursor", Message: "oops & retry"}}, Undetected: []usage.ProviderError{{Provider: "other", Message: "no credentials"}}}
	var out bytes.Buffer
	if err := RenderText(&out, r, now); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"PROVIDER", "WINDOW", "PACE", "REMAINING", "EXPECTED", "RESETS", "behind", "65.0%", "50.0%", "in 50m  (rate limited)", "n/a", "error: oops & retry", "not detected: no credentials"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
	out.Reset()
	if err := RenderJSON(&out, r, now); err != nil {
		t.Fatal(err)
	}
	var env struct {
		Windows    []map[string]any
		Errors     []map[string]any
		Undetected []map[string]any
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	base, _ := json.Marshal(w)
	var original map[string]any
	json.Unmarshal(base, &original)
	for k, v := range original {
		if env.Windows[0][k] != v {
			t.Errorf("lost %s", k)
		}
	}
	for k, v := range map[string]any{"pace": "behind", "remaining_percent": float64(65), "expected_remaining_percent": float64(50)} {
		if env.Windows[0][k] != v {
			t.Errorf("%s = %v", k, env.Windows[0][k])
		}
	}
	for _, win := range env.Windows {
		for _, key := range []string{"used_percent", "expected_used_percent"} {
			if _, ok := win[key]; ok {
				t.Errorf("obsolete field %s", key)
			}
		}
	}
	if v, ok := env.Windows[1]["expected_remaining_percent"]; !ok || v != nil {
		t.Fatal("n/a expectation must be explicit null")
	}
	if env.Errors[0]["message"] != "oops & retry" || env.Undetected[0]["reason"] != "no credentials" {
		t.Fatal("bad error forms")
	}
	out.Reset()
	if err := RenderJSON(&out, usage.Result{}, now); err != nil {
		t.Fatal(err)
	}
	if out.String() != "{\"windows\":[],\"errors\":[],\"undetected\":[]}\n" {
		t.Fatal(out.String())
	}
	out.Reset()
	if err := RenderText(&out, usage.Result{}, now); err != nil {
		t.Fatal(err)
	}
	if out.String() != "no providers detected\n" {
		t.Fatal(out.String())
	}
}

// A reset three and a half days away places a seven-day limit halfway
// through its period; consuming more than halfway is ahead of that baseline.
func TestWeeklyCodexPaceDirection(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		remaining float64
		want      string
	}{{30, "ahead"}, {70, "behind"}} {
		w := provider.Window{Provider: "codex", Name: "weekly", RemainingPercent: tc.remaining, Period: 7 * 24 * time.Hour, ResetsAt: now.Add(84 * time.Hour)}
		got := Calculate(w, now)
		if got.Pace != tc.want || got.ExpectedRemainingPercent == nil || *got.ExpectedRemainingPercent != 50 {
			t.Fatalf("remaining %.1f: %+v", tc.remaining, got)
		}
	}
}

func TestColoredPaceStatuses(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	r := usage.Result{Windows: []provider.Window{
		{Provider: "claude", Name: "weekly", RemainingPercent: 70, Period: time.Hour, ResetsAt: now.Add(30 * time.Minute)},
		{Provider: "codex", Name: "weekly", RemainingPercent: 50, Period: time.Hour, ResetsAt: now.Add(30 * time.Minute)},
		{Provider: "cursor", Name: "weekly", RemainingPercent: 30, Period: time.Hour, ResetsAt: now.Add(30 * time.Minute)},
		{Provider: "opencode-go", Name: "credits", RemainingPercent: 30},
	}}
	var plainOut, colored bytes.Buffer
	if err := RenderText(&plainOut, r, now); err != nil {
		t.Fatal(err)
	}
	renderer := lipgloss.NewRenderer(&colored)
	renderer.SetColorProfile(termenv.ANSI256)
	if err := renderTextStyled(&colored, r, now, renderer); err != nil {
		t.Fatal(err)
	}
	if ansi.Strip(colored.String()) != plainOut.String() {
		t.Fatal("ANSI changes alignment")
	}
	for label, color := range map[string]string{"behind": "208", "on pace": "10", "ahead": "11", "n/a": "8"} {
		want := renderer.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).Render(label)
		if !strings.Contains(colored.String(), want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(colored.String(), "[ahead]") {
		t.Fatal("CLI must not bracket statuses")
	}
}
