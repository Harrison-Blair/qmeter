package subprocess

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
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

func TestRealRunner_HonoursContextCancellation(t *testing.T) {
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
