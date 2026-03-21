package jiminy

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
	"mdemg/internal/embeddings"
	"mdemg/internal/models"
)

// ConsultingService defines the interface for the consulting.Service.Suggest() method.
type ConsultingService interface {
	Suggest(ctx context.Context, req models.SuggestRequest) (models.SuggestResponse, error)
}

// Service orchestrates all Jiminy guidance sources.
type Service struct {
	cfg               config.Config
	driver            neo4j.DriverWithContext
	consultant        ConsultingService
	embedder          embeddings.Embedder
	tracker           *EffectivenessTracker  // Phase AR-2: guidance effectiveness tracking
	persistence       *PersistenceStore      // F3: Neo4j write-through for guidance outcomes
	confidenceUpdater *ConfidenceUpdater     // F3: Bayesian confidence updates
	cache             *GuidanceCache         // F10: TTL-based LRU cache for guidance responses
	retriever         RetrievalProvider      // J7: full retrieval pipeline access
	synthesizer       *GuidanceSynthesizer   // J8: LLM guidance synthesis
	classifier        *OutcomeClassifier     // J11: semantic outcome classification
	escalation        *EscalationTracker     // J12: session-aware escalation
	evaluator         *Evaluator             // J9: agent output evaluation
	statsCollector    *StatsCollector        // J10: guidance stats for RSIC
}

// NewService creates a new Jiminy guidance service.
func NewService(cfg config.Config, driver neo4j.DriverWithContext, consultant ConsultingService, embedder embeddings.Embedder) *Service {
	tracker := NewEffectivenessTracker(1000, cfg.JiminyEffectivenessTTLSec)

	// F3: Initialize persistence + confidence updater if enabled
	var persistence *PersistenceStore
	var confidenceUpdater *ConfidenceUpdater
	if cfg.JiminyPersistenceEnabled && driver != nil {
		persistence = NewPersistenceStore(driver, cfg)
		confidenceUpdater = NewConfidenceUpdater(driver, cfg)
		log.Printf("jiminy: persistence enabled (boost=%.3f, decay=%.3f, archive=%.2f)",
			cfg.ConstraintConfidenceBoostPerPos, cfg.ConstraintConfidenceDecayPerNeg, cfg.ConstraintArchiveThreshold)
	}

	// F10: Initialize guidance cache if enabled
	var cache *GuidanceCache
	if cfg.JiminyCacheEnabled {
		cache = NewGuidanceCache(cfg.JiminyCacheSize, cfg.JiminyCacheTTLSec)
		log.Printf("jiminy: guidance cache enabled (size=%d, ttl=%ds)", cfg.JiminyCacheSize, cfg.JiminyCacheTTLSec)
	}

	// J11/J14: Initialize semantic outcome classifier if enabled
	var classifier *OutcomeClassifier
	if cfg.JiminyOutcomeClassifierEnabled && embedder != nil {
		classifierBaseURL := cfg.OpenAIEndpoint
		if cfg.JiminySynthesisProvider == "ollama" {
			classifierBaseURL = cfg.OllamaEndpoint
		}
		classifier = NewOutcomeClassifier(embedder, OutcomeClassifierConfig{
			LLMEnabled:    cfg.JiminyOutcomeLLMEnabled,
			LLMProvider:   cfg.JiminySynthesisProvider,
			LLMModel:      cfg.JiminySynthesisModel,
			LLMAPIKey:     cfg.OpenAIAPIKey,
			LLMBaseURL:    classifierBaseURL,
			HighThreshold: cfg.JiminyOutcomeSimilarityHigh,
			LowThreshold:  cfg.JiminyOutcomeSimilarityLow,
			MaxTokens:     cfg.JiminyOutcomeLLMMaxTokens,
			CacheSize:     cfg.JiminyOutcomeCacheSize,
		})
		log.Printf("jiminy: semantic outcome classifier enabled (high=%.2f, low=%.2f, llm=%v, cache=%d)",
			cfg.JiminyOutcomeSimilarityHigh, cfg.JiminyOutcomeSimilarityLow,
			cfg.JiminyOutcomeLLMEnabled, cfg.JiminyOutcomeCacheSize)
	}

	// J12: Initialize escalation tracker if enabled
	var escalation *EscalationTracker
	if cfg.JiminyEscalationEnabled {
		escalation = NewEscalationTracker(cfg)
		log.Printf("jiminy: escalation enabled (warn=%d, escalate=%d, block=%d, blockEnabled=%v)",
			cfg.JiminyEscalationWarnAfter, cfg.JiminyEscalationEscalateAfter,
			cfg.JiminyEscalationBlockAfter, cfg.JiminyEscalationBlockEnabled)
	}

	// J9: Initialize evaluator if enabled
	var evaluator *Evaluator
	if cfg.JiminyEvaluateEnabled && driver != nil {
		evaluator = NewEvaluator(cfg, driver, embedder)
		log.Printf("jiminy: evaluator enabled (timeout=%dms, maxConstraints=%d)",
			cfg.JiminyEvaluateTimeoutMs, cfg.JiminyEvaluateMaxConstraints)
	}

	// J10: Initialize stats collector for RSIC
	var statsCollector *StatsCollector
	if driver != nil {
		statsCollector = NewStatsCollector(driver, cfg)
	}

	return &Service{
		cfg:               cfg,
		driver:            driver,
		consultant:        consultant,
		embedder:          embedder,
		tracker:           tracker,
		persistence:       persistence,
		confidenceUpdater: confidenceUpdater,
		cache:             cache,
		classifier:        classifier,
		escalation:        escalation,
		evaluator:         evaluator,
		statsCollector:    statsCollector,
	}
}

