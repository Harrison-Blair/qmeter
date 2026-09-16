package banner_test

import (
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/dash/banner"
	"github.com/mattn/go-runewidth"
)

// want is the approved banner, lifted verbatim from the design mockup
// (design-u21/80x24-banner.txt, lines 1-6): FIGlet "standard", lowercase
// "qmeter", kerned, with the font's blank leading column trimmed and every
// row right-trimmed.
var want = []string{
	"                         _",
	"  __ _  _ __ ___    ___ | |_  ___  _ __",
	" / _` || '_ ` _ \\  / _ \\| __|/ _ \\| '__|",
	"| (_| || | | | | ||  __/| |_|  __/| |",
	" \\__, ||_| |_| |_| \\___| \\__|\\___||_|",
	"    |_|",
}

func TestRowsMatchesTheApprovedBanner(t *testing.T) {
	got := banner.Rows()
	if len(got) != len(want) {
		t.Fatalf("Rows() returned %d rows, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRowsIsExactlyHeightRowsNoWiderThanWidth(t *testing.T) {
	got := banner.Rows()
	if len(got) != banner.Height {
		t.Fatalf("Rows() returned %d rows, want Height=%d", len(got), banner.Height)
	}
	if banner.Height != 6 {
		t.Errorf("Height = %d, want 6", banner.Height)
	}
	if banner.Width != 40 {
		t.Errorf("Width = %d, want 40", banner.Width)
	}
	widest := 0
	for i, row := range got {
		w := runewidth.StringWidth(row)
		if w > banner.Width {
			t.Errorf("row %d is %d cells, wider than Width=%d", i, w, banner.Width)
		}
		if w > widest {
			widest = w
		}
	}
	if widest != banner.Width {
		t.Errorf("widest row is %d cells, want Width=%d", widest, banner.Width)
	}
}

func TestRowsIsNotAliased(t *testing.T) {
	first := banner.Rows()
	first[0] = "tampered"
	if second := banner.Rows(); second[0] == "tampered" {
		t.Error("Rows() handed out its backing array: mutating the result changed the next call")
	}
}
