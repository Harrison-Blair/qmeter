package codex

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
// codex binary against the real home directory. Most tests here build a
// Provider with New and keep the default runner; with an empty PATH a 401
// makes that runner fail with "codex not found".
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "qmeter-codex-path-")
	if err != nil {
		panic(err)
	}
	os.Setenv("PATH", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// refreshRunner is a fake vendor CLI. Each run is recorded; do is what the
// run does, typically writing a fresh auth.json.
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

func writeAuth(t *testing.T, path, token string) {
	t.Helper()
	data := fmt.Sprintf(`{"tokens":{"access_token":%q,"account_id":"acct"}}`, token)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

// tokenServer answers 200 for the bearer token good and 401 for anything
// else, and counts the requests.
func tokenServer(t *testing.T, good string) (*httptest.Server, *int) {
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
		fmt.Fprint(w, `{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":25}}}`)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func refreshFixture(t *testing.T, runner *refreshRunner, baseURL string) (*Provider, string) {
	t.Helper()
	clearEnv(t)
	path := filepath.Join(t.TempDir(), "auth.json")
	p := New(WithCredentialPath(path), WithBaseURL(baseURL), WithRefreshRunner(runner),
		WithPiCredentialPath(filepath.Join(os.Getenv("PI_CODING_AGENT_DIR"), "auth.json")))
	return p, path
}

func TestRefresh_UnauthorizedRunsCodexDoctorAndRetriesOnce(t *testing.T) {
	srv, requests := tokenServer(t, "fresh")
	runner := &refreshRunner{}
	p, path := refreshFixture(t, runner, srv.URL)
	writeAuth(t, path, "stale")
	runner.do = func(context.Context) error {
		writeAuth(t, path, "fresh")
		return nil
	}

	usage, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil after the refresh", err)
	}
	if len(usage.Windows) != 1 {
		t.Fatalf("usage = %+v, want one window", usage)
	}
	if runner.count() != 1 || runner.calls[0] != "codex doctor" {
		t.Fatalf("refresh calls = %q, want one codex doctor", runner.calls)
	}
	if *requests != 2 {
		t.Fatalf("requests = %d, want 2", *requests)
	}
}

func TestRefresh_NoRefreshWithoutA401(t *testing.T) {
	srv, _ := tokenServer(t, "valid")
	runner := &refreshRunner{}
	p, path := refreshFixture(t, runner, srv.URL)
	writeAuth(t, path, "valid")
	if _, err := p.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runner.count() != 0 {
		t.Fatalf("refreshes = %d, want 0", runner.count())
	}
}

func TestRefresh_AtMostOneRefreshPerFetch(t *testing.T) {
	srv, requests := tokenServer(t, "never")
	runner := &refreshRunner{} // "succeeds" but leaves the store as it was
	p, path := refreshFixture(t, runner, srv.URL)
	writeAuth(t, path, "stale")

	_, err := p.Fetch(context.Background())
	var expired provider.ErrTokenExpired
	if !errors.As(err, &expired) || expired.Tool != tool {
		t.Fatalf("Fetch() error = %v, want the codex token-expired error", err)
	}
	if runner.count() != 1 || *requests != 2 {
		t.Fatalf("refreshes = %d, requests = %d, want 1 and 2", runner.count(), *requests)
	}
}

func TestRefresh_FailedRefreshIsTriedAgainOnTheNextFetch(t *testing.T) {
	srv, requests := tokenServer(t, "fresh")
	runner := &refreshRunner{do: func(context.Context) error { return errors.New("boom") }}
	p, path := refreshFixture(t, runner, srv.URL)
	writeAuth(t, path, "stale")

	for i := 0; i < 2; i++ {
		if _, err := p.Fetch(context.Background()); !errors.Is(err, provider.ErrTokenExpired{}) {
			t.Fatalf("Fetch() #%d error = %v, want token expired", i+1, err)
		}
	}
	if runner.count() != 2 || *requests != 2 {
		t.Fatalf("refreshes = %d, requests = %d, want 2 and 2", runner.count(), *requests)
	}
}

func TestRefresh_NativeBeforePi(t *testing.T) {
	for _, tc := range []struct {
		name      string
		refreshOK bool
		want      []string
	}{
		{"refresh succeeds: native token", true, []string{"Bearer stale", "Bearer fresh"}},
		{"refresh fails: Pi token", false, []string{"Bearer stale", "Bearer pi-access"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var tokens []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				tokens = append(tokens, r.Header.Get("Authorization"))
				mu.Unlock()
				if r.Header.Get("Authorization") == "Bearer stale" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				fmt.Fprint(w, `{"rate_limit":{"primary_window":{"used_percent":25}}}`)
			}))
			defer srv.Close()
			runner := &refreshRunner{}
			p, path := refreshFixture(t, runner, srv.URL)
			writeAuth(t, path, "stale")
			p.piCredentialPath = writePi(t, "pi-access", "pi-account", time.Now().Add(time.Hour).UnixMilli())
			runner.do = func(context.Context) error {
				if !tc.refreshOK {
					return errors.New("boom")
				}
				writeAuth(t, path, "fresh")
				return nil
			}

			if _, err := p.Fetch(context.Background()); err != nil {
				t.Fatalf("Fetch() error = %v", err)
			}
			if runner.count() != 1 {
				t.Fatalf("refreshes = %d, want 1", runner.count())
			}
			if strings.Join(tokens, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("tokens sent = %q, want %q", tokens, tc.want)
			}
		})
	}
}

