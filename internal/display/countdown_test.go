package display

import (
	"testing"
	"time"
)

func TestFormatCountdownUsesLargestTwoUnits(t *testing.T) {
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{6*24*time.Hour + 8*time.Hour + 59*time.Minute, "6d8h"},
		{4*time.Hour + 59*time.Minute, "4h59m"},
		{16*time.Minute + 59*time.Second, "16m"},
		{12 * 24 * time.Hour, "12d"},
		{59 * time.Second, "0m"},
		{-time.Minute, "0m"},
	} {
		if got := FormatCountdown(tc.d); got != tc.want {
			t.Errorf("FormatCountdown(%s) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
