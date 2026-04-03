package llmclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

const (
	ctxKeyGuidanceID    contextKey = "llm_guidance_id"
	ctxKeySourcePath    contextKey = "llm_source_path"
	ctxKeyRetrievalCtx  contextKey = "llm_retrieval_ctx"
	ctxKeySessionID     contextKey = "llm_session_id"
	ctxKeySpaceID       contextKey = "llm_space_id"
)

// WithGuidanceID returns a context carrying a guidance_id for interaction logging.
func WithGuidanceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyGuidanceID, id)
}

// GuidanceIDFromContext extracts the guidance_id from context, or "" if not set.
func GuidanceIDFromContext(ctx context.Context) string {
	s, _ := ctx.Value(ctxKeyGuidanceID).(string)
	return s
}

// WithSourcePath returns a context carrying a source file path for interaction logging.
func WithSourcePath(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, ctxKeySourcePath, path)
}

// WithRetrievalContext returns a context carrying retrieval context (retrieved node IDs
// and scores) for RAFT-style training data enrichment.
func WithRetrievalContext(ctx context.Context, rc *RetrievalContext) context.Context {
	return context.WithValue(ctx, ctxKeyRetrievalCtx, rc)
}

// WithSessionID returns a context carrying a session_id for interaction logging.
func WithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeySessionID, id)
}

// SessionIDFromContext extracts the session_id from context, or "" if not set.
func SessionIDFromContext(ctx context.Context) string {
	s, _ := ctx.Value(ctxKeySessionID).(string)
	return s
}

// WithSpaceID returns a context carrying a space_id for interaction logging.
// When set, this overrides the Client's construction-time spaceID in TSDB records,
// ensuring request-scoped space attribution for all LLM calls in the request path.
func WithSpaceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeySpaceID, id)
}

// SpaceIDFromContext extracts the space_id from context, or "" if not set.
func SpaceIDFromContext(ctx context.Context) string {
	s, _ := ctx.Value(ctxKeySpaceID).(string)
	return s
}

// Client provides LLM completions via OpenAI or Ollama APIs.
type Client struct {
	provider   string // "openai" or "ollama"
	model      string
	apiKey     string // OpenAI API key (not used for Ollama)
	baseURL    string // e.g. "https://api.openai.com/v1" or "http://localhost:11434"
	httpClient *http.Client
	recorder   InteractionRecorder // optional; set via SetRecorder
	taskName   string              // context label for interaction logging
	spaceID    string              // context label for interaction logging
}

// Config holds the configuration for creating an LLM client.
type Config struct {
	Provider  string // "openai" or "ollama"
	Model     string
	APIKey    string // Required for OpenAI
	BaseURL   string // API base URL
	TimeoutMs int    // HTTP client timeout in milliseconds (default: 30000)
}

// defaultRecorder is automatically attached to every new Client.
// Set via SetDefaultRecorder before creating clients.
var defaultRecorder InteractionRecorder

// defaultInstanceID is propagated to every InteractionRecord.
// Set via SetDefaultInstanceID before creating clients.
var defaultInstanceID string

// defaultSpaceID is the fallback space_id for background LLM calls that
// pass "" to WithContext. Set via SetDefaultSpaceID at server startup.
var defaultSpaceID string

// SetDefaultRecorder sets a package-level recorder that is automatically
// attached to every subsequently created Client. Safe to call with nil
// to disable. This allows zero-modification wiring of all 16+ consumers.
func SetDefaultRecorder(r InteractionRecorder) {
	defaultRecorder = r
}

// SetDefaultInstanceID sets the instance identifier propagated to all
// InteractionRecords. Called once at server startup from cfg.InstanceID.
func SetDefaultInstanceID(id string) {
	defaultInstanceID = id
}

// SetDefaultSpaceID sets the fallback space_id for background LLM calls.
// When WithContext is called with an empty spaceID, this value is used instead.
// Called once at server startup from cfg.RSICWatchdogSpaceID.
func SetDefaultSpaceID(id string) {
	defaultSpaceID = id
}

// defaultSessionID is the fallback session_id for background LLM calls
// that have no request-scoped session context. Set via SetDefaultSessionID
// at server startup (typically to the instance ID).
var defaultSessionID string