func TestRefresh_NeverForEnvOrPiCredentials(t *testing.T) {
	t.Run("env override", func(t *testing.T) {
		srv, _ := tokenServer(t, "never")
		runner := &refreshRunner{}
		p, path := refreshFixture(t, runner, srv.URL)
		writeAuth(t, path, "stale")
		t.Setenv(tokenEnvVar, "override")
		if _, err := p.Fetch(context.Background()); !errors.Is(err, provider.ErrTokenExpired{}) {
			t.Fatalf("Fetch() error = %v, want token expired", err)
		}
		if runner.count() != 0 {
			t.Fatalf("refreshes = %d, want 0 for %s", runner.count(), tokenEnvVar)
		}
	})
	t.Run("pi source", func(t *testing.T) {
		srv, _ := tokenServer(t, "never")
		runner := &refreshRunner{}
		p, _ := refreshFixture(t, runner, srv.URL) // no native store
		p.piCredentialPath = writePi(t, "pi-access", "pi-account", time.Now().Add(time.Hour).UnixMilli())
		if _, err := p.Fetch(context.Background()); !errors.Is(err, provider.ErrTokenExpired{}) {
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
	writeAuth(t, path, "stale")
	if ok, reason := p.Detect(context.Background()); !ok {
		t.Fatalf("Detect() = false, %q", reason)
	}
	if runner.count() != 0 {
		t.Fatalf("Detect ran %d refreshes, want 0", runner.count())
	}
}

func TestRefresh_FailureReasonIsInTheHint(t *testing.T) {
	srv, _ := tokenServer(t, "never")
	runner := &refreshRunner{do: func(context.Context) error { return &exec.Error{Name: "codex", Err: exec.ErrNotFound} }}
	p, path := refreshFixture(t, runner, srv.URL)
	writeAuth(t, path, "stale")

	_, err := p.Fetch(context.Background())
	const want = "token expired, open codex to refresh (auto-refresh failed: codex not found)"
	var expired provider.ErrTokenExpired
	if !errors.As(err, &expired) || expired.Error() != want {
		t.Fatalf("Fetch() error = %v, want the typed error %q", err, want)
	}
}

func TestRefresh_NoReasonWhenTheRefreshSucceeds(t *testing.T) {
	srv, _ := tokenServer(t, "never")
	p, path := refreshFixture(t, &refreshRunner{}, srv.URL)
	writeAuth(t, path, "stale")

	_, err := p.Fetch(context.Background())
	var expired provider.ErrTokenExpired
	if !errors.As(err, &expired) || expired.Error() != "token expired, open codex to refresh" {
		t.Fatalf("Fetch() error = %v, want the plain hint", err)
	}
}

func TestRefresh_CutShortByTheFetchDeadlineReportsTheDeadline(t *testing.T) {
	srv, _ := tokenServer(t, "never")
	runner := &refreshRunner{do: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }}
	p, path := refreshFixture(t, runner, srv.URL)
	writeAuth(t, path, "stale")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
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
	// hang answers 401 to the native token and holds every other request
	// until the client gives up.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer native" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		<-r.Context().Done()
	}))
	defer srv.Close()
	deadline := func(t *testing.T) context.Context {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		t.Cleanup(cancel)
		return ctx
	}

	t.Run("rejected token, Pi request times out", func(t *testing.T) {
		runner := &refreshRunner{do: func(context.Context) error { return &exec.Error{Name: "codex", Err: exec.ErrNotFound} }}
		p, path := refreshFixture(t, runner, srv.URL)
		writeAuth(t, path, "native")
		p.piCredentialPath = writePi(t, "pi-access", "pi-account", time.Now().Add(time.Hour).UnixMilli())

		_, err := p.Fetch(deadline(t))
		if !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "not found") {
			t.Fatalf("Fetch() error = %v, want the Pi request deadline", err)
		}
	})

	// codex.go: the final error. The refresh fails with its own reason just
	// as the deadline passes, and no Pi credential exists.
	t.Run("rejected token, refresh fails as the deadline passes", func(t *testing.T) {
		late := func(ctx context.Context) error { <-ctx.Done(); return errors.New("exit status 1") }
		p, path := refreshFixture(t, &refreshRunner{do: late}, srv.URL)
		writeAuth(t, path, "native")

		_, err := p.Fetch(deadline(t))
		var expired provider.ErrTokenExpired
		if !errors.As(err, &expired) || expired.RefreshFailure != "codex doctor: exit status 1" {
			t.Fatalf("Fetch() error = %v, want the token hint with the refresh failure", err)
		}
	})
}
