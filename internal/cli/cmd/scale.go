package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dev-nathaniel/Adroit-platform/internal/cli/ui"
)

var flagScaleReplicas int

var scaleCmd = &cobra.Command{
	Use:   "scale [service-id]",
	Short: "Adjust the replica count of a service",
	RunE: func(cmd *cobra.Command, args []string) error {
		svcID := ""
		if len(args) > 0 {
			svcID = args[0]
		} else if Project() != nil && Project().Service.ID != "" {
			svcID = Project().Service.ID
		} else {
			return fmt.Errorf("service ID is required or run inside an adroit project directory")
		}

		if flagScaleReplicas < 0 {
			return fmt.Errorf("replicas cannot be negative")
		}

		if err := Client().ScaleService(cmd.Context(), svcID, flagScaleReplicas); err != nil {
			return fmt.Errorf("failed to scale service: %w", err)
		}

		if IsJSON() {
			return ui.PrintJSON(os.Stdout, map[string]any{
				"service_id": svcID,
				"replicas":   flagScaleReplicas,
				"status":     "scaled",
			})
		}

		fmt.Println(ui.Success(fmt.Sprintf("Scaled service %s to %d replicas", ui.StyleBold.Render(svcID), flagScaleReplicas)))
		return nil
	},
}

func init() {
	scaleCmd.Flags().IntVarP(&flagScaleReplicas, "replicas", "r", 1, "Desired number of running replicas")
	_ = scaleCmd.MarkFlagRequired("replicas")
}
