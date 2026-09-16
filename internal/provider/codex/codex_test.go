package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// clearEnv unsets both Codex environment overrides so a store-path test is
// never influenced by the developer's real environment. credstore treats an
// empty value as unset.
func clearEnv(t *testing.T) {
	t.Helper()
	t.Setenv(tokenEnvVar, "")
	t.Setenv(accountIDEnvVar, "")
}

// fixture returns the path of a checked-in testdata file.
func fixture(name string) string {
	return filepath.Join("testdata", name)
}

func TestID_MatchesCLIProviderName(t *testing.T) {
	if got := New().ID(); got != "codex" {
		t.Fatalf("ID() = %q, want %q", got, "codex")
	}
}

func TestDetect_StoreMissingReturnsReason(t *testing.T) {
	clearEnv(t)
	p := New(WithCredentialPath(filepath.Join(t.TempDir(), "auth.json")))

	ok, reason := p.Detect(context.Background())
	if ok {
		t.Fatalf("Detect() ok = true, want false")
	}
	if want := "not logged in, run codex to log in"; reason != want {
		t.Fatalf("Detect() reason = %q, want %q", reason, want)
	}
}

func TestDetect_StorePresent(t *testing.T) {
	clearEnv(t)
	p := New(WithCredentialPath(fixture("auth_chatgpt.json")))

	ok, reason := p.Detect(context.Background())
	if !ok {
		t.Fatalf("Detect() ok = false (reason %q), want true", reason)
	}
	if reason != "" {
		t.Fatalf("Detect() reason = %q, want empty", reason)
	}
}

func TestDetect_EnvOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv(tokenEnvVar, "env-access-token")
	// The store is deliberately absent: the override must skip it entirely.
	p := New(WithCredentialPath(filepath.Join(t.TempDir(), "auth.json")))

	ok, reason := p.Detect(context.Background())
	if !ok {
		t.Fatalf("Detect() ok = false (reason %q), want true", reason)
	}
	if reason != "" {
		t.Fatalf("Detect() reason = %q, want empty", reason)
	}
}

func TestCredentials_StoreSuppliesTokenAndAccountID(t *testing.T) {
	clearEnv(t)
	p := New(WithCredentialPath(fixture("auth_chatgpt.json")))

	cred, err := p.resolveCredential(context.Background())
	if err != nil {
		t.Fatalf("resolveCredential() error = %v, want nil", err)
	}
	if cred.AccessToken != "store-access-token" {
		t.Errorf("AccessToken = %q, want %q", cred.AccessToken, "store-access-token")
	}
	if cred.AccountID != "store-account-id" {
		t.Errorf("AccountID = %q, want %q", cred.AccountID, "store-account-id")
	}
}

func TestCredentials_MissingTokenReturnsNotLoggedIn(t *testing.T) {
	clearEnv(t)
	p := New(WithCredentialPath(fixture("auth_no_token.json")))

	_, err := p.resolveCredential(context.Background())
	if !errors.Is(err, provider.ErrNotLoggedIn{}) {
		t.Fatalf("resolveCredential() error = %v, want ErrNotLoggedIn", err)
	}
	var notLoggedIn provider.ErrNotLoggedIn
	if !errors.As(err, &notLoggedIn) {
		t.Fatalf("errors.As(%v) failed", err)
	}
	if notLoggedIn.Tool != "codex" {
		t.Errorf("Tool = %q, want %q", notLoggedIn.Tool, "codex")
	}
}

func TestCredentials_UnreadableStoreExplainsKeyringCase(t *testing.T) {
	clearEnv(t)

	unreadable := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(unreadable, []byte(`{"auth_mode":"chatgpt"}`), 0o200); err != nil {
		t.Fatalf("write unreadable fixture: %v", err)
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "unreadable file", path: unreadable},
		{name: "unparsable file", path: fixture("auth_malformed.json")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.path == unreadable && os.Geteuid() == 0 {
				t.Skip("running as root: an unreadable file is still readable")
			}
			p := New(WithCredentialPath(tc.path))

			_, err := p.resolveCredential(context.Background())
			if err == nil {
				t.Fatalf("resolveCredential() error = nil, want an error")
			}
			// A store that exists but cannot be used is NOT "not logged in":
			// the user may well be logged in, with the tokens in a keyring.
			if errors.Is(err, provider.ErrNotLoggedIn{}) {
				t.Fatalf("resolveCredential() error = %v, want a non-ErrNotLoggedIn error", err)
			}
			msg := err.Error()
			if !strings.HasPrefix(msg, "credential store: ") {
				t.Errorf("error %q does not read as a credential-store message", msg)
			}
			if !strings.Contains(msg, "keyring") {
				t.Errorf("error %q does not mention the OS keyring case", msg)
			}
			if !strings.Contains(msg, tc.path) {
				t.Errorf("error %q does not name the store path %q", msg, tc.path)
			}
		})
	}
}

