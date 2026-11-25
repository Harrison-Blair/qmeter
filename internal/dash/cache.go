package dash

import (
	"time"

	"github.com/Harrison-Blair/qmeter/internal/dash/layout"
	"github.com/Harrison-Blair/qmeter/internal/usage"
)

// cache holds the last good response of each provider, by provider ID. It
// lives in memory only.
type cache map[string]layout.CachedProvider

// update returns the cache after res, fetched at at, and leaves c unchanged.
// asked lists the providers passed to usage.Run; one of them with no error
// fetched successfully. A provider that fetched replaces its entry, or drops
// it when it brought no windows and no balances; a failure keeps the entry,
// except a token expiry, which drops it.
func (c cache) update(res usage.Result, asked []string, at time.Time) cache {
	next := make(cache, len(c))
	for id, entry := range c {
		next[id] = entry
	}
	failed := make(map[string]bool, len(res.Errors))
	for _, e := range res.Errors {
		failed[e.Provider] = true
		if e.Kind == usage.KindTokenExpired {
			delete(next, e.Provider)
		}
	}
	for _, id := range asked {
		if !failed[id] {
			delete(next, id)
		}
	}
	fresh := make(map[string]layout.CachedProvider)
	for _, w := range res.Windows {
		if !failed[w.Provider] {
			entry := fresh[w.Provider]
			entry.Windows = append(entry.Windows, w)
			fresh[w.Provider] = entry
		}
	}
	for _, b := range res.Balances {
		if !failed[b.Provider] {
			entry := fresh[b.Provider]
			entry.Balances = append(entry.Balances, b)
			fresh[b.Provider] = entry
		}
	}
	for id, entry := range fresh {
		entry.FetchedAt = at
		next[id] = entry
	}
	return next
}
