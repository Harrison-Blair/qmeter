package usage

import (
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/provider/claude"
	"github.com/Harrison-Blair/qmeter/internal/provider/codex"
	"github.com/Harrison-Blair/qmeter/internal/provider/cursor"
	"github.com/Harrison-Blair/qmeter/internal/provider/opencodego"
)

// Registry returns the concrete providers qmeter ships with, in the order
// they are detected, fetched and printed: claude, codex, opencode-go,
// cursor. This is the only place the four provider packages are
// constructed; everything else takes a []provider.Provider.
//
// Constructing a provider performs no I/O — each one resolves its default
// credential path and waits for Detect/Fetch to be called.
func Registry() []provider.Provider {
	return []provider.Provider{
		claude.New(),
		codex.New(),
		opencodego.New(),
		cursor.New(),
	}
}