func TestCredentials_APIKeyModeUnsupported(t *testing.T) {
	clearEnv(t)

	tests := []struct {
		name string
		path string
	}{
		{name: "explicit auth_mode", path: fixture("auth_apikey.json")},
		{name: "api key without tokens", path: fixture("auth_apikey_no_mode.json")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// The store exists and parses, which is the whole of the Detect
			// contract, so an API-key store is detected and then reported by
			// Fetch — never silently omitted from the default listing.
			srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				t.Error("usage endpoint called for an API-key store")
			}))
			t.Cleanup(srv.Close)
			p := testProvider(t, srv, tc.path)

			ok, reason := p.Detect(context.Background())
			if !ok {
				t.Errorf("Detect() ok = false (reason %q), want true for an API-key store", reason)
			}
			if reason != "" {
				t.Errorf("Detect() reason = %q, want empty", reason)
			}

			_, err := p.Fetch(testContext(t))
			if err == nil {
				t.Fatalf("Fetch() error = nil, want an unsupported-mode error")
			}
			if !errors.Is(err, errAPIKeyMode) {
				t.Errorf("Fetch() error = %v, want errAPIKeyMode", err)
			}
			// Not a login problem, and not a parse failure.
			if errors.Is(err, provider.ErrNotLoggedIn{}) {
				t.Fatalf("Fetch() error = %v, want a non-ErrNotLoggedIn error", err)
			}
			// internal/usage renders an untyped error verbatim, so no
			// wrapping prefix of any kind may survive.
			want := `signed in with an API key (auth_mode "apikey"); Codex usage limits exist only for ChatGPT sign-in`
			if got := err.Error(); got != want {
				t.Errorf("Fetch() error message =\n  %q\nwant\n  %q", got, want)
			}
		})
	}
}

func TestCredentials_EnvOverrideOmitsAccountIDUnlessSet(t *testing.T) {
	tests := []struct {
		name          string
		accountIDEnv  string
		wantAccountID string
	}{
		{name: "account id unset", accountIDEnv: "", wantAccountID: ""},
		{name: "account id set", accountIDEnv: "env-account-id", wantAccountID: "env-account-id"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv(tokenEnvVar, "env-access-token")
			t.Setenv(accountIDEnvVar, tc.accountIDEnv)
			// The store would supply an account id; the override must not use it.
			p := New(WithCredentialPath(fixture("auth_chatgpt.json")))

			cred, err := p.resolveCredential(context.Background())
			if err != nil {
				t.Fatalf("resolveCredential() error = %v, want nil", err)
			}
			if cred.AccessToken != "env-access-token" {
				t.Errorf("AccessToken = %q, want %q", cred.AccessToken, "env-access-token")
			}
			if cred.AccountID != tc.wantAccountID {
				t.Errorf("AccountID = %q, want %q", cred.AccountID, tc.wantAccountID)
			}
			// With no local store read there is no id_token to mine a plan from.
			if cred.Plan != "" {
				t.Errorf("Plan = %q, want empty for an env override", cred.Plan)
			}
		})
	}
}

// makeIDToken builds a JWT-shaped id_token around payload. The signature is
// a placeholder: qmeter decodes the payload and never verifies it, and no
// real token is ever checked into this repository.
func makeIDToken(t *testing.T, payload map[string]any) string {
	t.Helper()
	enc := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal id_token segment: %v", err)
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	header := enc(map[string]any{"alg": "RS256", "typ": "JWT"})
	return header + "." + enc(payload) + "." + base64.RawURLEncoding.EncodeToString([]byte("not-a-signature"))
}

