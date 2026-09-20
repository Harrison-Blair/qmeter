package providertest_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/provider/providertest"
)

func TestFakeFetchUsage(t *testing.T) {
	windows := []provider.Window{{Name: "5h"}}
	balances := []provider.Balance{{Name: "credits", Unit: "credits"}}
	failure := errors.New("synthetic failure")
	for _, tt := range []struct {
		name   string
		err    error
		cancel bool
	}{
		{name: "success"},
		{name: "error", err: failure},
		{name: "cancel", cancel: true, err: context.Canceled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := providertest.Succeeding("codex", windows)
			p.Balances = balances
			p.FetchErr = tt.err
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tt.cancel {
				p.Delay = time.Hour
				cancel()
			}
			got, err := p.Fetch(ctx)
			if !errors.Is(err, tt.err) {
				t.Fatalf("error = %v, want %v", err, tt.err)
			}
			want := provider.Usage{}
			if tt.err == nil {
				want = provider.Usage{Windows: windows, Balances: balances}
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("usage = %+v, want %+v", got, want)
			}
		})
	}
}
