package claude

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/lib/credstore"
	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// testNow sits between the expiresAt in testdata/credentials_expired.json
// (1757800000000) and the one in testdata/credentials_valid.json
// (1757900000000), so the same injected clock makes one store expired and the
// other usable.
var testNow = time.UnixMilli(1757850000000)

// fixture returns the path of a testdata file.
func fixture(name string) string { return filepath.Join("testdata", name) }

// newProvider builds a Provider pinned to a fixture credential file and the
// frozen test clock. Later options override the defaults.
func newProvider(t *testing.T, opts ...Option) *Provider {
	t.Helper()
	clearEnv(t)
	base := []Option{
		WithCredentialPath(fixture("credentials_valid.json")),
		WithClock(func() time.Time { return testNow }),
	}
	return New(append(base, opts...)...)
}

// clearEnv makes QMETER_CLAUDE_TOKEN look unset for this test even if the
// developer running the suite has it exported. t.Setenv forbids t.Parallel.
func clearEnv(t *testing.T) {
	t.Helper()
	t.Setenv(envVar, "")
}

// testContext returns a context with a deadline: httpx sets no timeout of its
// own, so every caller must supply one.
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestID_MatchesCLIProviderName(t *testing.T) {
	if got := New().ID(); got != "claude" {
		t.Errorf("ID() = %q, want %q", got, "claude")
	}
}

func TestCredentials_EnvOverrideWins(t *testing.T) {
	p := newProvider(t)
	t.Setenv(envVar, "  env-token  ")

	cred, src, err := p.resolve(testContext(t))
	if err != nil {
		t.Fatalf("resolve() error = %v, want nil", err)
	}
	if src != credstore.SourceEnv {
		t.Errorf("source = %q, want %q", src, credstore.SourceEnv)
	}
	// Verbatim, whitespace included: credstore hands the value straight to
	// fromEnv and this provider does not second-guess it.
	if cred.AccessToken != "  env-token  " {
		t.Errorf("AccessToken = %q, want %q", cred.AccessToken, "  env-token  ")
	}
	if cred.Plan != "" {
		t.Errorf("Plan = %q, want empty (only the local store knows the plan)", cred.Plan)
	}
}

func TestCredentials_StoreSuppliesTokenAndPlan(t *testing.T) {
	p := newProvider(t)

	cred, src, err := p.resolve(testContext(t))
	if err != nil {
		t.Fatalf("resolve() error = %v, want nil", err)
	}
	if src != credstore.SourceStore {
		t.Errorf("source = %q, want %q", src, credstore.SourceStore)
	}
	if cred.AccessToken != "sk-ant-oat01-store-token" {
		t.Errorf("AccessToken = %q, want the fixture's accessToken", cred.AccessToken)
	}
	if cred.Plan != "max" {
		t.Errorf("Plan = %q, want %q (subscriptionType)", cred.Plan, "max")
	}
}

func TestCredentials_MissingFileReturnsNotLoggedIn(t *testing.T) {
	p := newProvider(t, WithCredentialPath(filepath.Join(t.TempDir(), ".credentials.json")))

	_, _, err := p.resolve(testContext(t))
	if !errors.Is(err, provider.ErrNotLoggedIn{}) {
		t.Fatalf("resolve() error = %v, want provider.ErrNotLoggedIn", err)
	}
	var notLoggedIn provider.ErrNotLoggedIn
	if !errors.As(err, &notLoggedIn) {
		t.Fatalf("errors.As(%v, *ErrNotLoggedIn) = false", err)
	}
	if notLoggedIn.Tool != "claude" {
		t.Errorf("Tool = %q, want %q", notLoggedIn.Tool, "claude")
	}
	if got := err.Error(); got != "not logged in, run claude to log in" {
		t.Errorf("Error() = %q, want %q", got, "not logged in, run claude to log in")
	}
}

