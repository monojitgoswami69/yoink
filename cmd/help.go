package cmd

import (
	"fmt"

	"yoink/internal/ui"

	"github.com/spf13/cobra"
)

var helpCmd = &cobra.Command{
	Use:   "help",
	Short: "Show help message",
	Long:  `Display help information about Yoink and its commands.`,
	Run: func(cmd *cobra.Command, args []string) {
		showHelp()
	},
}

func showHelp() {
	fmt.Println(ui.Header(ui.HeaderArgs{Command: "help", Version: Version}))

	fmt.Println(ui.BoldStyle.Render("  Yoink — turn repositories into runnable environments"))
	fmt.Println()

	groups := []struct {
		title string
		rows  [][]string
	}{
		{"PROJECTS", [][]string{
			{"init", "<repo>", "Initialize a repository into a project"},
			{"list", "", "List initialized projects"},
			{"incinerate", "<project>", "Permanently remove a project"},
		}},
		{"RUNTIME", [][]string{
			{"up", "<project>", "Start a project"},
			{"down", "<project>", "Stop a project"},
			{"restart", "<project>", "Restart a project"},
			{"open", "<project>", "Open the application in your browser"},
			{"status", "[project]", "Show health and service status"},
			{"logs", "<project> [service]", "Show service logs (--follow to tail)"},
			{"stats", "<project>", "Show resource usage"},
		}},
		{"CONFIGURATION", [][]string{
			{"env", "<project>", "Manage application environment variables"},
			{"setup", "", "Configure global Yoink settings (LLM, PAT)"},
		}},
		{"INTELLIGENCE", [][]string{
			{"heal", "<project>", "Diagnose and repair build failures"},
			{"update", "<project>", "Pull changes, rebuild, and restart"},
			{"explain", "[project]", "Summarise what Yoink detected/repaired"},
		}},
		{"SYSTEM", [][]string{
			{"doctor", "", "Diagnose your local setup"},
			{"help", "", "Show this help message"},
		}},
	}

	for _, g := range groups {
		fmt.Println("  " + ui.BoldStyle.Render(g.title))
		fmt.Println()
		for _, r := range g.rows {
			name := ui.PrimaryStyle.Render(fmt.Sprintf("%-12s", r[0]))
			args := ui.HighlightStyle.Render(fmt.Sprintf("%-20s", r[1]))
			fmt.Printf("    %s %s %s\n", name, args, r[2])
		}
		fmt.Println()
	}

	fmt.Println("  " + ui.BoldStyle.Render("GLOBAL FLAGS"))
	fmt.Println()
	flags := []struct{ name, desc string }{
		{"--verbose, -v", "Show detailed logs"},
		{"--quiet, -q", "Suppress non-error output"},
		{"--no-color", "Disable ANSI colors (also honours NO_COLOR)"},
		{"--version", "Print version and exit"},
	}
	for _, f := range flags {
		fmt.Printf("    %s  %s\n", ui.MutedStyle.Render(fmt.Sprintf("%-16s", f.name)), f.desc)
	}

	fmt.Println()
	fmt.Println("  " + ui.BoldStyle.Render("TYPICAL WORKFLOW"))
	fmt.Println()
	flow := []string{
		"yoink init https://github.com/tiangolo/fastapi",
		"yoink env fastapi",
		"yoink up fastapi",
		"yoink status fastapi",
		"yoink open fastapi",
	}
	for _, e := range flow {
		fmt.Println("    " + ui.HighlightStyle.Render(e))
	}
	fmt.Println()
	fmt.Println(ui.DimStyle.Render("  Each command also supports --help, e.g. `yoink up --help`."))
	fmt.Println()
}
