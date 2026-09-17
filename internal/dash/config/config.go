// Package config loads the dashboard's optional TOML configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/lipgloss"

	"github.com/Harrison-Blair/qmeter/internal/dash/theme"
)

const (
	// DefaultMeterWidth is the preferred complete rendered gauge width.
	DefaultMeterWidth = 50
	MinMeterWidth     = 22
	MaxMeterWidth     = 200

	DefaultRefreshInterval = 60 * time.Second
	minRefreshSeconds      = 1
	maxRefreshSeconds      = 86400
)

// Settings are the validated dashboard presentation settings.
type Settings struct {
	MeterWidth      int
	Theme           theme.Theme
	RefreshInterval time.Duration
}

var (
	userConfigDir = os.UserConfigDir
	readFile      = os.ReadFile
	hexColor      = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
)

// Default returns the settings used without a configuration file.
func Default() Settings {
	return Settings{
		MeterWidth:      DefaultMeterWidth,
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
	if err != nil {
		return Default(), fmt.Errorf("%s: %w", path, err)
	}
	return settings, nil
}

type rawConfig struct {
	MeterWidth      *int      `toml:"meter_width"`
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
	if keys := meta.Undecoded(); len(keys) > 0 {
		return Settings{}, fmt.Errorf("unknown key %q", keys[0].String())
	}

	settings := Default()
	if raw.MeterWidth != nil {
		if *raw.MeterWidth < MinMeterWidth || *raw.MeterWidth > MaxMeterWidth {
			return Settings{}, fmt.Errorf("meter_width must be between %d and %d", MinMeterWidth, MaxMeterWidth)
		}
		settings.MeterWidth = *raw.MeterWidth
	}
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
