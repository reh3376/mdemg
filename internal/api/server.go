package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/alert"
	"mdemg/internal/anomaly"
	"mdemg/internal/ape"
	"mdemg/internal/auth"
	"mdemg/internal/backpressure"
	"mdemg/internal/backup"
	"mdemg/internal/circuitbreaker"
	"mdemg/internal/config"
	"mdemg/internal/consulting"
	"mdemg/internal/conversation"
	"mdemg/internal/dockerbin"
	"mdemg/internal/embeddings"
	"mdemg/internal/eventgraph"
	"mdemg/internal/filewatcher"
	"mdemg/internal/ftloop"
	"mdemg/internal/gaps"
	"mdemg/internal/guardrail"
	"mdemg/internal/hidden"
	"mdemg/internal/jiminy"
	"mdemg/internal/jobhealth"
	"mdemg/internal/jobs"
	"mdemg/internal/learning"
	"mdemg/internal/llmclient"
	"mdemg/internal/metalearn"
	"mdemg/internal/metrics"
	"mdemg/internal/models"
	"mdemg/internal/plugins"
	"mdemg/internal/ratelimit"
	"mdemg/internal/retrieval"
	"mdemg/internal/review"
	"mdemg/internal/scraper"
	"mdemg/internal/symbols"
	"mdemg/internal/transfer"
	"mdemg/internal/tsdb"
	"mdemg/internal/unts"
	"mdemg/internal/validation"
)

type Server struct {
	cfg       config.Config
	driver    neo4j.DriverWithContext
	retriever *retrieval.Service
	learner   *learning.Service
	embedder  embeddings.Embedder
	// Phase 14.2.1 — vector-based query→fingerprint cache. nil-safe;
	// initialized in NewServer when an embedder is available.
	contextFPCache          *contextFingerprintCache
	anomalyDetector         *anomaly.Service
	hiddenLayer             *hidden.Service
	pluginMgr               *plugins.Manager
	apeScheduler            *ape.Scheduler
	symbolStore             *symbols.Store
	consultant              *consulting.Service
	gapDetector             *gaps.GapDetector
	gapInterviewer          *gaps.GapInterviewer
	conversationSvc         *conversation.Service
	contextCooler           *conversation.ContextCooler
	sessionTracker          *conversation.SessionTracker
	hiddenSvc               *hidden.Service // alias for handleConversationConsolidate
	webhookDebouncer        *linearWebhookDebouncer
	genericWebhookDebouncer *webhookDebouncer
	fileWatcherMgr          *filewatcher.Manager
	stopConsolidate         chan struct{}
	stopCooler              chan struct{}
	stopInterviewer         chan struct{}
	stopScheduledSync       chan struct{}
	stopSpacePrune          chan struct{}
	bgWg                    sync.WaitGroup                                        // tracks background goroutine completion
	superviseFn             func(name string, fn func(ctx context.Context) error) // SUPERVISOR-002: injected supervisor launcher

	// Phase 3: Production readiness components
	cbRegistry      *circuitbreaker.Registry
	metricsRegistry *metrics.Registry
	metricsRecorder *metrics.MetricsRecorder

	// Phase 48.4: Connection pooling components
	memoryPressure *backpressure.MemoryPressure

	// Phase 60: CMS Advanced II
	templateService  *conversation.TemplateService
	snapshotService  *conversation.SnapshotService
	orgReviewService *conversation.OrgReviewService

	// Phase 60b: RSIC (Recursive Self-Improvement Cycle)
	rsicCycle    *ape.CycleOrchestrator
	rsicWatchdog *ape.Watchdog

	// SR-001: Alert dispatcher
	alertDispatcher *alert.Dispatcher

	// Phase 87: RSIC Orchestration
	orchestrationPolicy *ape.OrchestrationPolicy
	macroNextRun        time.Time
	macroCronCancel     context.CancelFunc

	// Phase 88: RSIC Safety
	snapshotStore *ape.SnapshotStore

	// Phase 89: RSIC Persistence
	rsicStore *ape.RSICStore

	// Phase 51: Web Scraper
	scraperSvc *scraper.Service

	// Phase 70: Neo4j Backup & Restore
	backupSvc       *backup.Service
	backupScheduler *backup.Scheduler

	// TSDB Backup & Restore
	tsdbBackupSvc       *tsdb.TSDBBackupService
	tsdbBackupScheduler *tsdb.TSDBBackupScheduler

	// Phase 75: Relationship extraction
	symbolParser   *symbols.Parser
	symbolResolver *symbols.Resolver

	// Phase 102: Intent Translation
	intentTranslator retrieval.IntentTranslator

	// Phase 104: Active MCP Guardrails
	guardrailValidator guardrail.Validator
	// GUARDRAIL-PRODUCER-001: bounds concurrent detached producer evaluations.
	guardrailProducerSem chan struct{}

	// Phase 105: Global Meta-Learning
	metaLearnSvc *metalearn.Service

	// Phase 80: Meta-Cognition
	signalLearner *ape.SignalLearner

	// Phase Jiminy: Jiminy Guidance
	jiminySvc *jiminy.Service
	warmStore *jiminy.WarmStore

	// Phase 38: UNTS Hash Verification
	untsRegistry *unts.Registry
	untsScanner  *unts.Scanner

	// Phase 9.4: Event dispatch for non-APE modules
	eventDispatcher *plugins.EventDispatcher

	// FSD-2026-001: Constraint Enforcement Event Log
	enforcementLog *enforcementEventLog

	// F4: Cross-Constraint Conflict Detection
	conflictDetector *hidden.ConflictDetector

	// TSDB Sprint: LiveCollectors for real-time Prometheus gauges
	liveCollectors *ape.LiveCollectors

	// TSDB Sprint: Historical metric writer
	tsdbClient               *tsdb.Client
	tsdbWriter               *tsdb.MetricWriter
	llmWriter                *tsdb.LLMInteractionWriter
	embeddingWriter          *tsdb.EmbeddingEventWriter
	retrievalWriter          *tsdb.RetrievalEventWriter
	retrievalAuditWriter     *tsdb.RetrievalAuditWriter
	sparseGateWriter         *tsdb.SparseGateMetricsWriter
	reinforcementWriter      *tsdb.ReinforcementEventsWriter
	eventgraphService        *eventgraph.Service
	constraintOutcomesWriter *tsdb.ConstraintOutcomesWriter
	guidanceTrainingWriter   *tsdb.GuidanceTrainingRowsWriter
	reviewWriter             *tsdb.ReviewGradesWriter
	contradictedDraftWriter  *tsdb.ContradictedDraftsWriter // JIMINY-CONTRADICTED-BRIDGE-001
	reviewRegistry           *review.Registry
	// HITL-AUTOGRADE-PREVIEW-001 (2026-08-12): lazily-built autograder for the
	// /v1/review/autograde-preview endpoint. Server holds a singleton; first-
	// request constructs, subsequent requests reuse. Guarded by reviewAutograderMu.
	reviewAutograder         *review.Autograder
	reviewAutograderMu       sync.Mutex
	llmEndpointHealthWriter  *tsdb.LLMEndpointHealthWriter

	// DOCKER-P2: Browser dashboard log buffer
	logBuffer *LogRingBuffer

	// Grafana Neo4j Dashboard: cached graph metrics (60s TTL)
	graphMetricsCache struct {
		sync.Mutex
		data    []metrics.SpaceGraphData
		updated time.Time
	}

	// Grafana Neo4j Dashboard: cached container stats (60s TTL)
	containerStatsCache struct {
		sync.Mutex
		data    *metrics.ContainerStats
		updated time.Time
	}
}

