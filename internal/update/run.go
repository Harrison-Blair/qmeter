package update

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/Harrison-Blair/qmeter/internal/version"
)

// Options are the inputs to Run. Everything that touches the network, the
// filesystem, the terminal or the clock is a field, so a test can drive the
// whole command without a real release or a real binary swap.
type Options struct {
	// Current is the running version; empty means version.String().
	Current string

	// Check only reports current vs latest: it never prompts, downloads or
	// replaces anything.
	Check bool

	// Yes skips the confirmation prompt. It is the only bypass.
	Yes bool

	// JSON switches the reported result to the machine-readable form. The
	// prompt stays text on stderr either way.
	JSON bool

	// BaseURL is the release host; empty means DefaultBaseURL.
	BaseURL string

	// Client is the http.Client used for every request; nil means
	// http.DefaultClient. No Timeout is ever set on it: the deadline comes
	// from the context.
	Client *http.Client

	// GOOS and GOARCH select the release asset; empty means the values
	// this binary was built for.
	GOOS, GOARCH string

	// ExecPath is the binary to replace; empty means the running one,
	// resolved through os.Executable and filepath.EvalSymlinks.
	ExecPath string

	// Stdin is where the confirmation answer is read from; nil means
	// os.Stdin.
	Stdin io.Reader

	// Out receives the result, Err the prompt and any warning. Nil means
	// os.Stdout and os.Stderr.
	Out, Err io.Writer

	// IsTerminal reports whether Stdin is an interactive terminal; nil
	// means a real check of os.Stdin.
	IsTerminal func() bool
}

// Result is what an update run reports, and the shape of its --json output.
type Result struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"update_available"`
	Updated         bool   `json:"updated"`
}

func (o Options) current() string {
	if o.Current != "" {
		return o.Current
	}
	return version.String()
}

func (o Options) goos() string {
	if o.GOOS != "" {
		return o.GOOS
	}
	return currentGOOS()
}

func (o Options) goarch() string {
	if o.GOARCH != "" {
		return o.GOARCH
	}
	return runtime.GOARCH
}

func (o Options) out() io.Writer {
	if o.Out != nil {
		return o.Out
	}
	return os.Stdout
}

func (o Options) errOut() io.Writer {
	if o.Err != nil {
		return o.Err
	}
	return os.Stderr
}

func (o Options) stdin() io.Reader {
	if o.Stdin != nil {
		return o.Stdin
	}
	return os.Stdin
}

func (o Options) isTerminal() bool {
	if o.IsTerminal != nil {
		return o.IsTerminal()
	}
	return StdinIsTerminal()
}

// StdinIsTerminal reports whether the process's stdin is an interactive
// terminal rather than a pipe or a file.
func StdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// Run performs `qmeter update`. It returns an error only for a genuine
// failure: a declined update is a successful, deliberate no-op and exits 0.
func Run(ctx context.Context, opts Options) error {
	src := Source{BaseURL: opts.BaseURL, Client: opts.Client}

	current := opts.current()
	latest, err := src.LatestTag(ctx)
	if err != nil {
		return err
	}

	res := Result{Current: current, Latest: latest}
	cmp, cmpErr := CompareVersions(current, latest)
	comparable := cmpErr == nil
	// An uncomparable current version — a dev build, a binary built from a
	// branch — cannot be ruled out as stale, so the latest release is
	// offered rather than assumed unnecessary.
	res.UpdateAvailable = !comparable || cmp < 0

	if !comparable {
		fmt.Fprintf(opts.errOut(),
			"warning: cannot compare the running version (%q) with the latest release; "+
				"this is not a released build\n", current)
	}

	if opts.Check {
		opts.report(res, checkLine(res, comparable))
		return nil
	}

	if comparable && !res.UpdateAvailable {
		opts.report(res, fmt.Sprintf("qmeter %s is up to date", current))
		return nil
	}

	if !opts.Yes {
		if !opts.isTerminal() {
			return fmt.Errorf("update: stdin is not a terminal, so there is nobody to confirm with; " +
				"re-run with --yes to update without a prompt")
		}
		prompt := fmt.Sprintf("Update qmeter from %s to %s?", current, latest)
		if !comparable {
			prompt = fmt.Sprintf("Install qmeter %s?", latest)
		}
		if !confirm(opts.stdin(), opts.errOut(), prompt) {
			fmt.Fprintln(opts.errOut(), "update canceled")
			opts.report(res, "")
			return nil
		}
	}

	execPath := opts.ExecPath
	if execPath == "" {
		execPath, err = resolveExecutable(os.Executable)
		if err != nil {
			return err
		}
	}

	bin, err := src.Binary(ctx, latest, opts.goos(), opts.goarch())
	if err != nil {
		return err
	}
	if err := (replacer{GOOS: opts.goos(), ExecPath: execPath}).replace(bin); err != nil {
		return err
	}

	res.Updated = true
	opts.report(res, fmt.Sprintf("qmeter updated to %s", latest))
	return nil
}

// checkLine is the one-line human answer --check gives.
func checkLine(res Result, comparable bool) string {
	switch {
	case !comparable:
		return fmt.Sprintf("the latest release is %s; run qmeter update to install it", res.Latest)
	case res.UpdateAvailable:
		return availableLine(res.Latest, res.Current)
	default:
		return fmt.Sprintf("qmeter %s is up to date", res.Current)
	}
}

// report writes the run's outcome: the JSON result, or the given text line
// when there is one. A canceled run passes an empty line because it has
// already said "update canceled" on stderr.
func (o Options) report(res Result, line string) {
	if o.JSON {
		enc := json.NewEncoder(o.out())
		enc.SetEscapeHTML(false)
		_ = enc.Encode(res)
		return
	}
	if line != "" {
		fmt.Fprintln(o.out(), line)
	}
}

// confirm writes prompt to w and reads one line from r. Only an exact "y"
// or "Y", once surrounding space is trimmed, is a yes; an empty line, any
// other answer and EOF are all no.
func confirm(r io.Reader, w io.Writer, prompt string) bool {
	fmt.Fprintf(w, "%s [y/N] ", prompt)
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && line == "" {
		// EOF (a closed stdin, a Ctrl-D) declines.
		fmt.Fprintln(w)
		return false
	}
	answer := strings.TrimSpace(line)
	return answer == "y" || answer == "Y"
}
