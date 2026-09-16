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
	Provider    string        // wire form: "provider" (see MarshalJSON, not struct tags)
	Name        string        // "name"; e.g. "5h", "weekly", "monthly", "sonnet weekly"
	Plan        string        // "plan", omitted when empty
	UsedPercent float64       // "used_percent"
	ResetsAt    time.Time     // "resets_at" (RFC3339), omitted when zero
	Period      time.Duration // "period_seconds" (whole seconds), omitted when zero
	RateLimited bool          // "rate_limited"
}

// MarshalJSON is Window's wire-form encoder (not struct tags — time.Time and
// time.Duration don't serialize the way tags would imply). It emits
// "provider", "name", "plan" (omitted when empty), "used_percent",
// "resets_at" (RFC3339, omitted when ResetsAt.IsZero()), "period_seconds"
// (int64(Period / time.Second), omitted when zero), and "rate_limited"
// (always present).
func (w Window) MarshalJSON() ([]byte, error) {
	wire := struct {
		Provider      string  `json:"provider"`
		Name          string  `json:"name"`
		Plan          string  `json:"plan,omitempty"`
		UsedPercent   float64 `json:"used_percent"`
		ResetsAt      string  `json:"resets_at,omitempty"`
		PeriodSeconds int64   `json:"period_seconds,omitempty"`
		RateLimited   bool    `json:"rate_limited"`
	}{
		Provider:    w.Provider,
		Name:        w.Name,
		Plan:        w.Plan,
		UsedPercent: w.UsedPercent,
		RateLimited: w.RateLimited,
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
type ErrNotLoggedIn struct {
	Tool string
}

func (e ErrNotLoggedIn) Error() string {
	return fmt.Sprintf("not logged in, run %s to log in", e.Tool)
}

// Is reports whether target is an ErrNotLoggedIn, regardless of Tool value,
// so callers can test with errors.Is(err, provider.ErrNotLoggedIn{}).
func (e ErrNotLoggedIn) Is(target error) bool {
	_, ok := target.(ErrNotLoggedIn)
	return ok
}

// ErrTokenExpired indicates a credential was found but is expired or was
// rejected by the vendor as unauthorized. Tool is the CLI name used in the
// "open <tool>" hint (one of: claude, codex, cursor-agent, opencode).
type ErrTokenExpired struct {
	Tool string
}

func (e ErrTokenExpired) Error() string {
	return fmt.Sprintf("token expired, open %s to refresh", e.Tool)
}

// Is reports whether target is an ErrTokenExpired, regardless of Tool
// value, so callers can test with errors.Is(err, provider.ErrTokenExpired{}).
func (e ErrTokenExpired) Is(target error) bool {
	_, ok := target.(ErrTokenExpired)
	return ok
}

// ErrRateLimited indicates the vendor responded 429. RetryAfter is the
// parsed retry-after duration (zero when the vendor did not supply one).
type ErrRateLimited struct {
	RetryAfter time.Duration
}

func (e ErrRateLimited) Error() string {
	return fmt.Sprintf("rate limited, retry in %s", e.RetryAfter)
}

// Is reports whether target is an ErrRateLimited, regardless of RetryAfter
// value, so callers can test with errors.Is(err, provider.ErrRateLimited{}).
func (e ErrRateLimited) Is(target error) bool {
	_, ok := target.(ErrRateLimited)
	return ok
}
