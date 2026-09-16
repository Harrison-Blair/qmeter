package dash

import (
	"context"
	"errors"
	"io"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// RunOptions are the dashboard's run-time choices: what to fetch, whether
// to draw the wordmark, and which streams to talk to.
type RunOptions struct {
	// Providers are fetched on every refresh, already narrowed by
	// --filter.
	Providers []provider.Provider

	// Banner draws the wordmark as the pinned header.
	Banner bool

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

	model := New(Options{
		Providers: opts.Providers,
		Banner:    opts.Banner,
		Ctx:       ctx,
	})

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
