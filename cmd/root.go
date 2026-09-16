package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Harrison-Blair/qmeter/cmd/update"
	"github.com/Harrison-Blair/qmeter/cmd/usage"
	"github.com/Harrison-Blair/qmeter/cmd/version"
	iupdate "github.com/Harrison-Blair/qmeter/internal/update"
)

// cleanupOld sweeps up the <exe>.old file a previous Windows update left
// behind. It is indirected through a variable so a test can prove the hook
// runs — and that a broken one cannot take a command down with it.
var cleanupOld = iupdate.CleanupOld

// NewRootCmd builds the root command and registers every subcommand.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "qmeter",
		Short:        "See your AI subscription usage limits",
		Long:         "qmeter is a CLI tool to see your AI subscription usage limits.",
		SilenceUsage: true,
		// Windows cannot replace a running image, so `qmeter update`
		// leaves the previous binary beside the new one. Every later run
		// tries to remove it. This is housekeeping, not part of any
		// command: it must never fail one, so even a panic is contained.
		PersistentPreRun: func(*cobra.Command, []string) {
			defer func() { _ = recover() }()
			cleanupOld()
		},
	}

	// --json is persistent so every subcommand that has a machine-readable
	// form reads the same flag; cmd/usage is the first.
	root.PersistentFlags().Bool("json", false, "output JSON instead of text")

	root.AddCommand(usage.New())
	root.AddCommand(version.New())
	root.AddCommand(update.New())

	return root
}
