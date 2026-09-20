package pace

import (
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// Projection describes exhaustion or the allowance remaining at reset.
type Projection struct {
	State            string
	ExhaustionAt     *time.Time
	RemainingAtReset *float64
}

// Forecast projects whole-window average consumption through the reset.
func Forecast(w provider.Window, now time.Time) Projection {
	if w.ResetsAt.IsZero() || w.Period <= 0 {
		return Projection{State: "none"}
	}
	start := w.ResetsAt.Add(-w.Period)
	if now.Before(start) || !now.Before(w.ResetsAt) {
		return Projection{State: "none"}
	}
	remaining := 0.0
	if w.RemainingPercent <= 0 || w.RateLimited {
		return Projection{State: "empty", ExhaustionAt: &now, RemainingAtReset: &remaining}
	}
	elapsed := float64(now.Sub(start))
	if elapsed < 0.05*float64(w.Period) {
		return Projection{State: "none"}
	}
	used := 100 - w.RemainingPercent
	if used <= 0 {
		remaining = w.RemainingPercent
		return Projection{State: "survives", RemainingAtReset: &remaining}
	}
	left := float64(w.ResetsAt.Sub(now))
	if w.RemainingPercent*elapsed < used*left {
		exhaustion := now.Add(time.Duration(w.RemainingPercent * elapsed / used))
		return Projection{State: "dry", ExhaustionAt: &exhaustion, RemainingAtReset: &remaining}
	}
	remaining = max(0, w.RemainingPercent-used*left/elapsed)
	return Projection{State: "survives", RemainingAtReset: &remaining}
}