func NewServer(cfg config.Config, driver neo4j.DriverWithContext, pluginMgr *plugins.Manager) *Server {
	ret := retrieval.NewService(cfg, driver)
	// CONFIG-DEADFLAG-001: CONFLICT_LOG_ENABLED — setter existed since
	// Phase 9.5 with zero callers; the package default (true) applied
	// regardless of the env var.
	retrieval.SetConflictLogEnabled(cfg.ConflictLogEnabled)

	// Wire reasoning modules into retrieval pipeline
	if pluginMgr != nil {
		reasoningProvider := retrieval.NewPluginReasoningProvider(pluginMgr)
		ret.SetReasoningProvider(reasoningProvider)
	}

	lea := learning.NewService(cfg, driver)

	// Initialize embedder (optional - nil if not configured)
	var emb embeddings.Embedder
	if cfg.EmbeddingProvider != "" {
		embCfg := embeddings.Config{
			Provider:         cfg.EmbeddingProvider,
			OpenAIAPIKey:     cfg.OpenAIAPIKey,
			OpenAIModel:      cfg.OpenAIModel,
			OpenAIEndpoint:   cfg.OpenAIEndpoint,
			OllamaEndpoint:   cfg.OllamaEndpoint,
			OllamaModel:      cfg.OllamaModel,
			TargetDimensions: cfg.EmbeddingTargetDims,
			CacheEnabled:     cfg.EmbeddingCacheEnabled,
			CacheSize:        cfg.EmbeddingCacheSize,
		}
		var err error
		emb, err = embeddings.New(embCfg)
		if err != nil {
			slog.Warn("embedding provider failed to initialize", "provider", cfg.EmbeddingProvider, "error", err)
		} else {
			slog.Info("embedding provider initialized", "provider", emb.Name(), "dimensions", emb.Dimensions())
		}
	} else {
		slog.Info("no embedding provider configured (set EMBEDDING_PROVIDER=openai or ollama)")
	}

	// Initialize anomaly detector
	anomalyCfg := anomaly.Config{
		Enabled:            cfg.AnomalyDetectionEnabled,
		DuplicateThreshold: cfg.AnomalyDuplicateThreshold,
		OutlierStdDevs:     cfg.AnomalyOutlierStdDevs,
		StaleDays:          cfg.AnomalyStaleDays,
		MaxCheckMs:         cfg.AnomalyMaxCheckMs,
		VectorIndexName:    cfg.VectorIndexName,
	}
	anom := anomaly.NewService(driver, anomalyCfg)
	if anomalyCfg.Enabled {
		slog.Info("anomaly detection enabled", "duplicate_threshold", anomalyCfg.DuplicateThreshold, "timeout_ms", anomalyCfg.MaxCheckMs)
	}

	// Initialize hidden layer service (circuit breaker wired later after cbRegistry init)
	hid := hidden.NewService(cfg, driver, nil)
	// F18: Wire edge pruner so RunConsolidation can auto-prune excess edges when enabled
	if cfg.LearningAutoPruneExcessEnabled {
		hid.SetEdgePruner(lea)
	}
	if cfg.HiddenLayerEnabled {
		slog.Info("hidden layer enabled", "eps", cfg.HiddenLayerClusterEps, "min_samples", cfg.HiddenLayerMinSamples, "max_hidden", cfg.HiddenLayerMaxHidden)
	}
	if cfg.EmergenceEnabled {
		slog.Info("dynamic emergence enabled", "provider", cfg.EmergenceProvider, "model", cfg.EmergenceModel, "min_weight", cfg.EmergenceMinWeight, "min_cluster_size", cfg.EmergenceMinClusterSize)
	}

	// Initialize symbol store.
	// CONFIG-DEADFLAG-001: the six REL_* env vars were parsed but the
	// extraction pipeline ran on literals (all tiers on, 50 calls/func,
	// 500-row batches, cross-file on, no timeout). Defaults preserved.
	symStore := symbols.NewStore(driver)
	symStore.SetRelationshipBatchSize(cfg.RelBatchSize) // REL_BATCH_SIZE
	symParser, symParserErr := symbols.NewParser(symbols.ParserConfig{
		Relationships: &symbols.RelationshipExtractionConfig{
			ExtractInheritance: cfg.RelExtractInheritance, // REL_EXTRACT_INHERITANCE
			ExtractCalls:       cfg.RelExtractCalls,       // REL_EXTRACT_CALLS
			MaxCallsPerFunc:    cfg.RelMaxCallsPerFunc,    // REL_MAX_CALLS_PER_FUNCTION
		},
	})
	if symParserErr != nil {
		slog.Warn("symbol parser init failed (relationship extraction disabled)", "error", symParserErr)
	}
	symResolver := symbols.NewResolverWithConfig(driver, symbols.ResolverConfig{
		CrossFileResolve:  cfg.RelCrossFileResolve,                               // REL_CROSS_FILE_RESOLVE
		ResolutionTimeout: time.Duration(cfg.RelResolutionTimeout) * time.Second, // REL_RESOLUTION_TIMEOUT_SEC
	})
	slog.Info("symbol store initialized (parser + resolver for relationship extraction)")

	// Initialize gap detector for capability gap detection
	// Collect registered ingestion sources from plugins
	var registeredSources []string
	if pluginMgr != nil {
		for _, mod := range pluginMgr.GetIngestionModules() {
			registeredSources = append(registeredSources, mod.Manifest.Capabilities.IngestionSources...)
		}
	}
	gapCfg := gaps.DetectorConfig{
		LowScoreThreshold: cfg.GapLowScoreThreshold,
		MinOccurrences:    cfg.GapMinOccurrences,
		AnalysisWindow:    time.Duration(cfg.GapAnalysisWindowHours) * time.Hour,
		MetricsWindowSize: cfg.GapMetricsWindowSize,
		RegisteredSources: registeredSources,
	}
	gapDet := gaps.NewGapDetector(driver, gapCfg)
	slog.Info("gap detector initialized", "threshold", gapCfg.LowScoreThreshold, "min_occurrences", gapCfg.MinOccurrences)

	// Initialize gap interviewer for weekly gap interview processing
	gapInt := gaps.NewGapInterviewer(driver)
	slog.Info("gap interviewer initialized")

	// Initialize conversation service (Phase 1: Observation Capture with Surprise Detection).
	// CONVERSATION-EMBEDDER-OPTIONAL-001 (2026-08-06): construct the service
	// UNCONDITIONALLY. The service already handles nil embedder gracefully
	// (Observe() skips embedding + surprise detection; observations still land
	// as raw content + metadata + graph nodes). Pre-sprint this block skipped
	// construction entirely when emb == nil, which meant a fresh install in
	// EMBEDDING_PROVIDER=disabled mode couldn't write ANY observations —
	// effectively read-only. Beta blocker B5.
	convSvc := conversation.NewServiceWithConfig(driver, emb, cfg.VectorIndexName, cfg)
	// EVENTGRAPH-003 fix: inject the learning service so Observe() triggers
	// CoactivateSession. This setter had no caller — the conversation
	// service's learningService stayed nil, so session co-activation
	// (CO_ACTIVATED_WITH edges between same-session observations) NEVER
	// fired (0 such edges ever in mdemg-dev). Discovered via EVENTGRAPH-003
	// live smoke when coactivate_session reinforcement events never landed.
	convSvc.SetLearningService(lea)
	slog.Info("conversation service initialized",
		"vector_index", cfg.VectorIndexName,
		"constraint_detection", cfg.ConstraintDetectionEnabled,
		"embedder_available", emb != nil)

	// Phase 14.2 Epic 3: wire ContextCatalog loader so Service.Observe
	// can compute observe-time fingerprints when CONTEXT_FINGERPRINT_ENABLED.
	// Only meaningful when we have an embedder to hash against.
	if cfg.ContextFingerprintEnabled && emb != nil {
		convSvc.SetCatalogLoader(hidden.NewNeo4jLoader(driver))
		slog.Info("conversation context fingerprint wired", "bit_budget", cfg.ContextFingerprintBitBudget)
	}

	// Initialize Context Cooler (Phase 3: Graduation logic for volatile observations).
	// Doesn't require an embedder — operates on graph-side stability scores.
	ctxCooler := conversation.NewContextCooler(driver, cfg)
	lea.SetStabilityReinforcer(ctxCooler)
	slog.Info("context cooler initialized",
		"graduation_threshold", cfg.CoolerGraduationThreshold,
		"decay_rate", cfg.CoolerStabilityDecayRate,
		"constraint_protection", cfg.ConstraintProtectFromDecay)

	// Initialize APE scheduler
	var apeSched *ape.Scheduler
	if pluginMgr != nil {
		modules := pluginMgr.ListModules()
		slog.Info("loaded plugin modules", "count", len(modules))
		for _, m := range modules {
			slog.Info("plugin module loaded", "id", m.ID, "version", m.Version, "state", m.State)
		}

		// Start APE scheduler
		apeSched = ape.NewScheduler(pluginMgr)
		if err := apeSched.Start(); err != nil {
			slog.Warn("APE scheduler failed to start", "error", err)
		}
	}

	// Initialize session tracker (CMS enforcement — Phase 3A)
	sessTracker := conversation.NewSessionTracker(2 * time.Hour)
	slog.Info("session tracker initialized", "ttl", "2h")

	// Phase 3: Initialize circuit breaker registry
	cbCfg := circuitbreaker.Config{
		Enabled:          cfg.CircuitBreakerEnabled,
		FailureThreshold: cfg.CircuitBreakerThreshold,
		SuccessThreshold: 2,
		Timeout:          time.Duration(cfg.CircuitBreakerTimeoutSec) * time.Second,
		MaxConcurrent:    1,
	}
	cbRegistry := circuitbreaker.NewRegistry(cbCfg)
	if cfg.CircuitBreakerEnabled {
		slog.Info("circuit breaker enabled", "threshold", cfg.CircuitBreakerThreshold, "timeout_sec", cfg.CircuitBreakerTimeoutSec)
	}

	// Wire circuit breaker registry to services that make external API calls
	ret.SetCircuitBreakerRegistry(cbRegistry)
	hid.SetCircuitBreakerRegistry(cbRegistry)

	// Wire circuit breaker to the BASE embedder (EMBED-WIRE-001): the old
	// type assertions checked the OUTERMOST value, which under the default
	// config is *CachedEmbedder (EmbeddingCacheEnabled=true) — so the breaker
	// was never wired in any default deployment, silently. Base() walks the
	// wrapper chain (cache, rate-limit, future Unwrap()ers) to the provider.
	if emb != nil {
		switch base := embeddings.Base(emb).(type) {
		case *embeddings.OpenAI:
			base.SetCircuitBreaker(cbRegistry.Get("openai-embeddings"))
			slog.Info("circuit breaker wired to OpenAI embedder")
		case *embeddings.Ollama:
			base.SetCircuitBreaker(cbRegistry.Get("ollama-embeddings"))
			slog.Info("circuit breaker wired to Ollama embedder")
		default:
			slog.Warn("embedding circuit breaker NOT wired — base embedder is neither OpenAI nor Ollama",
				"base_type", fmt.Sprintf("%T", base))
		}

		// Wrap embedder with rate limiting if enabled (Phase 48.4.3)
		if cfg.EmbeddingRateLimitEnabled {
			var rps float64
			var burst int
			if cfg.EmbeddingProvider == "openai" {
				rps = cfg.EmbeddingOpenAIRPS
				burst = cfg.EmbeddingOpenAIBurst
			} else {
				rps = cfg.EmbeddingOllamaRPS
				burst = cfg.EmbeddingOllamaBurst
			}
			emb = embeddings.NewRateLimitedEmbedder(emb, rps, burst, true)
			slog.Info("embedding rate limiting enabled", "rps", rps, "burst", burst)
		}
	}

	// Initialize synthesis engine (Phase 101: SME Synthesis)
	var synth consulting.Synthesizer
	if cfg.SynthesisEnabled {
		synthCfg := consulting.SynthesisConfig{
			Enabled:         true,
			Provider:        cfg.SynthesisProvider,
			Model:           cfg.SynthesisModel,
			MaxTokens:       cfg.SynthesisMaxTokens,
			TimeoutMs:       cfg.SynthesisTimeoutMs,
			OpenAIKey:       cfg.OpenAIAPIKey,
			OpenAIURL:       cfg.EffectiveLLMEndpoint(),
			OllamaURL:       cfg.OllamaEndpoint,
			CompressPrompts: cfg.SynthesisCompress,
		}
		synth = consulting.NewLLMSynthesizer(synthCfg, cbRegistry)
		slog.Info("SME synthesis enabled", "provider", cfg.SynthesisProvider, "model", cfg.SynthesisModel, "max_tokens", cfg.SynthesisMaxTokens)
	}

	// Phase 102: Initialize Intent Translator
	var intentTrans retrieval.IntentTranslator
	if cfg.IntentEnabled {
		intentCfg := retrieval.IntentConfig{
			Enabled:   true,
			Provider:  cfg.IntentProvider,
			Model:     cfg.IntentModel,
			MaxTokens: cfg.IntentMaxTokens,
			TimeoutMs: cfg.IntentTimeoutMs,
			OpenAIKey: cfg.OpenAIAPIKey,
			OpenAIURL: cfg.EffectiveLLMEndpoint(),
			OllamaURL: cfg.OllamaEndpoint,
		}
		intentTrans = retrieval.NewLLMIntentTranslator(intentCfg, cbRegistry)
		slog.Info("intent translation enabled", "provider", cfg.IntentProvider, "model", cfg.IntentModel, "timeout_ms", cfg.IntentTimeoutMs)
	}

	// Wire intent translator to retrieval service for BM25 query rewriting
	if intentTrans != nil {
		ret.SetIntentTranslator(intentTrans)
		slog.Info("intent translator wired to retrieval service for BM25 rewriting")
	}

	// PROD-READINESS: Initialize Query Classifier
	var queryClassifier *retrieval.QueryClassifier
	if cfg.QueryClassifyEnabled {
		qcCfg := retrieval.QueryClassifierConfig{
			Enabled:   true,
			Provider:  cfg.QueryClassifyProvider,
			Model:     cfg.QueryClassifyModel,
			MaxTokens: cfg.QueryClassifyMaxTokens,
			TimeoutMs: cfg.QueryClassifyTimeoutMs,
			OpenAIKey: cfg.OpenAIAPIKey,
			OpenAIURL: cfg.EffectiveLLMEndpoint(),
			OllamaURL: cfg.OllamaEndpoint,
		}
		queryClassifier = retrieval.NewQueryClassifier(qcCfg, cbRegistry)
		slog.Info("query classifier enabled", "provider", cfg.QueryClassifyProvider, "model", cfg.QueryClassifyModel)
	}

	// Wire query classifier to retrieval service
	if queryClassifier != nil {
		ret.SetQueryClassifier(queryClassifier)
		slog.Info("query classifier wired to retrieval service")
	}

	// Phase 104: Initialize Guardrail Validator
	var guardrailVal guardrail.Validator
	if cfg.GuardrailEnabled {
		guardrailCfg := guardrail.GuardrailConfig{
			Enabled:         true,
			Provider:        cfg.GuardrailProvider,
			Model:           cfg.GuardrailModel,
			MaxTokens:       cfg.GuardrailMaxTokens,
			TimeoutMs:       cfg.GuardrailTimeoutMs,
			OpenAIKey:       cfg.OpenAIAPIKey,
			OpenAIURL:       cfg.EffectiveLLMEndpoint(),
			OllamaURL:       cfg.OllamaEndpoint,
			MaxConstraints:  cfg.GuardrailMaxConstraints,
			VectorIndexName: cfg.VectorIndexName,
			// F7: Constraint Scope Filtering
			ConstraintScopeFilteringEnabled: cfg.ConstraintScopeFilteringEnabled,
			// F20: Authority Level Filtering
			ConstraintAuthorityEnabled: cfg.ConstraintAuthorityEnabled,
			ConstraintDefaultAuthority: cfg.ConstraintDefaultAuthority,
			CompressPrompts:            cfg.GuardrailCompress,
			ConstraintSimFloor:         cfg.GuardrailConstraintSimFloor,
			IncludeCorrections:         cfg.GuardrailIncludeCorrections,
		}
		// Sprint FT-LORA-B: build an llmclient for the guardrail service.
		// Pre-bound with the canonical ULTS task name so all interactions are
		// captured in the llm_interactions hypertable with task_name="guardrail.evaluate".
		guardrailBaseURL := cfg.EffectiveLLMEndpoint()
		guardrailAPIKey := cfg.OpenAIAPIKey
		if cfg.GuardrailProvider == "ollama" {
			guardrailBaseURL = cfg.OllamaEndpoint
			guardrailAPIKey = ""
		}
		guardrailLLM := llmclient.New(llmclient.Config{
			Provider:  cfg.GuardrailProvider,
			Model:     cfg.GuardrailModel,
			APIKey:    guardrailAPIKey,
			BaseURL:   guardrailBaseURL,
			TimeoutMs: cfg.GuardrailTimeoutMs,
		}).WithContext(guardrail.TaskName, "")
		guardrailVal = guardrail.NewGuardrailService(guardrailCfg, driver, emb, cbRegistry, guardrailLLM)
		slog.Info("active MCP guardrails enabled", "provider", cfg.GuardrailProvider, "model", cfg.GuardrailModel, "max_constraints", cfg.GuardrailMaxConstraints)
	}

	// Phase 105: Initialize Global Meta-Learning service
	var metaLearnSvc *metalearn.Service
	if cfg.MetaLearnEnabled && emb != nil {
		genCfg := metalearn.GeneralizerConfig{
			Enabled:   true,
			Provider:  cfg.MetaLearnProvider,
			Model:     cfg.MetaLearnModel,
			MaxTokens: cfg.MetaLearnMaxTokens,
			TimeoutMs: cfg.MetaLearnTimeoutMs,
			OpenAIKey: cfg.OpenAIAPIKey,
			OpenAIURL: cfg.EffectiveLLMEndpoint(),
			OllamaURL: cfg.OllamaEndpoint,
		}
		metaLearnSvc = metalearn.NewService(driver, emb, genCfg, cbRegistry, cfg.MetaLearnGlobalSpaceID)
		slog.Info("global meta-learning enabled", "provider", cfg.MetaLearnProvider, "model", cfg.MetaLearnModel, "global_space", cfg.MetaLearnGlobalSpaceID)
	}

	// Initialize consulting service (Agent Consulting API)
	cons := consulting.NewService(cfg, driver, ret, emb, symStore, synth, intentTrans)

	// Phase AR-3: Wire LLM-powered constraint classifier if enabled
	var sharedConstraintClassifier *consulting.ConstraintClassifier
	if cfg.ConsultingLLMConstraintsEnabled {
		sharedConstraintClassifier = consulting.NewConstraintClassifier(consulting.ConstraintClassifierConfig{
			Enabled:   true,
			Provider:  cfg.ConsultingLLMConstraintsProvider,
			Model:     cfg.ConsultingLLMConstraintsModel,
			MaxTokens: 500,
			TimeoutMs: cfg.ConsultingClassifyTimeoutMs,
			OpenAIKey: cfg.OpenAIAPIKey,
			// Phase 11.6: route via EffectiveLLMEndpoint so LLM_ENDPOINT override reaches consulting.classify
			OpenAIURL: cfg.EffectiveLLMEndpoint(),
			OllamaURL: cfg.OllamaEndpoint,
		}, cbRegistry)
		cons.SetConstraintClassifier(sharedConstraintClassifier)
		slog.Info("consulting LLM constraint classification enabled", "provider", cfg.ConsultingLLMConstraintsProvider, "model", cfg.ConsultingLLMConstraintsModel, "timeout_ms", cfg.ConsultingClassifyTimeoutMs)
	}

	// F6a: Wire LLM classifier gate into conversation service if enabled.
	// Reuses the same ConstraintClassifier instance (shared LRU cache + circuit breaker).
	if cfg.ConstraintClassifierGateEnabled && convSvc != nil && sharedConstraintClassifier != nil {
		convSvc.SetConstraintGateClassifier(&constraintGateAdapter{cc: sharedConstraintClassifier})
		slog.Info("F6a: constraint classifier gate enabled for conversation service")
	}
	slog.Info("consulting service initialized")

	// Phase Jiminy: Initialize Jiminy Guidance Service
	var jiminySvc *jiminy.Service
	if cfg.JiminyEnabled {
		jiminySvc = jiminy.NewService(cfg, driver, cons, emb)
		slog.Info("Jiminy guidance enabled", "timeout_ms", cfg.JiminyTimeoutMs, "max_items", cfg.JiminyMaxItems, "min_confidence", cfg.JiminyMinConfidence)

		if cfg.JiminyWarmEnabled {
			slog.Info("Jiminy warm store enabled", "debounce_sec", cfg.JiminyWarmDebounceSec, "max_age_sec", cfg.JiminyWarmMaxAgeSec)
		}

		// J7: Wire retrieval provider for full-spectrum access
		if cfg.JiminyRetrievalEnabled && ret != nil {
			jiminySvc.SetRetriever(&jiminyRetrievalAdapter{retriever: ret})
			slog.Info("Jiminy J7: retrieval pipeline enabled", "top_k", cfg.JiminyRetrievalTopK, "hop_depth", cfg.JiminyRetrievalHopDepth)
		}

		// J8/J15: Wire LLM synthesizer
		if cfg.JiminySynthesisEnabled {
			synCfg := jiminy.SynthesisConfig{
				Enabled:   true,
				Provider:  cfg.JiminySynthesisProvider,
				Model:     cfg.JiminySynthesisModel,
				MaxTokens: cfg.JiminySynthesisMaxTokens,
				TimeoutMs: cfg.JiminySynthesisTimeoutMs,
				OpenAIKey: cfg.OpenAIAPIKey,
				// Phase 11.6: route via EffectiveLLMEndpoint so LLM_ENDPOINT override reaches jiminy.synthesize
				OpenAIURL:       cfg.EffectiveLLMEndpoint(),
				OllamaURL:       cfg.OllamaEndpoint,
				Temperature:     cfg.JiminySynthesisTemperature,
				ContextMaxChars: cfg.JiminyGuidanceContextMaxChars,
				OutputMaxChars:  cfg.JiminyGuidanceOutputMaxChars,
				// JIMINY-ACTIONABILITY-001 Lever B: directive-mode synthesis.
				DirectiveMode:            cfg.JiminyDirectiveSynthesisEnabled,
				DirectiveMaxPromptTokens: cfg.JiminyDirectiveSynthesisMaxPromptTokens,
			}
			jiminySvc.SetSynthesizer(jiminy.NewGuidanceSynthesizer(synCfg, cbRegistry))
			slog.Info("Jiminy J8/J15: LLM synthesis enabled", "provider", cfg.JiminySynthesisProvider, "model", cfg.JiminySynthesisModel, "max_tokens", cfg.JiminySynthesisMaxTokens, "timeout_ms", cfg.JiminySynthesisTimeoutMs)
		}

		// J17-2: Wire constraint code generator
		if cfg.J17CodegenEnabled && convSvc != nil {
			codegenAPIKey := cfg.OpenAIAPIKey
			// Phase 11.6: route via EffectiveLLMEndpoint so LLM_ENDPOINT override reaches jiminy.codegen
			codegenBaseURL := cfg.EffectiveLLMEndpoint()
			if cfg.J17CodegenProvider == "ollama" {
				codegenAPIKey = "ollama"
				codegenBaseURL = cfg.OllamaEndpoint
			}
			codegenLLM := llmclient.New(llmclient.Config{
				Provider: cfg.J17CodegenProvider,
				Model:    cfg.J17CodegenModel,
				APIKey:   codegenAPIKey,
				BaseURL:  codegenBaseURL,
				// Phase 11.6: bumped 10s → 60s for local-LLM latency budget
				TimeoutMs: 60000,
			}).WithContext("jiminy.codegen", "")
			codegen := jiminy.NewConstraintCodeGenerator(codegenLLM)

			// Populate collision set from existing codes in Neo4j
			existingCodes := loadExistingConstraintCodes(context.Background(), driver)
			for _, code := range existingCodes {
				codegen.RegisterExistingCode(code)
			}
			if len(existingCodes) > 0 {
				slog.Info("J17-2: loaded existing constraint codes for collision avoidance", "count", len(existingCodes))
			}

			codegen.SetCircuitBreakerRegistry(cbRegistry)
			jiminySvc.SetCodeGenerator(codegen)
			convSvc.SetCodeGenerator(codegen)
			slog.Info("J17-2: constraint code generator enabled", "provider", cfg.J17CodegenProvider, "model", cfg.J17CodegenModel)
		}

		// J13: Wire LLM evaluator
		if cfg.JiminyEvaluateLLMEnabled {
			// Phase 11.6: route via EffectiveLLMEndpoint so LLM_ENDPOINT override reaches the J13 evaluator.
			evalBaseURL := cfg.EffectiveLLMEndpoint()
			evalAPIKey := cfg.OpenAIAPIKey
			if cfg.JiminyEvaluateLLMProvider == "ollama" {
				evalBaseURL = cfg.OllamaEndpoint
			}
			evalLLM := llmclient.New(llmclient.Config{
				Provider:  cfg.JiminyEvaluateLLMProvider,
				Model:     cfg.JiminyEvaluateLLMModel,
				APIKey:    evalAPIKey,
				BaseURL:   evalBaseURL,
				TimeoutMs: cfg.JiminyEvaluateLLMTimeoutMs,
				// Phase 11.6.x — the J13 evaluator emits eval_prompt.go's evalSystemPrompt
				// (hash caf70a3d...), which the ULTS spec assigns to jiminy.evaluate. Pre-fix
				// this site was tagging rows as jiminy.evaluate_llm; V0014 backfills the ~106
				// historical rows. The config flag stays JIMINY_EVALUATE_LLM_ENABLED for
				// backward compat — the flag name is decoupled from the task_name.
			}).WithContext("jiminy.evaluate", "")
			evaluator := jiminySvc.GetEvaluator()
			if evaluator != nil {
				evaluator.SetLLM(evalLLM, cbRegistry)
				slog.Info("Jiminy J13: evaluator LLM enabled", "provider", cfg.JiminyEvaluateLLMProvider, "model", cfg.JiminyEvaluateLLMModel)
			}
		}

		// G8: Wire circuit breaker to outcome classifier
		if oc := jiminySvc.GetOutcomeClassifier(); oc != nil {
			oc.SetCircuitBreakerRegistry(cbRegistry)
		}
	}

	// F4: Initialize conflict detector if enabled
	var conflictDet *hidden.ConflictDetector
	if cfg.ConstraintConflictDetectionEnabled {
		conflictDet = hidden.NewConflictDetector(driver, cfg)
		slog.Info("constraint conflict detection enabled", "sim_threshold", cfg.ConstraintConflictSimThreshold, "max_pairs", cfg.ConstraintConflictMaxPairs)
	}

	// Phase 3: Initialize metrics registry
	// Start with defaults (includes histogram buckets) and override specific fields
	metricsCfg := metrics.DefaultConfig()
	metricsCfg.Enabled = cfg.MetricsEnabled
	if cfg.MetricsNamespace != "" {
		metricsCfg.Namespace = cfg.MetricsNamespace
	}
	metricsRegistry := metrics.NewRegistry(metricsCfg)
	metrics.SetGlobalRegistry(metricsRegistry)
	if cfg.MetricsEnabled {
		metrics.InitStandardMetrics()
		slog.Info("Prometheus metrics enabled", "namespace", cfg.MetricsNamespace)
	}

	// Phase 48.4.4: Initialize memory pressure monitor
	memPressure := backpressure.NewMemoryPressure(uint64(cfg.MemoryPressureThresholdMB), cfg.MemoryPressureEnabled)
	if cfg.MemoryPressureEnabled {
		slog.Info("memory pressure monitoring enabled", "threshold_mb", cfg.MemoryPressureThresholdMB)
	}

	// Phase 51: Initialize Web Scraper service
	var scraperSvc *scraper.Service
	if cfg.ScraperEnabled {
		scraperCfg := scraper.Config{
			Enabled:            cfg.ScraperEnabled,
			DefaultSpaceID:     cfg.ScraperDefaultSpaceID,
			MaxConcurrentJobs:  cfg.ScraperMaxConcurrentJobs,
			DefaultDelayMs:     cfg.ScraperDefaultDelayMs,
			DefaultTimeoutMs:   cfg.ScraperDefaultTimeoutMs,
			CacheTTLSeconds:    cfg.ScraperCacheTTLSeconds,
			RespectRobotsTxt:   cfg.ScraperRespectRobotsTxt,
			MaxContentLengthKB: cfg.ScraperMaxContentLengthKB,
		}
		scraperSvc = scraper.NewService(scraperCfg, driver, emb, pluginMgr)
		if convSvc != nil {
			scraperSvc.SetConversationService(&scraperConvAdapter{svc: convSvc})
		}
		slog.Info("web scraper enabled", "space", cfg.ScraperDefaultSpaceID, "max_jobs", cfg.ScraperMaxConcurrentJobs)
	}

	// Phase 70: Initialize Backup service
	var backupSvc *backup.Service
	var backupSched *backup.Scheduler
	if cfg.BackupEnabled {
		backupCfg := backup.Config{
			Enabled:                cfg.BackupEnabled,
			StorageDir:             cfg.BackupStorageDir,
			FullCmd:                cfg.BackupFullCmd,
			Neo4jContainer:         cfg.BackupNeo4jContainer,
			FullIntervalHours:      cfg.BackupFullIntervalHours,
			PartialIntervalHours:   cfg.BackupPartialIntervalHours,
			RetentionFullCount:     cfg.BackupRetentionFullCount,
			RetentionPartialCount:  cfg.BackupRetentionPartialCount,
			RetentionMaxAgeDays:    cfg.BackupRetentionMaxAgeDays,
			RetentionMaxStorageGB:  cfg.BackupRetentionMaxStorageGB,
			RetentionRunAfter:      cfg.BackupRetentionRunAfter,
			SnapshotWaitTimeoutSec: cfg.BackupSnapshotWaitTimeoutSec,
			InitialBackupDelayMin:  cfg.BackupInitialDelayMin,
		}
		exp := transfer.NewExporter(driver)
		backupSvc = backup.NewService(backupCfg, driver, exp)
		backupSched = backup.NewScheduler(backupSvc)
		// SUPERVISOR-002: started via StartSupervisedBackground (serve.go)
		// so the loop runs under the goroutine supervisor.
		slog.Info("backup enabled", "storage_dir", backupCfg.StorageDir, "full_interval_hours", backupCfg.FullIntervalHours, "partial_interval_hours", backupCfg.PartialIntervalHours)
	}

	// Initialize TSDB Backup service
	var tsdbBackupSvc *tsdb.TSDBBackupService
	var tsdbBackupSched *tsdb.TSDBBackupScheduler
	if cfg.TSDBBackupEnabled {
		tsdbBackupCfg := tsdb.TSDBBackupConfig{
			Enabled:             cfg.TSDBBackupEnabled,
			StorageDir:          cfg.TSDBBackupStorageDir,
			ComposeFile:         cfg.TSDBBackupComposeFile,
			ServiceName:         cfg.TSDBBackupServiceName,
			Database:            cfg.TSDBDatabase,
			User:                cfg.TSDBUser,
			IntervalHours:       cfg.TSDBBackupIntervalHours,
			InitialDelayMin:     cfg.TSDBBackupInitialDelayMin,
			RetentionCount:      cfg.TSDBBackupRetentionCount,
			RetentionMaxAgeDays: cfg.TSDBBackupRetentionMaxAgeDays,
		}
		tsdbBackupSvc = tsdb.NewTSDBBackupService(tsdbBackupCfg)
		tsdbBackupSched = tsdb.NewTSDBBackupScheduler(tsdbBackupSvc)
		// SUPERVISOR-002: started via StartSupervisedBackground (serve.go)
		slog.Info("tsdb backup enabled", "storage_dir", tsdbBackupCfg.StorageDir, "interval_hours", tsdbBackupCfg.IntervalHours)
	}

	// Phase 60b: Initialize RSIC components
	var rsicCycle *ape.CycleOrchestrator
	var rsicWatchdog *ape.Watchdog

	// Create adapters for RSIC interfaces
	learnerAdapter := &rsicLearningAdapter{svc: lea}
	var convAdapter *rsicConvAdapter
	if ctxCooler != nil {
		convAdapter = &rsicConvAdapter{cooler: ctxCooler}
	}
	hiddenAdapter := &rsicHiddenAdapter{svc: hid}

	rsicAssessor := ape.NewAssessor(cfg, driver, learnerAdapter, convAdapter)
	// J10: Wire Jiminy stats provider to RSIC assessor
	if jiminySvc != nil {
		rsicAssessor.SetJiminyProvider(&rsicJiminyAdapter{svc: jiminySvc})
	}
	// J17: Wire protocol stats provider to RSIC assessor
	if jiminySvc != nil && cfg.J17MetricsEnabled {
		protoAdapter := &rsicProtocolAdapter{svc: jiminySvc}
		rsicAssessor.SetProtocolProvider(protoAdapter)
	}
	// Sidecar health: Wire checker to assessor for RSIC sidecar awareness
	if cfg.J17SidecarURL != "" {
		rsicAssessor.SetSidecarChecker(func(_ context.Context) bool {
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Get(cfg.J17SidecarURL + "/health")
			if err != nil {
				return false
			}
			resp.Body.Close()
			return resp.StatusCode == http.StatusOK
		})
	}
	// Synergy: Wire file reader for RSIC synergy health assessment
	if cfg.SynergyAssessmentEnabled {
		jiminyCheck := func() bool {
			return cfg.JiminyEnabled && jiminySvc != nil
		}
		rsicAssessor.SetSynergyReader(ape.NewFileSynergyReader(
			cfg.SynergyClaudeMDPath, cfg.SynergyMemoryMDPath, jiminyCheck,
			cfg.SynergyMemoryLineThreshold, cfg.SynergyOverlapSampleSize,
		))
		slog.Info("rsic: synergy reader wired")
	}
	rsicReflector := ape.NewReflector(cfg, driver)
	// J17: Wire protocol stats provider to reflector
	if jiminySvc != nil && cfg.J17MetricsEnabled {
		rsicReflector.SetProtocolProvider(&rsicProtocolAdapter{svc: jiminySvc})
	}

	rsicPlanner := ape.NewPlanner(cfg)
	rsicDispatcher := ape.NewDispatcher(driver, learnerAdapter, convAdapter, hiddenAdapter)
	// ENFORCE-AUTO-EXECUTE (2026-08-03): wire config so the enforcement
	// auto-archive executor can consult its guard flags at dispatch time.
	rsicDispatcher.SetConfig(cfg)
	// J17: Wire protocol evolver to dispatcher
	if jiminySvc != nil && cfg.J17MetricsEnabled {
		evolver := jiminySvc.NewProtocolEvolver()
		rsicDispatcher.SetProtocolEvolver(evolver)
	}
	// Phase 47.2: Wire freshness provider for RSIC ingest staleness detection
	var freshnessAdapter *rsicFreshnessAdapter
	if cfg.APEIngestSyncEnabled {
		freshnessAdapter = &rsicFreshnessAdapter{retriever: ret}
		rsicAssessor.SetFreshnessProvider(freshnessAdapter)
		rsicDispatcher.SetFreshnessProvider(freshnessAdapter)
	}
	rsicMonitor := ape.NewMonitor(rsicDispatcher)
	rsicCalibrator := ape.NewCalibrator(convAdapter, cfg.RSICMaxHistoryEntries)
	rsicPlanner.SetCalibrator(rsicCalibrator)

	// Phase AR-3: Wire LLM-powered reflector if enabled
	if cfg.RSICLLMReflectEnabled {
		llmReflector := ape.NewLLMReflector(ape.LLMReflectorConfig{
			Enabled:   true,
			Provider:  cfg.RSICLLMReflectProvider,
			Model:     cfg.RSICLLMReflectModel,
			MaxTokens: cfg.EmergenceMaxTokens,
			TimeoutMs: cfg.RSICLLMReflectTimeoutMs,
			OpenAIKey: cfg.OpenAIAPIKey,
			// Phase 11.6: route via EffectiveLLMEndpoint so LLM_ENDPOINT override reaches ape.reflect
			OpenAIURL:       cfg.EffectiveLLMEndpoint(),
			OllamaURL:       cfg.OllamaEndpoint,
			CompressPrompts: cfg.RSICLLMReflectCompress,
			// APE-PROMPT-BUDGET-001
			PromptBudgetTokens: cfg.RSICLLMReflectPromptBudgetTokens,
			HistoryCycles:      cfg.RSICLLMReflectHistoryCycles,
			IncludeDatasets:    cfg.RSICLLMReflectIncludeDatasets,
		}, cbRegistry, rsicCalibrator)
		rsicReflector.SetLLMReflector(llmReflector)
		slog.Info("RSIC LLM reflection enabled", "provider", cfg.RSICLLMReflectProvider, "model", cfg.RSICLLMReflectModel, "timeout_ms", cfg.RSICLLMReflectTimeoutMs)
	}

	// Watchdog and cycle orchestrator — closure captures rsicCycle variable (assigned below)
	rsicWatchdog = ape.NewWatchdog(cfg, cfg.RSICWatchdogSpaceID, func(ctx context.Context, spaceID string, meta ape.TriggerMetadata) {
		opts := &ape.RunCycleOpts{TriggerMeta: &meta}
		if _, err := rsicCycle.RunCycle(ctx, spaceID, ape.TierMeso, opts); err != nil {
			slog.Warn("RSIC watchdog meso cycle failed", "error", err)
		}
		if cfg.ConsolidateOnWatchdogEnabled && hid != nil {
			if _, err := hid.RunConsolidation(ctx, spaceID); err != nil {
				slog.Error("RSIC watchdog consolidation failed", "error", err)
			} else {
				slog.Info("RSIC watchdog: consolidation triggered alongside meso cycle")
			}
		}
		// Cleanup stale frozen-space entries
		if removed := lea.CleanupStaleFreezes(map[string]bool{spaceID: true}); removed > 0 {
			slog.Info("RSIC watchdog: cleaned up stale frozen-space entries", "removed", removed)
		}
	})
	rsicCycle = ape.NewCycleOrchestrator(cfg, rsicAssessor, rsicReflector, rsicPlanner, rsicDispatcher, rsicMonitor, rsicCalibrator, rsicWatchdog)
	slog.Info("RSIC initialized", "watchdog", cfg.RSICWatchdogEnabled, "micro", cfg.RSICMicroEnabled)

	// Phase 80: Wire WatchdogSignalProvider for multi-dimensional monitoring
	rsicWatchdog.SetSignalProvider(&rsicWatchdogSignalAdapter{
		sessionTracker: sessTracker,
		driver:         driver,
		jiminyEnabled:  cfg.JiminyEnabled,
		jiminySvc:      jiminySvc,
		sidecarURL:     cfg.J17SidecarURL,
	})

	// Phase 87: Create orchestration policy
	orchPolicy := ape.NewOrchestrationPolicy(cfg)
	rsicCycle.SetOrchestrationPolicy(orchPolicy)
	slog.Info("RSIC orchestration policy initialized", "cooldown_sec", cfg.RSICTriggerCooldownSec, "dedupe_sec", cfg.RSICTriggerDedupeSec)

	// Phase 14.2 Epic 2 catalog wiring moved below the Server literal
	// (TSDB-CONSUME-001): the V0020 VersionRecorder closure captures s so it
	// can read s.tsdbClient lazily at build time (catalog builds run long
	// after SetTSDBClient).

	// Phase 88: Create safety validator and snapshot store, wire to dispatcher
	safetyValidator := ape.NewSafetyValidator(driver)
	snapshotStore := ape.NewSnapshotStore(driver, cfg.RSICRollbackWindow)
	rsicDispatcher.SetSafetyValidator(safetyValidator)
	rsicDispatcher.SetSnapshotStore(snapshotStore)
	// COOLER-001: unify RSIC graduate_volatile onto the config-driven Context
	// Cooler, and align the rollback snapshot predicate with its threshold.
	if ctxCooler != nil {
		rsicDispatcher.SetGraduationProcessor(&coolerGraduationAdapter{cooler: ctxCooler})
		snapshotStore.SetGraduationThreshold(cfg.CoolerGraduationThreshold)
	}
	// RSIC-SK1: Wire guidance calibrator for self-calibrating guidance.
	// CONFIG-DEADFLAG-001: gate on RSIC_GUIDANCE_CALIBRATION_ENABLED —
	// parsed since RSIC-SK1 but the wiring ignored it (default true, so
	// zero-config behavior is unchanged; operators can now disable).
	if jiminySvc != nil && cfg.RSICGuidanceCalibrationEnabled {
		rsicDispatcher.SetGuidanceCalibrator(&rsicGuidanceCalibrationAdapter{svc: jiminySvc})
	}
	// CONFIG-DEADFLAG-001: RSIC_GUIDANCE_* thresholds (were literals 3/0.7/0.1/5).
	rsicDispatcher.SetGuidanceCalibrationThresholds(cfg.RSICGuidanceMinSurfaces, cfg.RSICGuidanceBoostThreshold, cfg.RSICGuidanceDecayThreshold, cfg.RSICGuidanceDecayMinSurfaces)
	rsicCycle.SetSnapshotStore(snapshotStore)
	// NLI feedback loop: wire tier effectiveness dataset builder
	if jiminySvc != nil {
		rsicCycle.SetTierEffectivenessProvider(&rsicTierEffectivenessAdapter{svc: jiminySvc})
	}
	slog.Info("RSIC safety enforcement initialized", "rollback_window_sec", cfg.RSICRollbackWindow)

	// Phase 89: Initialize RSIC persistence store
	var rsicStore *ape.RSICStore
	if cfg.RSICPersistenceEnabled {
		rsicStore = ape.NewRSICStore(driver)
		// CONFIG-DEADFLAG-001: RSIC_CALIBRATION_DAYS → RSICState retention.
		rsicStore.SetCalibrationRetentionDays(cfg.RSICCalibrationDays)
		// SUPERVISOR-002: flush loop started via StartSupervisedBackground

		// Wire store to components
		rsicCalibrator.SetStore(rsicStore)
		rsicWatchdog.SetStore(rsicStore)
		orchPolicy.SetStore(rsicStore)

		// Hydrate from persisted state
		if err := rsicCalibrator.Hydrate(cfg.RSICWatchdogSpaceID); err != nil {
			slog.Warn("RSIC calibration hydration failed", "error", err)
		}
		if ws, err := rsicStore.LoadWatchdogState(cfg.RSICWatchdogSpaceID); err == nil && ws != nil {
			rsicWatchdog.Hydrate(ws)
		} else if err != nil {
			slog.Warn("RSIC watchdog hydration failed", "error", err)
		}
		if triggers, counters, err := rsicStore.LoadOrchestrationState(); err == nil {
			orchPolicy.Hydrate(triggers, counters)
		} else {
			slog.Warn("RSIC orchestration hydration failed", "error", err)
		}

		slog.Info("RSIC persistence initialized", "flush_interval", "30s")
	} else {
		slog.Info("RSIC persistence disabled")
	}

	// Phase 38: Initialize UNTS Hash Verification
	var untsReg *unts.Registry
	var untsScan *unts.Scanner
	if cfg.UNTSEnabled {
		untsReg = unts.NewRegistry(cfg.UNTSBasePath)
		if err := untsReg.Load(); err != nil {
			slog.Warn("UNTS registry load failed", "error", err)
		}
		untsScan = unts.NewScanner(untsReg, cfg.UNTSBasePath)
		slog.Info("UNTS hash verification enabled", "base_path", cfg.UNTSBasePath)
	}

	// Phase 80: Initialize signal learner with Neo4j persistence
	signalLearner := ape.NewSignalLearner(cfg.MetaCogSignalDecayRate, cfg.MetaCogSignalBoostRate)
	signalLearner.SetDriver(driver)
	slog.Info("signal learner initialized", "decay", cfg.MetaCogSignalDecayRate, "boost", cfg.MetaCogSignalBoostRate)
	// RSIC-SK1: Wire signal learner to Jiminy for guidance emission/response tracking
	if jiminySvc != nil {
		jiminySvc.SetSignalLearner(signalLearner)
		// NEGFEED-001 Bridge A: wire the anti-Hebbian weaken path so a
		// contradicted guidance outcome can weaken its source co-activations.
		if lea != nil {
			jiminySvc.SetNegativeFeedbackApplier(&negativeFeedbackAdapter{learner: lea})
		}
	}

	// SR-001: Alert dispatcher
	alertDisp := alert.NewDispatcher(alert.Config{
		Enabled:           cfg.AlertEnabled,
		CooldownSec:       cfg.AlertCooldownSec,
		AlertFilePath:     cfg.AlertFilePath,
		MacOSNotify:       cfg.AlertMacOSNotify,
		MacOSNotifyMinSev: alert.Severity(cfg.AlertMacOSNotifyMinSev),
		MaxAlerts:         cfg.AlertMaxEntries,
	})

	// Wire CB state change → alert dispatcher
	if cfg.AlertEnabled {
		cbRegistry.SetOnStateChange(func(name string, from, to circuitbreaker.State) {
			if to == circuitbreaker.StateOpen {
				alertDisp.SendAlert(context.Background(), "circuit-breaker",
					"Circuit Breaker Opened: "+name,
					fmt.Sprintf("Circuit breaker %q transitioned %s → %s", name, from, to),
					alert.SeverityHigh)
			} else if from == circuitbreaker.StateOpen && to == circuitbreaker.StateClosed {
				alertDisp.SendAlert(context.Background(), "circuit-breaker",
					"Circuit Breaker Recovered: "+name,
					fmt.Sprintf("Circuit breaker %q recovered: %s → %s", name, from, to),
					alert.SeverityLow)
			}
		})
	}

	// Wire RSIC dispatcher → alert dispatcher
	rsicDispatcher.SetAlertDispatcher(&rsicAlertAdapter{dispatcher: alertDisp})

	embedderForFP := embeddings.NilSafe(emb)
	var fpCache *contextFingerprintCache
	if cfg.ContextFingerprintEnabled && emb != nil {
		fpCache = newContextFingerprintCache(embedderForFP)
	}
	s := &Server{
		cfg:                     cfg,
		driver:                  driver,
		retriever:               ret,
		learner:                 lea,
		embedder:                embedderForFP,
		contextFPCache:          fpCache,
		anomalyDetector:         anom,
		hiddenLayer:             hid,
		hiddenSvc:               hid,
		pluginMgr:               pluginMgr,
		apeScheduler:            apeSched,
		symbolStore:             symStore,
		symbolParser:            symParser,
		symbolResolver:          symResolver,
		consultant:              cons,
		gapDetector:             gapDet,
		gapInterviewer:          gapInt,
		conversationSvc:         convSvc,
		contextCooler:           ctxCooler,
		sessionTracker:          sessTracker,
		webhookDebouncer:        newLinearWebhookDebouncer(),
		genericWebhookDebouncer: newWebhookDebouncer(),
		fileWatcherMgr:          filewatcher.NewManager(),
		cbRegistry:              cbRegistry,
		metricsRegistry:         metricsRegistry,
		metricsRecorder:         metrics.NewMetricsRecorder(metricsRegistry, nil, cfg.RSICWatchdogSpaceID), // instanceID set below
		memoryPressure:          memPressure,
		templateService:         conversation.NewTemplateService(driver),
		snapshotService:         conversation.NewSnapshotService(driver),
		orgReviewService:        conversation.NewOrgReviewService(driver),
		rsicCycle:               rsicCycle,
		rsicWatchdog:            rsicWatchdog,
		orchestrationPolicy:     orchPolicy,
		snapshotStore:           snapshotStore,
		rsicStore:               rsicStore,
		scraperSvc:              scraperSvc,
		backupSvc:               backupSvc,
		backupScheduler:         backupSched,
		tsdbBackupSvc:           tsdbBackupSvc,
		tsdbBackupScheduler:     tsdbBackupSched,
		intentTranslator:        intentTrans,
		guardrailValidator:      guardrailVal,
		guardrailProducerSem:    make(chan struct{}, max(1, cfg.GuardrailProducerMaxConcurrent)),
		jiminySvc:               jiminySvc,
		warmStore:               jiminy.NewWarmStoreWithPersistence(cfg.JiminyWarmPersistDir),
		metaLearnSvc:            metaLearnSvc,
		signalLearner:           signalLearner,
		untsRegistry:            untsReg,
		untsScanner:             untsScan,
		eventDispatcher:         plugins.NewEventDispatcher(pluginMgr),
		enforcementLog:          newEnforcementEventLog(1000),
		conflictDetector:        conflictDet,
		alertDispatcher:         alertDisp,
	}

	// Phase 14.2 Epic 2: Wire ContextCatalog Builder + Loader for the
	// stage-6 fingerprint refresh hook. Disabling refresh via
	// CONTEXT_FINGERPRINT_REFRESH_ENABLED=false bypasses the hook regardless.
	// TSDB-CONSUME-001: each successful build also records a V0020
	// context_catalog_versions row (the table had zero writes ever); the
	// closure reads s.tsdbClient at build time — nil-safe before/without
	// SetTSDBClient.
	if cfg.ContextFingerprintEnabled {
		opts := hidden.BuilderOptsFromConfig(cfg)
		opts.VersionRecorder = func(ctx context.Context, rec hidden.CatalogVersionRecord) {
			row := tsdb.ContextCatalogVersionRow{
				SpaceID:           rec.SpaceID,
				Version:           rec.Version,
				TotalBits:         rec.TotalBits,
				AllocationJSON:    rec.AllocationJSON,
				TopSymbols:        rec.TopSymbols,
				TopPaths:          rec.TopPaths,
				TopTags:           rec.TopTags,
				BitsRoleTypeLayer: rec.BitsRoleTypeLayer,
				BitsTag:           rec.BitsTag,
				BitsPath:          rec.BitsPath,
				BitsSymbol:        rec.BitsSymbol,
			}
			if err := tsdb.RecordContextCatalogVersion(ctx, s.tsdbClient, row); err != nil {
				slog.Warn("context catalog: V0020 version record failed",
					"space_id", rec.SpaceID, "version", rec.Version, "error", err)
			}
		}
		catalogBuilder := hidden.NewNeo4jBuilder(driver, opts)
		catalogLoader := hidden.NewNeo4jLoader(driver)
		rsicCycle.SetContextCatalog(catalogBuilder, catalogLoader)
		rsicCycle.SetFingerprintDriver(driver)
		slog.Info("RSIC context catalog wired",
			"refresh_enabled", cfg.ContextFingerprintRefreshEnabled,
			"interval_hours", cfg.ContextFingerprintRefreshIntervalHours,
			"timeout_ms", cfg.ContextFingerprintRefreshTimeoutMs,
			"bit_budget", cfg.ContextFingerprintBitBudget,
		)
	}

	// Set instance ID for metric labels (e.g. "localhost:9999")
	if cfg.ListenAddr != "" {
		addr := cfg.ListenAddr
		if addr[0] == ':' {
			addr = "localhost" + addr
		}
		s.metricsRecorder.SetInstanceID(addr)
	}

	// TSDB Sprint: Create LiveCollectors for real-time Prometheus gauges
	if cfg.LiveMetricsEnabled {
		var jiminyProv ape.JiminyStatsProvider
		var protoProv ape.ProtocolStatsProvider
		if jiminySvc != nil {
			jiminyProv = &rsicJiminyAdapter{svc: jiminySvc}
		}
		if jiminySvc != nil && cfg.J17MetricsEnabled {
			protoProv = &rsicProtocolAdapter{svc: jiminySvc}
		}
		s.liveCollectors = ape.NewLiveCollectors(
			jiminyProv,
			protoProv,
			rsicAssessor,
			cfg.RSICWatchdogSpaceID,
			time.Duration(cfg.LiveGuidanceRefreshSec)*time.Second,
		)
		// Wire Assess() → LiveCollectors report cache bridge
		rsicAssessor.SetReportCallback(s.liveCollectors.StoreReport)
		slog.Info("live_collectors: enabled", "guidance_refresh_sec", cfg.LiveGuidanceRefreshSec)

		// Bootstrap: run initial assessment so health gauges are non-zero immediately
		go func() {
			time.Sleep(10 * time.Second)
			if rsicAssessor != nil {
				slog.Info("rsic: running bootstrap assessment")
				report, err := rsicAssessor.Assess(context.Background(), cfg.RSICWatchdogSpaceID, ape.TierMeso)
				if err != nil {
					slog.Warn("rsic: bootstrap assessment failed", "error", err)
					return
				}
				slog.Info("rsic: bootstrap assessment complete",
					"overall_health", report.OverallHealth,
					"protocol_health", report.ProtocolHealth,
					"guidance_health", report.GuidanceHealth,
				)
			}
		}()
	}

	// Phase 47.2: Set trigger callback now that Server is constructed
	if freshnessAdapter != nil {
		freshnessAdapter.triggerFn = s.runScheduledSyncCheck
	}

	// B7: Wire warm store into jiminy service for trust-based invalidation
	if jiminySvc != nil {
		jiminySvc.SetWarmStore(s.warmStore)
	}

	// LEVER-C-TIGHTEN-001: record live Lever C tuning at boot for operator visibility.
	if jiminySvc != nil && cfg.JiminyGuidanceConstraintBiasEnabled {
		slog.Info("jiminy: lever c actionable bias",
			"enabled", cfg.JiminyGuidanceConstraintBiasEnabled,
			"topk", cfg.JiminyGuidanceConstraintIncludeTopK,
			"sim_floor", cfg.JiminyGuidanceConstraintSimFloor,
		)
	}

	// HEBB-ETA-001: record precision-weighted η state at boot.
	slog.Info("hebb: precision-weighted eta",
		"enabled", cfg.PrecisionWeightedEtaEnabled,
		"confidence_alpha", cfg.ConfidenceAlpha,
		"confidence_beta", cfg.ConfidenceBeta,
		"confidence_gamma", cfg.ConfidenceGamma,
		"half_life_sec", cfg.ConfidenceHalfLifeSec,
	)

	// J17: Hydrate trust scores from Neo4j and start persistence flush loop
	if jiminySvc != nil {
		if err := jiminySvc.HydrateTrust(context.Background()); err != nil {
			slog.Warn("j17: trust hydration failed", "error", err)
		}
		if err := jiminySvc.HydrateEscalation(context.Background()); err != nil {
			slog.Warn("j12: escalation hydration failed", "error", err)
		}
		jiminySvc.StartTrustPersistence(context.Background())
	}

	// Phase 80: Hydrate signal learner from Neo4j; persistence loop starts
	// via StartSupervisedBackground (SUPERVISOR-002).
	if err := signalLearner.HydrateSignals(context.Background()); err != nil {
		slog.Warn("signal learner: hydration failed", "error", err)
	}

	// B3: Bootstrap codification — codify constraints without codes on startup
	if cfg.J17BootstrapCodification && jiminySvc != nil {
		go func() {
			// Constraint bootstrap gets its own ctx (was previously shared —
			// CORRECTION-CODE-GEN-001 live-caught: sharing a 60s budget with
			// the correction bootstrap starved the second phase mid-run when
			// the first phase consumed most of it).
			bctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			n, err := jiminySvc.BootstrapCodes(bctx, cfg.J17BootstrapSpaceID)
			if err != nil {
				slog.Warn("jiminy: bootstrap codification failed", "error", err)
			} else if n > 0 {
				slog.Info("jiminy: bootstrap codification complete", "codified", n)
			}
			// CORRECTION-CODE-GEN-001: same shape for role_type='correction' nodes.
			// Correction nodes didn't carry constraint_code before this sprint;
			// mdemg-dev had 35 uncoded corrections at ship. Bootstrap runs on
			// every startup — idempotent (skips nodes that already have codes).
			// Independent 120s context so its LLM budget isn't drained by
			// however long the constraint bootstrap took.
			cctx, ccancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer ccancel()
			nCorr, corrErr := jiminySvc.BootstrapCorrectionCodes(cctx, cfg.J17BootstrapSpaceID)
			if corrErr != nil {
				slog.Warn("jiminy: bootstrap correction codification failed", "error", corrErr)
			} else if nCorr > 0 {
				slog.Info("jiminy: bootstrap correction codification complete", "codified", nCorr)
				// Clear orphan metrics from pre-bootstrap window
				if collector := jiminySvc.GetProtocolMetricsCollector(); collector != nil {
					collector.Reset()
				}
			}
		}()
	}

	return s
}

