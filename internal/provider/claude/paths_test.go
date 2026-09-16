package claude

import (
	"path/filepath"
	"strings"
	"testing"
)

// The OS default credential path, one test per platform.
//
// credentialPathFor is the single seam that turns a platform into a default
// path, so every platform is asserted from whatever host the suite runs on.
// Both home variables are set to distinct temp directories: the Windows
// branch must resolve %USERPROFILE% and the Unix branches $HOME, and neither
// may fall through to the home the test runner happens to have — with HOME
// unset os.UserHomeDir fails outright, which would make the assertion about
// the environment instead of about the code.

// wantPath is the default path expected under the home directory home. The
// components below the home directory are the same on every platform — only
// the home directory itself differs — and filepath.Join spells them with the
// running host's separator, so each assertion is about those components
// rather than about the separator.
func wantPath(home string) string {
	return filepath.Join(home, ".claude", ".credentials.json")
}

// setHomes points both home-directory variables at temp directories of their
// own and returns them, so a path built from the wrong one is visible.
func setHomes(t *testing.T) (home, profile string) {
	t.Helper()
	home, profile = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", profile)
	return home, profile
}

func TestDefaultPath_Windows(t *testing.T) {
	home, profile := setHomes(t)

	got, err := credentialPathFor("windows")
	if err != nil {
		t.Fatalf("credentialPathFor(\"windows\") error = %v, want nil", err)
	}
	if want := wantPath(profile); got != want {
		t.Errorf("credentialPathFor(\"windows\") = %q, want %q", got, want)
	}
	if strings.HasPrefix(got, home) {
		t.Errorf("credentialPathFor(\"windows\") = %q, want it under %%USERPROFILE%% (%s), not $HOME", got, profile)
	}
}

func TestDefaultPath_MacOS(t *testing.T) {
	home, profile := setHomes(t)

	got, err := credentialPathFor("darwin")
	if err != nil {
		t.Fatalf("credentialPathFor(\"darwin\") error = %v, want nil", err)
	}
	if want := wantPath(home); got != want {
		t.Errorf("credentialPathFor(\"darwin\") = %q, want %q", got, want)
	}
	if strings.HasPrefix(got, profile) {
		t.Errorf("credentialPathFor(\"darwin\") = %q, want it under $HOME (%s), not %%USERPROFILE%%", got, home)
	}
}

func TestDefaultPath_Linux(t *testing.T) {
	home, profile := setHomes(t)

	got, err := credentialPathFor("linux")
	if err != nil {
		t.Fatalf("credentialPathFor(\"linux\") error = %v, want nil", err)
	}
	if want := wantPath(home); got != want {
		t.Errorf("credentialPathFor(\"linux\") = %q, want %q", got, want)
	}
	if strings.HasPrefix(got, profile) {
		t.Errorf("credentialPathFor(\"linux\") = %q, want it under $HOME (%s), not %%USERPROFILE%%", got, home)
	}
}
