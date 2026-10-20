package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestLoadUsesTheOSConfigDirectory(t *testing.T) {
	dir := t.TempDir()
	saved := userConfigDir
	t.Cleanup(func() { userConfigDir = saved })
	userConfigDir = func() (string, error) { return dir, nil }
	path := filepath.Join(dir, "qmeter", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("refresh_interval = 73\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.RefreshInterval != 73*time.Second {
		t.Fatalf("refresh interval = %s, want 73s", got.RefreshInterval)
	}
}

func TestLoadMissingFileAndUnavailableConfigDirUseDefaults(t *testing.T) {
	saved := userConfigDir
	t.Cleanup(func() { userConfigDir = saved })
	for _, tc := range []struct {
		name string
		dir  func() (string, error)
	}{
		{"missing file", func() (string, error) { return t.TempDir(), nil }},
		{"unavailable directory", func() (string, error) { return "", errors.New("no home") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			userConfigDir = tc.dir
			got, err := Load()
			if err != nil || got != Default() {
				t.Fatalf("Load = (%#v, %v), want defaults", got, err)
			}
		})
	}
}

func TestLoadDoesNotCreateAMissingConfiguration(t *testing.T) {
	dir := t.TempDir()
	saved := userConfigDir
	t.Cleanup(func() { userConfigDir = saved })
	userConfigDir = func() (string, error) { return dir, nil }
	if _, err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "qmeter")); !os.IsNotExist(err) {
		t.Fatalf("Load created the config directory or returned an unexpected stat error: %v", err)
	}
}

func TestParseEmptyAndCommentOnlyUseDefaults(t *testing.T) {
	for _, input := range []string{"", "# qmeter settings\n\n"} {
		got, err := parse([]byte(input))
		if err != nil || got != Default() {
			t.Fatalf("parse(%q) = (%#v, %v), want defaults", input, got, err)
		}
	}
}

func TestRefreshIntervalDefaultsAndPartialOverride(t *testing.T) {
	if got := Default().RefreshInterval; got != 60*time.Second {
		t.Fatalf("default refresh interval = %s, want 60s", got)
	}
	omitted, err := parse([]byte("[colors.claude]\nlight = \"#010203\"\n"))
	if err != nil {
		t.Fatalf("parse partial configuration: %v", err)
	}
	if omitted.RefreshInterval != 60*time.Second {
		t.Errorf("omitted interval = %s, want 60s", omitted.RefreshInterval)
	}
	overridden, err := parse([]byte("refresh_interval = 17\n"))
	if err != nil {
		t.Fatalf("parse overridden interval: %v", err)
	}
	if overridden.RefreshInterval != 17*time.Second || overridden.Theme != Default().Theme {
		t.Errorf("partial configuration = %#v", overridden)
	}
}

func TestParseFullConfiguration(t *testing.T) {
	got, err := parse([]byte(`
refresh_interval = 86400

[colors.claude]
light = "#010203"
dark = "#040506"
[colors.codex]
light = "#111213"
dark = "#141516"
[colors.opencode-go]
light = "#212223"
dark = "#242526"
[colors.cursor]
light = "#313233"
dark = "#343536"
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.RefreshInterval != 24*time.Hour {
		t.Errorf("refresh interval = %s, want 24h", got.RefreshInterval)
	}
	wants := map[string]lipgloss.AdaptiveColor{
		"claude":      {Light: "#010203", Dark: "#040506"},
		"codex":       {Light: "#111213", Dark: "#141516"},
		"opencode-go": {Light: "#212223", Dark: "#242526"},
		"cursor":      {Light: "#313233", Dark: "#343536"},
	}
	for id, want := range wants {
		if color := got.Theme.Accent(id); color != want {
			t.Errorf("%s = %#v, want %#v", id, color, want)
		}
	}
}

func TestParseMergesPartialColorsLeafByLeaf(t *testing.T) {
	got, err := parse([]byte("[colors.claude]\nlight = \"#ABCDEF\"\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if want := (lipgloss.AdaptiveColor{Light: "#ABCDEF", Dark: "#D97757"}); got.Theme.Accent("claude") != want {
		t.Errorf("claude = %#v, want %#v", got.Theme.Accent("claude"), want)
	}
	if got.Theme.Accent("codex") != Default().Theme.Accent("codex") {
		t.Errorf("partial file changed codex to %#v", got.Theme.Accent("codex"))
	}
}

func TestParseAcceptsRefreshIntervalEndpoints(t *testing.T) {
	for _, tc := range []struct {
		seconds int
		want    time.Duration
	}{{1, time.Second}, {86400, 24 * time.Hour}} {
		got, err := parse([]byte(fmt.Sprintf("refresh_interval = %d\n", tc.seconds)))
		if err != nil || got.RefreshInterval != tc.want {
			t.Errorf("interval %d = (%s, %v), want %s", tc.seconds, got.RefreshInterval, err, tc.want)
		}
	}
}

func TestParseStrictlyRejectsInvalidFiles(t *testing.T) {
	for _, tc := range []struct{ name, input string }{
		{"unknown top-level key", "surprise = true\n"},
		{"unknown provider", "[colors.future]\nlight = \"#112233\"\n"},
		{"unknown provider field", "[colors.claude]\nbright = \"#112233\"\n"},
		{"duplicate key", "refresh_interval = 50\nrefresh_interval = 60\n"},
		{"null-like value", "[colors.claude]\nlight = null\n"},
		{"wrong color type", "[colors.claude]\nlight = 123\n"},
		{"wrong colors type", "colors = [\"claude\"]\n"},
		{"short color", "[colors.claude]\nlight = \"#123\"\n"},
		{"named color", "[colors.claude]\ndark = \"red\"\n"},
		{"empty color", "[colors.claude]\nlight = \"\"\n"},
		{"zero refresh interval", "refresh_interval = 0\n"},
		{"negative refresh interval", "refresh_interval = -1\n"},
		{"refresh interval too large", "refresh_interval = 86401\n"},
		{"noninteger refresh interval", "refresh_interval = 1.5\n"},
		{"trailing content", "refresh_interval = 50\nthis is not toml\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parse([]byte(tc.input)); err == nil {
				t.Fatalf("parse(%q) succeeded, want an error", tc.input)
			}
		})
	}
}

func TestLoadExistingUnreadableOrInvalidFileReturnsPathQualifiedErrorAndDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "qmeter", "config.toml")
	savedDir, savedRead := userConfigDir, readFile
	t.Cleanup(func() { userConfigDir, readFile = savedDir, savedRead })
	userConfigDir = func() (string, error) { return dir, nil }
	for _, tc := range []struct {
		name string
		read func(string) ([]byte, error)
	}{
		{"unreadable", func(string) ([]byte, error) { return nil, os.ErrPermission }},
		{"malformed", func(string) ([]byte, error) { return []byte("refresh_interval = ["), nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			readFile = tc.read
			got, err := Load()
			if err == nil || !strings.Contains(err.Error(), path) || strings.Contains(err.Error(), "\n") || got != Default() {
				t.Fatalf("Load = (%#v, %v), want path-qualified error and defaults", got, err)
			}
		})
	}
}