// SetTSDBClient attaches an optional TimescaleDB client and creates a MetricWriter.
// Called from serve.go when TSDB is enabled and connected.
func (s *Server) SetTSDBClient(client *tsdb.Client) {
	s.tsdbClient = client
	if client != nil {
		// NOSILENT-001: wire the backup scheduler's outcome hook now that both
		// the TSDB pool and the alert dispatcher exist. A failed (or never-run)
		// scheduled backup now records a scheduled_job_events row + fires a
		// high-severity alert instead of a silent slog.Warn.
		if s.tsdbBackupScheduler != nil {
			pool := client.Pool()
			disp := s.alertDispatcher
			instanceID := s.cfg.InstanceID
			s.tsdbBackupScheduler.SetResultHook(func(success bool, latencyMS int64, runErr error) {
				ev := tsdb.JobEventRow{
					JobName: "tsdb-backup", InstanceID: instanceID,
					Success: success, LatencyMS: latencyMS,
				}
				if runErr != nil {
					ev.ErrorMessage = runErr.Error()
				}
				jobhealth.Report(context.Background(), pool, disp, ev)
			})
		}
		// BACKUP-RESTORE-VERIFY-001: the default-ON Neo4j backup scheduler was
		// the unmonitored one (inverted NOSILENT coverage) — wire the same
		// jobhealth hook with its own job_name.
		if s.backupScheduler != nil {
			pool := client.Pool()
			disp := s.alertDispatcher
			instanceID := s.cfg.InstanceID
			s.backupScheduler.SetResultHook(func(success bool, latencyMS int64, runErr error) {
				ev := tsdb.JobEventRow{
					JobName: "neo4j-backup", InstanceID: instanceID,
					Success: success, LatencyMS: latencyMS,
				}
				if runErr != nil {
					ev.ErrorMessage = runErr.Error()
				}
				jobhealth.Report(context.Background(), pool, disp, ev)
			})
		}
		// Phase 12 Epic 6: construct ConflictTracker once and inject into the
		// three Services that have hook sites. Per-space rate limiter defaults
		// to 1 row/space/minute (the value bound inside conversation/conflict_tracker.go);
		// emergency disable via CONFLICT_TRACKER_ENABLED=false stops Track() at
		// the call sites without requiring the tracker itself to be reconstructed.
		conflictTracker := conversation.NewConflictTracker(client.Pool(), 0)
		if s.jiminySvc != nil {
			s.jiminySvc.SetConflictTracker(conflictTracker)
			// ENFORCE-OVERRIDES-TSDB (2026-08-03): wire the override manager to
			// the constraint_overrides hypertable so RSIC + UI can query history
			// alongside outcomes. JSONL audit persists as forensic + portable.
			if om := s.jiminySvc.GetOverrides(); om != nil {
				om.SetTSDB(client.Pool(), s.cfg.RSICWatchdogSpaceID)
			}
		}
		if s.consultant != nil {
			s.consultant.SetConflictTracker(conflictTracker)
		}
		if s.rsicCycle != nil {
			s.rsicCycle.SetConflictTracker(conflictTracker)
		}
		slog.Info("conflict_tracker: production hooks wired",
			"enabled", s.cfg.ConflictTrackerEnabled,
			"sites", "jiminy.Guide+consulting.Suggest+ape.cycle.Reflect")
	}
	if client != nil {
		s.tsdbWriter = tsdb.NewMetricWriter(
			client,
			s.cfg.RSICWatchdogSpaceID,
			time.Duration(s.cfg.TSDBFlushIntervalSec)*time.Second,
		)
		// Wire writer into MetricsRecorder for periodic flushing
		if s.metricsRecorder != nil {
			s.metricsRecorder.SetWriter(s.tsdbWriter)
			// Pre-flush hook: refresh ALL gauge values before each TSDB write
			// so dashboards show current data for all panels.
			{
				lc := s.liveCollectors
				srv := s // capture server reference for infrastructure metrics
				s.metricsRecorder.SetPreFlushHook(func() {
					m := metrics.Metrics()

					// Live collectors: protocol, guidance, RSIC health
					if lc != nil {
						lc.CollectProtocolMetrics()
						lc.CollectGuidanceMetrics()
						lc.CollectHealthMetrics()
					}

					// Infrastructure: circuit breakers
					if srv.cbRegistry != nil {
						_ = srv.cbRegistry.Get("openai-embeddings")
						_ = srv.cbRegistry.Get("openai-rerank")
						_ = srv.cbRegistry.Get("ollama-rerank")
						_ = srv.cbRegistry.Get("ollama-embeddings")
						m.CollectCircuitBreakerMetrics(srv.cbRegistry)
					}

					// Infrastructure: cache hit ratios
					if srv.retriever != nil {
						cacheStats := map[string]map[string]any{
							"query":     srv.retriever.QueryCacheStats(),
							"embedding": srv.retriever.EmbeddingCacheStats(),
						}
						m.CollectCacheMetrics(cacheStats)
					}

					// Infrastructure: TSDB pgx pool (real stats — TSDB-CONSUME-001)
					// + rate-limit rejection deltas
					if srv.tsdbClient != nil && srv.tsdbClient.Pool() != nil {
						st := srv.tsdbClient.Pool().Stat()
						m.CollectTSDBPoolMetrics(int64(st.TotalConns()), int64(st.IdleConns()),
							int64(st.AcquiredConns()), int64(st.MaxConns()), st.EmptyAcquireCount())
					}
					m.CollectRateLimitMetrics()
					m.CollectTSDBWriterStats(tsdb.AllWriterStats())

					// Infrastructure: Neo4j graph, container
					graphData := srv.collectNeo4jGraphData()
					m.CollectNeo4jGraphMetrics(graphData)
					containerStats := srv.collectNeo4jContainerStats()
					m.CollectNeo4jContainerMetrics(containerStats)

					// Infrastructure: memory pressure
					if srv.memoryPressure != nil {
						m.CollectMemoryMetrics(srv.memoryPressure.HeapUsageMB()*1024*1024, srv.memoryPressure.RejectedCount())
					}
				})
			}
			s.metricsRecorder.Start(time.Duration(s.cfg.TSDBFlushIntervalSec) * time.Second)
		}
		// Wire TSDB client into RSIC reflector for schema drift detection
		if s.rsicCycle != nil {
			s.rsicCycle.SetTSDBClient(client)
			// Wire TSDB dataset builder for RSIC data-driven reflection.
			// AMD-7: drive the readiness threshold + per-task overrides from
			// config instead of the hardcoded default.
			datasetBuilder := tsdb.NewDatasetBuilder(client.Pool()).
				SetReadinessThresholds(s.cfg.TrainingReadinessThreshold, s.cfg.TrainingReadinessThresholdOverrides)
			s.rsicCycle.SetDatasetProvider(datasetBuilder)
			// FT-RECURSIVE-002: wire the trigger gate (SF-2) — it needs the
			// TSDB pool (available here) to read the ft_training_cycles ledger.
			s.rsicCycle.SetTrainingTriggerGate(ftloop.NewGate(client.Pool(), ftloop.GateConfig{
				Enabled:          s.cfg.FtLoopEnabled,
				RetrainInterval:  time.Duration(s.cfg.FtLoopMinRetrainIntervalHours) * time.Hour,
				MinFreshFraction: s.cfg.FtLoopMinFreshFraction,
			}))
		}
		slog.Info("tsdb: metric writer attached", "flush_interval_sec", s.cfg.TSDBFlushIntervalSec)

		// LLM interaction logger — record all LLM calls for FT pipeline.
		// If serve.go already created an early writer (via SetLLMWriter), reuse it
		// to avoid duplicate flush goroutines. Otherwise create one here.
		if s.cfg.LLMInteractionLogging {
			if s.llmWriter == nil {
				s.llmWriter = tsdb.NewLLMInteractionWriter(
					client.Pool(),
					time.Duration(s.cfg.TSDBFlushIntervalSec)*time.Second,
				)
			}
			// Re-set defaults (idempotent if already set in serve.go)
			llmclient.SetDefaultRecorder(s.llmWriter)
			llmclient.SetDefaultInstanceID(s.cfg.InstanceID)
			llmclient.SetDefaultSpaceID(s.cfg.RSICWatchdogSpaceID)
			llmclient.SetDefaultSessionID("") // empty — callers provide via WithSessionID
			slog.Info("tsdb: LLM interaction logger attached", "instance_id", s.cfg.InstanceID)
		}

		// Embedding event logger — record all Embed() calls for contrastive training data
		if s.cfg.EmbeddingEventLogging {
			s.embeddingWriter = tsdb.NewEmbeddingEventWriter(
				client.Pool(),
				time.Duration(s.cfg.TSDBFlushIntervalSec)*time.Second,
			)
			// EMBED-WIRE-001: find the cache layer by walking the wrapper
			// chain — the old direct assertion silently lost recording
			// whenever an outer wrapper was present or the cache disabled.
			if ce, ok := embeddings.FindCached(s.embedder); ok {
				ce.SetRecorder(&embeddingRecorderAdapter{
					writer:         s.embeddingWriter,
					instanceID:     s.cfg.InstanceID,
					defaultSpaceID: s.cfg.RSICWatchdogSpaceID,
				})
				slog.Info("tsdb: embedding event logger attached")
			} else {
				slog.Warn("tsdb: embedding event logger NOT attached — no CachedEmbedder layer (EMBEDDING_CACHE_ENABLED=false?); embedding training data will not be recorded")
			}
		}

		// Retrieval event logger — record all Retrieve() pipelines for contrastive training data
		if s.cfg.RetrievalEventLogging {
			s.retrievalWriter = tsdb.NewRetrievalEventWriter(
				client.Pool(),
				time.Duration(s.cfg.TSDBFlushIntervalSec)*time.Second,
			)
			s.retriever.SetRetrievalRecorder(&retrievalRecorderAdapter{
				writer:         s.retrievalWriter,
				instanceID:     s.cfg.InstanceID,
				defaultSpaceID: s.cfg.RSICWatchdogSpaceID,
			})
			slog.Info("tsdb: retrieval event logger attached")
		}

		// Constraint outcomes logger — record guidance outcomes for dynamic Grafana queries
		s.constraintOutcomesWriter = tsdb.NewConstraintOutcomesWriter(
			client.Pool(),
			time.Duration(s.cfg.TSDBFlushIntervalSec)*time.Second,
		)
		if s.jiminySvc != nil {
			s.jiminySvc.SetOutcomeWriter(s.constraintOutcomesWriter)
		}
		slog.Info("tsdb: constraint outcomes logger attached")

		// JIMINY-CONTRADICTED-BRIDGE-001 Epic 1 — V0030 contradicted_correction_drafts
		// writer. Always-attached (HITL dataset needs it for reads even when the
		// bridge hook is off); the write side is separately gated on
		// JIMINY_CONTRADICTED_BRIDGE_ENABLED at the RecordOutcome call site.
		s.contradictedDraftWriter = tsdb.NewContradictedDraftsWriter(
			client.Pool(),
			time.Duration(s.cfg.JiminyContradictedBridgeWriterFlushIntervalSec)*time.Second,
			s.cfg.JiminyContradictedBridgeWriterBufferSize,
		)
		if s.jiminySvc != nil {
			s.jiminySvc.SetContradictedDraftWriter(s.contradictedDraftWriter)
		}
		slog.Info("tsdb: contradicted_drafts writer attached",
			"flush_interval_sec", s.cfg.JiminyContradictedBridgeWriterFlushIntervalSec,
			"bridge_enabled", s.cfg.JiminyContradictedBridgeEnabled)

		// JIMINY-RELEVANCE-001 Epic 1 — V0027 guidance_training_rows writer.
		// Persists the training EVIDENCE (guidance text + action text + source
		// role/layer + verdict) discarded until now. Gated by
		// GUIDANCE_CORPUS_ENABLED (default true); buffered + async so the
		// /v1/jiminy/feedback hot path is never blocked.
		if s.cfg.GuidanceCorpusEnabled && s.jiminySvc != nil {
			s.guidanceTrainingWriter = tsdb.NewGuidanceTrainingRowsWriter(
				client.Pool(),
				time.Duration(s.cfg.GuidanceCorpusWriterFlushIntervalSec)*time.Second,
				s.cfg.GuidanceCorpusWriterBufferSize,
			)
			s.jiminySvc.SetGuidanceTrainingWriter(&guidanceTrainingAdapter{w: s.guidanceTrainingWriter})
			if std := metrics.Metrics(); std != nil {
				s.guidanceTrainingWriter.SetPrometheusCounters(
					std.GuidanceCorpusRowsEnqueued,
					std.GuidanceCorpusRowsDropped,
					std.GuidanceCorpusFlushFailure,
				)
			}
			slog.Info("tsdb: guidance_training_rows writer attached",
				"flush_interval_sec", s.cfg.GuidanceCorpusWriterFlushIntervalSec,
				"buffer_size", s.cfg.GuidanceCorpusWriterBufferSize)
		}

		// HITL-REVIEW-001 Epic 1 — the review platform: registry + review_grades
		// writer. Datasets register here (the stub for self-test; the guidance
		// dataset + sink are wired in Epic 5). Gated by REVIEW_ENABLED.
		if s.cfg.ReviewEnabled {
			s.reviewWriter = tsdb.NewReviewGradesWriter(
				client.Pool(),
				time.Duration(s.cfg.ReviewWriterFlushIntervalSec)*time.Second,
				s.cfg.ReviewWriterBufferSize,
			)
			s.reviewRegistry = review.NewRegistry()
			// HITL-REVIEW-001 Epic 5 — the guidance corpus as the first reviewable
			// dataset + the live-reinforcement GuidanceSink (trust EMA + node
			// confidence, reversible). Needs jiminy (the reinforcer) + the pool
			// (guidance_training_rows). Gated by REVIEW_GUIDANCE_SINK_ENABLED.
			if s.cfg.ReviewGuidanceSinkEnabled && s.jiminySvc != nil && client.Pool() != nil {
				gds := &guidanceDataset{
					pool:          client.Pool(),
					rubricVersion: s.cfg.ReviewRubricVersion,
					sink: review.GuidanceSink{
						R:               guidanceReinforcerAdapter{svc: s.jiminySvc},
						ConfidenceNudge: s.cfg.ReviewGuidanceConfidenceNudge,
					},
				}
				if err := s.reviewRegistry.Register(gds); err != nil {
					slog.Warn("review: guidance dataset registration failed", "error", err)
				} else {
					slog.Info("review: guidance dataset + live-reinforcement sink registered")
				}
			}

			// JIMINY-CONTRADICTED-BRIDGE-001 Epic 3 — contradicted-outcome
			// correction-draft dataset + sink. Registers even when the bridge
			// hook itself is off, so any existing pending drafts remain reviewable
			// under a flag-flip rollback. Gated by REVIEW_CONTRADICTED_DATASET_ENABLED
			// (default true) and needs the conversation service (Correct sink).
			if s.cfg.ReviewContradictedDatasetEnabled && s.contradictedDraftWriter != nil && s.conversationSvc != nil {
				cds := &contradictedDraftsDataset{
					writer:        s.contradictedDraftWriter,
					rubricVersion: s.cfg.ReviewRubricVersion,
					sink: contradictedDraftsSink{
						svc:    s.conversationSvc,
						writer: s.contradictedDraftWriter,
					},
				}
				if err := s.reviewRegistry.Register(cds); err != nil {
					slog.Warn("review: contradicted_drafts dataset registration failed", "error", err)
				} else {
					slog.Info("review: contradicted_drafts dataset + Correct sink registered")
				}
			}
			// HITL-REVIEW-001 — the 16 MDEMG LLM call sites as reviewable
			// datasets (gold-only review of llm_interactions outputs → SFT/quality
			// training data). Gated by REVIEW_LLM_DATASETS_ENABLED.
			if s.cfg.ReviewLLMDatasetsEnabled && client.Pool() != nil {
				n := 0
				for _, site := range llmCallSiteCatalog {
					if err := s.reviewRegistry.Register(llmCallSiteDataset{
						site: site, pool: client.Pool(), rubricVersion: s.cfg.ReviewRubricVersion,
					}); err != nil {
						slog.Warn("review: llm dataset registration failed", "task", site.task, "error", err)
					} else {
						n++
					}
				}
				slog.Info("review: LLM call-site datasets registered", "count", n)
			}
			slog.Info("review: platform attached",
				"flush_interval_sec", s.cfg.ReviewWriterFlushIntervalSec,
				"guidance_sink", s.cfg.ReviewGuidanceSinkEnabled)
		}

		// Phase 14 Epic 0 — V0017 retrieval_audit writer. Phase 13 Epic 6
		// shipped the schema + interface but the writer was never wired,
		// leaving V0017 empty. Wire it here so RETRIEVAL_AUDIT_ENABLED=true
		// actually produces rows. Buffered + flushed via CopyFrom every
		// TSDB_FLUSH_INTERVAL_SEC (default 30s).
		if s.cfg.RetrievalAuditEnabled {
			s.retrievalAuditWriter = tsdb.NewRetrievalAuditWriter(
				client.Pool(),
				time.Duration(s.cfg.TSDBFlushIntervalSec)*time.Second,
			)
			s.retriever.SetRetrievalAuditWriter(&retrievalAuditAdapter{
				writer: s.retrievalAuditWriter,
			})
			slog.Info("tsdb: retrieval audit writer attached")
		}

		// Phase 14 Epic 1 — V0019 sparse_gate_metrics writer. Always wire
		// (independent of SparseRetrievalEnabled default) so per-request
		// `?sparse=true` overrides record even when env default is off. The
		// recorder is called only when the gate fires, so the writer stays
		// idle when the feature is disabled.
		s.sparseGateWriter = tsdb.NewSparseGateMetricsWriter(
			client.Pool(),
			time.Duration(s.cfg.TSDBFlushIntervalSec)*time.Second,
		)
		s.retriever.SetSparseGateRecorder(&sparseGateRecorderAdapter{
			writer: s.sparseGateWriter,
		})
		slog.Info("tsdb: sparse_gate_metrics writer attached")

		// EVENTGRAPH-001 Epic 4 — V0022 reinforcement_events writer. Gated
		// by EVENTGRAPH_ENABLED (default true); when disabled the writer is
		// not constructed and learning.Service.reinforcementWriter stays nil
		// so the Hebbian path short-circuits cleanly.
		if s.cfg.EventGraphEnabled {
			s.reinforcementWriter = tsdb.NewReinforcementEventsWriter(
				client.Pool(),
				time.Duration(s.cfg.EventGraphWriterFlushIntervalSec)*time.Second,
				s.cfg.EventGraphWriterBufferSize,
			)
			s.learner.SetReinforcementWriter(s.reinforcementWriter)
			s.learner.SetMaxPairsPerEventBatch(s.cfg.EventGraphMaxPairsPerEventBatch)
			s.eventgraphService = eventgraph.NewService(s.driver, client.Pool())
			// Wire the writer's Prometheus counter mirrors (Epic 6). nil-safe.
			if std := metrics.Metrics(); std != nil {
				s.reinforcementWriter.SetPrometheusCounters(
					std.EventgraphRowsEnqueued,
					std.EventgraphRowsDropped,
					std.EventgraphFlushFailure,
				)
			}
			slog.Info("tsdb: reinforcement_events writer + federation service attached",
				"flush_interval_sec", s.cfg.EventGraphWriterFlushIntervalSec,
				"buffer_size", s.cfg.EventGraphWriterBufferSize)
		} else {
			slog.Info("tsdb: reinforcement_events writer disabled (EVENTGRAPH_ENABLED=false)")
		}

		// Phase 13.5 — LLM endpoint health events writer (V0018 hypertable).
		// Watchdog state-transition + fast-fail-burst events land here for
		// historical Grafana panels that survive mdemg restarts. Wire into
		// mlxprobe.OnTransition/SetFastFailObserver in cli/serve.go via the
		// LLMEndpointHealthWriter() getter.
		s.llmEndpointHealthWriter = tsdb.NewLLMEndpointHealthWriter(
			client.Pool(),
			time.Duration(s.cfg.TSDBFlushIntervalSec)*time.Second,
		)
		slog.Info("tsdb: llm-endpoint health writer attached")

		// Multi-instance collision detection
		if s.cfg.InstanceID != "" {
			var otherInstances []string
			rows, qErr := client.Pool().Query(context.Background(),
				`SELECT DISTINCT instance_id FROM llm_interactions
				 WHERE instance_id != '' AND instance_id != $1
				   AND space_id = $2
				   AND time > now() - interval '24 hours'
				 LIMIT 5`,
				s.cfg.InstanceID, s.cfg.RSICWatchdogSpaceID,
			)
			if qErr == nil {
				defer rows.Close()
				for rows.Next() {
					var id string
					if rows.Scan(&id) == nil {
						otherInstances = append(otherInstances, id)
					}
				}
			}
			if len(otherInstances) > 0 {
				slog.Warn("tsdb: other MDEMG instances detected on same space_id",
					"this_instance", s.cfg.InstanceID,
					"space_id", s.cfg.RSICWatchdogSpaceID,
					"other_instances", otherInstances,
				)
			}
		}
	}
}

