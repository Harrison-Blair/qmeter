package spend

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func number(v float64) *float64 { return &v }

func TestAmounts(t *testing.T) {
	for _, tt := range []struct {
		name        string
		b           provider.Balance
		left, limit string
	}{
		{"usd", provider.Balance{Unit: "usd", Remaining: number(5.75), Limit: number(20)}, "$5.75", "$20.00"},
		{"usd rounding", provider.Balance{Unit: "usd", Remaining: number(1.239), Limit: number(10)}, "$1.24", "$10.00"},
		{"credits", provider.Balance{Unit: "credits", Remaining: number(3.5), Limit: number(10)}, "3.5", "10"},
		{"percent", provider.Balance{Unit: "percent", Remaining: number(62.5), Limit: number(100)}, "62.5%", "100%"},
		{"unconfirmed", provider.Balance{Unit: "unconfirmed", Remaining: number(5.75), Limit: number(20)}, "5.75", "20"},
		{"credits repeating", provider.Balance{Unit: "credits", Remaining: number(33.333333333333336), Limit: number(100)}, "33.33", "100"},
		{"credits rounding", provider.Balance{Unit: "credits", Remaining: number(1.239), Limit: number(10)}, "1.24", "10"},
		{"percent repeating", provider.Balance{Unit: "percent", Remaining: number(66.66666666666667), Limit: number(100)}, "66.67%", "100%"},
		{"unconfirmed repeating", provider.Balance{Unit: "unconfirmed", Remaining: number(33.333333333333336), Limit: number(50)}, "33.33", "50"},
		{"zero dollars", provider.Balance{Unit: "usd", Remaining: number(0), Limit: number(0)}, "$0.00", "$0.00"},
		{"zero credits", provider.Balance{Unit: "credits", Remaining: number(0)}, "0", "-"},
		{"zero percent", provider.Balance{Unit: "percent", Remaining: number(0)}, "0%", "-"},
		{"zero unconfirmed", provider.Balance{Unit: "unconfirmed", Remaining: number(0)}, "0", "-"},
		{"unknown", provider.Balance{Unit: "usd", Used: number(8)}, "-", "-"},
		{"unknown remaining with limit", provider.Balance{Unit: "usd", Used: number(8), Limit: number(10)}, "-", "$10.00"},
		{"unlimited", provider.Balance{Unit: "credits", Remaining: number(0), Unlimited: true}, "unlimited", "-"},
		{"unlimited unknown", provider.Balance{Unit: "credits", Unlimited: true}, "unlimited", "-"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			left, limit := Amounts(tt.b)
			if left != tt.left || limit != tt.limit {
				t.Errorf("amounts = %q / %q, want %q / %q", left, limit, tt.left, tt.limit)
			}
		})
	}
}

func TestRenderTextExact(t *testing.T) {
	r := usage.Result{
		Windows: []provider.Window{{Provider: "opencode-go", Name: "not a balance"}},
		Balances: []provider.Balance{
			{Provider: "claude", Name: "extra usage", Unit: "usd", Used: number(99), Remaining: number(5), Limit: number(10)},
			{Provider: "codex", Name: "credits", Unit: "credits", Remaining: number(3.5)},
			{Provider: "cursor", Name: "included", Unit: "unconfirmed", Remaining: number(0), Limit: number(0)},
			{Provider: "cursor", Name: "on-demand", Unit: "unconfirmed", Unlimited: true},
			{Provider: "claude", Name: "extra usage", Unit: "percent", Remaining: number(25), Limit: number(100)},
		},
		Errors:     []usage.ProviderError{{Provider: "opencode-go", Message: "synthetic failure"}},
		Undetected: []usage.ProviderError{{Provider: "missing", Message: "synthetic missing login"}},
	}
	var out bytes.Buffer
	if err := RenderText(&out, r); err != nil {
		t.Fatal(err)
	}
	const want = "PROVIDER     NAME         LEFT       OF      BAR\n" +
		"claude       extra usage  $5.00      $10.00  ▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱▱▱▱\n" +
		"codex        credits      3.5        -       -\n" +
		"cursor       included     0          0       -\n" +
		"cursor       on-demand    unlimited  -       -\n" +
		"claude       extra usage  25%        100%    ▰▰▰▰▰▱▱▱▱▱▱▱▱▱▱▱▱▱▱▱\n" +
		"opencode-go  error: synthetic failure\n" +
		"missing      not detected: synthetic missing login\n"
	if out.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestRenderTextEmptyAndStatus(t *testing.T) {
	for _, tt := range []struct {
		name string
		r    usage.Result
		want string
	}{
		{"empty", usage.Result{}, "no balances reported\n"},
		{"windows only", usage.Result{Windows: []provider.Window{{Provider: "opencode-go"}}}, "no balances reported\n"},
		{"error", usage.Result{Errors: []usage.ProviderError{{Provider: "claude", Message: "failed"}}}, "PROVIDER  NAME  LEFT  OF  BAR\nclaude    error: failed\n"},
		{"undetected", usage.Result{Undetected: []usage.ProviderError{{Provider: "cursor", Message: "missing"}}}, "PROVIDER  NAME  LEFT  OF  BAR\ncursor    not detected: missing\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := RenderText(&out, tt.r); err != nil {
				t.Fatal(err)
			}
			if out.String() != tt.want {
				t.Errorf("got %q, want %q", out.String(), tt.want)
			}
		})
	}
}

