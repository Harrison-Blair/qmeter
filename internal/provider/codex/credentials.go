package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Harrison-Blair/qmeter/internal/lib/credstore"
)

const (
	// tokenEnvVar overrides the store with a bearer token used verbatim.
	tokenEnvVar = "QMETER_CODEX_TOKEN"

	// accountIDEnvVar supplies the ChatGPT-Account-Id header alongside
	// tokenEnvVar. It is only read when the token override is in play: the
	// local store carries its own account id.
	accountIDEnvVar = "QMETER_CODEX_ACCOUNT_ID"
)

// credential is everything qmeter needs to call the Codex usage endpoint.
// Plan is only ever populated from the local store's id_token claims — an
// env override is a bare token, so its plan name can only come from the
// response.
type credential struct {
	AccessToken string
	AccountID   string
	Plan        string
}

// authFile mirrors the parts of ~/.codex/auth.json qmeter reads. Unknown
// fields are ignored, and qmeter never writes this file.
type authFile struct {
	AuthMode     string      `json:"auth_mode"`
	OpenAIAPIKey string      `json:"OPENAI_API_KEY"`
	LastRefresh  string      `json:"last_refresh"`
	Tokens       *authTokens `json:"tokens"`
}

// authTokens is the "tokens" object of ~/.codex/auth.json.
type authTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	AccountID    string `json:"account_id"`
}

// windowsGOOS is the one platform that spells the home directory
// %USERPROFILE% rather than $HOME.
const windowsGOOS = "windows"

// defaultCredentialPath returns the OS default location of the Codex auth
// store for the platform this binary was built for.
func defaultCredentialPath() (string, error) {
	return credentialPathFor(runtime.GOOS)
}

// credentialPathFor returns the Codex auth store's default location for goos.
// It is the only place in this package a default path is decided, and the
// platform is passed in (runtime.GOOS in production) so every platform's
// answer is testable from any host.
//
// The layout is the same everywhere — auth.json in .codex under the home
// directory — so only the home directory itself is per-OS:
//
//	Windows        %USERPROFILE%\.codex\auth.json
//	macOS, Linux   $HOME/.codex/auth.json
//
// filepath.Join supplies the separator, so the Windows form above is what a
// Windows build produces, not a string spelled out here.
func credentialPathFor(goos string) (string, error) {
	home, err := homeDirFor(goos)
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, ".codex", "auth.json"), nil
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

// resolveCredential runs the credential lookup order: the QMETER_CODEX_TOKEN
// override first, then ~/.codex/auth.json.
func (p *Provider) resolveCredential(ctx context.Context) (credential, error) {
	cred, _, err := credstore.Resolve(ctx, tokenEnvVar, tool, p.loadStore, p.fromEnv)
	return cred, err
}

// fromEnv turns the QMETER_CODEX_TOKEN value into a credential. The value is
// used verbatim as the bearer token. The account id is attached only when
// QMETER_CODEX_ACCOUNT_ID is also set — the store's account id belongs to the
// store's token, not to an overriding one, and sending a mismatched
// ChatGPT-Account-Id is worse than sending none.
//
// It never fails: a bad token surfaces as the endpoint's own 401, and an
// error here would be reported against QMETER_CODEX_TOKEN even when the real
// problem is the account id variable.
func (p *Provider) fromEnv(token string) (credential, error) {
	cred := credential{AccessToken: token}
	if id, ok := os.LookupEnv(accountIDEnvVar); ok && id != "" {
		cred.AccountID = id
	}
	return cred, nil
}

// loadStore reads ~/.codex/auth.json.
//
// The error vocabulary is credstore's: an absent file returns os.ReadFile's
// error unchanged (credstore maps it to provider.ErrNotLoggedIn), a store
// that parses but holds no usable ChatGPT token returns credstore.ErrNotFound,
// and anything else is returned as a descriptive error that credstore passes
// through with a "credential store: " prefix.
//
// Expiry is deliberately not judged here: auth.json records only
// last_refresh, not the access token's lifetime (measured at 240h), so an
// expired token is discovered as the endpoint's 401, which httpx maps to
// provider.ErrTokenExpired.
func (p *Provider) loadStore(_ context.Context) (credential, error) {
	path, err := p.storePath()
	if err != nil {
		return credential{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Genuinely not logged in: let credstore say so.
			return credential{}, err
		}
		return credential{}, keyringHint(path, err)
	}

	var file authFile
	if err := json.Unmarshal(data, &file); err != nil {
		return credential{}, keyringHint(path, err)
	}

	if isAPIKeyMode(file) {
		return credential{}, errAPIKeyMode
	}
	if file.Tokens == nil || file.Tokens.AccessToken == "" {
		return credential{}, credstore.ErrNotFound
	}

	cred := credential{
		AccessToken: file.Tokens.AccessToken,
		AccountID:   file.Tokens.AccountID,
	}
	// The id_token carries the plan name and, for accounts whose store
	// predates the account_id field, the account id. It is routinely
	// expired by the time usage is read, which is fine: decodeIDToken
	// ignores exp. An undecodable id_token costs only the plan name, so it
	// is never fatal.
	if claims, err := decodeIDToken(file.Tokens.IDToken); err == nil {
		cred.Plan = claims.PlanType
		if cred.AccountID == "" {
			cred.AccountID = claims.AccountID
		}
	}
	return cred, nil
}

// storePath is the configured store path, or the OS default when none was
// injected.
func (p *Provider) storePath() (string, error) {
	if p.credentialPath != "" {
		return p.credentialPath, nil
	}
	return defaultCredentialPath()
}

// errAPIKeyMode reports a store signed in with an OpenAI API key rather than
// a ChatGPT account. The ChatGPT usage endpoint has nothing to say about such
// an account, so this is an unsupported configuration, not a login problem
// and not a parse failure.
//
// It is a sentinel because both callers match it: Detect still counts such a
// store as detected (it exists and parses), and Fetch reports this sentence
// verbatim, without the "credential store: " prefix credstore wraps it in.
var errAPIKeyMode = errors.New(
	`signed in with an API key (auth_mode "apikey"); Codex usage limits exist only for ChatGPT sign-in`)

// keyringHint wraps a store that exists but cannot be used, pointing at the
// most likely cause: newer Codex versions may keep tokens in an OS
// keyring/encrypted store, which qmeter does not read. Worded to read after
// credstore's "credential store: " prefix.
func keyringHint(path string, err error) error {
	return fmt.Errorf(
		"no usable %s: %w; newer Codex versions may keep tokens in an OS keyring instead, which qmeter cannot read",
		path, err)
}

// isAPIKeyMode reports whether the store holds an OpenAI API key instead of
// ChatGPT tokens, either declared outright by auth_mode or implied by an API
// key sitting next to empty tokens.
func isAPIKeyMode(file authFile) bool {
	if strings.EqualFold(strings.TrimSpace(file.AuthMode), "apikey") {
		return true
	}
	hasTokens := file.Tokens != nil && file.Tokens.AccessToken != ""
	return !hasTokens && file.OpenAIAPIKey != ""
}
