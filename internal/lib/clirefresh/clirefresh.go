// Package clirefresh runs a vendor CLI so that the CLI refreshes the token in
// its own credential store. qmeter then reads the store again; it never writes
// a vendor credential file itself.
package clirefresh

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/lib/subprocess"
	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// Timeout is the time limit for one refresh command.
const Timeout = 30 * time.Second

// Error is a failed refresh. Its message is the reason only, short enough to
// sit inside a one-line usage row; Unwrap gives the cause.
type Error struct {
	Reason string
	Err    error
}

func (e *Error) Error() string { return e.Reason }
func (e *Error) Unwrap() error { return e.Err }

// Run runs name with args through r under Timeout. The command's output is
// discarded. On failure the error names the reason: the binary is not found,
// the time limit expired, or the command exited with a non-zero status.
func Run(ctx context.Context, r subprocess.Runner, name string, args ...string) error {
	return run(ctx, r, Timeout, name, args...)
}

func run(ctx context.Context, r subprocess.Runner, timeout time.Duration, name string, args ...string) error {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	_, err := r.Run(runCtx, name, args...)
	if err == nil {
		return nil
	}
	command := strings.Join(append([]string{name}, args...), " ")
	var exitErr *exec.ExitError
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return &Error{Reason: name + " not found", Err: err}
	case ctx.Err() == nil && errors.Is(runCtx.Err(), context.DeadlineExceeded):
		return &Error{Reason: fmt.Sprintf("%s timed out after %s", command, timeout), Err: errors.Join(err, context.DeadlineExceeded)}
	case errors.As(err, &exitErr):
		return &Error{Reason: fmt.Sprintf("%s exited with status %d", command, exitErr.ExitCode()), Err: err}
	default:
		return &Error{Reason: fmt.Sprintf("%s: %s", command, err), Err: err}
	}
}

// Annotate gives the final error of a fetch whose refresh failed with
// refreshErr. When the caller's context interrupted the refresh itself,
// refreshErr is returned: it wraps the context error and is reported as a
// timeout or a cancellation. Otherwise, when err holds tool's
// provider.ErrTokenExpired, that error is returned with the refresh failure
// reason. In all other cases err is returned unchanged, so a context that
// ended during a later step reports that step's error.
func Annotate(ctx context.Context, err, refreshErr error, tool string) error {
	if refreshErr == nil {
		return err
	}
	if ctx.Err() != nil && errors.Is(refreshErr, ctx.Err()) {
		return refreshErr
	}
	var expired provider.ErrTokenExpired
	if !errors.As(err, &expired) || expired.Tool != tool {
		return err
	}
	expired.RefreshFailure = refreshErr.Error()
	return expired
}
