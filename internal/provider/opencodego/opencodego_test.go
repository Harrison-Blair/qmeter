package opencodego

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
	_ "time/tzdata"

	"github.com/Harrison-Blair/qmeter/internal/lib/credstore"
	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// missingStore returns a path inside a fresh temp dir where no file exists, so
// a test never reads the developer's real ~/.local/share/opencode/auth.json.
func missingStore(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "auth.json")
}

// clearEnv makes the env override look unset for the duration of the test.
// credstore treats an empty value as unset, so this neutralizes whatever the
// developer happens to have exported.
func clearEnv(t *testing.T) {
	t.Helper()
	t.Setenv("QMETER_OPENCODE_GO_KEY", "")
}

func TestID_MatchesCLIProviderName(t *testing.T) {
	if got := New().ID(); got != "opencode-go" {
		t.Errorf("ID() = %q, want %q", got, "opencode-go")
	}
}

func TestDetect_EnvOverride(t *testing.T) {
	t.Setenv("QMETER_OPENCODE_GO_KEY", "sk-from-env")

	// The store is deliberately absent: the override alone must be enough.
	p := New(WithCredentialPath(missingStore(t)))

	ok, reason := p.Detect(context.Background())
	if !ok {
		t.Errorf("Detect() ok = false, reason %q; want true", reason)
	}
	if reason != "" {
		t.Errorf("Detect() reason = %q, want empty", reason)
	}
}

func TestDetect_StorePresent(t *testing.T) {
	clearEnv(t)

	p := New(WithCredentialPath(filepath.Join("testdata", "auth_go.json")))

	ok, reason := p.Detect(context.Background())
	if !ok {
		t.Errorf("Detect() ok = false, reason %q; want true", reason)
	}
	if reason != "" {
		t.Errorf("Detect() reason = %q, want empty", reason)
	}
}

func TestDetect_StoreMissingReturnsReason(t *testing.T) {
	clearEnv(t)

	p := New(WithCredentialPath(missingStore(t)))

	ok, reason := p.Detect(context.Background())
	if ok {
		t.Error("Detect() ok = true, want false")
	}
	// Exact text: it is rendered verbatim and a swapped tool/env argument to
	// credstore.Resolve would show up right here.
	if want := "not logged in, run opencode to log in"; reason != want {
		t.Errorf("Detect() reason = %q, want %q", reason, want)
	}
}

func TestDetect_UnparsableStoreExplainsInsteadOfClaimingLogin(t *testing.T) {
	clearEnv(t)

	p := New(WithCredentialPath(filepath.Join("testdata", "auth_malformed.json")))

	ok, reason := p.Detect(context.Background())
	if ok {
		t.Error("Detect() ok = true, want false for a store that does not parse")
	}
	if !strings.HasPrefix(reason, "credential store: ") {
		t.Errorf("Detect() reason = %q, want it to start with %q", reason, "credential store: ")
	}
	if !strings.Contains(reason, "auth_malformed.json") {
		t.Errorf("Detect() reason = %q, want it to name the unreadable file", reason)
	}
	// The reason is shown next to the provider name already.
	if strings.Contains(reason, "opencode-go:") {
		t.Errorf("Detect() reason = %q, must not be prefixed with the provider name", reason)
	}
}

func TestCredentials_FileWithoutGoEntryIsNotLoggedIn(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
	}{
		{name: "no opencode-go entry", fixture: "auth_no_go_entry.json"},
		{name: "empty key", fixture: "auth_empty_key.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)

			path := filepath.Join("testdata", tt.fixture)

			// A file that parses but carries no usable key is "not logged
			// in", not a store error.
			if _, err := loadKey(path); !errors.Is(err, credstore.ErrNotFound) {
				t.Errorf("loadKey() err = %v, want credstore.ErrNotFound", err)
			}

			p := New(WithCredentialPath(path))
			ok, reason := p.Detect(context.Background())
			if ok {
				t.Error("Detect() ok = true, want false")
			}
			if want := "not logged in, run opencode to log in"; reason != want {
				t.Errorf("Detect() reason = %q, want %q", reason, want)
			}
		})
	}
}

