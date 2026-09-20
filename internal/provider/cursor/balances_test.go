package cursor

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

func ledgerNumber(v float64) *float64 { return &v }

func TestFetchBalances(t *testing.T) {
	amount := func(name string, used, limit, remaining *float64) provider.Balance {
		return provider.Balance{Provider: "cursor", Name: name, Unit: "unconfirmed", Used: used, Limit: limit, Remaining: remaining}
	}
	zeroPlan := amount("included", ledgerNumber(0), ledgerNumber(0), ledgerNumber(0))
	tests := []struct {
		name, fixture, plan, onDemand string
		want                          []provider.Balance
	}{
		{name: "synthetic known zeros and disabled on demand", fixture: "usage_ledger_synthetic.json", want: []provider.Balance{zeroPlan}},
		{name: "existing free", fixture: "usage_summary_free.json", want: []provider.Balance{amount("included", ledgerNumber(0.9), ledgerNumber(20), ledgerNumber(19.1))}},
		{name: "existing paid and on demand", fixture: "usage_summary_pro_team.json", want: []provider.Balance{amount("included", ledgerNumber(14.25), ledgerNumber(20), ledgerNumber(5.75)), amount("on-demand", ledgerNumber(3.5), ledgerNumber(50), ledgerNumber(46.5))}},
		{name: "existing null amounts and null object", fixture: "usage_summary_nulls.json"},
		{name: "enabled zero with unknown limit", onDemand: `{"enabled":true,"used":0,"limit":null,"remaining":null}`, want: []provider.Balance{zeroPlan, amount("on-demand", ledgerNumber(0), nil, nil)}},
		{name: "enabled all null", onDemand: `{"enabled":true,"used":null,"limit":null,"remaining":null}`, want: []provider.Balance{zeroPlan}},
		{name: "null on demand", onDemand: `null`, want: []provider.Balance{zeroPlan}},
		{name: "disabled with positive amounts", onDemand: `{"enabled":false,"used":5,"limit":10,"remaining":5}`, want: []provider.Balance{zeroPlan}},
		{name: "partial plan does not invent remaining", plan: `{"enabled":true,"used":0,"limit":20,"remaining":null,"totalPercentUsed":4.5,"autoPercentUsed":9}`, want: []provider.Balance{amount("included", ledgerNumber(0), ledgerNumber(20), nil)}},
		{name: "only remaining is known zero", plan: `{"enabled":true,"remaining":0,"totalPercentUsed":4.5,"autoPercentUsed":9}`, want: []provider.Balance{amount("included", nil, nil, ledgerNumber(0))}},
		{name: "no plan amounts", plan: `{"enabled":true,"totalPercentUsed":4.5,"autoPercentUsed":9}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := tt.fixture
			if name == "" {
				name = "usage_ledger_synthetic.json"
			}
			body := fixture(t, name)
			if tt.fixture == "" {
				var fields map[string]json.RawMessage
				if err := json.Unmarshal([]byte(body), &fields); err != nil {
					t.Fatal(err)
				}
				var individual map[string]json.RawMessage
				if err := json.Unmarshal(fields["individualUsage"], &individual); err != nil {
					t.Fatal(err)
				}
				if tt.plan != "" {
					individual["plan"] = json.RawMessage(tt.plan)
				}
				if tt.onDemand != "" {
					individual["onDemand"] = json.RawMessage(tt.onDemand)
				}
				var err error
				fields["individualUsage"], err = json.Marshal(individual)
				if err != nil {
					t.Fatal(err)
				}
				b, err := json.Marshal(fields)
				if err != nil {
					t.Fatal(err)
				}
				body = string(b)
			}
			srv, rec := serve(t, http.StatusOK, body)
			t.Setenv(envToken, makeJWT(t, map[string]any{"sub": "synthetic-ledger-user"}))
			got, err := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client())).Fetch(testCtx(t))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Balances, tt.want) {
				t.Errorf("balances = %+v, want %+v", got.Balances, tt.want)
			}
			if len(got.Windows) != 2 {
				t.Fatalf("lost existing windows: %+v", got.Windows)
			}
			if name == "usage_ledger_synthetic.json" && (got.Windows[0].RemainingPercent != 95.5 || got.Windows[1].RemainingPercent != 91) {
				t.Errorf("amounts changed percentages: %+v", got.Windows)
			}
			if calls := rec.snapshot().calls; calls != 1 {
				t.Errorf("HTTP calls = %d, want 1", calls)
			}
		})
	}
}

func TestFetchBalances_ErrorReturnsZeroUsage(t *testing.T) {
	t.Setenv(envToken, makeJWT(t, map[string]any{"sub": "synthetic-ledger-user"}))
	for _, body := range []string{
		`{"individualUsage":{"plan":{"enabled":false,"used":0,"limit":20,"remaining":20},"onDemand":{"enabled":true,"used":5}}}`,
		`{"individualUsage":{"plan":{"enabled":true,"used":0,"limit":20,"remaining":20}}}`,
		`{"individualUsage":`,
	} {
		srv, _ := serve(t, http.StatusOK, body)
		got, err := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client())).Fetch(testCtx(t))
		if err == nil || !reflect.DeepEqual(got, provider.Usage{}) {
			t.Errorf("Fetch = %+v, %v; want zero Usage and an error", got, err)
		}
	}
}
