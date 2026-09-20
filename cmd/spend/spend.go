// Package spend wires the one-shot spend command to provider collection and rendering.
package spend

import (
	"errors"
	"fmt"
	"strings"

	ispend "github.com/Harrison-Blair/qmeter/internal/spend"
	iupdate "github.com/Harrison-Blair/qmeter/internal/update"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/spf13/cobra"
)

var (
	registry = usage.Registry
	hint     = iupdate.Hint
)

// New returns the spend command.
func New() *cobra.Command {
	providers := registry()
	names := make([]string, 0, len(providers))
	for _, p := range providers {
		names = append(names, p.ID())
	}
	validProviders := strings.Join(names, ", ")
	cmd := &cobra.Command{
		Use: "spend", Short: "Show reported money and credit balances",
		Long: "List reported balances and limits for detected providers. Amounts with unconfirmed units are shown without a currency symbol.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			names, err := cmd.Flags().GetStringSlice("filter")
			if err != nil {
				return err
			}
			selected, unknown := usage.Select(providers, names)
			if unknown != "" {
				msg := fmt.Sprintf("unknown provider %q (valid: %s)", unknown, validProviders)
				fmt.Fprintln(cmd.ErrOrStderr(), msg)
				cmd.SilenceErrors = true
				return errors.New(msg)
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			ctx := cmd.Context()
			res := usage.Run(ctx, selected, "")
			if ctx.Err() != nil {
				return nil
			}
			if asJSON {
				return ispend.RenderJSON(cmd.OutOrStdout(), res)
			}
			if err := ispend.RenderText(cmd.OutOrStdout(), res); err != nil {
				return err
			}
			hint(ctx, cmd.ErrOrStderr(), iupdate.HintOptions{})
			return nil
		},
	}
	cmd.Flags().StringSlice("filter", nil, "show only these providers ("+validProviders+")")
	return cmd
}
