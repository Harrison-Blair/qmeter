package cursor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Harrison-Blair/qmeter/internal/lib/credstore"
)

const (
	// envToken is the per-provider environment variable override, step 1 of
	// the credential lookup order. Its value must itself be a JWT: route A
	// identifies the account by the user id in the token's sub claim, so an
	// opaque token could not be used even though it is a credential.
	envToken = "QMETER_CURSOR_TOKEN"

	// toolName is the CLI name used in every "open <tool>" / "run <tool>"
	// hint. It is the Cursor CLI's binary name, not the provider id.
	toolName = "cursor-agent"
)

// credential is what this provider needs to build a route A request: the
// access token and the user id decoded out of it.
type credential struct {
	AccessToken string
	UserID      string
}

// defaultCredentialPath returns the Cursor CLI store's default location.
//
// Only the CLI store (written by cursor-agent) is read. The desktop app's
// SQLite state.vscdb fallback is a separate, later unit; nothing here reaches
// for it.
//
// U13 owns this function: it is the single seam where per-OS paths get wired
// (Windows is %USERPROFILE%\.config\cursor\auth.json). Nothing else in this
// package hardcodes a path.
func defaultCredentialPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".config", "cursor", "auth.json"), nil
}

// credentialPath is the store path this provider reads: the injected one in
// tests, otherwise the OS default.
func (p *Provider) credentialPath() (string, error) {
	if p.credPath != "" {
		return p.credPath, nil
	}
	return defaultCredentialPath()
}

// credential runs the credential lookup order: the QMETER_CURSOR_TOKEN
// override first, then the CLI store. qmeter only ever reads.
func (p *Provider) credential(ctx context.Context) (credential, credstore.Source, error) {
	return credstore.Resolve(ctx, envToken, toolName, p.loadStore, credentialFromEnv)
}

// credentialFromEnv turns the environment override into a credential. The
// override has to be a JWT, since the request cookie needs the user id from
// its sub claim; anything else is a clear error rather than a silent failure
// later. credstore prefixes the message with the variable's name.
func credentialFromEnv(v string) (credential, error) {
	id, err := userIDFromJWT(v)
	if err != nil {
		return credential{}, fmt.Errorf("must be a JWT access token, whose sub claim supplies the Cursor user id: %w", err)
	}
	return credential{AccessToken: v, UserID: id}, nil
}

// loadStore reads the Cursor CLI store, step 2 of the lookup order. Its error
// vocabulary is credstore's contract: os.ReadFile's error unchanged when the
// file is absent (credstore turns that into provider.ErrNotLoggedIn),
// credstore.ErrNotFound when the file parses but holds no access token, and a
// descriptive error — surfaced as "credential store: <msg>" — when the file
// is there but unusable.
//
// The context is unused: this loader only touches the filesystem.
func (p *Provider) loadStore(context.Context) (credential, error) {
	path, err := p.credentialPath()
	if err != nil {
		return credential{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return credential{}, err
	}
	var store struct {
		AccessToken string `json:"accessToken"`
		// RefreshToken is read only to document the file's shape; qmeter is
		// read-only and never refreshes.
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.Unmarshal(data, &store); err != nil {
		return credential{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if store.AccessToken == "" {
		return credential{}, fmt.Errorf("no accessToken in %s: %w", path, credstore.ErrNotFound)
	}
	id, err := userIDFromJWT(store.AccessToken)
	if err != nil {
		return credential{}, fmt.Errorf("accessToken in %s: %w", path, err)
	}
	return credential{AccessToken: store.AccessToken, UserID: id}, nil
}
