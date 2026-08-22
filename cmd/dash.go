package cmd

import (
	"context"
	"fmt"
	"os"

	"yoink/internal/dashboard"
	"yoink/internal/project"
	"yoink/internal/ui"

	"github.com/spf13/cobra"
)

var dashCmd = &cobra.Command{
	Use:   "dash [project]",
	Short: "Open the live dashboard for a project",
	Long: `Open a multi-pane TUI dashboard for a previously-initialised project.
The dashboard polls docker compose ps + docker stats, shows logs, and lets
you edit env overrides and control services.

When no project is given, the most recently initialised one is used.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runDash(args); err != nil {
			fmt.Println(ui.ErrorBox.Render("Error: " + err.Error()))
			os.Exit(1)
		}
	},
}

func runDash(args []string) error {
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
	return dashboard.Run(context.Background(), p.Manager, p.Lock)
}