// SetLLMWriter attaches a pre-created LLM interaction writer.
// Called from serve.go BEFORE NewServer so that LLM clients created during
// NewServer (query classifier, intent translator) can record to TSDB.
func (s *Server) SetLLMWriter(w *tsdb.LLMInteractionWriter) {
	s.llmWriter = w
}

// SetLogBuffer attaches the log ring buffer for the /v1/admin/logs endpoint.
// Called from serve.go after logging initialization.
func (s *Server) SetLogBuffer(buf *LogRingBuffer) {
	s.logBuffer = buf
}

// AlertDispatcher returns the server's alert dispatcher for external callback wiring.
func (s *Server) AlertDispatcher() *alert.Dispatcher {
	return s.alertDispatcher
}

// LLMEndpointHealthWriter returns the V0018 health-event writer for external
// callback wiring. Returns nil if TSDB isn't configured. Phase 13.5 — used
// from cli/serve.go to subscribe the watchdog OnTransition + FastFail
// callbacks so endpoint stability is observable in Grafana over time ranges
// that survive process restarts (Prometheus counters reset on restart).
func (s *Server) LLMEndpointHealthWriter() *tsdb.LLMEndpointHealthWriter {
	return s.llmEndpointHealthWriter
}

// embeddingRecorderAdapter adapts tsdb.EmbeddingEventWriter to embeddings.EmbeddingEventRecorder.
type embeddingRecorderAdapter struct {
	writer         *tsdb.EmbeddingEventWriter
	instanceID     string
	defaultSpaceID string
}

