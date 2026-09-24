package opencodego

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/lib/credstore"

	_ "modernc.org/sqlite"
)

// credentialRow is one row to seed into a test `credential` table. Only the
// columns loadKeyFromDB's query touches are given meaningful defaults; the
// rest exist so the schema matches OpenCode's real one.
type credentialRow struct {
	id            string
	integrationID string
	value         string
	active        int
	timeCreated   int64
}

// newTestDB creates a fresh SQLite file at <dir>/opencode.db with the
// `credential` table OpenCode's real store has (plus a couple of unrelated
// columns, so the query proves it only cares about the ones it names) and
// seeds it with rows. It returns the path and the still-open writer
// connection: callers that want to exercise a WAL-resident row (data that
// exists only in -wal, never checkpointed into the main file) must read
// through loadKeyFromDB before closing it; callers that don't care can close
// it immediately, which triggers SQLite's automatic checkpoint-on-last-close.
func newTestDB(t *testing.T, rows []credentialRow) (path string, writer *sql.DB) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "opencode.db")

	// journal_mode(WAL) is set on the writer connection so a row inserted
	// without an explicit checkpoint stays only in opencode.db-wal, exactly
	// the layout a running OpenCode leaves qmeter to read around.
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`CREATE TABLE credential (
		id TEXT,
		integration_id TEXT,
		label TEXT,
		value TEXT,
		connector_id TEXT,
		method_id TEXT,
		active INTEGER,
		time_created INTEGER
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	for _, r := range rows {
		if _, err := db.Exec(
			`INSERT INTO credential (id, integration_id, label, value, connector_id, method_id, active, time_created)
			 VALUES (?, ?, 'label', ?, 'connector', 'method', ?, ?)`,
			r.id, r.integrationID, r.value, r.active, r.timeCreated,
		); err != nil {
			t.Fatalf("insert row %s: %v", r.id, err)
		}
	}
	return path, db
}

// keyValue builds the {"type":"key","key":"..."} JSON the `value` column
// stores, matching auth.json's per-entry shape. It goes through
// encoding/json rather than string concatenation so a key containing
// whitespace that needs escaping (a newline, a tab) round-trips correctly
// instead of producing invalid JSON.
func keyValue(key string) string {
	b, err := json.Marshal(dbEntry{Type: "key", Key: key})
	if err != nil {
		panic(err) // dbEntry is always marshalable
	}
	return string(b)
}

func TestLoadKeyFromDB_ReadsWALResidentRow(t *testing.T) {
	const want = "sk-opencode-go-wal-resident-key-0123456789012345678901"
	path, writer := newTestDB(t, []credentialRow{
		{id: "1", integrationID: storeEntryKey, value: keyValue(want), active: 1, timeCreated: 100},
	})

	// The row above was never checkpointed: confirm it is genuinely
	// WAL-resident before trusting the read that follows.
	if _, err := os.Stat(path + "-wal"); err != nil {
		t.Fatalf("expected a -wal file alongside %s: %v", path, err)
	}

	got, err := loadKeyFromDB(testContext(t), path)
	if err != nil {
		t.Fatalf("loadKeyFromDB() err = %v", err)
	}
	if got != want {
		t.Errorf("loadKeyFromDB() = %q, want %q", got, want)
	}

	// The writer must stay open until after the read above: closing it
	// would let SQLite auto-checkpoint the WAL into the main file, which
	// would defeat the point of this test.
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
}

func TestLoadKeyFromDB_PrefersActiveRow(t *testing.T) {
	const want = "sk-opencode-go-active-key-01234567890123456789012345"
	path, writer := newTestDB(t, []credentialRow{
		{id: "1", integrationID: storeEntryKey, value: keyValue("sk-opencode-go-stale-inactive-key-01234567890123456789"), active: 0, timeCreated: 200},
		{id: "2", integrationID: storeEntryKey, value: keyValue(want), active: 1, timeCreated: 100},
	})
	t.Cleanup(func() { _ = writer.Close() })

	got, err := loadKeyFromDB(testContext(t), path)
	if err != nil {
		t.Fatalf("loadKeyFromDB() err = %v", err)
	}
	if got != want {
		t.Errorf("loadKeyFromDB() = %q, want the active row's key %q", got, want)
	}
}

func TestLoadKeyFromDB_MultipleActiveRowsPreferMostRecent(t *testing.T) {
	const want = "sk-opencode-go-newest-active-key-0123456789012345678"
	path, writer := newTestDB(t, []credentialRow{
		{id: "1", integrationID: storeEntryKey, value: keyValue("sk-opencode-go-older-active-key-0123456789012345678"), active: 1, timeCreated: 100},
		{id: "2", integrationID: storeEntryKey, value: keyValue(want), active: 1, timeCreated: 200},
	})
	t.Cleanup(func() { _ = writer.Close() })

	got, err := loadKeyFromDB(testContext(t), path)
	if err != nil {
		t.Fatalf("loadKeyFromDB() err = %v", err)
	}
	if got != want {
		t.Errorf("loadKeyFromDB() = %q, want the most recently created active row's key %q", got, want)
	}
}

func TestLoadKeyFromDB_NoActiveRowsPreferMostRecent(t *testing.T) {
	const want = "sk-opencode-go-newest-inactive-key-012345678901234567"
	path, writer := newTestDB(t, []credentialRow{
		{id: "1", integrationID: storeEntryKey, value: keyValue("sk-opencode-go-older-inactive-key-012345678901234567"), active: 0, timeCreated: 100},
		{id: "2", integrationID: storeEntryKey, value: keyValue(want), active: 0, timeCreated: 200},
	})
	t.Cleanup(func() { _ = writer.Close() })

	got, err := loadKeyFromDB(testContext(t), path)
	if err != nil {
		t.Fatalf("loadKeyFromDB() err = %v", err)
	}
	if got != want {
		t.Errorf("loadKeyFromDB() = %q, want the most recently created row's key %q", got, want)
	}
}

func TestLoadKeyFromDB_NoRowForProviderIsNotFound(t *testing.T) {
	path, writer := newTestDB(t, []credentialRow{
		{id: "1", integrationID: "anthropic", value: keyValue("sk-not-opencode-go"), active: 1, timeCreated: 100},
	})
	t.Cleanup(func() { _ = writer.Close() })

	if _, err := loadKeyFromDB(testContext(t), path); !errors.Is(err, credstore.ErrNotFound) {
		t.Errorf("loadKeyFromDB() err = %v, want credstore.ErrNotFound", err)
	}
}

func TestLoadKeyFromDB_EmptyKeyIsNotFound(t *testing.T) {
	path, writer := newTestDB(t, []credentialRow{
		{id: "1", integrationID: storeEntryKey, value: keyValue(""), active: 1, timeCreated: 100},
	})
	t.Cleanup(func() { _ = writer.Close() })

	if _, err := loadKeyFromDB(testContext(t), path); !errors.Is(err, credstore.ErrNotFound) {
		t.Errorf("loadKeyFromDB() err = %v, want credstore.ErrNotFound", err)
	}
}

func TestLoadKeyFromDB_WhitespaceOnlyKeyIsTrimmedThenNotFound(t *testing.T) {
	path, writer := newTestDB(t, []credentialRow{
		{id: "1", integrationID: storeEntryKey, value: keyValue("   \n\t  "), active: 1, timeCreated: 100},
	})
	t.Cleanup(func() { _ = writer.Close() })

	if _, err := loadKeyFromDB(testContext(t), path); !errors.Is(err, credstore.ErrNotFound) {
		t.Errorf("loadKeyFromDB() err = %v, want credstore.ErrNotFound", err)
	}
}

func TestLoadKeyFromDB_TrailingWhitespaceInKeyIsTrimmed(t *testing.T) {
	const want = "sk-opencode-go-newline-key-0123456789012345678901234"
	path, writer := newTestDB(t, []credentialRow{
		{id: "1", integrationID: storeEntryKey, value: keyValue(want + "\n"), active: 1, timeCreated: 100},
	})
	t.Cleanup(func() { _ = writer.Close() })

	got, err := loadKeyFromDB(testContext(t), path)
	if err != nil {
		t.Fatalf("loadKeyFromDB() err = %v", err)
	}
	if got != want {
		t.Errorf("loadKeyFromDB() = %q, want %q", got, want)
	}
}

func TestLoadKeyFromDB_MissingCredentialTableIsNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE unrelated (a TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	if _, err := loadKeyFromDB(testContext(t), path); !errors.Is(err, credstore.ErrNotFound) {
		t.Errorf("loadKeyFromDB() err = %v, want credstore.ErrNotFound", err)
	}
}

func TestLoadKeyFromDB_MissingFileIsNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.db")
	if _, err := loadKeyFromDB(testContext(t), path); !errors.Is(err, credstore.ErrNotFound) {
		t.Errorf("loadKeyFromDB() err = %v, want credstore.ErrNotFound", err)
	}
}

func TestLoadKeyFromDB_EmptyPathIsNotFound(t *testing.T) {
	if _, err := loadKeyFromDB(testContext(t), ""); !errors.Is(err, credstore.ErrNotFound) {
		t.Errorf("loadKeyFromDB(\"\") err = %v, want credstore.ErrNotFound", err)
	}
}

func TestLoadKeyFromDB_CorruptFileIsDescriptiveError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.db")
	if err := os.WriteFile(path, []byte("this is not a SQLite database file"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	_, err := loadKeyFromDB(testContext(t), path)
	if err == nil {
		t.Fatal("loadKeyFromDB() err = nil, want a descriptive error for a corrupt file")
	}
	if errors.Is(err, credstore.ErrNotFound) {
		t.Errorf("loadKeyFromDB() err = %v, want it NOT treated as credstore.ErrNotFound", err)
	}
}

func TestLoadKeyFromDB_MalformedValueJSONIsDescriptiveError(t *testing.T) {
	path, writer := newTestDB(t, []credentialRow{
		{id: "1", integrationID: storeEntryKey, value: "{not valid json", active: 1, timeCreated: 100},
	})
	t.Cleanup(func() { _ = writer.Close() })

	_, err := loadKeyFromDB(testContext(t), path)
	if err == nil {
		t.Fatal("loadKeyFromDB() err = nil, want a descriptive error for malformed value JSON")
	}
	if errors.Is(err, credstore.ErrNotFound) {
		t.Errorf("loadKeyFromDB() err = %v, want it NOT treated as credstore.ErrNotFound", err)
	}
}

func TestReadOnlyDSN_RejectsWrites(t *testing.T) {
	path, writer := newTestDB(t, []credentialRow{
		{id: "1", integrationID: storeEntryKey, value: keyValue("sk-opencode-go-guard-key-01234567890123456789012345"), active: 1, timeCreated: 100},
	})
	t.Cleanup(func() { _ = writer.Close() })

	db, err := sql.Open("sqlite", readOnlyDSN(path))
	if err != nil {
		t.Fatalf("open %q: %v", readOnlyDSN(path), err)
	}
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO credential (id, integration_id) VALUES ('x', 'x')`); err == nil {
		t.Error("write through a mode=ro connection succeeded, want it rejected")
	}
}

