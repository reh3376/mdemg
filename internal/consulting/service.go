// Package consulting provides the Agent Consulting Service.
// This service acts as an SME (Subject Matter Expert) for coding agents,
// providing context-aware suggestions based on accumulated knowledge.
package consulting

import (
	"context"
	"fmt"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
	"mdemg/internal/embeddings"
	"mdemg/internal/models"
	"mdemg/internal/retrieval"
	"mdemg/internal/symbols"
)

// MaxConfidence is the upper bound for confidence scores.
// From a Bayesian perspective, absolute certainty (1.0) is epistemologically
// invalid - no prediction system should claim 100% confidence as it leaves
// no room for belief updating with new evidence.
const MaxConfidence = 0.95

// Retriever defines the interface for retrieving memory nodes.
// This interface allows for mocking in tests.
type Retriever interface {
	Retrieve(ctx context.Context, req models.RetrieveRequest) (models.RetrieveResponse, error)
}

// SymbolLookup defines the interface for looking up symbols.
// This interface allows for mocking in tests.
type SymbolLookup interface {
	GetSymbolsForMemoryNode(ctx context.Context, spaceID, nodeID string) ([]symbols.SymbolRecord, error)
}

// ConceptFetcher defines the interface for fetching related concepts.
// This interface allows for mocking in tests.
type ConceptFetcher interface {
	FetchRelatedConcepts(ctx context.Context, spaceID string, results []models.RetrieveResult) ([]models.RelatedConcept, error)
}

// IntentTranslator rewrites a conversational query into keyword-dense text
// for vector search. Defined locally to avoid circular imports with retrieval package.
// Go implicit interface satisfaction means *retrieval.LLMIntentTranslator satisfies this.
type IntentTranslator interface {
	Translate(ctx context.Context, query string) (string, error)
}

// Service provides the Agent Consulting API.
type Service struct {
	cfg              config.Config
	driver           neo4j.DriverWithContext
	retriever        Retriever
	embedder         embeddings.Embedder
	symbolStore      SymbolLookup
	conceptFetcher   ConceptFetcher   // Optional: if nil, uses internal fetchRelatedConcepts
	synthesizer      Synthesizer      // Optional: if nil, LLM synthesis is skipped (Phase 101)
	intentTranslator IntentTranslator // Optional: if nil, intent translation is skipped (Phase 102)
}

// NewService creates a new consulting service.
func NewService(cfg config.Config, driver neo4j.DriverWithContext, retriever *retrieval.Service, embedder embeddings.Embedder, symbolStore *symbols.Store, synthesizer Synthesizer, intentTranslator IntentTranslator) *Service {
	var r Retriever
	if retriever != nil {
		r = retriever
	}
	var ss SymbolLookup
	if symbolStore != nil {
		ss = symbolStore
	}
	return &Service{
		cfg:              cfg,
		driver:           driver,
		retriever:        r,
		embedder:         embedder,
		symbolStore:      ss,
		synthesizer:      synthesizer,
		intentTranslator: intentTranslator,
	}
}

// NewServiceWithMocks creates a consulting service with mock dependencies for testing.
func NewServiceWithMocks(cfg config.Config, driver neo4j.DriverWithContext, retriever Retriever, embedder embeddings.Embedder, symbolStore SymbolLookup, synthesizer Synthesizer, intentTranslator IntentTranslator) *Service {
	return &Service{
		cfg:              cfg,
		driver:           driver,
		retriever:        retriever,
		embedder:         embedder,
		symbolStore:      symbolStore,
		synthesizer:      synthesizer,
		intentTranslator: intentTranslator,
	}
}

// NewServiceWithAllMocks creates a consulting service with all mock dependencies including concept fetcher.
func NewServiceWithAllMocks(cfg config.Config, retriever Retriever, embedder embeddings.Embedder, symbolStore SymbolLookup, conceptFetcher ConceptFetcher, synthesizer Synthesizer, intentTranslator IntentTranslator) *Service {
	return &Service{
		cfg:              cfg,
		retriever:        retriever,
		embedder:         embedder,
		symbolStore:      symbolStore,
		conceptFetcher:   conceptFetcher,
		synthesizer:      synthesizer,
		intentTranslator: intentTranslator,
	}
}