func TestIDToken_DecodesPlanTypeAndAccountID(t *testing.T) {
	tests := []struct {
		name          string
		payload       map[string]any
		wantPlan      string
		wantAccountID string
	}{
		{
			name: "top-level claims",
			payload: map[string]any{
				"sub":                "user-123",
				"chatgpt_plan_type":  "pro",
				"chatgpt_account_id": "claim-account-id",
			},
			wantPlan:      "pro",
			wantAccountID: "claim-account-id",
		},
		{
			name: "namespaced auth claim",
			payload: map[string]any{
				"sub": "user-123",
				"https://api.openai.com/auth": map[string]any{
					"chatgpt_plan_type":  "plus",
					"chatgpt_account_id": "nested-account-id",
				},
			},
			wantPlan:      "plus",
			wantAccountID: "nested-account-id",
		},
		{
			name: "camelCase claims",
			payload: map[string]any{
				"chatgptPlanType":  "team",
				"chatgptAccountId": "camel-account-id",
			},
			wantPlan:      "team",
			wantAccountID: "camel-account-id",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := decodeIDToken(makeIDToken(t, tc.payload))
			if err != nil {
				t.Fatalf("decodeIDToken() error = %v, want nil", err)
			}
			if claims.PlanType != tc.wantPlan {
				t.Errorf("PlanType = %q, want %q", claims.PlanType, tc.wantPlan)
			}
			if claims.AccountID != tc.wantAccountID {
				t.Errorf("AccountID = %q, want %q", claims.AccountID, tc.wantAccountID)
			}
		})
	}
}

func TestIDToken_DecodesPaddedBase64Payload(t *testing.T) {
	// The JWT spec says unpadded base64url, but a padded payload costs
	// nothing to accept and one drifted producer would otherwise lose the
	// plan name.
	payload, err := json.Marshal(map[string]any{"chatgpt_plan_type": "plus"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	padded := "aGVhZGVy." + base64.URLEncoding.EncodeToString(payload) + ".c2ln"
	if !strings.Contains(padded, "=") {
		t.Fatalf("payload %q is not padded; the case under test is not exercised", padded)
	}

	claims, err := decodeIDToken(padded)
	if err != nil {
		t.Fatalf("decodeIDToken() error = %v, want nil", err)
	}
	if claims.PlanType != "plus" {
		t.Errorf("PlanType = %q, want %q", claims.PlanType, "plus")
	}
}

func TestIDToken_DecodableWhenExpired(t *testing.T) {
	// The id_token lives an hour; its exp is irrelevant to usage lookup, so
	// the decoder must ignore it entirely rather than reject the token.
	expired := makeIDToken(t, map[string]any{
		"exp":                time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC).Unix(),
		"iat":                time.Date(1999, time.December, 31, 23, 0, 0, 0, time.UTC).Unix(),
		"chatgpt_plan_type":  "pro",
		"chatgpt_account_id": "expired-account-id",
	})

	claims, err := decodeIDToken(expired)
	if err != nil {
		t.Fatalf("decodeIDToken() error = %v, want nil for an expired token", err)
	}
	if claims.PlanType != "pro" {
		t.Errorf("PlanType = %q, want %q", claims.PlanType, "pro")
	}
	if claims.AccountID != "expired-account-id" {
		t.Errorf("AccountID = %q, want %q", claims.AccountID, "expired-account-id")
	}
}

func TestIDToken_RejectsUndecodableTokens(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{name: "empty", token: ""},
		{name: "not a jwt", token: "just-a-string"},
		{name: "bad base64 payload", token: "aGVhZGVy.!!!not-base64!!!.c2ln"},
		{name: "payload is not json", token: "aGVhZGVy." + base64.RawURLEncoding.EncodeToString([]byte("nope")) + ".c2ln"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeIDToken(tc.token); err == nil {
				t.Fatalf("decodeIDToken(%q) error = nil, want an error", tc.token)
			}
		})
	}
}

func TestCredentials_StoreClaimsSupplyPlanAndAccountID(t *testing.T) {
	clearEnv(t)

	store := filepath.Join(t.TempDir(), "auth.json")
	body := map[string]any{
		"auth_mode":    "chatgpt",
		"last_refresh": "2026-09-15T10:04:11.123456Z",
		"tokens": map[string]any{
			"access_token":  "store-access-token",
			"refresh_token": "store-refresh-token",
			"id_token": makeIDToken(t, map[string]any{
				"chatgpt_plan_type":  "pro",
				"chatgpt_account_id": "claim-account-id",
			}),
			// No account_id here: the claim is the fallback.
			"account_id": "",
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal store: %v", err)
	}
	if err := os.WriteFile(store, raw, 0o600); err != nil {
		t.Fatalf("write store: %v", err)
	}

	cred, err := New(WithCredentialPath(store)).resolveCredential(context.Background())
	if err != nil {
		t.Fatalf("resolveCredential() error = %v, want nil", err)
	}
	if cred.Plan != "pro" {
		t.Errorf("Plan = %q, want %q", cred.Plan, "pro")
	}
	if cred.AccountID != "claim-account-id" {
		t.Errorf("AccountID = %q, want %q", cred.AccountID, "claim-account-id")
	}
}

func TestCredentials_StoreAccountIDWinsOverClaim(t *testing.T) {
	clearEnv(t)

	store := filepath.Join(t.TempDir(), "auth.json")
	body := map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"access_token": "store-access-token",
			"id_token": makeIDToken(t, map[string]any{
				"chatgpt_plan_type":  "pro",
				"chatgpt_account_id": "claim-account-id",
			}),
			"account_id": "store-account-id",
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal store: %v", err)
	}
	if err := os.WriteFile(store, raw, 0o600); err != nil {
		t.Fatalf("write store: %v", err)
	}

	cred, err := New(WithCredentialPath(store)).resolveCredential(context.Background())
	if err != nil {
		t.Fatalf("resolveCredential() error = %v, want nil", err)
	}
	if cred.AccountID != "store-account-id" {
		t.Errorf("AccountID = %q, want %q", cred.AccountID, "store-account-id")
	}
}

