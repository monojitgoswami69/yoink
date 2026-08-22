// Package llm wraps the chat APIs of the LLM providers Yoink supports
// (OpenAI, Anthropic, Groq, Gemini, Ollama) and exposes higher-level
// "validate this analysis" helpers used by the init flow.
package llm

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// callTimeout caps a single LLM round-trip. Build-heal responses can carry a
// full rewritten Dockerfile + docker-compose.yml, so 60s is too tight for
// the slower providers; 180s leaves headroom while still failing fast on a
// hung connection.
const callTimeout = 180 * time.Second

// Client talks to a single LLM provider.
type Client struct {
	provider string
	model    string
	apiKey   string
	endpoint string
	http     *http.Client
}

// NewClient constructs a client for one of the supported providers.
func NewClient(provider, model, apiKey string) (*Client, error) {
	c := &Client{
		provider: strings.ToLower(provider),
		model:    model,
		apiKey:   apiKey,
		http:     http.DefaultClient,
	}
	switch c.provider {
	case "openai":
		c.endpoint = "https://api.openai.com/v1/chat/completions"
	case "anthropic":
		c.endpoint = "https://api.anthropic.com/v1/messages"
	case "groq":
		c.endpoint = "https://api.groq.com/openai/v1/chat/completions"
	case "gemini":
		c.endpoint = fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", model)
	case "ollama":
		c.endpoint = "http://localhost:11434/api/generate"
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
	return c, nil
}

// Call sends a single (system, user) prompt pair and returns the model's reply.
func (c *Client) Call(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	switch c.provider {
	case "openai", "groq":
		return c.callOpenAICompatible(ctx, systemPrompt, userPrompt)
	case "anthropic":
		return c.callAnthropic(ctx, systemPrompt, userPrompt)
	case "gemini":
		return c.callGemini(ctx, systemPrompt, userPrompt)
	case "ollama":
		return c.callOllama(ctx, systemPrompt, userPrompt)
	}
	return "", fmt.Errorf("unsupported provider: %s", c.provider)
}