// Consult processes a consultation request and returns SME suggestions.
func (s *Service) Consult(ctx context.Context, req models.ConsultRequest) (models.ConsultResponse, error) {
	resp := models.ConsultResponse{
		SpaceID:     req.SpaceID,
		Suggestions: []models.Suggestion{},
		Debug:       make(map[string]any),
	}

	maxSuggestions := req.MaxSuggestions
	if maxSuggestions <= 0 {
		maxSuggestions = 5
	}

	// Combine context and question for retrieval
	queryText := fmt.Sprintf("%s\n\nQuestion: %s", req.Context, req.Question)

	// Phase 102: Intent Translation — rewrite query for embedding, keep original for synthesis
	var translatedIntent string
	if req.TranslateIntent && s.intentTranslator != nil {
		translated, translateErr := s.intentTranslator.Translate(ctx, queryText)
		if translateErr == nil && translated != queryText {
			translatedIntent = translated
			queryText = translated // Used for embedding + retrieval
		}
	}

	// Generate query embedding (required by retrieval service)
	if s.embedder == nil {
		return resp, fmt.Errorf("no embedding provider configured")
	}
	queryEmbedding, err := s.embedder.Embed(ctx, queryText)
	if err != nil {
		return resp, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// Step 1: Retrieve relevant memories using existing retrieval pipeline
	retrieveReq := models.RetrieveRequest{
		SpaceID:        req.SpaceID,
		QueryText:      queryText,
		QueryEmbedding: queryEmbedding,
		TopK:           maxSuggestions * 3, // Get more candidates for filtering
		HopDepth:       2,                  // Include related nodes
	}

	retrieveResp, err := s.retriever.Retrieve(ctx, retrieveReq)
	if err != nil {
		return resp, fmt.Errorf("retrieval failed: %w", err)
	}

	resp.Debug["retrieved_count"] = len(retrieveResp.Results)

	// Step 2: Fetch concept nodes (higher layers) for related concepts
	var concepts []models.RelatedConcept
	if s.conceptFetcher != nil {
		concepts, err = s.conceptFetcher.FetchRelatedConcepts(ctx, req.SpaceID, retrieveResp.Results)
	} else {
		concepts, err = s.fetchRelatedConcepts(ctx, req.SpaceID, retrieveResp.Results)
	}
	if err != nil {
		// Log but don't fail - concepts are optional enrichment
		resp.Debug["concept_error"] = err.Error()
	} else {
		resp.RelatedConcepts = concepts
	}

	// Step 3: Generate suggestions from retrieved results
	suggestions, err := s.generateSuggestions(ctx, req, retrieveResp.Results)
	if err != nil {
		return resp, fmt.Errorf("suggestion generation failed: %w", err)
	}

	// Limit to max suggestions
	if len(suggestions) > maxSuggestions {
		suggestions = suggestions[:maxSuggestions]
	}

	// Step 4: Optionally enrich with symbol evidence
	if req.IncludeEvidence && s.symbolStore != nil {
		for i := range suggestions {
			for _, nodeID := range suggestions[i].SourceNodes {
				syms, err := s.symbolStore.GetSymbolsForMemoryNode(ctx, req.SpaceID, nodeID)
				if err != nil {
					continue
				}
				for _, sym := range syms {
					suggestions[i].Evidence = append(suggestions[i].Evidence, models.SymbolEvidence{
						SymbolName: sym.Name,
						SymbolType: sym.SymbolType,
						FilePath:   sym.FilePath,
						Line:       sym.Line,
						LineEnd:    sym.LineEnd,
						Value:      sym.Value,
						RawValue:   sym.RawValue,
						Signature:  sym.Signature,
						DocComment: sym.DocComment,
					})
				}
			}
		}
	}

	resp.Suggestions = suggestions

	// Step 4.5: LLM Synthesis (Phase 101)
	if req.LlmSynthesis && s.synthesizer != nil {
		synthReq := SynthesisRequest{
			SpaceID:     req.SpaceID,
			Question:    req.Question,
			Context:     req.Context,
			Results:     retrieveResp.Results,
			Concepts:    concepts,
			Suggestions: suggestions,
		}
		synthResult, synthErr := s.synthesizer.Synthesize(ctx, synthReq)
		if synthErr != nil {
			// Graceful fallback: log, populate debug, do NOT fail the request
			resp.Debug["synthesis_error"] = synthErr.Error()
			resp.Debug["synthesis_fallback"] = true
		} else {
			resp.Synthesis = synthResult.Narrative
			resp.Debug["synthesis_tokens"] = synthResult.TokensUsed
			resp.Debug["synthesis_latency_ms"] = synthResult.LatencyMs
			resp.Debug["synthesis_provider"] = synthResult.Provider
			resp.Debug["synthesis_model"] = synthResult.Model
		}
	}

	// Step 5: Calculate overall confidence and rationale
	resp.Confidence = s.calculateOverallConfidence(suggestions, len(retrieveResp.Results))
	resp.Rationale = s.generateRationale(req, suggestions, concepts)

	// Phase 102: Include translated intent in response
	if translatedIntent != "" {
		resp.TranslatedIntent = translatedIntent
	}

	return resp, nil
}

// fetchRelatedConcepts retrieves concept nodes (layer >= 2) related to the results.
func (s *Service) fetchRelatedConcepts(ctx context.Context, spaceID string, results []models.RetrieveResult) ([]models.RelatedConcept, error) {
	if len(results) == 0 {
		return nil, nil
	}

	if s.driver == nil {
		return nil, fmt.Errorf("no database driver configured")
	}

	// Collect node IDs from results
	nodeIDs := make([]string, 0, len(results))
	for _, r := range results {
		nodeIDs = append(nodeIDs, r.NodeID)
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceId": spaceID,
		"nodeIds": nodeIDs,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Find concept nodes connected to the retrieved nodes
		cypher := `
MATCH (n:MemoryNode {space_id: $spaceId})-[:ABSTRACTS_TO*1..3]->(c:MemoryNode {space_id: $spaceId})
WHERE n.node_id IN $nodeIds AND c.layer >= 2
WITH c, count(DISTINCT n) AS relevance_count
RETURN c.node_id AS node_id,
       coalesce(c.name, c.node_id) AS name,
       c.summary AS summary,
       c.layer AS layer,
       toFloat(relevance_count) / toFloat(size($nodeIds)) AS relevance
ORDER BY relevance DESC, c.layer ASC
LIMIT 5`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var concepts []models.RelatedConcept
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("node_id")
			name, _ := rec.Get("name")
			summary, _ := rec.Get("summary")
			layer, _ := rec.Get("layer")
			relevance, _ := rec.Get("relevance")

			concept := models.RelatedConcept{
				NodeID:    fmt.Sprint(nodeID),
				Name:      fmt.Sprint(name),
				Layer:     int(layer.(int64)),
				Relevance: relevance.(float64),
			}
			if summary != nil {
				concept.Summary = fmt.Sprint(summary)
			}
			concepts = append(concepts, concept)
		}
		return concepts, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]models.RelatedConcept), nil
}

