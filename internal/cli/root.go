// Package cli defines the qmeter command tree.
package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCmd builds the root command and attaches all subcommands.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "qmeter",
		Short:         "See your AI subscription usage limits",
		Long:          "qmeter is a CLI tool to see your AI subscription usage limits.",
		SilenceUsage:  true,
		SilenceErrors: false,
	}

	root.AddCommand(newVersionCmd())

	return root
}

// Execute runs the root command. It returns any error so main can set the exit code.
func Execute() error {
	return NewRootCmd().Execute()
}