func TestCredentials_UndecodableIDTokenIsNotFatal(t *testing.T) {
	clearEnv(t)

	store := filepath.Join(t.TempDir(), "auth.json")
	raw := []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"store-access-token",` +
		`"id_token":"not-a-jwt","account_id":"store-account-id"}}`)
	if err := os.WriteFile(store, raw, 0o600); err != nil {
		t.Fatalf("write store: %v", err)
	}

	cred, err := New(WithCredentialPath(store)).resolveCredential(context.Background())
	if err != nil {
		t.Fatalf("resolveCredential() error = %v, want nil", err)
	}
	if cred.AccessToken != "store-access-token" {
		t.Errorf("AccessToken = %q, want %q", cred.AccessToken, "store-access-token")
	}
	if cred.Plan != "" {
		t.Errorf("Plan = %q, want empty", cred.Plan)
	}
}

// testNow is the fixed "now" every Fetch test runs against, so a
// resets_in_seconds turns into a predictable absolute time.
var testNow = time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)

// usageServer serves a fixture body and records the request it was asked
// for, so tests can assert the exact URL and headers qmeter sends.
func usageServer(t *testing.T, fixtureName string) (*httptest.Server, *http.Request) {
	t.Helper()
	body, err := os.ReadFile(fixture(fixtureName))
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixtureName, err)
	}
	var got http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = *r.Clone(context.Background())
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(body); err != nil {
			t.Errorf("write fixture body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

// testProvider builds a provider pointed at srv, with a fixed clock.
func testProvider(t *testing.T, srv *httptest.Server, credentialPath string) *Provider {
	t.Helper()
	return New(
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithCredentialPath(credentialPath),
		WithClock(func() time.Time { return testNow }),
	)
}

// testContext returns a context with a deadline: httpx sets no timeout of
// its own, so every caller must supply one.
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// assertWindows compares windows field by field, comparing times with
// Equal so a differing location never fails a correct result.
func assertWindows(t *testing.T, got, want []provider.Window) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d windows, want %d:\ngot  %+v\nwant %+v", len(got), len(want), got, want)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Provider != w.Provider || g.Name != w.Name || g.Plan != w.Plan ||
			g.RemainingPercent != w.RemainingPercent || g.Period != w.Period ||
			g.RateLimited != w.RateLimited || !g.ResetsAt.Equal(w.ResetsAt) {
			t.Errorf("window %d =\n  %+v\nwant\n  %+v", i, g, w)
		}
	}
}

func TestFetch_ParsesSnakeCaseFields(t *testing.T) {
	clearEnv(t)
	srv, req := usageServer(t, "usage_snake_case.json")
	p := testProvider(t, srv, fixture("auth_chatgpt.json"))

	got, err := p.Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}

	assertWindows(t, got, []provider.Window{
		{
			Provider:         "codex",
			Name:             "5h",
			Plan:             "plus",
			RemainingPercent: 57.5,
			ResetsAt:         time.Date(2026, time.September, 16, 18, 30, 0, 0, time.UTC),
			Period:           5 * time.Hour,
		},
		{
			Provider:         "codex",
			Name:             "weekly",
			Plan:             "plus",
			RemainingPercent: 81.75,
			ResetsAt:         time.Date(2026, time.September, 19, 0, 0, 0, 0, time.UTC),
			Period:           7 * 24 * time.Hour,
		},
	})

	if req.URL.Path != "/backend-api/wham/usage" {
		t.Errorf("request path = %q, want %q", req.URL.Path, "/backend-api/wham/usage")
	}
	if got, want := req.Header.Get("Authorization"), "Bearer store-access-token"; got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
	if got, want := req.Header.Get("ChatGPT-Account-Id"), "store-account-id"; got != want {
		t.Errorf("ChatGPT-Account-Id = %q, want %q", got, want)
	}
	if got, want := req.Header.Get("Accept"), "application/json"; got != want {
		t.Errorf("Accept = %q, want %q", got, want)
	}
}

func TestFetch_ParsesCamelCaseFields(t *testing.T) {
	clearEnv(t)
	srv, _ := usageServer(t, "usage_camel_case.json")
	p := testProvider(t, srv, fixture("auth_chatgpt.json"))

	got, err := p.Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}

	assertWindows(t, got, []provider.Window{
		{
			Provider:         "codex",
			Name:             "5h",
			Plan:             "pro",
			RemainingPercent: 0.5,
			// resetsInSeconds is relative to the injected clock.
			ResetsAt: testNow.Add(time.Hour),
			Period:   5 * time.Hour,
		},
		{
			Provider:         "codex",
			Name:             "weekly",
			Plan:             "pro",
			RemainingPercent: 0,
			// resetsAt given as an epoch rather than an ISO string.
			ResetsAt:    time.Unix(1789500000, 0).UTC(),
			Period:      7 * 24 * time.Hour,
			RateLimited: true,
		},
	})
}

func TestFetch_ParsesAdditionalRateLimits(t *testing.T) {
	clearEnv(t)
	srv, _ := usageServer(t, "usage_additional_limits.json")
	p := testProvider(t, srv, fixture("auth_chatgpt.json"))

	got, err := p.Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}

	assertWindows(t, got, []provider.Window{
		{
			Provider:         "codex",
			Name:             "5h",
			Plan:             "pro",
			RemainingPercent: 90,
			ResetsAt:         testNow.Add(10 * time.Minute),
			Period:           5 * time.Hour,
		},
		{
			Provider:         "codex",
			Name:             "gpt-5-codex primary",
			Plan:             "pro",
			RemainingPercent: 80,
			ResetsAt:         testNow.Add(time.Minute),
			Period:           5 * time.Hour,
		},
		{
			Provider:         "codex",
			Name:             "gpt-5-codex secondary",
			Plan:             "pro",
			RemainingPercent: 70,
			ResetsAt:         testNow.Add(2 * time.Minute),
			Period:           7 * 24 * time.Hour,
		},
		{
			Provider:         "codex",
			Name:             "sora primary",
			Plan:             "pro",
			RemainingPercent: 60,
			ResetsAt:         testNow.Add(30 * time.Second),
			Period:           24 * time.Hour,
		},
	})
}

func TestFetch_SkipsWindowsWithoutUsedPercent(t *testing.T) {
	clearEnv(t)
	srv, _ := usageServer(t, "usage_missing_windows.json")
	p := testProvider(t, srv, fixture("auth_chatgpt.json"))

	got, err := p.Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Fatalf("Fetch() = %+v, want no windows", got)
	}
}

func TestFetch_PlanFallsBackToIDTokenClaim(t *testing.T) {
	clearEnv(t)

	store := filepath.Join(t.TempDir(), "auth.json")
	raw := []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"store-access-token",` +
		`"id_token":"` + makeIDToken(t, map[string]any{"chatgpt_plan_type": "pro"}) + `",` +
		`"account_id":"store-account-id"}}`)
	if err := os.WriteFile(store, raw, 0o600); err != nil {
		t.Fatalf("write store: %v", err)
	}

	srv, _ := usageServer(t, "usage_no_plan_type.json")
	got, err := testProvider(t, srv, store).Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}
	if len(got) != 1 {
		t.Fatalf("Fetch() = %+v, want 1 window", got)
	}
	if got[0].Plan != "pro" {
		t.Errorf("Plan = %q, want %q from the id_token claim", got[0].Plan, "pro")
	}
}

