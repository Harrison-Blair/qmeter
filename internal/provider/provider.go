// Package provider defines the normalized usage-window data model, the
// Provider interface every vendor implementation satisfies, and the typed
// errors credential lookup and HTTP handling return.
//
// This package is a leaf with respect to its own subpackages: it imports
// nothing under internal/provider/* and nothing from internal/usage. The
// four-provider registry lives in internal/usage, not here.
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Window is one normalized usage window for a provider (e.g. a 5-hour
// rolling window, a weekly window, a monthly window).
type Window struct {
	Provider         string        // wire form: "provider" (see MarshalJSON, not struct tags)
	Name             string        // "name"; e.g. "5h", "weekly", "monthly", "sonnet weekly"
	Plan             string        // "plan", omitted when empty
	RemainingPercent float64       // "remaining_percent"; 0 = nothing left, 100 = untouched
	ResetsAt         time.Time     // "resets_at" (RFC3339), omitted when zero
	Period           time.Duration // "period_seconds" (whole seconds), omitted when zero
	RateLimited      bool          // "rate_limited"
}

// MarshalJSON is Window's wire-form encoder (not struct tags — time.Time and
// time.Duration don't serialize the way tags would imply). It emits
// "provider", "name", "plan" (omitted when empty), "remaining_percent",
// "resets_at" (RFC3339, omitted when ResetsAt.IsZero()), "period_seconds"
// (int64(Period / time.Second), omitted when zero), and "rate_limited"
// (always present).
func (w Window) MarshalJSON() ([]byte, error) {
	wire := struct {
		Provider         string  `json:"provider"`
		Name             string  `json:"name"`
		Plan             string  `json:"plan,omitempty"`
		RemainingPercent float64 `json:"remaining_percent"`
		ResetsAt         string  `json:"resets_at,omitempty"`
		PeriodSeconds    int64   `json:"period_seconds,omitempty"`
		RateLimited      bool    `json:"rate_limited"`
	}{
		Provider:         w.Provider,
		Name:             w.Name,
		Plan:             w.Plan,
		RemainingPercent: w.RemainingPercent,
		RateLimited:      w.RateLimited,
	}
	if !w.ResetsAt.IsZero() {
		wire.ResetsAt = w.ResetsAt.Format(time.RFC3339)
	}
	if w.Period != 0 {
		wire.PeriodSeconds = int64(w.Period / time.Second)
	}
	return json.Marshal(wire)
}

// Provider is a single vendor's usage-limit source.
type Provider interface {
	// ID returns exactly one of "claude", "codex", "cursor", "opencode-go".
	ID() string

	// Detect reports whether a credential is resolvable without network
	// I/O — either an env var override is set, or the vendor's local store
	// exists and parses. When it returns false, the string is the
	// user-facing reason, shown as-is to the user; it must NOT include the
	// provider name as a prefix (the caller already knows which provider it
	// asked), e.g. "not logged in, run claude to log in", not "claude: not
	// logged in, run claude to log in". The <tool> names used in every hint
	// are: claude, codex, cursor-agent, opencode.
	Detect(ctx context.Context) (bool, string)

	// Fetch retrieves and normalizes the provider's current usage windows.
	Fetch(ctx context.Context) ([]Window, error)
}

// ErrNotLoggedIn indicates no usable credential was found — no env override
// and no (or unparsable) vendor store. Tool is the CLI name used in the
// "open <tool>" hint (one of: claude, codex, cursor-agent, opencode).
//
// Always return this by value (ErrNotLoggedIn{...}), never by pointer:
// errors.As with a value target does not match a *ErrNotLoggedIn in the
// chain.
type ErrNotLoggedIn struct {
	Tool string
}

func (e ErrNotLoggedIn) Error() string {
	return fmt.Sprintf("not logged in, run %s to log in", e.Tool)
}

// Is compares by type only and ignores the fields entirely, in both
// directions — always test with the zero value,
// errors.Is(err, provider.ErrNotLoggedIn{}), and read the fields with
// errors.As. A populated target matches too, so it is never a field check.
func (e ErrNotLoggedIn) Is(target error) bool {
	_, ok := target.(ErrNotLoggedIn)
	return ok
}

// ErrTokenExpired indicates a credential was found but is expired or was
// rejected by the vendor as unauthorized. Tool is the CLI name used in the
// "open <tool>" hint (one of: claude, codex, cursor-agent, opencode).
//
// Always return this by value (ErrTokenExpired{...}), never by pointer:
// errors.As with a value target does not match a *ErrTokenExpired in the
// chain.
type ErrTokenExpired struct {
	Tool string
}

func (e ErrTokenExpired) Error() string {
	return fmt.Sprintf("token expired, open %s to refresh", e.Tool)
}

// Is compares by type only and ignores the fields entirely, in both
// directions — always test with the zero value,
// errors.Is(err, provider.ErrTokenExpired{}), and read the fields with
// errors.As. A populated target matches too, so it is never a field check.
func (e ErrTokenExpired) Is(target error) bool {
	_, ok := target.(ErrTokenExpired)
	return ok
}

// ErrRateLimited indicates the vendor responded 429. RetryAfter is the
// parsed retry-after duration (zero when the vendor did not supply one).
//
// Always return this by value (ErrRateLimited{...}), never by pointer:
// errors.As with a value target does not match a *ErrRateLimited in the
// chain.
type ErrRateLimited struct {
	RetryAfter time.Duration
}

func (e ErrRateLimited) Error() string {
	return fmt.Sprintf("rate limited, retry in %s", e.RetryAfter)
}

// Is compares by type only and ignores the fields entirely, in both
// directions — always test with the zero value,
// errors.Is(err, provider.ErrRateLimited{}), and read the fields with
// errors.As. A populated target matches too, so it is never a field check.
func (e ErrRateLimited) Is(target error) bool {
	_, ok := target.(ErrRateLimited)
	return ok
}

// RemainingFromUsed converts a vendor's used/utilization percentage into the
// remaining percentage every Window carries (Window.RemainingPercent: 0 =
// nothing left, 100 = untouched).
//
// The result is clamped to [0, 100] so a vendor that reports more than 100%
// used — an over-quota account, or a percentage that momentarily overshoots —
// never renders as a negative remainder, and one that reports a negative used
// never renders as more than a full window.
func RemainingFromUsed(used float64) float64 {
	remaining := 100 - used
	if remaining < 0 {
		return 0
	}
	if remaining > 100 {
		return 100
	}
	return remaining
}
