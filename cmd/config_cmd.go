package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/pmock/pmock-cli/internal/client"
	"github.com/pmock/pmock-cli/internal/config"
	"github.com/pmock/pmock-cli/internal/exitcode"
	"github.com/pmock/pmock-cli/internal/output"
)

func newConfigCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage pmock-cli credentials",
		Long: `Configure the team access key used to sign open API requests.

Credentials live in ~/.pmock-cli/config.json (0600). Environment variables
PMOCK_ACCESS_KEY and PMOCK_SECRET_KEY take precedence and are the recommended
way to inject credentials in CI.`,
	}
	cmd.AddCommand(newConfigInitCmd(g))
	cmd.AddCommand(newConfigShowCmd(g))
	return cmd
}

func newConfigInitCmd(g *globals) *cobra.Command {
	var accessKey, secretKey, baseURL string
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Save the access key and secret key",
		Long: `Save credentials, merging into the existing file.

With no flags the command prompts interactively. The secret is read without
echo when stdin is a terminal. Passing any flag makes the run non-interactive,
so omitted values keep their current setting.`,
		Example: `  pmock-cli config init
  pmock-cli config init --access-key AK... --secret-key SK...
  pmock-cli config init --base-url https://user.pmock.com/api`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadFile()
			if err != nil {
				// A broken file must not make init impossible: that is exactly
				// when the user needs to repair it.
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %v; starting from empty config\n", err)
				cfg = &config.Config{}
			}

			interactive := !cmd.Flags().Changed("access-key") &&
				!cmd.Flags().Changed("secret-key") &&
				!cmd.Flags().Changed("base-url") &&
				!cmd.Flags().Changed("timeout")

			in := bufio.NewReader(os.Stdin)

			if cmd.Flags().Changed("access-key") {
				cfg.AccessKey = accessKey
			} else if interactive {
				v, err := promptLine(in, "Access key: ", true)
				if err != nil {
					return exitcode.Usagef("failed to read access key: %v", err)
				}
				if v != "" {
					cfg.AccessKey = v
				}
			}

			if cmd.Flags().Changed("secret-key") {
				cfg.SecretKey = secretKey
			} else if interactive {
				v, err := promptSecret(in, "Secret key (input hidden): ")
				if err != nil {
					return exitcode.Usagef("failed to read secret key: %v", err)
				}
				if v != "" {
					cfg.SecretKey = v
				}
			}

			if cmd.Flags().Changed("base-url") {
				cfg.BaseURL = baseURL
			} else if interactive {
				def := cfg.BaseURL
				if def == "" {
					def = config.DefaultBaseURL
				}
				v, err := promptLine(in, fmt.Sprintf("Base URL [%s]: ", def), true)
				if err != nil {
					return exitcode.Usagef("failed to read base URL: %v", err)
				}
				if v != "" {
					cfg.BaseURL = v
				}
			}

			if cmd.Flags().Changed("timeout") {
				if timeout <= 0 {
					return exitcode.Usagef("--timeout must be positive")
				}
				cfg.TimeoutSeconds = int(timeout.Seconds())
			}

			cfg.AccessKey = strings.TrimSpace(cfg.AccessKey)
			cfg.SecretKey = strings.TrimSpace(cfg.SecretKey)
			if cfg.AccessKey == "" || cfg.SecretKey == "" {
				return exitcode.Usagef("access key and secret key are required")
			}
			if err := config.Save(cfg); err != nil {
				return exitcode.Usagef("%v", err)
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "Saved credentials to %s\n", config.Path())
			if names := config.EnvOverrides(); len(names) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"Warning: %s set in the environment and still take precedence\n", strings.Join(names, ", "))
			}

			base := cfg.BaseURL
			if base == "" {
				base = config.DefaultBaseURL
			}
			return renderData(cmd, g, 0, "configuration saved", map[string]any{
				"path":      config.Path(),
				"baseUrl":   base,
				"accessKey": config.MaskAccessKey(cfg.AccessKey),
			})
		},
	}

	cmd.Flags().StringVar(&accessKey, "access-key", "", "team access key")
	cmd.Flags().StringVar(&secretKey, "secret-key", "", "team secret key")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "API base URL (default "+config.DefaultBaseURL+")")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "request timeout")
	return cmd
}

func newConfigShowCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show the effective configuration (secrets masked)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadForDisplay()
			if err != nil {
				return exitcode.Usagef("%v", err)
			}
			base := cfg.BaseURL
			if g.baseURL != "" {
				base = strings.TrimRight(g.baseURL, "/")
			}
			data := map[string]any{
				"path":         config.Path(),
				"fileExists":   config.FileExists(),
				"baseUrl":      base,
				"accessKey":    config.MaskAccessKey(cfg.AccessKey),
				"secretKey":    config.MaskSecret(cfg.SecretKey),
				"timeout":      cfg.Timeout.String(),
				"envOverrides": config.EnvOverrides(),
			}
			return renderData(cmd, g, 0, "ok", data)
		},
	}
}

func renderData(cmd *cobra.Command, g *globals, code int, msg string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return exitcode.Runtimef("failed to encode response: %v", err)
	}
	env := &client.Envelope{Code: code, Msg: msg, Data: raw}
	return output.Render(cmd.OutOrStdout(), env, output.Options{Format: g.format, Compact: g.compact})
}

func promptLine(in *bufio.Reader, label string, show bool) (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) && show {
		fmt.Fprint(os.Stderr, label)
	}
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func promptSecret(in *bufio.Reader, label string) (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, label)
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
