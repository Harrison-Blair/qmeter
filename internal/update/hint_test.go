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
	"time"
)

var refNow = time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)

// hintServer serves the /releases/latest redirect for tag and counts how
// many times it was asked.
func hintServer(t *testing.T, tag string, calls *int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		if tag == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Location", repoPath+"/releases/tag/"+tag)
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func writeCache(t *testing.T, path, latest string, checkedAt time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	b, err := json.Marshal(map[string]string{
		"checked_at": checkedAt.Format(time.RFC3339),
		"latest":     latest,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write cache: %v", err)
	}
}

func readCache(t *testing.T, path string) (latest string, checkedAt time.Time) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("cache is not JSON (%v): %q", err, b)
	}
	ts, err := time.Parse(time.RFC3339, m["checked_at"])
	if err != nil {
		t.Fatalf("checked_at is not RFC3339 (%v): %q", err, m["checked_at"])
	}
	return m["latest"], ts
}

func hintOptions(t *testing.T, srv *httptest.Server, current string) (HintOptions, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "qmeter", "update-check.json")
	return HintOptions{
		Current:   current,
		BaseURL:   srv.URL,
		CachePath: path,
		Now:       func() time.Time { return refNow },
		Getenv:    func(string) string { return "" },
	}, path
}

func TestHint_FreshCacheWithANewerReleasePrintsOneLineWithoutANetworkCall(t *testing.T) {
	var calls int
	srv := hintServer(t, "v0.2.0", &calls)
	opts, path := hintOptions(t, srv, "v0.1.0")
	writeCache(t, path, "v0.2.0", refNow.Add(-1*time.Hour))

	var w bytes.Buffer
	Hint(context.Background(), &w, opts)

	want := "qmeter v0.2.0 is available (you have v0.1.0); run qmeter update\n"
	if w.String() != want {
		t.Fatalf("hint = %q, want %q", w.String(), want)
	}
	if calls != 0 {
		t.Fatalf("a fresh cache made %d network calls, want 0", calls)
	}
}

func TestHint_FreshCacheWithNoNewerReleaseIsSilent(t *testing.T) {
	var calls int
	srv := hintServer(t, "v0.2.0", &calls)
	opts, path := hintOptions(t, srv, "v0.2.0")
	writeCache(t, path, "v0.2.0", refNow.Add(-1*time.Hour))

	var w bytes.Buffer
	Hint(context.Background(), &w, opts)

	if w.Len() != 0 {
		t.Fatalf("hint = %q, want nothing", w.String())
	}
	if calls != 0 {
		t.Fatalf("a fresh cache made %d network calls, want 0", calls)
	}
}

func TestHint_StaleCacheChecksOnceAndRewritesTheFile(t *testing.T) {
	var calls int
	srv := hintServer(t, "v0.3.0", &calls)
	opts, path := hintOptions(t, srv, "v0.1.0")
	writeCache(t, path, "v0.2.0", refNow.Add(-25*time.Hour))

	var w bytes.Buffer
	Hint(context.Background(), &w, opts)

	if calls != 1 {
		t.Fatalf("network calls = %d, want exactly 1", calls)
	}
	if !strings.Contains(w.String(), "v0.3.0") {
		t.Fatalf("hint = %q, want the freshly fetched version", w.String())
	}
	latest, checkedAt := readCache(t, path)
	if latest != "v0.3.0" {
		t.Fatalf("cached latest = %q, want v0.3.0", latest)
	}
	if !checkedAt.Equal(refNow) {
		t.Fatalf("cached checked_at = %s, want %s", checkedAt, refNow)
	}
}

func TestHint_NoCacheFileChecksAndCreatesIt(t *testing.T) {
	var calls int
	srv := hintServer(t, "v0.3.0", &calls)
	opts, path := hintOptions(t, srv, "v0.1.0")

	var w bytes.Buffer
	Hint(context.Background(), &w, opts)

	if calls != 1 {
		t.Fatalf("network calls = %d, want 1", calls)
	}
	if latest, _ := readCache(t, path); latest != "v0.3.0" {
		t.Fatalf("cached latest = %q, want v0.3.0", latest)
	}
}

