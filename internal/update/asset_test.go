package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"os"
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

// zipWithLyingSize builds a zip whose central directory understates how
// much data the entry really expands to, which is what a hostile archive
// looks like: the declared size passes any up-front check, the stream then
// runs on.
func zipWithLyingSize(t *testing.T, name string, real []byte, declared uint64) []byte {
	t.Helper()
	var deflated bytes.Buffer
	fw, err := flate.NewWriter(&deflated, flate.DefaultCompression)
	if err != nil {
		t.Fatalf("flate writer: %v", err)
	}
	if _, err := fw.Write(real); err != nil {
		t.Fatalf("flate write: %v", err)
	}
	if err := fw.Close(); err != nil {
		t.Fatalf("flate close: %v", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	fh := &zip.FileHeader{
		Name:               name,
		Method:             zip.Deflate,
		CRC32:              crc32.ChecksumIEEE(real),
		CompressedSize64:   uint64(deflated.Len()),
		UncompressedSize64: declared,
	}
	fh.SetMode(0o755)
	w, err := zw.CreateRaw(fh)
	if err != nil {
		t.Fatalf("CreateRaw: %v", err)
	}
	if _, err := w.Write(deflated.Bytes()); err != nil {
		t.Fatalf("write raw: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func TestExtractBinary_LyingHeaderDoesNotYieldATruncatedBinary(t *testing.T) {
	// The header says 10 bytes, which slips past the declared-size check;
	// the stream then delivers 4096. Handing back a prefix of that as
	// "the new qmeter" would be the worst possible outcome — it must be
	// an error, whether archive/zip catches the overrun or readCapped
	// does.
	asset := zipWithLyingSize(t, "qmeter", bytes.Repeat([]byte("A"), 4096), 10)

	got, err := extractBinary(asset, "x.zip", "qmeter", 100)
	if err == nil {
		t.Fatalf("an entry that outruns its declared size must be an error, got %d bytes", len(got))
	}
	if got != nil {
		t.Fatalf("a rejected entry must return no bytes, got %d", len(got))
	}
}

func TestReadCapped_RejectsAReaderThatOutrunsTheLimit(t *testing.T) {
	// readCapped is the backstop under both archive readers: whatever a
	// header claimed, nothing over the cap is ever handed back as a
	// binary, and an overrun is an error rather than a silent truncation.
	_, err := readCapped(strings.NewReader(strings.Repeat("A", 4096)), "qmeter", 100)
	if err == nil {
		t.Fatal("a reader that outruns the limit must be an error, not a truncation")
	}
	if !strings.Contains(err.Error(), "larger than the 100 byte limit") {
		t.Fatalf("err = %q, want it to name the limit", err)
	}

	got, err := readCapped(strings.NewReader(strings.Repeat("A", 100)), "qmeter", 100)
	if err != nil {
		t.Fatalf("a reader exactly at the limit must be accepted: %v", err)
	}
	if len(got) != 100 {
		t.Fatalf("readCapped returned %d bytes, want 100", len(got))
	}
}

func TestExtractBinary_TruncatedTarEntryIsAnError(t *testing.T) {
	// The other half of a lying header: the tar claims more bytes than
	// the archive actually holds.
	var body bytes.Buffer
	tw := tar.NewWriter(&body)
	if err := tw.WriteHeader(&tar.Header{
		Name: "qmeter", Mode: 0o755, Size: 4096, Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write(bytes.Repeat([]byte("A"), 100)); err != nil {
		t.Fatalf("tar write: %v", err)
	}
	// Deliberately not tw.Close(): the entry is left short.
	var gzbuf bytes.Buffer
	gz := gzip.NewWriter(&gzbuf)
	if _, err := gz.Write(body.Bytes()); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	if _, err := extractBinary(gzbuf.Bytes(), "x.tar.gz", "qmeter", maxAssetBytes); err == nil {
		t.Fatal("a tar entry shorter than its header claims must be an error")
	}
}

func TestExtractBinary_OnlyTakesATopLevelEntry(t *testing.T) {
	// The release archives are flat (`tar -czf … -C stage qmeter LICENSE
	// README.md`), so a qmeter buried in a subdirectory is not the file
	// the release publishes and must not be mistaken for it.
	asset := tarGz(t, map[string]string{"nested/qmeter": "payload", "LICENSE": "MIT"})
	if _, err := extractBinary(asset, "x.tar.gz", "qmeter", maxAssetBytes); err == nil {
		t.Fatal("a nested qmeter must not be taken for the release binary")
	}

	zasset := zipOf(t, map[string]string{"nested/qmeter.exe": "payload"})
	if _, err := extractBinary(zasset, "x.zip", "qmeter.exe", maxAssetBytes); err == nil {
		t.Fatal("a nested qmeter.exe must not be taken for the release binary")
	}
}

func TestExtractBinary_SkipsANonRegularZipEntry(t *testing.T) {
	// A symlink named qmeter is not a binary; following one would let an
	// archive point the "update" at any file on the machine.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	fh := &zip.FileHeader{Name: "qmeter", Method: zip.Store}
	fh.SetMode(os.ModeSymlink | 0o777)
	w, err := zw.CreateHeader(fh)
	if err != nil {
		t.Fatalf("CreateHeader: %v", err)
	}
	if _, err := w.Write([]byte("/etc/passwd")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}

	if _, err := extractBinary(buf.Bytes(), "x.zip", "qmeter", maxAssetBytes); err == nil {
		t.Fatal("a symlink entry must not be extracted as the binary")
	}
}

func TestExtractBinary_TakesTheRealReleaseLayout(t *testing.T) {
	// Exactly what .github/workflows/release.yml produces: flat entries,
	// the binary first, LICENSE and README.md alongside it.
	asset := tarGz(t, map[string]string{"qmeter": "payload", "LICENSE": "MIT", "README.md": "# qmeter"})
	got, err := extractBinary(asset, AssetName("v1.2.3", "linux", "amd64"), "qmeter", maxAssetBytes)
	if err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	if string(got) != "payload" {
		t.Fatalf("got %q, want the binary", got)
	}
}
