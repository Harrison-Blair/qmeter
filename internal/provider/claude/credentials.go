package claude

// Credential sources for the Claude provider.
//
// Two platform seams live in this file, deliberately kept as separate
// functions so later work units never edit the same one:
//
//   - defaultCredentialPath is the FILE-PATH seam (U13's alone): it turns the
//     current OS into the default location of Claude's credential file.
//     Nothing else in this package decides a path, so adding the exact
//     Windows path means editing this function and nothing else.
//   - Provider.keychain, a keychainLoader, is the KEYCHAIN seam (U6's alone):
//     it returns the raw credential JSON from the macOS Keychain
//     (`/usr/bin/security find-generic-password -s "Claude Code-credentials"
//     -w`, via internal/lib/subprocess). On this unit it is noKeychain, which
//     always fails, so loadStore falls back to the file on every platform.
//
// Both seams yield raw JSON bytes and hand them to parseStore, so the store's
// JSON shape and the expiry rule stay in exactly one place no matter where
// the bytes came from.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/lib/credstore"
	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// credential is one usable Claude credential. Plan is the plan name, which
// only the local store can supply — the env override is a bare token.
type credential struct {
	AccessToken string
	Plan        string
}

// storeFile mirrors the part of ~/.claude/.credentials.json that qmeter
// reads. Unknown fields are ignored; qmeter never writes this file.
type storeFile struct {
	ClaudeAiOauth struct {
		AccessToken string `json:"accessToken"`
		// RefreshToken is read only to document the store's shape: qmeter is
		// permanently read-only and never refreshes.
		RefreshToken     string `json:"refreshToken"`
		ExpiresAt        int64  `json:"expiresAt"` // epoch milliseconds
		SubscriptionType string `json:"subscriptionType"`
	} `json:"claudeAiOauth"`
}

// keychainLoader returns the raw credential JSON from an OS keychain. It
// reports an error when this platform has no keychain source or the lookup
// failed; loadStore then falls back to the credential file.
type keychainLoader func(ctx context.Context) ([]byte, error)

// errNoKeychain is what noKeychain reports: there is no keychain credential
// source on this unit, on any platform.
var errNoKeychain = errors.New("no keychain credential source")

// noKeychain is the default keychainLoader and the U6 seam: it always fails,
// so every platform reads the credential file. U6 replaces it with the macOS
// Keychain lookup, gated on runtime.GOOS, keeping the same fall-back-to-file
// behavior on error.
func noKeychain(context.Context) ([]byte, error) { return nil, errNoKeychain }

// defaultCredentialPath returns the OS-default location of Claude's
// credential file. This is the U13 seam: it is the only place a default path
// is decided. Linux and macOS are $HOME-relative; the exact Windows path is
// U13's to add.
func defaultCredentialPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".claude", ".credentials.json"), nil
}

// credentialPath is the path this Provider reads: the injected override when
// set (tests, and anyone pointing qmeter at another profile), otherwise the
// OS default.
func (p *Provider) credentialPath() (string, error) {
	if p.credPath != "" {
		return p.credPath, nil
	}
	return defaultCredentialPath()
}

// resolve runs the credential lookup order: QMETER_CLAUDE_TOKEN first, then
// the local store. The Source tells the caller which step won — an env
// override carries no plan name.
func (p *Provider) resolve(ctx context.Context) (credential, credstore.Source, error) {
	return credstore.Resolve(ctx, envVar, toolName, p.loadStore, fromEnv)
}

// fromEnv turns the QMETER_CLAUDE_TOKEN value into a credential. The value is
// used verbatim as the bearer token — whitespace included, since only the
// endpoint can say whether a token is good — and Plan stays empty because no
// local store was consulted.
func fromEnv(v string) (credential, error) {
	return credential{AccessToken: v}, nil
}

// loadStore is the step-2 loader credstore calls: the keychain seam first,
// then the credential file. Any keychain error (including "no keychain
// source" on this unit) falls back to the file, which is the behavior U6
// relies on.
//
// Error vocabulary, per credstore's contract: os.ReadFile's error is returned
// unchanged when the file is absent (credstore maps os.ErrNotExist to
// provider.ErrNotLoggedIn), credstore.ErrNotFound when the store parses but
// holds no usable token, and provider.ErrTokenExpired by value when the token
// is expired. It never returns a zero credential with a nil error.
func (p *Provider) loadStore(ctx context.Context) (credential, error) {
	if data, err := p.keychain(ctx); err == nil {
		return p.parseStore(data, "keychain")
	}
	path, err := p.credentialPath()
	if err != nil {
		return credential{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return credential{}, err
	}
	return p.parseStore(data, path)
}

// parseStore decodes a credential store and applies the expiry rule. source
// names where the bytes came from (a file path, or "keychain") and appears in
// the parse error, which the user sees after credstore's "credential store: "
// prefix.
func (p *Provider) parseStore(data []byte, source string) (credential, error) {
	var sf storeFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return credential{}, fmt.Errorf("parse %s: %w", source, err)
	}
	oauth := sf.ClaudeAiOauth
	if strings.TrimSpace(oauth.AccessToken) == "" {
		return credential{}, fmt.Errorf("%s has no claudeAiOauth.accessToken: %w", source, credstore.ErrNotFound)
	}
	// expiresAt is epoch milliseconds and the credential is expired once now
	// has caught up with it (expiresAt <= now). A store with no expiresAt
	// says nothing about expiry, so the token is used and a 401 from the
	// endpoint — which httpx types as ErrTokenExpired — is the authority.
	if oauth.ExpiresAt > 0 && !p.now().Before(time.UnixMilli(oauth.ExpiresAt)) {
		return credential{}, provider.ErrTokenExpired{Tool: toolName}
	}
	return credential{AccessToken: oauth.AccessToken, Plan: oauth.SubscriptionType}, nil
}
