package dash

import (
	"strings"
	"testing"
	"time"

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
	if !strings.Contains(m.View(), "now") {
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
	if !strings.Contains(next.View(), "now") {
		t.Fatal("refresh changed page")
	}
	fresh := newModel(t, false)
	fresh.width, fresh.height, fresh.res, fresh.haveRes = 80, 8, sample(), true
	if strings.Contains(fresh.View(), "now") {
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

func TestCalendarAndTimelineSwitchDirectly(t *testing.T) {
	m := shown(t, true, sample(), 120, 12)
	main := m.View()
	for _, key := range []rune{'c', 't', 'c', 'c', 't', 't'} {
		m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnd})
		m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		if m.offset != 0 {
			t.Fatalf("%c did not reset scroll", key)
		}
		header, _, _ := m.frame()
		if len(header) != 6 {
			t.Fatal("view lost pinned banner")
		}
		if key == 'c' && !strings.Contains(m.View(), "today") && m.View() != main {
			t.Fatal("c did not open calendar or return to main")
		}
	}
	if m.View() != main {
		t.Fatal("toggle sequence did not restore main")
	}
	// Each direction is checked separately so two no-op keys cannot satisfy the sequence.
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if !strings.Contains(m.View(), "today") {
		t.Fatal("c did not open calendar")
	}
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if !strings.Contains(m.View(), "now") || strings.Contains(m.View(), "today") {
		t.Fatal("c -> t failed")
	}
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if !strings.Contains(m.View(), "today") {
		t.Fatal("t -> c failed")
	}
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if !m.loading || !strings.Contains(m.View(), "today") {
		t.Fatal("calendar refresh changed view")
	}
}

func TestCalendarViewportAndBothHints(t *testing.T) {
	for _, width := range []int{1, 20, 36, 40, 69, 70, 80, 120} {
		for _, height := range []int{1, 3, 12, 60} {
			m := shown(t, true, sample(), width, height)
			m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
			m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnd})
			lines := strings.Split(m.View(), "\n")
			if len(lines) != height {
				t.Fatalf("%dx%d height", width, height)
			}
			for _, line := range lines {
				if runewidth.StringWidth(line) != width {
					t.Fatalf("%dx%d width: %q", width, height, line)
				}
			}
			if width >= 36 {
				footer := lines[len(lines)-1]
				for _, key := range []string{"t ", "c "} {
					if !strings.Contains(footer, key) {
						t.Errorf("missing %s hint: %q", key, footer)
					}
				}
			}
		}
	}
	m := shown(t, false, sample(), 120, 60)
	if footer := footerOf(t, m); !strings.Contains(footer, "t timeline · c calendar") {
		t.Fatal(footer)
	}
}

func TestCalendarFitAndLoadingHints(t *testing.T) {
	for _, banner := range []bool{false, true} {
		m := New(Options{Fit: true, Banner: banner, Now: func() time.Time { return now }})
		m = resize(t, m, 120, 60)
		m, _ = step(t, m, resultMsg{sample()})
		m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
		_, body, capacity := m.frame()
		if len(body) != capacity || !strings.Contains(strings.Join(body, "\n"), "today") {
			t.Fatal("calendar did not fit model capacity")
		}
	}
	for _, width := range []int{36, 50, 120} {
		for _, haveRes := range []bool{false, true} {
			m := shown(t, false, sample(), width, 12)
			m.loading, m.haveRes = true, haveRes
			footer := footerOf(t, m)
			suffix := "c ↔"
			if width >= 70 {
				suffix = "c calendar"
			}
			if !strings.Contains(footer, suffix) {
				t.Fatalf("width %d haveRes %t: calendar hint cut off: %q", width, haveRes, footer)
			}
		}
	}
}
