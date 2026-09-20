package spend

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/provider/providertest"
	iupdate "github.com/Harrison-Blair/qmeter/internal/update"
	"github.com/spf13/cobra"
)

func testRoot(t *testing.T, ps ...provider.Provider) (*cobra.Command, *bytes.Buffer, *bytes.Buffer, *int) {
	t.Helper()
	savedRegistry, savedHint := registry, hint
	t.Cleanup(func() { registry, hint = savedRegistry, savedHint })
	registry = func() []provider.Provider { return ps }
	calls := 0
	var out, errOut bytes.Buffer
	hint = func(_ context.Context, w io.Writer, _ iupdate.HintOptions) {
		calls++
		if w != &errOut {
			t.Error("hint not sent to stderr")
		}
	}
	root := &cobra.Command{Use: "qmeter", SilenceUsage: true}
	root.PersistentFlags().Bool("json", false, "")
	root.AddCommand(New())
	root.SetOut(&out)
	root.SetErr(&errOut)
	return root, &out, &errOut, &calls
}

func balanceProvider(id string) *providertest.Fake {
	p := providertest.Succeeding(id, []provider.Window{{Name: "not a balance"}})
	zero := 0.0
	p.Balances = []provider.Balance{{Name: "credits", Unit: "credits", Remaining: &zero}}
	return p
}

func TestFilteringAndJSON(t *testing.T) {
	a, b := balanceProvider("claude"), balanceProvider("codex")
	c := providertest.Undetected("cursor", "must remain omitted")
	d := providertest.Erroring("opencode-go", errors.New("synthetic failure"))
	e := balanceProvider("excluded")
	root, out, errOut, hints := testRoot(t, a, b, c, d, e)
	root.SetArgs([]string{"spend", "--filter", "codex, claude ", "--filter", "claude,cursor,opencode-go", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	const want = `{"balances":[{"provider":"claude","name":"credits","unit":"credits","used":null,"limit":null,"remaining":0,"unlimited":false},{"provider":"codex","name":"credits","unit":"credits","used":null,"limit":null,"remaining":0,"unlimited":false}],"errors":[{"provider":"opencode-go","message":"synthetic failure"}],"undetected":[]}` + "\n"
	if out.String() != want {
		t.Errorf("JSON = %s, want %s", out.String(), want)
	}
	if *hints != 0 || errOut.Len() != 0 {
		t.Errorf("JSON hints=%d stderr=%q", *hints, errOut.String())
	}
	if a.Fetches() != 1 || b.Fetches() != 1 || d.Fetches() != 1 || c.Fetches() != 0 || e.Fetches() != 0 {
		t.Fatal("filtering did not fetch each selected detected provider exactly once")
	}
}

func TestDefaultAndTextHint(t *testing.T) {
	a := balanceProvider("codex")
	b := providertest.Succeeding("opencode-go", []provider.Window{{Name: "weekly"}})
	c := providertest.Undetected("cursor", "hidden reason")
	root, out, errOut, hints := testRoot(t, a, b, c)
	root.SetArgs([]string{"spend"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	const want = "PROVIDER  NAME     LEFT  OF  BAR\ncodex     credits  0     -   -\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
	if *hints != 1 || errOut.Len() != 0 {
		t.Errorf("hints=%d stderr=%q", *hints, errOut.String())
	}
	if a.Fetches() != 1 || b.Fetches() != 1 || c.Fetches() != 0 {
		t.Fatal("unexpected fetch count")
	}
}

func TestEmptyAndUndetectedFilter(t *testing.T) {
	for _, args := range [][]string{{"spend"}, {"spend", "--filter", "cursor"}, {"spend", "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			p := providertest.Undetected("cursor", "hidden reason")
			root, out, errOut, hints := testRoot(t, p)
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			want, wantHints := "no balances reported\n", 1
			if args[len(args)-1] == "--json" {
				want = "{\"balances\":[],\"errors\":[],\"undetected\":[]}\n"
				wantHints = 0
			}
			if out.String() != want || *hints != wantHints || errOut.Len() != 0 || p.Fetches() != 0 {
				t.Fatalf("output=%q hints=%d stderr=%q fetch=%d", out.String(), *hints, errOut.String(), p.Fetches())
			}
		})
	}
}

func TestInvalidInputs(t *testing.T) {
	for _, args := range [][]string{{"spend", "--filter", "claude,nope"}, {"spend", "argument"}, {"spend", "--bogus"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			p := balanceProvider("claude")
			root, out, errOut, hints := testRoot(t, p)
			root.SetArgs(args)
			if err := root.Execute(); err == nil {
				t.Fatal("expected error")
			}
			if p.Fetches() != 0 || out.Len() != 0 || errOut.Len() == 0 || *hints != 0 {
				t.Fatalf("fetch=%d output=%q stderr=%q hints=%d", p.Fetches(), out.String(), errOut.String(), *hints)
			}
			if args[1] == "--filter" && errOut.String() != "unknown provider \"nope\" (valid: claude)\n" {
				t.Errorf("unexpected error %q", errOut.String())
			}
		})
	}
}

func TestCancellation(t *testing.T) {
	for _, args := range [][]string{{"spend"}, {"spend", "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			p := providertest.Slow("claude", time.Hour, nil)
			root, out, errOut, hints := testRoot(t, p)
			root.SetArgs(args)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if err := root.ExecuteContext(ctx); err != nil {
				t.Fatal(err)
			}
			if out.Len() != 0 || errOut.Len() != 0 || *hints != 0 {
				t.Fatal("canceled run produced output or a hint")
			}
		})
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestOutputFailureSkipsHint(t *testing.T) {
	root, _, _, hints := testRoot(t, balanceProvider("codex"))
	failure := errors.New("output failed")
	root.SetOut(failingWriter{failure})
	root.SetArgs([]string{"spend"})
	if err := root.Execute(); !errors.Is(err, failure) {
		t.Errorf("error=%v, want %v", err, failure)
	}
	if *hints != 0 {
		t.Fatal("hint after output failure")
	}
}
