package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// tarGz builds a .tar.gz holding the given name -> content entries.
func tarGz(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range entries {
		hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

// zipOf builds a .zip holding the given name -> content entries.
func zipOf(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// releaseServer serves one release: the asset under its download path and a
// checksums.txt built from checksums (name -> hex), so a test can publish a
// deliberately wrong sum.
func releaseServer(t *testing.T, tag string, assets map[string][]byte, checksums map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	base := repoPath + "/releases/download/" + tag + "/"
	for name, body := range assets {
		mux.HandleFunc(base+name, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(body)
		})
	}
	mux.HandleFunc(base+"checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		var b strings.Builder
		for name, hexsum := range checksums {
			fmt.Fprintf(&b, "%s  %s\n", hexsum, name)
		}
		_, _ = w.Write([]byte(b.String()))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestBinary_TarGz(t *testing.T) {
	asset := tarGz(t, map[string]string{
		"qmeter":    "ELF-ish payload",
		"LICENSE":   "MIT",
		"README.md": "# qmeter",
	})
	name := AssetName("v1.2.3", "linux", "amd64")
	srv := releaseServer(t, "v1.2.3",
		map[string][]byte{name: asset},
		map[string]string{name: sum(asset)})

	src := Source{BaseURL: srv.URL}
	got, err := src.Binary(context.Background(), "v1.2.3", "linux", "amd64")
	if err != nil {
		t.Fatalf("Binary: %v", err)
	}
	if string(got) != "ELF-ish payload" {
		t.Fatalf("Binary = %q, want the qmeter entry", got)
	}
}

func TestBinary_Zip(t *testing.T) {
	asset := zipOf(t, map[string]string{
		"qmeter.exe": "PE-ish payload",
		"LICENSE":    "MIT",
	})
	name := AssetName("v1.2.3", "windows", "amd64")
	srv := releaseServer(t, "v1.2.3",
		map[string][]byte{name: asset},
		map[string]string{name: sum(asset)})

	src := Source{BaseURL: srv.URL}
	got, err := src.Binary(context.Background(), "v1.2.3", "windows", "amd64")
	if err != nil {
		t.Fatalf("Binary: %v", err)
	}
	if string(got) != "PE-ish payload" {
		t.Fatalf("Binary = %q, want the qmeter.exe entry", got)
	}
}

func TestBinary_ChecksumMismatchIsRejected(t *testing.T) {
	asset := tarGz(t, map[string]string{"qmeter": "payload"})
	name := AssetName("v1.2.3", "linux", "amd64")
	srv := releaseServer(t, "v1.2.3",
		map[string][]byte{name: asset},
		map[string]string{name: sum([]byte("something else"))})

	src := Source{BaseURL: srv.URL}
	_, err := src.Binary(context.Background(), "v1.2.3", "linux", "amd64")
	if err == nil {
		t.Fatal("a checksum mismatch must be a hard error")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %q, want it to mention the checksum", err)
	}
}

func TestBinary_MissingChecksumEntryIsRejected(t *testing.T) {
	asset := tarGz(t, map[string]string{"qmeter": "payload"})
	name := AssetName("v1.2.3", "linux", "amd64")
	srv := releaseServer(t, "v1.2.3",
		map[string][]byte{name: asset},
		map[string]string{"some_other_asset.tar.gz": sum(asset)})

	src := Source{BaseURL: srv.URL}
	_, err := src.Binary(context.Background(), "v1.2.3", "linux", "amd64")
	if err == nil {
		t.Fatal("an asset with no published checksum must be rejected")
	}
	if !strings.Contains(err.Error(), name) {
		t.Fatalf("err = %q, want it to name the asset", err)
	}
}

func TestBinary_MissingAssetIsAnError(t *testing.T) {
	srv := releaseServer(t, "v1.2.3", nil, map[string]string{})

	src := Source{BaseURL: srv.URL}
	_, err := src.Binary(context.Background(), "v1.2.3", "linux", "amd64")
	if err == nil {
		t.Fatal("a 404 asset must be an error")
	}
}

func TestExtractBinary_RejectsPathTraversal(t *testing.T) {
	for _, name := range []string{"../qmeter", "/etc/qmeter", "nested/../../qmeter"} {
		t.Run(name, func(t *testing.T) {
			asset := tarGz(t, map[string]string{name: "payload"})
			_, err := extractBinary(asset, "x.tar.gz", "qmeter", maxAssetBytes)
			if err == nil {
				t.Fatalf("entry %q must be rejected", name)
			}
			if !strings.Contains(err.Error(), "unsafe") {
				t.Fatalf("err = %q, want it to call the path unsafe", err)
			}
		})
	}
}

func TestExtractBinary_RejectsPathTraversalInZip(t *testing.T) {
	asset := zipOf(t, map[string]string{"../qmeter.exe": "payload"})
	if _, err := extractBinary(asset, "x.zip", "qmeter.exe", maxAssetBytes); err == nil {
		t.Fatal("a zip entry escaping the archive root must be rejected")
	}
}

func TestExtractBinary_EnforcesTheSizeCap(t *testing.T) {
	asset := tarGz(t, map[string]string{"qmeter": strings.Repeat("A", 4096)})
	_, err := extractBinary(asset, "x.tar.gz", "qmeter", 100)
	if err == nil {
		t.Fatal("an entry over the cap must be rejected")
	}
	if !strings.Contains(err.Error(), "larger") {
		t.Fatalf("err = %q, want it to say the entry is too large", err)
	}

	zasset := zipOf(t, map[string]string{"qmeter": strings.Repeat("A", 4096)})
	if _, err := extractBinary(zasset, "x.zip", "qmeter", 100); err == nil {
		t.Fatal("a zip entry over the cap must be rejected")
	}
}

func TestExtractBinary_MissingEntryIsAnError(t *testing.T) {
	asset := tarGz(t, map[string]string{"LICENSE": "MIT"})
	_, err := extractBinary(asset, "x.tar.gz", "qmeter", maxAssetBytes)
	if err == nil {
		t.Fatal("an archive with no qmeter entry must be an error")
	}
	if !strings.Contains(err.Error(), "qmeter") {
		t.Fatalf("err = %q, want it to name the missing entry", err)
	}
}

func TestExtractBinary_CorruptArchiveIsAnError(t *testing.T) {
	if _, err := extractBinary([]byte("not an archive"), "x.tar.gz", "qmeter", maxAssetBytes); err == nil {
		t.Fatal("a corrupt tar.gz must be an error")
	}
	if _, err := extractBinary([]byte("not an archive"), "x.zip", "qmeter", maxAssetBytes); err == nil {
		t.Fatal("a corrupt zip must be an error")
	}
}
