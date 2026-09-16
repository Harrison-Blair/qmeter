package update

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	iupdate "github.com/Harrison-Blair/qmeter/internal/update"
)

// newTestRoot builds a root shaped like cmd/root.go and captures the
// Options the update command would run with, so nothing reaches the
// network.
func newTestRoot(t *testing.T, err error) (*cobra.Command, *iupdate.Options, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	var got iupdate.Options
	saved := run
	t.Cleanup(func() { run = saved })
	run = func(_ context.Context, opts iupdate.Options) error {
		got = opts
		return err
	}

	root := &cobra.Command{Use: "qmeter", SilenceUsage: true}
	root.PersistentFlags().Bool("json", false, "output JSON")
	root.AddCommand(New())

	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	return root, &got, &out, &errOut
}

func TestCmd_Metadata(t *testing.T) {
	cmd := New()
	if cmd.Use != "update" {
		t.Errorf("Use = %q, want update", cmd.Use)
	}
	if cmd.Short != "Update qmeter to the latest release" {
		t.Errorf("Short = %q", cmd.Short)
	}
	for _, name := range []string{"check", "yes"} {
		f := cmd.Flags().Lookup(name)
		if f == nil {
			t.Fatalf("update has no --%s flag", name)
		}
		if f.Value.Type() != "bool" {
			t.Errorf("--%s is a %s, want bool", name, f.Value.Type())
		}
		if f.Usage == "" {
			t.Errorf("--%s has no help text", name)
		}
	}
}

func TestCmd_RejectsArguments(t *testing.T) {
	root, _, _, errOut := newTestRoot(t, nil)
	root.SetArgs([]string{"update", "stable"})

	if err := root.Execute(); err == nil {
		t.Fatal("update takes no arguments")
	}
	if errOut.Len() == 0 {
		t.Fatal("nothing on stderr for a bad argument")
	}
}

func TestCmd_FlagsReachTheRunner(t *testing.T) {
	tests := []struct {
		args               []string
		check, yes, asJSON bool
	}{
		{args: []string{"update"}},
		{args: []string{"update", "--check"}, check: true},
		{args: []string{"update", "--yes"}, yes: true},
		{args: []string{"update", "--json"}, asJSON: true},
		{args: []string{"update", "--check", "--json"}, check: true, asJSON: true},
		{args: []string{"update", "--yes", "--check"}, check: true, yes: true},
	}
	for _, tc := range tests {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			root, got, _, _ := newTestRoot(t, nil)
			root.SetArgs(tc.args)

			if err := root.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if got.Check != tc.check {
				t.Errorf("Check = %v, want %v", got.Check, tc.check)
			}
			if got.Yes != tc.yes {
				t.Errorf("Yes = %v, want %v", got.Yes, tc.yes)
			}
			if got.JSON != tc.asJSON {
				t.Errorf("JSON = %v, want %v", got.JSON, tc.asJSON)
			}
		})
	}
}

func TestCmd_WiresTheCommandStreams(t *testing.T) {
	root, got, out, errOut := newTestRoot(t, nil)
	root.SetArgs([]string{"update"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got.Out != out {
		t.Error("Options.Out is not the command's stdout")
	}
	if got.Err != errOut {
		t.Error("Options.Err is not the command's stderr")
	}
	if got.Stdin == nil {
		t.Error("Options.Stdin is nil; the prompt has nothing to read")
	}
}

func TestCmd_ReturnsTheRunnersError(t *testing.T) {
	want := errors.New("boom")
	root, _, _, _ := newTestRoot(t, want)
	root.SetArgs([]string{"update"})

	if err := root.Execute(); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestCmd_DefaultsToTheRealRunner(t *testing.T) {
	// The indirection exists for tests only; the command must ship
	// pointing at internal/update.Run.
	if run == nil {
		t.Fatal("run is nil")
	}
}