func TestCredentials_ExpiredReturnsTokenExpired(t *testing.T) {
	p := newProvider(t, WithCredentialPath(fixture("credentials_expired.json")))

	_, _, err := p.resolve(testContext(t))
	if !errors.Is(err, provider.ErrTokenExpired{}) {
		t.Fatalf("resolve() error = %v, want provider.ErrTokenExpired", err)
	}
	var expired provider.ErrTokenExpired
	if !errors.As(err, &expired) {
		t.Fatalf("errors.As(%v, *ErrTokenExpired) = false", err)
	}
	if expired.Tool != "claude" {
		t.Errorf("Tool = %q, want %q", expired.Tool, "claude")
	}
	if got := expired.Error(); got != "token expired, open claude to refresh" {
		t.Errorf("Error() = %q, want %q", got, "token expired, open claude to refresh")
	}
}

func TestCredentials_ExpiryIsInclusive(t *testing.T) {
	// expiresAt <= now means expired, so the exact expiry instant counts as
	// expired, not as one last usable millisecond.
	atExpiry := func() time.Time { return time.UnixMilli(1757900000000) }
	p := newProvider(t, WithClock(atExpiry))

	if _, _, err := p.resolve(testContext(t)); !errors.Is(err, provider.ErrTokenExpired{}) {
		t.Fatalf("resolve() error = %v, want provider.ErrTokenExpired", err)
	}
}

func TestCredentials_NoExpiresAtIsNotExpired(t *testing.T) {
	// A store without expiresAt says nothing about expiry; qmeter uses the
	// token and lets a 401 from the endpoint be the authority.
	p := newProvider(t, WithCredentialPath(fixture("credentials_no_expiry.json")))

	cred, _, err := p.resolve(testContext(t))
	if err != nil {
		t.Fatalf("resolve() error = %v, want nil", err)
	}
	if cred.AccessToken != "sk-ant-oat01-no-expiry-token" {
		t.Errorf("AccessToken = %q, want the fixture's accessToken", cred.AccessToken)
	}
}

func TestCredentials_EmptyAccessTokenReturnsNotLoggedIn(t *testing.T) {
	p := newProvider(t, WithCredentialPath(fixture("credentials_no_token.json")))

	_, _, err := p.resolve(testContext(t))
	if !errors.Is(err, provider.ErrNotLoggedIn{}) {
		t.Fatalf("resolve() error = %v, want provider.ErrNotLoggedIn", err)
	}
}

func TestCredentials_MalformedStoreReportsCredentialStoreError(t *testing.T) {
	path := fixture("credentials_malformed.json")
	p := newProvider(t, WithCredentialPath(path))

	_, _, err := p.resolve(testContext(t))
	if err == nil {
		t.Fatal("resolve() error = nil, want a parse error")
	}
	if errors.Is(err, provider.ErrNotLoggedIn{}) {
		t.Fatalf("resolve() error = %v, want a parse error, not ErrNotLoggedIn", err)
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "credential store: ") {
		t.Errorf("Error() = %q, want the credstore \"credential store: \" prefix", msg)
	}
	if !strings.Contains(msg, path) {
		t.Errorf("Error() = %q, want it to name the file %q", msg, path)
	}
}

func TestCredentials_UnreadableFileIsNotSwallowed(t *testing.T) {
	// A directory where the file should be: os.ReadFile fails with something
	// other than os.ErrNotExist, so it must surface, not become "not logged
	// in".
	dir := filepath.Join(t.TempDir(), ".credentials.json")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	p := newProvider(t, WithCredentialPath(dir))

	_, _, err := p.resolve(testContext(t))
	if err == nil || errors.Is(err, provider.ErrNotLoggedIn{}) {
		t.Fatalf("resolve() error = %v, want a read error", err)
	}
}

func TestDetect_EnvOverride(t *testing.T) {
	// No store at all: the override alone is enough, and Detect does no I/O
	// on the file.
	p := newProvider(t, WithCredentialPath(filepath.Join(t.TempDir(), ".credentials.json")))
	t.Setenv(envVar, "env-token")

	ok, reason := p.Detect(testContext(t))
	if !ok {
		t.Fatalf("Detect() = false, %q; want true", reason)
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty", reason)
	}
}

func TestDetect_StorePresent(t *testing.T) {
	p := newProvider(t)

	ok, reason := p.Detect(testContext(t))
	if !ok {
		t.Fatalf("Detect() = false, %q; want true", reason)
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty", reason)
	}
}

