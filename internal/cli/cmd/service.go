package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dev-nathaniel/Adroit-platform/internal/cli/ui"
)

var (
	flagSvcType       string
	flagSvcSourceType string
	flagSvcPort       int
	flagSvcImageURL   string
	flagSvcRepoURL    string
	flagSvcBranch     string
	flagSvcProjectID  string
)

var serviceCmd = &cobra.Command{
	Use:     "service",
	Aliases: []string{"svc", "app"},
	Short:   "Manage workloads and application services",
}

var serviceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List services in the active workspace",
	RunE: func(cmd *cobra.Command, args []string) error {
		return psCmd.RunE(cmd, args)
	},
}

var serviceInfoCmd = &cobra.Command{
	Use:   "info [service-id]",
	Short: "Display detailed metadata for a service",
	RunE: func(cmd *cobra.Command, args []string) error {
		svcID := ""
		if len(args) > 0 {
			svcID = args[0]
		} else if Project() != nil && Project().Service.ID != "" {
			svcID = Project().Service.ID
		} else {
			return fmt.Errorf("service ID is required or run inside an adroit project directory")
		}

		svc, err := Client().GetService(cmd.Context(), svcID)
		if err != nil {
			return fmt.Errorf("failed to get service: %w", err)
		}

		if IsJSON() {
			return ui.PrintJSON(os.Stdout, svc)
		}

		fmt.Println()
		fmt.Println(ui.StyleHeader.Render("Service Details: " + svc.Name))
		fmt.Println(ui.KeyValue("Service ID", svc.ID))
		fmt.Println(ui.KeyValue("Type", svc.Type))
		fmt.Println(ui.KeyValue("Source Type", svc.SourceType))
		fmt.Println(ui.KeyValue("Port", svc.Port))
		fmt.Println(ui.KeyValue("Replicas", svc.CurrentReplicas))
		if svc.ImageURL != "" {
			fmt.Println(ui.KeyValue("Image URL", svc.ImageURL))
		}
		if svc.RepoURL != "" {
			fmt.Println(ui.KeyValue("Repository", svc.RepoURL))
			fmt.Println(ui.KeyValue("Branch", svc.Branch))
		}
		if svc.ActiveURL != "" {
			fmt.Println(ui.KeyValue("Public URL", ui.StyleSuccess.Render(svc.ActiveURL)))
		}
		fmt.Println(ui.KeyValue("Created", svc.CreatedAt.Format("2006-01-02 15:04:05")))
		fmt.Println()
		return nil
	},
}

var serviceCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new service",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		orgID := ActiveOrgID()
		if orgID == "" {
			return fmt.Errorf("no organization selected. Use '--org <id>' or login")
		}

		name := args[0]
		req := map[string]any{
			"organization_id": orgID,
			"name":            name,
			"type":            flagSvcType,
			"source_type":     flagSvcSourceType,
			"port":            flagSvcPort,
		}
		if flagSvcProjectID != "" {
			req["project_id"] = flagSvcProjectID
		}
		if flagSvcImageURL != "" {
			req["image_url"] = flagSvcImageURL
		}
		if flagSvcRepoURL != "" {
			req["repo_url"] = flagSvcRepoURL
		}
		if flagSvcBranch != "" {
			req["branch"] = flagSvcBranch
		}

		svc, err := Client().CreateService(cmd.Context(), req)
		if err != nil {
			return fmt.Errorf("failed to create service: %w", err)
		}

		if IsJSON() {
			return ui.PrintJSON(os.Stdout, svc)
		}

		fmt.Println(ui.Success(fmt.Sprintf("Created service %s (%s)", ui.StyleBold.Render(svc.Name), svc.ID)))
		fmt.Printf("  Type: %s, Source: %s, Port: %d\n", svc.Type, svc.SourceType, svc.Port)
		return nil
	},
}

var serviceDeleteCmd = &cobra.Command{
	Use:   "delete [service-id]",
	Short: "Delete a service",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svcID := args[0]
		confirm, _ := ui.PromptConfirm(fmt.Sprintf("Are you sure you want to delete service %s?", svcID), false)
		if !confirm {
			fmt.Println(ui.Warning("Deletion cancelled."))
			return nil
		}

		if err := Client().DeleteService(cmd.Context(), svcID); err != nil {
			return fmt.Errorf("failed to delete service: %w", err)
		}

		fmt.Println(ui.Success(fmt.Sprintf("Deleted service %s", svcID)))
		return nil
	},
}

func init() {
	serviceCreateCmd.Flags().StringVar(&flagSvcType, "type", "web", "Service type (web, worker, cron, redis, postgres)")
	serviceCreateCmd.Flags().StringVar(&flagSvcSourceType, "source", "archive", "Source type (archive, git, image)")
	serviceCreateCmd.Flags().IntVar(&flagSvcPort, "port", 8080, "Container listening port")
	serviceCreateCmd.Flags().StringVar(&flagSvcImageURL, "image", "", "OCI container image tag (for source=image)")
	serviceCreateCmd.Flags().StringVar(&flagSvcRepoURL, "repo", "", "Git repository URL (for source=git)")
	serviceCreateCmd.Flags().StringVar(&flagSvcBranch, "branch", "main", "Git branch")
	serviceCreateCmd.Flags().StringVar(&flagSvcProjectID, "project", "", "Parent project ID")

	serviceCmd.AddCommand(serviceListCmd)
	serviceCmd.AddCommand(serviceInfoCmd)
	serviceCmd.AddCommand(serviceCreateCmd)
	serviceCmd.AddCommand(serviceDeleteCmd)
}
