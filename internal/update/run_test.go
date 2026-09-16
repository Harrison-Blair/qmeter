package update

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fullServer serves a whole release: the /releases/latest redirect, the
// checksums file and one linux/amd64 asset whose qmeter entry is the given
// payload. downloads counts asset fetches so a test can prove nothing was
// downloaded.
func fullServer(t *testing.T, tag, payload string, downloads *int) *httptest.Server {
	t.Helper()
	asset := tarGz(t, map[string]string{"qmeter": payload, "LICENSE": "MIT"})
	name := AssetName(tag, "linux", "amd64")
	base := repoPath + "/releases/download/" + tag + "/"

	mux := http.NewServeMux()
	mux.HandleFunc(repoPath+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", repoPath+"/releases/tag/"+tag)
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc(base+"checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sum(asset) + "  " + name + "\n"))
	})
	mux.HandleFunc(base+name, func(w http.ResponseWriter, r *http.Request) {
		if downloads != nil {
			*downloads++
		}
		_, _ = w.Write(asset)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// testOptions is a fully injected Options: a fake install dir, a fake
// release server, no terminal and no stdin unless the test says otherwise.
func testOptions(t *testing.T, srv *httptest.Server, current string) (*Options, *bytes.Buffer, *bytes.Buffer, string) {
	t.Helper()
	execPath := fakeInstall(t, 0o755)
	var out, errOut bytes.Buffer
	opts := &Options{
		Current:    current,
		BaseURL:    srv.URL,
		GOOS:       "linux",
		GOARCH:     "amd64",
		ExecPath:   execPath,
		Stdin:      strings.NewReader(""),
		Out:        &out,
		Err:        &errOut,
		IsTerminal: func() bool { return false },
	}
	return opts, &out, &errOut, execPath
}

func TestRun_CheckReportsAnAvailableUpdate(t *testing.T) {
	var downloads int
	srv := fullServer(t, "v0.2.0", "new binary", &downloads)
	opts, out, _, execPath := testOptions(t, srv, "v0.1.0")
	opts.Check = true

	if err := Run(context.Background(), *opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "v0.2.0") || !strings.Contains(got, "v0.1.0") {
		t.Fatalf("output = %q, want both versions", got)
	}
	if !strings.Contains(got, "qmeter update") {
		t.Fatalf("output = %q, want it to point at qmeter update", got)
	}
	if downloads != 0 {
		t.Fatalf("--check downloaded the asset %d times", downloads)
	}
	if b, _ := os.ReadFile(execPath); string(b) != "old binary" {
		t.Fatalf("--check replaced the binary: %q", b)
	}
}

func TestRun_CheckReportsUpToDate(t *testing.T) {
	srv := fullServer(t, "v0.2.0", "new binary", nil)
	opts, out, _, _ := testOptions(t, srv, "v0.2.0")
	opts.Check = true

	if err := Run(context.Background(), *opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "up to date") {
		t.Fatalf("output = %q, want an up-to-date line", out.String())
	}
}

func TestRun_CheckIgnoresANewerLocalBuild(t *testing.T) {
	srv := fullServer(t, "v0.2.0", "new binary", nil)
	opts, out, _, _ := testOptions(t, srv, "v0.9.0")
	opts.Check = true

	if err := Run(context.Background(), *opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "up to date") {
		t.Fatalf("output = %q, want a version ahead of the release to read as up to date", out.String())
	}
}

func decodeResult(t *testing.T, s string) Result {
	t.Helper()
	var r Result
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("output is not JSON (%v): %q", err, s)
	}
	return r
}

func TestRun_CheckJSON(t *testing.T) {
	srv := fullServer(t, "v0.2.0", "new binary", nil)
	opts, out, _, _ := testOptions(t, srv, "v0.1.0")
	opts.Check, opts.JSON = true, true

	if err := Run(context.Background(), *opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := decodeResult(t, out.String())
	want := Result{Current: "v0.1.0", Latest: "v0.2.0", UpdateAvailable: true, Updated: false}
	if got != want {
		t.Fatalf("result = %+v, want %+v", got, want)
	}
	for _, key := range []string{`"current"`, `"latest"`, `"update_available"`, `"updated"`} {
		if !strings.Contains(out.String(), key) {
			t.Fatalf("JSON %q is missing %s", out.String(), key)
		}
	}
}

func TestRun_DevVersionWarnsThenOffers(t *testing.T) {
	srv := fullServer(t, "v0.2.0", "new binary", nil)
	opts, _, errOut, execPath := testOptions(t, srv, "dev")
	opts.Yes = true

	if err := Run(context.Background(), *opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(errOut.String(), "cannot") {
		t.Fatalf("stderr = %q, want a warning that the version cannot be compared", errOut.String())
	}
	if b, _ := os.ReadFile(execPath); string(b) != "new binary" {
		t.Fatalf("binary = %q, want the dev build replaced after --yes", b)
	}
}

func TestRun_DevVersionCheckDoesNotInstall(t *testing.T) {
	var downloads int
	srv := fullServer(t, "v0.2.0", "new binary", &downloads)
	opts, out, errOut, execPath := testOptions(t, srv, "dev")
	opts.Check = true

	if err := Run(context.Background(), *opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(errOut.String(), "cannot") {
		t.Fatalf("stderr = %q, want the warning", errOut.String())
	}
	if !strings.Contains(out.String(), "v0.2.0") {
		t.Fatalf("stdout = %q, want the latest release named", out.String())
	}
	if downloads != 0 {
		t.Fatal("--check downloaded something")
	}
	if b, _ := os.ReadFile(execPath); string(b) != "old binary" {
		t.Fatalf("--check replaced the binary: %q", b)
	}
}

func TestRun_NoReleaseIsAClearError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(repoPath+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", repoPath+"/releases")
		w.WriteHeader(http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	opts, _, _, _ := testOptions(t, srv, "v0.1.0")
	opts.Check = true

	err := Run(context.Background(), *opts)
	if err == nil {
		t.Fatal("a repo with no releases must be reported as an error")
	}
	if !strings.Contains(err.Error(), "no releases published yet") {
		t.Fatalf("err = %q, want the no-releases message", err)
	}
}

func TestRun_PromptAnswers(t *testing.T) {
	tests := []struct {
		name    string
		stdin   string
		want    string
		updated bool
	}{
		{name: "y", stdin: "y\n", want: "new binary", updated: true},
		{name: "Y", stdin: "Y\n", want: "new binary", updated: true},
		{name: "y with spaces", stdin: "  y  \n", want: "new binary", updated: true},
		{name: "yes is not y", stdin: "yes\n", want: "old binary"},
		{name: "n", stdin: "n\n", want: "old binary"},
		{name: "empty line", stdin: "\n", want: "old binary"},
		{name: "EOF", stdin: "", want: "old binary"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var downloads int
			srv := fullServer(t, "v0.2.0", "new binary", &downloads)
			opts, _, errOut, execPath := testOptions(t, srv, "v0.1.0")
			opts.Stdin = strings.NewReader(tc.stdin)
			opts.IsTerminal = func() bool { return true }

			if err := Run(context.Background(), *opts); err != nil {
				t.Fatalf("Run: %v", err)
			}
			b, _ := os.ReadFile(execPath)
			if string(b) != tc.want {
				t.Fatalf("binary = %q, want %q", b, tc.want)
			}
			if !strings.Contains(errOut.String(), "[y/N]") {
				t.Fatalf("stderr = %q, want the prompt", errOut.String())
			}
			if tc.updated {
				if downloads != 1 {
					t.Fatalf("downloads = %d, want 1", downloads)
				}
				return
			}
			if downloads != 0 {
				t.Fatalf("a declined update downloaded %d assets", downloads)
			}
			if !strings.Contains(errOut.String(), "update canceled") {
				t.Fatalf("stderr = %q, want \"update canceled\"", errOut.String())
			}
		})
	}
}

func TestRun_NonTerminalWithoutYesRefuses(t *testing.T) {
	var downloads int
	srv := fullServer(t, "v0.2.0", "new binary", &downloads)
	opts, _, _, execPath := testOptions(t, srv, "v0.1.0")
	opts.IsTerminal = func() bool { return false }

	err := Run(context.Background(), *opts)
	if err == nil {
		t.Fatal("a non-interactive run without --yes must fail")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("err = %q, want it to point at --yes", err)
	}
	if downloads != 0 {
		t.Fatal("a refused run downloaded something")
	}
	if b, _ := os.ReadFile(execPath); string(b) != "old binary" {
		t.Fatalf("binary = %q, want it untouched", b)
	}
}

func TestRun_YesSkipsThePrompt(t *testing.T) {
	srv := fullServer(t, "v0.2.0", "new binary", nil)
	opts, out, errOut, execPath := testOptions(t, srv, "v0.1.0")
	opts.Yes = true
	opts.Stdin = strings.NewReader("n\n") // must not be read

	if err := Run(context.Background(), *opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(errOut.String(), "[y/N]") {
		t.Fatalf("stderr = %q, want no prompt with --yes", errOut.String())
	}
	if b, _ := os.ReadFile(execPath); string(b) != "new binary" {
		t.Fatalf("binary = %q, want the new one", b)
	}
	if !strings.Contains(out.String(), "v0.2.0") {
		t.Fatalf("stdout = %q, want a line naming the new version", out.String())
	}
}

func TestRun_UpToDateNeitherPromptsNorDownloads(t *testing.T) {
	var downloads int
	srv := fullServer(t, "v0.2.0", "new binary", &downloads)
	opts, out, errOut, _ := testOptions(t, srv, "v0.2.0")
	opts.IsTerminal = func() bool { return true }

	if err := Run(context.Background(), *opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if downloads != 0 {
		t.Fatal("an up-to-date install downloaded something")
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want nothing", errOut.String())
	}
	if !strings.Contains(out.String(), "up to date") {
		t.Fatalf("stdout = %q, want an up-to-date line", out.String())
	}
}

func TestRun_JSONResultAfterAnUpdate(t *testing.T) {
	srv := fullServer(t, "v0.2.0", "new binary", nil)
	opts, out, _, _ := testOptions(t, srv, "v0.1.0")
	opts.JSON, opts.Yes = true, true

	if err := Run(context.Background(), *opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := decodeResult(t, out.String())
	want := Result{Current: "v0.1.0", Latest: "v0.2.0", UpdateAvailable: true, Updated: true}
	if got != want {
		t.Fatalf("result = %+v, want %+v", got, want)
	}
}

func TestRun_JSONStillPromptsOnStderr(t *testing.T) {
	srv := fullServer(t, "v0.2.0", "new binary", nil)
	opts, out, errOut, _ := testOptions(t, srv, "v0.1.0")
	opts.JSON = true
	opts.IsTerminal = func() bool { return true }
	opts.Stdin = strings.NewReader("n\n")

	if err := Run(context.Background(), *opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(errOut.String(), "[y/N]") {
		t.Fatalf("stderr = %q, want the prompt even in JSON mode", errOut.String())
	}
	got := decodeResult(t, out.String())
	if got.Updated {
		t.Fatalf("result = %+v, want updated=false after a declined update", got)
	}
}

func TestRun_ChecksumMismatchLeavesTheBinaryAlone(t *testing.T) {
	tag := "v0.2.0"
	asset := tarGz(t, map[string]string{"qmeter": "new binary"})
	name := AssetName(tag, "linux", "amd64")
	base := repoPath + "/releases/download/" + tag + "/"
	mux := http.NewServeMux()
	mux.HandleFunc(repoPath+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", repoPath+"/releases/tag/"+tag)
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc(base+"checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sum([]byte("tampered")) + "  " + name + "\n"))
	})
	mux.HandleFunc(base+name, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(asset)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	opts, _, _, execPath := testOptions(t, srv, "v0.1.0")
	opts.Yes = true

	if err := Run(context.Background(), *opts); err == nil {
		t.Fatal("a checksum mismatch must fail the update")
	}
	if b, _ := os.ReadFile(execPath); string(b) != "old binary" {
		t.Fatalf("binary = %q, want it untouched after a checksum mismatch", b)
	}
	entries, _ := os.ReadDir(filepath.Dir(execPath))
	if len(entries) != 1 {
		t.Fatalf("install dir holds %d entries, want just the binary", len(entries))
	}
}

func TestRun_DefaultsResolveTheRunningBinary(t *testing.T) {
	// With no ExecPath the resolution goes through os.Executable; --check
	// never touches it, so this only asserts the defaults do not explode.
	srv := fullServer(t, "v0.2.0", "new binary", nil)
	var out, errOut bytes.Buffer
	err := Run(context.Background(), Options{
		Current: "v0.1.0",
		BaseURL: srv.URL,
		Check:   true,
		Out:     &out,
		Err:     &errOut,
	})
	if err != nil {
		t.Fatalf("Run with defaults: %v", err)
	}
	if !strings.Contains(out.String(), "v0.2.0") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestConfirm(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"y\n", true},
		{"Y\n", true},
		{" y \n", true},
		{"y", true},
		{"yes\n", false},
		{"n\n", false},
		{"N\n", false},
		{"\n", false},
		{"", false},
		{"maybe\n", false},
	}
	for _, tc := range tests {
		var w bytes.Buffer
		if got := confirm(strings.NewReader(tc.in), &w, "Proceed?"); got != tc.want {
			t.Errorf("confirm(%q) = %v, want %v", tc.in, got, tc.want)
		}
		if !strings.Contains(w.String(), "Proceed?") {
			t.Errorf("confirm(%q) wrote %q, want the prompt", tc.in, w.String())
		}
	}
}

func TestRun_CheckWinsOverYes(t *testing.T) {
	// --check is a question, --yes is an answer to a question that is
	// never asked. Together they must still report and stop: --yes must
	// not turn a check into an unattended install.
	var downloads int
	srv := fullServer(t, "v0.2.0", "new binary", &downloads)
	opts, out, _, execPath := testOptions(t, srv, "v0.1.0")
	opts.Check, opts.Yes = true, true

	if err := Run(context.Background(), *opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if downloads != 0 {
		t.Fatalf("--check --yes downloaded the asset %d times, want 0", downloads)
	}
	if b, _ := os.ReadFile(execPath); string(b) != "old binary" {
		t.Fatalf("--check --yes replaced the binary: %q", b)
	}
	if !strings.Contains(out.String(), "is available") {
		t.Fatalf("stdout = %q, want the check report", out.String())
	}
}
