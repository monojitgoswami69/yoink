package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"yoink/internal/project"
	"yoink/internal/ui"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List Yoink projects",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runList(cmd); err != nil {
			fmt.Println(ui.ErrorBox.Render("Error: " + err.Error()))
			os.Exit(1)
		}
	},
}

var (
	listRunning bool
	listStopped bool
)

func init() {
	listCmd.Flags().BoolVar(&listRunning, "running", false, "Show only running projects")
	listCmd.Flags().BoolVar(&listStopped, "stopped", false, "Show only stopped projects")
}

func runList(cmd *cobra.Command) error {
	io := &initIO{verbose: GetVerbose(cmd), quiet: GetQuiet(cmd)}
	all, err := project.All()
	if err != nil {
		return err
	}
	if !io.quiet {
		fmt.Print(ui.Header(ui.HeaderArgs{Command: "list", Version: Version}) + "\n\n")
	}
	if len(all) == 0 {
		fmt.Println(ui.MutedStyle.Render("  No projects yet."))
		fmt.Println(ui.MutedStyle.Render("  Run: yoink init <github-url>"))
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows := [][]string{}
	runningCount := 0
	stoppedCount := 0
	for _, p := range all {
		running, _ := p.IsRunning(ctx)
		if listRunning && !running {
			continue
		}
		if listStopped && running {
			continue
		}
		if running {
			runningCount++
		} else {
			stoppedCount++
		}
		status := ui.MutedStyle.Render(ui.SymStop + " stopped")
		if running {
			status = ui.SuccessStyle.Render(ui.SymRun + " running")
		}
		urls := project.ConfiguredURLs(p)
		urlStr := "-"
		if len(urls) > 0 {
			parts := make([]string, 0, len(urls))
			for _, u := range urls {
				parts = append(parts, u.URL)
			}
			urlStr = strings.Join(parts, ", ")
		}
		rows = append(rows, []string{
			ui.BoldStyle.Render(p.Name),
			status,
			fmt.Sprintf("%d", len(p.Lock.Services)),
			ui.MutedStyle.Render(urlStr),
		})
	}
	if len(rows) == 0 {
		label := "running"
		if listStopped {
			label = "stopped"
		}
		fmt.Println(ui.MutedStyle.Render("  No " + label + " projects."))
		return nil
	}
	fmt.Print(ui.Columns{
		Title:   "Projects",
		Headers: []string{"Name", "Status", "Services", "URLs"},
		Rows:    rows,
	}.Render())
	fmt.Printf("\n  %s", ui.MutedStyle.Render(plural(len(rows), "project")))
	if runningCount > 0 {
		fmt.Printf("  ·  %d running", runningCount)
	}
	if stoppedCount > 0 {
		fmt.Printf("  ·  %d stopped", stoppedCount)
	}
	fmt.Println()
	return nil
}

// shortRepo turns the canonical clone URL into a readable "github.com/owner/repo".
func shortRepo(url string) string {
	s := strings.TrimPrefix(url, "https://")
	s = strings.TrimSuffix(s, ".git")
	return s
}
