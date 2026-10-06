// Package dash wires the root command's own behaviour: bare `qmeter` is
// the live dashboard. It is wiring only — flags in, internal/dash out —
// and it owns exactly one decision the dashboard cannot make for itself:
// whether there is a terminal to draw on at all.
package dash

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	idash "github.com/Harrison-Blair/qmeter/internal/dash"
	dconfig "github.com/Harrison-Blair/qmeter/internal/dash/config"
	iupdate "github.com/Harrison-Blair/qmeter/internal/update"
	"github.com/Harrison-Blair/qmeter/internal/usage"
)

// The collaborators, indirected through variables so the tests in this
// package drive every branch without a credential store, the network or a
// terminal.
var (
	registry         = usage.Registry
	hint             = iupdate.Hint
	run              = idash.Run
	loadConfig       = dconfig.Load
	stdoutIsTerminal = func() bool { return term.IsTerminal(int(os.Stdout.Fd())) }
)

// validProviders is the registry's provider names, in registry order, for
// the --filter help text and the unknown-provider error.
const validProviders = "claude, codex, opencode-go, cursor"

// Attach gives root its own behaviour: the dashboard, its flags, and
// the refusal of any argument that is not a subcommand.
func Attach(root *cobra.Command) {
	root.Args = cobra.NoArgs
	root.Flags().StringSlice("filter", nil, "show only these providers ("+validProviders+")")
	root.Flags().Bool("no-banner", false, "hide the qmeter wordmark")
	root.Flags().Int("thickness", dconfig.DefaultMeterThickness, fmt.Sprintf("number of meter track rows (%d–%d)", dconfig.MinMeterThickness, dconfig.MaxMeterThickness))
	root.Flags().Bool("vertical", false, "stack providers vertically (meters honor --width)")
	root.Flags().Int("width", 0, fmt.Sprintf("gauge width including caps, in terminal cells (%d–%d); overrides meter_width, shrinks to fit", dconfig.MinMeterWidth, dconfig.MaxMeterWidth))

	root.RunE = func(cmd *cobra.Command, _ []string) error {
		names, err := cmd.Flags().GetStringSlice("filter")
		if err != nil {
			return err
		}
		noBanner, err := cmd.Flags().GetBool("no-banner")
		if err != nil {
			return err
		}
		vertical, err := cmd.Flags().GetBool("vertical")
		if err != nil {
			return err
		}
		thickness, err := cmd.Flags().GetInt("thickness")
		if err != nil {
			return err
		}
		if thickness < dconfig.MinMeterThickness || thickness > dconfig.MaxMeterThickness {
			msg := fmt.Sprintf("thickness must be between %d and %d", dconfig.MinMeterThickness, dconfig.MaxMeterThickness)
			fmt.Fprintln(cmd.ErrOrStderr(), msg)
			cmd.SilenceErrors = true
			return errors.New(msg)
		}
		width, err := cmd.Flags().GetInt("width")
		if err != nil {
			return err
		}
		if cmd.Flags().Changed("width") && (width < dconfig.MinMeterWidth || width > dconfig.MaxMeterWidth) {
			msg := fmt.Sprintf("width must be between %d and %d", dconfig.MinMeterWidth, dconfig.MaxMeterWidth)
			fmt.Fprintln(cmd.ErrOrStderr(), msg)
			cmd.SilenceErrors = true
			return errors.New(msg)
		}
		// --json is a persistent flag on the root command; a root built
		// without it simply draws.
		asJSON, _ := cmd.Flags().GetBool("json")

		// The filter is checked before anything is fetched, so a typo
		// costs nothing and reads the same as `usage --provider`'s.
		providers, unknown := usage.Select(registry(), names)
		if unknown != "" {
			msg := fmt.Sprintf("unknown provider %q (valid: %s)", unknown, validProviders)
			fmt.Fprintln(cmd.ErrOrStderr(), msg)
			// Silence cobra's own report of this one error so the line
			// above is not printed twice, without also swallowing its
			// reports of a bad argument or an unknown flag.
			cmd.SilenceErrors = true
			return errors.New(msg)
		}

		ctx := cmd.Context()

		// Two ways out of the dashboard: asked for machine-readable
		// output, or writing somewhere that cannot be drawn on (a pipe, a
		// file, a CI log). Both behave exactly like `qmeter usage`.
		if asJSON || !stdoutIsTerminal() {
			res := usage.Run(ctx, providers, "")
			// The run was cut short (an interrupt, a caller deadline), so
			// every provider reports "canceled"; print nothing rather
			// than a table of cancellations.
			if ctx.Err() != nil {
				return nil
			}
			if asJSON {
				return usage.RenderJSON(cmd.OutOrStdout(), res)
			}
			if err := usage.RenderText(cmd.OutOrStdout(), res); err != nil {
				return err
			}
			hint(ctx, cmd.ErrOrStderr(), iupdate.HintOptions{})
			return nil
		}

		settings, configErr := loadConfig()
		if configErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", configErr)
			settings = dconfig.Default()
		}
		if cmd.Flags().Changed("width") {
			settings.MeterWidth = width
		}
		if cmd.Flags().Changed("thickness") {
			settings.MeterThickness = thickness
		}
		if err := run(ctx, idash.RunOptions{
			Providers:       providers,
			Banner:          !noBanner,
			Vertical:        vertical,
			Theme:           settings.Theme,
			MeterWidth:      settings.MeterWidth,
			GaugeWidth:      width,
			MeterThickness:  settings.MeterThickness,
			RefreshInterval: settings.RefreshInterval,
		}); err != nil {
			return err
		}
		// Only once the dashboard has given the screen back: a courtesy
		// line, never drawn over the page and never in the JSON envelope.
		hint(ctx, cmd.ErrOrStderr(), iupdate.HintOptions{})
		return nil
	}
}
