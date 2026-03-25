package jiminy

import (
	"container/list"
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"sync"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/nrednav/cuid2"
	"mdemg/internal/circuitbreaker"
	"mdemg/internal/config"
	"mdemg/internal/embeddings"
	"mdemg/internal/llmclient"
)

// Evaluator assesses agent output against stored constraints and corrections.
type Evaluator struct {
	cfg        config.Config
	driver     neo4j.DriverWithContext
	embedder   embeddings.Embedder
	llm        *llmclient.Client          // J13: LLM for Tier 2 evaluation
	cbRegistry *circuitbreaker.Registry   // J13: circuit breaker registry

	// J13: LRU cache for evaluation results
	cacheMu   sync.Mutex
	cacheMap  map[string]*list.Element
	cacheList *list.List
	cacheCap  int
}

// evalCacheEntry holds a cached LLM evaluation result.
type evalCacheEntry struct {
	key    string
	result *llmEvalResult
}

// NewEvaluator creates a new Evaluator.
func NewEvaluator(cfg config.Config, driver neo4j.DriverWithContext, embedder embeddings.Embedder) *Evaluator {
	return &Evaluator{
		cfg:       cfg,
		driver:    driver,
		embedder:  embedder,
		cacheMap:  make(map[string]*list.Element),
		cacheList: list.New(),
		cacheCap:  512,
	}
}

// SetLLM configures the LLM client and circuit breaker registry for Tier 2 evaluation (J13).
func (e *Evaluator) SetLLM(llm *llmclient.Client, cbRegistry *circuitbreaker.Registry) {
	e.llm = llm
	e.cbRegistry = cbRegistry
}

// Evaluate checks agent output against constraints and corrections, returning findings.
// Tier 1: Vector similarity search. Tier 2 (J13): LLM reasoning for violation detection.
func (e *Evaluator) Evaluate(ctx context.Context, req EvaluateRequest) (EvaluateResponse, error) {
	if req.SpaceID == "" {
		return EvaluateResponse{}, fmt.Errorf("space_id is required")
	}
	if req.AgentOutput == "" {
		return EvaluateResponse{}, fmt.Errorf("agent_output is required")
	}

	evalID := cuid2.Generate()
	var constraintItems []EvaluationItem
	var correctionItems []EvaluationItem

	// Tier 1: Embed agent output and find candidate constraints/corrections via vector search
	if e.embedder != nil {
		embedding, err := e.embedder.Embed(ctx, req.AgentOutput)
		if err != nil {
			slog.Error("jiminy evaluator: embedding failed", "error", err)
		} else {
			cItems, cErr := e.findMatchingConstraints(ctx, req.SpaceID, embedding)
			if cErr != nil {
				slog.Error("jiminy evaluator: constraint search failed", "error", cErr)
			} else {
				constraintItems = append(constraintItems, cItems...)
			}

			corrItems, corrErr := e.findMatchingCorrections(ctx, req.SpaceID, embedding)
			if corrErr != nil {
				slog.Error("jiminy evaluator: correction search failed", "error", corrErr)
			} else {
				correctionItems = append(correctionItems, corrItems...)
			}
		}
	}

	allTier1Items := append(constraintItems, correctionItems...)

	// Tier 2 (J13): LLM evaluation of candidates against agent output
	if e.cfg.JiminyEvaluateLLMEnabled && e.llm != nil && len(allTier1Items) > 0 {
		llmItems, llmErr := e.llmEvaluate(ctx, req.AgentOutput, constraintItems, correctionItems)
		if llmErr != nil {
			slog.Warn("jiminy evaluator: LLM evaluation failed, using Tier 1 only", "error", llmErr)
			// Fail-open: use Tier 1 vector-only results
		} else if llmItems != nil {
			// Replace Tier 1 with revalidated LLM results
			allTier1Items = llmItems
		}
	}

	// Determine overall status
	status := "pass"
	for _, item := range allTier1Items {
		if item.Severity == "high" {
			status = "concern"
			break
		}
		if item.Severity == "medium" {
			status = "warning"
		}
	}

	// Build summary
	summary := ""
	if len(allTier1Items) > 0 {
		summary = fmt.Sprintf("Found %d potential concern(s) in agent output", len(allTier1Items))
	}

	// Ensure non-nil slice
	if allTier1Items == nil {
		allTier1Items = []EvaluationItem{}
	}

	return EvaluateResponse{
		EvaluationID: evalID,
		Status:       status,
		Items:        allTier1Items,
		Summary:      summary,
	}, nil
}

