package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TJC-LP/quartr-cli/internal/quartr"
)

func TestBuildConfigPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	err := os.WriteFile(path, []byte(`{
  "api_key": "file-key",
  "base_url": "https://file.example",
  "format": "csv",
  "timeout": "5s"
}`), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("QUARTR_API_KEY", "env-key")
	t.Setenv("QUARTR_BASE_URL", "https://env.example")
	t.Setenv("QUARTR_FORMAT", "json")

	apiKey := "flag-key"
	timeout := "45s"
	cfg, err := buildConfig(globalOverrides{
		APIKey:  &apiKey,
		Config:  &path,
		Timeout: &timeout,
	})
	if err != nil {
		t.Fatal(err)
	}

	if cfg.APIKey() != "flag-key" {
		t.Fatalf("APIKey: expected flag-key, got %q", cfg.APIKey())
	}
	if cfg.BaseURL() != "https://env.example" {
		t.Fatalf("BaseURL: expected env value, got %q", cfg.BaseURL())
	}
	if cfg.Format() != "json" {
		t.Fatalf("Format: expected json, got %q", cfg.Format())
	}
	if cfg.Timeout() != "45s" {
		t.Fatalf("Timeout: expected 45s, got %q", cfg.Timeout())
	}
}

func TestBuildConfigNoConfigSkipsFile(t *testing.T) {
	t.Setenv("QUARTR_API_KEY", "")
	t.Setenv("QUARTR_BASE_URL", "")
	t.Setenv("QUARTR_FORMAT", "")
	t.Setenv("QUARTR_TIMEOUT", "")

	path := filepath.Join(t.TempDir(), "config.json")
	err := os.WriteFile(path, []byte(`{"api_key":"file-key"}`), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := buildConfig(globalOverrides{Config: &path, NoConfig: true})
	if err != nil {
		t.Fatal(err)
	}

	if cfg.APIKey() != "" {
		t.Fatalf("APIKey: expected empty value, got %q", cfg.APIKey())
	}
	if cfg.BaseURL() != quartr.DefaultBaseURL {
		t.Fatalf("BaseURL: expected default, got %q", cfg.BaseURL())
	}
	if cfg.Format() != "table" {
		t.Fatalf("Format: expected table, got %q", cfg.Format())
	}
	if cfg.Timeout() != "30s" {
		t.Fatalf("Timeout: expected 30s, got %q", cfg.Timeout())
	}
	if cfg.ConfigPath() != path {
		t.Fatalf("ConfigPath: expected %q, got %q", path, cfg.ConfigPath())
	}
}