// SetRetriever sets the retrieval provider for full-spectrum knowledge access (J7).
func (s *Service) SetRetriever(r RetrievalProvider) {
	s.retriever = r
}

// SetSynthesizer sets the LLM guidance synthesizer (J8).
func (s *Service) SetSynthesizer(syn *GuidanceSynthesizer) {
	s.synthesizer = syn
}

// GetEvaluator returns the evaluator for handler wiring (J9).
func (s *Service) GetEvaluator() *Evaluator {
	return s.evaluator
}

// GetGuidanceStats returns aggregated guidance stats for RSIC integration (J10).
func (s *Service) GetGuidanceStats(ctx context.Context, spaceID string) (JiminyStats, error) {
	if s.statsCollector == nil {
		return JiminyStats{}, nil
	}
	return s.statsCollector.GetGuidanceStats(ctx, spaceID)
}

// Guide generates proactive guidance by fanning out to multiple knowledge sources
// in parallel: consulting.Suggest(), correction recall, contradiction checking,
// and frontier surfacing. Results are merged, deduplicated, and ranked.
func (s *Service) Guide(ctx context.Context, req GuidanceRequest) (GuidanceResponse, error) {
	if req.SpaceID == "" {
		return GuidanceResponse{}, fmt.Errorf("space_id is required")
	}
	if req.Context == "" {
		return GuidanceResponse{}, fmt.Errorf("context is required")
	}

	// F10: Check cache for fast path
	if s.cache != nil {
		if cached, ok := s.cache.Get(req.SpaceID, req.Context); ok {
			return cached, nil
		}
	}

	maxItems := req.MaxItems
	if maxItems <= 0 {
		maxItems = s.cfg.JiminyMaxItems
	}

	timeoutMs := s.cfg.JiminyTimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 6000
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	// Generate embedding for context
	contextText := req.Context
	if req.FilePath != "" {
		contextText = req.FilePath + ": " + contextText
	}

	var queryEmbedding []float32
	if s.embedder != nil {
		var err error
		queryEmbedding, err = s.embedder.Embed(ctx, contextText)
		if err != nil {
			log.Printf("jiminy: embedding failed (continuing without vector search): %v", err)
		}
	}

	// Fan out to 4 guidance sources in parallel
	var (
		mu       sync.Mutex
		items    []GuidanceItem
		warnings []string
		debug    = make(map[string]any)
	)

	var wg sync.WaitGroup

	// Source A: consulting.Suggest() — constraints, conflicts, patterns, concepts
	wg.Add(1)
	go func() {
		defer wg.Done()
		if s.consultant == nil {
			return
		}
		suggestResp, err := s.consultant.Suggest(ctx, models.SuggestRequest{
			SpaceID:            req.SpaceID,
			Context:            req.Context,
			FilePath:           req.FilePath,
			IncludeConflicts:   true,
			IncludeConstraints: true,
			MaxSuggestions:     maxItems,
			MinConfidence:      s.cfg.JiminyMinConfidence,
		})
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			debug["suggest_error"] = err.Error()
			warnings = append(warnings, "consulting.Suggest failed: "+err.Error())
			return
		}

		// Convert constraints to guidance items
		for _, c := range suggestResp.Constraints {
			items = append(items, GuidanceItem{
				Type:        GuidanceConstraint,
				Priority:    constraintPriority(c.ConstraintType),
				Content:     fmt.Sprintf("[%s] %s", c.ConstraintType, c.Description),
				Confidence:  c.Confidence,
				SourceNodes: c.SourceNodes,
			})
		}

		// Convert conflicts to guidance items
		for _, c := range suggestResp.Conflicts {
			items = append(items, GuidanceItem{
				Type:        GuidanceConflict,
				Priority:    c.Severity,
				Content:     c.Description,
				Confidence:  0.7,
				SourceNodes: c.SourceNodes,
			})
		}

		// Convert suggestions to pattern guidance items
		for _, sg := range suggestResp.Suggestions {
			items = append(items, GuidanceItem{
				Type:        GuidancePattern,
				Priority:    "medium",
				Content:     sg.Content,
				Confidence:  sg.Confidence,
				SourceNodes: sg.SourceNodes,
			})
		}

		debug["suggest_constraints"] = len(suggestResp.Constraints)
		debug["suggest_conflicts"] = len(suggestResp.Conflicts)
		debug["suggest_suggestions"] = len(suggestResp.Suggestions)
	}()

	// Source B+C: Correction vector search + contradiction checking (merged)
	// Previously Source C duplicated Source B's findRelevantCorrections call.
	// Now corrections feed directly into contradiction lookup in one goroutine.
	if queryEmbedding != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			corrections, err := s.findRelevantCorrections(ctx, req.SpaceID, queryEmbedding, 5)
			if err != nil {
				mu.Lock()
				debug["corrections_error"] = err.Error()
				mu.Unlock()
				return
			}

			// Collect correction items and node IDs for contradiction checking
			var correctionItems []GuidanceItem
			var nodeIDs []string
			for _, c := range corrections {
				content := c.Content
				if content == "" {
					content = c.Summary
				}
				correctionItems = append(correctionItems, GuidanceItem{
					Type:        GuidanceCorrection,
					Priority:    "high",
					Content:     content,
					Confidence:  c.Similarity,
					SourceNodes: []string{c.NodeID},
				})
				nodeIDs = append(nodeIDs, c.NodeID)
			}

			// Check contradictions using the same node IDs (no duplicate query)
			var contradictionItems []GuidanceItem
			if len(nodeIDs) > 0 {
				contradictions, cErr := s.findContradictions(ctx, req.SpaceID, nodeIDs)
				if cErr != nil {
					mu.Lock()
					debug["contradictions_error"] = cErr.Error()
					mu.Unlock()
				} else {
					for _, c := range contradictions {
						contradictionItems = append(contradictionItems, GuidanceItem{
							Type:     GuidanceConflict,
							Priority: "high",
							Content: fmt.Sprintf("%q contradicts %q (evidence: %d)",
								c.SourceName, c.TargetName, c.Evidence),
							Confidence:  c.Weight,
							SourceNodes: []string{c.SourceNodeID, c.TargetNodeID},
						})
					}
				}
			}

			mu.Lock()
			defer mu.Unlock()
			items = append(items, correctionItems...)
			items = append(items, contradictionItems...)
			debug["corrections_found"] = len(corrections)
			debug["contradictions_found"] = len(contradictionItems)
		}()
	}

	// Source D: Frontier node search (Phase J5)
	if queryEmbedding != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			frontiers, err := s.findRelevantFrontiers(ctx, req.SpaceID, queryEmbedding, 0, 3)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				debug["frontiers_error"] = err.Error()
				return
			}
			for _, f := range frontiers {
				content := f.Name
				if f.Summary != "" {
					content = f.Name + ": " + f.Summary
				}
				items = append(items, GuidanceItem{
					Type:        GuidanceFrontier,
					Priority:    "low",
					Content:     content,
					Confidence:  f.Similarity,
					SourceNodes: []string{f.NodeID},
				})
			}
			debug["frontiers_found"] = len(frontiers)
		}()
	}

	// Source E: Full retrieval pipeline (J7)
	if s.retriever != nil && s.cfg.JiminyRetrievalEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Use agent output as additional context if available
			queryText := req.Context
			if req.AgentOutput != "" {
				queryText = req.Context + " " + req.AgentOutput
			}
			results, err := s.retriever.RetrieveForJiminy(ctx, req.SpaceID, queryText,
				s.cfg.JiminyRetrievalTopK, s.cfg.JiminyRetrievalHopDepth)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				debug["retrieval_error"] = err.Error()
				return
			}
			retrievalItems := mapRetrievalToGuidance(results)
			items = append(items, retrievalItems...)
			debug["retrieval_found"] = len(retrievalItems)
		}()
	}

	wg.Wait()

	// Filter by minimum confidence
	minConf := s.cfg.JiminyMinConfidence
	var filtered []GuidanceItem
	for _, item := range items {
		if item.Confidence >= minConf {
			filtered = append(filtered, item)
		}
	}

	// Deduplicate by content (simple dedup)
	filtered = deduplicateItems(filtered)

	// Sort by priority (high > medium > low) then confidence (desc)
	sort.Slice(filtered, func(i, j int) bool {
		pi, pj := priorityRank(filtered[i].Priority), priorityRank(filtered[j].Priority)
		if pi != pj {
			return pi < pj
		}
		return filtered[i].Confidence > filtered[j].Confidence
	})

	// Truncate to max items
	if len(filtered) > maxItems {
		filtered = filtered[:maxItems]
	}

	// Ensure non-nil slices for JSON serialization (nil → null, [] → [])
	if filtered == nil {
		filtered = []GuidanceItem{}
	}
	if warnings == nil {
		warnings = []string{}
	}

	// J12: Apply escalation effects before final ranking
	if s.escalation != nil && req.SessionID != "" {
		for i := range filtered {
			for _, nodeID := range filtered[i].SourceNodes {
				level := s.escalation.RecordSurface(req.SessionID, nodeID)
				if level != EscalationInactive && level != EscalationSurfaced {
					ApplyEscalation(&filtered[i], level)
				}
			}
		}
	}

	// Compute source counts
	counts := SourceCounts{}
	for _, item := range filtered {
		switch item.Type {
		case GuidanceConstraint:
			counts.Constraints++
		case GuidanceCorrection:
			counts.Corrections++
		case GuidancePattern, GuidanceSuggestion:
			counts.Patterns++
		case GuidanceConflict, GuidanceRisk:
			counts.Conflicts++
		case GuidanceFrontier:
			counts.Frontiers++
		case GuidanceDecision, GuidanceLearning, GuidancePreference, GuidanceConcept:
			counts.Retrievals++
		}
	}

	// Compute overall confidence
	confidence := computeOverallConfidence(filtered)

	// Build rationale
	rationale := buildRationale(counts)

	// Format prompt augmentation (static fallback)
	augmentation := FormatPromptAugmentation(filtered, counts, confidence)

	// J8: LLM synthesis — replace static formatting if synthesizer is available
	var synthesizedNarrative string
	if s.synthesizer != nil && s.cfg.JiminySynthesisEnabled && len(filtered) > 0 {
		narrative, synthErr := s.synthesizer.Synthesize(ctx, filtered, req.Context, req.AgentOutput)
		if synthErr != nil {
			log.Printf("jiminy: synthesis failed (using static formatting): %v", synthErr)
			debug["synthesis_error"] = synthErr.Error()
		} else if narrative != "" {
			synthesizedNarrative = narrative
			// Use synthesized narrative as the prompt augmentation
			augmentation = "═══ JIMINY GUIDANCE ═══\n" + narrative + "\n═══ END JIMINY GUIDANCE ═══"
			debug["synthesis_used"] = true
		}
	}

	// Phase AR-2: Generate guidance_id and track items for effectiveness feedback
	guidanceID := uuid.New().String()
	if s.tracker != nil && len(filtered) > 0 {
		s.tracker.Track(guidanceID, filtered)
	}

	// J12: Build session escalation summary
	var sessionEscalation *SessionEscalation
	if s.escalation != nil && req.SessionID != "" {
		sessionEscalation = s.escalation.GetSessionEscalation(req.SessionID)
	}

	resp := GuidanceResponse{
		GuidanceID:           guidanceID,
		Guidance:             filtered,
		PromptAugmentation:   augmentation,
		SynthesizedNarrative: synthesizedNarrative,
		Confidence:           confidence,
		Rationale:            rationale,
		Warnings:             warnings,
		SourceCounts:         counts,
		SessionEscalation:    sessionEscalation,
		Debug:                debug,
	}

	// F10: Cache the response
	if s.cache != nil {
		s.cache.Put(req.SpaceID, req.Context, resp)
	}

	return resp, nil
}

