// Package banner holds the qmeter wordmark the dashboard prints at the top
// of the page: six rows of FIGlet "standard", lowercase "qmeter", kerned
// (figlet -k), with the font's one blank leading column trimmed.
//
// The rows are hard-coded rather than generated: the font would be a second
// asset to embed and to keep in step, and the art never changes. They are
// right-trimmed, so a row is at most Width cells and usually fewer; a caller
// that needs a rectangle pads them itself. Colour is the caller's business —
// these rows are plain text.
package banner

// Width is the widest row, in terminal cells; Height is the row count. The
// dashboard drops the banner for a summary line below Width.
const (
	Width  = 40
	Height = 6
)

// rows is the art itself. Every row is ASCII, so one rune is one cell.
var rows = [Height]string{
	"                         _",
	"  __ _  _ __ ___    ___ | |_  ___  _ __",
	" / _` || '_ ` _ \\  / _ \\| __|/ _ \\| '__|",
	"| (_| || | | | | ||  __/| |_|  __/| |",
	" \\__, ||_| |_| |_| \\___| \\__|\\___||_|",
	"    |_|",
}

// Rows returns the Height rows of the banner, top to bottom. Each call
// returns a fresh slice, so a caller may style or pad the result in place.
func Rows() []string {
	out := make([]string, Height)
	copy(out, rows[:])
	return out
}
