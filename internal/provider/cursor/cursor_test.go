package cursor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
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

// makeJWT builds an unsigned-but-well-formed JWT with the given claims. The
// signature is never verified by this package, so a constant stands in for it.
// Fixtures are built here rather than checked in so no real token, expired or
// not, ever lands in the repository.
func makeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	return strings.Join([]string{
		header,
		base64.RawURLEncoding.EncodeToString(payload),
		base64.RawURLEncoding.EncodeToString([]byte("not-a-real-signature")),
	}, ".")
}

// makePaddedJWT builds a JWT whose payload segment is standard, PADDED
// base64url rather than the unpadded form the JWT spec calls for. Real
// encoders differ, so the decoder accepts both; wantPad asserts how many "="
// characters the segment actually ends with, so a subject whose length stops
// producing padding fails the test instead of silently exercising the
// unpadded path again.
func makePaddedJWT(t *testing.T, sub string, wantPad int) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"sub": sub})
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	seg := base64.URLEncoding.EncodeToString(payload)
	if got := len(seg) - len(strings.TrimRight(seg, "=")); got != wantPad {
		t.Fatalf("payload segment %q carries %d padding characters, want %d: pick a subject of a different length", seg, got, wantPad)
	}
	return strings.Join([]string{
		base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)),
		seg,
		base64.RawURLEncoding.EncodeToString([]byte("not-a-real-signature")),
	}, ".")
}

func TestJWT_ParsesGoogleOAuth2AndAuth0Subjects(t *testing.T) {
	tests := []struct {
		name string
		sub  string
		// pad is how many "=" characters the payload segment must carry; 0
		// means the unpadded (spec) encoding.
		pad  int
		want string
	}{
		{name: "google-oauth2 subject", sub: "google-oauth2|103512345678901234567", want: "103512345678901234567"},
		{name: "auth0 subject", sub: "auth0|user_01ABCDEFGHIJKLMNOPQRSTUV", want: "user_01ABCDEFGHIJKLMNOPQRSTUV"},
		{name: "last pipe wins", sub: "auth0|google-oauth2|103512345678901234567", want: "103512345678901234567"},
		{name: "no pipe at all", sub: "1234567890", want: "1234567890"},
		// {"sub":"<sub>"} is 10 bytes plus the subject, so a subject length
		// of 0 mod 3 leaves two padding characters and 1 mod 3 leaves one.
		{name: "padded payload, two pad characters", sub: "auth0|abc", pad: 2, want: "abc"},
		{name: "padded payload, one pad character", sub: "google-oauth2|abcde", pad: 1, want: "abcde"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := makeJWT(t, map[string]any{"sub": tt.sub, "exp": 1789000000})
			if tt.pad > 0 {
				token = makePaddedJWT(t, tt.sub, tt.pad)
			}
			got, err := userIDFromJWT(token)
			if err != nil {
				t.Fatalf("userIDFromJWT() error = %v, want nil", err)
			}
			if got != tt.want {
				t.Errorf("userIDFromJWT() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJWT_RejectsMalformedTokens(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{name: "empty", token: ""},
		{name: "opaque string", token: "not-a-jwt"},
		{name: "two segments", token: "aGVhZGVy.cGF5bG9hZA"},
		{name: "four segments", token: "a.b.c.d"},
		{name: "payload is not base64url", token: "aGVhZGVy.!!!not-base64!!!.c2ln"},
		{name: "payload is not JSON", token: "aGVhZGVy." + base64.RawURLEncoding.EncodeToString([]byte("nope")) + ".c2ln"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := userIDFromJWT(tt.token)
			if err == nil {
				t.Fatalf("userIDFromJWT(%q) = %q, want an error", tt.token, got)
			}
			if !strings.Contains(err.Error(), "JWT") {
				t.Errorf("userIDFromJWT() error = %q, want it to say the value is not a JWT", err.Error())
			}
		})
	}
}

func TestJWT_RejectsMissingOrEmptySubject(t *testing.T) {
	tests := []struct {
		name   string
		claims map[string]any
	}{
		{name: "no sub claim", claims: map[string]any{"exp": 1789000000}},
		{name: "empty sub claim", claims: map[string]any{"sub": ""}},
		{name: "sub ends with a pipe", claims: map[string]any{"sub": "auth0|"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := userIDFromJWT(makeJWT(t, tt.claims)); err == nil {
				t.Fatalf("userIDFromJWT() = %q, want an error", got)
			}
		})
	}
}

// writeStore writes a CLI-store file into a temp dir and returns its path.
func writeStore(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write store: %v", err)
	}
	return path
}

// missingStore returns a path inside a temp dir where no file exists.
func missingStore(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "auth.json")
}

