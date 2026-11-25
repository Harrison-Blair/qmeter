package layout

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/dash/theme"
	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/spend"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

var rendererFactory = display.Renderer

const (
	screenFloorWidth  = 80
	screenFloorHeight = 24
	screenGutter      = 2
	compactSpinner    = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
)

// ScreenOptions holds the state that the pinned header and key bar display.
type ScreenOptions struct {
	Now          time.Time
	Theme        theme.Theme
	FetchedAt    time.Time
	Loading      bool
	SpinnerFrame int
	Renderer     *lipgloss.Renderer
	// Cached holds the last good response of each provider, by provider ID.
	// A provider with an entry and a failure in the result shows the entry.
	Cached map[string]CachedProvider
}

// CachedProvider is the last good response the dashboard kept for one
// provider.
type CachedProvider struct {
	Windows   []provider.Window
	Balances  []provider.Balance
	FetchedAt time.Time
}

type screenCard struct {
	provider provInfo
	windows  []provider.Window
	balances []display.Balance
	messages []display.CardMessage
	cached   bool
}

type screenCardRow struct {
	cards  []screenCard
	widths []int
	height int
}

type screenPanel struct {
	title   string
	width   int
	content []string
}

// Screen measures and composes a complete dashboard frame. Every returned
// line has width terminal cells, and the frame has height lines.
func Screen(result usage.Result, width, height int, options ScreenOptions) []string {
	if height <= 0 || width < 0 {
		return nil
	}
	if width == 0 {
		return make([]string, height)
	}

	renderer := options.Renderer
	if renderer == nil {
		renderer = rendererFactory(os.Stdout)
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	providers := presentProviders(result)
	if width < screenFloorWidth || height < screenFloorHeight {
		needWidth, needHeight := compactNeed(result, options.Cached, now, renderer)
		return tooSmallFrame(width, height, needWidth, needHeight)
	}
	if options.FetchedAt.IsZero() {
		return centeredBodyFrame(width, height, headerLine(result, len(providers), width, options, renderer), "fetching…", keyLine(options, width, renderer))
	}
	if len(providers) == 0 {
		return centeredBodyFrame(width, height, headerLine(result, 0, width, options, renderer), "no providers detected", keyLine(options, width, renderer))
	}

	cards := screenCards(result, providers, options.Cached)
	canUseTwo := len(cards) > 1 && (width-screenGutter)/2 >= 30
	if canUseTwo {
		for _, tier := range []struct{ resets, balances bool }{{true, true}, {false, true}, {false, false}} {
			if frame, ok := renderMeasured(result, cards, width, height, 2, tier.resets, tier.balances, now, options, renderer); ok {
				return frame
			}
		}
	} else if width >= 30 {
		for _, tier := range []struct{ resets, balances bool }{{true, true}, {false, true}, {false, false}} {
			if frame, ok := renderMeasured(result, cards, width, height, 1, tier.resets, tier.balances, now, options, renderer); ok {
				return frame
			}
		}
	}

	needWidth, needHeight := compactNeed(result, options.Cached, now, renderer)
	if width >= needWidth && height >= needHeight {
		return compactFrame(result, width, height, now, options, renderer)
	}
	return tooSmallFrame(width, height, needWidth, needHeight)
}

func share(total, gaps, index int) int {
	if gaps == 0 {
		return 0
	}
	n := total / gaps
	if index < total%gaps {
		n++
	}
	return n
}

func screenCards(result usage.Result, providers []provInfo, cached map[string]CachedProvider) []screenCard {
	cards := make([]screenCard, 0, len(providers))
	for _, p := range providers {
		card := screenCard{provider: p}
		windows, balances := result.Windows, result.Balances
		entry, ok := cachedFor(result, cached, p.id)
		if ok {
			windows, balances, card.cached = entry.Windows, entry.Balances, true
		}
		for _, w := range windows {
			if w.Provider == p.id {
				card.windows = append(card.windows, w)
			}
		}
		if len(card.windows) == 0 {
			for _, b := range balances {
				if b.Provider == p.id {
					card.balances = append(card.balances, display.Balance{Name: b.Name, Amount: balanceAmount(b)})
				}
			}
		}
		for _, e := range result.Errors {
			if e.Provider == p.id {
				text := "error: " + e.Message
				if ok {
					text = cachedMessage(entry, e.Message)
				}
				card.messages = append(card.messages, display.CardMessage{Text: text, Error: true})
			}
		}
		for _, u := range result.Undetected {
			if u.Provider == p.id {
				card.messages = append(card.messages, display.CardMessage{Text: "not detected: " + u.Message})
			}
		}
		cards = append(cards, card)
	}
	return cards
}

func balanceAmount(balance provider.Balance) string {
	left, limit := spend.Amounts(balance)
	if limit != "-" {
		return left + " / " + limit
	}
	return left
}

func renderMeasured(result usage.Result, cards []screenCard, width, height, columns int, withResets, withBalances bool, now time.Time, options ScreenOptions, renderer *lipgloss.Renderer) ([]string, bool) {
	rows := measureCardRows(cards, width, columns)
	panels := measurePanels(result, width, withResets, withBalances, now, renderer, options.Theme)
	gridHeight := 0
	for _, row := range rows {
		gridHeight += row.height
	}
	panelHeight := 0
	for _, panel := range panels {
		panelHeight = max(panelHeight, len(panel.content)+3)
	}
	needed := 2 + gridHeight + panelHeight
	if needed > height {
		return nil, false
	}

	extra := height - needed
	lines := []string{headerLine(result, len(cards), width, options, renderer)}
	for i, row := range rows {
		cardHeight := row.height + share(extra, len(rows), i)
		cardLines := make([][]string, len(row.cards))
		for col, card := range row.cards {
			draw := display.ProviderCard
			if card.cached {
				draw = display.CachedProviderCard
			}
			cardLines[col], _ = draw(card.provider.id, providerPlan(card.windows), card.windows, card.balances, card.messages, row.widths[col], cardHeight, now, renderer, options.Theme)
		}
		for y := 0; y < cardHeight; y++ {
			line := ""
			for col := range cardLines {
				if col > 0 {
					line += strings.Repeat(" ", screenGutter)
				}
				line += cardLines[col][y]
			}
			lines = append(lines, line)
		}
	}
	if len(panels) > 0 {
		lines = append(lines, renderPanels(panels, panelHeight, renderer)...)
	}
	lines = append(lines, keyLine(options, width, renderer))
	return lines, true
}

func measureCardRows(cards []screenCard, width, columns int) []screenCardRow {
	rows := make([]screenCardRow, 0, (len(cards)+columns-1)/columns)
	leftWidth := width
	if columns == 2 {
		leftWidth = (width - screenGutter) / 2
	}
	for start := 0; start < len(cards); start += columns {
		end := min(start+columns, len(cards))
		row := screenCardRow{cards: cards[start:end]}
		if columns == 2 && len(row.cards) == 2 {
			row.widths = []int{leftWidth, width - screenGutter - leftWidth}
		} else {
			row.widths = []int{width}
		}
		for i, card := range row.cards {
			natural := display.CardHeight(row.widths[i], card.windows, card.balances, card.messages)
			row.height = max(row.height, natural)
		}
		rows = append(rows, row)
	}
	return rows
}

func measurePanels(result usage.Result, width int, withResets, withBalances bool, now time.Time, renderer *lipgloss.Renderer, th theme.Theme) []screenPanel {
	if withResets && len(result.Balances) > 0 && withBalances {
		leftWidth := (width - screenGutter) * 2 / 3
		rightWidth := width - screenGutter - leftWidth
		resetRows := display.ResetRows(result.Windows, now, leftWidth-4, renderer, th)
		balanceRows := balanceRows(result.Balances, rightWidth-4, renderer, th)
		return []screenPanel{{title: "resets", width: leftWidth, content: resetRows}, {title: "balances", width: rightWidth, content: balanceRows}}
	}
	if withResets {
		return []screenPanel{{title: "resets", width: width, content: display.ResetRows(result.Windows, now, width-4, renderer, th)}}
	}
	if withBalances && len(result.Balances) > 0 {
		return []screenPanel{{title: "balances", width: width, content: balanceRows(result.Balances, width-4, renderer, th)}}
	}
	return nil
}

func balanceRows(balances []provider.Balance, width int, renderer *lipgloss.Renderer, th theme.Theme) []string {
	rows := make([]string, 0, len(balances))
	for _, balance := range balances {
		parts := []screenPart{
			{text: balance.Provider, style: renderer.NewStyle().Foreground(th.Accent(balance.Provider)).Bold(true)},
			{text: " " + balance.Name + "  " + balanceAmount(balance), style: renderer.NewStyle()},
		}
		rows = append(rows, renderScreenParts(parts, width, renderer))
	}
	return rows
}

func renderPanels(panels []screenPanel, height int, renderer *lipgloss.Renderer) []string {
	if len(panels) == 1 {
		return renderPanel(panels[0], height, renderer)
	}
	left := renderPanel(panels[0], height, renderer)
	right := renderPanel(panels[1], height, renderer)
	out := make([]string, height)
	for i := range out {
		out[i] = left[i] + strings.Repeat(" ", screenGutter) + right[i]
	}
	return out
}

func renderPanel(panel screenPanel, height int, renderer *lipgloss.Renderer) []string {
	border := renderer.NewStyle().Foreground(display.Neutral).Faint(true)
	inside := panel.width - 4
	lines := make([]string, 0, height)
	lines = append(lines, border.Render("╭"+strings.Repeat("─", panel.width-2)+"╮"))
	title := renderer.NewStyle().Foreground(display.Neutral).Faint(true).Render(panel.title)
	lines = append(lines, border.Render("│")+" "+fitAnsi(title, inside)+" "+border.Render("│"))
	for _, content := range panel.content {
		lines = append(lines, border.Render("│")+" "+fitAnsi(content, inside)+" "+border.Render("│"))
	}
	for len(lines) < height-1 {
		lines = append(lines, border.Render("│")+strings.Repeat(" ", panel.width-2)+border.Render("│"))
	}
	lines = append(lines, border.Render("╰"+strings.Repeat("─", panel.width-2)+"╯"))
	return lines
}

func fitAnsi(line string, width int) string {
	line = ansi.Truncate(line, max(0, width), "")
	return line + strings.Repeat(" ", max(0, width-ansi.StringWidth(line)))
}

func compactNeed(result usage.Result, cached map[string]CachedProvider, now time.Time, renderer *lipgloss.Renderer) (int, int) {
	width := screenFloorWidth
	rows := 0
	for _, row := range compactWindows(result, cached) {
		prefix := row.window.Provider + " " + row.window.Name
		width = max(width, compactWindowNeed(row.window, prefix, now, renderer))
		rows++
	}
	for _, message := range result.Errors {
		width = max(width, runewidth.StringWidth(message.Provider+" "+compactErrorText(result, cached, message)))
		rows++
	}
	for _, message := range result.Undetected {
		width = max(width, runewidth.StringWidth(message.Provider+" not detected: "+message.Message))
		rows++
	}
	return width, max(screenFloorHeight, rows+2)
}

func compactWindowNeed(window provider.Window, prefix string, now time.Time, renderer *lipgloss.Renderer) int {
	nameWidth := runewidth.StringWidth(prefix)
	blankName := window
	blankName.Name = ""
	for width := nameWidth; ; width++ {
		line := display.CompactBar(blankName, now, width, nameWidth, false, renderer)
		if ansi.StringWidth(line) <= width {
			return width
		}
	}
}

type screenPart struct {
	text  string
	style lipgloss.Style
}

func compactFrame(result usage.Result, width, height int, now time.Time, options ScreenOptions, renderer *lipgloss.Renderer) []string {
	lines := make([]string, height)
	for i := range lines {
		lines[i] = strings.Repeat(" ", width)
	}
	lines[0] = headerLine(result, len(presentProviders(result)), width, options, renderer)
	body := compactRows(result, options.Cached, width, now, options.Theme, renderer)
	start := 1 + max(0, (height-2-len(body))/2)
	copy(lines[start:], body)
	lines[height-1] = keyLine(options, width, renderer)
	return lines
}

func compactRows(result usage.Result, cached map[string]CachedProvider, width int, now time.Time, th theme.Theme, renderer *lipgloss.Renderer) []string {
	windows := compactWindows(result, cached)
	nameWidth := 0
	for _, row := range windows {
		nameWidth = max(nameWidth, runewidth.StringWidth(row.window.Provider+" "+row.window.Name))
	}
	var rows []string
	for _, row := range windows {
		window := row.window
		parts := []screenPart{
			{text: window.Provider, style: renderer.NewStyle().Foreground(th.Accent(window.Provider)).Bold(true)},
			{text: " " + window.Name, style: renderer.NewStyle()},
		}
		name := renderScreenParts(parts, nameWidth, renderer)
		remaining := width - nameWidth
		blankName := window
		blankName.Name = ""
		suffix := display.CompactBar(blankName, now, remaining, 0, row.cached, renderer)
		rows = append(rows, name+suffix)
	}
	for _, message := range result.Errors {
		rows = append(rows, compactMessage(message.Provider, compactErrorText(result, cached, message), width, th, renderer, true))
	}
	for _, message := range result.Undetected {
		rows = append(rows, compactMessage(message.Provider, "not detected: "+message.Message, width, th, renderer, false))
	}
	return rows
}

func compactMessage(providerID, message string, width int, th theme.Theme, renderer *lipgloss.Renderer, isError bool) string {
	color := display.Neutral
	if isError {
		color = display.Error
	}
	parts := []screenPart{
		{text: providerID, style: renderer.NewStyle().Foreground(th.Accent(providerID)).Bold(true)},
		{text: " " + message, style: renderer.NewStyle().Foreground(color)},
	}
	return renderScreenParts(parts, width, renderer)
}

func headerLine(result usage.Result, providers, width int, options ScreenOptions, renderer *lipgloss.Renderer) string {
	// The right side, widest first: a narrow line drops the clock, then all.
	timeTexts := []string{"fetching…"}
	if !options.FetchedAt.IsZero() {
		fetched := "fetched " + options.FetchedAt.Local().Format("15:04:05")
		timeTexts = []string{fetched + " · now " + options.Now.Local().Format("15:04:05"), fetched}
	}
	summary := fmt.Sprintf("%d %s · %d %s · %d %s", providers, plural(providers, "provider"), len(result.Windows), plural(len(result.Windows), "window"), len(result.Balances), plural(len(result.Balances), "balance"))
	leftParts := []screenPart{
		{text: "qmeter", style: renderer.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)},
		{text: " " + summary, style: renderer.NewStyle().Foreground(display.Neutral).Faint(true)},
	}
	leftWidth := runewidth.StringWidth("qmeter " + summary)
	for _, timeText := range timeTexts {
		rightWidth := runewidth.StringWidth(timeText)
		if leftWidth+rightWidth+1 > width {
			continue
		}
		left := renderScreenParts(leftParts, leftWidth, renderer)
		right := renderScreenParts([]screenPart{{text: timeText, style: renderer.NewStyle().Foreground(display.Neutral).Faint(true)}}, rightWidth, renderer)
		return left + strings.Repeat(" ", width-leftWidth-rightWidth) + right
	}
	return renderScreenParts(leftParts, width, renderer)
}