// retrievalRecorderAdapter adapts tsdb.RetrievalEventWriter to retrieval.RetrievalEventRecorder.
type retrievalRecorderAdapter struct {
	writer         *tsdb.RetrievalEventWriter
	instanceID     string
	defaultSpaceID string
}

// retrievalAuditAdapter adapts tsdb.RetrievalAuditWriter (which lives in tsdb
// to avoid an import cycle) to retrieval.RetrievalAuditWriter (the contract
// the retrieval package consumes). Phase 14 Epic 0 wired this — Phase 13 Epic
// 6 shipped the V0017 schema + interface but never the writer.
type retrievalAuditAdapter struct {
	writer *tsdb.RetrievalAuditWriter
}

// sparseGateRecorderAdapter adapts tsdb.SparseGateMetricsWriter (in tsdb, to
// avoid the import cycle) to retrieval.SparseGateRecorder (the contract
// retrieval consumes). One row per gate firing — enables Phase 14.1 retune
// from production traffic.
type sparseGateRecorderAdapter struct {
	writer *tsdb.SparseGateMetricsWriter
}

// guidanceTrainingAdapter adapts *tsdb.GuidanceTrainingRowsWriter (in tsdb, to
// avoid the import cycle) to jiminy.GuidanceTrainingWriter (the contract jiminy
// consumes). Maps the jiminy-side evidence record to the tsdb row shape.
// JIMINY-RELEVANCE-001 Epic 1.
type guidanceTrainingAdapter struct {
	w *tsdb.GuidanceTrainingRowsWriter
}

func (a *guidanceTrainingAdapter) RecordTrainingRow(row jiminy.GuidanceTrainingRecord) {
	a.w.Record(tsdb.GuidanceTrainingRow{
		SpaceID:          row.SpaceID,
		SessionID:        row.SessionID,
		InstanceID:       row.InstanceID,
		GuidanceID:       row.GuidanceID,
		GuidanceType:     row.GuidanceType,
		GuidanceContent:  row.GuidanceContent,
		SourceNodeID:     row.SourceNodeID,
		SourceRoleType:   row.SourceRoleType,
		SourceLayer:      row.SourceLayer,
		ActionSummary:    row.ActionSummary,
		OutcomeType:      row.OutcomeType,
		Similarity:       row.Similarity,
		ClassifierSource: row.ClassifierSource,
		ConstraintCode:   row.ConstraintCode,
	})
}

// RecordGate satisfies retrieval.SparseGateRecorder. Translates the retrieval-
// side metadata into the tsdb-side row shape and forwards to the buffered
// writer (no synchronous DB call — flushed by ticker).
func (a *sparseGateRecorderAdapter) RecordGate(spaceID string, meta retrieval.SparseGateMetadata, scorerVersion string) {
	a.writer.Record(tsdb.SparseGateMetricRow{
		SpaceID:           spaceID,
		PercentileApplied: meta.PercentileApplied,
		ThresholdScore:    meta.Threshold,
		InputCount:        meta.InputCount,
		ActiveCount:       meta.ActiveCount,
		DroppedCount:      meta.DroppedCount,
		FloorApplied:      meta.FloorApplied,
		CeilingApplied:    meta.CeilingApplied,
		ScorerVersion:     scorerVersion,
	})
}

// Write satisfies retrieval.RetrievalAuditWriter. Translates the per-column
// time.Duration map to int64 ms (tsdb-side) and forwards to the buffered
// writer. Always returns nil — actual persistence happens on flush; the
// service-level call site is fail-open by design.
func (a *retrievalAuditAdapter) Write(_ context.Context, rec retrieval.RetrievalAuditRecord) error {
	var perColMs map[string]int64
	if len(rec.PerColumnLatency) > 0 {
		perColMs = make(map[string]int64, len(rec.PerColumnLatency))
		for k, v := range rec.PerColumnLatency {
			perColMs[k] = v.Milliseconds()
		}
	}
	a.writer.Record(tsdb.RetrievalAuditRow{
		SpaceID:            rec.SpaceID,
		QueryTextHash:      rec.QueryTextHash,
		ScorerVersion:      rec.ScorerVersion,
		ConsensusStrength:  rec.ConsensusStrength,
		PerColumnLatencyMs: perColMs,
		ColumnsQueried:     rec.ColumnsQueried,
		ColumnsReturned:    rec.ColumnsReturned,
		TopKNodeIDs:        rec.TopKNodeIDs,
		TotalLatencyMs:     rec.TotalLatencyMs,
	})
	return nil
}

func (a *retrievalRecorderAdapter) RecordRetrieval(_ context.Context, event retrieval.RetrievalEvent) {
	spaceID := event.SpaceID
	if spaceID == "" {
		spaceID = a.defaultSpaceID
	}
	a.writer.Record(tsdb.RetrievalEventRow{
		Time:              event.Time,
		EventID:           event.EventID,
		SpaceID:           spaceID,
		CallSite:          event.CallSite,
		QueryText:         event.QueryText,
		QueryHash:         event.QueryHash,
		RecallNodeIDs:     event.RecallNodeIDs,
		RecallScores:      event.RecallScores,
		RecallK:           event.RecallK,
		BM25NodeIDs:       event.BM25NodeIDs,
		BM25Scores:        event.BM25Scores,
		RerankNodeIDs:     event.RerankNodeIDs,
		RerankScores:      event.RerankScores,
		RerankModel:       event.RerankModel,
		ResultNodeIDs:     event.ResultNodeIDs,
		ResultScores:      event.ResultScores,
		ResultCount:       event.ResultCount,
		GuidanceID:        event.GuidanceID,
		DownstreamQuality: event.DownstreamQuality,
		RecallLatencyMs:   event.RecallLatencyMs,
		RerankLatencyMs:   event.RerankLatencyMs,
		TotalLatencyMs:    event.TotalLatencyMs,
		InstanceID:        a.instanceID,
	})
}

func (a *embeddingRecorderAdapter) RecordEmbed(_ context.Context, event embeddings.EmbeddingEvent) {
	spaceID := event.SpaceID
	if spaceID == "" {
		spaceID = a.defaultSpaceID
	}
	a.writer.Record(tsdb.EmbeddingEventRow{
		Time:        event.Time,
		EventID:     event.EventID,
		EventType:   event.EventType,
		SpaceID:     spaceID,
		TextContent: event.TextContent,
		TextHash:    event.TextHash,
		TextLength:  event.TextLength,
		ElementKind: event.ElementKind,
		Language:    event.Language,
		FilePath:    event.FilePath,
		ChunkStart:  event.ChunkStart,
		ChunkEnd:    event.ChunkEnd,
		PackageName: event.PackageName,
		Signature:   event.Signature,
		Tags:        event.Tags,
		CallSite:    event.CallSite,
		QueryText:   event.QueryText,
		ModelName:   event.ModelName,
		Provider:    event.Provider,
		Dimensions:  event.Dimensions,
		LatencyMs:   event.LatencyMs,
		Cached:      event.Cached,
		NodeID:      event.NodeID,
		InstanceID:  a.instanceID,
	})
}

// Shutdown gracefully stops background services
func (s *Server) Shutdown() {
	if s.apeScheduler != nil {
		s.apeScheduler.Stop()
	}
	if s.sessionTracker != nil {
		s.sessionTracker.Stop()
	}
	if s.fileWatcherMgr != nil {
		s.fileWatcherMgr.StopAll()
	}
	s.StopPeriodicConsolidation()
	s.StopContextCoolerProcessing()
	s.StopWeeklyGapInterviews()
	s.StopScheduledSync()
	s.StopRSICWatchdog()
	s.StopSpacePruneScheduler()
	if s.rsicStore != nil {
		s.rsicStore.Stop()
	}
	if s.macroCronCancel != nil {
		s.macroCronCancel()
	}
	if s.backupScheduler != nil {
		s.backupScheduler.Stop()
	}
	if s.tsdbBackupScheduler != nil {
		s.tsdbBackupScheduler.Stop()
	}
	if s.metricsRecorder != nil {
		s.metricsRecorder.Stop()
	}
	if s.signalLearner != nil {
		s.signalLearner.StopPersistence()
	}
	if s.jiminySvc != nil {
		s.jiminySvc.StopTrustPersistence()
	}

	// Wait for all tracked background goroutines to exit
	s.bgWg.Wait()

	if s.llmWriter != nil {
		s.llmWriter.Close()
	}
	if s.embeddingWriter != nil {
		s.embeddingWriter.Close()
	}
	if s.retrievalWriter != nil {
		s.retrievalWriter.Close()
	}
	if s.retrievalAuditWriter != nil {
		s.retrievalAuditWriter.Close()
	}
	if s.sparseGateWriter != nil {
		s.sparseGateWriter.Close()
	}
	if s.reinforcementWriter != nil {
		s.reinforcementWriter.Close()
	}
	if s.constraintOutcomesWriter != nil {
		s.constraintOutcomesWriter.Close()
	}
	if s.guidanceTrainingWriter != nil {
		s.guidanceTrainingWriter.Close()
	}
	if s.reviewWriter != nil {
		s.reviewWriter.Close()
	}
	if s.contradictedDraftWriter != nil {
		s.contradictedDraftWriter.Close()
	}
	if s.llmEndpointHealthWriter != nil {
		s.llmEndpointHealthWriter.Close()
	}
	if s.tsdbWriter != nil {
		s.tsdbWriter.Close()
	}
}

// StartMacroCronScheduler starts the macro cron scheduler goroutine.
// It parses RSIC_MACRO_CRON and fires macro cycles on schedule.
func (s *Server) StartMacroCronScheduler() {
	cronExpr := s.cfg.RSICMacroCron
	if cronExpr == "" {
		slog.Info("RSIC macro cron disabled (RSIC_MACRO_CRON empty)")
		return
	}

	interval := parseCronInterval(cronExpr)
	if interval <= 0 {
		slog.Warn("RSIC macro cron: unrecognized expression, disabled", "expression", cronExpr)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.macroCronCancel = cancel
	s.macroNextRun = time.Now().Add(interval)

	s.goSupervised("rsic-macro-cron", func(runCtx context.Context) error {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		slog.Info("RSIC macro cron scheduler started", "interval", interval, "next_run", s.macroNextRun.Format(time.RFC3339))

		for {
			select {
			case <-runCtx.Done():
				return nil
			case <-ctx.Done():
				return nil
			case now := <-ticker.C:
				// Cleanup expired orchestration entries on every tick
				if s.orchestrationPolicy != nil {
					s.orchestrationPolicy.CleanupExpired()
				}

				if now.Before(s.macroNextRun) {
					continue
				}
				s.macroNextRun = now.Add(interval)

				if s.orchestrationPolicy == nil || s.rsicCycle == nil {
					continue
				}

				// Fire macro cycle for configured space
				spaceID := s.cfg.RSICMacroCronSpace
				decision := s.orchestrationPolicy.EvaluateTrigger(ape.TriggerMacroCron, spaceID, ape.TierMacro, "")
				if !decision.Allowed {
					slog.Info("RSIC macro cron: skipped", "space_id", spaceID, "reason", decision.Reason)
					continue
				}

				go func() { //nolint:gosec // G118: macro cycle must complete independently of the scheduler loop's ctx
					opts := &ape.RunCycleOpts{TriggerMeta: &decision.Meta}
					outcome, err := s.rsicCycle.RunCycle(context.Background(), spaceID, ape.TierMacro, opts)
					if err != nil {
						s.orchestrationPolicy.CompleteCycle(spaceID, ape.TierMacro)
						slog.Error("RSIC macro cron cycle failed", "error", err)
						return
					}
					s.orchestrationPolicy.RecordTrigger(decision.Meta, spaceID, ape.TierMacro, outcome.CycleID)
					// Skip CompleteCycle when timed out — let stale-cycle cleanup handle it
					if !outcome.TimedOut {
						s.orchestrationPolicy.CompleteCycle(spaceID, ape.TierMacro)
					}
					slog.Info("RSIC macro cron cycle complete", "cycle_id", outcome.CycleID, "timed_out", outcome.TimedOut)
				}()
			}
		}
	})
}

// parseCronInterval converts common cron expressions to intervals.
func parseCronInterval(expr string) time.Duration {
	expr = strings.TrimSpace(expr)
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return 0
	}

	// Handle "*/N * * * *" (every N minutes)
	if strings.HasPrefix(parts[0], "*/") && parts[1] == "*" && parts[2] == "*" && parts[3] == "*" && parts[4] == "*" {
		n, err := strconv.Atoi(strings.TrimPrefix(parts[0], "*/"))
		if err == nil && n > 0 {
			return time.Duration(n) * time.Minute
		}
	}

	// "0 * * * *" → every hour
	if parts[0] == "0" && parts[1] == "*" && parts[2] == "*" && parts[3] == "*" && parts[4] == "*" {
		return time.Hour
	}

	// "0 */N * * *" → every N hours
	if parts[0] == "0" && strings.HasPrefix(parts[1], "*/") && parts[2] == "*" && parts[3] == "*" && parts[4] == "*" {
		n, err := strconv.Atoi(strings.TrimPrefix(parts[1], "*/"))
		if err == nil && n > 0 {
			return time.Duration(n) * time.Hour
		}
	}

	// "0 H * * *" → daily at hour H
	if parts[0] == "0" && parts[2] == "*" && parts[3] == "*" && parts[4] == "*" {
		if _, err := strconv.Atoi(parts[1]); err == nil {
			return 24 * time.Hour
		}
	}

	// "0 H * * D" → weekly
	if parts[0] == "0" && parts[2] == "*" && parts[3] == "*" {
		if _, err := strconv.Atoi(parts[1]); err == nil {
			if _, err2 := strconv.Atoi(parts[4]); err2 == nil {
				return 7 * 24 * time.Hour
			}
		}
	}

	return 0
}

// StartFileWatchers starts file watchers based on configuration.
// Called during server startup if FILE_WATCHER_ENABLED=true.
func (s *Server) StartFileWatchers() {
	if !s.cfg.FileWatcherEnabled {
		slog.Info("file watcher disabled (FILE_WATCHER_ENABLED=false)")
		return
	}

	if s.cfg.FileWatcherConfigs == "" {
		slog.Info("file watcher enabled but no configs (FILE_WATCHER_CONFIGS empty)")
		return
	}

	configs := filewatcher.ParseConfigs(s.cfg.FileWatcherConfigs)
	if len(configs) == 0 {
		slog.Info("file watcher: no valid configs found")
		return
	}

	for _, cfg := range configs {
		cfg.OnChange = s.handleFileWatcherChange
		if err := s.fileWatcherMgr.AddWatcher(cfg); err != nil {
			slog.Error("file watcher: failed to start watcher", "space_id", cfg.SpaceID, "error", err)
		}
	}

	slog.Info("file watcher: started watchers", "count", len(configs))
}

// handleFileWatcherChange handles file changes from the file watcher.
func (s *Server) handleFileWatcherChange(ctx context.Context, spaceID string, files []string) {
	slog.Info("filewatcher: files changed", "count", len(files), "space_id", spaceID)

	// Call the internal file ingest API
	resp, err := s.ingestFilesInternal(ctx, spaceID, files)
	if err != nil {
		slog.Error("filewatcher: ingest failed", "space_id", spaceID, "error", err)
		return
	}

	slog.Info("filewatcher: ingested files", "success_count", resp.SuccessCount, "total_files", resp.TotalFiles, "space_id", spaceID)

	// Trigger APE event
	s.TriggerAPEEventWithContext("source_changed", map[string]string{
		"space_id":    spaceID,
		"ingest_type": "file-watcher",
	})
}

// ingestFilesInternal is the internal version of file ingestion that doesn't require HTTP.
func (s *Server) ingestFilesInternal(ctx context.Context, spaceID string, files []string) (*models.IngestFilesResponse, error) {
	resp := &models.IngestFilesResponse{
		SpaceID:    spaceID,
		TotalFiles: len(files),
	}

	results := make([]models.IngestFileResult, 0, len(files))
	for _, filePath := range files {
		result := models.IngestFileResult{File: filePath}

		// Check if file exists and is readable
		content, err := os.ReadFile(filePath)
		if err != nil {
			result.Status = "error"
			result.Error = fmt.Sprintf("failed to read file: %v", err)
			resp.ErrorCount++
			results = append(results, result)
			continue
		}

		// Build ingest request
		req := models.IngestRequest{
			SpaceID:   spaceID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Source:    "file-watcher",
			Content:   string(content),
			Path:      filePath,
			Name:      filepath.Base(filePath),
		}

		// Ingest the file
		ingestResp, err := s.retriever.IngestObservation(ctx, req)
		if err != nil {
			result.Status = "error"
			result.Error = fmt.Sprintf("ingest failed: %v", err)
			resp.ErrorCount++
		} else {
			result.Status = "success"
			result.NodeID = ingestResp.NodeID
			resp.SuccessCount++
		}
		results = append(results, result)
	}

	resp.Results = results

	return resp, nil
}

// SetSupervisor injects the goroutine-supervision launcher (SUPERVISOR-002).
// When set, background loops started afterwards run with panic recovery and
// a sliding-window restart budget. Call before the Start* methods.
func (s *Server) SetSupervisor(fn func(name string, fn func(ctx context.Context) error)) {
	s.superviseFn = fn
}

// goSupervised launches a blocking background loop under the injected
// supervisor, falling back to a bare goroutine when none is set (legacy
// behavior for tests and non-server callers). fn must block until its stop
// condition and return nil on graceful stop; only panics or returned errors
// trigger a supervised restart. bgWg brackets each run so Shutdown's
// bgWg.Wait() keeps its meaning under restarts.
func (s *Server) goSupervised(name string, fn func(ctx context.Context) error) {
	wrapped := func(ctx context.Context) error {
		s.bgWg.Add(1)
		defer s.bgWg.Done()
		return fn(ctx)
	}
	if s.superviseFn != nil {
		s.superviseFn(name, wrapped)
		return
	}
	go func() {
		_ = wrapped(context.Background())
	}()
}

