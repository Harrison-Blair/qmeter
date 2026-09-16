// Package dash is the qmeter dashboard: the live terminal app bare
// `qmeter` runs. The page itself is drawn by the nested packages — banner
// the wordmark, gauge one fuel gauge, layout the whole page at a width —
// and this package is the Bubble Tea model around them: it owns the
// terminal size, the current usage.Result, the scroll offset and the keys.
//
// The model is pure and needs no terminal: every input arrives as a
// tea.Msg and View returns the frame as a string. Run wires it to a real
// terminal, and is the only part of the package a test cannot drive.
package dash

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/Harrison-Blair/qmeter/internal/dash/banner"
	"github.com/Harrison-Blair/qmeter/internal/dash/layout"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
)

// footerStyle dims the key hints: they are the one thing on the page that
// is never about the numbers.
var footerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

// The footer's fixed half. While a fetch is in flight `r` does nothing, so
// the hint says what is happening instead of offering the key again — and
// before the first result there is nothing to scroll either, so quitting is
// the only key worth naming.
const (
	keyHints      = "↑↓ scroll · r refresh · q quit"
	busyKeyHints  = "↑↓ scroll · refreshing… · q quit"
	firstKeyHints = "fetching… · q quit"
	fetchingLabel = "fetching…"
)

// Options are everything the model needs from its caller.
type Options struct {
	// Providers are fetched on every refresh, in this order. It is the
	// registry, already narrowed by --filter.
	Providers []provider.Provider

	// Banner draws the wordmark as the pinned header; without it the
	// header is the one-line summary.
	Banner bool

	// Now is the instant countdowns are measured from, read again on
	// every frame so they tick. Nil means time.Now.
	Now func() time.Time

	// Ctx bounds every fetch the model starts. The model holds it (rather
	// than taking one per fetch) because Bubble Tea hands it no context:
	// commands run on their own goroutines, and cancelling this one is
	// what stops an in-flight fetch when the program quits. Nil means
	// context.Background.
	Ctx context.Context
}

// Model is the dashboard's Bubble Tea model.
type Model struct {
	providers []provider.Provider
	banner    bool
	now       func() time.Time
	ctx       context.Context

	width  int
	height int

	res     usage.Result
	haveRes bool
	loading bool
	offset  int
}

// resultMsg carries a finished fetch back into the update loop.
type resultMsg struct {
	res usage.Result
}

// New builds the model. It starts out loading: Init's fetch is already on
// its way by the time the first frame is drawn.
func New(o Options) Model {
	m := Model{
		providers: o.Providers,
		banner:    o.Banner,
		now:       o.Now,
		ctx:       o.Ctx,
		loading:   true,
	}
	if m.now == nil {
		m.now = time.Now
	}
	if m.ctx == nil {
		m.ctx = context.Background()
	}
	return m
}

// Init starts the first fetch.
func (m Model) Init() tea.Cmd {
	return m.fetch()
}

// fetch runs every provider and delivers the result as a resultMsg. The
// fetch is bounded by the model's context, so quitting the program cuts it
// short instead of leaving it to finish into nothing.
func (m Model) fetch() tea.Cmd {
	ctx, providers := m.ctx, m.providers
	return func() tea.Msg {
		return resultMsg{res: usage.Run(ctx, providers, "")}
	}
}

// Update folds one message into the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.offset = m.clamp(m.offset)
		return m, nil

	case resultMsg:
		m.res, m.haveRes, m.loading = msg.res, true, false
		m.offset = m.clamp(m.offset)
		return m, nil

	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

// key handles one key press. Scrolling is clamped to the page, so a key
// that cannot move never redraws anything different.
func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	_, body, fits := m.frame()
	page := fits
	if page < 1 {
		page = 1
	}

	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.offset = m.clamp(m.offset + 1)
	case "k", "up":
		m.offset = m.clamp(m.offset - 1)
	case "pgdown", " ":
		m.offset = m.clamp(m.offset + page)
	case "pgup", "b":
		m.offset = m.clamp(m.offset - page)
	case "g", "home":
		m.offset = 0
	case "G", "end":
		m.offset = m.clamp(len(body))
	case "r":
		// A second refresh while one is in flight would fetch every
		// provider twice and race its own result onto the screen.
		if m.loading {
			return m, nil
		}
		m.loading = true
		return m, m.fetch()
	}
	return m, nil
}

// View draws the frame: the pinned header, the visible slice of the body,
// and one footer row — always exactly height lines of exactly width cells,
// so Bubble Tea's alt screen never keeps a stale row.
func (m Model) View() string {
	if m.width < 1 || m.height < 1 {
		return ""
	}

	header, body, fits := m.frame()
	lines := make([]string, 0, m.height)
	lines = append(lines, header...)

	start := min(m.offset, len(body))
	end := min(start+fits, len(body))
	lines = append(lines, body[start:end]...)

	blank := strings.Repeat(" ", m.width)
	for len(lines) < m.height-1 {
		lines = append(lines, blank)
	}
	return strings.Join(append(lines, m.footer(len(body), fits)), "\n")
}

// frame splits the page into the header that stays put and the body that
// scrolls, and says how many body rows are on screen.
func (m Model) frame() (header, body []string, fits int) {
	page := layout.Render(m.res, m.width, layout.Options{Banner: m.banner, Now: m.now()})

	// The header is the banner, or the summary line that replaces it.
	// Below MinWidth the page is a single apology, and there is nothing to
	// pin: it is the body, so a narrow terminal still shows why it is
	// empty rather than an empty screen.
	head := 1
	if m.banner && m.width >= banner.Width {
		head = banner.Height
	}
	if m.width >= layout.MinWidth && len(page) > head {
		header, body = page[:head], page[head:]
	} else {
		body = page
	}

	if m.loading && !m.haveRes && m.width >= layout.MinWidth {
		body = []string{fit(fetchingLabel, m.width)}
	}

	// A terminal too short for the header loses its bottom rows rather
	// than pushing the footer off screen.
	if len(header) > m.height-1 {
		header = header[:max(0, m.height-1)]
	}
	return header, body, max(0, m.height-len(header)-1)
}

// clamp holds an offset inside the page: never below the top, never past
// the last screenful.
func (m Model) clamp(off int) int {
	_, body, fits := m.frame()
	return max(0, min(off, len(body)-fits))
}

// footer is the key hints, preceded by what is out of sight.
//
// On a terminal too narrow for both, the counts go and the keys stay: how
// to leave is the one thing the footer must never truncate, and the rows
// out of sight announce themselves the moment the page is scrolled.
func (m Model) footer(total, fits int) string {
	hints := keyHints
	switch {
	case m.loading && !m.haveRes:
		hints = firstKeyHints
	case m.loading:
		hints = busyKeyHints
	}

	var parts []string
	if m.offset > 0 {
		parts = append(parts, fmt.Sprintf("↑ %d more", m.offset))
	}
	if below := total - fits - m.offset; below > 0 {
		parts = append(parts, fmt.Sprintf("↓ %d more", below))
	}

	line := strings.Join(append(parts, hints), " · ")
	if runewidth.StringWidth(line) > m.width {
		line = hints
	}
	return footerStyle.Render(fit(line, m.width))
}

// fit trims s to width cells and pads it out to exactly that many.
func fit(s string, width int) string {
	if runewidth.StringWidth(s) > width {
		s = runewidth.Truncate(s, width, "")
	}
	return s + strings.Repeat(" ", width-runewidth.StringWidth(s))
}
