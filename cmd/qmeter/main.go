package main

import (
	"os"

	"github.com/Harrison-Blair/qmeter/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
