// Package config loads the dashboard's optional TOML configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/lipgloss"

	"github.com/Harrison-Blair/qmeter/internal/dash/theme"
)

const (
	DefaultRefreshInterval = 60 * time.Second
	minRefreshSeconds      = 1
	maxRefreshSeconds      = 86400
)

// Settings are the validated dashboard presentation settings.
type Settings struct {
	Theme           theme.Theme
	RefreshInterval time.Duration
}

// Warnings reports removed configuration keys that were ignored.
type Warnings []string

func (w Warnings) Error() string { return strings.Join(w, "\n") }

var (
	userConfigDir = os.UserConfigDir
	readFile      = os.ReadFile
	hexColor      = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
)

// Default returns the settings used without a configuration file.
func Default() Settings {
	return Settings{
		Theme:           theme.Default(),
		RefreshInterval: DefaultRefreshInterval,
	}
}

// Load reads the OS-native qmeter/config.toml. A missing file or unavailable
// user configuration directory is the same as no configuration. Every other
// error is path-qualified, and the returned settings are still the defaults so
// callers can warn and continue.
func Load() (Settings, error) {
	dir, err := userConfigDir()
	if err != nil {
		return Default(), nil
	}
	path := filepath.Join(dir, "qmeter", "config.toml")
	data, err := readFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Default(), fmt.Errorf("%s: %w", path, err)
	}
	settings, err := parse(data)
	if warnings, ok := err.(Warnings); ok {
		for i := range warnings {
			warnings[i] = path + ": " + warnings[i]
		}
		return settings, warnings
	}
	if err != nil {
		return Default(), fmt.Errorf("%s: %w", path, err)
	}
	return settings, nil
}

type rawConfig struct {
	RefreshInterval *int      `toml:"refresh_interval"`
	Colors          rawColors `toml:"colors"`
}

type rawColors struct {
	Claude     rawColor `toml:"claude"`
	Codex      rawColor `toml:"codex"`
	OpenCodeGo rawColor `toml:"opencode-go"`
	Cursor     rawColor `toml:"cursor"`
}

type rawColor struct {
	Light *string `toml:"light"`
	Dark  *string `toml:"dark"`
}

func parse(data []byte) (Settings, error) {
	var raw rawConfig
	meta, err := toml.Decode(string(data), &raw)
	if err != nil {
		return Settings{}, err
	}
	var warnings Warnings
	var unknown []string
	for _, key := range meta.Undecoded() {
		name := key.String()
		switch name {
		case "meter_width", "meter_thickness":
			warnings = append(warnings, fmt.Sprintf("ignoring removed key %q", name))
		default:
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		return Settings{}, fmt.Errorf("unknown key %q", unknown[0])
	}
	sort.Strings(warnings)

	settings := Default()
	if raw.RefreshInterval != nil {
		if *raw.RefreshInterval < minRefreshSeconds || *raw.RefreshInterval > maxRefreshSeconds {
			return Settings{}, fmt.Errorf("refresh_interval must be between %d and %d seconds", minRefreshSeconds, maxRefreshSeconds)
		}
		settings.RefreshInterval = time.Duration(*raw.RefreshInterval) * time.Second
	}

	if err := mergeColor("colors.claude", &settings.Theme.Claude, raw.Colors.Claude); err != nil {
		return Settings{}, err
	}
	if err := mergeColor("colors.codex", &settings.Theme.Codex, raw.Colors.Codex); err != nil {
		return Settings{}, err
	}
	if err := mergeColor("colors.opencode-go", &settings.Theme.OpenCodeGo, raw.Colors.OpenCodeGo); err != nil {
		return Settings{}, err
	}
	if err := mergeColor("colors.cursor", &settings.Theme.Cursor, raw.Colors.Cursor); err != nil {
		return Settings{}, err
	}
	if len(warnings) > 0 {
		return settings, warnings
	}
	return settings, nil
}

func mergeColor(name string, dst *lipgloss.AdaptiveColor, src rawColor) error {
	if src.Light != nil {
		if !hexColor.MatchString(*src.Light) {
			return fmt.Errorf("%s.light must be a six-digit hex color", name)
		}
		dst.Light = *src.Light
	}
	if src.Dark != nil {
		if !hexColor.MatchString(*src.Dark) {
			return fmt.Errorf("%s.dark must be a six-digit hex color", name)
		}
		dst.Dark = *src.Dark
	}
	return nil
}
