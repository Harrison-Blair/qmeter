// Package claude implements qmeter's read-only Claude usage provider: it
// resolves a credential (the QMETER_CLAUDE_TOKEN override, else Claude Code's
// own credential store), queries the OAuth usage endpoint, and normalizes the
// answer into provider.Window values.
//
// The package never writes to Claude's credential store and never refreshes a
// token.
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/lib/httpx"
	"github.com/Harrison-Blair/qmeter/internal/provider"
)

const (
	// providerID is the value ID returns and the Provider field every Window
	// carries.
	providerID = "claude"

	// toolName is the CLI named in the "run/open <tool>" hints.
	toolName = "claude"

	// envVar is this provider's credential override, step 1 of the lookup
	// order.
	envVar = "QMETER_CLAUDE_TOKEN"
)

// DefaultBaseURL is the usage endpoint's origin. Tests point a Provider at an
// httptest server instead with WithBaseURL.
const DefaultBaseURL = "https://api.anthropic.com"

const (
	// usagePath is the usage endpoint's path: GET
	// https://api.anthropic.com/api/oauth/usage.
	usagePath = "/api/oauth/usage"

	// betaHeader and betaVersion are the OAuth beta opt-in the usage endpoint
	// requires, sent verbatim as "anthropic-beta: oauth-2025-04-20".
	betaHeader  = "anthropic-beta"
	betaVersion = "oauth-2025-04-20"
)

var _ provider.Provider = (*Provider)(nil)

// Provider is the Claude usage provider. Use New to build one.
type Provider struct {
	// credPath overrides the OS-default credential file path; empty means
	// defaultCredentialPath().
	credPath string

	// baseURL overrides the usage endpoint's origin; empty means
	// DefaultBaseURL.
	baseURL string

	// client is passed through to httpx; nil means httpx.DefaultClient.
	// httpx sets no timeout on either — the deadline comes from the context.
	client *http.Client

	// now is the clock the expiry check reads, injectable for tests.
	now func() time.Time

	// keychain is the macOS Keychain seam (see credentials.go); noKeychain
	// on this unit.
	keychain keychainLoader
}

// Option configures a Provider.
type Option func(*Provider)

// WithCredentialPath reads the credential store from path instead of the
// OS-default location.
func WithCredentialPath(path string) Option {
	return func(p *Provider) { p.credPath = path }
}

// WithBaseURL sends usage requests to base instead of DefaultBaseURL. The
// path (/api/oauth/usage) is appended to it.
func WithBaseURL(base string) Option {
	return func(p *Provider) { p.baseURL = base }
}

// WithHTTPClient uses c for the usage request instead of httpx's default
// client.
func WithHTTPClient(c *http.Client) Option {
	return func(p *Provider) { p.client = c }
}

// WithClock replaces the clock the credential expiry check reads.
func WithClock(now func() time.Time) Option {
	return func(p *Provider) { p.now = now }
}

