package display

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestRendererHonorsDestinationAndNoColor(t *testing.T) {
	oldTerminal, oldProfile := isTerminal, colorProfile
	t.Cleanup(func() { isTerminal, colorProfile = oldTerminal, oldProfile })
	for _, tc := range []struct {
		name     string
		terminal bool
		noColor  string
		profile  termenv.Profile
		styled   bool
	}{
		{"terminal", true, "", termenv.ANSI256, true}, {"no color", true, "1", termenv.ANSI256, false},
		{"pipe", false, "", termenv.ANSI256, false}, {"dumb", true, "", termenv.Ascii, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", tc.noColor)
			t.Setenv("CLICOLOR_FORCE", "1")
			var out bytes.Buffer
			isTerminal = func(w io.Writer) bool {
				if w != &out {
					t.Error("wrong writer")
				}
				return tc.terminal
			}
			colorProfile = func(w io.Writer) termenv.Profile {
				if w != &out {
					t.Error("wrong writer")
				}
				return tc.profile
			}
			r := Renderer(&out)
			got := r.NewStyle().Bold(true).Foreground(lipgloss.Color("1")).Render("test")
			if strings.Contains(got, "\x1b") != tc.styled {
				t.Fatalf("got %q", got)
			}
		})
	}
}

func TestRealBufferAndPipeRemainPlain(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("FORCE_COLOR", "1")
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	for _, w := range []io.Writer{&bytes.Buffer{}, write} {
		if got := Renderer(w).NewStyle().Bold(true).Render("plain"); got != "plain" {
			t.Fatal(got)
		}
	}
}

func TestTableUsesVisibleWidthsAndTwoCellMessages(t *testing.T) {
	var out bytes.Buffer
	r := lipgloss.NewRenderer(&out)
	r.SetColorProfile(termenv.ANSI256)
	bold := r.NewStyle().Bold(true)
	rows := [][]Cell{{{Text: "PROVIDER"}, {Text: "WINDOW"}, {Text: "VALUE"}}, {{Text: "claude", Style: bold}, {Text: "日本", Style: bold}, {Text: "20%"}}, {{Text: "long-provider"}, {Text: "error: a long message", Style: bold}}}
	if err := Table(&out, rows); err != nil {
		t.Fatal(err)
	}
	stripped := strings.ReplaceAll(strings.ReplaceAll(out.String(), "\x1b[1m", ""), "\x1b[0m", "")
	want := "PROVIDER       WINDOW  VALUE\nclaude         日本    20%\nlong-provider  error: a long message\n"
	if stripped != want {
		t.Fatalf("got %q want %q", stripped, want)
	}
}
