package dash

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/provider/providertest"
)

// runOpts is a dashboard wired to a fake provider and to in-memory
// streams, so the program runs its real event loop without a terminal.
func runOpts(in string, out *bytes.Buffer, p provider.Provider) RunOptions {
	return RunOptions{
		Providers: []provider.Provider{p},
		Banner:    true,
		Input:     strings.NewReader(in),
		Output:    out,
	}
}

func TestRun_QuitsOnQ(t *testing.T) {
	// A deadline so a dashboard that never quits fails the test instead of
	// hanging the package.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fake := providertest.Succeeding("claude", []provider.Window{
		{Provider: "claude", Name: "5h", Plan: "max", RemainingPercent: 42},
	})
	var out bytes.Buffer

	if err := Run(ctx, runOpts("q", &out, fake)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatal("Run did not quit on q")
	}
	if out.Len() == 0 {
		t.Fatal("Run drew nothing")
	}
}

func TestRun_ACancelledContextIsNotAFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fake := providertest.Succeeding("claude", nil)
	var out bytes.Buffer

	// No input at all: only the cancelled context can end this run.
	if err := Run(ctx, runOpts("", &out, fake)); err != nil {
		t.Fatalf("Run on a cancelled context = %v, want nil", err)
	}
}
