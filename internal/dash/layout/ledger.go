package layout

import (
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/spend"
	"github.com/mattn/go-runewidth"
)

// ledgerRow shares the windows' centered block and reserves enough name
// space to keep balances distinguishable even when their amounts are long.
func ledgerRow(b provider.Balance, colw, meterWidth int) row {
	blockw := gaugeWidth(colw, meterWidth) + pctWidth + 1 + 1 + cdWidth
	left, limit := spend.Amounts(b)
	nameFloor := min(10, runewidth.StringWidth(b.Name))
	amounts := truncTail(left+" of "+limit, blockw-4-nameFloor)
	name := truncMid(b.Name, blockw-4-runewidth.StringWidth(amounts))
	return row{}.pad((colw-blockw)/2).
		put(plain, "  ").put(nameStyle, name).
		put(plain, "  ").put(dimStyle, amounts).pad(colw)
}
