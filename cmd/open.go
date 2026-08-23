package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"yoink/internal/project"
	"yoink/internal/ui"

	"github.com/spf13/cobra"
)

var openCmd = &cobra.Command{
	Use:   "open [project]",
	Short: "Open the application in your browser",
	Long: `Open the project's public URL in the system's default browser. When
no project is given, the most recently initialised one is used.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runOpen(cmd, args); err != nil {
			fmt.Println(ui.ErrorBox.Render("Error: " + err.Error()))
			os.Exit(1)
		}
	},
}

func runOpen(cmd *cobra.Command, args []string) error {
	io := &initIO{verbose: GetVerbose(cmd), quiet: GetQuiet(cmd)}
	if !io.quiet {
		fmt.Print(ui.Header(ui.HeaderArgs{Command: "open", Version: Version}) + "\n\n")
	}
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	p, err := project.Resolve(name)
	if err != nil {
		return err
	}

	// Check if the project is actually running before opening a browser.
	if requireDocker() == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if running, _ := p.IsRunning(ctx); !running {
			fmt.Println(ui.WarningStyle.Render("  ⚠ Project is not running."))
			fmt.Println(ui.DimStyle.Render("  Run: yoink up " + p.Name))
			return nil
		}
	}

	// Prefer a live URL; fall back to the configured port map so `open`
	// works even before `up` (we just can't verify the container is up).
	urls := project.ConfiguredURLs(p)
	if requireDocker() == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if live, e := p.URLs(ctx); e == nil && len(live) > 0 {
			urls = live
		}
	}
	if len(urls) == 0 {
		return fmt.Errorf("no public URL for %s (no app services with a published port)", p.Name)
	}
	target := urls[0].URL
	if !io.quiet {
		fmt.Printf("Opening %s\n", ui.HighlightStyle.Render(target))
	}
	return openBrowser(target)
}

// openBrowser opens url in the OS's default browser. It is best-effort:
// failure to exec the opener is reported, but we never crash the command
// over it.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default: // linux, *bsd
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not open browser: %w", err)
	}
	return nil
}
