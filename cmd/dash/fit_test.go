package dash

import "testing"

func TestCmdFit(t *testing.T) {
	for _, tc := range []struct {
		args                  []string
		fit, vertical, banner bool
	}{{nil, false, false, true}, {[]string{"--fit"}, true, false, true}, {[]string{"--fit=false"}, false, false, true}, {[]string{"--fit", "--vertical", "--no-banner", "--filter", "claude"}, true, true, false}} {
		t.Run("flags", func(t *testing.T) {
			tr := newTestRoot(t, true)
			f := tr.cmd.Flags().Lookup("fit")
			if f == nil || f.DefValue != "false" || f.Shorthand != "" || tr.cmd.PersistentFlags().Lookup("fit") != nil {
				t.Fatal("fit must be a local bool default false without shorthand")
			}
			if err := tr.execute(t, tc.args...); err != nil {
				t.Fatal(err)
			}
			got := tr.runs[0]
			if got.Fit != tc.fit || got.Vertical != tc.vertical || got.Banner != tc.banner {
				t.Fatalf("options: %#v", got)
			}
			if tc.vertical && len(got.Providers) != 1 {
				t.Fatal("filter lost")
			}
		})
	}
}

func TestCmdFitIgnoredForNoninteractive(t *testing.T) {
	for _, json := range []bool{false, true} {
		var baseline string
		for _, fit := range []bool{false, true} {
			t.Run("output", func(t *testing.T) {
				tr := newTestRoot(t, json)
				args := []string{"--filter", "claude"}
				if json {
					args = append(args, "--json")
				}
				if fit {
					args = append(args, "--fit", "--vertical")
				}
				if err := tr.execute(t, args...); err != nil {
					t.Fatal(err)
				}
				if len(tr.runs) != 0 || tr.configLoads != 0 {
					t.Fatal("entered dashboard")
				}
				if fit && tr.out.String() != baseline {
					t.Fatal("fit changed noninteractive output")
				}
				baseline = tr.out.String()
			})
		}
	}
}
