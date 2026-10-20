package layout_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 16, 14, 22, 7, 0, time.UTC)

func goldenLines(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading golden: %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}
