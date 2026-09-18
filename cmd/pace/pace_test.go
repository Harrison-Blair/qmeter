package pace

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
	savedRegistry, savedHint, savedNow := registry, hint, now
	t.Cleanup(func() { registry, hint, now = savedRegistry, savedHint, savedNow })
	registry = func() []provider.Provider { return ps }
	calls := 0
	var out, errOut bytes.Buffer
	hint = func(_ context.Context, w io.Writer, _ iupdate.HintOptions) {
		calls++
		if w != &errOut {
			t.Error("hint not stderr")
		}
	}
	now = func() time.Time { return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC) }
	root := &cobra.Command{Use: "qmeter", SilenceUsage: true}
	root.PersistentFlags().Bool("json", false, "")
	root.AddCommand(New())
	root.SetOut(&out)
	root.SetErr(&errOut)
	return root, &out, &errOut, &calls
}

func TestFilteringAndJSON(t *testing.T) {
	a := providertest.Succeeding("claude", []provider.Window{{Name: "weekly", RemainingPercent: 65, ResetsAt: time.Date(2026, 9, 18, 12, 50, 0, 0, time.UTC), Period: 100 * time.Minute}})
	b := providertest.Succeeding("codex", []provider.Window{{Name: "credits", RemainingPercent: 50}})
	c := providertest.Undetected("cursor", "secret missing reason")
	d := providertest.Erroring("opencode-go", errors.New("bad request"))
	root, out, _, calls := testRoot(t, a, b, c, d)
	timeCalls := 0
	now = func() time.Time {
		timeCalls++
		if a.Fetches() != 1 || b.Fetches() != 1 || d.Fetches() != 1 {
			t.Error("clock before fetch completed")
		}
		return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	}
	root.SetArgs([]string{"pace", "--filter", "codex, claude ", "--filter", "claude,cursor,opencode-go", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if *calls != 0 || timeCalls != 1 {
		t.Fatalf("hint calls %d time calls %d", *calls, timeCalls)
	}
	s := out.String()
	if strings.Count(s, `"provider":"claude"`) != 1 || strings.Index(s, `"provider":"claude"`) > strings.Index(s, `"provider":"codex"`) || strings.Contains(s, "secret missing reason") || !strings.Contains(s, `"pace":"behind"`) || !strings.Contains(s, `"message":"bad request"`) {
		t.Fatal(s)
	}
}

func TestTextAndEmptyAndFilter(t *testing.T) {
	for _, filter := range []string{"claude", "cursor", " , "} {
		t.Run(filter, func(t *testing.T) {
			a := providertest.Succeeding("claude", []provider.Window{{Name: "credits", RemainingPercent: 50}})
			b := providertest.Undetected("cursor", "missing")
			root, out, _, calls := testRoot(t, a, b)
			root.SetArgs([]string{"pace", "--filter", filter})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if *calls != 1 {
				t.Fatal("missing hint")
			}
			if filter == "cursor" {
				if out.String() != "no providers detected\n" || a.Fetches() != 0 {
					t.Fatal(out.String())
				}
			} else if !strings.Contains(out.String(), "n/a") {
				t.Fatal(out.String())
			}
		})
	}
}

func TestInvalidInputs(t *testing.T) {
	for _, args := range [][]string{{"pace", "--filter", "nope"}, {"pace", "argument"}, {"pace", "--bogus"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			a := providertest.Succeeding("claude", nil)
			root, out, errOut, calls := testRoot(t, a)
			root.SetArgs(args)
			if err := root.Execute(); err == nil {
				t.Fatal("expected error")
			}
			if a.Fetches() != 0 || out.Len() != 0 || errOut.Len() == 0 || *calls != 0 {
				t.Fatalf("fetch=%d out=%s err=%s hints=%d", a.Fetches(), out, errOut, *calls)
			}
			if args[1] == "--filter" && errOut.String() != "unknown provider \"nope\" (valid: claude)\n" {
				t.Fatal(errOut.String())
			}
		})
	}
}

func TestCancellation(t *testing.T) {
	for _, args := range [][]string{{"pace"}, {"pace", "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root, out, errOut, calls := testRoot(t, providertest.Slow("claude", time.Hour, nil))
			root.SetArgs(args)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := root.ExecuteContext(ctx); err != nil {
				t.Fatal(err)
			}
			if out.Len() != 0 || errOut.Len() != 0 || *calls != 0 {
				t.Fatal("cancellation produced output")
			}
		})
	}
}