// constraintPriority maps constraint types to priorities.
func constraintPriority(constraintType string) string {
	switch constraintType {
	case "must", "must_not":
		return "high"
	case "should", "should_not":
		return "medium"
	default:
		return "low"
	}
}

// computeOverallConfidence averages the top-N item confidences.
func computeOverallConfidence(items []GuidanceItem) float64 {
	if len(items) == 0 {
		return 0
	}
	sum := 0.0
	n := len(items)
	if n > 5 {
		n = 5
	}
	for i := 0; i < n; i++ {
		sum += items[i].Confidence
	}
	return sum / float64(n)
}

// buildRationale constructs a human-readable summary of guidance sources.
func buildRationale(counts SourceCounts) string {
	total := counts.Constraints + counts.Corrections + counts.Patterns + counts.Conflicts + counts.Frontiers + counts.Retrievals
	if total == 0 {
		return "No relevant guidance found for this context"
	}
	parts := []string{}
	if counts.Constraints > 0 {
		parts = append(parts, fmt.Sprintf("%d constraints", counts.Constraints))
	}
	if counts.Corrections > 0 {
		parts = append(parts, fmt.Sprintf("%d corrections", counts.Corrections))
	}
	if counts.Patterns > 0 {
		parts = append(parts, fmt.Sprintf("%d patterns", counts.Patterns))
	}
	if counts.Conflicts > 0 {
		parts = append(parts, fmt.Sprintf("%d conflicts", counts.Conflicts))
	}
	if counts.Frontiers > 0 {
		parts = append(parts, fmt.Sprintf("%d frontiers", counts.Frontiers))
	}
	if counts.Retrievals > 0 {
		parts = append(parts, fmt.Sprintf("%d retrievals", counts.Retrievals))
	}
	return fmt.Sprintf("Found %d guidance items: %s", total, join(parts, ", "))
}

