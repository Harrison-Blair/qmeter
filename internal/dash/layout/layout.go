// Package layout composes the qmeter dashboard page: it turns one
// usage.Result into the lines of a page at a given terminal width.
//
// It is pure — no terminal, no I/O, no Bubble Tea. Render returns every
// line of the page, padded to exactly the requested width in terminal
// cells, and the caller decides how many of them fit on screen and where
// the page is scrolled to.
//
// The page is a header (the FIGlet banner, or a one-line summary) over a
// grid of provider sections: up to two columns when each meter can retain its
// responsive packed floor, one column below. Each section is a rule with the
// provider's name and plan, then four rows per usage window — the window's
// name, the gauge bezel, the gauge itself between the percentage and the
// countdown, and the scale:
//
//	─ ◆ claude ────────────────────── max ─
//	▸ 5h [on pace]
//	       ╭┬────┬─────┬───▼┬─────┬╮
//	 68.0% ┴▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▲▱▱▱▱▱▱▱┴ 3h38m
//	        0         50        100
//
// Column arithmetic is fixed at every width: percentage 6, a space, the
// gauge, a space, countdown 6. A gauge grows toward the configured preference
// and is centred with those fields as one block. Below a 36-cell terminal (a
// 20-cell track) the layout says so and draws nothing.
package layout

