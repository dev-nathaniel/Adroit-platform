package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/dev-nathaniel/Adroit-platform/internal/cli/ui"
)

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "List active workloads, services, and their rollout status",
	RunE: func(cmd *cobra.Command, args []string) error {
		orgID := ActiveOrgID()
		if orgID == "" {
			return fmt.Errorf("no active organization selected. Run 'adroit auth whoami' or provide '--org <id>'")
		}

		services, err := Client().ListServices(cmd.Context(), orgID, "")
		if err != nil {
			return fmt.Errorf("failed to retrieve services: %w", err)
		}

		if IsJSON() {
			return ui.PrintJSON(os.Stdout, services)
		}

		if len(services) == 0 {
			fmt.Println(ui.Info("No services found in this workspace. Run 'adroit init' to get started."))
			return nil
		}

		table := ui.NewTable("SERVICE ID", "NAME", "TYPE", "SOURCE", "PORT", "REPLICAS", "URL")
		for _, svc := range services {
			url := svc.ActiveURL
			if url == "" {
				url = "-"
			}
			table.AddRow(
				svc.ID,
				ui.StyleBold.Render(svc.Name),
				svc.Type,
				svc.SourceType,
				strconv.Itoa(svc.Port),
				strconv.Itoa(svc.CurrentReplicas),
				url,
			)
		}

		fmt.Println(ui.StyleHeader.Render("Active Workloads"))
		table.Render(os.Stdout)
		return nil
	},
}
