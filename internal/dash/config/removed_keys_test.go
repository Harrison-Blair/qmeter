package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestLoadWarnsForRemovedKeysAndKeepsOtherSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "qmeter", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("meter_width = 50\nmeter_thickness = 3\n[colors.claude]\nlight = \"#010203\"\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	savedDir, savedRead := userConfigDir, readFile
	t.Cleanup(func() { userConfigDir, readFile = savedDir, savedRead })
	userConfigDir = func() (string, error) { return dir, nil }

	got, err := Load()
	if color := got.Theme.Claude; color != (lipgloss.AdaptiveColor{Light: "#010203", Dark: "#D97757"}) {
		t.Fatalf("claude color = %#v, want the override", color)
	}
	if err == nil {
		t.Fatal("Load returned no warning for removed config keys")
	}
	want := path + ": ignoring removed key \"meter_thickness\"\n" + path + ": ignoring removed key \"meter_width\""
	if got := err.Error(); got != want {
		t.Fatalf("warnings = %q, want %q", got, want)
	}
}
