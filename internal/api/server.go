package api

import (
	"context"
	"encoding/json"
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
	"mdemg/internal/backup"
	"mdemg/internal/backpressure"
	"mdemg/internal/circuitbreaker"
	"mdemg/internal/config"
	"mdemg/internal/consulting"
	"mdemg/internal/conversation"
	"mdemg/internal/embeddings"
	"mdemg/internal/filewatcher"
	"mdemg/internal/gaps"
	"mdemg/internal/guardrail"
	"mdemg/internal/jiminy"
	"mdemg/internal/llmclient"
	"mdemg/internal/hidden"
	"mdemg/internal/metalearn"
	"mdemg/internal/jobs"
	"mdemg/internal/learning"
	"mdemg/internal/metrics"
	"mdemg/internal/plugins"
	"mdemg/internal/ratelimit"
	"mdemg/internal/retrieval"
	"mdemg/internal/models"
	"mdemg/internal/scraper"
	"mdemg/internal/symbols"
	"mdemg/internal/transfer"
	"mdemg/internal/tsdb"
	"mdemg/internal/unts"
	"mdemg/internal/validation"
)

type Server struct {
	cfg             config.Config
	driver          neo4j.DriverWithContext
	retriever       *retrieval.Service
	learner         *learning.Service
	embedder        embeddings.Embedder
	anomalyDetector *anomaly.Service
	hiddenLayer     *hidden.Service
	pluginMgr       *plugins.Manager
	apeScheduler    *ape.Scheduler
	symbolStore     *symbols.Store
	consultant      *consulting.Service
	gapDetector     *gaps.GapDetector
	gapInterviewer  *gaps.GapInterviewer
	conversationSvc *conversation.Service
	contextCooler   *conversation.ContextCooler
	sessionTracker  *conversation.SessionTracker
	hiddenSvc       *hidden.Service // alias for handleConversationConsolidate
	webhookDebouncer        *linearWebhookDebouncer
	genericWebhookDebouncer *webhookDebouncer
	fileWatcherMgr          *filewatcher.Manager
	stopConsolidate         chan struct{}
	stopCooler         chan struct{}
	stopInterviewer    chan struct{}
	stopScheduledSync  chan struct{}
	stopSpacePrune     chan struct{}
	bgWg               sync.WaitGroup // tracks background goroutine completion

	// Phase 3: Production readiness components
	cbRegistry     *circuitbreaker.Registry
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
	tsdbClient      *tsdb.Client
	tsdbWriter      *tsdb.MetricWriter
	llmWriter       *tsdb.LLMInteractionWriter
	embeddingWriter          *tsdb.EmbeddingEventWriter
	retrievalWriter          *tsdb.RetrievalEventWriter
	constraintOutcomesWriter *tsdb.ConstraintOutcomesWriter

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

	// Initialize symbol store
	symStore := symbols.NewStore(driver)
	symParser, symParserErr := symbols.NewParser(symbols.ParserConfig{})
	if symParserErr != nil {
		slog.Warn("symbol parser init failed (relationship extraction disabled)", "error", symParserErr)
	}
	symResolver := symbols.NewResolver(driver)
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

	// Initialize conversation service (Phase 1: Observation Capture with Surprise Detection)
	var convSvc *conversation.Service
	var ctxCooler *conversation.ContextCooler
	if emb != nil {
		convSvc = conversation.NewServiceWithConfig(driver, emb, cfg.VectorIndexName, cfg)
		slog.Info("conversation service initialized", "vector_index", cfg.VectorIndexName, "constraint_detection", cfg.ConstraintDetectionEnabled)

		// Initialize Context Cooler (Phase 3: Graduation logic for volatile observations)
		ctxCooler = conversation.NewContextCooler(driver, cfg)
		lea.SetStabilityReinforcer(ctxCooler)
		slog.Info("context cooler initialized", "graduation_threshold", cfg.CoolerGraduationThreshold, "decay_rate", cfg.CoolerStabilityDecayRate, "constraint_protection", cfg.ConstraintProtectFromDecay)
	} else {
		slog.Info("conversation service disabled (requires embedder)")
	}

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

	// Wire circuit breaker to embedder if it supports it (OpenAI and Ollama)
	if emb != nil {
		if openAIEmb, ok := emb.(*embeddings.OpenAI); ok {
			openAIEmb.SetCircuitBreaker(cbRegistry.Get("openai-embeddings"))
			slog.Info("circuit breaker wired to OpenAI embedder")
		} else if ollamaEmb, ok := emb.(*embeddings.Ollama); ok {
			ollamaEmb.SetCircuitBreaker(cbRegistry.Get("ollama-embeddings"))
			slog.Info("circuit breaker wired to Ollama embedder")
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
		}
		guardrailVal = guardrail.NewGuardrailService(guardrailCfg, driver, emb, cbRegistry)
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
			TimeoutMs: cfg.EmergenceTimeoutMs,
			OpenAIKey: cfg.OpenAIAPIKey,
			OpenAIURL: cfg.OpenAIEndpoint,
			OllamaURL: cfg.OllamaEndpoint,
		}, cbRegistry)
		cons.SetConstraintClassifier(sharedConstraintClassifier)
		slog.Info("consulting LLM constraint classification enabled", "provider", cfg.ConsultingLLMConstraintsProvider, "model", cfg.ConsultingLLMConstraintsModel)
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
				Enabled:        true,
				Provider:       cfg.JiminySynthesisProvider,
				Model:          cfg.JiminySynthesisModel,
				MaxTokens:      cfg.JiminySynthesisMaxTokens,
				TimeoutMs:      cfg.JiminySynthesisTimeoutMs,
				OpenAIKey:      cfg.OpenAIAPIKey,
				OpenAIURL:      cfg.OpenAIEndpoint,
				OllamaURL:      cfg.OllamaEndpoint,
				Temperature:    cfg.JiminySynthesisTemperature,
				ContextMaxChars: cfg.JiminyGuidanceContextMaxChars,
				OutputMaxChars:  cfg.JiminyGuidanceOutputMaxChars,
			}
			jiminySvc.SetSynthesizer(jiminy.NewGuidanceSynthesizer(synCfg, cbRegistry))
			slog.Info("Jiminy J8/J15: LLM synthesis enabled", "provider", cfg.JiminySynthesisProvider, "model", cfg.JiminySynthesisModel, "max_tokens", cfg.JiminySynthesisMaxTokens, "timeout_ms", cfg.JiminySynthesisTimeoutMs)
		}

		// J17-2: Wire constraint code generator
		if cfg.J17CodegenEnabled && convSvc != nil {
			codegenAPIKey := cfg.OpenAIAPIKey
			codegenBaseURL := cfg.OpenAIEndpoint
			if cfg.J17CodegenProvider == "ollama" {
				codegenAPIKey = "ollama"
				codegenBaseURL = cfg.OllamaEndpoint
			}
			codegenLLM := llmclient.New(llmclient.Config{
				Provider:  cfg.J17CodegenProvider,
				Model:     cfg.J17CodegenModel,
				APIKey:    codegenAPIKey,
				BaseURL:   codegenBaseURL,
				TimeoutMs: 10000,
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
			evalBaseURL := cfg.OpenAIEndpoint
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
			}).WithContext("jiminy.evaluate_llm", "")
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
			Enabled:              cfg.BackupEnabled,
			StorageDir:           cfg.BackupStorageDir,
			FullCmd:              cfg.BackupFullCmd,
			Neo4jContainer:       cfg.BackupNeo4jContainer,
			FullIntervalHours:    cfg.BackupFullIntervalHours,
			PartialIntervalHours: cfg.BackupPartialIntervalHours,
			RetentionFullCount:   cfg.BackupRetentionFullCount,
			RetentionPartialCount: cfg.BackupRetentionPartialCount,
			RetentionMaxAgeDays:  cfg.BackupRetentionMaxAgeDays,
			RetentionMaxStorageGB: cfg.BackupRetentionMaxStorageGB,
			RetentionRunAfter:    cfg.BackupRetentionRunAfter,
		}
		exp := transfer.NewExporter(driver)
		backupSvc = backup.NewService(backupCfg, driver, exp)
		backupSched = backup.NewScheduler(backupSvc)
		backupSched.Start()
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
			RetentionCount:      cfg.TSDBBackupRetentionCount,
			RetentionMaxAgeDays: cfg.TSDBBackupRetentionMaxAgeDays,
		}
		tsdbBackupSvc = tsdb.NewTSDBBackupService(tsdbBackupCfg)
		tsdbBackupSched = tsdb.NewTSDBBackupScheduler(tsdbBackupSvc)
		tsdbBackupSched.Start()
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
			Enabled:         true,
			Provider:        cfg.RSICLLMReflectProvider,
			Model:           cfg.RSICLLMReflectModel,
			MaxTokens:       cfg.EmergenceMaxTokens,
			TimeoutMs:       cfg.EmergenceTimeoutMs,
			OpenAIKey:       cfg.OpenAIAPIKey,
			OpenAIURL:       cfg.OpenAIEndpoint,
			OllamaURL:       cfg.OllamaEndpoint,
			CompressPrompts: cfg.RSICLLMReflectCompress,
		}, cbRegistry, rsicCalibrator)
		rsicReflector.SetLLMReflector(llmReflector)
		slog.Info("RSIC LLM reflection enabled", "provider", cfg.RSICLLMReflectProvider, "model", cfg.RSICLLMReflectModel)
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

	// Phase 88: Create safety validator and snapshot store, wire to dispatcher
	safetyValidator := ape.NewSafetyValidator(driver)
	snapshotStore := ape.NewSnapshotStore(driver, cfg.RSICRollbackWindow)
	rsicDispatcher.SetSafetyValidator(safetyValidator)
	rsicDispatcher.SetSnapshotStore(snapshotStore)
	// RSIC-SK1: Wire guidance calibrator for self-calibrating guidance
	if jiminySvc != nil {
		rsicDispatcher.SetGuidanceCalibrator(&rsicGuidanceCalibrationAdapter{svc: jiminySvc})
	}
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
		rsicStore.Start(context.Background())

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

	s := &Server{
		cfg:             cfg,
		driver:          driver,
		retriever:       ret,
		learner:         lea,
		embedder:        embeddings.NilSafe(emb),
		anomalyDetector: anom,
		hiddenLayer:     hid,
		hiddenSvc:       hid,
		pluginMgr:       pluginMgr,
		apeScheduler:    apeSched,
		symbolStore:     symStore,
		symbolParser:    symParser,
		symbolResolver:  symResolver,
		consultant:      cons,
		gapDetector:     gapDet,
		gapInterviewer:  gapInt,
		conversationSvc: convSvc,
		contextCooler:   ctxCooler,
		sessionTracker:  sessTracker,
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
		orchestrationPolicy:    orchPolicy,
		snapshotStore:          snapshotStore,
		rsicStore:              rsicStore,
		scraperSvc:              scraperSvc,
		backupSvc:               backupSvc,
		backupScheduler:         backupSched,
		tsdbBackupSvc:           tsdbBackupSvc,
		tsdbBackupScheduler:     tsdbBackupSched,
		intentTranslator:        intentTrans,
		guardrailValidator:      guardrailVal,
		jiminySvc:               jiminySvc,
		warmStore:               jiminy.NewWarmStore(),
		metaLearnSvc:            metaLearnSvc,
		signalLearner:           signalLearner,
		untsRegistry:            untsReg,
		untsScanner:             untsScan,
		eventDispatcher:         plugins.NewEventDispatcher(pluginMgr),
		enforcementLog:          newEnforcementEventLog(1000),
		conflictDetector:        conflictDet,
		alertDispatcher:         alertDisp,
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

	// Phase 80: Hydrate signal learner from Neo4j and start persistence
	if err := signalLearner.HydrateSignals(context.Background()); err != nil {
		slog.Warn("signal learner: hydration failed", "error", err)
	}
	signalLearner.StartPersistence(context.Background())

	// B3: Bootstrap codification — codify constraints without codes on startup
	if cfg.J17BootstrapCodification && jiminySvc != nil {
		go func() {
			bctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			n, err := jiminySvc.BootstrapCodes(bctx, cfg.J17BootstrapSpaceID)
			if err != nil {
				slog.Warn("jiminy: bootstrap codification failed", "error", err)
			} else if n > 0 {
				slog.Info("jiminy: bootstrap codification complete", "codified", n)
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

					// Infrastructure: Neo4j pool, graph, container
					m.CollectNeo4jPoolMetrics()
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
			// Wire TSDB dataset builder for RSIC data-driven reflection
			datasetBuilder := tsdb.NewDatasetBuilder(client.Pool())
			s.rsicCycle.SetDatasetProvider(datasetBuilder)
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
			// Wire recorder into CachedEmbedder if available
			if ce, ok := s.embedder.(*embeddings.CachedEmbedder); ok {
				ce.SetRecorder(&embeddingRecorderAdapter{
					writer:         s.embeddingWriter,
					instanceID:     s.cfg.InstanceID,
					defaultSpaceID: s.cfg.RSICWatchdogSpaceID,
				})
			}
			slog.Info("tsdb: embedding event logger attached")
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
	if s.constraintOutcomesWriter != nil {
		s.constraintOutcomesWriter.Close()
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

	s.bgWg.Add(1)
	go func() {
		defer s.bgWg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		slog.Info("RSIC macro cron scheduler started", "interval", interval, "next_run", s.macroNextRun.Format(time.RFC3339))

		for {
			select {
			case <-ctx.Done():
				return
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

				go func() {
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
	}()
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
	s.bgWg.Add(1)
	go func() {
		defer s.bgWg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		slog.Info("periodic conversation consolidation started", "space_id", spaceID, "interval", interval)

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				result, err := s.hiddenSvc.RunFullConversationConsolidation(ctx, spaceID)
				cancel()
				if err != nil {
					slog.Error("periodic consolidation error", "error", err)
				} else {
					themesCreated := 0
					conceptsCreated := 0
					if result.ThemeResult != nil {
						themesCreated = result.ThemeResult.ThemesCreated
					}
					if result.ConceptResult != nil {
						for _, count := range result.ConceptResult.ConceptsCreated {
							conceptsCreated += count
						}
					}
					if themesCreated > 0 || conceptsCreated > 0 {
						slog.Info("periodic consolidation complete", "themes_created", themesCreated, "concepts_created", conceptsCreated)
					}
				}
			case <-s.stopConsolidate:
				slog.Info("periodic consolidation stopped")
				return
			}
		}
	}()
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
	s.bgWg.Add(1)
	go func() {
		defer s.bgWg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		slog.Info("context cooler processing started", "space_id", spaceID, "interval", interval)

		for {
			select {
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
			case <-s.stopCooler:
				slog.Info("context cooler processing stopped")
				return
			}
		}
	}()
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
	s.bgWg.Add(1)
	go func() {
		defer s.bgWg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		slog.Info("space prune scheduler started", "interval", interval)
		for {
			select {
			case <-ticker.C:
				pruned, deleted, errors := s.runAutoSpacePrune()
				if pruned > 0 || errors > 0 {
					slog.Info("auto-prune complete", "pruned_spaces", pruned, "deleted_nodes", deleted, "errors", errors)
				}
			case <-s.stopSpacePrune:
				slog.Info("space prune scheduler stopped")
				return
			}
		}
	}()
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
	s.bgWg.Add(1)
	go func() {
		defer s.bgWg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		slog.Info("weekly gap interviews started", "interval", interval)

		for {
			select {
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
			case <-s.stopInterviewer:
				slog.Info("weekly gap interviews stopped")
				return
			}
		}
	}()
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
	s.bgWg.Add(1)
	go func() {
		defer s.bgWg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		slog.Info("scheduled sync started", "interval", interval, "threshold_hours", s.cfg.SyncStaleThresholdHours)

		for {
			select {
			case <-ticker.C:
				s.runScheduledSyncCheck()
			case <-s.stopScheduledSync:
				slog.Info("scheduled sync stopped")
				return
			}
		}
	}()
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
	mux.HandleFunc("/v1/feedback", s.handleFeedback)

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
	mux.HandleFunc("/v1/jiminy/reformulate", s.handleJiminyReformulate)
	mux.HandleFunc("/v1/jiminy/classify", s.handleJiminyClassify)
	mux.HandleFunc("/v1/jiminy/extension", s.handleJ17Extension)

	// Constraint Module (Phase 45.5)
	mux.HandleFunc("/v1/constraints", s.handleConstraintsList)
	mux.HandleFunc("/v1/constraints/stats", s.handleConstraintStats)
	mux.HandleFunc("/v1/constraints/effectiveness", s.handleConstraintEffectiveness) // F3: per-constraint effectiveness metrics
	mux.HandleFunc("/v1/constraints/scope/", s.handleConstraintScopeUpdate)         // F7: PATCH scope override

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

	// SR-001: Grafana alert webhook
	mux.HandleFunc("POST /v1/alerts/grafana", s.handleGrafanaAlertWebhook)

	// File watcher management endpoints (Phase 9.4)
	mux.HandleFunc("/v1/filewatcher/start", s.handleFileWatcherStart)
	mux.HandleFunc("/v1/filewatcher/status", s.handleFileWatcherStatus)
	mux.HandleFunc("/v1/filewatcher/stop", s.handleFileWatcherStop)

	// Space freshness endpoints (Phase 9.2)
	mux.HandleFunc("/v1/memory/spaces/", s.handleSpacesRoute)
	mux.HandleFunc("/v1/memory/freshness", s.handleBatchFreshness)

	// Codebase ingestion endpoint
	mux.HandleFunc("/v1/memory/ingest-codebase", s.handleIngestCodebaseRoute)
	mux.HandleFunc("/v1/memory/ingest-codebase/", s.handleIngestCodebaseRoute)

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

// sanitizeError logs the detailed error for debugging but returns a generic
// message suitable for client responses. This prevents internal details
// (stack traces, file paths, database errors) from leaking to clients.
func sanitizeError(err error, operation string) string {
	// Log the full error for debugging
	slog.Error("operation failed", "operation", operation, "error", err)
	// Return generic message to client
	return "internal error during " + operation
}

// writeInternalError writes a sanitized internal server error response.
// The detailed error is logged but not exposed to the client.
func writeInternalError(w http.ResponseWriter, err error, operation string) {
	writeJSON(w, http.StatusInternalServerError, map[string]any{
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
			SpaceID:       sid,
			Nodes:         row.nodes,
			Edges:         row.edges,
			Observations:  row.observations,
			Orphans:       row.orphans,
			LearningEdges: row.learningEdges,
			HealthScore:   health,
		})
	}

	s.graphMetricsCache.data = data
	s.graphMetricsCache.updated = time.Now()
	return data
}

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

	var out []byte
	var err error
	for _, name := range unique {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		out, err = exec.CommandContext(ctx, "docker", "stats", name,
			"--no-stream", "--format", "{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}").Output()
		cancel()
		if err == nil {
			break
		}
	}
	if err != nil {
		slog.Error("metrics: docker stats failed", "error", err, "tried", unique)
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

		// Collect Neo4j pool metrics (Phase 48.4.1)
		m.CollectNeo4jPoolMetrics()

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