func TestCredentials_EnvOverrideNonJWTErrors(t *testing.T) {
	t.Setenv(envToken, "sk-an-opaque-token")

	// The store is perfectly good; the override must still win and fail loudly
	// rather than silently falling back to it.
	store := writeStore(t, `{"accessToken":"`+makeJWT(t, map[string]any{"sub": "auth0|store-user"})+`"}`)

	_, src, err := New(WithCredentialPath(store)).credential(t.Context())
	if err == nil {
		t.Fatal("credential() error = nil, want an error for a non-JWT override")
	}
	if !strings.HasPrefix(err.Error(), envToken+": ") {
		t.Errorf("credential() error = %q, want it prefixed %q", err.Error(), envToken+": ")
	}
	if !strings.Contains(err.Error(), "JWT") {
		t.Errorf("credential() error = %q, want it to explain the override must be a JWT", err.Error())
	}
	// The user did supply a credential, so "run cursor-agent to log in" would
	// be the wrong hint.
	if errors.Is(err, provider.ErrNotLoggedIn{}) {
		t.Errorf("credential() error = %v, want it NOT typed as provider.ErrNotLoggedIn", err)
	}
	if src != credstore.SourceNone {
		t.Errorf("credential() source = %q, want %q", src, credstore.SourceNone)
	}
}

func TestCredentials_EnvOverrideSuppliesTokenAndUserID(t *testing.T) {
	token := makeJWT(t, map[string]any{"sub": "google-oauth2|env-user-42"})
	t.Setenv(envToken, token)

	cred, src, err := New(WithCredentialPath(missingStore(t))).credential(t.Context())
	if err != nil {
		t.Fatalf("credential() error = %v, want nil", err)
	}
	if cred.AccessToken != token {
		t.Errorf("credential().AccessToken = %q, want the override verbatim", cred.AccessToken)
	}
	if want := "env-user-42"; cred.UserID != want {
		t.Errorf("credential().UserID = %q, want %q", cred.UserID, want)
	}
	if src != credstore.SourceEnv {
		t.Errorf("credential() source = %q, want %q", src, credstore.SourceEnv)
	}
}

func TestCredentials_StoreSuppliesTokenAndUserID(t *testing.T) {
	t.Setenv(envToken, "")
	token := makeJWT(t, map[string]any{"sub": "auth0|store-user-7"})
	store := writeStore(t, `{"accessToken":"`+token+`","refreshToken":"`+makeJWT(t, map[string]any{"sub": "auth0|store-user-7"})+`"}`)

	cred, src, err := New(WithCredentialPath(store)).credential(t.Context())
	if err != nil {
		t.Fatalf("credential() error = %v, want nil", err)
	}
	if cred.AccessToken != token {
		t.Errorf("credential().AccessToken = %q, want the store's accessToken", cred.AccessToken)
	}
	if want := "store-user-7"; cred.UserID != want {
		t.Errorf("credential().UserID = %q, want %q", cred.UserID, want)
	}
	if src != credstore.SourceStore {
		t.Errorf("credential() source = %q, want %q", src, credstore.SourceStore)
	}
}

func TestCredentials_MissingStoreReturnsNotLoggedIn(t *testing.T) {
	t.Setenv(envToken, "")

	_, _, err := New(WithCredentialPath(missingStore(t))).credential(t.Context())
	if !errors.Is(err, provider.ErrNotLoggedIn{}) {
		t.Fatalf("credential() error = %v, want provider.ErrNotLoggedIn", err)
	}
	var notLoggedIn provider.ErrNotLoggedIn
	if !errors.As(err, &notLoggedIn) {
		t.Fatalf("errors.As(%v, *provider.ErrNotLoggedIn) = false, want true", err)
	}
	if notLoggedIn.Tool != toolName {
		t.Errorf("ErrNotLoggedIn.Tool = %q, want %q", notLoggedIn.Tool, toolName)
	}
	if want := "not logged in, run cursor-agent to log in"; err.Error() != want {
		t.Errorf("credential() error text = %q, want %q", err.Error(), want)
	}
}

