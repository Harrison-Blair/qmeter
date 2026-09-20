package usage

import (
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/provider/providertest"
)

func TestCmd_JSONUnchangedWithBalances(t *testing.T) {
	calls := captureHint(t)
	p := providertest.Succeeding("codex", []provider.Window{{Name: "weekly", RemainingPercent: 80}})
	zero := 0.0
	p.Balances = []provider.Balance{{Name: "credits", Unit: "credits", Remaining: &zero}}
	root, out, errOut := newTestRoot(t, p)
	root.SetArgs([]string{"usage", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	const want = "{\"windows\":[{\"provider\":\"codex\",\"name\":\"weekly\",\"remaining_percent\":80,\"rate_limited\":false}],\"errors\":[],\"undetected\":[]}\n"
	if out.String() != want {
		t.Errorf("JSON = %s, want %s", out.String(), want)
	}
	if errOut.Len() != 0 || len(*calls) != 0 {
		t.Fatal("JSON output produced stderr or checked for updates")
	}
}
