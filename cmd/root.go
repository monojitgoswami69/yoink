package cmd

import (
	"fmt"
	"os"

	"yoink/internal/ui"

	"github.com/spf13/cobra"
)

var (
	// Version is set during build via ldflags.
	Version = "v0.2.0"
	// BuildTime is set during build via ldflags.
	BuildTime = "unknown"
)

var rootCmd = &cobra.Command{
	Use:           "yoink",
	Short:         "Autonomous environment orchestration for any GitHub repository",
	Long:          `Yoink clones a GitHub repository, detects the stack, and generates a runnable Docker environment.`,
	Version:       Version,
	SilenceErrors: true,
	SilenceUsage:  true,
	Run: func(cmd *cobra.Command, args []string) {
		showHelp()
	},
}

func Execute() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Honour NO_COLOR convention and --no-color flag before any output.
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		noColor, _ := cmd.Flags().GetBool("no-color")
		if noColor || os.Getenv("NO_COLOR") != "" {
			ui.SetNoColor()
		}
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, ui.ErrorLine(err.Error()))
		code := 1
		if coded, ok := err.(interface{ ExitCode() int }); ok {
			code = coded.ExitCode()
		}
		os.Exit(code)
	}
}

func init() {
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})

	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output (show LLM responses, detailed logs)")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Suppress non-error output")
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable ANSI colors in output")

	rootCmd.SetVersionTemplate(fmt.Sprintf("Yoink %s (built %s)\n", Version, BuildTime))

	rootCmd.AddCommand(helpCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(downCmd)
	rootCmd.AddCommand(restartCmd)
	rootCmd.AddCommand(openCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(envCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(healCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(dashCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(explainCmd)

	// Aliases that keep the canonical verbs short. Not advertised loudly.
	rootCmd.AddCommand(&cobra.Command{
		Use:   "start [project]",
		Short: "Alias for up",
		Run:   upCmd.Run,
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "stop [project]",
		Short: "Alias for down",
		Run:   downCmd.Run,
	})
}

// GetVerbose returns whether verbose mode is enabled.
func GetVerbose(cmd *cobra.Command) bool {
	verbose, _ := cmd.Flags().GetBool("verbose")
	return verbose
}

// GetQuiet returns whether quiet mode is enabled.
func GetQuiet(cmd *cobra.Command) bool {
	quiet, _ := cmd.Flags().GetBool("quiet")
	return quiet
}
