package opencodego

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

func piStore(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", dir)
	path := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPiDetectAndLocalFallback(t *testing.T) {
	for _, fixture := range []string{"missing", "auth_malformed.json", "auth_no_go_entry.json", "auth_empty_key.json"} {
		t.Run(fixture, func(t *testing.T) {
			clearEnv(t)
			piStore(t, `{"opencode-go":{"type":"api_key","key":"pi-key"}}`)
			path := missingStore(t)
			if fixture != "missing" {
				path = filepath.Join("testdata", fixture)
			}
			p := New(WithDBPath(missingDB(t)), WithCredentialPath(path))
			if ok, reason := p.Detect(context.Background()); !ok || reason != "" {
				t.Fatalf("Detect = %v, %q; want true with Pi credential", ok, reason)
			}
			key, _, err := p.credential(context.Background())
			if err != nil || key != "pi-key" {
				t.Fatalf("credential = %q, %v; want pi-key", key, err)
			}
		})
	}
}

func TestPiFetchPrecedenceAndAuthenticationFallback(t *testing.T) {
	for _, tc := range []struct {
		name         string
		status       int
		dbKey        string
		override     string
		piStatus     int
		wantRequests string
		wantError    bool
		wantTool     string
	}{
		{name: "native preferred", status: 200, piStatus: 200, wantRequests: "Bearer native-key"},
		{name: "SQLite preferred", status: 200, dbKey: "db-key", piStatus: 200, wantRequests: "Bearer db-key"},
		{name: "SQLite rejected retries Pi", status: 401, dbKey: "db-key", piStatus: 200, wantRequests: "Bearer db-key,Bearer pi-key"},
		{name: "unauthorized retries Pi", status: 401, piStatus: 200, wantRequests: "Bearer native-key,Bearer pi-key"},
		{name: "forbidden retries Pi", status: 403, piStatus: 200, wantRequests: "Bearer native-key,Bearer pi-key"},
		{name: "Pi rejected once", status: 401, piStatus: 401, wantRequests: "Bearer native-key,Bearer pi-key", wantError: true, wantTool: "pi"},
		{name: "override authoritative", status: 401, override: "override-key", piStatus: 200, wantRequests: "Bearer override-key", wantError: true, wantTool: "opencode"},
		{name: "rate limited", status: 429, piStatus: 200, wantRequests: "Bearer native-key", wantError: true},
		{name: "server error", status: 500, piStatus: 200, wantRequests: "Bearer native-key", wantError: true},
		{name: "malformed response", status: 200, piStatus: 200, wantRequests: "Bearer native-key", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t)
			piStore(t, `{"opencode-go":{"type":"api_key","key":"pi-key"}}`)
			t.Setenv("QMETER_OPENCODE_GO_KEY", tc.override)
			native := filepath.Join(t.TempDir(), "auth.json")
			if err := os.WriteFile(native, []byte(`{"opencode-go":{"type":"api","key":"native-key"}}`), 0o600); err != nil {
				t.Fatal(err)
			}
			requests := make(chan string, 4)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				auth := r.Header.Get("Authorization")
				requests <- auth
				status := tc.status
				if auth == "Bearer pi-key" {
					status = tc.piStatus
				}
				w.WriteHeader(status)
				if tc.name == "malformed response" {
					_, _ = w.Write([]byte(`{`))
					return
				}
				_, _ = w.Write([]byte(`{"usage":{"rolling":{"status":"ok","percent":42}}}`))
			}))
			defer srv.Close()
			dbPath := missingDB(t)
			if tc.dbKey != "" {
				dbPath = seedDB(t, tc.dbKey)
			}
			p := New(WithDBPath(dbPath), WithCredentialPath(native), WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
			got, err := p.Fetch(testContext(t))
			if (err != nil) != tc.wantError {
				t.Fatalf("Fetch error = %v, wantError %v", err, tc.wantError)
			}
			var observed []string
			for len(requests) > 0 {
				observed = append(observed, <-requests)
			}
			if joined := strings.Join(observed, ","); joined != tc.wantRequests {
				t.Fatalf("requests = %s, want %s", joined, tc.wantRequests)
			}
			if tc.wantTool != "" {
				var expired provider.ErrTokenExpired
				if !errors.As(err, &expired) || expired.Tool != tc.wantTool {
					t.Fatalf("error = %v, want expired tool %s", err, tc.wantTool)
				}
			}
			if !tc.wantError && (len(got.Windows) != 1 || got.Windows[0].Provider != "opencode-go" || got.Windows[0].RemainingPercent != 58) {
				t.Fatalf("usage = %+v", got)
			}
		})
	}
}

func TestPiCredentialsReloadOnRefresh(t *testing.T) {
	clearEnv(t)
	path := piStore(t, `{"opencode-go":{"type":"api_key","key":"first"}}`)
	p := New(WithDBPath(missingDB(t)), WithCredentialPath(missingStore(t)))
	if key, _, err := p.credential(context.Background()); err != nil || key != "first" {
		t.Fatalf("first credential = %q, %v", key, err)
	}
	if err := os.WriteFile(path, []byte(`{"opencode-go":{"type":"api_key","key":"second"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if key, _, err := p.credential(context.Background()); err != nil || key != "second" {
		t.Fatalf("second credential = %q, %v", key, err)
	}
}
