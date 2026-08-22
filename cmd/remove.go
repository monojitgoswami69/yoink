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

var incinerateCmd = &cobra.Command{
	Use:   "incinerate <project>",
	Short: "Permanently remove a project",
	Long: `Incinerate a Yoink-managed project: stop its containers, delete the
generated state, and forget the project. Persistent data volumes are
preserved by default — pass --volumes to delete them too.

Flags:
  --volumes   Also delete persistent volumes and application data (DESTRUCTIVE)
  --yes       Skip the confirmation prompt (for scripting)`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runIncinerate(cmd, args); err != nil {
			fmt.Println(ui.ErrorBox.Render("Error: " + err.Error()))
			os.Exit(1)
		}
	},
}

// removeCmd is a hidden backwards-compatible alias for incinerate.
var removeCmd = &cobra.Command{
	Use:    "remove <project>",
	Short:  "Alias for incinerate",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	Run:    incinerateCmd.Run,
}

var (
	removeVolumes bool
	removeYes     bool
)

func init() {
	incinerateCmd.Flags().BoolVar(&removeVolumes, "volumes", false, "Also delete persistent volumes (DESTRUCTIVE)")
	incinerateCmd.Flags().BoolVar(&removeYes, "yes", false, "Skip the confirmation prompt")
	removeCmd.Flags().BoolVar(&removeVolumes, "volumes", false, "Also delete persistent volumes (DESTRUCTIVE)")
	removeCmd.Flags().BoolVar(&removeYes, "yes", false, "Skip the confirmation prompt")
}

func runIncinerate(cmd *cobra.Command, args []string) error {
	io := &initIO{verbose: GetVerbose(cmd), quiet: GetQuiet(cmd)}
	if !io.quiet {
		fmt.Print(ui.Header(ui.HeaderArgs{Command: "incinerate", Version: Version}))
	}
	name := args[0]
	p, err := project.Resolve(name)
	if err != nil {
		return err
	}

	fmt.Printf("🔥 Incinerate project %q?\n\n", p.Name)
	fmt.Println("This will permanently remove:")
	fmt.Println("  • Yoink project state")
	fmt.Println("  • Generated Docker configuration")
	fmt.Println("  • Stopped containers")
	if removeVolumes {
		fmt.Println()
		fmt.Println(ui.ErrorStyle.Render("  ⚠ WARNING: Persistent volumes and application data will also be deleted."))
		fmt.Println(ui.ErrorStyle.Render("  This action cannot be undone."))
	} else {
		fmt.Println()
		fmt.Println(ui.DimStyle.Render("  Persistent volumes will be preserved."))
	}
	if !removeYes {
		fmt.Print("\nContinue? [y/N] ")
		if !confirm() {
			io.info("Aborted.")
			return nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if requireDocker() == nil {
		if out, err := p.Compose.Down(ctx, removeVolumes); err != nil {
			io.warn(fmt.Sprintf("compose down: %s", strings.TrimSpace(out)))
		}
	}

	repoPath := p.Lock.RepoPath
	if repoPath != "" {
		_ = os.RemoveAll(repoPath)
	}
	_ = os.RemoveAll(p.Manager.Dir)

	io.success(fmt.Sprintf("Project %s incinerated.", ui.HighlightStyle.Render(p.Name)))
	return nil
}

// confirm reads a y/N response from stdin. Anything not starting with y/Y
// (including empty/EOF) counts as "no".
func confirm() bool {
	var line string
	_, _ = fmt.Scanln(&line)
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y")
}
