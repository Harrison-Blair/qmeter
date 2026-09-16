// Package version reports the qmeter build version.
package version

import "runtime/debug"

// Version is set at build time via
// -ldflags "-X github.com/Harrison-Blair/qmeter/internal/version.Version=v1.2.3".
// When unset, String falls back to the module version recorded by `go install`.
var Version = ""

// String returns the best available version string.
func String() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
