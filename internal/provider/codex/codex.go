// Package codex reads Codex (ChatGPT) subscription usage limits.
//
// Credentials come from the QMETER_CODEX_TOKEN override or, failing that,
// from ~/.codex/auth.json — read-only, always: qmeter never writes to a
// vendor store and never refreshes a token.
package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Harrison-Blair/qmeter/internal/lib/httpx"
	"github.com/Harrison-Blair/qmeter/internal/provider"
)

var _ provider.Provider = (*Provider)(nil)

const (
	// providerID is the name qmeter knows this provider by.
	providerID = "codex"

	// tool is the vendor CLI name used in every "open <tool>" hint.
	tool = "codex"

	// defaultBaseURL is the origin of the usage endpoint. Tests point this
	// at an httptest server.
	defaultBaseURL = "https://chatgpt.com"

	// usagePath is the usage endpoint's path under the base URL.
	usagePath = "/backend-api/wham/usage"
)

// Provider reads Codex usage limits. Use New to build one.
type Provider struct {
	credentialPath string
	baseURL        string
	client         *http.Client
	now            func() time.Time
}

// Option configures a Provider.
type Option func(*Provider)

// WithCredentialPath overrides the path of the Codex auth store. Tests point
// it at a fixture or a temp dir.
func WithCredentialPath(path string) Option {
	return func(p *Provider) { p.credentialPath = path }
}

// WithBaseURL overrides the origin of the usage endpoint. Tests point it at
// an httptest server.
func WithBaseURL(baseURL string) Option {
	return func(p *Provider) { p.baseURL = baseURL }
}

// WithHTTPClient overrides the http.Client used for the usage request.
func WithHTTPClient(c *http.Client) Option {
	return func(p *Provider) { p.client = c }
}

// WithClock overrides the source of "now", which turns a relative
// resets_in_seconds into an absolute reset time.
func WithClock(now func() time.Time) Option {
	return func(p *Provider) { p.now = now }
}

