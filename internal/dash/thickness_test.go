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
	for _, n := range []int{1, 3, 5, 9} {
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
			if at < 0 || at+n+2 >= len(lines) {
				t.Fatal("window missing")
			}
			for i := 0; i < n; i++ {
				line := lines[at+2+i]
				if i == n-1 {
					if !strings.Contains(line, "┴") || !strings.Contains(line, "▲") {
						t.Fatalf("thickness %d width %d bottom missing", n, width)
					}
				} else if !strings.Contains(line, "│▰") || strings.Contains(line, "▲") {
					t.Fatalf("thickness %d width %d upper missing", n, width)
				}
			}
			if !strings.Contains(lines[at+n+2], "100") {
				t.Fatalf("thickness %d width %d scale misplaced", n, width)
			}
		}
	}
}
