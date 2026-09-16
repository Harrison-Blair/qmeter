package subprocess

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFakeRunner_ReturnsScriptedOutput(t *testing.T) {
	f := NewFake()
	f.Script([]byte("hello\n"), nil, "security", "find-generic-password", "-w")

	out, err := f.Run(context.Background(), "security", "find-generic-password", "-w")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "hello\n" {
		t.Fatalf("got %q, want %q", out, "hello\n")
	}
}

func TestFakeRunner_ReturnsScriptedError(t *testing.T) {
	wantErr := errors.New("exit status 1")
	f := NewFake()
	f.Script(nil, wantErr, "security", "find-generic-password", "-w")

	out, err := f.Run(context.Background(), "security", "find-generic-password", "-w")
	if !errors.Is(err, wantErr) {
		t.Fatalf("got err %v, want %v", err, wantErr)
	}
	if out != nil {
		t.Fatalf("got out %q, want nil", out)
	}
}

func TestFakeRunner_UnscriptedCallReturnsErrUnscripted(t *testing.T) {
	f := NewFake()

	out, err := f.Run(context.Background(), "security", "find-generic-password", "-w")
	if !errors.Is(err, ErrUnscripted) {
		t.Fatalf("got err %v, want it to wrap ErrUnscripted", err)
	}
	if out != nil {
		t.Fatalf("got out %q, want nil", out)
	}
}

func TestFakeRunner_ExpiredContextReturnsCtxErr(t *testing.T) {
	f := NewFake()
	f.Script([]byte("hello\n"), nil, "security", "find-generic-password", "-w")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	out, err := f.Run(ctx, "security", "find-generic-password", "-w")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got err %v, want it to wrap context.Canceled", err)
	}
	if out != nil {
		t.Fatalf("got out %q, want nil", out)
	}
}

func TestFakeRunner_ConcurrentUse(t *testing.T) {
	f := NewFake()
	f.Script([]byte("hello\n"), nil, "security", "find-generic-password", "-w")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = f.Run(context.Background(), "security", "find-generic-password", "-w")
			_ = f.Calls()
			f.Script([]byte("hello\n"), nil, "security", "find-generic-password", "-w")
		}()
	}
	wg.Wait()

	if got := len(f.Calls()); got != 20 {
		t.Fatalf("got %d recorded calls, want 20", got)
	}
}

func TestRealRunner_RunsEcho(t *testing.T) {
	if _, err := exec.LookPath("echo"); err != nil {
		t.Skip("echo not present on PATH")
	}

	var r Real
	out, err := r.Run(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(string(out)) != "hello" {
		t.Fatalf("got %q, want %q", out, "hello")
	}
}

func TestRealRunner_PreCanceledContextDoesNotRun(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not present on PATH")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var r Real
	_, err := r.Run(ctx, "sleep", "5")
	if err == nil {
		t.Fatal("expected an error from a canceled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got err %v, want it to wrap context.Canceled", err)
	}
}

func TestRealRunner_CancelWhileRunning(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not present on PATH")
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	var r Real
	_, err := r.Run(ctx, "sleep", "5")
	if err == nil {
		t.Fatal("expected an error from a context canceled mid-run, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got err %v, want it to wrap context.Canceled", err)
	}
}
