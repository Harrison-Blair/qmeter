package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// Binary downloads the release asset for one platform, verifies it against
// the release's checksums.txt and returns the qmeter executable extracted
// from it. A checksum mismatch is a hard error: nothing is returned and
// nothing is written anywhere.
func (s Source) Binary(ctx context.Context, tag, goos, goarch string) ([]byte, error) {
	name := AssetName(tag, goos, goarch)

	sums, err := s.fetch(ctx, s.assetURL(tag, "checksums.txt"), maxChecksumsBytes)
	if err != nil {
		return nil, err
	}
	want, ok := parseChecksums(sums)[name]
	if !ok {
		return nil, fmt.Errorf("update: release %s publishes no checksum for %s", tag, name)
	}

	asset, err := s.fetch(ctx, s.assetURL(tag, name), maxAssetBytes)
	if err != nil {
		return nil, err
	}
	if got := sha256.Sum256(asset); hex.EncodeToString(got[:]) != want {
		return nil, fmt.Errorf("update: checksum mismatch for %s: got %s, want %s",
			name, hex.EncodeToString(got[:]), want)
	}
	return extractBinary(asset, name, binaryName(goos), maxAssetBytes)
}

// extractBinary pulls the single entry named binName out of a .tar.gz or
// .zip asset. Every other entry is ignored, an entry whose path escapes the
// archive root is refused outright, and nothing over limit bytes is read
// into memory.
func extractBinary(asset []byte, assetName, binName string, limit int64) ([]byte, error) {
	if strings.HasSuffix(assetName, ".zip") {
		return extractFromZip(asset, binName, limit)
	}
	return extractFromTarGz(asset, binName, limit)
}

// safeEntry reports whether an archive entry name stays inside the archive
// root: no absolute path, no "..", no drive-ish or backslash trickery.
func safeEntry(name string) error {
	clean := path.Clean(strings.ReplaceAll(name, `\`, "/"))
	if path.IsAbs(clean) || strings.HasPrefix(clean, "../") || clean == ".." {
		return fmt.Errorf("update: archive entry %q has an unsafe path", name)
	}
	return nil
}

func extractFromTarGz(asset []byte, binName string, limit int64) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(asset))
	if err != nil {
		return nil, fmt.Errorf("update: read release archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("update: read release archive: %w", err)
		}
		if err := safeEntry(hdr.Name); err != nil {
			return nil, err
		}
		if path.Base(hdr.Name) != binName || hdr.Typeflag != tar.TypeReg {
			continue
		}
		if hdr.Size > limit {
			return nil, fmt.Errorf("update: %s in the release archive is larger than the %d byte limit",
				binName, limit)
		}
		return readCapped(tr, binName, limit)
	}
	return nil, fmt.Errorf("update: the release archive has no %s entry", binName)
}

func extractFromZip(asset []byte, binName string, limit int64) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(asset), int64(len(asset)))
	if err != nil {
		return nil, fmt.Errorf("update: read release archive: %w", err)
	}
	for _, f := range zr.File {
		if err := safeEntry(f.Name); err != nil {
			return nil, err
		}
	}
	for _, f := range zr.File {
		if path.Base(f.Name) != binName || f.FileInfo().IsDir() {
			continue
		}
		if f.UncompressedSize64 > uint64(limit) {
			return nil, fmt.Errorf("update: %s in the release archive is larger than the %d byte limit",
				binName, limit)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("update: read release archive: %w", err)
		}
		defer rc.Close()
		return readCapped(rc, binName, limit)
	}
	return nil, fmt.Errorf("update: the release archive has no %s entry", binName)
}

// readCapped reads at most limit bytes and treats an overrun — a header
// that lied about its size — as a failure rather than a truncation.
func readCapped(r io.Reader, binName string, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("update: read %s from the release archive: %w", binName, err)
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("update: %s in the release archive is larger than the %d byte limit",
			binName, limit)
	}
	return b, nil
}
