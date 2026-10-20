package dash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	dconfig "github.com/Harrison-Blair/qmeter/internal/dash/config"
)

func TestRemovedFlagsUseCobraUnknownFlagErrors(t *testing.T) {
	noBanner := "--no-" + "ban" + "ner"
	for _, args := range [][]string{{"--width", "50"}, {"--thickness", "3"}, {"--vertical"}, {noBanner}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			tr := newTestRoot(t, true)
			err := tr.execute(t, args...)
			if err == nil || !strings.Contains(err.Error(), "unknown flag") {
				t.Fatalf("execute error = %v, want Cobra's unknown-flag error", err)
			}
			if len(tr.runs) != 0 {
				t.Fatal("dashboard ran with a removed flag")
			}
		})
	}
}

func TestRemovedConfigKeyWarningKeepsColorOverrideInDashboard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "qmeter", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("meter_width = 50\n[colors.claude]\nlight = \"#010203\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)

	tr := newTestRoot(t, true)
	loadConfig = dconfig.Load
	if err := tr.execute(t); err != nil {
		t.Fatalf("execute: %v", err)
	}
	wantWarning := "warning: " + path + ": ignoring removed key \"meter_width\"\n"
	if got := tr.errOut.String(); got != wantWarning {
		t.Fatalf("stderr = %q, want %q", got, wantWarning)
	}
	if len(tr.runs) != 1 || tr.runs[0].Theme.Claude.Light != "#010203" {
		t.Fatalf("dashboard options = %#v, want the color override applied", tr.runs)
	}
}
