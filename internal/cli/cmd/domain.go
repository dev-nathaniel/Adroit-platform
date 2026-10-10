package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dev-nathaniel/Adroit-platform/internal/cli/ui"
)

var domainCmd = &cobra.Command{
	Use:   "domain",
	Short: "Configure custom domains and manage TLS verification",
}

var domainListCmd = &cobra.Command{
	Use:   "list [service-id]",
	Short: "List custom domains attached to a service",
	RunE: func(cmd *cobra.Command, args []string) error {
		svcID, _, err := resolveServiceID(args)
		if err != nil {
			return err
		}

		domains, err := Client().ListDomains(cmd.Context(), svcID)
		if err != nil {
			return fmt.Errorf("failed to fetch domains: %w", err)
		}

		if IsJSON() {
			return ui.PrintJSON(os.Stdout, domains)
		}

		if len(domains) == 0 {
			fmt.Println(ui.Info("No custom domains configured for this service."))
			return nil
		}

		table := ui.NewTable("DOMAIN ID", "DOMAIN", "RECORD TYPE", "TARGET VALUE", "STATUS")
		for _, d := range domains {
			table.AddRow(d.ID, ui.StyleBold.Render(d.Domain), d.DNSRecordType, d.TargetValue, ui.StatusBadge(d.Status))
		}

		fmt.Println(ui.StyleHeader.Render(fmt.Sprintf("Custom Domains (%s)", svcID)))
		table.Render(os.Stdout)
		return nil
	},
}

var domainAddCmd = &cobra.Command{
	Use:   "add [service-id] [domain-name]",
	Short: "Add a custom domain to a service",
	RunE: func(cmd *cobra.Command, args []string) error {
		svcID, remaining, err := resolveServiceID(args)
		if err != nil {
			return err
		}

		if len(remaining) == 0 {
			return fmt.Errorf("domain name is required (e.g. app.mycompany.com)")
		}

		domainName := remaining[0]
		dom, err := Client().AddDomain(cmd.Context(), svcID, domainName)
		if err != nil {
			return fmt.Errorf("failed to add domain: %w", err)
		}

		if IsJSON() {
			return ui.PrintJSON(os.Stdout, dom)
		}

		fmt.Println(ui.Success(fmt.Sprintf("Custom domain %s added!", ui.StyleBold.Render(dom.Domain))))
		fmt.Printf("  Point DNS %s record to: %s\n", dom.DNSRecordType, ui.StylePrimary.Render(dom.TargetValue))
		fmt.Printf("  Verify with: adroit domain verify %s\n", dom.ID)
		return nil
	},
}

var domainVerifyCmd = &cobra.Command{
	Use:   "verify [domain-id]",
	Short: "Trigger DNS challenge verification for a custom domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domainID := args[0]
		if err := Client().VerifyDomain(cmd.Context(), domainID); err != nil {
			return fmt.Errorf("verification failed: %w", err)
		}

		fmt.Println(ui.Success(fmt.Sprintf("Domain %s verified successfully! Ingress routing updated.", domainID)))
		return nil
	},
}

var domainDeleteCmd = &cobra.Command{
	Use:   "remove [domain-id]",
	Short: "Remove a custom domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domainID := args[0]
		if err := Client().DeleteDomain(cmd.Context(), domainID); err != nil {
			return fmt.Errorf("failed to delete domain: %w", err)
		}

		fmt.Println(ui.Success(fmt.Sprintf("Domain %s deleted.", domainID)))
		return nil
	},
}

func init() {
	domainCmd.AddCommand(domainListCmd)
	domainCmd.AddCommand(domainAddCmd)
	domainCmd.AddCommand(domainVerifyCmd)
	domainCmd.AddCommand(domainDeleteCmd)
}
