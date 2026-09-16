package update

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// defaultMode is the permission a replaced binary gets when the current one
// cannot be stat'ed.
const defaultMode fs.FileMode = 0o755

// replacer swaps the running executable for a freshly downloaded one.
//
// GOOS is a field rather than a direct read of runtime.GOOS so the
// Windows-only "move the running exe aside first" dance is unit-tested on
// every platform.
type replacer struct {
	GOOS     string
	ExecPath string
}

// replace writes data next to the current executable, gives it the current
// executable's mode and renames it over the top. The rename is atomic on
// POSIX; on Windows, where the running image cannot be replaced, the live
// exe is renamed to <exe>.old first and swept up by CleanupOld on a later
// run.
func (r replacer) replace(data []byte) error {
	dir := filepath.Dir(r.ExecPath)

	mode := defaultMode
	if fi, err := os.Stat(r.ExecPath); err == nil {
		mode = fi.Mode().Perm()
	}

	tmp, err := os.CreateTemp(dir, ".qmeter-update-*")
	if err != nil {
		return r.permissionError(dir, err)
	}
	tmpName := tmp.Name()
	// Any failure from here on must leave the install directory exactly as
	// it was found.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("update: write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("update: write %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("update: chmod %s: %w", tmpName, err)
	}

	if r.GOOS == "windows" {
		old := r.ExecPath + ".old"
		_ = os.Remove(old)
		if err := os.Rename(r.ExecPath, old); err != nil {
			return r.permissionError(dir, err)
		}
		if err := os.Rename(tmpName, r.ExecPath); err != nil {
			// Put the running exe back so the install is not left empty.
			_ = os.Rename(old, r.ExecPath)
			return r.permissionError(dir, err)
		}
		return nil
	}

	if err := os.Rename(tmpName, r.ExecPath); err != nil {
		return r.permissionError(dir, err)
	}
	return nil
}

// permissionError turns an EACCES into advice the user can act on and
// passes anything else through with the directory named.
func (r replacer) permissionError(dir string, err error) error {
	if !errors.Is(err, fs.ErrPermission) {
		return fmt.Errorf("update: replace %s: %w", r.ExecPath, err)
	}
	if r.GOOS == "windows" {
		return fmt.Errorf("update: %s is not writable; re-run `qmeter update` from a prompt "+
			"running as Administrator", dir)
	}
	return fmt.Errorf("update: %s is not writable; re-run with elevated permissions "+
		"(`sudo qmeter update`)", dir)
}

// resolveExecutable finds the real path of the running binary, following
// symlinks so a wrapper link in ~/.local/bin is not what gets replaced.
func resolveExecutable(executable func() (string, error)) (string, error) {
	p, err := executable()
	if err != nil {
		return "", fmt.Errorf("update: locate the running qmeter binary: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", fmt.Errorf("update: resolve %s: %w", p, err)
	}
	return resolved, nil
}

// CleanupOld deletes the <exe>.old file a previous Windows update left
// behind. It is best-effort by design: it is called on every run and must
// never fail a command, so every error is swallowed.
func CleanupOld() {
	p, err := resolveExecutable(os.Executable)
	if err != nil {
		return
	}
	_ = cleanupOldPath(p)
}

// cleanupOldPath removes execPath+".old" if it is there. A missing file is
// success, not an error.
func cleanupOldPath(execPath string) error {
	err := os.Remove(execPath + ".old")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("update: remove %s.old: %w", execPath, err)
	}
	return nil
}

// currentGOOS is runtime.GOOS, indirected so the default wiring has one
// place to read it from.
func currentGOOS() string { return runtime.GOOS }
