// Package update defines the `qmeter update` command. It is wiring only:
// flags in, internal/update out — finding the latest release, asking for
// confirmation, verifying the download and swapping the binary all live
// there.
package update

import (
	"context"

	"github.com/spf13/cobra"

	iupdate "github.com/Harrison-Blair/qmeter/internal/update"
)

// run is internal/update.Run, indirected through a variable so a wiring
// test can assert the Options the flags build without touching the network
// or the running binary.
var run = iupdate.Run

// New returns the update command.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update qmeter to the latest release",
		Long: "Update qmeter to the latest release, whichever way it was installed.\n\n" +
			"The new binary is downloaded, checked against the release's published\n" +
			"sha256 and swapped in place. Nothing is downloaded until you confirm.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			check, err := cmd.Flags().GetBool("check")
			if err != nil {
				return err
			}
			yes, err := cmd.Flags().GetBool("yes")
			if err != nil {
				return err
			}
			// --json is a persistent flag on the root command; an update
			// command used outside that root simply reports text.
			asJSON, _ := cmd.Flags().GetBool("json")

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return run(ctx, iupdate.Options{
				Check: check,
				Yes:   yes,
				JSON:  asJSON,
				Stdin: cmd.InOrStdin(),
				Out:   cmd.OutOrStdout(),
				Err:   cmd.ErrOrStderr(),
			})
		},
	}

	cmd.Flags().Bool("check", false, "only report whether a newer release exists")
	cmd.Flags().Bool("yes", false, "update without asking for confirmation")

	return cmd
}
