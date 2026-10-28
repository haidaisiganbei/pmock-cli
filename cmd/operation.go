package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/pmock/pmock-cli/internal/build"
	"github.com/pmock/pmock-cli/internal/client"
	"github.com/pmock/pmock-cli/internal/config"
	"github.com/pmock/pmock-cli/internal/exitcode"
	"github.com/pmock/pmock-cli/internal/output"
	"github.com/pmock/pmock-cli/internal/spec"
)

func newOperationCmd(sc *spec.Command, g *globals) *cobra.Command {
	byName := flagMap(sc)
	c := &cobra.Command{
		Use:     sc.Usage(),
		Aliases: append([]string{}, sc.Aliases...),
		Short:   sc.Summary,
		Long:    sc.LongHelp(),
		Example: exampleText(sc.Examples),
		Args:    cobra.NoArgs,
		Hidden:  sc.Internal,
	}
	for _, f := range sc.Flags {
		addFlag(c, f)
	}
	if sc.HasJSONInput() {
		c.Flags().String("json", "", "request body as a JSON object, or @file.json, or - for stdin; flags override its keys")
	}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		return runOperation(cmd, sc, byName, g)
	}
	return c
}

func exampleText(examples []string) string {
	if len(examples) == 0 {
		return ""
	}
	return "  " + strings.Join(examples, "\n  ")
}

func flagMap(sc *spec.Command) map[string]*spec.Flag {
	byName := make(map[string]*spec.Flag, len(sc.Flags))
	for _, f := range sc.Flags {
		byName[f.Name] = f
	}
	return byName
}

func addFlag(c *cobra.Command, f *spec.Flag) {
	switch f.Type {
	case spec.TypeString:
		c.Flags().String(f.Name, "", f.Help())
	case spec.TypeInteger:
		c.Flags().Int64(f.Name, 0, f.Help())
	case spec.TypeNumber:
		c.Flags().Float64(f.Name, 0, f.Help())
	case spec.TypeBoolean:
		c.Flags().Bool(f.Name, false, f.Help())
	case spec.TypeStringArray:
		c.Flags().StringSlice(f.Name, nil, f.Help())
	case spec.TypeIntArray:
		c.Flags().Int64Slice(f.Name, nil, f.Help())
	default:
		panic("unsupported flag type " + f.Type)
	}
}

func runOperation(cmd *cobra.Command, sc *spec.Command, byName map[string]*spec.Flag, g *globals) error {
	if sc.Risk == spec.RiskDestructive && !g.yes {
		return exitcode.Usagef("%s is destructive and cannot be undone; re-run with --yes", sc.FullPath())
	}
	if sc.Risk == spec.RiskCostly && !g.yes {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"Warning: %s can spend money; re-run with --yes to acknowledge silently\n", sc.FullPath())
	}

	path, query, body, err := buildRequest(cmd, sc, byName)
	if err != nil {
		return err
	}

	if g.dryRun {
		base := g.baseURL
		if base == "" {
			cfg, cfgErr := config.LoadForDisplay()
			if cfgErr != nil {
				return exitcode.Usagef("%v", cfgErr)
			}
			base = cfg.BaseURL
		}
		req := dryRun{
			Method: sc.Method,
			URL:    client.BuildURL(base, path, query),
		}
		if body != nil && len(body) > 0 {
			req.Body = body
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(req); err != nil {
			return exitcode.Runtimef("failed to render dry-run request: %v", err)
		}
		return nil
	}

	cfg, err := g.config()
	if err != nil {
		return err
	}

	c := client.New(cfg.BaseURL, cfg.AccessKey, cfg.SecretKey, cfg.Timeout)
	c.UserAgent = "pmock-cli/" + build.Version

	if g.verbose {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s %s\n", sc.Method, client.BuildURL(cfg.BaseURL, path, query))
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
	defer cancel()

	env, err := c.Do(ctx, client.Request{Method: sc.Method, Path: path, Query: query, Body: body})
	if env != nil {
		if rerr := output.Render(cmd.OutOrStdout(), env, output.Options{Format: g.format, Compact: g.compact}); rerr != nil {
			return exitcode.Runtimef("failed to render response: %v", rerr)
		}
	}
	if err == nil {
		return nil
	}

	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		if g.allowBusinessError {
			return nil
		}
		if apiErr.Code == 10123 {
			fmt.Fprintln(cmd.ErrOrStderr(),
				"Hint: the access key was rejected. Check `pmock-cli config show` or re-run `pmock-cli config init`.")
			return exitcode.New(exitcode.Runtime, err)
		}
		return exitcode.Businessf("%v", err)
	}
	if g.verbose {
		fmt.Fprintf(cmd.ErrOrStderr(), "request failed: %v\n", err)
	}
	return exitcode.Runtimef("%v", err)
}

