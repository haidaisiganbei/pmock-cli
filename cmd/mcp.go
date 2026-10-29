package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/haidaisiganbei/pmock-cli/internal/build"
	"github.com/haidaisiganbei/pmock-cli/internal/config"
	"github.com/haidaisiganbei/pmock-cli/internal/spec"
)

const mcpInstructions = "PMOCK cloud phone management via pmock-cli. " +
	"Each tool dispatches on an 'action' parameter; pass CLI flags via the 'params' object " +
	"(keys are flag names without '--', values are string/number/boolean/array; nested " +
	"objects can be passed as the 'json' key). Call pmock_help before an unfamiliar action. " +
	"Destructive actions require params.yes=true."

const mcpLong = `Run pmock-cli as a Model Context Protocol (MCP) server over stdio.

AI agents (Claude Code, Cursor, VS Code, ...) talk to this server to manage
cloud phones, apps, files, RPA tasks and account settings. Each tool dispatches
on an action and reuses the same command catalog as the CLI, so the MCP surface
cannot drift from the CLI.

Authentication is shared with the CLI: ~/.pmock-cli/config.json or
PMOCK_ACCESS_KEY / PMOCK_SECRET_KEY.

While the server is running stdout carries the MCP protocol only; diagnostics
go to stderr.

CLIENT CONFIGURATION:
    {
      "mcpServers": {
        "pmock": {
          "command": "pmock-cli",
          "args": ["mcp"],
          "env": {
            "PMOCK_ACCESS_KEY": "<ak>",
            "PMOCK_SECRET_KEY": "<sk>"
          }
        }
      }
    }`

// mcpArgs is the single input schema shared by all routed tools.
type mcpArgs struct {
	Action string         `json:"action" jsonschema:"the action to execute; must be one of the actions listed in this tool's description"`
	Params map[string]any `json:"params,omitempty" jsonschema:"CLI flags without the leading --; values may be string, number, boolean, array, or a nested object for --json"`
}

type helpArgs struct {
	Command string `json:"command" jsonschema:"command path without the binary name, e.g. phone start; empty for the root help"`
}

type mcpAction struct {
	Name    string
	Path    []string
	Summary string
	Risk    spec.Risk
}

type mcpTool struct {
	Name    string
	Summary string
	Actions []mcpAction
}

func (t *mcpTool) find(name string) *mcpAction {
	for i := range t.Actions {
		if t.Actions[i].Name == name {
			return &t.Actions[i]
		}
	}
	return nil
}

func (t *mcpTool) describe() string {
	var b strings.Builder
	b.WriteString(t.Summary)
	b.WriteString("\n\nActions (params are CLI flags without --; call pmock_help for the full flag list):\n")
	for _, a := range t.Actions {
		fmt.Fprintf(&b, "- %s: %s", a.Name, a.Summary)
		switch a.Risk {
		case spec.RiskDestructive:
			b.WriteString(" [destructive: pass params.yes=true]")
		case spec.RiskCostly:
			b.WriteString(" [costly: can spend money]")
		}
		b.WriteString("\n")
	}
	return b.String()
}

type mcpServer struct {
	catalog *spec.Catalog
	tools   []*mcpTool
}

func newMCPCmd(catalog *spec.Catalog) *cobra.Command {
	return &cobra.Command{
		Use:           "mcp",
		Short:         "Run an MCP server over stdio (for AI agents)",
		Long:          mcpLong,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			s := &mcpServer{catalog: catalog, tools: buildMCPTools(catalog)}
			srv := mcp.NewServer(&mcp.Implementation{
				Name:    "pmock-cli",
				Version: build.Version,
			}, &mcp.ServerOptions{Instructions: mcpInstructions})

			s.registerHelp(srv)
			for _, t := range s.tools {
				s.registerTool(srv, t)
			}

			if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil && !isMCPShutdown(err) {
				return err
			}
			return nil
		},
	}
}

