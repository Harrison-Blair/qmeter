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
	"github.com/Harrison-Blair/qmeter/internal/dash/theme"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
)

// footerStyle dims the key hints: they are the one thing on the page that
// is never about the numbers. The spinner takes the countdown's cyan: it is
// time passing, not a key.
var (
	footerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	spinStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)

// scheduleTick is the cancellable one-shot timer seam. Tests replace it so
// the refresh state machine can be driven without waiting on wall-clock time.
var scheduleTick = func(parent context.Context, d time.Duration, fn func(time.Time) tea.Msg) (tea.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	cmd := func() tea.Msg {
		timer := time.NewTimer(d)
		defer func() {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			cancel()
		}()

		select {
		case ticked := <-timer.C:
			return fn(ticked)
		case <-ctx.Done():
			return nil
		}
	}
	return cmd, cancel
}

// The footer's fixed half. While a fetch is in flight `r` does nothing, so
// the hint says what is happening instead of offering the key again, with
// a spinner where the key would be — and before the first result there is
// nothing to scroll either, so quitting is the only key worth naming.
const (
	keyHints      = "↑↓ scroll · r refresh · q quit"
	busyKeyHints  = " refreshing · q quit" // after the spinner
	firstKeyHints = " fetching · q quit"   // after the spinner
	scrollHint    = "↑↓ scroll · "
	fetchingLabel = "fetching…"

	// spinFrames is the braille spinner, one frame per spinInterval while a
	// fetch is in flight. The tick runs only then: a tick that lands after
	// the result is dropped, so an idle dashboard has no timer but the
	// refresh one.
	spinFrames   = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
	spinInterval = 80 * time.Millisecond

	// DefaultRefreshInterval keeps zero-value Options useful to direct callers.
	DefaultRefreshInterval = 60 * time.Second
)

// Options are everything the model needs from its caller.
type Options struct {
	// Providers are fetched on every refresh, in this order. It is the
	// registry, already narrowed by --filter.
	Providers []provider.Provider

	// Banner draws the wordmark as the pinned header; without it the
	// header is the one-line summary.
	Banner bool

	// Theme is the provider identity palette. The zero value uses the
	// built-in adaptive palette.
	Theme theme.Theme

	// MeterWidth is the preferred complete gauge width. Zero uses the
	// dashboard default.
	MeterWidth int

	// RefreshInterval is the delay after each completed fetch before the
	// next automatic refresh. Zero uses the dashboard default.
	RefreshInterval time.Duration

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
	providers         []provider.Provider
	banner            bool
	theme             theme.Theme
	meterWidth        int
	refreshInterval   time.Duration
	refreshGeneration uint64
	cancelRefresh     context.CancelFunc
	now               func() time.Time
	ctx               context.Context

	width  int
	height int

	res       usage.Result
	haveRes   bool
	updatedAt time.Time // when res arrived, by the model's clock
	loading   bool
	spin      int // the spinner frame, meaningful only while loading
	offset    int
}

// spinMsg is one spinner tick.
type spinMsg struct{}

// resultMsg carries a finished fetch back into the update loop.
type resultMsg struct {
	res usage.Result
}

// refreshMsg identifies the one-shot timer that produced it. Manual refreshes
// invalidate the current generation, so a timer already in flight cannot
// start an early fetch after the manual one completes.
type refreshMsg struct {
	generation uint64
}

// New builds the model. It starts out loading: Init's fetch is already on
// its way by the time the first frame is drawn.
func New(o Options) Model {
	m := Model{
		providers:       o.Providers,
		banner:          o.Banner,
		theme:           o.Theme,
		meterWidth:      o.MeterWidth,
		refreshInterval: o.RefreshInterval,
		now:             o.Now,
		ctx:             o.Ctx,
		loading:         true,
	}
	if m.now == nil {
		m.now = time.Now
	}
	if m.theme == (theme.Theme{}) {
		m.theme = theme.Default()
	}
	if m.meterWidth == 0 {
		m.meterWidth = layout.DefaultMeterWidth
	}
	if m.refreshInterval == 0 {
		m.refreshInterval = DefaultRefreshInterval
	}
	if m.ctx == nil {
		m.ctx = context.Background()
	}
	return m
}

// Init starts the first fetch.
func (m Model) Init() tea.Cmd {
	return m.startFetch()
}

// startFetch is the fetch together with the spinner that turns while it
// runs. Callers have already set loading.
func (m Model) startFetch() tea.Cmd {
	return tea.Batch(m.fetch(), spinTick())
}

