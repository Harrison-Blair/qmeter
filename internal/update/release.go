// Package update implements `qmeter update`: finding the latest published
// release, downloading and verifying the asset for this platform, replacing
// the running binary in place, and the passive "an update is available"
// hint printed after `qmeter usage`.
//
// Everything that touches the network, the filesystem or the terminal is
// injectable, so the whole package is testable without a real release, a
// real GitHub or a real binary swap.
package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// DefaultBaseURL is the host releases are published on. It is a field on
// Source rather than a constant use site so tests can point at httptest.
const DefaultBaseURL = "https://github.com"

// repoPath is the owner/repo segment of every release URL.
const repoPath = "/Harrison-Blair/qmeter"

// ErrNoRelease reports that the repository has no published release yet, as
// opposed to the lookup itself failing.
var ErrNoRelease = errors.New("update: no releases published yet")

// maxAssetBytes caps a downloaded asset and anything extracted from it, so
// a hostile or broken server cannot exhaust memory or fill the disk.
const maxAssetBytes int64 = 200 << 20

// maxChecksumsBytes caps the checksums file, which is a few hundred bytes
// in practice.
const maxChecksumsBytes int64 = 1 << 20

// Source fetches release metadata and assets over HTTP.
type Source struct {
	// BaseURL is the release host. Empty means DefaultBaseURL.
	BaseURL string

	// Client is the http.Client to use; nil means http.DefaultClient.
	// Source never sets a Timeout on it — the deadline comes from ctx.
	Client *http.Client
}

func (s Source) baseURL() string {
	if s.BaseURL == "" {
		return DefaultBaseURL
	}
	return strings.TrimSuffix(s.BaseURL, "/")
}

func (s Source) client() *http.Client {
	if s.Client == nil {
		return http.DefaultClient
	}
	return s.Client
}

// LatestTag returns the tag of the latest release.
//
// It deliberately avoids the REST API, whose unauthenticated rate limit is
// 60 requests an hour: GET /<owner>/<repo>/releases/latest with redirects
// disabled answers with a Location of /<owner>/<repo>/releases/tag/<tag>.
// A repository with no releases redirects to the releases index instead,
// which is ErrNoRelease rather than a failure.
func (s Source) LatestTag(ctx context.Context) (string, error) {
	target := s.baseURL() + repoPath + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", fmt.Errorf("update: new request: %w", err)
	}

	// Copy the caller's client so disabling redirects here cannot leak
	// into a client shared with anything else.
	c := *s.client()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("update: GET %s: %w", target, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxChecksumsBytes))

	if resp.StatusCode < 300 || resp.StatusCode > 399 {
		return "", fmt.Errorf("update: GET %s: unexpected status %d", target, resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("update: GET %s: redirect with no Location header", target)
	}
	// Location may be absolute or relative; only the path matters.
	path := loc
	if u, err := url.Parse(loc); err == nil {
		path = u.Path
	}
	const marker = "/releases/tag/"
	i := strings.Index(path, marker)
	if i < 0 {
		// GitHub sends a repo with no releases to the releases index.
		return "", ErrNoRelease
	}
	tag := strings.Trim(path[i+len(marker):], "/")
	if _, err := ParseVersion(tag); err != nil {
		return "", fmt.Errorf("update: latest release tag %q is not a vX.Y.Z version", tag)
	}
	return tag, nil
}

// AssetName is the release asset holding the binary for one platform.
func AssetName(tag, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("qmeter_%s_%s_%s%s", tag, goos, goarch, ext)
}

// binaryName is the name of the executable inside an asset.
func binaryName(goos string) string {
	if goos == "windows" {
		return "qmeter.exe"
	}
	return "qmeter"
}

// assetURL is the download URL for one asset of one release.
func (s Source) assetURL(tag, name string) string {
	return s.baseURL() + repoPath + "/releases/download/" + tag + "/" + name
}

// fetch GETs url and returns at most limit bytes of the body.
func (s Source) fetch(ctx context.Context, target string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("update: new request: %w", err)
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("update: GET %s: %w", target, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("update: GET %s: unexpected status %d", target, resp.StatusCode)
	}
	// limit+1 so a body that is exactly at the cap is distinguishable
	// from one that overruns it.
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("update: GET %s: %w", target, err)
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("update: GET %s: response is larger than the %d byte limit", target, limit)
	}
	return b, nil
}

// parseChecksums reads a sha256sum-format file into a filename -> hex map.
// Both the "  " (text) and " *" (binary) separators are accepted.
func parseChecksums(data []byte) map[string]string {
	sums := make(map[string]string)
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 {
			continue
		}
		sums[strings.TrimPrefix(fields[1], "*")] = fields[0]
	}
	return sums
}