func (s *mcpServer) registerTool(srv *mcp.Server, t *mcpTool) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        t.Name,
		Description: t.describe(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args mcpArgs) (*mcp.CallToolResult, any, error) {
		return s.callTool(ctx, t, args)
	})
}

func (s *mcpServer) registerHelp(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "pmock_help",
		Description: "Get the CLI help text (all flags) for any pmock-cli command, e.g. " +
			"\"phone start\", \"phone file upload\", \"rpa task add\". Call this before an " +
			"action whose flags you do not know. An empty command returns the root help.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args helpArgs) (*mcp.CallToolResult, any, error) {
		parts := strings.Fields(args.Command)
		for _, p := range parts {
			if !commandTokenRe.MatchString(p) {
				return mcpError("invalid command token: " + p), nil, nil
			}
		}
		stdout, stderr, err := s.runHelp(ctx, parts)
		text := strings.TrimSpace(stdout)
		if text == "" {
			text = strings.TrimSpace(stderr)
		}
		if text == "" && err != nil {
			text = err.Error()
		}
		if text == "" {
			text = "no help available"
		}
		return mcpText(text), nil, nil
	})
}

func (s *mcpServer) callTool(ctx context.Context, t *mcpTool, args mcpArgs) (*mcp.CallToolResult, any, error) {
	action := t.find(args.Action)
	if action == nil {
		return mcpError(fmt.Sprintf("unknown action %q for %s. Valid actions: %s",
			args.Action, t.Name, t.actionList())), nil, nil
	}
	if action.Name == "config-init" {
		if err := guardConfigInit(args.Params); err != nil {
			return mcpError(err.Error()), nil, nil
		}
	}
	if v, ok := args.Params["json"].(string); ok && v == "-" {
		return mcpError("--json - is not available in MCP mode; pass the object in params.json instead"), nil, nil
	}
	argv, err := buildCLIArgs(action.Path, args.Params)
	if err != nil {
		return mcpError(err.Error()), nil, nil
	}

	stdout, stderr, err := s.runCLI(ctx, argv)
	text := strings.TrimSpace(stdout)
	if err != nil {
		if extra := strings.TrimSpace(stderr); extra != "" {
			if text != "" {
				text += "\n"
			}
			text += extra
		}
		if text == "" {
			text = err.Error()
		}
		text += "\n\n" + s.helpText(ctx, action.Path)
		return mcpError(text), nil, nil
	}

	// The CLI already exits non-zero for a non-zero envelope; this is a
	// defence in depth for --allow-business-error style paths.
	var env struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal([]byte(stdout), &env) == nil && env.Code != 0 {
		text += "\n\n" + s.helpText(ctx, action.Path)
		return mcpError(text), nil, nil
	}
	if text == "" {
		text = "ok"
	}
	return mcpText(text), nil, nil
}

func (t *mcpTool) actionList() string {
	names := make([]string, 0, len(t.Actions))
	for _, a := range t.Actions {
		names = append(names, a.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func (s *mcpServer) runCLI(ctx context.Context, argv []string) (string, string, error) {
	root := newRootWith(s.catalog, false)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append(argv, "--format=json"))
	err := root.ExecuteContext(ctx)
	return out.String(), errOut.String(), err
}

func (s *mcpServer) runHelp(ctx context.Context, parts []string) (string, string, error) {
	root := newRootWith(s.catalog, false)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append(append([]string{}, parts...), "--help"))
	err := root.ExecuteContext(ctx)
	return out.String(), errOut.String(), err
}

func (s *mcpServer) helpText(ctx context.Context, path []string) string {
	out, _, _ := s.runHelp(ctx, path)
	out = strings.TrimSpace(out)
	if out == "" {
		return "[help unavailable]"
	}
	return "[Help: pmock-cli " + strings.Join(path, " ") + "]\n" + out
}

func mcpText(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

func mcpError(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: true,
	}
}

var (
	commandTokenRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]*$`)
	flagNameRe     = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]*$`)
)

