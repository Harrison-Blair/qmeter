package opencodego

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Harrison-Blair/qmeter/internal/lib/credstore"

	// The blank import registers the "sqlite" database/sql driver.
	// modernc.org/sqlite is a pure-Go, CGo-free port of SQLite, which matters
	// here specifically because .github/workflows/release.yml cross-compiles
	// qmeter with CGO_ENABLED=0 for six GOOS/GOARCH targets (linux, darwin and
	// windows, each amd64 and arm64); a cgo-based driver such as
	// mattn/go-sqlite3 would break that build.
	_ "modernc.org/sqlite"
)

// dbBusyTimeoutMillis bounds how long a read waits behind a writer holding
// the database's lock — OpenCode itself, mid-write — before giving up.
// OpenCode's own writes to this table are short (one login, one token
// refresh), so a few seconds comfortably absorbs a collision without qmeter
// itself hanging.
const dbBusyTimeoutMillis = 3000

// dbEntry is the JSON shape of the SQLite `credential` table's `value`
// column: {"type":"key","key":"..."}. It is the same per-entry shape
// storeEntry decodes for auth.json — OpenCode 2.x relocated the store, not
// the shape of what it keeps in it — so only Key matters here too.
type dbEntry struct {
	Type string `json:"type"`
	Key  string `json:"key"`
}

// loadKeyFromDB reads the OpenCode Go API key out of OpenCode's v2.x SQLite
// credential database at path.
//
// It follows a loader contract close to loadKey's:
//   - path is empty, or the file is absent: credstore.ErrNotFound.
//   - the file opens but has no `credential` table, no row for
//     "opencode-go", or that row's key is blank after trimming:
//     credstore.ErrNotFound.
//   - anything else (not a SQLite database, unreadable, the row's value not
//     the documented JSON) is a descriptive error — loadCredential decides
//     whether to surface that or quietly prefer auth.json.
//
// Among several rows for "opencode-go", active = 1 wins; a tie (several
// active, or none) goes to the most recent time_created.
//
// ctx is only checked up front: an already-done ctx makes this return
// immediately, before the database is even opened, with ctx's own error.
// Once a read is underway, a lock another connection holds is instead
// bounded by the busy timeout above (dbBusyTimeoutMillis) — SQLite's busy
// handler retries in its own blocking loop, deaf to ctx, so a ctx that is
// cancelled or expires mid-wait has no effect: the read still runs to that
// timeout and reports SQLITE_BUSY, not ctx's error.
func loadKeyFromDB(ctx context.Context, path string) (string, error) {
	if path == "" {
		return "", credstore.ErrNotFound
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", credstore.ErrNotFound
		}
		return "", fmt.Errorf("%s: %w", path, err)
	}

	db, err := sql.Open("sqlite", readOnlyDSN(path))
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer db.Close()

	// sql.Open never dials; PingContext forces the actual connection open
	// (and so honours a context that is already done) before the query below
	// spends a busy-timeout wait on it.
	if err := db.PingContext(ctx); err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}

	const query = `
		SELECT value FROM credential
		WHERE integration_id = ?
		ORDER BY active DESC, time_created DESC
		LIMIT 1`
	var value string
	err = db.QueryRowContext(ctx, query, storeEntryKey).Scan(&value)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", credstore.ErrNotFound
	case isMissingCredentialTable(err):
		// No `credential` table at all — this OpenCode's schema doesn't have
		// one (yet, or on a version this code has never seen). Treated the
		// same as a missing database: fall back to auth.json without comment.
		return "", credstore.ErrNotFound
	case err != nil:
		return "", fmt.Errorf("query %s: %w", path, err)
	}

	var entry dbEntry
	if err := json.Unmarshal([]byte(value), &entry); err != nil {
		return "", fmt.Errorf("%s has an unreadable %q credential value: %w", path, storeEntryKey, err)
	}
	// Trimmed for the same reason loadKey trims: a key with a trailing
	// newline would pass this check and then be rejected by net/http as an
	// invalid Authorization header value.
	key := strings.TrimSpace(entry.Key)
	if key == "" {
		return "", credstore.ErrNotFound
	}
	return key, nil
}

// isMissingCredentialTable reports whether err is SQLite's "no such table"
// failure for the query above. Matched on the message rather than a typed
// error or error code because modernc.org/sqlite surfaces every query
// failure as the same generic *sqlite.Error shape, with the distinguishing
// detail only in Error()'s text.
func isMissingCredentialTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}

// readOnlyDSN builds a SQLite DSN that opens path strictly read-only via
// SQLite's own "mode=ro" URI parameter, plus a busy timeout for the rare
// collision with OpenCode's own writer.
//
// mode=ro — not "immutable=1" — is required: OpenCode may hold the database
// in WAL mode with recent writes sitting only in opencode.db-wal, and
// "immutable=1" would tell SQLite to skip looking for that file, reading a
// stale (or empty) result. "mode=ro" instead follows WAL normally while still
// refusing writes: SQLite honours a stricter mode in the URI over the
// read-write-create flags modernc.org/sqlite's driver always passes to
// sqlite3_open_v2 (a more restrictive mode is allowed; a less restrictive one
// is an error).
func readOnlyDSN(path string) string {
	u := fileURI(path)
	q := u.Query()
	q.Set("mode", "ro")
	q.Set("_busy_timeout", strconv.Itoa(dbBusyTimeoutMillis))
	u.RawQuery = q.Encode()
	return u.String()
}

// fileURI turns a native filesystem path — including a Windows
// "C:\Users\...\opencode.db" one — into a "file:" *url.URL, escaping
// characters (a space, "?", "#") the path may contain.
func fileURI(path string) url.URL {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return url.URL{Scheme: "file", Path: p}
}
