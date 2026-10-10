package cmd

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/dev-nathaniel/Adroit-platform/internal/cli/ui"
)

var (
	// Version can be overwritten during build time using -ldflags
	Version   = "v1.0.0"
	GitCommit = "dev"
	BuildDate = time.Now().Format("2006-01-02")
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information of Adroit CLI and API Gateway",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
		defer cancel()

		var gatewayStatus struct {
			Status  string `json:"status"`
			Service string `json:"service"`
			Version string `json:"version"`
		}

		gwErr := Client().Get(ctx, "/healthz", &gatewayStatus)

		out := cmd.OutOrStdout()
		if IsJSON() {
			return ui.PrintJSON(out, map[string]any{
				"cli_version":  Version,
				"git_commit":   GitCommit,
				"build_date":   BuildDate,
				"go_version":   runtime.Version(),
				"os_arch":      fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
				"gateway_url":  Client().BaseURL,
				"gateway_live": gwErr == nil,
			})
		}

		fmt.Fprintln(out, ui.StylePrimary.Render("Adroit Platform CLI"))
		fmt.Fprintf(out, "  Version:    %s\n", ui.StyleBold.Render(Version))
		fmt.Fprintf(out, "  Commit:     %s\n", GitCommit)
		fmt.Fprintf(out, "  Go Version: %s\n", runtime.Version())
		fmt.Fprintf(out, "  Platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Fprintln(out)

		fmt.Fprintf(out, "  API Gateway: %s\n", Client().BaseURL)
		if gwErr == nil {
			fmt.Fprintf(out, "  Gateway:     %s (%s)\n", ui.StyleSuccess.Render("operational"), gatewayStatus.Service)
		} else {
			fmt.Fprintf(out, "  Gateway:     %s (%v)\n", ui.StyleWarning.Render("unreachable"), gwErr)
		}
		fmt.Fprintln(out)

		return nil
	},
}