func TestHint_NetworkFailureIsSilentAndBacksOff(t *testing.T) {
	var calls int
	srv := hintServer(t, "", &calls) // always 500
	opts, path := hintOptions(t, srv, "v0.1.0")

	var w bytes.Buffer
	Hint(context.Background(), &w, opts)

	if w.Len() != 0 {
		t.Fatalf("a failed check printed %q, want nothing", w.String())
	}
	if calls != 1 {
		t.Fatalf("network calls = %d, want 1", calls)
	}
	// The failure is recorded so the next run does not retry immediately.
	latest, checkedAt := readCache(t, path)
	if latest != "" {
		t.Fatalf("cached latest = %q, want empty after a failure", latest)
	}
	if !checkedAt.Equal(refNow) {
		t.Fatalf("cached checked_at = %s, want %s", checkedAt, refNow)
	}

	var w2 bytes.Buffer
	Hint(context.Background(), &w2, opts)
	if calls != 1 {
		t.Fatalf("network calls = %d after a second run, want the failure to back off", calls)
	}
	if w2.Len() != 0 {
		t.Fatalf("second run printed %q", w2.String())
	}
}

func TestHint_MalformedCacheIsTreatedAsStale(t *testing.T) {
	var calls int
	srv := hintServer(t, "v0.3.0", &calls)
	opts, path := hintOptions(t, srv, "v0.1.0")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	var w bytes.Buffer
	Hint(context.Background(), &w, opts)

	if calls != 1 {
		t.Fatalf("network calls = %d, want 1", calls)
	}
	if !strings.Contains(w.String(), "v0.3.0") {
		t.Fatalf("hint = %q", w.String())
	}
}

func TestHint_EnvOptOutSkipsEverything(t *testing.T) {
	var calls int
	srv := hintServer(t, "v0.3.0", &calls)
	opts, path := hintOptions(t, srv, "v0.1.0")
	opts.Getenv = func(k string) string {
		if k == "QMETER_NO_UPDATE_CHECK" {
			return "1"
		}
		return ""
	}

	var w bytes.Buffer
	Hint(context.Background(), &w, opts)

	if w.Len() != 0 || calls != 0 {
		t.Fatalf("opt-out printed %q and made %d calls, want nothing", w.String(), calls)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("opt-out wrote the state file")
	}
}

func TestHint_UnknownCurrentVersionSkipsEverything(t *testing.T) {
	for _, current := range []string{"dev", "", "(devel)"} {
		t.Run(current, func(t *testing.T) {
			var calls int
			srv := hintServer(t, "v0.3.0", &calls)
			opts, _ := hintOptions(t, srv, current)
			if current == "" {
				// An empty Current falls back to version.String(), which
				// under `go test` is "dev"; both must stay silent.
				opts.Current = ""
			}

			var w bytes.Buffer
			Hint(context.Background(), &w, opts)

			if w.Len() != 0 || calls != 0 {
				t.Fatalf("current %q printed %q and made %d calls", current, w.String(), calls)
			}
		})
	}
}

func TestHint_CancelledContextIsSilent(t *testing.T) {
	var calls int
	srv := hintServer(t, "v0.3.0", &calls)
	opts, _ := hintOptions(t, srv, "v0.1.0")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var w bytes.Buffer
	Hint(ctx, &w, opts)

	if w.Len() != 0 {
		t.Fatalf("a cancelled hint printed %q", w.String())
	}
}

func TestHint_UnwritableCacheStillPrints(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write into a mode 0500 directory")
	}
	var calls int
	srv := hintServer(t, "v0.3.0", &calls)
	opts, path := hintOptions(t, srv, "v0.1.0")
	dir := filepath.Dir(filepath.Dir(path))
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	var w bytes.Buffer
	Hint(context.Background(), &w, opts)

	if !strings.Contains(w.String(), "v0.3.0") {
		t.Fatalf("hint = %q, want the line even when the cache cannot be written", w.String())
	}
}

func TestHint_NoCachePathIsSilent(t *testing.T) {
	var calls int
	srv := hintServer(t, "v0.3.0", &calls)
	opts, _ := hintOptions(t, srv, "v0.1.0")
	opts.CachePath = ""
	// A machine with no user cache directory must not turn the hint into
	// a check on every single run.
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")

	var w bytes.Buffer
	Hint(context.Background(), &w, opts)

	if w.Len() != 0 || calls != 0 {
		t.Fatalf("printed %q and made %d calls, want nothing", w.String(), calls)
	}
}

func TestHint_TimeoutBudgetIsTwoSeconds(t *testing.T) {
	if hintTimeout != 2*time.Second {
		t.Fatalf("hintTimeout = %s, want 2s", hintTimeout)
	}
}

func TestHint_CacheTTLIsADay(t *testing.T) {
	if hintTTL != 24*time.Hour {
		t.Fatalf("hintTTL = %s, want 24h", hintTTL)
	}
}

func TestDefaultCachePath_LivesUnderTheUserCacheDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	got, err := DefaultCachePath()
	if err != nil {
		t.Fatalf("DefaultCachePath: %v", err)
	}
	if want := filepath.Join(dir, "qmeter", "update-check.json"); got != want {
		t.Fatalf("DefaultCachePath = %q, want %q", got, want)
	}
}
