package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
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

	// Phase 3: Production readiness components
	cbRegistry     *circuitbreaker.Registry
	metricsRegistry *metrics.Registry

	// Phase 48.4: Connection pooling components
	memoryPressure *backpressure.MemoryPressure

	// Phase 60: CMS Advanced II
	templateService  *conversation.TemplateService
	snapshotService  *conversation.SnapshotService
	orgReviewService *conversation.OrgReviewService

	// Phase 60b: RSIC (Recursive Self-Improvement Cycle)
	rsicCycle    *ape.CycleOrchestrator
	rsicWatchdog *ape.Watchdog

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

	// Phase 38: UNTS Hash Verification
	untsRegistry *unts.Registry
	untsScanner  *unts.Scanner

	// Phase 9.4: Event dispatch for non-APE modules
	eventDispatcher *plugins.EventDispatcher

	// FSD-2026-001: Constraint Enforcement Event Log
	enforcementLog *enforcementEventLog

	// F4: Cross-Constraint Conflict Detection
	conflictDetector *hidden.ConflictDetector

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
			log.Printf("WARNING: embedding provider %q failed to initialize: %v", cfg.EmbeddingProvider, err)
		} else {
			log.Printf("Embedding provider initialized: %s (dimensions: %d)", emb.Name(), emb.Dimensions())
		}
	} else {
		log.Printf("No embedding provider configured (set EMBEDDING_PROVIDER=openai or ollama)")
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
		log.Printf("Anomaly detection enabled (duplicate threshold: %.2f, timeout: %dms)", anomalyCfg.DuplicateThreshold, anomalyCfg.MaxCheckMs)
	}

	// Initialize hidden layer service (circuit breaker wired later after cbRegistry init)
	hid := hidden.NewService(cfg, driver, nil)
	// F18: Wire edge pruner so RunConsolidation can auto-prune excess edges when enabled
	if cfg.LearningAutoPruneExcessEnabled {
		hid.SetEdgePruner(lea)
	}
	if cfg.HiddenLayerEnabled {
		log.Printf("Hidden layer enabled (eps: %.2f, minSamples: %d, maxHidden: %d)",
			cfg.HiddenLayerClusterEps, cfg.HiddenLayerMinSamples, cfg.HiddenLayerMaxHidden)
	}
	if cfg.EmergenceEnabled {
		log.Printf("Dynamic emergence enabled (provider: %s, model: %s, minWeight: %.2f, minCluster: %d)",
			cfg.EmergenceProvider, cfg.EmergenceModel, cfg.EmergenceMinWeight, cfg.EmergenceMinClusterSize)
	}

	// Initialize symbol store
	symStore := symbols.NewStore(driver)
	symParser, symParserErr := symbols.NewParser(symbols.ParserConfig{})
	if symParserErr != nil {
		log.Printf("WARNING: symbol parser init failed (relationship extraction disabled): %v", symParserErr)
	}
	symResolver := symbols.NewResolver(driver)
	log.Printf("Symbol store initialized (parser + resolver for relationship extraction)")

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
	log.Printf("Gap detector initialized (threshold: %.2f, minOccurrences: %d)", gapCfg.LowScoreThreshold, gapCfg.MinOccurrences)

	// Initialize gap interviewer for weekly gap interview processing
	gapInt := gaps.NewGapInterviewer(driver)
	log.Printf("Gap interviewer initialized")

	// Initialize conversation service (Phase 1: Observation Capture with Surprise Detection)
	var convSvc *conversation.Service
	var ctxCooler *conversation.ContextCooler
	if emb != nil {
		convSvc = conversation.NewServiceWithConfig(driver, emb, cfg.VectorIndexName, cfg)
		log.Printf("Conversation service initialized (vector index: %s, constraint detection: %v)", cfg.VectorIndexName, cfg.ConstraintDetectionEnabled)

		// Initialize Context Cooler (Phase 3: Graduation logic for volatile observations)
		ctxCooler = conversation.NewContextCooler(driver, cfg)
		lea.SetStabilityReinforcer(ctxCooler)
		log.Printf("Context Cooler initialized (graduation: %.2f, decay: %.2f, constraint protection: %v)",
			cfg.CoolerGraduationThreshold, cfg.CoolerStabilityDecayRate, cfg.ConstraintProtectFromDecay)
	} else {
		log.Printf("Conversation service disabled (requires embedder)")
	}

	// Initialize APE scheduler
	var apeSched *ape.Scheduler
	if pluginMgr != nil {
		modules := pluginMgr.ListModules()
		log.Printf("Loaded %d plugin module(s)", len(modules))
		for _, m := range modules {
			log.Printf("  - %s (%s) [%s]", m.ID, m.Version, m.State)
		}

		// Start APE scheduler
		apeSched = ape.NewScheduler(pluginMgr)
		if err := apeSched.Start(); err != nil {
			log.Printf("WARNING: APE scheduler failed to start: %v", err)
		}
	}

	// Initialize session tracker (CMS enforcement — Phase 3A)
	sessTracker := conversation.NewSessionTracker(2 * time.Hour)
	log.Printf("Session tracker initialized (TTL: 2h)")

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
		log.Printf("Circuit breaker enabled (threshold: %d, timeout: %ds)",
			cfg.CircuitBreakerThreshold, cfg.CircuitBreakerTimeoutSec)
	}

	// Wire circuit breaker registry to services that make external API calls
	ret.SetCircuitBreakerRegistry(cbRegistry)
	hid.SetCircuitBreakerRegistry(cbRegistry)

	// Wire circuit breaker to embedder if it supports it (OpenAI and Ollama)
	if emb != nil {
		if openAIEmb, ok := emb.(*embeddings.OpenAI); ok {
			openAIEmb.SetCircuitBreaker(cbRegistry.Get("openai-embeddings"))
			log.Printf("Circuit breaker wired to OpenAI embedder")
		} else if ollamaEmb, ok := emb.(*embeddings.Ollama); ok {
			ollamaEmb.SetCircuitBreaker(cbRegistry.Get("ollama-embeddings"))
			log.Printf("Circuit breaker wired to Ollama embedder")
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
			log.Printf("Embedding rate limiting enabled (%.0f rps, burst: %d)", rps, burst)
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
		log.Printf("SME Synthesis enabled (provider: %s, model: %s, maxTokens: %d)",
			cfg.SynthesisProvider, cfg.SynthesisModel, cfg.SynthesisMaxTokens)
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
		log.Printf("Intent Translation enabled (provider: %s, model: %s, timeout: %dms)",
			cfg.IntentProvider, cfg.IntentModel, cfg.IntentTimeoutMs)
	}

	// Wire intent translator to retrieval service for BM25 query rewriting
	if intentTrans != nil {
		ret.SetIntentTranslator(intentTrans)
		log.Printf("Intent translator wired to retrieval service for BM25 rewriting")
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
		log.Printf("Active MCP Guardrails enabled (provider: %s, model: %s, maxConstraints: %d)",
			cfg.GuardrailProvider, cfg.GuardrailModel, cfg.GuardrailMaxConstraints)
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
		log.Printf("Global Meta-Learning enabled (provider: %s, model: %s, globalSpace: %s)",
			cfg.MetaLearnProvider, cfg.MetaLearnModel, cfg.MetaLearnGlobalSpaceID)
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
		log.Printf("Consulting LLM constraint classification enabled (provider: %s, model: %s)", cfg.ConsultingLLMConstraintsProvider, cfg.ConsultingLLMConstraintsModel)
	}

	// F6a: Wire LLM classifier gate into conversation service if enabled.
	// Reuses the same ConstraintClassifier instance (shared LRU cache + circuit breaker).
	if cfg.ConstraintClassifierGateEnabled && convSvc != nil && sharedConstraintClassifier != nil {
		convSvc.SetConstraintGateClassifier(&constraintGateAdapter{cc: sharedConstraintClassifier})
		log.Printf("F6a: Constraint classifier gate enabled for conversation service")
	}
	log.Printf("Consulting service initialized")

	// Phase Jiminy: Initialize Jiminy Guidance Service
	var jiminySvc *jiminy.Service
	if cfg.JiminyEnabled {
		jiminySvc = jiminy.NewService(cfg, driver, cons, emb)
		log.Printf("Jiminy guidance enabled (timeout: %dms, maxItems: %d, minConf: %.2f)",
			cfg.JiminyTimeoutMs, cfg.JiminyMaxItems, cfg.JiminyMinConfidence)

		// J7: Wire retrieval provider for full-spectrum access
		if cfg.JiminyRetrievalEnabled && ret != nil {
			jiminySvc.SetRetriever(&jiminyRetrievalAdapter{retriever: ret})
			log.Printf("Jiminy J7: retrieval pipeline enabled (topK=%d, hopDepth=%d)",
				cfg.JiminyRetrievalTopK, cfg.JiminyRetrievalHopDepth)
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
			log.Printf("Jiminy J8/J15: LLM synthesis enabled (provider=%s, model=%s, maxTokens=%d, timeout=%dms)",
				cfg.JiminySynthesisProvider, cfg.JiminySynthesisModel,
				cfg.JiminySynthesisMaxTokens, cfg.JiminySynthesisTimeoutMs)
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
			})
			codegen := jiminy.NewConstraintCodeGenerator(codegenLLM)

			// Populate collision set from existing codes in Neo4j
			existingCodes := loadExistingConstraintCodes(context.Background(), driver)
			for _, code := range existingCodes {
				codegen.RegisterExistingCode(code)
			}
			if len(existingCodes) > 0 {
				log.Printf("J17-2: Loaded %d existing constraint codes for collision avoidance", len(existingCodes))
			}

			jiminySvc.SetCodeGenerator(codegen)
			convSvc.SetCodeGenerator(codegen)
			log.Printf("J17-2: Constraint code generator enabled (provider=%s, model=%s)",
				cfg.J17CodegenProvider, cfg.J17CodegenModel)
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
			})
			evaluator := jiminySvc.GetEvaluator()
			if evaluator != nil {
				evaluator.SetLLM(evalLLM, cbRegistry)
				log.Printf("Jiminy J13: evaluator LLM enabled (provider=%s, model=%s)",
					cfg.JiminyEvaluateLLMProvider, cfg.JiminyEvaluateLLMModel)
			}
		}
	}

	// F4: Initialize conflict detector if enabled
	var conflictDet *hidden.ConflictDetector
	if cfg.ConstraintConflictDetectionEnabled {
		conflictDet = hidden.NewConflictDetector(driver, cfg)
		log.Printf("Constraint conflict detection enabled (simThreshold: %.2f, maxPairs: %d)",
			cfg.ConstraintConflictSimThreshold, cfg.ConstraintConflictMaxPairs)
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
		log.Printf("Prometheus metrics enabled (namespace: %s)", cfg.MetricsNamespace)
	}

	// Phase 48.4.4: Initialize memory pressure monitor
	memPressure := backpressure.NewMemoryPressure(uint64(cfg.MemoryPressureThresholdMB), cfg.MemoryPressureEnabled)
	if cfg.MemoryPressureEnabled {
		log.Printf("Memory pressure monitoring enabled (threshold: %dMB)", cfg.MemoryPressureThresholdMB)
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
		log.Printf("Web scraper enabled (space: %s, max_jobs: %d)", cfg.ScraperDefaultSpaceID, cfg.ScraperMaxConcurrentJobs)
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
		log.Printf("Backup enabled (storage: %s, full every %dh, partial every %dh)",
			backupCfg.StorageDir, backupCfg.FullIntervalHours, backupCfg.PartialIntervalHours)
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
	rsicMonitor := ape.NewMonitor(rsicDispatcher)
	rsicCalibrator := ape.NewCalibrator(convAdapter, cfg.RSICMaxHistoryEntries)

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
		log.Printf("RSIC LLM reflection enabled (provider: %s, model: %s)", cfg.RSICLLMReflectProvider, cfg.RSICLLMReflectModel)
	}

	// Watchdog and cycle orchestrator (watchdog trigger wired after cycle creation)
	rsicWatchdog = ape.NewWatchdog(cfg, cfg.RSICWatchdogSpaceID, nil)
	rsicCycle = ape.NewCycleOrchestrator(cfg, rsicAssessor, rsicReflector, rsicPlanner, rsicDispatcher, rsicMonitor, rsicCalibrator, rsicWatchdog)
	// Wire the watchdog's force-trigger to the cycle orchestrator
	// When force-triggered, also run consolidation deterministically (Phase 45.5)
	rsicWatchdog = ape.NewWatchdog(cfg, cfg.RSICWatchdogSpaceID, func(ctx context.Context, spaceID string, meta ape.TriggerMetadata) {
		opts := &ape.RunCycleOpts{TriggerMeta: &meta}
		if _, err := rsicCycle.RunCycle(ctx, spaceID, ape.TierMeso, opts); err != nil {
			log.Printf("[WARN] RSIC watchdog meso cycle failed: %v", err)
		}
		if cfg.ConsolidateOnWatchdogEnabled && hid != nil {
			if _, err := hid.RunConsolidation(ctx, spaceID); err != nil {
				log.Printf("RSIC watchdog: consolidation failed: %v", err)
			} else {
				log.Printf("RSIC watchdog: consolidation triggered alongside meso cycle")
			}
		}
		// Cleanup stale frozen-space entries
		if removed := lea.CleanupStaleFreezes(map[string]bool{spaceID: true}); removed > 0 {
			log.Printf("RSIC watchdog: cleaned up %d stale frozen-space entries", removed)
		}
	})
	rsicCycle = ape.NewCycleOrchestrator(cfg, rsicAssessor, rsicReflector, rsicPlanner, rsicDispatcher, rsicMonitor, rsicCalibrator, rsicWatchdog)
	log.Printf("RSIC initialized (watchdog=%v, micro=%v)", cfg.RSICWatchdogEnabled, cfg.RSICMicroEnabled)

	// Phase 80: Wire WatchdogSignalProvider for multi-dimensional monitoring
	rsicWatchdog.SetSignalProvider(&rsicWatchdogSignalAdapter{
		sessionTracker: sessTracker,
		driver:         driver,
	})

	// Phase 87: Create orchestration policy
	orchPolicy := ape.NewOrchestrationPolicy(cfg)
	rsicCycle.SetOrchestrationPolicy(orchPolicy)
	log.Printf("RSIC orchestration policy initialized (cooldown=%ds, dedupe=%ds)", cfg.RSICTriggerCooldownSec, cfg.RSICTriggerDedupeSec)

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
	log.Printf("RSIC safety enforcement initialized (rollback_window=%ds)", cfg.RSICRollbackWindow)

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
			log.Printf("[WARN] RSIC calibration hydration failed: %v", err)
		}
		if ws, err := rsicStore.LoadWatchdogState(cfg.RSICWatchdogSpaceID); err == nil && ws != nil {
			rsicWatchdog.Hydrate(ws)
		} else if err != nil {
			log.Printf("[WARN] RSIC watchdog hydration failed: %v", err)
		}
		if triggers, counters, err := rsicStore.LoadOrchestrationState(); err == nil {
			orchPolicy.Hydrate(triggers, counters)
		} else {
			log.Printf("[WARN] RSIC orchestration hydration failed: %v", err)
		}

		log.Printf("RSIC persistence initialized (flush every 30s)")
	} else {
		log.Printf("RSIC persistence disabled")
	}

	// Phase 38: Initialize UNTS Hash Verification
	var untsReg *unts.Registry
	var untsScan *unts.Scanner
	if cfg.UNTSEnabled {
		untsReg = unts.NewRegistry(cfg.UNTSBasePath)
		if err := untsReg.Load(); err != nil {
			log.Printf("WARNING: UNTS registry load failed: %v", err)
		}
		untsScan = unts.NewScanner(untsReg, cfg.UNTSBasePath)
		log.Printf("UNTS hash verification enabled (base: %s)", cfg.UNTSBasePath)
	}

	// Phase 80: Initialize signal learner
	signalLearner := ape.NewSignalLearner(cfg.MetaCogSignalDecayRate, cfg.MetaCogSignalBoostRate)
	log.Printf("Signal learner initialized (decay=%.2f, boost=%.2f)", cfg.MetaCogSignalDecayRate, cfg.MetaCogSignalBoostRate)
	// RSIC-SK1: Wire signal learner to Jiminy for guidance emission/response tracking
	if jiminySvc != nil {
		jiminySvc.SetSignalLearner(signalLearner)
	}

	return &Server{
		cfg:             cfg,
		driver:          driver,
		retriever:       ret,
		learner:         lea,
		embedder:        emb,
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
		intentTranslator:        intentTrans,
		guardrailValidator:      guardrailVal,
		jiminySvc:               jiminySvc,
		metaLearnSvc:            metaLearnSvc,
		signalLearner:           signalLearner,
		untsRegistry:            untsReg,
		untsScanner:             untsScan,
		eventDispatcher:         plugins.NewEventDispatcher(pluginMgr),
		enforcementLog:          newEnforcementEventLog(1000),
		conflictDetector:        conflictDet,
	}
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
}

