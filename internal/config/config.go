package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"mdemg/migrations"
)

type Config struct {
	ListenAddr           string
	Neo4jURI             string
	Neo4jUser            string
	Neo4jPass            string
	Neo4jBoltPort        int // NEO4J_BOLT_PORT — preferred bolt port (default: 7687)
	Neo4jHTTPPort        int // NEO4J_HTTP_PORT — preferred HTTP port (default: 7474)
	RequiredSchemaVersion int

	VectorIndexName string
	DefaultCandidateK int
	DefaultTopK int
	DefaultHopDepth int

	MaxNeighborsPerNode int
	MaxTotalEdgesFetched int
	AllowedRelationshipTypes []string

	LearningEdgeCapPerRequest int
	LearningMinActivation     float64
	LearningEta               float64 // Hebbian learning rate (η)
	LearningMu                float64 // Hebbian decay/regularization (μ)
	LearningWMin              float64 // Minimum weight clamp
	LearningWMax              float64 // Maximum weight clamp
	LearningDecayPerDay       float64 // Time-based decay per day of inactivity
	LearningPruneThreshold    float64 // Weight threshold below which edges are pruned
	LearningMaxEdgesPerNode   int     // Max CO_ACTIVATED_WITH edges per node

	// Top-level LLM settings (cascade to all features)
	LLMProvider string // LLM_PROVIDER — top-level text-gen provider (default: "openai")
	LLMModel    string // LLM_MODEL — top-level text-gen model (default: "gpt-5-nano")

	// Embedding provider settings
	EmbeddingProvider   string // "openai", "ollama", or "" (disabled)
	OpenAIAPIKey        string
	OpenAIModel         string // default: text-embedding-3-large
	OpenAIEndpoint      string // default: https://api.openai.com/v1
	LLMEndpoint         string // LLM_ENDPOINT — override endpoint for LLM text-generation (default: uses OpenAIEndpoint)
	OllamaEndpoint      string // default: http://localhost:11434
	OllamaModel         string // default: qwen3-embedding:4b

	// Embedding cache settings
	EmbeddingCacheEnabled bool // Feature toggle (default: true)
	EmbeddingCacheSize    int  // LRU cache capacity (default: 1000)

	// Query result cache settings (Phase 10)
	QueryCacheEnabled    bool // Feature toggle (default: true)
	QueryCacheCapacity   int  // LRU cache capacity (default: 500)
	QueryCacheTTLSeconds int  // TTL in seconds (default: 300)

	// Semantic edge creation on ingest settings
	SemanticEdgeOnIngest      bool    // Feature toggle (default: true)
	SemanticEdgeTopN          int     // Max similar nodes to query (default: 5)
	SemanticEdgeMinSimilarity float64 // Minimum similarity threshold (default: 0.7)
	SemanticEdgeInitialWeight float64 // Initial edge weight (default: 0.5)

	// Batch ingest settings
	BatchIngestMaxItems int // Maximum items per batch request (default: 500)

	// HTTP server timeouts
	HTTPReadTimeout  int // Read timeout in seconds (default: 600)
	HTTPWriteTimeout int // Write timeout in seconds (default: 600)

	// Anomaly detection settings
	AnomalyDetectionEnabled bool    // Feature toggle (default: true)
	AnomalyDuplicateThreshold float64 // Vector similarity threshold for duplicates (default: 0.95)
	AnomalyOutlierStdDevs   float64 // Standard deviations for outlier detection (default: 2.0)
	AnomalyStaleDays        int     // Days after which an update is considered stale (default: 30)
	AnomalyMaxCheckMs       int     // Maximum time for anomaly checks in ms (default: 100)

	// Temporal reasoning settings (Phase 1: Time-Aware Retrieval)
	TemporalEnabled             bool    // Feature toggle for temporal reasoning (default: true)
	TemporalSoftBoostMultiplier float64 // Gamma multiplier for soft-mode recency boost (default: 3.0, range: [1.0, 10.0])
	TemporalHardFilterEnabled   bool    // Enable hard-mode time range filtering (default: true)

	// Phase 2 Temporal: Source-type-specific decay rates
	TemporalSourceTypeDecayEnabled bool    // TEMPORAL_SOURCE_TYPE_DECAY (default: false)
	ScoringRhoDocumentation        float64 // SCORING_RHO_DOCUMENTATION (default: 0.01)
	ScoringRhoConfig               float64 // SCORING_RHO_CONFIG (default: 0.03)
	ScoringRhoConversation         float64 // SCORING_RHO_CONVERSATION (default: 0.10)
	ScoringRhoChangelog            float64 // SCORING_RHO_CHANGELOG (default: 0.08)

	// Phase 2 Temporal: Stale reference detection
	TemporalStaleRefDays    int     // TEMPORAL_STALE_REF_DAYS (default: 0 = disabled)
	TemporalStaleRefMaxPen  float64 // TEMPORAL_STALE_REF_MAX_PENALTY (default: 0.15)

	// Scoring hyperparameters for retrieval ranking
	ScoringAlpha       float64 // Vector similarity weight (default: 0.60)
	ScoringBeta        float64 // Activation weight (default: 0.20)
	ScoringGamma       float64 // Recency weight (default: 0.15)
	ScoringDelta       float64 // Confidence weight (default: 0.05)
	ScoringPhi         float64 // Hub penalty coefficient (default: 0.08)
	ScoringKappa       float64 // Redundancy penalty coefficient (default: 0.12)
	ScoringRho         float64 // Recency decay rate per day (default: 0.05) - legacy, used as fallback
	ScoringRhoL0       float64 // Layer 0 decay rate per day (default: 0.05 - faster decay for files)
	ScoringRhoL1       float64 // Layer 1 decay rate per day (default: 0.02 - slower for hidden/concepts)
	ScoringRhoL2       float64 // Layer 2+ decay rate per day (default: 0.01 - slowest for abstractions)
	ScoringConfigBoost float64 // Score multiplier for config nodes (default: 1.15)
	ScoringPathBoost   float64 // Boost coefficient for path-matching nodes (default: 0.15)

	// Logging settings
	LogFormat     string // "text" (default) or "json"
	LogSkipHealth bool   // Skip logging for /healthz and /readyz endpoints (default: false)

	// Hidden layer settings (V0005)
	HiddenLayerEnabled       bool    // Feature toggle for hidden layer processing (default: true)
	HiddenLayerClusterEps    float64 // DBSCAN epsilon - max distance for neighborhood (default: 0.1)
	HiddenLayerMinSamples    int     // DBSCAN min samples to form cluster (default: 3)
	HiddenLayerMaxHidden     int     // Max hidden nodes to create per consolidation run (default: 100)
	HiddenLayerMaxClusterSize int    // Max members per cluster before splitting (default: 200)
	HiddenLayerPathGroupDepth int    // Path segments for pre-grouping (default: 2)
	HiddenLayerBatchSize     int     // Batch size for clustering (0 = no limit, default: 0)
	HiddenLayerForwardAlpha  float64 // Weight of current embedding in forward pass (default: 0.6)
	HiddenLayerForwardBeta   float64 // Weight of aggregated embedding in forward pass (default: 0.4)
	HiddenLayerBackwardSelf  float64 // Weight of self in backward pass (default: 0.2)
	HiddenLayerBackwardBase  float64 // Weight of base signal in backward pass (default: 0.5)
	HiddenLayerBackwardConc  float64 // Weight of concept signal in backward pass (default: 0.3)

	// Concept merge settings (V0007) - deduplication during consolidation
	ConceptMergeEnabled   bool    // Enable concept deduplication (default: true)
	ConceptMergeThreshold float64 // Cosine similarity threshold for merging (default: 0.90)

	// Edge-type attention settings (V0008) - query-aware edge weighting in activation spreading
	EdgeAttentionEnabled     bool    // Feature toggle (default: true)
	EdgeAttentionCoActivated float64 // Base weight for CO_ACTIVATED_WITH edges (default: 0.85)
	EdgeAttentionAssociated  float64 // Base weight for ASSOCIATED_WITH edges (default: 0.65)
	EdgeAttentionGeneralizes float64 // Base weight for GENERALIZES edges (default: 0.65)
	EdgeAttentionAbstractsTo float64 // Base weight for ABSTRACTS_TO edges (default: 0.60)
	EdgeAttentionTemporal    float64 // Base weight for TEMPORALLY_ADJACENT edges (default: 0.45)
	EdgeAttentionCodeBoost   float64 // Multiplier for CO_ACTIVATED in code queries (default: 1.2)
	EdgeAttentionArchBoost   float64 // Multiplier for hierarchical in arch queries (default: 1.5)

	// Query-Aware Expansion settings (V0009) - attention-based neighbor selection during graph expansion
	QueryAwareExpansionEnabled bool    // Feature toggle (default: true)
	QueryAwareAttentionWeight  float64 // Weight of query-node similarity vs edge weight (default: 0.5)
	NodeEmbeddingCacheSize     int     // LRU cache size for node embeddings (default: 5000)

	// Hybrid Edge Type Strategy settings (V0010) - different edge types at different hop depths
	EdgeTypeStrategy    string   // Strategy: "all", "structural_first", "learned_only", "hybrid" (default: "hybrid")
	StructuralEdgeTypes []string // Edge types for structural hops (default: ASSOCIATED_WITH, GENERALIZES, ABSTRACTS_TO)
	LearnedEdgeTypes    []string // Edge types for learned hops (default: CO_ACTIVATED_WITH)
	HybridSwitchHop     int      // Hop depth at which to switch from structural to learned (default: 1)

	// Hybrid retrieval settings (V0006)
	HybridRetrievalEnabled bool    // Enable hybrid vector+BM25 retrieval (default: true)
	BM25TopK               int     // Candidates from BM25 search (default: 100)
	BM25Weight             float64 // Weight of BM25 in RRF fusion (default: 0.3)
	VectorWeight           float64 // Weight of vector in RRF fusion (default: 0.7)
	RRFConstant            int     // RRF constant k in 1/(k+rank) formula (default: 60)

	// LLM Re-ranking settings (V0006)
	RerankEnabled   bool    // Enable LLM re-ranking (default: false)
	RerankProvider  string  // LLM provider for rerank (openai/ollama/jina)
	RerankModel     string  // Model for re-ranking (default: gpt-4o-mini)
	RerankTopN      int     // Candidates to re-rank (default: 30)
	RerankWeight    float64 // Weight of rerank score in final (default: 0.4)
	RerankTimeoutMs int     // Timeout for rerank call in ms (default: 3000)
	RerankJinaKey   string  // RERANK_JINA_API_KEY — Jina API key for cross-encoder reranking
	RerankJinaModel string  // RERANK_JINA_MODEL — Jina reranker model (default: jina-reranker-v2-base-multilingual)
	RerankJinaURL   string  // RERANK_JINA_URL — Jina API base URL (default: https://api.jina.ai/v1)

	// LLM Summary settings (semantic summaries for ingest)
	LLMSummaryEnabled   bool   // Feature toggle for LLM summaries (default: true)
	LLMSummaryProvider  string // LLM provider for summaries (openai/ollama, default: openai)
	LLMSummaryModel     string // Model for summaries (default: gpt-4o-mini)
	LLMSummaryMaxTokens int    // Max tokens per summary (default: 150)
	LLMSummaryBatchSize int    // Files per API call (default: 10)
	LLMSummaryTimeoutMs int    // Request timeout in ms (default: 30000)
	LLMSummaryCacheSize int    // Max cached summaries (default: 5000)

	// SME Synthesis settings (Phase 101)
	SynthesisEnabled   bool   // SYNTHESIS_ENABLED — enable LLM synthesis in /consult (default: false)
	SynthesisProvider  string // SYNTHESIS_PROVIDER — LLM provider for synthesis (openai/ollama, default: openai)
	SynthesisModel     string // SYNTHESIS_MODEL — model for synthesis (default: gpt-4o-mini)
	SynthesisMaxTokens int    // SYNTHESIS_MAX_TOKENS — max tokens for synthesis response (default: 2000)
	SynthesisTimeoutMs int    // SYNTHESIS_TIMEOUT_MS — timeout for synthesis call in ms (default: 30000)

	// Intent Translation settings (Phase 102)
	IntentEnabled   bool   // INTENT_ENABLED — enable query rewriting before embedding (default: false)
	IntentProvider  string // INTENT_PROVIDER — LLM provider for intent translation (openai/ollama, default: openai)
	IntentModel     string // INTENT_MODEL — model for intent translation (default: gpt-4o-mini)
	IntentMaxTokens int    // INTENT_MAX_TOKENS — max tokens for rewritten query (default: 150)
	IntentTimeoutMs int    // INTENT_TIMEOUT_MS — timeout for intent translation in ms (default: 2000)

	// Dynamic Emergence settings (Phase 103)
	EmergenceEnabled        bool    // EMERGENCE_ENABLED — enable LLM-driven concept naming (default: false)
	EmergenceProvider       string  // EMERGENCE_PROVIDER — LLM provider for naming (openai/ollama, default: openai)
	EmergenceModel          string  // EMERGENCE_MODEL — model for naming (default: gpt-4o-mini)
	EmergenceMaxTokens      int     // EMERGENCE_MAX_TOKENS — max tokens for naming response (default: 500, range 100-4000)
	EmergenceTimeoutMs      int     // EMERGENCE_TIMEOUT_MS — timeout for naming call in ms (default: 10000, min 1000)
	EmergenceMinWeight      float64 // EMERGENCE_MIN_WEIGHT — min CO_ACTIVATED_WITH weight for clustering (default: 0.3, range 0.0-1.0)
	EmergenceMinClusterSize int     // EMERGENCE_MIN_CLUSTER_SIZE — min nodes per cluster (default: 3, min 2)
	EmergenceMaxClusters    int     // EMERGENCE_MAX_CLUSTERS — max clusters per run (default: 10, min 1)

	// Active MCP Guardrails settings (Phase 104)
	GuardrailEnabled        bool   // GUARDRAIL_ENABLED — enable guardrail validation (default: false)
	GuardrailProvider       string // GUARDRAIL_PROVIDER — LLM provider for evaluation (openai/ollama, default: openai)
	GuardrailModel          string // GUARDRAIL_MODEL — model for evaluation (default: gpt-4o-mini)
	GuardrailMaxTokens      int    // GUARDRAIL_MAX_TOKENS — max tokens for evaluation response (default: 1000, range 100-4000)
	GuardrailTimeoutMs      int    // GUARDRAIL_TIMEOUT_MS — timeout for evaluation in ms (default: 5000, min 1000)
	GuardrailMaxConstraints int    // GUARDRAIL_MAX_CONSTRAINTS — max constraints per evaluation (default: 10, range 1-50)

	// Dynamic Reclassification settings
	ReclassEnabled       bool    // RECLASS_ENABLED — enable LLM-based reclassification of oversized categories (default: true)
	ReclassThreshold     float64 // RECLASS_THRESHOLD — min fraction of total nodes to trigger (default: 0.25, range: 0.05-0.90)
	ReclassMaxSampleSize int     // RECLASS_MAX_SAMPLE_SIZE — max summaries sent to LLM per category (default: 150, range: 20-500)
	ReclassMaxCategories int     // RECLASS_MAX_CATEGORIES — max sub-categories LLM may propose (default: 10, range: 3-20)
	ReclassMaxIterations int     // RECLASS_MAX_ITERATIONS — max reclassification loops until convergence (default: 5, range: 1-10)
	ReclassMaxDepth      int     // RECLASS_MAX_DEPTH — max dot-path taxonomy depth (default: 4, range: 1-10)
	ReclassProvider      string  // RECLASS_PROVIDER — LLM provider (openai/ollama, default: from EMERGENCE_PROVIDER)
	ReclassModel         string  // RECLASS_MODEL — model name (default: gpt-5.4)
	ReclassMaxTokens     int     // RECLASS_MAX_TOKENS — max response tokens (default: 2000, range: 500-8000)
	ReclassTimeoutMs     int     // RECLASS_TIMEOUT_MS — LLM call timeout in ms (default: 30000, min: 5000)

	// Global Meta-Learning settings (Phase 105)
	MetaLearnEnabled        bool   // METALEARN_ENABLED — enable cross-space concept promotion (default: false)
	MetaLearnGlobalSpaceID  string // METALEARN_GLOBAL_SPACE_ID — target global space (default: "mdemg-global")
	MetaLearnMinLayer       int    // METALEARN_MIN_LAYER — minimum layer for promotion candidates (default: 4)
	MetaLearnMinUpdateCount int    // METALEARN_MIN_UPDATE_COUNT — minimum update_count for candidates (default: 5)
	MetaLearnProvider       string // METALEARN_PROVIDER — LLM provider for generalization (openai/ollama, default: from EMERGENCE_PROVIDER)
	MetaLearnModel          string // METALEARN_MODEL — model for generalization (default: from EMERGENCE_MODEL)
	MetaLearnMaxTokens      int    // METALEARN_MAX_TOKENS — max tokens for generalization response (default: 500, range 100-4000)
	MetaLearnTimeoutMs      int    // METALEARN_TIMEOUT_MS — timeout for generalization in ms (default: 15000, min 1000)

	// Jiminy Guidance settings (Phase Jiminy)
	JiminyEnabled          bool    // JIMINY_ENABLED — enable Jiminy inner voice guidance (default: true)
	JiminyTimeoutMs        int     // JIMINY_TIMEOUT_MS — overall timeout for Guide() in ms (default: 6000)
	JiminyMaxItems         int     // JIMINY_MAX_ITEMS — max guidance items returned (default: 10)
	JiminyMinConfidence    float64 // JIMINY_MIN_CONFIDENCE — minimum confidence to include item (default: 0.3)
	JiminyIncludeFrontiers bool    // JIMINY_INCLUDE_FRONTIERS — include frontier node suggestions (default: true)
	JiminyFrontierMinSim          float64 // JIMINY_FRONTIER_MIN_SIM — min similarity for frontier nodes (default: 0.5)
	JiminyEffectivenessEnabled    bool    // JIMINY_EFFECTIVENESS_ENABLED — enable guidance effectiveness tracking (default: true)
	JiminyEffectivenessTTLSec     int     // JIMINY_EFFECTIVENESS_TTL_SEC — TTL for tracked guidance in seconds (default: 1800)

	// Plugin system settings (V0006)
	PluginsEnabled  bool   // Feature toggle for plugin system (default: true)
	PluginsDir      string // Path to plugins directory (default: ./plugins)
	PluginSocketDir string // Path to Unix socket directory (default: /tmp/mdemg-plugins)
	MdemgVersion    string // MDEMG version string for handshake

	// Linear integration settings (Phase 4)
	LinearTeamID      string // Default team ID for issue creation
	LinearWorkspaceID string // Workspace identifier

	// Linear webhook settings (Phase 9.4)
	LinearWebhookSecret  string // LINEAR_WEBHOOK_SECRET — HMAC-SHA256 signing secret
	LinearWebhookSpaceID string // LINEAR_WEBHOOK_SPACE_ID — space for webhook observations

	// Generic webhook settings (Phase 9.4)
	WebhookConfigs string // WEBHOOK_CONFIGS — format: "source:secret:space_id,source2:secret2:space_id2"

	// File watcher settings (Phase 9.4)
	FileWatcherEnabled bool   // FILE_WATCHER_ENABLED — enable in-process file watching (default: false)
	FileWatcherConfigs string // FILE_WATCHER_CONFIGS — format: "space_id:/path:extensions:debounce_ms,..."

	// Conflict logging settings (Phase 9.5)
	ConflictLogEnabled bool // CONFLICT_LOG_ENABLED — enable structured conflict logging (default: true)

	// Scheduled orphan cleanup settings (Phase 9.5)
	OrphanCleanupIntervalHours int // ORPHAN_CLEANUP_INTERVAL_HOURS — scheduled cleanup interval (0=disabled)

	// Optimistic retry settings (Phase 47)
	OptimisticRetryEnabled     bool    // OPTIMISTIC_RETRY_ENABLED — enable optimistic locking with retry (default: true)
	OptimisticRetryMaxAttempts int     // OPTIMISTIC_RETRY_MAX_ATTEMPTS — max retry attempts (default: 5)
	OptimisticRetryBaseDelayMs int     // OPTIMISTIC_RETRY_BASE_DELAY_MS — initial delay in ms (default: 10)
	OptimisticRetryMaxDelayMs  int     // OPTIMISTIC_RETRY_MAX_DELAY_MS — max delay in ms (default: 1000)
	OptimisticRetryMultiplier  float64 // OPTIMISTIC_RETRY_MULTIPLIER — exponential backoff multiplier (default: 2.0)

	// Edge staleness settings (Phase 47)
	EdgeStalenessCascadeEnabled   bool    // EDGE_STALENESS_CASCADE_ENABLED — enable edge staleness cascade (default: true)
	EdgeStalenessRefreshBatchSize int     // EDGE_STALENESS_REFRESH_BATCH_SIZE — edges per refresh call (default: 100)
	EdgeStalenessReclusterThresh  float64 // EDGE_STALENESS_RECLUSTER_THRESHOLD — centroid drift threshold (default: 0.3)

	// Capability gap detection settings (Task #23)
	GapLowScoreThreshold   float64 // Queries below this avg score are considered poor (default: 0.5)
	GapMinOccurrences      int     // Min occurrences to create a gap (default: 3)
	GapAnalysisWindowHours int     // Time window for pattern analysis in hours (default: 24)
	GapMetricsWindowSize   int     // Number of queries to keep in history (default: 1000)

	// RSIC (Recursive Self-Improvement Cycle) settings (Phase 60b)
	RSICMicroEnabled       bool    // RSIC_MICRO_ENABLED — enable per-request micro cycles (default: false)
	RSICMesoPeriodHours    int     // RSIC_MESO_PERIOD_HOURS — hours between meso cycles (default: 6)
	RSICMesoPeriodSessions int     // RSIC_MESO_PERIOD_SESSIONS — sessions between meso cycles (default: 10)
	RSICMacroCron          string  // RSIC_MACRO_CRON — cron expression for macro cycles (default: "0 3 * * 0")
	RSICMaxNodePrunePct    float64 // RSIC_MAX_NODE_PRUNE_PCT — max % of nodes a single action can prune (default: 0.05)
	RSICMaxEdgePrunePct    float64 // RSIC_MAX_EDGE_PRUNE_PCT — max % of edges a single action can prune (default: 0.10)
	RSICRollbackWindow     int     // RSIC_ROLLBACK_WINDOW — seconds to keep rollback snapshots (default: 3600)
	RSICWatchdogEnabled    bool    // RSIC_WATCHDOG_ENABLED — enable decay watchdog (default: true)
	RSICWatchdogCheckSec   int     // RSIC_WATCHDOG_CHECK_SEC — seconds between watchdog checks (default: 300)
	RSICWatchdogDecayRate  float64 // RSIC_WATCHDOG_DECAY_RATE — decay score increase per hour without cycle (default: 0.1)
	RSICNudgeThreshold     float64 // RSIC_NUDGE_THRESHOLD — decay score for nudge-level escalation (default: 0.3)
	RSICWarnThreshold      float64 // RSIC_WARN_THRESHOLD — decay score for warn-level escalation (default: 0.6)
	RSICForceThreshold     float64 // RSIC_FORCE_THRESHOLD — decay score for force-trigger escalation (default: 0.9)
	RSICCalibrationDays    int     // RSIC_CALIBRATION_DAYS — days of history for calibration (default: 30)
	RSICMaxHistoryEntries  int     // RSIC_MAX_HISTORY_ENTRIES — max calibration history entries per type (default: 1000)
	RSICMinConfidence      float64 // RSIC_MIN_CONFIDENCE — minimum confidence to execute an action (default: 0.3)
	RSICTriggerCooldownSec  int     // RSIC_TRIGGER_COOLDOWN_SEC — cooldown between triggers from same source (default: 300)
	RSICTriggerDedupeSec    int     // RSIC_TRIGGER_DEDUPE_SEC — dedupe window for identical trigger IDs (default: 600)
	RSICWatchdogSpaceID     string  // RSIC_WATCHDOG_SPACE_ID — space monitored by watchdog (default: "mdemg-dev")
	RSICPersistenceEnabled  bool    // RSIC_PERSISTENCE_ENABLED — enable write-behind persistence (default: true)
	SpacePruneIntervalHours int    // SPACE_PRUNE_INTERVAL_HOURS — auto-prune interval in hours (default: 24, 0=disabled)

	// Phase AR-3: LLM-powered RSIC reflection
	RSICLLMReflectEnabled  bool   // RSIC_LLM_REFLECT_ENABLED — enable LLM-powered reflection (default: false)
	RSICLLMReflectProvider string // RSIC_LLM_REFLECT_PROVIDER — LLM provider (openai/ollama, default: from EMERGENCE_PROVIDER)
	RSICLLMReflectModel    string // RSIC_LLM_REFLECT_MODEL — model for reflection (default: from EMERGENCE_MODEL)

	// Phase AR-3: LLM-powered constraint classification
	ConsultingLLMConstraintsEnabled  bool   // CONSULTING_LLM_CONSTRAINTS_ENABLED — enable LLM constraint classification (default: false)
	ConsultingLLMConstraintsProvider string // CONSULTING_LLM_CONSTRAINTS_PROVIDER — LLM provider (default: from EMERGENCE_PROVIDER)
	ConsultingLLMConstraintsModel    string // CONSULTING_LLM_CONSTRAINTS_MODEL — model for classification (default: from EMERGENCE_MODEL)

	// Phase AR-3: LLM-powered query classification
	RetrievalLLMClassifyEnabled  bool   // RETRIEVAL_LLM_CLASSIFY_ENABLED — enable LLM query classification (default: false)
	RetrievalLLMClassifyProvider string // RETRIEVAL_LLM_CLASSIFY_PROVIDER — LLM provider (default: from EMERGENCE_PROVIDER)
	RetrievalLLMClassifyModel    string // RETRIEVAL_LLM_CLASSIFY_MODEL — model for classification (default: from EMERGENCE_MODEL)

	// Phase 38: UNTS Hash Verification
	UNTSEnabled  bool   // UNTS_ENABLED — enable hash verification REST API (default: false)
	UNTSBasePath string // UNTS_BASE_PATH — repository root for file hashing (default: ".")

	// Context Cooler tuning (Phase 45.5)
	CoolerReinforcementWindowHours  int     // COOLER_REINFORCEMENT_WINDOW_HOURS — reinforcement window (default: 2)
	CoolerStabilityIncreasePerReinf float64 // COOLER_STABILITY_INCREASE_PER_REINFORCEMENT (default: 0.15)
	CoolerStabilityDecayRate        float64 // COOLER_STABILITY_DECAY_RATE — daily decay for unreinforced nodes (default: 0.1)
	CoolerTombstoneThreshold        float64 // COOLER_TOMBSTONE_THRESHOLD — stability below which nodes are tombstoned (default: 0.05)
	CoolerGraduationThreshold       float64 // COOLER_GRADUATION_THRESHOLD — stability for graduation (default: 0.8)

	// Constraint Module (Phase 45.5)
	ConstraintDetectionEnabled bool    // CONSTRAINT_DETECTION_ENABLED — enable constraint detection in observations (default: true)
	ConstraintMinConfidence    float64 // CONSTRAINT_MIN_CONFIDENCE — minimum confidence to tag as constraint (default: 0.6)
	ConstraintProtectFromDecay bool    // CONSTRAINT_PROTECT_FROM_DECAY — protect constraint-tagged obs from tombstoning (default: true)

	// Web Scraper Module (Phase 51)
	ScraperEnabled            bool   // SCRAPER_ENABLED — enable web scraper module (default: false)
	ScraperDefaultSpaceID     string // SCRAPER_DEFAULT_SPACE_ID — default target space for scraped content (default: "web-scraper")
	ScraperMaxConcurrentJobs  int    // SCRAPER_MAX_CONCURRENT_JOBS — max concurrent scrape jobs (default: 3)
	ScraperDefaultDelayMs     int    // SCRAPER_DEFAULT_DELAY_MS — default delay between requests in ms (default: 1000)
	ScraperDefaultTimeoutMs   int    // SCRAPER_DEFAULT_TIMEOUT_MS — default HTTP timeout in ms (default: 30000)
	ScraperCacheTTLSeconds    int    // SCRAPER_CACHE_TTL_SECONDS — robots.txt cache TTL in seconds (default: 3600)
	ScraperRespectRobotsTxt   bool   // SCRAPER_RESPECT_ROBOTS_TXT — respect robots.txt (default: true)
	ScraperMaxContentLengthKB int    // SCRAPER_MAX_CONTENT_LENGTH_KB — max content length in KB (default: 500)

	// Neo4j Backup & Restore (Phase 70)
	BackupEnabled              bool   // BACKUP_ENABLED — enable backup module (default: true)
	BackupStorageDir           string // BACKUP_STORAGE_DIR — directory for backup artifacts (default: ".mdemg/backups")
	BackupFullCmd              string // BACKUP_FULL_CMD — command for full backups (default: "docker")
	BackupNeo4jContainer       string // BACKUP_NEO4J_CONTAINER — Docker container name (default: "mdemg-neo4j")
	BackupFullIntervalHours    int    // BACKUP_FULL_INTERVAL_HOURS — hours between full backups (default: 168)
	BackupPartialIntervalHours int    // BACKUP_PARTIAL_INTERVAL_HOURS — hours between partial backups (default: 24)
	BackupRetentionFullCount   int    // BACKUP_RETENTION_FULL_COUNT — keep last N full backups (default: 4)
	BackupRetentionPartialCount int   // BACKUP_RETENTION_PARTIAL_COUNT — keep last N partial backups (default: 14)
	BackupRetentionMaxAgeDays  int    // BACKUP_RETENTION_MAX_AGE_DAYS — delete backups older than N days (default: 90)
	BackupRetentionMaxStorageGB int   // BACKUP_RETENTION_MAX_STORAGE_GB — storage quota in GB (default: 50)
	BackupRetentionRunAfter    bool   // BACKUP_RETENTION_RUN_AFTER_BACKUP — run retention after each backup (default: true)

	// Phase 75: Relationship Extraction
	RelExtractImports     bool    // REL_EXTRACT_IMPORTS — extract import relationships (default: true)
	RelExtractInheritance bool    // REL_EXTRACT_INHERITANCE — extract inheritance relationships (default: true)
	RelExtractCalls       bool    // REL_EXTRACT_CALLS — extract function call relationships (default: true)
	RelCrossFileResolve   bool    // REL_CROSS_FILE_RESOLVE — enable cross-file symbol resolution (default: true)
	GoTypesEnabled        bool    // GO_TYPES_ANALYSIS_ENABLED — use go/types for accurate analysis (default: false)
	RelMaxCallsPerFunc    int     // REL_MAX_CALLS_PER_FUNCTION — max calls extracted per function (default: 50)
	RelBatchSize          int     // REL_BATCH_SIZE — batch size for relationship insertion (default: 500)
	RelResolutionTimeout  int     // REL_RESOLUTION_TIMEOUT_SEC — timeout for symbol resolution in seconds (default: 60)

	// Phase 75B: Topology Hardening
	DynamicEdgesEnabled      bool    // DYNAMIC_EDGES_ENABLED — enable dynamic edge creation during retrieval (default: true)
	DynamicEdgeDegreeCap     int     // DYNAMIC_EDGE_DEGREE_CAP — max dynamic edges per node (default: 10)
	DynamicEdgeMinConfidence float64 // DYNAMIC_EDGE_MIN_CONFIDENCE — minimum confidence for dynamic edges (default: 0.5)
	L5EmergentEnabled        bool    // L5_EMERGENT_ENABLED — enable Layer 5 emergent concept nodes (default: true)
	L5BridgeEvidenceMin      int     // L5_BRIDGE_EVIDENCE_MIN — minimum bridge evidence for L5 promotion (default: 1)
	L5SourceMinLayer         int     // L5_SOURCE_MIN_LAYER — minimum layer for L5/dynamic edge source nodes (default: 3)
	SymbolActivationEnabled  bool    // SYMBOL_ACTIVATION_ENABLED — enable symbol-aware activation boost (default: true)
	SecondaryLabelsEnabled   bool    // SECONDARY_LABELS_ENABLED — enable secondary node labels (default: true)
	ThemeOfEdgeEnabled       bool    // THEME_OF_EDGE_ENABLED — enable THEME_OF edge creation (default: true)

	// Deterministic consolidation trigger (Phase 45.5)
	ConsolidateOnWatchdogEnabled bool // CONSOLIDATE_ON_WATCHDOG_ENABLED — trigger consolidation alongside RSIC force (default: true)

	// Data transmission optimization settings (Phase 10.3)
	CompressionEnabled  bool // Enable gzip compression for responses (default: true)
	CompressionMinSize  int  // Minimum response size in bytes to compress (default: 1024)
	PaginationMaxLimit  int  // Maximum items per page (default: 500)
	PaginationDefLimit  int  // Default items per page (default: 50)

	// Neo4j connection pool settings (Phase 10.4)
	Neo4jMaxPoolSize         int // Maximum connections in pool (default: 100)
	Neo4jAcquireTimeoutSec   int // Connection acquire timeout in seconds (default: 60)
	Neo4jMaxConnLifetimeSec  int // Maximum connection lifetime in seconds (default: 3600)
	Neo4jConnIdleTimeoutSec  int // Connection idle timeout in seconds (default: 0 = disabled)

	// Dynamic port allocation
	PortRangeStart int    // Start of fallback port range (default: derived from ListenAddr port)
	PortRangeEnd   int    // End of fallback port range (default: PortRangeStart + 100)
	PortFilePath   string // Path to write allocated port for client discovery (default: .mdemg.port)

	// Scheduled sync settings (Phase 9.2)
	SyncIntervalMinutes     int               // Check interval for scheduled sync (default: 0 = disabled)
	SyncSpaceIDs            []string          // Comma-separated space IDs to monitor (empty = all)
	SyncStaleThresholdHours int               // Hours before a space is considered stale (default: 24)
	SyncRepoPathMap         map[string]string // space_id -> repo_path mapping for auto-ingest

	// ===== Phase 3: Production Readiness =====

	// Rate limiting settings (Phase 3.1)
	RateLimitEnabled bool    // Feature toggle for rate limiting (default: true)
	RateLimitRPS     float64 // Requests per second limit (default: 100)
	RateLimitBurst   int     // Burst allowance (default: 200)
	RateLimitByIP    bool    // Per-IP rate limiting vs global (default: true)

	// Circuit breaker settings (Phase 3.1)
	CircuitBreakerEnabled   bool // Feature toggle for circuit breaking (default: true)
	CircuitBreakerThreshold int  // Failures before opening circuit (default: 5)
	CircuitBreakerTimeoutSec int // Seconds before half-open (default: 30)

	// Authentication settings (Phase 3.2)
	AuthEnabled      bool     // Feature toggle for authentication (default: false for dev)
	AuthMode         string   // Auth mode: "none", "apikey", "bearer" (default: "none")
	AuthAPIKeys      []string // Comma-separated valid API keys
	AuthJWTSecret    string   // JWT secret for bearer mode
	AuthJWTIssuer    string   // Expected JWT issuer
	AuthSkipEndpoints []string // Endpoints that bypass auth (default: /healthz,/readyz)

	// CORS settings (Phase 3.2)
	CORSEnabled          bool     // Feature toggle for CORS (default: false)
	CORSAllowedOrigins   []string // Allowed origins (comma-separated, or "*" for all)
	CORSAllowedMethods   []string // Allowed methods (default: GET,POST,PUT,DELETE)
	CORSAllowedHeaders   []string // Allowed headers
	CORSAllowCredentials bool     // Allow credentials (default: false)

	// TLS settings (Phase 3.2)
	TLSEnabled  bool   // Feature toggle for HTTPS (default: false)
	TLSCertFile string // Path to TLS certificate file
	TLSKeyFile  string // Path to TLS key file

	// Prometheus metrics settings (Phase 3.3)
	MetricsEnabled   bool   // Feature toggle for Prometheus metrics (default: true)
	MetricsNamespace string // Metrics namespace prefix (default: "mdemg")

	// Graceful shutdown settings (Phase 3.4)
	GracefulShutdownTimeoutSec int // Shutdown timeout in seconds (default: 30)

	// ===== Phase 48.3-48.4: Data Transmission & Connection Pooling =====

	// Embedding rate limiting (Phase 48.4.3)
	EmbeddingRateLimitEnabled bool    // EMBEDDING_RATE_LIMIT_ENABLED — enable embedding rate limiting (default: false)
	EmbeddingOpenAIRPS        float64 // EMBEDDING_OPENAI_RPS — OpenAI requests per second (default: 500)
	EmbeddingOpenAIBurst      int     // EMBEDDING_OPENAI_BURST — OpenAI burst allowance (default: 1000)
	EmbeddingOllamaRPS        float64 // EMBEDDING_OLLAMA_RPS — Ollama requests per second (default: 100)
	EmbeddingOllamaBurst      int     // EMBEDDING_OLLAMA_BURST — Ollama burst allowance (default: 200)

	// Memory pressure monitoring (Phase 48.4.4)
	MemoryPressureEnabled     bool // MEMORY_PRESSURE_ENABLED — enable memory backpressure (default: false)
	MemoryPressureThresholdMB int  // MEMORY_PRESSURE_THRESHOLD_MB — heap threshold for rejection (default: 4096)

	// ===== Phase 80: CMS Meta-Cognition =====
	MetaCogEnabled          bool    // METACOG_ENABLED — master toggle for meta-cognitive anomaly detection (default: true)
	MetaCogEmptyResumeCheck bool    // METACOG_EMPTY_RESUME_CHECK — check for empty resume anomaly (default: true)
	MetaCogSignalDecayRate  float64 // METACOG_SIGNAL_DECAY_RATE — Hebbian decay per ignored signal emission (default: 0.05)
	MetaCogSignalBoostRate  float64 // METACOG_SIGNAL_BOOST_RATE — Hebbian boost per signal response (default: 0.1)

	// ===== CMS Hardening: Configurable Defaults =====
	CMSResumeMaxObs        int     // CMS_RESUME_MAX_OBS — default max observations on resume (default: 20)
	CMSRecallTopK          int     // CMS_RECALL_TOP_K — default top-K for recall queries (default: 10)
	CMSSummaryMaxChars     int     // CMS_SUMMARY_MAX_CHARS — max character length for generated summaries (default: 200)
	CMSJiminyBaseConfidence float64 // CMS_JIMINY_BASE_CONFIDENCE — base confidence for Jiminy rationale (default: 0.5)

	// ===== ANN Optimization: Learning Subsystem =====
	LearningCautiousDecayWindowHours int     // LEARNING_CAUTIOUS_DECAY_WINDOW_HOURS — skip decay for recently reinforced edges (default: 24, 0=disabled)
	LearningEtaConversationMult      float64 // LEARNING_ETA_CONVERSATION_MULT — eta multiplier for conversation observations (default: 2.0)
	LearningEtaConfigMult            float64 // LEARNING_ETA_CONFIG_MULT — eta multiplier for config↔code edges (default: 1.5)
	LearningEtaSameDirMult           float64 // LEARNING_ETA_SAME_DIR_MULT — eta multiplier for same-directory nodes (default: 1.2)
	LearningScheduleEnabled          bool    // LEARNING_SCHEDULE_ENABLED — enable maturity-based learning rate schedule (default: true)
	LearningScheduleColdMult         float64 // LEARNING_SCHEDULE_COLD_MULT — eta multiplier for cold spaces (0 edges) (default: 2.0)
	LearningScheduleLearningMult     float64 // LEARNING_SCHEDULE_LEARNING_MULT — eta multiplier for learning spaces (1-10k edges) (default: 1.0)
	LearningScheduleWarmMult         float64 // LEARNING_SCHEDULE_WARM_MULT — eta multiplier for warm spaces (10k-50k edges) (default: 0.5)
	LearningScheduleSatMult          float64 // LEARNING_SCHEDULE_SAT_MULT — eta multiplier for saturated spaces (50k+ edges) (default: 0.25)

	// ===== ANN Optimization: Retrieval Subsystem =====
	ScoringActivationFloor   float64 // SCORING_ACTIVATION_FLOOR — floor for squared activation (default: 0.05)
	ScoringActivationSquared bool    // SCORING_ACTIVATION_SQUARED — enable squared activation for sparser signals (default: true)
	ScoringBM25Weight        float64 // SCORING_BM25_WEIGHT — weight for BM25 score in final scoring formula (default: 0.15)
	ActivationHop0MinWeight  float64 // ACTIVATION_HOP0_MIN_WEIGHT — min edge weight for hop 0 spreading (default: 0.5)
	ActivationHop1MinWeight  float64 // ACTIVATION_HOP1_MIN_WEIGHT — min edge weight for hop 1 spreading (default: 0.2)
	ActivationHop2MinWeight  float64 // ACTIVATION_HOP2_MIN_WEIGHT — min edge weight for hop 2+ spreading (default: 0.05)
	ActivationSteps          int     // ACTIVATION_STEPS — number of spreading activation steps (default: 2, range: [1, 10])
	ActivationLambda         float64 // ACTIVATION_LAMBDA — spreading activation step decay (default: 0.15, range: [0, 0.9])
	ScoringBypassThreshold   float64 // SCORING_BYPASS_THRESHOLD — VectorSim threshold for value residual bypass (default: 0.85)
	ScoringBypassWeight      float64 // SCORING_BYPASS_WEIGHT — weight of bypass bonus (default: 0.15)
	ScoringBypassCodeMult    float64 // SCORING_BYPASS_CODE_MULT — bypass multiplier for code queries (default: 1.3)
	ScoringBypassArchMult    float64 // SCORING_BYPASS_ARCH_MULT — bypass multiplier for architecture queries (default: 0.5)

	// ===== ANN Optimization: Consolidation Subsystem =====
	L5GroundingMaxEdges      int     // L5_GROUNDING_MAX_EDGES — max GROUNDED_BY edges per L5 node (default: 5)
	L5GroundingMinSim        float64 // L5_GROUNDING_MIN_SIM — min cosine similarity for grounding edge (default: 0.4)
	L5GroundingInitialWeight float64 // L5_GROUNDING_INITIAL_WEIGHT — initial weight for GROUNDED_BY edges (default: 0.5)
	EdgeAttentionGroundedBy  float64 // EDGE_ATTENTION_GROUNDED_BY — attention weight for GROUNDED_BY edges (default: 0.70)

	// ===== ANN Optimization: Cluster Summary =====
	ClusterSummaryEnabled   bool   // CLUSTER_SUMMARY_ENABLED — enable LLM cluster summarization for L1-L4 (default: false)
	ClusterSummaryProvider  string // CLUSTER_SUMMARY_PROVIDER — LLM provider for summaries (openai/ollama, default: cascade)
	ClusterSummaryModel     string // CLUSTER_SUMMARY_MODEL — model for summaries (default: cascade)
	ClusterSummaryMaxTokens int    // CLUSTER_SUMMARY_MAX_TOKENS — max tokens per summary (default: 100)
	ClusterSummaryTimeoutMs int    // CLUSTER_SUMMARY_TIMEOUT_MS — timeout for summary call in ms (default: 5000)
	ClusterSummaryBatchSize int    // CLUSTER_SUMMARY_BATCH_SIZE — max nodes per consolidation run (default: 50)

	// ===== ANN Optimization: Negative Feedback =====
	LearningNegativeWeight        float64 // LEARNING_NEGATIVE_WEIGHT — weight reduction per negative feedback (default: 0.15)
	LearningNegativeDecayMult     float64 // LEARNING_NEGATIVE_DECAY_MULT — decay multiplier for contradicted edges (default: 2.0)
	LearningNegativeMaxPerRequest int     // LEARNING_NEGATIVE_MAX_PER_REQUEST — max rejected nodes per request (default: 20)

	// ===== ANN Optimization: Frontier Detection =====
	FrontierMinEvidence int // FRONTIER_MIN_EVIDENCE — min evidence_count for frontier candidates (default: 3)
	FrontierMaxOutgoing int // FRONTIER_MAX_OUTGOING — max outgoing edges for frontier candidates (default: 2)
}

