package usage

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/provider/providertest"
	"github.com/Harrison-Blair/qmeter/internal/usage"
)

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
	return provider.Window{Provider: id, Name: name, Plan: "max", UsedPercent: 10}
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
	if !strings.Contains(out.String(), `"used_percent"`) {
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
