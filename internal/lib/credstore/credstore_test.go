package credstore_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/lib/credstore"
	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// cred is a stand-in for a provider's credential type: something richer than a
// bare string, so the generic parameter is exercised the way a real provider
// (token plus a plan name only the local store knows) would use it.
type cred struct {
	Token string
	Plan  string
}

func TestResolve_EnvVarTakesPrecedence(t *testing.T) {
	t.Setenv("QMETER_TEST_TOKEN", "env-token")

	loaderCalled := false
	load := func(context.Context) (cred, error) {
		loaderCalled = true
		return cred{Token: "store-token", Plan: "max"}, nil
	}
	fromEnv := func(v string) (cred, error) {
		return cred{Token: v}, nil
	}

	got, src, err := credstore.Resolve(context.Background(), "QMETER_TEST_TOKEN", "claude", load, fromEnv)
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if loaderCalled {
		t.Error("Resolve() called the store loader although the env override was set")
	}
	if want := (cred{Token: "env-token"}); got != want {
		t.Errorf("Resolve() credential = %+v, want %+v", got, want)
	}
	if src != credstore.SourceEnv {
		t.Errorf("Resolve() source = %q, want %q", src, credstore.SourceEnv)
	}
}

func TestResolve_FallsBackToLoader(t *testing.T) {
	// The env var is deliberately not set for this test.
	fromEnvCalled := false
	load := func(context.Context) (cred, error) {
		return cred{Token: "store-token", Plan: "max"}, nil
	}
	fromEnv := func(string) (cred, error) {
		fromEnvCalled = true
		return cred{}, nil
	}

	got, src, err := credstore.Resolve(context.Background(), "QMETER_TEST_TOKEN_UNSET", "claude", load, fromEnv)
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if fromEnvCalled {
		t.Error("Resolve() called fromEnv although the env override was unset")
	}
	if want := (cred{Token: "store-token", Plan: "max"}); got != want {
		t.Errorf("Resolve() credential = %+v, want %+v", got, want)
	}
	if src != credstore.SourceStore {
		t.Errorf("Resolve() source = %q, want %q", src, credstore.SourceStore)
	}
}

func TestResolve_LoaderNotFoundReturnsHintedError(t *testing.T) {
	// A real *fs.PathError, exactly what a provider's file loader returns when
	// the vendor store does not exist.
	_, missingFileErr := os.Open(filepath.Join(t.TempDir(), "auth.json"))
	if missingFileErr == nil {
		t.Fatal("os.Open(nonexistent) returned no error")
	}

	tests := []struct {
		name    string
		loadErr error
	}{
		{name: "credstore sentinel", loadErr: credstore.ErrNotFound},
		{name: "wrapped credstore sentinel", loadErr: fmt.Errorf("open auth.json: %w", credstore.ErrNotFound)},
		{name: "os.ErrNotExist", loadErr: os.ErrNotExist},
		{name: "os.Open path error", loadErr: missingFileErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			load := func(context.Context) (cred, error) { return cred{}, tt.loadErr }
			fromEnv := func(v string) (cred, error) { return cred{Token: v}, nil }

			got, src, err := credstore.Resolve(context.Background(), "QMETER_TEST_TOKEN_UNSET", "claude", load, fromEnv)
			if !errors.Is(err, provider.ErrNotLoggedIn{}) {
				t.Fatalf("Resolve() error = %v, want provider.ErrNotLoggedIn", err)
			}
			var notLoggedIn provider.ErrNotLoggedIn
			if !errors.As(err, &notLoggedIn) {
				t.Fatalf("errors.As(%v, *provider.ErrNotLoggedIn) = false, want true", err)
			}
			if notLoggedIn.Tool != "claude" {
				t.Errorf("ErrNotLoggedIn.Tool = %q, want %q", notLoggedIn.Tool, "claude")
			}
			if want := "not logged in, run claude to log in"; err.Error() != want {
				t.Errorf("Resolve() error text = %q, want %q", err.Error(), want)
			}
			if got != (cred{}) {
				t.Errorf("Resolve() credential = %+v, want the zero value", got)
			}
			if src != credstore.SourceNone {
				t.Errorf("Resolve() source = %q, want %q", src, credstore.SourceNone)
			}
		})
	}
}

