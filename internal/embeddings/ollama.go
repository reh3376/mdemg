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
	defaultOllamaEndpoint = "http://localhost:11434"
	defaultOllamaModel = "qwen3-embedding:4b"
)

// Ollama implements the Embedder interface using Ollama's local API.
type Ollama struct {
	endpoint   string
	model      string
	client     *http.Client
	dimensions int
	cb         *circuitbreaker.Breaker
}

// NewOllama creates a new Ollama embedder.
func NewOllama(cfg Config) (*Ollama, error) {
	endpoint := cfg.OllamaEndpoint
	if endpoint == "" {
		endpoint = defaultOllamaEndpoint
	}
	model := cfg.OllamaModel
	if model == "" {
		model = defaultOllamaModel
	}

	o := &Ollama{
		endpoint: endpoint,
		model:    model,
		client: &http.Client{
			Timeout: 60 * time.Second, // Ollama can be slower
		},
		dimensions: 1536, // Default for qwen3-embedding:4b
	}

	// Adjust dimensions based on known models
	switch model {
	case "nomic-embed-text":
		o.dimensions = 768
	case "mxbai-embed-large":
		o.dimensions = 1024
	case "all-minilm", "all-minilm:l6-v2":
		o.dimensions = 384
	case "qwen3-embedding:4b", "qwen3-embedding":
		o.dimensions = 1536
	}

	return o, nil
}

func (o *Ollama) Name() string {
	return "ollama:" + o.model
}

func (o *Ollama) Dimensions() int {
	return o.dimensions
}

// SetCircuitBreaker sets the circuit breaker for API calls.
func (o *Ollama) SetCircuitBreaker(cb *circuitbreaker.Breaker) {
	o.cb = cb
}

// Embed generates an embedding for a single text.
func (o *Ollama) Embed(ctx context.Context, text string) ([]float32, error) {
	// If circuit breaker is set, check if we should allow the request
	if o.cb != nil && !o.cb.Allow() {
		return nil, circuitbreaker.ErrCircuitOpen
	}

	result, err := o.doEmbed(ctx, text)

	// Record result with circuit breaker
	if o.cb != nil {
		if err != nil {
			o.cb.RecordFailure()
		} else {
			o.cb.RecordSuccess()
		}
	}

	return result, err
}

// doEmbed performs the actual embedding request.
func (o *Ollama) doEmbed(ctx context.Context, text string) ([]float32, error) {
	reqBody := ollamaEmbeddingRequest{
		Model:  o.model,
		Prompt: text,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.endpoint+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

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
		return nil, fmt.Errorf("ollama api error (%d): %s", resp.StatusCode, string(respBody))
	}

	var embResp ollamaEmbeddingResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	// Convert float64 to float32
	result := make([]float32, len(embResp.Embedding))
	for i, v := range embResp.Embedding {
		result[i] = float32(v)
	}

	// Update dimensions if different from expected
	if len(result) != o.dimensions {
		o.dimensions = len(result)
	}

	return result, nil
}

// EmbedBatch generates embeddings for multiple texts.
// Ollama doesn't have native batch support, so we call sequentially.
func (o *Ollama) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	// Instrument embedding latency for Prometheus metrics
	start := time.Now()
	defer func() {
		metrics.Metrics().EmbeddingLatency.Observe(time.Since(start).Seconds())
		metrics.Metrics().EmbeddingBatches.Inc()
	}()

	results := make([][]float32, len(texts))
	for i, text := range texts {
		emb, err := o.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("embed text %d: %w", i, err)
		}
		results[i] = emb
	}
	return results, nil
}

// Ollama API types

type ollamaEmbeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbeddingResponse struct {
	Embedding []float64 `json:"embedding"`
}
