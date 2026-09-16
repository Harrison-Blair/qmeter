// Package subprocess provides a narrow, fake-able wrapper around running
// external commands. It exists so that packages which need to shell out to an
// OS-provided binary — chiefly the macOS Keychain read
// (/usr/bin/security find-generic-password) that internal/provider/claude
// performs to obtain the current user's Claude credentials — can be tested
// on any OS without invoking a real subprocess.
package subprocess

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// ErrUnscripted is returned by Fake.Run when no result was scripted for the
// given command, so a typo or missing Script call is distinguishable from
// both a scripted success and a scripted failure.
var ErrUnscripted = errors.New("subprocess: no scripted result for command")

// Runner runs an external command and returns its standard output.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (stdout []byte, err error)
}

var (
	_ Runner = Real{}
	_ Runner = (*Fake)(nil)
)

// Real is a Runner backed by os/exec. The zero value is ready to use.
type Real struct{}

// Run runs name with args via exec.CommandContext, honouring ctx
// cancellation/deadline, and returns its standard output. If the process
// fails because ctx was canceled or its deadline was exceeded, the returned
// error wraps ctx.Err() so callers can detect that with errors.Is, even
// though the underlying process error (e.g. "signal: killed") does not.
func (Real) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	if err != nil && ctx.Err() != nil {
		return out, fmt.Errorf("subprocess: %s: %w", name, ctx.Err())
	}
	return out, err
}

// Fake is a scriptable Runner for tests. Script the stdout/err a given
// command should return, then use it wherever a Runner is expected. Calls
// are recorded and can be inspected with Calls. A Fake is safe for
// concurrent use.
type Fake struct {
	mu      sync.Mutex
	scripts map[string]fakeResult
	calls   []Call
}

type fakeResult struct {
	stdout []byte
	err    error
}

// Call records one invocation made against a Fake.
type Call struct {
	Name string
	Args []string
}

// NewFake returns an empty, ready-to-use Fake.
func NewFake() *Fake {
	return &Fake{scripts: make(map[string]fakeResult)}
}

// Script sets the stdout/err that Run should return for the given command
// (name plus args, matched exactly).
func (f *Fake) Script(stdout []byte, err error, name string, args ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scripts[fakeKey(name, args)] = fakeResult{stdout: stdout, err: err}
}

// Run records the call and returns the scripted result for it. If ctx is
// already done, it returns ctx.Err() without consulting the script. If no
// result was scripted for this exact command, it returns an error wrapping
// ErrUnscripted.
func (f *Fake) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Name: name, Args: append([]string(nil), args...)})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	res, ok := f.scripts[fakeKey(name, args)]
	if !ok {
		return nil, fmt.Errorf("%w: %s %s", ErrUnscripted, name, strings.Join(args, " "))
	}
	return res.stdout, res.err
}

// Calls returns every call made to Run so far, in order.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

func fakeKey(name string, args []string) string {
	key := name
	for _, a := range args {
		key += "\x00" + a
	}
	return key
}
