package usage

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/provider/providertest"
)

type ledgerProvider struct {
	*providertest.Fake
	fetch func(context.Context) (provider.Usage, error)
}

func (p ledgerProvider) Fetch(ctx context.Context) (provider.Usage, error) { return p.fetch(ctx) }

func TestRunBalances_OrderAndErrorIsolation(t *testing.T) {
	lastCompleted := make(chan struct{})
	first := ledgerProvider{Fake: providertest.Succeeding("claude", nil), fetch: func(ctx context.Context) (provider.Usage, error) {
		select {
		case <-lastCompleted:
		case <-ctx.Done():
			return provider.Usage{}, ctx.Err()
		}
		return provider.Usage{Windows: []provider.Window{{Name: "5h"}}, Balances: []provider.Balance{{Name: "extra usage", Unit: "usd"}}}, nil
	}}
	failed := ledgerProvider{Fake: providertest.Succeeding("codex", nil), fetch: func(context.Context) (provider.Usage, error) {
		// Even a misbehaving provider returning partial data with an error
		// must contribute neither windows nor balances.
		return provider.Usage{Windows: []provider.Window{{Name: "discard"}}, Balances: []provider.Balance{{Name: "discard"}}}, errors.New("synthetic failure")
	}}
	last := ledgerProvider{Fake: providertest.Succeeding("cursor", nil), fetch: func(context.Context) (provider.Usage, error) {
		defer close(lastCompleted)
		return provider.Usage{Windows: []provider.Window{{Name: "total"}}, Balances: []provider.Balance{{Name: "included", Unit: "unconfirmed"}, {Provider: "reported-provider", Name: "on-demand", Unit: "unconfirmed"}}}, nil
	}}
	got := Run(t.Context(), []provider.Provider{first, failed, last}, "")
	want := []provider.Balance{
		{Provider: "claude", Name: "extra usage", Unit: "usd"},
		{Provider: "cursor", Name: "included", Unit: "unconfirmed"},
		{Provider: "reported-provider", Name: "on-demand", Unit: "unconfirmed"},
	}
	if !reflect.DeepEqual(got.Balances, want) {
		t.Errorf("balances = %+v, want %+v", got.Balances, want)
	}
	if !equalStrings(windowKeys(got.Windows), []string{"claude/5h", "cursor/total"}) {
		t.Errorf("windows = %+v", got.Windows)
	}
	if !reflect.DeepEqual(got.Errors, []ProviderError{{Provider: "codex", Message: "synthetic failure"}}) {
		t.Errorf("errors = %+v", got.Errors)
	}
}

func TestRunBalances_FilterAndUndetected(t *testing.T) {
	first := providertest.Succeeding("claude", nil)
	first.Balances = []provider.Balance{{Name: "extra usage", Unit: "usd"}}
	second := providertest.Succeeding("codex", nil)
	second.Balances = []provider.Balance{{Name: "credits", Unit: "credits"}}
	missing := providertest.Undetected("cursor", "synthetic missing login")
	missing.Balances = []provider.Balance{{Name: "discard"}}
	got := Run(t.Context(), []provider.Provider{first, second, missing}, "codex")
	want := []provider.Balance{{Provider: "codex", Name: "credits", Unit: "credits"}}
	if !reflect.DeepEqual(got.Balances, want) {
		t.Errorf("balances = %+v, want %+v", got.Balances, want)
	}
	if first.Fetches() != 0 || second.Fetches() != 1 || missing.Fetches() != 0 {
		t.Fatal("fetched outside filter")
	}
	got = Run(t.Context(), []provider.Provider{missing}, "cursor")
	if len(got.Balances) != 0 || len(got.Undetected) != 1 || missing.Fetches() != 0 {
		t.Errorf("undetected result = %+v", got)
	}
}
