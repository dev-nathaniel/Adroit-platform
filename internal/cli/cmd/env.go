package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dev-nathaniel/Adroit-platform/internal/cli/client"
	"github.com/dev-nathaniel/Adroit-platform/internal/cli/ui"
)

var (
	flagEnvSecret bool
	flagEnvScope  string
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage environment variables and Vault-backed secrets",
}

func resolveServiceID(args []string) (string, []string, error) {
	if len(args) > 0 && strings.HasPrefix(args[0], "svc_") {
		return args[0], args[1:], nil
	}
	if Project() != nil && Project().Service.ID != "" {
		return Project().Service.ID, args, nil
	}
	if len(args) > 0 {
		return args[0], args[1:], nil
	}
	return "", nil, fmt.Errorf("service ID is required or run inside an adroit project directory")
}

var envListCmd = &cobra.Command{
	Use:   "list [service-id]",
	Short: "List environment variables for a service",
	RunE: func(cmd *cobra.Command, args []string) error {
		svcID, _, err := resolveServiceID(args)
		if err != nil {
			return err
		}

		vars, err := Client().GetServiceEnv(cmd.Context(), svcID)
		if err != nil {
			return fmt.Errorf("failed to fetch environment variables: %w", err)
		}

		if IsJSON() {
			return ui.PrintJSON(os.Stdout, vars)
		}

		if len(vars) == 0 {
			fmt.Println(ui.Info("No environment variables found."))
			return nil
		}

		table := ui.NewTable("KEY", "VALUE", "SECRET", "SCOPE")
		for _, v := range vars {
			val := v.Value
			isSecretStr := "false"
			if v.IsSecret {
				isSecretStr = ui.StylePrimary.Render("true (Vault)")
				if val == "[vault:secret]" {
					val = "••••••••"
				}
			}
			table.AddRow(ui.StyleBold.Render(v.Key), val, isSecretStr, v.Scope)
		}

		fmt.Println(ui.StyleHeader.Render(fmt.Sprintf("Environment Variables (%s)", svcID)))
		table.Render(os.Stdout)
		return nil
	},
}

var envSetCmd = &cobra.Command{
	Use:   "set [service-id] KEY=VALUE [KEY=VALUE...]",
	Short: "Set environment variables or secrets",
	RunE: func(cmd *cobra.Command, args []string) error {
		svcID, remaining, err := resolveServiceID(args)
		if err != nil {
			return err
		}

		if len(remaining) == 0 {
			return fmt.Errorf("specify at least one KEY=VALUE pair to set")
		}

		var newVars []client.EnvVar
		for _, arg := range remaining {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid environment variable format: %s (expected KEY=VALUE)", arg)
			}
			key := strings.TrimSpace(parts[0])
			val := parts[1]

			newVars = append(newVars, client.EnvVar{
				Key:      key,
				Value:    val,
				IsSecret: flagEnvSecret,
				Scope:    flagEnvScope,
			})
		}

		res, err := Client().SetServiceEnv(cmd.Context(), svcID, newVars)
		if err != nil {
			return fmt.Errorf("failed to update environment variables: %w", err)
		}

		if IsJSON() {
			return ui.PrintJSON(os.Stdout, res)
		}

		for _, v := range newVars {
			secretTag := ""
			if v.IsSecret {
				secretTag = " [Vault Secret]"
			}
			fmt.Println(ui.Success(fmt.Sprintf("Set %s%s (scope: %s)", ui.StyleBold.Render(v.Key), secretTag, v.Scope)))
		}

		return nil
	},
}

var envUnsetCmd = &cobra.Command{
	Use:   "unset [service-id] KEY",
	Short: "Delete an environment variable from a service",
	RunE: func(cmd *cobra.Command, args []string) error {
		svcID, remaining, err := resolveServiceID(args)
		if err != nil {
			return err
		}

		if len(remaining) == 0 {
			return fmt.Errorf("specify the variable KEY to delete")
		}

		key := remaining[0]
		if err := Client().DeleteServiceEnv(cmd.Context(), svcID, key); err != nil {
			return fmt.Errorf("failed to delete variable %s: %w", key, err)
		}

		fmt.Println(ui.Success(fmt.Sprintf("Removed %s from service %s", ui.StyleBold.Render(key), svcID)))
		return nil
	},
}

var envRevealCmd = &cobra.Command{
	Use:   "reveal [service-id] KEY",
	Short: "Reveal the unmasked plaintext value of a Vault-backed secret",
	RunE: func(cmd *cobra.Command, args []string) error {
		svcID, remaining, err := resolveServiceID(args)
		if err != nil {
			return err
		}

		if len(remaining) == 0 {
			return fmt.Errorf("specify the secret KEY to reveal")
		}

		key := remaining[0]
		val, err := Client().RevealServiceSecret(cmd.Context(), svcID, key)
		if err != nil {
			return fmt.Errorf("failed to reveal secret: %w", err)
		}

		if IsJSON() {
			return ui.PrintJSON(os.Stdout, map[string]string{"key": key, "value": val})
		}

		fmt.Println()
		fmt.Printf("Secret %s: %s\n\n", ui.StyleBold.Render(key), ui.StyleSuccess.Render(val))
		return nil
	},
}

func init() {
	envSetCmd.Flags().BoolVar(&flagEnvSecret, "secret", false, "Store value securely in HashiCorp Vault KV v2")
	envSetCmd.Flags().StringVar(&flagEnvScope, "scope", "runtime", "Scope of variable (runtime, build, all)")

	envCmd.AddCommand(envListCmd)
	envCmd.AddCommand(envSetCmd)
	envCmd.AddCommand(envUnsetCmd)
	envCmd.AddCommand(envRevealCmd)
}
