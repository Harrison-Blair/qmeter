package codex

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

func writePi(t *testing.T, access, account string, expires int64) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", dir)
	data, err := json.Marshal(map[string]any{"openai-codex": map[string]any{"type": "oauth", "access": access, "accountId": account, "expires": expires}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPiFallback(t *testing.T) {
	for _, native := range []string{"missing", "malformed", "apikey", "valid"} {
		for _, status := range []int{200, 401, 403, 429, 500} {
			t.Run(native+http.StatusText(status), func(t *testing.T) {
				clearEnv(t)
				writePi(t, "pi-access", "pi-account", time.Now().Add(time.Hour).UnixMilli())
				path := filepath.Join(t.TempDir(), "auth.json")
				switch native {
				case "malformed":
					os.WriteFile(path, []byte("bad-json"), 0600)
				case "apikey":
					os.WriteFile(path, []byte(`{"auth_mode":"apikey"}`), 0600)
				case "valid":
					path = fixture("auth_chatgpt.json")
				}
				calls := 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.Header.Get("Authorization") == "Bearer pi-access" {
						if r.Header.Get("ChatGPT-Account-Id") != "pi-account" {
							t.Error("Pi account header missing")
						}
					} else if native == "valid" {
						if status != 200 {
							w.WriteHeader(status)
							return
						}
					} else {
						t.Error("unexpected native request")
					}
					w.Write([]byte(`{"rate_limit":{"primary_window":{"used_percent":25}}}`))
				}))
				defer srv.Close()
				p := New(WithCredentialPath(path), WithBaseURL(srv.URL))
				if ok, reason := p.Detect(context.Background()); !ok {
					t.Fatalf("Detect false: %s", reason)
				}
				usage, err := p.Fetch(context.Background())
				if native == "valid" && (status == 429 || status == 500) {
					if err == nil || calls != 1 {
						t.Fatalf("non-auth error retried: %v, calls %d", err, calls)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				want := 1
				if native == "valid" && (status == 401 || status == 403) {
					want = 2
				}
				if calls != want || len(usage.Windows) != 1 {
					t.Fatalf("calls %d want %d; usage %+v", calls, want, usage)
				}
			})
		}
	}
}

func TestPiExpiredAndOverride(t *testing.T) {
	clearEnv(t)
	writePi(t, "pi-access", "", 1)
	p := New(WithCredentialPath(filepath.Join(t.TempDir(), "missing")))
	if ok, reason := p.Detect(context.Background()); !ok {
		t.Fatalf("expired Pi not detected: %s", reason)
	}
	_, err := p.Fetch(context.Background())
	var expired provider.ErrTokenExpired
	if !errors.As(err, &expired) || expired.Tool != "pi" {
		t.Fatalf("error %v, want Pi expiry", err)
	}
	t.Setenv(tokenEnvVar, "explicit")
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(401) }))
	defer srv.Close()
	p = New(WithCredentialPath(fixture("auth_chatgpt.json")), WithBaseURL(srv.URL))
	_, err = p.Fetch(context.Background())
	if !errors.As(err, &expired) || expired.Tool != "codex" || calls != 1 {
		t.Fatalf("override fallback: %v calls %d", err, calls)
	}
}

func TestPiClaimsAndReload(t *testing.T) {
	clearEnv(t)
	token := makeIDToken(t, map[string]any{openAIAuthClaim: map[string]any{"chatgpt_account_id": "claim-account", "chatgpt_plan_type": "pro"}})
	path := writePi(t, token, "explicit-account", time.Now().Add(time.Hour).UnixMilli())
	p := New(WithCredentialPath(filepath.Join(t.TempDir(), "missing")), WithPiCredentialPath(path))
	cred, err := p.resolveCredential(context.Background())
	if err != nil || cred.AccountID != "explicit-account" || cred.Plan != "pro" {
		t.Fatalf("credential %+v, %v", cred, err)
	}
	data, _ := json.Marshal(map[string]any{"openai-codex": map[string]any{"type": "oauth", "access": token}})
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cred, err = p.resolveCredential(context.Background())
	if err != nil || cred.AccountID != "claim-account" {
		t.Fatalf("reload credential %+v, %v", cred, err)
	}
}

func TestPiRetryFailureHintAndMissingPreservesNative(t *testing.T) {
	for _, piPresent := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "present"}[piPresent], func(t *testing.T) {
			clearEnv(t)
			if piPresent {
				writePi(t, "pi-access", "", time.Now().Add(time.Hour).UnixMilli())
			}
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(403) }))
			defer srv.Close()
			p := New(WithCredentialPath(fixture("auth_chatgpt.json")), WithBaseURL(srv.URL))
			_, err := p.Fetch(context.Background())
			var expired provider.ErrTokenExpired
			wantTool, wantCalls := "codex", 1
			if piPresent {
				wantTool, wantCalls = "pi", 2
			}
			if !errors.As(err, &expired) || expired.Tool != wantTool || calls != wantCalls {
				t.Fatalf("error %v, calls %d", err, calls)
			}
		})
	}
}