// join is a simple string join.
func join(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}

// deduplicateItems removes items with identical content.
func deduplicateItems(items []GuidanceItem) []GuidanceItem {
	seen := make(map[string]bool)
	var result []GuidanceItem
	for _, item := range items {
		if !seen[item.Content] {
			seen[item.Content] = true
			result = append(result, item)
		}
	}
	return result
}

// RecordOutcome processes feedback for a prior guidance response.
// Uses semantic classifier (J11) if available, otherwise falls back to text overlap.
// Feeds escalation tracker (J12) for session-aware escalation state updates.
func (s *Service) RecordOutcome(ctx context.Context, req GuidanceFeedbackRequest) (*GuidanceFeedbackResponse, error) {
	if req.GuidanceID == "" {
		return nil, fmt.Errorf("guidance_id is required")
	}

	items := s.tracker.Lookup(req.GuidanceID)
	if items == nil {
		return &GuidanceFeedbackResponse{
			GuidanceID: req.GuidanceID,
			Results:    []GuidanceItemFeedback{},
			Applied:    false,
		}, nil
	}

	actionLower := strings.ToLower(req.ActionSummary)
	var results []GuidanceItemFeedback

	for _, item := range items {
		var cr ClassificationResult

		// J14: Use semantic classifier returning ClassificationResult if available
		if s.classifier != nil {
			cr = s.classifier.Classify(ctx, item, req.ActionSummary)
		} else {
			outcome, sim := classifyOutcome(item, actionLower)
			cr = ClassificationResult{Outcome: outcome, Confidence: sim}
		}

		results = append(results, GuidanceItemFeedback{
			Type:       item.Type,
			Content:    item.Content,
			Outcome:    cr.Outcome,
			Similarity: cr.Confidence,
			Reasoning:  cr.Reasoning,
		})

		// Alias for downstream use
		outcome := cr.Outcome

		// J12: Feed escalation tracker with outcome
		if s.escalation != nil && len(item.SourceNodes) > 0 {
			sessionID := req.SpaceID // Use spaceID as session fallback
			for _, nodeID := range item.SourceNodes {
				s.escalation.RecordOutcome(sessionID, nodeID, outcome)
			}
		}

		// F3: Persist guidance outcome to Neo4j and update constraint confidence
		if s.persistence != nil && outcome != OutcomeUnknown {
			if err := s.persistence.PersistGuidanceOutcome(ctx, req.SpaceID, req.GuidanceID, "", item, outcome, cr.Confidence); err != nil {
				log.Printf("jiminy: persist outcome error: %v", err)
			}
			// Update confidence for constraint-type guidance items
			if s.confidenceUpdater != nil && item.Type == GuidanceConstraint && len(item.SourceNodes) > 0 {
				if err := s.confidenceUpdater.UpdateConfidence(ctx, item.SourceNodes[0], outcome); err != nil {
					log.Printf("jiminy: confidence update error: %v", err)
				}
			}
		}
	}

	return &GuidanceFeedbackResponse{
		GuidanceID: req.GuidanceID,
		Results:    results,
		Applied:    true,
	}, nil
}

