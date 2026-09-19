package dash

import (
	"strings"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/provider/providertest"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

func TestTimelineToggleAndRefresh(t *testing.T) {
	p := providertest.Succeeding("claude", nil)
	m := newModel(t, false, p)
	m.width, m.height, m.res, m.haveRes = 80, 8, sample(), true
	m.loading = false
	initial := m.View()
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if !strings.Contains(m.View(), "+7d") {
		t.Fatalf("t did not open timeline: %s", m.View())
	}
	if !strings.Contains(m.View(), "t timeline") {
		t.Fatal("missing timeline hint")
	}
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.offset != 1 {
		t.Fatalf("timeline did not scroll: %d", m.offset)
	}
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if m.View() != initial {
		t.Fatal("t did not restore gauges at top")
	}
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	next, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if !next.loading || cmd == nil {
		t.Fatal("r did not refresh on timeline")
	}
	_ = next.fetch()()
	if p.Fetches() != 1 {
		t.Fatalf("fetches = %d", p.Fetches())
	}
	if !strings.Contains(next.View(), "+7d") {
		t.Fatal("refresh changed page")
	}
	fresh := newModel(t, false)
	fresh.width, fresh.height, fresh.res, fresh.haveRes = 80, 8, sample(), true
	if strings.Contains(fresh.View(), "+7d") {
		t.Fatal("page persisted")
	}
}

func TestTimelineViewDimensions(t *testing.T) {
	for _, width := range []int{1, 20, 40, 69, 70, 80, 120} {
		for _, height := range []int{1, 3, 12} {
			m := newModel(t, true)
			m.width, m.height, m.res, m.haveRes = width, height, sample(), true
			m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
			lines := strings.Split(m.View(), "\n")
			if len(lines) != height {
				t.Fatalf("%dx%d: %d lines", width, height, len(lines))
			}
			for _, line := range lines {
				if runewidth.StringWidth(line) != width {
					t.Fatalf("%dx%d: bad width %d: %q", width, height, runewidth.StringWidth(line), line)
				}
			}
		}
	}
}

func TestLoadingHintsCompact(t *testing.T) {
	for _, timeline := range []bool{false, true} {
		m := shown(t, false, sample(), 50, 8)
		m.timeline, m.loading = timeline, true
		footer := footerOf(t, m)
		if !strings.Contains(footer, "↓ ") || !strings.Contains(footer, "more") {
			t.Errorf("loading footer (timeline=%t) lost count: %q", timeline, footer)
		}
		if !strings.Contains(footer, "refreshing") || !strings.Contains(footer, "t ↔") || !strings.Contains(footer, "q quit") {
			t.Errorf("loading footer (timeline=%t) lacks compact hints: %q", timeline, footer)
		}
	}
	m := newModel(t, false)
	m.width, m.height = 50, 8
	footer := footerOf(t, m)
	if !strings.Contains(footer, "fetching") || !strings.Contains(footer, "t ↔") {
		t.Errorf("first-fetch footer lacks compact hints: %q", footer)
	}
}
