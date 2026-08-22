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

var removeCmd = &cobra.Command{
	Use:   "remove <project>",
	Short: "Remove a project",
	Long: `Remove a Yoink-managed project: stop its containers, delete the
generated state, and forget the project. Persistent data volumes are
preserved by default — pass --volumes to delete them too.

Flags:
  --volumes   Also remove persistent volumes (DESTRUCTIVE — database data)
  --yes       Skip the confirmation prompt (for scripting)`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runRemove(cmd, args); err != nil {
			fmt.Println(ui.ErrorBox.Render("Error: " + err.Error()))
			os.Exit(1)
		}
	},
}

var (
	removeVolumes bool
	removeYes     bool
)

func init() {
	removeCmd.Flags().BoolVar(&removeVolumes, "volumes", false, "Also remove persistent volumes (DESTRUCTIVE)")
	removeCmd.Flags().BoolVar(&removeYes, "yes", false, "Skip the confirmation prompt")
}

func runRemove(cmd *cobra.Command, args []string) error {
	io := &initIO{verbose: GetVerbose(cmd), quiet: GetQuiet(cmd)}
	if !io.quiet {
		fmt.Print(ui.Header(ui.HeaderArgs{Command: "remove", Version: Version}))
	}
	name := args[0]
	p, err := project.Resolve(name)
	if err != nil {
		return err
	}

	fmt.Printf("This will remove:\n  • containers\n  • generated Yoink state\n  • project metadata\n\n")
	if removeVolumes {
		fmt.Println(ui.ErrorStyle.Render("  ⚠ Persistent volumes WILL BE DELETED (--volumes)."))
	} else {
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

	// Remove the cloned repo + generated output.
	repoPath := p.Lock.RepoPath
	if repoPath != "" {
		_ = os.RemoveAll(repoPath)
	}
	// Remove the project state directory.
	_ = os.RemoveAll(p.Manager.Dir)

	io.success(fmt.Sprintf("Project %s removed.", ui.HighlightStyle.Render(p.Name)))
	return nil
}

// confirm reads a y/N response from stdin. Anything not starting with y/Y
// (including empty/EOF) counts as "no".
func confirm() bool {
	var line string
	_, _ = fmt.Scanln(&line)
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y")
}