// llmEvaluate runs the LLM Tier 2 evaluation with cache and circuit breaker.
func (e *Evaluator) llmEvaluate(ctx context.Context, agentOutput string, constraints, corrections []EvaluationItem) ([]EvaluationItem, error) {
	// Build prompt
	prompt := buildEvalPrompt(agentOutput, constraints, corrections, e.cfg.JiminyEvaluateOutputMaxChars, e.cfg.JiminyEvaluateItemMaxChars)

	// Build constraint lookup map for revalidation
	constraintMap := make(map[string]EvaluationItem)
	for _, c := range constraints {
		if c.SourceNode != "" {
			constraintMap[c.SourceNode] = c
		}
	}
	for _, c := range corrections {
		if c.SourceNode != "" {
			constraintMap[c.SourceNode] = c
		}
	}

	maxTokens := e.cfg.JiminyEvaluateLLMMaxTokens
	if maxTokens < 2000 {
		maxTokens = 2000
	}

	msgs := []llmclient.Message{
		{Role: "system", Content: evalSystemPrompt},
		{Role: "user", Content: prompt},
	}

	opts := llmclient.CompleteOpts{MaxTokens: maxTokens}

	// Ollama: set JSON schema for structured output
	if e.cfg.JiminyEvaluateLLMProvider == "ollama" {
		opts.Format = ollamaEvalSchema
	}

	// Execute with circuit breaker
	cbName := "jiminy-eval-llm"
	if e.cfg.JiminyEvaluateLLMProvider == "ollama" {
		cbName = "jiminy-eval-llm-ollama"
	}

	var raw string

	if e.cbRegistry != nil {
		cb := e.cbRegistry.Get(cbName)
		err := cb.Execute(ctx, func(cbCtx context.Context) error {
			var innerErr error
			raw, _, innerErr = e.llm.CompleteWithUsage(cbCtx, msgs, opts)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return nil, fmt.Errorf("%s circuit breaker open", cbName)
		}
		if err != nil {
			return nil, fmt.Errorf("LLM evaluation: %w", err)
		}
	} else {
		var err error
		raw, _, err = e.llm.CompleteWithUsage(ctx, msgs, opts)
		if err != nil {
			return nil, fmt.Errorf("LLM evaluation: %w", err)
		}
	}

	// Parse response
	result, err := parseEvalResponse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse LLM eval response: %w", err)
	}

	// Revalidate: demote should/should_not violations to warnings
	items := revalidateResults(result, constraintMap)

	slog.Info("jiminy evaluator: LLM Tier 2 completed",
		"violations", len(result.Violations), "warnings", len(result.Warnings))

	return items, nil
}

// revalidateResults applies post-LLM revalidation rules following guardrail/response_builder.go.
// - LLM violation with must/must_not → stays as violation, severity high
// - LLM violation with should/should_not/correction → demoted to warning, severity medium
// - LLM violation with unknown node_id → demoted to warning
// - LLM warnings → kept as-is, severity medium
func revalidateResults(result *llmEvalResult, constraintMap map[string]EvaluationItem) []EvaluationItem {
	var items []EvaluationItem

	for _, v := range result.Violations {
		item := EvaluationItem{
			Content:     v.Description,
			Reasoning:   v.Reasoning,
			Remediation: v.Remediation,
			SourceNode:  v.ConstraintNodeID,
		}

		// Look up the original constraint to determine type
		original, found := constraintMap[v.ConstraintNodeID]
		if !found {
			// Unknown node_id → demote to warning
			item.Type = GuidanceConstraint
			item.Severity = "medium"
			item.Reasoning += " (constraint node not found; demoted)"
			items = append(items, item)
			continue
		}

		item.Type = original.Type
		cType := constraintTypeFromContent(original.Content)

		if cType == "must" || cType == "must_not" {
			// Hard constraints stay as violations
			item.Severity = "high"
		} else {
			// should/should_not/corrections demoted to warnings
			item.Severity = "medium"
			item.Reasoning += fmt.Sprintf(" (demoted: type is %s)", cType)
		}

		items = append(items, item)
	}

	for _, w := range result.Warnings {
		item := EvaluationItem{
			Content:     w.Description,
			Severity:    "medium",
			Reasoning:   w.Reasoning,
			Remediation: w.Remediation,
			SourceNode:  w.ConstraintNodeID,
		}
		if original, found := constraintMap[w.ConstraintNodeID]; found {
			item.Type = original.Type
		} else {
			item.Type = GuidanceCorrection
		}
		items = append(items, item)
	}

	return items
}

// --- LRU Cache for LLM evaluation results ---

// evalCacheKey builds a cache key from agent output + constraint node ID.
// Hashes the full output to avoid collisions on shared prefixes.
func evalCacheKey(agentOutput, constraintNodeID string) string {
	h := sha256.Sum256([]byte(agentOutput + ":" + constraintNodeID))
	return fmt.Sprintf("%x", h[:16])
}

// evalCacheGet retrieves a cached evaluation result.
func (e *Evaluator) evalCacheGet(key string) *llmEvalResult {
	e.cacheMu.Lock()
	defer e.cacheMu.Unlock()

	if elem, ok := e.cacheMap[key]; ok {
		e.cacheList.MoveToFront(elem)
		return elem.Value.(*evalCacheEntry).result
	}
	return nil
}

