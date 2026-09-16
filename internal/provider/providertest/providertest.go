// Package providertest provides a scriptable fake implementing
// provider.Provider, shared by the orchestrator's tests and every provider
// package's tests. It imports only internal/provider so no other package
// needs its own fake.
package providertest

import (
	"context"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// Fake is a scriptable provider.Provider. Prefer the constructors below
// (Succeeding, Erroring, Slow, Undetected) over building one by hand.
type Fake struct {
	// IDValue is returned by ID().
	IDValue string

	// DetectOK and DetectReason are returned by Detect().
	DetectOK     bool
	DetectReason string

	// Windows and FetchErr are returned by Fetch(), after Delay elapses (or
	// the context is done, whichever comes first). When FetchErr is set,
	// Fetch returns it instead of Windows.
	Windows  []provider.Window
	FetchErr error
	Delay    time.Duration
}

// ID returns IDValue.
func (f *Fake) ID() string { return f.IDValue }

// Detect returns DetectOK, DetectReason.
func (f *Fake) Detect(ctx context.Context) (bool, string) {
	return f.DetectOK, f.DetectReason
}

// Fetch waits Delay (or until ctx is done, whichever is first), then
// returns FetchErr if set, else Windows.
func (f *Fake) Fetch(ctx context.Context) ([]provider.Window, error) {
	if f.Delay > 0 {
		timer := time.NewTimer(f.Delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.FetchErr != nil {
		return nil, f.FetchErr
	}
	return f.Windows, nil
}

// Succeeding returns a Fake that is detected and whose Fetch immediately
// succeeds with windows.
func Succeeding(id string, windows []provider.Window) *Fake {
	return &Fake{IDValue: id, DetectOK: true, Windows: windows}
}

// Erroring returns a Fake that is detected but whose Fetch immediately
// fails with err.
func Erroring(id string, err error) *Fake {
	return &Fake{IDValue: id, DetectOK: true, FetchErr: err}
}

// Slow returns a Fake that is detected and whose Fetch waits delay (or the
// caller's context, whichever comes first) before returning windows.
func Slow(id string, delay time.Duration, windows []provider.Window) *Fake {
	return &Fake{IDValue: id, DetectOK: true, Delay: delay, Windows: windows}
}

// Undetected returns a Fake whose Detect reports false with reason.
func Undetected(id, reason string) *Fake {
	return &Fake{IDValue: id, DetectOK: false, DetectReason: reason}
}