func TestCredentials_StoreWithoutAccessTokenReturnsNotLoggedIn(t *testing.T) {
	t.Setenv(envToken, "")

	tests := []struct {
		name     string
		contents string
	}{
		{name: "empty object", contents: `{}`},
		{name: "empty accessToken", contents: `{"accessToken":"","refreshToken":"x"}`},
		{name: "only a refresh token", contents: `{"refreshToken":"x"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := New(WithCredentialPath(writeStore(t, tt.contents))).credential(t.Context())
			if !errors.Is(err, provider.ErrNotLoggedIn{}) {
				t.Fatalf("credential() error = %v, want provider.ErrNotLoggedIn", err)
			}
		})
	}
}

func TestCredentials_StoreAccessTokenNotAJWTErrors(t *testing.T) {
	t.Setenv(envToken, "")

	tests := []struct {
		name     string
		contents string
	}{
		{name: "opaque access token", contents: `{"accessToken":"sk-opaque","refreshToken":"y"}`},
		{name: "malformed JSON", contents: `{"accessToken":`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := New(WithCredentialPath(writeStore(t, tt.contents))).credential(t.Context())
			if err == nil {
				t.Fatal("credential() error = nil, want an error")
			}
			// A store that exists but cannot be used is not "not logged in".
			if errors.Is(err, provider.ErrNotLoggedIn{}) {
				t.Errorf("credential() error = %v, want it NOT typed as provider.ErrNotLoggedIn", err)
			}
			if !strings.HasPrefix(err.Error(), "credential store: ") {
				t.Errorf("credential() error = %q, want it prefixed %q", err.Error(), "credential store: ")
			}
		})
	}
}

func TestDefaultCredentialPath_IsHomeRelative(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("os.UserHomeDir() error = %v", err)
	}
	got, err := defaultCredentialPath()
	if err != nil {
		t.Fatalf("defaultCredentialPath() error = %v, want nil", err)
	}
	if want := filepath.Join(home, ".config", "cursor", "auth.json"); got != want {
		t.Errorf("defaultCredentialPath() = %q, want %q", got, want)
	}
}

func TestID_MatchesCLIProviderName(t *testing.T) {
	if got, want := New().ID(), "cursor"; got != want {
		t.Errorf("ID() = %q, want %q", got, want)
	}
	// The provider id and the CLI binary named in hints are deliberately
	// different strings; a swap between them must not compile away unnoticed.
	if New().ID() == toolName {
		t.Errorf("ID() = %q, want it to differ from the tool name %q", New().ID(), toolName)
	}
}

func TestDetect_EnvOverride(t *testing.T) {
	t.Setenv(envToken, makeJWT(t, map[string]any{"sub": "auth0|env-user"}))

	// No store, and an endpoint that fails the test if Detect touches it:
	// Detect must do no network I/O and must not need the store at all.
	p := New(
		WithCredentialPath(missingStore(t)),
		WithBaseURL(noRequestServer(t).URL),
	)
	ok, reason := p.Detect(t.Context())
	if !ok {
		t.Fatalf("Detect() = false, %q, want true", reason)
	}
	if reason != "" {
		t.Errorf("Detect() reason = %q, want empty when detected", reason)
	}
}

func TestDetect_StorePresent(t *testing.T) {
	t.Setenv(envToken, "")
	store := writeStore(t, `{"accessToken":"`+makeJWT(t, map[string]any{"sub": "google-oauth2|store-user"})+`"}`)

	ok, reason := New(WithCredentialPath(store), WithBaseURL(noRequestServer(t).URL)).Detect(t.Context())
	if !ok {
		t.Fatalf("Detect() = false, %q, want true", reason)
	}
	if reason != "" {
		t.Errorf("Detect() reason = %q, want empty when detected", reason)
	}
}

func TestDetect_StoreMissingReturnsReason(t *testing.T) {
	t.Setenv(envToken, "")

	ok, reason := New(WithCredentialPath(missingStore(t))).Detect(t.Context())
	if ok {
		t.Fatal("Detect() = true, want false when the store is missing")
	}
	// Exact text: it is rendered verbatim after "not detected: ", and the
	// hint must name the CLI (cursor-agent), not the provider id (cursor).
	if want := "not logged in, run cursor-agent to log in"; reason != want {
		t.Errorf("Detect() reason = %q, want %q", reason, want)
	}
}

func TestDetect_ReasonHasNoProviderPrefix(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		contents string // empty means "write no store file"
		wantText string
	}{
		{name: "missing store", env: "", wantText: "not logged in"},
		{name: "store access token is not a JWT", env: "", contents: `{"accessToken":"sk-opaque"}`, wantText: "JWT"},
		{name: "malformed store", env: "", contents: `{"accessToken":`, wantText: "parse"},
		{name: "non-JWT env override", env: "sk-opaque", contents: `{"accessToken":"sk-opaque"}`, wantText: envToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envToken, tt.env)
			path := missingStore(t)
			if tt.contents != "" {
				path = writeStore(t, tt.contents)
			}

			ok, reason := New(WithCredentialPath(path)).Detect(t.Context())
			if ok {
				t.Fatal("Detect() = true, want false")
			}
			if !strings.Contains(reason, tt.wantText) {
				t.Errorf("Detect() reason = %q, want it to contain %q", reason, tt.wantText)
			}
			if strings.HasPrefix(reason, "cursor:") || strings.HasPrefix(reason, "cursor ") {
				t.Errorf("Detect() reason = %q, want no provider-name prefix", reason)
			}
		})
	}
}

// noRequestServer is an endpoint that fails the test if it is ever called.
func noRequestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// request is what the fake route A endpoint saw.
type request struct {
	calls  int
	method string
	path   string
	query  string
	cookie string
	auth   string
}

// capture records requests from the server goroutine for the test goroutine
// to read back.
type capture struct {
	mu   sync.Mutex
	last request
}

func (c *capture) record(r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.last = request{
		calls:  c.last.calls + 1,
		method: r.Method,
		path:   r.URL.Path,
		query:  r.URL.RawQuery,
		cookie: r.Header.Get("Cookie"),
		auth:   r.Header.Get("Authorization"),
	}
}

func (c *capture) snapshot() request {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last
}

// fixture reads a canned route A response body.
func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(b)
}

// serve starts a fake route A endpoint returning status and body.
func serve(t *testing.T, status int, body string) (*httptest.Server, *capture) {
	t.Helper()
	c := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.record(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, c
}

// testCtx returns a context with a deadline: httpx sets no timeout of its
// own, so every caller supplies one.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// storeFor writes a CLI store holding a JWT with the given subject and
// returns the store path and the token it holds.
func storeFor(t *testing.T, sub string) (path, token string) {
	t.Helper()
	token = makeJWT(t, map[string]any{"sub": sub, "exp": 1789000000})
	return writeStore(t, `{"accessToken":"`+token+`","refreshToken":"`+token+`"}`), token
}

func TestFetch_ParsesRouteAResponse(t *testing.T) {
	t.Setenv(envToken, "")
	srv, got := serve(t, http.StatusOK, fixture(t, "usage_summary_free.json"))
	store, _ := storeFor(t, "google-oauth2|103512345678901234567")

	fetched, err := New(
		WithCredentialPath(store),
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	).Fetch(testCtx(t))
	windows := fetched.Windows
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}

	req := got.snapshot()
	if req.calls != 1 {
		t.Errorf("endpoint calls = %d, want 1", req.calls)
	}
	if req.method != http.MethodGet {
		t.Errorf("request method = %q, want %q", req.method, http.MethodGet)
	}
	if want := "/api/usage-summary"; req.path != want {
		t.Errorf("request path = %q, want %q", req.path, want)
	}
	if req.query != "" {
		t.Errorf("request query = %q, want none", req.query)
	}
	// Route A authenticates with the session cookie only; the Connect-RPC
	// bearer route is not implemented in this unit.
	if req.auth != "" {
		t.Errorf("Authorization header = %q, want none", req.auth)
	}

	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	want := []provider.Window{
		{Provider: "cursor", Name: "total", Plan: "free", RemainingPercent: 95.5, ResetsAt: end, Period: end.Sub(start)},
		{Provider: "cursor", Name: "auto", Plan: "free", RemainingPercent: 97.9, ResetsAt: end, Period: end.Sub(start)},
	}
	assertWindows(t, windows, want)
}

func TestFetch_SendsWorkosSessionCookie(t *testing.T) {
	t.Setenv(envToken, "")
	srv, got := serve(t, http.StatusOK, fixture(t, "usage_summary_free.json"))
	store, token := storeFor(t, "auth0|user_01ABCDEF")

	if _, err := New(
		WithCredentialPath(store),
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	).Fetch(testCtx(t)); err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}

	// The separator is the literal, already-percent-encoded "%3A%3A" — the
	// vendor expects those six characters on the wire, not a raw "::" and not
	// a double-encoded "%253A%253A".
	want := "WorkosCursorSessionToken=user_01ABCDEF%3A%3A" + token
	if req := got.snapshot(); req.cookie != want {
		t.Errorf("Cookie header =\n\t%q\nwant\n\t%q", req.cookie, want)
	}
}

func TestFetch_EmitsTotalAndAutoWindows(t *testing.T) {
	t.Setenv(envToken, "")
	srv, _ := serve(t, http.StatusOK, fixture(t, "usage_summary_pro_team.json"))
	store, _ := storeFor(t, "auth0|team-user")

	fetched, err := New(WithCredentialPath(store), WithBaseURL(srv.URL), WithHTTPClient(srv.Client())).Fetch(testCtx(t))
	windows := fetched.Windows
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}

	start := time.Date(2026, 8, 15, 12, 30, 0, 0, time.UTC)
	end := time.Date(2026, 9, 15, 12, 30, 0, 0, time.UTC)
	want := []provider.Window{
		{Provider: "cursor", Name: "total", Plan: "pro", RemainingPercent: 28.75, ResetsAt: end, Period: end.Sub(start)},
		{Provider: "cursor", Name: "auto", Plan: "pro", RemainingPercent: 55, ResetsAt: end, Period: end.Sub(start)},
	}
	assertWindows(t, windows, want)

	// Both windows describe the same billing cycle, so they must agree.
	if !windows[0].ResetsAt.Equal(windows[1].ResetsAt) || windows[0].Period != windows[1].Period {
		t.Errorf("windows disagree on the billing cycle: %+v vs %+v", windows[0], windows[1])
	}
}

func TestFetch_ToleratesEmptyTeamUsageObject(t *testing.T) {
	t.Setenv(envToken, "")
	store, _ := storeFor(t, "auth0|tolerant")

	tests := []struct {
		name    string
		fixture string
		want    []provider.Window
	}{
		{
			name:    "teamUsage is an empty object on a personal account",
			fixture: "usage_summary_free.json",
			want: []provider.Window{
				{Provider: "cursor", Name: "total", Plan: "free", RemainingPercent: 95.5},
				{Provider: "cursor", Name: "auto", Plan: "free", RemainingPercent: 97.9},
			},
		},
		{
			name:    "teamUsage is populated on a team account",
			fixture: "usage_summary_pro_team.json",
			want: []provider.Window{
				{Provider: "cursor", Name: "total", Plan: "pro", RemainingPercent: 28.75},
				{Provider: "cursor", Name: "auto", Plan: "pro", RemainingPercent: 55},
			},
		},
		{
			name:    "nulls and unknown fields throughout",
			fixture: "usage_summary_nulls.json",
			want: []provider.Window{
				{Provider: "cursor", Name: "total", Plan: "free", RemainingPercent: 87.5},
				{Provider: "cursor", Name: "auto", Plan: "free", RemainingPercent: 90},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := serve(t, http.StatusOK, fixture(t, tt.fixture))
			fetched, err := New(WithCredentialPath(store), WithBaseURL(srv.URL), WithHTTPClient(srv.Client())).Fetch(testCtx(t))
			windows := fetched.Windows
			if err != nil {
				t.Fatalf("Fetch() error = %v, want nil", err)
			}
			if len(windows) != len(tt.want) {
				t.Fatalf("Fetch() returned %d windows, want %d: %+v", len(windows), len(tt.want), windows)
			}
			for i, w := range tt.want {
				if windows[i].Name != w.Name || windows[i].Plan != w.Plan || windows[i].RemainingPercent != w.RemainingPercent {
					t.Errorf("window %d = {Name:%q Plan:%q RemainingPercent:%v}, want {Name:%q Plan:%q RemainingPercent:%v}",
						i, windows[i].Name, windows[i].Plan, windows[i].RemainingPercent, w.Name, w.Plan, w.RemainingPercent)
				}
			}
		})
	}
}

func TestFetch_NoUsableIndividualPlanErrors(t *testing.T) {
	t.Setenv(envToken, "")
	store, _ := storeFor(t, "auth0|no-plan")

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "plan.enabled is explicitly false",
			body: fixture(t, "usage_summary_plan_disabled.json"),
			want: `no individual plan usage for this account (membershipType "enterprise"); team and enterprise pooled usage is not read yet`,
		},
		{
			name: "plan.enabled key is absent",
			body: fixture(t, "usage_summary_plan_enabled_absent.json"),
			want: "usage summary has no individualUsage.plan.enabled field; the response shape may have changed",
		},
		{
			name: "body is an empty object",
			body: fixture(t, "usage_summary_empty_object.json"),
			want: "usage summary has no individualUsage.plan.enabled field; the response shape may have changed",
		},
		{
			name: "body is JSON null",
			body: `null`,
			want: "usage summary has no individualUsage.plan.enabled field; the response shape may have changed",
		},
		{
			name: "plan.enabled is null",
			body: `{"membershipType":"free","individualUsage":{"plan":{"enabled":null,"totalPercentUsed":4.5,"autoPercentUsed":2.1}}}`,
			want: "usage summary has no individualUsage.plan.enabled field; the response shape may have changed",
		},
		{
			name: "individualUsage is null",
			body: `{"membershipType":"free","individualUsage":null}`,
			want: "usage summary has no individualUsage.plan.enabled field; the response shape may have changed",
		},
		{
			name: "individualUsage was renamed",
			body: `{"membershipType":"free","individualUsageV2":{"plan":{"enabled":true,"totalPercentUsed":4.5,"autoPercentUsed":2.1}}}`,
			want: "usage summary has no individualUsage.plan.enabled field; the response shape may have changed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := serve(t, http.StatusOK, tt.body)

			fetched, err := New(WithCredentialPath(store), WithBaseURL(srv.URL), WithHTTPClient(srv.Client())).Fetch(testCtx(t))
			windows := fetched.Windows
			// Silently reporting "no usage" would read as a healthy account
			// with nothing used; Fetch must never return (nil, nil).
			if err == nil {
				t.Fatalf("Fetch() = %+v, nil; want an error", windows)
			}
			if windows != nil {
				t.Errorf("Fetch() windows = %+v, want nil alongside the error", windows)
			}
			if err.Error() != tt.want {
				t.Errorf("Fetch() error = %q, want %q", err.Error(), tt.want)
			}
			assertNoSelfPrefix(t, err)
		})
	}
}

func TestFetch_MissingPercentagesError(t *testing.T) {
	t.Setenv(envToken, "")
	store, _ := storeFor(t, "auth0|no-percentages")

	tests := []struct {
		name string
		body string
	}{
		{name: "both keys absent", body: fixture(t, "usage_summary_missing_percentages.json")},
		{name: "totalPercentUsed absent", body: `{"membershipType":"free","individualUsage":{"plan":{"enabled":true,"autoPercentUsed":2.1}}}`},
		{name: "autoPercentUsed absent", body: `{"membershipType":"free","individualUsage":{"plan":{"enabled":true,"totalPercentUsed":4.5}}}`},
		{name: "both null", body: `{"membershipType":"free","individualUsage":{"plan":{"enabled":true,"totalPercentUsed":null,"autoPercentUsed":null}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := serve(t, http.StatusOK, tt.body)

			fetched, err := New(WithCredentialPath(store), WithBaseURL(srv.URL), WithHTTPClient(srv.Client())).Fetch(testCtx(t))
			windows := fetched.Windows
			// Rendering an absent percentage as 0.0% would read as "nothing
			// used", the opposite of "we do not know".
			if err == nil {
				t.Fatalf("Fetch() = %+v, nil; want an error", windows)
			}
			if windows != nil {
				t.Errorf("Fetch() windows = %+v, want nil alongside the error", windows)
			}
			if want := "usage summary is missing totalPercentUsed/autoPercentUsed"; err.Error() != want {
				t.Errorf("Fetch() error = %q, want %q", err.Error(), want)
			}
			assertNoSelfPrefix(t, err)
		})
	}
}

// A zero used percentage that IS reported must come through as a full 100%
// remaining, rather than being dropped as if it were absent.
func TestFetch_ZeroPercentagesAreReported(t *testing.T) {
	t.Setenv(envToken, "")
	store, _ := storeFor(t, "auth0|fresh-cycle")
	srv, _ := serve(t, http.StatusOK, `{"membershipType":"pro","individualUsage":{"plan":{"enabled":true,"totalPercentUsed":0,"autoPercentUsed":0}}}`)

	fetched, err := New(WithCredentialPath(store), WithBaseURL(srv.URL), WithHTTPClient(srv.Client())).Fetch(testCtx(t))
	windows := fetched.Windows
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}
	if len(windows) != 2 {
		t.Fatalf("Fetch() returned %d windows, want 2", len(windows))
	}
	for _, w := range windows {
		if w.RemainingPercent != 100 {
			t.Errorf("window %q RemainingPercent = %v, want 100", w.Name, w.RemainingPercent)
		}
	}
}

func TestFetch_UnparsableBillingCycleKeepsThePercentages(t *testing.T) {
	t.Setenv(envToken, "")
	store, _ := storeFor(t, "auth0|no-cycle")
	end := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		body string
		// wantResetsAt is the zero time when the END bound is unusable; a
		// usable end is still worth reporting even when the start is not.
		wantResetsAt time.Time
	}{
		{
			name: "both bounds unusable",
			body: fixture(t, "usage_summary_missing_cycle.json"),
		},
		{
			name:         "start unparsable, end good",
			body:         fixture(t, "usage_summary_bad_start.json"),
			wantResetsAt: end,
		},
		{
			name:         "start not before end",
			body:         `{"membershipType":"free","billingCycleStart":"2026-10-01T00:00:00Z","billingCycleEnd":"2026-10-01T00:00:00Z","individualUsage":{"plan":{"enabled":true,"totalPercentUsed":4,"autoPercentUsed":1.5}}}`,
			wantResetsAt: end,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := serve(t, http.StatusOK, tt.body)

			fetched, err := New(WithCredentialPath(store), WithBaseURL(srv.URL), WithHTTPClient(srv.Client())).Fetch(testCtx(t))
			windows := fetched.Windows
			if err != nil {
				t.Fatalf("Fetch() error = %v, want nil: unusable cycle bounds must not lose the percentages", err)
			}
			if len(windows) != 2 {
				t.Fatalf("Fetch() returned %d windows, want 2", len(windows))
			}
			for _, w := range windows {
				if !w.ResetsAt.Equal(tt.wantResetsAt) {
					t.Errorf("window %q ResetsAt = %v, want %v", w.Name, w.ResetsAt, tt.wantResetsAt)
				}
				// A period can only be computed from two usable bounds.
				if w.Period != 0 {
					t.Errorf("window %q Period = %v, want 0", w.Name, w.Period)
				}
			}
			if windows[0].RemainingPercent != 96 || windows[1].RemainingPercent != 98.5 {
				t.Errorf("percentages = %v, %v, want 96 and 98.5", windows[0].RemainingPercent, windows[1].RemainingPercent)
			}
		})
	}
}

