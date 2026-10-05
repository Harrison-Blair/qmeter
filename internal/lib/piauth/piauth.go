// Package piauth reads compatible subscription credentials from Pi without
// refreshing credentials, executing configured commands, or changing its store.
package piauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/lib/credstore"
	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// Credential holds the fields needed by qmeter's existing provider adapters.
type Credential struct{ Key, Access, AccountID string }

// DefaultPath follows Pi's agent directory override and home directory default.
// An unknown home directory returns no default, rather than a relative path.
func DefaultPath() string {
	dir := os.Getenv("PI_CODING_AGENT_DIR")
	home, err := os.UserHomeDir()
	if dir == "" {
		if err != nil || home == "" {
			return ""
		}
		dir = filepath.Join(home, ".pi", "agent")
	}
	if dir == "~" || strings.HasPrefix(dir, "~/") || (runtime.GOOS == "windows" && strings.HasPrefix(dir, `~\`)) {
		if err != nil || home == "" {
			return ""
		}
		if dir == "~" {
			dir = home
		} else {
			dir = filepath.Join(home, dir[2:])
		}
	}
	path, err := filepath.Abs(filepath.Join(dir, "auth.json"))
	if err != nil {
		return ""
	}
	return path
}

// IsMissing identifies absent stores and absent or incompatible credentials.
func IsMissing(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, credstore.ErrNotFound)
}

// Load rereads only the requested entry, ignoring unrelated credential shapes.
func Load(path, entry string, now time.Time) (Credential, error) {
	if path == "" {
		return Credential{}, credstore.ErrNotFound
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Credential{}, fmt.Errorf("Pi credential store: %w", err)
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil || entries == nil {
		return Credential{}, fmt.Errorf("Pi credential store %q: invalid auth.json object", path)
	}
	raw, ok := entries[entry]
	if !ok || string(raw) == "null" {
		return Credential{}, credstore.ErrNotFound
	}
	var kind struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &kind); err != nil {
		return Credential{}, fmt.Errorf("Pi credential store %q: invalid credential entry", path)
	}
	switch {
	case entry == "opencode-go" && kind.Type == "api_key":
		var value struct {
			Key string            `json:"key"`
			Env map[string]string `json:"env"`
		}
		if err := json.Unmarshal(raw, &value); err != nil {
			return Credential{}, fmt.Errorf("Pi credential store %q: invalid API key entry", path)
		}
		if strings.HasPrefix(value.Key, "!") {
			return Credential{}, errors.New("Pi command-based API keys are unsupported; set QMETER_OPENCODE_GO_KEY instead")
		}
		key, ok := interpolate(value.Key, value.Env)
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return Credential{}, credstore.ErrNotFound
		}
		return Credential{Key: key}, nil
	case (entry == "anthropic" || entry == "openai-codex") && kind.Type == "oauth":
		var value struct {
			Access    string   `json:"access"`
			AccountID string   `json:"accountId"`
			Expires   *float64 `json:"expires"`
		}
		if err := json.Unmarshal(raw, &value); err != nil {
			return Credential{}, fmt.Errorf("Pi credential store %q: invalid OAuth entry", path)
		}
		if strings.TrimSpace(value.Access) == "" {
			return Credential{}, credstore.ErrNotFound
		}
		if value.Expires != nil && *value.Expires <= float64(now.UnixMilli()) {
			return Credential{}, provider.ErrTokenExpired{Tool: "pi"}
		}
		return Credential{Access: value.Access, AccountID: value.AccountID}, nil
	default:
		return Credential{}, credstore.ErrNotFound
	}
}

var variablePrefix = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*`)
var variableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// interpolate follows Pi's template rules, including leaving invalid references
// literal and treating any unresolved reference as an unavailable credential.
func interpolate(config string, env map[string]string) (string, bool) {
	var out strings.Builder
	for i := 0; i < len(config); {
		if config[i] != '$' || i+1 == len(config) {
			out.WriteByte(config[i])
			i++
			continue
		}
		next := config[i+1]
		if next == '$' || next == '!' {
			out.WriteByte(next)
			i += 2
			continue
		}
		name := ""
		end := i + 1
		if next == '{' {
			close := strings.IndexByte(config[i+2:], '}')
			if close < 0 {
				out.WriteByte('$')
				i++
				continue
			}
			end = i + 2 + close + 1
			name = config[i+2 : end-1]
			if !variableName.MatchString(name) {
				out.WriteString(config[i:end])
				i = end
				continue
			}
		} else {
			name = variablePrefix.FindString(config[i+1:])
			if name == "" {
				out.WriteByte('$')
				i++
				continue
			}
			end = i + 1 + len(name)
		}
		value := env[name]
		if value == "" {
			value = os.Getenv(name)
		}
		if value == "" {
			return "", false
		}
		out.WriteString(value)
		i = end
	}
	return out.String(), true
}