func TestDetect_ExpiredStoreStillCountsAsDetected(t *testing.T) {
	// The store exists and parses, so the provider is detected; Fetch is what
	// reports the expiry.
	p := newProvider(t, WithCredentialPath(fixture("credentials_expired.json")))

	ok, reason := p.Detect(testContext(t))
	if !ok {
		t.Fatalf("Detect() = false, %q; want true for an expired-but-present store", reason)
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty", reason)
	}
}

func TestDetect_StoreMissingReturnsReason(t *testing.T) {
	p := newProvider(t, WithCredentialPath(filepath.Join(t.TempDir(), ".credentials.json")))

	ok, reason := p.Detect(testContext(t))
	if ok {
		t.Fatal("Detect() = true, want false")
	}
	// Exact text: it is shown to the user as-is, carries the tool name (not
	// the env var name), and must have no provider-name prefix.
	if reason != "not logged in, run claude to log in" {
		t.Errorf("reason = %q, want %q", reason, "not logged in, run claude to log in")
	}
}

func TestDetect_MalformedStoreReturnsReason(t *testing.T) {
	p := newProvider(t, WithCredentialPath(fixture("credentials_malformed.json")))

	ok, reason := p.Detect(testContext(t))
	if ok {
		t.Fatal("Detect() = true, want false for a store that does not parse")
	}
	if !strings.HasPrefix(reason, "credential store: ") {
		t.Errorf("reason = %q, want the \"credential store: \" prefix", reason)
	}
	if strings.HasPrefix(reason, "claude") {
		t.Errorf("reason = %q, must not be prefixed with the provider name", reason)
	}
}

func TestDefaultCredentialPath_IsHomeRelative(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := defaultCredentialPath()
	if err != nil {
		t.Fatalf("defaultCredentialPath() error = %v", err)
	}
	if want := filepath.Join(home, ".claude", ".credentials.json"); got != want {
		t.Errorf("defaultCredentialPath() = %q, want %q", got, want)
	}
}

// recorder captures what the fake usage endpoint was asked for. Its mutex
// keeps the handler goroutine and the test goroutine race-free.
type recorder struct {
	mu     sync.Mutex
	calls  int
	method string
	path   string
	header http.Header
}

func (r *recorder) record(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.method, r.path, r.header = req.Method, req.URL.Path, req.Header.Clone()
}

func (r *recorder) snapshot() (calls int, method, path string, header http.Header) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, r.method, r.path, r.header
}

// serveJSON starts a fake usage endpoint answering with status and body.
func serveJSON(t *testing.T, status int, headers map[string]string, body []byte) (*httptest.Server, *recorder) {
	t.Helper()
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

// serveFixture starts a fake usage endpoint answering 200 with a testdata
// file.
func serveFixture(t *testing.T, name string) (*httptest.Server, *recorder) {
	t.Helper()
	body, err := os.ReadFile(fixture(name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return serveJSON(t, http.StatusOK, nil, body)
}

// pointAt aims a Provider at a fake endpoint.
func pointAt(srv *httptest.Server) []Option {
	return []Option{WithBaseURL(srv.URL), WithHTTPClient(srv.Client())}
}

func assertWindows(t *testing.T, got, want []provider.Window) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d windows %+v, want %d %+v", len(got), got, len(want), want)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Provider != w.Provider || g.Name != w.Name || g.Plan != w.Plan ||
			g.UsedPercent != w.UsedPercent || g.Period != w.Period || !g.ResetsAt.Equal(w.ResetsAt) {
			t.Errorf("window %d = %+v, want %+v", i, g, w)
		}
		if g.RateLimited {
			t.Errorf("window %d RateLimited = true; a 200 response is never rate limited", i)
		}
	}
}

func TestFetch_ParsesFourWindows(t *testing.T) {
	srv, rec := serveFixture(t, "usage_four_windows.json")
	p := newProvider(t, pointAt(srv)...)

	got, err := p.Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}

	fiveHourReset := time.Date(2026, 9, 16, 18, 30, 0, 0, time.UTC)
	weeklyReset := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	const day = 24 * time.Hour
	assertWindows(t, got, []provider.Window{
		{Provider: "claude", Name: "5h", Plan: "max", UsedPercent: 42, ResetsAt: fiveHourReset, Period: 5 * time.Hour},
		{Provider: "claude", Name: "weekly", Plan: "max", UsedPercent: 18.5, ResetsAt: weeklyReset, Period: 7 * day},
		{Provider: "claude", Name: "sonnet weekly", Plan: "max", UsedPercent: 7, ResetsAt: weeklyReset, Period: 7 * day},
		{Provider: "claude", Name: "opus weekly", Plan: "max", UsedPercent: 0, ResetsAt: weeklyReset, Period: 7 * day},
	})

	calls, method, path, header := rec.snapshot()
	if calls != 1 {
		t.Errorf("endpoint calls = %d, want 1", calls)
	}
	if method != http.MethodGet {
		t.Errorf("method = %q, want GET", method)
	}
	if path != "/api/oauth/usage" {
		t.Errorf("path = %q, want %q", path, "/api/oauth/usage")
	}
	if got, want := header.Get("Authorization"), "Bearer sk-ant-oat01-store-token"; got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
	if got, want := header.Get("anthropic-beta"), "oauth-2025-04-20"; got != want {
		t.Errorf("anthropic-beta = %q, want %q", got, want)
	}
}

