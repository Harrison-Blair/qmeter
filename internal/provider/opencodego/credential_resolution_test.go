package opencodego

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/lib/credstore"
	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// missingDB returns a path inside a fresh temp dir where no file exists, so
// a test never reads the developer's real
// ~/.local/share/opencode/opencode.db.
func missingDB(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "opencode.db")
}

// seedDB creates a minimal opencode.db at a fresh temp path holding one
// active "opencode-go" row with the given key, and returns the path. The
// writer connection is closed before returning, so the row is checkpointed
// into the main file -- these tests care about resolution order, not WAL
// mechanics (db_test.go already covers those).
func seedDB(t *testing.T, key string) string {
	t.Helper()
	path, writer := newTestDB(t, []credentialRow{
		{id: "1", integrationID: storeEntryKey, value: keyValue(key), active: 1, timeCreated: 100},
	})
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return path
}

func TestLoadCredential_DBMissingFallsBackToAuthJSON(t *testing.T) {
	got, err := loadCredential(testContext(t), missingDB(t), filepath.Join("testdata", "auth_go.json"))
	if err != nil {
		t.Fatalf("loadCredential() err = %v", err)
	}
	if want := "sk-opencode-go-fixture-key"; got != want {
		t.Errorf("loadCredential() = %q, want %q", got, want)
	}
}

func TestLoadCredential_DBPreferredWhenBothPresent(t *testing.T) {
	const want = "sk-opencode-go-db-wins-key-0123456789012345678901234"
	dbPath := seedDB(t, want)

	got, err := loadCredential(testContext(t), dbPath, filepath.Join("testdata", "auth_go.json"))
	if err != nil {
		t.Fatalf("loadCredential() err = %v", err)
	}
	if got != want {
		t.Errorf("loadCredential() = %q, want the DB's key %q, not the auth.json fixture's", got, want)
	}
}

func TestLoadCredential_NeitherPresentIsNotFound(t *testing.T) {
	// loadCredential falls through to loadKey's own contract for the
	// auth.json half of the pair, which returns os.ReadFile's raw
	// *fs.PathError for a missing file rather than credstore.ErrNotFound
	// itself; credstore.Resolve is what maps both to provider.ErrNotLoggedIn
	// (see TestCredentials_MissingFileReturnsNotLoggedInFromFetch for that
	// higher-level assertion).
	_, err := loadCredential(testContext(t), missingDB(t), missingStore(t))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("loadCredential() err = %v, want it to match os.ErrNotExist", err)
	}
}

func TestLoadCredential_DBEmptyKeyFallsBackToAuthJSON(t *testing.T) {
	dbPath := seedDB(t, "")

	got, err := loadCredential(testContext(t), dbPath, filepath.Join("testdata", "auth_go.json"))
	if err != nil {
		t.Fatalf("loadCredential() err = %v", err)
	}
	if want := "sk-opencode-go-fixture-key"; got != want {
		t.Errorf("loadCredential() = %q, want %q", got, want)
	}
}

func TestLoadCredential_CorruptDBFallsBackSilentlyWhenAuthJSONHasAKey(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "opencode.db")
	if err := os.WriteFile(dbPath, []byte("not a sqlite file"), 0o600); err != nil {
		t.Fatalf("write corrupt db: %v", err)
	}

	got, err := loadCredential(testContext(t), dbPath, filepath.Join("testdata", "auth_go.json"))
	if err != nil {
		t.Fatalf("loadCredential() err = %v, want the corrupt DB's failure to be masked by a working auth.json", err)
	}
	if want := "sk-opencode-go-fixture-key"; got != want {
		t.Errorf("loadCredential() = %q, want %q", got, want)
	}
}

func TestLoadCredential_CorruptDBSurfacesWhenAuthJSONHasNoKeyEither(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "opencode.db")
	if err := os.WriteFile(dbPath, []byte("not a sqlite file"), 0o600); err != nil {
		t.Fatalf("write corrupt db: %v", err)
	}

	_, err := loadCredential(testContext(t), dbPath, missingStore(t))
	if err == nil {
		t.Fatal("loadCredential() err = nil, want a descriptive error naming the corrupt database")
	}
	if errors.Is(err, credstore.ErrNotFound) {
		t.Errorf("loadCredential() err = %v, want it NOT reported as credstore.ErrNotFound (auth.json alone being absent is not the problem here)", err)
	}
	if !strings.Contains(err.Error(), "opencode.db") {
		t.Errorf("loadCredential() err = %q, want it to name opencode.db", err)
	}
}

// The tests below exercise loadCredential's DB-over-auth.json ordering
// through the actual Provider wiring (WithDBPath, credential(), Detect,
// Fetch) rather than by calling loadCredential directly: the tests above
// already cover loadCredential's own logic, but none of them would catch a
// bug in how Provider passes its two configured paths to it (for example,
// credential() forgetting to pass p.dbPath through and hardcoding an empty
// one instead).