func buildMCPTools(catalog *spec.Catalog) []*mcpTool {
	tools := []*mcpTool{
		{Name: "pmock_phone", Summary: "Manage PMOCK cloud phones: list, create, start, stop, restart, reset, renew, screenshot, GPS, ADB info, network config and more."},
		{Name: "pmock_phone_action", Summary: "UI automation actions on a running cloud phone: click, double-click, long-click, swipe, drag, input and app start/stop/uninstall."},
		{Name: "pmock_phone_file", Summary: "Files on a cloud phone: scan, download, and upload a local file or APK (multipart upload with MD5)."},
		{Name: "pmock_app", Summary: "Applications on cloud phones: list, categories, install, upgrade list and operate."},
		{Name: "pmock_network", Summary: "Proxies and network detection: list, detect, set network."},
		{Name: "pmock_rpa_template", Summary: "RPA templates: market, team list, collect and delete."},
		{Name: "pmock_rpa_task", Summary: "RPA tasks: add, list, get, details, update, stop, retry and delete."},
		{Name: "pmock_product", Summary: "Products, SKUs and pricing."},
		{Name: "pmock_brand", Summary: "Cloud phone brands."},
		{Name: "pmock_team", Summary: "Team members and phone authorization."},
		{Name: "pmock_task", Summary: "Background task records."},
		{Name: "pmock_account", Summary: "CLI account and configuration: config init/show and auth status."},
	}
	byName := make(map[string]*mcpTool, len(tools))
	for _, t := range tools {
		byName[t.Name] = t
	}

	pick := func(path []string) *mcpTool {
		switch {
		case path[0] == "phone" && hasPrefix(path, []string{"phone", "action"}):
			return byName["pmock_phone_action"]
		case path[0] == "phone" && hasPrefix(path, []string{"phone", "file"}):
			return byName["pmock_phone_file"]
		case path[0] == "phone":
			return byName["pmock_phone"]
		case path[0] == "app":
			return byName["pmock_app"]
		case path[0] == "network":
			return byName["pmock_network"]
		case hasPrefix(path, []string{"rpa", "template"}):
			return byName["pmock_rpa_template"]
		case hasPrefix(path, []string{"rpa", "task"}):
			return byName["pmock_rpa_task"]
		case path[0] == "product":
			return byName["pmock_product"]
		case path[0] == "brand":
			return byName["pmock_brand"]
		case path[0] == "team":
			return byName["pmock_team"]
		case path[0] == "task":
			return byName["pmock_task"]
		case path[0] == "config" || path[0] == "auth" || path[0] == "doctor":
			return byName["pmock_account"]
		}
		return nil
	}

	for _, c := range catalog.Commands {
		if c.Internal {
			continue
		}
		tool := pick(c.PathParts)
		if tool == nil {
			continue
		}
		name := c.PathParts[len(c.PathParts)-1]
		switch c.PathParts[0] {
		case "config":
			name = "config-" + name
		case "auth":
			name = "auth-" + name
		}
		tool.Actions = append(tool.Actions, mcpAction{
			Name: name, Path: append([]string{}, c.PathParts...), Summary: c.Summary, Risk: c.Risk,
		})
	}

	byName["pmock_phone_file"].Actions = append(byName["pmock_phone_file"].Actions, mcpAction{
		Name:    "upload",
		Path:    []string{"phone", "file", "upload"},
		Summary: "Upload a local file or APK to a cloud phone (multipart, MD5, merge and push)",
		Risk:    spec.RiskWrite,
	})
	byName["pmock_account"].Actions = append(byName["pmock_account"].Actions,
		mcpAction{Name: "config-init", Path: []string{"config", "init"}, Summary: "Save the access key and secret key", Risk: spec.RiskWrite},
		mcpAction{Name: "config-show", Path: []string{"config", "show"}, Summary: "Show the effective configuration with secrets masked", Risk: spec.RiskRead},
		mcpAction{Name: "auth-status", Path: []string{"auth", "status"}, Summary: "Verify credentials with a lightweight API call", Risk: spec.RiskRead},
		mcpAction{Name: "doctor", Path: []string{"doctor"}, Summary: "Diagnose config, credentials and API connectivity", Risk: spec.RiskRead},
	)

	for _, t := range tools {
		sort.Slice(t.Actions, func(i, j int) bool { return t.Actions[i].Name < t.Actions[j].Name })
	}
	return tools
}

