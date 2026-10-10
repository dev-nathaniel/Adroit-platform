package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultContextName is the fallback profile context name.
const DefaultContextName = "local"

// DefaultLocalAPIURL is the default API Gateway target for local development.
const DefaultLocalAPIURL = "http://localhost:8080"

// Config represents the global user-level CLI configuration stored at ~/.adroit/config.yaml.
type Config struct {
	Version        string              `yaml:"version"`
	CurrentContext string              `yaml:"current_context"`
	Contexts       map[string]*Context `yaml:"contexts"`
}

// Context contains API endpoints and credentials for a specific environment.
type Context struct {
	APIURL        string `yaml:"api_url"`
	Token         string `yaml:"token,omitempty"`
	ActiveOrgID   string `yaml:"active_org_id,omitempty"`
	ActiveOrgName string `yaml:"active_org_name,omitempty"`
	UserEmail     string `yaml:"user_email,omitempty"`
}

// DefaultConfig returns a newly initialized Config with local development defaults.
func DefaultConfig() *Config {
	return &Config{
		Version:        "1",
		CurrentContext: DefaultContextName,
		Contexts: map[string]*Context{
			DefaultContextName: {
				APIURL: DefaultLocalAPIURL,
			},
		},
	}
}

// ConfigDir returns the directory path for Adroit CLI configuration.
func ConfigDir() (string, error) {
	if custom := os.Getenv("ADROIT_CONFIG_DIR"); custom != "" {
		return custom, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	return filepath.Join(home, ".adroit"), nil
}

// ConfigFilePath returns the absolute path to ~/.adroit/config.yaml.
func ConfigFilePath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Load reads and parses ~/.adroit/config.yaml, creating default configuration if missing.
func Load() (*Config, error) {
	filePath, err := ConfigFilePath()
	if err != nil {
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config file %s: %w", filePath, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse yaml config %s: %w", filePath, err)
	}

	if cfg.Contexts == nil {
		cfg.Contexts = make(map[string]*Context)
	}
	if cfg.CurrentContext == "" {
		cfg.CurrentContext = DefaultContextName
	}
	if _, exists := cfg.Contexts[cfg.CurrentContext]; !exists {
		cfg.Contexts[cfg.CurrentContext] = &Context{
			APIURL: DefaultLocalAPIURL,
		}
	}

	return cfg, nil
}

// Save writes the Config to ~/.adroit/config.yaml.
func Save(cfg *Config) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory %s: %w", dir, err)
	}

	filePath := filepath.Join(dir, "config.yaml")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", filePath, err)
	}

	return nil
}

// ActiveContext returns the currently active context from the configuration.
func (c *Config) ActiveContext() *Context {
	if c.Contexts == nil {
		c.Contexts = make(map[string]*Context)
	}

	ctx, exists := c.Contexts[c.CurrentContext]
	if !exists || ctx == nil {
		ctx = &Context{
			APIURL: DefaultLocalAPIURL,
		}
		c.Contexts[c.CurrentContext] = ctx
	}

	// Environment variable overrides
	if envAPI := os.Getenv("ADROIT_API_URL"); envAPI != "" {
		ctx.APIURL = envAPI
	}
	if envToken := os.Getenv("ADROIT_TOKEN"); envToken != "" {
		ctx.Token = envToken
	}
	if envOrg := os.Getenv("ADROIT_ORG_ID"); envOrg != "" {
		ctx.ActiveOrgID = envOrg
	}

	return ctx
}