import (
	"fmt"
	"github.com/Harrison-Blair/qmeter/internal/display"
	ipace "github.com/Harrison-Blair/qmeter/internal/pace"
	"strings"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/dash/banner"
	"github.com/Harrison-Blair/qmeter/internal/dash/gauge"
	"github.com/Harrison-Blair/qmeter/internal/dash/theme"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Column geometry. These five numbers are the whole layout: everything else
// is derived from them.
const (
	pctWidth    = 6                                           // "100.0%"
	cdWidth     = 6                                           // "6d23h", "3h38m", "-"
	gaugeIndent = pctWidth + 1                                // where the gauge column starts
	badgeWidth  = 4                                           // "[RL]"
	MinColumn   = pctWidth + 1 + gauge.MinWidth + 1 + cdWidth // 36
	MinWidth    = MinColumn                                   // the narrowest page that can be drawn at all
	// DefaultMeterWidth is the preferred complete gauge width, caps included.
	DefaultMeterWidth = 50
)

// Width thresholds, in terminal cells.
const (
	sectionGapMin = 100 // a blank line between section rows from here up
	wideGutterMin = 120 // a 4-cell gutter, and a blank line under the banner, from here up
)

// Options are the page-level choices the caller makes.
type Options struct {
	// Banner draws the FIGlet wordmark instead of the one-line summary.
	// It is ignored below the banner's own width, where the summary line
	// is drawn regardless.
	Banner bool

	// Now is the instant countdowns are measured from. The zero value
	// means time.Now(); tests pass a fixed instant.
	Now time.Time

	// Theme is the provider identity palette. Its zero value uses the
	// built-in adaptive light/dark colours.
	Theme theme.Theme

	// MeterWidth is the preferred complete gauge width, caps included. Zero
	// uses DefaultMeterWidth. It may shrink to gauge.MinWidth when the
	// terminal cannot fit the preference.
	MeterWidth int
}

// provInfo is a provider's presentation: the marker drawn before its name
// and the colour that marks everything belonging to it.
type provInfo struct {
	id    string
	icon  string
	color lipgloss.TerminalColor
}

// order is the drawing order of the sections, and must stay in step with
// usage.Registry(): claude, codex, opencode-go, cursor. Sections are laid
// out row-major, so at two columns the second row is opencode-go on the
// left and cursor on the right.
var order = []provInfo{
	{id: "claude", icon: "◆"},
	{id: "codex", icon: "●"},
	{id: "opencode-go", icon: "○"},
	{id: "cursor", icon: "▲"},
}

// The palette outside the gauge. Provider-coloured styles are built per
// section, since the colour is the provider's.
var (
	plain     = lipgloss.NewStyle()
	dimStyle  = lipgloss.NewStyle().Foreground(display.Neutral)
	nameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	cdStyle   = lipgloss.NewStyle().Foreground(display.Countdown)
	markStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
	errStyle  = lipgloss.NewStyle().Foreground(display.Error).Bold(true)
	warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	rlStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(display.RateLimited).Bold(true)
	rlCdStyle = lipgloss.NewStyle().Foreground(display.RateLimited).Bold(true)
)

// Render draws the whole page for r at width cells and returns its lines,
// each padded to exactly width cells. It never trims the page to a height:
// the caller scrolls.
//
// Below MinWidth cells there is no page to draw — the gauge is the last
// thing the layout gives up — and Render returns the single line
// "terminal too narrow".
func Render(r usage.Result, width int, o Options) []string {
	if width < 1 {
		return nil
	}
	if width < MinWidth {
		return []string{fitPlain("terminal too narrow", width)}
	}

	now := o.Now
	if now.IsZero() {
		now = time.Now()
	}

	target := meterTarget(o.MeterWidth)
	rows := header(r, width, o.Banner, o.Theme)

	present := presentProviders(r, o.Theme)
	if len(present) == 0 {
		rows = append(rows, row{}.put(dimStyle, "no providers detected"))
		return finish(rows, width)
	}

	cols, colw, gutter := columns(width, len(present), target)
	for i := 0; i < len(present); i += cols {
		if i > 0 && width >= sectionGapMin {
			rows = append(rows, row{})
		}
		end := i + cols
		if end > len(present) {
			end = len(present)
		}
		rows = append(rows, sectionRow(r, present[i:end], colw, gutter, now, target)...)
	}
	return finish(rows, width)
}

// columns is the page's column arithmetic: two columns only when there are at
// least two providers and both gauges retain their responsive packed floor.
// They split evenly around a gutter that widens to four cells on a wide
// terminal. Any odd cell is left at the right edge.
func columns(width, providers, target int) (cols, colw, gutter int) {
	if providers < 2 {
		return 1, width, 0
	}
	gutter = 2
	if width >= wideGutterMin {
		gutter = 4
	}
	packed := max(gauge.MinWidth, (target*4+4)/5) // ceil(target * 0.8)
	if width < 2*(packed+pctWidth+1+1+cdWidth)+gutter {
		return 1, width, 0
	}
	return 2, (width - gutter) / 2, gutter
}

// header is the banner, or the summary line that replaces it.
func header(r usage.Result, width int, want bool, th theme.Theme) []row {
	if !want || width < banner.Width {
		return []row{summary(r, width)}
	}
	rows := make([]row, 0, banner.Height+1)
	for _, art := range banner.Rows() {
		rows = append(rows, bannerRow(art, th))
	}
	if width >= wideGutterMin {
		rows = append(rows, row{})
	}
	return rows
}

// bannerRow colours one row of the wordmark in four fixed provider bands.
// Runs of one colour are styled together, and blanks are left unstyled.
func bannerRow(art string, th theme.Theme) row {
	out := row{}
	runes := []rune(art)
	for i := 0; i < len(runes); {
		j := i
		for j < len(runes) && bannerBand(j) == bannerBand(i) && (runes[j] == ' ') == (runes[i] == ' ') {
			j++
		}
		text := string(runes[i:j])
		if runes[i] == ' ' {
			out = out.put(plain, text)
		} else {
			out = out.put(lipgloss.NewStyle().Foreground(th.Accent(order[bannerBand(i)].id)), text)
		}
		i = j
	}
	return out
}

func bannerBand(col int) int {
	i := col * len(order) / banner.Width
	if i >= len(order) {
		i = len(order) - 1
	}
	return i
}

// summary is the one-line header: the wordmark and what the run found,
// with every zero count left out.
func summary(r usage.Result, width int) row {
	rateLimited := 0
	for _, w := range r.Windows {
		if w.RateLimited {
			rateLimited++
		}
	}
	parts := []string{}
	if n := len(r.Windows); n > 0 {
		parts = append(parts, count(n, "window", "windows"))
	}
	if rateLimited > 0 {
		parts = append(parts, fmt.Sprintf("%d rate limited", rateLimited))
	}
	if n := len(r.Errors); n > 0 {
		parts = append(parts, count(n, "error", "errors"))
	}
	if n := len(r.Undetected); n > 0 {
		parts = append(parts, fmt.Sprintf("%d not detected", n))
	}

	tail := ""
	if len(parts) > 0 {
		tail = " · " + strings.Join(parts, " · ")
	}
	// Truncate as plain text, then style: the wordmark is the first six
	// cells of whatever survives.
	text := truncTail("qmeter"+tail, width)
	if runewidth.StringWidth(text) <= len("qmeter") {
		return row{}.put(markStyle, text)
	}
	return row{}.put(markStyle, "qmeter").put(dimStyle, text[len("qmeter"):])
}

func count(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// presentProviders is the sections to draw, in registry order: a provider
// with no windows, no error and no not-detected reason is not drawn at
// all. A provider the registry has never heard of is drawn after the ones
// it has, so a new provider shows up even before it is listed here.
func presentProviders(r usage.Result, th theme.Theme) []provInfo {
	seen := map[string]bool{}
	note := func(id string) {
		if id != "" {
			seen[id] = true
		}
	}
	for _, w := range r.Windows {
		note(w.Provider)
	}
	for _, e := range r.Errors {
		note(e.Provider)
	}
	for _, u := range r.Undetected {
		note(u.Provider)
	}

	out := make([]provInfo, 0, len(seen))
	known := map[string]bool{}
	for _, p := range order {
		known[p.id] = true
		if seen[p.id] {
			p.color = th.Accent(p.id)
			out = append(out, p)
		}
	}
	for _, id := range firstSeenOrder(r) {
		if !known[id] {
			out = append(out, provInfo{id: id, icon: "•", color: th.Accent(id)})
		}
	}
	return out
}

// firstSeenOrder lists the provider IDs in r in the order they appear.
func firstSeenOrder(r usage.Result) []string {
	var ids []string
	seen := map[string]bool{}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, w := range r.Windows {
		add(w.Provider)
	}
	for _, e := range r.Errors {
		add(e.Provider)
	}
	for _, u := range r.Undetected {
		add(u.Provider)
	}
	return ids
}

// sectionRow lays one row of sections side by side, padding the shorter
// column so the next row starts on a clean line.
func sectionRow(r usage.Result, ps []provInfo, colw, gutter int, now time.Time, meterWidth int) []row {
	cols := make([][]row, len(ps))
	height := 0
	for i, p := range ps {
		cols[i] = section(r, p, colw, now, meterWidth)
		if len(cols[i]) > height {
			height = len(cols[i])
		}
	}
	out := make([]row, height)
	for i, lines := range cols {
		for y := 0; y < height; y++ {
			if i > 0 {
				out[y] = out[y].pad(i * (colw + gutter))
			}
			if y < len(lines) {
				out[y] = out[y].join(lines[y])
			}
		}
	}
	return out
}

// section is one provider's block: its rule, its windows, and whatever the
// run has to say about it.
func section(r usage.Result, p provInfo, colw int, now time.Time, meterWidth int) []row {
	var windows []provider.Window
	for _, w := range r.Windows {
		if w.Provider == p.id {
			windows = append(windows, w)
		}
	}
	plan := "-"
	if len(windows) > 0 && windows[0].Plan != "" {
		plan = windows[0].Plan
	}

	out := []row{sectionHead(p, plan, colw)}
	for _, w := range windows {
		out = append(out, windowBlock(w, p, colw, now, meterWidth)...)
	}
	for _, e := range r.Errors {
		if e.Provider == p.id {
			out = append(out, statusRow(errStyle, "!", "error: ", e.Message, colw))
		}
	}
	for _, u := range r.Undetected {
		if u.Provider == p.id {
			out = append(out, statusRow(warnStyle, "?", "not detected: ", u.Message, colw))
		}
	}
	return out
}

// sectionHead is the provider's rule: ─ ◆ claude ──…── max ─
func sectionHead(p provInfo, plan string, colw int) row {
	rule := lipgloss.NewStyle().Foreground(p.color).Faint(true)
	bold := lipgloss.NewStyle().Foreground(p.color).Bold(true)
	planStyle := lipgloss.NewStyle().Foreground(p.color)

	name := truncTail(p.id, max(1, colw-10))
	head := row{}.put(rule, "─ ").put(bold, p.icon+" ").put(bold, name).put(plain, " ")
	plan = truncTail(plan, max(1, colw-head.cells-3))
	tailW := 1 + runewidth.StringWidth(plan) + 2

	out := head
	if fill := colw - head.cells - tailW; fill > 0 {
		out = out.put(rule, strings.Repeat("─", fill))
	}
	return out.put(plain, " ").put(planStyle, plan).put(rule, " ─").pad(colw)
}

// windowBlock is one usage window: four rows, each exactly colw cells.
//
//	▸ 5h [on pace]
//	       ╭┬────┬─────┬───▼┬─────┬╮   [RL]
//	 68.0% ┴▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▲▱▱▱▱▱▱▱┴ 3h38m
//	        0         50        100
func windowBlock(w provider.Window, p provInfo, colw int, now time.Time, meterWidth int) []row {
	gw := gaugeWidth(colw, meterWidth)
	blockw := gw + pctWidth + 1 + 1 + cdWidth
	left := (colw - blockw) / 2
	g, err := gauge.Render(w.RemainingPercent, gw, w.RateLimited, pace(w, now))
	if err != nil {
		// Unreachable: Render refuses a page too narrow for a 36-cell
		// column, which is exactly a 22-cell gauge. Rather than panic on
		// a future miscalculation, leave the gauge blank and keep the
		// page's geometry intact.
		g = gauge.Block{Bezel: blanks(gw), Track: blanks(gw), Scale: blanks(gw)}
	}

	arrow := lipgloss.NewStyle().Foreground(p.color).Bold(true)
	pct := lipgloss.NewStyle().Foreground(gauge.Band(w.RemainingPercent, w.RateLimited)).Bold(true)

	status := ipace.Calculate(w, now).Pace
	badge := "[" + status + "]"
	badgeStyle := lipgloss.NewStyle().Foreground(display.PaceColor(status)).Bold(true)
	nameWidth := blockw - 2 - 1 - runewidth.StringWidth(badge)
	name := row{}.put(arrow, "▸ ").put(nameStyle, truncMid(w.Name, nameWidth)).
		put(plain, " ").put(badgeStyle, badge).pad(blockw)

	bezel := row{}.pad(gaugeIndent).raw(g.Bezel, gw).pad(blockw - badgeWidth)
	if w.RateLimited {
		bezel = bezel.put(rlStyle, "[RL]")
	} else {
		bezel = bezel.pad(blockw)
	}

	pctText := fmt.Sprintf("%.1f%%", w.RemainingPercent)
	track := rightAlign(row{}, pct, pctText, pctWidth).
		pad(pctWidth+1).raw(g.Track, gw).
		pad(pctWidth+1+gw+1).
		put(countdownStyle(w), countdown(w, now)).pad(blockw)

	scale := row{}.pad(gaugeIndent).raw(g.Scale, gw).pad(blockw)

	center := func(r row) row { return row{}.pad(left).join(r).pad(colw) }
	return []row{center(name), center(bezel), center(track), center(scale)}
}

// pace is the fraction of w's period still ahead of now, clamped to [0, 1],
// or gauge.NoPace when the provider gave no period or no reset time: there
// is then no even pace to draw.
func pace(w provider.Window, now time.Time) float64 {
	if w.Period <= 0 || w.ResetsAt.IsZero() {
		return gauge.NoPace
	}
	f := float64(w.ResetsAt.Sub(now)) / float64(w.Period)
	return max(0, min(1, f))
}

// statusRow is an error or not-detected line inside a section: the glyph
// carries the colour, the message is quoted from the run verbatim and
// truncated to the column.
func statusRow(glyph lipgloss.Style, mark, label, message string, colw int) row {
	head := mark + " " + label
	return row{}.put(glyph, head).
		put(dimStyle, truncTail(message, max(1, colw-runewidth.StringWidth(head)))).
		pad(colw)
}

// countdown is the window's reset time as the largest two units, the same
// rule the text renderer prints ("3h38m", "4d17h", "0m" when the window is
// already due), or "-" when the provider gave no reset time at all.
func countdown(w provider.Window, now time.Time) string {
	if w.ResetsAt.IsZero() {
		return "-"
	}
	return truncTail(formatResets(w.ResetsAt.Sub(now)), cdWidth)
}

// countdownStyle is cyan, the time colour, except for a rate-limited
// window: there the countdown is the only number that matters, so it takes
// the health colour along with the gauge's frame.
func countdownStyle(w provider.Window) lipgloss.Style {
	switch {
	case w.ResetsAt.IsZero():
		return dimStyle
	case w.RateLimited:
		return rlCdStyle
	}
	return cdStyle
}

// formatResets renders d as its largest two non-zero units: "<d>d<h>h",
// "<h>h<m>m" or "<m>m", dropping a trailing zero unit ("12d", not "12d0h").
// A duration that has already elapsed is "0m" — the window is due, which is
// not the same as having no reset time at all.
//
// This is deliberately a copy of the same rule in internal/usage: that one
// belongs to the text renderer's layout contract and is unexported, and the
// dashboard is not allowed to change it out from under it.
func formatResets(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	days := int64(d / (24 * time.Hour))
	hours := int64(d/time.Hour) % 24
	minutes := int64(d/time.Minute) % 60

	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("%dd%dh", days, hours)
	case days > 0:
		return fmt.Sprintf("%dd", days)
	case hours > 0 && minutes > 0:
		return fmt.Sprintf("%dh%dm", hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

// gaugeWidth is what a column leaves the gauge once the percentage, the
// countdown and their two spaces are paid for.
func gaugeWidth(colw, target int) int {
	return min(target, colw-(pctWidth+1+1+cdWidth))
}

func meterTarget(configured int) int {
	if configured == 0 {
		return DefaultMeterWidth
	}
	return max(configured, gauge.MinWidth)
}

// finish pads every row to the page width and hands back plain strings.
func finish(rows []row, width int) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.pad(width).text
	}
	return out
}

// --- rows ------------------------------------------------------------------

// row is a line under construction: styled text plus the number of terminal
// cells it occupies, which the escape sequences in the text would otherwise
// hide. Every method returns a new row, so a row can be built up in an
// expression.
type row struct {
	text  string
	cells int
}

// put appends text in a style.
func (r row) put(s lipgloss.Style, text string) row {
	if text == "" {
		return r
	}
	return row{r.text + s.Render(text), r.cells + runewidth.StringWidth(text)}
}

// raw appends text that is already styled and whose cell width is known.
func (r row) raw(text string, cells int) row {
	return row{r.text + text, r.cells + cells}
}

// join appends another row.
func (r row) join(o row) row {
	return row{r.text + o.text, r.cells + o.cells}
}

// pad extends the row to to cells with blanks. It never truncates: every
// caller builds its content to fit first.
func (r row) pad(to int) row {
	if r.cells >= to {
		return r
	}
	return row{r.text + strings.Repeat(" ", to-r.cells), to}
}

// rightAlign appends text right-aligned in a w-cell field, with the padding
// left unstyled.
func rightAlign(r row, s lipgloss.Style, text string, w int) row {
	text = truncTail(text, w)
	return r.pad(r.cells+w-runewidth.StringWidth(text)).put(s, text)
}

// --- text ------------------------------------------------------------------

func blanks(n int) string {
	if n < 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

// fitPlain pads or truncates unstyled text to exactly w cells.
func fitPlain(s string, w int) string {
	if runewidth.StringWidth(s) > w {
		return runewidth.Truncate(s, w, "")
	}
	return s + blanks(w-runewidth.StringWidth(s))
}

// truncTail shortens s to w cells, marking the cut with an ellipsis.
func truncTail(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= w {
		return s
	}
	return runewidth.Truncate(s, w, "…")
}

// truncMid shortens s to w cells by taking the ellipsis out of the middle,
// which keeps both ends of a name like "GPT-5.3-Codex-Spark secondary"
// readable — the ends are what tell two windows apart.
func truncMid(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	keep := w - 1
	tail := keep / 2
	head := keep - tail
	return runewidth.Truncate(s, head, "") + "…" + lastCells(s, tail)
}

// lastCells returns the trailing runes of s that fit in w cells.
func lastCells(s string, w int) string {
	if w <= 0 {
		return ""
	}
	runes := []rune(s)
	used, i := 0, len(runes)
	for i > 0 {
		rw := runewidth.RuneWidth(runes[i-1])
		if used+rw > w {
			break
		}
		used += rw
		i--
	}
	return string(runes[i:])
}
