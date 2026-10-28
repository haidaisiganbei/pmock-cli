// pmock-cli is the standalone command line client for the PMOCK open API.
package main

import (
	"embed"
	"os"

	"github.com/pmock/pmock-cli/cmd"
)

// openapi carries the vendored spec and the product overlay. Embedding keeps
// the binary self-contained, which is the main reason for choosing Go.
//
//go:embed openapi/*.yaml
var openapiFS embed.FS

func main() {
	os.Exit(cmd.Execute(openapiFS))
}
