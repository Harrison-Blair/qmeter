package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRoot_RegistersUsageAndPersistentJSONFlag(t *testing.T) {
	root := NewRootCmd()

	if f := root.PersistentFlags().Lookup("json"); f == nil {
		t.Error("root has no persistent --json flag")
	}

	for _, name := range []string{"usage", "version"} {
		found := false
		for _, sub := range root.Commands() {
			if sub.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("root does not register the %q subcommand", name)
		}
	}
}

func TestExecuteWithArgs_Version(t *testing.T) {
	var out bytes.Buffer

	if err := ExecuteWithArgs([]string{"version"}, &out); err != nil {
		t.Fatalf("ExecuteWithArgs(version): %v", err)
	}
	if !strings.HasPrefix(out.String(), "qmeter ") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}
