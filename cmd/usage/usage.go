// Package usage defines the `qmeter usage` command. It is wiring only:
// flags in, internal/usage out — every decision about what to fetch, what
// a failure reads like and how a result is formatted lives there.
package usage

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	iupdate "github.com/Harrison-Blair/qmeter/internal/update"
	"github.com/Harrison-Blair/qmeter/internal/usage"
)

// registry is the set of providers the command runs, indirected through a
// variable so tests can substitute fakes instead of reaching for real
// credential stores and the network.
var registry = usage.Registry

// hint is internal/update.Hint, indirected through a variable so the tests
// in this package never reach the network and can assert the hint is left
// out of machine-readable output.
var hint = iupdate.Hint

// validProviders is the registry's provider names, in registry order, for
// the --provider help text and the unknown-provider error.
const validProviders = "claude, codex, opencode-go, cursor"

// New returns the usage command.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Show your AI subscription usage limits",
		Long: "Show usage limits for every detected provider.\n\n" +
			"Undetected providers are omitted unless --provider names one.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			only, err := cmd.Flags().GetString("provider")
			if err != nil {
				return err
			}
			// --json is a persistent flag on the root command; a usage
			// command used outside that root simply renders text.
			asJSON, _ := cmd.Flags().GetBool("json")

			ctx := cmd.Context()
			res := usage.Run(ctx, registry(), only)

			// The only exit-1 condition: --provider named a provider that
			// does not exist. It is checked before everything else,
			// because then no provider was detected or fetched and every
			// other field of res is empty.
			if res.UnknownProvider {
				msg := fmt.Sprintf("unknown provider %q (valid: %s)", only, validProviders)
				fmt.Fprintln(cmd.ErrOrStderr(), msg)
				// Silence cobra's own report of this one error so the line
				// above is not printed twice. It is set here rather than on
				// the command literal, where it would also swallow cobra's
				// reports of a bad argument or an unknown flag and turn
				// those into a silent exit 1.
				cmd.SilenceErrors = true
				return errors.New(msg)
			}
			// The run was cut short (an interrupt, a caller deadline), so
			// every provider reports "canceled"; print nothing rather than
			// a table of cancellations.
			if ctx.Err() != nil {
				return nil
			}

			if asJSON {
				return usage.RenderJSON(cmd.OutOrStdout(), res)
			}
			if err := usage.RenderText(cmd.OutOrStdout(), res); err != nil {
				return err
			}
			// A courtesy line on stderr, never in the JSON envelope and
			// never on a run that was cut short. It cannot fail: Hint
			// returns nothing and swallows every error of its own.
			hint(ctx, cmd.ErrOrStderr(), iupdate.HintOptions{})
			return nil
		},
	}

	cmd.Flags().String("provider", "", "show only this provider ("+validProviders+")")

	return cmd
}