// StartSupervisedBackground starts the background loops that historically
// launched inside NewServer (backup schedulers, RSIC store flush,
// signal-learner persistence), routing them through the injected supervisor
// when one is set (SUPERVISOR-002). Call after SetSupervisor. Without a
// supervisor the loops run as plain goroutines (legacy behavior), so callers
// that never inject one lose nothing.
func (s *Server) StartSupervisedBackground() {
	if s.rsicStore != nil {
		s.rsicStore.SetSupervise(s.superviseFn)
		s.rsicStore.Start(context.Background())
	}
	if s.signalLearner != nil {
		s.signalLearner.SetSupervise(s.superviseFn)
		s.signalLearner.StartPersistence(context.Background())
	}
	if s.backupScheduler != nil {
		s.backupScheduler.SetSupervise(s.superviseFn)
		s.backupScheduler.Start()
	}
	if s.tsdbBackupScheduler != nil {
		s.tsdbBackupScheduler.SetSupervise(s.superviseFn)
		s.tsdbBackupScheduler.Start()
	}
	// FT-RECURSIVE-002 Epic 5: the recursive-retrain controller (supervised,
	// default-off behind FT_LOOP_ENABLED — Run returns immediately when off).
	if s.tsdbClient != nil {
		repoDir, _ := os.Getwd()
		leasePath := s.cfg.FtLoopLeasePath
		if leasePath == "" {
			if home, err := os.UserHomeDir(); err == nil {
				leasePath = filepath.Join(home, ".mdemg", "ft-loop.lease")
			} else {
				leasePath = filepath.Join(os.TempDir(), "mdemg-ft-loop.lease")
			}
		}
		ctrl := ftloop.NewController(s.tsdbClient.Pool(), s.orchestrationPolicy, s.alertDispatcher,
			ftloop.ControllerConfig{
				Enabled:         s.cfg.FtLoopEnabled,
				PollInterval:    time.Duration(s.cfg.FtLoopPollIntervalSec) * time.Second,
				LeasePath:       leasePath,
				LeaseMax:        time.Duration(s.cfg.FtLoopLeaseMaxHours) * time.Hour,
				MinFreeDiskGB:   s.cfg.FtLoopMinFreeDiskGB,
				PythonBin:       s.cfg.FtLoopPythonBin,
				ModelVersion:    s.cfg.FtLoopModelVersion,
				EpochsCap:       s.cfg.FtLoraEpochsCap,
				EarlyStopFactor: s.cfg.FtEarlyStopValLossFactor,
				RepoDir:         repoDir,
				InstanceID:      s.cfg.InstanceID,
				SpaceID:         s.cfg.RSICWatchdogSpaceID,
				// Epic-6 pipeline wiring (the proven curate→train→convert→gate commands).
				WorkDir:         s.cfg.FtLoopWorkDir,
				BaseModel:       s.cfg.FtLoopBaseModel,
				BaseSHA:         s.cfg.FtLoopBaseSHA,
				UaitsSpec:       s.cfg.FtLoopUaitsSpec,
				BenchmarkConfig: s.cfg.FtLoopBenchmarkConfig,
				LoraRank:        s.cfg.FtLoopLoraRank,
				LoraAlpha:       s.cfg.FtLoopLoraAlpha,
				GatePort:        s.cfg.FtLoopGatePort,
				ExportSinceDays: s.cfg.FtLoopExportSinceDays,
				GateTaskFilter:  s.cfg.FtLoopGateTaskFilter,
				ConvertScript:   s.cfg.FtLoopConvertScript,
				GateMinScore:    s.cfg.FtLoopGateMinScore,
				MdemgBin:        resolveMdemgBin(),
				AutoPromoteAfter: s.cfg.FtLoopAutoPromoteAfter,
				Promotion: ftloop.PromotionConfig{
					Serving: ftloop.ServingConfig{
						SymlinkPath:   ftServingLink(repoDir, s.cfg.FtLoopServingSymlink),
						PlistLabel:    s.cfg.FtLoopServingPlistLabel,
						HealthURL:     s.cfg.FtLoopServingHealthURL,
						HealthTimeout: time.Duration(s.cfg.FtLoopSwapHealthTimeoutSec) * time.Second,
					},
					CanaryEnabled: s.cfg.FtLoopCanaryEnabled,
					CanaryProbes:  s.cfg.FtLoopCanaryProbes,
					CanaryCount:   s.cfg.FtLoopCanaryProbeCount,
					CanaryProdURL: s.cfg.FtLoopCanaryProdURL,
					GatePort:      s.cfg.FtLoopGatePort,
					RepoDir:       repoDir,
					BaseModel:     s.cfg.FtLoopBaseModel,
					ModelVersion:  s.cfg.FtLoopModelVersion,
				},
			})
		// FT-RECURSIVE-003 E7: class-5 escalator (repeated failure
		// fingerprints → CapabilityGap + fingerprint-idempotent GitHub issue).
		if s.cfg.FtLoopIssueFilerEnabled {
			var gapSink ftloop.GapSink
			if s.gapDetector != nil {
				gapSink = s.gapDetector.GetStore()
			}
			filer := ftloop.NewIssueFiler(s.tsdbClient.Pool(), ftloop.IssueFilerConfig{
				Enabled:         true,
				RepeatThreshold: s.cfg.FtLoopIssueRepeatThreshold,
				LookbackDays:    s.cfg.FtLoopIssueLookbackDays,
				Repo:            s.cfg.FtLoopIssueRepo,
				TokenPath:       s.cfg.FtLoopIssueTokenPath,
				SweepMinutes:    s.cfg.FtLoopIssueSweepMin,
			}, gapSink, func(ctx context.Context, jobName string, success bool, latencyMs int64, errMsg string) {
				ev := tsdb.JobEventRow{JobName: jobName, SpaceID: s.cfg.RSICWatchdogSpaceID,
					InstanceID: s.cfg.InstanceID, Success: success, LatencyMS: latencyMs, ErrorMessage: errMsg}
				jobhealth.ReportWithService(ctx, s.tsdbClient.Pool(), s.alertDispatcher, ev, "ft-loop")
			})
			ctrl.SetIssueFiler(filer)
		}
		s.goSupervised("ft-loop-controller", ctrl.Run)

		// FT-RECURSIVE-004 E4: scheduled benchmark refresh (feeds
		// ft_production_drift + keeps ft_benchmark_stale green). Independent
		// of FtLoopEnabled — model-quality freshness matters regardless.
		if s.cfg.FtBenchScheduleEnabled {
			bench := ftloop.NewBenchScheduler(s.tsdbClient.Pool(), ftloop.BenchScheduleConfig{
				Enabled:         true,
				IntervalDays:    s.cfg.FtBenchScheduleDays,
				InitialDelayMin: s.cfg.FtBenchScheduleInitialDelayMin,
				RepoDir:         repoDir,
				PythonBin:       filepath.Join(repoDir, "neural", ".venv", "bin", "python"),
				ConfigYAML:      s.cfg.FtLoopBenchmarkConfig,
				BaseURL:         s.cfg.FtLoopCanaryProdURL,
				ModelName:       s.cfg.FtLoopModelVersion,
				RowsPerSpec:     s.cfg.FtBenchScheduleRowsPerSpec,
			}, func(ctx context.Context, jobName string, success bool, latencyMs int64, errMsg string) {
				ev := tsdb.JobEventRow{JobName: jobName, SpaceID: s.cfg.RSICWatchdogSpaceID,
					InstanceID: s.cfg.InstanceID, Success: success, LatencyMS: latencyMs, ErrorMessage: errMsg}
				jobhealth.ReportWithService(ctx, s.tsdbClient.Pool(), s.alertDispatcher, ev, "ft-benchmark")
			})
			s.goSupervised("scheduled-ft-benchmark", bench.Run)
		}

		// FT-RECURSIVE-003 E5: post-swap tripwire (auto-rollback on elevated
		// real-error rate inside the canary window). Separate flag — the
		// watcher only ever acts inside a window opened by a promotion.
		if s.cfg.FtLoopTripwireEnabled {
			servingLink := ftServingLink(repoDir, s.cfg.FtLoopServingSymlink)
			tw := ftloop.NewTripwire(s.tsdbClient.Pool(), ftloop.TripwireConfig{
				Enabled:   true,
				Window:    time.Duration(s.cfg.FtLoopCanaryWindowMin) * time.Minute,
				ErrorRate: s.cfg.FtLoopTripwireErrorRate,
				MinCalls:  s.cfg.FtLoopTripwireMinCalls,
				PollSec:   s.cfg.FtLoopTripwirePollSec,
				Serving: ftloop.ServingConfig{
					SymlinkPath:   servingLink,
					PlistLabel:    s.cfg.FtLoopServingPlistLabel,
					HealthURL:     s.cfg.FtLoopServingHealthURL,
					HealthTimeout: time.Duration(s.cfg.FtLoopSwapHealthTimeoutSec) * time.Second,
				},
			}, func(title, detail string) {
				if s.alertDispatcher != nil {
					s.alertDispatcher.Send(context.Background(), alert.Alert{
						Service: "ft-loop-tripwire", Severity: alert.SeverityHigh,
						Title: title, Message: detail,
					})
				}
			})
			s.goSupervised("ft-loop-tripwire", tw.Run)
		}

		// AUTOGRADE-SCHEDULE-001 (2026-08-04): scheduled autograde loop —
		// closes the HITL-AUTO-DISMISS-001 + JIMINY-CONTRADICTED-BRIDGE-
		// QUALITY-001 arc so the queue self-drains without operator ceremony.
		// Default OFF; opt-in via REVIEW_AUTOGRADE_SCHEDULE_ENABLED.
		if s.cfg.ReviewAutogradeScheduleEnabled {
			endpoint := "http://127.0.0.1" + s.cfg.ListenAddr
			// ListenAddr may be ":9999" or "127.0.0.1:9999" — normalize to a
			// dial-able form for the loopback CLI POST.
			if !strings.Contains(s.cfg.ListenAddr, ":") {
				endpoint = fmt.Sprintf("http://127.0.0.1:%s", s.cfg.ListenAddr)
			} else if s.cfg.ListenAddr[0] != ':' {
				endpoint = "http://" + s.cfg.ListenAddr
			}
			spaceID := s.cfg.RSICWatchdogSpaceID
			asch := review.NewAutogradeScheduler(review.AutogradeScheduleConfig{
				Enabled:         true,
				IntervalHours:   s.cfg.ReviewAutogradeScheduleIntervalHours,
				InitialDelayMin: s.cfg.ReviewAutogradeScheduleInitialDelayMin,
				Datasets:        s.cfg.ReviewAutogradeScheduleDatasets,
				SpaceID:         spaceID,
				MinConfidence:   s.cfg.ReviewAutogradeScheduleMinConfidence,
				Limit:           s.cfg.ReviewAutogradeScheduleLimit,
				MdemgBin:        resolveMdemgBin(),
				Endpoint:        endpoint,
			}, func(ctx context.Context, jobName string, success bool, latencyMs int64, errMsg string) {
				ev := tsdb.JobEventRow{JobName: jobName, SpaceID: spaceID,
					InstanceID: s.cfg.InstanceID, Success: success, LatencyMS: latencyMs, ErrorMessage: errMsg}
				// Distinct service label per NOSILENT-001 cooldown-key contract
				// so autograde failures don't share cooldown with unrelated jobs.
				jobhealth.ReportWithService(ctx, s.tsdbClient.Pool(), s.alertDispatcher, ev, "scheduled-autograde")
			})
			s.goSupervised("scheduled-autograde", asch.Run)
		}
	}
}

// StartPeriodicConsolidation starts a background goroutine that consolidates conversation memory
// on a regular interval. Default interval is 5 minutes.
func (s *Server) StartPeriodicConsolidation(spaceID string, interval time.Duration) {
	if s.hiddenSvc == nil {
		slog.Info("periodic consolidation disabled: hidden service not available")
		return
	}
	if interval <= 0 {
		interval = 5 * time.Minute
	}

	s.stopConsolidate = make(chan struct{})
	stopCh := s.stopConsolidate
	s.goSupervised("periodic-consolidation", func(runCtx context.Context) error {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		slog.Info("periodic conversation consolidation started", "space_id", spaceID, "interval", interval)

		for {
			select {
			case <-runCtx.Done():
				return nil
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				result, err := s.hiddenSvc.RunFullConversationConsolidation(ctx, spaceID)
				cancel()
				if err != nil {
					slog.Error("periodic consolidation error", "error", err)
				} else {
					themesCreated := 0
					themesUpdated := 0
					noiseAssigned := 0
					conceptsCreated := 0
					if result.ThemeResult != nil {
						themesCreated = result.ThemeResult.ThemesCreated
						themesUpdated = result.ThemeResult.ThemesUpdated
						noiseAssigned = result.ThemeResult.NoiseAssigned
					}
					if result.ConceptResult != nil {
						for _, count := range result.ConceptResult.ConceptsCreated {
							conceptsCreated += count
						}
					}
					// Post HIDDEN-CHURN-001 stable identity, created is usually 0
					// on healthy cycles — updated/assigned must keep the log alive.
					if themesCreated > 0 || themesUpdated > 0 || noiseAssigned > 0 || conceptsCreated > 0 {
						slog.Info("periodic consolidation complete", "themes_created", themesCreated, "themes_updated", themesUpdated, "noise_assigned", noiseAssigned, "concepts_created", conceptsCreated)
					}
				}
			case <-stopCh:
				slog.Info("periodic consolidation stopped")
				return nil
			}
		}
	})
}

// StopPeriodicConsolidation stops the background consolidation goroutine
func (s *Server) StopPeriodicConsolidation() {
	if s.stopConsolidate != nil {
		close(s.stopConsolidate)
		s.stopConsolidate = nil
	}
}

// StartContextCoolerProcessing starts a background goroutine that processes
// Context Cooler graduations and decay. Default interval is 10 minutes.
func (s *Server) StartContextCoolerProcessing(spaceID string, interval time.Duration) {
	if s.contextCooler == nil {
		slog.Info("context cooler processing disabled: cooler not available")
		return
	}
	if interval <= 0 {
		interval = 10 * time.Minute
	}

	s.stopCooler = make(chan struct{})
	stopCh := s.stopCooler
	s.goSupervised("context-cooler", func(runCtx context.Context) error {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		slog.Info("context cooler processing started", "space_id", spaceID, "interval", interval)

		for {
			select {
			case <-runCtx.Done():
				return nil
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)

				// Step 1: Apply decay to inactive volatile nodes
				decayed, err := s.contextCooler.ApplyDecay(ctx, spaceID)
				if err != nil {
					slog.Error("context cooler decay error", "error", err)
				}

				// Step 2: Process graduations and tombstones
				summary, err := s.contextCooler.ProcessGraduations(ctx, spaceID)
				cancel()

				if err != nil {
					slog.Error("context cooler graduation error", "error", err)
				} else if summary.Graduated > 0 || summary.Tombstoned > 0 || decayed > 0 {
					slog.Info("context cooler cycle complete", "graduated", summary.Graduated, "tombstoned", summary.Tombstoned, "decayed", decayed, "remaining_volatile", summary.RemainingVolatile)
				}
			case <-stopCh:
				slog.Info("context cooler processing stopped")
				return nil
			}
		}
	})
}

// StopContextCoolerProcessing stops the background Context Cooler goroutine
func (s *Server) StopContextCoolerProcessing() {
	if s.stopCooler != nil {
		close(s.stopCooler)
		s.stopCooler = nil
	}
}

// StartSpacePruneScheduler starts a background goroutine that periodically
// prunes spaces marked prunable or orphaned (no TapRoot).
func (s *Server) StartSpacePruneScheduler(interval time.Duration) {
	if interval <= 0 {
		slog.Info("space prune scheduler disabled (interval=0)")
		return
	}
	s.stopSpacePrune = make(chan struct{})
	stopCh := s.stopSpacePrune
	s.goSupervised("space-prune-scheduler", func(runCtx context.Context) error {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		slog.Info("space prune scheduler started", "interval", interval)
		for {
			select {
			case <-runCtx.Done():
				return nil
			case <-ticker.C:
				pruned, deleted, errors := s.runAutoSpacePrune()
				if pruned > 0 || errors > 0 {
					slog.Info("auto-prune complete", "pruned_spaces", pruned, "deleted_nodes", deleted, "errors", errors)
				}
			case <-stopCh:
				slog.Info("space prune scheduler stopped")
				return nil
			}
		}
	})
}

// StopSpacePruneScheduler stops the background space prune goroutine.
func (s *Server) StopSpacePruneScheduler() {
	if s.stopSpacePrune != nil {
		close(s.stopSpacePrune)
		s.stopSpacePrune = nil
	}
}

// StartWeeklyGapInterviews starts a background goroutine that runs gap interviews
// on a weekly schedule. Default interval is 7 days.
func (s *Server) StartWeeklyGapInterviews(interval time.Duration) {
	if s.gapInterviewer == nil {
		slog.Info("weekly gap interviews disabled: interviewer not available")
		return
	}
	if interval <= 0 {
		interval = 7 * 24 * time.Hour // Default: weekly
	}

	s.stopInterviewer = make(chan struct{})
	stopCh := s.stopInterviewer
	s.goSupervised("weekly-gap-interviews", func(runCtx context.Context) error {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		slog.Info("weekly gap interviews started", "interval", interval)

		for {
			select {
			case <-runCtx.Done():
				return nil
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

				cfg := gaps.DefaultInterviewConfig()
				result, err := s.gapInterviewer.RunWeeklyInterview(ctx, cfg)
				cancel()

				if err != nil {
					slog.Error("weekly gap interview error", "error", err)
				} else if result.PromptsGenerated > 0 {
					slog.Info("weekly gap interview complete", "gaps_analyzed", result.TotalGapsAnalyzed, "prompts_generated", result.PromptsGenerated, "high_priority", result.HighPriorityCount)
				}
			case <-stopCh:
				slog.Info("weekly gap interviews stopped")
				return nil
			}
		}
	})
}

// StopWeeklyGapInterviews stops the background gap interview goroutine
func (s *Server) StopWeeklyGapInterviews() {
	if s.stopInterviewer != nil {
		close(s.stopInterviewer)
		s.stopInterviewer = nil
	}
}

// StartScheduledSync starts a background goroutine that periodically checks for
// stale spaces and triggers incremental re-ingestion for those with configured repo paths.
func (s *Server) StartScheduledSync(interval time.Duration) {
	if interval <= 0 {
		slog.Info("scheduled sync disabled (interval <= 0)")
		return
	}

	s.stopScheduledSync = make(chan struct{})
	stopCh := s.stopScheduledSync
	s.goSupervised("scheduled-sync", func(runCtx context.Context) error {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		slog.Info("scheduled sync started", "interval", interval, "threshold_hours", s.cfg.SyncStaleThresholdHours)

		for {
			select {
			case <-runCtx.Done():
				return nil
			case <-ticker.C:
				s.runScheduledSyncCheck()
			case <-stopCh:
				slog.Info("scheduled sync stopped")
				return nil
			}
		}
	})
}

// StopScheduledSync stops the background scheduled sync goroutine
func (s *Server) StopScheduledSync() {
	if s.stopScheduledSync != nil {
		close(s.stopScheduledSync)
		s.stopScheduledSync = nil
	}
}

// StartRSICWatchdog starts the RSIC decay watchdog.
func (s *Server) StartRSICWatchdog() {
	if s.rsicWatchdog != nil {
		s.rsicWatchdog.SetSupervise(s.superviseFn) // SUPERVISOR-002
		s.rsicWatchdog.Start()
	}
}

// StopRSICWatchdog stops the RSIC decay watchdog.
func (s *Server) StopRSICWatchdog() {
	if s.rsicWatchdog != nil {
		s.rsicWatchdog.Stop()
	}
}

// runScheduledSyncCheck queries all TapRoot nodes for staleness and triggers
// incremental re-ingestion for stale spaces with configured repo paths.
func (s *Server) runScheduledSyncCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	allFreshness, err := s.retriever.GetAllTapRootFreshness(ctx)
	if err != nil {
		slog.Error("scheduled sync: failed to query TapRoot freshness", "error", err)
		return
	}

	threshold := time.Duration(s.cfg.SyncStaleThresholdHours) * time.Hour
	filterSpaces := make(map[string]bool)
	for _, sid := range s.cfg.SyncSpaceIDs {
		filterSpaces[sid] = true
	}

	for _, props := range allFreshness {
		spaceID, _ := props["space_id"].(string)
		if spaceID == "" {
			continue
		}

		// Filter to configured space IDs if set
		if len(filterSpaces) > 0 && !filterSpaces[spaceID] {
			continue
		}

		// Check if stale
		isStale := true
		if lastIngest, ok := props["last_ingest_at"]; ok {
			var lastTime time.Time
			switch v := lastIngest.(type) {
			case time.Time:
				lastTime = v
			case string:
				if parsed, parseErr := time.Parse(time.RFC3339, v); parseErr == nil {
					lastTime = parsed
				}
			}
			if !lastTime.IsZero() {
				isStale = time.Since(lastTime) >= threshold
			}
		}

		if !isStale {
			continue
		}

		// Check if we have a repo path configured for this space
		repoPath, hasPath := s.cfg.SyncRepoPathMap[spaceID]
		if !hasPath {
			slog.Warn("scheduled sync: space is stale but no repo path configured", "space_id", spaceID)
			continue
		}

		slog.Info("scheduled sync: triggering incremental re-ingest for stale space", "space_id", spaceID, "path", repoPath)
		s.triggerScheduledIngest(spaceID, repoPath)
	}
}

// triggerScheduledIngest creates a background ingest job for a stale space.
func (s *Server) triggerScheduledIngest(spaceID, repoPath string) {
	queue := jobs.GetQueue()
	jobID := "sync-" + spaceID + "-" + time.Now().Format("20060102150405")
	config := map[string]any{
		"space_id":    spaceID,
		"path":        repoPath,
		"incremental": true,
		"since":       "HEAD~1",
	}

	job, ctx := queue.CreateJob(jobID, "scheduled-sync", config)
	go s.runIngestJob(ctx, job)

	slog.Info("scheduled sync: created job", "job_id", jobID, "space_id", spaceID)
}

// TriggerAPEEvent triggers APE modules subscribed to the given event
func (s *Server) TriggerAPEEvent(event string) {
	if s.apeScheduler != nil {
		s.apeScheduler.TriggerEvent(event)
	}
}

