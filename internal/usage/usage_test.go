package usage

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/provider/providertest"
)

// testTimeout is the per-provider timeout the tests inject via run(), so no
// test ever waits DefaultProviderTimeout.
const testTimeout = 2 * time.Second

func win(id, name string) provider.Window {
	return provider.Window{Provider: id, Name: name}
}

func windowKeys(windows []provider.Window) []string {
	keys := make([]string, 0, len(windows))
	for _, w := range windows {
		keys = append(keys, w.Provider+"/"+w.Name)
	}
	return keys
}

func errorProviders(errs []ProviderError) []string {
	ids := make([]string, 0, len(errs))
	for _, e := range errs {
		ids = append(ids, e.Provider)
	}
	return ids
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func messageFor(t *testing.T, errs []ProviderError, id string) string {
	t.Helper()
	for _, e := range errs {
		if e.Provider == id {
			return e.Message
		}
	}
	t.Fatalf("no ProviderError for %q in %v", id, errs)
	return ""
}

func TestDefaultProviderTimeoutIsTenSeconds(t *testing.T) {
	if DefaultProviderTimeout != 10*time.Second {
		t.Errorf("DefaultProviderTimeout = %s, want 10s", DefaultProviderTimeout)
	}
}

func TestRun_FetchesDetectedProvidersConcurrently(t *testing.T) {
	const delay = 80 * time.Millisecond
	providers := []provider.Provider{
		providertest.Slow("claude", delay, []provider.Window{win("claude", "5h")}),
		providertest.Slow("codex", delay, []provider.Window{win("codex", "weekly")}),
		providertest.Slow("opencode-go", delay, []provider.Window{win("opencode-go", "monthly")}),
		providertest.Slow("cursor", delay, []provider.Window{win("cursor", "total")}),
	}

	start := time.Now()
	got := run(context.Background(), providers, "", testTimeout)
	elapsed := time.Since(start)

	want := []string{"claude/5h", "codex/weekly", "opencode-go/monthly", "cursor/total"}
	if !equalStrings(windowKeys(got.Windows), want) {
		t.Errorf("Windows = %v, want %v", windowKeys(got.Windows), want)
	}
	if len(got.Errors) != 0 {
		t.Errorf("Errors = %v, want empty", got.Errors)
	}
	// Serial execution would take 4*delay; concurrent execution takes ~delay.
	// The margin is wide enough that a loaded machine does not flake, and
	// still narrow enough to catch a serial (sum-of-delays) implementation.
	if max := 3 * delay; elapsed >= max {
		t.Errorf("elapsed = %s, want < %s (fetches must overlap, not run serially)", elapsed, max)
	}
}

func TestRun_SlowProviderTimesOutIndependently(t *testing.T) {
	const timeout = 40 * time.Millisecond
	slow := providertest.Slow("claude", time.Hour, []provider.Window{win("claude", "5h")})
	fast := providertest.Succeeding("codex", []provider.Window{win("codex", "weekly")})

	// run in a goroutine: without the per-provider timeout the slow fake
	// blocks for an hour, and this must fail with a message, not hang the
	// suite until the test binary's own deadline.
	done := make(chan Result, 1)
	go func() { done <- run(context.Background(), []provider.Provider{slow, fast}, "", timeout) }()

	var got Result
	select {
	case got = <-done:
	case <-time.After(20 * timeout):
		t.Fatalf("run did not return within %s; the per-provider timeout must bound it", 20*timeout)
	}

	if want := []string{"codex/weekly"}; !equalStrings(windowKeys(got.Windows), want) {
		t.Errorf("Windows = %v, want %v (the fast provider still reports)", windowKeys(got.Windows), want)
	}
	if want := []string{"claude"}; !equalStrings(errorProviders(got.Errors), want) {
		t.Fatalf("Errors = %v, want %v", got.Errors, want)
	}
	if msg, want := messageFor(t, got.Errors, "claude"), "timed out after 40ms"; msg != want {
		t.Errorf("message = %q, want %q", msg, want)
	}
}

func TestRun_TimeoutMessageReportsConfiguredTimeout(t *testing.T) {
	for _, tc := range []struct {
		timeout time.Duration
		want    string
	}{
		{30 * time.Millisecond, "timed out after 30ms"},
		{150 * time.Millisecond, "timed out after 150ms"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			slow := providertest.Slow("claude", time.Hour, nil)

			got := run(context.Background(), []provider.Provider{slow}, "", tc.timeout)

			if msg := messageFor(t, got.Errors, "claude"); msg != tc.want {
				t.Errorf("message = %q, want %q", msg, tc.want)
			}
		})
	}
}