// generateSuggestions creates categorized suggestions from retrieved results.
func (s *Service) generateSuggestions(ctx context.Context, req models.ConsultRequest, results []models.RetrieveResult) ([]models.Suggestion, error) {
	var suggestions []models.Suggestion

	// Categorize results and generate suggestions
	for _, r := range results {
		suggestionType := s.classifySuggestionType(r)
		content := s.formatSuggestionContent(suggestionType, r, req.Question)

		if content == "" {
			continue
		}

		suggestions = append(suggestions, models.Suggestion{
			Type:        suggestionType,
			Content:     content,
			Confidence:  r.Score, // Use retrieval score as base confidence
			SourceNodes: []string{r.NodeID},
		})
	}

	// Sort by confidence and deduplicate similar suggestions
	suggestions = s.deduplicateSuggestions(suggestions)

	return suggestions, nil
}

// classifySuggestionType determines the type of suggestion based on result characteristics.
func (s *Service) classifySuggestionType(r models.RetrieveResult) models.SuggestionType {
	name := strings.ToLower(r.Name)
	path := strings.ToLower(r.Path)
	summary := strings.ToLower(r.Summary)

	// Risk indicators
	riskKeywords := []string{"error", "fail", "issue", "problem", "bug", "fix", "workaround", "hack", "todo", "deprecated"}
	for _, kw := range riskKeywords {
		if strings.Contains(name, kw) || strings.Contains(summary, kw) {
			return models.SuggestionRisk
		}
	}

	// Process indicators (workflows, procedures)
	processKeywords := []string{"workflow", "process", "step", "guide", "howto", "readme", "doc", "procedure"}
	for _, kw := range processKeywords {
		if strings.Contains(name, kw) || strings.Contains(path, kw) {
			return models.SuggestionProcess
		}
	}

	// Concept indicators (patterns, architecture)
	conceptKeywords := []string{"pattern", "architecture", "design", "concept", "abstract", "interface", "protocol"}
	for _, kw := range conceptKeywords {
		if strings.Contains(name, kw) || strings.Contains(path, kw) || strings.Contains(summary, kw) {
			return models.SuggestionConcept
		}
	}

	// Default to context
	return models.SuggestionContext
}

