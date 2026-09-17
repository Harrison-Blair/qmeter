package dash

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	idash "github.com/Harrison-Blair/qmeter/internal/dash"
	dconfig "github.com/Harrison-Blair/qmeter/internal/dash/config"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/provider/providertest"
	iupdate "github.com/Harrison-Blair/qmeter/internal/update"
	"github.com/Harrison-Blair/qmeter/internal/usage"
)

// testRoot is a root command shaped like cmd/root.go — the persistent
// --json flag and SilenceUsage — with the dashboard attached and every
// collaborator replaced, so no test in this package reaches a credential
// store, the network or a terminal.
type testRoot struct {
	cmd    *cobra.Command
	out    *bytes.Buffer
	errOut *bytes.Buffer

	hints       []io.Writer          // one entry per update-hint call
	runs        []idash.RunOptions   // one entry per dashboard run
	fakes       []*providertest.Fake // the stubbed registry, in order
	configLoads int
}

func newTestRoot(t *testing.T, terminal bool) *testRoot {
	t.Helper()

	tr := &testRoot{
		fakes: []*providertest.Fake{
			providertest.Succeeding("claude", []provider.Window{{Provider: "claude", Name: "5h", Plan: "max", RemainingPercent: 68}}),
			providertest.Succeeding("codex", []provider.Window{{Provider: "codex", Name: "weekly", Plan: "pro", RemainingPercent: 99}}),
			providertest.Succeeding("opencode-go", []provider.Window{{Provider: "opencode-go", Name: "monthly", RemainingPercent: 5}}),
			providertest.Succeeding("cursor", []provider.Window{{Provider: "cursor", Name: "total", RemainingPercent: 50}}),
		},
	}

	savedRegistry, savedHint, savedRun, savedTerm, savedLoad := registry, hint, run, stdoutIsTerminal, loadConfig
	t.Cleanup(func() {
		registry, hint, run, stdoutIsTerminal, loadConfig = savedRegistry, savedHint, savedRun, savedTerm, savedLoad
	})

	registry = func() []provider.Provider {
		out := make([]provider.Provider, len(tr.fakes))
		for i, f := range tr.fakes {
			out[i] = f
		}
		return out
	}
	hint = func(_ context.Context, w io.Writer, _ iupdate.HintOptions) {
		tr.hints = append(tr.hints, w)
	}
	run = func(_ context.Context, opts idash.RunOptions) error {
		tr.runs = append(tr.runs, opts)
		return nil
	}
	stdoutIsTerminal = func() bool { return terminal }
	loadConfig = func() (dconfig.Settings, error) {
		tr.configLoads++
		return dconfig.Default(), nil
	}

	root := &cobra.Command{Use: "qmeter", SilenceUsage: true}
	root.PersistentFlags().Bool("json", false, "output JSON")
	Attach(root)

	tr.cmd = root
	tr.out, tr.errOut = &bytes.Buffer{}, &bytes.Buffer{}
	root.SetOut(tr.out)
	root.SetErr(tr.errOut)
	return tr
}

func (tr *testRoot) execute(t *testing.T, args ...string) error {
	t.Helper()
	tr.cmd.SetArgs(args)
	return tr.cmd.Execute()
}

// fetched is the IDs of the providers whose Fetch ran, in registry order.
func (tr *testRoot) fetched() []string {
	var out []string
	for _, f := range tr.fakes {
		if f.Fetches() > 0 {
			out = append(out, f.IDValue)
		}
	}
	return out
}

func providerIDs(ps []provider.Provider) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.ID()
	}
	return out
}

func joined(ids []string) string { return strings.Join(ids, ",") }

func TestCmd_JSONPrintsTheSameEnvelopeAsUsage(t *testing.T) {
	tr := newTestRoot(t, true)

	if err := tr.execute(t, "--json"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var env struct {
		Windows []struct {
			Provider string `json:"provider"`
		} `json:"windows"`
	}
	if err := json.Unmarshal(tr.out.Bytes(), &env); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, tr.out.String())
	}
	if len(env.Windows) != 4 {
		t.Fatalf("envelope has %d windows, want 4: %s", len(env.Windows), tr.out.String())
	}
	if len(tr.runs) != 0 {
		t.Error("--json started the dashboard")
	}
	if len(tr.hints) != 0 {
		t.Errorf("--json checked for updates %d times, want 0", len(tr.hints))
	}
}

