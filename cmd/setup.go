package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"yoink/internal/config"
	"yoink/internal/llm"
	"yoink/internal/termio"
	"yoink/internal/ui"

	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure your LLM provider and API key",
	Long: `Interactive wizard to configure Yoink with your LLM provider, API key, and optional GitHub PAT.

This creates ~/.yoink/config.json with your configuration (chmod 0600).

Flags:
  --force    Force reconfiguration even if config already exists

Examples:
  yoink setup
  yoink setup --force`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runSetup(); err != nil {
			fmt.Println(ui.ErrorLine(err.Error()))
			os.Exit(1)
		}
	},
}

var setupForce bool

func init() {
	setupCmd.Flags().BoolVar(&setupForce, "force", false, "Force reconfiguration even if config exists")
}

var providerOptions = []struct {
	key, label string
}{
	{"openai", "OpenAI"},
	{"anthropic", "Anthropic (Claude)"},
	{"groq", "Groq"},
	{"gemini", "Google Gemini"},
	{"ollama", "Ollama (local, no API key)"},
}

func runSetup() error {
	if !setupForce && config.Exists() {
		configPath, _ := config.ConfigPath()
		fmt.Println(ui.Header(ui.HeaderArgs{Command: "setup", Version: Version}))
		fmt.Println(ui.WarningLine("Configuration already exists"))
		fmt.Println(ui.InfoLine(fmt.Sprintf("Config file: %s", configPath)))
		fmt.Println()
		fmt.Println(ui.DimStyle.Render("To reconfigure, run:"))
		fmt.Println(ui.HighlightStyle.Render("  yoink setup --force"))
		return nil
	}

	fmt.Print(ui.Header(ui.HeaderArgs{Command: "setup", Version: Version}) + "\n\n")

	provider, err := pickProvider()
	if err != nil {
		return err
	}

	apiKey, err := readAPIKey(provider)
	if err != nil {
		return err
	}

	model, err := pickModel(provider, apiKey)
	if err != nil {
		return err
	}

	if err := optionallyVerify(provider, model, apiKey); err != nil {
		return err
	}

	pat, err := readOptionalPAT()
	if err != nil {
		return err
	}

	cfg := &config.Config{
		LLMProvider: provider,
		LLMModel:    model,
		LLMAPIKey:   apiKey,
		GitHubPAT:   pat,
	}
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("could not save config: %w", err)
	}

	cfgPath, _ := config.ConfigPath()
	msg := fmt.Sprintf(
		"Yoink is ready.\n\n  Provider · %s\n  Model    · %s\n  Config   · %s\n\nTry:\n  %s",
		ui.HighlightStyle.Render(titleCase(provider)),
		ui.HighlightStyle.Render(model),
		ui.DimStyle.Render(cfgPath),
		ui.HighlightStyle.Render("yoink init <github-url>"),
	)
	fmt.Println()
	fmt.Println(ui.SuccessBox.Render(msg))
	fmt.Println()
	return nil
}

func pickProvider() (string, error) {
	fmt.Println(ui.StepLine(1, 5, "Choose an LLM provider"))
	labels := make([]string, len(providerOptions))
	for i, p := range providerOptions {
		labels[i] = p.label
	}
	idx := selectFromList(labels)
	if idx == -1 {
		return "", fmt.Errorf("setup cancelled")
	}
	fmt.Println(ui.SuccessLine("Selected " + providerOptions[idx].label))
	return providerOptions[idx].key, nil
}

func readAPIKey(provider string) (string, error) {
	if provider == "ollama" {
		fmt.Println(ui.StepLine(2, 5, "Ollama detected — checking local daemon"))
		return "", nil
	}
	fmt.Println(ui.StepLine(2, 5, fmt.Sprintf("Paste your %s API key", titleCase(provider))))
	fmt.Println(ui.DimStyle.Render("  (input is hidden)"))
	fmt.Print("  ")
	key, err := termio.ReadSecret()
	if err != nil {
		return "", fmt.Errorf("failed to read API key: %w", err)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("API key cannot be empty")
	}
	fmt.Println(ui.SuccessLine("API key recorded"))
	return key, nil
}

func pickModel(provider, apiKey string) (string, error) {
	fmt.Println(ui.StepLine(3, 5, "Pick a model"))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	models, err := fetchModelsFromProvider(ctx, provider, apiKey)
	if err != nil || len(models) == 0 {
		if err != nil {
			fmt.Println(ui.WarningLine(fmt.Sprintf("Could not list models automatically (%v) — enter a name manually.", err)))
		} else {
			fmt.Println(ui.WarningLine("No models returned — enter a name manually."))
		}
		fmt.Print("  Model name: ")
		manual := strings.TrimSpace(termio.ReadLine())
		if manual == "" {
			return "", fmt.Errorf("model name cannot be empty")
		}
		fmt.Println(ui.SuccessLine("Selected " + manual))
		return manual, nil
	}

	display := make([]string, len(models))
	for i, m := range models {
		if isRecommendedModel(provider, m) {
			display[i] = "★ " + m
		} else {
			display[i] = m
		}
	}
	idx := selectFromList(display)
	if idx == -1 {
		return "", fmt.Errorf("setup cancelled")
	}
	fmt.Println(ui.SuccessLine("Selected " + models[idx]))
	return models[idx], nil
}

func optionallyVerify(provider, model, apiKey string) error {
	fmt.Println(ui.StepLine(4, 5, "Verify connection?"))
	idx := selectFromList([]string{"Yes (recommended)", "Skip"})
	if idx == -1 {
		return fmt.Errorf("setup cancelled")
	}
	if idx != 0 {
		fmt.Println(ui.WarningLine("Skipped verification"))
		return nil
	}
	client, err := llm.NewClient(provider, model, apiKey)
	if err != nil {
		return fmt.Errorf("could not create LLM client: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := client.Call(ctx, "You are a helpful assistant. Reply with only the word pong.", "ping"); err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}
	fmt.Println(ui.SuccessLine("Connection verified"))
	return nil
}

func readOptionalPAT() (string, error) {
	fmt.Println(ui.StepLine(5, 5, "Add a GitHub PAT for private repos? (optional)"))
	idx := selectFromList([]string{"No", "Yes"})
	if idx == -1 {
		return "", fmt.Errorf("setup cancelled")
	}
	if idx == 0 {
		return "", nil
	}
	fmt.Println(ui.DimStyle.Render("  Needs `repo` scope. Create one at https://github.com/settings/tokens"))
	fmt.Print("  PAT: ")
	pat, err := termio.ReadSecret()
	if err != nil {
		return "", fmt.Errorf("failed to read PAT: %w", err)
	}
	pat = strings.TrimSpace(pat)
	if pat != "" && !strings.HasPrefix(pat, "ghp_") && !strings.HasPrefix(pat, "github_pat_") {
		fmt.Println(ui.WarningLine("PAT does not start with ghp_ or github_pat_ — saving anyway"))
	}
	return pat, nil
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
