// Package resets orders usage windows by their next reset and renders the R1 list.
package resets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/display"
	"github.com/Harrison-Blair/qmeter/internal/provider"
	"github.com/Harrison-Blair/qmeter/internal/usage"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// Sort returns a stable copy, with unknown reset times last.
func Sort(windows []provider.Window) []provider.Window {
	return display.SortWindows(windows)
}

// RenderText uses the destination's terminal width, or 80 cells for pipes.
func RenderText(w io.Writer, result usage.Result, now time.Time) error {
	width := 80
	tty := false
	if f, ok := w.(interface{ Fd() uintptr }); ok && term.IsTerminal(int(f.Fd())) {
		tty = true
		if cols, _, err := term.GetSize(int(f.Fd())); err == nil && cols > 0 {
			width = cols
		}
	}
	return renderText(w, result, now, width, tty, display.Renderer(w))
}

func renderText(w io.Writer, result usage.Result, now time.Time, width int, tty bool, r *lipgloss.Renderer) error {
	if !tty {
		width = 80
	}
	if len(result.Windows) == 0 && len(result.Errors) == 0 && len(result.Undetected) == 0 {
		_, err := fmt.Fprintln(w, "no providers detected")
		return err
	}
	rows := display.ResetRows(result.Windows, now, width, r, display.Default())
	messages := usage.MessageRows(r, result)
	if len(messages) > 0 {
		var b strings.Builder
		_ = display.Table(&b, messages)
		rows = append(rows, strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")...)
	}
	_, err := fmt.Fprintln(w, strings.Join(rows, "\n"))
	return err
}

// RenderJSON extends usage's envelope without changing its window wire fields.
func RenderJSON(w io.Writer, result usage.Result, now time.Time) error {
	result.Windows = Sort(result.Windows)
	var b bytes.Buffer
	if err := usage.RenderJSON(&b, result); err != nil {
		return err
	}
	var envelope struct {
		Windows    []map[string]json.RawMessage `json:"windows"`
		Errors     json.RawMessage              `json:"errors"`
		Undetected json.RawMessage              `json:"undetected"`
	}
	if err := json.Unmarshal(b.Bytes(), &envelope); err != nil {
		return err
	}
	for i, win := range result.Windows {
		var seconds *int64
		if !win.ResetsAt.IsZero() {
			n := int64(win.ResetsAt.Sub(now) / time.Second)
			seconds = &n
		}
		raw, err := json.Marshal(seconds)
		if err != nil {
			return err
		}
		envelope.Windows[i]["resets_in_seconds"] = raw
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(envelope)
}