func TestResolve_EmptyEnvVarTreatedAsUnset(t *testing.T) {
	t.Setenv("QMETER_TEST_TOKEN", "")

	loaderCalled := false
	load := func(context.Context) (cred, error) {
		loaderCalled = true
		return cred{Token: "store-token"}, nil
	}
	fromEnv := func(v string) (cred, error) {
		t.Errorf("fromEnv(%q) called although the env override is empty", v)
		return cred{}, nil
	}

	got, src, err := credstore.Resolve(context.Background(), "QMETER_TEST_TOKEN", "claude", load, fromEnv)
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if !loaderCalled {
		t.Error("Resolve() did not fall back to the store loader for an empty env override")
	}
	if want := (cred{Token: "store-token"}); got != want {
		t.Errorf("Resolve() credential = %+v, want %+v", got, want)
	}
	if src != credstore.SourceStore {
		t.Errorf("Resolve() source = %q, want %q", src, credstore.SourceStore)
	}
}

func TestResolve_LoaderErrorPassesThrough(t *testing.T) {
	errMalformed := errors.New("unexpected end of JSON input")

	tests := []struct {
		name    string
		loadErr error
		// want is an error the result must match with errors.Is.
		want error
	}{
		{name: "plain error", loadErr: errMalformed, want: errMalformed},
		{
			name:    "typed token-expired stays typed",
			loadErr: provider.ErrTokenExpired{Tool: "codex"},
			want:    provider.ErrTokenExpired{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			load := func(context.Context) (cred, error) { return cred{}, tt.loadErr }
			fromEnv := func(v string) (cred, error) { return cred{Token: v}, nil }

			got, src, err := credstore.Resolve(context.Background(), "QMETER_TEST_TOKEN_UNSET", "codex", load, fromEnv)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Resolve() error = %v, want it to match %v", err, tt.want)
			}
			if errors.Is(err, provider.ErrNotLoggedIn{}) {
				t.Errorf("Resolve() error = %v, want it NOT converted to provider.ErrNotLoggedIn", err)
			}
			if !strings.Contains(err.Error(), tt.loadErr.Error()) {
				t.Errorf("Resolve() error text = %q, want it to contain the loader's text %q", err.Error(), tt.loadErr.Error())
			}
			// Detect reasons are rendered without a provider prefix, so the
			// wrapping this package adds must never name the provider.
			if strings.Contains(err.Error(), "codex:") {
				t.Errorf("Resolve() error text = %q, want no provider-name prefix", err.Error())
			}
			if got != (cred{}) {
				t.Errorf("Resolve() credential = %+v, want the zero value", got)
			}
			if src != credstore.SourceNone {
				t.Errorf("Resolve() source = %q, want %q", src, credstore.SourceNone)
			}
		})
	}
}

func TestResolve_TypedLoaderErrorKeepsItsFields(t *testing.T) {
	load := func(context.Context) (cred, error) { return cred{}, provider.ErrTokenExpired{Tool: "cursor-agent"} }
	fromEnv := func(v string) (cred, error) { return cred{Token: v}, nil }

	_, _, err := credstore.Resolve(context.Background(), "QMETER_TEST_TOKEN_UNSET", "cursor-agent", load, fromEnv)
	var expired provider.ErrTokenExpired
	if !errors.As(err, &expired) {
		t.Fatalf("errors.As(%v, *provider.ErrTokenExpired) = false, want true", err)
	}
	if expired.Tool != "cursor-agent" {
		t.Errorf("ErrTokenExpired.Tool = %q, want %q", expired.Tool, "cursor-agent")
	}
	if want := "token expired, open cursor-agent to refresh"; expired.Error() != want {
		t.Errorf("extracted error text = %q, want %q", expired.Error(), want)
	}
}