// EffectiveLLMEndpoint returns the endpoint for LLM text-generation calls.
// If LLMEndpoint is set, it is used; otherwise falls back to OpenAIEndpoint.
func (c Config) EffectiveLLMEndpoint() string {
	if c.LLMEndpoint != "" {
		return c.LLMEndpoint
	}
	return c.OpenAIEndpoint
}

func FromEnv() (Config, error) {
	get := func(k, def string) string {
		v := strings.TrimSpace(os.Getenv(k))
		if v == "" {
			return def
		}
		return v
	}
	atoi := func(k string, def int) (int, error) {
		v := strings.TrimSpace(os.Getenv(k))
		if v == "" {
			return def, nil
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("%s must be int: %w", k, err)
		}
		return n, nil
	}
	atof := func(k string, def float64) (float64, error) {
		v := strings.TrimSpace(os.Getenv(k))
		if v == "" {
			return def, nil
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, fmt.Errorf("%s must be float: %w", k, err)
		}
		return f, nil
	}
	getBool := func(k string, def bool) bool {
		v := strings.ToLower(strings.TrimSpace(os.Getenv(k)))
		if v == "" {
			return def
		}
		return v == "true" || v == "1" || v == "yes"
	}

	listen := get("LISTEN_ADDR", ":9999")
	uri := get("NEO4J_URI", "")
	user := get("NEO4J_USER", "")
	pass := get("NEO4J_PASS", "")
	if uri == "" || user == "" || pass == "" {
		return Config{}, errors.New("NEO4J_URI/NEO4J_USER/NEO4J_PASS are required")
	}

	neo4jBoltPort, err := atoi("NEO4J_BOLT_PORT", 7687)
	if err != nil {
		return Config{}, err
	}
	neo4jHTTPPort, err := atoi("NEO4J_HTTP_PORT", 7474)
	if err != nil {
		return Config{}, err
	}

	reqVer, err := atoi("REQUIRED_SCHEMA_VERSION", 0)
	if err != nil {
		return Config{}, err
	}
	if reqVer <= 0 {
		// Auto-detect from embedded migrations
		autoVer, verErr := migrations.MaxVersion()
		if verErr != nil {
			return Config{}, fmt.Errorf("REQUIRED_SCHEMA_VERSION not set and auto-detect failed: %w", verErr)
		}
		reqVer = autoVer
	}

	candK, err := atoi("DEFAULT_CANDIDATE_K", 200)
	if err != nil {
		return Config{}, err
	}
	topK, err := atoi("DEFAULT_TOP_K", 20)
	if err != nil {
		return Config{}, err
	}
	hops, err := atoi("DEFAULT_HOP_DEPTH", 2)
	if err != nil {
		return Config{}, err
	}
	maxNbr, err := atoi("MAX_NEIGHBORS_PER_NODE", 50)
	if err != nil {
		return Config{}, err
	}
	maxEdges, err := atoi("MAX_TOTAL_EDGES_FETCHED", 5000)
	if err != nil {
		return Config{}, err
	}
	learnCap, err := atoi("LEARNING_EDGE_CAP_PER_REQUEST", 200)
	if err != nil {
		return Config{}, err
	}

	// Learning minimum activation threshold (0.0-1.0)
	learnMinActStr := get("LEARNING_MIN_ACTIVATION", "0.20")
	learnMinAct, err := strconv.ParseFloat(learnMinActStr, 64)
	if err != nil {
		return Config{}, fmt.Errorf("LEARNING_MIN_ACTIVATION must be float: %w", err)
	}
	if learnMinAct < 0 || learnMinAct > 1 {
		return Config{}, errors.New("LEARNING_MIN_ACTIVATION must be in range [0, 1]")
	}

	// Hebbian learning rate (η) - controls how much activation product strengthens edges
	learnEtaStr := get("LEARNING_ETA", "0.02")
	learnEta, err := strconv.ParseFloat(learnEtaStr, 64)
	if err != nil {
		return Config{}, fmt.Errorf("LEARNING_ETA must be float: %w", err)
	}
	if learnEta < 0 {
		return Config{}, errors.New("LEARNING_ETA must be >= 0")
	}

	// Hebbian decay/regularization (μ) - controls weight decay toward zero
	learnMuStr := get("LEARNING_MU", "0.01")
	learnMu, err := strconv.ParseFloat(learnMuStr, 64)
	if err != nil {
		return Config{}, fmt.Errorf("LEARNING_MU must be float: %w", err)
	}
	if learnMu < 0 || learnMu > 1 {
		return Config{}, errors.New("LEARNING_MU must be in range [0, 1]")
	}

	// Weight clamp bounds for Hebbian updates
	learnWMinStr := get("LEARNING_WMIN", "0.0")
	learnWMin, err := strconv.ParseFloat(learnWMinStr, 64)
	if err != nil {
		return Config{}, fmt.Errorf("LEARNING_WMIN must be float: %w", err)
	}

	learnWMaxStr := get("LEARNING_WMAX", "1.0")
	learnWMax, err := strconv.ParseFloat(learnWMaxStr, 64)
	if err != nil {
		return Config{}, fmt.Errorf("LEARNING_WMAX must be float: %w", err)
	}
	if learnWMin >= learnWMax {
		return Config{}, errors.New("LEARNING_WMIN must be < LEARNING_WMAX")
	}

	// Learning edge decay parameters
	learnDecayPerDayStr := get("LEARNING_DECAY_PER_DAY", "0.05")
	learnDecayPerDay, err := strconv.ParseFloat(learnDecayPerDayStr, 64)
	if err != nil {
		return Config{}, fmt.Errorf("LEARNING_DECAY_PER_DAY must be float: %w", err)
	}

	learnPruneThresholdStr := get("LEARNING_PRUNE_THRESHOLD", "0.05")
	learnPruneThreshold, err := strconv.ParseFloat(learnPruneThresholdStr, 64)
	if err != nil {
		return Config{}, fmt.Errorf("LEARNING_PRUNE_THRESHOLD must be float: %w", err)
	}

	learnMaxEdgesPerNodeStr := get("LEARNING_MAX_EDGES_PER_NODE", "50")
	learnMaxEdgesPerNode, err := strconv.Atoi(learnMaxEdgesPerNodeStr)
	if err != nil {
		return Config{}, fmt.Errorf("LEARNING_MAX_EDGES_PER_NODE must be int: %w", err)
	}

	allowed := get("ALLOWED_RELATIONSHIP_TYPES", "ASSOCIATED_WITH,TEMPORALLY_ADJACENT,CO_ACTIVATED_WITH,CAUSES,ENABLES,ABSTRACTS_TO,INSTANTIATES,GENERALIZES,IMPORTS,CALLS,EXTENDS,IMPLEMENTS,ANALOGOUS_TO,BRIDGES,COMPOSES_WITH,INFLUENCES,CONTRASTS_WITH,SPECIALIZES,GENERALIZES_TO,THEME_OF,DEFINES_SYMBOL,ORIGINATED_FROM,GROUNDED_BY,CONTRADICTS")
	parts := strings.Split(allowed, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}

	idx := get("VECTOR_INDEX_NAME", "memNodeEmbedding")

	// Semantic edge creation on ingest settings
	semEdgeEnabled := getBool("SEMANTIC_EDGE_ON_INGEST", true)
	semEdgeTopN, err := atoi("SEMANTIC_EDGE_TOP_N", 5)
	if err != nil {
		return Config{}, err
	}
	if semEdgeTopN < 1 {
		return Config{}, errors.New("SEMANTIC_EDGE_TOP_N must be >= 1")
	}
	semEdgeMinSim, err := atof("SEMANTIC_EDGE_MIN_SIMILARITY", 0.7)
	if err != nil {
		return Config{}, err
	}
	if semEdgeMinSim < 0 || semEdgeMinSim > 1 {
		return Config{}, errors.New("SEMANTIC_EDGE_MIN_SIMILARITY must be in range [0, 1]")
	}
	semEdgeInitWeight, err := atof("SEMANTIC_EDGE_INITIAL_WEIGHT", 0.5)
	if err != nil {
		return Config{}, err
	}
	if semEdgeInitWeight < 0 || semEdgeInitWeight > 1 {
		return Config{}, errors.New("SEMANTIC_EDGE_INITIAL_WEIGHT must be in range [0, 1]")
	}

	// Batch ingest settings
	batchMaxItems, err := atoi("BATCH_INGEST_MAX_ITEMS", 500)
	if err != nil {
		return Config{}, err
	}
	if batchMaxItems < 1 || batchMaxItems > 2000 {
		return Config{}, errors.New("BATCH_INGEST_MAX_ITEMS must be in range [1, 2000]")
	}

	// HTTP server timeouts
	httpReadTimeout, err := atoi("HTTP_READ_TIMEOUT", 600)
	if err != nil {
		return Config{}, err
	}
	if httpReadTimeout < 1 {
		return Config{}, errors.New("HTTP_READ_TIMEOUT must be >= 1")
	}
	httpWriteTimeout, err := atoi("HTTP_WRITE_TIMEOUT", 600)
	if err != nil {
		return Config{}, err
	}
	if httpWriteTimeout < 1 {
		return Config{}, errors.New("HTTP_WRITE_TIMEOUT must be >= 1")
	}

	// Anomaly detection settings
	anomalyEnabled := getBool("ANOMALY_DETECTION_ENABLED", true)
	anomalyDupThreshold, err := atof("ANOMALY_DUPLICATE_THRESHOLD", 0.95)
	if err != nil {
		return Config{}, err
	}
	if anomalyDupThreshold < 0 || anomalyDupThreshold > 1 {
		return Config{}, errors.New("ANOMALY_DUPLICATE_THRESHOLD must be in range [0, 1]")
	}
	anomalyOutlierStdDevs, err := atof("ANOMALY_OUTLIER_STDDEVS", 2.0)
	if err != nil {
		return Config{}, err
	}
	if anomalyOutlierStdDevs <= 0 {
		return Config{}, errors.New("ANOMALY_OUTLIER_STDDEVS must be > 0")
	}
	anomalyStaleDays, err := atoi("ANOMALY_STALE_DAYS", 30)
	if err != nil {
		return Config{}, err
	}
	if anomalyStaleDays < 0 {
		return Config{}, errors.New("ANOMALY_STALE_DAYS must be >= 0")
	}
	anomalyMaxCheckMs, err := atoi("ANOMALY_MAX_CHECK_MS", 100)
	if err != nil {
		return Config{}, err
	}
	if anomalyMaxCheckMs < 1 {
		return Config{}, errors.New("ANOMALY_MAX_CHECK_MS must be >= 1")
	}

	// Scoring hyperparameters for retrieval ranking
	// Weights (alpha, beta, gamma, delta) must be in [0, 1]
	scoringAlpha, err := atof("SCORING_ALPHA", 0.60)
	if err != nil {
		return Config{}, err
	}
	if scoringAlpha < 0 || scoringAlpha > 1 {
		return Config{}, errors.New("SCORING_ALPHA must be in range [0, 1]")
	}
	scoringBeta, err := atof("SCORING_BETA", 0.20)
	if err != nil {
		return Config{}, err
	}
	if scoringBeta < 0 || scoringBeta > 1 {
		return Config{}, errors.New("SCORING_BETA must be in range [0, 1]")
	}
	scoringGamma, err := atof("SCORING_GAMMA", 0.15)
	if err != nil {
		return Config{}, err
	}
	if scoringGamma < 0 || scoringGamma > 1 {
		return Config{}, errors.New("SCORING_GAMMA must be in range [0, 1]")
	}
	scoringDelta, err := atof("SCORING_DELTA", 0.05)
	if err != nil {
		return Config{}, err
	}
	if scoringDelta < 0 || scoringDelta > 1 {
		return Config{}, errors.New("SCORING_DELTA must be in range [0, 1]")
	}
	scoringPhi, err := atof("SCORING_PHI", 0.08)
	if err != nil {
		return Config{}, err
	}
	if scoringPhi < 0 {
		return Config{}, errors.New("SCORING_PHI must be >= 0")
	}
	scoringKappa, err := atof("SCORING_KAPPA", 0.12)
	if err != nil {
		return Config{}, err
	}
	if scoringKappa < 0 {
		return Config{}, errors.New("SCORING_KAPPA must be >= 0")
	}
	scoringRho, err := atof("SCORING_RHO", 0.05)
	if err != nil {
		return Config{}, err
	}
	if scoringRho < 0 {
		return Config{}, errors.New("SCORING_RHO must be >= 0")
	}
	// Layer-specific decay rates (faster decay for L0 files, slower for abstractions)
	scoringRhoL0, err := atof("SCORING_RHO_L0", 0.05)
	if err != nil {
		return Config{}, err
	}
	if scoringRhoL0 < 0 {
		return Config{}, errors.New("SCORING_RHO_L0 must be >= 0")
	}
	scoringRhoL1, err := atof("SCORING_RHO_L1", 0.02)
	if err != nil {
		return Config{}, err
	}
	if scoringRhoL1 < 0 {
		return Config{}, errors.New("SCORING_RHO_L1 must be >= 0")
	}
	scoringRhoL2, err := atof("SCORING_RHO_L2", 0.01)
	if err != nil {
		return Config{}, err
	}
	if scoringRhoL2 < 0 {
		return Config{}, errors.New("SCORING_RHO_L2 must be >= 0")
	}
	scoringConfigBoost, err := atof("SCORING_CONFIG_BOOST", 1.15)
	if err != nil {
		return Config{}, err
	}
	if scoringConfigBoost < 1.0 {
		return Config{}, errors.New("SCORING_CONFIG_BOOST must be >= 1.0")
	}
	scoringPathBoost, err := atof("SCORING_PATH_BOOST", 0.15)
	if err != nil {
		return Config{}, err
	}
	if scoringPathBoost < 0 {
		return Config{}, errors.New("SCORING_PATH_BOOST must be >= 0")
	}

	// Temporal reasoning settings (Phase 1: Time-Aware Retrieval)
	temporalEnabled := getBool("TEMPORAL_ENABLED", true)
	temporalSoftBoost, err := atof("TEMPORAL_SOFT_BOOST", 3.0)
	if err != nil {
		return Config{}, err
	}
	if temporalSoftBoost < 1.0 || temporalSoftBoost > 10.0 {
		return Config{}, errors.New("TEMPORAL_SOFT_BOOST must be in range [1.0, 10.0]")
	}
	temporalHardFilterEnabled := getBool("TEMPORAL_HARD_FILTER", true)

	// Phase 2 Temporal: Source-type-specific decay rates
	temporalSourceTypeDecayEnabled := getBool("TEMPORAL_SOURCE_TYPE_DECAY", false)
	scoringRhoDocumentation, err := atof("SCORING_RHO_DOCUMENTATION", 0.01)
	if err != nil {
		return Config{}, err
	}
	if scoringRhoDocumentation < 0 {
		return Config{}, errors.New("SCORING_RHO_DOCUMENTATION must be >= 0")
	}
	scoringRhoConfig, err := atof("SCORING_RHO_CONFIG", 0.03)
	if err != nil {
		return Config{}, err
	}
	if scoringRhoConfig < 0 {
		return Config{}, errors.New("SCORING_RHO_CONFIG must be >= 0")
	}
	scoringRhoConversation, err := atof("SCORING_RHO_CONVERSATION", 0.10)
	if err != nil {
		return Config{}, err
	}
	if scoringRhoConversation < 0 {
		return Config{}, errors.New("SCORING_RHO_CONVERSATION must be >= 0")
	}
	scoringRhoChangelog, err := atof("SCORING_RHO_CHANGELOG", 0.08)
	if err != nil {
		return Config{}, err
	}
	if scoringRhoChangelog < 0 {
		return Config{}, errors.New("SCORING_RHO_CHANGELOG must be >= 0")
	}

	// Phase 2 Temporal: Stale reference detection
	temporalStaleRefDays, err := atoi("TEMPORAL_STALE_REF_DAYS", 0)
	if err != nil {
		return Config{}, err
	}
	if temporalStaleRefDays < 0 {
		return Config{}, errors.New("TEMPORAL_STALE_REF_DAYS must be >= 0")
	}
	temporalStaleRefMaxPen, err := atof("TEMPORAL_STALE_REF_MAX_PENALTY", 0.15)
	if err != nil {
		return Config{}, err
	}
	if temporalStaleRefMaxPen < 0 {
		return Config{}, errors.New("TEMPORAL_STALE_REF_MAX_PENALTY must be >= 0")
	}

	// Top-level LLM cascade (defaults for all text-generation features)
	llmProvider := get("LLM_PROVIDER", "openai")
	llmModel := get("LLM_MODEL", "gpt-5-nano")

	// Embedding provider settings
	embProvider := get("EMBEDDING_PROVIDER", "openai")
	openaiKey := get("OPENAI_API_KEY", "")
	openaiModel := get("OPENAI_MODEL", "text-embedding-3-large")
	openaiEndpoint := get("OPENAI_ENDPOINT", "https://api.openai.com/v1")
	llmEndpoint := get("LLM_ENDPOINT", "")
	ollamaEndpoint := get("OLLAMA_ENDPOINT", "http://localhost:11434")
	ollamaModel := get("OLLAMA_MODEL", "qwen3-embedding:4b")

	// Embedding cache settings
	embCacheEnabled := getBool("EMBEDDING_CACHE_ENABLED", true)
	embCacheSize, err := atoi("EMBEDDING_CACHE_SIZE", 1000)
	if err != nil {
		return Config{}, err
	}
	if embCacheEnabled && embCacheSize <= 0 {
		return Config{}, errors.New("EMBEDDING_CACHE_SIZE must be > 0 when caching is enabled")
	}

	// Query result cache settings (Phase 10)
	queryCacheEnabled := getBool("QUERY_CACHE_ENABLED", true)
	queryCacheCapacity, err := atoi("QUERY_CACHE_CAPACITY", 500)
	if err != nil {
		return Config{}, err
	}
	queryCacheTTL, err := atoi("QUERY_CACHE_TTL_SECONDS", 300)
	if err != nil {
		return Config{}, err
	}

	// Logging settings
	logFormat := get("LOG_FORMAT", "text")
	if logFormat != "text" && logFormat != "json" {
		return Config{}, errors.New("LOG_FORMAT must be 'text' or 'json'")
	}
	logSkipHealth := getBool("LOG_SKIP_HEALTH", false)

	// Hidden layer settings (V0005)
	hiddenEnabled := getBool("HIDDEN_LAYER_ENABLED", true)
	hiddenClusterEps, err := atof("HIDDEN_LAYER_CLUSTER_EPS", 0.1)
	if err != nil {
		return Config{}, err
	}
	if hiddenClusterEps <= 0 || hiddenClusterEps > 1 {
		return Config{}, errors.New("HIDDEN_LAYER_CLUSTER_EPS must be in range (0, 1]")
	}
	hiddenMinSamples, err := atoi("HIDDEN_LAYER_MIN_SAMPLES", 3)
	if err != nil {
		return Config{}, err
	}
	if hiddenMinSamples < 2 {
		return Config{}, errors.New("HIDDEN_LAYER_MIN_SAMPLES must be >= 2")
	}
	hiddenMaxHidden, err := atoi("HIDDEN_LAYER_MAX_HIDDEN", 100)
	if err != nil {
		return Config{}, err
	}
	if hiddenMaxHidden < 1 {
		return Config{}, errors.New("HIDDEN_LAYER_MAX_HIDDEN must be >= 1")
	}
	hiddenBatchSize, err := atoi("HIDDEN_LAYER_BATCH_SIZE", 0)
	if err != nil {
		return Config{}, err
	}
	if hiddenBatchSize < 0 {
		return Config{}, errors.New("HIDDEN_LAYER_BATCH_SIZE must be >= 0")
	}
	hiddenMaxClusterSize, err := atoi("HIDDEN_LAYER_MAX_CLUSTER_SIZE", 200)
	if err != nil {
		return Config{}, err
	}
	if hiddenMaxClusterSize < 10 {
		return Config{}, errors.New("HIDDEN_LAYER_MAX_CLUSTER_SIZE must be >= 10")
	}
	hiddenPathGroupDepth, err := atoi("HIDDEN_LAYER_PATH_GROUP_DEPTH", 2)
	if err != nil {
		return Config{}, err
	}
	if hiddenPathGroupDepth < 1 || hiddenPathGroupDepth > 5 {
		return Config{}, errors.New("HIDDEN_LAYER_PATH_GROUP_DEPTH must be in range [1, 5]")
	}
	hiddenForwardAlpha, err := atof("HIDDEN_LAYER_FORWARD_ALPHA", 0.6)
	if err != nil {
		return Config{}, err
	}
	hiddenForwardBeta, err := atof("HIDDEN_LAYER_FORWARD_BETA", 0.4)
	if err != nil {
		return Config{}, err
	}
	hiddenBackwardSelf, err := atof("HIDDEN_LAYER_BACKWARD_SELF", 0.2)
	if err != nil {
		return Config{}, err
	}
	hiddenBackwardBase, err := atof("HIDDEN_LAYER_BACKWARD_BASE", 0.5)
	if err != nil {
		return Config{}, err
	}
	hiddenBackwardConc, err := atof("HIDDEN_LAYER_BACKWARD_CONC", 0.3)
	if err != nil {
		return Config{}, err
	}

	// Concept merge settings (V0007)
	conceptMergeEnabled := getBool("CONCEPT_MERGE_ENABLED", true)
	conceptMergeThreshold, err := atof("CONCEPT_MERGE_THRESHOLD", 0.90)
	if err != nil {
		return Config{}, err
	}
	if conceptMergeThreshold < 0.5 || conceptMergeThreshold > 1.0 {
		return Config{}, errors.New("CONCEPT_MERGE_THRESHOLD must be in range [0.5, 1.0]")
	}

	// Edge-type attention settings (V0008)
	edgeAttentionEnabled := getBool("EDGE_ATTENTION_ENABLED", true)
	edgeAttentionCoActivated, err := atof("EDGE_ATTENTION_CO_ACTIVATED", 0.85)
	if err != nil {
		return Config{}, err
	}
	if edgeAttentionCoActivated < 0 || edgeAttentionCoActivated > 1 {
		return Config{}, errors.New("EDGE_ATTENTION_CO_ACTIVATED must be in range [0, 1]")
	}
	edgeAttentionAssociated, err := atof("EDGE_ATTENTION_ASSOCIATED", 0.65)
	if err != nil {
		return Config{}, err
	}
	if edgeAttentionAssociated < 0 || edgeAttentionAssociated > 1 {
		return Config{}, errors.New("EDGE_ATTENTION_ASSOCIATED must be in range [0, 1]")
	}
	edgeAttentionGeneralizes, err := atof("EDGE_ATTENTION_GENERALIZES", 0.65)
	if err != nil {
		return Config{}, err
	}
	if edgeAttentionGeneralizes < 0 || edgeAttentionGeneralizes > 1 {
		return Config{}, errors.New("EDGE_ATTENTION_GENERALIZES must be in range [0, 1]")
	}
	edgeAttentionAbstractsTo, err := atof("EDGE_ATTENTION_ABSTRACTS_TO", 0.60)
	if err != nil {
		return Config{}, err
	}
	if edgeAttentionAbstractsTo < 0 || edgeAttentionAbstractsTo > 1 {
		return Config{}, errors.New("EDGE_ATTENTION_ABSTRACTS_TO must be in range [0, 1]")
	}
	edgeAttentionTemporal, err := atof("EDGE_ATTENTION_TEMPORAL", 0.45)
	if err != nil {
		return Config{}, err
	}
	if edgeAttentionTemporal < 0 || edgeAttentionTemporal > 1 {
		return Config{}, errors.New("EDGE_ATTENTION_TEMPORAL must be in range [0, 1]")
	}
	edgeAttentionCodeBoost, err := atof("EDGE_ATTENTION_CODE_BOOST", 1.2)
	if err != nil {
		return Config{}, err
	}
	if edgeAttentionCodeBoost < 0.5 || edgeAttentionCodeBoost > 3.0 {
		return Config{}, errors.New("EDGE_ATTENTION_CODE_BOOST must be in range [0.5, 3.0]")
	}
	edgeAttentionArchBoost, err := atof("EDGE_ATTENTION_ARCH_BOOST", 1.5)
	if err != nil {
		return Config{}, err
	}
	if edgeAttentionArchBoost < 0.5 || edgeAttentionArchBoost > 3.0 {
		return Config{}, errors.New("EDGE_ATTENTION_ARCH_BOOST must be in range [0.5, 3.0]")
	}

	// Query-Aware Expansion settings (V0009)
	queryAwareExpansionEnabled := getBool("QUERY_AWARE_EXPANSION_ENABLED", true)
	queryAwareAttentionWeight, err := atof("QUERY_AWARE_ATTENTION_WEIGHT", 0.5)
	if err != nil {
		return Config{}, err
	}
	if queryAwareAttentionWeight < 0 || queryAwareAttentionWeight > 1 {
		return Config{}, errors.New("QUERY_AWARE_ATTENTION_WEIGHT must be in range [0, 1]")
	}
	nodeEmbeddingCacheSize, err := atoi("NODE_EMBEDDING_CACHE_SIZE", 5000)
	if err != nil {
		return Config{}, err
	}
	if nodeEmbeddingCacheSize < 100 {
		return Config{}, errors.New("NODE_EMBEDDING_CACHE_SIZE must be >= 100")
	}

	// Hybrid Edge Type Strategy settings (V0010)
	edgeTypeStrategy := get("EDGE_TYPE_STRATEGY", "hybrid")
	validStrategies := map[string]bool{"all": true, "structural_first": true, "learned_only": true, "hybrid": true}
	if !validStrategies[edgeTypeStrategy] {
		return Config{}, errors.New("EDGE_TYPE_STRATEGY must be one of: all, structural_first, learned_only, hybrid")
	}

	structuralEdgeTypesStr := get("STRUCTURAL_EDGE_TYPES", "ASSOCIATED_WITH,GENERALIZES,ABSTRACTS_TO,IMPORTS,CALLS,EXTENDS,IMPLEMENTS,ANALOGOUS_TO,BRIDGES,COMPOSES_WITH,INFLUENCES")
	structuralEdgeTypes := make([]string, 0)
	for _, p := range strings.Split(structuralEdgeTypesStr, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			structuralEdgeTypes = append(structuralEdgeTypes, p)
		}
	}
	if len(structuralEdgeTypes) == 0 {
		return Config{}, errors.New("STRUCTURAL_EDGE_TYPES must not be empty")
	}

	learnedEdgeTypesStr := get("LEARNED_EDGE_TYPES", "CO_ACTIVATED_WITH")
	learnedEdgeTypes := make([]string, 0)
	for _, p := range strings.Split(learnedEdgeTypesStr, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			learnedEdgeTypes = append(learnedEdgeTypes, p)
		}
	}
	if len(learnedEdgeTypes) == 0 {
		return Config{}, errors.New("LEARNED_EDGE_TYPES must not be empty")
	}

	hybridSwitchHop, err := atoi("HYBRID_SWITCH_HOP", 1)
	if err != nil {
		return Config{}, err
	}
	if hybridSwitchHop < 0 || hybridSwitchHop > 3 {
		return Config{}, errors.New("HYBRID_SWITCH_HOP must be in range [0, 3]")
	}

	// Hybrid retrieval settings (V0006)
	hybridEnabled := getBool("HYBRID_RETRIEVAL_ENABLED", true)
	bm25TopK, err := atoi("BM25_TOP_K", 100)
	if err != nil {
		return Config{}, err
	}
	if bm25TopK < 1 {
		return Config{}, errors.New("BM25_TOP_K must be >= 1")
	}
	bm25Weight, err := atof("BM25_WEIGHT", 0.3)
	if err != nil {
		return Config{}, err
	}
	if bm25Weight < 0 || bm25Weight > 1 {
		return Config{}, errors.New("BM25_WEIGHT must be in range [0, 1]")
	}
	vectorWeight, err := atof("VECTOR_WEIGHT", 0.7)
	if err != nil {
		return Config{}, err
	}
	if vectorWeight < 0 || vectorWeight > 1 {
		return Config{}, errors.New("VECTOR_WEIGHT must be in range [0, 1]")
	}
	rrfConstant, err := atoi("RRF_CONSTANT", 60)
	if err != nil {
		return Config{}, err
	}
	if rrfConstant < 1 {
		return Config{}, errors.New("RRF_CONSTANT must be >= 1")
	}

	// LLM Re-ranking settings (V0006)
	rerankEnabled := getBool("RERANK_ENABLED", false)
	rerankProvider := get("RERANK_PROVIDER", llmProvider)
	rerankModel := get("RERANK_MODEL", llmModel)
	rerankTopN, err := atoi("RERANK_TOP_N", 30)
	if err != nil {
		return Config{}, err
	}
	if rerankTopN < 5 {
		return Config{}, errors.New("RERANK_TOP_N must be >= 5")
	}
	rerankWeight, err := atof("RERANK_WEIGHT", 0.4)
	if err != nil {
		return Config{}, err
	}
	if rerankWeight < 0 || rerankWeight > 1 {
		return Config{}, errors.New("RERANK_WEIGHT must be in range [0, 1]")
	}
	rerankTimeoutMs, err := atoi("RERANK_TIMEOUT_MS", 3000)
	if err != nil {
		return Config{}, err
	}
	if rerankTimeoutMs < 100 {
		return Config{}, errors.New("RERANK_TIMEOUT_MS must be >= 100")
	}
	rerankJinaKey := get("RERANK_JINA_API_KEY", "")
	rerankJinaModel := get("RERANK_JINA_MODEL", "jina-reranker-v2-base-multilingual")
	rerankJinaURL := get("RERANK_JINA_URL", "https://api.jina.ai/v1")

	// Plugin system settings (V0006)
	pluginsEnabled := getBool("PLUGINS_ENABLED", true)
	pluginsDir := get("PLUGINS_DIR", "./plugins")
	pluginSocketDir := get("PLUGIN_SOCKET_DIR", "/tmp/mdemg-plugins")
	mdemgVersion := get("MDEMG_VERSION", "0.6.0")

	// Linear integration settings (Phase 4)
	linearTeamID := get("LINEAR_TEAM_ID", "")
	linearWorkspaceID := get("LINEAR_WORKSPACE_ID", "")

	// Linear webhook settings (Phase 9.4)
	linearWebhookSecret := get("LINEAR_WEBHOOK_SECRET", "")
	linearWebhookSpaceID := get("LINEAR_WEBHOOK_SPACE_ID", "")

	// Generic webhook settings (Phase 9.4)
	webhookConfigs := get("WEBHOOK_CONFIGS", "")

	// File watcher settings (Phase 9.4)
	fileWatcherEnabled := getBool("FILE_WATCHER_ENABLED", false)
	fileWatcherConfigs := get("FILE_WATCHER_CONFIGS", "")

	// Conflict logging settings (Phase 9.5)
	conflictLogEnabled := getBool("CONFLICT_LOG_ENABLED", true)

	// Scheduled orphan cleanup settings (Phase 9.5)
	orphanCleanupIntervalHours, err := atoi("ORPHAN_CLEANUP_INTERVAL_HOURS", 0)
	if err != nil {
		return Config{}, err
	}
	if orphanCleanupIntervalHours < 0 {
		return Config{}, errors.New("ORPHAN_CLEANUP_INTERVAL_HOURS must be >= 0")
	}

	// Optimistic retry settings (Phase 47)
	optimisticRetryEnabled := getBool("OPTIMISTIC_RETRY_ENABLED", true)
	optimisticRetryMaxAttempts, err := atoi("OPTIMISTIC_RETRY_MAX_ATTEMPTS", 5)
	if err != nil {
		return Config{}, err
	}
	if optimisticRetryMaxAttempts < 0 {
		return Config{}, errors.New("OPTIMISTIC_RETRY_MAX_ATTEMPTS must be >= 0")
	}
	optimisticRetryBaseDelayMs, err := atoi("OPTIMISTIC_RETRY_BASE_DELAY_MS", 10)
	if err != nil {
		return Config{}, err
	}
	if optimisticRetryBaseDelayMs < 0 {
		return Config{}, errors.New("OPTIMISTIC_RETRY_BASE_DELAY_MS must be >= 0")
	}
	optimisticRetryMaxDelayMs, err := atoi("OPTIMISTIC_RETRY_MAX_DELAY_MS", 1000)
	if err != nil {
		return Config{}, err
	}
	if optimisticRetryMaxDelayMs < optimisticRetryBaseDelayMs {
		return Config{}, errors.New("OPTIMISTIC_RETRY_MAX_DELAY_MS must be >= OPTIMISTIC_RETRY_BASE_DELAY_MS")
	}
	optimisticRetryMultiplier, err := atof("OPTIMISTIC_RETRY_MULTIPLIER", 2.0)
	if err != nil {
		return Config{}, err
	}
	if optimisticRetryMultiplier < 1.0 {
		return Config{}, errors.New("OPTIMISTIC_RETRY_MULTIPLIER must be >= 1.0")
	}

	// Edge staleness settings (Phase 47)
	edgeStalenessCascadeEnabled := getBool("EDGE_STALENESS_CASCADE_ENABLED", true)
	edgeStalenessRefreshBatchSize, err := atoi("EDGE_STALENESS_REFRESH_BATCH_SIZE", 100)
	if err != nil {
		return Config{}, err
	}
	if edgeStalenessRefreshBatchSize < 1 {
		return Config{}, errors.New("EDGE_STALENESS_REFRESH_BATCH_SIZE must be >= 1")
	}
	edgeStalenessReclusterThresh, err := atof("EDGE_STALENESS_RECLUSTER_THRESHOLD", 0.3)
	if err != nil {
		return Config{}, err
	}
	if edgeStalenessReclusterThresh < 0 || edgeStalenessReclusterThresh > 1 {
		return Config{}, errors.New("EDGE_STALENESS_RECLUSTER_THRESHOLD must be in range [0, 1]")
	}

	// LLM Summary settings (semantic summaries for ingest)
	llmSummaryEnabled := getBool("LLM_SUMMARY_ENABLED", true)
	llmSummaryProvider := get("LLM_SUMMARY_PROVIDER", llmProvider)
	llmSummaryModel := get("LLM_SUMMARY_MODEL", llmModel)
	llmSummaryMaxTokens, err := atoi("LLM_SUMMARY_MAX_TOKENS", 150)
	if err != nil {
		return Config{}, err
	}
	if llmSummaryMaxTokens < 50 || llmSummaryMaxTokens > 500 {
		return Config{}, errors.New("LLM_SUMMARY_MAX_TOKENS must be in range [50, 500]")
	}
	llmSummaryBatchSize, err := atoi("LLM_SUMMARY_BATCH_SIZE", 10)
	if err != nil {
		return Config{}, err
	}
	if llmSummaryBatchSize < 1 || llmSummaryBatchSize > 50 {
		return Config{}, errors.New("LLM_SUMMARY_BATCH_SIZE must be in range [1, 50]")
	}
	llmSummaryTimeoutMs, err := atoi("LLM_SUMMARY_TIMEOUT_MS", 30000)
	if err != nil {
		return Config{}, err
	}
	if llmSummaryTimeoutMs < 1000 {
		return Config{}, errors.New("LLM_SUMMARY_TIMEOUT_MS must be >= 1000")
	}
	llmSummaryCacheSize, err := atoi("LLM_SUMMARY_CACHE_SIZE", 5000)
	if err != nil {
		return Config{}, err
	}
	if llmSummaryCacheSize < 0 {
		return Config{}, errors.New("LLM_SUMMARY_CACHE_SIZE must be >= 0")
	}

	// SME Synthesis settings (Phase 101)
	synthesisEnabled := getBool("SYNTHESIS_ENABLED", false)
	synthesisProvider := get("SYNTHESIS_PROVIDER", llmProvider)
	synthesisModel := get("SYNTHESIS_MODEL", llmModel)
	synthesisMaxTokens, err := atoi("SYNTHESIS_MAX_TOKENS", 2000)
	if err != nil {
		return Config{}, err
	}
	if synthesisMaxTokens < 100 || synthesisMaxTokens > 8000 {
		return Config{}, errors.New("SYNTHESIS_MAX_TOKENS must be in range [100, 8000]")
	}
	synthesisTimeoutMs, err := atoi("SYNTHESIS_TIMEOUT_MS", 30000)
	if err != nil {
		return Config{}, err
	}
	if synthesisTimeoutMs < 1000 {
		return Config{}, errors.New("SYNTHESIS_TIMEOUT_MS must be >= 1000")
	}

	// Intent Translation settings (Phase 102)
	intentEnabled := getBool("INTENT_ENABLED", false)
	intentProvider := get("INTENT_PROVIDER", llmProvider)
	intentModel := get("INTENT_MODEL", llmModel)
	intentMaxTokens, err := atoi("INTENT_MAX_TOKENS", 150)
	if err != nil {
		return Config{}, err
	}
	if intentMaxTokens < 10 || intentMaxTokens > 500 {
		return Config{}, errors.New("INTENT_MAX_TOKENS must be in range [10, 500]")
	}
	intentTimeoutMs, err := atoi("INTENT_TIMEOUT_MS", 2000)
	if err != nil {
		return Config{}, err
	}
	if intentTimeoutMs < 200 {
		return Config{}, errors.New("INTENT_TIMEOUT_MS must be >= 200")
	}

	// Dynamic Emergence settings (Phase 103)
	emergenceEnabled := getBool("EMERGENCE_ENABLED", false)
	emergenceProvider := get("EMERGENCE_PROVIDER", llmProvider)
	emergenceModel := get("EMERGENCE_MODEL", llmModel)
	emergenceMaxTokens, err := atoi("EMERGENCE_MAX_TOKENS", 500)
	if err != nil {
		return Config{}, err
	}
	if emergenceMaxTokens < 100 || emergenceMaxTokens > 4000 {
		return Config{}, errors.New("EMERGENCE_MAX_TOKENS must be in range [100, 4000]")
	}
	emergenceTimeoutMs, err := atoi("EMERGENCE_TIMEOUT_MS", 10000)
	if err != nil {
		return Config{}, err
	}
	if emergenceTimeoutMs < 1000 {
		return Config{}, errors.New("EMERGENCE_TIMEOUT_MS must be >= 1000")
	}
	emergenceMinWeight, err := atof("EMERGENCE_MIN_WEIGHT", 0.3)
	if err != nil {
		return Config{}, err
	}
	if emergenceMinWeight < 0.0 || emergenceMinWeight > 1.0 {
		return Config{}, errors.New("EMERGENCE_MIN_WEIGHT must be in range [0.0, 1.0]")
	}
	emergenceMinClusterSize, err := atoi("EMERGENCE_MIN_CLUSTER_SIZE", 3)
	if err != nil {
		return Config{}, err
	}
	if emergenceMinClusterSize < 2 {
		return Config{}, errors.New("EMERGENCE_MIN_CLUSTER_SIZE must be >= 2")
	}
	emergenceMaxClusters, err := atoi("EMERGENCE_MAX_CLUSTERS", 10)
	if err != nil {
		return Config{}, err
	}
	if emergenceMaxClusters < 1 {
		return Config{}, errors.New("EMERGENCE_MAX_CLUSTERS must be >= 1")
	}

	// Active MCP Guardrails settings (Phase 104)
	guardrailEnabled := getBool("GUARDRAIL_ENABLED", false)
	guardrailProvider := get("GUARDRAIL_PROVIDER", llmProvider)
	guardrailModel := get("GUARDRAIL_MODEL", llmModel)
	guardrailMaxTokens, err := atoi("GUARDRAIL_MAX_TOKENS", 1000)
	if err != nil {
		return Config{}, err
	}
	if guardrailMaxTokens < 100 || guardrailMaxTokens > 4000 {
		return Config{}, errors.New("GUARDRAIL_MAX_TOKENS must be in range [100, 4000]")
	}
	guardrailTimeoutMs, err := atoi("GUARDRAIL_TIMEOUT_MS", 5000)
	if err != nil {
		return Config{}, err
	}
	if guardrailTimeoutMs < 1000 {
		return Config{}, errors.New("GUARDRAIL_TIMEOUT_MS must be >= 1000")
	}
	guardrailMaxConstraints, err := atoi("GUARDRAIL_MAX_CONSTRAINTS", 10)
	if err != nil {
		return Config{}, err
	}
	if guardrailMaxConstraints < 1 || guardrailMaxConstraints > 50 {
		return Config{}, errors.New("GUARDRAIL_MAX_CONSTRAINTS must be in range [1, 50]")
	}

	// Dynamic Reclassification settings
	reclassEnabled := getBool("RECLASS_ENABLED", true)
	reclassThreshold, err := atof("RECLASS_THRESHOLD", 0.25)
	if err != nil {
		return Config{}, err
	}
	if reclassThreshold < 0.05 || reclassThreshold > 0.90 {
		return Config{}, errors.New("RECLASS_THRESHOLD must be in range [0.05, 0.90]")
	}
	reclassMaxSampleSize, err := atoi("RECLASS_MAX_SAMPLE_SIZE", 150)
	if err != nil {
		return Config{}, err
	}
	if reclassMaxSampleSize < 20 || reclassMaxSampleSize > 500 {
		return Config{}, errors.New("RECLASS_MAX_SAMPLE_SIZE must be in range [20, 500]")
	}
	reclassMaxCategories, err := atoi("RECLASS_MAX_CATEGORIES", 10)
	if err != nil {
		return Config{}, err
	}
	if reclassMaxCategories < 3 || reclassMaxCategories > 20 {
		return Config{}, errors.New("RECLASS_MAX_CATEGORIES must be in range [3, 20]")
	}
	reclassMaxIterations, err := atoi("RECLASS_MAX_ITERATIONS", 5)
	if err != nil {
		return Config{}, err
	}
	if reclassMaxIterations < 1 || reclassMaxIterations > 10 {
		return Config{}, errors.New("RECLASS_MAX_ITERATIONS must be in range [1, 10]")
	}
	reclassMaxDepth, err := atoi("RECLASS_MAX_DEPTH", 4)
	if err != nil {
		return Config{}, err
	}
	if reclassMaxDepth < 1 || reclassMaxDepth > 10 {
		return Config{}, errors.New("RECLASS_MAX_DEPTH must be in range [1, 10]")
	}
	reclassProvider := get("RECLASS_PROVIDER", emergenceProvider)
	reclassModel := get("RECLASS_MODEL", "gpt-5.4")
	reclassMaxTokens, err := atoi("RECLASS_MAX_TOKENS", 2000)
	if err != nil {
		return Config{}, err
	}
	if reclassMaxTokens < 500 || reclassMaxTokens > 8000 {
		return Config{}, errors.New("RECLASS_MAX_TOKENS must be in range [500, 8000]")
	}
	reclassTimeoutMs, err := atoi("RECLASS_TIMEOUT_MS", 30000)
	if err != nil {
		return Config{}, err
	}
	if reclassTimeoutMs < 5000 {
		return Config{}, errors.New("RECLASS_TIMEOUT_MS must be >= 5000")
	}

	// Global Meta-Learning settings (Phase 105)
	metaLearnEnabled := getBool("METALEARN_ENABLED", false)
	metaLearnGlobalSpaceID := get("METALEARN_GLOBAL_SPACE_ID", "mdemg-global")
	metaLearnMinLayer, err := atoi("METALEARN_MIN_LAYER", 4)
	if err != nil {
		return Config{}, err
	}
	if metaLearnMinLayer < 1 || metaLearnMinLayer > 5 {
		return Config{}, errors.New("METALEARN_MIN_LAYER must be in range [1, 5]")
	}
	metaLearnMinUpdateCount, err := atoi("METALEARN_MIN_UPDATE_COUNT", 5)
	if err != nil {
		return Config{}, err
	}
	if metaLearnMinUpdateCount < 0 {
		return Config{}, errors.New("METALEARN_MIN_UPDATE_COUNT must be >= 0")
	}
	metaLearnProvider := get("METALEARN_PROVIDER", emergenceProvider)
	metaLearnModel := get("METALEARN_MODEL", emergenceModel)
	metaLearnMaxTokens, err := atoi("METALEARN_MAX_TOKENS", 500)
	if err != nil {
		return Config{}, err
	}
	if metaLearnMaxTokens < 100 || metaLearnMaxTokens > 4000 {
		return Config{}, errors.New("METALEARN_MAX_TOKENS must be in range [100, 4000]")
	}
	metaLearnTimeoutMs, err := atoi("METALEARN_TIMEOUT_MS", 15000)
	if err != nil {
		return Config{}, err
	}
	if metaLearnTimeoutMs < 1000 {
		return Config{}, errors.New("METALEARN_TIMEOUT_MS must be >= 1000")
	}

	// Jiminy Guidance settings (Phase Jiminy)
	jiminyEnabled := getBool("JIMINY_ENABLED", true)
	jiminyTimeoutMs, err := atoi("JIMINY_TIMEOUT_MS", 6000)
	if err != nil {
		return Config{}, err
	}
	jiminyMaxItems, err := atoi("JIMINY_MAX_ITEMS", 10)
	if err != nil {
		return Config{}, err
	}
	jiminyMinConfidence, err := atof("JIMINY_MIN_CONFIDENCE", 0.3)
	if err != nil {
		return Config{}, err
	}
	jiminyIncludeFrontiers := getBool("JIMINY_INCLUDE_FRONTIERS", true)
	jiminyFrontierMinSim, err := atof("JIMINY_FRONTIER_MIN_SIM", 0.5)
	if err != nil {
		return Config{}, err
	}
	jiminyEffectivenessEnabled := getBool("JIMINY_EFFECTIVENESS_ENABLED", true)
	jiminyEffectivenessTTLSec, err := atoi("JIMINY_EFFECTIVENESS_TTL_SEC", 1800)
	if err != nil {
		return Config{}, err
	}

	// Capability gap detection settings (Task #23)
	gapLowScoreThreshold, err := atof("GAP_LOW_SCORE_THRESHOLD", 0.5)
	if err != nil {
		return Config{}, err
	}
	if gapLowScoreThreshold < 0 || gapLowScoreThreshold > 1 {
		return Config{}, errors.New("GAP_LOW_SCORE_THRESHOLD must be in range [0, 1]")
	}
	gapMinOccurrences, err := atoi("GAP_MIN_OCCURRENCES", 3)
	if err != nil {
		return Config{}, err
	}
	if gapMinOccurrences < 1 {
		return Config{}, errors.New("GAP_MIN_OCCURRENCES must be >= 1")
	}
	gapAnalysisWindowHours, err := atoi("GAP_ANALYSIS_WINDOW_HOURS", 24)
	if err != nil {
		return Config{}, err
	}
	if gapAnalysisWindowHours < 1 {
		return Config{}, errors.New("GAP_ANALYSIS_WINDOW_HOURS must be >= 1")
	}
	gapMetricsWindowSize, err := atoi("GAP_METRICS_WINDOW_SIZE", 1000)
	if err != nil {
		return Config{}, err
	}
	if gapMetricsWindowSize < 100 {
		return Config{}, errors.New("GAP_METRICS_WINDOW_SIZE must be >= 100")
	}

	// RSIC settings (Phase 60b)
	rsicMicroEnabled := getBool("RSIC_MICRO_ENABLED", false)
	rsicMesoPeriodHours, err := atoi("RSIC_MESO_PERIOD_HOURS", 6)
	if err != nil {
		return Config{}, err
	}
	rsicMesoPeriodSessions, err := atoi("RSIC_MESO_PERIOD_SESSIONS", 10)
	if err != nil {
		return Config{}, err
	}
	rsicMacroCron := get("RSIC_MACRO_CRON", "0 3 * * 0")
	rsicMaxNodePrunePct, err := atof("RSIC_MAX_NODE_PRUNE_PCT", 0.05)
	if err != nil {
		return Config{}, err
	}
	rsicMaxEdgePrunePct, err := atof("RSIC_MAX_EDGE_PRUNE_PCT", 0.10)
	if err != nil {
		return Config{}, err
	}
	rsicRollbackWindow, err := atoi("RSIC_ROLLBACK_WINDOW", 3600)
	if err != nil {
		return Config{}, err
	}
	rsicWatchdogEnabled := getBool("RSIC_WATCHDOG_ENABLED", true)
	rsicWatchdogCheckSec, err := atoi("RSIC_WATCHDOG_CHECK_SEC", 300)
	if err != nil {
		return Config{}, err
	}
	rsicWatchdogDecayRate, err := atof("RSIC_WATCHDOG_DECAY_RATE", 0.1)
	if err != nil {
		return Config{}, err
	}
	rsicNudgeThreshold, err := atof("RSIC_NUDGE_THRESHOLD", 0.3)
	if err != nil {
		return Config{}, err
	}
	rsicWarnThreshold, err := atof("RSIC_WARN_THRESHOLD", 0.6)
	if err != nil {
		return Config{}, err
	}
	rsicForceThreshold, err := atof("RSIC_FORCE_THRESHOLD", 0.9)
	if err != nil {
		return Config{}, err
	}
	rsicCalibrationDays, err := atoi("RSIC_CALIBRATION_DAYS", 30)
	if err != nil {
		return Config{}, err
	}
	rsicMaxHistoryEntries, err := atoi("RSIC_MAX_HISTORY_ENTRIES", 1000)
	if err != nil {
		return Config{}, err
	}
	rsicMinConfidence, err := atof("RSIC_MIN_CONFIDENCE", 0.3)
	if err != nil {
		return Config{}, err
	}
	rsicTriggerCooldownSec, err := atoi("RSIC_TRIGGER_COOLDOWN_SEC", 300)
	if err != nil {
		return Config{}, err
	}
	rsicTriggerDedupeSec, err := atoi("RSIC_TRIGGER_DEDUPE_SEC", 600)
	if err != nil {
		return Config{}, err
	}
	rsicWatchdogSpaceID := get("RSIC_WATCHDOG_SPACE_ID", "mdemg-dev")
	rsicPersistenceEnabled := getBool("RSIC_PERSISTENCE_ENABLED", true)
	spacePruneIntervalHours, err := atoi("SPACE_PRUNE_INTERVAL_HOURS", 24)
	if err != nil {
		return Config{}, err
	}

	// Phase AR-3: LLM-powered intelligence
	rsicLLMReflectEnabled := getBool("RSIC_LLM_REFLECT_ENABLED", false)
	rsicLLMReflectProvider := get("RSIC_LLM_REFLECT_PROVIDER", emergenceProvider)
	rsicLLMReflectModel := get("RSIC_LLM_REFLECT_MODEL", emergenceModel)

	consultingLLMConstraintsEnabled := getBool("CONSULTING_LLM_CONSTRAINTS_ENABLED", false)
	consultingLLMConstraintsProvider := get("CONSULTING_LLM_CONSTRAINTS_PROVIDER", emergenceProvider)
	consultingLLMConstraintsModel := get("CONSULTING_LLM_CONSTRAINTS_MODEL", emergenceModel)

	retrievalLLMClassifyEnabled := getBool("RETRIEVAL_LLM_CLASSIFY_ENABLED", false)
	retrievalLLMClassifyProvider := get("RETRIEVAL_LLM_CLASSIFY_PROVIDER", emergenceProvider)
	retrievalLLMClassifyModel := get("RETRIEVAL_LLM_CLASSIFY_MODEL", emergenceModel)

	// Phase 38: UNTS Hash Verification
	untsEnabled := getBool("UNTS_ENABLED", false)
	untsBasePath := get("UNTS_BASE_PATH", ".")

	// Context Cooler tuning (Phase 45.5)
	coolerReinfWindowHours, err := atoi("COOLER_REINFORCEMENT_WINDOW_HOURS", 2)
	if err != nil {
		return Config{}, err
	}
	coolerStabilityIncrease, err := atof("COOLER_STABILITY_INCREASE_PER_REINFORCEMENT", 0.15)
	if err != nil {
		return Config{}, err
	}
	coolerDecayRate, err := atof("COOLER_STABILITY_DECAY_RATE", 0.1)
	if err != nil {
		return Config{}, err
	}
	coolerTombstoneThresh, err := atof("COOLER_TOMBSTONE_THRESHOLD", 0.05)
	if err != nil {
		return Config{}, err
	}
	coolerGradThresh, err := atof("COOLER_GRADUATION_THRESHOLD", 0.8)
	if err != nil {
		return Config{}, err
	}

	// Constraint Module (Phase 45.5)
	constraintDetectionEnabled := getBool("CONSTRAINT_DETECTION_ENABLED", true)
	constraintMinConfidence, err := atof("CONSTRAINT_MIN_CONFIDENCE", 0.6)
	if err != nil {
		return Config{}, err
	}
	constraintProtectFromDecay := getBool("CONSTRAINT_PROTECT_FROM_DECAY", true)

	// Web Scraper Module (Phase 51)
	scraperEnabled := getBool("SCRAPER_ENABLED", false)
	scraperDefaultSpaceID := get("SCRAPER_DEFAULT_SPACE_ID", "web-scraper")
	scraperMaxConcurrentJobs, err := atoi("SCRAPER_MAX_CONCURRENT_JOBS", 3)
	if err != nil {
		return Config{}, err
	}
	if scraperMaxConcurrentJobs < 1 {
		return Config{}, errors.New("SCRAPER_MAX_CONCURRENT_JOBS must be >= 1")
	}
	scraperDefaultDelayMs, err := atoi("SCRAPER_DEFAULT_DELAY_MS", 1000)
	if err != nil {
		return Config{}, err
	}
	if scraperDefaultDelayMs < 0 {
		return Config{}, errors.New("SCRAPER_DEFAULT_DELAY_MS must be >= 0")
	}
	scraperDefaultTimeoutMs, err := atoi("SCRAPER_DEFAULT_TIMEOUT_MS", 30000)
	if err != nil {
		return Config{}, err
	}
	if scraperDefaultTimeoutMs < 1000 {
		return Config{}, errors.New("SCRAPER_DEFAULT_TIMEOUT_MS must be >= 1000")
	}
	scraperCacheTTL, err := atoi("SCRAPER_CACHE_TTL_SECONDS", 3600)
	if err != nil {
		return Config{}, err
	}
	scraperRespectRobots := getBool("SCRAPER_RESPECT_ROBOTS_TXT", true)
	scraperMaxContentKB, err := atoi("SCRAPER_MAX_CONTENT_LENGTH_KB", 500)
	if err != nil {
		return Config{}, err
	}
	if scraperMaxContentKB < 10 {
		return Config{}, errors.New("SCRAPER_MAX_CONTENT_LENGTH_KB must be >= 10")
	}

	// Neo4j Backup & Restore (Phase 70)
	backupEnabled := getBool("BACKUP_ENABLED", true)
	backupStorageDir := get("BACKUP_STORAGE_DIR", ".mdemg/backups")
	backupFullCmd := get("BACKUP_FULL_CMD", "docker")
	backupNeo4jContainer := get("BACKUP_NEO4J_CONTAINER", "mdemg-neo4j")
	backupFullIntervalHours, err := atoi("BACKUP_FULL_INTERVAL_HOURS", 168)
	if err != nil {
		return Config{}, err
	}
	if backupFullIntervalHours < 1 {
		return Config{}, errors.New("BACKUP_FULL_INTERVAL_HOURS must be >= 1")
	}
	backupPartialIntervalHours, err := atoi("BACKUP_PARTIAL_INTERVAL_HOURS", 24)
	if err != nil {
		return Config{}, err
	}
	if backupPartialIntervalHours < 1 {
		return Config{}, errors.New("BACKUP_PARTIAL_INTERVAL_HOURS must be >= 1")
	}
	backupRetentionFullCount, err := atoi("BACKUP_RETENTION_FULL_COUNT", 2)
	if err != nil {
		return Config{}, err
	}
	backupRetentionPartialCount, err := atoi("BACKUP_RETENTION_PARTIAL_COUNT", 2)
	if err != nil {
		return Config{}, err
	}
	backupRetentionMaxAgeDays, err := atoi("BACKUP_RETENTION_MAX_AGE_DAYS", 30)
	if err != nil {
		return Config{}, err
	}
	backupRetentionMaxStorageGB, err := atoi("BACKUP_RETENTION_MAX_STORAGE_GB", 2)
	if err != nil {
		return Config{}, err
	}
	backupRetentionRunAfter := getBool("BACKUP_RETENTION_RUN_AFTER_BACKUP", true)

	// Phase 75: Relationship Extraction
	relExtractImports := getBool("REL_EXTRACT_IMPORTS", true)
	relExtractInheritance := getBool("REL_EXTRACT_INHERITANCE", true)
	relExtractCalls := getBool("REL_EXTRACT_CALLS", true)
	relCrossFileResolve := getBool("REL_CROSS_FILE_RESOLVE", true)
	goTypesEnabled := getBool("GO_TYPES_ANALYSIS_ENABLED", false)
	relMaxCallsPerFunc, err := atoi("REL_MAX_CALLS_PER_FUNCTION", 50)
	if err != nil {
		return Config{}, err
	}
	relBatchSize, err := atoi("REL_BATCH_SIZE", 500)
	if err != nil {
		return Config{}, err
	}
	relResolutionTimeout, err := atoi("REL_RESOLUTION_TIMEOUT_SEC", 60)
	if err != nil {
		return Config{}, err
	}

	// Phase 75B: Topology Hardening
	dynamicEdgesEnabled := getBool("DYNAMIC_EDGES_ENABLED", true)
	dynamicEdgeDegreeCap, err := atoi("DYNAMIC_EDGE_DEGREE_CAP", 10)
	if err != nil {
		return Config{}, err
	}
	dynamicEdgeMinConfidenceStr := get("DYNAMIC_EDGE_MIN_CONFIDENCE", "0.5")
	dynamicEdgeMinConfidence, err := strconv.ParseFloat(dynamicEdgeMinConfidenceStr, 64)
	if err != nil {
		return Config{}, fmt.Errorf("DYNAMIC_EDGE_MIN_CONFIDENCE must be float: %w", err)
	}
	l5EmergentEnabled := getBool("L5_EMERGENT_ENABLED", true)
	l5BridgeEvidenceMin, err := atoi("L5_BRIDGE_EVIDENCE_MIN", 1)
	if err != nil {
		return Config{}, err
	}
	l5SourceMinLayer, err := atoi("L5_SOURCE_MIN_LAYER", 3)
	if err != nil {
		return Config{}, err
	}
	symbolActivationEnabled := getBool("SYMBOL_ACTIVATION_ENABLED", true)
	secondaryLabelsEnabled := getBool("SECONDARY_LABELS_ENABLED", true)
	themeOfEdgeEnabled := getBool("THEME_OF_EDGE_ENABLED", true)

	// Phase 80: Meta-Cognition
	metaCogEnabled := getBool("METACOG_ENABLED", true)
	metaCogEmptyResumeCheck := getBool("METACOG_EMPTY_RESUME_CHECK", true)
	metaCogSignalDecayRate, err := atof("METACOG_SIGNAL_DECAY_RATE", 0.05)
	if err != nil {
		return Config{}, err
	}
	metaCogSignalBoostRate, err := atof("METACOG_SIGNAL_BOOST_RATE", 0.1)
	if err != nil {
		return Config{}, err
	}

	// CMS Hardening: configurable defaults
	cmsResumeMaxObs, err := atoi("CMS_RESUME_MAX_OBS", 20)
	if err != nil {
		return Config{}, err
	}
	cmsRecallTopK, err := atoi("CMS_RECALL_TOP_K", 10)
	if err != nil {
		return Config{}, err
	}
	cmsSummaryMaxChars, err := atoi("CMS_SUMMARY_MAX_CHARS", 200)
	if err != nil {
		return Config{}, err
	}
	cmsJiminyBaseConfidence, err := atof("CMS_JIMINY_BASE_CONFIDENCE", 0.5)
	if err != nil {
		return Config{}, err
	}

	// ===== ANN Optimization: Learning Subsystem =====
	learningCautiousDecayWindowHours, err := atoi("LEARNING_CAUTIOUS_DECAY_WINDOW_HOURS", 24)
	if err != nil {
		return Config{}, err
	}
	learningEtaConvMult, err := atof("LEARNING_ETA_CONVERSATION_MULT", 2.0)
	if err != nil {
		return Config{}, err
	}
	learningEtaConfigMult, err := atof("LEARNING_ETA_CONFIG_MULT", 1.5)
	if err != nil {
		return Config{}, err
	}
	learningEtaSameDirMult, err := atof("LEARNING_ETA_SAME_DIR_MULT", 1.2)
	if err != nil {
		return Config{}, err
	}
	learningScheduleEnabled := getBool("LEARNING_SCHEDULE_ENABLED", true)
	learningScheduleColdMult, err := atof("LEARNING_SCHEDULE_COLD_MULT", 2.0)
	if err != nil {
		return Config{}, err
	}
	learningScheduleLearningMult, err := atof("LEARNING_SCHEDULE_LEARNING_MULT", 1.0)
	if err != nil {
		return Config{}, err
	}
	learningScheduleWarmMult, err := atof("LEARNING_SCHEDULE_WARM_MULT", 0.5)
	if err != nil {
		return Config{}, err
	}
	learningScheduleSatMult, err := atof("LEARNING_SCHEDULE_SAT_MULT", 0.25)
	if err != nil {
		return Config{}, err
	}

	// ===== ANN Optimization: Retrieval Subsystem =====
	scoringActivationFloor, err := atof("SCORING_ACTIVATION_FLOOR", 0.05)
	if err != nil {
		return Config{}, err
	}
	scoringActivationSquared := getBool("SCORING_ACTIVATION_SQUARED", true)
	activationHop0MinWeight, err := atof("ACTIVATION_HOP0_MIN_WEIGHT", 0.5)
	if err != nil {
		return Config{}, err
	}
	activationHop1MinWeight, err := atof("ACTIVATION_HOP1_MIN_WEIGHT", 0.2)
	if err != nil {
		return Config{}, err
	}
	activationHop2MinWeight, err := atof("ACTIVATION_HOP2_MIN_WEIGHT", 0.05)
	if err != nil {
		return Config{}, err
	}
	scoringBypassThreshold, err := atof("SCORING_BYPASS_THRESHOLD", 0.85)
	if err != nil {
		return Config{}, err
	}
	scoringBypassWeight, err := atof("SCORING_BYPASS_WEIGHT", 0.15)
	if err != nil {
		return Config{}, err
	}
	scoringBypassCodeMult, err := atof("SCORING_BYPASS_CODE_MULT", 1.3)
	if err != nil {
		return Config{}, err
	}
	scoringBypassArchMult, err := atof("SCORING_BYPASS_ARCH_MULT", 0.5)
	if err != nil {
		return Config{}, err
	}
	scoringBM25Weight, err := atof("SCORING_BM25_WEIGHT", 0.15)
	if err != nil {
		return Config{}, err
	}
	if scoringBM25Weight < 0 || scoringBM25Weight > 1 {
		return Config{}, errors.New("SCORING_BM25_WEIGHT must be in range [0, 1]")
	}
	activationSteps, err := atoi("ACTIVATION_STEPS", 2)
	if err != nil {
		return Config{}, err
	}
	if activationSteps < 1 || activationSteps > 10 {
		return Config{}, errors.New("ACTIVATION_STEPS must be in range [1, 10]")
	}
	activationLambda, err := atof("ACTIVATION_LAMBDA", 0.15)
	if err != nil {
		return Config{}, err
	}
	if activationLambda < 0 || activationLambda > 0.9 {
		return Config{}, errors.New("ACTIVATION_LAMBDA must be in range [0, 0.9]")
	}

	// ===== ANN Optimization: Consolidation Subsystem =====
	l5GroundingMaxEdges, err := atoi("L5_GROUNDING_MAX_EDGES", 5)
	if err != nil {
		return Config{}, err
	}
	l5GroundingMinSim, err := atof("L5_GROUNDING_MIN_SIM", 0.4)
	if err != nil {
		return Config{}, err
	}
	l5GroundingInitialWeight, err := atof("L5_GROUNDING_INITIAL_WEIGHT", 0.5)
	if err != nil {
		return Config{}, err
	}
	edgeAttentionGroundedBy, err := atof("EDGE_ATTENTION_GROUNDED_BY", 0.70)
	if err != nil {
		return Config{}, err
	}

	// ===== ANN Optimization: Negative Feedback =====
	learningNegativeWeight, err := atof("LEARNING_NEGATIVE_WEIGHT", 0.15)
	if err != nil {
		return Config{}, err
	}
	learningNegativeDecayMult, err := atof("LEARNING_NEGATIVE_DECAY_MULT", 2.0)
	if err != nil {
		return Config{}, err
	}
	learningNegativeMaxPerRequest, err := atoi("LEARNING_NEGATIVE_MAX_PER_REQUEST", 20)
	if err != nil {
		return Config{}, err
	}

	// ===== ANN Optimization: Frontier Detection =====
	frontierMinEvidence, err := atoi("FRONTIER_MIN_EVIDENCE", 3)
	if err != nil {
		return Config{}, err
	}
	frontierMaxOutgoing, err := atoi("FRONTIER_MAX_OUTGOING", 2)
	if err != nil {
		return Config{}, err
	}

	// ===== ANN Optimization: Cluster Summary =====
	clusterSummaryEnabled := getBool("CLUSTER_SUMMARY_ENABLED", false)
	clusterSummaryProvider := get("CLUSTER_SUMMARY_PROVIDER", llmProvider)
	clusterSummaryModel := get("CLUSTER_SUMMARY_MODEL", llmModel)
	clusterSummaryMaxTokens, err := atoi("CLUSTER_SUMMARY_MAX_TOKENS", 100)
	if err != nil {
		return Config{}, err
	}
	if clusterSummaryMaxTokens < 20 || clusterSummaryMaxTokens > 1000 {
		return Config{}, errors.New("CLUSTER_SUMMARY_MAX_TOKENS must be in range [20, 1000]")
	}
	clusterSummaryTimeoutMs, err := atoi("CLUSTER_SUMMARY_TIMEOUT_MS", 5000)
	if err != nil {
		return Config{}, err
	}
	if clusterSummaryTimeoutMs < 500 {
		return Config{}, errors.New("CLUSTER_SUMMARY_TIMEOUT_MS must be >= 500")
	}
	clusterSummaryBatchSize, err := atoi("CLUSTER_SUMMARY_BATCH_SIZE", 50)
	if err != nil {
		return Config{}, err
	}
	if clusterSummaryBatchSize < 1 || clusterSummaryBatchSize > 500 {
		return Config{}, errors.New("CLUSTER_SUMMARY_BATCH_SIZE must be in range [1, 500]")
	}

	// Deterministic consolidation trigger
	consolidateOnWatchdog := getBool("CONSOLIDATE_ON_WATCHDOG_ENABLED", true)

	// Data transmission optimization settings
	compressionEnabled := getBool("COMPRESSION_ENABLED", true)
	compressionMinSize, err := atoi("COMPRESSION_MIN_SIZE", 1024)
	if err != nil {
		return Config{}, err
	}
	paginationMaxLimit, err := atoi("PAGINATION_MAX_LIMIT", 500)
	if err != nil {
		return Config{}, err
	}
	paginationDefLimit, err := atoi("PAGINATION_DEFAULT_LIMIT", 50)
	if err != nil {
		return Config{}, err
	}

	// Neo4j connection pool settings
	neo4jMaxPoolSize, err := atoi("NEO4J_MAX_POOL_SIZE", 100)
	if err != nil {
		return Config{}, err
	}
	neo4jAcquireTimeout, err := atoi("NEO4J_ACQUIRE_TIMEOUT_SEC", 60)
	if err != nil {
		return Config{}, err
	}
	neo4jMaxConnLifetime, err := atoi("NEO4J_MAX_CONN_LIFETIME_SEC", 3600)
	if err != nil {
		return Config{}, err
	}
	neo4jConnIdleTimeout, err := atoi("NEO4J_CONN_IDLE_TIMEOUT_SEC", 0)
	if err != nil {
		return Config{}, err
	}

	// Dynamic port allocation
	// Derive default PortRangeStart from ListenAddr port
	defaultPortStart := 9999
	if idx := strings.LastIndex(listen, ":"); idx >= 0 {
		if p, parseErr := strconv.Atoi(listen[idx+1:]); parseErr == nil {
			defaultPortStart = p
		}
	}
	portRangeStart, err := atoi("PORT_RANGE_START", defaultPortStart)
	if err != nil {
		return Config{}, err
	}
	// Default PORT_RANGE_END to portRangeStart + 100 to ensure a valid ascending range
	portRangeEnd, err := atoi("PORT_RANGE_END", portRangeStart+100)
	if err != nil {
		return Config{}, err
	}
	if portRangeStart > portRangeEnd {
		return Config{}, errors.New("PORT_RANGE_START must be <= PORT_RANGE_END")
	}
	portFilePath := get("PORT_FILE_PATH", ".mdemg.port")

	// Scheduled sync settings (Phase 9.2)
	syncIntervalMinutes, err := atoi("SYNC_INTERVAL_MINUTES", 0)
	if err != nil {
		return Config{}, err
	}
	if syncIntervalMinutes < 0 {
		return Config{}, errors.New("SYNC_INTERVAL_MINUTES must be >= 0")
	}
	syncStaleThresholdHours, err := atoi("SYNC_STALE_THRESHOLD_HOURS", 24)
	if err != nil {
		return Config{}, err
	}
	if syncStaleThresholdHours < 1 {
		return Config{}, errors.New("SYNC_STALE_THRESHOLD_HOURS must be >= 1")
	}
	var syncSpaceIDs []string
	if v := get("SYNC_SPACE_IDS", ""); v != "" {
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				syncSpaceIDs = append(syncSpaceIDs, s)
			}
		}
	}
	syncRepoPathMap := make(map[string]string)
	if v := get("SYNC_REPO_PATHS", ""); v != "" {
		for _, pair := range strings.Split(v, ",") {
			kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
			if len(kv) == 2 {
				syncRepoPathMap[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}
	}

	// ===== Phase 3: Production Readiness =====

	// Rate limiting settings
	rateLimitEnabled := getBool("RATE_LIMIT_ENABLED", true)
	rateLimitRPS, err := atof("RATE_LIMIT_RPS", 100.0)
	if err != nil {
		return Config{}, err
	}
	if rateLimitRPS <= 0 {
		return Config{}, errors.New("RATE_LIMIT_RPS must be > 0")
	}
	rateLimitBurst, err := atoi("RATE_LIMIT_BURST", 200)
	if err != nil {
		return Config{}, err
	}
	if rateLimitBurst <= 0 {
		return Config{}, errors.New("RATE_LIMIT_BURST must be > 0")
	}
	rateLimitByIP := getBool("RATE_LIMIT_BY_IP", true)

	// Circuit breaker settings
	cbEnabled := getBool("CIRCUIT_BREAKER_ENABLED", true)
	cbThreshold, err := atoi("CIRCUIT_BREAKER_THRESHOLD", 5)
	if err != nil {
		return Config{}, err
	}
	if cbThreshold < 1 {
		return Config{}, errors.New("CIRCUIT_BREAKER_THRESHOLD must be >= 1")
	}
	cbTimeout, err := atoi("CIRCUIT_BREAKER_TIMEOUT", 30)
	if err != nil {
		return Config{}, err
	}
	if cbTimeout < 1 {
		return Config{}, errors.New("CIRCUIT_BREAKER_TIMEOUT must be >= 1")
	}

	// Authentication settings
	authEnabled := getBool("AUTH_ENABLED", false)
	authMode := get("AUTH_MODE", "none")
	if authMode != "none" && authMode != "apikey" && authMode != "bearer" {
		return Config{}, errors.New("AUTH_MODE must be one of: none, apikey, bearer")
	}
	var authAPIKeys []string
	if v := get("AUTH_API_KEYS", ""); v != "" {
		for _, k := range strings.Split(v, ",") {
			k = strings.TrimSpace(k)
			if k != "" {
				authAPIKeys = append(authAPIKeys, k)
			}
		}
	}
	authJWTSecret := get("AUTH_JWT_SECRET", "")
	authJWTIssuer := get("AUTH_JWT_ISSUER", "")
	var authSkipEndpoints []string
	if v := get("AUTH_SKIP_ENDPOINTS", "/healthz,/readyz"); v != "" {
		for _, ep := range strings.Split(v, ",") {
			ep = strings.TrimSpace(ep)
			if ep != "" {
				authSkipEndpoints = append(authSkipEndpoints, ep)
			}
		}
	}

	// CORS settings
	corsEnabled := getBool("CORS_ENABLED", false)
	var corsAllowedOrigins []string
	if v := get("CORS_ALLOWED_ORIGINS", ""); v != "" {
		for _, o := range strings.Split(v, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				corsAllowedOrigins = append(corsAllowedOrigins, o)
			}
		}
	}
	var corsAllowedMethods []string
	if v := get("CORS_ALLOWED_METHODS", "GET,POST,PUT,DELETE,OPTIONS"); v != "" {
		for _, m := range strings.Split(v, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				corsAllowedMethods = append(corsAllowedMethods, m)
			}
		}
	}
	var corsAllowedHeaders []string
	if v := get("CORS_ALLOWED_HEADERS", "Accept,Authorization,Content-Type,X-API-Key,X-Request-ID"); v != "" {
		for _, h := range strings.Split(v, ",") {
			h = strings.TrimSpace(h)
			if h != "" {
				corsAllowedHeaders = append(corsAllowedHeaders, h)
			}
		}
	}
	corsAllowCredentials := getBool("CORS_ALLOW_CREDENTIALS", false)

	// TLS settings
	tlsEnabled := getBool("TLS_ENABLED", false)
	tlsCertFile := get("TLS_CERT_FILE", "")
	tlsKeyFile := get("TLS_KEY_FILE", "")
	if tlsEnabled && (tlsCertFile == "" || tlsKeyFile == "") {
		return Config{}, errors.New("TLS_CERT_FILE and TLS_KEY_FILE are required when TLS_ENABLED=true")
	}

	// Prometheus metrics settings
	metricsEnabled := getBool("METRICS_ENABLED", true)
	metricsNamespace := get("METRICS_NAMESPACE", "mdemg")

	// Graceful shutdown settings
	gracefulShutdownTimeout, err := atoi("GRACEFUL_SHUTDOWN_TIMEOUT", 30)
	if err != nil {
		return Config{}, err
	}
	if gracefulShutdownTimeout < 1 {
		return Config{}, errors.New("GRACEFUL_SHUTDOWN_TIMEOUT must be >= 1")
	}

	// Phase 48.3-48.4: Data Transmission & Connection Pooling
	// Embedding rate limiting
	embeddingRateLimitEnabled := getBool("EMBEDDING_RATE_LIMIT_ENABLED", false)
	embeddingOpenAIRPS, err := atof("EMBEDDING_OPENAI_RPS", 500)
	if err != nil {
		return Config{}, err
	}
	if embeddingOpenAIRPS <= 0 {
		return Config{}, errors.New("EMBEDDING_OPENAI_RPS must be > 0")
	}
	embeddingOpenAIBurst, err := atoi("EMBEDDING_OPENAI_BURST", 1000)
	if err != nil {
		return Config{}, err
	}
	if embeddingOpenAIBurst <= 0 {
		return Config{}, errors.New("EMBEDDING_OPENAI_BURST must be > 0")
	}
	embeddingOllamaRPS, err := atof("EMBEDDING_OLLAMA_RPS", 100)
	if err != nil {
		return Config{}, err
	}
	if embeddingOllamaRPS <= 0 {
		return Config{}, errors.New("EMBEDDING_OLLAMA_RPS must be > 0")
	}
	embeddingOllamaBurst, err := atoi("EMBEDDING_OLLAMA_BURST", 200)
	if err != nil {
		return Config{}, err
	}
	if embeddingOllamaBurst <= 0 {
		return Config{}, errors.New("EMBEDDING_OLLAMA_BURST must be > 0")
	}

	// Memory pressure monitoring
	memoryPressureEnabled := getBool("MEMORY_PRESSURE_ENABLED", false)
	memoryPressureThresholdMB, err := atoi("MEMORY_PRESSURE_THRESHOLD_MB", 4096)
	if err != nil {
		return Config{}, err
	}
	if memoryPressureThresholdMB < 256 {
		return Config{}, errors.New("MEMORY_PRESSURE_THRESHOLD_MB must be >= 256")
	}

	return Config{
		ListenAddr: listen,
		Neo4jURI: uri,
		Neo4jUser: user,
		Neo4jPass: pass,
		Neo4jBoltPort: neo4jBoltPort,
		Neo4jHTTPPort: neo4jHTTPPort,
		RequiredSchemaVersion: reqVer,
		VectorIndexName: idx,
		DefaultCandidateK: candK,
		DefaultTopK: topK,
		DefaultHopDepth: hops,
		MaxNeighborsPerNode: maxNbr,
		MaxTotalEdgesFetched: maxEdges,
		AllowedRelationshipTypes:  out,
		LearningEdgeCapPerRequest: learnCap,
		LearningMinActivation:     learnMinAct,
		LearningEta:               learnEta,
		LearningMu:                learnMu,
		LearningWMin:              learnWMin,
		LearningWMax:              learnWMax,
		LearningDecayPerDay:       learnDecayPerDay,
		LearningPruneThreshold:    learnPruneThreshold,
		LearningMaxEdgesPerNode:   learnMaxEdgesPerNode,
		LLMProvider:               llmProvider,
		LLMModel:                  llmModel,
		EmbeddingProvider:         embProvider,
		OpenAIAPIKey: openaiKey,
		OpenAIModel: openaiModel,
		OpenAIEndpoint: openaiEndpoint,
		LLMEndpoint:    llmEndpoint,
		OllamaEndpoint: ollamaEndpoint,
		OllamaModel: ollamaModel,
		EmbeddingCacheEnabled:     embCacheEnabled,
		EmbeddingCacheSize:        embCacheSize,
		QueryCacheEnabled:         queryCacheEnabled,
		QueryCacheCapacity:        queryCacheCapacity,
		QueryCacheTTLSeconds:      queryCacheTTL,
		SemanticEdgeOnIngest:      semEdgeEnabled,
		SemanticEdgeTopN:          semEdgeTopN,
		SemanticEdgeMinSimilarity: semEdgeMinSim,
		SemanticEdgeInitialWeight: semEdgeInitWeight,
		BatchIngestMaxItems:       batchMaxItems,
		HTTPReadTimeout:           httpReadTimeout,
		HTTPWriteTimeout:          httpWriteTimeout,
		AnomalyDetectionEnabled:   anomalyEnabled,
		AnomalyDuplicateThreshold: anomalyDupThreshold,
		AnomalyOutlierStdDevs:     anomalyOutlierStdDevs,
		AnomalyStaleDays:          anomalyStaleDays,
		AnomalyMaxCheckMs:         anomalyMaxCheckMs,
		ScoringAlpha:              scoringAlpha,
		ScoringBeta:               scoringBeta,
		ScoringGamma:              scoringGamma,
		ScoringDelta:              scoringDelta,
		ScoringPhi:                scoringPhi,
		ScoringKappa:              scoringKappa,
		ScoringRho:                scoringRho,
		ScoringRhoL0:              scoringRhoL0,
		ScoringRhoL1:              scoringRhoL1,
		ScoringRhoL2:              scoringRhoL2,
		ScoringConfigBoost:        scoringConfigBoost,
		ScoringPathBoost:          scoringPathBoost,
		TemporalEnabled:                temporalEnabled,
		TemporalSoftBoostMultiplier:    temporalSoftBoost,
		TemporalHardFilterEnabled:      temporalHardFilterEnabled,
		TemporalSourceTypeDecayEnabled: temporalSourceTypeDecayEnabled,
		ScoringRhoDocumentation:        scoringRhoDocumentation,
		ScoringRhoConfig:               scoringRhoConfig,
		ScoringRhoConversation:         scoringRhoConversation,
		ScoringRhoChangelog:            scoringRhoChangelog,
		TemporalStaleRefDays:           temporalStaleRefDays,
		TemporalStaleRefMaxPen:         temporalStaleRefMaxPen,
		LogFormat:                 logFormat,
		LogSkipHealth:             logSkipHealth,
		HiddenLayerEnabled:        hiddenEnabled,
		HiddenLayerClusterEps:     hiddenClusterEps,
		HiddenLayerMinSamples:     hiddenMinSamples,
		HiddenLayerMaxHidden:      hiddenMaxHidden,
		HiddenLayerMaxClusterSize: hiddenMaxClusterSize,
		HiddenLayerPathGroupDepth: hiddenPathGroupDepth,
		HiddenLayerBatchSize:      hiddenBatchSize,
		HiddenLayerForwardAlpha:   hiddenForwardAlpha,
		HiddenLayerForwardBeta:    hiddenForwardBeta,
		HiddenLayerBackwardSelf:   hiddenBackwardSelf,
		HiddenLayerBackwardBase:   hiddenBackwardBase,
		HiddenLayerBackwardConc:   hiddenBackwardConc,
		ConceptMergeEnabled:       conceptMergeEnabled,
		ConceptMergeThreshold:     conceptMergeThreshold,
		EdgeAttentionEnabled:      edgeAttentionEnabled,
		EdgeAttentionCoActivated:  edgeAttentionCoActivated,
		EdgeAttentionAssociated:   edgeAttentionAssociated,
		EdgeAttentionGeneralizes:  edgeAttentionGeneralizes,
		EdgeAttentionAbstractsTo:  edgeAttentionAbstractsTo,
		EdgeAttentionTemporal:     edgeAttentionTemporal,
		EdgeAttentionCodeBoost:       edgeAttentionCodeBoost,
		EdgeAttentionArchBoost:       edgeAttentionArchBoost,
		QueryAwareExpansionEnabled:   queryAwareExpansionEnabled,
		QueryAwareAttentionWeight:    queryAwareAttentionWeight,
		NodeEmbeddingCacheSize:       nodeEmbeddingCacheSize,
		EdgeTypeStrategy:             edgeTypeStrategy,
		StructuralEdgeTypes:          structuralEdgeTypes,
		LearnedEdgeTypes:             learnedEdgeTypes,
		HybridSwitchHop:              hybridSwitchHop,
		HybridRetrievalEnabled:       hybridEnabled,
		BM25TopK:                  bm25TopK,
		BM25Weight:                bm25Weight,
		VectorWeight:              vectorWeight,
		RRFConstant:               rrfConstant,
		RerankEnabled:             rerankEnabled,
		RerankProvider:            rerankProvider,
		RerankModel:               rerankModel,
		RerankTopN:                rerankTopN,
		RerankWeight:              rerankWeight,
		RerankTimeoutMs:           rerankTimeoutMs,
		RerankJinaKey:             rerankJinaKey,
		RerankJinaModel:           rerankJinaModel,
		RerankJinaURL:             rerankJinaURL,
		PluginsEnabled:            pluginsEnabled,
		PluginsDir:                pluginsDir,
		PluginSocketDir:           pluginSocketDir,
		MdemgVersion:              mdemgVersion,
		LinearTeamID:              linearTeamID,
		LinearWorkspaceID:         linearWorkspaceID,
		LinearWebhookSecret:         linearWebhookSecret,
		LinearWebhookSpaceID:        linearWebhookSpaceID,
		WebhookConfigs:              webhookConfigs,
		FileWatcherEnabled:          fileWatcherEnabled,
		FileWatcherConfigs:          fileWatcherConfigs,
		ConflictLogEnabled:          conflictLogEnabled,
		OrphanCleanupIntervalHours:  orphanCleanupIntervalHours,

		// Phase 47: Optimistic Retry + Edge Consistency
		OptimisticRetryEnabled:       optimisticRetryEnabled,
		OptimisticRetryMaxAttempts:   optimisticRetryMaxAttempts,
		OptimisticRetryBaseDelayMs:   optimisticRetryBaseDelayMs,
		OptimisticRetryMaxDelayMs:    optimisticRetryMaxDelayMs,
		OptimisticRetryMultiplier:    optimisticRetryMultiplier,
		EdgeStalenessCascadeEnabled:  edgeStalenessCascadeEnabled,
		EdgeStalenessRefreshBatchSize: edgeStalenessRefreshBatchSize,
		EdgeStalenessReclusterThresh: edgeStalenessReclusterThresh,

		LLMSummaryEnabled:           llmSummaryEnabled,
		LLMSummaryProvider:        llmSummaryProvider,
		LLMSummaryModel:           llmSummaryModel,
		LLMSummaryMaxTokens:       llmSummaryMaxTokens,
		LLMSummaryBatchSize:       llmSummaryBatchSize,
		LLMSummaryTimeoutMs:       llmSummaryTimeoutMs,
		LLMSummaryCacheSize:       llmSummaryCacheSize,

		// Phase 101: SME Synthesis
		SynthesisEnabled:   synthesisEnabled,
		SynthesisProvider:  synthesisProvider,
		SynthesisModel:     synthesisModel,
		SynthesisMaxTokens: synthesisMaxTokens,
		SynthesisTimeoutMs: synthesisTimeoutMs,

		// Phase 102: Intent Translation
		IntentEnabled:   intentEnabled,
		IntentProvider:  intentProvider,
		IntentModel:     intentModel,
		IntentMaxTokens: intentMaxTokens,
		IntentTimeoutMs: intentTimeoutMs,

		// Phase 103: Dynamic Emergence
		EmergenceEnabled:        emergenceEnabled,
		EmergenceProvider:       emergenceProvider,
		EmergenceModel:          emergenceModel,
		EmergenceMaxTokens:      emergenceMaxTokens,
		EmergenceTimeoutMs:      emergenceTimeoutMs,
		EmergenceMinWeight:      emergenceMinWeight,
		EmergenceMinClusterSize: emergenceMinClusterSize,
		EmergenceMaxClusters:    emergenceMaxClusters,

		// Phase 104: Active MCP Guardrails
		GuardrailEnabled:        guardrailEnabled,
		GuardrailProvider:       guardrailProvider,
		GuardrailModel:          guardrailModel,
		GuardrailMaxTokens:      guardrailMaxTokens,
		GuardrailTimeoutMs:      guardrailTimeoutMs,
		GuardrailMaxConstraints: guardrailMaxConstraints,

		// Phase Jiminy: Jiminy Guidance
		JiminyEnabled:          jiminyEnabled,
		JiminyTimeoutMs:        jiminyTimeoutMs,
		JiminyMaxItems:         jiminyMaxItems,
		JiminyMinConfidence:    jiminyMinConfidence,
		JiminyIncludeFrontiers: jiminyIncludeFrontiers,
		JiminyFrontierMinSim:          jiminyFrontierMinSim,
		JiminyEffectivenessEnabled:    jiminyEffectivenessEnabled,
		JiminyEffectivenessTTLSec:     jiminyEffectivenessTTLSec,

		// Dynamic Reclassification
		ReclassEnabled:       reclassEnabled,
		ReclassThreshold:     reclassThreshold,
		ReclassMaxSampleSize: reclassMaxSampleSize,
		ReclassMaxCategories: reclassMaxCategories,
		ReclassMaxIterations: reclassMaxIterations,
		ReclassMaxDepth:      reclassMaxDepth,
		ReclassProvider:      reclassProvider,
		ReclassModel:         reclassModel,
		ReclassMaxTokens:     reclassMaxTokens,
		ReclassTimeoutMs:     reclassTimeoutMs,

		// Phase 105: Global Meta-Learning
		MetaLearnEnabled:        metaLearnEnabled,
		MetaLearnGlobalSpaceID:  metaLearnGlobalSpaceID,
		MetaLearnMinLayer:       metaLearnMinLayer,
		MetaLearnMinUpdateCount: metaLearnMinUpdateCount,
		MetaLearnProvider:       metaLearnProvider,
		MetaLearnModel:          metaLearnModel,
		MetaLearnMaxTokens:      metaLearnMaxTokens,
		MetaLearnTimeoutMs:      metaLearnTimeoutMs,

		GapLowScoreThreshold:      gapLowScoreThreshold,
		GapMinOccurrences:         gapMinOccurrences,
		GapAnalysisWindowHours:    gapAnalysisWindowHours,
		GapMetricsWindowSize:      gapMetricsWindowSize,
		CompressionEnabled:        compressionEnabled,
		CompressionMinSize:        compressionMinSize,
		PaginationMaxLimit:        paginationMaxLimit,
		PaginationDefLimit:        paginationDefLimit,
		Neo4jMaxPoolSize:          neo4jMaxPoolSize,
		Neo4jAcquireTimeoutSec:    neo4jAcquireTimeout,
		Neo4jMaxConnLifetimeSec:   neo4jMaxConnLifetime,
		Neo4jConnIdleTimeoutSec:   neo4jConnIdleTimeout,
		PortRangeStart:            portRangeStart,
		PortRangeEnd:              portRangeEnd,
		PortFilePath:              portFilePath,
		SyncIntervalMinutes:       syncIntervalMinutes,
		SyncSpaceIDs:              syncSpaceIDs,
		SyncStaleThresholdHours:   syncStaleThresholdHours,
		SyncRepoPathMap:           syncRepoPathMap,

		// Phase 3: Production Readiness
		RateLimitEnabled:           rateLimitEnabled,
		RateLimitRPS:               rateLimitRPS,
		RateLimitBurst:             rateLimitBurst,
		RateLimitByIP:              rateLimitByIP,
		CircuitBreakerEnabled:      cbEnabled,
		CircuitBreakerThreshold:    cbThreshold,
		CircuitBreakerTimeoutSec:   cbTimeout,
		AuthEnabled:                authEnabled,
		AuthMode:                   authMode,
		AuthAPIKeys:                authAPIKeys,
		AuthJWTSecret:              authJWTSecret,
		AuthJWTIssuer:              authJWTIssuer,
		AuthSkipEndpoints:          authSkipEndpoints,
		CORSEnabled:                corsEnabled,
		CORSAllowedOrigins:         corsAllowedOrigins,
		CORSAllowedMethods:         corsAllowedMethods,
		CORSAllowedHeaders:         corsAllowedHeaders,
		CORSAllowCredentials:       corsAllowCredentials,
		TLSEnabled:                 tlsEnabled,
		TLSCertFile:                tlsCertFile,
		TLSKeyFile:                 tlsKeyFile,
		MetricsEnabled:             metricsEnabled,
		MetricsNamespace:           metricsNamespace,
		GracefulShutdownTimeoutSec: gracefulShutdownTimeout,

		// Phase 48.3-48.4: Data Transmission & Connection Pooling
		EmbeddingRateLimitEnabled: embeddingRateLimitEnabled,
		EmbeddingOpenAIRPS:        embeddingOpenAIRPS,
		EmbeddingOpenAIBurst:      embeddingOpenAIBurst,
		EmbeddingOllamaRPS:        embeddingOllamaRPS,
		EmbeddingOllamaBurst:      embeddingOllamaBurst,
		MemoryPressureEnabled:     memoryPressureEnabled,
		MemoryPressureThresholdMB: memoryPressureThresholdMB,

		// Phase 60b: RSIC
		RSICMicroEnabled:       rsicMicroEnabled,
		RSICMesoPeriodHours:    rsicMesoPeriodHours,
		RSICMesoPeriodSessions: rsicMesoPeriodSessions,
		RSICMacroCron:          rsicMacroCron,
		RSICMaxNodePrunePct:    rsicMaxNodePrunePct,
		RSICMaxEdgePrunePct:    rsicMaxEdgePrunePct,
		RSICRollbackWindow:     rsicRollbackWindow,
		RSICWatchdogEnabled:    rsicWatchdogEnabled,
		RSICWatchdogCheckSec:   rsicWatchdogCheckSec,
		RSICWatchdogDecayRate:  rsicWatchdogDecayRate,
		RSICNudgeThreshold:     rsicNudgeThreshold,
		RSICWarnThreshold:      rsicWarnThreshold,
		RSICForceThreshold:     rsicForceThreshold,
		RSICCalibrationDays:    rsicCalibrationDays,
		RSICMaxHistoryEntries:  rsicMaxHistoryEntries,
		RSICMinConfidence:      rsicMinConfidence,
		RSICTriggerCooldownSec:  rsicTriggerCooldownSec,
		RSICTriggerDedupeSec:    rsicTriggerDedupeSec,
		RSICWatchdogSpaceID:     rsicWatchdogSpaceID,
		RSICPersistenceEnabled:  rsicPersistenceEnabled,
		SpacePruneIntervalHours: spacePruneIntervalHours,

		// Phase AR-3: LLM-powered intelligence
		RSICLLMReflectEnabled:            rsicLLMReflectEnabled,
		RSICLLMReflectProvider:           rsicLLMReflectProvider,
		RSICLLMReflectModel:              rsicLLMReflectModel,
		ConsultingLLMConstraintsEnabled:  consultingLLMConstraintsEnabled,
		ConsultingLLMConstraintsProvider: consultingLLMConstraintsProvider,
		ConsultingLLMConstraintsModel:    consultingLLMConstraintsModel,
		RetrievalLLMClassifyEnabled:      retrievalLLMClassifyEnabled,
		RetrievalLLMClassifyProvider:     retrievalLLMClassifyProvider,
		RetrievalLLMClassifyModel:        retrievalLLMClassifyModel,

		// Phase 38: UNTS Hash Verification
		UNTSEnabled:  untsEnabled,
		UNTSBasePath: untsBasePath,

		CoolerReinforcementWindowHours:  coolerReinfWindowHours,
		CoolerStabilityIncreasePerReinf: coolerStabilityIncrease,
		CoolerStabilityDecayRate:        coolerDecayRate,
		CoolerTombstoneThreshold:        coolerTombstoneThresh,
		CoolerGraduationThreshold:       coolerGradThresh,

		ConstraintDetectionEnabled: constraintDetectionEnabled,
		ConstraintMinConfidence:    constraintMinConfidence,
		ConstraintProtectFromDecay: constraintProtectFromDecay,

		ConsolidateOnWatchdogEnabled: consolidateOnWatchdog,

		// Phase 51: Web Scraper
		ScraperEnabled:            scraperEnabled,
		ScraperDefaultSpaceID:     scraperDefaultSpaceID,
		ScraperMaxConcurrentJobs:  scraperMaxConcurrentJobs,
		ScraperDefaultDelayMs:     scraperDefaultDelayMs,
		ScraperDefaultTimeoutMs:   scraperDefaultTimeoutMs,
		ScraperCacheTTLSeconds:    scraperCacheTTL,
		ScraperRespectRobotsTxt:   scraperRespectRobots,
		ScraperMaxContentLengthKB: scraperMaxContentKB,

		// Phase 70: Neo4j Backup & Restore
		BackupEnabled:              backupEnabled,
		BackupStorageDir:           backupStorageDir,
		BackupFullCmd:              backupFullCmd,
		BackupNeo4jContainer:       backupNeo4jContainer,
		BackupFullIntervalHours:    backupFullIntervalHours,
		BackupPartialIntervalHours: backupPartialIntervalHours,
		BackupRetentionFullCount:   backupRetentionFullCount,
		BackupRetentionPartialCount: backupRetentionPartialCount,
		BackupRetentionMaxAgeDays:  backupRetentionMaxAgeDays,
		BackupRetentionMaxStorageGB: backupRetentionMaxStorageGB,
		BackupRetentionRunAfter:    backupRetentionRunAfter,

		// Phase 75: Relationship Extraction & Topology Hardening
		RelExtractImports:        relExtractImports,
		RelExtractInheritance:    relExtractInheritance,
		RelExtractCalls:          relExtractCalls,
		RelCrossFileResolve:      relCrossFileResolve,
		GoTypesEnabled:           goTypesEnabled,
		RelMaxCallsPerFunc:       relMaxCallsPerFunc,
		RelBatchSize:             relBatchSize,
		RelResolutionTimeout:     relResolutionTimeout,
		DynamicEdgesEnabled:      dynamicEdgesEnabled,
		DynamicEdgeDegreeCap:     dynamicEdgeDegreeCap,
		DynamicEdgeMinConfidence: dynamicEdgeMinConfidence,
		L5EmergentEnabled:        l5EmergentEnabled,
		L5BridgeEvidenceMin:      l5BridgeEvidenceMin,
		L5SourceMinLayer:         l5SourceMinLayer,
		SymbolActivationEnabled:  symbolActivationEnabled,
		SecondaryLabelsEnabled:   secondaryLabelsEnabled,
		ThemeOfEdgeEnabled:       themeOfEdgeEnabled,

		// Phase 80: Meta-Cognition
		MetaCogEnabled:          metaCogEnabled,
		MetaCogEmptyResumeCheck: metaCogEmptyResumeCheck,
		MetaCogSignalDecayRate:  metaCogSignalDecayRate,
		MetaCogSignalBoostRate:  metaCogSignalBoostRate,

		// CMS Hardening
		CMSResumeMaxObs:         cmsResumeMaxObs,
		CMSRecallTopK:           cmsRecallTopK,
		CMSSummaryMaxChars:      cmsSummaryMaxChars,
		CMSJiminyBaseConfidence: cmsJiminyBaseConfidence,

		// ANN Optimization: Learning
		LearningCautiousDecayWindowHours: learningCautiousDecayWindowHours,
		LearningEtaConversationMult:      learningEtaConvMult,
		LearningEtaConfigMult:            learningEtaConfigMult,
		LearningEtaSameDirMult:           learningEtaSameDirMult,
		LearningScheduleEnabled:          learningScheduleEnabled,
		LearningScheduleColdMult:         learningScheduleColdMult,
		LearningScheduleLearningMult:     learningScheduleLearningMult,
		LearningScheduleWarmMult:         learningScheduleWarmMult,
		LearningScheduleSatMult:          learningScheduleSatMult,

		// ANN Optimization: Retrieval
		ScoringActivationFloor:   scoringActivationFloor,
		ScoringActivationSquared: scoringActivationSquared,
		ScoringBM25Weight:        scoringBM25Weight,
		ActivationHop0MinWeight:  activationHop0MinWeight,
		ActivationHop1MinWeight:  activationHop1MinWeight,
		ActivationHop2MinWeight:  activationHop2MinWeight,
		ActivationSteps:          activationSteps,
		ActivationLambda:         activationLambda,
		ScoringBypassThreshold:   scoringBypassThreshold,
		ScoringBypassWeight:      scoringBypassWeight,
		ScoringBypassCodeMult:    scoringBypassCodeMult,
		ScoringBypassArchMult:    scoringBypassArchMult,

		// ANN Optimization: Consolidation
		L5GroundingMaxEdges:      l5GroundingMaxEdges,
		L5GroundingMinSim:        l5GroundingMinSim,
		L5GroundingInitialWeight: l5GroundingInitialWeight,
		EdgeAttentionGroundedBy:  edgeAttentionGroundedBy,

		// ANN Optimization: Cluster Summary
		ClusterSummaryEnabled:   clusterSummaryEnabled,
		ClusterSummaryProvider:  clusterSummaryProvider,
		ClusterSummaryModel:     clusterSummaryModel,
		ClusterSummaryMaxTokens: clusterSummaryMaxTokens,
		ClusterSummaryTimeoutMs: clusterSummaryTimeoutMs,
		ClusterSummaryBatchSize: clusterSummaryBatchSize,

		// ANN Optimization: Negative Feedback + Frontier
		LearningNegativeWeight:        learningNegativeWeight,
		LearningNegativeDecayMult:     learningNegativeDecayMult,
		LearningNegativeMaxPerRequest: learningNegativeMaxPerRequest,
		FrontierMinEvidence:           frontierMinEvidence,
		FrontierMaxOutgoing:           frontierMaxOutgoing,
	}, nil
}

// ResolveEndpoint determines the MDEMG API endpoint using a priority chain:
//  1. MDEMG_ENDPOINT env var (explicit override)
//  2. .mdemg.port file (dynamic port discovery)
//  3. LISTEN_ADDR env var (static config)
//  4. defaultAddr fallback
func ResolveEndpoint(defaultAddr string) string {
	// Priority 1: explicit env override
	if ep := strings.TrimSpace(os.Getenv("MDEMG_ENDPOINT")); ep != "" {
		return ep
	}

	// Priority 2: read port file
	portFile := strings.TrimSpace(os.Getenv("PORT_FILE_PATH"))
	if portFile == "" {
		portFile = ".mdemg.port"
	}
	if data, err := os.ReadFile(portFile); err == nil {
		port := strings.TrimSpace(string(data))
		if port != "" {
			return "http://localhost:" + port
		}
	}

	// Priority 3: LISTEN_ADDR env var
	if addr := strings.TrimSpace(os.Getenv("LISTEN_ADDR")); addr != "" {
		if strings.HasPrefix(addr, ":") {
			return "http://localhost" + addr
		}
		return "http://" + addr
	}

	// Priority 4: fallback
	return defaultAddr
}
