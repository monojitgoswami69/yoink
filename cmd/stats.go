package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"yoink/internal/docker"
	"yoink/internal/project"
	"yoink/internal/ui"

	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats <project>",
	Short: "Show resource usage",
	Long:  `Show per-service CPU, memory, and network I/O from docker stats.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runStats(cmd, args); err != nil {
			fmt.Println(ui.ErrorBox.Render("Error: " + err.Error()))
			os.Exit(1)
		}
	},
}

func runStats(cmd *cobra.Command, args []string) error {
	io := &initIO{verbose: GetVerbose(cmd), quiet: GetQuiet(cmd)}
	if err := requireDocker(); err != nil {
		return err
	}
	p, err := project.Resolve(args[0])
	if err != nil {
		return err
	}
	if !io.quiet {
		fmt.Println(ui.ProjectHeader(p.Name, ""))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Get this project's containers (already filtered by compose project),
	// then cross-reference with docker stats by container name. This avoids
	// fragile name-prefix matching that broke when the container_name prefix
	// differed from the compose project name.
	containers, err := p.Compose.Ps(ctx)
	if err != nil {
		return fmt.Errorf("docker ps: %w", err)
	}
	if len(containers) == 0 {
		fmt.Println("\n  " + ui.MutedStyle.Render("No running containers."))
		fmt.Println(ui.MutedStyle.Render("  Run: yoink up " + p.Name))
		return nil
	}
	stats, _ := p.Compose.Stats(ctx)
	byName := map[string]docker.Stat{}
	for _, s := range stats {
		byName[s.Name] = s
	}

	rows := [][]string{}
	var totCPU float64
	for _, c := range containers {
		s, ok := byName[c.Name]
		if !ok {
			continue
		}
		rows = append(rows, []string{
			c.Service,
			fmt.Sprintf("%.1f%%", s.CPUPct),
			s.MemUsage,
			s.NetIO,
		})
		totCPU += s.CPUPct
	}
	if len(rows) == 0 {
		fmt.Println("\n  " + ui.MutedStyle.Render("No stats available yet."))
		return nil
	}
	rows = append(rows, []string{
		ui.BoldStyle.Render("TOTAL"),
		ui.BoldStyle.Render(fmt.Sprintf("%.1f%%", totCPU)),
		ui.BoldStyle.Render(""),
		ui.BoldStyle.Render(""),
	})
	fmt.Print("\n" + ui.Columns{
		Title:   "Resource usage",
		Headers: []string{"Service", "CPU", "Memory", "Network"},
		Rows:    rows,
	}.Render())
	fmt.Println()
	return nil
}
