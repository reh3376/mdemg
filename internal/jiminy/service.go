package jiminy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/nrednav/cuid2"
	"mdemg/internal/circuitbreaker"
	"mdemg/internal/config"
	"mdemg/internal/embeddings"
	"mdemg/internal/llmclient"
	"mdemg/internal/models"
	"mdemg/internal/sanitize"
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
	tierPredictor     *TierPredictor           // Gap 6: ML tier prediction
	nliScorer           *NLIComprehensionScorer  // Gap 6: NLI comprehension scoring
	arbitrator          *SidecarArbitrator       // NS-01: sidecar mode arbitration
	dataCollector       *ProtocolDataCollector   // NS-14: protocol training data collection
	calibrationTracker  *NLICalibrationTracker   // NLI feedback loop: NLI-vs-heuristic calibration
	warmStore           *WarmStore               // B7: WarmStore reference for trust-based invalidation
	trustStore          *TrustStore              // J17: write-behind trust persistence to Neo4j
	trustCancel         context.CancelFunc       // cancels trust persistence goroutine

	// B4: Per-session feedback tracking for protocol status endpoint
	feedbackMu     sync.RWMutex
	feedbackCounts map[string]*sessionFeedback // sessionID → feedback stats
}

// sessionFeedback tracks per-session feedback statistics for the protocol status endpoint.
type sessionFeedback struct {
	Count      int
	LastFeedAt time.Time
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
		slog.Info("jiminy: persistence enabled",
			"boost", cfg.ConstraintConfidenceBoostPerPos, "decay", cfg.ConstraintConfidenceDecayPerNeg, "archive_threshold", cfg.ConstraintArchiveThreshold)
	}

	// F10: Initialize guidance cache if enabled
	var cache *GuidanceCache
	if cfg.JiminyCacheEnabled {
		cache = NewGuidanceCache(cfg.JiminyCacheSize, cfg.JiminyCacheTTLSec)
		slog.Info("jiminy: guidance cache enabled", "size", cfg.JiminyCacheSize, "ttl_sec", cfg.JiminyCacheTTLSec)
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
		slog.Info("jiminy: semantic outcome classifier enabled",
			"high_threshold", cfg.JiminyOutcomeSimilarityHigh, "low_threshold", cfg.JiminyOutcomeSimilarityLow,
			"llm_enabled", cfg.JiminyOutcomeLLMEnabled, "cache_size", cfg.JiminyOutcomeCacheSize)
	}

	// J12: Initialize escalation tracker if enabled
	var escalation *EscalationTracker
	if cfg.JiminyEscalationEnabled {
		escalation = NewEscalationTracker(cfg)
		slog.Info("jiminy: escalation enabled",
			"warn_after", cfg.JiminyEscalationWarnAfter, "escalate_after", cfg.JiminyEscalationEscalateAfter,
			"block_after", cfg.JiminyEscalationBlockAfter, "block_enabled", cfg.JiminyEscalationBlockEnabled)
	}

	// J9: Initialize evaluator if enabled
	var evaluator *Evaluator
	if cfg.JiminyEvaluateEnabled && driver != nil {
		evaluator = NewEvaluator(cfg, driver, embedder)
		slog.Info("jiminy: evaluator enabled", "timeout_ms", cfg.JiminyEvaluateTimeoutMs, "max_constraints", cfg.JiminyEvaluateMaxConstraints)
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
		slog.Info("jiminy: J17 protocol enabled", "ttl_hours", cfg.J17TicketTTLHours, "buffer_size", cfg.J17SequenceBufferSize)
	}

	// J17-4: Initialize protocol metrics collector
	var protocolMetrics *ProtocolMetricsCollector
	if cfg.J17MetricsEnabled {
		protocolMetrics = NewProtocolMetricsCollector()
		slog.Info("jiminy: J17 protocol metrics collection enabled")
	}

	// J17-5: Initialize extension registry
	var extensions *ExtensionRegistry
	if cfg.J17ExtensionsEnabled {
		extensions = NewExtensionRegistry(cfg.J17AllowedExtensions)
		slog.Info("jiminy: J17 extensions enabled", "allowed_count", len(cfg.J17AllowedExtensions))
	}

	// J17: Initialize protocol encoder and trust scorer if enabled
	var encoder *ProtocolEncoder
	var trustScorer *TrustScorer
	if cfg.J17Enabled {
		encoder = NewProtocolEncoder(cfg.J17DefaultTier)
		encoder.SetTierThresholds(cfg.J17TrustHighThreshold, cfg.J17TrustLowThreshold)
		trustScorer = NewTrustScorer(TrustConfig{
			Initial:            cfg.J17TrustInitial,
			BoostPerFollow:     cfg.J17TrustBoostPerFollow,
			DecayPerIgnore:     cfg.J17TrustDecayPerIgnore,
			DecayPerContradict: cfg.J17TrustDecayPerContradict,
			HighThreshold:      cfg.J17TrustHighThreshold,
			LowThreshold:       cfg.J17TrustLowThreshold,
			TTL:                time.Duration(cfg.J17TrustTTLHours) * time.Hour,
		})
		slog.Info("jiminy: J17 trust scoring enabled",
			"initial", cfg.J17TrustInitial, "high_threshold", cfg.J17TrustHighThreshold, "low_threshold", cfg.J17TrustLowThreshold,
			"ttl_hours", cfg.J17TrustTTLHours)
	}

	// J17: Trust persistence store
	var trustStore *TrustStore
	if cfg.J17Enabled && driver != nil && trustScorer != nil {
		trustStore = NewTrustStore(driver)
		trustScorer.SetOnDirty(trustStore.MarkDirty)
		slog.Info("jiminy: J17 trust persistence enabled")
	}

	// NS-01: ML components with sidecar arbitration (shadow → causal promotion)
	var tierPredictor *TierPredictor
	var nliScorer *NLIComprehensionScorer
	var arbitrator *SidecarArbitrator
	if cfg.J17SidecarURL != "" {
		if cfg.J17MLTierPredictionEnabled {
			tierPredictor = NewTierPredictor(cfg.J17SidecarURL, cfg.J17SidecarTimeoutMs, true)
			tierPredictor.minConfidence = cfg.J17SidecarConfidenceFloor
			slog.Info("jiminy: tier predictor enabled", "mode", cfg.J17SidecarMode, "url", cfg.J17SidecarURL)
		}
		if cfg.J17NLIComprehensionEnabled {
			nliScorer = NewNLIComprehensionScorer(cfg.J17SidecarURL, cfg.J17SidecarTimeoutMs, true)
			nliScorer.minConfidence = cfg.J17SidecarConfidenceFloor
			slog.Info("jiminy: NLI comprehension scorer enabled", "mode", cfg.J17SidecarMode, "score_of_record", cfg.J17NLIScoreOfRecord)
		}
		// Create arbitrator regardless of which ML components are enabled
		arbitrator = NewSidecarArbitrator(
			cfg.J17SidecarMode,
			cfg.J17SidecarCanaryPct,
			cfg.J17SidecarConfidenceFloor,
			cfg.J17PrecedentProtectedCodes,
			cfg.J17PrecedentLogEnabled,
		)
		slog.Info("jiminy: sidecar arbitrator configured",
			"mode", cfg.J17SidecarMode, "canary_pct", cfg.J17SidecarCanaryPct, "confidence_floor", cfg.J17SidecarConfidenceFloor)
	}

	// NS-06: Circuit breaker for sidecar calls (fail-open to rule-based fallback)
	if cfg.J17SidecarCBEnabled {
		cbCfg := circuitbreaker.Config{
			Enabled:          true,
			FailureThreshold: cfg.J17SidecarCBFailureThreshold,
			SuccessThreshold: 1,
			Timeout:          time.Duration(cfg.J17SidecarCBTimeoutSec) * time.Second,
			MaxConcurrent:    1,
		}
		if tierPredictor != nil {
			tierCB := circuitbreaker.New("sidecar-tier-predictor", cbCfg)
			tierPredictor.SetCircuitBreaker(tierCB)
		}
		if nliScorer != nil {
			nliCB := circuitbreaker.New("sidecar-nli-scorer", cbCfg)
			nliScorer.SetCircuitBreaker(nliCB)
		}
		slog.Info("jiminy: sidecar circuit breaker enabled", "threshold", cfg.J17SidecarCBFailureThreshold, "timeout_sec", cfg.J17SidecarCBTimeoutSec)
	}

	// NS-14: Protocol data collector (wired to service for training data)
	var dataCollector *ProtocolDataCollector
	if cfg.J17ProtocolDataCollection {
		dataDir := ".mdemg/neural/protocol-data"
		dataCollector = NewProtocolDataCollector(dataDir)
		slog.Info("jiminy: protocol data collection enabled", "dir", dataDir)
	}

	// NLI feedback loop: calibration tracker for NLI-vs-heuristic bias detection
	var calibrationTracker *NLICalibrationTracker
	if nliScorer != nil && cfg.J17NLICalibrationWindowSize > 0 {
		calibrationTracker = NewNLICalibrationTracker(cfg.J17NLICalibrationWindowSize, cfg.J17NLICalibrationBiasThreshold)
		slog.Info("jiminy: NLI calibration tracker enabled", "window", cfg.J17NLICalibrationWindowSize, "bias_threshold", cfg.J17NLICalibrationBiasThreshold)
	}

	// Gap 7: Warn if J17 is enabled but ticket secret is auto-generated
	if cfg.J17Enabled && cfg.J17TicketSecret == "" {
		slog.Warn("J17_TICKET_SECRET not set — auto-generated key, not persistent across restarts")
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
		nliScorer:           nliScorer,
		arbitrator:          arbitrator,
		dataCollector:       dataCollector,
		calibrationTracker:  calibrationTracker,
		trustStore:          trustStore,
		feedbackCounts:      make(map[string]*sessionFeedback),
	}
}