func TestFetch_EnvOverrideSendsAccountHeaderOnlyWhenSet(t *testing.T) {
	tests := []struct {
		name          string
		accountIDEnv  string
		wantHeaderSet bool
	}{
		{name: "account id unset", accountIDEnv: "", wantHeaderSet: false},
		{name: "account id set", accountIDEnv: "env-account-id", wantHeaderSet: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv(tokenEnvVar, "env-access-token")
			t.Setenv(accountIDEnvVar, tc.accountIDEnv)

			srv, req := usageServer(t, "usage_no_plan_type.json")
			// The store would supply an account id; the override must not use it.
			got, err := testProvider(t, srv, fixture("auth_chatgpt.json")).Fetch(testContext(t))
			if err != nil {
				t.Fatalf("Fetch() error = %v, want nil", err)
			}
			if got, want := req.Header.Get("Authorization"), "Bearer env-access-token"; got != want {
				t.Errorf("Authorization = %q, want %q", got, want)
			}
			header := req.Header.Get("ChatGPT-Account-Id")
			if tc.wantHeaderSet && header != tc.accountIDEnv {
				t.Errorf("ChatGPT-Account-Id = %q, want %q", header, tc.accountIDEnv)
			}
			if !tc.wantHeaderSet && header != "" {
				t.Errorf("ChatGPT-Account-Id = %q, want no header", header)
			}
			// No local store was read, so the response is the only plan source.
			if len(got) != 1 || got[0].Plan != "" {
				t.Errorf("Fetch() = %+v, want one window with an empty plan", got)
			}
		})
	}
}