// formatSuggestionContent formats the suggestion text based on type.
func (s *Service) formatSuggestionContent(suggType models.SuggestionType, r models.RetrieveResult, question string) string {
	// Use summary if available, otherwise use name
	content := r.Summary
	if content == "" {
		content = r.Name
	}
	if content == "" {
		return ""
	}

	// Truncate long content
	if len(content) > 500 {
		content = content[:497] + "..."
	}

	switch suggType {
	case models.SuggestionContext:
		return fmt.Sprintf("Based on this codebase's patterns: %s", content)
	case models.SuggestionProcess:
		return fmt.Sprintf("The typical workflow for this type of change: %s", content)
	case models.SuggestionConcept:
		return fmt.Sprintf("This relates to the higher-level principle: %s", content)
	case models.SuggestionRisk:
		return fmt.Sprintf("Caution - previous related finding: %s", content)
	default:
		return content
	}
}

// deduplicateSuggestions removes similar suggestions and sorts by confidence.
func (s *Service) deduplicateSuggestions(suggestions []models.Suggestion) []models.Suggestion {
	if len(suggestions) <= 1 {
		return suggestions
	}

	// Simple deduplication by checking content similarity
	seen := make(map[string]bool)
	var unique []models.Suggestion

	for _, sugg := range suggestions {
		// Create a simple key from first 50 chars of content
		key := sugg.Content
		if len(key) > 50 {
			key = key[:50]
		}
		key = strings.ToLower(key)

		if !seen[key] {
			seen[key] = true
			unique = append(unique, sugg)
		}
	}

	return unique
}

// calculateOverallConfidence computes an overall confidence score.
func (s *Service) calculateOverallConfidence(suggestions []models.Suggestion, totalRetrieved int) float64 {
	if len(suggestions) == 0 {
		return 0.0
	}

	// Average confidence of suggestions, weighted by retrieval coverage
	var sum float64
	for _, sugg := range suggestions {
		sum += sugg.Confidence
	}
	avgConfidence := sum / float64(len(suggestions))

	// Boost if we found many relevant results
	coverageBoost := 1.0
	if totalRetrieved >= 5 {
		coverageBoost = 1.1
	}
	if totalRetrieved >= 10 {
		coverageBoost = 1.2
	}

	confidence := avgConfidence * coverageBoost
	if confidence > MaxConfidence {
		confidence = MaxConfidence
	}
	return confidence
}

// generateRationale creates an explanation for the suggestions.
func (s *Service) generateRationale(req models.ConsultRequest, suggestions []models.Suggestion, concepts []models.RelatedConcept) string {
	if len(suggestions) == 0 {
		return "No relevant patterns found in the knowledge base for this query."
	}

	// Count suggestion types
	typeCounts := make(map[models.SuggestionType]int)
	for _, sugg := range suggestions {
		typeCounts[sugg.Type]++
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("Found %d relevant patterns", len(suggestions)))

	if typeCounts[models.SuggestionContext] > 0 {
		parts = append(parts, fmt.Sprintf("%d context patterns", typeCounts[models.SuggestionContext]))
	}
	if typeCounts[models.SuggestionRisk] > 0 {
		parts = append(parts, fmt.Sprintf("%d risk warnings", typeCounts[models.SuggestionRisk]))
	}
	if typeCounts[models.SuggestionProcess] > 0 {
		parts = append(parts, fmt.Sprintf("%d process guidelines", typeCounts[models.SuggestionProcess]))
	}
	if typeCounts[models.SuggestionConcept] > 0 {
		parts = append(parts, fmt.Sprintf("%d related concepts", typeCounts[models.SuggestionConcept]))
	}

	if len(concepts) > 0 {
		parts = append(parts, fmt.Sprintf("connected to %d higher-level abstractions", len(concepts)))
	}

	return strings.Join(parts, ", ") + "."
}

