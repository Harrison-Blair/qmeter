package config

import (
	"fmt"
	"strings"
	"testing"
)

func TestThicknessConfig(t *testing.T) {
	for _, n := range []int{3, 1, 9, 0, 10, -1} {
		text := fmt.Sprintf("meter_thickness = %d", n)
		if n == 3 {
			text = ""
		}
		got, err := parse([]byte(text))
		if n >= 1 && n <= 9 {
			if err != nil {
				t.Fatal(err)
			}
			if got.MeterThickness != n {
				t.Fatalf("got %d want %d", got.MeterThickness, n)
			}
		} else if err == nil || !strings.Contains(err.Error(), "meter_thickness") || !strings.Contains(err.Error(), "1 and 9") {
			t.Fatalf("%d: error %v", n, err)
		}
	}
}