func TestCmd_FilterNarrowsTheProviders(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "one name", args: []string{"--filter", "codex"}, want: "codex"},
		{name: "comma separated", args: []string{"--filter", "codex,claude"}, want: "claude,codex"},
		{name: "repeated", args: []string{"--filter", "cursor", "--filter", "claude"}, want: "claude,cursor"},
		{name: "absent", args: nil, want: "claude,codex,opencode-go,cursor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := newTestRoot(t, true)
			if err := tr.execute(t, append([]string{"--json"}, tc.args...)...); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if got := joined(tr.fetched()); got != tc.want {
				t.Fatalf("fetched %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCmd_UnknownFilterNameFailsBeforeAnyFetch(t *testing.T) {
	tr := newTestRoot(t, true)

	err := tr.execute(t, "--filter", "claude,nope")
	if err == nil {
		t.Fatal("expected an error")
	}
	want := `unknown provider "nope" (valid: claude, codex, opencode-go, cursor)` + "\n"
	if got := tr.errOut.String(); got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
	if got := tr.fetched(); got != nil {
		t.Errorf("providers %v were fetched, want none", got)
	}
	if len(tr.runs) != 0 {
		t.Error("the dashboard ran despite an unknown provider")
	}
	if len(tr.hints) != 0 {
		t.Error("a failed run checked for updates")
	}
}

func TestCmd_PipedOutputPrintsTheTableAndChecksForUpdates(t *testing.T) {
	tr := newTestRoot(t, false)

	if err := tr.execute(t); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(tr.runs) != 0 {
		t.Fatal("the dashboard ran on a pipe")
	}
	out := tr.out.String()
	for _, want := range []string{"PROVIDER", "claude", "codex", "cursor"} {
		if !strings.Contains(out, want) {
			t.Errorf("piped output = %q, want it to contain %q", out, want)
		}
	}
	if len(tr.hints) != 1 {
		t.Fatalf("the update hint ran %d times, want 1", len(tr.hints))
	}
	if tr.hints[0] != tr.errOut {
		t.Error("the hint was not written to stderr")
	}
}

func TestCmd_PipedOutputHonoursTheFilter(t *testing.T) {
	tr := newTestRoot(t, false)

	if err := tr.execute(t, "--filter", "cursor"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := joined(tr.fetched()); got != "cursor" {
		t.Fatalf("fetched %q, want cursor", got)
	}
}

func TestCmd_TerminalRunsTheDashboard(t *testing.T) {
	tr := newTestRoot(t, true)

	if err := tr.execute(t); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(tr.runs) != 1 {
		t.Fatalf("the dashboard ran %d times, want 1", len(tr.runs))
	}
	got := tr.runs[0]
	if !got.Banner {
		t.Error("the dashboard ran without the banner")
	}
	if ids := joined(providerIDs(got.Providers)); ids != "claude,codex,opencode-go,cursor" {
		t.Errorf("the dashboard got providers %q, want all four", ids)
	}
	if tr.out.Len() != 0 {
		t.Errorf("the dashboard path also printed %q", tr.out.String())
	}
	if len(tr.hints) != 1 {
		t.Fatalf("the update hint ran %d times after the dashboard, want 1", len(tr.hints))
	}
	if tr.hints[0] != tr.errOut {
		t.Error("the hint was not written to stderr")
	}
}

func TestCmd_InteractiveDashboardLoadsAndPassesConfiguration(t *testing.T) {
	tr := newTestRoot(t, true)
	want := dconfig.Default()
	want.MeterWidth = 83
	want.Theme.Claude.Light = "#010203"
	want.RefreshInterval = 17 * time.Second
	loadConfig = func() (dconfig.Settings, error) {
		tr.configLoads++
		return want, nil
	}

	if err := tr.execute(t); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if tr.configLoads != 1 {
		t.Fatalf("configuration loaded %d times, want 1", tr.configLoads)
	}
	if len(tr.runs) != 1 {
		t.Fatalf("dashboard ran %d times, want 1", len(tr.runs))
	}
	if got := tr.runs[0]; got.MeterWidth != want.MeterWidth || got.Theme != want.Theme || got.RefreshInterval != want.RefreshInterval {
		t.Errorf("dashboard settings = (%d, %#v, %s), want (%d, %#v, %s)",
			got.MeterWidth, got.Theme, got.RefreshInterval, want.MeterWidth, want.Theme, want.RefreshInterval)
	}
}

func TestCmd_InvalidConfigurationWarnsOnceAndUsesAllDefaults(t *testing.T) {
	tr := newTestRoot(t, true)
	loadConfig = func() (dconfig.Settings, error) {
		tr.configLoads++
		partial := dconfig.Default()
		partial.MeterWidth = 83
		partial.Theme.Claude.Light = "#010203"
		partial.RefreshInterval = 17 * time.Second
		return partial, errors.New("/tmp/qmeter/config.toml: invalid color")
	}

	if err := tr.execute(t); err != nil {
		t.Fatalf("execute: %v", err)
	}
	wantWarning := "warning: /tmp/qmeter/config.toml: invalid color\n"
	if got := tr.errOut.String(); got != wantWarning {
		t.Errorf("stderr = %q, want %q", got, wantWarning)
	}
	if len(tr.runs) != 1 {
		t.Fatalf("dashboard ran %d times, want 1", len(tr.runs))
	}
	want := dconfig.Default()
	if got := tr.runs[0]; got.MeterWidth != want.MeterWidth || got.Theme != want.Theme || got.RefreshInterval != want.RefreshInterval {
		t.Errorf("dashboard did not use all defaults after invalid config: %#v", got)
	}
}

func TestCmd_NonInteractiveRoutesDoNotLoadDashboardConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name     string
		terminal bool
		args     []string
	}{
		{"piped", false, nil},
		{"json", true, []string{"--json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := newTestRoot(t, tc.terminal)
			if err := tr.execute(t, tc.args...); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if tr.configLoads != 0 {
				t.Errorf("configuration loaded %d times, want 0", tr.configLoads)
			}
		})
	}
}

func TestCmd_NoBannerReachesTheDashboard(t *testing.T) {
	tr := newTestRoot(t, true)

	if err := tr.execute(t, "--no-banner", "--filter", "claude"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(tr.runs) != 1 {
		t.Fatalf("the dashboard ran %d times, want 1", len(tr.runs))
	}
	if tr.runs[0].Banner {
		t.Error("--no-banner still ran with the banner")
	}
	if ids := joined(providerIDs(tr.runs[0].Providers)); ids != "claude" {
		t.Errorf("the dashboard got providers %q, want claude", ids)
	}
}

func TestCmd_AFailingDashboardFailsTheCommand(t *testing.T) {
	tr := newTestRoot(t, true)
	run = func(context.Context, idash.RunOptions) error { return io.ErrUnexpectedEOF }

	if err := tr.execute(t); err == nil {
		t.Fatal("expected the dashboard's error")
	}
	if len(tr.hints) != 0 {
		t.Error("a failed dashboard checked for updates")
	}
}

func TestCmd_CancelledRunPrintsNothing(t *testing.T) {
	tr := newTestRoot(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tr.cmd.SetArgs(nil)
	if err := tr.cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if tr.out.Len() != 0 {
		t.Errorf("a cancelled run printed %q", tr.out.String())
	}
	if len(tr.hints) != 0 {
		t.Error("a cancelled run checked for updates")
	}
}

func TestCmd_RejectsArguments(t *testing.T) {
	tr := newTestRoot(t, true)

	err := tr.execute(t, "bogus")
	if err == nil {
		t.Fatal("expected an error for an unknown command")
	}
	if !strings.Contains(err.Error(), `unknown command "bogus"`) {
		t.Fatalf("error = %v, want an unknown-command error", err)
	}
	if len(tr.runs) != 0 {
		t.Error("the dashboard ran for an unknown command")
	}
}

func TestCmd_ValidProvidersMatchesTheRealRegistry(t *testing.T) {
	// The constant is what --filter's help text offers and what its error
	// calls valid, so a provider added to (or renamed in) the registry must
	// show up here too.
	want := strings.Join(providerIDs(usage.Registry()), ", ")
	if validProviders != want {
		t.Fatalf("validProviders = %q, want %q", validProviders, want)
	}
}

func TestCmd_DefaultsToTheRealCollaborators(t *testing.T) {
	if registry == nil || hint == nil || run == nil || stdoutIsTerminal == nil || loadConfig == nil {
		t.Fatal("a collaborator is nil")
	}
	// Calling it proves the default reads the real stdout rather than
	// panicking on a nil file; the answer depends on how the tests run.
	_ = stdoutIsTerminal()
}
