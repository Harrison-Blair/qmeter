package dash

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Harrison-Blair/qmeter/internal/dash/theme"
	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/charmbracelet/lipgloss"
)

var (
	destinationRenderer = display.Renderer
	queryBackground     = func(r *lipgloss.Renderer) bool { return r.HasDarkBackground() }
)

func newDashboardRenderer(w io.Writer) *lipgloss.Renderer {
	r := destinationRenderer(w)
	r.SetHasDarkBackground(queryBackground(r))
	return r
}

// RunOptions are the dashboard's run-time choices and terminal streams.
type RunOptions struct {
	// Providers are fetched on every refresh, already narrowed by
	// --filter.
	Providers []provider.Provider

	// Theme is the provider identity palette. Its zero value uses the
	// built-in adaptive palette.
	Theme theme.Theme

	// RefreshInterval is the delay after each completed fetch before the
	// next automatic refresh. Zero uses the dashboard default.
	RefreshInterval time.Duration

	// Input and Output are the terminal to run on. Both nil means the
	// process's own stdin and stdout; tests pass buffers.
	Input  io.Reader
	Output io.Writer
}

// Run draws the dashboard until the user quits, on the alternate screen so
// the terminal is left exactly as it was found.
//
// Cancelling ctx ends the run — and cuts any fetch in flight short — which
// is how an interrupt reaches it, so a cancelled run is reported as
// success: there is nothing to tell the user that pressing the key did not
// already say.
func Run(ctx context.Context, opts RunOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	output := opts.Output
	if output == nil {
		output = os.Stdout
	}
	renderer := newDashboardRenderer(output)
	model := New(modelOptions(ctx, opts, renderer))

	popts := []tea.ProgramOption{tea.WithAltScreen(), tea.WithContext(ctx)}
	if opts.Input != nil {
		popts = append(popts, tea.WithInput(opts.Input))
	}
	if opts.Output != nil {
		popts = append(popts, tea.WithOutput(opts.Output))
	}

	if _, err := tea.NewProgram(model, popts...).Run(); err != nil {
		if errors.Is(err, tea.ErrProgramKilled) {
			return nil
		}
		return err
	}
	return nil
}

func modelOptions(ctx context.Context, opts RunOptions, renderer *lipgloss.Renderer) Options {
	return Options{
		Providers:       opts.Providers,
		Theme:           opts.Theme,
		RefreshInterval: opts.RefreshInterval,
		Ctx:             ctx,
		Renderer:        renderer,
	}
}
