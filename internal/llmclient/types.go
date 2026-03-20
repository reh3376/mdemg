// Package llmclient provides a unified LLM client for OpenAI and Ollama APIs.
// It eliminates the duplicated request/response types and HTTP call patterns
// that existed across summarize, hidden, consulting, metalearn, ape, and retrieval.
package llmclient

import "encoding/json"

// Message represents a chat message with a role and content.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// --- OpenAI-compatible types ---

// OpenAIChatRequest is the request body for the OpenAI chat completions API.
type OpenAIChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_completion_tokens"`
	Temperature *float64  `json:"temperature,omitempty"`
}

// OpenAIChatResponse is the response from the OpenAI chat completions API.
type OpenAIChatResponse struct {
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage"`
	Error   *OpenAIError   `json:"error,omitempty"`
}

// OpenAIChoice is a single choice in an OpenAI response.
type OpenAIChoice struct {
	Message Message `json:"message"`
}

// OpenAIUsage contains token usage information.
type OpenAIUsage struct {
	TotalTokens int `json:"total_tokens"`
}

// OpenAIError is the error object in an OpenAI response.
type OpenAIError struct {
	Message string `json:"message"`
}

// --- Ollama types ---

// OllamaGenerateRequest is the request body for the Ollama generate API.
type OllamaGenerateRequest struct {
	Model   string          `json:"model"`
	Prompt  string          `json:"prompt"`
	Stream  bool            `json:"stream"`
	Format  json.RawMessage `json:"format,omitempty"`
	Options map[string]any  `json:"options,omitempty"`
}

// OllamaGenerateResponse is the response from the Ollama generate API.
type OllamaGenerateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}
