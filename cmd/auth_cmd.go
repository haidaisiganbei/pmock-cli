package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/spf13/cobra"

	"github.com/pmock/pmock-cli/internal/build"
	"github.com/pmock/pmock-cli/internal/client"
	"github.com/pmock/pmock-cli/internal/config"
	"github.com/pmock/pmock-cli/internal/exitcode"
	"github.com/pmock/pmock-cli/internal/output"
)

func newAuthCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication helpers",
	}
	cmd.AddCommand(newAuthStatusCmd(g))
	return cmd
}

func newAuthStatusCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Verify credentials with a lightweight API call",
		Long: `Verify the configured AK/SK by calling the phone list endpoint with a
page size of one. Nothing is written and no phone is touched.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := g.config()
			if err != nil {
				return err
			}
			c := client.New(cfg.BaseURL, cfg.AccessKey, cfg.SecretKey, cfg.Timeout)
			c.UserAgent = "pmock-cli/" + build.Version

			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
			defer cancel()

			start := time.Now()
			env, err := c.Do(ctx, client.Request{
				Method: "GET",
				Path:   "/open/api/phone/list",
				Query:  url.Values{"pageNum": {"1"}, "pageSize": {"1"}},
			})
			latency := time.Since(start).Milliseconds()

			if env != nil && err == nil {
				return renderData(cmd, g, 0, "authenticated", map[string]any{
					"authenticated": true,
					"baseUrl":       cfg.BaseURL,
					"accessKey":     config.MaskAccessKey(cfg.AccessKey),
					"latencyMs":     latency,
					"traceId":       env.TraceID,
				})
			}
			if err == nil {
				return exitcode.Runtimef("empty response from API")
			}

			var apiErr *client.APIError
			if errors.As(err, &apiErr) {
				if env != nil {
					if rerr := output.Render(cmd.OutOrStdout(), env, output.Options{Format: g.format, Compact: g.compact}); rerr != nil {
						return exitcode.Runtimef("failed to render response: %v", rerr)
					}
				}
				if apiErr.Code == 10123 {
					fmt.Fprintln(cmd.ErrOrStderr(),
						"Hint: the API rejected the access key. Check `pmock-cli config show`, "+
							"re-run `pmock-cli config init`, or confirm the key is enabled in the PMOCK client.")
					return exitcode.New(exitcode.Runtime, err)
				}
				return exitcode.Businessf("%v", err)
			}
			return exitcode.Runtimef("%v", err)
		},
	}
}