func TestResolve_EnvConversionErrorPassesThrough(t *testing.T) {
	t.Setenv("QMETER_TEST_TOKEN", "not-a-jwt")
	errNotJWT := errors.New("override is not a JWT")

	load := func(context.Context) (cred, error) {
		t.Error("Resolve() called the store loader although the env override was set")
		return cred{}, nil
	}
	fromEnv := func(string) (cred, error) { return cred{}, errNotJWT }

	got, src, err := credstore.Resolve(context.Background(), "QMETER_TEST_TOKEN", "cursor-agent", load, fromEnv)
	if !errors.Is(err, errNotJWT) {
		t.Fatalf("Resolve() error = %v, want it to match %v", err, errNotJWT)
	}
	// A supplied-but-unusable override is not "not logged in": the hint would
	// send the user to a login they already completed.
	if errors.Is(err, provider.ErrNotLoggedIn{}) {
		t.Errorf("Resolve() error = %v, want it NOT converted to provider.ErrNotLoggedIn", err)
	}
	if !strings.Contains(err.Error(), "QMETER_TEST_TOKEN") {
		t.Errorf("Resolve() error text = %q, want it to name the env var", err.Error())
	}
	if got != (cred{}) {
		t.Errorf("Resolve() credential = %+v, want the zero value", got)
	}
	if src != credstore.SourceNone {
		t.Errorf("Resolve() source = %q, want %q", src, credstore.SourceNone)
	}
}

func TestResolve_EnvNotFoundErrorIsNotConvertedToHintedError(t *testing.T) {
	t.Setenv("QMETER_TEST_TOKEN", "some-token")

	load := func(context.Context) (cred, error) { return cred{}, nil }
	fromEnv := func(string) (cred, error) { return cred{}, credstore.ErrNotFound }

	_, _, err := credstore.Resolve(context.Background(), "QMETER_TEST_TOKEN", "claude", load, fromEnv)
	if !errors.Is(err, credstore.ErrNotFound) {
		t.Fatalf("Resolve() error = %v, want it to match credstore.ErrNotFound", err)
	}
	if errors.Is(err, provider.ErrNotLoggedIn{}) {
		t.Errorf("Resolve() error = %v, want it NOT converted to provider.ErrNotLoggedIn", err)
	}
}

func TestResolve_PassesContextToLoader(t *testing.T) {
	type ctxKey struct{}
	want := "sentinel"
	ctx := context.WithValue(context.Background(), ctxKey{}, want)

	var seen any
	load := func(c context.Context) (cred, error) {
		seen = c.Value(ctxKey{})
		return cred{Token: "store-token"}, nil
	}
	fromEnv := func(v string) (cred, error) { return cred{Token: v}, nil }

	if _, _, err := credstore.Resolve(ctx, "QMETER_TEST_TOKEN_UNSET", "claude", load, fromEnv); err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if seen != want {
		t.Errorf("loader saw context value %v, want %q", seen, want)
	}
}

func TestResolve_WorksForAStringCredential(t *testing.T) {
	// The generic parameter must accept a bare token too, which is all the
	// OpenCode Go provider needs.
	t.Setenv("QMETER_TEST_TOKEN", "key-123")

	got, src, err := credstore.Resolve(
		context.Background(),
		"QMETER_TEST_TOKEN",
		"opencode",
		func(context.Context) (string, error) { return "", credstore.ErrNotFound },
		func(v string) (string, error) { return v, nil },
	)
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if got != "key-123" {
		t.Errorf("Resolve() credential = %q, want %q", got, "key-123")
	}
	if src != credstore.SourceEnv {
		t.Errorf("Resolve() source = %q, want %q", src, credstore.SourceEnv)
	}
}

// ExampleResolve shows the shape a provider package uses: its own credential
// type, its own store loader, and the env override applied for it.
func ExampleResolve() {
	type claudeCred struct {
		AccessToken string
		Plan        string // only the local store knows this
	}

	// Stands in for reading ~/.claude/.credentials.json; a real loader returns
	// credstore.ErrNotFound when the file is missing.
	loadFile := func(context.Context) (claudeCred, error) {
		return claudeCred{AccessToken: "from-store", Plan: "max"}, nil
	}
	fromEnv := func(v string) (claudeCred, error) {
		return claudeCred{AccessToken: v}, nil // no plan name to be had
	}

	// The real Claude provider passes "QMETER_CLAUDE_TOKEN"; the example uses a
	// name guaranteed to be unset so it always demonstrates the store path.
	const envVar = "QMETER_CLAUDE_TOKEN_EXAMPLE"

	cred, src, err := credstore.Resolve(context.Background(), envVar, "claude", loadFile, fromEnv)
	fmt.Println(cred.AccessToken, cred.Plan, src, err)
	// Output: from-store max store <nil>
}
