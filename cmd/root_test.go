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

	for _, name := range []string{"usage", "version", "update"} {
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

func TestRoot_CleansUpALeftoverWindowsBinaryOnEveryRun(t *testing.T) {
	var calls int
	saved := cleanupOld
	t.Cleanup(func() { cleanupOld = saved })
	cleanupOld = func() { calls++ }

	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"version"}, &out); err != nil {
		t.Fatalf("ExecuteWithArgs: %v", err)
	}
	if calls != 1 {
		t.Fatalf("cleanupOld ran %d times, want 1", calls)
	}
}

func TestRoot_CleanupCannotFailTheCommand(t *testing.T) {
	saved := cleanupOld
	t.Cleanup(func() { cleanupOld = saved })
	cleanupOld = func() { panic("leftover cleanup blew up") }

	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"version"}, &out); err != nil {
		t.Fatalf("a failing cleanup must not fail the command: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("the command produced no output")
	}
}
