package dash

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/dash/theme"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/provider/providertest"
	"github.com/charmbracelet/lipgloss"
)

// runOpts wires a fake provider and in-memory terminal streams.
func runOpts(in string, out *bytes.Buffer, p provider.Provider) RunOptions {
	return RunOptions{
		Providers: []provider.Provider{p},
		Input:     strings.NewReader(in),
		Output:    out,
	}
}

type runContextKey struct{}

type backgroundCheckingInput struct {
	reader    io.Reader
	queries   *atomic.Int32
	premature chan struct{}
}

func (r backgroundCheckingInput) Read(p []byte) (int, error) {
	if r.queries.Load() == 0 {
		select {
		case r.premature <- struct{}{}:
		default:
		}
	}
	return r.reader.Read(p)
}

func TestRunPreparesOneDestinationRendererBeforeReadingInput(t *testing.T) {
	oldFactory, oldQuery := destinationRenderer, queryBackground
	t.Cleanup(func() {
		destinationRenderer, queryBackground = oldFactory, oldQuery
	})

	var factoryCalls, queryCalls atomic.Int32
	var renderer *lipgloss.Renderer
	var out bytes.Buffer
	destinationRenderer = func(w io.Writer) *lipgloss.Renderer {
		factoryCalls.Add(1)
		if w != &out {
			t.Errorf("renderer destination = %T, want the Run output", w)
		}
		renderer = oldFactory(w)
		return renderer
	}
	queryBackground = func(*lipgloss.Renderer) bool {
		queryCalls.Add(1)
		return true
	}
	premature := make(chan struct{}, 1)
	input := backgroundCheckingInput{
		reader:    strings.NewReader("q"),
		queries:   &queryCalls,
		premature: premature,
	}
	fake := providertest.Succeeding("claude", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := Run(ctx, RunOptions{
		Providers: []provider.Provider{fake},
		Input:     input,
		Output:    &out,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := factoryCalls.Load(); got != 1 {
		t.Errorf("renderer factory calls = %d, want 1", got)
	}
	if got := queryCalls.Load(); got != 1 {
		t.Errorf("background query calls = %d, want 1", got)
	}
	if renderer == nil || !renderer.HasDarkBackground() {
		t.Error("renderer did not keep the detected light/dark choice")
	}
	select {
	case <-premature:
		t.Error("Bubble Tea read input before background detection")
	default:
	}
}

func TestNewDashboardRendererKeepsDetectedBackground(t *testing.T) {
	oldFactory, oldQuery := destinationRenderer, queryBackground
	t.Cleanup(func() {
		destinationRenderer, queryBackground = oldFactory, oldQuery
	})
	destinationRenderer = func(w io.Writer) *lipgloss.Renderer {
		return lipgloss.NewRenderer(w)
	}
	for _, wantDark := range []bool{false, true} {
		queryBackground = func(*lipgloss.Renderer) bool { return wantDark }
		renderer := newDashboardRenderer(io.Discard)
		if got := renderer.HasDarkBackground(); got != wantDark {
			t.Errorf("HasDarkBackground() = %t, want %t", got, wantDark)
		}
	}
}

func TestRunOptionsReachModelOptions(t *testing.T) {
	ctx := context.WithValue(context.Background(), runContextKey{}, "bounded")
	th := theme.Default()
	th.Cursor.Dark = "#010203"
	p := providertest.Succeeding("cursor", nil)

	renderer := lipgloss.NewRenderer(io.Discard)
	renderer.SetHasDarkBackground(true)
	got := modelOptions(ctx, RunOptions{
		Providers:       []provider.Provider{p},
		Theme:           th,
		RefreshInterval: 17 * time.Second,
	}, renderer)
	if got.Ctx != ctx || len(got.Providers) != 1 || got.Providers[0] != p {
		t.Errorf("model options lost run state: %#v", got)
	}
	if got.Theme != th {
		t.Errorf("model theme = %#v, want %#v", got.Theme, th)
	}
	if got.RefreshInterval != 17*time.Second {
		t.Errorf("model refresh interval = %s, want 17s", got.RefreshInterval)
	}
	if got.Renderer != renderer {
		t.Errorf("model renderer = %p, want %p", got.Renderer, renderer)
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
	<-ctx.Done()
	close(b.observed)
	return provider.Usage{}, ctx.Err()
}

func TestRun_QuittingCancelsTheFetchInFlight(t *testing.T) {
	for _, tc := range []struct {
		name string
		keys []byte
	}{{"q", []byte("q")}, {"ctrl+c", []byte{0x03}}, {"combined r and q", []byte("rq")}} {
		t.Run(tc.name, func(t *testing.T) {
			before := runtime.NumGoroutine()

			b := newBlocker()
			in, keys := io.Pipe()
			var out bytes.Buffer
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			defer func() {
				cancel()
				_ = keys.Close()
				_ = in.Close()
			}()

			// Send input only after Fetch has started. The combined case
			// checks the parser's single KeyMsg for adjacent runes.
			go func() {
				<-b.started
				_, _ = keys.Write(tc.keys)
			}()
			start := time.Now()
			go func() {
				done <- Run(ctx, RunOptions{
					Providers: []provider.Provider{b},
					Input:     in,
					Output:    &out,
				})
			}()

			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("Run: %v", err)
				}
				if elapsed := time.Since(start); elapsed > time.Second {
					t.Fatalf("Run took %s to quit, want under 1s", elapsed)
				}
			case <-time.After(time.Second):
				t.Fatal("Run did not quit within 1s while Fetch was blocked")
			}

			select {
			case <-b.observed:
			case <-time.After(time.Second):
				t.Fatal("quitting left Fetch running: its context was not canceled")
			}

			var got int
			for i := 0; i < 100; i++ {
				if got = runtime.NumGoroutine(); got <= before+2 {
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
			t.Errorf("%d goroutines are still running, want no more than %d", got, before+2)
		})
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
