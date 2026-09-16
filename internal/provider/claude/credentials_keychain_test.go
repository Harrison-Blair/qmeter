package claude

// Tests for the macOS Keychain credential source. They drive the Keychain
// path through subprocess.Fake and an injected GOOS, so they run on every OS
// and there are deliberately no build tags here.
//
// The plan named this file credentials_darwin_test.go. That name cannot work:
// a _darwin suffix is itself a GOOS build constraint, ANDed with any
// //go:build line, so Go would compile this file only on macOS and the very
// CI that has to exercise the Keychain path would skip it (`go list` reports
// it under IgnoredGoFiles on Linux). The name below keeps the tests running
// everywhere, which is what this unit's acceptance criteria actually ask for.
//
// The real `/usr/bin/security` shell-out is still untested on real hardware
// until someone runs qmeter on a Mac; see credentials.go.

import (
	"errors"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Harrison-Blair/qmeter/internal/lib/credstore"
	"github.com/Harrison-Blair/qmeter/internal/lib/subprocess"
	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// keychainJSON is a credential store as the Keychain would hand it back: the
// same shape as the file fixture but a different token and plan, so a test
// can tell which source won. Its expiresAt matches
// testdata/credentials_valid.json, so testNow leaves it usable.
const keychainJSON = `{
  "claudeAiOauth": {
    "accessToken": "sk-ant-oat01-keychain-token",
    "refreshToken": "sk-ant-ort01-keychain-refresh",
    "expiresAt": 1757900000000,
    "subscriptionType": "pro"
  }
}
`

// wantCall is the one command the Keychain source may run, spelled out in
// full: an absolute binary path and the exact arguments. Asserting against it
// means a typo cannot pass by accidentally missing the Fake's script.
var wantCall = subprocess.Call{
	Name: "/usr/bin/security",
	Args: []string{"find-generic-password", "-s", "Claude Code-credentials", "-w"},
}

// scriptKeychain scripts the Fake's answer for exactly that command.
func scriptKeychain(f *subprocess.Fake, stdout []byte, err error) {
	f.Script(stdout, err, wantCall.Name, wantCall.Args...)
}

// assertOneKeychainCall checks the Fake saw the Keychain lookup once, spelled
// exactly right.
func assertOneKeychainCall(t *testing.T, f *subprocess.Fake) {
	t.Helper()
	want := []subprocess.Call{wantCall}
	if got := f.Calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("Calls() = %#v, want %#v", got, want)
	}
}

func TestCredentials_DarwinPrefersKeychain(t *testing.T) {
	f := subprocess.NewFake()
	scriptKeychain(f, []byte(keychainJSON), nil)
	// The credential file is present and usable: the Keychain must still win.
	p := newProvider(t, withKeychain(f, "darwin"))

	cred, src, err := p.resolve(testContext(t))
	if err != nil {
		t.Fatalf("resolve() error = %v, want nil", err)
	}
	if src != credstore.SourceStore {
		t.Errorf("source = %q, want %q", src, credstore.SourceStore)
	}
	if cred.AccessToken != "sk-ant-oat01-keychain-token" {
		t.Errorf("AccessToken = %q, want the Keychain token", cred.AccessToken)
	}
	if cred.Plan != "pro" {
		t.Errorf("Plan = %q, want %q (the Keychain store's subscriptionType)", cred.Plan, "pro")
	}
	assertOneKeychainCall(t, f)
}

func TestCredentials_DarwinFallsBackToFileOnKeychainError(t *testing.T) {
	f := subprocess.NewFake()
	// Scripted explicitly, never left unscripted: an ErrUnscripted miss would
	// make this test pass even if the command were spelled wrong.
	scriptKeychain(f, nil, errors.New("exit status 44"))
	p := newProvider(t, withKeychain(f, "darwin"))

	cred, src, err := p.resolve(testContext(t))
	if err != nil {
		t.Fatalf("resolve() error = %v, want nil", err)
	}
	if src != credstore.SourceStore {
		t.Errorf("source = %q, want %q", src, credstore.SourceStore)
	}
	if cred.AccessToken != "sk-ant-oat01-store-token" {
		t.Errorf("AccessToken = %q, want the file's token", cred.AccessToken)
	}
	if cred.Plan != "max" {
		t.Errorf("Plan = %q, want %q (the file's subscriptionType)", cred.Plan, "max")
	}
	assertOneKeychainCall(t, f)
}

func TestCredentials_NonDarwinSkipsKeychain(t *testing.T) {
	f := subprocess.NewFake()
	// Scripted with a usable store: if the lookup ran anyway, the Keychain
	// token would win and the test would fail loudly.
	scriptKeychain(f, []byte(keychainJSON), nil)
	p := newProvider(t, withKeychain(f, "linux"))

	cred, _, err := p.resolve(testContext(t))
	if err != nil {
		t.Fatalf("resolve() error = %v, want nil", err)
	}
	if cred.AccessToken != "sk-ant-oat01-store-token" {
		t.Errorf("AccessToken = %q, want the file's token", cred.AccessToken)
	}
	if got := f.Calls(); len(got) != 0 {
		t.Errorf("Calls() = %#v, want no subprocess at all off macOS", got)
	}
}

