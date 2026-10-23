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
	c := cache{}.update(codexSuccess(40), firstFetch).update(codexSuccess(30), secondFetch)

	want := layout.CachedProvider{
		Windows:   codexSuccess(30).Windows,
		Balances:  codexSuccess(30).Balances,
		FetchedAt: secondFetch,
	}
	if got := c["codex"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("entry = %#v, want %#v", got, want)
	}
}

func TestCacheOrdinaryFailureKeepsThePreviousEntryAndItsFetchTime(t *testing.T) {
	c := cache{}.update(codexSuccess(40), firstFetch).update(codexFailure(""), secondFetch).update(codexFailure(""), thirdFetch)

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
	c := cache{}.update(codexSuccess(40), firstFetch).update(codexFailure(usage.KindTokenExpired), secondFetch)

	if got, ok := c["codex"]; ok {
		t.Fatalf("entry = %#v, want none after a token expiry", got)
	}
}

func TestCacheFirstFailureHasNoEntry(t *testing.T) {
	c := cache{}.update(codexFailure(""), firstFetch)

	if got, ok := c["codex"]; ok {
		t.Fatalf("entry = %#v, want none before any success", got)
	}
}

func TestCacheUpdateLeavesTheEarlierCacheUnchanged(t *testing.T) {
	before := cache{}.update(codexSuccess(40), firstFetch)
	_ = before.update(codexFailure(usage.KindTokenExpired), secondFetch)

	if _, ok := before["codex"]; !ok {
		t.Fatal("update changed the cache it was called on")
	}
}
