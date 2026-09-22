package dash

import (
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

func TestFitModelViewportAndReflow(t *testing.T) {
	for _, banner := range []bool{false, true} {
		m := New(Options{Banner: banner, Now: func() time.Time { return now }})
		m = resize(t, m, 120, 12)
		m, _ = step(t, m, resultMsg{sample()})
		m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnd})
		if m.offset == 0 {
			t.Fatal("compact overflow must scroll")
		}
		m = resize(t, m, 120, 60)
		header, body, capacity := m.frame()
		wantHead := 1
		if banner {
			wantHead = 6
		}
		if len(header) != wantHead || len(body) != capacity || m.offset != 0 {
			t.Fatalf("frame header=%d body=%d capacity=%d offset=%d", len(header), len(body), capacity, m.offset)
		}
		if strings.Contains(m.View(), "more") {
			t.Fatal("fit padding introduced scroll")
		}
		m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
		_, body, capacity = m.frame()
		if len(body) != capacity || m.offset != 0 {
			t.Fatal("timeline did not refit")
		}
		m = resize(t, m, 120, 10)
		m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnd})
		m, _ = step(t, m, resultMsg{usage.Result{Windows: []provider.Window{{Provider: "claude", Name: "one"}}}})
		_, body, capacity = m.frame()
		if want := max(0, len(body)-capacity); m.offset != want {
			t.Fatalf("result scroll offset = %d, want %d", m.offset, want)
		}
		for _, height := range []int{1, 2, 5, 10, 60} {
			m = resize(t, m, 120, height)
			lines := strings.Split(m.View(), "\n")
			if len(lines) != height {
				t.Fatalf("height %d got %d rows", height, len(lines))
			}
			for _, line := range lines {
				if runewidth.StringWidth(line) != 120 {
					t.Fatal("incorrect viewport width")
				}
			}
		}
	}
}

func TestFitLoadingAndEmptyStayAtTop(t *testing.T) {
	m := New(Options{Banner: true})
	m = resize(t, m, 120, 40)
	_, body, _ := m.frame()
	if len(body) != 1 || !strings.Contains(body[0], fetchingLabel) {
		t.Fatalf("loading moved: %q", body)
	}
	m, _ = step(t, m, resultMsg{})
	_, body, _ = m.frame()
	if len(body) != 1 || !strings.Contains(body[0], "no providers detected") {
		t.Fatalf("empty moved: %q", body)
	}
	m = resize(t, m, 20, 40)
	header, body, _ := m.frame()
	if len(header) != 0 || len(body) != 1 || !strings.Contains(body[0], "terminal too narrow") {
		t.Fatalf("narrow moved: %q", body)
	}
}
