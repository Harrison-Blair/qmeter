package dash

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	dconfig "github.com/Harrison-Blair/qmeter/internal/dash/config"
)

func TestThicknessFlagMetadata(t *testing.T) {
	tr := newTestRoot(t, true)
	f := tr.cmd.Flags().Lookup("thickness")
	if f == nil || f.DefValue != "3" || !strings.Contains(f.Usage, "1–9") {
		t.Fatalf("thickness flag = %+v, want default 3 and usage range 1–9", f)
	}
}

func TestThicknessWiring(t *testing.T) {
	for _, tc := range []struct {
		config int
		args   []string
		want   int
	}{{3, []string{"--thickness", "5"}, 5}, {9, nil, 9}, {9, []string{"--thickness", "5"}, 5}, {3, nil, 3}, {9, []string{"--thickness", "3"}, 3}} {
		t.Run(strings.Join(tc.args, " ")+string(rune('0'+tc.config)), func(t *testing.T) {
			tr := newTestRoot(t, true)
			loadConfig = func() (dconfig.Settings, error) { s := dconfig.Default(); s.MeterThickness = tc.config; return s, nil }
			if err := tr.execute(t, tc.args...); err != nil {
				t.Fatal(err)
			}
			if len(tr.runs) != 1 || tr.runs[0].MeterThickness != tc.want {
				t.Fatalf("runs %+v want thickness %d", tr.runs, tc.want)
			}
		})
	}
}
func TestThicknessInvalidBeforeFetch(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		for _, n := range []string{"0", "10", "-1"} {
			for _, json := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/json=%v/terminal=%v", n, json, terminal), func(t *testing.T) {
					tr := newTestRoot(t, terminal)
					args := []string{"--thickness", n}
					if json {
						args = append(args, "--json")
					}
					if err := tr.execute(t, args...); err == nil {
						t.Fatal("invalid thickness accepted")
					}
					if got := tr.errOut.String(); got != "thickness must be between 1 and 9\n" {
						t.Fatalf("stderr %q", got)
					}
					if len(tr.fetched()) != 0 || len(tr.runs) != 0 || tr.configLoads != 0 || tr.out.Len() != 0 {
						t.Fatal("work performed before validation")
					}
				})
			}
		}
	}
}
func TestThicknessPiped(t *testing.T) {
	plain := newTestRoot(t, false)
	if err := plain.execute(t); err != nil {
		t.Fatal(err)
	}
	want := plain.out.String()
	thick := newTestRoot(t, false)
	if err := thick.execute(t, "--thickness", "5"); err != nil {
		t.Fatal(err)
	}
	if thick.out.String() != want || len(thick.runs) != 0 || thick.configLoads != 0 {
		t.Fatal("piped thickness changed table")
	}
}
func TestThicknessConfigWarning(t *testing.T) {
	tr := newTestRoot(t, true)
	loadConfig = func() (dconfig.Settings, error) {
		s := dconfig.Default()
		s.MeterThickness = 9
		return s, errors.New("meter_thickness must be between 1 and 9")
	}
	if err := tr.execute(t); err != nil {
		t.Fatal(err)
	}
	if tr.runs[0].MeterThickness != 3 || tr.errOut.String() != "warning: meter_thickness must be between 1 and 9\n" {
		t.Fatal("config warning did not restore defaults")
	}
}