// TriggerAPEEventWithContext triggers APE modules subscribed to the given event,
// passing additional context (e.g., space_id, ingest_type) to module execution.
// Also dispatches to non-APE modules that declared EventSubscriptions in their manifest.
func (s *Server) TriggerAPEEventWithContext(event string, ctx map[string]string) {
	if s.apeScheduler != nil {
		s.apeScheduler.TriggerEventWithContext(event, ctx)
	}
	if s.eventDispatcher != nil {
		s.eventDispatcher.DispatchEvent(event, ctx)
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	mux.HandleFunc("/v1/embedding/health", s.handleEmbeddingHealth)
	mux.HandleFunc("/v1/memory/retrieve", s.handleRetrieve)
	mux.Handle("/v1/memory/ingest", scopedHandler(auth.ScopeWriteMemory, s.handleIngest))
	mux.Handle("/v1/memory/ingest/batch", scopedHandler(auth.ScopeWriteMemory, s.handleBatchIngest))
	mux.HandleFunc("/v1/memory/reflect", s.handleReflect)
	mux.HandleFunc("/v1/memory/stats", s.handleStats)
	mux.HandleFunc("/v1/memory/node/meta", s.handleNodeMeta)
	mux.HandleFunc("/v1/metrics", s.handleMetrics)
	mux.HandleFunc("/v1/metrics/snapshot", s.handleMetricsSnapshot)
	mux.HandleFunc("/v1/prometheus", s.handlePrometheusMetrics)
	mux.HandleFunc("/v1/metrics/trends", s.handleMetricsTrends)
	mux.Handle("/v1/memory/archive/bulk", scopedHandler(auth.ScopeDeleteMemory, s.handleBulkArchive))
	mux.HandleFunc("/v1/memory/nodes/", s.handleNodeOperation)
	mux.Handle("/v1/memory/consolidate", scopedHandler(auth.ScopeWriteMemory, s.handleConsolidate))
	mux.HandleFunc("/v1/modules", s.handleModules)
	mux.HandleFunc("/v1/modules/", s.handleModuleSync)
	mux.HandleFunc("/v1/plugins", s.handlePluginOperation)
	mux.HandleFunc("/v1/plugins/", s.handlePluginOperation)
	mux.HandleFunc("/v1/ape/status", s.handleAPEStatus)
	mux.HandleFunc("/v1/ape/trigger", s.handleAPETrigger)
	mux.HandleFunc("/v1/learning/prune", s.handleLearningPrune)
	mux.HandleFunc("/v1/learning/stats", s.handleLearningStats)
	mux.HandleFunc("/v1/learning/freeze", s.handleLearningFreeze)
	mux.HandleFunc("/v1/learning/unfreeze", s.handleLearningUnfreeze)
	mux.HandleFunc("/v1/learning/freeze/status", s.handleLearningFreezeStatus)
	mux.HandleFunc("/v1/learning/negative-feedback", s.handleNegativeFeedback)
	mux.HandleFunc("/v1/memory/frontiers", s.handleFrontierDetection)
	mux.HandleFunc("/v1/memory/consult", s.handleConsult)
	mux.HandleFunc("/v1/memory/suggest", s.handleSuggest)
	mux.HandleFunc("/v1/memory/cache/stats", s.handleCacheStats)
	mux.HandleFunc("/v1/memory/cache", s.handleCacheClear)
	mux.HandleFunc("/v1/memory/query/metrics", s.handleQueryMetrics)
	mux.HandleFunc("/v1/memory/distribution", s.handleDistributionStats)
	mux.HandleFunc("/v1/memory/symbols", s.handleSymbolSearch)
	mux.HandleFunc("/v1/memory/edges/stale/stats", s.handleStaleEdgeStats)
	mux.HandleFunc("/v1/memory/edges/stale/refresh", s.handleRefreshStaleEdges)

	// Neo4j state monitor
	mux.HandleFunc("/v1/neo4j/overview", s.handleNeo4jOverview)

	// Grafana Node Graph API endpoints (GAP-20)
	mux.HandleFunc("/v1/memory/graph/topology", s.handleGraphTopology)
	mux.HandleFunc("/v1/memory/graph/neighborhood", s.handleGraphNeighborhood)

	// Node Graph API plugin expected endpoints (GAP-20)
	mux.HandleFunc("/api/graph/data", s.handleNodeGraphData)
	mux.HandleFunc("/api/graph/fields", s.handleNodeGraphFields)
	mux.HandleFunc("/api/graph/health", s.handleNodeGraphHealth)

	// 3D topology viewer
	mux.HandleFunc("/viz/topology", s.handleVizTopology)

	// Ingestion job management endpoints
	mux.HandleFunc("/v1/memory/ingest/trigger", s.handleIngestTrigger)
	mux.HandleFunc("/v1/memory/ingest/status/", s.handleIngestStatus)
	mux.HandleFunc("/v1/memory/ingest/cancel/", s.handleIngestCancel)
	mux.HandleFunc("/v1/memory/ingest/jobs", s.handleIngestJobs)
	mux.HandleFunc("/v1/memory/ingest/files", s.handleIngestFiles)

	// Capability gap detection endpoints
	mux.HandleFunc("/v1/system/capability-gaps", s.handleCapabilityGaps)
	mux.HandleFunc("/v1/system/capability-gaps/", s.handleCapabilityGapOperation)
	// /v1/feedback pruned in DORMANT-CENSUS-001 (zero producers; the live
	// feedback channel is /v1/jiminy/feedback via post-tool-observe.py)

	// Gap interview endpoints (weekly APE job for addressing capability gaps)
	mux.HandleFunc("/v1/system/gap-interviews", s.handleGapInterviews)
	mux.HandleFunc("/v1/system/gap-interviews/", s.handleGapInterviewOperation)

	// System metrics endpoints
	mux.HandleFunc("/v1/system/pool-metrics", s.handlePoolMetrics)

	// Conversation memory endpoints (Phase 1-5: Observation Capture, Resume, Recall)
	mux.HandleFunc("/v1/conversation/observe", s.handleObserve)
	mux.HandleFunc("/v1/conversation/correct", s.handleCorrect)
	mux.HandleFunc("/v1/conversation/resume", s.handleResume)
	mux.HandleFunc("/v1/conversation/recall", s.handleRecall)
	mux.HandleFunc("/v1/conversation/consolidate", s.handleConversationConsolidate)
	mux.HandleFunc("/v1/conversation/volatile/stats", s.handleVolatileStats)
	mux.HandleFunc("/v1/conversation/graduate", s.handleProcessGraduations)
	mux.HandleFunc("/v1/conversation/session/health", s.handleSessionHealth)
	mux.HandleFunc("/v1/conversation/session/anomalies", s.handleSessionAnomalies)

	// Active MCP Guardrails (Phase 104)
	mux.HandleFunc("/v1/memory/guardrail/validate", s.handleGuardrailValidate)
	mux.HandleFunc("/v1/guardrail/events", s.handleGuardrailEvents)

	// Global Meta-Learning (Phase 105)
	mux.HandleFunc("/v1/memory/meta-learn", s.handleMetaLearn)

	// Phase Jiminy: Jiminy Guidance
	mux.HandleFunc("/v1/jiminy/healthz", s.handleJiminyHealthz)
	mux.HandleFunc("/v1/jiminy/ready", s.handleJiminyReady)
	mux.HandleFunc("/v1/jiminy/guide", s.handleJiminyGuide)
	mux.HandleFunc("/v1/jiminy/warm", s.handleJiminyWarm)
	mux.HandleFunc("/v1/jiminy/latest", s.handleJiminyLatest)
	mux.HandleFunc("/v1/jiminy/feedback", s.handleJiminyFeedback)
	mux.HandleFunc("/v1/jiminy/evaluate", s.handleJiminyEvaluate) // J9

	// J17: AI-to-AI Communication Protocol
	mux.HandleFunc("/v1/jiminy/checkpoint", s.handleJ17Checkpoint)
	mux.HandleFunc("/v1/jiminy/resume-protocol", s.handleJ17ResumeProtocol)
	mux.HandleFunc("/v1/jiminy/bootstrap", s.handleJ17Bootstrap)
	mux.HandleFunc("/v1/jiminy/protocol/metrics", s.handleJ17ProtocolMetrics)
	mux.HandleFunc("/v1/jiminy/protocol/tier-effectiveness", s.handleJ17TierEffectiveness)
	mux.HandleFunc("/v1/jiminy/protocol/feedback", s.handleJ17ProtocolFeedback)
	mux.HandleFunc("/v1/jiminy/protocol/learn", s.handleJ17ProtocolLearn)
	mux.HandleFunc("/v1/jiminy/protocol/status", s.handleJiminyProtocolStatus)
	mux.HandleFunc("/v1/jiminy/strict", s.handleJiminyStrict)
	// JIMINY-ENFORCE-003: operator escape-hatch. GET list / POST apply / DELETE revoke.
	mux.HandleFunc("/v1/jiminy/override", s.handleJiminyOverride)
	// ENFORCE-UI-OVERRIDES (2026-08-03): TSDB-backed history for the UI timeline
	// + future RSIC action-execution reads. Separate path so it doesn't collide
	// with the active-list GET on /v1/jiminy/override.
	mux.HandleFunc("/v1/jiminy/override/history", s.handleJiminyOverrideHistory)
	mux.HandleFunc("/v1/jiminy/reformulate", s.handleJiminyReformulate)
	mux.HandleFunc("/v1/jiminy/classify", s.handleJiminyClassify)
	mux.HandleFunc("/v1/jiminy/extension", s.handleJ17Extension)

	// Constraint Module (Phase 45.5)
	mux.HandleFunc("/v1/constraints", s.handleConstraintsList)
	mux.HandleFunc("/v1/constraints/stats", s.handleConstraintStats)
	mux.HandleFunc("/v1/constraints/effectiveness", s.handleConstraintEffectiveness) // F3: per-constraint effectiveness metrics
	mux.HandleFunc("/v1/constraints/scope/", s.handleConstraintScopeUpdate)          // F7: PATCH scope override

	// F9: Determinism Score
	mux.HandleFunc("/v1/metrics/determinism", s.handleDeterminismScore)

	// F4: Cross-Constraint Conflict Detection
	mux.HandleFunc("/v1/constraints/detect-conflicts", s.handleDetectConstraintConflicts)
	mux.HandleFunc("/v1/constraints/conflicts", s.handleListConstraintConflicts)
	mux.HandleFunc("/v1/constraints/conflicts/", s.handleResolveConstraintConflict) // PATCH .../conflicts/{id}/resolve

	// NR-3: Neural sidecar status
	mux.HandleFunc("/v1/neural/status", s.handleNeuralStatus)

	// CMS Templates (Phase 60)
	mux.HandleFunc("/v1/conversation/templates", s.handleTemplates)
	mux.HandleFunc("/v1/conversation/templates/", s.handleTemplateByID)

	// CMS Snapshots (Phase 60)
	mux.HandleFunc("/v1/conversation/snapshot", s.handleSnapshots)
	mux.HandleFunc("/v1/conversation/snapshot/latest", s.handleLatestSnapshot)
	mux.HandleFunc("/v1/conversation/snapshot/cleanup", s.handleCleanupSnapshots)
	mux.HandleFunc("/v1/conversation/snapshot/", s.handleSnapshotByID)

	// CMS Org Reviews (Phase 60)
	mux.HandleFunc("/v1/conversation/org-reviews", s.handleListOrgReviews)
	mux.HandleFunc("/v1/conversation/org-reviews/stats", s.handleOrgReviewStats)
	mux.HandleFunc("/v1/conversation/org-reviews/", s.handleOrgReviewDecision)
	mux.HandleFunc("/v1/conversation/observations/", s.handleFlagForOrgReview)

	// RSIC (Recursive Self-Improvement Cycle) endpoints (Phase 60b)
	mux.HandleFunc("/v1/self-improve/assess", s.handleSelfImproveAssess)
	mux.HandleFunc("/v1/self-improve/report", s.handleSelfImproveReport)
	mux.HandleFunc("/v1/self-improve/report/", s.handleSelfImproveReportByID)
	mux.HandleFunc("/v1/self-improve/cycle", s.handleSelfImproveCycle)
	mux.HandleFunc("/v1/self-improve/history", s.handleSelfImproveHistory)
	mux.HandleFunc("/v1/self-improve/calibration", s.handleSelfImproveCalibration)
	mux.Handle("/v1/self-improve/orchestration/reset", scopedHandler(auth.ScopeAdminSpaces, s.handleOrchestrationReset))
	mux.HandleFunc("/v1/self-improve/health", s.handleSelfImproveHealth)
	mux.HandleFunc("/v1/self-improve/signals", s.handleSelfImproveSignals)
	mux.Handle("/v1/self-improve/rollback", scopedHandler(auth.ScopeAdminSpaces, s.handleSelfImproveRollback))

	// Skill Registry (Phase 48)
	mux.HandleFunc("/v1/skills", s.handleSkills)
	mux.HandleFunc("/v1/skills/", s.handleSkillOperation)

	// Web Scraper (Phase 51)
	mux.HandleFunc("/v1/scraper/jobs", s.handleScraperJobs)
	mux.HandleFunc("/v1/scraper/jobs/", s.handleScraperJobByID)
	mux.HandleFunc("/v1/scraper/spaces", s.handleListScrapeSpaces)

	// Neo4j Backup & Restore (Phase 70)
	mux.Handle("/v1/backup/trigger", scopedHandler(auth.ScopeWriteSpaces, s.handleBackupTrigger))
	mux.HandleFunc("/v1/backup/status/", s.handleBackupStatus)
	mux.HandleFunc("/v1/backup/list", s.handleBackupList)
	mux.HandleFunc("/v1/backup/manifest/", s.handleBackupManifest)
	mux.Handle("/v1/backup/restore", scopedHandler(auth.ScopeAdminSpaces, s.handleBackupRestore))
	mux.HandleFunc("/v1/backup/restore/status/", s.handleRestoreStatus)
	mux.HandleFunc("/v1/backup/", s.handleBackupByID)

	// Phase 75: Symbol Relationships
	mux.HandleFunc("/v1/symbols/relationships", s.handleRelationshipStats)
	mux.HandleFunc("/v1/symbols/", s.handleSymbolRelationships)

	// Linear CRUD endpoints (Phase 4)
	mux.HandleFunc("/v1/linear/issues", s.handleLinearIssues)
	mux.HandleFunc("/v1/linear/issues/", s.handleLinearIssues)
	mux.HandleFunc("/v1/linear/projects", s.handleLinearProjects)
	mux.HandleFunc("/v1/linear/projects/", s.handleLinearProjects)
	mux.HandleFunc("/v1/linear/comments", s.handleLinearComments)

	// Cleanup endpoints (Phase 9.5)
	mux.Handle("/v1/memory/cleanup/orphans", scopedHandler(auth.ScopeWriteSpaces, s.handleCleanupOrphans))
	mux.HandleFunc("/v1/memory/cleanup/schedule", s.handleScheduleCleanup)
	mux.HandleFunc("/v1/memory/cleanup/schedules", s.handleListCleanupSchedules)
	mux.HandleFunc("/v1/memory/cleanup/stats", s.handleCleanupStats)
	mux.Handle("/v1/memory/cleanup/graph-orphans", scopedHandler(auth.ScopeWriteSpaces, s.handleGraphOrphanCleanup))

	// Webhook endpoints (Phase 9.4)
	mux.HandleFunc("/v1/webhooks/linear", s.handleLinearWebhook)
	mux.HandleFunc("/v1/webhooks/", s.handleGenericWebhook)

	// POST /v1/alerts/grafana pruned in DORMANT-CENSUS-001 (superseded by
	// the server-native alert evaluator; contactpoint was commented out)
	mux.HandleFunc("POST /v1/alerts/clear", s.handleAlertsClear)
	mux.HandleFunc("POST /v1/hooks/event", s.handleHookEvent)

	// File watcher management endpoints (Phase 9.4)
	mux.HandleFunc("/v1/filewatcher/start", s.handleFileWatcherStart)
	mux.HandleFunc("/v1/filewatcher/status", s.handleFileWatcherStatus)
	mux.HandleFunc("/v1/filewatcher/stop", s.handleFileWatcherStop)

	// Space freshness endpoints (Phase 9.2)
	mux.HandleFunc("/v1/memory/spaces/", s.handleSpacesRoute)
	mux.HandleFunc("/v1/memory/freshness", s.handleBatchFreshness)

	// /v1/memory/ingest-codebase[/] pruned in DORMANT-CENSUS-001 (deprecated
	// with Deprecation header since Phase 94; successor /v1/memory/ingest/*)

	// Training Data Export (FT-DATA Sprint)
	mux.Handle("/v1/training-data/export", scopedHandler(auth.ScopeAdminSpaces, s.handleTrainingDataExport))
	mux.HandleFunc("/v1/training-data/status/", s.handleTrainingDataStatus)
	mux.HandleFunc("/v1/training-data/download/", s.handleTrainingDataDownload)

	// SSE streaming endpoint for job progress (Phase 48.3.3)
	mux.HandleFunc("/v1/jobs/", s.handleJobStream)

	// Admin: space transfer (export/import)
	mux.HandleFunc("/v1/admin/spaces/export/preview", s.handleSpaceExportPreview)
	mux.Handle("/v1/admin/spaces/export", scopedHandler(auth.ScopeAdminSpaces, s.handleSpaceExport))
	mux.Handle("/v1/admin/spaces/import", scopedHandler(auth.ScopeAdminSpaces, s.handleSpaceImport))

	// Admin: space lifecycle management
	mux.Handle("/v1/admin/spaces/prune", scopedHandler(auth.ScopeAdminSpaces, s.handleAdminSpacePrune))
	mux.Handle("/v1/admin/spaces/", scopedHandler(auth.ScopeAdminSpaces, s.handleAdminSpaceUpdate))
	mux.HandleFunc("/v1/admin/spaces", s.handleAdminSpaces)

	// Hash Verification (Phase 38: UNTS REST API)
	mux.HandleFunc("/v1/hash-verification/register", s.handleHashVerificationRegister)
	mux.HandleFunc("/v1/hash-verification/files/", s.handleHashVerificationFileRoute)
	mux.HandleFunc("/v1/hash-verification/files", s.handleHashVerificationList)
	mux.HandleFunc("/v1/hash-verification/verify-all", s.handleHashVerificationVerifyAll)
	mux.HandleFunc("/v1/hash-verification/verify", s.handleHashVerificationVerify)
	mux.HandleFunc("/v1/hash-verification/update", s.handleHashVerificationUpdate)
	mux.HandleFunc("/v1/hash-verification/revert", s.handleHashVerificationRevert)
	mux.HandleFunc("/v1/hash-verification/scan", s.handleHashVerificationScan)

	// DOCKER-P2: Browser dashboard + admin endpoints
	mux.HandleFunc("/v1/admin/config", s.handleAdminConfig)
	mux.HandleFunc("/v1/admin/logs", s.handleAdminLogs)
	mux.HandleFunc("/v1/admin/restart", s.handleServerRestart)
	mux.HandleFunc("/v1/admin/rsic/start", s.handleRSICStart)
	mux.HandleFunc("/v1/admin/rsic/stop", s.handleRSICStop)
	mux.HandleFunc("/v1/admin/rsic/restart", s.handleRSICRestart)
	mux.HandleFunc("/v1/admin/features", s.handleFeatures)
	mux.HandleFunc("/v1/admin/features/start", s.handleFeatureLifecycle)
	mux.HandleFunc("/v1/admin/features/stop", s.handleFeatureLifecycle)
	mux.HandleFunc("/v1/admin/features/restart", s.handleFeatureLifecycle)
	mux.HandleFunc("/v1/admin/instances", s.handleAdminInstances)
	// DH-004 E4.3: circuit breaker inspection + manual reset
	mux.HandleFunc("/v1/admin/breakers", s.handleBreakersList)
	mux.HandleFunc("/v1/admin/breakers/reset", s.handleBreakersReset)
	// HITL-REVIEW-001 — review platform (admin-gated; mutates the live substrate).
	mux.Handle("/v1/review/datasets", scopedHandler(auth.ScopeAdminSpaces, s.handleReviewDatasets))
	mux.Handle("/v1/review/next", scopedHandler(auth.ScopeAdminSpaces, s.handleReviewNext))
	// HITL-CURATION-002 E1: bulk-fetch un-graded candidates without the
	// human-sampler bias — the auto-grader iterates over ALL pending items.
	mux.Handle("/v1/review/candidates", scopedHandler(auth.ScopeAdminSpaces, s.handleReviewCandidates))
	mux.Handle("/v1/review/grade", scopedHandler(auth.ScopeAdminSpaces, s.handleReviewGrade))
	mux.Handle("/v1/review/reverse", scopedHandler(auth.ScopeAdminSpaces, s.handleReviewReverse))
	mux.Handle("/v1/review/autograde-preview", scopedHandler(auth.ScopeAdminSpaces, s.handleReviewAutogradePreview)) // HITL-AUTOGRADE-PREVIEW-001
	mux.HandleFunc("/v1/eventgraph/reinforcement-neighborhood", s.handleEventgraphReinforcementNeighborhood)
	mux.HandleFunc("/v1/eventgraph/guidance-outcome-neighborhood", s.handleEventgraphGuidanceOutcomeNeighborhood)
	mux.Handle("/ui/", http.StripPrefix("/ui/", uiHandler()))

	// Synergy: Claude Code ↔ MDEMG token optimization
	mux.HandleFunc("/v1/synergy/status", s.handleSynergyStatus)

	// Wrap mux with middleware stack
	// Order (outermost to innermost):
	// 1. Compression (outermost)
	// 2. Logging
	// 3. Metrics
	// 4. CORS
	// 5. Auth
	// 6. Rate Limit
	// 7. Session Warning (innermost before handler)

	var handler http.Handler = mux

	// Session-not-resumed warning middleware (Phase 3A: CMS enforcement) - innermost
	handler = SessionResumeWarningMiddleware(handler, s.sessionTracker)

	// Rate limiting middleware (Phase 3.1)
	if s.cfg.RateLimitEnabled {
		rlSkip := map[string]bool{"/healthz": true, "/readyz": true, "/v1/metrics": true}
		rlCfg := ratelimit.Config{
			Enabled:           true,
			RequestsPerSecond: s.cfg.RateLimitRPS,
			BurstSize:         s.cfg.RateLimitBurst,
			ByIP:              s.cfg.RateLimitByIP,
			SkipEndpoints:     rlSkip,
		}
		handler = ratelimit.Middleware(rlCfg)(handler)
		slog.Info("rate limiting enabled", "rps", s.cfg.RateLimitRPS, "burst", s.cfg.RateLimitBurst, "by_ip", s.cfg.RateLimitByIP)
	}

	// Authentication middleware (Phase 3.2)
	if s.cfg.AuthEnabled {
		authSkip := make(map[string]bool)
		for _, ep := range s.cfg.AuthSkipEndpoints {
			authSkip[ep] = true
		}
		authCfg := auth.Config{
			Enabled:       true,
			Mode:          auth.AuthMode(s.cfg.AuthMode),
			APIKeys:       s.cfg.AuthAPIKeys,
			JWTSecret:     s.cfg.AuthJWTSecret,
			JWTIssuer:     s.cfg.AuthJWTIssuer,
			SkipEndpoints: authSkip,
		}
		handler = auth.Middleware(authCfg)(handler)
		slog.Info("authentication enabled", "mode", s.cfg.AuthMode)
	}

	// CORS middleware (Phase 3.2)
	// Always apply — when disabled, still allows localhost cross-port for dashboard instance switching.
	corsCfg := CORSConfig{
		Enabled:          s.cfg.CORSEnabled,
		AllowedOrigins:   s.cfg.CORSAllowedOrigins,
		AllowedMethods:   s.cfg.CORSAllowedMethods,
		AllowedHeaders:   s.cfg.CORSAllowedHeaders,
		AllowCredentials: s.cfg.CORSAllowCredentials,
		MaxAge:           86400,
	}
	handler = CORSMiddleware(corsCfg)(handler)
	if s.cfg.CORSEnabled {
		slog.Info("CORS enabled", "origins", s.cfg.CORSAllowedOrigins)
	}

	// Prometheus metrics middleware (Phase 3.3)
	if s.cfg.MetricsEnabled {
		handler = metrics.HTTPMiddleware(s.metricsRegistry)(handler)
	}

	// Logging middleware
	logCfg := LogConfig{
		Format:     s.cfg.LogFormat,
		SkipHealth: s.cfg.LogSkipHealth,
	}
	handler = LoggingMiddleware(handler, logCfg)

	// Enable gzip compression for responses > 1KB when CompressionEnabled (outermost)
	if s.cfg.CompressionEnabled {
		handler = CompressionMiddleware(handler, s.cfg.CompressionMinSize)
	}

	// Memory pressure monitoring middleware (Phase 48.4.4) - outermost
	if s.memoryPressure != nil && s.cfg.MemoryPressureEnabled {
		handler = s.memoryPressure.Middleware(handler)
	}

	return handler
}

// handleSpacesRoute routes requests under /v1/memory/spaces/{space_id}/... to the appropriate handler
func (s *Server) handleSpacesRoute(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/freshness") {
		s.handleSpaceFreshness(w, r)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
}

// handleNodeOperation routes requests under /v1/memory/nodes/{node_id}/... to the appropriate handler
// based on the path suffix and HTTP method:
//   - POST /v1/memory/nodes/{node_id}/archive   -> handleArchiveNode
//   - POST /v1/memory/nodes/{node_id}/unarchive -> handleUnarchiveNode
//   - DELETE /v1/memory/nodes/{node_id}         -> handleDeleteNode
func (s *Server) handleNodeOperation(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case strings.HasSuffix(path, "/archive"):
		s.handleArchiveNode(w, r)
	case strings.HasSuffix(path, "/unarchive"):
		s.handleUnarchiveNode(w, r)
	default:
		// DELETE /v1/memory/nodes/{node_id} - permanent deletion
		s.handleDeleteNode(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON encoding failed", "error", err)
		metrics.Metrics().CMSWriteJSONFails.Inc()
	}
}

// httpStatusClientClosedRequest — nginx-style "Client Closed Request" for a
// request that failed because the CALLER cancelled its context, not because
// the server errored. Not in net/http's constants (nginx-specific), but widely
// recognised and outside the ^5 alert regex.
// RETRIEVE-CALLER-CANCEL-001 (2026-08-04).
const httpStatusClientClosedRequest = 499

// isCallerCancelled distinguishes "the caller walked away" from "the server
// hit its own deadline or errored." Only returns true when the ROOT CAUSE is
// context.Canceled (the request context was cancelled by the client) and NOT
// context.DeadlineExceeded (the server's own timeout). Same principle as the
// llmclient `caller_canceled:` recorder tag (LLM-HEALTH-INVESTIGATION-001) —
// applied at the HTTP handler layer so caller cancellations no longer show up
// as 5xx in mdemg_http_requests_total and no longer trip high_error_rate.
func isCallerCancelled(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return errors.Is(err, context.Canceled)
}

// sanitizeError logs the detailed error for debugging but returns a generic
// message suitable for client responses. This prevents internal details
// (stack traces, file paths, database errors) from leaking to clients.
//
// Caller-cancellations log at INFO (not ERROR) — the SERVER did its job right
// up to the point the client walked away; an ERROR line for every impatient
// curl is noise, not signal.
func sanitizeError(err error, operation string) string {
	if isCallerCancelled(err) {
		slog.Info("operation cancelled by caller", "operation", operation, "error", err)
		return "request cancelled during " + operation
	}
	slog.Error("operation failed", "operation", operation, "error", err)
	return "internal error during " + operation
}

// writeInternalError writes a sanitized error response. Caller-cancellations
// return HTTP 499 (nginx "Client Closed Request"); real server errors return
// HTTP 500. The 499 status is outside the ^5 regex used by high_error_rate,
// so caller-cancels no longer trip the SLO alert.
func writeInternalError(w http.ResponseWriter, err error, operation string) {
	status := http.StatusInternalServerError
	if isCallerCancelled(err) {
		status = httpStatusClientClosedRequest
	}
	writeJSON(w, status, map[string]any{
		"error": sanitizeError(err, operation),
	})
}

// scopedHandler wraps a handler with per-route scope enforcement (GAP-16).
// When auth is disabled or the principal has no scopes (backward compat),
// the handler executes without scope checks.
func scopedHandler(scope string, h http.HandlerFunc) http.Handler {
	return auth.RequireScope(scope)(http.HandlerFunc(h))
}

// writeServiceUnavailableError writes a sanitized 503 response for upstream
// service failures (e.g. plugin/CRUD module unavailable).
func writeServiceUnavailableError(w http.ResponseWriter, err error, operation string) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{
		"error": sanitizeError(err, operation),
	})
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		slog.Error("readJSON decode failed", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return false
	}
	return true
}

