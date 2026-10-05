package dash

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	tea "github.com/charmbracelet/bubbletea"
)

func TestThicknessModelResize(t *testing.T) {
	for n := 1; n <= 9; n++ {
		m := New(modelOptions(context.Background(), RunOptions{MeterThickness: n}))
		m.now = func() time.Time { return now }
		m.res = usage.Result{Windows: []provider.Window{{Provider: "claude", Name: "5h", RemainingPercent: 68}}}
		m.haveRes = true
		m.loading = false
		for _, width := range []int{120, 36, 80} {
			updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
			m = updated.(Model)
			view := m.View()
			at := -1
			lines := strings.Split(view, "\n")
			for i, line := range lines {
				if strings.Contains(line, "▸ 5h") {
					at = i
					break
				}
			}
			if at < 0 || at+n+3 >= len(lines) {
				t.Fatal("window missing")
			}
			var track string
			for i := 0; i < n; i++ {
				line := lines[at+2+i]
				start := strings.Index(line, "│▰")
				if start < 0 || strings.ContainsAny(line, "▲┴") {
					t.Fatalf("thickness %d width %d track %d lacks plain capped fill", n, width, i)
				}
				end := strings.Index(line[start+len("│"):], "│") + start + len("│")
				got := line[start : end+len("│")]
				if i > 0 && got != track {
					t.Fatalf("thickness %d width %d track rows differ", n, width)
				}
				track = got
			}
			bottom := lines[at+n+2]
			if !strings.Contains(bottom, "╰") || !strings.Contains(bottom, "╯") || !strings.Contains(bottom, "▲") {
				t.Fatalf("thickness %d width %d bottom rail missing", n, width)
			}
			if !strings.Contains(lines[at+n+3], "100") {
				t.Fatalf("thickness %d width %d scale misplaced", n, width)
			}
		}
	}
}
