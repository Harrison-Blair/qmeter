package piauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/lib/credstore"
	"github.com/Harrison-Blair/qmeter/internal/provider"
)

func store(t *testing.T, data string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(p, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestLoadKey(t *testing.T) {
	t.Setenv("PI_TEST_KEY", "process")
	t.Setenv("PI_TEST_MISSING", "")
	for _, tt := range []struct {
		name, key string
		env       map[string]string
		want      string
	}{
		{"literal", "literal", nil, "literal"},
		{"process", "$PI_TEST_KEY", nil, "process"},
		{"braced", "prefix-${PI_TEST_KEY}-suffix", nil, "prefix-process-suffix"},
		{"entry env", "$PI_TEST_KEY", map[string]string{"PI_TEST_KEY": "entry"}, "entry"},
		{"empty entry env", "$PI_TEST_KEY", map[string]string{"PI_TEST_KEY": ""}, "process"},
		{"escape", "$!literal-$$PI_TEST_KEY", nil, "!literal-$PI_TEST_KEY"},
		{"invalid variable", "${BAD-NAME}-$9-${OPEN", nil, "${BAD-NAME}-$9-${OPEN"},
		{"missing", "prefix-$PI_TEST_MISSING", nil, ""},
		{"empty", "", nil, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"opencode-go": map[string]any{"type": "api_key", "key": tt.key, "env": tt.env}, "other": 42})
			got, err := Load(store(t, string(body)), "opencode-go", time.Now())
			if tt.want == "" {
				if !IsMissing(err) {
					t.Fatalf("want missing, got %v", err)
				}
				return
			}
			if err != nil || got.Key != tt.want {
				t.Fatalf("got %+v %v, want %q", got, err, tt.want)
			}
		})
	}
}
func TestLoadOAuth(t *testing.T) {
	now := time.UnixMilli(1234567890000)
	for _, entry := range []string{"anthropic", "openai-codex"} {
		for _, offset := range []int64{-1, 0, 1} {
			t.Run(fmt.Sprintf("%s/%d", entry, offset), func(t *testing.T) {
				data := fmt.Sprintf(`{"%s":{"type":"oauth","access":"access","refresh":"unused","accountId":"acct","expires":%d}}`, entry, now.UnixMilli()+offset)
				got, err := Load(store(t, data), entry, now)
				if offset <= 0 {
					var expired provider.ErrTokenExpired
					if !errors.As(err, &expired) || expired.Tool != "pi" {
						t.Fatalf("expiry = %v", err)
					}
				} else if err != nil || got.Access != "access" || got.AccountID != "acct" {
					t.Fatalf("got %+v %v", got, err)
				}
			})
		}
	}
	got, err := Load(store(t, `{"anthropic":{"type":"oauth","access":"access"}}`), "anthropic", now)
	if err != nil || got.Access != "access" {
		t.Fatalf("unknown expiry: %+v %v", got, err)
	}
}
func TestLoadMissingAndMalformed(t *testing.T) {
	for _, body := range []string{`{}`, `{"anthropic":{"type":"api_key","key":"secret"}}`, `{"opencode-go":{"type":"oauth","access":"secret"}}`, `{"anthropic":{"type":"oauth","access":""}}`, `{"anthropic":{"type":"unknown"}}`, `{"anthropic":null}`} {
		entry := "anthropic"
		if strings.Contains(body, "opencode-go") {
			entry = "opencode-go"
		}
		_, err := Load(store(t, body), entry, time.Now())
		if !IsMissing(err) {
			t.Errorf("%s: want missing, got %v", body, err)
		}
	}
	for _, body := range []string{`{`, `[]`, `null`, `{"anthropic":42}`, `{"anthropic":{"type":"oauth","access":42}}`, `{"anthropic":{"type":"oauth","access":"secret","expires":"secret"}}`} {
		_, err := Load(store(t, body), "anthropic", time.Now())
		if err == nil || IsMissing(err) || !strings.Contains(err.Error(), "Pi") || strings.Contains(err.Error(), "secret") {
			t.Errorf("malformed %s: %v", body, err)
		}
	}
	_, err := Load(filepath.Join(t.TempDir(), "missing"), "anthropic", time.Now())
	if !IsMissing(err) {
		t.Fatalf("missing file: %v", err)
	}
	_, err = Load(t.TempDir(), "anthropic", time.Now())
	if err == nil || IsMissing(err) || !strings.Contains(err.Error(), "Pi") {
		t.Fatalf("unreadable: %v", err)
	}
	if !IsMissing(fmt.Errorf("wrap: %w", credstore.ErrNotFound)) || IsMissing(errors.New("other")) {
		t.Fatal("missing classification")
	}
}
func TestCommandRejected(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "executed")
	body, _ := json.Marshal(map[string]any{"opencode-go": map[string]any{"type": "api_key", "key": "!touch " + marker}})
	_, err := Load(store(t, string(body)), "opencode-go", time.Now())
	if err == nil || !strings.Contains(err.Error(), "QMETER_OPENCODE_GO_KEY") || strings.Contains(err.Error(), marker) {
		t.Fatalf("command error: %v", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("command executed")
	}
}
func TestDefaultPath(t *testing.T) {
	home := t.TempDir()
	name := "HOME"
	if runtime.GOOS == "windows" {
		name = "USERPROFILE"
	}
	t.Setenv(name, home)
	t.Setenv("PI_CODING_AGENT_DIR", "")
	if got, want := DefaultPath(), filepath.Join(home, ".pi", "agent", "auth.json"); got != want {
		t.Fatalf("default %q want %q", got, want)
	}
	for _, dir := range []string{home, "~/custom", "~", "relative"} {
		t.Setenv("PI_CODING_AGENT_DIR", dir)
		want := dir
		if dir == "~" {
			want = home
		}
		if dir == "~/custom" {
			want = filepath.Join(home, "custom")
		}
		want = filepath.Join(want, "auth.json")
		want, _ = filepath.Abs(want)
		if got := DefaultPath(); got != want {
			t.Errorf("%q got %q want %q", dir, got, want)
		}
	}
	t.Setenv(name, "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	if got := DefaultPath(); got != "" {
		t.Errorf("unknown home %q", got)
	}
	t.Setenv("PI_CODING_AGENT_DIR", home)
	if got := DefaultPath(); got != filepath.Join(home, "auth.json") {
		t.Errorf("absolute without home %q", got)
	}
}

