package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/dev-nathaniel/Adroit-platform/internal/cli/ui"
)

var flagProjectDesc string

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage multi-tenant projects within an organization",
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all projects in the active organization",
	RunE: func(cmd *cobra.Command, args []string) error {
		orgID := ActiveOrgID()
		if orgID == "" {
			return fmt.Errorf("no organization selected. Use '--org <id>' or login")
		}

		projects, err := Client().ListProjects(cmd.Context(), orgID)
		if err != nil {
			return fmt.Errorf("failed to list projects: %w", err)
		}

		if IsJSON() {
			return ui.PrintJSON(os.Stdout, projects)
		}

		if len(projects) == 0 {
			fmt.Println(ui.Info("No projects found. Create one with 'adroit project create <name>'."))
			return nil
		}

		table := ui.NewTable("PROJECT ID", "NAME", "SERVICES", "DESCRIPTION", "CREATED")
		for _, p := range projects {
			desc := p.Description
			if desc == "" {
				desc = "-"
			}
			table.AddRow(
				p.ID,
				ui.StyleBold.Render(p.Name),
				strconv.Itoa(p.ServiceCount),
				desc,
				p.CreatedAt.Format("2006-01-02"),
			)
		}

		fmt.Println(ui.StyleHeader.Render("Projects"))
		table.Render(os.Stdout)
		return nil
	},
}

var projectCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		orgID := ActiveOrgID()
		if orgID == "" {
			return fmt.Errorf("no organization selected. Use '--org <id>' or login")
		}

		name := args[0]
		project, err := Client().CreateProject(cmd.Context(), orgID, name, flagProjectDesc)
		if err != nil {
			return fmt.Errorf("failed to create project: %w", err)
		}

		if IsJSON() {
			return ui.PrintJSON(os.Stdout, project)
		}

		fmt.Println(ui.Success(fmt.Sprintf("Created project %s (%s)", ui.StyleBold.Render(project.Name), project.ID)))
		return nil
	},
}

var projectDeleteCmd = &cobra.Command{
	Use:   "delete [project-id]",
	Short: "Delete a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectID := args[0]
		confirm, _ := ui.PromptConfirm(fmt.Sprintf("Are you sure you want to delete project %s?", projectID), false)
		if !confirm {
			fmt.Println(ui.Warning("Deletion cancelled."))
			return nil
		}

		if err := Client().DeleteProject(cmd.Context(), projectID); err != nil {
			return fmt.Errorf("failed to delete project: %w", err)
		}

		fmt.Println(ui.Success(fmt.Sprintf("Deleted project %s", projectID)))
		return nil
	},
}

func init() {
	projectCreateCmd.Flags().StringVar(&flagProjectDesc, "desc", "", "Optional project description")

	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectCreateCmd)
	projectCmd.AddCommand(projectDeleteCmd)
}
