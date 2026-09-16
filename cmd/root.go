package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Harrison-Blair/qmeter/cmd/usage"
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

	// --json is persistent so every subcommand that has a machine-readable
	// form reads the same flag; cmd/usage is the first.
	root.PersistentFlags().Bool("json", false, "output JSON instead of text")

	root.AddCommand(usage.New())
	root.AddCommand(version.New())

	return root
}
