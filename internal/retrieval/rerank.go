package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"mdemg/internal/circuitbreaker"
	"mdemg/internal/models"
)

// RerankRequest specifies what to re-rank
type RerankRequest struct {
	Query      string
	Candidates []models.RetrieveResult
	TopN       int // How many candidates to re-rank
	ReturnK    int // How many results to return
}

// RerankResult contains the re-ranked results with additional metadata
type RerankResult struct {
	Results      []models.RetrieveResult
	RerankScores []float64
	LatencyMs    float64
	TokensUsed   int
}

// Rerank uses an LLM to re-score candidates based on semantic relevance.
// It combines the original score with the LLM-assigned score using configured weights.
func (s *Service) Rerank(ctx context.Context, req RerankRequest) (*RerankResult, error) {
	if !s.cfg.RerankEnabled {
		return &RerankResult{Results: req.Candidates}, nil
	}

	if len(req.Candidates) == 0 {
		return &RerankResult{Results: req.Candidates}, nil
	}

	// Limit candidates to rerank
	topN := req.TopN
	if topN <= 0 {
		topN = s.cfg.RerankTopN
	}
	if topN > len(req.Candidates) {
		topN = len(req.Candidates)
	}

	returnK := req.ReturnK
	if returnK <= 0 {
		returnK = len(req.Candidates)
	}

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.RerankTimeoutMs)*time.Millisecond)
	defer cancel()

	start := time.Now()

	// Build the prompt
	prompt := buildRerankPrompt(req.Query, req.Candidates[:topN])

	// Call LLM based on provider
	var scores []float64
	var tokensUsed int
	var err error

	switch s.cfg.RerankProvider {
	case "openai":
		scores, tokensUsed, err = s.rerankWithOpenAI(timeoutCtx, prompt)
	case "ollama":
		scores, tokensUsed, err = s.rerankWithOllama(timeoutCtx, prompt)
	case "jina":
		scores, tokensUsed, err = s.rerankWithJina(timeoutCtx, req.Query, req.Candidates[:topN])
	case "neural":
		// Neural sidecar re-ranking (NR-3)
		docTexts := make([]string, topN)
		for i, c := range req.Candidates[:topN] {
			var sb strings.Builder
			sb.WriteString(c.Name)
			if c.Path != "" {
				sb.WriteString(" | ")
				sb.WriteString(c.Path)
			}
			if c.Summary != "" {
				sb.WriteString(" | ")
				sb.WriteString(c.Summary)
			}
			docTexts[i] = sb.String()
		}
		var neuralScores []float64
		neuralScores, err = neuralRerank(timeoutCtx, NeuralRerankConfig{
			URL:       s.cfg.NeuralRerankURL,
			TimeoutMs: s.cfg.NeuralRerankTimeoutMs,
			Fallback:  s.cfg.NeuralRerankFallback,
		}, req.Query, docTexts, topN, s.cbRegistry)
		scores = neuralScores
	default:
		scores, tokensUsed, err = s.rerankWithOpenAI(timeoutCtx, prompt)
	}

	if err != nil {
		// Return original results on error
		return &RerankResult{
			Results:   req.Candidates,
			LatencyMs: float64(time.Since(start).Milliseconds()),
		}, fmt.Errorf("rerank failed: %w", err)
	}

	// Combine original scores with rerank scores
	rerankWeight := s.cfg.RerankWeight
	originalWeight := 1.0 - rerankWeight

	type scoredResult struct {
		Result       models.RetrieveResult
		OriginalScore float64
		RerankScore   float64
		FinalScore    float64
	}

	scored := make([]scoredResult, 0, len(req.Candidates))

	// Score the re-ranked candidates
	for i, c := range req.Candidates[:topN] {
		rerankScore := 0.5 // Default if scores array is incomplete
		if i < len(scores) {
			rerankScore = scores[i]
		}

		finalScore := originalWeight*c.Score + rerankWeight*rerankScore
		scored = append(scored, scoredResult{
			Result:        c,
			OriginalScore: c.Score,
			RerankScore:   rerankScore,
			FinalScore:    finalScore,
		})
	}

	// Add remaining candidates (not re-ranked) with slightly penalized score
	for _, c := range req.Candidates[topN:] {
		// Candidates not re-ranked get their original score with small penalty
		finalScore := c.Score * 0.95
		scored = append(scored, scoredResult{
			Result:        c,
			OriginalScore: c.Score,
			RerankScore:   0.0,
			FinalScore:    finalScore,
		})
	}

	// Sort by final score
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].FinalScore > scored[j].FinalScore
	})

	// Build result
	results := make([]models.RetrieveResult, 0, returnK)
	rerankScores := make([]float64, 0, returnK)

	for i := 0; i < len(scored) && i < returnK; i++ {
		r := scored[i].Result
		r.Score = scored[i].FinalScore // Update score to final combined score
		results = append(results, r)
		rerankScores = append(rerankScores, scored[i].RerankScore)
	}

	result := &RerankResult{
		Results:      results,
		RerankScores: rerankScores,
		LatencyMs:    float64(time.Since(start).Milliseconds()),
		TokensUsed:   tokensUsed,
	}

	// Collect training data for neural re-ranker (NR-1)
	if s.dataCollector != nil {
		s.dataCollector.Collect(req.Query, req.Candidates[:topN], rerankScores, result.LatencyMs)
	}

	return result, nil
}