func TestCredentials_MissingFileReturnsNotLoggedInFromFetch(t *testing.T) {
	clearEnv(t)

	p := New(WithCredentialPath(missingStore(t)))

	_, err := p.Fetch(context.Background())
	if !errors.Is(err, provider.ErrNotLoggedIn{}) {
		t.Fatalf("Fetch() err = %v, want provider.ErrNotLoggedIn", err)
	}
	var notLoggedIn provider.ErrNotLoggedIn
	if !errors.As(err, &notLoggedIn) {
		t.Fatalf("Fetch() err = %v, want errors.As to a provider.ErrNotLoggedIn", err)
	}
	if notLoggedIn.Tool != "opencode" {
		t.Errorf("ErrNotLoggedIn.Tool = %q, want %q", notLoggedIn.Tool, "opencode")
	}
}

func TestDefaultCredentialPath_IsHomeRelativeOpenCodeAuthFile(t *testing.T) {
	// Set the home directory this test reads rather than depending on the
	// one the test runner happens to have: with HOME unset, os.UserHomeDir
	// fails and the assertion would be about the environment, not the code.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // what os.UserHomeDir reads on Windows

	want := filepath.Join(home, ".local", "share", "opencode", "auth.json")
	if got := defaultCredentialPath(); got != want {
		t.Errorf("defaultCredentialPath() = %q, want %q", got, want)
	}
}

func TestCredentials_UnknownHomeIsNotLoggedIn(t *testing.T) {
	// defaultCredentialPath returns "" when the home directory cannot be
	// determined; the loader must treat that as an absent store rather than
	// reading some relative path.
	if _, err := loadKey(""); !errors.Is(err, credstore.ErrNotFound) {
		t.Errorf("loadKey(\"\") err = %v, want credstore.ErrNotFound", err)
	}
}

func TestCredentials_TrailingWhitespaceInStoredKeyIsTrimmed(t *testing.T) {
	clearEnv(t)
	path := filepath.Join("testdata", "auth_trailing_newline.json")

	const want = "sk-opencode-go-newline-key"
	got, err := loadKey(path)
	if err != nil {
		t.Fatalf("loadKey() err = %v", err)
	}
	if got != want {
		t.Errorf("loadKey() = %q, want %q", got, want)
	}

	// The real damage of an untrimmed key shows up on the wire: net/http
	// rejects a header value containing a newline, so the request never
	// leaves the process even though Detect said all was well.
	srv, rec := fixtureServer(t, "usage_weekly_only.json")
	p := New(
		WithCredentialPath(path),
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)
	if _, err := p.Fetch(testContext(t)); err != nil {
		t.Fatalf("Fetch() err = %v", err)
	}
	_, _, header := rec.snapshot()
	if got, want := header.Get("Authorization"), "Bearer "+want; got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

// recorder captures the single request the provider makes, under a mutex so
// the race detector sees a proper happens-before edge between the server
// goroutine and the assertions.
type recorder struct {
	mu     sync.Mutex
	method string
	path   string
	header http.Header
}

func (r *recorder) record(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.method, r.path, r.header = req.Method, req.URL.Path, req.Header.Clone()
}

func (r *recorder) snapshot() (method, path string, header http.Header) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.method, r.path, r.header
}

// fixtureServer serves one testdata file as the usage response.
func fixtureServer(t *testing.T, fixture string) (*httptest.Server, *recorder) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return statusServer(t, http.StatusOK, body)
}

// statusServer serves a fixed status and body.
func statusServer(t *testing.T, status int, body []byte) (*httptest.Server, *recorder) {
	t.Helper()
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rec.record(req)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

// testProvider points a Provider at a test server and the valid store fixture.
func testProvider(t *testing.T, srv *httptest.Server) *Provider {
	t.Helper()
	return New(
		WithCredentialPath(filepath.Join("testdata", "auth_go.json")),
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)
}

// testContext returns a context with a deadline: httpx sets no timeout of its
// own, so every caller must supply one.
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return ts
}

func TestDefaultEndpoint_MatchesVendorURL(t *testing.T) {
	if got, want := New().usageURL(), "https://opencode.ai/zen/go/v1/usage"; got != want {
		t.Errorf("usageURL() = %q, want %q", got, want)
	}
}

