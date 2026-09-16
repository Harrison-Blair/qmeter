// Package opencodego reads OpenCode Zen "go" plan usage limits.
//
// Credentials come from the QMETER_OPENCODE_GO_KEY override or from
// OpenCode's own auth store; qmeter only ever reads them.
package opencodego

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/lib/httpx"
	"github.com/Harrison-Blair/qmeter/internal/provider"
)

const (
	// id is what this provider is called on the command line.
	id = "opencode-go"
	// tool is the CLI name used in the "run/open <tool>" hints.
	tool = "opencode"
	// envVar is this provider's credential override.
	envVar = "QMETER_OPENCODE_GO_KEY"
	// planName is a constant: the Zen "go" plan is the only one this
	// credential can report on, and the response carries no plan field.
	planName = "go"

	// defaultBaseURL and usagePath together form the vendor endpoint.
	defaultBaseURL = "https://opencode.ai"
	usagePath      = "/zen/go/v1/usage"

	// statusRateLimited is the window status meaning the cap is spent.
	statusRateLimited = "rate-limited"

	// rollingPeriod and weeklyPeriod are fixed; only the monthly window's
	// length has to be derived from its reset time.
	rollingPeriod = 5 * time.Hour
	weeklyPeriod  = 7 * 24 * time.Hour
)

// Provider reads OpenCode Go usage. The zero value is not usable; call New.
type Provider struct {
	credentialPath string
	baseURL        string
	httpClient     *http.Client
}

var _ provider.Provider = (*Provider)(nil)

// Option configures a Provider. Every seam a test needs to replace — the
// credential file, the endpoint, and the HTTP client — is one of these.
type Option func(*Provider)

// WithCredentialPath overrides the path of OpenCode's auth store.
func WithCredentialPath(path string) Option {
	return func(p *Provider) { p.credentialPath = path }
}

// WithBaseURL overrides the scheme-and-host the usage path is appended to, so
// tests can point at an httptest server.
func WithBaseURL(baseURL string) Option {
	return func(p *Provider) { p.baseURL = baseURL }
}

// WithHTTPClient overrides the http.Client used for the usage request.
func WithHTTPClient(c *http.Client) Option {
	return func(p *Provider) { p.httpClient = c }
}

