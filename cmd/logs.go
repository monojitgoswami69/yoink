package cmd

import (
	"context"
	"fmt"
	"os"

	"yoink/internal/project"
	"yoink/internal/ui"

	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs <project> [service]",
	Short: "Show service logs",
	Long: `Show docker compose logs for a project. Pass a service name to scope to
one service. Use --follow for a live tail.

Flags:
  --follow     Stream logs live (ctrl-C to stop)
  --tail N     Number of lines to show (default 200)`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runLogs(cmd, args); err != nil {
			fmt.Println(ui.ErrorBox.Render("Error: " + err.Error()))
			os.Exit(1)
		}
	},
}

var (
	logsFollow bool
	logsTail   int
)

func init() {
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().IntVarP(&logsTail, "tail", "t", 200, "Number of lines to show")
}

func runLogs(cmd *cobra.Command, args []string) error {
	io := &initIO{verbose: GetVerbose(cmd), quiet: GetQuiet(cmd)}
	if err := requireDocker(); err != nil {
		return err
	}
	p, err := project.Resolve(args[0])
	if err != nil {
		return err
	}
	service := ""
	if len(args) > 1 {
		service = args[1]
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	label := p.Name
	if service != "" {
		label = service
	}
	if !io.quiet {
		fmt.Println(ui.ProjectHeader(label, ""))
		if logsFollow {
			fmt.Println("  " + ui.MutedStyle.Render(ui.SymWork+" following") + "\n")
		} else {
			fmt.Println()
		}
	}

	if logsFollow {
		return p.Compose.Follow(ctx, service, logsTail)
	}

	out, err := p.Compose.Logs(ctx, service, logsTail)
	if err != nil {
		return fmt.Errorf("logs: %w", err)
	}
	if !io.quiet && out == "" {
		fmt.Println(ui.MutedStyle.Render("  No logs yet."))
		return nil
	}
	fmt.Print(out)
	return nil
}
