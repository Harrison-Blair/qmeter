package resets

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

var now = time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)

func TestRenderTextR1LongNamesAtWidths(t *testing.T) {
	result := usage.Result{Windows: []provider.Window{
		{Provider: "claude", Name: strings.Repeat("界", 30), RemainingPercent: 81, ResetsAt: now.Add(16 * time.Minute)},
		{Provider: "cursor", Name: "auto", RemainingPercent: 0, ResetsAt: now.Add(3*24*time.Hour + 12*time.Hour), RateLimited: true},
	}}
	wideName := strings.Repeat("界", 30)
	shortName := strings.Repeat("界", 15) + "…"
	want80 := "   16m claude " + shortName + " Sat 19 12:16 now 81.0%" + strings.Repeat(" ", 12) + "\n" +
		" 3d12h cursor auto" + strings.Repeat(" ", 27) + " Wed 23 00:00 now 0.0% rate limited\n"
	want120 := "   16m  claude  " + wideName + "  Sat 19 12:16   now 81.0%" + strings.Repeat(" ", 12) + "\n" +
		" 3d12h  cursor  auto" + strings.Repeat(" ", 58) + "Wed 23 00:00   now 0.0% rate limited\n"
	for _, tc := range []struct {
		width int
		want  string
	}{{80, want80}, {120, want120}} {
		t.Run(strconv.Itoa(tc.width), func(t *testing.T) {
			var out bytes.Buffer
			if err := renderText(&out, result, now, tc.width, true, display.Renderer(&out)); err != nil {
				t.Fatal(err)
			}
			if got := out.String(); got != tc.want {
				t.Fatalf("width %d mismatch\n got: %q\nwant: %q", tc.width, got, tc.want)
			}
		})
	}
	var pipe bytes.Buffer
	if err := renderText(&pipe, result, now, 120, false, display.Renderer(&pipe)); err != nil {
		t.Fatal(err)
	}
	if got := pipe.String(); got != want80 {
		t.Fatalf("nonterminal width output differs from 80-cell golden: %q", got)
	}
}

func TestRenderTextUsesR1RowsWithoutHeader(t *testing.T) {
	result := usage.Result{
		Windows:    []provider.Window{{Provider: "claude", Name: "5h", RemainingPercent: 81, ResetsAt: now.Add(16 * time.Minute)}},
		Errors:     []usage.ProviderError{{Provider: "codex", Message: "failed"}},
		Undetected: []usage.ProviderError{{Provider: "cursor", Message: "missing"}},
	}
	var out bytes.Buffer
	if err := renderText(&out, result, now, 80, true, display.Renderer(&out)); err != nil {
		t.Fatal(err)
	}
	want := "   16m  claude  5h  Sat 19 12:16   now 81.0%\n" +
		"codex   error: failed\n" +
		"cursor  not detected: missing\n"
	if got := out.String(); got != want {
		t.Fatalf("R1 output mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestTextUsesDefaultProviderTheme(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	result := usage.Result{Windows: []provider.Window{{Provider: "codex", Name: "5h", RemainingPercent: 80, ResetsAt: now.Add(time.Hour)}}}
	for _, width := range []int{69, 70} {
		var out bytes.Buffer
		if err := renderText(&out, result, now, width, true, r); err != nil {
			t.Fatal(err)
		}
		want := r.NewStyle().Foreground(display.Default().Accent("codex")).Bold(true).Render("codex")
		if !strings.Contains(out.String(), want) {
			t.Errorf("width %d: missing default identity %q in %q", width, want, out.String())
		}
	}
}

func TestSort(t *testing.T) {
	windows := []provider.Window{
		{Name: "unknown first"}, {Name: "late", ResetsAt: now.Add(time.Hour)},
		{Name: "equal first", ResetsAt: now}, {Name: "unknown last"}, {Name: "equal last", ResetsAt: now},
	}
	before := append([]provider.Window(nil), windows...)
	got := Sort(windows)
	var names []string
	for _, w := range got {
		names = append(names, w.Name)
	}
	want := []string{"equal first", "equal last", "late", "unknown first", "unknown last"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("sort = %v, want %v", names, want)
	}
	if !reflect.DeepEqual(windows, before) {
		t.Fatal("Sort mutated its input")
	}
}

func TestTextUsesR1AtEveryWidth(t *testing.T) {
	result := usage.Result{Windows: []provider.Window{
		{Provider: "codex", Name: "unknown", RemainingPercent: 80},
		{Provider: "claude", Name: "next", RemainingPercent: 12, ResetsAt: now.Add(time.Hour)},
	}}
	for _, tc := range []struct {
		name  string
		width int
		tty   bool
	}{
		{"narrow terminal", 69, true}, {"pipe", 120, false}, {"terminal", 120, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := renderText(&b, result, now, tc.width, tc.tty, display.Renderer(&b)); err != nil {
				t.Fatal(err)
			}
			got := b.String()
			if strings.Contains(got, "PROVIDER") || strings.Contains(got, "+7d") || strings.ContainsAny(got, "◆●·") {
				t.Fatalf("non-R1 output: %q", got)
			}
			if strings.Index(got, "next") > strings.Index(got, "unknown") {
				t.Fatalf("windows are not sorted by reset time: %q", got)
			}
		})
	}
	var b bytes.Buffer
	if err := RenderText(&b, result, now); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "PROVIDER") || !strings.Contains(b.String(), "next") {
		t.Fatalf("nonterminal writer did not get an R1 list: %q", b.String())
	}
}

