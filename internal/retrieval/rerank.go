package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"mdemg/internal/circuitbreaker"
	"mdemg/internal/encoding"
	"mdemg/internal/llmclient"
	"mdemg/internal/metrics"
	"mdemg/internal/models"
)

// rerankTemperature is the fixed temperature for rerank_cross LLM calls
// (RERANK-LENGTH-STRICT-001, 2026-08-13). Reranking is a relevance-scoring
// task — the correct temperature is 0 (deterministic). Pre-fix, the call
// omitted Temperature so the LLM used its server-side default (typically
// 0.8), producing Fable's observed nondeterminism ("same query + candidates
// gives different score arrays across rows").
var rerankTemperature = 0.0

const rerankCrossSystemPrompt = `You are a relevance judge for a code knowledge base. Rate how relevant each candidate is to answering a query. Score from 0.0 (irrelevant) to 1.0 (perfectly answers the query). Consider: Does this code/document directly help answer the question? Return ONLY a JSON array of scores in order. Do not include any other text or explanation.`

const rerankNLISystemPrompt = `Relevance judge: rate each candidate 0.0-1.0 for answering the query. Respond as JSON array of scores.`

// RerankRequest specifies what to re-rank
type RerankRequest struct {
	SpaceID    string // Propagated to TSDB via WithContext for correct space attribution
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
	// SEC-TRANCHE-3: defensive cap on the allocation driven by returnK.
	// `req.ReturnK` and `len(req.Candidates)` are both effectively
	// operator-tunable; hard cap so a request cannot force a huge
	// preallocation on `results`/`rerankScores` below.
	if returnK > 10000 {
		returnK = 10000
	}

	// LLM-HEALTH-INVESTIGATION-001 E2 + NEURAL-RERANK-PRECHECK-001: pre-check
	// the CALLER'S deadline before dispatching. If remaining < min budget the
	// call is guaranteed to be canceled — skip and return pre-rerank RRF order
	// (fail-open). Deadline-absent callers (CLI, tests) bypass the check.
	//
	// Budget is provider-aware: rerank_cross via LLM (openai/ollama) has
	// p99=~11.7s → RerankMinBudgetMs default 12000; neural sidecar has
	// default timeout 1000ms → NeuralRerankMinBudgetMs default 1500. Applying
	// the LLM budget to a neural call would over-skip callers that had plenty
	// of budget for neural.
	minBudgetMs := s.cfg.RerankMinBudgetMs
	if s.cfg.RerankProvider == "neural" {
		minBudgetMs = s.cfg.NeuralRerankMinBudgetMs
	}
	if minBudgetMs > 0 {
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining < time.Duration(minBudgetMs)*time.Millisecond {
				slog.Warn("rerank skipped: insufficient budget",
					"remaining_ms", remaining.Milliseconds(),
					"min_budget_ms", minBudgetMs,
					"provider", s.cfg.RerankProvider,
					"space_id", req.SpaceID,
					"candidates", len(req.Candidates))
				return &RerankResult{
					Results:   req.Candidates,
					LatencyMs: 0,
				}, nil
			}
		}
	}

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.RerankTimeoutMs)*time.Millisecond)
	defer cancel()

	start := time.Now()

	// Build the prompt
	prompt := buildRerankPrompt(req.Query, req.Candidates[:topN], s.cfg.RerankCompress)
	systemPrompt := rerankCrossSystemPrompt
	if s.cfg.RerankCompress {
		systemPrompt = rerankNLISystemPrompt
	}

	// Wire RAFT retrieval context for training data capture
	if topN > 0 {
		var nodeIDs []string
		var candidateScores []float64
		for _, c := range req.Candidates[:topN] {
			nodeIDs = append(nodeIDs, c.NodeID)
			candidateScores = append(candidateScores, c.Score)
		}
		if len(nodeIDs) > 0 {
			timeoutCtx = llmclient.WithRetrievalContext(timeoutCtx, &llmclient.RetrievalContext{
				NodeIDs: nodeIDs,
				Scores:  candidateScores,
			})
		}
	}

	// Call LLM based on provider
	var scores []float64
	var tokensUsed int
	var err error

	switch s.cfg.RerankProvider {
	case "openai":
		scores, tokensUsed, err = s.rerankWithOpenAI(timeoutCtx, systemPrompt, prompt, req.SpaceID)
	case "ollama":
		scores, tokensUsed, err = s.rerankWithOllama(timeoutCtx, systemPrompt, prompt, req.SpaceID)
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
		scores, tokensUsed, err = s.rerankWithOpenAI(timeoutCtx, systemPrompt, prompt, req.SpaceID)
	}

	// RERANK-LENGTH-STRICT-001 (2026-08-13): length-mismatch observability
	// + one corrective retry. If the initial call returned a valid parse
	// but the array length ≠ candidate count, log a WARN, increment the
	// length-mismatch counter (by reason: short/long), and retry ONCE
	// with a corrective prompt. If the retry recovers a matching length,
	// swap in the corrected scores + increment the retry_recovered counter.
	// If the retry still mismatches, keep the original scores + let the
	// downstream `if i < len(scores)` guard default missing entries to
	// 0.5. Retry is single-attempt to bound worst-case latency.
	//
	// Applies only to LLM providers (openai/ollama); neural + jina paths
	// return their own length-bounded outputs by construction.
	if err == nil && len(scores) != topN && (s.cfg.RerankProvider == "openai" || s.cfg.RerankProvider == "ollama" || s.cfg.RerankProvider == "") {
		reason := "short"
		if len(scores) > topN {
			reason = "long"
		}
		slog.Warn("rerank: length mismatch",
			"provider", s.cfg.RerankProvider,
			"expected", topN,
			"got", len(scores),
			"reason", reason,
			"space_id", req.SpaceID,
		)
		metrics.Metrics().RetrievalRerankLengthMismatch(reason).Inc()

		// One corrective retry with the expected count named verbatim.
		retryPrompt := buildRerankRetryPrompt(prompt, topN, len(scores))
		var retryScores []float64
		var retryTokens int
		var retryErr error
		switch s.cfg.RerankProvider {
		case "openai", "":
			retryScores, retryTokens, retryErr = s.rerankWithOpenAI(timeoutCtx, systemPrompt, retryPrompt, req.SpaceID)
		case "ollama":
			retryScores, retryTokens, retryErr = s.rerankWithOllama(timeoutCtx, systemPrompt, retryPrompt, req.SpaceID)
		}
		if retryErr == nil && len(retryScores) == topN {
			slog.Info("rerank: length mismatch recovered via corrective retry",
				"provider", s.cfg.RerankProvider,
				"expected", topN,
				"space_id", req.SpaceID,
			)
			metrics.Metrics().RetrievalRerankLengthMismatch("retry_recovered").Inc()
			scores = retryScores
			tokensUsed += retryTokens
		}
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
		Result        models.RetrieveResult
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

	// Collect training data for neural re-ranker (NR-1).
	// SIDECAR-LOOP-001: log result.Results + result.RerankScores — these are
	// built together from the SORTED scored slice, so candidate[i] aligns 1:1
	// (length AND position) with rerank_scores[i]. The previous call passed
	// req.Candidates[:topN] (unsorted input) against the sorted scores, which
	// mislabeled 100% of records (84% length-mismatched, the rest positionally
	// wrong) — making the collected corpus untrainable.
	if s.dataCollector != nil {
		s.dataCollector.Collect(req.Query, result.Results, result.RerankScores, result.LatencyMs)
	}

	return result, nil
}

