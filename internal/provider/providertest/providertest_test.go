package providertest_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/provider/providertest"
)

// compileCheck pins Fake's method set to provider.Provider at compile time.
var _ provider.Provider = (*providertest.Fake)(nil)

func TestSucceeding_DetectsAndFetchesWindows(t *testing.T) {
	windows := []provider.Window{{Provider: "claude", Name: "5h", UsedPercent: 10}}
	p := providertest.Succeeding("claude", windows)

	if got := p.ID(); got != "claude" {
		t.Errorf("ID() = %q, want %q", got, "claude")
	}

	ok, reason := p.Detect(context.Background())
	if !ok {
		t.Errorf("Detect() ok = false, want true (reason %q)", reason)
	}

	got, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "5h" {
		t.Errorf("Fetch() = %v, want %v", got, windows)
	}
}

func TestErroring_DetectsButFetchFails(t *testing.T) {
	fetchErr := errors.New("boom")
	p := providertest.Erroring("codex", fetchErr)

	ok, _ := p.Detect(context.Background())
	if !ok {
		t.Error("Detect() ok = false, want true")
	}

	_, err := p.Fetch(context.Background())
	if !errors.Is(err, fetchErr) {
		t.Errorf("Fetch() error = %v, want %v", err, fetchErr)
	}
}

func TestSlow_FetchReturnsAfterDelay(t *testing.T) {
	windows := []provider.Window{{Provider: "cursor", Name: "total", UsedPercent: 5}}
	p := providertest.Slow("cursor", 20*time.Millisecond, windows)

	start := time.Now()
	got, err := p.Fetch(context.Background())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if elapsed < 20*time.Millisecond {
		t.Errorf("Fetch() returned after %v, want >= 20ms", elapsed)
	}
	if len(got) != 1 || got[0].Name != "total" {
		t.Errorf("Fetch() = %v, want %v", got, windows)
	}
}

func TestSlow_FetchRespectsContextDeadline(t *testing.T) {
	p := providertest.Slow("cursor", time.Hour, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := p.Fetch(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Fetch() error = nil, want context deadline error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Fetch() error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("Fetch() took %v, want to return promptly after the deadline", elapsed)
	}
}

func TestUndetected_DetectFalseWithReason(t *testing.T) {
	p := providertest.Undetected("opencode-go", "not logged in, run opencode to log in")

	ok, reason := p.Detect(context.Background())
	if ok {
		t.Error("Detect() ok = true, want false")
	}
	if reason != "not logged in, run opencode to log in" {
		t.Errorf("Detect() reason = %q, want %q", reason, "not logged in, run opencode to log in")
	}

	// An undetected fake should still be safely callable; Fetch is not
	// expected to be invoked by well-behaved callers once Detect is false,
	// but it must not panic if it is.
	if _, err := p.Fetch(context.Background()); err != nil {
		t.Errorf("Fetch() error = %v, want nil (no windows, no error) for an undetected fake with no scripted error", err)
	}
}

func TestUndetected_FetchesZeroWhenNeverCalled(t *testing.T) {
	p := providertest.Undetected("opencode-go", "not logged in, run opencode to log in")

	if got := p.Fetches(); got != 0 {
		t.Errorf("Fetches() = %d, want 0 for a fake whose Fetch was never called", got)
	}
}

func TestFake_Fetches_CountsCalls(t *testing.T) {
	windows := []provider.Window{{Provider: "claude", Name: "5h", UsedPercent: 10}}
	p := providertest.Succeeding("claude", windows)

	if got := p.Fetches(); got != 0 {
		t.Fatalf("Fetches() = %d, want 0 before any call", got)
	}

	for i := 1; i <= 3; i++ {
		if _, err := p.Fetch(context.Background()); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got := p.Fetches(); got != i {
			t.Errorf("Fetches() = %d, want %d after %d call(s)", got, i, i)
		}
	}
}

func TestFake_Fetches_ConcurrencySafe(t *testing.T) {
	p := providertest.Succeeding("claude", nil)

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = p.Fetch(context.Background())
		}()
	}
	wg.Wait()

	if got := p.Fetches(); got != n {
		t.Errorf("Fetches() = %d, want %d after %d concurrent calls", got, n, n)
	}
}