// Suggest implements context-triggered suggestions - proactive surfacing of relevant
// information without requiring an explicit question. This is the "Active Participation"
// mode where MDEMG analyzes what the user is working on and surfaces:
// - Related patterns/decisions from the graph
// - Previous solutions to similar problems
// - Architectural constraints that apply
// - Potential conflicts with existing knowledge
func (s *Service) Suggest(ctx context.Context, req models.SuggestRequest) (models.SuggestResponse, error) {
	resp := models.SuggestResponse{
		SpaceID:     req.SpaceID,
		Suggestions: []models.Suggestion{},
		Debug:       make(map[string]any),
	}

	maxSuggestions := req.MaxSuggestions
	if maxSuggestions <= 0 {
		maxSuggestions = 5
	}

	minConfidence := req.MinConfidence
	if minConfidence <= 0 {
		minConfidence = 0.5 // Default threshold
	}

	// Step 1: Analyze context to identify triggers
	triggers := s.analyzeContextTriggers(req.Context, req.FilePath)
	resp.Triggers = triggers
	resp.Debug["trigger_count"] = len(triggers)

	// Phase 102: Intent Translation — rewrite context for embedding
	var translatedIntent string
	queryForEmbedding := req.Context
	if req.TranslateIntent && s.intentTranslator != nil {
		translated, translateErr := s.intentTranslator.Translate(ctx, req.Context)
		if translateErr == nil && translated != req.Context {
			translatedIntent = translated
			queryForEmbedding = translated
		}
	}

	// Step 2: Generate query embedding from context
	if s.embedder == nil {
		return resp, fmt.Errorf("no embedding provider configured")
	}
	queryEmbedding, err := s.embedder.Embed(ctx, queryForEmbedding)
	if err != nil {
		return resp, fmt.Errorf("failed to generate context embedding: %w", err)
	}

	// Step 3: Retrieve relevant memories using context
	retrieveReq := models.RetrieveRequest{
		SpaceID:        req.SpaceID,
		QueryText:      queryForEmbedding,
		QueryEmbedding: queryEmbedding,
		TopK:           maxSuggestions * 4, // Get more candidates for filtering
		HopDepth:       2,
	}

	retrieveResp, err := s.retriever.Retrieve(ctx, retrieveReq)
	if err != nil {
		return resp, fmt.Errorf("retrieval failed: %w", err)
	}

	resp.Debug["retrieved_count"] = len(retrieveResp.Results)

	// Step 4: Filter by minimum confidence threshold
	var filteredResults []models.RetrieveResult
	for _, r := range retrieveResp.Results {
		if r.Score >= minConfidence {
			filteredResults = append(filteredResults, r)
		}
	}
	resp.Debug["filtered_count"] = len(filteredResults)

	// Step 5: Generate proactive suggestions
	suggestions := s.generateProactiveSuggestions(ctx, req, filteredResults, triggers)

	// Step 6: Detect conflicts if requested
	if req.IncludeConflicts {
		conflicts := s.detectConflicts(ctx, req.SpaceID, req.Context, filteredResults)
		resp.Conflicts = conflicts
		resp.Debug["conflicts_detected"] = len(conflicts)
	}

	// Step 7: Find applicable constraints if requested
	if req.IncludeConstraints {
		constraints := s.findApplicableConstraints(ctx, req.SpaceID, filteredResults, triggers)
		resp.Constraints = constraints
		resp.Debug["constraints_found"] = len(constraints)
	}

	// Step 8: Fetch related concepts
	var concepts []models.RelatedConcept
	if s.conceptFetcher != nil {
		concepts, err = s.conceptFetcher.FetchRelatedConcepts(ctx, req.SpaceID, filteredResults)
	} else {
		concepts, err = s.fetchRelatedConcepts(ctx, req.SpaceID, filteredResults)
	}
	if err != nil {
		resp.Debug["concept_error"] = err.Error()
	} else {
		resp.RelatedConcepts = concepts
	}

	// Step 9: Enrich with symbol evidence if requested
	if req.IncludeEvidence && s.symbolStore != nil {
		for i := range suggestions {
			for _, nodeID := range suggestions[i].SourceNodes {
				syms, err := s.symbolStore.GetSymbolsForMemoryNode(ctx, req.SpaceID, nodeID)
				if err != nil {
					continue
				}
				for _, sym := range syms {
					suggestions[i].Evidence = append(suggestions[i].Evidence, models.SymbolEvidence{
						SymbolName: sym.Name,
						SymbolType: sym.SymbolType,
						FilePath:   sym.FilePath,
						Line:       sym.Line,
						LineEnd:    sym.LineEnd,
						Value:      sym.Value,
						Signature:  sym.Signature,
					})
				}
			}
		}
	}

	// Limit to max suggestions
	if len(suggestions) > maxSuggestions {
		suggestions = suggestions[:maxSuggestions]
	}

	resp.Suggestions = suggestions

	// Step 10: Calculate overall confidence
	resp.Confidence = s.calculateSuggestConfidence(suggestions, len(filteredResults), len(triggers))

	// Phase 102: Include translated intent in response
	if translatedIntent != "" {
		resp.TranslatedIntent = translatedIntent
	}

	return resp, nil
}

