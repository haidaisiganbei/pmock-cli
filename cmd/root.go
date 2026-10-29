// Package cmd wires the generated OpenAPI catalog into a Cobra command tree.
package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/pmock/pmock-cli/internal/build"
	"github.com/pmock/pmock-cli/internal/config"
	"github.com/pmock/pmock-cli/internal/exitcode"
	"github.com/pmock/pmock-cli/internal/spec"
)

// globals are the persistent flags shared by every command.
type globals struct {
	format             string
	compact            bool
	baseURL            string
	accessKey          string
	secretKey          string
	timeout            time.Duration
	dryRun             bool
	yes                bool
	allowBusinessError bool
	verbose            bool
}

func (g *globals) config() (*config.Config, error) {
	cfg, err := config.LoadForDisplay()
	if err != nil {
		return nil, exitcode.Usagef("%v", err)
	}
	if g.baseURL != "" {
		cfg.BaseURL = strings.TrimRight(g.baseURL, "/")
	}
	if g.accessKey != "" {
		cfg.AccessKey = g.accessKey
	}
	if g.secretKey != "" {
		cfg.SecretKey = g.secretKey
	}
	if g.timeout > 0 {
		cfg.Timeout = g.timeout
	}
	if err := cfg.Validate(); err != nil {
		return nil, exitcode.Usagef("%v", err)
	}
	return cfg, nil
}

var groupShort = map[string]string{
	"phone":        "Cloud phone assets, lifecycle, files and screen operations",
	"phone action": "UI automation actions (click, swipe, input, ...)",
	"phone file":   "Files inside a cloud phone",
	"app":          "Applications installed on cloud phones",
	"network":      "Proxies and network detection",
	"rpa":          "RPA templates and tasks",
	"rpa template": "RPA flow templates",
	"rpa task":     "RPA scheduled tasks",
	"product":      "Products, SKUs and pricing",
	"brand":        "Phone brands",
	"team":         "Team members and authorization",
	"task":         "Background task records",
}

// Execute builds the command tree and returns the process exit code.
func Execute(assets fs.FS) int {
	doc, err := spec.LoadFS(assets, "openapi/pmock-openapi.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitcode.Runtime
	}
	overlay, err := spec.LoadOverlayFS(assets, "openapi/overlay.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitcode.Runtime
	}
	catalog, err := spec.BuildCatalog(doc, overlay)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitcode.Runtime
	}

	root := newRoot(catalog)
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)

	err = root.Execute()
	if err == nil {
		return exitcode.OK
	}

	code := exitcode.CodeOf(err)
	var coded *exitcode.Error
	if !errors.As(err, &coded) {
		// Cobra's own errors (unknown command, bad flag) are caller mistakes.
		code = exitcode.Usage
	}
	if !exitcode.IsQuiet(err) {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
	return code
}

func newRoot(catalog *spec.Catalog) *cobra.Command {
	return newRootWith(catalog, true)
}

// newRootWith builds the command tree. includeMCP is false while the MCP
// server executes a tool, otherwise `pmock-cli mcp` would be registered inside
// every tool invocation.
func newRootWith(catalog *spec.Catalog, includeMCP bool) *cobra.Command {
	g := &globals{}

	root := &cobra.Command{
		Use:   "pmock-cli",
		Short: "PMOCK open API command line client",
		Long: `pmock-cli talks to the PMOCK open API with a team access key.

It is standalone: no PMOCK desktop client, no local session, no Electron.
Credentials are read from ~/.pmock-cli/config.json or from PMOCK_ACCESS_KEY /
PMOCK_SECRET_KEY.

EXIT CODES
  0  success
  1  business failure (API envelope code != 0)
  2  usage or configuration error (nothing sent)
  3  runtime failure (network, timeout, 5xx, bad credentials)

EXAMPLES
  pmock-cli config init
  pmock-cli auth status
  pmock-cli phone list --page-size 10
  pmock-cli phone start --id 123
  pmock-cli phone action click --code ABC123 --pos-type 2 --pos 100,200

Use "pmock-cli docs <command>" for the full flag table of any command.`,
		Version:       build.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			switch g.format {
			case "json", "pretty", "table":
				return nil
			default:
				return exitcode.Usagef("invalid --format %q (want json, pretty or table)", g.format)
			}
		},
	}
	root.SetVersionTemplate("pmock-cli version {{.Version}}\n")

	pf := root.PersistentFlags()
	pf.StringVar(&g.format, "format", "json", "output format: json | pretty | table")
	pf.BoolVar(&g.compact, "compact", false, "emit compact JSON instead of indented JSON")
	pf.StringVar(&g.baseURL, "base-url", "", "API base URL (default "+config.DefaultBaseURL+")")
	pf.StringVar(&g.accessKey, "access-key", "", "access key (overrides config and env; prefer env in CI)")
	pf.StringVar(&g.secretKey, "secret-key", "", "secret key (overrides config and env; prefer env in CI)")
	pf.DurationVar(&g.timeout, "timeout", 0, "request timeout (default 30s)")
	pf.BoolVar(&g.dryRun, "dry-run", false, "print the request without sending it")
	pf.BoolVar(&g.yes, "yes", false, "confirm destructive operations and silence costly warnings")
	pf.BoolVar(&g.allowBusinessError, "allow-business-error", false, "exit 0 even when the API returns a non-zero code")
	pf.BoolVar(&g.verbose, "verbose", false, "log request lines and trace ids to stderr")

	root.AddCommand(newConfigCmd(g))
	root.AddCommand(newAuthCmd(g))
	root.AddCommand(newDoctorCmd(g))
	root.AddCommand(newDocsCmd(catalog))
	root.AddCommand(newCommandsCmd(catalog))
	root.AddCommand(newVersionCmd())

	addCatalog(root, catalog, g)
	addExtraCommands(root, g)
	if includeMCP {
		root.AddCommand(newMCPCmd(catalog))
	}
	return root
}

// addExtraCommands registers hand-written commands that need more than one
// API call. They live next to the generated tree and share the same flags,
// output and exit contract.
func addExtraCommands(root *cobra.Command, g *globals) {
	phoneFile := ensureGroup(root, []string{"phone", "file"})
	phoneFile.AddCommand(newPhoneFileUploadCmd(g))
}

func addCatalog(root *cobra.Command, catalog *spec.Catalog, g *globals) {
	// Sort by path depth then name so groups are created before their leaves
	// and help output is stable.
	cmds := append([]*spec.Command{}, catalog.Commands...)
	sort.Slice(cmds, func(i, j int) bool {
		if len(cmds[i].PathParts) != len(cmds[j].PathParts) {
			return len(cmds[i].PathParts) < len(cmds[j].PathParts)
		}
		return cmds[i].FullPath() < cmds[j].FullPath()
	})

	for _, sc := range cmds {
		parent := ensureGroup(root, sc.PathParts[:len(sc.PathParts)-1])
		parent.AddCommand(newOperationCmd(sc, g))
	}
}

func ensureGroup(root *cobra.Command, parts []string) *cobra.Command {
	cur := root
	for i, part := range parts {
		var next *cobra.Command
		for _, c := range cur.Commands() {
			if c.Name() == part {
				next = c
				break
			}
		}
		if next == nil {
			key := strings.Join(parts[:i+1], " ")
			short := groupShort[key]
			if short == "" {
				short = part + " commands"
			}
			next = &cobra.Command{
				Use:   part,
				Short: short,
				RunE: func(cmd *cobra.Command, args []string) error {
					return cmd.Help()
				},
			}
			cur.AddCommand(next)
		}
		cur = next
	}
	return cur
}
