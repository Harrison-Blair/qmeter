package layout

import (
	"github.com/Harrison-Blair/qmeter/internal/display"
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

func providerPlan(result usage.Result, id string) string {
	for _, window := range result.Windows {
		if window.Provider == id {
			if window.Plan != "" {
				return window.Plan
			}
			return "-"
		}
	}
	return "-"
}