func TestWhitespaceCredentials(t *testing.T) {
	for _, entry := range []string{"opencode-go", "anthropic", "openai-codex"} {
		t.Run(entry, func(t *testing.T) {
			kind, field := "oauth", "access"
			if entry == "opencode-go" {
				kind, field = "api_key", "key"
			}
			body, _ := json.Marshal(map[string]any{entry: map[string]any{"type": kind, field: " \n\t "}})
			_, err := Load(store(t, string(body)), entry, time.Now())
			if !IsMissing(err) {
				t.Fatalf("whitespace credential should be missing: %v", err)
			}
		})
	}
}
func TestKeyTrimAfterInterpolation(t *testing.T) {
	t.Setenv("PI_TRIM_KEY", " \tkey\n")
	for _, key := range []string{" \tkey\n", "$PI_TRIM_KEY"} {
		body, _ := json.Marshal(map[string]any{"opencode-go": map[string]any{"type": "api_key", "key": key}})
		got, err := Load(store(t, string(body)), "opencode-go", time.Now())
		if err != nil || got.Key != "key" {
			t.Fatalf("got %+v %v", got, err)
		}
	}
}
func TestOAuthAccessRemainsLiteral(t *testing.T) {
	t.Setenv("PI_OAUTH_LITERAL", "expanded")
	access := " $PI_OAUTH_LITERAL\n"
	body, _ := json.Marshal(map[string]any{"anthropic": map[string]any{"type": "oauth", "access": access}})
	got, err := Load(store(t, string(body)), "anthropic", time.Now())
	if err != nil || got.Access != access {
		t.Fatalf("access changed: %+v %v", got, err)
	}
}
func TestExpandedAndWhitespacePrefixedCommandsRemainKeys(t *testing.T) {
	t.Setenv("PI_BANG_KEY", "!literal")
	for _, key := range []string{"$PI_BANG_KEY", "$!literal", " !literal "} {
		body, _ := json.Marshal(map[string]any{"opencode-go": map[string]any{"type": "api_key", "key": key}})
		got, err := Load(store(t, string(body)), "opencode-go", time.Now())
		if err != nil || got.Key != "!literal" {
			t.Fatalf("key treated as command: %+v %v", got, err)
		}
	}
}
func TestMalformedErrorsIdentifyPiPath(t *testing.T) {
	for _, body := range []string{`{`, `{"anthropic":42}`, `{"anthropic":{"type":"oauth","access":"secret","expires":"secret"}}`, `{"opencode-go":{"type":"api_key","key":42}}`} {
		entry := "anthropic"
		if strings.Contains(body, "opencode-go") {
			entry = "opencode-go"
		}
		path := store(t, body)
		_, err := Load(path, entry, time.Now())
		if err == nil || !strings.Contains(err.Error(), path) || strings.Contains(err.Error(), "secret") {
			t.Fatalf("diagnostic = %v, want path without secret", err)
		}
	}
}

func TestDefaultPathBackslashTildeIsWindowsOnly(t *testing.T) {
	home := t.TempDir()
	homeVar := "HOME"
	if runtime.GOOS == "windows" {
		homeVar = "USERPROFILE"
	}
	t.Setenv(homeVar, home)
	t.Setenv("PI_CODING_AGENT_DIR", `~\custom`)
	want := filepath.Join(`~\custom`, "auth.json")
	if runtime.GOOS == "windows" {
		want = filepath.Join(home, "custom", "auth.json")
	}
	want, err := filepath.Abs(want)
	if err != nil {
		t.Fatal(err)
	}
	if got := DefaultPath(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
