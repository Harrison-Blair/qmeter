package usage

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// renderNow is the fixed "now" every renderer test injects, so the RESETS
// countdown is deterministic.
var renderNow = time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)

// goldenBlock is the exact golden text output this package renders, per the
// layout contract in renderText's doc comment. It is the real output of
// tabwriter.NewWriter(out, 0, 8, 2, ' ', 0) — do not hand-align it.
const goldenBlock = "" +
	"PROVIDER     WINDOW   PLAN  USED    RESETS\n" +
	"claude       5h       max   42.0%   in 2h13m\n" +
	"claude       weekly   max   18.5%   in 3d4h\n" +
	"opencode-go  monthly  go    100.0%  in 12d  (rate limited)\n" +
	"cursor       total    free  4.5%    in 7d22h\n" +
	"codex        error: token expired, open codex to refresh\n"

func TestRenderText_MatchesGoldenOutputBlock(t *testing.T) {
	res := Result{
		Windows: []provider.Window{
			{Provider: "claude", Name: "5h", Plan: "max", UsedPercent: 42.0, ResetsAt: renderNow.Add(2*time.Hour + 13*time.Minute)},
			{Provider: "claude", Name: "weekly", Plan: "max", UsedPercent: 18.5, ResetsAt: renderNow.Add(3*24*time.Hour + 4*time.Hour)},
			{Provider: "opencode-go", Name: "monthly", Plan: "go", UsedPercent: 100.0, ResetsAt: renderNow.Add(12 * 24 * time.Hour), RateLimited: true},
			{Provider: "cursor", Name: "total", Plan: "free", UsedPercent: 4.5, ResetsAt: renderNow.Add(7*24*time.Hour + 22*time.Hour)},
		},
		Errors: []ProviderError{
			{Provider: "codex", Message: "token expired, open codex to refresh"},
		},
	}

	var out bytes.Buffer
	if err := renderText(&out, res, renderNow); err != nil {
		t.Fatalf("renderText: %v", err)
	}
	if got := out.String(); got != goldenBlock {
		t.Fatalf("output mismatch\n got:\n%s\nwant:\n%s", got, goldenBlock)
	}
}

