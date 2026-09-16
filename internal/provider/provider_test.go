package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

func TestErrTokenExpired_Error_IncludesHint(t *testing.T) {
	err := provider.ErrTokenExpired{Tool: "claude"}

	got := err.Error()
	want := "token expired, open claude to refresh"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestErrNotLoggedIn_Error_IncludesHint(t *testing.T) {
	err := provider.ErrNotLoggedIn{Tool: "codex"}

	got := err.Error()
	want := "not logged in, run codex to log in"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestErrRateLimited_CarriesRetryAfter(t *testing.T) {
	err := provider.ErrRateLimited{RetryAfter: 90 * time.Second}

	if err.RetryAfter != 90*time.Second {
		t.Errorf("RetryAfter = %v, want %v", err.RetryAfter, 90*time.Second)
	}

	got := err.Error()
	want := "rate limited, retry in 1m30s"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestErrors_WrapForErrorsIs(t *testing.T) {
	t.Run("ErrTokenExpired", func(t *testing.T) {
		wrapped := fmt.Errorf("reading credentials: %w", provider.ErrTokenExpired{Tool: "claude"})

		if !errors.Is(wrapped, provider.ErrTokenExpired{}) {
			t.Error("errors.Is(wrapped, ErrTokenExpired{}) = false, want true")
		}
		if errors.Is(wrapped, provider.ErrNotLoggedIn{}) {
			t.Error("errors.Is(wrapped, ErrNotLoggedIn{}) = true, want false")
		}

		var te provider.ErrTokenExpired
		if !errors.As(wrapped, &te) {
			t.Fatal("errors.As(wrapped, &te) = false, want true")
		}
		if te.Tool != "claude" {
			t.Errorf("te.Tool = %q, want %q", te.Tool, "claude")
		}
	})

	t.Run("ErrNotLoggedIn", func(t *testing.T) {
		wrapped := fmt.Errorf("reading credentials: %w", provider.ErrNotLoggedIn{Tool: "codex"})

		if !errors.Is(wrapped, provider.ErrNotLoggedIn{}) {
			t.Error("errors.Is(wrapped, ErrNotLoggedIn{}) = false, want true")
		}

		var nl provider.ErrNotLoggedIn
		if !errors.As(wrapped, &nl) {
			t.Fatal("errors.As(wrapped, &nl) = false, want true")
		}
		if nl.Tool != "codex" {
			t.Errorf("nl.Tool = %q, want %q", nl.Tool, "codex")
		}
	})

	t.Run("ErrRateLimited", func(t *testing.T) {
		wrapped := fmt.Errorf("fetching usage: %w", provider.ErrRateLimited{RetryAfter: 5 * time.Minute})

		if !errors.Is(wrapped, provider.ErrRateLimited{}) {
			t.Error("errors.Is(wrapped, ErrRateLimited{}) = false, want true")
		}

		var rl provider.ErrRateLimited
		if !errors.As(wrapped, &rl) {
			t.Fatal("errors.As(wrapped, &rl) = false, want true")
		}
		if rl.RetryAfter != 5*time.Minute {
			t.Errorf("rl.RetryAfter = %v, want %v", rl.RetryAfter, 5*time.Minute)
		}
	})
}

func TestWindowJSON_OmitsZeroResetsAt(t *testing.T) {
	w := provider.Window{
		Provider:    "claude",
		Name:        "5h",
		UsedPercent: 42.0,
	}

	b, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if _, ok := m["resets_at"]; ok {
		t.Errorf("resets_at present in %s, want omitted", b)
	}
}

func TestWindowJSON_PeriodIsWholeSeconds(t *testing.T) {
	w := provider.Window{
		Provider:    "claude",
		Name:        "5h",
		UsedPercent: 42.0,
		Period:      5 * time.Hour,
	}

	b, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	got, ok := m["period_seconds"]
	if !ok {
		t.Fatalf("period_seconds absent in %s", b)
	}
	if got != float64(18000) {
		t.Errorf("period_seconds = %v, want %v", got, 18000)
	}
}

func TestWindowJSON_OmitsZeroPeriod(t *testing.T) {
	w := provider.Window{
		Provider:    "claude",
		Name:        "5h",
		UsedPercent: 42.0,
	}

	b, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if _, ok := m["period_seconds"]; ok {
		t.Errorf("period_seconds present in %s, want omitted", b)
	}
}

func TestWindowJSON_OmitsEmptyPlan(t *testing.T) {
	w := provider.Window{
		Provider:    "cursor",
		Name:        "total",
		UsedPercent: 4.5,
	}

	b, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if _, ok := m["plan"]; ok {
		t.Errorf("plan present in %s, want omitted", b)
	}
}

func TestWindowJSON_IncludesPlanWhenSet(t *testing.T) {
	w := provider.Window{
		Provider:    "claude",
		Name:        "5h",
		Plan:        "max",
		UsedPercent: 42.0,
	}

	b, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got := m["plan"]; got != "max" {
		t.Errorf("plan = %v, want %q", got, "max")
	}
}

func TestWindowJSON_RateLimitedAndUsedPercentAlwaysPresent(t *testing.T) {
	w := provider.Window{
		Provider:    "opencode-go",
		Name:        "monthly",
		UsedPercent: 0,
		RateLimited: false,
	}

	b, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got, ok := m["rate_limited"]; !ok || got != false {
		t.Errorf("rate_limited = %v, ok=%v, want false, ok=true", got, ok)
	}
	if got, ok := m["used_percent"]; !ok || got != float64(0) {
		t.Errorf("used_percent = %v, ok=%v, want 0, ok=true", got, ok)
	}
}

func TestWindowJSON_ResetsAtIsRFC3339(t *testing.T) {
	resetsAt := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	w := provider.Window{
		Provider:    "claude",
		Name:        "5h",
		UsedPercent: 42.0,
		ResetsAt:    resetsAt,
	}

	b, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	got, ok := m["resets_at"].(string)
	if !ok {
		t.Fatalf("resets_at not a string in %s", b)
	}
	if got != resetsAt.Format(time.RFC3339) {
		t.Errorf("resets_at = %q, want %q", got, resetsAt.Format(time.RFC3339))
	}
}

// compileCheckProvider pins the Provider interface's method set at compile
// time; it is never called.
var _ provider.Provider = compileCheckProvider{}

type compileCheckProvider struct{}

func (compileCheckProvider) ID() string { return "compile-check" }
func (compileCheckProvider) Detect(ctx context.Context) (bool, string) {
	return false, ""
}
func (compileCheckProvider) Fetch(ctx context.Context) ([]provider.Window, error) {
	return nil, nil
}
