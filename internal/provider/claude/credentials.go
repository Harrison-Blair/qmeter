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
//   - Provider.keychain, a keychainLoader, is the KEYCHAIN seam (U6's
//     alone): it returns the raw credential JSON from the macOS Keychain
//     (`/usr/bin/security find-generic-password -s "Claude Code-credentials"
//     -w`, via internal/lib/subprocess). It is live from this unit on,
//     gated on GOOS, and a failed lookup still falls back to the file.
//
// Both seams yield raw JSON bytes and hand them to parseStore, so the store's
// JSON shape and the expiry rule stay in exactly one place no matter where
// the bytes came from.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/lib/credstore"
	"github.com/Harrison-Blair/qmeter/internal/lib/subprocess"
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

const (
	// keychainService is the Keychain service name Claude Code stores the
	// very same credential JSON under on macOS.
	keychainService = "Claude Code-credentials"

	// securityBinary is macOS's keychain CLI, spelled absolutely: this is a
	// credential read, so it must not be resolvable through $PATH.
	securityBinary = "/usr/bin/security"

	// keychainSource is what parse errors call the Keychain, in place of the
	// file path the file source contributes.
	keychainSource = "keychain"

	// darwinGOOS is the only platform with a Keychain to read.
	darwinGOOS = "darwin"
)

var (
	// errNoKeychain means this platform has no keychain source at all (any
	// GOOS but darwin, or no runner to reach it with). It is a miss, not a
	// failure: the file is then the only source and its own absence decides
	// whether the user is logged in.
	errNoKeychain = errors.New("no keychain credential source")

	// errKeychainEmpty means security(1) succeeded but printed nothing. That
	// is a miss too — an empty store is no store — not a credential that
	// happens to be blank.
	errKeychainEmpty = errors.New("keychain lookup returned no data")
)

// keychainLookup returns a keychainLoader that reads Claude's credential JSON
// from the macOS Keychain by running, exactly:
//
//	/usr/bin/security find-generic-password -s "Claude Code-credentials" -w
//
// goos is the platform to act as (runtime.GOOS in production, injected in
// tests): off darwin the loader never spawns anything at all, so no other
// platform pays for a command that could not work there. The subprocess is
// the reason Detect can raise a Keychain prompt; see Detect's doc comment.
//
// UNTESTED ON REAL HARDWARE: every test drives this through subprocess.Fake.
// The exact wire behavior of security(1) — its exit codes, its prompt, the
// trailing newline on -w output — is taken from its documentation and stays
// unverified until someone runs qmeter on a Mac.
func keychainLookup(r subprocess.Runner, goos string) keychainLoader {
	return func(ctx context.Context) ([]byte, error) {
		if goos != darwinGOOS || r == nil {
			return nil, errNoKeychain
		}
		return r.Run(ctx, securityBinary, "find-generic-password", "-s", keychainService, "-w")
	}
}

// noKeychain is the default keychainLoader. It keeps the name U5 gave the
// seam — claude.go's New refers to it and that file belongs to another unit —
// but it is no longer a no-op: on macOS it reads the Keychain through a real
// subprocess, and everywhere else it reports errNoKeychain so loadStore goes
// straight to the file.
func noKeychain(ctx context.Context) ([]byte, error) {
	return keychainLookup(subprocess.Real{}, runtime.GOOS)(ctx)
}

// WithKeychainRunner runs the macOS Keychain lookup through r instead of a
// real subprocess. Tests pass a subprocess.Fake; a nil r disables the
// Keychain source, leaving the credential file. The lookup still only happens
// on macOS — see withKeychain for the seam tests use to pretend otherwise.
func WithKeychainRunner(r subprocess.Runner) Option {
	return withKeychain(r, runtime.GOOS)
}

// withKeychain is the GOOS seam. It is an unexported Option rather than a
// Provider field because the Provider struct lives in claude.go, which this
// unit does not own; an Option keeps the whole Keychain source in this file.
func withKeychain(r subprocess.Runner, goos string) Option {
	return func(p *Provider) { p.keychain = keychainLookup(r, goos) }
}

