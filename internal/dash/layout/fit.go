package layout

import (
	"time"

	"github.com/Harrison-Blair/qmeter/internal/dash/banner"
	"github.com/Harrison-Blair/qmeter/internal/usage"
)

func pageHeader(result usage.Result, width int, o Options) []row {
	rows := header(result, width, o.Banner, o.Theme)
	if o.Fit && o.Banner && width >= wideGutterMin {
		rows = rows[:banner.Height]
	}
	return rows
}

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

// fitSections measures compact grid rows before allocating any extra space.
// A paired row owns as many elastic slots as its more divided section.
func fitSections(result usage.Result, providers []provInfo, cols, colw, gutter int, now time.Time, target, capacity int) []row {
	type gridRow struct {
		sections     [][][]row
		height, gaps int
	}
	var grid []gridRow
	compact, slots := 0, 0
	for i := 0; i < len(providers); i += cols {
		g := gridRow{}
		for _, p := range providers[i:min(i+cols, len(providers))] {
			blocks := sectionBlocks(result, p, colw, now, target)
			height := 0
			for _, b := range blocks {
				height += len(b)
			}
			g.sections = append(g.sections, blocks)
			g.height = max(g.height, height)
			g.gaps = max(g.gaps, len(blocks)-1)
		}
		grid = append(grid, g)
		compact += g.height
		slots += g.gaps
	}
	slots += len(grid) - 1
	extra := max(0, capacity-compact)
	var out []row
	slot := 0
	if slots == 0 {
		out = append(out, make([]row, extra/2)...)
	}
	for i, g := range grid {
		if i > 0 {
			out = append(out, make([]row, share(extra, slots, slot))...)
			slot++
		}
		expansion := 0
		for j := 0; j < g.gaps; j++ {
			expansion += share(extra, slots, slot)
			slot++
		}
		lines := make([]row, g.height+expansion)
		for col, blocks := range g.sections {
			section := spreadBlocks(blocks, expansion)
			for y := range lines {
				if col > 0 {
					lines[y] = lines[y].pad(col * (colw + gutter))
				}
				if y < len(section) {
					lines[y] = lines[y].join(section[y])
				}
			}
		}
		out = append(out, lines...)
	}
	if slots == 0 {
		out = append(out, make([]row, extra-extra/2)...)
	}
	return out
}

// fitTimeline keeps the ruler/table heading attached to the first item.
func fitTimeline(lines []string, capacity int) []row {
	blocks := [][]row{{}}
	for i, line := range lines {
		r := row{text: line}
		if i < 2 {
			blocks[0] = append(blocks[0], r)
		} else {
			blocks = append(blocks, []row{r})
		}
	}
	extra := max(0, capacity-len(lines))
	if len(blocks) > 1 {
		return spreadBlocks(blocks, extra)
	}
	out := make([]row, extra/2)
	out = append(out, blocks[0]...)
	return append(out, make([]row, extra-extra/2)...)
}
