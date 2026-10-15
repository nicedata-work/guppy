// Package version carries the guppy version string.
//
// Release builds stamp it via -ldflags "-X
// github.com/nicedata-work/guppy/internal/version.Version=<v>";
// development builds (go run/bazel run) report "dev".
package version

// Version is the guppy version. Overridden at build time in releases.
var Version = "dev"
