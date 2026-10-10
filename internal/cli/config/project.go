package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ProjectFileName is the standard configuration filename for Adroit projects.
const ProjectFileName = "adroit.yaml"

// ProjectConfig represents the local project definition stored in adroit.yaml.
type ProjectConfig struct {
	Project ProjectDetails `yaml:"project"`
	Service ServiceDetails `yaml:"service"`
	Deploy  DeployDetails  `yaml:"deploy"`
}

// ProjectDetails holds the parent project identifiers.
type ProjectDetails struct {
	ID   string `yaml:"id,omitempty"`
	Name string `yaml:"name,omitempty"`
}

// ServiceDetails holds service configuration.
type ServiceDetails struct {
	ID         string `yaml:"id,omitempty"`
	Name       string `yaml:"name,omitempty"`
	Type       string `yaml:"type,omitempty"`
	Port       int    `yaml:"port,omitempty"`
	SourceType string `yaml:"source_type,omitempty"` // "archive", "git", "image"
	ImageURL   string `yaml:"image_url,omitempty"`
	RepoURL    string `yaml:"repo_url,omitempty"`
}

// DeployDetails specifies default deployment options.
type DeployDetails struct {
	Strategy string   `yaml:"strategy,omitempty"` // "rolling", "canary", "blue-green"
	Ignore   []string `yaml:"ignore,omitempty"`
}

// DefaultProjectConfig creates a template project config.
func DefaultProjectConfig(serviceName string, port int) *ProjectConfig {
	return &ProjectConfig{
		Service: ServiceDetails{
			Name:       serviceName,
			Type:       "web",
			Port:       port,
			SourceType: "archive",
		},
		Deploy: DeployDetails{
			Strategy: "rolling",
			Ignore: []string{
				".git",
				"node_modules",
				"vendor",
				".env*",
				"dist",
				"*.tmp",
				"*.log",
			},
		},
	}
}

// FindProjectConfig searches current directory and its parent directories for adroit.yaml.
func FindProjectConfig(startDir string) (*ProjectConfig, string, error) {
	curr := startDir
	for {
		candidate := filepath.Join(curr, ProjectFileName)
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			data, err := os.ReadFile(candidate)
			if err != nil {
				return nil, "", fmt.Errorf("failed to read %s: %w", candidate, err)
			}
			var pcfg ProjectConfig
			if err := yaml.Unmarshal(data, &pcfg); err != nil {
				return nil, "", fmt.Errorf("failed to parse %s: %w", candidate, err)
			}
			return &pcfg, candidate, nil
		}

		parent := filepath.Dir(curr)
		if parent == curr {
			break // Reached root directory
		}
		curr = parent
	}

	return nil, "", os.ErrNotExist
}

// SaveProjectConfig writes the project configuration to the specified path.
func SaveProjectConfig(filePath string, pcfg *ProjectConfig) error {
	data, err := yaml.Marshal(pcfg)
	if err != nil {
		return fmt.Errorf("failed to serialize project config: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write project config file %s: %w", filePath, err)
	}

	return nil
}