func TestFetch_OKStatusUsesPercent(t *testing.T) {
	clearEnv(t)
	srv, rec := fixtureServer(t, "usage_ok.json")

	got, err := testProvider(t, srv).Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() err = %v", err)
	}

	method, path, header := rec.snapshot()
	if method != http.MethodGet {
		t.Errorf("request method = %q, want GET", method)
	}
	if want := "/zen/go/v1/usage"; path != want {
		t.Errorf("request path = %q, want %q", path, want)
	}
	if want := "Bearer sk-opencode-go-fixture-key"; header.Get("Authorization") != want {
		t.Errorf("Authorization = %q, want %q", header.Get("Authorization"), want)
	}
	if want := "application/json"; header.Get("Accept") != want {
		t.Errorf("Accept = %q, want %q", header.Get("Accept"), want)
	}

	want := []provider.Window{
		{
			Provider:    "opencode-go",
			Name:        "5h",
			Plan:        "go",
			UsedPercent: 42,
			ResetsAt:    mustTime(t, "2026-09-16T15:04:05Z"),
			Period:      5 * time.Hour,
		},
		{
			Provider:    "opencode-go",
			Name:        "weekly",
			Plan:        "go",
			UsedPercent: 18,
			ResetsAt:    mustTime(t, "2026-09-21T00:00:00Z"),
			Period:      7 * 24 * time.Hour,
		},
		{
			Provider:    "opencode-go",
			Name:        "monthly",
			Plan:        "go",
			UsedPercent: 7,
			ResetsAt:    mustTime(t, "2026-10-01T00:00:00Z"),
			Period:      30 * 24 * time.Hour,
		},
	}
	assertWindows(t, got, want)
}

// assertWindows compares windows field by field, so a time.Time carrying a
// different monotonic/location representation of the same instant still
// compares equal.
func assertWindows(t *testing.T, got, want []provider.Window) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d windows, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Provider != w.Provider || g.Name != w.Name || g.Plan != w.Plan ||
			g.UsedPercent != w.UsedPercent || g.Period != w.Period ||
			g.RateLimited != w.RateLimited || !g.ResetsAt.Equal(w.ResetsAt) {
			t.Errorf("window %d = %+v, want %+v", i, g, w)
		}
	}
}

func TestFetch_RollingAndWeeklyPeriodConstants(t *testing.T) {
	clearEnv(t)
	srv, _ := fixtureServer(t, "usage_ok.json")

	got, err := testProvider(t, srv).Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() err = %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("got %d windows, want at least 2", len(got))
	}
	if got[0].Name != "5h" || got[0].Period != 5*time.Hour {
		t.Errorf("rolling window = %q/%v, want %q/%v", got[0].Name, got[0].Period, "5h", 5*time.Hour)
	}
	if got[1].Name != "weekly" || got[1].Period != 7*24*time.Hour {
		t.Errorf("weekly window = %q/%v, want %q/%v", got[1].Name, got[1].Period, "weekly", 7*24*time.Hour)
	}
}

func TestFetch_MonthlyPeriodDerivedFromResetsAt(t *testing.T) {
	clearEnv(t)
	// resetsAt 2026-10-31T10:00Z: one month back clamps to 2026-09-30T10:00Z,
	// so the calendar month ending there is 31 days long.
	srv, _ := fixtureServer(t, "usage_month_end.json")

	got, err := testProvider(t, srv).Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() err = %v", err)
	}
	assertWindows(t, got, []provider.Window{{
		Provider:    "opencode-go",
		Name:        "monthly",
		Plan:        "go",
		UsedPercent: 61,
		ResetsAt:    mustTime(t, "2026-10-31T10:00:00Z"),
		Period:      31 * 24 * time.Hour,
	}})
}

func TestMonthlyPeriod_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		resetsAt string
		want     time.Duration
	}{
		{name: "31-day month", resetsAt: "2026-11-01T00:00:00Z", want: 31 * 24 * time.Hour},
		{name: "30-day month", resetsAt: "2026-10-01T00:00:00Z", want: 30 * 24 * time.Hour},
		{name: "february, non-leap year", resetsAt: "2026-03-01T00:00:00Z", want: 28 * 24 * time.Hour},
		{name: "february, leap year", resetsAt: "2024-03-01T00:00:00Z", want: 29 * 24 * time.Hour},
		{name: "month-end clamped to february", resetsAt: "2026-03-31T09:30:00Z", want: 31 * 24 * time.Hour},
		{name: "month-end clamped to leap february", resetsAt: "2024-03-31T09:30:00Z", want: 31 * 24 * time.Hour},
		{name: "month-end clamped to 30-day month", resetsAt: "2026-10-31T10:00:00Z", want: 31 * 24 * time.Hour},
		{name: "january wraps to december", resetsAt: "2026-01-15T12:00:00Z", want: 31 * 24 * time.Hour},
		{name: "march 30 needs no clamping", resetsAt: "2026-03-30T00:00:00Z", want: 30 * 24 * time.Hour},
		{name: "non-UTC offset", resetsAt: "2026-05-31T23:59:59+02:00", want: 31 * 24 * time.Hour},
		{name: "zero time has no period", resetsAt: "", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var at time.Time
			if tt.resetsAt != "" {
				at = mustTime(t, tt.resetsAt)
			}
			if got := monthlyPeriod(at); got != tt.want {
				t.Errorf("monthlyPeriod(%s) = %v, want %v", tt.resetsAt, got, tt.want)
			}
		})
	}
}

