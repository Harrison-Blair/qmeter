package cmd

import (
	"bytes"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/version"
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
	// Exact match, not a prefix: the root command's own Long text also starts
	// with "qmeter ", so a prefix check would still pass if ExecuteWithArgs
	// stopped applying args and printed the help banner instead.
	want := "qmeter " + version.String() + "\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
