package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dev-nathaniel/Adroit-platform/internal/cli/client"
	"github.com/dev-nathaniel/Adroit-platform/internal/cli/config"
	"github.com/dev-nathaniel/Adroit-platform/internal/cli/ui"
)

var (
	flagAPIURL   string
	flagToken    string
	flagOrgID    string
	flagContext  string
	flagJSON     bool
	flagQuiet    bool
	flagVerbose  bool
	flagNoColor  bool

	cfg          *config.Config
	apiClient    *client.Client
	projectCfg   *config.ProjectConfig
	projectPath  string
)

// RootCmd is the base command when called without any subcommands.
var RootCmd = &cobra.Command{
	Use:   "adroit",
	Short: "Adroit — Developer Cloud Platform CLI",
	Long: ui.Banner() + `
Adroit is an enterprise-grade Cloud Platform-as-a-Service (PaaS).
Deploy web applications, background workers, and containerized workloads
instantly to HashiCorp Nomad with zero infrastructure overhead.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// 1. Load user configuration
		var err error
		cfg, err = config.Load()
		if err != nil {
			return fmt.Errorf("failed to load CLI configuration: %w", err)
		}

		if flagContext != "" {
			cfg.CurrentContext = flagContext
		}

		activeCtx := cfg.ActiveContext()

		// 2. Resolve flags and overrides
		apiURL := activeCtx.APIURL
		if flagAPIURL != "" {
			apiURL = flagAPIURL
		}

		token := activeCtx.Token
		if flagToken != "" {
			token = flagToken
		}

		// 3. Initialize API client
		apiClient = client.New(apiURL, token)

		// 4. Inspect current directory for adroit.yaml
		cwd, err := os.Getwd()
		if err == nil {
			pcfg, ppath, err := config.FindProjectConfig(cwd)
			if err == nil {
				projectCfg = pcfg
				projectPath = ppath
			}
		}

		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return RootCmd.ExecuteContext(context.Background())
}

func init() {
	RootCmd.PersistentFlags().StringVar(&flagAPIURL, "api-url", "", "Override API Gateway URL (default: http://localhost:8080)")
	RootCmd.PersistentFlags().StringVar(&flagToken, "token", "", "Override JWT/PAT authentication token")
	RootCmd.PersistentFlags().StringVar(&flagOrgID, "org", "", "Target organization ID")
	RootCmd.PersistentFlags().StringVar(&flagContext, "context", "", "CLI configuration context profile (default: local)")
	RootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output machine-readable JSON")
	RootCmd.PersistentFlags().BoolVarP(&flagQuiet, "quiet", "q", false, "Suppress non-essential output and banners")
	RootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "Enable verbose debug logging")
	RootCmd.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "Disable ANSI color formatting")

	// Register subcommands
	RootCmd.AddCommand(versionCmd)
	RootCmd.AddCommand(authCmd)
	RootCmd.AddCommand(initCmd)
	RootCmd.AddCommand(psCmd)
	RootCmd.AddCommand(projectCmd)
	RootCmd.AddCommand(serviceCmd)
	RootCmd.AddCommand(deployCmd)
	RootCmd.AddCommand(logsCmd)
	RootCmd.AddCommand(envCmd)
	RootCmd.AddCommand(scaleCmd)
	RootCmd.AddCommand(rollbackCmd)
	RootCmd.AddCommand(domainCmd)
	RootCmd.AddCommand(completionCmd)
}

// Client returns the configured API client.
func Client() *client.Client {
	return apiClient
}

// Config returns the global CLI configuration.
func Config() *config.Config {
	return cfg
}

// Project returns the detected project config, or nil if unlinked.
func Project() *config.ProjectConfig {
	return projectCfg
}

// IsJSON returns true if machine JSON output was requested.
func IsJSON() bool {
	return flagJSON
}

// ActiveOrgID resolves target organization ID from flag, project, or active context.
func ActiveOrgID() string {
	if flagOrgID != "" {
		return flagOrgID
	}
	if cfg != nil && cfg.ActiveContext().ActiveOrgID != "" {
		return cfg.ActiveContext().ActiveOrgID
	}
	return ""
}