func TestRenderJSON(t *testing.T) {
	result := usage.Result{
		Windows: []provider.Window{
			{Provider: "cursor", Name: "unknown", RemainingPercent: 30},
			{Provider: "codex", Name: "later", ResetsAt: now.Add(90*time.Second + 500*time.Millisecond), Period: time.Hour, Plan: "pro"},
			{Provider: "claude", Name: "first", ResetsAt: now.Add(time.Second)},
		},
		Errors:     []usage.ProviderError{{Provider: "failed", Message: "a & b"}},
		Undetected: []usage.ProviderError{{Provider: "missing", Message: "why"}},
	}
	var b bytes.Buffer
	if err := RenderJSON(&b, result, now); err != nil {
		t.Fatal(err)
	}
	const wantWire = `{"windows":[{"name":"first","provider":"claude","rate_limited":false,"remaining_percent":0,"resets_at":"2026-09-19T12:00:01Z","resets_in_seconds":1},{"name":"later","period_seconds":3600,"plan":"pro","provider":"codex","rate_limited":false,"remaining_percent":0,"resets_at":"2026-09-19T12:01:30Z","resets_in_seconds":90},{"name":"unknown","provider":"cursor","rate_limited":false,"remaining_percent":30,"resets_in_seconds":null}],"errors":[{"provider":"failed","message":"a & b"}],"undetected":[{"provider":"missing","reason":"why"}]}` + "\n"
	if b.String() != wantWire {
		t.Fatalf("JSON bytes changed: %s", b.String())
	}
	var got struct {
		Windows    []map[string]any
		Errors     []map[string]string
		Undetected []map[string]string
	}
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"first", "later", "unknown"} {
		if got.Windows[i]["name"] != name {
			t.Fatalf("order = %v", got.Windows)
		}
		value, ok := got.Windows[i]["resets_in_seconds"]
		want := []any{float64(1), float64(90), nil}[i]
		if !ok || value != want {
			t.Errorf("%s seconds = %v (present %t), want %v", name, value, ok, want)
		}
	}
	if got.Windows[1]["plan"] != "pro" || got.Windows[1]["period_seconds"] != float64(3600) || got.Windows[1]["resets_at"] != now.Add(90*time.Second).Format(time.RFC3339) {
		t.Fatal(got.Windows[1])
	}
	if got.Errors[0]["message"] != "a & b" || got.Undetected[0]["reason"] != "why" || strings.Contains(b.String(), `\u0026`) {
		t.Fatal(b.String())
	}
	b.Reset()
	if err := RenderJSON(&b, usage.Result{}, now); err != nil {
		t.Fatal(err)
	}
	var empty map[string]any
	if err := json.Unmarshal(b.Bytes(), &empty); err != nil {
		t.Fatal(err)
	}
	if len(empty) != 3 {
		t.Fatal(empty)
	}
	for _, key := range []string{"windows", "errors", "undetected"} {
		if arr, ok := empty[key].([]any); !ok || len(arr) != 0 {
			t.Fatalf("%s = %v", key, empty[key])
		}
	}
}