func TestFetch_401Or403ReturnsTokenExpired(t *testing.T) {
	t.Setenv(envToken, "")
	store, _ := storeFor(t, "auth0|expired-user")

	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv, _ := serve(t, status, `{"error":"unauthorized"}`)

			_, err := New(WithCredentialPath(store), WithBaseURL(srv.URL), WithHTTPClient(srv.Client())).Fetch(testCtx(t))
			if !errors.Is(err, provider.ErrTokenExpired{}) {
				t.Fatalf("Fetch() error = %v, want provider.ErrTokenExpired", err)
			}
			var expired provider.ErrTokenExpired
			if !errors.As(err, &expired) {
				t.Fatalf("errors.As(%v, *provider.ErrTokenExpired) = false, want true", err)
			}
			if expired.Tool != toolName {
				t.Errorf("ErrTokenExpired.Tool = %q, want %q", expired.Tool, toolName)
			}
			if want := "token expired, open cursor-agent to refresh"; expired.Error() != want {
				t.Errorf("extracted error text = %q, want %q", expired.Error(), want)
			}
		})
	}
}

func TestFetch_MalformedBodyErrors(t *testing.T) {
	t.Setenv(envToken, "")
	srv, _ := serve(t, http.StatusOK, fixture(t, "usage_summary_malformed.json"))
	store, _ := storeFor(t, "auth0|malformed")

	fetched, err := New(WithCredentialPath(store), WithBaseURL(srv.URL), WithHTTPClient(srv.Client())).Fetch(testCtx(t))
	windows := fetched.Windows
	if err == nil {
		t.Fatalf("Fetch() = %+v, want an error for a malformed body", windows)
	}
	if windows != nil {
		t.Errorf("Fetch() windows = %+v, want nil alongside the error", windows)
	}
	assertNoSelfPrefix(t, err)
}

