package dash

import (
	"reflect"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/dash/layout"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
)

var (
	firstFetch  = time.Date(2026, 9, 16, 14, 22, 7, 0, time.Local)
	secondFetch = time.Date(2026, 9, 16, 14, 23, 8, 0, time.Local)
	thirdFetch  = time.Date(2026, 9, 16, 14, 24, 9, 0, time.Local)
)

// asked is the provider set the model passed to usage.Run.
var asked = []string{"codex"}

func codexSuccess(percent float64) usage.Result {
	return usage.Result{
		Windows:  []provider.Window{{Provider: "codex", Name: "weekly", Plan: "prolite", RemainingPercent: percent}},
		Balances: []provider.Balance{{Provider: "codex", Name: "credits", Unit: "credits"}},
	}
}

func codexFailure(kind usage.ErrorKind) usage.Result {
	return usage.Result{Errors: []usage.ProviderError{{Provider: "codex", Message: "offline", Kind: kind}}}
}

func TestCacheSuccessReplacesTheEntry(t *testing.T) {
	c := cache{}.update(codexSuccess(40), asked, firstFetch).update(codexSuccess(30), asked, secondFetch)

	want := layout.CachedProvider{
		Windows:   codexSuccess(30).Windows,
		Balances:  codexSuccess(30).Balances,
		FetchedAt: secondFetch,
	}
	if got := c["codex"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("entry = %#v, want %#v", got, want)
	}
}

func TestCacheEmptySuccessRemovesTheEntry(t *testing.T) {
	c := cache{}.update(codexSuccess(40), asked, firstFetch).update(usage.Result{}, asked, secondFetch)

	if got, ok := c["codex"]; ok {
		t.Fatalf("entry = %#v, want none after a success with no windows and no balances", got)
	}
	if got, ok := c.update(codexFailure(""), asked, thirdFetch)["codex"]; ok {
		t.Fatalf("entry = %#v, want none for a failure after an empty success", got)
	}
}

func TestCacheOrdinaryFailureKeepsThePreviousEntryAndItsFetchTime(t *testing.T) {
	c := cache{}.update(codexSuccess(40), asked, firstFetch).update(codexFailure(""), asked, secondFetch).update(codexFailure(""), asked, thirdFetch)

	want := layout.CachedProvider{
		Windows:   codexSuccess(40).Windows,
		Balances:  codexSuccess(40).Balances,
		FetchedAt: firstFetch,
	}
	if got, ok := c["codex"]; !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("entry = %#v (present %t), want %#v", got, ok, want)
	}
}

func TestCacheTokenExpiryDropsTheEntry(t *testing.T) {
	c := cache{}.update(codexSuccess(40), asked, firstFetch).update(codexFailure(usage.KindTokenExpired), asked, secondFetch)

	if got, ok := c["codex"]; ok {
		t.Fatalf("entry = %#v, want none after a token expiry", got)
	}
}

func TestCacheFirstFailureHasNoEntry(t *testing.T) {
	c := cache{}.update(codexFailure(""), asked, firstFetch)

	if got, ok := c["codex"]; ok {
		t.Fatalf("entry = %#v, want none before any success", got)
	}
}

func TestCacheUpdateLeavesTheEarlierCacheUnchanged(t *testing.T) {
	before := cache{}.update(codexSuccess(40), asked, firstFetch)
	_ = before.update(codexFailure(usage.KindTokenExpired), asked, secondFetch)

	if _, ok := before["codex"]; !ok {
		t.Fatal("update changed the cache it was called on")
	}
}
