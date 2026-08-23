package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"yoink/internal/docker"
	"yoink/internal/project"
	"yoink/internal/ui"

	"github.com/spf13/cobra"
)

var restartCmd = &cobra.Command{
	Use:   "restart [project]",
	Short: "Restart a project",
	Long: `Stop and start a project, waiting for healthchecks. More efficient than
running down then up because it keeps the network/volumes intact.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runRestart(cmd, args); err != nil {
			fmt.Println(ui.ErrorBox.Render("Error: " + err.Error()))
			os.Exit(1)
		}
	},
}

func runRestart(cmd *cobra.Command, args []string) error {
	io := &initIO{verbose: GetVerbose(cmd), quiet: GetQuiet(cmd)}
	if !io.quiet {
		fmt.Print(ui.Header(ui.HeaderArgs{Command: "restart", Version: Version}) + "\n\n")
	}
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
		fmt.Printf("Restarting %s...\n", ui.HighlightStyle.Render(p.Name))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if out, err := p.Compose.Down(ctx, false); err != nil {
		return fmt.Errorf("compose down failed: %w: %s", err, strings.TrimSpace(out))
	}
	if err := renderMergedEnvs(p.Manager, p.Lock, io); err != nil {
		return err
	}
	out, err := p.Compose.Up(ctx)
	if io.verbose && strings.TrimSpace(out) != "" {
		fmt.Println(ui.DimStyle.Render(strings.TrimRight(out, "\n")))
	}
	if err != nil {
		return fmt.Errorf("compose up failed: %s\n\nrun `yoink logs %s` or `yoink heal %s`", docker.TailLines(out, 30), p.Name, p.Name)
	}
	if err := waitForHealthy(ctx, p, io); err != nil {
		return err
	}
	p.Lock.LastUp = time.Now().UTC()
	_ = p.Manager.SaveLock(p.Lock)
	if !io.quiet {
		printURLs(ctx, p, io)
	}
	return nil
}