func TestDetect_SucceedsFromDBAloneWithNoAuthJSON(t *testing.T) {
	// auth.json is deliberately missing: with only the DB holding a key,
	// Detect can only succeed by actually reading p.dbPath. Pointing this at
	// a present, valid auth.json fixture (as an earlier version of this test
	// did) would let Detect succeed even if credential() silently dropped
	// p.dbPath, since the fixture alone is enough to satisfy Detect.
	clearEnv(t)
	p := New(
		WithDBPath(seedDB(t, "sk-db")),
		WithCredentialPath(missingStore(t)),
	)

	ok, reason := p.Detect(context.Background())
	if !ok {
		t.Errorf("Detect() ok = false, reason %q; want true", reason)
	}
	if reason != "" {
		t.Errorf("Detect() reason = %q, want empty", reason)
	}
}

func TestFetch_DBTakesPriorityOverAuthJSON(t *testing.T) {
	clearEnv(t)
	srv, rec := fixtureServer(t, "usage_weekly_only.json")
	p := New(
		WithDBPath(seedDB(t, "sk-db")),
		WithCredentialPath(filepath.Join("testdata", "auth_go.json")),
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)

	if _, err := p.Fetch(testContext(t)); err != nil {
		t.Fatalf("Fetch() err = %v", err)
	}
	_, _, header := rec.snapshot()
	if want := "Bearer sk-db"; header.Get("Authorization") != want {
		t.Errorf("Authorization = %q, want %q (the DB's key, not the auth.json fixture's)", header.Get("Authorization"), want)
	}
}

func TestFetch_EnvOverrideBeatsDBEvenWhenAuthJSONIsAlsoPresent(t *testing.T) {
	t.Setenv("QMETER_OPENCODE_GO_KEY", "sk-from-env")
	srv, rec := fixtureServer(t, "usage_weekly_only.json")
	p := New(
		WithDBPath(seedDB(t, "sk-db")),
		WithCredentialPath(filepath.Join("testdata", "auth_go.json")),
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)

	if _, err := p.Fetch(testContext(t)); err != nil {
		t.Fatalf("Fetch() err = %v", err)
	}
	_, _, header := rec.snapshot()
	if want := "Bearer sk-from-env"; header.Get("Authorization") != want {
		t.Errorf("Authorization = %q, want %q", header.Get("Authorization"), want)
	}
}

func TestLoadCredential_CancelledContextReturnsPromptly(t *testing.T) {
	// auth.json is deliberately absent too: a present, valid auth.json would
	// mask the cancellation the same way a corrupt DB's error is masked (see
	// TestLoadCredential_CorruptDBFallsBackSilentlyWhenAuthJSONHasAKey), which
	// would make this test pass without ever actually observing ctx
	// propagate out of loadKeyFromDB.
	dbPath := seedDB(t, "sk-db")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := loadCredential(ctx, dbPath, missingStore(t))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("loadCredential(cancelled ctx) err = %v, want it to match context.Canceled", err)
	}
}

// TestFetch_AlreadyCancelledContextIsNotSwallowed exercises the same
// already-cancelled-ctx property through the public Provider API rather than
// loadCredential directly, so it also catches credential() itself silently
// substituting a fresh, uncancelled context for the one it was given (a bug
// none of the loadCredential-level tests above can see, since they call
// loadCredential themselves and so always pass the ctx they mean to test).
//
// The assertion on the error's "resolve credential: " prefix is what
// actually pins the failure to credential resolution rather than the network
// request that follows it: under that bug, DB resolution would succeed
// (Background() is never cancelled) and Fetch would proceed to call
// httpx.Get with the real, still-cancelled ctx, which fails there instead
// and comes back prefixed "fetch usage: " -- a different, easily-missed
// symptom of the same bug that the prefix check tells apart from the
// correct behaviour.
func TestFetch_AlreadyCancelledContextIsNotSwallowed(t *testing.T) {
	clearEnv(t)
	// The server exists only so WithBaseURL has somewhere valid to point;
	// correct code never reaches it, because credential resolution fails
	// first on the cancelled ctx.
	srv, rec := fixtureServer(t, "usage_weekly_only.json")
	p := New(
		WithDBPath(seedDB(t, "sk-db")),
		WithCredentialPath(missingStore(t)),
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := p.Fetch(ctx)
	if err == nil {
		t.Fatal("Fetch() err = nil, want an error for an already-cancelled context")
	}
	if !reflect.DeepEqual(got, provider.Usage{}) {
		t.Errorf("Fetch() usage = %+v, want the zero value", got)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Fetch() err = %v, want it to match context.Canceled", err)
	}
	if want := "resolve credential: "; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("Fetch() err = %q, want it to start with %q", err.Error(), want)
	}
	if method, _, _ := rec.snapshot(); method != "" {
		t.Errorf("Fetch() reached the network (method %q), want it to fail during credential resolution instead", method)
	}
}
