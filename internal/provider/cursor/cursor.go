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
// Only route A is implemented. The Connect-RPC route stays a documented
// fallback in the plan; no code here reaches for it.
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
	return summary.windows(p.ID()), nil
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
// tolerant: unknown fields (the plan's breakdown object, teamUsage — which is
// present as an empty object on a personal account — and anything the vendor
// adds later) are ignored, and a JSON null leaves a field at its zero value
// rather than failing the decode.
type usageSummary struct {
	// BillingCycleStart and BillingCycleEnd are RFC3339 timestamps bounding
	// the current cycle; both windows derive ResetsAt and Period from them.
	BillingCycleStart string `json:"billingCycleStart"`
	BillingCycleEnd   string `json:"billingCycleEnd"`

	// MembershipType is the plan name, e.g. "free" or "pro".
	MembershipType string `json:"membershipType"`

	// LimitType and IsUnlimited are parsed but not yet surfaced in a Window;
	// whether an unlimited plan should suppress or relabel the percentage is
	// an open question in the plan.
	LimitType   string `json:"limitType"`
	IsUnlimited bool   `json:"isUnlimited"`

	IndividualUsage struct {
		Plan struct {
			// Enabled false means this account has no plan usage to report;
			// Fetch then returns no windows at all.
			Enabled   bool    `json:"enabled"`
			Used      float64 `json:"used"`
			Limit     float64 `json:"limit"`
			Remaining float64 `json:"remaining"`

			AutoPercentUsed  float64 `json:"autoPercentUsed"`
			APIPercentUsed   float64 `json:"apiPercentUsed"`
			TotalPercentUsed float64 `json:"totalPercentUsed"`
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

// windows normalizes the response into exactly two windows, "total" and
// "auto", in that order — the plan's primary window first. Both describe the
// same billing cycle, so both carry the same ResetsAt and Period.
//
// A plan that is not enabled yields no windows and no error: there is nothing
// to report for the account, which is not a failure.
func (s usageSummary) windows(id string) []provider.Window {
	if !s.IndividualUsage.Plan.Enabled {
		return nil
	}
	resetsAt, period := s.billingCycle()
	plan := s.IndividualUsage.Plan
	return []provider.Window{
		{
			Provider:    id,
			Name:        "total",
			Plan:        s.MembershipType,
			UsedPercent: plan.TotalPercentUsed,
			ResetsAt:    resetsAt,
			Period:      period,
		},
		{
			Provider:    id,
			Name:        "auto",
			Plan:        s.MembershipType,
			UsedPercent: plan.AutoPercentUsed,
			ResetsAt:    resetsAt,
			Period:      period,
		},
	}
}

// billingCycle converts the cycle bounds into a reset time and a period.
// Unparsable or absent bounds yield zero values rather than an error: the
// percentages are still worth reporting, and the renderer prints "-" for a
// zero ResetsAt.
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
