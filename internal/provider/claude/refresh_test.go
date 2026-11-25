package claude

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// TestMain empties PATH so that no test in this package can start a real
// claude binary against the real home directory, even one that forgets to
// inject a refresh runner.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "qmeter-claude-path-")
	if err != nil {
		panic(err)
	}
	os.Setenv("PATH", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// refreshRunner is a fake vendor CLI. Each run is recorded; do is what the
// run does, typically writing a fresh credential store.
type refreshRunner struct {
	mu    sync.Mutex
	calls []string
	do    func(ctx context.Context) error
}

func (r *refreshRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	r.calls = append(r.calls, strings.Join(append([]string{name}, args...), " "))
	r.mu.Unlock()
	if r.do == nil {
		return nil, nil
	}
	return nil, r.do(ctx)
}

func (r *refreshRunner) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

// writeStore writes a Claude credential store holding token, expiring at
// expiresAt (epoch milliseconds).
func writeStore(t *testing.T, path, token string, expiresAt int64) {
	t.Helper()
	data := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":%q,"expiresAt":%d,"subscriptionType":"max"}}`, token, expiresAt)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

// usageServer answers 200 for the bearer token good and 401 for anything
// else, and counts the requests.
func usageServer(t *testing.T, good string) (*httptest.Server, *int) {
	t.Helper()
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+good {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"five_hour":{"utilization":25}}`)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

// refreshFixture sets up a temp store, an empty Pi directory and a provider
// that uses runner for refreshes.
func refreshFixture(t *testing.T, runner *refreshRunner, baseURL string) (*Provider, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".credentials.json")
	p := newProvider(t, WithCredentialPath(path), WithBaseURL(baseURL), WithRefreshRunner(runner))
	return p, path
}

func TestRefresh_ExpiredStoreRunsClaudeAuthStatusAndRetries(t *testing.T) {
	srv, requests := usageServer(t, "fresh")
	runner := &refreshRunner{}
	p, path := refreshFixture(t, runner, srv.URL)
	writeStore(t, path, "stale", testNow.UnixMilli())
	runner.do = func(context.Context) error {
		writeStore(t, path, "fresh", testNow.UnixMilli()+3600_000)
		return nil
	}

	usage, err := p.Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil after the refresh", err)
	}
	if len(usage.Windows) != 1 || usage.Windows[0].Plan != "max" {
		t.Fatalf("usage = %+v, want the native window", usage)
	}
	if runner.count() != 1 || runner.calls[0] != "claude auth status" {
		t.Fatalf("refresh calls = %q, want one claude auth status", runner.calls)
	}
	if *requests != 1 {
		t.Fatalf("requests = %d, want 1 (the expired token is never sent)", *requests)
	}
}

func TestRefresh_UnauthorizedRunsRefreshAndRetriesOnce(t *testing.T) {
	srv, requests := usageServer(t, "fresh")
	runner := &refreshRunner{}
	p, path := refreshFixture(t, runner, srv.URL)
	writeStore(t, path, "revoked", testNow.UnixMilli()+3600_000)
	runner.do = func(context.Context) error {
		writeStore(t, path, "fresh", testNow.UnixMilli()+3600_000)
		return nil
	}

	if _, err := p.Fetch(testContext(t)); err != nil {
		t.Fatalf("Fetch() error = %v, want nil after the refresh", err)
	}
	if runner.count() != 1 || *requests != 2 {
		t.Fatalf("refreshes = %d, requests = %d, want 1 and 2", runner.count(), *requests)
	}
}

func TestRefresh_AtMostOneRefreshPerFetch(t *testing.T) {
	for _, tc := range []struct {
		name      string
		expiresAt int64
		requests  int
	}{
		// The refresh "succeeds" but leaves the store as it was.
		{"expired stays expired", testNow.UnixMilli(), 0},
		{"rejected stays rejected", testNow.UnixMilli() + 3600_000, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, requests := usageServer(t, "never")
			runner := &refreshRunner{}
			p, path := refreshFixture(t, runner, srv.URL)
			writeStore(t, path, "stale", tc.expiresAt)

			_, err := p.Fetch(testContext(t))
			var expired provider.ErrTokenExpired
			if !errors.As(err, &expired) || expired.Tool != toolName {
				t.Fatalf("Fetch() error = %v, want the claude token-expired error", err)
			}
			if runner.count() != 1 || *requests != tc.requests {
				t.Fatalf("refreshes = %d, requests = %d, want 1 and %d", runner.count(), *requests, tc.requests)
			}
		})
	}
}

func TestRefresh_FailedRefreshIsTriedAgainOnTheNextFetch(t *testing.T) {
	srv, _ := usageServer(t, "fresh")
	runner := &refreshRunner{do: func(context.Context) error { return errors.New("boom") }}
	p, path := refreshFixture(t, runner, srv.URL)
	writeStore(t, path, "stale", testNow.UnixMilli())

	for i := 0; i < 2; i++ {
		if _, err := p.Fetch(testContext(t)); !errors.Is(err, provider.ErrTokenExpired{}) {
			t.Fatalf("Fetch() #%d error = %v, want token expired", i+1, err)
		}
	}
	if runner.count() != 2 {
		t.Fatalf("refreshes = %d, want one per fetch", runner.count())
	}
}

func TestRefresh_NativeBeforePi(t *testing.T) {
	for _, tc := range []struct {
		name      string
		refreshOK bool
		wantPlan  string
	}{
		{"refresh succeeds: native token", true, "max"},
		{"refresh fails: Pi token", false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var tokens []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				tokens = append(tokens, r.Header.Get("Authorization"))
				mu.Unlock()
				fmt.Fprint(w, `{"five_hour":{"utilization":25}}`)
			}))
			defer srv.Close()
			runner := &refreshRunner{}
			p, path := refreshFixture(t, runner, srv.URL)
			writeStore(t, path, "stale", testNow.UnixMilli())
			writePi(t, testNow.UnixMilli()+1)
			runner.do = func(context.Context) error {
				if !tc.refreshOK {
					return errors.New("boom")
				}
				writeStore(t, path, "fresh", testNow.UnixMilli()+3600_000)
				return nil
			}

			usage, err := p.Fetch(testContext(t))
			if err != nil {
				t.Fatalf("Fetch() error = %v", err)
			}
			if runner.count() != 1 {
				t.Fatalf("refreshes = %d, want 1", runner.count())
			}
			if len(usage.Windows) != 1 || usage.Windows[0].Plan != tc.wantPlan {
				t.Fatalf("usage = %+v, want plan %q", usage, tc.wantPlan)
			}
			want := "Bearer fresh"
			if !tc.refreshOK {
				want = "Bearer pi-token"
			}
			if len(tokens) != 1 || tokens[0] != want {
				t.Fatalf("tokens sent = %q, want [%q]", tokens, want)
			}
		})
	}
}

func TestRefresh_UnauthorizedRefreshFailsThenPi(t *testing.T) {
	srv, requests := usageServer(t, "pi-token")
	runner := &refreshRunner{do: func(context.Context) error { return errors.New("boom") }}
	p, path := refreshFixture(t, runner, srv.URL)
	writeStore(t, path, "revoked", testNow.UnixMilli()+3600_000)
	writePi(t, testNow.UnixMilli()+1)

	if _, err := p.Fetch(testContext(t)); err != nil {
		t.Fatalf("Fetch() error = %v, want the Pi fallback to succeed", err)
	}
	if runner.count() != 1 || *requests != 2 {
		t.Fatalf("refreshes = %d, requests = %d, want 1 and 2", runner.count(), *requests)
	}
}

func TestRefresh_NeverForEnvOrPiCredentials(t *testing.T) {
	t.Run("env override", func(t *testing.T) {
		srv, _ := usageServer(t, "never")
		runner := &refreshRunner{}
		p, path := refreshFixture(t, runner, srv.URL)
		writeStore(t, path, "stale", testNow.UnixMilli())
		t.Setenv(envVar, "override")
		if _, err := p.Fetch(testContext(t)); !errors.Is(err, provider.ErrTokenExpired{}) {
			t.Fatalf("Fetch() error = %v, want token expired", err)
		}
		if runner.count() != 0 {
			t.Fatalf("refreshes = %d, want 0 for %s", runner.count(), envVar)
		}
	})
	t.Run("pi source", func(t *testing.T) {
		srv, _ := usageServer(t, "never")
		runner := &refreshRunner{}
		p, _ := refreshFixture(t, runner, srv.URL) // no native store
		writePi(t, testNow.UnixMilli()+1)
		if _, err := p.Fetch(testContext(t)); !errors.Is(err, provider.ErrTokenExpired{}) {
			t.Fatalf("Fetch() error = %v, want token expired", err)
		}
		if runner.count() != 0 {
			t.Fatalf("refreshes = %d, want 0 for a Pi credential", runner.count())
		}
	})
}

func TestRefresh_DetectNeverRefreshes(t *testing.T) {
	runner := &refreshRunner{}
	p, path := refreshFixture(t, runner, "http://127.0.0.1:1")
	writeStore(t, path, "stale", testNow.UnixMilli())
	if ok, reason := p.Detect(context.Background()); !ok {
		t.Fatalf("Detect() = false, %q", reason)
	}
	if runner.count() != 0 {
		t.Fatalf("Detect ran %d refreshes, want 0", runner.count())
	}
}

func TestRefresh_DefaultRunnerFindsNoClaudeOnAnEmptyPath(t *testing.T) {
	// New's default runner starts the real binary by name. TestMain emptied
	// PATH, so this proves both that it looks claude up on PATH and that no
	// test here can reach a real one.
	srv, _ := usageServer(t, "never")
	path := filepath.Join(t.TempDir(), ".credentials.json")
	writeStore(t, path, "stale", testNow.UnixMilli())
	clearEnv(t)
	p := New(WithCredentialPath(path), WithBaseURL(srv.URL), WithClock(func() time.Time { return testNow }))
	_, err := p.Fetch(testContext(t))
	var expired provider.ErrTokenExpired
	if !errors.As(err, &expired) || expired.RefreshFailure != "claude not found" {
		t.Fatalf("Fetch() error = %v, want the refresh to fail with claude not found", err)
	}
}

func TestRefresh_FailureReasonIsInTheHint(t *testing.T) {
	notFound := func(context.Context) error { return &exec.Error{Name: "claude", Err: exec.ErrNotFound} }
	const want = "token expired, open claude to refresh (auto-refresh failed: claude not found)"
	for _, tc := range []struct {
		name      string
		expiresAt int64
	}{
		{"expired store", testNow.UnixMilli()},
		{"rejected token", testNow.UnixMilli() + 3600_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := usageServer(t, "never")
			p, path := refreshFixture(t, &refreshRunner{do: notFound}, srv.URL)
			writeStore(t, path, "stale", tc.expiresAt)

			_, err := p.Fetch(testContext(t))
			var expired provider.ErrTokenExpired
			if !errors.As(err, &expired) || expired.Error() != want {
				t.Fatalf("Fetch() error = %v, want the typed error %q", err, want)
			}
		})
	}
}

func TestRefresh_NoReasonWhenTheRefreshSucceeds(t *testing.T) {
	srv, _ := usageServer(t, "never")
	p, path := refreshFixture(t, &refreshRunner{}, srv.URL)
	writeStore(t, path, "stale", testNow.UnixMilli()+3600_000)

	_, err := p.Fetch(testContext(t))
	var expired provider.ErrTokenExpired
	if !errors.As(err, &expired) || expired.Error() != "token expired, open claude to refresh" {
		t.Fatalf("Fetch() error = %v, want the plain hint", err)
	}
}

func TestRefresh_CutShortByTheFetchDeadlineReportsTheDeadline(t *testing.T) {
	srv, _ := usageServer(t, "never")
	runner := &refreshRunner{do: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }}
	p, path := refreshFixture(t, runner, srv.URL)
	writeStore(t, path, "stale", testNow.UnixMilli())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := p.Fetch(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, provider.ErrTokenExpired{}) {
		t.Fatalf("Fetch() error = %v, want the deadline, not a token hint", err)
	}
}

// TestRefresh_ContextEndingAfterAFailedRefresh covers a fetch context that
// ends after the refresh has already failed for a reason of its own. The
// final error must be the one of the step the deadline interrupted.
func TestRefresh_ContextEndingAfterAFailedRefresh(t *testing.T) {
	notFound := func(context.Context) error { return &exec.Error{Name: "claude", Err: exec.ErrNotFound} }
	// hang answers 401 to the native token and holds every other request
	// until the client gives up.
	hang := func(t *testing.T) string {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "Bearer native" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			<-r.Context().Done()
		}))
		t.Cleanup(srv.Close)
		return srv.URL
	}
	deadline := func(t *testing.T) context.Context {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		t.Cleanup(cancel)
		return ctx
	}

	// claude.go: the usage request error after the Pi fallback.
	for _, tc := range []struct {
		name      string
		expiresAt int64
	}{
		{"expired store, Pi request times out", testNow.UnixMilli()},
		{"rejected token, Pi request times out", testNow.UnixMilli() + 3600_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, path := refreshFixture(t, &refreshRunner{do: notFound}, hang(t))
			writeStore(t, path, "native", tc.expiresAt)
			writePi(t, testNow.UnixMilli()+1)

			_, err := p.Fetch(deadline(t))
			if !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "not found") {
				t.Fatalf("Fetch() error = %v, want the Pi request deadline", err)
			}
		})
	}

	// claude.go: the credential error. The refresh fails with its own
	// reason just as the deadline passes, and no Pi credential exists.
	t.Run("expired store, refresh fails as the deadline passes", func(t *testing.T) {
		late := func(ctx context.Context) error { <-ctx.Done(); return errors.New("exit status 1") }
		p, path := refreshFixture(t, &refreshRunner{do: late}, hang(t))
		writeStore(t, path, "native", testNow.UnixMilli())

		_, err := p.Fetch(deadline(t))
		var expired provider.ErrTokenExpired
		if !errors.As(err, &expired) || expired.RefreshFailure != "claude auth status: exit status 1" {
			t.Fatalf("Fetch() error = %v, want the token hint with the refresh failure", err)
		}
	})
}
