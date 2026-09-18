package usage

import (
	"strings"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// Select narrows providers to the ones named, keeping the registry's own
// order however the names were given, and listing a provider named twice
// only once. Empty and blank names are ignored, and no names at all means
// every provider.
//
// The second return is the first name no provider claims, and is empty
// when every name was recognized. Selection is all or nothing: one bad
// name selects nothing, because the caller reports it and stops.
func Select(providers []provider.Provider, names []string) ([]provider.Provider, string) {
	want := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		known := false
		for _, p := range providers {
			if p.ID() == name {
				known = true
				break
			}
		}
		if !known {
			return nil, name
		}
		want[name] = true
	}
	if len(want) == 0 {
		return providers, ""
	}

	out := make([]provider.Provider, 0, len(want))
	for _, p := range providers {
		if want[p.ID()] {
			out = append(out, p)
		}
	}
	return out, ""
}
