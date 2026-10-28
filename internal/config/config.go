// Package config resolves the credentials and endpoint used by pmock-cli.
//
// Precedence (highest first):
//
//	flags (handled by the caller)
//	PMOCK_ACCESS_KEY / PMOCK_SECRET_KEY / PMOCK_BASE_URL / PMOCK_TIMEOUT
//	~/.pmock-cli/config.json
//	built-in defaults
//
// The file is created with 0600 and its directory with 0700 because it holds a
// long-lived team secret.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the host prefix; every spec path starts with /open/api.
	DefaultBaseURL = "https://user.pmock.com/api"
	// DefaultTimeout is the per-request budget.
	DefaultTimeout = 30 * time.Second

	EnvAccessKey = "PMOCK_ACCESS_KEY"
	EnvSecretKey = "PMOCK_SECRET_KEY"
	EnvBaseURL   = "PMOCK_BASE_URL"
	EnvTimeout   = "PMOCK_TIMEOUT"
	// EnvConfigFile overrides the config path (tests, CI, multiple profiles).
	EnvConfigFile = "PMOCK_CLI_CONFIG"
)

// Config is the persisted and effective CLI configuration.
type Config struct {
	AccessKey      string `json:"accessKey,omitempty"`
	SecretKey      string `json:"secretKey,omitempty"`
	BaseURL        string `json:"baseUrl,omitempty"`
	TimeoutSeconds int    `json:"timeoutSeconds,omitempty"`

	// Timeout is derived, never serialized.
	Timeout time.Duration `json:"-"`
}

// Dir returns the config directory.
func Dir() string {
	if p := strings.TrimSpace(os.Getenv(EnvConfigFile)); p != "" {
		return filepath.Dir(p)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".pmock-cli"
	}
	return filepath.Join(home, ".pmock-cli")
}

// Path returns the config file path.
func Path() string {
	if p := strings.TrimSpace(os.Getenv(EnvConfigFile)); p != "" {
		return p
	}
	return filepath.Join(Dir(), "config.json")
}

// LoadFile reads only the config file, ignoring environment overrides. It is
// used by `config init` so a CI-injected credential does not get copied into
// the user's file.
func LoadFile() (*Config, error) {
	var cfg Config
	data, err := os.ReadFile(Path())
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", Path(), err)
		}
	case errors.Is(err, os.ErrNotExist):
		return &Config{}, nil
	case err != nil:
		return nil, fmt.Errorf("failed to read %s: %w", Path(), err)
	}
	cfg.AccessKey = strings.TrimSpace(cfg.AccessKey)
	cfg.SecretKey = strings.TrimSpace(cfg.SecretKey)
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	return &cfg, nil
}

// LoadForDisplay reads the file and applies environment overrides, tolerating
// a missing token (so `config show` works before setup).
func LoadForDisplay() (*Config, error) {
	var cfg Config

	data, err := os.ReadFile(Path())
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", Path(), err)
		}
	case errors.Is(err, os.ErrNotExist):
		// First run.
	case err != nil:
		return nil, fmt.Errorf("failed to read %s: %w", Path(), err)
	}

	// Trim hand-edited or injected whitespace. A trailing newline pasted into
	// a CI secret would otherwise be signed and rejected as a bad key.
	cfg.AccessKey = strings.TrimSpace(cfg.AccessKey)
	cfg.SecretKey = strings.TrimSpace(cfg.SecretKey)
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)

	if v := strings.TrimSpace(os.Getenv(EnvAccessKey)); v != "" {
		cfg.AccessKey = v
	}
	if v := strings.TrimSpace(os.Getenv(EnvSecretKey)); v != "" {
		cfg.SecretKey = v
	}
	if v := strings.TrimSpace(os.Getenv(EnvBaseURL)); v != "" {
		cfg.BaseURL = v
	}

	cfg.Timeout = DefaultTimeout
	if cfg.TimeoutSeconds > 0 {
		cfg.Timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}
	if v := strings.TrimSpace(os.Getenv(EnvTimeout)); v != "" {
		d, err := parseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid %s %q: %w", EnvTimeout, v, err)
		}
		cfg.Timeout = d
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")

	return &cfg, nil
}

// Load returns a usable configuration or an actionable error.
func Load() (*Config, error) {
	cfg, err := LoadForDisplay()
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate checks that request signing is possible.
func (c *Config) Validate() error {
	if c.AccessKey == "" || c.SecretKey == "" {
		return fmt.Errorf("no API credentials configured, run `pmock-cli config init` or set %s and %s",
			EnvAccessKey, EnvSecretKey)
	}
	return nil
}

// Save writes the configuration atomically with 0600 permissions. Empty
// endpoints are written as empty so a future default change is not pinned.
func Save(cfg *Config) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	out := *cfg
	out.AccessKey = strings.TrimSpace(out.AccessKey)
	out.SecretKey = strings.TrimSpace(out.SecretKey)
	out.BaseURL = strings.TrimRight(strings.TrimSpace(out.BaseURL), "/")
	if out.BaseURL == DefaultBaseURL {
		out.BaseURL = ""
	}

	data, err := json.MarshalIndent(&out, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, ".config-*.json")
	if err != nil {
		return fmt.Errorf("failed to create temp config: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to set config permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to write config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close config: %w", err)
	}
	if err := os.Rename(tmpName, Path()); err != nil {
		return fmt.Errorf("failed to replace config: %w", err)
	}
	return nil
}

// EnvOverrides lists the environment variables currently taking precedence.
func EnvOverrides() []string {
	names := make([]string, 0, 4)
	for _, name := range []string{EnvAccessKey, EnvSecretKey, EnvBaseURL, EnvTimeout} {
		if strings.TrimSpace(os.Getenv(name)) != "" {
			names = append(names, name)
		}
	}
	return names
}

// MaskAccessKey keeps the key recognizable without disclosing it.
func MaskAccessKey(v string) string {
	if v == "" {
		return "(not set)"
	}
	if len(v) <= 8 {
		return "****"
	}
	return v[:4] + "****" + v[len(v)-4:]
}

// MaskSecret never reveals the secret.
func MaskSecret(v string) string {
	if v == "" {
		return "(not set)"
	}
	return "****"
}

// FileExists reports whether the config file exists.
func FileExists() bool {
	_, err := os.Stat(Path())
	return err == nil
}

func parseDuration(v string) (time.Duration, error) {
	if secs, err := strconv.Atoi(v); err == nil {
		if secs <= 0 {
			return 0, errors.New("must be positive")
		}
		return time.Duration(secs) * time.Second, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, errors.New("must be positive")
	}
	return d, nil
}
