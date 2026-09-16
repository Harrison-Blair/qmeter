// Package usage orchestrates the providers behind `qmeter usage`: it applies
// the --provider filter, detects credentials, fetches every detected
// provider concurrently under a per-provider timeout, and maps provider
// errors to the exact user-facing message text. It does no rendering — the
// text and JSON renderers consume Result.
package usage

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// DefaultProviderTimeout is the per-provider deadline Run gives each Fetch.
// It is owned by this package; internal/lib/httpx takes its deadline from
// the context it is passed and sets none of its own.
const DefaultProviderTimeout = 10 * time.Second

// ProviderError is one provider's failure or not-detected reason, already
// mapped to the text the renderer prints verbatim (no provider-name prefix:
// Provider carries that separately).
type ProviderError struct {
	Provider string
	Message  string
}

// Result is everything one `qmeter usage` run produced.
type Result struct {
	// Windows holds the usage windows of every provider that fetched
	// successfully, in the order the providers were passed to Run (never
	// completion order), and within a provider in its declared order.
	Windows []provider.Window

	// Errors holds one entry per detected provider whose Fetch failed or
	// timed out, in the order the providers were passed to Run.
	Errors []ProviderError

	// Undetected is non-empty only when only names a provider that exists
	// but was not detected; it then holds exactly one entry carrying
	// Detect's reason. Undetected providers are silently omitted from the
	// default all-providers listing.
	Undetected []ProviderError

	// UnknownProvider reports that only named a provider no element of
	// providers claims — i.e. `--provider` was given a name that does not
	// exist at all. Every other field is then empty and no provider was
	// detected or fetched. This is the one condition the command layer
	// turns into exit 1; an existing-but-undetected provider (Undetected)
	// still exits 0.
	UnknownProvider bool
}

// Run detects, fetches and collects usage for providers. When only is
// non-empty it restricts the run to the provider whose ID() equals only.
//
// Every Fetch runs on its own goroutine under a context derived from ctx
// and bounded by DefaultProviderTimeout, and Run waits for all of them
// before returning. A Provider must therefore return promptly once the
// context it was given is done — one that ignores its context holds up the
// whole run, since Run cannot abandon a goroutine still writing its result.
// Canceling ctx cancels every in-flight Fetch.
func Run(ctx context.Context, providers []provider.Provider, only string) Result {
	return run(ctx, providers, only, DefaultProviderTimeout)
}

// run is Run with an injectable per-provider timeout so tests need not wait
// DefaultProviderTimeout.
func run(ctx context.Context, providers []provider.Provider, only string, timeout time.Duration) Result {
	var res Result

	candidates := providers
	if only != "" {
		candidates = nil
		for _, p := range providers {
			if p.ID() == only {
				candidates = append(candidates, p)
			}
		}
		if len(candidates) == 0 {
			res.UnknownProvider = true
			return res
		}
	}

	// Detect is contractually free of network I/O, so it runs inline; only
	// Fetch is worth a goroutine.
	detected := make([]provider.Provider, 0, len(candidates))
	for _, p := range candidates {
		ok, reason := p.Detect(ctx)
		if !ok {
			// An undetected provider is silently skipped in the default
			// listing; only an explicit filter surfaces its reason.
			if only != "" {
				res.Undetected = append(res.Undetected, ProviderError{Provider: p.ID(), Message: reason})
			}
			continue
		}
		detected = append(detected, p)
	}

	// Each Fetch writes its own slot, so results stay in input order however
	// the goroutines interleave, and one slow or failing provider never
	// holds up another.
	type outcome struct {
		windows []provider.Window
		err     error
	}
	outcomes := make([]outcome, len(detected))

	var wg sync.WaitGroup
	for i, p := range detected {
		wg.Add(1)
		go func(slot int, p provider.Provider) {
			defer wg.Done()
			// A panicking provider must cost only its own result, not the
			// whole process and its siblings' output.
			defer func() {
				if r := recover(); r != nil {
					outcomes[slot] = outcome{err: fmt.Errorf("provider panicked: %v", r)}
				}
			}()
			fetchCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			windows, err := p.Fetch(fetchCtx)
			outcomes[slot] = outcome{windows: windows, err: err}
		}(i, p)
	}
	wg.Wait()

	for i, p := range detected {
		if err := outcomes[i].err; err != nil {
			res.Errors = append(res.Errors, ProviderError{Provider: p.ID(), Message: message(ctx, err, timeout)})
			continue
		}
		for _, w := range outcomes[i].windows {
			// A provider may leave Provider empty; the orchestrator knows
			// which provider it asked, so it fills the name in.
			if w.Provider == "" {
				w.Provider = p.ID()
			}
			res.Windows = append(res.Windows, w)
		}
	}
	return res
}

// message maps a Fetch error to the exact text the renderer prints. Typed
// provider errors are extracted with errors.As and their Error() is called
// on the extracted value, never on the wrapping error, so a provider's
// "claude: fetch usage: " style prefix never leaks into the output. ctx is
// the caller's context, consulted so a caller-side cancellation or deadline
// is never misreported as this package's per-provider timeout.
func message(ctx context.Context, err error, timeout time.Duration) string {
	var expired provider.ErrTokenExpired
	if errors.As(err, &expired) {
		return expired.Error()
	}
	var notLoggedIn provider.ErrNotLoggedIn
	if errors.As(err, &notLoggedIn) {
		return notLoggedIn.Error()
	}
	var rateLimited provider.ErrRateLimited
	if errors.As(err, &rateLimited) {
		// Error() would render a missing retry-after as "retry in 0s",
		// which reads as "retry immediately" — the opposite of the truth.
		if rateLimited.RetryAfter == 0 {
			return "rate limited, retry later"
		}
		return rateLimited.Error()
	}
	// The caller gave up: their cancellation or deadline reached the
	// provider through the derived context, so this is not our timeout.
	if ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		return "canceled"
	}
	if ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
		return fmt.Sprintf("timed out after %s", timeout)
	}
	return err.Error()
}