// New returns a Claude provider configured by opts.
func New(opts ...Option) *Provider {
	p := &Provider{
		baseURL:  DefaultBaseURL,
		now:      time.Now,
		keychain: noKeychain,
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.baseURL == "" {
		p.baseURL = DefaultBaseURL
	}
	if p.now == nil {
		p.now = time.Now
	}
	if p.keychain == nil {
		p.keychain = noKeychain
	}
	return p
}

// ID returns "claude".
func (p *Provider) ID() string { return providerID }

// Detect reports whether a credential is resolvable without any network I/O.
// A store credential that has expired still counts as detected — the store is
// there and parses, so the user is logged in; Fetch is what reports the
// expiry. When the answer is false the reason is shown to the user verbatim
// and carries no provider-name prefix.
//
// On macOS this is not free of side effects: resolving a credential reads the
// Keychain by spawning /usr/bin/security (see credentials.go), which can put
// a Keychain authorization prompt in front of the user. Denying it does not
// report them as logged out — the lookup error is surfaced instead.
func (p *Provider) Detect(ctx context.Context) (bool, string) {
	_, _, err := p.resolve(ctx)
	switch {
	case err == nil:
		return true, ""
	case errors.Is(err, provider.ErrTokenExpired{}):
		return true, ""
	default:
		return false, err.Error()
	}
}

// Fetch resolves a credential and returns the current usage windows: the
// documented five_hour, seven_day, seven_day_sonnet and seven_day_opus
// windows, in that order, followed by any scoped per-model limits newer
// responses carry. Parsing is tolerant — unknown fields are ignored and a
// window qmeter cannot read is skipped rather than failing the fetch.
//
// A 200 that yields no window at all is an error, not an empty success: the
// user asked for usage and there is none to show.
//
// An expired store credential is reported without any network I/O.
func (p *Provider) Fetch(ctx context.Context) ([]provider.Window, error) {
	// Credential errors pass through unwrapped: credstore has already worded
	// them for display ("not logged in, run claude to log in", or "credential
	// store: ..."), and a second prefix would double up in the failure line.
	cred, _, err := p.resolve(ctx)
	if err != nil {
		return nil, err
	}

	// Decoding the top level as raw messages keeps the unknown-key handling
	// in one place: known window keys are decoded below, everything else is
	// offered to the scoped-limit parser and skipped when it does not fit.
	var body map[string]json.RawMessage
	opts := httpx.Options{
		URL:  p.usageURL(),
		Tool: toolName,
		Headers: map[string]string{
			"Authorization": "Bearer " + cred.AccessToken,
			betaHeader:      betaVersion,
		},
		Client: p.client,
	}
	if err := httpx.Get(ctx, opts, &body); err != nil {
		return nil, fmt.Errorf("usage request: %w", err)
	}
	windows := windowsFrom(body, cred.Plan)
	if len(windows) == 0 {
		// An empty object, a null body, or a response whose shape has moved
		// on entirely: reporting nothing would look like a provider with no
		// limits rather than one qmeter could not read.
		return nil, errNoKnownWindows
	}
	return windows, nil
}

// errNoKnownWindows is the failure for a 200 that carried nothing qmeter
// recognizes. It is worded for display: internal/usage prints it verbatim
// after the provider name.
var errNoKnownWindows = errors.New("usage response carried no known windows")

// usageURL is the endpoint this Provider queries.
func (p *Provider) usageURL() string {
	return strings.TrimSuffix(p.baseURL, "/") + usagePath
}

// knownWindows maps each documented top-level response key to the window name
// and period qmeter reports for it, in the order the windows are emitted.
var knownWindows = []struct {
	key    string
	name   string
	period time.Duration
}{
	{key: "five_hour", name: "5h", period: 5 * time.Hour},
	{key: "seven_day", name: "weekly", period: 7 * 24 * time.Hour},
	{key: "seven_day_sonnet", name: "sonnet weekly", period: 7 * 24 * time.Hour},
	{key: "seven_day_opus", name: "opus weekly", period: 7 * 24 * time.Hour},
}

// windowPayload is one documented window: {utilization, resets_at}.
// Utilization is a pointer so an absent percentage is distinguishable from
// zero usage — a window with no utilization is skipped, a window at 0 is
// reported.
type windowPayload struct {
	Utilization *float64  `json:"utilization"`
	ResetsAt    timestamp `json:"resets_at"`
}

// scopedLimit is one entry of the scoped per-model limit list newer responses
// may carry: {percent, resets_at, group}. The window is named after group;
// its period is unknown, so it stays zero.
type scopedLimit struct {
	Percent  *float64  `json:"percent"`
	ResetsAt timestamp `json:"resets_at"`
	Group    string    `json:"group"`
}

// windowsFrom normalizes a decoded response. plan is the credential's plan
// name, which every window repeats.
func windowsFrom(body map[string]json.RawMessage, plan string) []provider.Window {
	var out []provider.Window
	seen := make(map[string]bool, len(knownWindows))

	for _, kw := range knownWindows {
		raw, ok := body[kw.key]
		if !ok {
			continue
		}
		var payload windowPayload
		if err := json.Unmarshal(raw, &payload); err != nil || payload.Utilization == nil {
			continue // a window qmeter cannot read is skipped, not fatal
		}
		out = append(out, provider.Window{
			Provider:    providerID,
			Name:        kw.name,
			Plan:        plan,
			UsedPercent: *payload.Utilization,
			ResetsAt:    payload.ResetsAt.Time,
			Period:      kw.period,
		})
		seen[kw.name] = true
	}
	return append(out, scopedWindows(body, plan, seen)...)
}

// scopedWindows reads the scoped per-model limits. The response key holding
// them is not fixed by the reference, so every key that is not a documented
// window is tried as a list of scoped limits and skipped when it is not one.
// Keys are visited in sorted order so the window order is deterministic
// whatever order the JSON object came in.
func scopedWindows(body map[string]json.RawMessage, plan string, seen map[string]bool) []provider.Window {
	keys := make([]string, 0, len(body))
	for key := range body {
		if !isKnownWindowKey(key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	var out []provider.Window
	for _, key := range keys {
		var entries []json.RawMessage
		if err := json.Unmarshal(body[key], &entries); err != nil {
			continue // not a list: not the scoped-limit section
		}
		for _, entry := range entries {
			var limit scopedLimit
			if err := json.Unmarshal(entry, &limit); err != nil {
				continue
			}
			name := strings.TrimSpace(limit.Group)
			// All three fields are required, which is also what tells a real
			// scoped limit apart from some other list that happens to carry a
			// group and a percent: a notice like
			// {"group":"billing","percent":100} has no reset time and must
			// not render as a window sitting at 100% used. An entry repeating
			// a window already reported would double it.
			if name == "" || limit.Percent == nil || limit.ResetsAt.IsZero() || seen[name] {
				continue
			}
			out = append(out, provider.Window{
				Provider:    providerID,
				Name:        name,
				Plan:        plan,
				UsedPercent: *limit.Percent,
				ResetsAt:    limit.ResetsAt.Time,
			})
			seen[name] = true
		}
	}
	return out
}

func isKnownWindowKey(key string) bool {
	for _, kw := range knownWindows {
		if kw.key == key {
			return true
		}
	}
	return false
}

// millisThreshold splits epoch seconds from epoch milliseconds: the endpoint
// sends resets_at either as an ISO-8601 string or as an epoch number, without
// saying which unit. 1e11 seconds is the year 5138 and 1e11 milliseconds is
// 1973, so no plausible reset instant is ambiguous.
const millisThreshold = 1e11

// timestamp decodes resets_at in either documented form — an ISO-8601 string
// or an epoch number — and tolerates anything else by staying zero, so one
// unreadable reset time never costs the caller the whole window (a zero
// ResetsAt renders as "-").
type timestamp struct {
	time.Time
}

// isoLayouts are the accepted string forms: RFC3339 (with or without
// fractional seconds) and the zone-less variant, read as UTC.
var isoLayouts = []string{time.RFC3339, "2006-01-02T15:04:05"}

func (t *timestamp) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		for _, layout := range isoLayouts {
			if parsed, err := time.Parse(layout, strings.TrimSpace(text)); err == nil {
				t.Time = parsed.UTC()
				return nil
			}
		}
		return nil
	}
	var epoch float64
	if err := json.Unmarshal(data, &epoch); err == nil && epoch > 0 {
		if epoch >= millisThreshold {
			t.Time = time.UnixMilli(int64(epoch)).UTC()
		} else {
			t.Time = time.Unix(int64(epoch), 0).UTC()
		}
	}
	return nil
}
