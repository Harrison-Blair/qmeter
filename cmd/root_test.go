package main

import "testing"

func TestRoot_RegistersUsageAndPersistentJSONFlag(t *testing.T) {
	root := NewRootCmd()

	if f := root.PersistentFlags().Lookup("json"); f == nil {
		t.Error("root has no persistent --json flag")
	}

	for _, name := range []string{"usage", "version"} {
		found := false
		for _, sub := range root.Commands() {
			if sub.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("root does not register the %q subcommand", name)
		}
	}
}
