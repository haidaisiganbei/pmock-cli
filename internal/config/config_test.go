package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv(EnvConfigFile, path)
	t.Setenv(EnvAccessKey, "")
	t.Setenv(EnvSecretKey, "")
	t.Setenv(EnvBaseURL, "")
	t.Setenv(EnvTimeout, "")
	return path
}

func TestSaveAndLoad(t *testing.T) {
	isolate(t)
	cfg := &Config{AccessKey: "AK", SecretKey: "SK", TimeoutSeconds: 45}
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(Path())
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AccessKey != "AK" || loaded.SecretKey != "SK" {
		t.Fatalf("unexpected credentials: %+v", loaded)
	}
	if loaded.BaseURL != DefaultBaseURL {
		t.Fatalf("base URL = %q, want default", loaded.BaseURL)
	}
	if loaded.Timeout != 45*time.Second {
		t.Fatalf("timeout = %s, want 45s", loaded.Timeout)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	isolate(t)
	if err := Save(&Config{AccessKey: "file-ak", SecretKey: "file-sk"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvAccessKey, "env-ak")
	t.Setenv(EnvSecretKey, "env-sk")
	t.Setenv(EnvBaseURL, "https://example.test/api/")
	t.Setenv(EnvTimeout, "12")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccessKey != "env-ak" || cfg.SecretKey != "env-sk" {
		t.Fatalf("env did not win: %+v", cfg)
	}
	if cfg.BaseURL != "https://example.test/api" {
		t.Fatalf("base URL = %q", cfg.BaseURL)
	}
	if cfg.Timeout != 12*time.Second {
		t.Fatalf("timeout = %s", cfg.Timeout)
	}
	if len(EnvOverrides()) != 4 {
		t.Fatalf("EnvOverrides = %v", EnvOverrides())
	}
}

func TestLoadFileIgnoresEnv(t *testing.T) {
	isolate(t)
	if err := Save(&Config{AccessKey: "file-ak", SecretKey: "file-sk"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvAccessKey, "env-ak")
	cfg, err := LoadFile()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccessKey != "file-ak" {
		t.Fatalf("LoadFile picked up env: %q", cfg.AccessKey)
	}
}

func TestLoadRequiresCredentials(t *testing.T) {
	isolate(t)
	if _, err := Load(); err == nil {
		t.Fatal("expected an error when no credentials are configured")
	}
}

func TestMask(t *testing.T) {
	if got := MaskAccessKey("1234567890"); got != "1234****7890" {
		t.Fatalf("mask = %q", got)
	}
	if got := MaskAccessKey("short"); got != "****" {
		t.Fatalf("short mask = %q", got)
	}
	if got := MaskSecret("anything"); got != "****" {
		t.Fatalf("secret mask = %q", got)
	}
}

func TestSaveOmitsDefaultBaseURL(t *testing.T) {
	path := isolate(t)
	if err := Save(&Config{AccessKey: "a", SecretKey: "s", BaseURL: DefaultBaseURL}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "baseUrl") {
		t.Fatalf("default base URL should not be persisted: %s", data)
	}
}
