package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"mdemg/internal/circuitbreaker"
	"mdemg/internal/metrics"
)

const (
	defaultOpenAIEndpoint = "https://api.openai.com/v1"
	defaultOpenAIModel    = "gpt-5-mini"
	openAIDimensions      = 1536
)

// OpenAI implements the Embedder interface using OpenAI's API.
type OpenAI struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
	cb       *circuitbreaker.Breaker
}

// NewOpenAI creates a new OpenAI embedder.
func NewOpenAI(cfg Config) (*OpenAI, error) {
	endpoint := cfg.OpenAIEndpoint
	if endpoint == "" {
		endpoint = defaultOpenAIEndpoint
	}
	model := cfg.OpenAIModel
	if model == "" {
		model = defaultOpenAIModel
	}

	return &OpenAI{
		apiKey:   cfg.OpenAIAPIKey,
		model:    model,
		endpoint: endpoint,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// SetCircuitBreaker sets a circuit breaker for the embedder.
// When set, API calls will be protected by the circuit breaker.
func (o *OpenAI) SetCircuitBreaker(cb *circuitbreaker.Breaker) {
	o.cb = cb
}

func (o *OpenAI) Name() string {
	return "openai:" + o.model
}

func (o *OpenAI) Dimensions() int {
	return openAIDimensions
}

// Embed generates an embedding for a single text.
func (o *OpenAI) Embed(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := o.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}

// EmbedBatch generates embeddings for multiple texts.
func (o *OpenAI) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	// Instrument embedding latency for Prometheus metrics
	start := time.Now()
	defer func() {
		metrics.Metrics().EmbeddingLatency.Observe(time.Since(start).Seconds())
		metrics.Metrics().EmbeddingBatches.Inc()
	}()

	if len(texts) == 0 {
		return nil, nil
	}

	// Use circuit breaker if configured
	if o.cb != nil {
		var result [][]float32
		err := o.cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			result, innerErr = o.doEmbedBatch(ctx, texts)
			return innerErr
		})
		return result, err
	}

	return o.doEmbedBatch(ctx, texts)
}

// doEmbedBatch performs the actual API call without circuit breaker protection.
func (o *OpenAI) doEmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := openAIEmbeddingRequest{
		Model: o.model,
		Input: texts,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.endpoint+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp openAIErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("openai api error (%d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("openai api error (%d): %s", resp.StatusCode, string(respBody))
	}

	var embResp openAIEmbeddingResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	// Sort by index to maintain order
	result := make([][]float32, len(texts))
	for _, data := range embResp.Data {
		if data.Index < len(result) {
			result[data.Index] = data.Embedding
		}
	}

	return result, nil
}

// OpenAI API request/response types

type openAIEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}