func TestFetch_EnvOverrideLeavesPlanEmpty(t *testing.T) {
	srv, rec := serveFixture(t, "usage_four_windows.json")
	p := newProvider(t, pointAt(srv)...)
	t.Setenv(envVar, "env-token")

	got, err := p.Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d windows, want 4", len(got))
	}
	for i, w := range got {
		if w.Plan != "" {
			t.Errorf("window %d Plan = %q, want empty: the override carries no plan name", i, w.Plan)
		}
	}
	// The override token is what reaches the endpoint; the store is not read.
	_, _, _, header := rec.snapshot()
	if got, want := header.Get("Authorization"), "Bearer env-token"; got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

func TestFetch_ParsesScopedPerModelLimits_SkipsUnknown(t *testing.T) {
	srv, _ := serveFixture(t, "usage_scoped_limits.json")
	p := newProvider(t, pointAt(srv)...)

	got, err := p.Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}

	// Kept: the documented five_hour window, then the two well-formed scoped
	// entries, named by their group. Skipped: an entry with no group, one
	// with no percent, a non-object list element, an entry whose group
	// duplicates a window already reported, and an unknown object that is not
	// a scoped-limit list at all.
	assertWindows(t, got, []provider.Window{
		{Provider: "claude", Name: "5h", Plan: "max", UsedPercent: 12,
			ResetsAt: time.Date(2026, 9, 16, 18, 30, 0, 0, time.UTC), Period: 5 * time.Hour},
		{Provider: "claude", Name: "claude-opus-4-6", Plan: "max", UsedPercent: 55,
			ResetsAt: time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)},
		{Provider: "claude", Name: "claude-haiku-4-5", Plan: "max", UsedPercent: 3.5,
			ResetsAt: time.Unix(1789776000, 0).UTC()},
	})
}

func TestFetch_ResetsAtAcceptsISOStringAndEpochNumber(t *testing.T) {
	srv, _ := serveFixture(t, "usage_epoch_resets.json")
	p := newProvider(t, pointAt(srv)...)

	got, err := p.Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d windows %+v, want 4", len(got), got)
	}

	epoch := time.Unix(1789776000, 0).UTC()
	// Epoch seconds and the same instant in epoch milliseconds both decode to
	// that instant; an unparseable string and a null both leave ResetsAt
	// zero rather than dropping the window.
	for i, want := range []time.Time{epoch, epoch, {}, {}} {
		if !got[i].ResetsAt.Equal(want) {
			t.Errorf("window %d (%s) ResetsAt = %v, want %v", i, got[i].Name, got[i].ResetsAt, want)
		}
	}
	if !got[2].ResetsAt.IsZero() || !got[3].ResetsAt.IsZero() {
		t.Errorf("unparseable and null resets_at must leave a zero ResetsAt, got %v and %v",
			got[2].ResetsAt, got[3].ResetsAt)
	}
}

