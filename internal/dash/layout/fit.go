package layout

import (
	"strings"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
)

// share assigns the remainder to the earliest gaps.
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

func spreadBlocks(blocks [][]row, extra int) []row {
	var out []row
	for i, block := range blocks {
		if i > 0 {
			out = append(out, make([]row, share(extra, len(blocks)-1, i-1))...)
		}
		out = append(out, block...)
	}
	return out
}

// fitSections keeps each provider's content packed and centres it in a card.
func fitSections(result usage.Result, providers []provInfo, width, cols, colw, gutter int, now time.Time, capacity int) []row {
	if colw < MinColumn+4 {
		sections := make([][]row, 0, len(providers))
		compact := 0
		for i := 0; i < len(providers); i += cols {
			lines := sectionRow(result, providers[i:min(i+cols, len(providers))], colw, gutter, now, colw-(pctWidth+1+1+cdWidth))
			sections = append(sections, lines)
			compact += len(lines)
		}
		extra := max(0, capacity-compact)
		var out []row
		for i, lines := range sections {
			out = append(out, make([]row, share(extra, len(sections)+1, i))...)
			out = append(out, lines...)
		}
		return append(out, make([]row, share(extra, len(sections)+1, len(sections)))...)
	}
	type cardRow struct {
		content       [][]row
		width, height int
	}
	var grid []cardRow
	compact := 0
	for i := 0; i < len(providers); i += cols {
		ps := providers[i:min(i+cols, len(providers))]
		g := cardRow{width: colw}
		if len(ps) == 1 {
			g.width = width
		}
		inner := g.width - 4
		for _, p := range ps {
			content := section(result, p, inner, now, inner-(pctWidth+1+1+cdWidth))[1:]
			g.content = append(g.content, content)
			g.height = max(g.height, len(content)+2)
		}
		grid = append(grid, g)
		compact += g.height
	}
	extra := max(0, capacity-compact)
	var out []row
	for i, g := range grid {
		height := g.height + share(extra, len(grid), i)
		lines := make([]row, height)
		for col, content := range g.content {
			p := providers[i*cols+col]
			rule := lipgloss.NewStyle().Foreground(p.color).Faint(true)
			top := row{}.put(rule, "╭").join(sectionHead(p, providerPlan(result, p.id), g.width-2)).put(rule, "╮")
			above := (height - 2 - len(content)) / 2
			for y := range lines {
				var line row
				switch y {
				case 0:
					line = top
				case height - 1:
					line = row{}.put(rule, "╰"+strings.Repeat("─", g.width-2)+"╯")
				default:
					line = row{}.put(rule, "│ ")
					at := y - 1 - above
					if at >= 0 && at < len(content) {
						line = line.join(content[at])
					}
					line = line.pad(g.width-2).put(rule, " │")
				}
				if col > 0 {
					lines[y] = lines[y].pad(col * (colw + gutter))
				}
				lines[y] = lines[y].join(line)
			}
		}
		out = append(out, lines...)
	}
	return out
}
