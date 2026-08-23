package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"yoink/internal/project"
	"yoink/internal/ui"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [project]",
	Short: "Show project health and service status",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runStatus(cmd, args); err != nil {
			fmt.Println(ui.ErrorBox.Render("Error: " + err.Error()))
			os.Exit(1)
		}
	},
}

func runStatus(cmd *cobra.Command, args []string) error {
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	p, err := project.Resolve(name)
	if err != nil {
		return err
	}

	// Minimal project header: name + repo.
	fmt.Println(ui.ProjectHeader(p.Name, shortRepo(p.Lock.RepoURL)))

	if err := requireDocker(); err != nil {
		fmt.Println("\n  " + ui.MutedStyle.Render(ui.SymStop+" unknown (docker unavailable)"))
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	h, err := p.Health(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("\n  %s\n", ui.OverallStatus(h.Overall))
	if h.Started > 0 {
		fmt.Printf("  %s %s ago\n", ui.MutedStyle.Render("up"), ui.MutedStyle.Render(humanDuration(h.Started)))
	}

	if len(h.Services) == 0 {
		fmt.Println("\n  " + ui.MutedStyle.Render("No running containers."))
		fmt.Println(ui.MutedStyle.Render("  Run: yoink up " + p.Name))
		return nil
	}

	rows := [][]string{}
	for _, s := range h.Services {
		dot, label := ui.ServiceStatus(s.State, s.Health)
		url := ui.MutedStyle.Render("internal")
		if s.Public {
			url = ui.HighlightStyle.Render(s.URL)
		}
		rows = append(rows, []string{dot + " " + s.Service, label, url})
	}
	fmt.Print("\n" + ui.Columns{
		Title:   "Services",
		Headers: []string{"Service", "Health", "URL"},
		Rows:    rows,
	}.Render())

	// Public URLs get their own prominent block (§12).
	urls, _ := p.URLs(ctx)
	if len(urls) > 0 {
		fmt.Println("\n  " + ui.Section("URL"))
		for _, u := range urls {
			fmt.Println(ui.URL(u.URL))
		}
	}
	fmt.Println()
	return nil
}

func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}
