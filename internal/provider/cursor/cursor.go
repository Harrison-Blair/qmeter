// Package cursor reads Cursor subscription usage, read-only.
//
// Credentials come from the Cursor CLI store (~/.config/cursor/auth.json,
// written by cursor-agent) or from the QMETER_CURSOR_TOKEN override; qmeter
// never writes to either. Usage comes from route A,
// GET https://cursor.com/api/usage-summary, authenticated with the
// WorkosCursorSessionToken cookie.
package cursor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/lib/httpx"
	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// defaultBaseURL is the origin route A lives on. Tests point this at an
// httptest server.
const defaultBaseURL = "https://cursor.com"

// Provider implements provider.Provider for Cursor.
var _ provider.Provider = (*Provider)(nil)

// Provider is the Cursor usage provider. Use New to build one.
type Provider struct {
	// credPath overrides the CLI store location; empty means the OS default.
	credPath string
	// baseURL overrides the route A origin; empty means defaultBaseURL.
	baseURL string
	// client overrides the HTTP client; nil means httpx's default.
	client *http.Client
}

// Option configures a Provider. Every seam a test needs to replace — the
// credential file, the endpoint origin, the HTTP client — is one of these;
// the environment override is read from the real environment, so tests use
// t.Setenv.
type Option func(*Provider)

// WithCredentialPath reads the CLI store from path instead of the OS default.
func WithCredentialPath(path string) Option {
	return func(p *Provider) { p.credPath = path }
}

// WithBaseURL sends route A requests to base instead of https://cursor.com.
func WithBaseURL(base string) Option {
	return func(p *Provider) { p.baseURL = base }
}

// WithHTTPClient issues requests with c instead of httpx's default client.
func WithHTTPClient(c *http.Client) Option {
	return func(p *Provider) { p.client = c }
}

