package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dev-nathaniel/Adroit-platform/internal/cli/ui"
)

var rollbackCmd = &cobra.Command{
	Use:   "rollback [deployment-id]",
	Short: "Roll back to a previous historical deployment release",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		depID := args[0]
		confirm, _ := ui.PromptConfirm(fmt.Sprintf("Are you sure you want to rollback to deployment %s?", depID), true)
		if !confirm {
			fmt.Println(ui.Warning("Rollback cancelled."))
			return nil
		}

		newDep, err := Client().RollbackDeployment(cmd.Context(), depID)
		if err != nil {
			return fmt.Errorf("failed to execute rollback: %w", err)
		}

		if IsJSON() {
			return ui.PrintJSON(os.Stdout, newDep)
		}

		fmt.Println(ui.Success(fmt.Sprintf("Rollback initiated! New deployment ID: %s", ui.StyleBold.Render(newDep.ID))))
		return nil
	},
}
