package claude

import (
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

func ledgerNumber(v float64) *float64 { return &v }

func TestFetchBalances(t *testing.T) {
	usd := func(used, limit, remaining *float64) []provider.Balance {
		return []provider.Balance{{Provider: "claude", Name: "extra usage", Unit: "usd", Used: used, Limit: limit, Remaining: remaining}}
	}
	percent := func(used float64) []provider.Balance {
		return []provider.Balance{{Provider: "claude", Name: "extra usage", Unit: "percent", Used: ledgerNumber(used), Limit: ledgerNumber(100), Remaining: ledgerNumber(100 - used)}}
	}
	tests := []struct {
		name, fixture, spend, extra string
		want                        []provider.Balance
	}{
		{name: "synthetic live shape is disabled", fixture: "usage_ledger_synthetic.json"},
		{name: "enabled spend keeps its money", spend: `{"used":{"amount_minor":250,"currency":"USD","exponent":2},"limit":{"amount_minor":2000,"currency":"USD","exponent":2},"enabled":true}`, want: usd(ledgerNumber(2.5), ledgerNumber(20), ledgerNumber(17.5))},
		{name: "disabled spend falls back to enabled extra usage", spend: `{"used":{"amount_minor":0,"currency":"USD","exponent":2},"limit":{"amount_minor":100,"currency":"USD","exponent":2},"enabled":false}`, extra: `{"is_enabled":true,"utilization":40}`, want: percent(40)},
		{name: "disabled spend and disabled extra usage report nothing", spend: `{"used":{"amount_minor":0,"currency":"USD","exponent":2},"limit":{"amount_minor":100,"currency":"USD","exponent":2},"enabled":false}`, extra: `{"is_enabled":false,"utilization":0}`},
		{name: "old extra section is not money", fixture: "usage_live_shape.json"},
		{name: "absent sections", spend: `null`, extra: `null`},
		{name: "each explicit exponent", spend: `{"used":{"amount_minor":125,"currency":"USD","exponent":1},"limit":{"amount_minor":20000,"currency":"USD","exponent":3},"balance":null}`, extra: `{"utilization":90}`, want: usd(ledgerNumber(12.5), ledgerNumber(20), ledgerNumber(7.5))},
		{name: "zero exponent", spend: `{"used":{"amount_minor":0,"currency":"USD","exponent":0}}`, want: usd(ledgerNumber(0), nil, nil)},
		{name: "limit only", spend: `{"used":null,"limit":{"amount_minor":100,"currency":"USD","exponent":2},"balance":null}`, want: usd(nil, ledgerNumber(1), nil)},
		{name: "percent fallback known zero", spend: `null`, extra: `{"is_enabled":true,"monthly_limit":100,"used_credits":0,"currency":"USD","decimal_places":2,"utilization":0}`, want: percent(0)},
		{name: "disabled extra usage reports nothing", spend: `null`, extra: `{"is_enabled":false,"monthly_limit":100,"used_credits":0,"currency":"USD","decimal_places":2,"utilization":0}`},
		{name: "percent fallback nonzero", spend: `{}`, extra: `{"monthly_limit":100,"used_credits":70,"currency":"USD","decimal_places":2,"utilization":27.5}`, want: percent(27.5)},
		{name: "non usd currency falls back", spend: `{"used":{"amount_minor":125,"currency":"EUR","exponent":2},"limit":{"amount_minor":1000,"currency":"EUR","exponent":2},"balance":null}`, extra: `{"utilization":40}`, want: percent(40)},
		{name: "no explicit amounts", spend: `{"used":{"amount_minor":null,"currency":"USD","exponent":2},"limit":{"currency":"USD","exponent":2},"balance":null,"cap":{"money":null,"credits":{"amount_minor":100,"exponent":2}}}`, extra: `{"utilization":12}`, want: percent(12)},
		{name: "missing precision and currency", spend: `{"used":{"amount_minor":10,"currency":"USD"},"limit":{"amount_minor":100,"exponent":2}}`, extra: `{"utilization":25}`, want: percent(25)},
		{name: "null utilization is unknown", spend: `null`, extra: `{"monthly_limit":100,"used_credits":0,"currency":"USD","decimal_places":2,"utilization":null}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := os.ReadFile(fixture("usage_ledger_synthetic.json"))
			if err != nil {
				t.Fatal(err)
			}
			if tt.fixture != "" {
				body, err = os.ReadFile(fixture(tt.fixture))
				if err != nil {
					t.Fatal(err)
				}
			} else {
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(body, &fields); err != nil {
					t.Fatal(err)
				}
				delete(fields, "spend")
				delete(fields, "extra_usage")
				if tt.spend != "" {
					fields["spend"] = json.RawMessage(tt.spend)
				}
				if tt.extra != "" {
					fields["extra_usage"] = json.RawMessage(tt.extra)
				}
				body, err = json.Marshal(fields)
				if err != nil {
					t.Fatal(err)
				}
			}
			srv, rec := serveJSON(t, http.StatusOK, nil, body)
			t.Setenv(envVar, "synthetic-ledger-token")
			got, err := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client())).Fetch(testContext(t))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Balances, tt.want) {
				t.Errorf("balances = %+v, want %+v", got.Balances, tt.want)
			}
			if len(got.Windows) < 2 || got.Windows[0].Name != "5h" {
				t.Errorf("lost existing windows: %+v", got.Windows)
			}
			if calls, _, _, _ := rec.snapshot(); calls != 1 {
				t.Errorf("HTTP calls = %d, want 1", calls)
			}
		})
	}
}

func TestFetchBalances_ErrorReturnsZeroUsage(t *testing.T) {
	t.Setenv(envVar, "synthetic-ledger-token")
	for _, body := range []string{
		`{"spend":{"used":{"amount_minor":0,"currency":"USD","exponent":2}}}`,
		`{"five_hour":`,
	} {
		srv, _ := serveJSON(t, http.StatusOK, nil, []byte(body))
		got, err := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client())).Fetch(testContext(t))
		if err == nil || !reflect.DeepEqual(got, provider.Usage{}) {
			t.Errorf("Fetch = %+v, %v; want zero Usage and an error", got, err)
		}
	}
}
