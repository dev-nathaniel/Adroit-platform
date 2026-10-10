package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/dev-nathaniel/Adroit-platform/internal/cli/archive"
	"github.com/dev-nathaniel/Adroit-platform/internal/cli/config"
	"github.com/dev-nathaniel/Adroit-platform/internal/cli/ui"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize an Adroit project in the current directory",
	Long:  "Analyzes local source files, detects runtime framework, and creates a local adroit.yaml configuration file.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		// Check if already initialized
		targetFile := filepath.Join(cwd, config.ProjectFileName)
		if _, err := os.Stat(targetFile); err == nil {
			overwrite, err := ui.PromptConfirm(fmt.Sprintf("%s already exists. Overwrite?", config.ProjectFileName), false)
			if err != nil || !overwrite {
				fmt.Println(ui.Warning("Initialization cancelled."))
				return nil
			}
		}

		// Detect application runtime
		detected := archive.DetectRuntime(cwd)
		defaultName := filepath.Base(cwd)

		fmt.Println()
		fmt.Println(ui.StylePrimary.Render("Adroit Project Setup"))
		fmt.Printf("  Detected Runtime: %s (default port: %d)\n\n", ui.StyleSuccess.Render(detected.Name), detected.DefaultPort)

		name, err := ui.PromptInput("Service Name", defaultName, defaultName)
		if err != nil {
			return err
		}

		portStr, err := ui.PromptInput("Service HTTP Port", strconv.Itoa(detected.DefaultPort), strconv.Itoa(detected.DefaultPort))
		if err != nil {
			return err
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			port = detected.DefaultPort
		}

		pcfg := config.DefaultProjectConfig(name, port)

		// Create service in Adroit backend if authenticated
		if Client().Token != "" && ActiveOrgID() != "" {
			createRemote, _ := ui.PromptConfirm("Create remote service on Adroit platform now?", true)
			if createRemote {
				req := map[string]any{
					"organization_id": ActiveOrgID(),
					"name":            name,
					"type":            "web",
					"source_type":     "archive",
					"port":            port,
				}
				svc, err := Client().CreateService(cmd.Context(), req)
				if err != nil {
					fmt.Println(ui.Warning(fmt.Sprintf("Could not create remote service automatically: %v", err)))
					fmt.Println(ui.Info("Continuing with local config creation..."))
				} else {
					pcfg.Service.ID = svc.ID
					fmt.Println(ui.Success(fmt.Sprintf("Created remote service ID: %s", ui.StyleBold.Render(svc.ID))))
				}
			}
		}

		if err := config.SaveProjectConfig(targetFile, pcfg); err != nil {
			return fmt.Errorf("failed to write %s: %w", config.ProjectFileName, err)
		}

		fmt.Println()
		fmt.Println(ui.Success(fmt.Sprintf("Initialized Adroit project in %s", ui.StyleBold.Render(targetFile))))
		fmt.Println(ui.Info("Ready to deploy! Run: " + ui.StyleBold.Render("adroit deploy")))
		fmt.Println()
		return nil
	},
}
