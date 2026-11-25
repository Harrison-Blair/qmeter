package clirefresh

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/lib/subprocess"
	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// runFunc adapts a function to subprocess.Runner.
type runFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

func (f runFunc) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return f(ctx, name, args...)
}

func TestRun_SuccessRunsTheCommandOnce(t *testing.T) {
	fake := subprocess.NewFake()
	fake.Script([]byte("ok"), nil, "claude", "auth", "status")

	if err := Run(context.Background(), fake, "claude", "auth", "status"); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	calls := fake.Calls()
	if len(calls) != 1 || calls[0].Name != "claude" || strings.Join(calls[0].Args, " ") != "auth status" {
		t.Fatalf("calls = %+v, want one call of claude auth status", calls)
	}
}

func TestRun_SetsA30SecondLimit(t *testing.T) {
	var remaining time.Duration
	r := runFunc(func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("the command context has no deadline")
		}
		remaining = time.Until(deadline)
		return nil, nil
	})
	if err := Run(context.Background(), r, "codex", "doctor"); err != nil {
		t.Fatal(err)
	}
	if remaining <= 29*time.Second || remaining > 30*time.Second {
		t.Fatalf("time left on the deadline = %s, want about 30s", remaining)
	}
}

func TestRun_FailureNamesTheReason(t *testing.T) {
	exitErr := exitError(t)
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"not found", &exec.Error{Name: "claude", Err: exec.ErrNotFound}, "claude not found"},
		{"non-zero exit", exitErr, "claude auth status exited with status 3"},
		{"other", errors.New("permission denied"), "claude auth status: permission denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runFunc(func(context.Context, string, ...string) ([]byte, error) { return nil, tc.err })
			err := Run(context.Background(), r, "claude", "auth", "status")
			if err == nil || err.Error() != tc.want {
				t.Fatalf("Run() error = %v, want %q", err, tc.want)
			}
			if !errors.Is(err, tc.err) {
				t.Fatalf("Run() error does not wrap the cause %v", tc.err)
			}
		})
	}
}

func TestRun_TimeoutNamesTheLimit(t *testing.T) {
	r := runFunc(func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	err := run(context.Background(), r, time.Millisecond, "codex", "doctor")
	if err == nil || err.Error() != "codex doctor timed out after 1ms" {
		t.Fatalf("run() error = %v, want the limit named", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("run() error = %v, want it to wrap context.DeadlineExceeded", err)
	}
}

func TestRun_CallerCancellationIsNotATimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := runFunc(func(ctx context.Context, _ string, _ ...string) ([]byte, error) { return nil, ctx.Err() })
	err := Run(ctx, r, "codex", "doctor")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if strings.Contains(err.Error(), "timed out") {
		t.Fatalf("Run() error = %q reports a timeout for a caller cancellation", err)
	}
}

// exitError returns a real *exec.ExitError with exit status 3.
func exitError(t *testing.T) error {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh to produce a real exit error")
	}
	_, err = exec.Command(sh, "-c", "exit 3").Output()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("sh -c 'exit 3' error = %v, want *exec.ExitError", err)
	}
	return err
}

func TestAnnotate(t *testing.T) {
	refreshErr := &Error{Reason: "claude not found"}
	expired := fmt.Errorf("usage request: %w", provider.ErrTokenExpired{Tool: "claude"})
	live := context.Background()
	done, cancel := context.WithCancel(context.Background())
	cancel()
	canceledRefresh := &Error{Reason: "claude auth status: context canceled", Err: context.Canceled}

	for _, tc := range []struct {
		name       string
		ctx        context.Context
		err        error
		refreshErr error
		want       string
	}{
		{"adds the reason", live, expired, refreshErr, "token expired, open claude to refresh (auto-refresh failed: claude not found)"},
		{"no refresh failure", live, expired, nil, "usage request: token expired, open claude to refresh"},
		{"another tool's error", live, provider.ErrTokenExpired{Tool: "pi"}, refreshErr, "token expired, open pi to refresh"},
		{"not a token error", live, errors.New("boom"), refreshErr, "boom"},
		{"context done: the refresh error", done, expired, canceledRefresh, "claude auth status: context canceled"},
		{"context done after the refresh failed: adds the reason", done, expired, refreshErr, "token expired, open claude to refresh (auto-refresh failed: claude not found)"},
		{"context done after the refresh failed: keeps a later request error", done, fmt.Errorf("usage request: %w", context.Canceled), refreshErr, "usage request: context canceled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Annotate(tc.ctx, tc.err, tc.refreshErr, "claude")
			if got.Error() != tc.want {
				t.Fatalf("Annotate() = %q, want %q", got, tc.want)
			}
		})
	}
	if got := Annotate(done, expired, canceledRefresh, "claude"); !errors.Is(got, context.Canceled) {
		t.Fatalf("Annotate() = %v, want it to wrap context.Canceled", got)
	}
}