func TestReadOnlyDSN_EscapesSpecialCharacters(t *testing.T) {
	dir := t.TempDir()
	odd := filepath.Join(dir, "weird dir with spaces and a ? and a #")
	if err := os.MkdirAll(odd, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(odd, "opencode.db")

	// The writer must go through the same "file:" URI escaping readOnlyDSN
	// uses (via fileURI), just without mode=ro: a bare, unescaped path DSN
	// splits on this path's literal "?" and "#" characters (see
	// modernc.org/sqlite's newConn), which would silently create the
	// database at a truncated path instead of the one this test means to
	// seed.
	writerURI := fileURI(path)
	w, err := sql.Open("sqlite", writerURI.String())
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	if _, err := w.Exec(`CREATE TABLE credential (id TEXT, integration_id TEXT, label TEXT, value TEXT, connector_id TEXT, method_id TEXT, active INTEGER, time_created INTEGER)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	const want = "sk-opencode-go-odd-path-key-0123456789012345678901"
	if _, err := w.Exec(`INSERT INTO credential (id, integration_id, label, value, connector_id, method_id, active, time_created) VALUES ('1', ?, 'l', ?, 'c', 'm', 1, 1)`, storeEntryKey, keyValue(want)); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	got, err := loadKeyFromDB(testContext(t), path)
	if err != nil {
		t.Fatalf("loadKeyFromDB(%q) err = %v", path, err)
	}
	if got != want {
		t.Errorf("loadKeyFromDB(%q) = %q, want %q", path, got, want)
	}
}

func TestReadOnlyDSN_WindowsPathPinnedString(t *testing.T) {
	// A slash-form Windows path, as filepath.ToSlash produces on an actual
	// Windows build. filepath.ToSlash on this (Linux) test host only rewrites
	// its own separator, which is already "/", so a literal path already in
	// that form exercises the same string readOnlyDSN would build there.
	const path = "C:/Users/x y/.local/share/opencode/opencode.db"
	const want = "file:///C:/Users/x%20y/.local/share/opencode/opencode.db?_busy_timeout=3000&mode=ro"

	if got := readOnlyDSN(path); got != want {
		t.Errorf("readOnlyDSN(%q) = %q, want %q", path, got, want)
	}
}

func TestLoadKeyFromDB_CancelledContextReturnsPromptly(t *testing.T) {
	path, writer := newTestDB(t, []credentialRow{
		{id: "1", integrationID: storeEntryKey, value: keyValue("sk-opencode-go-guard-key-01234567890123456789012345"), active: 1, timeCreated: 100},
	})
	t.Cleanup(func() { _ = writer.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := loadKeyFromDB(ctx, path); !errors.Is(err, context.Canceled) {
		t.Errorf("loadKeyFromDB(cancelled ctx) err = %v, want it to match context.Canceled", err)
	}
}