// New returns a Provider reading the real OpenCode auth store and the real
// endpoint unless an Option says otherwise.
func New(opts ...Option) *Provider {
	p := &Provider{
		credentialPath: defaultCredentialPath(),
		baseURL:        defaultBaseURL,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// ID returns "opencode-go".
func (p *Provider) ID() string { return id }

// usageURL is the full usage endpoint for this Provider.
func (p *Provider) usageURL() string {
	return strings.TrimSuffix(p.baseURL, "/") + usagePath
}

// Detect reports whether a credential is resolvable without any network I/O:
// the env override is set, or the auth store exists and parses. The reason is
// shown verbatim to the user and never repeats the provider name.
func (p *Provider) Detect(ctx context.Context) (bool, string) {
	if _, _, err := p.credential(ctx); err != nil {
		return false, err.Error()
	}
	return true, ""
}

// Fetch retrieves the current usage windows, in the order rolling, weekly,
// monthly; windows the response omits are skipped.
func (p *Provider) Fetch(ctx context.Context) ([]provider.Window, error) {
	key, _, err := p.credential(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve credential: %w", err)
	}

	var resp usageResponse
	if err := httpx.Get(ctx, httpx.Options{
		URL:  p.usageURL(),
		Tool: tool,
		Headers: map[string]string{
			"Authorization": "Bearer " + key,
			"Accept":        "application/json",
		},
		Client: p.httpClient,
	}, &resp); err != nil {
		return nil, fmt.Errorf("fetch usage: %w", err)
	}

	windows, err := resp.windows()
	if err != nil {
		return nil, fmt.Errorf("parse usage response: %w", err)
	}
	return windows, nil
}

// usageResponse is the documented shape of GET /zen/go/v1/usage. Unknown
// fields and unknown window names are ignored; a known window that is absent
// is simply skipped, which is why each one is a pointer.
type usageResponse struct {
	Usage struct {
		Rolling *windowUsage `json:"rolling"`
		Weekly  *windowUsage `json:"weekly"`
		Monthly *windowUsage `json:"monthly"`
	} `json:"usage"`
}

// windowUsage is one window in that response. Percent is a json.Number so a
// value that is not a whole 0-100 percentage — a 0..1 fraction, say — can be
// rejected rather than rendered as a wrong meter.
type windowUsage struct {
	Status   string      `json:"status"`
	Percent  json.Number `json:"percent"`
	ResetsAt string      `json:"resetsAt"`
}

// windows normalizes the response, in the provider-declared order the
// renderer prints.
func (r usageResponse) windows() ([]provider.Window, error) {
	sources := []struct {
		name   string
		usage  *windowUsage
		period func(time.Time) time.Duration
	}{
		{name: "5h", usage: r.Usage.Rolling, period: fixedPeriod(rollingPeriod)},
		{name: "weekly", usage: r.Usage.Weekly, period: fixedPeriod(weeklyPeriod)},
		{name: "monthly", usage: r.Usage.Monthly, period: monthlyPeriod},
	}

	var out []provider.Window
	for _, s := range sources {
		if s.usage == nil {
			continue
		}
		w, err := s.usage.window(s.name)
		if err != nil {
			return nil, err
		}
		w.Period = s.period(w.ResetsAt)
		out = append(out, w)
	}
	if len(out) == 0 {
		// Every known window missing at once is a changed response, not an
		// account with nothing to report.
		return nil, errors.New("no known usage window (rolling, weekly, monthly) present")
	}
	return out, nil
}

// fixedPeriod adapts a constant window length to the per-window signature.
func fixedPeriod(d time.Duration) func(time.Time) time.Duration {
	return func(time.Time) time.Duration { return d }
}

// window normalizes one window. A "rate-limited" status means the cap is
// spent: 100% used, whatever percent says.
func (u windowUsage) window(name string) (provider.Window, error) {
	w := provider.Window{Provider: id, Name: name, Plan: planName}

	if u.ResetsAt != "" {
		at, err := time.Parse(time.RFC3339, u.ResetsAt)
		if err != nil {
			return provider.Window{}, fmt.Errorf("window %q: resetsAt %q is not RFC3339: %w", name, u.ResetsAt, err)
		}
		w.ResetsAt = at
	}

	if u.Status == statusRateLimited {
		w.UsedPercent = 100
		w.RateLimited = true
		return w, nil
	}

	pct, err := usedPercent(u.Percent)
	if err != nil {
		return provider.Window{}, fmt.Errorf("window %q: %w", name, err)
	}
	w.UsedPercent = pct
	return w, nil
}

// usedPercent reads the documented "percent: int 0-100". Anything else — a
// 0..1 fraction, an out-of-range number, a missing field — means the response
// shape changed, and a wrong meter is worse than an error line.
func usedPercent(n json.Number) (float64, error) {
	if n == "" {
		return 0, errors.New("percent is missing; the response shape has changed")
	}
	v, err := n.Int64()
	if err != nil {
		return 0, fmt.Errorf("percent %s is not a whole number; the response shape has changed", n)
	}
	if v < 0 || v > 100 {
		return 0, fmt.Errorf("percent %d is outside 0-100; the response shape has changed", v)
	}
	return float64(v), nil
}

// monthlyPeriod is the length of the calendar month ending at resetsAt: one
// month back on the same day of the month and at the same time of day, with
// the day clamped to the length of the shorter month (a 31st with a 30-day
// month before it lands on the 30th). It is zero when resetsAt is unknown.
func monthlyPeriod(resetsAt time.Time) time.Duration {
	if resetsAt.IsZero() {
		return 0
	}
	return resetsAt.Sub(monthBefore(resetsAt))
}

// monthBefore steps t back one calendar month without time.AddDate's
// normalization, which would turn a clamped 31 February into early March.
func monthBefore(t time.Time) time.Time {
	year, month, day := t.Date()
	prevYear, prevMonth := year, month-1
	if prevMonth < time.January {
		prevYear, prevMonth = year-1, time.December
	}
	if last := daysInMonth(prevYear, prevMonth); day > last {
		day = last
	}
	hour, min, sec := t.Clock()
	return time.Date(prevYear, prevMonth, day, hour, min, sec, t.Nanosecond(), t.Location())
}

// daysInMonth returns the number of days in the given month: day 0 of the
// following month is the last day of this one.
func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
