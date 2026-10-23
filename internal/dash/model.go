// Package dash provides the Bubble Tea model for the full-screen dashboard.
package dash

import (
	"context"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Harrison-Blair/qmeter/internal/dash/layout"
	"github.com/Harrison-Blair/qmeter/internal/dash/theme"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
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

const (
	spinFrames   = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
	spinInterval = 80 * time.Millisecond

	// DefaultRefreshInterval keeps zero-value Options useful to direct callers.
	DefaultRefreshInterval = 60 * time.Second
)

// Options are everything the model needs from its caller.
type Options struct {
	Providers       []provider.Provider
	Theme           theme.Theme
	RefreshInterval time.Duration
	Now             func() time.Time
	Ctx             context.Context
	Renderer        *lipgloss.Renderer
}

// Model is the dashboard's Bubble Tea model.
type Model struct {
	providers         []provider.Provider
	theme             theme.Theme
	refreshInterval   time.Duration
	refreshGeneration uint64
	cancelRefresh     context.CancelFunc
	now               func() time.Time
	ctx               context.Context
	renderer          *lipgloss.Renderer

	width     int
	height    int
	res       usage.Result
	cache     cache
	haveRes   bool
	updatedAt time.Time
	loading   bool
	spin      int
}

type spinMsg struct{}

type resultMsg struct {
	res usage.Result
}

type refreshMsg struct {
	generation uint64
}

// New builds the model. It starts out loading: Init's fetch is already on
// its way by the time the first frame is drawn.
func New(o Options) Model {
	m := Model{
		providers:       o.Providers,
		theme:           o.Theme,
		refreshInterval: o.RefreshInterval,
		now:             o.Now,
		ctx:             o.Ctx,
		renderer:        o.Renderer,
		loading:         true,
	}
	if m.now == nil {
		m.now = time.Now
	}
	if m.theme == (theme.Theme{}) {
		m.theme = theme.Default()
	}
	if m.refreshInterval == 0 {
		m.refreshInterval = DefaultRefreshInterval
	}
	if m.ctx == nil {
		m.ctx = context.Background()
	}
	if m.renderer == nil {
		m.renderer = newDashboardRenderer(os.Stdout)
	}
	return m
}

// Init starts the first fetch.
func (m Model) Init() tea.Cmd {
	return m.startFetch()
}

// startFetch is the fetch together with the spinner that turns while it runs.
func (m Model) startFetch() tea.Cmd {
	return tea.Batch(m.fetch(), spinTick())
}

func spinTick() tea.Cmd {
	return tea.Tick(spinInterval, func(time.Time) tea.Msg { return spinMsg{} })
}

func (m Model) fetch() tea.Cmd {
	ctx, providers := m.ctx, m.providers
	return func() tea.Msg {
		return resultMsg{res: usage.Run(ctx, providers, "")}
	}
}

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
		return m, nil
	case resultMsg:
		m.res, m.haveRes, m.loading = msg.res, true, false
		m.updatedAt = m.now()
		m.cache = m.cache.update(msg.res, m.updatedAt)
		m.spin = 0
		return m, m.scheduleRefresh()
	case refreshMsg:
		if m.loading || msg.generation != m.refreshGeneration {
			return m, nil
		}
		m.cancelRefreshTimer()
		m.loading = true
		return m, m.startFetch()
	case spinMsg:
		if !m.loading {
			return m, nil
		}
		m.spin = (m.spin + 1) % len([]rune(spinFrames))
		return m, spinTick()
	case tea.KeyMsg:
		// Bubble Tea can group adjacent rune keys into one message.
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !msg.Alt && !msg.Paste {
			for _, key := range msg.Runes {
				if key == 'q' {
					m.cancelRefreshTimer()
					return m, tea.Quit
				}
			}
			for _, key := range msg.Runes {
				if key == 'r' {
					if m.loading {
						return m, nil
					}
					m.cancelRefreshTimer()
					m.refreshGeneration++
					m.loading = true
					return m, m.startFetch()
				}
			}
			return m, nil
		}
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.cancelRefreshTimer()
			return m, tea.Quit
		case "r":
			if m.loading {
				return m, nil
			}
			m.cancelRefreshTimer()
			m.refreshGeneration++
			m.loading = true
			return m, m.startFetch()
		}
	}
	return m, nil
}

// View composes the full dashboard at the current terminal size.
func (m Model) View() string {
	return strings.Join(layout.Screen(m.res, m.width, m.height, layout.ScreenOptions{
		Now:          m.now(),
		Theme:        m.theme,
		FetchedAt:    m.updatedAt,
		Loading:      m.loading,
		SpinnerFrame: m.spin,
		Renderer:     m.renderer,
		Cached:       m.cache,
	}), "\n")
}