// evalCachePut stores an evaluation result in the LRU cache.
func (e *Evaluator) evalCachePut(key string, result *llmEvalResult) {
	e.cacheMu.Lock()
	defer e.cacheMu.Unlock()

	if elem, ok := e.cacheMap[key]; ok {
		e.cacheList.MoveToFront(elem)
		elem.Value.(*evalCacheEntry).result = result
		return
	}

	entry := &evalCacheEntry{key: key, result: result}
	elem := e.cacheList.PushFront(entry)
	e.cacheMap[key] = elem

	// Evict least recently used
	for e.cacheList.Len() > e.cacheCap {
		back := e.cacheList.Back()
		if back != nil {
			e.cacheList.Remove(back)
			delete(e.cacheMap, back.Value.(*evalCacheEntry).key)
		}
	}
}

// findMatchingConstraints searches for constraints that might be violated by the agent output.
func (e *Evaluator) findMatchingConstraints(ctx context.Context, spaceID string, embedding []float32) ([]EvaluationItem, error) {
	if e.driver == nil {
		return nil, nil
	}

	maxConstraints := e.cfg.JiminyEvaluateMaxConstraints
	if maxConstraints <= 0 {
		maxConstraints = 10
	}

	sess := e.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx) //nolint:errcheck

	indexName := e.cfg.VectorIndexName
	if indexName == "" {
		indexName = "memNodeEmbedding"
	}

	cypher := `
	CALL db.index.vector.queryNodes($indexName, 200, $embedding)
	YIELD node AS c, score AS sim
	WHERE c.space_id = $spaceId
	  AND c.role_type = 'constraint'
	  AND NOT coalesce(c.is_archived, false)
	  AND sim > 0.4
	RETURN c.node_id AS node_id, c.name AS name,
	       c.constraint_type AS constraint_type,
	       c.content AS content, sim
	ORDER BY sim DESC LIMIT $maxResults`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, txErr := tx.Run(ctx, cypher, map[string]any{
			"spaceId":    spaceID,
			"embedding":  embedding,
			"indexName":  indexName,
			"maxResults": maxConstraints,
		})
		if txErr != nil {
			return nil, txErr
		}

		var items []EvaluationItem
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("node_id")
			name, _ := rec.Get("name")
			cType, _ := rec.Get("constraint_type")
			content, _ := rec.Get("content")
			sim, _ := rec.Get("sim")

			severity := "medium"
			cTypeStr := asStringVal(cType)
			if cTypeStr == "must" || cTypeStr == "must_not" {
				severity = "high"
			}

			desc := asStringVal(name)
			if c := asStringVal(content); c != "" {
				desc = c
			}

			simVal := asFloat64Val(sim)
			items = append(items, EvaluationItem{
				Type:       GuidanceConstraint,
				Content:    fmt.Sprintf("[%s] %s (sim: %.2f)", cTypeStr, desc, simVal),
				Severity:   severity,
				SourceNode: asStringVal(nodeID),
			})
		}
		return items, res.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("constraint search: %w", err)
	}
	if result == nil {
		return nil, nil
	}
	return result.([]EvaluationItem), nil
}

// findMatchingCorrections searches for corrections relevant to the agent output.
func (e *Evaluator) findMatchingCorrections(ctx context.Context, spaceID string, embedding []float32) ([]EvaluationItem, error) {
	if e.driver == nil {
		return nil, nil
	}

	sess := e.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx) //nolint:errcheck

	indexName := e.cfg.VectorIndexName
	if indexName == "" {
		indexName = "memNodeEmbedding"
	}

	cypher := `
	CALL db.index.vector.queryNodes($indexName, 200, $embedding)
	YIELD node AS c, score AS sim
	WHERE c.space_id = $spaceId
	  AND c.obs_type = 'correction'
	  AND sim > 0.5
	RETURN c.node_id AS node_id, c.name AS name,
	       c.content AS content, c.summary AS summary, sim
	ORDER BY sim DESC LIMIT 5`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, txErr := tx.Run(ctx, cypher, map[string]any{
			"spaceId":   spaceID,
			"embedding": embedding,
			"indexName": indexName,
		})
		if txErr != nil {
			return nil, txErr
		}

		var items []EvaluationItem
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("node_id")
			content, _ := rec.Get("content")
			summary, _ := rec.Get("summary")
			sim, _ := rec.Get("sim")

			desc := asStringVal(content)
			if desc == "" {
				desc = asStringVal(summary)
			}

			simVal := asFloat64Val(sim)
			items = append(items, EvaluationItem{
				Type:       GuidanceCorrection,
				Content:    fmt.Sprintf("Prior correction: %s (sim: %.2f)", desc, simVal),
				Severity:   "medium",
				SourceNode: asStringVal(nodeID),
			})
		}
		return items, res.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("correction search: %w", err)
	}
	if result == nil {
		return nil, nil
	}
	return result.([]EvaluationItem), nil
}

// asStringVal safely converts interface{} to string.
func asStringVal(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// asFloat64Val safely converts interface{} to float64.
func asFloat64Val(v any) float64 {
	if v == nil {
		return 0.0
	}
	switch f := v.(type) {
	case float64:
		return f
	case int64:
		return float64(f)
	default:
		return 0.0
	}
}
