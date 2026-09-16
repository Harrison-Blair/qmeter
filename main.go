// Command qmeter is a CLI tool to see your AI subscription usage limits.
package main

import (
	"os"

	"github.com/Harrison-Blair/qmeter/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