func TestFetch_UnauthorizedReturnsTokenExpired(t *testing.T) {
	clearEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	_, err := testProvider(t, srv, fixture("auth_chatgpt.json")).Fetch(testContext(t))
	if !errors.Is(err, provider.ErrTokenExpired{}) {
		t.Fatalf("Fetch() error = %v, want ErrTokenExpired", err)
	}
	var expired provider.ErrTokenExpired
	if !errors.As(err, &expired) {
		t.Fatalf("errors.As(%v) failed", err)
	}
	if expired.Tool != "codex" {
		t.Errorf("Tool = %q, want %q", expired.Tool, "codex")
	}
}

func TestFetch_TooManyRequestsReturnsRateLimited(t *testing.T) {
	clearEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	_, err := testProvider(t, srv, fixture("auth_chatgpt.json")).Fetch(testContext(t))
	if !errors.Is(err, provider.ErrRateLimited{}) {
		t.Fatalf("Fetch() error = %v, want ErrRateLimited", err)
	}
	var limited provider.ErrRateLimited
	if !errors.As(err, &limited) {
		t.Fatalf("errors.As(%v) failed", err)
	}
	if limited.RetryAfter != 2*time.Minute {
		t.Errorf("RetryAfter = %v, want %v", limited.RetryAfter, 2*time.Minute)
	}
}

func TestFetch_MalformedResponseIsAnError(t *testing.T) {
	clearEnv(t)
	srv, _ := usageServer(t, "usage_malformed.json")

	got, err := testProvider(t, srv, fixture("auth_chatgpt.json")).Fetch(testContext(t))
	if err == nil {
		t.Fatalf("Fetch() = %+v, error = nil; want a decode error", got)
	}
	if got != nil {
		t.Errorf("Fetch() = %+v, want no windows alongside the error", got)
	}
}

func TestFetch_CredentialErrorIsReturnedUnprefixed(t *testing.T) {
	clearEnv(t)
	srv, _ := usageServer(t, "usage_snake_case.json")
	p := testProvider(t, srv, filepath.Join(t.TempDir(), "auth.json"))

	_, err := p.Fetch(testContext(t))
	if !errors.Is(err, provider.ErrNotLoggedIn{}) {
		t.Fatalf("Fetch() error = %v, want ErrNotLoggedIn", err)
	}
	// internal/usage renders untyped errors verbatim, so no provider name
	// may creep into the message.
	if want := "not logged in, run codex to log in"; err.Error() != want {
		t.Errorf("Fetch() error message = %q, want %q", err.Error(), want)
	}
}

func TestWindowName_DerivesFriendlyNamesFromPeriod(t *testing.T) {
	tests := []struct {
		name     string
		period   time.Duration
		fallback string
		want     string
	}{
		{name: "five hours", period: 300 * time.Minute, fallback: "primary", want: "5h"},
		{name: "one week", period: 10080 * time.Minute, fallback: "secondary", want: "weekly"},
		{name: "one day", period: 24 * time.Hour, fallback: "primary", want: "daily"},
		{name: "thirty days", period: 30 * 24 * time.Hour, fallback: "primary", want: "monthly"},
		{name: "whole hours", period: 3 * time.Hour, fallback: "primary", want: "3h"},
		{name: "whole minutes", period: 90 * time.Second, fallback: "primary", want: "primary"},
		{name: "ten minutes", period: 10 * time.Minute, fallback: "primary", want: "10m"},
		{name: "unknown period", period: 0, fallback: "secondary", want: "secondary"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := windowName(tc.period, tc.fallback); got != tc.want {
				t.Errorf("windowName(%v, %q) = %q, want %q", tc.period, tc.fallback, got, tc.want)
			}
		})
	}
}

