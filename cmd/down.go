package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"yoink/internal/project"
	"yoink/internal/ui"

	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down [project]",
	Short: "Stop a project",
	Long: `Stop a previously-started project without removing its data.

When no project is given, the most recently initialised one is used.

Flags:
  --volumes   Also remove named volumes (DESTRUCTIVE — database data, etc.)`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runDown(cmd, args); err != nil {
			fmt.Println(ui.ErrorBox.Render("Error: " + err.Error()))
			os.Exit(1)
		}
	},
}

var downVolumes bool

func init() {
	downCmd.Flags().BoolVar(&downVolumes, "volumes", false, "Also remove named volumes (DESTRUCTIVE)")
}

func runDown(cmd *cobra.Command, args []string) error {
	io := &initIO{verbose: GetVerbose(cmd), quiet: GetQuiet(cmd)}
	if err := requireDocker(); err != nil {
		return err
	}
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	p, err := project.Resolve(name)
	if err != nil {
		return err
	}
	if !io.quiet {
		fmt.Println(ui.ProjectHeader(p.Name, ""))
		fmt.Println("\n  " + ui.MutedStyle.Render("Stopping…"))
	}
	if downVolumes {
		fmt.Println(ui.ErrorStyle.Render("  ⚠ WARNING: --volumes will permanently delete all data volumes."))
		fmt.Print("  Continue? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y") {
			io.info("Aborted.")
			return nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := p.Compose.Down(ctx, downVolumes)
	if io.verbose && strings.TrimSpace(out) != "" {
		fmt.Println(ui.DimStyle.Render(strings.TrimRight(out, "\n")))
	}
	if err != nil {
		return fmt.Errorf("compose down failed: %w", err)
	}
	msg := ui.SymDone + " Stopped"
	if downVolumes {
		msg = ui.SymDone + " Stopped (volumes deleted)"
	}
	fmt.Println("\n  " + ui.SuccessStyle.Render(msg) + "  " + ui.MutedStyle.Render(p.Name))
	if !io.quiet {
		fmt.Println(ui.DimStyle.Render("  Restart with: yoink up " + p.Name))
	}
	return nil
}
