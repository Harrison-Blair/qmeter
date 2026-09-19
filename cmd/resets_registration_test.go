package cmd

import "testing"

func TestResetsRegistered(t *testing.T) {
	root := NewRootCmd()
	found, _, err := root.Find([]string{"resets"})
	if err != nil || found.Name() != "resets" {
		t.Fatalf("resets command missing: %v", err)
	}
	if found.Flags().Lookup("filter") == nil {
		t.Fatal("resets filter missing")
	}
}
