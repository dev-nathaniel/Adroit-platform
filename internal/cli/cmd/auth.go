package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dev-nathaniel/Adroit-platform/internal/cli/config"
	"github.com/dev-nathaniel/Adroit-platform/internal/cli/ui"
)

var (
	flagEmail    string
	flagPassword string
	flagName     string
	flagOrgName  string
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication, login sessions, and API tokens",
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to an Adroit account",
	RunE: func(cmd *cobra.Command, args []string) error {
		email := flagEmail
		password := flagPassword

		if email == "" || password == "" {
			var err error
			email, password, err = ui.PromptLogin()
			if err != nil {
				return fmt.Errorf("login cancelled: %w", err)
			}
		}

		res, err := Client().Login(cmd.Context(), email, password)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}

		// Save token to config
		activeCtx := Config().ActiveContext()
		activeCtx.Token = res.Token
		activeCtx.UserEmail = res.User.Email
		if res.Organization != nil {
			activeCtx.ActiveOrgID = res.Organization.ID
			activeCtx.ActiveOrgName = res.Organization.Name
		}

		if err := config.Save(Config()); err != nil {
			return fmt.Errorf("failed to save credentials: %w", err)
		}

		if IsJSON() {
			return ui.PrintJSON(os.Stdout, res)
		}

		fmt.Println()
		fmt.Println(ui.Success(fmt.Sprintf("Successfully logged in as %s", ui.StyleBold.Render(res.User.Email))))
		if res.Organization != nil {
			fmt.Printf("  Active Workspace: %s (%s)\n", ui.StylePrimary.Render(res.Organization.Name), res.Organization.ID)
		}
		fmt.Println()
		return nil
	},
}

var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "Create a new Adroit account and workspace",
	RunE: func(cmd *cobra.Command, args []string) error {
		name := flagName
		email := flagEmail
		password := flagPassword
		orgName := flagOrgName

		var err error
		if name == "" {
			name, err = ui.PromptInput("Full Name", "Alex Mercer", "")
			if err != nil {
				return err
			}
		}
		if email == "" {
			email, err = ui.PromptInput("Work Email", "alex@company.com", "")
			if err != nil {
				return err
			}
		}
		if password == "" {
			password, err = ui.PromptPassword("Choose Password")
			if err != nil {
				return err
			}
		}
		if orgName == "" {
			orgName, err = ui.PromptInput("Organization / Workspace Name", "My Team", name+" Workspace")
			if err != nil {
				return err
			}
		}

		res, err := Client().Register(cmd.Context(), email, password, name, orgName)
		if err != nil {
			return fmt.Errorf("registration failed: %w", err)
		}

		activeCtx := Config().ActiveContext()
		activeCtx.Token = res.Token
		activeCtx.UserEmail = res.User.Email
		if res.Organization != nil {
			activeCtx.ActiveOrgID = res.Organization.ID
			activeCtx.ActiveOrgName = res.Organization.Name
		}

		if err := config.Save(Config()); err != nil {
			return fmt.Errorf("failed to save credentials: %w", err)
		}

		if IsJSON() {
			return ui.PrintJSON(os.Stdout, res)
		}

		fmt.Println()
		fmt.Println(ui.Success(fmt.Sprintf("Account created successfully! Logged in as %s", ui.StyleBold.Render(res.User.Email))))
		if res.Organization != nil {
			fmt.Printf("  Workspace: %s (%s)\n", ui.StylePrimary.Render(res.Organization.Name), res.Organization.ID)
		}
		fmt.Println()
		return nil
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out and clear stored authentication credentials",
	RunE: func(cmd *cobra.Command, args []string) error {
		activeCtx := Config().ActiveContext()
		activeCtx.Token = ""
		activeCtx.UserEmail = ""

		if err := config.Save(Config()); err != nil {
			return fmt.Errorf("failed to update configuration: %w", err)
		}

		if IsJSON() {
			return ui.PrintJSON(os.Stdout, map[string]string{"status": "logged_out"})
		}

		fmt.Println(ui.Success("Successfully logged out."))
		return nil
	},
}

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Display the currently authenticated user and active context",
	RunE: func(cmd *cobra.Command, args []string) error {
		activeCtx := Config().ActiveContext()
		if activeCtx.Token == "" {
			return fmt.Errorf("not logged in. Run 'adroit auth login' first")
		}

		user, err := Client().Me(cmd.Context())
		if err != nil {
			return fmt.Errorf("failed to verify credentials: %w", err)
		}

		if IsJSON() {
			return ui.PrintJSON(os.Stdout, map[string]any{
				"user":         user,
				"context":      Config().CurrentContext,
				"api_url":      activeCtx.APIURL,
				"organization": activeCtx.ActiveOrgName,
				"org_id":       activeCtx.ActiveOrgID,
			})
		}

		fmt.Println()
		fmt.Println(ui.StylePrimary.Render("Authenticated User"))
		fmt.Println(ui.KeyValue("Email", user.Email))
		fmt.Println(ui.KeyValue("Name", user.Name))
		fmt.Println(ui.KeyValue("User ID", user.ID))
		if activeCtx.ActiveOrgName != "" {
			fmt.Println(ui.KeyValue("Organization", fmt.Sprintf("%s (%s)", activeCtx.ActiveOrgName, activeCtx.ActiveOrgID)))
		}
		fmt.Println(ui.KeyValue("Context", Config().CurrentContext))
		fmt.Println(ui.KeyValue("API Target", activeCtx.APIURL))
		fmt.Println()
		return nil
	},
}

func init() {
	loginCmd.Flags().StringVar(&flagEmail, "email", "", "Account email address")
	loginCmd.Flags().StringVar(&flagPassword, "password", "", "Account password")

	registerCmd.Flags().StringVar(&flagName, "name", "", "Full Name")
	registerCmd.Flags().StringVar(&flagEmail, "email", "", "Account email address")
	registerCmd.Flags().StringVar(&flagPassword, "password", "", "Account password")
	registerCmd.Flags().StringVar(&flagOrgName, "org-name", "", "Organization workspace name")

	authCmd.AddCommand(loginCmd)
	authCmd.AddCommand(registerCmd)
	authCmd.AddCommand(logoutCmd)
	authCmd.AddCommand(whoamiCmd)
}