// New builds a Codex provider.
func New(opts ...Option) *Provider {
	p := &Provider{
		baseURL: defaultBaseURL,
		now:     time.Now,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// ID returns "codex".
func (p *Provider) ID() string { return providerID }

// Detect reports whether a Codex credential is resolvable without any
// network I/O. It is true when QMETER_CODEX_TOKEN is set, or when
// ~/.codex/auth.json exists, parses, and holds a ChatGPT access token — an
// expired token still counts as detected, since only the endpoint can say so
// and Fetch reports that as provider.ErrTokenExpired.
//
// An API-key store counts as detected too: the file exists and parses,
// which is the whole of the Detect contract. That its sign-in mode has no
// usage endpoint is Fetch's to report, so codex shows an explanatory line
// rather than vanishing from the default listing.
func (p *Provider) Detect(ctx context.Context) (bool, string) {
	_, err := p.resolveCredential(ctx)
	switch {
	case err == nil, errors.Is(err, errAPIKeyMode):
		return true, ""
	default:
		return false, detectReason(err)
	}
}

// detectReason renders an error as a Detect reason: final user-facing text
// with no provider-name prefix. Typed errors are read out of the chain with
// errors.As so the wrapping never leaks into the message.
func detectReason(err error) string {
	var expired provider.ErrTokenExpired
	if errors.As(err, &expired) {
		return expired.Error()
	}
	var notLoggedIn provider.ErrNotLoggedIn
	if errors.As(err, &notLoggedIn) {
		return notLoggedIn.Error()
	}
	return err.Error()
}

// Fetch retrieves the current Codex usage windows.
//
// The main rate limit contributes up to two windows and every additional
// rate limit up to two more; a window the response does not report a
// percentage for is skipped rather than shown as 0%.
func (p *Provider) Fetch(ctx context.Context) ([]provider.Window, error) {
	cred, err := p.resolveCredential(ctx)
	if err != nil {
		if errors.Is(err, errAPIKeyMode) {
			// credstore wraps every non-not-found loader error with
			// "credential store: ", but this message is already the final
			// user-facing sentence, so it is reported without that prefix.
			return nil, errAPIKeyMode
		}
		// Returned as-is: the message is already final, prefix-free text
		// (internal/usage renders untyped errors verbatim) and the typed
		// errors still match errors.Is/errors.As.
		return nil, err
	}

	headers := map[string]string{
		"Authorization": "Bearer " + cred.AccessToken,
		"Accept":        "application/json",
	}
	if cred.AccountID != "" {
		headers["ChatGPT-Account-Id"] = cred.AccountID
	}

	var resp usageResponse
	err = httpx.Get(ctx, httpx.Options{
		URL:     p.usageURL(),
		Tool:    tool,
		Headers: headers,
		Client:  p.client,
	}, &resp)
	if err != nil {
		return nil, fmt.Errorf("fetch usage: %w", err)
	}
	// A response that mentions no rate limit at all — a bare null, an empty
	// object, credits only — is a broken or unexpected shape, not an account
	// with nothing to report; saying so beats printing nothing.
	if !resp.HasRateLimit && !resp.HasAdditional {
		return nil, errNoRateLimits
	}
	return p.windows(resp, cred), nil
}

// usageURL is the usage endpoint under the configured base URL.
func (p *Provider) usageURL() string {
	return strings.TrimSuffix(p.baseURL, "/") + usagePath
}

// windows normalizes a usage response, in a stable order: the main limit's
// primary then secondary window, then each additional limit's, in the order
// the response listed them.
func (p *Provider) windows(resp usageResponse, cred credential) []provider.Window {
	// The response is the authority on the plan name; the store's id_token
	// claim is the fallback, and an env override has neither.
	plan := resp.PlanType
	if plan == "" {
		plan = cred.Plan
	}
	now := p.now()

	var out []provider.Window
	add := func(limit limitWindow, name string) {
		if w, ok := limit.normalize(name, plan, now); ok {
			out = append(out, w)
		}
	}

	// The main limit is the one the user thinks of as "their" limit, so it
	// gets the friendlier name its window length implies (300 minutes ->
	// "5h", 10080 -> "weekly"), matching how the other providers name their
	// windows; "primary"/"secondary" remain for lengths with no such name.
	add(resp.RateLimit.Primary, windowName(resp.RateLimit.Primary.period(), "primary"))
	add(resp.RateLimit.Secondary, windowName(resp.RateLimit.Secondary.period(), "secondary"))

	// Additional limits are per-feature, so their own name carries the
	// meaning and the position stays literal.
	for _, extra := range resp.Additional {
		name := extra.displayName()
		primary, secondary := extra.windows()
		add(primary, additionalName(name, "primary"))
		add(secondary, additionalName(name, "secondary"))
	}
	return out
}

// windowName derives a display name from a window length, falling back to
// the given position name for a length with no idiomatic name (including an
// unreported one).
func windowName(period time.Duration, fallback string) string {
	switch {
	case period <= 0:
		return fallback
	case period == 24*time.Hour:
		return "daily"
	case period == 7*24*time.Hour:
		return "weekly"
	case period == 30*24*time.Hour:
		return "monthly"
	case period%time.Hour == 0:
		return fmt.Sprintf("%dh", period/time.Hour)
	case period%time.Minute == 0:
		return fmt.Sprintf("%dm", period/time.Minute)
	default:
		return fallback
	}
}

// additionalName names one window of an additional rate limit, e.g.
// "gpt-5-codex primary".
func additionalName(name, position string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return position
	}
	return name + " " + position
}

// --- usage response ---------------------------------------------------------

// errNoRateLimits reports a usage response that carried neither a main rate
// limit nor any additional ones. Worded as final user-facing text, since
// internal/usage renders an untyped error verbatim.
var errNoRateLimits = errors.New("usage response carried no rate limits")

// usageResponse is the part of GET /backend-api/wham/usage qmeter reads.
// "credits" and anything else the endpoint adds are ignored.
type usageResponse struct {
	PlanType string

	RateLimit    rateLimit
	HasRateLimit bool

	Additional    []additionalLimit
	HasAdditional bool
}

// UnmarshalJSON decodes the response tolerantly: unknown fields are ignored,
// and a field whose type has drifted is skipped rather than failing the
// whole fetch, since the remaining windows are still worth showing.
func (r *usageResponse) UnmarshalJSON(data []byte) error {
	return decodeObject(data, func(key string, raw json.RawMessage) {
		switch key {
		case "plantype":
			r.PlanType = decodeString(raw)
		case "ratelimit":
			if !isJSONNull(raw) && json.Unmarshal(raw, &r.RateLimit) == nil {
				r.HasRateLimit = true
			}
		case "additionalratelimits":
			if !isJSONNull(raw) && json.Unmarshal(raw, &r.Additional) == nil {
				r.HasAdditional = true
			}
		}
	})
}

// rateLimit is the main rate limit's pair of windows.
type rateLimit struct {
	Primary   limitWindow
	Secondary limitWindow
}

// UnmarshalJSON decodes the window pair tolerantly.
func (l *rateLimit) UnmarshalJSON(data []byte) error {
	return decodeObject(data, func(key string, raw json.RawMessage) {
		switch key {
		case "primarywindow":
			_ = json.Unmarshal(raw, &l.Primary)
		case "secondarywindow":
			_ = json.Unmarshal(raw, &l.Secondary)
		}
	})
}

// additionalLimit is one named per-feature rate limit.
//
// Two spellings are accepted. The reference table's is a flat entry —
// {name, primary_window, secondary_window} — while the endpoint currently
// sends {limit_name, metered_feature, rate_limit: {primary_window,
// secondary_window}}, with the windows one level down. Both are read, so
// neither a rollback nor a further rename silently drops these limits.
type additionalLimit struct {
	Name           string // "name" (reference spelling)
	LimitName      string // "limit_name" (live spelling)
	MeteredFeature string // "metered_feature" (live fallback name)

	Primary      limitWindow
	HasPrimary   bool
	Secondary    limitWindow
	HasSecondary bool

	// Nested holds the windows the live shape puts under the entry's own
	// "rate_limit" object.
	Nested    rateLimit
	HasNested bool
}

// UnmarshalJSON decodes one additional limit tolerantly.
func (a *additionalLimit) UnmarshalJSON(data []byte) error {
	return decodeObject(data, func(key string, raw json.RawMessage) {
		switch key {
		case "name":
			a.Name = decodeString(raw)
		case "limitname":
			a.LimitName = decodeString(raw)
		case "meteredfeature":
			a.MeteredFeature = decodeString(raw)
		case "primarywindow":
			if !isJSONNull(raw) && json.Unmarshal(raw, &a.Primary) == nil {
				a.HasPrimary = true
			}
		case "secondarywindow":
			if !isJSONNull(raw) && json.Unmarshal(raw, &a.Secondary) == nil {
				a.HasSecondary = true
			}
		case "ratelimit":
			if !isJSONNull(raw) && json.Unmarshal(raw, &a.Nested) == nil {
				a.HasNested = true
			}
		}
	})
}

// displayName is the name this limit's windows are labelled with, in
// precedence order: the reference spelling "name", then the live
// "limit_name", then "metered_feature" for an entry that leaves its name
// empty. All blank yields "", and additionalName then falls back to the bare
// position.
func (a additionalLimit) displayName() string {
	for _, candidate := range []string{a.Name, a.LimitName, a.MeteredFeature} {
		if name := strings.TrimSpace(candidate); name != "" {
			return name
		}
	}
	return ""
}

// windows returns this limit's two windows: the ones the entry carries
// directly when it has them, otherwise the ones nested under its own
// rate_limit object. Each position falls back independently, so a response
// that mixes the two spellings still yields both windows.
func (a additionalLimit) windows() (primary, secondary limitWindow) {
	primary, secondary = a.Primary, a.Secondary
	if !a.HasPrimary && a.HasNested {
		primary = a.Nested.Primary
	}
	if !a.HasSecondary && a.HasNested {
		secondary = a.Nested.Secondary
	}
	return primary, secondary
}

// limitWindow is one window of a rate limit. Every field is optional, so
// presence is tracked rather than inferred from a zero value: 0% used and
// "not reported" are different things.
//
// Each quantity has both the reference table's key name and the one the
// endpoint currently sends; see resetsAt and period for the precedence when
// a response carries both.
type limitWindow struct {
	UsedPercent *float64

	ResetsAt    time.Time // "resets_at" (reference spelling)
	HasResetsAt bool
	ResetAt     time.Time // "reset_at" (live spelling)
	HasResetAt  bool

	ResetsIn   *float64 // "resets_in_seconds" (reference spelling)
	ResetAfter *float64 // "reset_after_seconds" (live spelling)

	WindowMinutes      *float64 // "window_minutes" (reference spelling)
	WindowSeconds      *float64 // "window_seconds" (reference spelling)
	LimitWindowSeconds *float64 // "limit_window_seconds" (live spelling)
}

// UnmarshalJSON decodes one window tolerantly.
func (w *limitWindow) UnmarshalJSON(data []byte) error {
	return decodeObject(data, func(key string, raw json.RawMessage) {
		switch key {
		case "usedpercent":
			w.UsedPercent = decodeNumber(raw)
		case "resetsat":
			if t, ok := decodeTimestamp(raw); ok {
				w.ResetsAt, w.HasResetsAt = t, true
			}
		case "resetat":
			if t, ok := decodeTimestamp(raw); ok {
				w.ResetAt, w.HasResetAt = t, true
			}
		case "resetsinseconds":
			w.ResetsIn = decodeNumber(raw)
		case "resetafterseconds":
			w.ResetAfter = decodeNumber(raw)
		case "windowminutes":
			w.WindowMinutes = decodeNumber(raw)
		case "windowseconds":
			w.WindowSeconds = decodeNumber(raw)
		case "limitwindowseconds":
			w.LimitWindowSeconds = decodeNumber(raw)
		}
	})
}

// normalize turns one window into a provider.Window. It reports false when
// the response gave no percentage for the window, which is how a window the
// account does not have is skipped.
func (w limitWindow) normalize(name, plan string, now time.Time) (provider.Window, bool) {
	if w.UsedPercent == nil {
		return provider.Window{}, false
	}
	out := provider.Window{
		Provider:    providerID,
		Name:        name,
		Plan:        plan,
		UsedPercent: *w.UsedPercent,
		Period:      w.period(),
		// The endpoint has no explicit rate-limited flag; a window with
		// nothing left is exactly what that flag means.
		RateLimited: *w.UsedPercent >= 100,
	}
	if resetsAt, ok := w.resetsAt(now); ok {
		out.ResetsAt = resetsAt
	}
	return out, true
}

// resetsAt resolves when this window next resets. An absolute reset time
// beats a relative one, since it survives a slow request; within each pair
// the reference table's spelling is tried before the live one. The two
// spellings are expected to agree, so this order only has to be stable.
func (w limitWindow) resetsAt(now time.Time) (time.Time, bool) {
	switch {
	case w.HasResetsAt:
		return w.ResetsAt, true
	case w.HasResetAt:
		return w.ResetAt, true
	case w.ResetsIn != nil:
		return now.Add(seconds(*w.ResetsIn)), true
	case w.ResetAfter != nil:
		return now.Add(seconds(*w.ResetAfter)), true
	default:
		return time.Time{}, false
	}
}

// period is the window length. When a response carries more than one
// spelling, the reference table's window_minutes wins, then its
// window_seconds, then the live limit_window_seconds; they are expected to
// agree, so this order only has to be stable.
func (w limitWindow) period() time.Duration {
	switch {
	case w.WindowMinutes != nil:
		return time.Duration(*w.WindowMinutes * float64(time.Minute))
	case w.WindowSeconds != nil:
		return seconds(*w.WindowSeconds)
	case w.LimitWindowSeconds != nil:
		return seconds(*w.LimitWindowSeconds)
	default:
		return 0
	}
}

// seconds turns a possibly fractional count of seconds into a Duration.
func seconds(v float64) time.Duration {
	return time.Duration(v * float64(time.Second))
}

// --- tolerant JSON decoding -------------------------------------------------
//
// Codex's field names have already drifted between snake_case and camelCase,
// and both have to keep working. Rather than declaring every field twice,
// this package matches keys by their normalized form, so "used_percent",
// "usedPercent" and "UsedPercent" all land on the same branch, and any key it
// does not recognize is ignored.

// normalizeKey folds a JSON key to the form this package switches on:
// lowercase, with word separators removed.
func normalizeKey(key string) string {
	var b strings.Builder
	b.Grow(len(key))
	for _, r := range key {
		switch r {
		case '_', '-', ' ':
			continue
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// forEachField calls fn once per field of a decoded JSON object, with the
// key normalized.
func forEachField(obj map[string]json.RawMessage, fn func(key string, raw json.RawMessage)) {
	for key, raw := range obj {
		fn(normalizeKey(key), raw)
	}
}

// decodeObject decodes a JSON object and calls fn for each of its fields,
// with the key normalized. A value that is not an object (null included)
// simply yields no fields.
func decodeObject(data []byte, fn func(key string, raw json.RawMessage)) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	forEachField(obj, fn)
	return nil
}

// decodeString reads a JSON string, yielding "" for any other shape rather
// than failing the whole response.
func decodeString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// isJSONNull reports whether a raw value is an explicit null, which this
// package reads as "the endpoint said nothing here" rather than as a value.
func isJSONNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

// decodeNumber reads a JSON number, also accepting one spelled as a string,
// and yields nil for any other shape.
func decodeNumber(raw json.RawMessage) *float64 {
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return &f
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return &f
		}
	}
	return nil
}

// decodeTimestamp reads a reset time in any of the spellings this endpoint
// has used: an ISO-8601 string, an epoch number, or an epoch in a string.
func decodeTimestamp(raw json.RawMessage) (time.Time, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t, true
		}
	}
	if n := decodeNumber(raw); n != nil {
		return epochTime(*n), true
	}
	return time.Time{}, false
}

// epochTime reads an epoch as seconds, or as milliseconds when the value is
// far too large to be seconds (anything past the year 5138).
func epochTime(v float64) time.Time {
	const millisecondsThreshold = 1e11
	if math.Abs(v) >= millisecondsThreshold {
		return time.UnixMilli(int64(v)).UTC()
	}
	secs := math.Trunc(v)
	return time.Unix(int64(secs), int64((v-secs)*float64(time.Second))).UTC()
}