// New returns a Cursor provider.
func New(opts ...Option) *Provider {
	p := &Provider{}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// ID returns the provider id used by --provider and in rendered output. It is
// not the CLI name used in hints, which is cursor-agent.
func (p *Provider) ID() string { return "cursor" }

// Detect reports whether a Cursor credential is resolvable without any
// network I/O: the environment override is set to a usable token, or the CLI
// store exists, parses, and yields an access token this package can read a
// user id out of.
//
// Detect deliberately does not judge whether the token is still valid — the
// CLI store has no expiry field and the JWT's exp is not checked — so a store
// holding an expired token still counts as detected, and Fetch is what
// surfaces the vendor's 401/403 as provider.ErrTokenExpired.
func (p *Provider) Detect(ctx context.Context) (bool, string) {
	if _, _, err := p.credential(ctx); err != nil {
		return false, detectReason(err)
	}
	return true, ""
}

// Fetch retrieves the current billing cycle's usage from route A and
// normalizes it into the "total" and "auto" windows.
//
// Only route A is implemented. The Connect-RPC route stays a known fallback
// should route A be withdrawn; no code here reaches for it.
func (p *Provider) Fetch(ctx context.Context) ([]provider.Window, error) {
	cred, _, err := p.credential(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve credential: %w", err)
	}

	var summary usageSummary
	opts := httpx.Options{
		URL:  p.usageSummaryURL(),
		Tool: toolName,
		Headers: map[string]string{
			// Route A authenticates with this cookie alone. The "%3A%3A"
			// separator is sent literally: the vendor expects the token
			// pair already percent-encoded, so it must not be escaped again
			// and must not be written as a raw "::".
			"Cookie": "WorkosCursorSessionToken=" + cred.UserID + "%3A%3A" + cred.AccessToken,
		},
		Client: p.client,
	}
	// httpx maps 401/403 to provider.ErrTokenExpired{Tool: toolName} and 429
	// to provider.ErrRateLimited; %w keeps both matchable.
	if err := httpx.Get(ctx, opts, &summary); err != nil {
		return nil, fmt.Errorf("fetch usage summary: %w", err)
	}
	// windows' errors already name the usage summary, so they are returned
	// unwrapped rather than doubling the words. Either way nothing here ever
	// prefixes an error with "cursor": the renderer puts the provider name in
	// its own column.
	return summary.windows(p.ID())
}

// usageSummaryURL is the route A endpoint.
func (p *Provider) usageSummaryURL() string {
	base := p.baseURL
	if base == "" {
		base = defaultBaseURL
	}
	return base + "/api/usage-summary"
}

// detectReason renders a credential error as the user-facing reason Detect
// returns. Typed errors are unwrapped first so their own wording is used
// rather than a wrapped message; either way the text never carries a
// provider-name prefix, because the caller already knows which provider it
// asked.
func detectReason(err error) string {
	var notLoggedIn provider.ErrNotLoggedIn
	if errors.As(err, &notLoggedIn) {
		return notLoggedIn.Error()
	}
	var expired provider.ErrTokenExpired
	if errors.As(err, &expired) {
		return expired.Error()
	}
	return err.Error()
}

// usageSummary is the route A response. It is deliberately partial and
// tolerant: unknown fields (the breakdown object, teamUsage — which is present
// as an empty object on a personal account — and anything the vendor adds
// later) are ignored, and a JSON null leaves a field at its zero value rather
// than failing the decode.
type usageSummary struct {
	// BillingCycleStart and BillingCycleEnd are RFC3339 timestamps bounding
	// the current cycle; both windows derive ResetsAt and Period from them.
	BillingCycleStart string `json:"billingCycleStart"`
	BillingCycleEnd   string `json:"billingCycleEnd"`

	// MembershipType is the plan name, e.g. "free" or "pro".
	MembershipType string `json:"membershipType"`

	// LimitType and IsUnlimited are parsed but not yet surfaced in a Window;
	// whether an unlimited plan should suppress or relabel the percentage is
	// still an open question.
	LimitType   string `json:"limitType"`
	IsUnlimited bool   `json:"isUnlimited"`

	IndividualUsage struct {
		Plan struct {
			// Enabled is a pointer so an absent key is distinguishable from
			// an explicit false: the first means the response shape moved
			// under us (or individualUsage was null or renamed), the second
			// that this account genuinely has no individual plan usage.
			// Both are reported as errors, never as "no windows".
			Enabled   *bool   `json:"enabled"`
			Used      float64 `json:"used"`
			Limit     float64 `json:"limit"`
			Remaining float64 `json:"remaining"`

			// The two percentages this provider actually reports are
			// pointers for the same reason: rendering an absent value as
			// 0.0% would claim the account has used nothing, which is the
			// opposite of not knowing. APIPercentUsed is not surfaced, so a
			// plain float64 is enough for it.
			AutoPercentUsed  *float64 `json:"autoPercentUsed"`
			APIPercentUsed   float64  `json:"apiPercentUsed"`
			TotalPercentUsed *float64 `json:"totalPercentUsed"`
		} `json:"plan"`

		// OnDemand is parsed for completeness; its limit and remaining are
		// null on accounts without on-demand spending. Not surfaced yet.
		OnDemand struct {
			Enabled   bool    `json:"enabled"`
			Used      float64 `json:"used"`
			Limit     float64 `json:"limit"`
			Remaining float64 `json:"remaining"`
		} `json:"onDemand"`
	} `json:"individualUsage"`
}

// errNoPlanEnabled is returned when individualUsage.plan.enabled is absent
// altogether — an empty or null body, a null or renamed individualUsage, or a
// plan object that no longer carries the key.
var errNoPlanEnabled = errors.New("usage summary has no individualUsage.plan.enabled field; the response shape may have changed")

// errNoPercentages is returned when the response omits the two percentages
// this provider reports.
var errNoPercentages = errors.New("usage summary is missing totalPercentUsed/autoPercentUsed")

// windows normalizes the response into exactly two windows, "total" and
// "auto", in that order — the primary window first. Both describe the same
// billing cycle, so both carry the same ResetsAt and Period.
//
// It never returns an empty slice with a nil error: an account with nothing
// to report and a response qmeter can no longer read look identical once the
// windows are gone, and "no windows" renders as silence rather than as a
// problem. Every such case comes back as an error instead, worded so the
// failure line says which of the two it was.
func (s usageSummary) windows(id string) ([]provider.Window, error) {
	plan := s.IndividualUsage.Plan
	switch {
	case plan.Enabled == nil:
		return nil, errNoPlanEnabled
	case !*plan.Enabled:
		return nil, fmt.Errorf(
			"no individual plan usage for this account (membershipType %q); team and enterprise pooled usage is not read yet",
			s.MembershipType)
	case plan.TotalPercentUsed == nil || plan.AutoPercentUsed == nil:
		return nil, errNoPercentages
	}
	resetsAt, period := s.billingCycle()
	return []provider.Window{
		{
			Provider:    id,
			Name:        "total",
			Plan:        s.MembershipType,
			UsedPercent: *plan.TotalPercentUsed,
			ResetsAt:    resetsAt,
			Period:      period,
		},
		{
			Provider:    id,
			Name:        "auto",
			Plan:        s.MembershipType,
			UsedPercent: *plan.AutoPercentUsed,
			ResetsAt:    resetsAt,
			Period:      period,
		},
	}, nil
}

// billingCycle converts the cycle bounds into a reset time and a period.
// Unusable bounds yield zero values rather than an error — the percentages
// are still worth reporting, and the renderer prints "-" for a zero ResetsAt:
//
//   - an unparsable END leaves both zero, since nothing can be said about the
//     cycle without it;
//   - an unparsable start, or a start not before the end, keeps the reset
//     time and leaves only the Period zero, because a period needs both
//     bounds but a reset time does not.
func (s usageSummary) billingCycle() (resetsAt time.Time, period time.Duration) {
	end, err := time.Parse(time.RFC3339, s.BillingCycleEnd)
	if err != nil {
		return time.Time{}, 0
	}
	start, err := time.Parse(time.RFC3339, s.BillingCycleStart)
	if err != nil || !start.Before(end) {
		return end, 0
	}
	return end, end.Sub(start)
}