func hasPrefix(path, prefix []string) bool {
	if len(path) < len(prefix) {
		return false
	}
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
}

// guardConfigInit keeps the interactive `config init` prompt away from the MCP
// transport: reading os.Stdin here would consume JSON-RPC frames.
func guardConfigInit(params map[string]any) error {
	_, hasAK := params["access-key"]
	_, hasSK := params["secret-key"]
	if hasAK && hasSK {
		return nil
	}
	cfg, err := config.LoadFile()
	if err == nil && cfg.AccessKey != "" && cfg.SecretKey != "" {
		if len(params) == 0 {
			return fmt.Errorf("config-init in MCP mode is non-interactive; pass access-key and secret-key, or config-show to inspect")
		}
		return nil
	}
	return fmt.Errorf("config-init in MCP mode requires params.access-key and params.secret-key")
}

func buildCLIArgs(path []string, params map[string]any) ([]string, error) {
	argv := append([]string{}, path...)
	if len(params) == 0 {
		return argv, nil
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if !flagNameRe.MatchString(k) {
			return nil, fmt.Errorf("invalid flag name %q (letters, digits, dashes)", k)
		}
		v, err := formatMCPParam(k, params[k])
		if err != nil {
			return nil, fmt.Errorf("invalid value for --%s: %w", k, err)
		}
		argv = append(argv, "--"+k+"="+v)
	}
	return argv, nil
}

func formatMCPParam(key string, v any) (string, error) {
	switch t := v.(type) {
	case nil:
		return "", fmt.Errorf("null value")
	case string:
		return t, nil
	case bool:
		return strconv.FormatBool(t), nil
	case float64:
		if t == math.Trunc(t) && math.Abs(t) < 1e15 {
			return strconv.FormatInt(int64(t), 10), nil
		}
		return strconv.FormatFloat(t, 'f', -1, 64), nil
	case json.Number:
		return t.String(), nil
	case int:
		return strconv.Itoa(t), nil
	case int64:
		return strconv.FormatInt(t, 10), nil
	case []string:
		return strings.Join(t, ","), nil
	case []int64:
		parts := make([]string, len(t))
		for i, n := range t {
			parts[i] = strconv.FormatInt(n, 10)
		}
		return strings.Join(parts, ","), nil
	case []any:
		if isScalarArray(t) {
			parts := make([]string, 0, len(t))
			for _, e := range t {
				s, err := formatMCPParam(key, e)
				if err != nil {
					return "", err
				}
				parts = append(parts, s)
			}
			return strings.Join(parts, ","), nil
		}
		b, err := json.Marshal(t)
		if err != nil {
			return "", err
		}
		return string(b), nil
	case map[string]any:
		b, err := json.Marshal(t)
		if err != nil {
			return "", err
		}
		return string(b), nil
	default:
		return fmt.Sprint(t), nil
	}
}

func isScalarArray(arr []any) bool {
	for _, v := range arr {
		switch v.(type) {
		case string, bool, float64, json.Number, int, int64:
		default:
			return false
		}
	}
	return true
}

func isMCPShutdown(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
		return true
	}
	var rpcErr *jsonrpc.Error
	return errors.As(err, &rpcErr) && rpcErr.Code == -32004
}