func TestFetch_SkipsWindowsWithoutUtilization(t *testing.T) {
	srv, _ := serveFixture(t, "usage_partial.json")
	p := newProvider(t, pointAt(srv)...)

	got, err := p.Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}
	assertWindows(t, got, []provider.Window{
		{Provider: "claude", Name: "5h", Plan: "max", UsedPercent: 5,
			ResetsAt: time.Date(2026, 9, 16, 18, 30, 0, 0, time.UTC), Period: 5 * time.Hour},
	})
}

func TestFetch_429RendersAsErrRateLimited(t *testing.T) {
	srv, _ := serveJSON(t, http.StatusTooManyRequests,
		map[string]string{"Retry-After": "1500"},
		[]byte(`{"error":{"type":"rate_limit_error"}}`))
	p := newProvider(t, pointAt(srv)...)

	_, err := p.Fetch(testContext(t))
	if !errors.Is(err, provider.ErrRateLimited{}) {
		t.Fatalf("Fetch() error = %v, want provider.ErrRateLimited", err)
	}
	var limited provider.ErrRateLimited
	if !errors.As(err, &limited) {
		t.Fatalf("errors.As(%v, *ErrRateLimited) = false", err)
	}
	if limited.RetryAfter != 25*time.Minute {
		t.Errorf("RetryAfter = %v, want %v", limited.RetryAfter, 25*time.Minute)
	}
	if got := limited.Error(); got != "rate limited, retry in 25m0s" {
		t.Errorf("Error() = %q, want %q", got, "rate limited, retry in 25m0s")
	}
}

func TestFetch_401ReturnsTokenExpiredNamingClaude(t *testing.T) {
	srv, _ := serveJSON(t, http.StatusUnauthorized, nil, []byte(`{"error":{"type":"authentication_error"}}`))
	p := newProvider(t, pointAt(srv)...)

	_, err := p.Fetch(testContext(t))
	var expired provider.ErrTokenExpired
	if !errors.As(err, &expired) {
		t.Fatalf("Fetch() error = %v, want provider.ErrTokenExpired", err)
	}
	// httpx builds this from Options.Tool; an unset Tool would render
	// "token expired, open  to refresh".
	if got := expired.Error(); got != "token expired, open claude to refresh" {
		t.Errorf("Error() = %q, want %q", got, "token expired, open claude to refresh")
	}
}

func TestFetch_ExpiredStoreMakesNoRequest(t *testing.T) {
	srv, rec := serveFixture(t, "usage_four_windows.json")
	opts := append(pointAt(srv), WithCredentialPath(fixture("credentials_expired.json")))
	p := newProvider(t, opts...)

	_, err := p.Fetch(testContext(t))
	if !errors.Is(err, provider.ErrTokenExpired{}) {
		t.Fatalf("Fetch() error = %v, want provider.ErrTokenExpired", err)
	}
	if calls, _, _, _ := rec.snapshot(); calls != 0 {
		t.Errorf("endpoint calls = %d, want 0: an expired credential is caught locally", calls)
	}
}

func TestFetch_MissingCredentialReturnsNotLoggedInVerbatim(t *testing.T) {
	srv, _ := serveFixture(t, "usage_four_windows.json")
	opts := append(pointAt(srv), WithCredentialPath(filepath.Join(t.TempDir(), ".credentials.json")))
	p := newProvider(t, opts...)

	_, err := p.Fetch(testContext(t))
	if !errors.Is(err, provider.ErrNotLoggedIn{}) {
		t.Fatalf("Fetch() error = %v, want provider.ErrNotLoggedIn", err)
	}
	// Credential errors pass through unwrapped: credstore has already said
	// everything there is to say, and a second prefix would double up in the
	// rendered failure line.
	if got := err.Error(); got != "not logged in, run claude to log in" {
		t.Errorf("Error() = %q, want %q", got, "not logged in, run claude to log in")
	}
}

func TestFetch_MalformedStoreSaysCredentialStoreOnce(t *testing.T) {
	srv, _ := serveFixture(t, "usage_four_windows.json")
	opts := append(pointAt(srv), WithCredentialPath(fixture("credentials_malformed.json")))
	p := newProvider(t, opts...)

	_, err := p.Fetch(testContext(t))
	if err == nil {
		t.Fatal("Fetch() error = nil, want a parse error")
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "credential store: parse ") {
		t.Errorf("Error() = %q, want it to start with %q", msg, "credential store: parse ")
	}
	if n := strings.Count(msg, "credential store: "); n != 1 {
		t.Errorf("Error() = %q, says %q %d times, want exactly once", msg, "credential store: ", n)
	}
}

