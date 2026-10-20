// Package resets wires the one-shot resets command to provider collection and rendering.
package resets

import (
	"errors"
	"fmt"
	"strings"
	"time"

	iresets "github.com/Harrison-Blair/qmeter/internal/resets"
	iupdate "github.com/Harrison-Blair/qmeter/internal/update"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/spf13/cobra"
)

var (
	registry = usage.Registry
	hint     = iupdate.Hint
	now      = time.Now
)

// New returns the resets command.
func New() *cobra.Command {
	providers := registry()
	names := make([]string, 0, len(providers))
	for _, p := range providers {
		names = append(names, p.ID())
	}
	validProviders := strings.Join(names, ", ")
	cmd := &cobra.Command{
		Use: "resets", Short: "Show usage windows in reset order",
		Long: "List each detected usage window in reset order, soonest first. Each row shows the countdown, provider, window, reset date and time, and current percentage. Windows with no reset time appear last.",
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
			fetchedAt := now()
			if asJSON {
				return iresets.RenderJSON(cmd.OutOrStdout(), res, fetchedAt)
			}
			if err := iresets.RenderText(cmd.OutOrStdout(), res, fetchedAt); err != nil {
				return err
			}
			hint(ctx, cmd.ErrOrStderr(), iupdate.HintOptions{})
			return nil
		},
	}
	cmd.Flags().StringSlice("filter", nil, "show only these providers ("+validProviders+")")
	return cmd
}
