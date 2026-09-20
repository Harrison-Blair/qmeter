package provider_test

import (
	"encoding/json"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

func TestBalanceMarshalJSON(t *testing.T) {
	zero, used, limit, remaining := 0.0, 1.25, 10.0, 8.75
	for _, tt := range []struct {
		name    string
		balance provider.Balance
		want    string
	}{
		{"unknown", provider.Balance{Provider: "codex", Name: "credits", Unit: "credits", Unlimited: true}, `{"provider":"codex","name":"credits","unit":"credits","used":null,"limit":null,"remaining":null,"unlimited":true}`},
		{"known zero", provider.Balance{Provider: "cursor", Name: "included", Unit: "unconfirmed", Used: &zero, Limit: &zero, Remaining: &zero}, `{"provider":"cursor","name":"included","unit":"unconfirmed","used":0,"limit":0,"remaining":0,"unlimited":false}`},
		{"money", provider.Balance{Provider: "claude", Name: "extra usage", Unit: "usd", Used: &used, Limit: &limit, Remaining: &remaining}, `{"provider":"claude","name":"extra usage","unit":"usd","used":1.25,"limit":10,"remaining":8.75,"unlimited":false}`},
		{"mixed", provider.Balance{Unit: "percent", Used: &zero}, `{"provider":"","name":"","unit":"percent","used":0,"limit":null,"remaining":null,"unlimited":false}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.balance)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("JSON = %s, want %s", got, tt.want)
			}
		})
	}
}