// StartMacroCronScheduler starts the macro cron scheduler goroutine.
// It parses RSIC_MACRO_CRON and fires macro cycles on schedule.
func (s *Server) StartMacroCronScheduler() {
	cronExpr := s.cfg.RSICMacroCron
	if cronExpr == "" {
		log.Println("RSIC macro cron disabled (RSIC_MACRO_CRON empty)")
		return
	}

	interval := parseCronInterval(cronExpr)
	if interval <= 0 {
		log.Printf("RSIC macro cron: unrecognized expression %q, disabled", cronExpr)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.macroCronCancel = cancel
	s.macroNextRun = time.Now().Add(interval)

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		log.Printf("RSIC macro cron scheduler started (interval=%s, next=%s)", interval, s.macroNextRun.Format(time.RFC3339))

		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if now.Before(s.macroNextRun) {
					continue
				}
				s.macroNextRun = now.Add(interval)

				if s.orchestrationPolicy == nil || s.rsicCycle == nil {
					continue
				}

				// Fire macro cycle for mdemg-dev space
				spaceID := "mdemg-dev"
				decision := s.orchestrationPolicy.EvaluateTrigger(ape.TriggerMacroCron, spaceID, ape.TierMacro, "")
				if !decision.Allowed {
					log.Printf("RSIC macro cron: skipped for %s — %s", spaceID, decision.Reason)
					continue
				}

				go func() {
					opts := &ape.RunCycleOpts{TriggerMeta: &decision.Meta}
					outcome, err := s.rsicCycle.RunCycle(context.Background(), spaceID, ape.TierMacro, opts)
					if err != nil {
						s.orchestrationPolicy.CompleteCycle(spaceID, ape.TierMacro)
						log.Printf("RSIC macro cron cycle failed: %v", err)
						return
					}
					s.orchestrationPolicy.RecordTrigger(decision.Meta, spaceID, ape.TierMacro, outcome.CycleID)
					s.orchestrationPolicy.CompleteCycle(spaceID, ape.TierMacro)
					log.Printf("RSIC macro cron cycle complete: %s", outcome.CycleID)
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
		log.Println("file watcher disabled (FILE_WATCHER_ENABLED=false)")
		return
	}

	if s.cfg.FileWatcherConfigs == "" {
		log.Println("file watcher enabled but no configs (FILE_WATCHER_CONFIGS empty)")
		return
	}

	configs := filewatcher.ParseConfigs(s.cfg.FileWatcherConfigs)
	if len(configs) == 0 {
		log.Println("file watcher: no valid configs found")
		return
	}

	for _, cfg := range configs {
		cfg.OnChange = s.handleFileWatcherChange
		if err := s.fileWatcherMgr.AddWatcher(cfg); err != nil {
			log.Printf("file watcher: failed to start watcher for space %s: %v", cfg.SpaceID, err)
		}
	}

	log.Printf("file watcher: started %d watchers", len(configs))
}

// handleFileWatcherChange handles file changes from the file watcher.
func (s *Server) handleFileWatcherChange(ctx context.Context, spaceID string, files []string) {
	log.Printf("[filewatcher] %d files changed in space %s", len(files), spaceID)

	// Call the internal file ingest API
	resp, err := s.ingestFilesInternal(ctx, spaceID, files)
	if err != nil {
		log.Printf("[filewatcher] ingest failed for space %s: %v", spaceID, err)
		return
	}

	log.Printf("[filewatcher] ingested %d/%d files for space %s",
		resp.SuccessCount, resp.TotalFiles, spaceID)

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
		log.Println("periodic consolidation disabled: hidden service not available")
		return
	}
	if interval <= 0 {
		interval = 5 * time.Minute
	}

	s.stopConsolidate = make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Printf("periodic conversation consolidation started (space=%s, interval=%v)", spaceID, interval)

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				result, err := s.hiddenSvc.RunFullConversationConsolidation(ctx, spaceID)
				cancel()
				if err != nil {
					log.Printf("periodic consolidation error: %v", err)
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
						log.Printf("periodic consolidation: %d themes, %d concepts created",
							themesCreated, conceptsCreated)
					}
				}
			case <-s.stopConsolidate:
				log.Println("periodic consolidation stopped")
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
		log.Println("Context Cooler processing disabled: cooler not available")
		return
	}
	if interval <= 0 {
		interval = 10 * time.Minute
	}

	s.stopCooler = make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Printf("Context Cooler processing started (space=%s, interval=%v)", spaceID, interval)

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)

				// Step 1: Apply decay to inactive volatile nodes
				decayed, err := s.contextCooler.ApplyDecay(ctx, spaceID)
				if err != nil {
					log.Printf("Context Cooler decay error: %v", err)
				}

				// Step 2: Process graduations and tombstones
				summary, err := s.contextCooler.ProcessGraduations(ctx, spaceID)
				cancel()

				if err != nil {
					log.Printf("Context Cooler graduation error: %v", err)
				} else if summary.Graduated > 0 || summary.Tombstoned > 0 || decayed > 0 {
					log.Printf("Context Cooler: graduated=%d, tombstoned=%d, decayed=%d, remaining_volatile=%d",
						summary.Graduated, summary.Tombstoned, decayed, summary.RemainingVolatile)
				}
			case <-s.stopCooler:
				log.Println("Context Cooler processing stopped")
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
		log.Println("Space prune scheduler disabled (interval=0)")
		return
	}
	s.stopSpacePrune = make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		log.Printf("Space prune scheduler started (interval=%v)", interval)
		for {
			select {
			case <-ticker.C:
				pruned, deleted, errors := s.runAutoSpacePrune()
				if pruned > 0 || errors > 0 {
					log.Printf("[auto-prune] pruned=%d spaces, deleted=%d nodes, errors=%d", pruned, deleted, errors)
				}
			case <-s.stopSpacePrune:
				log.Println("Space prune scheduler stopped")
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
		log.Println("Weekly gap interviews disabled: interviewer not available")
		return
	}
	if interval <= 0 {
		interval = 7 * 24 * time.Hour // Default: weekly
	}

	s.stopInterviewer = make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Printf("Weekly gap interviews started (interval=%v)", interval)

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

				cfg := gaps.DefaultInterviewConfig()
				result, err := s.gapInterviewer.RunWeeklyInterview(ctx, cfg)
				cancel()

				if err != nil {
					log.Printf("Weekly gap interview error: %v", err)
				} else if result.PromptsGenerated > 0 {
					log.Printf("Weekly gap interview: analyzed=%d gaps, generated=%d prompts, high_priority=%d",
						result.TotalGapsAnalyzed, result.PromptsGenerated, result.HighPriorityCount)
				}
			case <-s.stopInterviewer:
				log.Println("Weekly gap interviews stopped")
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
		log.Println("scheduled sync disabled (interval <= 0)")
		return
	}

	s.stopScheduledSync = make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Printf("scheduled sync started (interval=%v, threshold=%dh)", interval, s.cfg.SyncStaleThresholdHours)

		for {
			select {
			case <-ticker.C:
				s.runScheduledSyncCheck()
			case <-s.stopScheduledSync:
				log.Println("scheduled sync stopped")
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
		log.Printf("scheduled sync: failed to query TapRoot freshness: %v", err)
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
			log.Printf("scheduled sync: space %s is stale but no repo path configured", spaceID)
			continue
		}

		log.Printf("scheduled sync: triggering incremental re-ingest for stale space %s (path=%s)", spaceID, repoPath)
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

	log.Printf("scheduled sync: created job %s for space %s", jobID, spaceID)
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
	mux.HandleFunc("/v1/memory/ingest", s.handleIngest)
	mux.HandleFunc("/v1/memory/ingest/batch", s.handleBatchIngest)
	mux.HandleFunc("/v1/memory/reflect", s.handleReflect)
	mux.HandleFunc("/v1/memory/stats", s.handleStats)
	mux.HandleFunc("/v1/metrics", s.handleMetrics)
	mux.HandleFunc("/v1/prometheus", s.handlePrometheusMetrics)
	mux.HandleFunc("/v1/memory/archive/bulk", s.handleBulkArchive)
	mux.HandleFunc("/v1/memory/nodes/", s.handleNodeOperation)
	mux.HandleFunc("/v1/memory/consolidate", s.handleConsolidate)
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
	mux.HandleFunc("/v1/self-improve/orchestration/reset", s.handleOrchestrationReset)
	mux.HandleFunc("/v1/self-improve/health", s.handleSelfImproveHealth)
	mux.HandleFunc("/v1/self-improve/signals", s.handleSelfImproveSignals)
	mux.HandleFunc("/v1/self-improve/rollback", s.handleSelfImproveRollback)

	// Skill Registry (Phase 48)
	mux.HandleFunc("/v1/skills", s.handleSkills)
	mux.HandleFunc("/v1/skills/", s.handleSkillOperation)

	// Web Scraper (Phase 51)
	mux.HandleFunc("/v1/scraper/jobs", s.handleScraperJobs)
	mux.HandleFunc("/v1/scraper/jobs/", s.handleScraperJobByID)
	mux.HandleFunc("/v1/scraper/spaces", s.handleListScrapeSpaces)

	// Neo4j Backup & Restore (Phase 70)
	mux.HandleFunc("/v1/backup/trigger", s.handleBackupTrigger)
	mux.HandleFunc("/v1/backup/status/", s.handleBackupStatus)
	mux.HandleFunc("/v1/backup/list", s.handleBackupList)
	mux.HandleFunc("/v1/backup/manifest/", s.handleBackupManifest)
	mux.HandleFunc("/v1/backup/restore", s.handleBackupRestore)
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
	mux.HandleFunc("/v1/memory/cleanup/orphans", s.handleCleanupOrphans)
	mux.HandleFunc("/v1/memory/cleanup/schedule", s.handleScheduleCleanup)
	mux.HandleFunc("/v1/memory/cleanup/schedules", s.handleListCleanupSchedules)
	mux.HandleFunc("/v1/memory/cleanup/stats", s.handleCleanupStats)
	mux.HandleFunc("/v1/memory/cleanup/graph-orphans", s.handleGraphOrphanCleanup)

	// Webhook endpoints (Phase 9.4)
	mux.HandleFunc("/v1/webhooks/linear", s.handleLinearWebhook)
	mux.HandleFunc("/v1/webhooks/", s.handleGenericWebhook)

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

	// SSE streaming endpoint for job progress (Phase 48.3.3)
	mux.HandleFunc("/v1/jobs/", s.handleJobStream)

	// Admin: space transfer (export/import)
	mux.HandleFunc("/v1/admin/spaces/export/preview", s.handleSpaceExportPreview)
	mux.HandleFunc("/v1/admin/spaces/export", s.handleSpaceExport)
	mux.HandleFunc("/v1/admin/spaces/import", s.handleSpaceImport)

	// Admin: space lifecycle management
	mux.HandleFunc("/v1/admin/spaces/prune", s.handleAdminSpacePrune)
	mux.HandleFunc("/v1/admin/spaces/", s.handleAdminSpaceUpdate)
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
		log.Printf("Rate limiting enabled (%.0f rps, burst: %d, by_ip: %v)",
			s.cfg.RateLimitRPS, s.cfg.RateLimitBurst, s.cfg.RateLimitByIP)
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
		log.Printf("Authentication enabled (mode: %s)", s.cfg.AuthMode)
	}

	// CORS middleware (Phase 3.2)
	if s.cfg.CORSEnabled {
		corsCfg := CORSConfig{
			Enabled:          true,
			AllowedOrigins:   s.cfg.CORSAllowedOrigins,
			AllowedMethods:   s.cfg.CORSAllowedMethods,
			AllowedHeaders:   s.cfg.CORSAllowedHeaders,
			AllowCredentials: s.cfg.CORSAllowCredentials,
			MaxAge:           86400,
		}
		handler = CORSMiddleware(corsCfg)(handler)
		log.Printf("CORS enabled (origins: %v)", s.cfg.CORSAllowedOrigins)
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
		log.Printf("[ERROR] writeJSON encoding failed: %v", err)
		metrics.Metrics().CMSWriteJSONFails.Inc()
	}
}

