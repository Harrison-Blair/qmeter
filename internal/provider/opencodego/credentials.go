package opencodego

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Harrison-Blair/qmeter/internal/lib/credstore"
)

// storeEntryKey is the key under which OpenCode records the Zen "go" plan
// credential in its auth store. It is also this provider's ID.
const storeEntryKey = "opencode-go"

// windowsGOOS is the one platform that spells the home directory
// %USERPROFILE% rather than $HOME.
const windowsGOOS = "windows"

// defaultCredentialPath returns the OS-default location of OpenCode's auth
// store for the platform this binary was built for.
func defaultCredentialPath() string {
	return credentialPathFor(runtime.GOOS)
}

// credentialPathFor returns OpenCode's auth store for goos. It is the only
// place in this package a default path is decided, and the platform is passed
// in (runtime.GOOS in production) so every platform's answer is testable from
// any host.
//
// OpenCode keeps the store in the same XDG-shaped layout on every platform —
// it is not %APPDATA%-relative on Windows — so only the home directory itself
// differs:
//
//	Windows        %USERPROFILE%\.local\share\opencode\auth.json
//	macOS, Linux   $HOME/.local/share/opencode/auth.json
//
// filepath.Join supplies the separator, so the Windows form above is what a
// Windows build produces, not a string spelled out here.
//
// When the home directory cannot be determined the path is returned empty,
// which makes the loader report the store as absent — i.e. "not logged in" —
// rather than reading something unintended.
func credentialPathFor(goos string) string {
	home, err := homeDirFor(goos)
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "opencode", "auth.json")
}

// homeDirFor returns the home directory goos keeps credentials under:
// %USERPROFILE% on Windows, $HOME everywhere else. os.UserHomeDir already
// reads exactly those variables per platform, so this only makes the choice
// explicit and injectable — it is what lets a Linux CI runner assert the
// Windows path — and os.UserHomeDir remains the fallback (and the source of
// the "home directory unknown" error) whenever the variable is unset.
func homeDirFor(goos string) (string, error) {
	env := "HOME"
	if goos == windowsGOOS {
		env = "USERPROFILE"
	}
	if dir := os.Getenv(env); dir != "" {
		return dir, nil
	}
	return os.UserHomeDir()
}

// storeEntry is one provider's record in OpenCode's auth store. Only the API
// key matters here; "type" and every other field are ignored.
type storeEntry struct {
	Type string `json:"type"`
	Key  string `json:"key"`
}

// loadKey reads the OpenCode Go API key out of the auth store at path.
//
// It follows the credstore loader contract:
//   - the file is absent: os.ReadFile's error is returned unchanged, so
//     credstore maps os.ErrNotExist to provider.ErrNotLoggedIn;
//   - the file parses but has no usable "opencode-go" key: credstore.ErrNotFound,
//     which credstore maps to the same "not logged in" hint;
//   - anything else (unreadable, not JSON) is a descriptive error that reads
//     well after credstore's "credential store: " prefix.
//
// The store is decoded one entry at a time so that an unrelated provider's
// entry — OpenCode keeps them all in this one file, with differing shapes —
// can never make the whole file unreadable.
func loadKey(path string) (string, error) {
	if path == "" {
		return "", credstore.ErrNotFound
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil {
		return "", fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	raw, ok := entries[storeEntryKey]
	if !ok {
		return "", credstore.ErrNotFound
	}
	var entry storeEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return "", fmt.Errorf("%s has an unreadable %q entry: %w", path, storeEntryKey, err)
	}
	// Trim before both the emptiness check and the return: a key with a
	// trailing newline would otherwise pass Detect and then be rejected by
	// net/http as an invalid Authorization header value, so the request would
	// never leave the process.
	key := strings.TrimSpace(entry.Key)
	if key == "" {
		// The entry exists but carries no key — the user is logged out of
		// the Go plan even though the file is there.
		return "", credstore.ErrNotFound
	}
	return key, nil
}

// credential resolves the API key for one call, applying the env override
// first and OpenCode's auth store second. The credential is a plain string:
// unlike Claude, the store carries no plan name, so there is nothing else to
// keep.
func (p *Provider) credential(ctx context.Context) (string, credstore.Source, error) {
	load := func(context.Context) (string, error) {
		return loadKey(p.credentialPath)
	}
	// The override is used verbatim, exactly as credstore documents.
	fromEnv := func(v string) (string, error) { return v, nil }

	return credstore.Resolve(ctx, envVar, tool, load, fromEnv)
}
