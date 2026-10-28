// Package build carries version metadata injected at build time.
package build

// These values are overridden with -ldflags "-X ..." by the Makefile and
// GoReleaser. Keep the defaults usable for `go run .` and `go test`.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