func TestCredentials_KeychainHitWithUnparsableJSONDoesNotFallBackToFile(t *testing.T) {
	f := subprocess.NewFake()
	scriptKeychain(f, []byte("{not json"), nil)
	// The file beside it is perfectly good: a Keychain HIT that cannot be
	// parsed is still a hit, so it must error rather than quietly reading a
	// possibly staler file.
	p := newProvider(t, withKeychain(f, "darwin"))

	cred, _, err := p.resolve(testContext(t))
	if err == nil {
		t.Fatalf("resolve() = %+v, want a parse error", cred)
	}
	if errors.Is(err, provider.ErrNotLoggedIn{}) {
		t.Fatalf("resolve() error = %v, want a parse error, not ErrNotLoggedIn", err)
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "credential store: ") {
		t.Errorf("Error() = %q, want the credstore %q prefix", msg, "credential store: ")
	}
	if !strings.Contains(msg, "keychain") {
		t.Errorf("Error() = %q, want it to name the keychain as the source", msg)
	}
}

func TestCredentials_KeychainDeniedWithNoFileSurfacesKeychainError(t *testing.T) {
	f := subprocess.NewFake()
	// What a denied Keychain prompt looks like: the item may well be there,
	// the user just did not let us read it. Telling them they are not logged
	// in would send them to re-run `claude` for no reason.
	scriptKeychain(f, nil, errors.New("exit status 51: User interaction is not allowed"))
	p := newProvider(t,
		withKeychain(f, "darwin"),
		WithCredentialPath(filepath.Join(t.TempDir(), ".credentials.json")),
	)

	_, _, err := p.resolve(testContext(t))
	if err == nil {
		t.Fatal("resolve() error = nil, want the Keychain lookup error")
	}
	if errors.Is(err, provider.ErrNotLoggedIn{}) {
		t.Fatalf("resolve() error = %v, want the Keychain error, not ErrNotLoggedIn", err)
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "credential store: ") {
		t.Errorf("Error() = %q, want the credstore %q prefix", msg, "credential store: ")
	}
	if !strings.Contains(msg, "exit status 51") {
		t.Errorf("Error() = %q, want it to keep the security(1) error", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "keychain") {
		t.Errorf("Error() = %q, want it to say the Keychain lookup failed", msg)
	}
}

func TestCredentials_KeychainItemNotFoundWithNoFileIsNotLoggedIn(t *testing.T) {
	f := subprocess.NewFake()
	// Exit status 44 is security(1) for "item could not be found": nothing
	// was denied, there is simply no credential anywhere, which is exactly
	// what "not logged in" means.
	scriptKeychain(f, nil, errors.New("exit status 44"))
	p := newProvider(t,
		withKeychain(f, "darwin"),
		WithCredentialPath(filepath.Join(t.TempDir(), ".credentials.json")),
	)

	_, _, err := p.resolve(testContext(t))
	if !errors.Is(err, provider.ErrNotLoggedIn{}) {
		t.Fatalf("resolve() error = %v, want provider.ErrNotLoggedIn", err)
	}
	if got := err.Error(); got != "not logged in, run claude to log in" {
		t.Errorf("Error() = %q, want %q", got, "not logged in, run claude to log in")
	}
}

func TestCredentials_KeychainEmptyStdoutUsesFile(t *testing.T) {
	f := subprocess.NewFake()
	// security(1) exiting 0 with nothing on stdout is a miss, not a store
	// holding an empty credential.
	scriptKeychain(f, []byte("\n"), nil)
	p := newProvider(t, withKeychain(f, "darwin"))

	cred, _, err := p.resolve(testContext(t))
	if err != nil {
		t.Fatalf("resolve() error = %v, want nil", err)
	}
	if cred.AccessToken != "sk-ant-oat01-store-token" {
		t.Errorf("AccessToken = %q, want the file's token", cred.AccessToken)
	}
	assertOneKeychainCall(t, f)
}

func TestCredentials_KeychainEmptyStdoutWithNoFileIsNotLoggedIn(t *testing.T) {
	f := subprocess.NewFake()
	scriptKeychain(f, nil, nil)
	p := newProvider(t,
		withKeychain(f, "darwin"),
		WithCredentialPath(filepath.Join(t.TempDir(), ".credentials.json")),
	)

	_, _, err := p.resolve(testContext(t))
	if !errors.Is(err, provider.ErrNotLoggedIn{}) {
		t.Fatalf("resolve() error = %v, want provider.ErrNotLoggedIn", err)
	}
}

func TestWithKeychainRunner_GatedOnRuntimeGOOS(t *testing.T) {
	// The exported option takes the real runtime.GOOS, so what it does here
	// depends on the host: on macOS it looks in the Keychain first, anywhere
	// else it must not spawn anything at all.
	f := subprocess.NewFake()
	scriptKeychain(f, []byte(keychainJSON), nil)
	p := newProvider(t, WithKeychainRunner(f))

	cred, _, err := p.resolve(testContext(t))
	if err != nil {
		t.Fatalf("resolve() error = %v, want nil", err)
	}
	if runtime.GOOS == "darwin" {
		if cred.AccessToken != "sk-ant-oat01-keychain-token" {
			t.Errorf("AccessToken = %q, want the Keychain token on darwin", cred.AccessToken)
		}
		assertOneKeychainCall(t, f)
		return
	}
	if cred.AccessToken != "sk-ant-oat01-store-token" {
		t.Errorf("AccessToken = %q, want the file's token off darwin", cred.AccessToken)
	}
	if got := f.Calls(); len(got) != 0 {
		t.Errorf("Calls() = %#v, want no subprocess on %s", got, runtime.GOOS)
	}
}

func TestWithKeychainRunner_NilRunnerFallsBackToFile(t *testing.T) {
	p := newProvider(t, WithKeychainRunner(nil))

	cred, _, err := p.resolve(testContext(t))
	if err != nil {
		t.Fatalf("resolve() error = %v, want nil", err)
	}
	if cred.AccessToken != "sk-ant-oat01-store-token" {
		t.Errorf("AccessToken = %q, want the file's token", cred.AccessToken)
	}
}