// spinTick asks for the next spinner frame after spinInterval.
func spinTick() tea.Cmd {
	return tea.Tick(spinInterval, func(time.Time) tea.Msg { return spinMsg{} })
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

// scheduleRefresh arms one one-shot timer after a fetch has completed.
func (m *Model) scheduleRefresh() tea.Cmd {
	m.cancelRefreshTimer()
	m.refreshGeneration++
	generation := m.refreshGeneration
	cmd, cancel := scheduleTick(m.ctx, m.refreshInterval, func(time.Time) tea.Msg {
		return refreshMsg{generation: generation}
	})
	m.cancelRefresh = cancel
	return cmd
}

// cancelRefreshTimer is safe across the value copies Bubble Tea makes of the
// model: context cancellation is idempotent, and the returned model drops its
// ownership of the handle.
func (m *Model) cancelRefreshTimer() {
	if m.cancelRefresh == nil {
		return
	}
	m.cancelRefresh()
	m.cancelRefresh = nil
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
		m.updatedAt = m.now()
		m.spin = 0
		m.offset = m.clamp(m.offset)
		cmd := m.scheduleRefresh()
		return m, cmd

	case refreshMsg:
		if m.loading || msg.generation != m.refreshGeneration {
			return m, nil
		}
		m.cancelRefreshTimer()
		m.loading = true
		return m, m.startFetch()

	case spinMsg:
		// The spinner outlives nothing: a tick after the result is the
		// timer winding down, and it is not re-armed.
		if !m.loading {
			return m, nil
		}
		m.spin = (m.spin + 1) % len([]rune(spinFrames))
		return m, spinTick()

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
		m.cancelRefreshTimer()
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
		m.cancelRefreshTimer()
		m.refreshGeneration++
		m.loading = true
		return m, m.startFetch()
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
	page := layout.Render(m.res, m.width, layout.Options{
		Banner:     m.banner,
		Now:        m.now(),
		Theme:      m.theme,
		MeterWidth: m.meterWidth,
	})

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

// footer is the key hints, preceded by what is out of sight and followed,
// at the right edge, by when the numbers on screen were fetched.
//
// On a terminal too narrow for all of it, the update time goes first, then
// the counts, and the keys stay: how to leave is the one thing the footer
// must never truncate, and the rows out of sight announce themselves the
// moment the page is scrolled.
func (m Model) footer(total, fits int) string {
	hints := m.hints()

	var counts []string
	if m.offset > 0 {
		counts = append(counts, fmt.Sprintf("↑ %d more", m.offset))
	}
	if below := total - fits - m.offset; below > 0 {
		counts = append(counts, fmt.Sprintf("↓ %d more", below))
	}
	left := hints
	if len(counts) > 0 {
		left = append([]seg{{strings.Join(counts, " · ") + " · ", footerStyle}}, hints...)
	}

	if m.haveRes {
		stamp := seg{"updated " + m.updatedAt.Format("15:04:05"), footerStyle}
		if gap := m.width - cells(left...) - cells(stamp); gap >= 2 {
			return render(left...) + strings.Repeat(" ", gap) + render(stamp)
		}
	}
	if cells(left...) > m.width {
		left = hints
	}
	if cells(left...) > m.width {
		return footerStyle.Render(fit(plain(left...), m.width))
	}
	return render(left...) + strings.Repeat(" ", m.width-cells(left...))
}

// hints is the footer's key half: the keys, or what is happening instead
// of the key that would do nothing.
func (m Model) hints() []seg {
	frame := seg{string([]rune(spinFrames)[m.spin]), spinStyle}
	switch {
	case m.loading && !m.haveRes:
		return []seg{frame, {firstKeyHints, footerStyle}}
	case m.loading:
		return []seg{{scrollHint, footerStyle}, frame, {busyKeyHints, footerStyle}}
	}
	return []seg{{keyHints, footerStyle}}
}

// seg is a run of footer text in one style; the footer is measured on the
// text and rendered segment by segment, so a styled run inside it does
// not reset the style of what follows.
type seg struct {
	text  string
	style lipgloss.Style
}

func cells(segs ...seg) int {
	n := 0
	for _, s := range segs {
		n += runewidth.StringWidth(s.text)
	}
	return n
}

func plain(segs ...seg) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.text)
	}
	return b.String()
}

func render(segs ...seg) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.style.Render(s.text))
	}
	return b.String()
}

// fit trims s to width cells and pads it out to exactly that many.
func fit(s string, width int) string {
	if runewidth.StringWidth(s) > width {
		s = runewidth.Truncate(s, width, "")
	}
	return s + strings.Repeat(" ", width-runewidth.StringWidth(s))
}