// SetSignalLearner sets the Hebbian signal learner for guidance emission/response tracking (RSIC-SK1).
func (s *Service) SetSignalLearner(sl SignalLearnerProvider) {
	s.signalLearner = sl
}

// StartTrustPersistence begins the background trust flush loop.
// Must be called after NewService from server.go to provide context.
func (s *Service) StartTrustPersistence(ctx context.Context) {
	if s.trustStore == nil {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	s.trustCancel = cancel
	s.trustStore.Start(ctx)

	// Background flush goroutine — reads trust scores and feedback counts, writes to Neo4j
	go func() { //nolint:gosec // flush uses Background() intentionally
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				// Final flush
				_ = s.FlushTrust(context.Background())
				return
			case <-ticker.C:
				if err := s.FlushTrust(context.Background()); err != nil {
					slog.Warn("j17: trust persistence flush failed", "error", err)
				}
			}
		}
	}()
}

// StopTrustPersistence stops the background trust flush loop and performs a final flush.
func (s *Service) StopTrustPersistence() {
	if s.trustCancel != nil {
		s.trustCancel()
	}
}

// FlushTrust writes all dirty trust + feedback state to Neo4j.
func (s *Service) FlushTrust(ctx context.Context) error {
	if s.trustStore == nil || s.trustScorer == nil {
		return nil
	}

	dirtyIDs := s.trustStore.DrainDirty()
	if len(dirtyIDs) == 0 {
		return nil
	}

	snapshots := make([]TrustSnapshot, 0, len(dirtyIDs))
	for _, sessionID := range dirtyIDs {
		score := s.trustScorer.GetScore(sessionID)
		snap := TrustSnapshot{
			SessionID:  sessionID,
			Score:      score,
			LastUpdate: time.Now(),
		}

		// Include feedback counts
		s.feedbackMu.RLock()
		if sf := s.feedbackCounts[sessionID]; sf != nil {
			snap.FeedbackCount = sf.Count
			snap.LastFeedAt = sf.LastFeedAt
		}
		s.feedbackMu.RUnlock()

		snapshots = append(snapshots, snap)
	}

	return s.trustStore.FlushSnapshots(ctx, snapshots)
}

