package dash

import (
	"bytes"
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/dash/theme"
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

func TestRunOptionsReachModelOptions(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "bounded")
	th := theme.Default()
	th.Cursor.Dark = "#010203"
	p := providertest.Succeeding("cursor", nil)

	got := modelOptions(ctx, RunOptions{
		Providers:       []provider.Provider{p},
		Banner:          true,
		Vertical:        true,
		Fit:             true,
		Theme:           th,
		MeterWidth:      83,
		RefreshInterval: 17 * time.Second,
	})
	if got.Ctx != ctx || !got.Banner || !got.Vertical || !got.Fit || len(got.Providers) != 1 || got.Providers[0] != p {
		t.Errorf("model options lost run state: %#v", got)
	}
	if got.Theme != th || got.MeterWidth != 83 {
		t.Errorf("model appearance = (%#v, %d), want (%#v, 83)", got.Theme, got.MeterWidth, th)
	}
	if got.RefreshInterval != 17*time.Second {
		t.Errorf("model refresh interval = %s, want 17s", got.RefreshInterval)
	}
}

// blocker is a provider whose Fetch parks until the context it was handed
// is cancelled, so a test can watch what quitting does to a fetch that is
// still in flight. Bubble Tea deliberately does not wait for a command's
// goroutine, so nothing but Run's own cancel can end this one.
type blocker struct {
	started  chan struct{} // closed once Fetch is under way
	observed chan struct{} // closed once Fetch has seen its context cancelled
	once     sync.Once
}

func newBlocker() *blocker {
	return &blocker{started: make(chan struct{}), observed: make(chan struct{})}
}

func (b *blocker) ID() string                            { return "claude" }
func (b *blocker) Detect(context.Context) (bool, string) { return true, "" }

func (b *blocker) Fetch(ctx context.Context) (provider.Usage, error) {
	b.once.Do(func() { close(b.started) })
	select {
	case <-ctx.Done():
		close(b.observed)
		return provider.Usage{}, ctx.Err()
	case <-time.After(30 * time.Second):
		return provider.Usage{}, errors.New("the fetch was left to run to completion")
	}
}

func TestRun_QuittingCancelsTheFetchInFlight(t *testing.T) {
	before := runtime.NumGoroutine()

	b := newBlocker()
	in, keys := io.Pipe()
	var out bytes.Buffer

	// q is pressed only once the fetch is under way, so the test cannot
	// pass by quitting before anything had started.
	go func() {
		<-b.started
		_, _ = keys.Write([]byte("q"))
	}()

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		done <- Run(context.Background(), RunOptions{
			Providers: []provider.Provider{b},
			Banner:    true,
			Input:     in,
			Output:    &out,
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("Run took %s to quit, want under 2s: it waited for the fetch", elapsed)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Run never returned")
	}

	select {
	case <-b.observed:
	case <-time.After(time.Second):
		t.Fatal("quitting left the fetch running: its context was never cancelled")
	}

	// Let the input reader go before counting: it is parked on a pipe that
	// only the test can close.
	keys.Close()
	in.Close()

	var got int
	for i := 0; i < 100; i++ {
		if got = runtime.NumGoroutine(); got <= before+2 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("%d goroutines are still running, want no more than %d", got, before+2)
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
	// The dashboard takes the alternate screen and gives it back, so the
	// terminal is left with whatever was on it before.
	for _, want := range []struct{ seq, what string }{
		{"\x1b[?1049h", "enter the alternate screen"},
		{"\x1b[?1049l", "leave the alternate screen"},
	} {
		if !strings.Contains(out.String(), want.seq) {
			t.Errorf("the output never asks the terminal to %s", want.what)
		}
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
