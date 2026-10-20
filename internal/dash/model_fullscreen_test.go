package dash

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Harrison-Blair/qmeter/internal/dash/layout"
	"github.com/Harrison-Blair/qmeter/internal/dash/theme"
	"github.com/Harrison-Blair/qmeter/internal/usage"
)

func TestViewMatchesScreenAtEveryResize(t *testing.T) {
	m := New(Options{Theme: theme.Default(), Now: func() time.Time { return now }})
	m, _ = step(t, m, resultMsg{res: sample()})

	for _, size := range []struct{ width, height int }{{0, 0}, {79, 23}, {80, 24}, {140, 45}} {
		m, _ = step(t, m, tea.WindowSizeMsg{Width: size.width, Height: size.height})
		want := strings.Join(layout.Screen(sample(), size.width, size.height, layout.ScreenOptions{
			Now:       now,
			Theme:     theme.Default(),
			FetchedAt: now,
		}), "\n")
		if got := m.View(); got != want {
			t.Errorf("View at %dx%d does not match the full screen frame", size.width, size.height)
		}
	}
}

func TestViewShowsFetchTimeInHeader(t *testing.T) {
	fetchedAt := time.Date(2026, 9, 16, 14, 22, 7, 0, time.Local)
	m := New(Options{Now: func() time.Time { return fetchedAt }})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = step(t, m, resultMsg{res: sample()})
	if got := m.View(); !strings.Contains(strings.Split(got, "\n")[0], "fetched 14:22") {
		t.Fatalf("header = %q, want fetched 14:22", strings.Split(got, "\n")[0])
	}
}

func screenShown(t *testing.T, result usage.Result, width, height int) Model {
	t.Helper()
	m := New(Options{Now: func() time.Time { return now }})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: width, Height: height})
	m, _ = step(t, m, resultMsg{res: result})
	return m
}

func TestRemovedKeysDoNothing(t *testing.T) {
	m := screenShown(t, sample(), 80, 24)
	before := m.View()
	for _, key := range []string{"t", "c", "j", "k", "down", "up", "pgdown", "pgup", "g", "G", "home", "end", " ", "b"} {
		next, cmd := press(t, m, key)
		if cmd != nil {
			t.Errorf("key %q returned a command", key)
		}
		if got := next.View(); got != before {
			t.Errorf("key %q changed the frame", key)
		}
		m = next
	}
}