func TestMonthlyPeriod_IndependentOfHostTimeZone(t *testing.T) {
	// A resetsAt with a numeric offset parses into time.Local when the offset
	// happens to match the host zone, and stepping a month back inside a
	// DST-observing location silently shortens the period by an hour. The
	// same instant at the same offset must always yield the same period.
	nyc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	// 2026-04-05T12:00:00-04:00 — one month back lands on 2026-03-05, before
	// that year's US DST change, so a location-aware step changes offset.
	inNYC := time.Date(2026, 4, 5, 12, 0, 0, 0, nyc)
	fixed := inNYC.In(time.FixedZone("", -4*60*60))
	inBerlin := inNYC.In(berlin)

	const want = 31 * 24 * time.Hour
	for _, tc := range []struct {
		name string
		at   time.Time
	}{
		{name: "fixed -04:00 offset", at: fixed},
		{name: "America/New_York", at: inNYC},
		{name: "Europe/Berlin", at: inBerlin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := monthlyPeriod(tc.at); got != want {
				t.Errorf("monthlyPeriod(%s) = %v, want %v", tc.at, got, want)
			}
		})
	}
}

func TestFetch_RateLimitedStatusForces100Percent(t *testing.T) {
	clearEnv(t)
	// The rolling window reports percent 37 alongside status "rate-limited";
	// the status wins.
	srv, _ := fixtureServer(t, "usage_rate_limited.json")

	got, err := testProvider(t, srv).Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() err = %v", err)
	}
	assertWindows(t, got, []provider.Window{
		{
			Provider:    "opencode-go",
			Name:        "5h",
			Plan:        "go",
			UsedPercent: 100,
			ResetsAt:    mustTime(t, "2026-09-16T15:04:05Z"),
			Period:      5 * time.Hour,
			RateLimited: true,
		},
		{
			Provider:    "opencode-go",
			Name:        "weekly",
			Plan:        "go",
			UsedPercent: 18,
			ResetsAt:    mustTime(t, "2026-09-21T00:00:00Z"),
			Period:      7 * 24 * time.Hour,
		},
	})
}

func TestFetch_MissingWindowIsSkipped(t *testing.T) {
	clearEnv(t)
	srv, _ := fixtureServer(t, "usage_weekly_only.json")

	got, err := testProvider(t, srv).Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() err = %v", err)
	}
	assertWindows(t, got, []provider.Window{{
		Provider:    "opencode-go",
		Name:        "weekly",
		Plan:        "go",
		UsedPercent: 55,
		ResetsAt:    mustTime(t, "2026-09-21T00:00:00Z"),
		Period:      7 * 24 * time.Hour,
	}})
}

func TestFetch_IgnoresUnknownFieldsAndUnknownWindows(t *testing.T) {
	clearEnv(t)
	srv, _ := fixtureServer(t, "usage_unknown_fields.json")

	got, err := testProvider(t, srv).Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() err = %v", err)
	}
	assertWindows(t, got, []provider.Window{{
		Provider:    "opencode-go",
		Name:        "5h",
		Plan:        "go",
		UsedPercent: 42,
		ResetsAt:    mustTime(t, "2026-09-16T15:04:05Z"),
		Period:      5 * time.Hour,
	}})
}

