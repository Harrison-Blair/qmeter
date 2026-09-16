package usage

import (
	"testing"
)

func TestRegistry_OrdersClaudeCodexOpenCodeGoCursor(t *testing.T) {
	// Registry() constructs the real providers; their constructors resolve
	// default credential paths from the home directory. Point HOME at an
	// empty temp dir so the test never depends on the developer's own
	// credential stores (construction alone does no I/O, but the paths are
	// computed from it).
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	got := make([]string, 0, 4)
	for _, p := range Registry() {
		got = append(got, p.ID())
	}
	want := []string{"claude", "codex", "opencode-go", "cursor"}
	if !equalStrings(got, want) {
		t.Fatalf("Registry() ids = %v, want %v", got, want)
	}
}