// classifyOutcome determines whether a guidance item was followed, contradicted, or ignored
// based on simple text overlap with the action summary.
func classifyOutcome(item GuidanceItem, actionLower string) (GuidanceOutcome, float64) {
	contentLower := strings.ToLower(item.Content)

	// Extract significant words (>= 4 chars) from guidance content
	contentWords := significantWords(contentLower)
	if len(contentWords) == 0 {
		return OutcomeUnknown, 0.0
	}

	// Count how many significant content words appear in the action
	matches := 0
	for _, w := range contentWords {
		if strings.Contains(actionLower, w) {
			matches++
		}
	}
	similarity := float64(matches) / float64(len(contentWords))

	// Check for negation patterns indicating contradiction
	negationPatterns := []string{"instead of", "did not", "didn't", "ignored", "skipped", "contrary to"}
	hasNegation := false
	for _, neg := range negationPatterns {
		if strings.Contains(actionLower, neg) {
			hasNegation = true
			break
		}
	}

	if similarity >= 0.4 && hasNegation {
		return OutcomeContradicted, similarity
	}
	if similarity >= 0.3 {
		return OutcomeFollowed, similarity
	}
	if similarity > 0.0 {
		return OutcomeIgnored, similarity
	}
	return OutcomeUnknown, 0.0
}

// GetConstraintEffectiveness delegates to the persistence store to return
// per-constraint effectiveness metrics. Returns empty slice if persistence is disabled.
func (s *Service) GetConstraintEffectiveness(ctx context.Context, spaceID string) ([]ConstraintEffectiveness, error) {
	if s.persistence == nil {
		return []ConstraintEffectiveness{}, nil
	}
	return s.persistence.GetConstraintEffectiveness(ctx, spaceID)
}

// significantWords extracts words >= 4 chars from text.
func significantWords(text string) []string {
	words := strings.Fields(text)
	var significant []string
	for _, w := range words {
		// Strip common punctuation
		w = strings.Trim(w, ".,;:!?\"'()[]{}")
		if len(w) >= 4 {
			significant = append(significant, w)
		}
	}
	return significant
}