func TestRenderTextBars(t *testing.T) {
	for _, tt := range []struct {
		name             string
		remaining, limit *float64
		filled           int
	}{
		{"full", number(10), number(10), 20},
		{"half", number(5), number(10), 10},
		{"quarter", number(2.5), number(10), 5},
		{"empty", number(0), number(10), 0},
		{"above limit", number(12), number(10), 20},
		{"negative remaining", number(-2), number(10), 0},
		{"missing limit", number(5), nil, -1},
		{"zero limit", number(5), number(0), -1},
		{"negative limit", number(5), number(-1), -1},
		{"missing remaining", nil, number(10), -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := RenderText(&out, usage.Result{Balances: []provider.Balance{{Provider: "claude", Name: "test", Unit: "credits", Remaining: tt.remaining, Limit: tt.limit}}}); err != nil {
				t.Fatal(err)
			}
			fields := strings.Fields(strings.Split(out.String(), "\n")[1])
			bar := fields[len(fields)-1]
			want := "-"
			if tt.filled >= 0 {
				want = strings.Repeat("▰", tt.filled) + strings.Repeat("▱", 20-tt.filled)
			}
			if bar != want {
				t.Errorf("bar=%q, want %q", bar, want)
			}
			if tt.filled >= 0 && lipgloss.Width(bar) != 20 {
				t.Errorf("bar width=%d", lipgloss.Width(bar))
			}
		})
	}
}

func TestRenderTextBandColors(t *testing.T) {
	for _, pct := range []float64{0, 25, 50, 75, 100} {
		var out bytes.Buffer
		renderer := lipgloss.NewRenderer(&out)
		renderer.SetColorProfile(termenv.ANSI256)
		r := usage.Result{Balances: []provider.Balance{{Provider: "claude", Name: "test", Unit: "percent", Remaining: number(pct), Limit: number(100)}}}
		if err := renderTextStyled(&out, r, renderer); err != nil {
			t.Fatal(err)
		}
		filled := int(pct / 5)
		bar := strings.Repeat("▰", filled) + strings.Repeat("▱", 20-filled)
		want := renderer.NewStyle().Foreground(display.Band(pct, false)).Render(bar)
		if !strings.Contains(want, "\x1b[") || !strings.Contains(out.String(), want) {
			t.Errorf("%g%% bar missing expected color: %q, want %q", pct, out.String(), want)
		}
	}
}

func TestRenderTextPipesAndNoColor(t *testing.T) {
	for _, noColor := range []string{"", "1"} {
		t.Run("NO_COLOR="+noColor, func(t *testing.T) {
			t.Setenv("NO_COLOR", noColor)
			t.Setenv("CLICOLOR_FORCE", "1")
			var out bytes.Buffer
			if err := RenderText(&out, usage.Result{Balances: []provider.Balance{{Provider: "claude", Name: "extra usage", Remaining: number(50), Limit: number(100)}}}); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), "\x1b") {
				t.Fatalf("styled pipe: %q", out.String())
			}
		})
	}
}

func TestRenderJSONExact(t *testing.T) {
	var out bytes.Buffer
	r := usage.Result{
		Windows:    []provider.Window{{Provider: "opencode-go"}},
		Balances:   []provider.Balance{{Provider: "codex", Name: "credits", Unit: "credits", Remaining: number(0), Unlimited: true}},
		Errors:     []usage.ProviderError{{Provider: "claude", Message: "failed & <retry>"}},
		Undetected: []usage.ProviderError{{Provider: "cursor", Message: "missing"}},
	}
	if err := RenderJSON(&out, r); err != nil {
		t.Fatal(err)
	}
	const want = `{"balances":[{"provider":"codex","name":"credits","unit":"credits","used":null,"limit":null,"remaining":0,"unlimited":true}],"errors":[{"provider":"claude","message":"failed & <retry>"}],"undetected":[{"provider":"cursor","reason":"missing"}]}` + "\n"
	if out.String() != want {
		t.Errorf("got %s, want %s", out.String(), want)
	}
	out.Reset()
	if err := RenderJSON(&out, usage.Result{}); err != nil {
		t.Fatal(err)
	}
	if want := "{\"balances\":[],\"errors\":[],\"undetected\":[]}\n"; out.String() != want {
		t.Errorf("empty = %q, want %q", out.String(), want)
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestRenderWriterErrors(t *testing.T) {
	failure := errors.New("write failed")
	for _, render := range []func(io.Writer, usage.Result) error{RenderText, RenderJSON} {
		for _, r := range []usage.Result{{}, {Balances: []provider.Balance{{Name: "credits"}}}} {
			if err := render(failingWriter{failure}, r); !errors.Is(err, failure) {
				t.Errorf("error=%v, want %v", err, failure)
			}
		}
	}
}