// analyzeContextTriggers identifies what in the context warrants proactive suggestions.
func (s *Service) analyzeContextTriggers(context, filePath string) []models.ContextTrigger {
	var triggers []models.ContextTrigger
	contextLower := strings.ToLower(context)

	// Pattern-based triggers
	patternTriggers := map[string][]string{
		"error_handling": {"try", "catch", "error", "exception", "throw", "panic", "recover"},
		"authentication": {"auth", "login", "logout", "token", "jwt", "session", "password"},
		"database":       {"query", "select", "insert", "update", "delete", "transaction", "sql", "db."},
		"api":            {"endpoint", "route", "handler", "request", "response", "http", "rest", "graphql"},
		"testing":        {"test", "spec", "mock", "stub", "assert", "expect", "describe", "it("},
		"config":         {"config", "env", "environment", "setting", "option", "flag"},
		"async":          {"async", "await", "promise", "callback", "observable", "concurrent"},
		"security":       {"encrypt", "decrypt", "hash", "salt", "sanitize", "escape", "xss", "csrf"},
	}

	for triggerType, keywords := range patternTriggers {
		var matched []string
		for _, kw := range keywords {
			if strings.Contains(contextLower, kw) {
				matched = append(matched, kw)
			}
		}
		if len(matched) > 0 {
			triggers = append(triggers, models.ContextTrigger{
				TriggerType: "pattern_match",
				Matched:     triggerType,
				Keywords:    matched,
			})
		}
	}

	// File-type based triggers
	if filePath != "" {
		filePathLower := strings.ToLower(filePath)
		if strings.HasSuffix(filePathLower, ".test.ts") || strings.HasSuffix(filePathLower, "_test.go") || strings.HasSuffix(filePathLower, ".spec.ts") {
			triggers = append(triggers, models.ContextTrigger{
				TriggerType: "file_type",
				Matched:     "test_file",
			})
		}
		if strings.Contains(filePathLower, "/config/") || strings.Contains(filePathLower, "config.") {
			triggers = append(triggers, models.ContextTrigger{
				TriggerType: "file_type",
				Matched:     "config_file",
			})
		}
		if strings.Contains(filePathLower, "/api/") || strings.Contains(filePathLower, "/handler") {
			triggers = append(triggers, models.ContextTrigger{
				TriggerType: "file_type",
				Matched:     "api_handler",
			})
		}
	}

	return triggers
}

// generateProactiveSuggestions creates suggestions based on context analysis.
func (s *Service) generateProactiveSuggestions(ctx context.Context, req models.SuggestRequest, results []models.RetrieveResult, triggers []models.ContextTrigger) []models.Suggestion {
	var suggestions []models.Suggestion

	// Build a set of trigger types for quick lookup
	triggerTypes := make(map[string]bool)
	for _, t := range triggers {
		triggerTypes[t.Matched] = true
	}

	for _, r := range results {
		suggType := s.classifyProactiveSuggestionType(r, triggerTypes)
		content := s.formatProactiveSuggestionContent(suggType, r, triggers)

		if content == "" {
			continue
		}

		suggestions = append(suggestions, models.Suggestion{
			Type:        suggType,
			Content:     content,
			Confidence:  r.Score,
			SourceNodes: []string{r.NodeID},
		})
	}

	// Deduplicate
	suggestions = s.deduplicateSuggestions(suggestions)

	return suggestions
}

