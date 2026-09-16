// Package subprocess provides a narrow, fake-able wrapper around running
// external commands. It exists so that packages which need to shell out to an
// OS-provided binary — chiefly the macOS Keychain read
// (/usr/bin/security find-generic-password) that internal/provider/claude
// performs to obtain the current user's Claude credentials — can be tested
// on any OS without invoking a real subprocess.
package subprocess

import (
	"context"
	"os/exec"
)

// Runner runs an external command and returns its standard output.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (stdout []byte, err error)
}

// Real is a Runner backed by os/exec. The zero value is ready to use.
type Real struct{}

// Run runs name with args via exec.CommandContext, honouring ctx
// cancellation/deadline, and returns its standard output.
func (Real) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

// Fake is a scriptable Runner for tests. Script the stdout/err a given
// command should return, then use it wherever a Runner is expected. Calls
// are recorded and can be inspected with Calls.
type Fake struct {
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
	f.scripts[fakeKey(name, args)] = fakeResult{stdout: stdout, err: err}
}

// Run records the call and returns the scripted result for it.
func (f *Fake) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, Call{Name: name, Args: append([]string(nil), args...)})
	res := f.scripts[fakeKey(name, args)]
	return res.stdout, res.err
}

// Calls returns every call made to Run so far, in order.
func (f *Fake) Calls() []Call {
	return append([]Call(nil), f.calls...)
}

func fakeKey(name string, args []string) string {
	key := name
	for _, a := range args {
		key += "\x00" + a
	}
	return key
}
