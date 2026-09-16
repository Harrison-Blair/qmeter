package main

import (
	"github.com/spf13/cobra"

	"github.com/Harrison-Blair/qmeter/cmd/version"
)

// NewRootCmd builds the root command and registers every subcommand.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "qmeter",
		Short:        "See your AI subscription usage limits",
		Long:         "qmeter is a CLI tool to see your AI subscription usage limits.",
		SilenceUsage: true,
	}

	root.AddCommand(version.New())

	return root
}