// buildRerankPrompt creates the prompt for the LLM.
// When compress is true, uses compact single-line format to reduce tokens.
func buildRerankPrompt(query string, candidates []models.RetrieveResult, compress bool) string {
	var sb strings.Builder

	sb.WriteString("Query: ")
	sb.WriteString(query)
	sb.WriteString("\n\nCandidates:\n")

	for i, c := range candidates {
		summary := c.Summary
		if len(summary) > 300 {
			summary = encoding.TruncateAtWord(summary, 300)
		}
		if compress {
			// Single-line pipe-separated format
			sb.WriteString(fmt.Sprintf("[%d] %s | %s", i, c.Name, c.Path))
			if summary != "" {
				sb.WriteString(" | ")
				sb.WriteString(summary)
			}
			sb.WriteString("\n")
		} else {
			sb.WriteString(fmt.Sprintf("[%d] %s\n", i, c.Name))
			sb.WriteString(fmt.Sprintf("    Path: %s\n", c.Path))
			if summary != "" {
				sb.WriteString(fmt.Sprintf("    Summary: %s\n", summary))
			}
			sb.WriteString("\n")
		}
	}

	// RERANK-LENGTH-STRICT-001 (2026-08-13): explicit count-of-scores
	// contract at the bottom of the user prompt. The system prompt says
	// "Return ONLY a JSON array of scores in order" but doesn't name N,
	// and Fable HITL pass 2 observed the LLM returning 15-cand queries as
	// 16-17-score arrays. Naming N in the immediate context of the
	// candidates list (bottom of prompt) shifts LLM attention to the
	// count constraint at generation time.
	sb.WriteString(fmt.Sprintf("\nReturn exactly %d scores in a JSON array, one per candidate, in the same order as the candidates above.\n", len(candidates)))

	return sb.String()
}

