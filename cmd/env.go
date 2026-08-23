package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"yoink/internal/project"
	"yoink/internal/termio"
	"yoink/internal/ui"

	"github.com/spf13/cobra"
)

var envCmd = &cobra.Command{
	Use:   "env <project> [set KEY=VAL | unset KEY | list]",
	Short: "Manage application environment variables",
	Long: `Manage the environment variables a project's services read at runtime.

  yoink env my-app                 Interactive editor (shows overrides)
  yoink env my-app list            List vars for each service (masked)
  yoink env my-app set KEY=value   Set an override (non-interactive)
  yoink env my-app unset KEY       Remove an override

Secrets are masked in listings. Overrides are stored in project state
(env-overrides.json) and applied on the next ` + "`yoink up`" + `.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runEnv(cmd, args); err != nil {
			fmt.Println(ui.ErrorBox.Render("Error: " + err.Error()))
			os.Exit(1)
		}
	},
}

func runEnv(cmd *cobra.Command, args []string) error {
	io := &initIO{verbose: GetVerbose(cmd), quiet: GetQuiet(cmd)}
	if !io.quiet {
		fmt.Print(ui.Header(ui.HeaderArgs{Command: "env", Version: Version}))
	}
	p, err := project.Resolve(args[0])
	if err != nil {
		return err
	}
	action := ""
	if len(args) > 1 {
		action = args[1]
	}

	switch action {
	case "set":
		if len(args) < 3 {
			return fmt.Errorf("usage: yoink env %s set KEY=value", p.Name)
		}
		return envSet(p, args[2])
	case "unset":
		if len(args) < 3 {
			return fmt.Errorf("usage: yoink env %s unset KEY", p.Name)
		}
		return envUnset(p, args[2])
	case "list", "ls":
		return envList(p, io)
	case "":
		return envInteractive(p, io)
	default:
		return fmt.Errorf("unknown env action %q (use set|unset|list)", action)
	}
}

// envDir returns the env-vars directory for a service.
func envDir(p *project.Project, serviceID string) string {
	return filepath.Join(p.Lock.RepoPath, p.Lock.OutputSubdir, "env-vars", serviceID)
}

// envList prints the .env.example keys for each service with values masked
// from the template; overrides are noted with *.
func envList(p *project.Project, io *initIO) error {
	overrides, _ := p.Manager.LoadOverrides()
	for _, svc := range p.Lock.Services {
		example := filepath.Join(envDir(p, svc.ID), ".env.example")
		data, err := os.ReadFile(example)
		if err != nil {
			continue
		}
		fmt.Printf("\n  %s · %s\n", ui.HighlightStyle.Render(svc.ID), ui.DimStyle.Render(svc.Framework))
		keys := envKeys(string(data))
		for _, k := range keys {
			val := maskValue(string(data), k)
			marker := " "
			if v, ok := overrides[svc.ID][k]; ok {
				val = mask(v)
				marker = ui.SuccessStyle.Render("*")
			}
			fmt.Printf("  %s %-28s %s\n", marker, k, ui.DimStyle.Render(val))
		}
	}
	fmt.Println()
	return nil
}

func envSet(p *project.Project, kv string) error {
	key, val, ok := strings.Cut(kv, "=")
	if !ok {
		return fmt.Errorf("expected KEY=value, got %q", kv)
	}
	targets := []string{}
	for _, svc := range p.Lock.Services {
		if svcHasVar(p, svc.ID, key) || len(p.Lock.Services) == 1 {
			targets = append(targets, svc.ID)
		}
	}
	if len(targets) == 0 {
		targets = []string{p.Lock.Services[0].ID}
	}
	for _, sid := range targets {
		if err := p.Manager.SetOverride(sid, key, val); err != nil {
			return err
		}
	}
	fmt.Printf("Set %s on %d service(s). Run `yoink up %s` to apply.\n", key, len(targets), p.Name)
	return nil
}

func envUnset(p *project.Project, key string) error {
	overrides, err := p.Manager.LoadOverrides()
	if err != nil {
		return err
	}
	removed := 0
	for sid := range overrides {
		if _, ok := overrides[sid][key]; ok {
			delete(overrides[sid], key)
			removed++
		}
	}
	if err := p.Manager.SaveOverrides(overrides); err != nil {
		return err
	}
	if removed == 0 {
		fmt.Println(ui.DimStyle.Render("No override for " + key))
		return nil
	}
	fmt.Printf("Removed override for %s.\n", key)
	return nil
}

func envInteractive(p *project.Project, io *initIO) error {
	overrides, _ := p.Manager.LoadOverrides()

	// Pick a service once.
	svcIDs := make([]string, len(p.Lock.Services))
	for i, s := range p.Lock.Services {
		svcIDs[i] = fmt.Sprintf("%s (%s)", s.ID, s.Framework)
	}
	idx := selectFromList(svcIDs)
	if idx == -1 || idx >= len(p.Lock.Services) {
		return nil
	}
	svc := p.Lock.Services[idx]

	example := filepath.Join(envDir(p, svc.ID), ".env.example")
	data, err := os.ReadFile(example)
	if err != nil {
		return fmt.Errorf("no .env.example for %s", svc.ID)
	}
	keys := envKeys(string(data))

	// Persistent loop: keep editing until user quits.
	for {
		// Reload overrides each iteration so display stays fresh.
		overrides, _ = p.Manager.LoadOverrides()

		if len(keys) == 0 && len(overrides[svc.ID]) == 0 {
			io.info("No variables to edit.")
			return nil
		}

		// Build labels showing override values (if set) and template defaults.
		allKeys := make([]string, 0, len(keys))
		seen := map[string]bool{}
		for _, k := range keys {
			if !seen[k] {
				allKeys = append(allKeys, k)
				seen[k] = true
			}
		}
		// Also show any override keys not in .env.example.
		for k := range overrides[svc.ID] {
			if !seen[k] {
				allKeys = append(allKeys, k)
				seen[k] = true
			}
		}
		sort.Strings(allKeys)

		vlabels := make([]string, len(allKeys))
		for i, k := range allKeys {
			displayVal := ""
			source := ""
			if ov, hasOverride := overrides[svc.ID][k]; hasOverride {
				displayVal = mask(ov)
				source = "override"
			} else {
				tmplVal := envValue(string(data), k)
				displayVal = mask(tmplVal)
				if tmplVal == "" {
					source = "not set"
				} else {
					source = "template"
				}
			}
			vlabels[i] = fmt.Sprintf("%-28s %s  (%s)", k, ui.DimStyle.Render(displayVal), source)
		}
		vlabels = append(vlabels, "Add new variable")
		vlabels = append(vlabels, "Quit (save and exit)")

		vi := selectFromList(vlabels)
		if vi == -1 || vi == len(vlabels)-1 {
			io.success(fmt.Sprintf("Saved. Run `yoink up %s` to apply.", p.Name))
			return nil
		}

		var key, def string
		if vi == len(vlabels)-2 {
			// Add new variable
			fmt.Print("  New KEY: ")
			key = strings.TrimSpace(termio.ReadLine())
			if key == "" {
				io.info("Empty key — skipping.")
				continue
			}
		} else {
			key = allKeys[vi]
			if ov, hasOverride := overrides[svc.ID][key]; hasOverride {
				def = ov
			} else {
				def = envValue(string(data), key)
			}
		}

		prompt := fmt.Sprintf("  Value for %s", key)
		if def != "" {
			prompt += " [" + mask(def) + "]"
		}
		prompt += ": "
		fmt.Print(prompt)
		val := strings.TrimSpace(termio.ReadLine())
		if val == "" {
			io.info("Empty value — nothing changed.")
			continue
		}
		if err := p.Manager.SetOverride(svc.ID, key, val); err != nil {
			return err
		}
		io.success(fmt.Sprintf("Saved %s", key))
		// Loop continues — user can edit more variables.
	}
}

// envKeys returns the sorted KEY= lines (ignoring comments) from a .env.example.
func envKeys(content string) []string {
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if k != "" && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func envValue(content, key string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) == key {
			return v
		}
	}
	return ""
}

func maskValue(content, key string) string {
	return mask(envValue(content, key))
}

// mask hides sensitive values. Empty shows "(not set)".
func mask(v string) string {
	if v == "" {
		return ui.MutedStyle.Render("(not set)")
	}
	if len(v) <= 4 {
		return strings.Repeat("*", len(v))
	}
	return v[:2] + strings.Repeat("*", len(v)-4) + v[len(v)-2:]
}

func svcHasVar(p *project.Project, serviceID, key string) bool {
	example := filepath.Join(envDir(p, serviceID), ".env.example")
	data, err := os.ReadFile(example)
	if err != nil {
		return false
	}
	for _, k := range envKeys(string(data)) {
		if k == key {
			return true
		}
	}
	return false
}
