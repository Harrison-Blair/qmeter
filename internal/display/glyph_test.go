package display

import "testing"

func TestProviderGlyph(t *testing.T) {
	for id, want := range map[string]string{"claude": "◆", "codex": "●", "opencode-go": "○", "cursor": "▲", "new": "•"} {
		if got := ProviderGlyph(id); got != want {
			t.Errorf("%s glyph = %q, want %q", id, got, want)
		}
	}
}