// validateRequest validates a request struct using the validation package.
// Returns false and writes an error response if validation fails.
// Use after readJSON with the same pattern: if !validateRequest(w, &req) { return }
func validateRequest(w http.ResponseWriter, v any) bool {
	if err := validation.Validate(v); err != nil {
		resp := validation.FormatValidationErrors(err)
		writeJSON(w, http.StatusBadRequest, resp)
		return false
	}
	return true
}

// collectNeo4jGraphData queries Neo4j for per-space graph stats with 60s cache.
func (s *Server) collectNeo4jGraphData() []metrics.SpaceGraphData {
	s.graphMetricsCache.Lock()
	defer s.graphMetricsCache.Unlock()

	if time.Since(s.graphMetricsCache.updated) < 60*time.Second && s.graphMetricsCache.data != nil {
		return s.graphMetricsCache.data
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	// Query 1: Per-space node counts + observations
	type spaceRow struct {
		nodes, edges, observations, orphans, learningEdges int
		nullWeightEdges                                    int
		conversationCoverage                               float64
	}
	spaces := make(map[string]*spaceRow)

	collected1, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			`MATCH (n:MemoryNode)
			 WHERE n.space_id IS NOT NULL
			 WITH n.space_id AS sid,
			      count(n) AS nodes,
			      sum(CASE WHEN n.role_type = 'conversation_observation' THEN 1 ELSE 0 END) AS obs
			 RETURN sid, nodes, obs
			 ORDER BY sid`,
			nil)
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if err != nil {
		slog.Error("metrics: neo4j graph query (nodes) failed", "error", err)
		return s.graphMetricsCache.data
	}
	for _, rec := range collected1.([]*neo4j.Record) {
		sid, _ := rec.Get("sid")
		nodes, _ := rec.Get("nodes")
		obs, _ := rec.Get("obs")
		sr := &spaceRow{nodes: int(nodes.(int64)), observations: int(obs.(int64))}
		spaces[sid.(string)] = sr
	}

	// Query 2: Per-space edge counts + learning edges
	collected2, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			`MATCH (a:MemoryNode)-[r]-(b:MemoryNode)
			 WHERE a.space_id IS NOT NULL
			 WITH a.space_id AS sid,
			      count(DISTINCT r) AS edges,
			      sum(CASE WHEN type(r) = 'LEARNING' THEN 1 ELSE 0 END) AS learning
			 RETURN sid, edges, learning`,
			nil)
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if err != nil {
		slog.Error("metrics: neo4j graph query (edges) failed", "error", err)
		return s.graphMetricsCache.data
	}
	for _, rec := range collected2.([]*neo4j.Record) {
		sid, _ := rec.Get("sid")
		edges, _ := rec.Get("edges")
		learning, _ := rec.Get("learning")
		if row, ok := spaces[sid.(string)]; ok {
			row.edges = int(edges.(int64))
			row.learningEdges = int(learning.(int64))
		}
	}

	// Query 3: Per-space orphan counts
	collected3, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			`MATCH (n:MemoryNode)
			 WHERE n.space_id IS NOT NULL AND NOT (n)--()
			   AND NOT coalesce(n.is_archived, false)
			 WITH n.space_id AS sid, count(n) AS orphans
			 RETURN sid, orphans`,
			nil)
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if err != nil {
		slog.Error("metrics: neo4j graph query (orphans) failed", "error", err)
		return s.graphMetricsCache.data
	}
	for _, rec := range collected3.([]*neo4j.Record) {
		sid, _ := rec.Get("sid")
		orphans, _ := rec.Get("orphans")
		if row, ok := spaces[sid.(string)]; ok {
			row.orphans = int(orphans.(int64))
		}
	}

	// Query 4: Per-space NULL-weight abstraction edges (HIDDEN-WEIGHT-001).
	// Steady state is 0 post-backfill; any reappearance means the
	// point.distance bug class regressed at a creation site.
	collected4, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			`MATCH ()-[r:GENERALIZES|ABSTRACTS_TO]->()
			 WHERE r.space_id IS NOT NULL AND r.weight IS NULL
			 WITH r.space_id AS sid, count(r) AS nullEdges
			 RETURN sid, nullEdges`,
			nil)
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if err != nil {
		slog.Error("metrics: neo4j graph query (null-weight edges) failed", "error", err)
		return s.graphMetricsCache.data
	}
	for _, rec := range collected4.([]*neo4j.Record) {
		sid, _ := rec.Get("sid")
		nullEdges, _ := rec.Get("nullEdges")
		if row, ok := spaces[sid.(string)]; ok {
			row.nullWeightEdges = int(nullEdges.(int64))
		}
	}

	// Query 5: Per-space conversation coverage (HIDDEN-CHURN-001 PR-B) —
	// fraction of conversation observations inside the theme hierarchy.
	collected5, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			`MATCH (o:MemoryNode {role_type: 'conversation_observation'})
			 WHERE o.space_id IS NOT NULL AND NOT coalesce(o.is_archived, false)
			 WITH o.space_id AS sid, count(o) AS total,
			      SUM(CASE WHEN (o)-[:GENERALIZES]->() THEN 1 ELSE 0 END) AS themed
			 RETURN sid, total, themed`,
			nil)
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if err != nil {
		slog.Error("metrics: neo4j graph query (conversation coverage) failed", "error", err)
		return s.graphMetricsCache.data
	}
	// Spaces below the observation floor emit NO coverage gauge (sentinel
	// -1, skipped by the collector): coverage over a handful of test-space
	// observations is statistically meaningless and would alarm forever.
	covMinObs := int64(s.cfg.ConversationCoverageMinObs)
	for _, row := range spaces {
		row.conversationCoverage = -1
	}
	for _, rec := range collected5.([]*neo4j.Record) {
		sid, _ := rec.Get("sid")
		total, _ := rec.Get("total")
		themed, _ := rec.Get("themed")
		if row, ok := spaces[sid.(string)]; ok {
			if t := total.(int64); t >= covMinObs && t > 0 {
				row.conversationCoverage = float64(themed.(int64)) / float64(t)
			}
		}
	}

	// Build result with health scores
	data := make([]metrics.SpaceGraphData, 0, len(spaces))
	for sid, row := range spaces {
		health := 1.0
		if row.nodes > 0 {
			orphanRatio := float64(row.orphans) / float64(row.nodes)
			edgeDensity := 0.0
			if row.nodes > 1 {
				edgeDensity = float64(row.edges) / float64(row.nodes)
				if edgeDensity > 1.0 {
					edgeDensity = 1.0
				}
			}
			health = (1.0-orphanRatio)*0.6 + edgeDensity*0.4
		}
		data = append(data, metrics.SpaceGraphData{
			SpaceID:              sid,
			Nodes:                row.nodes,
			Edges:                row.edges,
			Observations:         row.observations,
			Orphans:              row.orphans,
			LearningEdges:        row.learningEdges,
			HealthScore:          health,
			NullWeightEdges:      row.nullWeightEdges,
			ConversationCoverage: row.conversationCoverage,
		})
	}

	s.graphMetricsCache.data = data
	s.graphMetricsCache.updated = time.Now()
	return data
}

// dockerStatsUnavailableWarn ensures the "docker CLI not found" notice is
// logged once, not on every 60s container-stats refresh.
var dockerStatsUnavailableWarn sync.Once

// collectNeo4jContainerStats gets CPU/memory from docker stats with 60s cache.
func (s *Server) collectNeo4jContainerStats() *metrics.ContainerStats {
	s.containerStatsCache.Lock()
	defer s.containerStatsCache.Unlock()

	if time.Since(s.containerStatsCache.updated) < 60*time.Second && s.containerStatsCache.data != nil {
		return s.containerStatsCache.data
	}

	containerName := s.cfg.BackupNeo4jContainer
	if containerName == "" {
		containerName = "mdemg-neo4j"
	}

	// Try multiple container name variants:
	// 1. Configured name (e.g. "mdemg-neo4j-mdemg" from ContainerNameForProject)
	// 2. That name + "-1" (Docker Compose v2 suffix)
	// 3. Docker Compose v2 standard: "{project}-neo4j-1" (project = directory name)
	candidates := []string{containerName, containerName + "-1", "mdemg-neo4j-1"}
	// Deduplicate
	seen := make(map[string]bool, len(candidates))
	var unique []string
	for _, c := range candidates {
		if !seen[c] {
			seen[c] = true
			unique = append(unique, c)
		}
	}

	// Container resource stats are optional telemetry (Neo4j CPU/mem gauges).
	// When the docker CLI can't be located — e.g. a non-Docker deployment, or
	// a launchd/systemd process with a minimal PATH that excludes the Docker
	// Desktop symlink — degrade gracefully and warn ONCE instead of erroring on
	// the 60s cache-refresh loop. The data plane (Neo4j Bolt + TSDB) is over
	// the network and unaffected.
	if !dockerbin.Available() {
		dockerStatsUnavailableWarn.Do(func() {
			slog.Warn("metrics: docker CLI not found — Neo4j container resource gauges disabled (data plane unaffected); set MDEMG_DOCKER_BIN or add docker to PATH to enable",
				"probed_env", dockerbin.EnvOverride)
		})
		return s.containerStatsCache.data
	}

	dockerExe := dockerbin.Path()
	var out []byte
	var err error
	for _, name := range unique {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		out, err = exec.CommandContext(ctx, dockerExe, "stats", name, //nolint:gosec // G204: docker path resolved by dockerbin, container name from config
			"--no-stream", "--format", "{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}").Output()
		cancel()
		if err == nil {
			break
		}
	}
	if err != nil {
		slog.Error("metrics: docker stats failed", "error", err, "tried", unique, "docker", dockerExe)
		return s.containerStatsCache.data
	}

	line := strings.TrimSpace(string(out))
	parts := strings.Split(line, "\t")
	if len(parts) < 3 {
		return s.containerStatsCache.data
	}

	stats := &metrics.ContainerStats{}

	// Parse CPU percent: "3.49%"
	stats.CPUPercent, _ = strconv.ParseFloat(strings.TrimSuffix(parts[0], "%"), 64)

	// Parse memory: "7.552GiB / 31.54GiB"
	memParts := strings.Split(parts[1], " / ")
	if len(memParts) == 2 {
		stats.MemUsed = parseDockerSize(strings.TrimSpace(memParts[0]))
		stats.MemLimit = parseDockerSize(strings.TrimSpace(memParts[1]))
	}

	// Parse memory percent: "23.94%"
	stats.MemPercent, _ = strconv.ParseFloat(strings.TrimSuffix(parts[2], "%"), 64)

	s.containerStatsCache.data = stats
	s.containerStatsCache.updated = time.Now()
	return stats
}

// parseDockerSize converts Docker size strings (e.g. "7.552GiB", "512MiB") to bytes.
func parseDockerSize(s string) float64 {
	s = strings.TrimSpace(s)
	multipliers := map[string]float64{
		"B":   1,
		"KiB": 1024,
		"MiB": 1024 * 1024,
		"GiB": 1024 * 1024 * 1024,
		"TiB": 1024 * 1024 * 1024 * 1024,
		"kB":  1000,
		"MB":  1000 * 1000,
		"GB":  1000 * 1000 * 1000,
	}
	for suffix, mult := range multipliers {
		if trimmed, ok := strings.CutSuffix(s, suffix); ok {
			val, err := strconv.ParseFloat(trimmed, 64)
			if err == nil {
				return val * mult
			}
		}
	}
	return 0
}

// handlePrometheusMetrics serves Prometheus-format metrics.
func (s *Server) handlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if s.cfg.MetricsEnabled {
		m := metrics.Metrics()

		// Collect circuit breaker metrics
		if s.cbRegistry != nil {
			// Ensure known circuit breakers are registered (they're created on-demand)
			// This ensures metrics are emitted even if services haven't been called yet
			_ = s.cbRegistry.Get("openai-embeddings")
			_ = s.cbRegistry.Get("openai-rerank")
			_ = s.cbRegistry.Get("ollama-rerank")
			_ = s.cbRegistry.Get("ollama-embeddings")
			m.CollectCircuitBreakerMetrics(s.cbRegistry)
		}

		// Collect cache hit ratio metrics
		if s.retriever != nil {
			cacheStats := map[string]map[string]any{
				"query":     s.retriever.QueryCacheStats(),
				"embedding": s.retriever.EmbeddingCacheStats(),
			}
			m.CollectCacheMetrics(cacheStats)
		}

		// Collect TSDB pgx pool metrics (real stats — TSDB-CONSUME-001)
		if s.tsdbClient != nil && s.tsdbClient.Pool() != nil {
			st := s.tsdbClient.Pool().Stat()
			m.CollectTSDBPoolMetrics(int64(st.TotalConns()), int64(st.IdleConns()),
				int64(st.AcquiredConns()), int64(st.MaxConns()), st.EmptyAcquireCount())
		}
		m.CollectRateLimitMetrics()
		m.CollectTSDBWriterStats(tsdb.AllWriterStats())

		// Collect Neo4j graph per-space metrics (Grafana Neo4j Dashboard)
		graphData := s.collectNeo4jGraphData()
		m.CollectNeo4jGraphMetrics(graphData)

		// Collect Neo4j container resource metrics (Grafana Neo4j Dashboard)
		containerStats := s.collectNeo4jContainerStats()
		m.CollectNeo4jContainerMetrics(containerStats)

		// Collect memory metrics (Phase 48.4.4)
		if s.memoryPressure != nil {
			m.CollectMemoryMetrics(s.memoryPressure.HeapUsageMB()*1024*1024, s.memoryPressure.RejectedCount())
		}

		// TSDB Sprint: Collect live RSIC/J17/Jiminy metrics (every scrape)
		if s.liveCollectors != nil {
			s.liveCollectors.CollectProtocolMetrics()
			s.liveCollectors.CollectGuidanceMetrics()
			s.liveCollectors.CollectHealthMetrics()
		}

		// TSDB Sprint: Record gauge snapshot to TSDB writer (batched)
		// The writer's auto-flush ticker handles the actual DB writes at the configured interval.
		if s.tsdbWriter != nil && s.liveCollectors != nil {
			gauges := s.liveCollectors.LastGaugeValues()
			if len(gauges) > 0 {
				s.tsdbWriter.RecordGaugeSnapshot(gauges, "live", "nominal")
			}
		}
	}

	writeJSON(w, http.StatusGone, map[string]string{"error": "prometheus endpoint removed, use /v1/metrics/snapshot"})
}

// handleMetricsSnapshot serves a JSON snapshot of all registered metrics.
func (s *Server) handleMetricsSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.metricsRecorder == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "metrics recorder not initialized"})
		return
	}
	// Refresh live gauge values before snapshot (same as pre-flush hook)
	if s.liveCollectors != nil {
		s.liveCollectors.CollectProtocolMetrics()
		s.liveCollectors.CollectGuidanceMetrics()
		s.liveCollectors.CollectHealthMetrics()
	}
	snap := s.metricsRecorder.SnapshotAll()
	writeJSON(w, http.StatusOK, map[string]any{"data": snap})
}

// CircuitBreaker returns the circuit breaker for a given service name.
// Used by embeddings and other packages to wrap external API calls.
func (s *Server) CircuitBreaker(service string) *circuitbreaker.Breaker {
	if s.cbRegistry == nil {
		return nil
	}
	return s.cbRegistry.Get(service)
}

// ftServingLink resolves the serving symlink to an absolute path.
func ftServingLink(repoDir, link string) string {
	if filepath.IsAbs(link) {
		return link
	}
	return filepath.Join(repoDir, link)
}
