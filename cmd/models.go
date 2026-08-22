package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// fetchModelsFromProvider returns the list of model IDs offered by the given
// provider, sorted with recommended models first where appropriate.
func fetchModelsFromProvider(ctx context.Context, provider, apiKey string) ([]string, error) {
	switch provider {
	case "openai":
		return fetchOpenAIModels(ctx, apiKey)
	case "anthropic":
		return staticAnthropicModels(), nil
	case "groq":
		return fetchOpenAIStyle(ctx, "https://api.groq.com/openai/v1/models", apiKey, nil), nil
	case "gemini":
		return fetchGeminiModels(ctx, apiKey)
	case "ollama":
		return fetchOllamaModels(ctx)
	}
	return nil, fmt.Errorf("unsupported provider: %s", provider)
}

// openAIStyleResponse is the shape both OpenAI and Groq return.
type openAIStyleResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// fetchOpenAIStyle hits an OpenAI-style /models endpoint and returns the IDs,
// optionally filtered. Errors are converted to an empty slice — callers want
// fallback behaviour, not propagation.
func fetchOpenAIStyle(ctx context.Context, url, apiKey string, keep func(string) bool) []string {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var parsed openAIStyleResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil
	}
	out := make([]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		if keep != nil && !keep(m.ID) {
			continue
		}
		out = append(out, m.ID)
	}
	sort.Strings(out)
	return out
}

func fetchOpenAIModels(ctx context.Context, apiKey string) ([]string, error) {
	chat := fetchOpenAIStyle(ctx, "https://api.openai.com/v1/models", apiKey, func(id string) bool {
		return strings.Contains(id, "gpt") && !strings.Contains(id, "instruct")
	})
	preferred := []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "gpt-4", "gpt-3.5-turbo"}
	return sortRecommendedFirst(chat, preferred), nil
}

func staticAnthropicModels() []string {
	return []string{
		"claude-opus-4-7",
		"claude-sonnet-4-6",
		"claude-haiku-4-5",
		"claude-3-5-sonnet-20241022",
		"claude-3-haiku-20240307",
	}
}

func fetchGeminiModels(ctx context.Context, apiKey string) ([]string, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models?key=%s", apiKey)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	models := make([]string, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		// Names come back as "models/<id>"; we want just the id.
		_, id, ok := strings.Cut(m.Name, "/")
		if !ok || !strings.Contains(id, "gemini") || strings.Contains(id, "embedding") {
			continue
		}
		models = append(models, id)
	}
	sort.Strings(models)
	return models, nil
}

func fetchOllamaModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost:11434/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama not running on localhost:11434")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama API error (%d)", resp.StatusCode)
	}
	var parsed struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		names = append(names, m.Name)
	}
	sort.Strings(names)
	return names, nil
}

func sortRecommendedFirst(models, preferred []string) []string {
	pref := map[string]int{}
	for i, p := range preferred {
		pref[p] = i + 1
	}
	sort.SliceStable(models, func(i, j int) bool {
		pi, pj := pref[models[i]], pref[models[j]]
		switch {
		case pi != 0 && pj != 0:
			return pi < pj
		case pi != 0:
			return true
		case pj != 0:
			return false
		}
		return models[i] < models[j]
	})
	return models
}

var recommendedByProvider = map[string]map[string]bool{
	"openai":    {"gpt-4o": true, "gpt-4o-mini": true, "gpt-5": true},
	"anthropic": {"claude-sonnet-4-6": true, "claude-opus-4-7": true},
	"groq":      {"llama-3.3-70b-versatile": true, "llama3-70b-8192": true},
	"gemini":    {"gemini-3.1-flash-lite": true, "gemini-3.5-flash": true, "gemini-3.6-flash": true, "gemini-3.7-flash": true, "gemini-2.5-pro": true},
	"ollama":    {"llama3": true, "mistral": true},
}

func isRecommendedModel(provider, model string) bool {
	return recommendedByProvider[provider][model]
}
