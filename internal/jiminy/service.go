package jiminy

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/nrednav/cuid2"
	"mdemg/internal/config"
	"mdemg/internal/embeddings"
	"mdemg/internal/llmclient"
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
	ticketManager     *TicketManager         // J17: session ticket management
	sequenceTracker   *SequenceTracker       // J17: monotonic sequence counter
	encoder           *ProtocolEncoder       // J17: three-tier encoding
	codeGenerator     *ConstraintCodeGenerator // J17: constraint code generation
	trustScorer       *TrustScorer           // J17: per-session trust scoring
	protocolMetrics   *ProtocolMetricsCollector // J17-4: protocol metrics for RSIC
	extensions        *ExtensionRegistry       // J17-5: per-session protocol extensions
	signalLearner     SignalLearnerProvider    // RSIC-SK1: Hebbian signal learner for guidance
	tierPredictor     *TierPredictor           // Gap 6: shadow-mode ML tier prediction
	nliScorer         *NLIComprehensionScorer  // Gap 6: shadow-mode NLI comprehension scoring
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
			LLMEnabled:      cfg.JiminyOutcomeLLMEnabled,
			LLMProvider:     cfg.JiminySynthesisProvider,
			LLMModel:        cfg.JiminySynthesisModel,
			LLMAPIKey:       cfg.OpenAIAPIKey,
			LLMBaseURL:      classifierBaseURL,
			HighThreshold:   cfg.JiminyOutcomeSimilarityHigh,
			LowThreshold:    cfg.JiminyOutcomeSimilarityLow,
			MaxTokens:       cfg.JiminyOutcomeLLMMaxTokens,
			CacheSize:       cfg.JiminyOutcomeCacheSize,
			CompressPrompts: cfg.JiminyClassifyCompress,
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

	// J17: Initialize session ticket manager and sequence tracker if enabled
	var ticketManager *TicketManager
	var sequenceTracker *SequenceTracker
	if cfg.J17Enabled {
		ticketManager = NewTicketManager(cfg.J17TicketSecret, cfg.J17TicketTTLHours)
		sequenceTracker = NewSequenceTracker(cfg.J17SequenceBufferSize)
		log.Printf("jiminy: J17 protocol enabled (ttl=%dh, bufferSize=%d)",
			cfg.J17TicketTTLHours, cfg.J17SequenceBufferSize)
	}

	// J17-4: Initialize protocol metrics collector
	var protocolMetrics *ProtocolMetricsCollector
	if cfg.J17MetricsEnabled {
		protocolMetrics = NewProtocolMetricsCollector()
		log.Printf("jiminy: J17 protocol metrics collection enabled")
	}

	// J17-5: Initialize extension registry
	var extensions *ExtensionRegistry
	if cfg.J17ExtensionsEnabled {
		extensions = NewExtensionRegistry(cfg.J17AllowedExtensions)
		log.Printf("jiminy: J17 extensions enabled (%d allowed)", len(cfg.J17AllowedExtensions))
	}

	// J17: Initialize protocol encoder and trust scorer if enabled
	var encoder *ProtocolEncoder
	var trustScorer *TrustScorer
	if cfg.J17Enabled {
		encoder = NewProtocolEncoder(cfg.J17DefaultTier)
		trustScorer = NewTrustScorer(TrustConfig{
			Initial:            cfg.J17TrustInitial,
			BoostPerFollow:     cfg.J17TrustBoostPerFollow,
			DecayPerIgnore:     cfg.J17TrustDecayPerIgnore,
			DecayPerContradict: cfg.J17TrustDecayPerContradict,
			HighThreshold:      cfg.J17TrustHighThreshold,
			LowThreshold:       cfg.J17TrustLowThreshold,
		})
		log.Printf("jiminy: J17 trust scoring enabled (initial=%.2f, high=%.2f, low=%.2f)",
			cfg.J17TrustInitial, cfg.J17TrustHighThreshold, cfg.J17TrustLowThreshold)
	}

	// Gap 6: Shadow-mode ML components (log only, no behavioral effect)
	var tierPredictor *TierPredictor
	var nliScorer *NLIComprehensionScorer
	if cfg.J17SidecarURL != "" {
		if cfg.J17MLTierPredictionEnabled {
			tierPredictor = NewTierPredictor(cfg.J17SidecarURL, cfg.J17SidecarTimeoutMs, true)
			log.Printf("jiminy: shadow tier predictor enabled (%s)", cfg.J17SidecarURL)
		}
		if cfg.J17NLIComprehensionEnabled {
			nliScorer = NewNLIComprehensionScorer(cfg.J17SidecarURL, cfg.J17SidecarTimeoutMs, true)
			log.Printf("jiminy: shadow NLI comprehension scorer enabled (%s)", cfg.J17SidecarURL)
		}
	}

	// Gap 7: Warn if J17 is enabled but ticket secret is auto-generated
	if cfg.J17Enabled && cfg.J17TicketSecret == "" {
		log.Printf("WARN: J17_TICKET_SECRET not set — auto-generated key, not persistent across restarts")
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
		ticketManager:     ticketManager,
		sequenceTracker:   sequenceTracker,
		encoder:           encoder,
		trustScorer:       trustScorer,
		protocolMetrics:   protocolMetrics,
		extensions:        extensions,
		tierPredictor:     tierPredictor,
		nliScorer:         nliScorer,
	}
}