// buildRerankRetryPrompt constructs a corrective user prompt for the
// RERANK-LENGTH-STRICT-001 one-retry pathway. Fires only when the initial
// call returned a scores array whose length differs from the candidate
// count. The retry names both the previous count and the expected count
// verbatim, plus re-appends the required count contract.
func buildRerankRetryPrompt(originalPrompt string, expected, got int) string {
	var sb strings.Builder
	sb.WriteString("Your previous response returned ")
	sb.WriteString(fmt.Sprintf("%d scores but there are %d candidates. ", got, expected))
	sb.WriteString(fmt.Sprintf("Return exactly %d scores, one per candidate, in the same order as the candidates.\n\n", expected))
	sb.WriteString(originalPrompt)
	return sb.String()
}

func (s *Service) rerankWithOpenAI(ctx context.Context, systemPrompt, prompt, spaceID string) ([]float64, int, error) {
	// Use circuit breaker if available
	if s.cbRegistry != nil {
		cb := s.cbRegistry.Get("openai-rerank")
		var scores []float64
		var tokens int
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			scores, tokens, innerErr = s.doRerankWithOpenAI(ctx, systemPrompt, prompt, spaceID)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return nil, 0, fmt.Errorf("openai rerank circuit breaker open")
		}
		return scores, tokens, err
	}

	return s.doRerankWithOpenAI(ctx, systemPrompt, prompt, spaceID)
}

// doRerankWithOpenAI performs the actual OpenAI rerank API call.
func (s *Service) doRerankWithOpenAI(ctx context.Context, systemPrompt, prompt, spaceID string) ([]float64, int, error) {
	s.rerankOpenAIOnce.Do(func() {
		s.rerankOpenAIClient = llmclient.New(llmclient.Config{
			Provider: "openai",
			Model:    s.cfg.RerankModel,
			APIKey:   s.cfg.OpenAIAPIKey,
			BaseURL:  s.cfg.EffectiveLLMEndpoint(),
		})
	})
	client := s.rerankOpenAIClient.WithContext("retrieval.rerank_cross", spaceID)

	msgs := []llmclient.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}

	content, _, tokens, err := client.CompleteWithUsage(ctx, msgs, llmclient.CompleteOpts{
		MaxTokens:   2000, // Reasoning models consume tokens for internal thought
		Temperature: &rerankTemperature, // RERANK-LENGTH-STRICT-001: deterministic scoring
	})
	if err != nil {
		return nil, 0, err
	}

	// Parse the scores from the response
	scores, err := parseScores(content)
	if err != nil {
		return nil, tokens, fmt.Errorf("parse scores: %w", err)
	}

	return scores, tokens, nil
}

func (s *Service) rerankWithOllama(ctx context.Context, systemPrompt, prompt, spaceID string) ([]float64, int, error) {
	// Use circuit breaker if available
	if s.cbRegistry != nil {
		cb := s.cbRegistry.Get("ollama-rerank")
		var scores []float64
		var tokens int
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			scores, tokens, innerErr = s.doRerankWithOllama(ctx, systemPrompt, prompt, spaceID)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return nil, 0, fmt.Errorf("ollama rerank circuit breaker open")
		}
		return scores, tokens, err
	}

	return s.doRerankWithOllama(ctx, systemPrompt, prompt, spaceID)
}

// doRerankWithOllama performs the actual Ollama rerank API call.
func (s *Service) doRerankWithOllama(ctx context.Context, systemPrompt, prompt, spaceID string) ([]float64, int, error) {
	s.rerankOllamaOnce.Do(func() {
		s.rerankOllamaClient = llmclient.New(llmclient.Config{
			Provider: "ollama",
			Model:    s.cfg.RerankModel,
			BaseURL:  s.cfg.OllamaEndpoint,
		})
	})
	client := s.rerankOllamaClient.WithContext("retrieval.rerank_nli", spaceID)

	msgs := []llmclient.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}

	content, err := client.Complete(ctx, msgs, llmclient.CompleteOpts{
		Temperature: &rerankTemperature, // RERANK-LENGTH-STRICT-001: deterministic scoring
	})
	if err != nil {
		return nil, 0, err
	}

	// Parse the scores from the response
	scores, err := parseScores(content)
	if err != nil {
		return nil, 0, fmt.Errorf("parse scores: %w", err)
	}

	return scores, 0, nil // Ollama doesn't report token usage
}

// parseScores extracts a float array from LLM response
func parseScores(content string) ([]float64, error) {
	content = llmclient.SanitizeResponse(content)
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