// classifyProactiveSuggestionType determines the type for proactive suggestions.
func (s *Service) classifyProactiveSuggestionType(r models.RetrieveResult, triggerTypes map[string]bool) models.SuggestionType {
	name := strings.ToLower(r.Name)
	path := strings.ToLower(r.Path)
	summary := strings.ToLower(r.Summary)

	// Check for pattern indicators
	patternKeywords := []string{"pattern", "convention", "standard", "practice", "approach", "style"}
	for _, kw := range patternKeywords {
		if strings.Contains(name, kw) || strings.Contains(summary, kw) {
			return models.SuggestionPattern
		}
	}

	// Check for solution indicators (similar problems solved before)
	solutionKeywords := []string{"solution", "fix", "resolve", "implement", "handle", "workaround"}
	for _, kw := range solutionKeywords {
		if strings.Contains(name, kw) || strings.Contains(summary, kw) {
			return models.SuggestionSolution
		}
	}

	// Check for constraint indicators
	constraintKeywords := []string{"constraint", "must", "require", "enforce", "rule", "policy", "limit"}
	for _, kw := range constraintKeywords {
		if strings.Contains(name, kw) || strings.Contains(summary, kw) {
			return models.SuggestionConstraint
		}
	}

	// Risk indicators (inherited from consult)
	riskKeywords := []string{"error", "fail", "issue", "problem", "bug", "deprecated", "warning"}
	for _, kw := range riskKeywords {
		if strings.Contains(name, kw) || strings.Contains(summary, kw) {
			return models.SuggestionRisk
		}
	}

	// Match against trigger types for context relevance
	if triggerTypes["error_handling"] && (strings.Contains(path, "error") || strings.Contains(summary, "error")) {
		return models.SuggestionPattern
	}
	if triggerTypes["authentication"] && (strings.Contains(path, "auth") || strings.Contains(summary, "auth")) {
		return models.SuggestionPattern
	}
	if triggerTypes["database"] && (strings.Contains(path, "db") || strings.Contains(path, "repository") || strings.Contains(summary, "database")) {
		return models.SuggestionPattern
	}

	// Default to pattern for proactive suggestions
	return models.SuggestionPattern
}

// formatProactiveSuggestionContent formats proactive suggestion text.
func (s *Service) formatProactiveSuggestionContent(suggType models.SuggestionType, r models.RetrieveResult, triggers []models.ContextTrigger) string {
	content := r.Summary
	if content == "" {
		content = r.Name
	}
	if content == "" {
		return ""
	}

	// Truncate long content
	if len(content) > 500 {
		content = content[:497] + "..."
	}

	// Find the most relevant trigger for context
	var relevantTrigger string
	for _, t := range triggers {
		if relevantTrigger == "" {
			relevantTrigger = t.Matched
		}
	}

	switch suggType {
	case models.SuggestionPattern:
		if relevantTrigger != "" {
			return fmt.Sprintf("Related %s pattern: %s", strings.ReplaceAll(relevantTrigger, "_", " "), content)
		}
		return fmt.Sprintf("Related pattern in this codebase: %s", content)
	case models.SuggestionSolution:
		return fmt.Sprintf("Previous solution to similar problem: %s", content)
	case models.SuggestionConstraint:
		return fmt.Sprintf("Architectural constraint that applies: %s", content)
	case models.SuggestionConflict:
		return fmt.Sprintf("Potential conflict with existing: %s", content)
	case models.SuggestionRisk:
		return fmt.Sprintf("Risk warning: %s", content)
	default:
		return fmt.Sprintf("Relevant context: %s", content)
	}
}