func TestIDToken_PrefersNamespacedClaimOverOtherNestedObjects(t *testing.T) {
	// Several claims can be objects; the OpenAI namespace is the one that
	// actually carries these claims, so the result must not depend on map
	// iteration order.
	token := makeIDToken(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_plan_type":  "pro",
			"chatgpt_account_id": "namespaced-account-id",
		},
		// A decoy that sorts before the namespace and would otherwise win
		// roughly half the time.
		"aaa_other_claim": map[string]any{
			"chatgpt_plan_type":  "decoy-plan",
			"chatgpt_account_id": "decoy-account-id",
		},
		"zzz_other_claim": map[string]any{
			"chatgpt_plan_type":  "decoy-plan",
			"chatgpt_account_id": "decoy-account-id",
		},
	})

	// Repeat: a map-order bug passes intermittently on a single attempt.
	for i := range 50 {
		claims, err := decodeIDToken(token)
		if err != nil {
			t.Fatalf("attempt %d: decodeIDToken() error = %v, want nil", i, err)
		}
		if claims.PlanType != "pro" {
			t.Fatalf("attempt %d: PlanType = %q, want %q", i, claims.PlanType, "pro")
		}
		if claims.AccountID != "namespaced-account-id" {
			t.Fatalf("attempt %d: AccountID = %q, want %q", i, claims.AccountID, "namespaced-account-id")
		}
	}
}

func TestIDToken_NestedClaimsAreReadInSortedOrder(t *testing.T) {
	// With no OpenAI namespace present, the remaining object claims are
	// tried in sorted key order, so the winner is at least deterministic.
	token := makeIDToken(t, map[string]any{
		"aaa_claim": map[string]any{"chatgpt_plan_type": "first-plan"},
		"bbb_claim": map[string]any{"chatgpt_plan_type": "second-plan"},
	})

	for i := range 50 {
		claims, err := decodeIDToken(token)
		if err != nil {
			t.Fatalf("attempt %d: decodeIDToken() error = %v, want nil", i, err)
		}
		if claims.PlanType != "first-plan" {
			t.Fatalf("attempt %d: PlanType = %q, want %q", i, claims.PlanType, "first-plan")
		}
	}
}

func TestFetch_ServerErrorIsNotSelfPrefixed(t *testing.T) {
	clearEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream exploded", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	_, err := testProvider(t, srv, fixture("auth_chatgpt.json")).Fetch(testContext(t))
	if err == nil {
		t.Fatalf("Fetch() error = nil, want an error for status 500")
	}
	// internal/usage prints "codex\terror: <message>", so a message that
	// names the provider again reads as "codex error: codex: ...".
	if strings.HasPrefix(err.Error(), providerID) {
		t.Errorf("Fetch() error %q prefixes itself with the provider name", err.Error())
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Fetch() error %q does not carry the status code", err.Error())
	}
}

func TestFetch_ResponseWithoutRateLimitsIsAnError(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "null body", body: "null"},
		{name: "empty object", body: "{}"},
		{name: "only credits", body: `{"credits":{"balance":5}}`},
		{name: "explicit null rate limit", body: `{"plan_type":"plus","rate_limit":null}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(tc.body)); err != nil {
					t.Errorf("write body: %v", err)
				}
			}))
			t.Cleanup(srv.Close)

			got, err := testProvider(t, srv, fixture("auth_chatgpt.json")).Fetch(testContext(t))
			if err == nil {
				t.Fatalf("Fetch() = %+v, error = nil; want an error for a response with no rate limits", got)
			}
			if !errors.Is(err, errNoRateLimits) {
				t.Errorf("Fetch() error = %v, want errNoRateLimits", err)
			}
			if got != nil {
				t.Errorf("Fetch() = %+v, want no windows alongside the error", got)
			}
		})
	}
}

func TestFetch_EmptyRateLimitObjectIsNotAnError(t *testing.T) {
	// A present-but-empty rate_limit means "no windows to report", which is
	// not the same as a response that carried no rate limits at all.
	clearEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"plan_type":"plus","rate_limit":{}}`)); err != nil {
			t.Errorf("write body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	got, err := testProvider(t, srv, fixture("auth_chatgpt.json")).Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("Fetch() = %+v, want no windows", got)
	}
}

