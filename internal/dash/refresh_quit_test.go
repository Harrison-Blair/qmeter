package dash

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/provider/claude"
)

// blockingRefresh is a vendor CLI that runs until its context is canceled.
type blockingRefresh struct {
	started  chan struct{}
	canceled chan struct{}
	once     sync.Once
}

func (b *blockingRefresh) Run(ctx context.Context, _ string, _ ...string) ([]byte, error) {
	b.once.Do(func() { close(b.started) })
	<-ctx.Done()
	close(b.canceled)
	return nil, ctx.Err()
}

// TestRun_QuitsWhileAProviderRefreshRuns runs the real Claude provider on an
// expired store, so Fetch starts `claude auth status`. The fake command blocks;
// q must still end the dashboard at once and cancel the command.
func TestRun_QuitsWhileAProviderRefreshRuns(t *testing.T) {
	t.Setenv("QMETER_CLAUDE_TOKEN", "")
	store := filepath.Join(t.TempDir(), ".credentials.json")
	if err := os.WriteFile(store, []byte(`{"claudeAiOauth":{"accessToken":"stale","expiresAt":1}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	refresh := &blockingRefresh{started: make(chan struct{}), canceled: make(chan struct{})}
	p := claude.New(
		claude.WithCredentialPath(store),
		claude.WithPiCredentialPath(filepath.Join(t.TempDir(), "auth.json")),
		claude.WithKeychainRunner(nil),
		claude.WithBaseURL("http://127.0.0.1:1"),
		claude.WithRefreshRunner(refresh),
	)

	in, keys := io.Pipe()
	defer func() { _ = keys.Close(); _ = in.Close() }()
	go func() {
		<-refresh.started
		_, _ = keys.Write([]byte("q"))
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		var out bytes.Buffer
		done <- Run(ctx, RunOptions{Providers: []provider.Provider{p}, Input: in, Output: &out})
	}()

	select {
	case <-refresh.started:
	case <-time.After(5 * time.Second):
		t.Fatal("Fetch never started the refresh command")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not quit within 1s while the refresh command ran")
	}
	select {
	case <-refresh.canceled:
	case <-time.After(time.Second):
		t.Fatal("quitting left the refresh command running")
	}
}
