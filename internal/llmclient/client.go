package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client provides LLM completions via OpenAI or Ollama APIs.
type Client struct {
	provider   string // "openai" or "ollama"
	model      string
	apiKey     string // OpenAI API key (not used for Ollama)
	baseURL    string // e.g. "https://api.openai.com/v1" or "http://localhost:11434"
	httpClient *http.Client
}

// Config holds the configuration for creating an LLM client.
type Config struct {
	Provider  string // "openai" or "ollama"
	Model     string
	APIKey    string // Required for OpenAI
	BaseURL   string // API base URL
	TimeoutMs int    // HTTP client timeout in milliseconds (default: 30000)
}

// New creates a new LLM client from the given configuration.
func New(cfg Config) *Client {
	timeoutMs := cfg.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}

	return &Client{
		provider: cfg.Provider,
		model:    cfg.Model,
		apiKey:   cfg.APIKey,
		baseURL:  cfg.BaseURL,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutMs) * time.Millisecond,
		},
	}
}

// CompleteOpts holds optional parameters for a completion request.
type CompleteOpts struct {
	MaxTokens   int
	Temperature *float64        // nil = omit from request (use API default)
	Format      json.RawMessage // Ollama structured output schema (ignored for OpenAI)
	Options     map[string]any  // Ollama generation options (ignored for OpenAI)
}

// Complete sends messages to the LLM and returns the response text.
// For OpenAI, it uses the chat completions endpoint.
// For Ollama, it concatenates messages into a single prompt and uses the generate endpoint.
func (c *Client) Complete(ctx context.Context, messages []Message, opts CompleteOpts) (string, error) {
	switch c.provider {
	case "ollama":
		return c.completeOllama(ctx, messages, opts)
	default: // "openai" or unset
		return c.completeOpenAI(ctx, messages, opts)
	}
}

// CompleteWithUsage is like Complete but also returns token usage (OpenAI only; Ollama returns 0).
func (c *Client) CompleteWithUsage(ctx context.Context, messages []Message, opts CompleteOpts) (string, int, error) {
	switch c.provider {
	case "ollama":
		text, err := c.completeOllama(ctx, messages, opts)
		return text, 0, err
	default:
		return c.completeOpenAIWithUsage(ctx, messages, opts)
	}
}

// --- OpenAI ---

func (c *Client) completeOpenAI(ctx context.Context, messages []Message, opts CompleteOpts) (string, error) {
	text, _, err := c.completeOpenAIWithUsage(ctx, messages, opts)
	return text, err
}

func (c *Client) completeOpenAIWithUsage(ctx context.Context, messages []Message, opts CompleteOpts) (string, int, error) {
	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2000
	}

	reqBody := OpenAIChatRequest{
		Model:       c.model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: opts.Temperature,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, fmt.Errorf("marshal request: %w", err)
	}

	endpoint := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("openai error %d: %s", resp.StatusCode, string(body))
	}

	var chatResp OpenAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", 0, fmt.Errorf("decode response: %w", err)
	}

	if chatResp.Error != nil {
		return "", 0, fmt.Errorf("openai error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", 0, fmt.Errorf("no choices in response")
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), chatResp.Usage.TotalTokens, nil
}

// --- Ollama ---

func (c *Client) completeOllama(ctx context.Context, messages []Message, opts CompleteOpts) (string, error) {
	// Concatenate all messages into a single prompt (Ollama generate API)
	var sb strings.Builder
	for i, msg := range messages {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(msg.Content)
	}

	reqBody := OllamaGenerateRequest{
		Model:   c.model,
		Prompt:  sb.String(),
		Stream:  false,
		Format:  opts.Format,
		Options: opts.Options,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	endpoint := c.baseURL + "/api/generate"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(body))
	}

	var ollamaResp OllamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return strings.TrimSpace(ollamaResp.Response), nil
}

// StripCodeFence removes markdown code fences that LLMs sometimes wrap around JSON.
func StripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if after, ok := strings.CutPrefix(s, "```json"); ok {
		s = strings.TrimSuffix(after, "```")
	} else if after, ok := strings.CutPrefix(s, "```"); ok {
		s = strings.TrimSuffix(after, "```")
	}
	return strings.TrimSpace(s)
}

// TruncateForLog truncates a string for error log messages.
func TruncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
