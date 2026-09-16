package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/provider/providertest"
	iupdate "github.com/Harrison-Blair/qmeter/internal/update"
	"github.com/Harrison-Blair/qmeter/internal/usage"
)

// captureHint replaces the passive update hint with a recorder, so no test
// in this package reaches github.com, and returns the writers it was handed
// (one per call).
func captureHint(t *testing.T) *[]io.Writer {
	t.Helper()
	saved := hint
	t.Cleanup(func() { hint = saved })
	var calls []io.Writer
	hint = func(_ context.Context, w io.Writer, _ iupdate.HintOptions) {
		calls = append(calls, w)
	}
	return &calls
}

func TestCmd_TextOutputPrintsTheUpdateHintToStderr(t *testing.T) {
	calls := captureHint(t)
	root, _, errOut := newTestRoot(t,
		providertest.Succeeding("claude", []provider.Window{window("claude", "5h")}),
	)
	root.SetArgs([]string{"usage"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("hint ran %d times, want exactly 1", len(*calls))
	}
	if (*calls)[0] != errOut {
		t.Fatal("the hint was not written to the command's stderr")
	}
}

func TestCmd_JSONOutputNeverChecksForUpdates(t *testing.T) {
	calls := captureHint(t)
	root, _, _ := newTestRoot(t,
		providertest.Succeeding("claude", []provider.Window{window("claude", "5h")}),
	)
	root.SetArgs([]string{"usage", "--json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("hint ran %d times in JSON mode, want 0", len(*calls))
	}
}

func TestCmd_CancelledRunNeverChecksForUpdates(t *testing.T) {
	calls := captureHint(t)
	root, _, _ := newTestRoot(t,
		providertest.Succeeding("claude", []provider.Window{window("claude", "5h")}),
	)
	root.SetArgs([]string{"usage"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := root.ExecuteContext(ctx); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("hint ran %d times on a cancelled run, want 0", len(*calls))
	}
}

func TestCmd_UnknownProviderNeverChecksForUpdates(t *testing.T) {
	calls := captureHint(t)
	root, _, _ := newTestRoot(t,
		providertest.Succeeding("claude", []provider.Window{window("claude", "5h")}),
	)
	root.SetArgs([]string{"usage", "--provider", "nope"})

	if err := root.Execute(); err == nil {
		t.Fatal("expected an error")
	}
	if len(*calls) != 0 {
		t.Fatalf("hint ran %d times on a failed run, want 0", len(*calls))
	}
}

func TestCmd_DefaultsToTheRealHint(t *testing.T) {
	if hint == nil {
		t.Fatal("hint is nil")
	}
}

// newTestRoot builds a root command shaped like cmd/root.go — the
// persistent --json flag and SilenceUsage — with the usage subcommand
// attached, and points the command's registry at the given fakes for the
// duration of the test so nothing reads a real credential store or the
// network.
func newTestRoot(t *testing.T, providers ...provider.Provider) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	saved := registry
	t.Cleanup(func() { registry = saved })
	registry = func() []provider.Provider { return providers }

	root := &cobra.Command{Use: "qmeter", SilenceUsage: true}
	root.PersistentFlags().Bool("json", false, "output JSON")
	root.AddCommand(New())

	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	return root, &out, &errOut
}

func window(id, name string) provider.Window {
	return provider.Window{Provider: id, Name: name, Plan: "max", RemainingPercent: 10}
}

func TestCmd_ProviderFlagRestrictsOutput(t *testing.T) {
	root, out, _ := newTestRoot(t,
		providertest.Succeeding("claude", []provider.Window{window("claude", "5h")}),
		providertest.Succeeding("codex", []provider.Window{window("codex", "weekly")}),
	)
	root.SetArgs([]string{"usage", "--provider", "claude"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "claude") {
		t.Fatalf("missing claude row: %q", out.String())
	}
	if strings.Contains(out.String(), "codex") {
		t.Fatalf("codex was not filtered out: %q", out.String())
	}
}

func TestCmd_ProviderFlagOnUndetectedProviderShowsReason(t *testing.T) {
	root, out, errOut := newTestRoot(t,
		providertest.Succeeding("claude", []provider.Window{window("claude", "5h")}),
		providertest.Undetected("codex", "not logged in, run codex to log in"),
	)
	root.SetArgs([]string{"usage", "--provider", "codex"})

	// An existing-but-undetected provider is not an error: exit 0.
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := "codex     not detected: not logged in, run codex to log in\n"
	if !strings.HasSuffix(out.String(), want) {
		t.Fatalf("got %q, want it to end with %q", out.String(), want)
	}
	if errOut.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", errOut.String())
	}
}

func TestCmd_UnknownProviderExitsNonZero(t *testing.T) {
	root, out, errOut := newTestRoot(t,
		providertest.Succeeding("claude", []provider.Window{window("claude", "5h")}),
	)
	root.SetArgs([]string{"usage", "--provider", "nope"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error so main exits 1")
	}
	const want = "unknown provider \"nope\" (valid: claude, codex, opencode-go, cursor)\n"
	if got := errOut.String(); got != want {
		t.Fatalf("stderr = %q, want exactly %q", got, want)
	}
	if out.Len() != 0 {
		t.Fatalf("unexpected stdout: %q", out.String())
	}
}

func TestCmd_UnknownArgumentIsReported(t *testing.T) {
	root, _, errOut := newTestRoot(t,
		providertest.Succeeding("claude", []provider.Window{window("claude", "5h")}),
	)
	root.SetArgs([]string{"usage", "foo"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for an unexpected argument")
	}
	// Silencing cobra's own error reporting for the whole command would
	// make this exit 1 with an empty stderr — a silent failure.
	if errOut.Len() == 0 {
		t.Fatalf("nothing on stderr for %v", err)
	}
}

func TestCmd_UnknownFlagIsReported(t *testing.T) {
	root, _, errOut := newTestRoot(t,
		providertest.Succeeding("claude", []provider.Window{window("claude", "5h")}),
	)
	root.SetArgs([]string{"usage", "--bogus"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for an unknown flag")
	}
	if errOut.Len() == 0 {
		t.Fatalf("nothing on stderr for %v", err)
	}
}

func TestCmd_CancelledContextPrintsNothingAndExitsZero(t *testing.T) {
	for _, args := range [][]string{{"usage"}, {"usage", "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root, out, errOut := newTestRoot(t,
				providertest.Succeeding("claude", []provider.Window{window("claude", "5h")}),
			)
			root.SetArgs(args)

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			if err := root.ExecuteContext(ctx); err != nil {
				t.Fatalf("a cancelled run is not an error: %v", err)
			}
			if out.Len() != 0 {
				t.Fatalf("a cancelled run printed output: %q", out.String())
			}
			if errOut.Len() != 0 {
				t.Fatalf("a cancelled run printed to stderr: %q", errOut.String())
			}
		})
	}
}

func TestCmd_ValidProvidersMatchesTheRealRegistry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	ids := make([]string, 0, 4)
	for _, p := range usage.Registry() {
		ids = append(ids, p.ID())
	}
	if want := strings.Join(ids, ", "); validProviders != want {
		t.Fatalf("validProviders = %q, want %q", validProviders, want)
	}
}

func TestCmd_RootJSONFlagSelectsJSONRenderer(t *testing.T) {
	root, out, _ := newTestRoot(t,
		providertest.Succeeding("claude", []provider.Window{window("claude", "5h")}),
	)
	root.SetArgs([]string{"usage", "--json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var env map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output is not JSON (%v): %q", err, out.String())
	}
	for _, key := range []string{"windows", "errors", "undetected"} {
		if _, ok := env[key]; !ok {
			t.Fatalf("envelope is missing key %q: %q", key, out.String())
		}
	}
	if !strings.Contains(out.String(), `"remaining_percent"`) {
		t.Fatalf("windows were not rendered through Window.MarshalJSON: %q", out.String())
	}
}

func TestCmd_DefaultsToTheRealRegistry(t *testing.T) {
	// The real registry reads credential paths under the home directory
	// when detecting, so keep this test away from the developer's stores;
	// it only constructs providers, it never runs the command.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	got := make([]string, 0, 4)
	for _, p := range registry() {
		got = append(got, p.ID())
	}
	want := strings.Join([]string{"claude", "codex", "opencode-go", "cursor"}, ",")
	if strings.Join(got, ",") != want {
		t.Fatalf("registry() ids = %v, want %v", got, want)
	}
	if len(usage.Registry()) != len(got) {
		t.Fatal("registry() is not usage.Registry")
	}
}

func TestCmd_ProviderFlagIsLocalAndDocumented(t *testing.T) {
	cmd := New()
	flag := cmd.Flags().Lookup("provider")
	if flag == nil {
		t.Fatal("usage has no --provider flag")
	}
	if flag.Usage == "" {
		t.Fatal("--provider has no help text")
	}
	if !strings.Contains(flag.Usage, "opencode-go") {
		t.Fatalf("--provider help does not list the valid names: %q", flag.Usage)
	}
}
