// Package credstore implements the provider-independent half of qmeter's
// credential lookup order: step 1, the per-provider environment variable
// override, and step 3, turning a missing credential into the typed
// provider.ErrNotLoggedIn error that carries the "run <tool> to log in" hint.
//
// Step 2 — reading the vendor's own local store — stays with each provider
// package, which knows its file path, its JSON shape, and (for Claude on
// macOS) its Keychain entry. Those packages hand that work to Resolve as a
// loader function, so no provider-specific parsing lives here.
//
// qmeter never writes to a vendor store; this package only reads.
package credstore

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// ErrNotFound is the sentinel a loader returns when the vendor's store is
// simply absent — no file, no Keychain entry, no credential in it. Resolve
// maps it (and os.ErrNotExist, which a plain os.Open already returns) to
// provider.ErrNotLoggedIn. Loaders that find a store but cannot use it should
// return a descriptive error instead, which Resolve passes through.
var ErrNotFound = errors.New("credential not found")

// Source reports which step of the lookup order produced a credential, since
// what a provider can say about it differs: an env override is a bare token,
// while the local store usually also carries the plan name (Claude's
// subscriptionType, for example). It is the empty SourceNone when Resolve
// returns an error.
type Source string

const (
	// SourceNone means no credential was resolved.
	SourceNone Source = ""
	// SourceEnv means the credential came from the environment variable
	// override, step 1 of the lookup order.
	SourceEnv Source = "env"
	// SourceStore means the credential came from the vendor's local store,
	// step 2 of the lookup order.
	SourceStore Source = "store"
)

// Resolve runs the credential lookup order for one provider and reports which
// step produced the credential.
//
// If the environment variable envVar is set to a non-empty value, that value
// is passed verbatim to fromEnv and the store loader is never called; the
// override wins outright. Otherwise load is called with ctx.
//
// A load error that is ErrNotFound or os.ErrNotExist (directly or wrapped)
// becomes provider.ErrNotLoggedIn{Tool: tool}, returned by value so callers
// can match it with errors.Is(err, provider.ErrNotLoggedIn{}) and read Tool
// with errors.As. Any other load error — an unparsable store, an expired
// credential the loader already typed as provider.ErrTokenExpired, a Keychain
// failure — is passed through wrapped, so errors.Is and errors.As still see
// it. The wrapping prefix deliberately never names the provider: callers use
// these messages as Detect reasons, which must not repeat the provider name.
//
// An error from fromEnv is likewise passed through wrapped, and is never
// converted to provider.ErrNotLoggedIn: the user did supply a credential, it
// just could not be used, so "run <tool> to log in" would be the wrong hint.
//
// tool is the CLI name used in that hint — one of claude, codex, cursor-agent,
// opencode.
func Resolve[T any](
	ctx context.Context,
	envVar string,
	tool string,
	load func(context.Context) (T, error),
	fromEnv func(string) (T, error),
) (T, Source, error) {
	var zero T

	if v, ok := os.LookupEnv(envVar); ok && v != "" {
		cred, err := fromEnv(v)
		if err != nil {
			return zero, SourceNone, fmt.Errorf("%s: %w", envVar, err)
		}
		return cred, SourceEnv, nil
	}

	cred, err := load(ctx)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return zero, SourceNone, provider.ErrNotLoggedIn{Tool: tool}
		}
		return zero, SourceNone, fmt.Errorf("credential store: %w", err)
	}
	return cred, SourceStore, nil
}