// SetDefaultSessionID sets the fallback session_id for LLM interactions
// that have no request-scoped session. Called once at server startup.
func SetDefaultSessionID(id string) {
	defaultSessionID = id
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
		recorder: defaultRecorder,
	}
}

// SetRecorder attaches an interaction recorder. All subsequent Complete/CompleteWithUsage
// calls will log their request/response to the recorder. Safe to call with nil to disable.
func (c *Client) SetRecorder(r InteractionRecorder) {
	c.recorder = r
}

// WithContext returns a shallow copy of the Client with task-level metadata set.
// This allows per-consumer labeling without modifying the shared client.
// If spaceID is empty, falls back to the package-level defaultSpaceID.
func (c *Client) WithContext(taskName, spaceID string) *Client {
	copy := *c
	copy.taskName = taskName
	if spaceID == "" {
		copy.spaceID = defaultSpaceID
	} else {
		copy.spaceID = spaceID
	}
	return &copy
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
	start := time.Now()
	var text string
	var err error

	switch c.provider {
	case "ollama":
		text, err = c.completeOllama(ctx, messages, opts)
	default:
		text, err = c.completeOpenAI(ctx, messages, opts)
	}

	c.recordInteraction(ctx, messages, text, 0, int(time.Since(start).Milliseconds()), err)
	return text, err
}

// CompleteWithUsage is like Complete but also returns token usage (OpenAI only; Ollama returns 0).
func (c *Client) CompleteWithUsage(ctx context.Context, messages []Message, opts CompleteOpts) (string, int, error) {
	start := time.Now()
	var text string
	var tokens int
	var err error

	switch c.provider {
	case "ollama":
		text, err = c.completeOllama(ctx, messages, opts)
	default:
		text, tokens, err = c.completeOpenAIWithUsage(ctx, messages, opts)
	}

	c.recordInteraction(ctx, messages, text, tokens, int(time.Since(start).Milliseconds()), err)
	return text, tokens, err
}

// recordInteraction sends an interaction record to the recorder if one is set.
func (c *Client) recordInteraction(ctx context.Context, messages []Message, response string, tokens, latencyMs int, callErr error) {
	if c.recorder == nil {
		return
	}

	var system, user string
	for _, m := range messages {
		switch m.Role {
		case "system":
			system = m.Content
		case "user":
			user = m.Content
		}
	}

	rec := InteractionRecord{
		Time:         time.Now(),
		TaskName:     c.taskName,
		SpaceID:      c.spaceID,
		InstanceID:   defaultInstanceID,
		SystemPrompt: system,
		UserPrompt:   user,
		Response:     response,
		LatencyMs:    latencyMs,
		TokensOut:    tokens,
		ModelName:    c.model,
		Provider:     c.provider,
	}
	if callErr != nil {
		rec.Error = callErr.Error()
	}
	if system != "" {
		rec.SystemPromptHash = fmt.Sprintf("%x", sha256.Sum256([]byte(system)))
	}

	// Extract <think>...</think> blocks from response
	if idx := strings.Index(response, "<think>"); idx != -1 {
		if endIdx := strings.Index(response, "</think>"); endIdx > idx {
			rec.ThinkContent = response[idx+7 : endIdx]
			rec.ThinkMode = true
		}
	}

	// Pull correlation metadata from context (if present)
	if gid, ok := ctx.Value(ctxKeyGuidanceID).(string); ok {
		rec.GuidanceID = gid
	}
	if sp, ok := ctx.Value(ctxKeySourcePath).(string); ok {
		rec.SourcePath = sp
	}
	if rc, ok := ctx.Value(ctxKeyRetrievalCtx).(*RetrievalContext); ok {
		rec.RetrievalCtx = rc
	}
	if spid, ok := ctx.Value(ctxKeySpaceID).(string); ok && spid != "" {
		rec.SpaceID = spid
	}
	if sid, ok := ctx.Value(ctxKeySessionID).(string); ok && sid != "" {
		rec.SessionID = sid
	}
	if rec.SessionID == "" && defaultSessionID != "" {
		rec.SessionID = defaultSessionID
	}

	c.recorder.Record(ctx, rec)
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

// TruncateForLog truncates a string for error log messages.
func TruncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