// sanitizeError logs the detailed error for debugging but returns a generic
// message suitable for client responses. This prevents internal details
// (stack traces, file paths, database errors) from leaking to clients.
func sanitizeError(err error, operation string) string {
	// Log the full error for debugging
	log.Printf("ERROR [%s]: %v", operation, err)
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

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		log.Printf("ERROR [readJSON]: %v", err)
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

	result, err := session.Run(ctx,
		`MATCH (n:MemoryNode)
		 WHERE n.space_id IS NOT NULL
		 WITH n.space_id AS sid,
		      count(n) AS nodes,
		      sum(CASE WHEN n.role_type = 'conversation_observation' THEN 1 ELSE 0 END) AS obs
		 RETURN sid, nodes, obs
		 ORDER BY sid`,
		nil)
	if err != nil {
		log.Printf("[metrics] neo4j graph query (nodes) failed: %v", err)
		return s.graphMetricsCache.data
	}
	for result.Next(ctx) {
		rec := result.Record()
		sid, _ := rec.Get("sid")
		nodes, _ := rec.Get("nodes")
		obs, _ := rec.Get("obs")
		s := &spaceRow{nodes: int(nodes.(int64)), observations: int(obs.(int64))}
		spaces[sid.(string)] = s
	}

	// Query 2: Per-space edge counts + learning edges
	result2, err := session.Run(ctx,
		`MATCH (a:MemoryNode)-[r]-(b:MemoryNode)
		 WHERE a.space_id IS NOT NULL
		 WITH a.space_id AS sid,
		      count(DISTINCT r) AS edges,
		      sum(CASE WHEN type(r) = 'LEARNING' THEN 1 ELSE 0 END) AS learning
		 RETURN sid, edges, learning`,
		nil)
	if err != nil {
		log.Printf("[metrics] neo4j graph query (edges) failed: %v", err)
		return s.graphMetricsCache.data
	}
	for result2.Next(ctx) {
		rec := result2.Record()
		sid, _ := rec.Get("sid")
		edges, _ := rec.Get("edges")
		learning, _ := rec.Get("learning")
		if row, ok := spaces[sid.(string)]; ok {
			row.edges = int(edges.(int64))
			row.learningEdges = int(learning.(int64))
		}
	}

	// Query 3: Per-space orphan counts
	result3, err := session.Run(ctx,
		`MATCH (n:MemoryNode)
		 WHERE n.space_id IS NOT NULL AND NOT (n)--()
		   AND NOT coalesce(n.is_archived, false)
		 WITH n.space_id AS sid, count(n) AS orphans
		 RETURN sid, orphans`,
		nil)
	if err != nil {
		log.Printf("[metrics] neo4j graph query (orphans) failed: %v", err)
		return s.graphMetricsCache.data
	}
	for result3.Next(ctx) {
		rec := result3.Record()
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// docker stats --no-stream --format '{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}'
	out, err := exec.CommandContext(ctx, "docker", "stats", containerName,
		"--no-stream", "--format", "{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}").Output()
	if err != nil {
		log.Printf("[metrics] docker stats failed: %v", err)
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
		if strings.HasSuffix(s, suffix) {
			val, err := strconv.ParseFloat(strings.TrimSuffix(s, suffix), 64)
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
	}

	metrics.MetricsHandler(s.metricsRegistry)(w, r)
}

// CircuitBreaker returns the circuit breaker for a given service name.
// Used by embeddings and other packages to wrap external API calls.
func (s *Server) CircuitBreaker(service string) *circuitbreaker.Breaker {
	if s.cbRegistry == nil {
		return nil
	}
	return s.cbRegistry.Get(service)
}
