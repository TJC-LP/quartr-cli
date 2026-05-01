package cli

import (
	"os"
	"strings"

	"quartr-cli/internal/quartr"
)

type globalOverrides struct {
	APIKey   *string
	BaseURL  *string
	Config   *string
	Format   *string
	Timeout  *string
	NoConfig bool
	Debug    bool
	Help     bool
	Version  bool
	RawArgs  []string
	Stripped []string
}

type effectiveConfig struct {
	apiKey     string
	baseURL    string
	format     string
	timeout    string
	configPath string
	noConfig   bool
	debug      bool
}

func defaultEffectiveConfig(g globalOverrides) effectiveConfig {
	cfg := effectiveConfig{
		baseURL:    quartr.DefaultBaseURL,
		format:     "table",
		timeout:    "30s",
		configPath: quartr.DefaultConfigPath(),
		noConfig:   g.NoConfig,
		debug:      g.Debug,
	}
	if g.Config != nil && strings.TrimSpace(*g.Config) != "" {
		cfg.configPath = *g.Config
	}
	return cfg
}

func buildConfig(g globalOverrides) (effectiveConfig, error) {
	cfg := defaultEffectiveConfig(g)
	if !cfg.noConfig {
		fileCfg, err := quartr.LoadConfig(cfg.configPath)
		if err != nil {
			return cfg, err
		}
		cfg.applyFileConfig(fileCfg)
	}
	cfg.applyEnv()
	cfg.applyOverrides(g)
	return cfg, nil
}

func (c *effectiveConfig) applyFileConfig(fileCfg quartr.Config) {
	if fileCfg.APIKey != "" {
		c.apiKey = fileCfg.APIKey
	}
	if fileCfg.BaseURL != "" {
		c.baseURL = fileCfg.BaseURL
	}
	if fileCfg.Format != "" {
		c.format = fileCfg.Format
	}
	if fileCfg.Timeout != "" {
		c.timeout = fileCfg.Timeout
	}
}

func (c *effectiveConfig) applyEnv() {
	if v := os.Getenv("QUARTR_API_KEY"); v != "" {
		c.apiKey = v
	}
	if v := os.Getenv("QUARTR_BASE_URL"); v != "" {
		c.baseURL = v
	}
	if v := os.Getenv("QUARTR_FORMAT"); v != "" {
		c.format = v
	}
	if v := os.Getenv("QUARTR_TIMEOUT"); v != "" {
		c.timeout = v
	}
}

func (c *effectiveConfig) applyOverrides(g globalOverrides) {
	if g.APIKey != nil {
		c.apiKey = *g.APIKey
	}
	if g.BaseURL != nil {
		c.baseURL = *g.BaseURL
	}
	if g.Format != nil {
		c.format = *g.Format
	}
	if g.Timeout != nil {
		c.timeout = *g.Timeout
	}
}

func (c effectiveConfig) APIKey() string {
	return c.apiKey
}

func (c effectiveConfig) BaseURL() string {
	return c.baseURL
}

func (c effectiveConfig) Format() string {
	return c.format
}

func (c effectiveConfig) Timeout() string {
	return c.timeout
}

func (c effectiveConfig) ConfigPath() string {
	return c.configPath
}

func (c effectiveConfig) Debug() bool {
	return c.debug
}

func (c effectiveConfig) newClient() *quartr.Client {
	return quartr.NewClient(c.baseURL, c.apiKey, quartr.EffectiveTimeout(c.timeout), c.debug)
}