// detectConflicts identifies potential conflicts between context and existing knowledge.
func (s *Service) detectConflicts(ctx context.Context, spaceID, contextText string, results []models.RetrieveResult) []models.ConflictWarning {
	var conflicts []models.ConflictWarning
	contextLower := strings.ToLower(contextText)

	// Simple conflict detection based on contradictory patterns
	for _, r := range results {
		summaryLower := strings.ToLower(r.Summary)
		nameLower := strings.ToLower(r.Name)

		// Check for deprecated patterns being used
		if strings.Contains(summaryLower, "deprecated") {
			// Check if context might be using deprecated approach
			if r.Score > 0.7 { // High similarity suggests relevant
				conflicts = append(conflicts, models.ConflictWarning{
					Severity:      "medium",
					Description:   fmt.Sprintf("You may be using a deprecated pattern: %s", r.Name),
					ConflictsWith: r.NodeID,
					Evidence:      r.Summary,
					SourceNodes:   []string{r.NodeID},
				})
			}
		}

		// Check for "don't" or "avoid" patterns
		if strings.Contains(summaryLower, "avoid") || strings.Contains(summaryLower, "don't") || strings.Contains(summaryLower, "do not") {
			if r.Score > 0.6 {
				conflicts = append(conflicts, models.ConflictWarning{
					Severity:      "low",
					Description:   fmt.Sprintf("Consider reviewing: %s", r.Name),
					ConflictsWith: r.NodeID,
					Evidence:      r.Summary,
					SourceNodes:   []string{r.NodeID},
				})
			}
		}

		// Check for naming/style conflicts
		if strings.Contains(summaryLower, "naming") || strings.Contains(nameLower, "convention") {
			if r.Score > 0.65 {
				conflicts = append(conflicts, models.ConflictWarning{
					Severity:      "low",
					Description:   "Review naming convention",
					ConflictsWith: r.NodeID,
					Evidence:      r.Summary,
					SourceNodes:   []string{r.NodeID},
				})
			}
		}
	}

	// Check for direct contradictions in context
	contradictionPairs := []struct{ pattern1, pattern2 string }{
		{"sync", "async"},
		{"class", "function"},
		{"sql", "nosql"},
		{"rest", "graphql"},
	}

	for _, pair := range contradictionPairs {
		if strings.Contains(contextLower, pair.pattern1) {
			// Look for results about the opposite pattern
			for _, r := range results {
				if strings.Contains(strings.ToLower(r.Summary), pair.pattern2) && r.Score > 0.6 {
					conflicts = append(conflicts, models.ConflictWarning{
						Severity:      "low",
						Description:   fmt.Sprintf("Context uses %s but codebase has %s patterns", pair.pattern1, pair.pattern2),
						ConflictsWith: r.NodeID,
						Evidence:      r.Summary,
						SourceNodes:   []string{r.NodeID},
					})
				}
			}
		}
	}

	return conflicts
}

// findApplicableConstraints finds architectural constraints relevant to the context.
func (s *Service) findApplicableConstraints(ctx context.Context, spaceID string, results []models.RetrieveResult, triggers []models.ContextTrigger) []models.Constraint {
	var constraints []models.Constraint

	// Extract constraints from high-scoring results
	for _, r := range results {
		summaryLower := strings.ToLower(r.Summary)
		nameLower := strings.ToLower(r.Name)

		// Look for must/should patterns
		constraintType := ""
		if strings.Contains(summaryLower, "must") || strings.Contains(summaryLower, "required") {
			constraintType = "must"
		} else if strings.Contains(summaryLower, "must not") || strings.Contains(summaryLower, "forbidden") {
			constraintType = "must_not"
		} else if strings.Contains(summaryLower, "should") || strings.Contains(summaryLower, "recommended") {
			constraintType = "should"
		} else if strings.Contains(summaryLower, "should not") || strings.Contains(summaryLower, "discouraged") {
			constraintType = "should_not"
		}

		if constraintType != "" && r.Score > 0.6 {
			constraints = append(constraints, models.Constraint{
				Name:           r.Name,
				Description:    r.Summary,
				ConstraintType: constraintType,
				Scope:          r.Path,
				SourceNodes:    []string{r.NodeID},
				Confidence:     r.Score,
			})
		}

		// Look for rule/policy nodes
		if strings.Contains(nameLower, "rule") || strings.Contains(nameLower, "policy") || strings.Contains(nameLower, "constraint") {
			if r.Score > 0.55 {
				ct := "should"
				if strings.Contains(summaryLower, "must") {
					ct = "must"
				}
				constraints = append(constraints, models.Constraint{
					Name:           r.Name,
					Description:    r.Summary,
					ConstraintType: ct,
					Scope:          r.Path,
					SourceNodes:    []string{r.NodeID},
					Confidence:     r.Score,
				})
			}
		}
	}

	// Deduplicate by name
	seen := make(map[string]bool)
	var unique []models.Constraint
	for _, c := range constraints {
		if !seen[c.Name] {
			seen[c.Name] = true
			unique = append(unique, c)
		}
	}

	return unique
}

// calculateSuggestConfidence computes confidence for suggest response.
func (s *Service) calculateSuggestConfidence(suggestions []models.Suggestion, filteredCount, triggerCount int) float64 {
	if len(suggestions) == 0 {
		return 0.0
	}

	var sum float64
	for _, sugg := range suggestions {
		sum += sugg.Confidence
	}
	avgConfidence := sum / float64(len(suggestions))

	// Boost based on triggers and filtered results
	boost := 1.0
	if triggerCount > 0 {
		boost += 0.05 * float64(triggerCount)
	}
	if filteredCount >= 5 {
		boost += 0.1
	}

	confidence := avgConfidence * boost
	if confidence > MaxConfidence {
		confidence = MaxConfidence
	}
	return confidence
}
