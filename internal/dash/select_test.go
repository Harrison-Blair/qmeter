package dash

import (
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/provider/providertest"
)

func registry() []provider.Provider {
	return []provider.Provider{
		providertest.Undetected("claude", "not logged in"),
		providertest.Undetected("codex", "not logged in"),
		providertest.Undetected("opencode-go", "not logged in"),
		providertest.Undetected("cursor", "not logged in"),
	}
}

func ids(ps []provider.Provider) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.ID()
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSelect(t *testing.T) {
	for _, tc := range []struct {
		name    string
		names   []string
		want    []string
		unknown string
	}{
		{name: "no filter keeps every provider", names: nil, want: []string{"claude", "codex", "opencode-go", "cursor"}},
		{name: "one name", names: []string{"codex"}, want: []string{"codex"}},
		{name: "several names keep registry order", names: []string{"cursor", "claude"}, want: []string{"claude", "cursor"}},
		{name: "a repeated name is listed once", names: []string{"claude", "claude"}, want: []string{"claude"}},
		{name: "surrounding space is ignored", names: []string{" codex "}, want: []string{"codex"}},
		{name: "an empty name is ignored", names: []string{"codex", ""}, want: []string{"codex"}},
		{name: "an unknown name is reported", names: []string{"claude", "nope"}, unknown: "nope"},
		{name: "the first unknown name is reported", names: []string{"nope", "nah"}, unknown: "nope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, unknown := Select(registry(), tc.names)
			if unknown != tc.unknown {
				t.Fatalf("unknown = %q, want %q", unknown, tc.unknown)
			}
			if tc.unknown != "" {
				return
			}
			if !equal(ids(got), tc.want) {
				t.Fatalf("selected %v, want %v", ids(got), tc.want)
			}
		})
	}
}