func TestFetch_OutOfRangePercentErrors(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		want    string
	}{
		{name: "above 100", fixture: "usage_percent_too_high.json", want: "150"},
		{name: "below zero", fixture: "usage_percent_negative.json", want: "-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			srv, _ := fixtureServer(t, tt.fixture)

			got, err := testProvider(t, srv).Fetch(testContext(t))
			if err == nil {
				t.Fatalf("Fetch() err = nil, want an error; got windows %+v", got)
			}
			if got != nil {
				t.Errorf("Fetch() windows = %+v, want none alongside the error", got)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Fetch() err = %q, want it to quote the bad percent %s", err, tt.want)
			}
		})
	}
}

func TestFetch_DecimalPercentIsPreserved(t *testing.T) {
	clearEnv(t)
	srv, _ := fixtureServer(t, "usage_percent_decimal.json")

	got, err := testProvider(t, srv).Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() err = %v", err)
	}
	assertWindows(t, got, []provider.Window{{
		Provider:    "opencode-go",
		Name:        "5h",
		Plan:        "go",
		UsedPercent: 42.5,
		ResetsAt:    mustTime(t, "2026-09-16T15:04:05Z"),
		Period:      5 * time.Hour,
	}})
}

func TestFetch_MalformedBodyIsADecodeError(t *testing.T) {
	clearEnv(t)
	srv, _ := fixtureServer(t, "usage_malformed.json")

	_, err := testProvider(t, srv).Fetch(testContext(t))
	if err == nil {
		t.Fatal("Fetch() err = nil, want a decode error for a truncated body")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("Fetch() err = %q, want it to say the body could not be decoded", err)
	}
	// The renderer already prints the provider name in its own column.
	if strings.Contains(err.Error(), "opencode-go") {
		t.Errorf("Fetch() err = %q, must not carry a provider-name prefix", err)
	}
}

func TestFetch_NoKnownWindowsErrors(t *testing.T) {
	clearEnv(t)
	srv, _ := fixtureServer(t, "usage_empty.json")

	if _, err := testProvider(t, srv).Fetch(testContext(t)); err == nil {
		t.Error("Fetch() err = nil, want an error when the response carries no known window")
	}
}

func TestFetch_EnvOverrideIsSentVerbatimAndSkipsStore(t *testing.T) {
	t.Setenv("QMETER_OPENCODE_GO_KEY", "sk-from-env")
	srv, rec := fixtureServer(t, "usage_weekly_only.json")

	// The store fixture holds a different key; the override must win.
	p := New(
		WithCredentialPath(filepath.Join("testdata", "auth_go.json")),
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)
	if _, err := p.Fetch(testContext(t)); err != nil {
		t.Fatalf("Fetch() err = %v", err)
	}
	_, _, header := rec.snapshot()
	if want := "Bearer sk-from-env"; header.Get("Authorization") != want {
		t.Errorf("Authorization = %q, want %q", header.Get("Authorization"), want)
	}
}

func TestFetch_UnauthorizedMapsToTokenExpiredWithOpenCodeHint(t *testing.T) {
	clearEnv(t)
	srv, _ := statusServer(t, http.StatusUnauthorized, []byte(`{"error":"unauthorized"}`))

	_, err := testProvider(t, srv).Fetch(testContext(t))
	if !errors.Is(err, provider.ErrTokenExpired{}) {
		t.Fatalf("Fetch() err = %v, want provider.ErrTokenExpired", err)
	}
	var expired provider.ErrTokenExpired
	if !errors.As(err, &expired) {
		t.Fatalf("Fetch() err = %v, want errors.As to a provider.ErrTokenExpired", err)
	}
	if expired.Tool != "opencode" {
		t.Errorf("ErrTokenExpired.Tool = %q, want %q", expired.Tool, "opencode")
	}
}

func TestFetch_TooManyRequestsMapsToRateLimited(t *testing.T) {
	clearEnv(t)
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rec.record(req)
		w.Header().Set("Retry-After", "42")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	_, err := testProvider(t, srv).Fetch(testContext(t))
	if !errors.Is(err, provider.ErrRateLimited{}) {
		t.Fatalf("Fetch() err = %v, want provider.ErrRateLimited", err)
	}
	var limited provider.ErrRateLimited
	if !errors.As(err, &limited) {
		t.Fatalf("Fetch() err = %v, want errors.As to a provider.ErrRateLimited", err)
	}
	if limited.RetryAfter != 42*time.Second {
		t.Errorf("ErrRateLimited.RetryAfter = %v, want %v", limited.RetryAfter, 42*time.Second)
	}
}
