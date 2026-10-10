package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/dev-nathaniel/Adroit-platform/internal/cli/client"
	"github.com/dev-nathaniel/Adroit-platform/internal/cli/ui"
)

var (
	flagLogsTail   int
	flagLogsStream string
	flagLogsSearch string
	flagLogsFollow bool
)

var logsCmd = &cobra.Command{
	Use:   "logs [service-id]",
	Short: "Stream real-time container and application logs",
	RunE: func(cmd *cobra.Command, args []string) error {
		svcID := ""
		if len(args) > 0 {
			svcID = args[0]
		} else if Project() != nil && Project().Service.ID != "" {
			svcID = Project().Service.ID
		} else {
			return fmt.Errorf("service ID is required or run inside an adroit project directory")
		}

		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		opts := client.LogStreamOptions{
			Tail:   flagLogsTail,
			Stream: flagLogsStream,
			Search: flagLogsSearch,
		}

		if !flagQuiet {
			fmt.Println(ui.StyleHeader.Render(fmt.Sprintf("Streaming logs for %s (Press Ctrl+C to exit)", svcID)))
		}

		err := Client().StreamLogs(ctx, svcID, opts, func(entry client.LogEntry) {
			if IsJSON() {
				_ = ui.PrintJSON(os.Stdout, entry)
				return
			}

			timeStr := entry.Timestamp.Format("15:04:05.000")
			streamLabel := entry.Stream
			if entry.Stream == "stderr" {
				streamLabel = ui.StyleError.Render("ERR")
			} else {
				streamLabel = ui.StyleMuted.Render("OUT")
			}

			fmt.Printf("%s [%s] %s\n", ui.StyleMuted.Render(timeStr), streamLabel, entry.Message)
		})

		if err != nil && err != ctx.Err() {
			return fmt.Errorf("log stream terminated: %w", err)
		}

		return nil
	},
}

func init() {
	logsCmd.Flags().IntVarP(&flagLogsTail, "tail", "n", 50, "Number of historical lines to fetch")
	logsCmd.Flags().StringVar(&flagLogsStream, "stream", "all", "Filter stream (stdout, stderr, all)")
	logsCmd.Flags().StringVar(&flagLogsSearch, "search", "", "Filter logs matching text keyword")
	logsCmd.Flags().BoolVarP(&flagLogsFollow, "follow", "f", true, "Continuously stream new incoming logs")
}