func TestFetch_MalformedResponseBodyErrors(t *testing.T) {
	srv, _ := serveFixture(t, "usage_malformed.json")
	p := newProvider(t, pointAt(srv)...)

	got, err := p.Fetch(testContext(t))
	if err == nil {
		t.Fatalf("Fetch() = %+v, nil; want a decode error", got)
	}
	if !strings.HasPrefix(err.Error(), "usage request: ") {
		t.Errorf("Error() = %q, want it wrapped with an operation prefix", err.Error())
	}
}

func TestFetch_UsesDefaultBaseURLWhenUnset(t *testing.T) {
	// Nothing is fetched here: the point is that the endpoint URL is built
	// from the documented origin and path.
	p := New()
	if got, want := p.usageURL(), "https://api.anthropic.com/api/oauth/usage"; got != want {
		t.Errorf("usageURL() = %q, want %q", got, want)
	}
}

func TestFetch_ScopedLimitWithoutResetsAtIsSkipped(t *testing.T) {
	// A top-level list can be something other than scoped limits. Requiring a
	// parseable resets_at alongside group and percent is what keeps a notice
	// like {"group":"billing","percent":100} from rendering as a fabricated
	// 100%-used window.
	srv, _ := serveFixture(t, "usage_notice_list.json")
	p := newProvider(t, pointAt(srv)...)

	got, err := p.Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}
	assertWindows(t, got, []provider.Window{
		{Provider: "claude", Name: "5h", Plan: "max", UsedPercent: 12,
			ResetsAt: time.Date(2026, 9, 16, 18, 30, 0, 0, time.UTC), Period: 5 * time.Hour},
	})
}

func TestFetch_ScopedLimitsOrderedBySectionKey(t *testing.T) {
	// Known windows come first, then the scoped sections in sorted key order,
	// so window order never depends on Go's randomized map iteration.
	srv, _ := serveFixture(t, "usage_scoped_ordering.json")
	p := newProvider(t, pointAt(srv)...)

	got, err := p.Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}
	// Four scoped sections, so an unsorted implementation cannot pass by
	// landing on the right permutation: only 1 of the 24 orders is sorted.
	scopedReset := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	assertWindows(t, got, []provider.Window{
		{Provider: "claude", Name: "5h", Plan: "max", UsedPercent: 12,
			ResetsAt: time.Date(2026, 9, 16, 18, 30, 0, 0, time.UTC), Period: 5 * time.Hour},
		{Provider: "claude", Name: "alpha-model", Plan: "max", UsedPercent: 20, ResetsAt: scopedReset},
		{Provider: "claude", Name: "bravo-model", Plan: "max", UsedPercent: 40, ResetsAt: scopedReset},
		{Provider: "claude", Name: "mike-model", Plan: "max", UsedPercent: 60, ResetsAt: scopedReset},
		{Provider: "claude", Name: "zulu-model", Plan: "max", UsedPercent: 80, ResetsAt: scopedReset},
	})
}

func TestFetch_ResponseWithNoKnownWindowsErrors(t *testing.T) {
	// A 200 qmeter cannot read anything out of is a failure to report, not a
	// provider that silently contributes no lines.
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "empty object", body: `{}`},
		{name: "null", body: `null`},
		{name: "only unknown fields", body: `{"account_uuid":"1111","organization":{"name":"x"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := serveJSON(t, http.StatusOK, nil, []byte(tc.body))
			p := newProvider(t, pointAt(srv)...)

			got, err := p.Fetch(testContext(t))
			if err == nil {
				t.Fatalf("Fetch() = %+v, nil; want an error", got)
			}
			if err.Error() != "usage response carried no known windows" {
				t.Errorf("Error() = %q, want %q", err.Error(), "usage response carried no known windows")
			}
			if got != nil {
				t.Errorf("windows = %+v, want nil", got)
			}
		})
	}
}
