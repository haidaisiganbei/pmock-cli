// Package build carries version metadata injected at build time.
package build

import (
	"runtime/debug"
	"strings"
)

// These values are overridden with -ldflags "-X ..." by the Makefile and
// GoReleaser. Keep the defaults usable for `go run .` and `go test`.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// init recovers the module version for `go install module@version`, where
// ldflags are not available. Release builds always provide Version explicitly.
func init() {
	if Version != "dev" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if v := strings.TrimSpace(info.Main.Version); v != "" && v != "(devel)" {
		Version = strings.TrimPrefix(v, "v")
	}
}
