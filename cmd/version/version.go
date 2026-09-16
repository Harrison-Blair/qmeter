// Package version defines the `qmeter version` command.
package version

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Harrison-Blair/qmeter/internal/version"
)

// New returns the version command.
func New() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the qmeter version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "qmeter", version.String())
			return err
		},
	}
}