// keychainMiss reports whether err means the Keychain simply holds nothing
// for us, as opposed to a lookup that failed for some other reason.
//
// The distinction decides what the user is told when the credential file is
// missing too. A miss is indistinguishable from having never logged in, so it
// may degrade to "not logged in, run claude to log in". Anything else — most
// importantly a denied or unavailable Keychain prompt — must keep its own
// error: a user who clicked Deny is logged in perfectly well, and sending
// them off to re-run `claude` would hide the real problem.
//
// The misses are: no keychain source on this platform, empty output,
// security(1) not being on disk at all, and security(1) reporting the item is
// absent. That last one is matched on the error text because the Runner
// interface deliberately hides os/exec: the real runner surfaces an
// *exec.ExitError whose message is "exit status 44", security(1)'s documented
// code for "The specified item could not be found in the keychain". Any other
// exit code is treated as a real failure, which is the safe direction to be
// wrong in.
func keychainMiss(err error) bool {
	switch {
	case errors.Is(err, errNoKeychain), errors.Is(err, errKeychainEmpty):
		return true
	case errors.Is(err, os.ErrNotExist):
		// os/exec reports a missing binary as os.ErrNotExist ("fork/exec
		// /usr/bin/security: no such file or directory"). No Keychain was
		// ever reached, so there is nothing to report to the user beyond
		// whatever the file source says. Stated explicitly rather than left
		// to leak through the %w chain into credstore's not-found mapping.
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "exit status 44") ||
		strings.Contains(msg, "could not be found")
}

// windowsGOOS is the one platform that spells the home directory
// %USERPROFILE% rather than $HOME.
const windowsGOOS = "windows"

// defaultCredentialPath returns the OS-default location of Claude's
// credential file for the platform this binary was built for.
func defaultCredentialPath() (string, error) {
	return credentialPathFor(runtime.GOOS)
}

// credentialPathFor returns Claude's default credential file for goos. This
// is the U13 seam: the only place a default path is decided, with the
// platform passed in (runtime.GOOS in production) so every platform's answer
// is testable from any host.
//
// The layout is the same everywhere — the credential file sits in .claude
// under the home directory — so only the home directory itself is per-OS:
//
//	Windows        %USERPROFILE%\.claude\.credentials.json
//	macOS, Linux   $HOME/.claude/.credentials.json
//
// filepath.Join supplies the separator, so the Windows form above is what a
// Windows build produces, not a string spelled out here. On macOS the
// Keychain is tried before this file; see loadStore.
func credentialPathFor(goos string) (string, error) {
	home, err := homeDirFor(goos)
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".claude", ".credentials.json"), nil
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
// then the credential file. A failed keychain LOOKUP falls back to the file;
// a keychain HIT does not — bytes that came out of the Keychain but do not
// parse are reported as the error they are, rather than silently reading a
// file that may be staler.
//
// When the file IS present, any keychain lookup failure is simply the
// fallback path: the file's credential is used and the keychain error is
// dropped.
//
// When neither source yields a credential, the keychain error decides the
// wording. A genuine miss (see keychainMiss) is left to the file error alone,
// so an absent file still renders "not logged in". Any other keychain failure
// is reported alongside it: on its own when the file is merely absent, so a
// denied Keychain prompt never reads as being logged out, and joined with the
// file's own error when that read failed for a reason of its own, so neither
// half is lost.
//
// Error vocabulary, per credstore's contract: os.ReadFile's error is returned
// unchanged when the file is absent (credstore maps os.ErrNotExist to
// provider.ErrNotLoggedIn), credstore.ErrNotFound when the store parses but
// holds no usable token, and provider.ErrTokenExpired by value when the token
// is expired. It never returns a zero credential with a nil error.
func (p *Provider) loadStore(ctx context.Context) (credential, error) {
	data, keychainErr := p.keychain(ctx)
	if keychainErr == nil && len(bytes.TrimSpace(data)) == 0 {
		keychainErr = errKeychainEmpty
	}
	if keychainErr == nil {
		return p.parseStore(data, keychainSource)
	}

	path, err := p.credentialPath()
	if err != nil {
		return credential{}, err
	}
	data, err = os.ReadFile(path)
	if err != nil {
		if keychainMiss(keychainErr) {
			// Nothing was in the keychain to begin with, so the file error is
			// the whole story — including os.ErrNotExist, which credstore
			// turns into "not logged in".
			return credential{}, err
		}
		if errors.Is(err, os.ErrNotExist) {
			// Only the keychain actually failed: keep its error so credstore
			// reports it instead of mapping the missing file to "not logged
			// in".
			return credential{}, fmt.Errorf(
				"macOS Keychain lookup (service %q) failed and there is no credential file at %s: %w",
				keychainService, path, keychainErr)
		}
		// Both sources failed for their own reasons. Both are wrapped (one
		// message, two %w verbs, so errors.Is still matches either) rather
		// than errors.Join'd, whose newline would break the one-line-per-row
		// rendering in internal/usage.
		return credential{}, fmt.Errorf(
			"%w; macOS Keychain lookup (service %q) also failed: %w",
			err, keychainService, keychainErr)
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
