// Package pace wires the one-shot pace command to provider collection and rendering.
package pace

import (
	"errors"
	"fmt"
	"strings"
	"time"

	ipace "github.com/Harrison-Blair/qmeter/internal/pace"
	iupdate "github.com/Harrison-Blair/qmeter/internal/update"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/spf13/cobra"
)

var (
	registry = usage.Registry
	hint     = iupdate.Hint
	now      = time.Now
)

// New returns the pace command.
func New() *cobra.Command {
	providers := registry()
	names := make([]string, 0, len(providers))
	for _, p := range providers {
		names = append(names, p.ID())
	}
	validProviders := strings.Join(names, ", ")
	cmd := &cobra.Command{
		Use: "pace", Short: "Compare remaining allowance with remaining time",
		Long: "List every detected usage limit and compare remaining allowance with remaining time.\n\nAhead means less allowance remains than expected; on pace is within five percentage points.",
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
				return ipace.RenderJSON(cmd.OutOrStdout(), res, fetchedAt)
			}
			if err := ipace.RenderText(cmd.OutOrStdout(), res, fetchedAt); err != nil {
				return err
			}
			hint(ctx, cmd.ErrOrStderr(), iupdate.HintOptions{})
			return nil
		},
	}
	cmd.Flags().StringSlice("filter", nil, "show only these providers ("+validProviders+")")
	return cmd
}
