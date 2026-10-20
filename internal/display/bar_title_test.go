package display

import (
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/mattn/go-runewidth"
)

func TestBarShortensResetBeforeTruncatingName(t *testing.T) {
	window := provider.Window{
		Name:             "fable weekly",
		RemainingPercent: 96,
		ResetsAt:         barNow.Add(6*24*time.Hour + 8*time.Hour),
	}
	title := Bar(window, barNow, 35, plainRenderer())[0]
	if got, want := strings.TrimRight(title, " "), "▸ fable weekly 96.0% · 6d8h"; got != want {
		t.Fatalf("title = %q, want %q", got, want)
	}
}

func TestBarKeepsTheWindowNameAndShortensTheRightSideFirst(t *testing.T) {
	window := provider.Window{
		Name:             "automatically",
		RemainingPercent: 0,
		ResetsAt:         barNow.Add(3*24*time.Hour + 12*time.Hour),
		RateLimited:      true,
	}
	title := Bar(window, barNow, 30, plainRenderer())[0]
	left, _, ok := strings.Cut(title, "0.0%")
	if !ok {
		t.Fatalf("title has no percentage: %q", title)
	}
	name := strings.TrimSpace(strings.TrimPrefix(left, "▸ "))
	if width := runewidth.StringWidth(name); width < 8 {
		t.Fatalf("visible window name %q has %d cells, want at least 8", name, width)
	}
	if !strings.Contains(title, " · 3d12h · RL") {
		t.Fatalf("title did not shorten reset and rate-limit text in order: %q", title)
	}
	if strings.Contains(title, "resets in") || strings.Contains(title, "rate limited") {
		t.Fatalf("title kept long right-side text after it ran out of room: %q", title)
	}
}