func TestRun_OneProviderFailureDoesNotBlockOthers(t *testing.T) {
	failing := providertest.Erroring("claude", errors.New("boom"))
	ok := providertest.Succeeding("codex", []provider.Window{win("codex", "weekly")})
	timingOut := providertest.Slow("cursor", time.Hour, nil)

	got := run(context.Background(), []provider.Provider{failing, ok, timingOut}, "", 40*time.Millisecond)

	if want := []string{"codex/weekly"}; !equalStrings(windowKeys(got.Windows), want) {
		t.Errorf("Windows = %v, want %v", windowKeys(got.Windows), want)
	}
	if want := []string{"claude", "cursor"}; !equalStrings(errorProviders(got.Errors), want) {
		t.Fatalf("Errors = %v, want %v", got.Errors, want)
	}
	if msg := messageFor(t, got.Errors, "claude"); msg != "boom" {
		t.Errorf("untyped error message = %q, want %q", msg, "boom")
	}
}

func TestRun_RateLimitedErrorProducesRetryAfterMessage(t *testing.T) {
	for _, tc := range []struct {
		name       string
		retryAfter time.Duration
		want       string
	}{
		{name: "vendor supplied a retry-after", retryAfter: 90 * time.Second, want: "rate limited, retry in 1m30s"},
		{name: "vendor supplied none", retryAfter: 0, want: "rate limited, retry later"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rateLimited := providertest.Erroring("opencode-go", provider.ErrRateLimited{RetryAfter: tc.retryAfter})

			got := run(context.Background(), []provider.Provider{rateLimited}, "", testTimeout)

			if msg := messageFor(t, got.Errors, "opencode-go"); msg != tc.want {
				t.Errorf("message = %q, want %q", msg, tc.want)
			}
		})
	}
}

func TestRun_MessageUsesExtractedTypedErrorNotWrapper(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{
			name: "token expired wrapped",
			err:  fmt.Errorf("claude: fetch usage: %w", provider.ErrTokenExpired{Tool: "claude"}),
			want: "token expired, open claude to refresh",
		},
		{
			name: "not logged in wrapped",
			err:  fmt.Errorf("codex: credentials: %w", provider.ErrNotLoggedIn{Tool: "codex"}),
			want: "not logged in, run codex to log in",
		},
		{
			name: "rate limited wrapped",
			err:  fmt.Errorf("cursor: GET /usage: %w", provider.ErrRateLimited{RetryAfter: 30 * time.Second}),
			want: "rate limited, retry in 30s",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := providertest.Erroring("claude", tc.err)

			got := run(context.Background(), []provider.Provider{p}, "", testTimeout)

			if msg := messageFor(t, got.Errors, "claude"); msg != tc.want {
				t.Errorf("message = %q, want %q (wrapping prefixes must not leak)", msg, tc.want)
			}
		})
	}
}

func TestRun_ProviderFilterRestrictsToOne(t *testing.T) {
	claude := providertest.Succeeding("claude", []provider.Window{win("claude", "5h")})
	codex := providertest.Succeeding("codex", []provider.Window{win("codex", "weekly")})

	got := run(context.Background(), []provider.Provider{claude, codex}, "codex", testTimeout)

	if want := []string{"codex/weekly"}; !equalStrings(windowKeys(got.Windows), want) {
		t.Errorf("Windows = %v, want %v", windowKeys(got.Windows), want)
	}
	if n := claude.Fetches(); n != 0 {
		t.Errorf("claude.Fetches() = %d, want 0 (filtered out)", n)
	}
	if len(got.Errors) != 0 || len(got.Undetected) != 0 {
		t.Errorf("Errors = %v, Undetected = %v, want both empty", got.Errors, got.Undetected)
	}
	if got.UnknownProvider {
		t.Error("UnknownProvider = true, want false for a provider that exists")
	}
}

func TestRun_UnknownProviderFilterIsReportedAndFetchesNothing(t *testing.T) {
	claude := providertest.Succeeding("claude", []provider.Window{win("claude", "5h")})

	got := run(context.Background(), []provider.Provider{claude}, "nope", testTimeout)

	if !got.UnknownProvider {
		t.Error("UnknownProvider = false, want true")
	}
	if len(got.Windows) != 0 || len(got.Errors) != 0 || len(got.Undetected) != 0 {
		t.Errorf("Result = %+v, want empty windows/errors/undetected", got)
	}
	if n := claude.Fetches(); n != 0 {
		t.Errorf("claude.Fetches() = %d, want 0", n)
	}
}

