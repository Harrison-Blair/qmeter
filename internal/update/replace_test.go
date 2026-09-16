package update

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeInstall writes a stand-in "current binary" into a fresh temp dir and
// returns its path.
func fakeInstall(t *testing.T, mode os.FileMode) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "qmeter")
	if err := os.WriteFile(p, []byte("old binary"), mode); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatalf("chmod fake binary: %v", err)
	}
	return p
}

func TestReplace_SwapsTheBinaryAndKeepsItsMode(t *testing.T) {
	p := fakeInstall(t, 0o755)

	if err := (replacer{GOOS: "linux", ExecPath: p}).replace([]byte("new binary")); err != nil {
		t.Fatalf("replace: %v", err)
	}

	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "new binary" {
		t.Fatalf("content = %q, want the new binary", got)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v, want 0755", fi.Mode().Perm())
	}
}

func TestReplace_PreservesAnUnusualMode(t *testing.T) {
	p := fakeInstall(t, 0o700)

	if err := (replacer{GOOS: "linux", ExecPath: p}).replace([]byte("new binary")); err != nil {
		t.Fatalf("replace: %v", err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Fatalf("mode = %v, want 0700", fi.Mode().Perm())
	}
}

func TestReplace_LeavesNoTempFileBehind(t *testing.T) {
	p := fakeInstall(t, 0o755)

	if err := (replacer{GOOS: "linux", ExecPath: p}).replace([]byte("new binary")); err != nil {
		t.Fatalf("replace: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(p))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "qmeter" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("install dir holds %v, want just qmeter", names)
	}
}

func TestReplace_WindowsMovesTheRunningExeAside(t *testing.T) {
	p := fakeInstall(t, 0o755)

	// GOOS is injected, so the Windows-only dance is exercised on Linux.
	if err := (replacer{GOOS: "windows", ExecPath: p}).replace([]byte("new binary")); err != nil {
		t.Fatalf("replace: %v", err)
	}

	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "new binary" {
		t.Fatalf("content = %q, want the new binary", got)
	}
	old, err := os.ReadFile(p + ".old")
	if err != nil {
		t.Fatalf("the running exe was not moved to .old: %v", err)
	}
	if string(old) != "old binary" {
		t.Fatalf(".old content = %q, want the old binary", old)
	}
}

func TestReplace_NonWindowsLeavesNoDotOld(t *testing.T) {
	p := fakeInstall(t, 0o755)

	if err := (replacer{GOOS: "linux", ExecPath: p}).replace([]byte("new binary")); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if _, err := os.Stat(p + ".old"); err == nil {
		t.Fatal("a POSIX rename must not leave a .old file")
	}
}

func TestReplace_UnwritableDirectoryNamesItAndSuggestsElevation(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write to a mode 0500 directory")
	}
	p := fakeInstall(t, 0o755)
	dir := filepath.Dir(p)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := (replacer{GOOS: "linux", ExecPath: p}).replace([]byte("new binary"))
	if err == nil {
		t.Fatal("writing into an unwritable directory must fail")
	}
	if !strings.Contains(err.Error(), dir) {
		t.Fatalf("err = %q, want it to name %q", err, dir)
	}
	if !strings.Contains(err.Error(), "sudo qmeter update") {
		t.Fatalf("err = %q, want it to suggest sudo qmeter update", err)
	}
}

func TestReplace_UnwritableDirectoryOnWindowsDoesNotSuggestSudo(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write to a mode 0500 directory")
	}
	p := fakeInstall(t, 0o755)
	dir := filepath.Dir(p)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := (replacer{GOOS: "windows", ExecPath: p}).replace([]byte("new binary"))
	if err == nil {
		t.Fatal("writing into an unwritable directory must fail")
	}
	if strings.Contains(err.Error(), "sudo") {
		t.Fatalf("err = %q, want no sudo hint on Windows", err)
	}
	if !strings.Contains(err.Error(), "Administrator") {
		t.Fatalf("err = %q, want it to suggest an elevated prompt", err)
	}
}

func TestCleanupOldPath_RemovesTheLeftoverExe(t *testing.T) {
	p := fakeInstall(t, 0o755)
	if err := os.WriteFile(p+".old", []byte("previous"), 0o755); err != nil {
		t.Fatalf("write .old: %v", err)
	}

	if err := cleanupOldPath(p); err != nil {
		t.Fatalf("cleanupOldPath: %v", err)
	}
	if _, err := os.Stat(p + ".old"); err == nil {
		t.Fatal(".old still exists")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("cleanup removed the live binary: %v", err)
	}
}

func TestCleanupOldPath_NoLeftoverIsNotAnError(t *testing.T) {
	p := fakeInstall(t, 0o755)
	if err := cleanupOldPath(p); err != nil {
		t.Fatalf("cleanupOldPath with nothing to clean: %v", err)
	}
}

func TestCleanupOld_NeverPanics(t *testing.T) {
	// The startup hook must be safe to call unconditionally.
	CleanupOld()
}

func TestResolveExecutable_FollowsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	p := fakeInstall(t, 0o755)
	link := filepath.Join(t.TempDir(), "qmeter-link")
	if err := os.Symlink(p, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	got, err := resolveExecutable(func() (string, error) { return link, nil })
	if err != nil {
		t.Fatalf("resolveExecutable: %v", err)
	}
	if got != p {
		t.Fatalf("resolveExecutable = %q, want the symlink target %q", got, p)
	}
}
