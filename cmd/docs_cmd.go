package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pmock/pmock-cli/internal/build"
	"github.com/pmock/pmock-cli/internal/exitcode"
	"github.com/pmock/pmock-cli/internal/spec"
)

func newDocsCmd(catalog *spec.Catalog) *cobra.Command {
	return &cobra.Command{
		Use:   "docs [command...]",
		Short: "Show flags and request fields for a command",
		Long: `Show the generated documentation for a command, e.g.

  pmock-cli docs phone start
  pmock-cli docs rpa task add

With no argument, lists the command groups. This command writes human-readable
text, unlike the JSON envelope used by API commands.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if len(args) == 0 {
				printGroups(out, catalog)
				return nil
			}
			path := strings.Join(args, " ")
			if path == "phone file upload" {
				printUploadDoc(out)
				return nil
			}
			if path == "mcp" {
				c := newMCPCmd(catalog)
				fmt.Fprintf(out, "pmock-cli mcp\n\n%s\n\nFLAGS\n%s", c.Long, c.Flags().FlagUsages())
				return nil
			}
			var match *spec.Command
			for _, c := range catalog.Commands {
				if c.FullPath() == path {
					match = c
					break
				}
			}
			if match == nil {
				children := childrenOf(catalog, path)
				if len(children) > 0 {
					fmt.Fprintf(out, "%s\n\n", path)
					for _, c := range children {
						fmt.Fprintf(out, "  %-28s %s\n", c.Usage(), c.Summary)
					}
					return nil
				}
				return exitcode.Usagef("unknown command %q; run `pmock-cli docs` to list groups", path)
			}
			printCommandDoc(out, match)
			return nil
		},
	}
}

type flagInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Field    string `json:"field"`
	In       string `json:"in"`
	Required bool   `json:"required"`
}

type commandInfo struct {
	Path        string     `json:"path"`
	Aliases     []string   `json:"aliases,omitempty"`
	Method      string     `json:"method"`
	HTTPPath    string     `json:"httpPath"`
	Summary     string     `json:"summary"`
	Risk        string     `json:"risk"`
	Hidden      bool       `json:"hidden,omitempty"`
	HasBody     bool       `json:"hasBody,omitempty"`
	JSONInput   bool       `json:"jsonInput,omitempty"`
	Async       string     `json:"async,omitempty"`
	OperateType string     `json:"operateType,omitempty"`
	Flags       []flagInfo `json:"flags"`
}

func newCommandsCmd(catalog *spec.Catalog) *cobra.Command {
	return &cobra.Command{
		Use:   "commands",
		Short: "List every command as JSON (machine-readable catalog)",
		Long: `Print the full command catalog. This is the stable input for agents,
shell completion and the future MCP server. Hand-written multi-step commands
are included next to the generated ones.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := make([]commandInfo, 0, len(catalog.Commands)+2)
			for _, c := range catalog.Commands {
				ci := commandInfo{
					Path:        c.FullPath(),
					Aliases:     c.Aliases,
					Method:      c.Method,
					HTTPPath:    c.Path,
					Summary:     c.Summary,
					Risk:        string(c.Risk),
					Hidden:      c.Internal,
					HasBody:     c.HasBody(),
					JSONInput:   c.HasJSONInput(),
					Async:       c.Async,
					OperateType: c.OperateType,
					Flags:       make([]flagInfo, 0, len(c.Flags)),
				}
				for _, f := range c.Flags {
					ci.Flags = append(ci.Flags, flagInfo{
						Name: f.Name, Type: f.Type, Field: f.Field, In: f.In, Required: f.Required,
					})
				}
				out = append(out, ci)
			}
			out = append(out, extraCommandInfos()...)
			b, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				return exitcode.Runtimef("failed to encode catalog: %v", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return nil
		},
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information as JSON",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := json.MarshalIndent(map[string]string{
				"version": build.Version,
				"commit":  build.Commit,
				"date":    build.Date,
			}, "", "  ")
			if err != nil {
				return exitcode.Runtimef("%v", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return nil
		},
	}
}

func printGroups(w io.Writer, catalog *spec.Catalog) {
	counts := map[string]int{}
	for _, c := range catalog.Commands {
		if c.Internal {
			continue
		}
		counts[c.PathParts[0]]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Fprintln(w, "Command groups:")
	for _, k := range keys {
		short := groupShort[k]
		if short == "" {
			short = ""
		}
		fmt.Fprintf(w, "  %-12s %2d commands  %s\n", k, counts[k], short)
	}
	fmt.Fprintln(w, "\nRun `pmock-cli docs <group>` or `pmock-cli <group> --help` for details.")
	fmt.Fprintln(w, "Run `pmock-cli commands` for the machine-readable catalog.")
}

func childrenOf(catalog *spec.Catalog, prefix string) []*spec.Command {
	parts := strings.Fields(prefix)
	var out []*spec.Command
	for _, c := range catalog.Commands {
		if c.Internal || len(c.PathParts) <= len(parts) {
			continue
		}
		ok := true
		for i, p := range parts {
			if c.PathParts[i] != p {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FullPath() < out[j].FullPath() })
	return out
}

func printCommandDoc(w io.Writer, c *spec.Command) {
	fmt.Fprintf(w, "pmock-cli %s\n\n%s\n\n", c.FullPath(), c.LongHelp())
	if len(c.Flags) > 0 {
		fmt.Fprintln(w, "FLAGS")
		for _, f := range c.Flags {
			fmt.Fprintf(w, "  --%-24s %s\n", f.Name+" "+f.Type, f.Help())
		}
	}
	if c.HasJSONInput() {
		fmt.Fprintln(w, "  --json <object|@file|->   merge a JSON object into the request body")
	}
	if c.CanWait() {
		fmt.Fprintln(w, "  --wait                    wait for the async task to reach a terminal state")
		fmt.Fprintln(w, "  --wait-timeout duration   maximum wait (default 3m)")
		fmt.Fprintln(w, "  --poll-interval duration  polling interval (default 2s)")
	}
}

func printUploadDoc(w io.Writer) {
	c := newPhoneFileUploadCmd(&globals{})
	fmt.Fprintf(w, "pmock-cli phone file upload\n\n%s\n\nFLAGS\n%s", phoneFileUploadLong, c.Flags().FlagUsages())
}
