package opencodego

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

func TestFetchBalances_None(t *testing.T) {
	for _, name := range []string{"usage_ledger_synthetic.json", "usage_unknown_fields.json"} {
		t.Run(name, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("testdata", name))
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				_, _ = w.Write(body)
			}))
			t.Cleanup(srv.Close)
			t.Setenv(envVar, "synthetic-ledger-key")
			got, err := New(WithDBPath(missingDB(t)), WithBaseURL(srv.URL), WithHTTPClient(srv.Client())).Fetch(testContext(t))
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Balances) != 0 {
				t.Errorf("invented balances: %+v", got.Balances)
			}
			if len(got.Windows) == 0 {
				t.Fatal("lost existing windows")
			}
			if name == "usage_ledger_synthetic.json" {
				if len(got.Windows) != 3 || got.Windows[0].RemainingPercent != 99 || got.Windows[1].RemainingPercent != 99 || got.Windows[2].RemainingPercent != 69 {
					t.Errorf("windows = %+v, want 99/99/69 remaining", got.Windows)
				}
			}
			if calls.Load() != 1 {
				t.Errorf("HTTP calls = %d, want 1", calls.Load())
			}
		})
	}
}

func TestFetchBalances_ErrorReturnsZeroUsage(t *testing.T) {
	t.Setenv(envVar, "synthetic-ledger-key")
	for _, body := range []string{
		`{"usage":{"rolling":{"status":"ok","percent":1},"weekly":{"status":"ok","percent":101}}}`,
		`{"usage":`,
	} {
		srv, _ := statusServer(t, http.StatusOK, []byte(body))
		got, err := New(WithDBPath(missingDB(t)), WithBaseURL(srv.URL), WithHTTPClient(srv.Client())).Fetch(testContext(t))
		if err == nil || !reflect.DeepEqual(got, provider.Usage{}) {
			t.Errorf("Fetch = %+v, %v; want zero Usage and an error", got, err)
		}
	}
}
