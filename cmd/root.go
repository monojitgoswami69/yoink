package cmd

import (
	"fmt"
	"os"

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
}

func Execute() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output (show LLM responses, detailed logs)")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Suppress non-error output")
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable ANSI colors in output")

	rootCmd.SetVersionTemplate(fmt.Sprintf("Yoink %s (built %s)\n", Version, BuildTime))
}