// assertNoSelfPrefix pins the rule that this package never prefixes its own
// errors with the provider name: internal/usage renders "<provider>  error:
// <message>", so a self-prefix would print the name twice.
func assertNoSelfPrefix(t *testing.T, err error) {
	t.Helper()
	if strings.HasPrefix(err.Error(), "cursor") {
		t.Errorf("Fetch() error = %q, want no provider-name prefix", err.Error())
	}
}

func TestFetch_NoCredentialReturnsNotLoggedInWithoutCallingTheEndpoint(t *testing.T) {
	t.Setenv(envToken, "")
	srv := noRequestServer(t)

	_, err := New(
		WithCredentialPath(missingStore(t)),
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	).Fetch(testCtx(t))
	if !errors.Is(err, provider.ErrNotLoggedIn{}) {
		t.Fatalf("Fetch() error = %v, want provider.ErrNotLoggedIn", err)
	}
	var notLoggedIn provider.ErrNotLoggedIn
	if !errors.As(err, &notLoggedIn) {
		t.Fatalf("errors.As(%v, *provider.ErrNotLoggedIn) = false, want true", err)
	}
	if notLoggedIn.Tool != toolName {
		t.Errorf("ErrNotLoggedIn.Tool = %q, want %q", notLoggedIn.Tool, toolName)
	}
}

