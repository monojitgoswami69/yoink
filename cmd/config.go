package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"yoink/internal/project"
	"yoink/internal/termio"
	"yoink/internal/ui"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config <project>",
	Short: "Configure Yoink settings for a project",
	Long: `Manage project-specific Yoink behaviour: heal attempts, auto-heal,
default service, and per-project LLM provider/model overrides. Global
settings live in ~/.yoink/config.json (see yoink setup); this only stores
per-project overrides on top of those.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runConfig(cmd, args); err != nil {
			fmt.Println(ui.ErrorBox.Render("Error: " + err.Error()))
			os.Exit(1)
		}
	},
}

func runConfig(cmd *cobra.Command, args []string) error {
	io := &initIO{verbose: GetVerbose(cmd), quiet: GetQuiet(cmd)}
	if !io.quiet {
		fmt.Print(ui.Header(ui.HeaderArgs{Command: "config", Version: Version}))
	}
	p, err := project.Resolve(args[0])
	if err != nil {
		return err
	}
	s, err := p.Manager.LoadSettings()
	if err != nil {
		return err
	}

	for {
		fmt.Printf("\n  %s  ·  %s\n\n", ui.BoldStyle.Render("YOINK CONFIG"), ui.HighlightStyle.Render(p.Name))
		items := []struct {
			label string
			value string
			set   func(string) error
		}{
			{"Heal attempts", intStr(s.HealTries), func(v string) error {
				n, e := strconv.Atoi(v)
				if e != nil {
					return e
				}
				s.HealTries = n
				return nil
			}},
			{"Auto heal", boolStr(s.AutoHeal), func(v string) error { s.AutoHeal = parseBool(v); return nil }},
			{"Default service", orDash(s.DefaultService), func(v string) error { s.DefaultService = v; return nil }},
			{"LLM provider override", orDash(s.LLMProvider), func(v string) error { s.LLMProvider = v; return nil }},
			{"LLM model override", orDash(s.LLMModel), func(v string) error { s.LLMModel = v; return nil }},
		}
		labels := make([]string, len(items))
		for i, it := range items {
			labels[i] = fmt.Sprintf("%-22s %s", it.label, ui.DimStyle.Render(it.value))
		}
		labels = append(labels, "Save & exit", "Quit without saving")
		idx := selectFromList(labels)
		if idx == len(labels)-1 || idx == -1 {
			io.info("No changes saved.")
			return nil
		}
		if idx == len(labels)-2 {
			if err := p.Manager.SaveSettings(s); err != nil {
				return err
			}
			io.success("Settings saved.")
			return nil
		}
		it := items[idx]
		fmt.Printf("  New value for %s: ", it.label)
		v := strings.TrimSpace(termio.ReadLine())
		if v != "" {
			if err := it.set(v); err != nil {
				io.warn(fmt.Sprintf("invalid value: %v", err))
			}
		}
	}
}

func intStr(n int) string {
	if n == 0 {
		return "(global default)"
	}
	return strconv.Itoa(n)
}
func boolStr(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}
func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "y", "yes", "true", "1", "on":
		return true
	}
	return false
}