// buildRerankPrompt creates the prompt for the LLM
func buildRerankPrompt(query string, candidates []models.RetrieveResult) string {
	var sb strings.Builder

	sb.WriteString("You are a relevance judge for a code knowledge base.\n\n")
	sb.WriteString("Query: ")
	sb.WriteString(query)
	sb.WriteString("\n\n")
	sb.WriteString("Rate how relevant each candidate is to answering this query.\n")
	sb.WriteString("Score from 0.0 (irrelevant) to 1.0 (perfectly answers the query).\n")
	sb.WriteString("Consider: Does this code/document directly help answer the question?\n\n")
	sb.WriteString("Candidates:\n")

	for i, c := range candidates {
		sb.WriteString(fmt.Sprintf("[%d] %s\n", i, c.Name))
		sb.WriteString(fmt.Sprintf("    Path: %s\n", c.Path))
		if c.Summary != "" {
			sb.WriteString(fmt.Sprintf("    Summary: %s\n", c.Summary))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Return ONLY a JSON array of scores in order, like: [0.85, 0.32, 0.71, ...]\n")
	sb.WriteString("Do not include any other text or explanation.")

	return sb.String()
}

// OpenAI chat completion request/response structures
type openAIChatRequest struct {
	Model     string          `json:"model"`
	Messages  []openAIMessage `json:"messages"`
	MaxTokens int             `json:"max_completion_tokens"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

func (s *Service) rerankWithOpenAI(ctx context.Context, prompt string) ([]float64, int, error) {
	// Use circuit breaker if available
	if s.cbRegistry != nil {
		cb := s.cbRegistry.Get("openai-rerank")
		var scores []float64
		var tokens int
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			scores, tokens, innerErr = s.doRerankWithOpenAI(ctx, prompt)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return nil, 0, fmt.Errorf("openai rerank circuit breaker open")
		}
		return scores, tokens, err
	}

	return s.doRerankWithOpenAI(ctx, prompt)
}

// doRerankWithOpenAI performs the actual OpenAI rerank API call.
func (s *Service) doRerankWithOpenAI(ctx context.Context, prompt string) ([]float64, int, error) {
	reqBody := openAIChatRequest{
		Model: s.cfg.RerankModel,
		Messages: []openAIMessage{
			{Role: "user", Content: prompt},
		},
		MaxTokens: 2000, // Reasoning models consume tokens for internal thought
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal request: %w", err)
	}

	endpoint := s.cfg.EffectiveLLMEndpoint() + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.OpenAIAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("openai error %d: %s", resp.StatusCode, string(body))
	}

	var chatResp openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, 0, fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, 0, fmt.Errorf("no choices in response")
	}

	// Parse the scores from the response
	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	scores, err := parseScores(content)
	if err != nil {
		return nil, chatResp.Usage.TotalTokens, fmt.Errorf("parse scores: %w", err)
	}

	return scores, chatResp.Usage.TotalTokens, nil
}

// Ollama completion structures
type ollamaGenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
}

func (s *Service) rerankWithOllama(ctx context.Context, prompt string) ([]float64, int, error) {
	// Use circuit breaker if available
	if s.cbRegistry != nil {
		cb := s.cbRegistry.Get("ollama-rerank")
		var scores []float64
		var tokens int
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			scores, tokens, innerErr = s.doRerankWithOllama(ctx, prompt)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return nil, 0, fmt.Errorf("ollama rerank circuit breaker open")
		}
		return scores, tokens, err
	}

	return s.doRerankWithOllama(ctx, prompt)
}

// doRerankWithOllama performs the actual Ollama rerank API call.
func (s *Service) doRerankWithOllama(ctx context.Context, prompt string) ([]float64, int, error) {
	reqBody := ollamaGenerateRequest{
		Model:  s.cfg.RerankModel,
		Prompt: prompt,
		Stream: false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal request: %w", err)
	}

	endpoint := s.cfg.OllamaEndpoint + "/api/generate"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(body))
	}

	var ollamaResp ollamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, 0, fmt.Errorf("decode response: %w", err)
	}

	// Parse the scores from the response
	scores, err := parseScores(strings.TrimSpace(ollamaResp.Response))
	if err != nil {
		return nil, 0, fmt.Errorf("parse scores: %w", err)
	}

	return scores, 0, nil // Ollama doesn't report token usage in same way
}

// parseScores extracts a float array from LLM response
func parseScores(content string) ([]float64, error) {
	// Try to find JSON array in the response
	start := strings.Index(content, "[")
	end := strings.LastIndex(content, "]")

	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("no JSON array found in response: %s", content[:min(len(content), 100)])
	}

	jsonStr := content[start : end+1]

	var scores []float64
	if err := json.Unmarshal([]byte(jsonStr), &scores); err != nil {
		return nil, fmt.Errorf("unmarshal scores: %w (content: %s)", err, jsonStr[:min(len(jsonStr), 100)])
	}

	// Clamp scores to [0, 1] range
	for i := range scores {
		if scores[i] < 0 {
			scores[i] = 0
		}
		if scores[i] > 1 {
			scores[i] = 1
		}
	}

	return scores, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// --- Jina cross-encoder reranking ---

type jinaRerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n"`
}

type jinaRerankResponse struct {
	Results []jinaRerankResult `json:"results"`
}

type jinaRerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

func (s *Service) rerankWithJina(ctx context.Context, query string, candidates []models.RetrieveResult) ([]float64, int, error) {
	if s.cbRegistry != nil {
		cb := s.cbRegistry.Get("jina-rerank")
		var scores []float64
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			scores, innerErr = s.doRerankWithJina(ctx, query, candidates)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return nil, 0, fmt.Errorf("jina rerank circuit breaker open")
		}
		return scores, 0, err
	}
	scores, err := s.doRerankWithJina(ctx, query, candidates)
	return scores, 0, err
}

func (s *Service) doRerankWithJina(ctx context.Context, query string, candidates []models.RetrieveResult) ([]float64, error) {
	// Build document strings: "Name | Path | Summary"
	docs := make([]string, len(candidates))
	for i, c := range candidates {
		var sb strings.Builder
		sb.WriteString(c.Name)
		if c.Path != "" {
			sb.WriteString(" | ")
			sb.WriteString(c.Path)
		}
		if c.Summary != "" {
			sb.WriteString(" | ")
			sb.WriteString(c.Summary)
		}
		docs[i] = sb.String()
	}

	reqBody := jinaRerankRequest{
		Model:     s.cfg.RerankJinaModel,
		Query:     query,
		Documents: docs,
		TopN:      len(candidates),
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	endpoint := s.cfg.RerankJinaURL + "/rerank"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.RerankJinaKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("jina error %d: %s", resp.StatusCode, string(body))
	}

	var jinaResp jinaRerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&jinaResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Map Jina results back to ordered scores array
	scores := make([]float64, len(candidates))
	for i := range scores {
		scores[i] = 0.5 // Default if not in response
	}
	for _, r := range jinaResp.Results {
		if r.Index >= 0 && r.Index < len(scores) {
			scores[r.Index] = r.RelevanceScore
		}
	}

	return scores, nil
}