func TestFetch_UsesEnvOverrideUserID(t *testing.T) {
	token := makeJWT(t, map[string]any{"sub": "google-oauth2|env-only-user"})
	t.Setenv(envToken, token)
	srv, got := serve(t, http.StatusOK, fixture(t, "usage_summary_free.json"))

	fetched, err := New(
		WithCredentialPath(missingStore(t)),
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	).Fetch(testCtx(t))
	windows := fetched.Windows
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}
	if want := "WorkosCursorSessionToken=env-only-user%3A%3A" + token; got.snapshot().cookie != want {
		t.Errorf("Cookie header = %q, want %q", got.snapshot().cookie, want)
	}
	// The plan name is part of the response, not the credential, so the
	// override does not cost it.
	if want := "free"; windows[0].Plan != want {
		t.Errorf("window Plan = %q, want %q", windows[0].Plan, want)
	}
}

// assertWindows compares whole windows field by field, so a renamed or
// dropped field fails loudly.
func assertWindows(t *testing.T, got, want []provider.Window) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d windows, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Provider != w.Provider || g.Name != w.Name || g.Plan != w.Plan ||
			g.RemainingPercent != w.RemainingPercent || !g.ResetsAt.Equal(w.ResetsAt) ||
			g.Period != w.Period || g.RateLimited != w.RateLimited {
			t.Errorf("window %d =\n\t%+v\nwant\n\t%+v", i, g, w)
		}
	}
}
