package layout

import (
	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
)

type provInfo struct {
	id string
}

func presentProviders(result usage.Result) []provInfo {
	seen := make(map[string]bool)
	add := func(id string) {
		if id != "" {
			seen[id] = true
		}
	}
	for _, window := range result.Windows {
		add(window.Provider)
	}
	for _, balance := range result.Balances {
		add(balance.Provider)
	}
	for _, message := range result.Errors {
		add(message.Provider)
	}
	for _, message := range result.Undetected {
		add(message.Provider)
	}

	known := make(map[string]bool)
	providers := make([]provInfo, 0, len(seen))
	for _, id := range display.ProviderOrder() {
		known[id] = true
		if seen[id] {
			providers = append(providers, provInfo{id: id})
		}
	}
	for _, id := range firstSeenOrder(result) {
		if !known[id] {
			providers = append(providers, provInfo{id: id})
		}
	}
	return providers
}

func firstSeenOrder(result usage.Result) []string {
	var ids []string
	seen := make(map[string]bool)
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, window := range result.Windows {
		add(window.Provider)
	}
	for _, balance := range result.Balances {
		add(balance.Provider)
	}
	for _, message := range result.Errors {
		add(message.Provider)
	}
	for _, message := range result.Undetected {
		add(message.Provider)
	}
	return ids
}

func providerPlan(windows []provider.Window) string {
	if len(windows) > 0 && windows[0].Plan != "" {
		return windows[0].Plan
	}
	return "-"
}

// cachedFor returns the cached entry that stands in for id: id failed in
// result and the dashboard kept an earlier response for it.
func cachedFor(result usage.Result, cached map[string]CachedProvider, id string) (CachedProvider, bool) {
	for _, e := range result.Errors {
		if e.Provider == id {
			entry, ok := cached[id]
			return entry, ok
		}
	}
	return CachedProvider{}, false
}

// cachedMessage is the line that replaces the error line of a cached
// provider. The glyph and the word carry the state without colour.
func cachedMessage(entry CachedProvider, reason string) string {
	return "◌ cached · fetched " + entry.FetchedAt.Local().Format("15:04:05") + " · " + reason
}

type compactWindow struct {
	window provider.Window
	cached bool
}

// compactWindows lists the compact window rows in provider order, with a
// cached provider's kept windows in its own slot.
func compactWindows(result usage.Result, cached map[string]CachedProvider) []compactWindow {
	var rows []compactWindow
	for _, p := range presentProviders(result) {
		windows, isCached := result.Windows, false
		if entry, ok := cachedFor(result, cached, p.id); ok {
			windows, isCached = entry.Windows, true
		}
		for _, w := range windows {
			if w.Provider == p.id {
				rows = append(rows, compactWindow{window: w, cached: isCached})
			}
		}
	}
	return rows
}

// compactErrorText is the message of a failed provider's compact row.
func compactErrorText(result usage.Result, cached map[string]CachedProvider, message usage.ProviderError) string {
	if entry, ok := cachedFor(result, cached, message.Provider); ok {
		return cachedMessage(entry, message.Message)
	}
	return "error: " + message.Message
}