// SetSignalLearner sets the Hebbian signal learner for guidance emission/response tracking (RSIC-SK1).
func (s *Service) SetSignalLearner(sl SignalLearnerProvider) {
	s.signalLearner = sl
}

// UpdateNodeConfidence delegates to the confidence updater to apply outcome-based
// confidence changes to a single node (RSIC-SK1).
func (s *Service) UpdateNodeConfidence(ctx context.Context, nodeID string, outcome GuidanceOutcome) error {
	if s.confidenceUpdater == nil {
		return fmt.Errorf("confidence updater not available")
	}
	return s.confidenceUpdater.UpdateConfidence(ctx, nodeID, outcome)
}

// ArchiveStaleConstraints delegates to the confidence updater to archive constraints
// whose confidence has fallen below the archive threshold (RSIC-SK1).
func (s *Service) ArchiveStaleConstraints(ctx context.Context, spaceID string) (int, error) {
	if s.confidenceUpdater == nil {
		return 0, fmt.Errorf("confidence updater not available")
	}
	return s.confidenceUpdater.ArchiveStaleConstraints(ctx, spaceID)
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
	// Gap 3: Bypass cache when J17 is active with a session ID to prevent cross-session contamination
	cacheBypass := s.cfg.JiminyCacheJ17Bypass && s.cfg.J17Enabled && req.SessionID != ""
	if s.cache != nil && !cacheBypass {
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

	// J17-2: Populate ConstraintCode from Neo4j for constraint guidance items
	if s.cfg.J17Enabled && s.driver != nil {
		var constraintSourceNodes []string
		for _, item := range items {
			if item.Type == GuidanceConstraint {
				constraintSourceNodes = append(constraintSourceNodes, item.SourceNodes...)
			}
		}
		if len(constraintSourceNodes) > 0 {
			codes := s.lookupConstraintCodes(ctx, constraintSourceNodes)
			if len(codes) > 0 {
				for i := range items {
					if items[i].Type == GuidanceConstraint {
						for _, srcNode := range items[i].SourceNodes {
							if code, ok := codes[srcNode]; ok {
								items[i].ConstraintCode = code
								break
							}
						}
					}
				}
			}
		}
	}

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

	// RSIC-SK1: Record signal emissions for each surfaced guidance item
	if s.signalLearner != nil {
		for _, item := range filtered {
			if code := guidanceSignalCode(item); code != "" {
				s.signalLearner.RecordEmission(code)
			}
		}
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

	// Format prompt augmentation
	var augmentation string
	if s.encoder != nil && s.cfg.J17Enabled {
		// J17: Use three-tier encoding with trust-modulated tier selection
		trustScore := 0.5
		if s.trustScorer != nil && req.SessionID != "" {
			trustScore = s.trustScorer.GetScore(req.SessionID)
		}
		augmentation = s.encoder.Encode(filtered, counts, confidence, trustScore)
	} else {
		// Static fallback (pre-J17)
		augmentation = FormatPromptAugmentation(filtered, counts, confidence)
	}

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
	guidanceID := cuid2.Generate()
	if s.tracker != nil && len(filtered) > 0 {
		s.tracker.Track(guidanceID, filtered)
	}

	// J12: Build session escalation summary
	var sessionEscalation *SessionEscalation
	if s.escalation != nil && req.SessionID != "" {
		sessionEscalation = s.escalation.GetSessionEscalation(req.SessionID)
	}

	// J17-4: Record protocol metrics for RSIC learning loop
	if s.protocolMetrics != nil && s.encoder != nil && s.cfg.J17Enabled {
		var constraintTotal, constraintWithCode int
		for _, item := range filtered {
			codes := []string{}
			if item.ConstraintCode != "" {
				codes = []string{item.ConstraintCode}
			} else if item.Type == GuidanceConstraint && len(item.SourceNodes) > 0 {
				// Gap 5: Track uncoded constraints by node ID for T2 frequency visibility
				codes = []string{item.SourceNodes[0]}
			}
			// Estimate token count based on tier
			tokenEst := 80 // T3 baseline
			if item.Tier == TierCoded {
				tokenEst = 15
			} else if item.Tier == TierTelegraphic {
				tokenEst = 50
			}
			s.protocolMetrics.RecordGuidance(item.Tier, tokenEst, codes)

			// Gap 1: Count constraint items and coded constraints
			if item.Type == GuidanceConstraint {
				constraintTotal++
				if item.ConstraintCode != "" {
					constraintWithCode++
				}
			}
		}
		// Gap 1: Record code coverage so Snapshot().CodeCoverage reflects reality
		s.protocolMetrics.RecordConstraintCoverage(constraintTotal, constraintWithCode)
	}

	// Gap 6: Shadow-mode ML tier prediction (log only, no behavioral effect)
	if s.tierPredictor != nil {
		trustScore := 0.5
		if s.trustScorer != nil && req.SessionID != "" {
			trustScore = s.trustScorer.GetScore(req.SessionID)
		}
		for _, item := range filtered {
			if item.Type == GuidanceConstraint {
				mlTier, mlConf := s.tierPredictor.PredictTier(ctx, item.Content, req.Context, trustScore)
				if mlTier > 0 {
					log.Printf("j17-shadow: tier_predict ml_tier=%d rule_tier=%d ml_conf=%.2f constraint=%s",
						mlTier, item.Tier, mlConf, item.ConstraintCode)
				}
			}
		}
	}

	// J17: Assign sequence number and record event
	var guidanceSeq int64
	var protocolInfo *ProtocolInfo
	if s.sequenceTracker != nil {
		summary := fmt.Sprintf("%d items (%d constraints, %d corrections)",
			len(filtered), counts.Constraints, counts.Corrections)
		guidanceSeq = s.sequenceTracker.RecordGuidance(req.SpaceID, req.SessionID, len(filtered), summary)
		protocolInfo = &ProtocolInfo{
			Version: J17Version,
			Seq:     guidanceSeq,
		}
		if s.trustScorer != nil && req.SessionID != "" {
			protocolInfo.TrustScore = s.trustScorer.GetScore(req.SessionID)
		}
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
		GuidanceSeq:          guidanceSeq,
		Protocol:             protocolInfo,
		Debug:                debug,
	}

	// F10: Cache the response (Gap 3: skip when J17 session is active)
	if s.cache != nil && !cacheBypass {
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

	// J17: Resolve session ID for trust scoring — prefer explicit SessionID, fall back to SpaceID
	feedbackSessionID := req.SessionID
	if feedbackSessionID == "" {
		feedbackSessionID = req.SpaceID
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
			for _, nodeID := range item.SourceNodes {
				s.escalation.RecordOutcome(feedbackSessionID, nodeID, outcome)
			}
		}

		// J17: Update trust score based on outcome (per-session, not per-space)
		if s.trustScorer != nil {
			s.trustScorer.RecordOutcome(feedbackSessionID, outcome)
		}

		// J17-4: Record outcome for protocol metrics and data collection
		if s.protocolMetrics != nil && item.ConstraintCode != "" {
			// Map outcome to comprehension score
			var compScore float64
			switch outcome {
			case OutcomeFollowed:
				compScore = 1.0
			case OutcomeContradicted:
				compScore = 1.0 // understood but violated
			case OutcomeIgnored:
				compScore = 0.0
			default:
				compScore = 0.5
			}
			s.protocolMetrics.RecordOutcome(item.ConstraintCode, compScore)
		}

		// F3: Persist guidance outcome to Neo4j and update constraint confidence
		if s.persistence != nil && outcome != OutcomeUnknown {
			if err := s.persistence.PersistGuidanceOutcome(ctx, req.SpaceID, req.GuidanceID, "", item, outcome, cr.Confidence); err != nil {
				log.Printf("jiminy: persist outcome error: %v", err)
			}
			// RSIC-SK1: Update confidence for all guidance types with source nodes
			if s.confidenceUpdater != nil && len(item.SourceNodes) > 0 {
				if err := s.confidenceUpdater.UpdateConfidence(ctx, item.SourceNodes[0], outcome); err != nil {
					log.Printf("jiminy: confidence update error: %v", err)
				}
			}
		}
		// RSIC-SK1: Record signal response for positive outcomes (independent of persistence)
		if s.signalLearner != nil && (outcome == OutcomeFollowed || outcome == OutcomePartialCompliance) {
			if code := guidanceSignalCode(item); code != "" {
				s.signalLearner.RecordResponse(code)
			}
		}

		// Gap 6: Shadow-mode NLI comprehension scoring (log only, no behavioral effect)
		if s.nliScorer != nil && item.Type == GuidanceConstraint {
			followed := outcome == OutcomeFollowed || outcome == OutcomePartialCompliance
			nliScore := s.nliScorer.ScoreComprehension(ctx, item.Content, req.ActionSummary, followed)
			heuristicScore := cr.Confidence
			log.Printf("j17-shadow: nli_score nli=%.2f heuristic=%.2f constraint=%s",
				nliScore, heuristicScore, item.ConstraintCode)
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

// Checkpoint creates a session ticket capturing current Jiminy state.
// Called by the pre-compact hook before context compaction.
func (s *Service) Checkpoint(_ context.Context, req CheckpointRequest) (*CheckpointResponse, error) {
	if s.ticketManager == nil {
		return nil, fmt.Errorf("j17: protocol not enabled")
	}

	var lastSeq int64
	if s.sequenceTracker != nil {
		lastSeq = s.sequenceTracker.Current()
	}

	// Build escalation snapshot
	var escalationSnapshot map[string]EscalationEntry
	var activeConstraintIDs []string
	if s.escalation != nil {
		escalationSnapshot = s.escalation.ExportState(req.SessionID)
		for nodeID := range escalationSnapshot {
			activeConstraintIDs = append(activeConstraintIDs, nodeID)
		}
	}

	// Get trust score (real value from J17-3 trust scorer)
	trustScore := 0.5
	if s.trustScorer != nil {
		trustScore = s.trustScorer.GetScore(req.SessionID)
	}

	payload := TicketPayload{
		SpaceID:             req.SpaceID,
		SessionID:           req.SessionID,
		LastSeq:             lastSeq,
		TrustScore:          trustScore,
		EscalationSnapshot:  escalationSnapshot,
		ActiveConstraintIDs: activeConstraintIDs,
	}

	ticket, err := s.ticketManager.IssueTicket(payload)
	if err != nil {
		return nil, err
	}

	return &CheckpointResponse{
		Ticket:    ticket,
		LastSeq:   lastSeq,
		IssuedAt:  payload.IssuedAt.Format(time.RFC3339),
		ExpiresAt: payload.IssuedAt.Add(payload.TTL).Format(time.RFC3339),
	}, nil
}

// ResumeProtocol restores Jiminy state from a session ticket.
// Called by the session-start hook after context reset.
func (s *Service) ResumeProtocol(_ context.Context, req ResumeProtocolRequest) (*ResumeProtocolResponse, error) {
	if s.ticketManager == nil {
		// Gap 2: Record failed restore (no ticket manager)
		if s.protocolMetrics != nil {
			s.protocolMetrics.RecordTicketRestore(false)
		}
		return &ResumeProtocolResponse{
			Restored: false,
			Message:  "j17: protocol not enabled, performing full re-guidance",
		}, nil
	}

	if req.Ticket == nil {
		// Gap 2: Record failed restore (no ticket provided)
		if s.protocolMetrics != nil {
			s.protocolMetrics.RecordTicketRestore(false)
		}
		return &ResumeProtocolResponse{
			Restored: false,
			Message:  "j17: no ticket provided, performing full re-guidance",
		}, nil
	}

	// Validate and restore from ticket
	payload, err := s.ticketManager.RestoreFromTicket(req.Ticket, s.escalation)
	if err != nil {
		log.Printf("jiminy: J17 ticket restore failed (graceful fallback): %v", err)
		// Gap 2: Record failed restore (invalid ticket)
		if s.protocolMetrics != nil {
			s.protocolMetrics.RecordTicketRestore(false)
		}
		return &ResumeProtocolResponse{
			Restored: false,
			Message:  fmt.Sprintf("j17: ticket invalid (%v), performing full re-guidance", err),
		}, nil
	}

	// Gap 2: Record successful restore
	if s.protocolMetrics != nil {
		s.protocolMetrics.RecordTicketRestore(true)
	}

	// Restore trust score
	if s.trustScorer != nil {
		s.trustScorer.SetScore(payload.SessionID, payload.TrustScore)
	}

	// Replay missed events
	var replayedEvents []SequenceEvent
	if s.sequenceTracker != nil && req.LastSeq > 0 {
		maxReplay := s.cfg.J17ReplayMaxEvents
		if maxReplay <= 0 {
			maxReplay = 50
		}
		replayedEvents = s.sequenceTracker.EventsSince(req.LastSeq, maxReplay)
	}

	// Gap 2: Record replay events
	if s.protocolMetrics != nil && len(replayedEvents) > 0 {
		s.protocolMetrics.RecordReplay(len(replayedEvents))
	}

	return &ResumeProtocolResponse{
		Restored:        true,
		TrustScore:      payload.TrustScore,
		EscalationState: payload.EscalationSnapshot,
		ReplayedEvents:  replayedEvents,
		Message: fmt.Sprintf("j17: state restored (seq %d→%d, %d escalations, %d events replayed)",
			req.LastSeq, payload.LastSeq, len(payload.EscalationSnapshot), len(replayedEvents)),
	}, nil
}

// GetSequenceTracker returns the J17 sequence tracker for external wiring.
func (s *Service) GetSequenceTracker() *SequenceTracker {
	return s.sequenceTracker
}

// GetTicketManager returns the J17 ticket manager for external wiring.
func (s *Service) GetTicketManager() *TicketManager {
	return s.ticketManager
}

// SetCodeGenerator sets the J17 constraint code generator.
func (s *Service) SetCodeGenerator(gen *ConstraintCodeGenerator) {
	s.codeGenerator = gen
}

// GetProtocolMetricsSnapshot returns an immutable snapshot of J17 protocol metrics.
// Returns nil if metrics collection is not enabled.
func (s *Service) GetProtocolMetricsSnapshot() *ProtocolMetrics {
	if s.protocolMetrics == nil {
		return nil
	}
	return s.protocolMetrics.Snapshot()
}

// GetProtocolMetricsCollector returns the J17 protocol metrics collector for RSIC wiring.
func (s *Service) GetProtocolMetricsCollector() *ProtocolMetricsCollector {
	return s.protocolMetrics
}

// NewProtocolEvolver creates a fully-wired ProtocolEvolver from service internals.
func (s *Service) NewProtocolEvolver() *ProtocolEvolver {
	return NewProtocolEvolver(s.protocolMetrics, s.codeGenerator, s.encoder,
		s.sequenceTracker, s.driver, s.trustScorer)
}

// NegotiateExtension processes an agent's request for a protocol extension.
func (s *Service) NegotiateExtension(req ExtensionRequest) ExtensionResponse {
	if s.extensions == nil {
		return ExtensionResponse{
			Granted:   false,
			Extension: req.Extension,
			Reason:    "extensions not enabled",
		}
	}
	return s.extensions.Negotiate(req)
}

// ProtocolFeedbackRequest carries comprehension test results back into the learning loop.
type ProtocolFeedbackRequest struct {
	Trials []ProtocolFeedbackTrial `json:"trials"`
}

// ProtocolFeedbackTrial is one trial result from a comprehension test.
type ProtocolFeedbackTrial struct {
	ConstraintCode string  `json:"constraint_code"`
	Tier           int     `json:"tier"`
	Score          float64 `json:"score"`           // 0-10 comprehension score
	Interpretation string  `json:"interpretation"`  // receiver's interpretation
	SenderIntent   string  `json:"sender_intent"`   // ground truth
}

// ProtocolFeedbackResponse returns aggregated learning results.
type ProtocolFeedbackResponse struct {
	Ingested      int                `json:"ingested"`
	WeakCodes     []WeakCodeReport   `json:"weak_codes,omitempty"`
	Improvements  []CodeImprovement  `json:"improvements,omitempty"`
	MetricsAfter  *ProtocolMetrics   `json:"metrics_after,omitempty"`
}

// WeakCodeReport identifies a code with low comprehension at a specific tier.
type WeakCodeReport struct {
	Code          string  `json:"code"`
	Tier          int     `json:"tier"`
	AvgScore      float64 `json:"avg_score"`
	TrialCount    int     `json:"trial_count"`
	FailureReason string  `json:"failure_reason,omitempty"`
}

// CodeImprovement records a code regeneration result.
type CodeImprovement struct {
	OldCode     string  `json:"old_code"`
	NewCode     string  `json:"new_code"`
	Reason      string  `json:"reason"`
	ScoreBefore float64 `json:"score_before"`
}

// RecordProtocolFeedback ingests comprehension test results into protocol metrics
// and identifies codes that need improvement.
func (s *Service) RecordProtocolFeedback(req ProtocolFeedbackRequest) ProtocolFeedbackResponse {
	resp := ProtocolFeedbackResponse{}

	// Aggregate scores per code per tier
	type codeStats struct {
		scores []float64
		tier   int
	}
	codeScores := make(map[string]*codeStats)

	for _, trial := range req.Trials {
		// Feed into protocol metrics
		if s.protocolMetrics != nil && trial.ConstraintCode != "" {
			// Normalize 0-10 score to 0.0-1.0 comprehension
			compScore := trial.Score / 10.0
			s.protocolMetrics.RecordOutcome(trial.ConstraintCode, compScore)
		}
		resp.Ingested++

		// Track per-code stats (only for T1 — that's where learning matters)
		if trial.Tier == TierCoded && trial.ConstraintCode != "" {
			key := trial.ConstraintCode
			if _, ok := codeScores[key]; !ok {
				codeScores[key] = &codeStats{tier: trial.Tier}
			}
			codeScores[key].scores = append(codeScores[key].scores, trial.Score)
		}
	}

	// Identify weak codes (T1 avg score < 9.0)
	for code, stats := range codeScores {
		avg := 0.0
		for _, s := range stats.scores {
			avg += s
		}
		avg /= float64(len(stats.scores))

		if avg < 9.0 {
			// Find the trial with lowest score to analyze failure
			reason := ""
			for _, trial := range req.Trials {
				if trial.ConstraintCode == code && trial.Tier == TierCoded {
					if trial.Score < 9.0 {
						reason = fmt.Sprintf("interpretation diverged: %q", trial.Interpretation)
						break
					}
				}
			}
			resp.WeakCodes = append(resp.WeakCodes, WeakCodeReport{
				Code:          code,
				Tier:          TierCoded,
				AvgScore:      avg,
				TrialCount:    len(stats.scores),
				FailureReason: reason,
			})
		}
	}

	// Snapshot metrics after ingestion
	if s.protocolMetrics != nil {
		resp.MetricsAfter = s.protocolMetrics.Snapshot()
	}

	return resp
}

// RegenerateCode uses the LLM to generate an improved code for a constraint,
// given context about why the previous code failed.
func (s *Service) RegenerateCode(ctx context.Context, constraintType, description, oldCode, failureReason string) (string, error) {
	if s.codeGenerator == nil || s.codeGenerator.client == nil {
		return "", fmt.Errorf("code generator not available")
	}

	prompt := fmt.Sprintf(`Generate an improved mnemonic kebab-case code (2-5 words) for this constraint.

Constraint type: %s
Description: %s

The PREVIOUS code was: %s
It scored poorly because: %s

Requirements for the new code:
- Must be more descriptive and unambiguous than the old code
- Should clearly convey the ACTION that is required or forbidden
- Must not be confused with other concepts (the old code was ambiguous)
- 2-5 words, kebab-case, lowercase

Respond with ONLY the new kebab-case code, nothing else.`, constraintType, description, oldCode, failureReason)

	resp, err := s.codeGenerator.client.Complete(ctx, []llmclient.Message{
		{Role: "user", Content: prompt},
	}, llmclient.CompleteOpts{})
	if err != nil {
		return "", fmt.Errorf("LLM code regeneration failed: %w", err)
	}

	code := sanitizeCode(resp)
	if code == "" || code == oldCode {
		return "", fmt.Errorf("regeneration produced empty or identical code")
	}

	return code, nil
}

// GetGlossary returns a map of constraint_code → summary for all constraints
// with assigned codes in the given space. Used by the bootstrap handler.
func (s *Service) GetGlossary(ctx context.Context, spaceID string) map[string]string {
	if s.driver == nil || spaceID == "" {
		return nil
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode)
			WHERE n.space_id = $spaceId AND n.constraint_code IS NOT NULL AND n.constraint_code <> ''
			RETURN n.constraint_code AS code, COALESCE(n.summary, LEFT(n.content, 80)) AS summary
		`
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		glossary := make(map[string]string)
		for res.Next(ctx) {
			record := res.Record()
			code, _ := record.Get("code")
			summary, _ := record.Get("summary")
			if codeStr, ok := code.(string); ok {
				if summaryStr, ok := summary.(string); ok {
					glossary[codeStr] = summaryStr
				}
			}
		}
		return glossary, res.Err()
	})
	if err != nil {
		log.Printf("jiminy: GetGlossary error: %v", err)
		return nil
	}
	glossary, _ := result.(map[string]string)
	return glossary
}

// lookupConstraintCodes batch-queries Neo4j for constraint_code properties.
// Returns a map of node_id → constraint_code for all nodes that have one.
func (s *Service) lookupConstraintCodes(ctx context.Context, nodeIDs []string) map[string]string {
	if s.driver == nil || len(nodeIDs) == 0 {
		return nil
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode)
			WHERE n.node_id IN $nodeIds AND n.constraint_code IS NOT NULL AND n.constraint_code <> ''
			RETURN n.node_id AS id, n.constraint_code AS code
		`
		res, err := tx.Run(ctx, cypher, map[string]any{"nodeIds": nodeIDs})
		if err != nil {
			return nil, err
		}
		codes := make(map[string]string)
		for res.Next(ctx) {
			record := res.Record()
			id, _ := record.Get("id")
			code, _ := record.Get("code")
			if idStr, ok := id.(string); ok {
				if codeStr, ok := code.(string); ok {
					codes[idStr] = codeStr
				}
			}
		}
		return codes, res.Err()
	})
	if err != nil {
		log.Printf("jiminy: lookupConstraintCodes error: %v", err)
		return nil
	}
	codes, _ := result.(map[string]string)
	return codes
}

// guidanceSignalCode returns a Hebbian signal code for a guidance item.
// Uses constraint_code if available, otherwise falls back to the guidance type.
func guidanceSignalCode(item GuidanceItem) string {
	if item.ConstraintCode != "" {
		return "guidance:" + item.ConstraintCode
	}
	if item.Type != "" {
		return "guidance:" + string(item.Type)
	}
	return ""
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
