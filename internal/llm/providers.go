package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// postJSON sends body as JSON to url, applies headers, and decodes the
// response into out. Content-Type is always set to application/json.
func (c *Client) postJSON(ctx context.Context, url string, body any, headers map[string]string, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(raw))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) callOpenAICompatible(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	body := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0.1,
		// A healed build fix or compose rewrite can be large; default token
		// limits on some providers are small and would truncate the JSON
		// mid-object, making it unparseable. 16384 is a safe high value that
		// every OpenAI-compatible provider clamps to its own model max.
		"max_tokens": 16384,
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	headers := map[string]string{"Authorization": "Bearer " + c.apiKey}
	if err := c.postJSON(ctx, c.endpoint, body, headers, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from LLM")
	}
	return resp.Choices[0].Message.Content, nil
}

func (c *Client) callAnthropic(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	body := map[string]any{
		"model":       c.model,
		"messages":    []map[string]string{{"role": "user", "content": userPrompt}},
		"system":      systemPrompt,
		"max_tokens":  16384,
		"temperature": 0.1,
	}
	var resp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	headers := map[string]string{
		"x-api-key":         c.apiKey,
		"anthropic-version": "2023-06-01",
	}
	if err := c.postJSON(ctx, c.endpoint, body, headers, &resp); err != nil {
		return "", err
	}
	if len(resp.Content) == 0 {
		return "", fmt.Errorf("no response from Anthropic")
	}
	return resp.Content[0].Text, nil
}

func (c *Client) callGemini(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	body := map[string]any{
		"systemInstruction": map[string]any{
			"parts": []map[string]string{{"text": systemPrompt}},
		},
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]string{{"text": userPrompt}}},
		},
		"generationConfig": map[string]any{
			"temperature":      0.1,
			"maxOutputTokens":  65536,
			"responseMimeType": "application/json",
		},
	}
	var resp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := c.postJSON(ctx, c.endpoint+"?key="+c.apiKey, body, nil, &resp); err != nil {
		return "", err
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no response from Gemini")
	}
	return resp.Candidates[0].Content.Parts[0].Text, nil
}

func (c *Client) callOllama(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	body := map[string]any{
		"model":   c.model,
		"prompt":  systemPrompt + "\n\n" + userPrompt,
		"stream":  false,
		"options": map[string]any{"temperature": 0.1},
	}
	var resp struct {
		Response string `json:"response"`
	}
	if err := c.postJSON(ctx, c.endpoint, body, nil, &resp); err != nil {
		return "", err
	}
	return resp.Response, nil
}
