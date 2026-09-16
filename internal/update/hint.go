package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/version"
)

// NoCheckEnv disables the passive update check when set to any non-empty
// value.
const NoCheckEnv = "QMETER_NO_UPDATE_CHECK"

// hintTTL is how long a recorded check stands before another one is made:
// at most one network request a day, whatever the outcome of the last one.
const hintTTL = 24 * time.Hour

// hintTimeout is the whole budget for the passive check. It is short on
// purpose — the hint is a courtesy and must never hold up `qmeter usage`.
const hintTimeout = 2 * time.Second

// HintOptions are the inputs to Hint. Like Options, everything external is
// a field so the hint is testable without a network, a clock or a real
// cache directory.
type HintOptions struct {
	// Current is the running version; empty means version.String().
	Current string

	// BaseURL is the release host; empty means DefaultBaseURL.
	BaseURL string

	// Client is the http.Client to use; nil means http.DefaultClient.
	Client *http.Client

	// CachePath is the state file; empty means DefaultCachePath().
	CachePath string

	// Now is the clock; nil means time.Now.
	Now func() time.Time

	// Getenv reads the environment; nil means os.Getenv.
	Getenv func(string) string
}

// hintState is the on-disk record of the last check.
type hintState struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
}

// DefaultCachePath is where the passive check records its state.
func DefaultCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("update: locate the user cache directory: %w", err)
	}
	return filepath.Join(dir, "qmeter", "update-check.json"), nil
}

// Hint writes a single "an update is available" line to w, or nothing.
//
// It is best-effort by construction and returns nothing: no cache
// directory, no network, a malformed state file, an unknown running
// version — every one of those is silence, never an error and never a
// change to the caller's exit code. At most one network request is made per
// hintTTL; in between, the answer comes from the state file.
func Hint(ctx context.Context, w io.Writer, opts HintOptions) {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	if getenv(NoCheckEnv) != "" {
		return
	}
	if ctx.Err() != nil {
		return
	}

	current := opts.Current
	if current == "" {
		current = version.String()
	}
	// A dev build has no place on the release line; comparing it would
	// nag on every run.
	if _, err := ParseVersion(current); err != nil {
		return
	}

	path := opts.CachePath
	if path == "" {
		var err error
		if path, err = DefaultCachePath(); err != nil {
			return
		}
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}

	latest := ""
	state, err := readHintState(path)
	switch {
	case err == nil && now().Sub(state.CheckedAt) < hintTTL:
		latest = state.Latest
	default:
		checkCtx, cancel := context.WithTimeout(ctx, hintTimeout)
		defer cancel()
		src := Source{BaseURL: opts.BaseURL, Client: opts.Client}
		// A failed lookup is still recorded, with an empty latest, so a
		// broken network or an unreleased repo backs off for a day
		// instead of costing every run the timeout.
		latest, _ = src.LatestTag(checkCtx)
		writeHintState(path, hintState{CheckedAt: now(), Latest: latest})
	}

	if latest == "" {
		return
	}
	if cmp, err := CompareVersions(current, latest); err != nil || cmp >= 0 {
		return
	}
	fmt.Fprintln(w, availableLine(latest, current))
}

// availableLine is the one line both --check and the passive hint use to
// say a newer release exists.
func availableLine(latest, current string) string {
	return fmt.Sprintf("qmeter %s is available (you have %s); run qmeter update", latest, current)
}

func readHintState(path string) (hintState, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return hintState{}, err
	}
	var s hintState
	if err := json.Unmarshal(b, &s); err != nil {
		return hintState{}, err
	}
	if s.CheckedAt.IsZero() {
		return hintState{}, fmt.Errorf("update: %s has no checked_at", path)
	}
	return s, nil
}

// writeHintState records a check. Failures are ignored: an unwritable cache
// costs a request per run, which is still better than a visible error.
func writeHintState(path string, s hintState) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	b, err := json.Marshal(s)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, b, 0o600)
}