func plural(n int, singular string) string {
	if n == 1 {
		return singular
	}
	return singular + "s"
}

func keyLine(options ScreenOptions, width int, renderer *lipgloss.Renderer) string {
	text := "r refresh · q quit"
	if options.Loading {
		frames := []rune(compactSpinner)
		frame := frames[options.SpinnerFrame%len(frames)]
		if options.SpinnerFrame < 0 {
			frame = frames[(len(frames)+options.SpinnerFrame%len(frames))%len(frames)]
		}
		text = string(frame) + " refreshing · q quit"
	}
	return renderScreenParts([]screenPart{{text: text, style: renderer.NewStyle().Foreground(display.Neutral).Faint(true)}}, width, renderer)
}

func centeredBodyFrame(width, height int, header, message, key string) []string {
	lines := make([]string, height)
	for i := range lines {
		lines[i] = strings.Repeat(" ", width)
	}
	lines[0], lines[height-1] = header, key
	bodyHeight := height - 2
	at := 1 + (bodyHeight-1)/2
	lines[at] = centeredLine(message, width)
	return lines
}

func tooSmallFrame(width, height, needWidth, needHeight int) []string {
	message := fmt.Sprintf("terminal too small: %dx%d, need %dx%d", width, height, needWidth, needHeight)
	lines := make([]string, height)
	for i := range lines {
		lines[i] = strings.Repeat(" ", width)
	}
	lines[height/2] = centeredLine(message, width)
	return lines
}

func centeredLine(text string, width int) string {
	text = runewidth.Truncate(text, width, "…")
	cells := runewidth.StringWidth(text)
	left := max(0, (width-cells)/2)
	return strings.Repeat(" ", left) + text + strings.Repeat(" ", max(0, width-left-cells))
}

func renderScreenParts(parts []screenPart, width int, renderer *lipgloss.Renderer) string {
	width = max(0, width)
	var out strings.Builder
	remaining := width
	used := 0
	for _, part := range parts {
		if remaining == 0 {
			break
		}
		text := part.text
		cells := runewidth.StringWidth(text)
		if cells > remaining {
			text = runewidth.Truncate(text, remaining, "…")
			cells = runewidth.StringWidth(text)
		}
		out.WriteString(part.style.Render(text))
		used += cells
		remaining -= cells
	}
	out.WriteString(strings.Repeat(" ", width-used))
	return out.String()
}
