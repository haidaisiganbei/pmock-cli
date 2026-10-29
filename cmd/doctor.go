package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/pmock/pmock-cli/internal/build"
	"github.com/pmock/pmock-cli/internal/client"
	"github.com/pmock/pmock-cli/internal/config"
	"github.com/pmock/pmock-cli/internal/exitcode"
)

type doctorCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

func newDoctorCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose config, credentials and API connectivity",
		Long: `Run local checks without touching a phone:

  - config file exists and is private (0600 on Unix)
  - access key / secret key are resolvable
  - base URL is a valid http(s) endpoint
  - a lightweight API call succeeds, with latency and traceId

The result is a JSON envelope whose data.ok is false when any check fails.
Exit code is 2 for missing credentials and 3 for network/auth failures.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			checks := make([]doctorCheck, 0, 4)

			path := config.Path()
			if runtime.GOOS == "windows" {
				if config.FileExists() {
					checks = append(checks, doctorCheck{Name: "config_file", OK: true, Detail: path})
				} else {
					checks = append(checks, doctorCheck{Name: "config_file", OK: true, Detail: path + " (not created; using environment)"})
				}
			} else if info, err := os.Stat(path); err == nil {
				mode := info.Mode().Perm()
				ok := mode&0o077 == 0
				checks = append(checks, doctorCheck{
					Name: "config_file", OK: ok,
					Detail: fmt.Sprintf("%s mode=%o", path, mode),
				})
			} else if os.IsNotExist(err) {
				checks = append(checks, doctorCheck{Name: "config_file", OK: true, Detail: path + " (not created; using environment)"})
			} else {
				checks = append(checks, doctorCheck{Name: "config_file", OK: false, Detail: err.Error()})
			}

			cfg, err := config.LoadForDisplay()
			if err != nil {
				checks = append(checks, doctorCheck{Name: "config_parse", OK: false, Detail: err.Error()})
				cfg = &config.Config{BaseURL: config.DefaultBaseURL, Timeout: config.DefaultTimeout}
			}
			if g.baseURL != "" {
				cfg.BaseURL = g.baseURL
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
			if cfg.Timeout <= 0 {
				cfg.Timeout = config.DefaultTimeout
			}

			credsOK := cfg.AccessKey != "" && cfg.SecretKey != ""
			detail := "accessKey=" + config.MaskAccessKey(cfg.AccessKey)
			if overrides := config.EnvOverrides(); len(overrides) > 0 {
				detail += "; env overrides: " + fmt.Sprint(overrides)
			}
			checks = append(checks, doctorCheck{Name: "credentials", OK: credsOK, Detail: detail})

			urlOK := false
			if u, err := url.Parse(cfg.BaseURL); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
				urlOK = true
			}
			checks = append(checks, doctorCheck{Name: "base_url", OK: urlOK, Detail: cfg.BaseURL})

			if credsOK && urlOK {
				c := client.New(cfg.BaseURL, cfg.AccessKey, cfg.SecretKey, cfg.Timeout)
				c.UserAgent = "pmock-cli/" + build.Version
				ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
				start := time.Now()
				env, apiErr := c.Do(ctx, client.Request{
					Method: "GET",
					Path:   "/open/api/phone/list",
					Query:  url.Values{"pageNum": {"1"}, "pageSize": {"1"}},
				})
				cancel()
				latency := time.Since(start).Milliseconds()
				if apiErr == nil && env != nil && env.Code == 0 {
					checks = append(checks, doctorCheck{
						Name: "api", OK: true,
						Detail: fmt.Sprintf("code=0 latency=%dms traceId=%s", latency, env.TraceID),
					})
				} else {
					msg := "request failed"
					if apiErr != nil {
						msg = apiErr.Error()
					} else if env != nil {
						msg = fmt.Sprintf("code=%d %s", env.Code, env.Msg)
					}
					checks = append(checks, doctorCheck{Name: "api", OK: false, Detail: msg})
				}
			}

			allOK := true
			for _, c := range checks {
				if !c.OK {
					allOK = false
				}
			}
			msg := "ok"
			if !allOK {
				msg = "degraded"
			}
			if err := renderData(cmd, g, 0, msg, map[string]any{
				"ok":      allOK,
				"baseUrl": cfg.BaseURL,
				"checks":  checks,
			}); err != nil {
				return err
			}
			if allOK {
				return nil
			}
			if !credsOK {
				return exitcode.Usagef("doctor: credentials are not configured")
			}
			return exitcode.Runtimef("doctor: one or more checks failed")
		},
	}
}
