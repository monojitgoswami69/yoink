package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"yoink/internal/config"
	"yoink/internal/docker"
	"yoink/internal/ui"

	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose the local Yoink environment",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runDoctor(cmd); err != nil {
			fmt.Println(ui.ErrorBox.Render("Error: " + err.Error()))
			os.Exit(1)
		}
	},
}

func runDoctor(cmd *cobra.Command) error {
	if !GetQuiet(cmd) {
		fmt.Print(ui.Header(ui.HeaderArgs{Command: "doctor", Version: Version}) + "\n\n")
	}
	fmt.Println("  " + ui.Section("System check"))
	fmt.Println()

	var failed []string
	check := func(ok bool, label, detail string) {
		sym := ui.SymDone
		style := ui.SuccessStyle
		if !ok {
			sym = ui.SymFail
			style = ui.ErrorStyle
			failed = append(failed, label)
		}
		line := "  " + style.Render(sym+" "+label)
		if detail != "" {
			line += "  " + ui.DimStyle.Render(detail)
		}
		fmt.Println(line)
	}

	check(execLookPath("git"), "Git", "")
	if !docker.Available() {
		check(false, "Docker", "")
	} else {
		check(true, "Docker", "")
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		daemon := docker.DaemonRunning(ctx)
		cancel()
		check(daemon, "Docker daemon", "")
		if daemon {
			ctx2, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
			out, err := exec.CommandContext(ctx2, "docker", "compose", "version", "--short").Output()
			cancel2()
			check(err == nil, "Docker Compose v2", strings.TrimSpace(string(out)))
		}
	}

	cfg, cfgErr := config.Load()
	if cfgErr != nil || cfg == nil || cfg.LLMProvider == "" {
		check(false, "LLM provider", "")
		fmt.Println(ui.DimStyle.Render("    Run: yoink setup"))
	} else {
		check(true, "LLM provider", cfg.LLMProvider)
		if cfg.LLMAPIKey == "" && cfg.LLMProvider != "ollama" {
			check(false, "LLM API key", "")
			fmt.Println(ui.DimStyle.Render("    Run: yoink setup"))
		} else {
			check(true, "LLM API key", "")
		}
	}

	fmt.Println()
	if len(failed) == 0 {
		fmt.Println("  " + ui.SuccessStyle.Render("Everything looks good."))
		fmt.Println(ui.MutedStyle.Render("  Run: yoink init <github-url>"))
		return nil
	}
	if containsStr(failed, "Docker daemon") {
		fmt.Println("  " + ui.ErrorStyle.Render("Docker is installed but not running."))
		fmt.Println(ui.MutedStyle.Render("  Start Docker and run: yoink doctor"))
		return nil
	}
	fmt.Println("  " + ui.WarningStyle.Render("Some checks failed. Fix the items marked "+ui.SymFail+" and re-run yoink doctor."))
	return nil
}

func execLookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
