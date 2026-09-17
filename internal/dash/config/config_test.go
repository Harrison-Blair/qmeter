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
	if err := os.WriteFile(path, []byte("meter_width = 73\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.MeterWidth != 73 {
		t.Fatalf("meter width = %d, want 73", got.MeterWidth)
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
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got != Default() {
				t.Fatalf("settings = %#v, want defaults %#v", got, Default())
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
		if err != nil {
			t.Fatalf("parse(%q): %v", input, err)
		}
		if got != Default() {
			t.Fatalf("settings = %#v, want defaults %#v", got, Default())
		}
	}
}

func TestRefreshIntervalDefaultsAndPartialOverride(t *testing.T) {
	if got := Default().RefreshInterval; got != 60*time.Second {
		t.Fatalf("default refresh interval = %s, want 60s", got)
	}

	omitted, err := parse([]byte("meter_width = 73\n"))
	if err != nil {
		t.Fatalf("parse omitted interval: %v", err)
	}
	if omitted.RefreshInterval != 60*time.Second {
		t.Errorf("omitted interval = %s, want 60s", omitted.RefreshInterval)
	}

	overridden, err := parse([]byte("refresh_interval = 17\n"))
	if err != nil {
		t.Fatalf("parse overridden interval: %v", err)
	}
	if overridden.RefreshInterval != 17*time.Second {
		t.Errorf("overridden interval = %s, want 17s", overridden.RefreshInterval)
	}
	if overridden.MeterWidth != DefaultMeterWidth || overridden.Theme != Default().Theme {
		t.Error("refresh-only configuration changed another default")
	}
}

func TestParseFullConfiguration(t *testing.T) {
	got, err := parse([]byte(`
meter_width = 200
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
	if got.MeterWidth != 200 {
		t.Errorf("meter width = %d, want 200", got.MeterWidth)
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
	want := lipgloss.AdaptiveColor{Light: "#ABCDEF", Dark: "#D97757"}
	if color := got.Theme.Accent("claude"); color != want {
		t.Errorf("claude = %#v, want %#v", color, want)
	}
	if color := got.Theme.Accent("codex"); color != Default().Theme.Accent("codex") {
		t.Errorf("partial file changed codex to %#v", color)
	}
}

func TestParseAcceptsMeterWidthEndpoints(t *testing.T) {
	for _, width := range []string{"22", "200"} {
		got, err := parse([]byte("meter_width = " + width + "\n"))
		if err != nil {
			t.Fatalf("width %s: %v", width, err)
		}
		if got.MeterWidth != mustAtoi(t, width) {
			t.Errorf("width = %d, want %s", got.MeterWidth, width)
		}
	}
}

func TestParseAcceptsRefreshIntervalEndpoints(t *testing.T) {
	for _, tc := range []struct {
		seconds int
		want    time.Duration
	}{
		{1, time.Second},
		{86400, 24 * time.Hour},
	} {
		got, err := parse([]byte(fmt.Sprintf("refresh_interval = %d\n", tc.seconds)))
		if err != nil {
			t.Fatalf("interval %d: %v", tc.seconds, err)
		}
		if got.RefreshInterval != tc.want {
			t.Errorf("interval = %s, want %s", got.RefreshInterval, tc.want)
		}
	}
}

func TestParseStrictlyRejectsInvalidFiles(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"unknown top-level key", "surprise = true\n"},
		{"unknown provider", "[colors.future]\nlight = \"#112233\"\n"},
		{"unknown provider field", "[colors.claude]\nbright = \"#112233\"\n"},
		{"duplicate key", "meter_width = 50\nmeter_width = 60\n"},
		{"noninteger width", "meter_width = 50.5\n"},
		{"string width", "meter_width = \"50\"\n"},
		{"null-like value", "[colors.claude]\nlight = null\n"},
		{"wrong color type", "[colors.claude]\nlight = 123\n"},
		{"wrong colors type", "colors = [\"claude\"]\n"},
		{"short color", "[colors.claude]\nlight = \"#123\"\n"},
		{"named color", "[colors.claude]\ndark = \"red\"\n"},
		{"empty color", "[colors.claude]\nlight = \"\"\n"},
		{"width too small", "meter_width = 21\n"},
		{"width too large", "meter_width = 201\n"},
		{"zero refresh interval", "refresh_interval = 0\n"},
		{"negative refresh interval", "refresh_interval = -1\n"},
		{"refresh interval too large", "refresh_interval = 86401\n"},
		{"noninteger refresh interval", "refresh_interval = 1.5\n"},
		{"trailing content", "meter_width = 50\nthis is not toml\n"},
	}
	for _, tc := range tests {
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
		{"malformed", func(string) ([]byte, error) { return []byte("meter_width = ["), nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			readFile = tc.read
			got, err := Load()
			if err == nil {
				t.Fatal("Load succeeded, want an error")
			}
			if !strings.Contains(err.Error(), path) {
				t.Fatalf("error %q does not contain path %q", err, path)
			}
			if strings.Contains(err.Error(), "\n") {
				t.Fatalf("error is not concise: %q", err)
			}
			if got != Default() {
				t.Fatalf("settings = %#v, want defaults %#v", got, Default())
			}
		})
	}
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	if s == "22" {
		return 22
	}
	if s == "200" {
		return 200
	}
	t.Fatalf("unexpected test integer %q", s)
	return 0
}
