package usage

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// RenderText writes r as the human-readable table described by the layout
// contract on renderText below, counting the RESETS column down from
// time.Now().
//
// It formats only what Run already decided: every message in r.Errors and
// r.Undetected is final text and is printed verbatim, never re-wrapped.
func RenderText(w io.Writer, r Result) error {
	return renderText(w, r, time.Now())
}

// renderText is RenderText with an injectable "now" so the golden test is
// deterministic.
//
// Layout contract — the fixed table layout this package renders, asserted by
// the golden test in render_test.go: the writer
// is tabwriter.NewWriter(out, 0, 8, 2, ' ', 0); data rows are written as
// "provider\twindow\tplan\tremaining\tresets\n" so the RESETS cell — the last
// cell on the line, and therefore never tab-terminated — is neither padded
// nor counted towards any column width; failure and not-detected rows are
// two-cell "provider\tmessage\n" lines, which keeps their long message out
// of the WINDOW column's width while still padding their PROVIDER cell to
// the widest provider name printed.
func renderText(w io.Writer, r Result, now time.Time) error {
	if len(r.Windows) == 0 && len(r.Errors) == 0 && len(r.Undetected) == 0 {
		_, err := fmt.Fprintln(w, "no providers detected")
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 8, 2, ' ', 0)
	if _, err := fmt.Fprint(tw, "PROVIDER\tWINDOW\tPLAN\tREMAINING\tRESETS\n"); err != nil {
		return err
	}
	for _, win := range r.Windows {
		plan := win.Plan
		if plan == "" {
			plan = "-"
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%.1f%%\t%s\n",
			win.Provider, win.Name, plan, win.RemainingPercent, resetsCell(win, now)); err != nil {
			return err
		}
	}
	for _, e := range r.Errors {
		if _, err := fmt.Fprintf(tw, "%s\terror: %s\n", e.Provider, e.Message); err != nil {
			return err
		}
	}
	for _, u := range r.Undetected {
		if _, err := fmt.Fprintf(tw, "%s\tnot detected: %s\n", u.Provider, u.Message); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// resetsCell renders a window's whole RESETS cell: the countdown, plus the
// two-space-separated "(rate limited)" suffix when the window is rate
// limited. It is one cell so the suffix never becomes a column of its own.
func resetsCell(w provider.Window, now time.Time) string {
	cell := "-"
	if !w.ResetsAt.IsZero() {
		cell = "in " + formatResets(w.ResetsAt.Sub(now))
	}
	if w.RateLimited {
		cell += "  (rate limited)"
	}
	return cell
}

// formatResets renders d as its largest two non-zero units: "<d>d<h>h",
// "<h>h<m>m" or "<m>m", dropping a trailing zero unit ("12d", not "12d0h").
// A duration that has already elapsed, or is shorter than a minute, is
// "0m" — the window is due, which is not the same as having no reset time
// at all (that renders as "-", see resetsCell).
func formatResets(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	days := int64(d / (24 * time.Hour))
	hours := int64(d/time.Hour) % 24
	minutes := int64(d/time.Minute) % 60

	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("%dd%dh", days, hours)
	case days > 0:
		return fmt.Sprintf("%dd", days)
	case hours > 0 && minutes > 0:
		return fmt.Sprintf("%dh%dm", hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

// jsonEnvelope is the --json wire form: three keys, always present, never
// null. Windows marshal through provider.Window's own MarshalJSON.
type jsonEnvelope struct {
	Windows    []provider.Window `json:"windows"`
	Errors     []jsonError       `json:"errors"`
	Undetected []jsonUndetected  `json:"undetected"`
}

// jsonError is one failed provider: ProviderError.Message under "message".
type jsonError struct {
	Provider string `json:"provider"`
	Message  string `json:"message"`
}

// jsonUndetected is one undetected provider: ProviderError.Message under
// "reason", since for these the message is Detect's reason.
type jsonUndetected struct {
	Provider string `json:"provider"`
	Reason   string `json:"reason"`
}

// RenderJSON writes r as the JSON envelope this package's callers consume —
// {"windows":[...],"errors":[...],"undetected":[...]} — with all three keys
// always present, every array non-null even when Result's slices are nil,
// and one trailing newline.
func RenderJSON(w io.Writer, r Result) error {
	env := jsonEnvelope{
		Windows:    r.Windows,
		Errors:     make([]jsonError, 0, len(r.Errors)),
		Undetected: make([]jsonUndetected, 0, len(r.Undetected)),
	}
	if env.Windows == nil {
		env.Windows = []provider.Window{}
	}
	for _, e := range r.Errors {
		env.Errors = append(env.Errors, jsonError{Provider: e.Provider, Message: e.Message})
	}
	for _, u := range r.Undetected {
		env.Undetected = append(env.Undetected, jsonUndetected{Provider: u.Provider, Reason: u.Message})
	}
	enc := json.NewEncoder(w)
	// Messages carry credential paths and provider reasons, not HTML: a
	// path containing & must stay readable rather than become &.
	enc.SetEscapeHTML(false)
	return enc.Encode(env)
}