func TestRenderText_NoProvidersDetectedLine(t *testing.T) {
	var out bytes.Buffer
	if err := renderText(&out, Result{}, renderNow); err != nil {
		t.Fatalf("renderText: %v", err)
	}
	const want = "no providers detected\n"
	if got := out.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderText_FailureLineIsTwoCellPaddedRow(t *testing.T) {
	res := Result{
		Windows: []provider.Window{
			{Provider: "claude", Name: "5h", Plan: "max", UsedPercent: 42.0},
		},
		Errors: []ProviderError{
			{Provider: "codex", Message: "token expired, open codex to refresh"},
		},
	}

	var out bytes.Buffer
	if err := renderText(&out, res, renderNow); err != nil {
		t.Fatalf("renderText: %v", err)
	}
	// The failure line's PROVIDER cell is padded to the widest provider
	// name printed, while its message — a non-tab-terminated last cell —
	// is excluded from the WINDOW column's width.
	want := "" +
		"PROVIDER  WINDOW  PLAN  USED   RESETS\n" +
		"claude    5h      max   42.0%  -\n" +
		"codex     error: token expired, open codex to refresh\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderText_NotDetectedLineIsTwoCellPaddedRow(t *testing.T) {
	res := Result{
		Undetected: []ProviderError{
			{Provider: "codex", Message: "not logged in, run codex to log in"},
		},
	}

	var out bytes.Buffer
	if err := renderText(&out, res, renderNow); err != nil {
		t.Fatalf("renderText: %v", err)
	}
	want := "" +
		"PROVIDER  WINDOW  PLAN  USED  RESETS\n" +
		"codex     not detected: not logged in, run codex to log in\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderText_DashForEmptyPlanAndZeroResets(t *testing.T) {
	res := Result{
		Windows: []provider.Window{
			{Provider: "claude", Name: "5h"},
		},
	}

	var out bytes.Buffer
	if err := renderText(&out, res, renderNow); err != nil {
		t.Fatalf("renderText: %v", err)
	}
	want := "" +
		"PROVIDER  WINDOW  PLAN  USED  RESETS\n" +
		"claude    5h      -     0.0%  -\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderText_RateLimitedSuffixOnDashResets(t *testing.T) {
	res := Result{
		Windows: []provider.Window{
			{Provider: "opencode-go", Name: "monthly", Plan: "go", UsedPercent: 100, RateLimited: true},
		},
	}

	var out bytes.Buffer
	if err := renderText(&out, res, renderNow); err != nil {
		t.Fatalf("renderText: %v", err)
	}
	if !strings.HasSuffix(out.String(), "-  (rate limited)\n") {
		t.Fatalf("missing two-space-separated suffix on the RESETS cell: %q", out.String())
	}
}

func TestRenderText_UsesRealNowByDefault(t *testing.T) {
	res := Result{
		Windows: []provider.Window{
			{Provider: "claude", Name: "5h", UsedPercent: 1, ResetsAt: time.Now().Add(2*time.Hour + 13*time.Minute)},
		},
	}

	var out bytes.Buffer
	if err := RenderText(&out, res); err != nil {
		t.Fatalf("RenderText: %v", err)
	}
	if !strings.Contains(out.String(), "in 2h1") {
		t.Fatalf("RenderText did not count down from time.Now(): %q", out.String())
	}
}

func TestFormatResets_LargestTwoNonZeroUnits(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"days and hours", 3*24*time.Hour + 4*time.Hour, "3d4h"},
		{"whole days", 12 * 24 * time.Hour, "12d"},
		{"days with zero hours but minutes", 3*24*time.Hour + 7*time.Minute, "3d"},
		{"hours and minutes", 2*time.Hour + 13*time.Minute, "2h13m"},
		{"whole hours", 5 * time.Hour, "5h"},
		{"minutes only", 45 * time.Minute, "45m"},
		{"sub-minute", 30 * time.Second, "0m"},
		{"zero", 0, "0m"},
		{"already past", -5 * time.Minute, "0m"},
		{"days hours and minutes truncates", 7*24*time.Hour + 22*time.Hour + 59*time.Minute, "7d22h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatResets(tt.d); got != tt.want {
				t.Fatalf("formatResets(%s) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestRenderText_ZeroResetsAtRendersDashNotCountdown(t *testing.T) {
	if got := resetsCell(provider.Window{}, renderNow); got != "-" {
		t.Fatalf("got %q, want %q", got, "-")
	}
}

func TestRenderJSON_MatchesEnvelopeWithAllThreeKeysAlwaysPresent(t *testing.T) {
	res := Result{
		Windows: []provider.Window{
			{
				Provider:    "claude",
				Name:        "5h",
				Plan:        "max",
				UsedPercent: 42.5,
				ResetsAt:    time.Date(2026, time.September, 16, 14, 13, 0, 0, time.UTC),
				Period:      5 * time.Hour,
			},
		},
		Errors: []ProviderError{
			{Provider: "codex", Message: "token expired, open codex to refresh"},
		},
		Undetected: []ProviderError{
			{Provider: "cursor", Message: "not logged in, run cursor-agent to log in"},
		},
	}

	var out bytes.Buffer
	if err := RenderJSON(&out, res); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	want := `{"windows":[{"provider":"claude","name":"5h","plan":"max","used_percent":42.5,` +
		`"resets_at":"2026-09-16T14:13:00Z","period_seconds":18000,"rate_limited":false}],` +
		`"errors":[{"provider":"codex","message":"token expired, open codex to refresh"}],` +
		`"undetected":[{"provider":"cursor","reason":"not logged in, run cursor-agent to log in"}]}` + "\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestRenderJSON_DoesNotEscapeHTMLInMessages(t *testing.T) {
	res := Result{
		Undetected: []ProviderError{
			{Provider: "cursor", Message: `no readable /home/me/a&b/auth.json <or> other`},
		},
	}

	var out bytes.Buffer
	if err := RenderJSON(&out, res); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	// A path or reason carrying & < > must survive verbatim rather than
	// arriving as & — these are messages, not HTML.
	if !strings.Contains(out.String(), `/home/me/a&b/auth.json <or> other`) {
		t.Fatalf("message was HTML-escaped: %s", out.String())
	}
}

func TestRenderJSON_NilSlicesRenderAsEmptyArrays(t *testing.T) {
	var out bytes.Buffer
	if err := RenderJSON(&out, Result{}); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	want := `{"windows":[],"errors":[],"undetected":[]}` + "\n"
	if got := out.String(); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