// HydrateTrust loads persisted trust scores and feedback counts from Neo4j.
func (s *Service) HydrateTrust(ctx context.Context) error {
	if s.trustStore == nil || s.trustScorer == nil {
		return nil
	}

	snapshots, err := s.trustStore.LoadAll(ctx)
	if err != nil {
		return err
	}

	hydrated := 0
	for sessionID, snap := range snapshots {
		s.trustScorer.SetScore(sessionID, snap.Score)
		if snap.FeedbackCount > 0 {
			s.feedbackMu.Lock()
			s.feedbackCounts[sessionID] = &sessionFeedback{
				Count:      snap.FeedbackCount,
				LastFeedAt: snap.LastFeedAt,
			}
			s.feedbackMu.Unlock()
		}
		hydrated++
	}

	if hydrated > 0 {
		slog.Info("j17: trust + feedback state hydrated from Neo4j", "sessions", hydrated)
	}
	return nil
}

// RefreshTrackedGuidance re-registers guidance items in the EffectivenessTracker,
// resetting their TTL. Called on warm store reads and cache hits to prevent
// tracker entries from expiring while guidance_ids are still in active use.
func (s *Service) RefreshTrackedGuidance(guidanceID string, items []GuidanceItem) {
	if s.tracker != nil && guidanceID != "" && len(items) > 0 {
		s.tracker.Track(guidanceID, items)
	}
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

// GetOutcomeClassifier returns the outcome classifier for CB wiring (G8).
func (s *Service) GetOutcomeClassifier() *OutcomeClassifier {
	return s.classifier
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

	// F10: Check cache for fast path (session-scoped key prevents cross-session contamination)
	// Skip cache when J17 bypass is enabled — cached responses have pre-computed tier
	// assignments that become stale when trust evolves via feedback.
	cacheBypass := s.cfg.J17Enabled && s.cfg.JiminyCacheJ17Bypass
	if s.cache != nil && !cacheBypass {
		if cached, ok := s.cache.Get(req.SpaceID, req.SessionID, req.Context); ok {
			s.recordCacheHitMetrics(cached)
			// Re-register in tracker so feedback can still correlate after cache hit
			if s.tracker != nil && cached.GuidanceID != "" {
				s.tracker.Track(cached.GuidanceID, cached.Guidance)
			}
			return cached, nil
		}
	}

	// Generate guidance_id early so it's available for protocol JSONL and LLM context correlation.
	// Placed after cache-hit early return (line 447-453) to avoid wasted generation on cache hits.
	guidanceID := cuid2.Generate()
	ctx = llmclient.WithGuidanceID(ctx, guidanceID)

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
		ctx = embeddings.WithEmbeddingMeta(ctx, embeddings.EmbeddingMeta{
			CallSite:  "jiminy.guide",
			SpaceID:   req.SpaceID,
			FilePath:  req.FilePath,
			QueryText: contextText,
		})
		var err error
		queryEmbedding, err = s.embedder.Embed(ctx, contextText)
		if err != nil {
			slog.Warn("jiminy: embedding failed, continuing without vector search", "error", err)
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
				s.cfg.JiminyRetrievalTopK, s.cfg.JiminyRetrievalHopDepth, queryEmbedding)
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

	// Normalize structured metadata to natural language for embedding similarity.
	// Ingested summaries use a keyword-list format ("Module: X. Related to: a, b")
	// that embeds into a different semantic region than action descriptions, producing
	// a cosine similarity ceiling of ~0.33. Natural language produces ~0.70.
	for i := range items {
		items[i].Content = normalizeGuidanceContent(items[i].Content)
	}

	// J17-2: Populate ConstraintCode from Neo4j for all guidance items.
	// Constraint codes live on Constraint nodes (promoted from ConversationObs),
	// while guidance source nodes come from vector search (file-ingested nodes).
	// These are different populations, so we match by content similarity.
	// Codes are applied to ALL guidance types (constraints, corrections, patterns)
	// since any item may correspond to a codified constraint.
	if s.cfg.J17Enabled && s.driver != nil && len(items) > 0 {
		constraints := s.loadSpaceConstraintCodes(ctx, req.SpaceID)
		if len(constraints) > 0 {
			for i := range items {
				items[i].ConstraintCode = matchConstraintCode(items[i], constraints)
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

	// NS-01: ML tier prediction + arbitration BEFORE encoding
	// This allows the arbitrator to pre-assign tiers that the encoder will respect.
	trustScore := 0.5
	if s.trustScorer != nil && req.SessionID != "" {
		trustScore = s.trustScorer.GetScore(req.SessionID)
	}
	if s.tierPredictor != nil && s.arbitrator != nil {
		for i := range filtered {
			if filtered[i].Type != GuidanceConstraint {
				continue
			}
			pred := s.tierPredictor.PredictTier(ctx, filtered[i].Content, req.Context, trustScore)

			// Get rule-based tier for comparison (encoder.selectTier is the rule-based path)
			ruleTier := 0
			if s.encoder != nil {
				ruleTier = s.encoder.selectTier(filtered[i], trustScore)
			}

			result := s.arbitrator.ArbitrateTier(filtered[i], ruleTier, pred.Tier, pred.Conf)

			// Log shadow/compare data
			if pred.Tier > 0 {
				slog.Info("j17-sidecar: tier arbitration",
					"mode", s.arbitrator.Mode(), "ml_tier", pred.Tier, "rule_tier", ruleTier,
					"chosen", result.ChosenTier, "source", result.Source,
					"conf", pred.Conf, "agreed", result.Agreed, "code", filtered[i].ConstraintCode)
			}

			// Pre-assign tier if arbitrator chose ML (encoder will respect it via NS-01 override)
			if result.Source == "ml" {
				filtered[i].Tier = result.ChosenTier
			}

			// Record sidecar metrics (with actual latency and error from HTTP call)
			if s.protocolMetrics != nil {
				overridden := result.Source == "ml" && !result.Agreed
				isTimeout := pred.Err != nil && errors.Is(pred.Err, context.DeadlineExceeded)
				s.protocolMetrics.RecordSidecarCall(pred.LatencyMs, pred.Err, isTimeout, pred.Tier, ruleTier, overridden)
			}

			// NS-14: Collect training data
			if s.dataCollector != nil {
				s.dataCollector.Collect(protocolTrainingRecord{
					ConstraintCode:   filtered[i].ConstraintCode,
					ConstraintNodeID: firstSourceNode(filtered[i]),
					ConstraintText:   filtered[i].Content,
					TierUsed:         result.ChosenTier,
					TrustScore:       trustScore,
					SessionID:        req.SessionID,
					MLTier:           pred.Tier,
					RuleTier:         ruleTier,
					MLConfidence:     pred.Conf,
					TierSource:       result.Source,
					SidecarMode:      s.arbitrator.Mode(),
					GuidanceID:       guidanceID,
				})
			}
		}
	}

	// Format prompt augmentation
	var augmentation string
	if s.encoder != nil && s.cfg.J17Enabled {
		// J17: Use three-tier encoding with trust-modulated tier selection
		augmentation = s.encoder.Encode(filtered, counts, confidence, trustScore)
	} else {
		// Static fallback (pre-J17)
		augmentation = FormatPromptAugmentation(filtered, counts, confidence)
	}

	// Enrich context with RAFT retrieval context for training data
	if len(filtered) > 0 {
		var nodeIDs []string
		var scores []float64
		for _, item := range filtered {
			for _, nid := range item.SourceNodes {
				nodeIDs = append(nodeIDs, nid)
				scores = append(scores, item.Confidence)
			}
		}
		if len(nodeIDs) > 0 {
			ctx = llmclient.WithRetrievalContext(ctx, &llmclient.RetrievalContext{
				NodeIDs: nodeIDs,
				Scores:  scores,
			})
		}
	}

	// Save J17-encoded form before synthesis may overwrite it
	encodedAugmentation := augmentation

	// J8: LLM synthesis — replace static formatting if synthesizer is available.
	// J17: Skip synthesis at T1 — compact coded format IS the augmentation.
	// Synthesis would replace ~15-token-per-item codes with a ~2000-token narrative,
	// destroying the 5.2x compression that trust promotion was designed to achieve.
	var synthesizedNarrative string
	if s.synthesizer != nil && s.cfg.JiminySynthesisEnabled && len(filtered) > 0 {
		highT := 0.75
		if s.trustScorer != nil {
			highT = s.trustScorer.HighThreshold()
		}
		if trustScore > highT {
			slog.Info("jiminy: T1 trust — skipping synthesis, using J17 encoded augmentation",
				"trust", trustScore, "items", len(filtered))
			debug["synthesis_skipped"] = "T1_trust"
		} else {
			narrative, synthErr := s.synthesizer.Synthesize(ctx, filtered, req.Context, req.AgentOutput)
			if synthErr != nil {
				slog.Warn("jiminy: synthesis failed, using static formatting", "error", synthErr)
				debug["synthesis_error"] = synthErr.Error()
			} else if narrative != "" {
				synthesizedNarrative = narrative
				// Use synthesized narrative as the prompt augmentation
				augmentation = "═══ JIMINY GUIDANCE ═══\n" + sanitize.StripControlChars(narrative) + "\n═══ END JIMINY GUIDANCE ═══"
				debug["synthesis_used"] = true
			}
		}
	}

	// Phase AR-2: Track guidance items for effectiveness feedback
	// (guidanceID generated earlier, after cache check, for protocol JSONL correlation)
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
		EncodedAugmentation:  encodedAugmentation,
		Confidence:           confidence,
		Rationale:            rationale,
		Warnings:             warnings,
		SourceCounts:         counts,
		SessionEscalation:    sessionEscalation,
		GuidanceSeq:          guidanceSeq,
		Protocol:             protocolInfo,
		Debug:                debug,
	}

	// F10: Cache the response (session-scoped key prevents cross-session contamination)
	// Skip cache put when J17 bypass is enabled (tiers are trust-dependent, cache would stale)
	if s.cache != nil && !cacheBypass {
		s.cache.Put(req.SpaceID, req.SessionID, req.Context, resp)
	}

	return resp, nil
}

// recordCacheHitMetrics records J17 protocol metrics for a cache-hit guidance response,
// ensuring TotalEvents is incremented even when the full Guide path is skipped.
func (s *Service) recordCacheHitMetrics(resp GuidanceResponse) {
	if s.protocolMetrics == nil || s.encoder == nil || !s.cfg.J17Enabled {
		return
	}
	var constraintTotal, constraintWithCode int
	for _, item := range resp.Guidance {
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

	// Thread guidance_id into context for LLM interaction correlation
	ctx = llmclient.WithGuidanceID(ctx, req.GuidanceID)

	items := s.tracker.Lookup(req.GuidanceID)
	if items == nil {
		slog.Warn("jiminy: feedback dropped — guidance_id expired from tracker",
			"guidance_id", req.GuidanceID, "session_id", req.SessionID)
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

	// NS-14: Get trust score for training data collection
	var trustScoreForFeedback float64
	if s.trustScorer != nil {
		trustScoreForFeedback = s.trustScorer.GetScore(feedbackSessionID)
	}

	for _, item := range items {
		var cr ClassificationResult

		// B4: Honor explicit outcome from caller, bypassing heuristic/classifier
		if req.Outcome != "" {
			cr = ClassificationResult{Outcome: req.Outcome, Confidence: 1.0}
		} else if s.classifier != nil {
			// J14: Use semantic classifier returning ClassificationResult if available
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

		// J12: Feed escalation tracker with outcome (skip not_applicable — topically unrelated)
		if s.escalation != nil && len(item.SourceNodes) > 0 && outcome != OutcomeNotApplicable {
			for _, nodeID := range item.SourceNodes {
				s.escalation.RecordOutcome(feedbackSessionID, nodeID, outcome)
			}
		}

		// J17-4: Record outcome for protocol metrics and data collection
		// NLI feedback loop: when NLI scorer is available, defer recording to the NLI block
		// to avoid double-counting — but ONLY for GuidanceConstraint items (which the NLI
		// block processes). Non-constraint items (corrections, patterns, etc.) always use
		// heuristic scores since the NLI block only handles GuidanceConstraint.
		if s.protocolMetrics != nil && item.ConstraintCode != "" && outcome != OutcomeNotApplicable && (s.nliScorer == nil || item.Type != GuidanceConstraint) {
			var compScore float64
			switch outcome {
			case OutcomeFollowed:
				compScore = 1.0
			case OutcomePartialCompliance:
				compScore = 0.7
			case OutcomeContradicted:
				compScore = 1.0 // understood but violated
			case OutcomeIgnored:
				compScore = 0.0
			default:
				compScore = 0.5
			}
			s.protocolMetrics.RecordOutcomeWithTier(item.ConstraintCode, item.Tier, compScore)
		}

		// F3: Persist guidance outcome to Neo4j and update constraint confidence
		// Skip not_applicable — topically unrelated items should not create edges or decay confidence
		if s.persistence != nil && outcome != OutcomeUnknown && outcome != OutcomeNotApplicable {
			if err := s.persistence.PersistGuidanceOutcome(ctx, req.SpaceID, req.GuidanceID, "", item, outcome, cr.Confidence); err != nil {
				slog.Error("jiminy: persist outcome failed", "error", err)
			}
			// RSIC-SK1: Update confidence for all guidance types with source nodes
			if s.confidenceUpdater != nil && len(item.SourceNodes) > 0 {
				if err := s.confidenceUpdater.UpdateConfidence(ctx, item.SourceNodes[0], outcome); err != nil {
					slog.Error("jiminy: confidence update failed", "error", err)
				}
			}
		}
		// RSIC-SK1: Record signal response for positive outcomes (independent of persistence)
		if s.signalLearner != nil && (outcome == OutcomeFollowed || outcome == OutcomePartialCompliance) {
			if code := guidanceSignalCode(item); code != "" {
				s.signalLearner.RecordResponse(code)
			}
		}

		// NS-03: Build multi-dimensional feedback (comprehension, applicability, score source)
		var dims *FeedbackDimensions
		if item.Type == GuidanceConstraint && outcome != OutcomeNotApplicable {
			followed := outcome == OutcomeFollowed || outcome == OutcomePartialCompliance
			var comprehension float64
			scoreSource := "heuristic"

			if s.nliScorer != nil {
				nliScore, isFallback := s.nliScorer.ScoreComprehension(ctx, item.Content, req.ActionSummary, followed)

				if isFallback {
					// NLI unavailable — do NOT record fallback score into comprehension metrics.
					// Use heuristic comprehension to prevent degraded-state cascades.
					scoreSource = "nli_fallback"
					slog.Debug("j17-sidecar: NLI fallback, using heuristic for comprehension",
						"constraint", item.ConstraintCode, "followed", followed)

					// Track fallback rate for RSIC awareness
					if s.protocolMetrics != nil {
						s.protocolMetrics.RecordNLIFallback()
					}

					// Heuristic comprehension (same as the else block)
					switch outcome {
					case OutcomeFollowed:
						comprehension = 1.0
					case OutcomePartialCompliance:
						comprehension = 0.7
					case OutcomeContradicted:
						comprehension = 1.0
					case OutcomeIgnored:
						comprehension = 0.0
					default:
						comprehension = 0.5
					}
					// DO NOT record to protocolMetrics.RecordOutcomeWithTier (prevents metric poisoning)
					// DO NOT feed to calibrationTracker.Track (prevents false bias alarm)
				} else {
					// Genuine NLI score — record to all pipelines
					comprehension = nliScore
					scoreSource = "nli"

					slog.Info("j17-sidecar: NLI score computed",
						"nli", nliScore, "heuristic", cr.Confidence, "constraint", item.ConstraintCode, "score_of_record", s.cfg.J17NLIScoreOfRecord)

					// NLI feedback loop: Record NLI score to metrics in ALL modes (shadow/compare/canary/active).
					// NLI is OBSERVATIONAL data — RSIC needs it regardless of arbitration mode.
					if s.cfg.J17NLIObservationalEnabled && s.protocolMetrics != nil && item.ConstraintCode != "" {
						s.protocolMetrics.RecordOutcomeWithTier(item.ConstraintCode, item.Tier, nliScore)
					}

					// NLI calibration: compare NLI with heuristic for bias detection
					if s.calibrationTracker != nil {
						var heuristicComp float64
						switch outcome {
						case OutcomeFollowed:
							heuristicComp = 1.0
						case OutcomePartialCompliance:
							heuristicComp = 0.7
						case OutcomeContradicted:
							heuristicComp = 1.0
						case OutcomeIgnored:
							heuristicComp = 0.0
						default:
							heuristicComp = 0.5
						}
						s.calibrationTracker.Track(nliScore, heuristicComp)
					}
				}
			} else {
				// Heuristic fallback: map outcome to comprehension
				switch outcome {
				case OutcomeFollowed:
					comprehension = 1.0
				case OutcomePartialCompliance:
					comprehension = 0.7
				case OutcomeContradicted:
					comprehension = 1.0 // understood but violated
				case OutcomeIgnored:
					comprehension = 0.0
				default:
					comprehension = 0.5
				}
			}

			// Applicability: 1.0 if agent acted on it at all (followed or contradicted), 0.0 if ignored
			var applicability float64
			switch outcome {
			case OutcomeFollowed, OutcomePartialCompliance, OutcomeContradicted:
				applicability = 1.0
			case OutcomeIgnored:
				applicability = 0.0
			default:
				applicability = 0.5
			}

			dims = &FeedbackDimensions{
				Adherence:     outcome,
				Comprehension: comprehension,
				Applicability: applicability,
				ScoreSource:   scoreSource,
			}
		}

		// Attach dimensions to the result
		results[len(results)-1].Dimensions = dims

		// NS-14: Collect outcome training data
		if s.dataCollector != nil && dims != nil {
			s.dataCollector.Collect(protocolTrainingRecord{
				ConstraintCode:     item.ConstraintCode,
				ConstraintNodeID:   firstSourceNode(item),
				ConstraintText:     item.Content,
				TierUsed:           item.Tier,
				AgentAction:        req.ActionSummary,
				Outcome:            string(outcome),
				ComprehensionScore: dims.Comprehension,
				ApplicabilityScore: dims.Applicability,
				ScoreSource:        dims.ScoreSource,
				TrustScore:         trustScoreForFeedback,
				SessionID:          feedbackSessionID,
				GuidanceID:         req.GuidanceID,
			})
		}
	}

	// J17: Aggregate trust update — single update per feedback call, not per item.
	// Prevents trust burst when multiple items are all "followed" in one call.
	//
	// Only items with sufficient classification confidence (similarity >= 0.5) affect
	// trust scoring. Items with low similarity were "not applicable" to the action —
	// the agent didn't ignore the guidance, the guidance wasn't relevant. Without
	// this filter, trust systematically decays because every feedback cycle includes
	// many irrelevant items that are falsely counted as "ignored."
	if s.trustScorer != nil && feedbackSessionID != "" && len(results) > 0 {
		const trustRelevanceThreshold = 0.20 // minimum classifier similarity for trust impact
		var followCount, partialCount, ignoreCount, contradictCount int
		for _, r := range results {
			if r.Similarity < trustRelevanceThreshold {
				continue // not applicable to this action — skip for trust
			}
			switch r.Outcome {
			case OutcomeFollowed:
				followCount++
			case OutcomePartialCompliance:
				partialCount++
			case OutcomeIgnored:
				ignoreCount++
			case OutcomeContradicted:
				contradictCount++
			}
		}
		var aggregateOutcome GuidanceOutcome
		switch {
		case contradictCount > 0:
			aggregateOutcome = OutcomeContradicted // high-confidence contradiction dominates
		case followCount > ignoreCount:
			aggregateOutcome = OutcomeFollowed
		case partialCount > 0 && ignoreCount == 0:
			aggregateOutcome = OutcomePartialCompliance // net-positive, boost trust
		case ignoreCount > 0:
			aggregateOutcome = OutcomeIgnored
		default:
			aggregateOutcome = OutcomeUnknown
		}
		if aggregateOutcome != OutcomeUnknown {
			newTrust := s.trustScorer.RecordOutcome(feedbackSessionID, aggregateOutcome)

			// B7: Invalidate WarmStore if trust crossed a tier threshold
			if s.warmStore != nil {
				highT := s.trustScorer.HighThreshold()
				lowT := s.trustScorer.LowThreshold()
				if (trustScoreForFeedback <= highT && newTrust > highT) || // T2→T1 promotion
					(trustScoreForFeedback < lowT && newTrust >= lowT) || // T3→T2 promotion
					(trustScoreForFeedback > highT && newTrust <= highT) || // T1→T2 demotion
					(trustScoreForFeedback >= lowT && newTrust < lowT) { // T2→T3 demotion
					s.warmStore.Invalidate(req.SpaceID)
				}
			}
			_ = newTrust // used above in threshold check
		}
	}

	// B4: Track per-session feedback count and timestamp for protocol status
	if feedbackSessionID != "" {
		s.feedbackMu.Lock()
		sf := s.feedbackCounts[feedbackSessionID]
		if sf == nil {
			sf = &sessionFeedback{}
			s.feedbackCounts[feedbackSessionID] = sf
		}
		sf.Count++
		sf.LastFeedAt = time.Now()
		s.feedbackMu.Unlock()

		// Mark dirty for trust persistence (feedbackCounts changed)
		if s.trustStore != nil {
			s.trustStore.MarkDirty(feedbackSessionID)
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
	if similarity >= 0.5 {
		return OutcomeFollowed, similarity
	}
	if similarity >= 0.2 {
		return OutcomeIgnored, similarity
	}
	if similarity > 0.0 {
		return OutcomeNotApplicable, similarity
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
		slog.Warn("jiminy: J17 ticket restore failed, graceful fallback", "error", err)
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

// SetWarmStore sets the warm store reference for trust-based invalidation (B7).
func (s *Service) SetWarmStore(ws *WarmStore) {
	s.warmStore = ws
}

// BootstrapCodes codifies all constraints that lack a constraint_code, breaking
// the cold-start deadlock where no codes → all T3 → no T2 frequency → RSIC never fires.
func (s *Service) BootstrapCodes(ctx context.Context, spaceID string) (int, error) {
	if s.codeGenerator == nil || s.driver == nil {
		return 0, nil
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.Run(ctx,
		`MATCH (n:MemoryNode {space_id: $spaceId})
		 WHERE n.role_type = 'constraint'
		   AND (n.constraint_code IS NULL OR n.constraint_code = '')
		 RETURN n.node_id AS nodeId, n.content AS content`,
		map[string]any{"spaceId": spaceID})
	if err != nil {
		return 0, fmt.Errorf("j17: bootstrap query: %w", err)
	}

	type uncoded struct {
		nodeID  string
		content string
	}
	var items []uncoded
	for result.Next(ctx) {
		rec := result.Record()
		nid, _ := rec.Get("nodeId")
		cont, _ := rec.Get("content")
		if nid != nil && cont != nil {
			items = append(items, uncoded{
				nodeID:  nid.(string),
				content: cont.(string),
			})
		}
	}
	if err := result.Err(); err != nil {
		return 0, fmt.Errorf("j17: bootstrap iterate: %w", err)
	}

	if len(items) == 0 {
		return 0, nil
	}

	codified := 0
	writeSess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer writeSess.Close(ctx)

	for _, item := range items {
		code, genErr := s.codeGenerator.GenerateCode(ctx, "constraint", item.content)
		if genErr != nil {
			slog.Warn("j17: bootstrap codegen failed", "node_id", item.nodeID, "error", genErr)
			continue
		}

		_, writeErr := writeSess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			cypher := `MATCH (n:MemoryNode)
				WHERE n.node_id = $nodeId AND n.space_id = $spaceId
				  AND (n.constraint_code IS NULL OR n.constraint_code = '')
				SET n.constraint_code = $code,
					n.constraint_code_assigned_at = datetime(),
					n.constraint_code_assigned_by = "mdemg-bootstrap"
				RETURN count(n) AS updated`
			_, runErr := tx.Run(ctx, cypher, map[string]any{
				"nodeId":  item.nodeID,
				"spaceId": spaceID,
				"code":    code,
			})
			return nil, runErr
		})
		if writeErr != nil {
			slog.Warn("j17: bootstrap write failed", "node_id", item.nodeID, "error", writeErr)
			continue
		}
		codified++
	}

	return codified, nil
}

// GetProtocolMetricsSnapshot returns an immutable snapshot of J17 protocol metrics.
// Returns nil if metrics collection is not enabled.
// Trust score aggregates are merged from the TrustScorer if available.
func (s *Service) GetProtocolMetricsSnapshot() *ProtocolMetrics {
	if s.protocolMetrics == nil {
		return nil
	}
	snap := s.protocolMetrics.Snapshot()
	if s.trustScorer != nil {
		avg, min, max, count := s.trustScorer.Aggregates()
		snap.AvgTrustScore = avg
		snap.MinTrustScore = min
		snap.MaxTrustScore = max
		snap.TrustSessionCount = count
	}
	return snap
}

// GetProtocolMetricsCollector returns the J17 protocol metrics collector for RSIC wiring.
func (s *Service) GetProtocolMetricsCollector() *ProtocolMetricsCollector {
	return s.protocolMetrics
}

// ProtocolStatus holds the current J17 protocol state for a session (B4).
type ProtocolStatus struct {
	SessionID      string     `json:"session_id"`
	TrustScore     float64    `json:"trust_score"`
	Tier           string     `json:"tier"`
	FeedbackCount  int        `json:"feedback_count"`
	LastFeedbackAt *time.Time `json:"last_feedback_at,omitempty"`
	Enabled        bool       `json:"enabled"`
}

// GetProtocolStatus returns the current J17 protocol status for a session (B4).
func (s *Service) GetProtocolStatus(sessionID string) ProtocolStatus {
	status := ProtocolStatus{
		SessionID: sessionID,
		Enabled:   s.cfg.JiminyEnabled,
	}

	// Read current trust score
	var trustScore float64
	if s.trustScorer != nil {
		trustScore = s.trustScorer.GetScore(sessionID)
		status.TrustScore = trustScore
	} else {
		// Default initial trust when scorer is not configured
		trustScore = 0.5
		status.TrustScore = trustScore
	}

	// Determine current tier label using the same thresholds as selectTier.
	// selectTier also considers whether an item has a constraint code, but for
	// the status endpoint we report the dominant tier the session would receive.
	if s.encoder != nil {
		high, low := s.encoder.GetTierThresholds()
		switch {
		case trustScore > high:
			status.Tier = "T1"
		case trustScore >= low:
			status.Tier = "T2"
		default:
			status.Tier = "T3"
		}
	} else if s.trustScorer != nil {
		high := s.trustScorer.HighThreshold()
		low := s.trustScorer.LowThreshold()
		switch {
		case trustScore > high:
			status.Tier = "T1"
		case trustScore >= low:
			status.Tier = "T2"
		default:
			status.Tier = "T3"
		}
	} else {
		status.Tier = "T2" // default mid-tier when no trust/encoder configured
	}

	// Read per-session feedback stats
	s.feedbackMu.RLock()
	if sf := s.feedbackCounts[sessionID]; sf != nil {
		status.FeedbackCount = sf.Count
		ts := sf.LastFeedAt
		status.LastFeedbackAt = &ts
	}
	s.feedbackMu.RUnlock()

	return status
}

// BuildTierEffectivenessDataset creates a curated tier effectiveness dataset and optionally writes it.
func (s *Service) BuildTierEffectivenessDataset() *TierEffectivenessDataset {
	snapshot := s.GetProtocolMetricsSnapshot()
	if snapshot == nil {
		return nil
	}
	dataset := BuildTierEffectivenessDataset(snapshot, s.cfg.J17TierEffectivenessMinSamples, s.cfg.J17TierIneffectiveThreshold)
	if dataset != nil && s.dataCollector != nil {
		s.dataCollector.CollectDataset(dataset)
	}
	return dataset
}

// GetNLICalibrationReport returns the NLI calibration report, or nil if calibration tracking is not active.
func (s *Service) GetNLICalibrationReport() *NLICalibrationReport {
	if s.calibrationTracker == nil {
		return nil
	}
	return s.calibrationTracker.Report()
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
		slog.Error("jiminy: GetGlossary failed", "error", err)
		return nil
	}
	glossary, _ := result.(map[string]string)
	return glossary
}

// lookupConstraintCodes batch-queries Neo4j for constraint_code properties.
// Returns a map of node_id → constraint_code for all nodes that have one.
// constraintCodeEntry holds a constraint code and its content for matching.
type constraintCodeEntry struct {
	Code    string
	Content string
	Words   map[string]bool // significant words for matching
}

// loadSpaceConstraintCodes loads all constraint codes for a space from Neo4j.
func (s *Service) loadSpaceConstraintCodes(ctx context.Context, spaceID string) []constraintCodeEntry {
	if s.driver == nil {
		return nil
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
			MATCH (c:MemoryNode)
			WHERE c.space_id = $spaceId
			  AND c.constraint_code IS NOT NULL AND c.constraint_code <> ''
			RETURN DISTINCT c.constraint_code AS code, c.content AS content
		`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		var entries []constraintCodeEntry
		for res.Next(ctx) {
			record := res.Record()
			codeVal, _ := record.Get("code")
			contentVal, _ := record.Get("content")
			code, _ := codeVal.(string)
			content, _ := contentVal.(string)
			if code == "" {
				continue
			}
			entry := constraintCodeEntry{
				Code:    code,
				Content: content,
				Words:   significantWordSet(content),
			}
			entries = append(entries, entry)
		}
		return entries, res.Err()
	})
	if err != nil {
		slog.Error("jiminy: loadSpaceConstraintCodes failed", "error", err)
		return nil
	}
	entries, _ := result.([]constraintCodeEntry)
	return entries
}

// matchConstraintCode finds the best constraint code for a guidance item by keyword overlap.
// Returns the code and match score; empty string if no match meets the minimum threshold.
func matchConstraintCode(item GuidanceItem, constraints []constraintCodeEntry) string {
	if len(constraints) == 0 {
		return ""
	}
	itemWords := significantWordSet(item.Content)
	if len(itemWords) == 0 {
		return ""
	}

	bestCode := ""
	bestScore := 0

	for _, c := range constraints {
		if len(c.Words) == 0 {
			continue
		}
		overlap := 0
		for w := range itemWords {
			if c.Words[w] {
				overlap++
			}
		}
		if overlap > bestScore {
			bestScore = overlap
			bestCode = c.Code
		}
	}

	// Require at least 3 word overlap to avoid spurious matches
	if bestScore < 3 {
		return ""
	}
	return bestCode
}

// significantWordSet extracts unique lowercase words >= 4 chars from text.
func significantWordSet(text string) map[string]bool {
	words := significantWords(text)
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[w] = true
	}
	return set
}

// guidanceSignalCode returns a Hebbian signal code for a guidance item.
// Uses constraint_code if available, otherwise falls back to the guidance type.
// firstSourceNode returns the first source node ID from a guidance item, or empty string.
func firstSourceNode(item GuidanceItem) string {
	if len(item.SourceNodes) > 0 {
		return item.SourceNodes[0]
	}
	return ""
}

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