func TestFetch_ParsesLiveResponseShape(t *testing.T) {
	// The shape the endpoint actually returns (observed 2026-09-16): windows
	// keyed limit_window_seconds / reset_after_seconds / reset_at, additional
	// limits keyed limit_name with their windows nested under their own
	// rate_limit object, and a null secondary_window.
	clearEnv(t)
	srv, _ := usageServer(t, "usage_live_shape.json")

	got, err := testProvider(t, srv, fixture("auth_chatgpt.json")).Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}

	assertWindows(t, got, []provider.Window{
		{
			Provider:         "codex",
			Name:             "5h",
			Plan:             "pro",
			RemainingPercent: 87.5,
			ResetsAt:         time.Unix(1789500000, 0).UTC(),
			Period:           5 * time.Hour,
		},
		{
			Provider:         "codex",
			Name:             "gpt-5-codex primary",
			Plan:             "pro",
			RemainingPercent: 80,
			ResetsAt:         time.Unix(1789490000, 0).UTC(),
			Period:           5 * time.Hour,
		},
		{
			Provider:         "codex",
			Name:             "gpt-5-codex secondary",
			Plan:             "pro",
			RemainingPercent: 70,
			ResetsAt:         time.Unix(1789900000, 0).UTC(),
			Period:           7 * 24 * time.Hour,
		},
		{
			// limit_name is empty here, so the metered feature names it.
			Provider:         "codex",
			Name:             "code_review primary",
			Plan:             "pro",
			RemainingPercent: 95,
			ResetsAt:         time.Unix(1789600000, 0).UTC(),
			Period:           7 * 24 * time.Hour,
		},
	})

	// The whole point of the repair: a real reset time, not "-".
	for i, w := range got {
		if w.ResetsAt.IsZero() {
			t.Errorf("window %d (%s) has a zero ResetsAt", i, w.Name)
		}
		if w.Period == 0 {
			t.Errorf("window %d (%s) has a zero Period", i, w.Name)
		}
	}
}

func TestFetch_LiveWindowFallsBackToResetAfterSeconds(t *testing.T) {
	// Some windows report only the relative reset; it is measured against
	// the injected clock.
	clearEnv(t)
	body := `{"plan_type":"plus","rate_limit":{"allowed":true,"limit_reached":false,` +
		`"primary_window":{"limit_window_seconds":604800,"reset_after_seconds":900,"used_percent":42},` +
		`"secondary_window":null}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	got, err := testProvider(t, srv, fixture("auth_chatgpt.json")).Fetch(testContext(t))
	if err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}
	assertWindows(t, got, []provider.Window{
		{
			Provider:         "codex",
			Name:             "weekly",
			Plan:             "plus",
			RemainingPercent: 58,
			ResetsAt:         testNow.Add(15 * time.Minute),
			Period:           7 * 24 * time.Hour,
		},
	})
}

func TestFetch_ReferenceSpellingWinsWhenBothArePresent(t *testing.T) {
	// Drift tolerance runs both ways, so a response could carry both
	// spellings. The documented precedence is the reference table's spelling
	// first, and it must not depend on map iteration order.
	clearEnv(t)
	body := `{"plan_type":"pro","rate_limit":{"primary_window":{` +
		`"used_percent":10,` +
		`"window_minutes":300,"limit_window_seconds":604800,` +
		`"resets_at":"2026-09-16T18:30:00Z","reset_at":1000000000,` +
		`"resets_in_seconds":60,"reset_after_seconds":120}},` +
		`"additional_rate_limits":[{` +
		`"name":"reference-name","limit_name":"live-name","metered_feature":"feature-name",` +
		`"primary_window":{"used_percent":20,"window_minutes":300,"resets_in_seconds":30},` +
		`"rate_limit":{"primary_window":{"used_percent":99,"window_minutes":10080,"resets_in_seconds":30}}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	p := testProvider(t, srv, fixture("auth_chatgpt.json"))
	// Repeated: a map-order-dependent winner passes intermittently.
	for i := range 20 {
		got, err := p.Fetch(testContext(t))
		if err != nil {
			t.Fatalf("attempt %d: Fetch() error = %v, want nil", i, err)
		}
		assertWindows(t, got, []provider.Window{
			{
				Provider:         "codex",
				Name:             "5h", // window_minutes 300, not limit_window_seconds 604800
				Plan:             "pro",
				RemainingPercent: 90,
				ResetsAt:         time.Date(2026, time.September, 16, 18, 30, 0, 0, time.UTC),
				Period:           5 * time.Hour,
			},
			{
				Provider:         "codex",
				Name:             "reference-name primary", // not limit_name, not metered_feature
				Plan:             "pro",
				RemainingPercent: 80, // the entry's own window, not the nested one
				ResetsAt:         testNow.Add(30 * time.Second),
				Period:           5 * time.Hour,
			},
		})
		if t.Failed() {
			t.Fatalf("attempt %d failed", i)
		}
	}
}
