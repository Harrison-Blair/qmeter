package codex

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

func ledgerNumber(v float64) *float64 { return &v }

func TestFetchBalances(t *testing.T) {
	credit := func(remaining *float64, unlimited bool) []provider.Balance {
		return []provider.Balance{{Provider: "codex", Name: "credits", Unit: "credits", Remaining: remaining, Unlimited: unlimited}}
	}
	tests := []struct {
		name, fixture, credits string
		want                   []provider.Balance
	}{
		{name: "synthetic string zero", fixture: "usage_ledger_synthetic.json", want: credit(ledgerNumber(0), false)},
		{name: "existing numeric balance", fixture: "usage_live_shape.json", want: credit(ledgerNumber(3.5), false)},
		{name: "camel case known zero", fixture: "usage_camel_case.json", want: credit(ledgerNumber(0), false)},
		{name: "existing empty credits", fixture: "usage_missing_windows.json"},
		{name: "numeric string", credits: `{"balance":"12.75"}`, want: credit(ledgerNumber(12.75), false)},
		{name: "unlimited with balance", credits: `{"unlimited":true,"balance":0}`, want: credit(ledgerNumber(0), true)},
		{name: "unlimited without balance", credits: `{"unlimited":true}`, want: credit(nil, true)},
		{name: "unlimited null balance", credits: `{"unlimited":true,"balance":null}`, want: credit(nil, true)},
		{name: "unlimited malformed balance", credits: `{"unlimited":true,"balance":{}}`, want: credit(nil, true)},
		{name: "empty credits", credits: `{}`},
		{name: "absent credits"},
		{name: "null credits", credits: `null`},
		{name: "malformed credits", credits: `[]`},
		{name: "null balance", credits: `{"has_credits":true,"balance":null}`},
		{name: "invalid string", credits: `{"balance":"unknown"}`},
		{name: "invalid number type", credits: `{"balance":false}`},
		{name: "nonfinite string", credits: `{"balance":"NaN"}`},
		{name: "infinite string", credits: `{"balance":"Inf"}`},
		{name: "malformed unlimited preserves amount", credits: `{"unlimited":{},"balance":2}`, want: credit(ledgerNumber(2), false)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := tt.fixture
			if name == "" {
				name = "usage_ledger_synthetic.json"
			}
			body, err := os.ReadFile(fixture(name))
			if err != nil {
				t.Fatal(err)
			}
			if tt.fixture == "" {
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(body, &fields); err != nil {
					t.Fatal(err)
				}
				delete(fields, "credits")
				if tt.credits != "" {
					fields["credits"] = json.RawMessage(tt.credits)
				}
				body, err = json.Marshal(fields)
				if err != nil {
					t.Fatal(err)
				}
			}
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				_, _ = w.Write(body)
			}))
			t.Cleanup(srv.Close)
			t.Setenv("QMETER_CODEX_TOKEN", "synthetic-ledger-token")
			got, err := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client())).Fetch(testContext(t))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Balances, tt.want) {
				t.Errorf("balances = %+v, want %+v", got.Balances, tt.want)
			}
			if name == "usage_ledger_synthetic.json" && (len(got.Windows) != 1 || got.Windows[0].RemainingPercent != 80) {
				t.Errorf("lost existing windows: %+v", got.Windows)
			}
			if calls.Load() != 1 {
				t.Errorf("HTTP calls = %d, want 1", calls.Load())
			}
		})
	}
}

func TestFetchBalances_ErrorReturnsZeroUsage(t *testing.T) {
	t.Setenv("QMETER_CODEX_TOKEN", "synthetic-ledger-token")
	for _, body := range []string{`{"credits":{"balance":0}}`, `{"rate_limit":`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(srv.Close)
		got, err := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client())).Fetch(testContext(t))
		if err == nil || !reflect.DeepEqual(got, provider.Usage{}) {
			t.Errorf("Fetch = %+v, %v; want zero Usage and an error", got, err)
		}
	}
}
