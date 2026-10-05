package usage

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/provider/claude"
	"github.com/Harrison-Blair/qmeter/internal/provider/codex"
	"github.com/Harrison-Blair/qmeter/internal/provider/opencodego"
)

func TestPiFallbackProducesOneSubscriptionPerProvider(t *testing.T) {
	for _, name := range []string{"native", "Pi fallback"} {
		t.Run(name, func(t *testing.T) {
			for _, variable := range []string{"QMETER_CLAUDE_TOKEN", "QMETER_CODEX_TOKEN", "QMETER_CODEX_ACCOUNT_ID", "QMETER_OPENCODE_GO_KEY"} {
				t.Setenv(variable, "")
			}
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			t.Setenv("USERPROFILE", dir)
			t.Setenv("PI_CODING_AGENT_DIR", t.TempDir())
			write := func(name, content string) string {
				path := filepath.Join(dir, name)
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
				return path
			}
			pi := write("pi.json", `{"anthropic":{"type":"oauth","access":"pi-claude"},"openai-codex":{"type":"oauth","access":"pi-codex","accountId":"pi-account"},"opencode-go":{"type":"api_key","key":"pi-go"}}`)
			nativeClaude := write("claude.json", `{"claudeAiOauth":{"accessToken":"native-claude","subscriptionType":"max"}}`)
			nativeCodex := write("codex.json", `{"tokens":{"access_token":"native-codex","account_id":"native-account"}}`)
			nativeGo := write("go.json", `{"opencode-go":{"type":"api","key":"native-go"}}`)
			requests := make(chan string, 8)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				auth := r.Header.Get("Authorization")
				requests <- auth
				if name == "Pi fallback" && strings.HasPrefix(auth, "Bearer native-") {
					w.WriteHeader(401)
					return
				}
				switch r.URL.Path {
				case "/api/oauth/usage":
					fmt.Fprint(w, `{"five_hour":{"utilization":25},"extra_usage":{"is_enabled":true,"utilization":20}}`)
				case "/backend-api/wham/usage":
					fmt.Fprint(w, `{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":25,"limit_window_seconds":18000}},"credits":{"has_credits":true,"unlimited":false,"balance":"10"}}`)
				case "/zen/go/v1/usage":
					fmt.Fprint(w, `{"usage":{"rolling":{"status":"ok","percent":25}}}`)
				default:
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			providers := []provider.Provider{
				claude.New(claude.WithCredentialPath(nativeClaude), claude.WithPiCredentialPath(pi), claude.WithKeychainRunner(nil), claude.WithBaseURL(server.URL)),
				codex.New(codex.WithCredentialPath(nativeCodex), codex.WithPiCredentialPath(pi), codex.WithBaseURL(server.URL)),
				opencodego.New(opencodego.WithCredentialPath(nativeGo), opencodego.WithPiCredentialPath(pi), opencodego.WithBaseURL(server.URL)),
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			result := Run(ctx, providers, "")
			if len(result.Errors) != 0 || len(result.Undetected) != 0 {
				t.Fatalf("result errors = %+v, %+v", result.Errors, result.Undetected)
			}
			if len(result.Windows) != 3 || len(result.Balances) != 2 {
				t.Fatalf("usage duplicated or missing: windows=%+v balances=%+v", result.Windows, result.Balances)
			}
			for i, id := range []string{"claude", "codex", "opencode-go"} {
				if result.Windows[i].Provider != id || result.Windows[i].RemainingPercent != 75 {
					t.Fatalf("window %d = %+v", i, result.Windows[i])
				}
			}
			wantRequests := 3
			if name == "Pi fallback" {
				wantRequests = 6
			}
			if len(requests) != wantRequests {
				t.Fatalf("requests = %d, want %d", len(requests), wantRequests)
			}
			var out bytes.Buffer
			if err := RenderJSON(&out, result); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), `"provider":"pi"`) || strings.Contains(out.String(), "native-") || strings.Contains(out.String(), "pi-account") {
				t.Fatalf("unexpected source/account in JSON: %s", out.String())
			}
		})
	}
}