// buildRequest turns flags and --json into a path, query and JSON body.
func buildRequest(cmd *cobra.Command, sc *spec.Command, byName map[string]*spec.Flag) (string, url.Values, map[string]any, error) {
	path := sc.Path
	query := url.Values{}
	var body map[string]any
	if sc.HasBody() {
		body = map[string]any{}
	}

	if cmd.Flags().Changed("json") {
		raw, err := cmd.Flags().GetString("json")
		if err != nil {
			return "", nil, nil, exitcode.Usagef("%v", err)
		}
		m, err := parseJSONObject(raw)
		if err != nil {
			return "", nil, nil, exitcode.Usagef("invalid --json: %v", err)
		}
		if body == nil {
			body = map[string]any{}
		}
		for k, v := range m {
			body[k] = v
		}
	}

	changed := map[string]any{}
	var flagErr error
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if f.Name == "json" || flagErr != nil {
			return
		}
		sf := byName[f.Name]
		if sf == nil {
			return
		}
		v, err := readFlag(cmd, sf)
		if err != nil {
			flagErr = exitcode.Usagef("%v", err)
			return
		}
		changed[sf.Field] = v
		switch sf.In {
		case "path":
			path = strings.ReplaceAll(path, "{"+sf.Field+"}", url.PathEscape(fmt.Sprint(v)))
		case "query":
			addQuery(query, sf.Field, v)
		case "body":
			if body == nil {
				body = map[string]any{}
			}
			body[sf.Field] = v
		}
	})
	if flagErr != nil {
		return "", nil, nil, flagErr
	}

	// Some POST endpoints repeat a query field in the body (app/list pageSize,
	// pageNum). If the flag was set, populate both representations.
	if body != nil && sc.Body != nil {
		for field := range sc.Body.Properties {
			if _, ok := body[field]; ok {
				continue
			}
			if v, ok := changed[field]; ok {
				body[field] = v
			}
		}
	}

	var missing []string
	for _, f := range sc.Flags {
		if !f.Required {
			continue
		}
		switch f.In {
		case "path", "query":
			if _, ok := changed[f.Field]; !ok {
				missing = append(missing, "--"+f.Name)
			}
		case "body":
			if body == nil {
				missing = append(missing, "--"+f.Name)
				continue
			}
			if _, ok := body[f.Field]; !ok {
				missing = append(missing, "--"+f.Name)
			}
		}
	}
	for _, field := range sc.ComplexRequired {
		if body == nil {
			missing = append(missing, field+" (use --json)")
			continue
		}
		if _, ok := body[field]; !ok {
			missing = append(missing, field+" (use --json)")
		}
	}
	if len(missing) > 0 {
		return "", nil, nil, exitcode.Usagef(
			"missing required %s; run `pmock-cli docs %s` for the full field list",
			strings.Join(missing, ", "), sc.FullPath())
	}

	return path, query, body, nil
}

func readFlag(cmd *cobra.Command, f *spec.Flag) (any, error) {
	switch f.Type {
	case spec.TypeString:
		return cmd.Flags().GetString(f.Name)
	case spec.TypeInteger:
		return cmd.Flags().GetInt64(f.Name)
	case spec.TypeNumber:
		return cmd.Flags().GetFloat64(f.Name)
	case spec.TypeBoolean:
		return cmd.Flags().GetBool(f.Name)
	case spec.TypeStringArray:
		return cmd.Flags().GetStringSlice(f.Name)
	case spec.TypeIntArray:
		return cmd.Flags().GetInt64Slice(f.Name)
	default:
		return nil, fmt.Errorf("unsupported flag type %q", f.Type)
	}
}

func addQuery(q url.Values, field string, v any) {
	switch t := v.(type) {
	case []string:
		for _, e := range t {
			q.Add(field, e)
		}
	case []int64:
		for _, e := range t {
			q.Add(field, fmt.Sprint(e))
		}
	default:
		q.Set(field, fmt.Sprint(v))
	}
}

func parseJSONObject(arg string) (map[string]any, error) {
	var data []byte
	switch {
	case arg == "-":
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read stdin: %w", err)
		}
		data = b
	case strings.HasPrefix(arg, "@"):
		b, err := os.ReadFile(strings.TrimPrefix(arg, "@"))
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", strings.TrimPrefix(arg, "@"), err)
		}
		data = b
	default:
		data = []byte(arg)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("expected a JSON object, got null")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("unexpected trailing JSON value")
		}
		return nil, fmt.Errorf("trailing data: %w", err)
	}
	return m, nil
}

type dryRun struct {
	Method string `json:"method"`
	URL    string `json:"url"`
	Body   any    `json:"body,omitempty"`
}
