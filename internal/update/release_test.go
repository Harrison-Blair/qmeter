package update

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// redirectServer serves /Harrison-Blair/qmeter/releases/latest with the
// given status and Location header.
func redirectServer(t *testing.T, status int, location string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Harrison-Blair/qmeter/releases/latest" {
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if location != "" {
			w.Header().Set("Location", location)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLatestTag_ReadsTheRedirectLocation(t *testing.T) {
	srv := redirectServer(t, http.StatusFound, "/Harrison-Blair/qmeter/releases/tag/v0.2.0")

	src := Source{BaseURL: srv.URL}
	got, err := src.LatestTag(context.Background())
	if err != nil {
		t.Fatalf("LatestTag: %v", err)
	}
	if got != "v0.2.0" {
		t.Fatalf("LatestTag = %q, want v0.2.0", got)
	}
}

func TestLatestTag_AbsoluteLocationWorks(t *testing.T) {
	srv := redirectServer(t, http.StatusMovedPermanently,
		"https://github.com/Harrison-Blair/qmeter/releases/tag/v1.0.0")

	src := Source{BaseURL: srv.URL}
	got, err := src.LatestTag(context.Background())
	if err != nil {
		t.Fatalf("LatestTag: %v", err)
	}
	if got != "v1.0.0" {
		t.Fatalf("LatestTag = %q, want v1.0.0", got)
	}
}

func TestLatestTag_RedirectToReleasesIndexMeansNoRelease(t *testing.T) {
	// This is what github.com actually does for a repo with no releases.
	srv := redirectServer(t, http.StatusFound, "/Harrison-Blair/qmeter/releases")

	src := Source{BaseURL: srv.URL}
	_, err := src.LatestTag(context.Background())
	if !errors.Is(err, ErrNoRelease) {
		t.Fatalf("err = %v, want ErrNoRelease", err)
	}
	if !strings.Contains(err.Error(), "no releases published yet") {
		t.Fatalf("err = %q, want it to say no releases published yet", err)
	}
}

func TestLatestTag_NonRedirectIsAnError(t *testing.T) {
	srv := redirectServer(t, http.StatusOK, "")

	src := Source{BaseURL: srv.URL}
	_, err := src.LatestTag(context.Background())
	if err == nil {
		t.Fatal("a 200 with no Location is not a usable answer")
	}
	if errors.Is(err, ErrNoRelease) {
		t.Fatalf("err = %v, want a status error, not ErrNoRelease", err)
	}
	if !strings.Contains(err.Error(), "200") {
		t.Fatalf("err = %q, want it to name the status", err)
	}
}

func TestLatestTag_MalformedTagIsAnError(t *testing.T) {
	srv := redirectServer(t, http.StatusFound, "/Harrison-Blair/qmeter/releases/tag/not-a-version")

	src := Source{BaseURL: srv.URL}
	_, err := src.LatestTag(context.Background())
	if err == nil {
		t.Fatal("a tag that is not vX.Y.Z must be rejected")
	}
	if !strings.Contains(err.Error(), "not-a-version") {
		t.Fatalf("err = %q, want it to quote the tag", err)
	}
}

func TestLatestTag_MissingLocationIsAnError(t *testing.T) {
	srv := redirectServer(t, http.StatusFound, "")

	src := Source{BaseURL: srv.URL}
	if _, err := src.LatestTag(context.Background()); err == nil {
		t.Fatal("a redirect with no Location must be an error")
	}
}

func TestLatestTag_DoesNotFollowTheRedirect(t *testing.T) {
	var followed bool
	mux := http.NewServeMux()
	mux.HandleFunc("/Harrison-Blair/qmeter/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/Harrison-Blair/qmeter/releases/tag/v3.4.5")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/Harrison-Blair/qmeter/releases/tag/v3.4.5", func(w http.ResponseWriter, r *http.Request) {
		followed = true
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	src := Source{BaseURL: srv.URL}
	if _, err := src.LatestTag(context.Background()); err != nil {
		t.Fatalf("LatestTag: %v", err)
	}
	if followed {
		t.Fatal("LatestTag followed the redirect; it must read Location instead")
	}
}

func TestLatestTag_HonoursContextCancellation(t *testing.T) {
	srv := redirectServer(t, http.StatusFound, "/Harrison-Blair/qmeter/releases/tag/v1.0.0")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	src := Source{BaseURL: srv.URL}
	if _, err := src.LatestTag(ctx); err == nil {
		t.Fatal("a cancelled context must fail the request")
	}
}

func TestAssetName(t *testing.T) {
	tests := []struct {
		goos, goarch, want string
	}{
		{"linux", "amd64", "qmeter_v1.2.3_linux_amd64.tar.gz"},
		{"linux", "arm64", "qmeter_v1.2.3_linux_arm64.tar.gz"},
		{"darwin", "arm64", "qmeter_v1.2.3_darwin_arm64.tar.gz"},
		{"windows", "amd64", "qmeter_v1.2.3_windows_amd64.zip"},
	}
	for _, tc := range tests {
		if got := AssetName("v1.2.3", tc.goos, tc.goarch); got != tc.want {
			t.Errorf("AssetName(v1.2.3, %s, %s) = %q, want %q", tc.goos, tc.goarch, got, tc.want)
		}
	}
}

func TestBinaryName(t *testing.T) {
	if got := binaryName("windows"); got != "qmeter.exe" {
		t.Errorf("binaryName(windows) = %q, want qmeter.exe", got)
	}
	if got := binaryName("linux"); got != "qmeter" {
		t.Errorf("binaryName(linux) = %q, want qmeter", got)
	}
}

func TestParseChecksums(t *testing.T) {
	const raw = "aaaa  qmeter_v1.2.3_linux_amd64.tar.gz\n" +
		"bbbb  qmeter_v1.2.3_windows_amd64.zip\n" +
		"\n" +
		"cccc *qmeter_v1.2.3_darwin_arm64.tar.gz\n"

	sums := parseChecksums([]byte(raw))
	want := map[string]string{
		"qmeter_v1.2.3_linux_amd64.tar.gz":  "aaaa",
		"qmeter_v1.2.3_windows_amd64.zip":   "bbbb",
		"qmeter_v1.2.3_darwin_arm64.tar.gz": "cccc",
	}
	for name, sum := range want {
		if sums[name] != sum {
			t.Errorf("parseChecksums()[%q] = %q, want %q", name, sums[name], sum)
		}
	}
	if len(sums) != len(want) {
		t.Errorf("parseChecksums() has %d entries, want %d: %v", len(sums), len(want), sums)
	}
}

func TestFetch_RejectsABodyOverTheLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("A", 4096)))
	}))
	t.Cleanup(srv.Close)

	src := Source{BaseURL: srv.URL}
	_, err := src.fetch(context.Background(), srv.URL, 100)
	if err == nil {
		t.Fatal("a body over the limit must be an error, not a silent truncation")
	}
	if !strings.Contains(err.Error(), "larger than the 100 byte limit") {
		t.Fatalf("err = %q, want it to name the limit", err)
	}
}

func TestFetch_AcceptsABodyExactlyAtTheLimit(t *testing.T) {
	body := strings.Repeat("A", 100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	src := Source{BaseURL: srv.URL}
	got, err := src.fetch(context.Background(), srv.URL, 100)
	if err != nil {
		t.Fatalf("a body exactly at the limit must be accepted: %v", err)
	}
	if string(got) != body {
		t.Fatalf("fetch returned %d bytes, want %d", len(got), len(body))
	}
}

func TestBinary_OversizedChecksumsFileIsRejected(t *testing.T) {
	// The real constant, not an injected one: an endless checksums.txt
	// must not be read into memory.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("A"), int(maxChecksumsBytes)+1))
	}))
	t.Cleanup(srv.Close)

	src := Source{BaseURL: srv.URL}
	_, err := src.Binary(context.Background(), "v1.2.3", "linux", "amd64")
	if err == nil {
		t.Fatal("an oversized checksums.txt must be an error")
	}
	if !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("err = %q, want it to name the limit", err)
	}
}
