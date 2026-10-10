package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/dev-nathaniel/Adroit-platform/internal/cli/archive"
	"github.com/dev-nathaniel/Adroit-platform/internal/cli/ui"
)

var (
	flagDeployServiceID string
	flagDeployStrategy  string
	flagDeployReplicas  int
	flagDeployFollow    bool
)

var deployCmd = &cobra.Command{
	Use:   "deploy [directory]",
	Short: "Package and deploy the current application to Adroit PaaS",
	Long: `Packages the specified directory into a normalized archive stream, uploads it to the Adroit platform, 
triggers BuildKit/Nixpacks compilation, and schedules the workload onto HashiCorp Nomad with zero downtime.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}

		// 1. Resolve target service ID
		svcID := flagDeployServiceID
		if svcID == "" && Project() != nil {
			svcID = Project().Service.ID
		}

		if svcID == "" {
			return fmt.Errorf("no linked service found. Run 'adroit init' or specify '--service <id>'")
		}

		// 2. Fetch service details
		svc, err := Client().GetService(cmd.Context(), svcID)
		if err != nil {
			return fmt.Errorf("failed to fetch service %s: %w", svcID, err)
		}

		if !flagQuiet {
			fmt.Println()
			fmt.Printf("🚀 %s %s (%s)\n", ui.StylePrimary.Render("Deploying"), ui.StyleBold.Render(svc.Name), svc.ID)
			fmt.Println()
		}

		var deploymentID string

		// 3. Handle Archive Upload vs Prebuilt Image
		if svc.SourceType == "image" {
			// Direct deployment bypass
			if !flagQuiet {
				fmt.Printf("  %s %s\n", ui.Info("Source type: container image"), svc.ImageURL)
			}
			req := map[string]any{
				"strategy": flagDeployStrategy,
				"replicas": flagDeployReplicas,
			}
			dep, err := Client().Deploy(cmd.Context(), svc.ID, req)
			if err != nil {
				return fmt.Errorf("failed to schedule container image deployment: %w", err)
			}
			deploymentID = dep.ID
		} else {
			// Default: Archive upload mode
			var customIgnores []string
			if Project() != nil {
				customIgnores = Project().Deploy.Ignore
			}

			if !flagQuiet {
				fmt.Printf("  %s Packaging directory %s...\n", ui.StylePrimary.Render("1."), dir)
			}

			packRes, err := archive.PackDirectory(dir, customIgnores)
			if err != nil {
				return fmt.Errorf("failed to package directory: %w", err)
			}

			if !flagQuiet {
				fmt.Println(ui.Success(fmt.Sprintf("Bundled %d files (%.1f KB) [Stack: %s]",
					packRes.FileCount,
					float64(packRes.SizeBytes)/1024.0,
					packRes.RuntimeType.Name,
				)))
				fmt.Printf("  %s Uploading archive to Adroit storage...\n", ui.StylePrimary.Render("2."))
			}

			uploadRes, err := Client().UploadSource(cmd.Context(), svc.ID, "application/gzip", packRes.Buffer, true)
			if err != nil {
				return fmt.Errorf("failed to upload archive: %w", err)
			}

			if !flagQuiet {
				fmt.Println(ui.Success(fmt.Sprintf("Archive ingested (SHA256: %s)", uploadRes.SHA256[:12])))
			}

			deploymentID = uploadRes.DeploymentID
		}

		if IsJSON() {
			return ui.PrintJSON(os.Stdout, map[string]string{
				"service_id":    svc.ID,
				"deployment_id": deploymentID,
				"status":        "initiated",
			})
		}

		// 4. Follow rollout status
		if flagDeployFollow && deploymentID != "" {
			fmt.Printf("  %s Awaiting Nomad scheduler rollout...\n", ui.StylePrimary.Render("3."))

			ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
			defer cancel()

			var finalURL string
			active := false

			for {
				select {
				case <-ctx.Done():
					fmt.Println(ui.Warning("Rollout wait timed out; workload is still progressing in background."))
					return nil
				case <-time.After(1 * time.Second):
					dep, err := Client().GetDeployment(ctx, deploymentID)
					if err == nil {
						if dep.Status == "active" {
							finalURL = dep.ActiveURL
							active = true
							break
						} else if dep.Status == "failed" {
							return fmt.Errorf("deployment failed during rollout")
						}
					}
				}
				if active {
					break
				}
			}

			// Re-fetch service to get ingress domain if not in deployment
			if finalURL == "" {
				updatedSvc, err := Client().GetService(context.Background(), svc.ID)
				if err == nil && updatedSvc.ActiveURL != "" {
					finalURL = updatedSvc.ActiveURL
				}
			}

			fmt.Println()
			fmt.Println(ui.Success("Deployment successfully completed!"))
			if finalURL != "" {
				fmt.Printf("  🌐 Public URL: %s\n", ui.StyleSuccess.Render(finalURL))
			}
			fmt.Println()
		}

		return nil
	},
}

func init() {
	deployCmd.Flags().StringVar(&flagDeployServiceID, "service", "", "Target service ID (defaults to adroit.yaml service ID)")
	deployCmd.Flags().StringVar(&flagDeployStrategy, "strategy", "rolling", "Deployment rollout strategy (rolling, canary, blue-green)")
	deployCmd.Flags().IntVar(&flagDeployReplicas, "replicas", 1, "Desired number of running container replicas")
	deployCmd.Flags().BoolVarP(&flagDeployFollow, "follow", "f", true, "Follow deployment rollout until healthy")
}