func TestRun_UndetectedProviderReasonSurfacedOnlyWhenFiltered(t *testing.T) {
	reason := "not logged in, run codex to log in"

	t.Run("default listing omits it", func(t *testing.T) {
		claude := providertest.Succeeding("claude", []provider.Window{win("claude", "5h")})
		codex := providertest.Undetected("codex", reason)

		got := run(context.Background(), []provider.Provider{claude, codex}, "", testTimeout)

		if len(got.Undetected) != 0 {
			t.Errorf("Undetected = %v, want empty in the default listing", got.Undetected)
		}
		if len(got.Errors) != 0 {
			t.Errorf("Errors = %v, want empty (undetected is not an error)", got.Errors)
		}
		if want := []string{"claude/5h"}; !equalStrings(windowKeys(got.Windows), want) {
			t.Errorf("Windows = %v, want %v", windowKeys(got.Windows), want)
		}
	})

	t.Run("filter surfaces the reason", func(t *testing.T) {
		claude := providertest.Succeeding("claude", []provider.Window{win("claude", "5h")})
		codex := providertest.Undetected("codex", reason)

		got := run(context.Background(), []provider.Provider{claude, codex}, "codex", testTimeout)

		want := ProviderError{Provider: "codex", Message: reason}
		if len(got.Undetected) != 1 || got.Undetected[0] != want {
			t.Errorf("Undetected = %v, want [%v]", got.Undetected, want)
		}
		if len(got.Windows) != 0 || len(got.Errors) != 0 {
			t.Errorf("Windows = %v, Errors = %v, want both empty", got.Windows, got.Errors)
		}
		if got.UnknownProvider {
			t.Error("UnknownProvider = true, want false (the provider exists, it is just undetected)")
		}
	})
}

func TestRun_UndetectedProviderFetchNeverCalled(t *testing.T) {
	for _, only := range []string{"", "codex"} {
		codex := providertest.Undetected("codex", "not logged in, run codex to log in")

		run(context.Background(), []provider.Provider{codex}, only, testTimeout)

		if n := codex.Fetches(); n != 0 {
			t.Errorf("only=%q: Fetches() = %d, want 0", only, n)
		}
	}
}

func TestRun_ResultsInInputOrderNotCompletionOrder(t *testing.T) {
	slow := providertest.Slow("claude", 60*time.Millisecond, []provider.Window{win("claude", "5h")})
	fast := providertest.Succeeding("codex", []provider.Window{win("codex", "weekly")})
	slowFail := &providertest.Fake{
		IDValue:  "opencode-go",
		DetectOK: true,
		Delay:    40 * time.Millisecond,
		FetchErr: provider.ErrNotLoggedIn{Tool: "opencode"},
	}
	fastFail := providertest.Erroring("cursor", provider.ErrTokenExpired{Tool: "cursor-agent"})

	got := run(context.Background(), []provider.Provider{slow, fast, slowFail, fastFail}, "", testTimeout)

	if want := []string{"claude/5h", "codex/weekly"}; !equalStrings(windowKeys(got.Windows), want) {
		t.Errorf("Windows = %v, want %v (input order, not completion order)", windowKeys(got.Windows), want)
	}
	if want := []string{"opencode-go", "cursor"}; !equalStrings(errorProviders(got.Errors), want) {
		t.Errorf("Errors = %v, want %v (input order, not completion order)", errorProviders(got.Errors), want)
	}
	if msg, want := messageFor(t, got.Errors, "opencode-go"), "not logged in, run opencode to log in"; msg != want {
		t.Errorf("message = %q, want %q", msg, want)
	}
	if msg, want := messageFor(t, got.Errors, "cursor"), "token expired, open cursor-agent to refresh"; msg != want {
		t.Errorf("message = %q, want %q", msg, want)
	}
}

func TestRun_FillsEmptyWindowProviderWithID(t *testing.T) {
	p := providertest.Succeeding("cursor", []provider.Window{
		{Name: "total"},
		{Provider: "explicit", Name: "auto"},
	})

	got := run(context.Background(), []provider.Provider{p}, "", testTimeout)

	if want := []string{"cursor/total", "explicit/auto"}; !equalStrings(windowKeys(got.Windows), want) {
		t.Errorf("Windows = %v, want %v", windowKeys(got.Windows), want)
	}
}

func TestRun_UsesDefaultProviderTimeout(t *testing.T) {
	var deadline time.Time
	var hasDeadline bool
	p := &deadlineRecorder{id: "claude", record: func(d time.Time, ok bool) {
		deadline, hasDeadline = d, ok
	}}

	start := time.Now()
	got := Run(context.Background(), []provider.Provider{p}, "")

	if len(got.Errors) != 0 {
		t.Fatalf("Errors = %v, want empty", got.Errors)
	}
	if !hasDeadline {
		t.Fatal("Fetch context had no deadline, want DefaultProviderTimeout applied")
	}
	if budget := deadline.Sub(start); budget < DefaultProviderTimeout || budget > DefaultProviderTimeout+time.Second {
		t.Errorf("Fetch deadline budget = %s, want ~%s", budget, DefaultProviderTimeout)
	}
}

func TestRun_FetchContextDerivesFromCallerContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rec := newCtxRecorder("claude")
	done := make(chan Result, 1)
	// A generous per-provider timeout: only the caller's cancellation can
	// end this run promptly, so a Fetch context built from
	// context.Background() instead of ctx would stall here.
	go func() { done <- run(ctx, []provider.Provider{rec}, "", time.Minute) }()

	<-rec.started
	if fetchCtx := rec.recorded(); fetchCtx == nil {
		t.Fatal("Fetch received no context")
	}
	cancel()

	var got Result
	select {
	case got = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after the caller's context was canceled: the Fetch context is not derived from it")
	}

	select {
	case <-rec.recorded().Done():
	default:
		t.Error("Fetch context is not Done after the caller's context was canceled: it is not derived from the caller")
	}
	if msg, want := messageFor(t, got.Errors, "claude"), "canceled"; msg != want {
		t.Errorf("message = %q, want %q", msg, want)
	}
}

func TestRun_CallerContextDoneReportsCanceledNotTimedOut(t *testing.T) {
	for _, tc := range []struct {
		name   string
		parent func() (context.Context, context.CancelFunc)
	}{
		{
			name: "caller canceled",
			parent: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
		},
		{
			name: "caller deadline exceeded",
			parent: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := tc.parent()
			defer cancel()
			slow := providertest.Slow("claude", time.Hour, nil)

			got := run(ctx, []provider.Provider{slow}, "", testTimeout)

			if msg, want := messageFor(t, got.Errors, "claude"), "canceled"; msg != want {
				t.Errorf("message = %q, want %q (the per-provider timeout did not fire)", msg, want)
			}
		})
	}
}

func TestRun_PanickingProviderDoesNotBlockOthers(t *testing.T) {
	ok := providertest.Succeeding("codex", []provider.Window{win("codex", "weekly")})
	boom := panicker{id: "claude", value: "nil map write"}

	got := run(context.Background(), []provider.Provider{boom, ok}, "", testTimeout)

	if want := []string{"codex/weekly"}; !equalStrings(windowKeys(got.Windows), want) {
		t.Errorf("Windows = %v, want %v (a sibling panic must not lose this)", windowKeys(got.Windows), want)
	}
	if want := []string{"claude"}; !equalStrings(errorProviders(got.Errors), want) {
		t.Fatalf("Errors = %v, want %v", got.Errors, want)
	}
	if msg, want := messageFor(t, got.Errors, "claude"), "provider panicked: nil map write"; msg != want {
		t.Errorf("message = %q, want %q", msg, want)
	}
}

// panicker is a detected provider whose Fetch panics; Run must contain the
// panic so sibling providers still report.
type panicker struct {
	id    string
	value any
}

func (p panicker) ID() string { return p.id }

func (p panicker) Detect(ctx context.Context) (bool, string) { return true, "" }

func (p panicker) Fetch(ctx context.Context) ([]provider.Window, error) { panic(p.value) }

// ctxRecorder is a detected provider that records the context its Fetch
// received, announces that Fetch is in flight, and then blocks until that
// context is done (with a safety net so a broken implementation fails the
// test rather than hanging the suite).
type ctxRecorder struct {
	id      string
	started chan struct{}

	mu  sync.Mutex
	ctx context.Context
}

func newCtxRecorder(id string) *ctxRecorder {
	return &ctxRecorder{id: id, started: make(chan struct{})}
}

func (r *ctxRecorder) ID() string { return r.id }

func (r *ctxRecorder) Detect(ctx context.Context) (bool, string) { return true, "" }

func (r *ctxRecorder) Fetch(ctx context.Context) ([]provider.Window, error) {
	r.mu.Lock()
	r.ctx = ctx
	r.mu.Unlock()
	close(r.started)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		return nil, errors.New("ctxRecorder safety net fired: context never became done")
	}
}

func (r *ctxRecorder) recorded() context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ctx
}

// deadlineRecorder is a detected provider that reports the deadline of the
// context its Fetch received; Run must apply DefaultProviderTimeout.
type deadlineRecorder struct {
	id     string
	record func(time.Time, bool)
}

func (d *deadlineRecorder) ID() string { return d.id }

func (d *deadlineRecorder) Detect(ctx context.Context) (bool, string) { return true, "" }

func (d *deadlineRecorder) Fetch(ctx context.Context) ([]provider.Window, error) {
	deadline, ok := ctx.Deadline()
	d.record(deadline, ok)
	return nil, nil
}
