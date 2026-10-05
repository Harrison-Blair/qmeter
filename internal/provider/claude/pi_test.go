package claude

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

func writePi(t *testing.T, expires int64) {
	t.Helper()
	data := fmt.Sprintf(`{"anthropic":{"type":"oauth","access":"pi-token","expires":%d}}`, expires)
	if err := os.WriteFile(filepath.Join(os.Getenv("PI_CODING_AGENT_DIR"), "auth.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestPi_LocalFallback(t *testing.T) {
	for _, name := range []string{"missing", "credentials_expired.json", "malformed"} {
		t.Run(name, func(t *testing.T) {
			path := fixture(name)
			if name == "malformed" {
				path = filepath.Join(t.TempDir(), "credentials.json")
				if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			p := newProvider(t, WithCredentialPath(path))
			writePi(t, testNow.UnixMilli()+1)
			cred, src, err := p.resolve(context.Background())
			if err != nil || cred.AccessToken != "pi-token" || cred.Plan != "" || string(src) != "pi" {
				t.Fatalf("resolve = %+v, %q, %v", cred, src, err)
			}
			if ok, reason := p.Detect(context.Background()); !ok {
				t.Fatal(reason)
			}
		})
	}
}

func TestPi_NativePreferred(t *testing.T) {
	p := newProvider(t)
	writePi(t, testNow.UnixMilli()+1)
	cred, _, err := p.resolve(context.Background())
	if err != nil || cred.AccessToken != "sk-ant-oat01-store-token" || cred.Plan != "max" {
		t.Fatalf("resolve = %+v, %v", cred, err)
	}
}

func TestPi_RequestFallback(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		override bool
		requests int
		success  bool
	}{
		{"unauthorized", 401, false, 2, true}, {"forbidden", 403, false, 2, true},
		{"override", 401, true, 1, false}, {"rate_limit", 429, false, 1, false}, {"outage", 500, false, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Authorization") == "Bearer pi-token" {
					fmt.Fprint(w, `{"five_hour":{"utilization":25}}`)
					return
				}
				w.WriteHeader(tc.status)
			}))
			defer server.Close()
			p := newProvider(t, WithBaseURL(server.URL))
			writePi(t, testNow.UnixMilli()+1)
			if tc.override {
				t.Setenv(envVar, "override")
			}
			usage, err := p.Fetch(testContext(t))
			if (err == nil) != tc.success || calls != tc.requests {
				t.Fatalf("Fetch err=%v requests=%d", err, calls)
			}
			if tc.success && (len(usage.Windows) != 1 || usage.Windows[0].Plan != "") {
				t.Fatalf("usage=%+v", usage)
			}
		})
	}
}

func TestPi_ExpiryAndRejectedHints(t *testing.T) {
	for _, expired := range []bool{true, false} {
		t.Run(fmt.Sprint(expired), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(401) }))
			defer server.Close()
			p := newProvider(t, WithCredentialPath(fixture("missing")), WithBaseURL(server.URL))
			expiry := testNow.UnixMilli() + 1
			if expired {
				expiry = testNow.UnixMilli()
			}
			writePi(t, expiry)
			_, err := p.Fetch(testContext(t))
			var tokenErr provider.ErrTokenExpired
			if !errors.As(err, &tokenErr) || tokenErr.Tool != "pi" {
				t.Fatalf("err=%v", err)
			}
			if expired && calls != 0 {
				t.Fatalf("expired token made %d requests", calls)
			}
			if ok, reason := p.Detect(context.Background()); !ok {
				t.Fatal(reason)
			}
		})
	}
}

func TestPi_KeychainFallback(t *testing.T) {
	p := newProvider(t, WithCredentialPath(fixture("missing")))
	p.keychain = func(context.Context) ([]byte, error) { return nil, errors.New("lookup denied") }
	_, _, err := p.resolve(context.Background())
	if err == nil || !strings.Contains(err.Error(), "lookup denied") {
		t.Fatalf("err=%v", err)
	}
	writePi(t, testNow.UnixMilli()+1)
	cred, _, err := p.resolve(context.Background())
	if err != nil || cred.AccessToken != "pi-token" {
		t.Fatalf("cred=%+v err=%v", cred, err)
	}
}

func TestPi_PathOverrideAndReload(t *testing.T) {
	p := newProvider(t, WithCredentialPath(fixture("missing")))
	writePi(t, testNow.UnixMilli()+1)
	path := filepath.Join(os.Getenv("PI_CODING_AGENT_DIR"), "auth.json")
	p = New(WithCredentialPath(fixture("missing")), WithPiCredentialPath(path), WithClock(func() time.Time { return testNow }))
	t.Setenv("PI_CODING_AGENT_DIR", t.TempDir())
	cred, _, err := p.resolve(context.Background())
	if err != nil || cred.AccessToken != "pi-token" {
		t.Fatalf("cred=%+v err=%v", cred, err)
	}
	if err := os.WriteFile(path, []byte(`{"anthropic":{"type":"oauth","access":"refreshed"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	cred, _, err = p.resolve(context.Background())
	if err != nil || cred.AccessToken != "refreshed" {
		t.Fatalf("cred=%+v err=%v", cred, err)
	}
}

func TestPi_HTTPRetryStopsAfterPi(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusForbidden) }))
	defer server.Close()
	p := newProvider(t, WithBaseURL(server.URL))
	writePi(t, testNow.UnixMilli()+1)
	_, err := p.Fetch(testContext(t))
	var tokenErr provider.ErrTokenExpired
	if calls != 2 || !errors.As(err, &tokenErr) || tokenErr.Tool != "pi" {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestPi_CanceledContextDoesNotRetry(t *testing.T) {
	p := newProvider(t, WithBaseURL("http://127.0.0.1:1"))
	writePi(t, testNow.UnixMilli()+1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.Fetch(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

func TestPi_MissingAfterRejectedNativePreservesHint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer server.Close()
	p := newProvider(t, WithBaseURL(server.URL))
	_, err := p.Fetch(testContext(t))
	var tokenErr provider.ErrTokenExpired
	if !errors.As(err, &tokenErr) || tokenErr.Tool != "claude" {
		t.Fatalf("err=%v", err)
	}
}

func TestPi_CanceledNativeLookupPreservesCancellation(t *testing.T) {
	p := newProvider(t, WithCredentialPath(fixture("missing")))
	p.keychain = func(ctx context.Context) ([]byte, error) { return nil, ctx.Err() }
	writePi(t, testNow.UnixMilli()+1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := p.resolve(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want cancellation", err)
	}
}
