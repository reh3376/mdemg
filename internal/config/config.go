package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mdemg/migrations"
)

type Config struct {
	ListenAddr            string
	Neo4jURI              string
	Neo4jUser             string
	Neo4jPass             string
	Neo4jBoltPort         int // NEO4J_BOLT_PORT — preferred bolt port (default: 7687)
	Neo4jHTTPPort         int // NEO4J_HTTP_PORT — preferred HTTP port (default: 7474)
	RequiredSchemaVersion int

	VectorIndexName   string
	DefaultCandidateK int
	DefaultTopK       int
	DefaultHopDepth   int

	MaxNeighborsPerNode      int
	MaxTotalEdgesFetched     int
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
	// NEGFEED-001 — max prior session members the NEW observation
	// co-activates with per Observe (delta emission). 0 = all prior
	// members. Bounds the formerly-unbounded O(n^2) full-clique regen.
	LearningSessionCliqueWindow int // LEARNING_SESSION_CLIQUE_WINDOW (default 50)

	// Top-level LLM settings (cascade to all features)
	LLMProvider string // LLM_PROVIDER — top-level text-gen provider (default: "openai")
	LLMModel    string // LLM_MODEL — top-level text-gen model (default: "gpt-5.4-mini")

	// Embedding provider settings
	EmbeddingProvider   string // "openai", "ollama", or "" (disabled)
	OpenAIAPIKey        string
	OpenAIModel         string // default: text-embedding-3-large
	OpenAIEndpoint      string // default: https://api.openai.com/v1
	LLMEndpoint         string // LLM_ENDPOINT — override endpoint for LLM text-generation (default: uses OpenAIEndpoint)
	OllamaEndpoint      string // default: http://localhost:11434
	OllamaModel         string // default: qwen3-embedding:8b
	EmbeddingTargetDims int    // EMBEDDING_TARGET_DIMS — target embedding dimensions for MRL truncation (0 = use model native)

	// Model distribution (Sprint MODEL-DIST-001 — `mdemg model pull` and friends).
	// Every value is dynamic per the framework's no-hardcoding rule. The CLI surface
	// uses a ModelFetcher interface; v1 ships OllamaFetcher; future backends (hf, s3,
	// github-release, file) plug in via factory dispatch on ModelBackend without
	// touching the CLI surface.
	ModelBackend       string // MDEMG_MODEL_BACKEND — default: ollama
	ModelNamespace     string // MDEMG_MODEL_NAMESPACE — default: reh3376
	ModelName          string // MDEMG_MODEL_NAME — default: mdemg-llm-v1
	ModelQuants        string // MDEMG_MODEL_QUANTS — comma-separated allowlist; default: Q4_K_M,Q5_K_M,Q8_0
	ModelRamTiers      string // MDEMG_MODEL_RAM_TIERS — JSON map e.g. {"<16":"Q4_K_M","<24":"Q5_K_M","default":"Q8_0"}
	ModelQuant         string // MDEMG_MODEL_QUANT — selected quant; "auto" triggers RAM-tier dispatch (default: auto)
	AdapterBase        string // MDEMG_ADAPTER_BASE — base model tag for adapter Modelfile (default: qwen3:14b)
	ModelDir           string // MDEMG_MODEL_DIR — where mdemg model pull symlinks GGUFs (default: $HOME/.mdemg/models)
	OllamaModelsRoot   string // OLLAMA_MODELS — ollama-standard env (default: $HOME/.ollama/models)
	OllamaRegistryHost string // OLLAMA_HOST — ollama-standard env (default: registry.ollama.ai)
	ModelManifestPath  string // MDEMG_MODEL_MANIFEST_PATH — override for runtime quant manifest (default: empty = use embedded)

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

	// RETRIEVE_TIMEOUT_MS — bounded server-side deadline for a single retrieve
	// (RETRIEVAL-TYPED-EDGES-002 robustness fix). handleRetrieve previously used
	// the raw request ctx, bounded only by HTTPWriteTimeout=600s, so under Neo4j
	// load (a concurrent consolidation) the unbounded column Cypher queries hung
	// for tens of seconds to MINUTES — starving the hooks (short timeouts → no
	// memory recall) and holding Neo4j connections/slots that compounded the
	// contention. A tight deadline makes the per-column timeout fraction
	// meaningful and lets a slow retrieve fail-fast (not cached, per the
	// empty-result cache guard) instead of hanging. 0 disables (default: 20000).
	RetrieveTimeoutMs int

	// Anomaly detection settings
	AnomalyDetectionEnabled   bool    // Feature toggle (default: true)
	AnomalyDuplicateThreshold float64 // Vector similarity threshold for duplicates (default: 0.95)
	AnomalyOutlierStdDevs     float64 // Standard deviations for outlier detection (default: 2.0)
	AnomalyStaleDays          int     // Days after which an update is considered stale (default: 30)
	AnomalyMaxCheckMs         int     // Maximum time for anomaly checks in ms (default: 100)

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
	TemporalStaleRefDays   int     // TEMPORAL_STALE_REF_DAYS (default: 0 = disabled)
	TemporalStaleRefMaxPen float64 // TEMPORAL_STALE_REF_MAX_PENALTY (default: 0.15)

	// Scoring hyperparameters for retrieval ranking
	ScoringAlpha       float64 // Vector similarity weight (default: 0.60)
	ScoringBeta        float64 // Activation weight (default: 0.20)
	ScoringGamma       float64 // Recency weight (default: 0.15)
	ScoringDelta       float64 // Confidence weight (default: 0.05)
	ScoringPhi         float64 // Hub penalty coefficient (default: 0.08)
	ScoringKappa       float64 // Redundancy penalty coefficient (default: 0.12)
	ScoringRhoL0       float64 // Layer 0 decay rate per day (default: 0.05 - faster decay for files)
	ScoringRhoL1       float64 // Layer 1 decay rate per day (default: 0.02 - slower for hidden/concepts)
	ScoringRhoL2       float64 // Layer 2+ decay rate per day (default: 0.01 - slowest for abstractions)
	ScoringConfigBoost float64 // Score multiplier for config nodes (default: 1.15)
	ScoringPathBoost   float64 // Boost coefficient for path-matching nodes (default: 0.15)

	// Logging settings
	LogFormat     string // "text" (default) or "json"
	LogLevel      string // "debug", "info" (default), "warn", "error"
	LogSkipHealth bool   // Skip logging for /healthz and /readyz endpoints (default: false)

	// Hidden layer settings (V0005)
	HiddenLayerEnabled        bool    // Feature toggle for hidden layer processing (default: true)
	HiddenLayerClusterEps     float64 // DBSCAN epsilon - max distance for neighborhood (default: 0.1)
	HiddenLayerMinSamples     int     // DBSCAN min samples to form cluster (default: 3)
	HiddenLayerMaxHidden      int     // Max hidden nodes to create per consolidation run (default: 100)
	HiddenLayerMaxClusterSize int     // Max members per cluster before splitting (default: 200)
	HiddenLayerPathGroupDepth int     // Path segments for pre-grouping (default: 2)
	HiddenLayerBatchSize      int     // Batch size for clustering (0 = no limit, default: 0)
	HiddenLayerForwardAlpha   float64 // Weight of current embedding in forward pass (default: 0.6)
	HiddenLayerForwardBeta    float64 // Weight of aggregated embedding in forward pass (default: 0.4)
	HiddenLayerBackwardSelf   float64 // Weight of self in backward pass (default: 0.2)
	HiddenLayerBackwardBase   float64 // Weight of base signal in backward pass (default: 0.5)
	HiddenLayerBackwardConc   float64 // Weight of concept signal in backward pass (default: 0.3)

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
	NodeEmbeddingCacheTTLSec   int     // NODE_EMBEDDING_CACHE_TTL_SEC — TTL for embedding cache entries in seconds (default: 3600, 0 = no TTL)

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
	RerankEnabled           bool    // Enable LLM re-ranking (default: false)
	RerankProvider          string  // LLM provider for rerank (openai/ollama/jina)
	RerankModel             string  // Model for re-ranking (cascades from LLM_MODEL)
	RerankTopN              int     // Candidates to re-rank (default: 30)
	RerankWeight            float64 // Weight of rerank score in final (default: 0.4)
	RerankTimeoutMs         int     // Timeout for rerank call in ms (default: 3000)
	RerankMinBudgetMs       int     // RERANK_MIN_BUDGET_MS — LLM-HEALTH-INVESTIGATION-001: skip rerank when caller ctx has less than this remaining (default: 12000; 0 = disabled; floor 3000)
	NeuralRerankMinBudgetMs int     // NEURAL_RERANK_MIN_BUDGET_MS — NEURAL-RERANK-PRECHECK-001: same pre-check for provider=neural (default: 1500 = neural timeout 1000ms + 500ms margin; 0 = disabled; floor 500)
	RerankCompress          bool    // RERANK_COMPRESS — compress rerank candidate prompts (default: true)
	RerankJinaKey           string  // RERANK_JINA_API_KEY — Jina API key for cross-encoder reranking
	RerankJinaModel         string  // RERANK_JINA_MODEL — Jina reranker model (default: jina-reranker-v2-base-multilingual)
	RerankJinaURL           string  // RERANK_JINA_URL — Jina API base URL (default: https://api.jina.ai/v1)

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
	SynthesisCompress  bool   // SYNTHESIS_COMPRESS — compress SME synthesis prompts (default: true)

	// Intent Translation settings (Phase 102)
	IntentEnabled   bool   // INTENT_ENABLED — enable query rewriting before embedding (default: false)
	IntentProvider  string // INTENT_PROVIDER — LLM provider for intent translation (openai/ollama, default: openai)
	IntentModel     string // INTENT_MODEL — model for intent translation (default: gpt-4o-mini)
	IntentMaxTokens int    // INTENT_MAX_TOKENS — max tokens for rewritten query (default: 150)
	IntentTimeoutMs int    // INTENT_TIMEOUT_MS — timeout for intent translation in ms (default: 15000; INTENT-DISABLE-001 raised from 2000 which was below avg local-model latency ~4400ms → guaranteed timeouts. Intent is fail-open so operators MAY set lower for fast-fail, but the default honors the no-tight-timeout rule)

	// Query Classification settings (PROD-READINESS)
	QueryClassifyEnabled   bool   // QUERY_CLASSIFY_ENABLED — enable LLM query type classification (default: false)
	QueryClassifyProvider  string // QUERY_CLASSIFY_PROVIDER — LLM provider for classification (openai/ollama, default: llmProvider)
	QueryClassifyModel     string // QUERY_CLASSIFY_MODEL — model for classification (default: llmModel)
	QueryClassifyMaxTokens int    // QUERY_CLASSIFY_MAX_TOKENS — max tokens for classification response (default: 500)
	QueryClassifyTimeoutMs int    // QUERY_CLASSIFY_TIMEOUT_MS — timeout for classification in ms (default: 5000)

	// Dynamic Emergence settings (Phase 103)
	EmergenceEnabled                    bool    // EMERGENCE_ENABLED — enable LLM-driven concept naming (default: false)
	HiddenThemeIdentitySimThreshold     float64 // HIDDEN_THEME_IDENTITY_SIM_THRESHOLD — centroid cosine floor for matching a cluster to an EXISTING theme (in-place update, stable node identity) instead of recreating (default: 0.90; HIDDEN-CHURN-001)
	HiddenPatternIdentitySimThreshold   float64 // HIDDEN_PATTERN_IDENTITY_SIM_THRESHOLD — centroid cosine floor for the FALLBACK match of a cluster to an EXISTING L1 hidden pattern when member-overlap finds none (in-place update, stable node identity) instead of recreating (default: 0.90; HIDDEN-CHURN-002)
	HiddenPatternMemberJaccardThreshold float64 // HIDDEN_PATTERN_MEMBER_JACCARD_THRESHOLD — PRIMARY identity match: Jaccard overlap of a cluster's L0 member set with an existing pattern's members. Stable under KMeans repartition jitter (centroid-cosine alone left ~28% churn/cycle). 0 disables member matching (centroid-only). (default: 0.5; HIDDEN-CHURN-002)
	HiddenIncrementalPassesEnabled      bool    // HIDDEN_INCREMENTAL_PASSES_ENABLED — CONSOLIDATE-PERF-002: forward/backward passes gate on last_*_pass vs member/edge recency, recomputing only nodes whose inputs changed (default: false; flip after live smoke)
	DynamicEdgeIncrementalLookbackHours int     // DYNAMIC_EDGE_INCREMENTAL_LOOKBACK_HOURS — CONSOLIDATE-PERF-002: dynamic-edge seeds restricted to nodes created/updated within this window; 0 = full sweep (default: 6; only active when HIDDEN_INCREMENTAL_PASSES_ENABLED)
	HiddenIncrementalEnabled            bool    // HIDDEN_INCREMENTAL_ENABLED — default consolidation hidden step assigns only ORPHAN L0 nodes to existing patterns (incremental), never re-clustering stable patterns → ~0% identity churn + lower CPU; false = fall back to the CHURN-002 full re-cluster (default: true; HIDDEN-CHURN-003)
	HiddenIncrementalAssignSimThreshold float64 // HIDDEN_INCREMENTAL_ASSIGN_SIM_THRESHOLD — cosine floor for assigning an orphan L0 node to an existing pattern (below it the orphan clusters into a new pattern instead) (default: 0.80; HIDDEN-CHURN-003)
	ConsolidateTimeoutMs                int     // CONSOLIDATE_TIMEOUT_MS — server-side deadline for a full /v1/memory/consolidate cycle, DETACHED from caller cancellation so a client timeout can't abort it mid-cycle. CONSOLIDATE-PERF-001 raised the default 30→60 min: live cycles run ~38 min on an 83k-node graph, so the old 30-min default aborted the manual path mid-ForwardPass leaving partial state (the watchdog path has no deadline and completed). Floor 60000; HIDDEN-CHURN-002. (default: 3600000 = 60 min)
	HiddenThemeTargetRatio              float64 // HIDDEN_THEME_TARGET_RATIO — themes-per-observation ratio for KMeans k (default: 0.1 = ceil(n/10); was an inline equation; HIDDEN-CHURN-001 PR-B)
	HiddenThemeAssignSimThreshold       float64 // HIDDEN_THEME_ASSIGN_SIM_THRESHOLD — cosine floor for density-assigning NOISE observations to their nearest theme (edges only, no new themes; 0 disables; default: 0.70; PR-B coverage retune)
	ConversationCoverageAlertFloor      float64 // CONVERSATION_COVERAGE_ALERT_FLOOR — alert when themed/total conversation-observation coverage stays below this (default: 0.2; PR-B)
	ConversationCoverageMinObs          int     // CONVERSATION_COVERAGE_MIN_OBS — spaces with fewer live conversation observations don't emit the coverage gauge (statistically meaningless; default: 50, DH-005 confidence-threshold pattern)
	EmergenceProvider                   string  // EMERGENCE_PROVIDER — LLM provider for naming (openai/ollama, default: openai)
	EmergenceModel                      string  // EMERGENCE_MODEL — model for naming (default: gpt-4o-mini)
	EmergenceMaxTokens                  int     // EMERGENCE_MAX_TOKENS — max tokens for naming response (default: 500, range 100-4000)
	EmergenceTimeoutMs                  int     // EMERGENCE_TIMEOUT_MS — timeout for naming call in ms (default: 10000, min 1000)
	EmergenceMinWeight                  float64 // EMERGENCE_MIN_WEIGHT — min CO_ACTIVATED_WITH weight for clustering (default: 0.3, range 0.0-1.0)
	EmergenceMinClusterSize             int     // EMERGENCE_MIN_CLUSTER_SIZE — min nodes per cluster (default: 3, min 2)
	EmergenceMaxClusters                int     // EMERGENCE_MAX_CLUSTERS — max clusters per run (default: 10, min 1)

	// Active MCP Guardrails settings (Phase 104)
	GuardrailEnabled        bool   // GUARDRAIL_ENABLED — enable guardrail validation (default: false)
	GuardrailProvider       string // GUARDRAIL_PROVIDER — LLM provider for evaluation (openai/ollama, default: openai)
	GuardrailModel          string // GUARDRAIL_MODEL — model for evaluation (default: gpt-4o-mini)
	GuardrailMaxTokens      int    // GUARDRAIL_MAX_TOKENS — max tokens for evaluation response (default: 1000, range 100-4000)
	GuardrailTimeoutMs      int    // GUARDRAIL_TIMEOUT_MS — timeout for evaluation in ms (default: 5000, min 1000)
	GuardrailMaxConstraints int    // GUARDRAIL_MAX_CONSTRAINTS — max constraints per evaluation (default: 10, range 1-50)
	GuardrailCompress       bool   // GUARDRAIL_COMPRESS — compress guardrail eval prompts (default: true)

	// GUARDRAIL-PRODUCER-001: async producer path — PostToolUse hook fires
	// fire-and-forget evaluations of real Write/Edit changes so
	// guardrail.evaluate accumulates production training rows.
	GuardrailProducerEnabled       bool // GUARDRAIL_PRODUCER_ENABLED — accept async producer evaluations (default: false; flip after live smoke)
	GuardrailProducerMaxConcurrent int  // GUARDRAIL_PRODUCER_MAX_CONCURRENT — max simultaneous detached evaluations, excess dropped (default: 1, floor 1)
	GuardrailIncludeCorrections    bool // GUARDRAIL_INCLUDE_CORRECTIONS — retrieval also matches role_type='correction' nodes (Warning-tier; default: false; flip after live smoke)

	// Dynamic Reclassification settings
	ReclassEnabled       bool    // RECLASS_ENABLED — enable LLM-based reclassification of oversized categories (default: true)
	ReclassThreshold     float64 // RECLASS_THRESHOLD — min fraction of total nodes to trigger (default: 0.25, range: 0.05-0.90)
	ReclassMaxSampleSize int     // RECLASS_MAX_SAMPLE_SIZE — max summaries sent to LLM per category (default: 150, range: 20-500)
	ReclassMaxCategories int     // RECLASS_MAX_CATEGORIES — max sub-categories LLM may propose (default: 10, range: 3-20)
	ReclassMaxIterations int     // RECLASS_MAX_ITERATIONS — max reclassification loops until convergence (default: 5, range: 1-10)
	ReclassMaxDepth      int     // RECLASS_MAX_DEPTH — max dot-path taxonomy depth (default: 4, range: 1-10)
	ReclassProvider      string  // RECLASS_PROVIDER — LLM provider (openai/ollama, default: from EMERGENCE_PROVIDER)
	ReclassModel         string  // RECLASS_MODEL — model name (default: gpt-4.1)
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
	JiminyEnabled bool // JIMINY_ENABLED — enable Jiminy inner voice guidance (default: true)
	// NEGFEED-001 — on a contradicted guidance outcome, weaken co-activations
	// among the guidance's source nodes (anti-Hebbian). Safe (q<>r guard,
	// weight floors at 0, single-source = no-op). Default true.
	JiminyContradictedWeakenEnabled bool    // JIMINY_CONTRADICTED_WEAKEN_ENABLED
	JiminyTimeoutMs                 int     // JIMINY_TIMEOUT_MS — direct Guide() budget; 0 = derive from JIMINY_WARM_COMPUTE_TIMEOUT_MS (default: 0; the old independent 15s starved fresh installs)
	JiminyMaxItems                  int     // JIMINY_MAX_ITEMS — max guidance items returned (default: 10)
	JiminyMinConfidence             float64 // JIMINY_MIN_CONFIDENCE — minimum confidence to include item (default: 0.3)
	// JIMINY-ACTIONABILITY-001 — bias the surfaced guidance set toward the
	// actionable class (constraint/correction) over abstractions. Default-preserving.
	JiminySurfaceActionableWeight           float64 // JIMINY_SURFACE_ACTIONABLE_WEIGHT — sort-key multiplier for actionable types (default: 1.0 = no-op, floor >0)
	JiminySurfaceMinActionable              int     // JIMINY_SURFACE_MIN_ACTIONABLE — reserve at least this many surfaced slots for actionable items (default: 0 = off)
	JiminySurfaceMinActionableFraction      float64 // JIMINY_SURFACE_MIN_ACTIONABLE_FRACTION — reserve at least this fraction of maxItems for actionable items (default: 0.0 = off, [0,1])
	JiminySurfaceMaxAbstractionFraction     float64 // JIMINY_SURFACE_MAX_ABSTRACTION_FRACTION — cap the abstraction share of the surfaced set (default: 1.0 = no cap, (0,1])
	JiminyDirectiveSynthesisEnabled         bool    // JIMINY_DIRECTIVE_SYNTHESIS_ENABLED — Lever B: synthesize abstraction items as imperative directives (default: false)
	JiminyDirectiveSynthesisMaxPromptTokens int     // JIMINY_DIRECTIVE_SYNTHESIS_MAX_PROMPT_TOKENS — bound on the directive synthesis prompt (default: 3500, floor 1000)
	// JIMINY-CORPUS-001 Epic 3 — repetition control for guidance surfacing.
	// Lever A: per-session surfacing cooldown — a node ignored N consecutive times
	// in a session is suppressed from surfacing (releasable: a followed/partial
	// outcome resets the counter; never suppresses ALL guidance — least-recently-
	// ignored items are released as a fallback and the min-actionable quota stays satisfiable).
	JiminySurfaceCooldownIgnoredCount int // JIMINY_SURFACE_COOLDOWN_IGNORED_COUNT — consecutive ignored outcomes in a session before a node cools down (default: 3, 0 disables)
	JiminySurfaceCooldownCapacity     int // JIMINY_SURFACE_COOLDOWN_CAPACITY — LRU bound on tracked (session,node) cooldown entries (default: 5000)
	// Lever B: effectiveness prior — soft down-weight of chronically-ignored nodes in the
	// surfaced-set ordering. Gates on the per-constraint effectiveness RATE (followed/surfaced,
	// stable [0,1] outcome ratio — RRF-SCALE-001-safe, NEVER the RRF retrieval score).
	JiminySurfaceEffectivenessPriorWeight     float64 // JIMINY_SURFACE_EFFECTIVENESS_PRIOR_WEIGHT — prior strength in [0,1]; multiplier is (1-w)+w·rate, soft re-rank only (default: 0.3, 0 disables)
	JiminySurfaceEffectivenessPriorTTLSec     int     // JIMINY_SURFACE_EFFECTIVENESS_PRIOR_TTL_SEC — per-space effectiveness cache TTL (default: 300)
	JiminySurfaceEffectivenessPriorMinSamples int     // JIMINY_SURFACE_EFFECTIVENESS_PRIOR_MIN_SAMPLES — minimum surfaced outcomes before the prior applies to a node; below it the node is neutral, not penalised (default: 5)
	// JIMINY-ACTIONABILITY-001 Lever C (Epic 5): guarantee actionable candidates enter the guidance pool.
	JiminyGuidanceConstraintBiasEnabled bool    // JIMINY_GUIDANCE_CONSTRAINT_BIAS_ENABLED — Lever C: fetch top-K constraint/correction nodes by embedding similarity and merge into the guidance pool (default: false). Addresses the Epic-4 finding that retrieval surfaces no actionable candidates for most contexts.
	JiminyGuidanceConstraintIncludeTopK int     // JIMINY_GUIDANCE_CONSTRAINT_INCLUDE_TOPK — how many actionable nodes to merge (default: 5)
	JiminyGuidanceConstraintSimFloor    float64 // JIMINY_GUIDANCE_CONSTRAINT_SIM_FLOOR — cosine (vector-index) similarity floor for a merged actionable node — STABLE [0,1] scale, NOT the RRF score (RRF-SCALE-001-safe). 0 disables the floor. (default: 0.30)
	// JIMINY-OUTCOME-001 — minimum vector-index cosine similarity for an embedding-based
	// constraint-code match. Concept-abstracted guidance rarely shares 3+ literal words
	// with raw constraint text, so keyword matching missed everything and the Neo4j
	// GUIDANCE_OUTCOME edge sink went dormant; embedding match links them. Keyword
	// matching remains the fallback. (default: 0.55)
	JiminyConstraintCodeSimThreshold float64 // JIMINY_CONSTRAINT_CODE_SIM_THRESHOLD
	// DORMANT-CENSUS-001 — blend weight for the Hebbian signal strength
	// (ape.SignalLearner.GetStrength, V0024-persisted) in the Guide() sort:
	// within equal priority, items order by (1-w)·confidence + w·strength.
	// Ordering only — selection/filtering untouched. 0 disables (pure
	// confidence, the pre-census behavior). The learner's emission/response
	// stream had been live since HOOKWIRE-001 with a never-read read side.
	JiminySignalStrengthWeight float64 // JIMINY_SIGNAL_STRENGTH_WEIGHT (default: 0.2; 0 = off)

	// Jiminy Warm Store (event-driven pre-computation)
	JiminyWarmEnabled     bool // JIMINY_WARM_ENABLED — enable warm store for pre-computed guidance (default: true)
	JiminyWarmDebounceSec int  // JIMINY_WARM_DEBOUNCE_SEC — min seconds between warm computations (default: 10)
	// GUIDANCE-SYNTH-001 — timeout for the Guide() compute on the warm path AND
	// the synchronous /v1/jiminy/guide handler. Was a hardcoded 30s (in both
	// handlers) that starved synthesis (per-node classifier ~15s + synthesis
	// 8-50s > 30s). Default 90s leaves headroom. Resolve via the single
	// JiminyWarmComputeTimeout() method — do NOT re-default the literal at call
	// sites. No-hardcoding rule (single source of truth).
	JiminyWarmComputeTimeoutMs int     // JIMINY_WARM_COMPUTE_TIMEOUT_MS (default: DefaultJiminyWarmComputeTimeoutMs)
	JiminyReformulateTimeoutMs int     // JIMINY_REFORMULATE_TIMEOUT_MS — /reformulate budget; 0 = derive from warm-compute budget (default: 0; was hardcoded 10s)
	JiminyFeedbackTimeoutMs    int     // JIMINY_FEEDBACK_TIMEOUT_MS — server-side budget for /v1/jiminy/feedback outcome processing, detached from the hook's connection lifetime (default: 60000; 0 = no timeout)
	JiminyWarmMaxAgeSec        int     // JIMINY_WARM_MAX_AGE_SEC — max age before guidance is considered stale (default: 300)
	JiminyIncludeFrontiers     bool    // JIMINY_INCLUDE_FRONTIERS — include frontier node suggestions (default: true)
	JiminyFrontierMinSim       float64 // JIMINY_FRONTIER_MIN_SIM — min similarity for frontier nodes (default: 0.5)
	JiminyEffectivenessTTLSec  int     // JIMINY_EFFECTIVENESS_TTL_SEC — TTL for tracked guidance in seconds (default: 86400; must exceed realistic feedback delay)

	// Jiminy J7: Full-Spectrum Retrieval
	JiminyRetrievalEnabled  bool // JIMINY_RETRIEVAL_ENABLED — use full retrieval pipeline (default: true)
	JiminyRetrievalTopK     int  // JIMINY_RETRIEVAL_TOP_K — max results from retrieval pipeline (default: 10)
	JiminyRetrievalHopDepth int  // JIMINY_RETRIEVAL_HOP_DEPTH — graph hop depth for retrieval (default: 2)

	// Jiminy J8: LLM Synthesis
	JiminySynthesisEnabled   bool   // JIMINY_SYNTHESIS_ENABLED — enable LLM guidance synthesis (default: false)
	JiminySynthesisProvider  string // JIMINY_SYNTHESIS_PROVIDER — LLM provider (default: inherits LLM_PROVIDER)
	JiminySynthesisModel     string // JIMINY_SYNTHESIS_MODEL — LLM model (default: inherits LLM_MODEL)
	JiminySynthesisMaxTokens int    // JIMINY_SYNTHESIS_MAX_TOKENS — max tokens for synthesis (default: 1000)
	JiminySynthesisTimeoutMs int    // JIMINY_SYNTHESIS_TIMEOUT_MS — timeout for LLM synthesis (default: 30000)

	// Jiminy J9: Output Evaluation
	JiminyEvaluateEnabled        bool // JIMINY_EVALUATE_ENABLED — enable output evaluation endpoint (default: true)
	JiminyEvaluateTimeoutMs      int  // JIMINY_EVALUATE_TIMEOUT_MS — timeout for evaluation (default: 30000)
	JiminyEvaluateMaxConstraints int  // JIMINY_EVALUATE_MAX_CONSTRAINTS — max constraints to check (default: 10)

	// Jiminy J13: Evaluator LLM Reasoning
	JiminyEvaluateLLMEnabled   bool   // JIMINY_EVALUATE_LLM_ENABLED — enable LLM Tier 2 evaluation (default: false)
	JiminyEvaluateLLMProvider  string // JIMINY_EVALUATE_LLM_PROVIDER — LLM provider (default: inherits LLM_PROVIDER)
	JiminyEvaluateLLMModel     string // JIMINY_EVALUATE_LLM_MODEL — LLM model (default: inherits LLM_MODEL)
	JiminyEvaluateLLMTimeoutMs int    // JIMINY_EVALUATE_LLM_TIMEOUT_MS — LLM timeout (default: 30000)
	JiminyEvaluateLLMMaxTokens int    // JIMINY_EVALUATE_LLM_MAX_TOKENS — max response tokens (default: 2000)

	// Jiminy J11: Semantic Outcome Classification
	JiminyOutcomeClassifierEnabled  bool    // JIMINY_OUTCOME_CLASSIFIER_ENABLED — enable semantic classifier (default: true)
	JiminyOutcomeLLMEnabled         bool    // JIMINY_OUTCOME_LLM_ENABLED — enable LLM Tier 2 classification (default: true)
	JiminyOutcomeSimilarityHigh     float64 // JIMINY_OUTCOME_SIMILARITY_HIGH — threshold for "followed" (default: 0.55)
	JiminyOutcomeSimilarityLow      float64 // JIMINY_OUTCOME_SIMILARITY_LOW — threshold for "ignored" (default: 0.20)
	JiminyOutcomeNotApplicableSim   float64 // JIMINY_OUTCOME_NOT_APPLICABLE_SIMILARITY — relevance gate: below this the guidance did not apply → not_applicable; [this, LOW) is a real "ignored" (default: 0.10; ≤0 disables — whole sub-LOW tail is not_applicable)
	JiminyOutcomeLLMMaxTokens       int     // JIMINY_OUTCOME_LLM_MAX_TOKENS — max tokens for classification (default: 100)
	JiminyOutcomeCacheSize          int     // JIMINY_OUTCOME_CACHE_SIZE — LRU cache capacity (default: 256)
	JiminyClassifyCompress          bool    // JIMINY_CLASSIFY_COMPRESS — compress outcome classification prompts (default: true)
	JiminyNonViolationCreditEnabled bool    // JIMINY_NONVIOLATION_CREDIT_ENABLED — extend tier-2 LLM classifier prompt with a "non-violation credit for must_not" clause. Routes unrelated-context ignored verdicts to not_applicable (already filtered from constraint_outcomes by the writer gate). Predicted to lift constraint follow rate 10%→~20% by shrinking the actionable denominator. Default false; operator runs the 3-day A/B recipe (docs/development/jiminy-actionability-compliance-credit-001/ab_recipe.md) before flipping (JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001).
	JiminyDedupSimilarityThreshold  float64 // JIMINY_DEDUP_SIMILARITY_THRESHOLD — semantic dedup cosine threshold (default: 0.85)
	JiminyCorrectionDecayRate       float64 // JIMINY_CORRECTION_DECAY_RATE — time-decay lambda for corrections (default: 0.01)
	J17TicketCacheSize              int     // J17_TICKET_CACHE_SIZE — max entries in ticket LRU cache (default: 1000)

	// Jiminy J15: Synthesis temperature
	JiminySynthesisTemperature *float64 // JIMINY_SYNTHESIS_TEMPERATURE — LLM temperature (default: nil = API default)

	// Jiminy J16: Input Context Limits (0 = unlimited; default 200000 ≈ 50K tokens, safe for 128K model windows)
	JiminyGuidanceContextMaxChars int // JIMINY_GUIDANCE_CONTEXT_MAX_CHARS — max chars for agent context in guidance (default: 200000)
	JiminyGuidanceOutputMaxChars  int // JIMINY_GUIDANCE_OUTPUT_MAX_CHARS — max chars for agent output in guidance (default: 200000)
	JiminyEvaluateOutputMaxChars  int // JIMINY_EVALUATE_OUTPUT_MAX_CHARS — max chars for agent output in eval (default: 200000)
	JiminyEvaluateItemMaxChars    int // JIMINY_EVALUATE_ITEM_MAX_CHARS — max chars per constraint/correction in eval (default: 0 = unlimited)

	// Jiminy J12: Session Escalation
	JiminyEscalationEnabled        bool   // JIMINY_ESCALATION_ENABLED — enable session escalation (default: true)
	JiminyEscalationWarnAfter      int    // JIMINY_ESCALATION_WARN_AFTER — ignores before WARNED (default: 2)
	JiminyEscalationEscalateAfter  int    // JIMINY_ESCALATION_ESCALATE_AFTER — ignores before ESCALATED (default: 4)
	JiminyEscalationBlockAfter     int    // JIMINY_ESCALATION_BLOCK_AFTER — ignores before BLOCKED (default: 6)
	JiminyEscalationBlockEnabled   bool   // JIMINY_ESCALATION_BLOCK_ENABLED — enable BLOCKED state (default: false)
	JiminyEscalationDecayMinutes   int    // JIMINY_ESCALATION_DECAY_MINUTES — reset after inactivity (default: 60)
	JiminyEscalationPersistEnabled bool   // JIMINY_ESCALATION_PERSIST_ENABLED — persist escalation to Neo4j (default: true)
	JiminyStrictStatePath          string // JIMINY_STRICT_STATE_PATH — path to strict-mode state file (default: ~/.mdemg/.jiminy-strict-mode)

	// Jiminy Code Comprehension Feedback Loop
	JiminyCodeRegenEnabled    bool    // JIMINY_CODE_REGEN_ENABLED — enable code comprehension feedback loop (default: false)
	JiminyCodeRegenThreshold  float64 // JIMINY_CODE_REGEN_THRESHOLD — avg comprehension below this triggers regen (default: 0.3)
	JiminyCodeRegenMinSamples int     // JIMINY_CODE_REGEN_MIN_SAMPLES — minimum samples before evaluating (default: 10)

	// RSIC Jiminy Integration (J10)

	// J17: AI-to-AI Communication Protocol
	J17Enabled            bool   // J17_ENABLED — enable J17 protocol for compact communication (default: true)
	J17TicketSecret       string // J17_TICKET_SECRET — HMAC-SHA256 signing key (auto-generated if empty)
	J17TicketTTLHours     int    // J17_TICKET_TTL_HOURS — session ticket TTL in hours (default: 4)
	J17SequenceBufferSize int    // J17_SEQUENCE_BUFFER_SIZE — ring buffer size for event replay (default: 1000)
	J17ReplayMaxEvents    int    // J17_REPLAY_MAX_EVENTS — max events replayed on resume (default: 50)
	J17BootstrapEnabled   bool   // J17_BOOTSTRAP_ENABLED — enable bootstrap protocol for new sessions (default: true)

	// J17-2: Three-Tier Encoding
	J17CodegenEnabled  bool   // J17_CODEGEN_ENABLED — enable constraint code generation (default: inherits J17)
	J17CodegenProvider string // J17_CODEGEN_PROVIDER — LLM provider for codegen (default: inherits synthesis)
	J17CodegenModel    string // J17_CODEGEN_MODEL — LLM model for codegen (default: inherits synthesis)
	J17DefaultTier     int    // J17_DEFAULT_TIER — default encoding tier 1-3 (default: 1)

	// J17-3: Trust Score
	J17TrustInitial                float64 // J17_TRUST_INITIAL — starting trust score (default: 0.65)
	J17TrustBoostPerFollow         float64 // J17_TRUST_BOOST_PER_FOLLOW — trust increase per followed constraint (default: 0.05)
	J17TrustDecayPerIgnore         float64 // J17_TRUST_DECAY_PER_IGNORE — trust decrease per ignored constraint (default: 0.02)
	J17TrustDecayPerContradict     float64 // J17_TRUST_DECAY_PER_CONTRADICT — trust decrease per contradicted constraint (default: 0.04)
	J17TrustHighThreshold          float64 // J17_TRUST_HIGH_THRESHOLD — above this → dense encoding (default: 0.75)
	J17TrustLowThreshold           float64 // J17_TRUST_LOW_THRESHOLD — below this → more explanation (default: 0.35)
	J17TierGateMode                string  // J17_TIER_GATE_MODE — "trust" (legacy compliance-EMA tier gating) or "comprehension" (T1 promotion keyed on measured comprehension — the axis dense encoding actually risks; J17-TIER-GATE-001). Invalid → "trust" + WARN (default: "trust")
	J17TierComprehensionHigh       float64 // J17_TIER_COMPREHENSION_HIGH — comprehension promotion floor for T1 in comprehension mode; aligned with J17_TIER_INEFFECTIVE_THRESHOLD's 0.6 anchor; live 48h avg 0.732 clears (default: 0.6)
	J17TierComprehensionMinSamples int     // J17_TIER_COMPREHENSION_MIN_SAMPLES — protocol events required before comprehension mode activates; below → legacy trust logic (cold-start; in-memory stats reset on restart) (default: 20)
	J17TrustTTLHours               int     // J17_TRUST_TTL_HOURS — trust entry expiry in hours (default: 168)
	// DASHBOARD-TRUTH-001 Epic 4: significance floor for the trust gauges.
	// A session counts toward mdemg_j17_{min,max,avg}_trust_score and
	// _trust_session_count only if it is within the TTL of its last trust
	// update AND has ≥ this many recorded feedback events. ≤0 disables the
	// feedback floor (recency-only filtering).
	J17TrustMinFeedbackCount int // J17_TRUST_MIN_FEEDBACK_COUNT — min feedback events for gauge significance (default: 5)
	// JIMINY-EFFECTIVENESS-001: trust update rule. "ema" tracks recent
	// effectiveness and RECOVERS (the J17-T1 unblocker); "ratchet" is the legacy
	// monotonic boost/decay that floors permanently.
	J17TrustMode             string  // J17_TRUST_MODE — ema|ratchet (default: ema)
	J17TrustEMAAlpha         float64 // J17_TRUST_EMA_ALPHA — EMA smoothing factor (0,1] (default: 0.1)
	J17BootstrapCodification bool    // J17_BOOTSTRAP_CODIFICATION — codify all constraints on startup (default: true)
	J17BootstrapSpaceID      string  // J17_BOOTSTRAP_SPACE_ID — space to bootstrap codes for (default: "mdemg-dev")

	// J17-4: Protocol Metrics + RSIC Evolution
	J17MetricsEnabled            bool    // J17_METRICS_ENABLED — enable protocol metrics collection (default: inherits J17)
	J17CodificationThreshold     int     // J17_CODIFICATION_THRESHOLD — T2 send count before RSIC proposes codification (default: 30)
	J17ComprehensionMinThreshold float64 // J17_COMPREHENSION_MIN_THRESHOLD — comprehension rate below which code is retired (default: 0.7)
	J17CompressionMinRatio       float64 // J17_COMPRESSION_MIN_RATIO — compression ratio below which RSIC flags regression (default: 2.0)
	J17ReplayFrequencyMax        float64 // J17_REPLAY_FREQUENCY_MAX — replays/hour above which RSIC adjusts buffer (default: 5.0)
	J17NLIComprehensionEnabled   bool    // J17_NLI_COMPREHENSION_ENABLED — use NLI model for comprehension scoring (default: false)
	J17ProtocolDataCollection    bool    // J17_PROTOCOL_DATA_COLLECTION — enable JSONL protocol training data collection (default: false)

	// J17-5: Extensions + System-Wide Encoding + ML Tier Selection
	J17ExtensionsEnabled       bool     // J17_EXTENSIONS_ENABLED — enable agent-negotiated extensions (default: inherits J17)
	J17AllowedExtensions       []string // J17_ALLOWED_EXTENSIONS — comma-separated list of allowed extensions
	J17MLTierPredictionEnabled bool     // J17_ML_TIER_PREDICTION_ENABLED — enable ML-powered tier selection (default: false)
	J17SidecarURL              string   // J17_SIDECAR_URL — neural sidecar URL for shadow ML predictions (default: "")
	J17SidecarTimeoutMs        int      // J17_SIDECAR_TIMEOUT_MS — timeout for sidecar calls in ms (default: 1000, floor: 100)

	// J17-NS: Neural Sidecar Promotion (shadow → causal)
	J17SidecarMode             string  // J17_SIDECAR_MODE — arbitration mode: shadow, compare, canary, active (default: "shadow")
	J17SidecarCanaryPct        int     // J17_SIDECAR_CANARY_PERCENTAGE — % of eligible requests routed to ML in canary mode (default: 100)
	J17SidecarConfidenceFloor  float64 // J17_SIDECAR_CONFIDENCE_FLOOR — min ML confidence to use prediction (default: 0.6)
	J17NLIScoreOfRecord        bool    // J17_NLI_SCORE_OF_RECORD — when true + mode >= canary, NLI becomes comprehension score-of-record (default: false)
	J17PrecedentProtectedCodes string  // J17_PRECEDENT_PROTECTED_CODES — comma-separated constraint codes that NEVER use ML tier (default: "")
	J17PrecedentLogEnabled     bool    // J17_PRECEDENT_LOG_ENABLED — audit log when ML would change a protected constraint's tier (default: true)

	// J17-NS: Sidecar Circuit Breaker
	J17SidecarCBEnabled          bool // J17_SIDECAR_CB_ENABLED — enable circuit breaker for sidecar calls (default: true)
	J17SidecarCBFailureThreshold int  // J17_SIDECAR_CB_FAILURE_THRESHOLD — failures before opening circuit (default: 3)
	J17SidecarCBTimeoutSec       int  // J17_SIDECAR_CB_TIMEOUT_SEC — seconds before half-open retry (default: 15)

	// J17: T1 Comprehension Gate
	J17T1ComprehensionGate float64 // J17_T1_COMPREHENSION_GATE — minimum T1 follow rate to continue using T1 (default: 0.5)

	// NLI Feedback Loop: Per-Tier Effectiveness + Calibration
	J17NLIObservationalEnabled     bool    // J17_NLI_OBSERVATIONAL_ENABLED — NLI scores flow to metrics in all modes (default: true)
	J17TierEffectivenessMinSamples int     // J17_TIER_EFFECTIVENESS_MIN_SAMPLES — min outcomes per tier/code before grading (default: 5)
	J17TierIneffectiveThreshold    float64 // J17_TIER_INEFFECTIVE_THRESHOLD — comprehension below this = ineffective (default: 0.6)
	J17TierDriftDetectionEnabled   bool    // J17_TIER_DRIFT_DETECTION_ENABLED — enable j17_tier_ineffective RSIC pattern (default: true)
	J17NLICalibrationWindowSize    int     // J17_NLI_CALIBRATION_WINDOW_SIZE — ring buffer size for NLI/heuristic comparison (default: 500)
	J17NLICalibrationBiasThreshold float64 // J17_NLI_CALIBRATION_BIAS_THRESHOLD — max acceptable NLI-vs-heuristic mean bias over the like-for-like (ignored-excluded) window (default: 0.15; DASHBOARD-TRUTH-001)
	J17NLICalibrationMinSamples    int     // J17_NLI_CALIBRATION_MIN_SAMPLES — min like-for-like samples before the bias alert can fire; below this the window is "insufficient data" (default: 50, 0 disables the floor; DASHBOARD-TRUTH-001)
	J17CompressionTargetRatio      float64 // J17_COMPRESSION_TARGET_RATIO — compression ratio scored as "excellent" (sub-score 1.0) by the RSIC Protocol dimension; must be > 1.0 (ratio 1.0 = no compression), else falls back to default with a warning (default: 2.0 — recalibrated by DASHBOARD-TRUTH-002 from DASHBOARD-TRUTH-001's 3.0)

	// Plugin system settings (V0006)
	PluginsEnabled  bool   // Feature toggle for plugin system (default: true)
	PluginsDir      string // Path to plugins directory (default: ./plugins)
	PluginSocketDir string // Path to Unix socket directory (default: /tmp/mdemg-plugins)
	MdemgVersion    string // MDEMG version string for handshake
	MdemgCommit     string // MDEMG build commit hash for version comparison

	// Linear integration settings (Phase 4)
	LinearTeamID string // Default team ID for issue creation

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

	// Optimistic retry settings (Phase 47)
	OptimisticRetryEnabled     bool    // OPTIMISTIC_RETRY_ENABLED — enable optimistic locking with retry (default: true)
	OptimisticRetryMaxAttempts int     // OPTIMISTIC_RETRY_MAX_ATTEMPTS — max retry attempts (default: 5)
	OptimisticRetryBaseDelayMs int     // OPTIMISTIC_RETRY_BASE_DELAY_MS — initial delay in ms (default: 10)
	OptimisticRetryMaxDelayMs  int     // OPTIMISTIC_RETRY_MAX_DELAY_MS — max delay in ms (default: 1000)
	OptimisticRetryMultiplier  float64 // OPTIMISTIC_RETRY_MULTIPLIER — exponential backoff multiplier (default: 2.0)

	// Edge staleness settings (Phase 47)
	EdgeStalenessCascadeEnabled   bool // EDGE_STALENESS_CASCADE_ENABLED — enable edge staleness cascade (default: true)
	EdgeStalenessRefreshBatchSize int  // EDGE_STALENESS_REFRESH_BATCH_SIZE — edges per refresh call (default: 100)

	// Capability gap detection settings (Task #23)
	GapLowScoreThreshold   float64 // Queries below this avg score are considered poor (default: 0.5)
	GapMinOccurrences      int     // Min occurrences to create a gap (default: 3)
	GapAnalysisWindowHours int     // Time window for pattern analysis in hours (default: 24)
	GapMetricsWindowSize   int     // Number of queries to keep in history (default: 1000)

	// RSIC (Recursive Self-Improvement Cycle) settings (Phase 60b)
	RSICMicroEnabled       bool    // RSIC_MICRO_ENABLED — enable per-request micro cycles (default: true)
	RSICMesoPeriodHours    int     // RSIC_MESO_PERIOD_HOURS — hours between meso cycles (default: 6)
	RSICMesoPeriodSessions int     // RSIC_MESO_PERIOD_SESSIONS — sessions between meso cycles (default: 10)
	RSICMacroCron          string  // RSIC_MACRO_CRON — cron expression for macro cycles (default: "0 3 * * *")
	RSICMaxNodePrunePct    float64 // RSIC_MAX_NODE_PRUNE_PCT — max % of nodes a single action can prune (default: 0.05)
	RSICMaxEdgePrunePct    float64 // RSIC_MAX_EDGE_PRUNE_PCT — max % of edges a single action can prune (default: 0.10)
	RSICRollbackWindow     int     // RSIC_ROLLBACK_WINDOW — seconds to keep rollback snapshots (default: 3600)
	RSICWatchdogEnabled    bool    // RSIC_WATCHDOG_ENABLED — enable decay watchdog (default: true)
	RSICWatchdogCheckSec   int     // RSIC_WATCHDOG_CHECK_SEC — seconds between watchdog checks (default: 300)
	RSICWatchdogDecayRate  float64 // RSIC_WATCHDOG_DECAY_RATE — decay score increase per hour without cycle (default: 0.1)
	// JIMINY-SIGNAL-001: window for the honest guidance follow-rate computed from
	// constraint_outcomes TSDB (the dashboard panels' source), replacing the
	// inflated Neo4j dedup gauge. 7d default is a stable window matching the panels.
	RSICGuidanceEffectivenessWindowHours int      // RSIC_GUIDANCE_EFFECTIVENESS_WINDOW_HOURS (default: 168; range [1,720])
	RSICNudgeThreshold                   float64  // RSIC_NUDGE_THRESHOLD — decay score for nudge-level escalation (default: 0.3)
	RSICWarnThreshold                    float64  // RSIC_WARN_THRESHOLD — decay score for warn-level escalation (default: 0.6)
	RSICForceThreshold                   float64  // RSIC_FORCE_THRESHOLD — decay score for force-trigger escalation (default: 0.9)
	RSICCalibrationDays                  int      // RSIC_CALIBRATION_DAYS — days of history for calibration (default: 30)
	RSICMaxHistoryEntries                int      // RSIC_MAX_HISTORY_ENTRIES — max calibration history entries per type (default: 1000)
	RSICMinConfidence                    float64  // RSIC_MIN_CONFIDENCE — minimum confidence to execute an action (default: 0.3)
	RSICTriggerCooldownSec               int      // RSIC_TRIGGER_COOLDOWN_SEC — cooldown between triggers from same source (default: 300)
	RSICTriggerDedupeSec                 int      // RSIC_TRIGGER_DEDUPE_SEC — dedupe window for identical trigger IDs (default: 600)
	RSICWatchdogSpaceID                  string   // RSIC_WATCHDOG_SPACE_ID — space monitored by watchdog (default: "mdemg-dev")
	RSICPersistenceEnabled               bool     // RSIC_PERSISTENCE_ENABLED — enable write-behind persistence (default: true)
	RSICProtectedSpaces                  []string // RSIC_PROTECTED_SPACES — comma-separated spaces blocked from destructive RSIC actions (default: "" = none)
	RSICMacroCronSpace                   string   // RSIC_MACRO_CRON_SPACE — space targeted by macro cron cycles (default: "mdemg-dev")
	RSICMinActionConfidence              float64  // RSIC_MIN_ACTION_CONFIDENCE — suppress planner actions below this calibration success rate (default: 0.2)

	// RSIC-SK1: Guidance self-calibration
	RSICGuidanceCalibrationEnabled bool    // RSIC_GUIDANCE_CALIBRATION_ENABLED — master switch for RSIC-SK1 actions (default: true)
	RSICGuidanceMinSurfaces        int     // RSIC_GUIDANCE_MIN_SURFACES — min surfaces before effectiveness is meaningful (default: 3)
	RSICGuidanceBoostThreshold     float64 // RSIC_GUIDANCE_BOOST_THRESHOLD — effectiveness rate above which confidence is boosted (default: 0.7)
	RSICGuidanceDecayThreshold     float64 // RSIC_GUIDANCE_DECAY_THRESHOLD — effectiveness rate below which confidence is decayed (default: 0.1)
	RSICGuidanceDecayMinSurfaces   int     // RSIC_GUIDANCE_DECAY_MIN_SURFACES — min surfaces before decay applies (default: 5)

	SpacePruneIntervalHours    int  // SPACE_PRUNE_INTERVAL_HOURS — auto-prune interval in hours (default: 24, 0=disabled)
	ContextCoolerEnabled       bool // CONTEXT_COOLER_ENABLED — enable background context cooler processing (default: false)
	WeeklyGapInterviewsEnabled bool // WEEKLY_GAP_INTERVIEWS_ENABLED — enable background weekly gap interviews (default: false)

	// Phase AR-3: LLM-powered RSIC reflection
	RSICLLMReflectEnabled   bool   // RSIC_LLM_REFLECT_ENABLED — enable LLM-powered reflection (default: true)
	RSICLLMReflectProvider  string // RSIC_LLM_REFLECT_PROVIDER — LLM provider (openai/ollama, default: from EMERGENCE_PROVIDER)
	RSICLLMReflectModel     string // RSIC_LLM_REFLECT_MODEL — model for reflection (default: from EMERGENCE_MODEL)
	RSICLLMReflectCompress  bool   // RSIC_LLM_REFLECT_COMPRESS — compress RSIC reflection prompts (default: true)
	RSICLLMReflectTimeoutMs int    // RSIC_LLM_REFLECT_TIMEOUT_MS — timeout for reflection LLM call in ms (default: 15000, min 5000)
	// APE-PROMPT-BUDGET-001: bound the ape.reflect prompt so output never starves
	// the llama-server per-slot KV budget (live-measured 7489-tok prompts left only
	// ~700 tok for output → ~87% truncated invalid JSON on the largest training target).
	RSICLLMReflectPromptBudgetTokens int  // RSIC_LLM_REFLECT_PROMPT_BUDGET_TOKENS — max assembled user-prompt tokens; 0 disables the guard (default: 3500, range 0 or [1000,7000])
	RSICLLMReflectHistoryCycles      int  // RSIC_LLM_REFLECT_HISTORY_CYCLES — recent cycles included in the prompt (default: 3, range [0,10])
	RSICLLMReflectIncludeDatasets    bool // RSIC_LLM_REFLECT_INCLUDE_DATASETS — include verbose TSDB dataset fields in the prompt (default: false)

	// Phase 11.6.x: per-space concurrency cap on RSIC LLM-bound stages (Reflect)
	// to prevent unbounded fan-out from saturating the local mlx server and
	// triggering Metal OOM. CycleOrchestrator acquires a slot before
	// reflector.Reflect() and releases on completion.
	RSICLLMConcurrencyLimit int // RSIC_LLM_CONCURRENCY_LIMIT — max in-flight RSIC LLM calls (default: 2, min 1, max 8)

	// Phase 12 Epic 6 (Workstream C #1): production wiring for the
	// conflicting-guidance tracker. The recorder + V0015 hypertable
	// shipped in Phase 11.6.x; this knob gates whether the three
	// production decision sites (jiminy guidance, ape RSIC reflect,
	// consulting suggestions) actually call ConflictTracker.Track().
	// Default true — emergency-disable lever for if hooks misbehave.
	ConflictTrackerEnabled bool // CONFLICT_TRACKER_ENABLED — enable production divergence-detection hooks (default: true)

	// Phase 11.6.3 — MLX Watchdog. Background prober + llmclient fast-fail
	// gate that detect mlx_lm.server outages within seconds and short-circuit
	// the 6-attempt × ~30 s retry loop that produced the retry-storm pattern
	// observed in Phase 12 (1642% CPU when mlx died under load). All knobs
	// default to safe-off so existing deployments are unaffected until the
	// operator opts in via .env.
	// Phase 13.6 — env-var primaries are LLM_*; legacy MLX_* names accepted
	// as aliases with a deprecation log. Struct field names retained as MLX*
	// to minimize blast radius (internal symbols only; rename in a future
	// pure-rename sprint if desired).
	MLXWatchdogEnabled  bool // LLM_WATCHDOG_ENABLED (alias: MLX_WATCHDOG_ENABLED) — start the watchdog goroutine + metrics (default: true; flipped from false in hotfix 11.6.3.1 per always-on policy)
	MLXProbeIntervalSec int  // LLM_PROBE_INTERVAL_SEC (alias: MLX_PROBE_INTERVAL_SEC) — seconds between probe ticks (default: 5, min 1)
	MLXProbeTimeoutSec  int  // LLM_PROBE_TIMEOUT_SEC (alias: MLX_PROBE_TIMEOUT_SEC) — per-probe HTTP timeout in seconds; must be < LLM_PROBE_INTERVAL_SEC (default: 2, min 1)
	MLXFailFastEnabled  bool // LLM_FAIL_FAST_ENABLED (alias: MLX_FAIL_FAST_ENABLED) — let llmclient short-circuit when probe says StateDown (default: true; only effective when MLXWatchdogEnabled is true)

	// Phase 13 — Note 04 Column-Voting Retrieval. Replaces the linear-
	// combination ranker at scoring.go:797 with a Reciprocal Rank Fusion
	// ensemble across 4 columns (Embedding + BM25 + GraphProximity +
	// Structural; Temporal + RoleScoped deferred per Epic 0 data audit).
	// Default false until the UVTS A/B verdict passes — flag-flip in same
	// commit if the merge gate clears (B mean ≥ A mean AND no per-question
	// regression > 10%).
	RetrievalColumnVotingEnabled     bool    // RETRIEVAL_COLUMN_VOTING_ENABLED — route to RRF aggregator instead of linear scorer (default: true after Phase 13.1 embedding-heavy preset passed full 120q A/B with mean +0.023, 30 improvements, 2 boundary regressions)
	RetrievalGraphTypedEdgesEnabled  bool    // RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED — RRF graph column spreads activation through typed semantic edges (ANALOGOUS_TO/BRIDGES/etc.) via SpreadingActivationWithAttention instead of the CO_ACTIVATED_WITH-only basic spreading (RETRIEVAL-TYPED-EDGES-001/002; default: true — the clean full-120q UVTS A/B passed on a quiet system: +0.001 mean / +0.009 correct-file, 0 regressions, no latency cost, once the semantic edges were grown by the dynamic_edges vector-index rewrite)
	RetrievalRRFK                    int     // RETRIEVAL_RRF_K — RRF constant `score = w / (k + rank)` (default: 60 per Cormack et al.)
	RetrievalColumnTimeoutFrac       float64 // RETRIEVAL_COLUMN_TIMEOUT_FRACTION — fraction of parent ctx remaining each column may consume (default: 0.8)
	RetrievalStructuralHops          int     // RETRIEVAL_STRUCTURAL_HOPS — max hops walked by the structural column (default: 2; clamp to 1–9)
	RetrievalColumnEmbeddingEnabled  bool    // RETRIEVAL_COLUMN_EMBEDDING_ENABLED — per-column suppression knob (default: true)
	RetrievalColumnBM25Enabled       bool    // RETRIEVAL_COLUMN_BM25_ENABLED — per-column suppression knob (default: true)
	RetrievalColumnGraphEnabled      bool    // RETRIEVAL_COLUMN_GRAPH_ENABLED — per-column suppression knob (default: true)
	RetrievalColumnStructuralEnabled bool    // RETRIEVAL_COLUMN_STRUCTURAL_ENABLED — per-column suppression knob (default: true)

	// Phase 13.1 — per-column RRF weights. Phase 13 shipped equal weights (1.0
	// each via nil ColumnWeights map). Forensic diagnosis (phase_13_1_forensic_diagnosis.md)
	// proved equal weights cause q 69 + q hard_sym_4 to regress to 0.000 because
	// Graph+Structural collectively over-vote on structurally-connected code,
	// crowding out Embedding+BM25's better matches for precise-symbol queries.
	// 1.0 default per column = same as Phase 13 nil-map → equal weights → no
	// behavior change unless operator opts in.
	RetrievalColumnWeightEmbedding  float64 // RETRIEVAL_COLUMN_WEIGHT_EMBEDDING — RRF weight on the embedding column (default: 0.50, Phase 13.1 embedding-heavy preset)
	RetrievalColumnWeightBM25       float64 // RETRIEVAL_COLUMN_WEIGHT_BM25 — RRF weight on the BM25 column (default: 0.20, Phase 13.1 embedding-heavy preset)
	RetrievalColumnWeightGraph      float64 // RETRIEVAL_COLUMN_WEIGHT_GRAPH — RRF weight on the graph (spreading-activation) column (default: 0.15, Phase 13.1 embedding-heavy preset)
	RetrievalColumnWeightStructural float64 // RETRIEVAL_COLUMN_WEIGHT_STRUCTURAL — RRF weight on the structural Cypher-walk column (default: 0.15, Phase 13.1 embedding-heavy preset)

	// Phase 13 Epic 5 — Downstream consumers. Both flags default false:
	// the wiring exists so Phase 14 doesn't have to touch this code,
	// but the actual consumption (prompt-template inject in rerank,
	// dimension input in DH-005) ships in Phase 14 alongside the sparse
	// fingerprint + percentile gate features that need consensus_strength.

	// Phase 13 Epic 6 — retrieval audit hypertable (V0017). When enabled,
	// service.Retrieve writes one row per call to `retrieval_audit` so
	// Phase 14 has a historical baseline of consensus_strength + per-column
	// latency available the day Notes 05+06 ship. Default false to avoid
	// unbounded TSDB growth — operators opt in via .env for observation
	// windows.
	RetrievalAuditEnabled bool // RETRIEVAL_AUDIT_ENABLED — write retrieval_audit rows on every retrieve call (default: true since TSDB-CONSUME-001 — feeds the scorer-drift tripwires)

	// Phase 14 Epic 1 — Note 06 sparse activation gate. After RRF aggregation
	// (consensus_strength + ranked candidates), the gate cuts the candidate
	// set to those whose score crosses the per-call activation percentile.
	// Defaults derived from phase_14_score_distribution_analysis.md (Epic 0):
	// observed retrieval_scores p98/p50 = 4-5x, K-cap dominantly 20-50, so
	// p95 within-call paired with MIN_ACTIVE=3 floor and MAX_ACTIVE=20 cap
	// matches production K shape. Phase 14.1.1 (2026-05-04) flipped the
	// gate default-on with the hybrid config (MIN=15 global +
	// data_flow_integration MIN=20 override) after the canonical 120q
	// A/B passed mean +0.003 / 0 regressions / 10 improvements.
	SparseRetrievalEnabled     bool    // SPARSE_RETRIEVAL_ENABLED — apply percentile gate post-aggregation pre-rerank (default: true since Phase 14.1.1)
	SparseActivationPercentile float64 // SPARSE_ACTIVATION_PERCENTILE — within-call score percentile cutoff (default: 0.95; range 0.5-0.999)
	SparseMinActive            int     // SPARSE_MIN_ACTIVE — floor on active set size; gate cannot drop below this (default: 3)
	SparseMaxActive            int     // SPARSE_MAX_ACTIVE — cap on active set size; gate cannot exceed this (default: 20; matches observed top-K cap)

	// Phase 14.1 Epic 1 — Per-category sparse-gate overrides. Phase 14 120q
	// full A/B revealed boundary regressions concentrated in specific
	// categories (architecture_structure 3/20, data_flow_integration 2/20).
	// This map lets operators set per-category MIN_ACTIVE / MAX_ACTIVE /
	// percentile that override the global defaults above when the request
	// carries a `Category` field. Pointer fields mean "not set; fall back to
	// global default". Empty map (default) preserves Phase 14 behavior.
	//
	// JSON env shape:
	//   SPARSE_GATE_CATEGORY_OVERRIDES='{"architecture_structure":{"min_active":20},"data_flow_integration":{"min_active":15}}'
	SparseGateCategoryOverrides map[string]SparseGateOverride

	// Phase 14.2 Epic 1+2 — Note 05 sparse fingerprints (context catalog +
	// 5th RRF column). Two-phase fingerprint design: observe-time uses
	// observation-local features (path, role_type, layer, top-N tags);
	// post-hoc refresh in CycleOrchestrator macro-cycle adds symbol bits
	// from CO_ACTIVATED_WITH edges. Adaptive Builder allocates bits per
	// space by feature density (Phase 14 + 14.2 Epic 0 forensic confirmed
	// no production space has populated symbol/role).
	//
	// Defaults flag-off until Epic 6 A/B verdict — operator can enable
	// observe-time fingerprint computation alone (no retrieval-side use)
	// to start populating data for a future A/B.
	ContextFingerprintEnabled              bool // CONTEXT_FINGERPRINT_ENABLED — Service.Observe computes + writes context_fingerprint_active on new MemoryNodes (default: false until Epic 6 A/B passes)
	ContextFingerprintBitBudget            int  // CONTEXT_FINGERPRINT_BIT_BUDGET — total bits per fingerprint (default: 256 per Note 05 spec)
	ContextFingerprintRefreshEnabled       bool // CONTEXT_FINGERPRINT_REFRESH_ENABLED — CycleOrchestrator stage 6 runs Builder.BuildForSpace + post-hoc refresh (default: false initially)
	ContextFingerprintRefreshIntervalHours int  // CONTEXT_FINGERPRINT_REFRESH_INTERVAL_HOURS — minimum hours between refresh ticks per space (default: 168 = weekly)
	ContextFingerprintRefreshTimeoutMs     int  // CONTEXT_FINGERPRINT_REFRESH_TIMEOUT_MS — per-cycle time budget for post-hoc refresh batch (default: 60000 = 60s)
	// CONTEXT-LIVE-001 — stage-6 version-skew heal: max stale-fingerprint
	// nodes recomputed per cycle (0 disables; resumable across cycles).
	ContextFingerprintHealMaxPerCycle int // CONTEXT_FINGERPRINT_HEAL_MAX_PER_CYCLE (default: 2000)
	// CONTEXT-LIVE-001 — maps QueryClassifier output types to the UVTS
	// category vocabulary used by SPARSE_GATE_CATEGORY_OVERRIDES and
	// RETRIEVAL_CONTEXT_COLUMN_CATEGORY_WEIGHTS, so per-category protections
	// fire on live traffic (previously only ?category= benchmark calls).
	// Explicit request category always wins. Empty map disables dispatch.
	// service_relationships/business_logic_constraints have no classifier
	// equivalent and stay benchmark-only until the vocabulary grows.
	QueryClassifyCategoryMap map[string]string // QUERY_CLASSIFY_CATEGORY_MAP (JSON; default maps data_flow/architecture/relationship)
	// CONTEXT-LIVE-001 — derive the query context fingerprint server-side
	// by DEFAULT (previously only on ?context=auto, which no live caller
	// passed — the 5th RRF column was dormant on all real traffic).
	// Per-call opt-out: ?context=off|false|0. Requires healed node
	// fingerprints (the version guard zeroes stale ones regardless).
	ContextQueryAutoDefault bool // CONTEXT_QUERY_AUTO_DEFAULT (default: true)
	// CONTEXT-LIVE-001 — Phase-B refine: max current-version observations
	// run through RefineWithCoactivations per cycle (0 disables).
	ContextFingerprintRefineMaxPerCycle int     // CONTEXT_FINGERPRINT_REFINE_MAX_PER_CYCLE (default: 200)
	ContextCatalogTopNPaths             int     // CONTEXT_CATALOG_TOP_N_PATHS — cap on path bits in catalog (default: 192)
	ContextCatalogTopNTags              int     // CONTEXT_CATALOG_TOP_N_TAGS — cap on tag bits in catalog (default: 32)
	ContextCatalogFloorBitsPerKind      int     // CONTEXT_CATALOG_FLOOR_BITS_PER_KIND — minimum bits allocated to any kind with ≥10 distinct values (default: 16)
	ContextCatalogRoleTypeLayerBits     int     // CONTEXT_CATALOG_ROLE_TYPE_LAYER_BITS — reserved bits for top-N (role_type × layer) tuples (default: 32)
	RetrievalContextColumnEnabled       bool    // RETRIEVAL_CONTEXT_COLUMN_ENABLED — 5th RRF column gates on this (default: true since Phase 14.2.3, 2026-05-06; per-category zero-weight overrides for service_relationships / business_logic_constraints / relationship live in RetrievalContextColumnCategoryWeights)
	RetrievalContextColumnWeight        float64 // RETRIEVAL_CONTEXT_COLUMN_WEIGHT — RRF weight on the context column (default: 0.10 per Note 05 spec)
	RetrievalContextStrictThreshold     float64 // RETRIEVAL_CTX_STRICT_THRESHOLD — Jaccard threshold for ?strict_context=true mode (default: 0.25 per Note 05 spec)
	ContextFingerprintQueryTopK         int     // CONTEXT_FINGERPRINT_QUERY_TOPK — Phase 14.2.1: top-K catalog refs (by cosine sim to query embedding) included in derived query fingerprint (default: 8)

	// Phase 14.2.3 — Per-category override of the context-column RRF weight.
	// Default seed reflects the Phase 14.2.2 120q forensic: zero-weight on
	// the 3 categories that consistently regressed (service_relationships
	// -0.043, business_logic_constraints -0.023, relationship -0.017),
	// keep default RetrievalContextColumnWeight on the others.
	// Operator may extend or replace via RETRIEVAL_CONTEXT_COLUMN_CATEGORY_WEIGHTS
	// JSON env (env value REPLACES the default seed entirely; merge isn't supported).
	RetrievalContextColumnCategoryWeights map[string]float64 // RETRIEVAL_CONTEXT_COLUMN_CATEGORY_WEIGHTS — JSON {"<category>":<weight>}; per-category override of the global RetrievalContextColumnWeight (default: 3-category zero-weight seed)

	// Phase AR-3: LLM-powered constraint classification
	ConsultingLLMConstraintsEnabled  bool   // CONSULTING_LLM_CONSTRAINTS_ENABLED — enable LLM constraint classification (default: false)
	ConsultingLLMConstraintsProvider string // CONSULTING_LLM_CONSTRAINTS_PROVIDER — LLM provider (default: from EMERGENCE_PROVIDER)
	ConsultingLLMConstraintsModel    string // CONSULTING_LLM_CONSTRAINTS_MODEL — model for classification (default: from EMERGENCE_MODEL)
	ConsultingClassifyTimeoutMs      int    // CONSULTING_CLASSIFY_TIMEOUT_MS — timeout for constraint classification LLM call in ms (default: 30000, min 5000)
	// GUIDANCE-SYNTH-001 — bounded concurrency for the per-node LLM constraint
	// classifier in findApplicableConstraints. Serial classification (~1.5s/node ×
	// ~10 nodes) starved guidance synthesis of its time budget. Default 4 matches
	// llama-server --parallel 4; floor 1 = serial (rollback). No-hardcoding rule.
	ConsultingClassifyConcurrency int // CONSULTING_CLASSIFY_CONCURRENCY (default: 4, floor 1)
	// RRF-SCALE-001 — score gates for the consulting suggestion/constraint path.
	// These were hardcoded (0.55/0.6/0.65/0.7) and calibrated for the legacy
	// linear scorer; Phase 13.1 RRF default-on dropped the score scale (strong
	// matches now top out ~0.49-0.58), so the legacy gates rejected everything
	// and the Jiminy guidance loop went dormant. Defaults below are RRF-calibrated.
	ConsultingConstraintScoreFloor      float64 // CONSULTING_CONSTRAINT_SCORE_FLOOR — min retrieval score for a result to become a constraint (default: 0.45)
	ConsultingAuthorityScoreFloor       float64 // CONSULTING_AUTHORITY_SCORE_FLOOR — min retrieval score for high-authority constraint tier / confidence boost (default: 0.50)
	ConsultingConflictScoreFloor        float64 // CONSULTING_CONFLICT_SCORE_FLOOR — min retrieval score for conflict/contradiction detection (default: 0.50)
	ConsultingSuggestMinConfidence      float64 // CONSULTING_SUGGEST_MIN_CONFIDENCE — RRF-SCALE-002: default min-confidence floor for /v1/memory/suggest (default: 0.2, live-calibrated; was hardcoded 0.5 against a scale topping out ~0.58)
	MCPReflectScoreHigh                 float64 // MCP_REFLECT_SCORE_HIGH — RRF-SCALE-002: memory_reflect high-relevance tier floor (default: 0.45; was hardcoded 0.7 — unreachable on the RRF scale)
	MCPReflectScoreMedium               float64 // MCP_REFLECT_SCORE_MEDIUM — RRF-SCALE-002: memory_reflect medium tier floor (default: 0.25; was hardcoded 0.4)
	GuardrailConstraintSimFloor         float64 // GUARDRAIL_CONSTRAINT_SIM_FLOOR — RRF-SCALE-002: cosine floor in guardrail constraint retrieval Cypher (default: 0.3; was hardcoded)
	RetrievalConfidenceSigmoidMidpoint  float64 // RETRIEVAL_CONFIDENCE_SIGMOID_MIDPOINT — sigmoid midpoint for score→confidence normalization (default: 0.45, was 1.5 legacy)
	RetrievalConfidenceSigmoidSteepness float64 // RETRIEVAL_CONFIDENCE_SIGMOID_STEEPNESS — sigmoid steepness for score→confidence normalization (default: 8.0, was 1.5 legacy)

	// Phase AR-3: LLM-powered query classification

	// Phase 38: UNTS Hash Verification
	UNTSEnabled  bool   // UNTS_ENABLED — enable hash verification REST API (default: false)
	UNTSBasePath string // UNTS_BASE_PATH — repository root for file hashing (default: ".")

	// Context Cooler tuning (Phase 45.5)
	CoolerReinforcementWindowHours  int     // COOLER_REINFORCEMENT_WINDOW_HOURS — reinforcement window (default: 2)
	CoolerStabilityIncreasePerReinf float64 // COOLER_STABILITY_INCREASE_PER_REINFORCEMENT (default: 0.15)
	CoolerStabilityDecayRate        float64 // COOLER_STABILITY_DECAY_RATE — daily decay for unreinforced nodes (default: 0.1)
	CoolerTombstoneThreshold        float64 // COOLER_TOMBSTONE_THRESHOLD — stability below which nodes are tombstoned (default: 0.05)
	CoolerTombstoneMaxPerRun        int     // COOLER_TOMBSTONE_MAX_PER_RUN — max nodes tombstoned per graduation run; 0 = unlimited (default: 500)
	CoolerGraduationThreshold       float64 // COOLER_GRADUATION_THRESHOLD — stability for graduation (default: 0.8)

	// Constraint Module (Phase 45.5)
	ConstraintDetectionEnabled bool    // CONSTRAINT_DETECTION_ENABLED — enable constraint detection in observations (default: true)
	ConstraintMinConfidence    float64 // CONSTRAINT_MIN_CONFIDENCE — minimum confidence to tag as constraint (default: 0.6)
	ConstraintProtectFromDecay bool    // CONSTRAINT_PROTECT_FROM_DECAY — protect constraint-tagged obs from tombstoning (default: true)

	// JIMINY-CORPUS-001 Epic 1: ConvObs→constraint promotion gate.
	// Guards the ONLY site that mints role_type='constraint' nodes from
	// conversation observations (hidden.CreateConstraintNodes, run in
	// consolidation). Rejects transient/status content (fabricated
	// "Build/test succeeded" observations, bash errors, PR/sprint status
	// notes, doc/template dumps) so junk never becomes a constraint node
	// that Lever C would then surface as guidance.
	ConstraintPromotionGateEnabled    bool     // CONSTRAINT_PROMOTION_GATE_ENABLED — enable the promotion gate (default: true)
	ConstraintPromotionDenyObsTypes   []string // CONSTRAINT_PROMOTION_DENY_OBS_TYPES — comma-separated obs_types that are NEVER promoted (transient provenance; default: "progress,error,task,context")
	ConstraintPromotionRejectPatterns []string // CONSTRAINT_PROMOTION_REJECT_PATTERNS — JSON array of Go regexes; observation content matching ANY is never promoted (default: built-in junk-class set; "[]" disables the pattern layer)

	// JIMINY-CORRECTION-PRODUCER-001 — L0 obs → correction node promotion gate.
	// The predicate itself already filters by obs_type='correction' (unlike the
	// constraint side where tag-based promotion needs a provenance deny-set), so
	// this gate is a defensive backstop against pathological content shapes only.
	CorrectionPromotionEnabled        bool     // CORRECTION_PROMOTION_ENABLED — enable the promotion gate (default: true)
	CorrectionPromotionMinContentLen  int      // CORRECTION_PROMOTION_MIN_CONTENT_LEN — reject obs shorter than this (default: 20)
	CorrectionPromotionRejectPatterns []string // CORRECTION_PROMOTION_REJECT_PATTERNS — JSON array of Go regexes; observation content matching ANY is never promoted (default: same junk-class set as the constraint gate)
	// COOLER-001 — decay-rate multiplier for CO_ACTIVATED_WITH edges incident
	// to a GRADUATED (volatile=false) node: this is what "graduated" means to
	// retrieval — stable memory's associations resist time decay. 1.0 = no
	// protection, 0 = no decay, 0.5 = half rate (default).
	GraduatedDecayProtectionFactor float64 // GRADUATED_DECAY_PROTECTION_FACTOR

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
	BackupEnabled                bool   // BACKUP_ENABLED — enable backup module (default: true)
	BackupStorageDir             string // BACKUP_STORAGE_DIR — directory for backup artifacts (default: ".mdemg/backups")
	BackupFullCmd                string // BACKUP_FULL_CMD — command for full backups (default: "docker")
	BackupNeo4jContainer         string // BACKUP_NEO4J_CONTAINER — Docker container name (default: "mdemg-neo4j")
	BackupFullIntervalHours      int    // BACKUP_FULL_INTERVAL_HOURS — hours between full backups (default: 168)
	BackupPartialIntervalHours   int    // BACKUP_PARTIAL_INTERVAL_HOURS — hours between partial backups (default: 24)
	BackupRetentionFullCount     int    // BACKUP_RETENTION_FULL_COUNT — keep last N full backups (default: 4)
	BackupRetentionPartialCount  int    // BACKUP_RETENTION_PARTIAL_COUNT — keep last N partial backups (default: 14)
	BackupRetentionMaxAgeDays    int    // BACKUP_RETENTION_MAX_AGE_DAYS — delete backups older than N days (default: 90)
	BackupRetentionMaxStorageGB  int    // BACKUP_RETENTION_MAX_STORAGE_GB — storage quota in GB (default: 50)
	BackupRetentionRunAfter      bool   // BACKUP_RETENTION_RUN_AFTER_BACKUP — run retention after each backup (default: true)
	BackupSnapshotWaitTimeoutSec int    // BACKUP_SNAPSHOT_WAIT_TIMEOUT_SEC — max wait for triggered backup jobs (pre-restore snapshot, scheduled-run reporting) (default: 3600)
	BackupJobStalenessHours      int    // BACKUP_JOB_STALENESS_HOURS — neo4j-backup staleness alert window; 0 = partial interval × 2 (default: 0)
	BackupInitialDelayMin        int    // BACKUP_INITIAL_DELAY_MIN — minutes after start before an initial partial backup; 0 disables (default: 5)

	// TimescaleDB Backup & Restore
	TSDBBackupEnabled             bool   // TSDB_BACKUP_ENABLED — enable TSDB backup module (default: false)
	TSDBBackupStorageDir          string // TSDB_BACKUP_STORAGE_DIR — directory for TSDB backup artifacts (default: ".mdemg/backups/tsdb")
	TSDBBackupComposeFile         string // TSDB_BACKUP_COMPOSE_FILE — docker compose file path (default: auto-detect)
	TSDBBackupServiceName         string // TSDB_BACKUP_SERVICE — compose service name (default: "timescaledb")
	TSDBBackupIntervalHours       int    // TSDB_BACKUP_INTERVAL_HOURS — hours between backups (default: 24)
	TSDBBackupInitialDelayMin     int    // TSDB_BACKUP_INITIAL_DELAY_MIN — minutes after start before an initial backup; 0 disables (default: 10)
	TSDBBackupRetentionCount      int    // TSDB_BACKUP_RETENTION_COUNT — keep last N backups (default: 14)
	TSDBBackupRetentionMaxAgeDays int    // TSDB_BACKUP_RETENTION_MAX_AGE_DAYS — delete backups older than N days (default: 90)

	// Phase 75: Relationship Extraction
	RelExtractImports     bool // REL_EXTRACT_IMPORTS — extract import relationships (default: true)
	RelExtractInheritance bool // REL_EXTRACT_INHERITANCE — extract inheritance relationships (default: true)
	RelExtractCalls       bool // REL_EXTRACT_CALLS — extract function call relationships (default: true)
	RelCrossFileResolve   bool // REL_CROSS_FILE_RESOLVE — enable cross-file symbol resolution (default: true)
	RelMaxCallsPerFunc    int  // REL_MAX_CALLS_PER_FUNCTION — max calls extracted per function (default: 50)
	RelBatchSize          int  // REL_BATCH_SIZE — batch size for relationship insertion (default: 500)
	RelResolutionTimeout  int  // REL_RESOLUTION_TIMEOUT_SEC — timeout for symbol resolution in seconds (default: 60)

	// Phase 75B: Topology Hardening
	DynamicEdgesEnabled bool // DYNAMIC_EDGES_ENABLED — enable dynamic edge creation during retrieval (default: true)
	// DynamicEdgesMaxNodes (DYNAMIC_EDGES_MAX_NODES) removed by RETRIEVAL-TYPED-EDGES-002:
	// the O(n²) cross-join circuit-breaker it guarded is gone (CreateDynamicEdges
	// now uses a per-node vector-index query, O(n·logn)).
	DynamicEdgeDegreeCap     int     // DYNAMIC_EDGE_DEGREE_CAP — max dynamic edges per node (default: 10)
	DynamicEdgeMinConfidence float64 // DYNAMIC_EDGE_MIN_CONFIDENCE — minimum confidence for dynamic edges (default: 0.5)
	// Vector-index dynamic-edge creation (RETRIEVAL-TYPED-EDGES-002): the O(n²)
	// cross-join is replaced by per-node top-K via the memNodeEmbedding vector
	// index (O(n·logn)). MinLayer default lowered 3→1 because a fresh corpus has
	// almost no L3+ concepts (lnl-demo-whk: 4) — semantic edges must connect the
	// abundant L1/L2 concept layers for the population to grow.
	DynamicEdgeMinLayer     int     // DYNAMIC_EDGE_MIN_LAYER — minimum layer for dynamic semantic-edge endpoints (default: 1; L0 observations excluded)
	DynamicEdgeTopK         int     // DYNAMIC_EDGE_TOPK — per-node nearest-neighbor edges to consider (default: 10)
	DynamicEdgeSimThreshold float64 // DYNAMIC_EDGE_SIM_THRESHOLD — minimum cosine similarity for a dynamic edge (default: 0.30)
	DynamicEdgeOversample   int     // DYNAMIC_EDGE_OVERSAMPLE — vector-index fetch multiplier (fetch TopK×Oversample then layer/space/degree-filter; accounts for L0 crowding the global top-K) (default: 8)
	L5EmergentEnabled       bool    // L5_EMERGENT_ENABLED — enable Layer 5 emergent concept nodes (default: true)
	L5BridgeEvidenceMin     int     // L5_BRIDGE_EVIDENCE_MIN — minimum bridge evidence for L5 promotion (default: 1)
	L5SourceMinLayer        int     // L5_SOURCE_MIN_LAYER — minimum layer for L5/dynamic edge source nodes (default: 3)
	SymbolActivationEnabled bool    // SYMBOL_ACTIVATION_ENABLED — enable symbol-aware activation boost (default: true)

	// Deterministic consolidation trigger (Phase 45.5)
	ConsolidateOnWatchdogEnabled bool // CONSOLIDATE_ON_WATCHDOG_ENABLED — trigger consolidation alongside RSIC force (default: true)

	// Data transmission optimization settings (Phase 10.3)
	CompressionEnabled bool // Enable gzip compression for responses (default: true)
	CompressionMinSize int  // Minimum response size in bytes to compress (default: 1024)
	PaginationMaxLimit int  // Maximum items per page (default: 500)
	PaginationDefLimit int  // Default items per page (default: 50)

	// Neo4j connection pool settings (Phase 10.4)
	Neo4jMaxPoolSize        int // Maximum connections in pool (default: 100)
	Neo4jAcquireTimeoutSec  int // Connection acquire timeout in seconds (default: 60)
	Neo4jMaxConnLifetimeSec int // Maximum connection lifetime in seconds (default: 3600)
	Neo4jConnIdleTimeoutSec int // Connection idle timeout in seconds (default: 0 = disabled)

	// Dynamic port allocation
	PortRangeStart int    // Start of fallback port range (default: derived from ListenAddr port)
	PortRangeEnd   int    // End of fallback port range (default: PortRangeStart + 100)
	PortFilePath   string // Path to write allocated port for client discovery (default: .mdemg.port)

	// Scheduled sync settings (Phase 9.2)
	SyncIntervalMinutes     int               // Check interval for scheduled sync (default: 0 = disabled)
	SyncSpaceIDs            []string          // Comma-separated space IDs to monitor (empty = all)
	SyncStaleThresholdHours int               // Hours before a space is considered stale (default: 24)
	SyncRepoPathMap         map[string]string // space_id -> repo_path mapping for auto-ingest
	APEIngestSyncEnabled    bool              // Enable RSIC-driven ingest staleness detection (default: false)

	// ===== Phase 3: Production Readiness =====

	// Rate limiting settings (Phase 3.1)
	RateLimitEnabled bool    // Feature toggle for rate limiting (default: true)
	RateLimitRPS     float64 // Requests per second limit (default: 100)
	RateLimitBurst   int     // Burst allowance (default: 200)
	RateLimitByIP    bool    // Per-IP rate limiting vs global (default: true)

	// Circuit breaker settings (Phase 3.1)
	CircuitBreakerEnabled    bool // Feature toggle for circuit breaking (default: true)
	CircuitBreakerThreshold  int  // Failures before opening circuit (default: 5)
	CircuitBreakerTimeoutSec int  // Seconds before half-open (default: 30)

	// Authentication settings (Phase 3.2)
	AuthEnabled       bool     // Feature toggle for authentication (default: false for dev)
	AuthMode          string   // Auth mode: "none", "apikey", "bearer" (default: "none")
	AuthAPIKeys       []string // Comma-separated valid API keys
	AuthJWTSecret     string   // JWT secret for bearer mode
	AuthJWTIssuer     string   // Expected JWT issuer
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

	// Metrics settings (Phase 3.3)
	MetricsEnabled   bool   // Feature toggle for metrics collection (default: true)
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
	CMSResumeMaxObs         int     // CMS_RESUME_MAX_OBS — default max observations on resume (default: 20)
	CMSRecallTopK           int     // CMS_RECALL_TOP_K — default top-K for recall queries (default: 10)
	CMSSummaryMaxChars      int     // CMS_SUMMARY_MAX_CHARS — max character length for generated summaries (default: 200)
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

	// Typed semantic-edge attention weights (RETRIEVAL-TYPED-EDGES-001 — were hardcoded in activation.go).
	EdgeAttentionAnalogousTo   float64 // EDGE_ATTENTION_ANALOGOUS_TO — attention weight for ANALOGOUS_TO edges (default: 0.55)
	EdgeAttentionBridges       float64 // EDGE_ATTENTION_BRIDGES — attention weight for BRIDGES edges (default: 0.60)
	EdgeAttentionComposesWith  float64 // EDGE_ATTENTION_COMPOSES_WITH — attention weight for COMPOSES_WITH edges (default: 0.50)
	EdgeAttentionContrastsWith float64 // EDGE_ATTENTION_CONTRASTS_WITH — attention weight for CONTRASTS_WITH edges (default: 0.40)
	EdgeAttentionInfluences    float64 // EDGE_ATTENTION_INFLUENCES — attention weight for INFLUENCES edges (default: 0.45)
	EdgeAttentionDefinesSymbol float64 // EDGE_ATTENTION_DEFINES_SYMBOL — attention weight for DEFINES_SYMBOL edges (default: 0.70)
	EdgeAttentionThemeOf       float64 // EDGE_ATTENTION_THEME_OF — attention weight for THEME_OF edges (default: 0.65)

	// ===== ANN Optimization: Cluster Summary =====
	ClusterSummaryEnabled   bool   // CLUSTER_SUMMARY_ENABLED — enable LLM cluster summarization for L1-L4 (default: false)
	ClusterSummaryProvider  string // CLUSTER_SUMMARY_PROVIDER — LLM provider for summaries (openai/ollama, default: cascade)
	ClusterSummaryModel     string // CLUSTER_SUMMARY_MODEL — model for summaries (default: cascade)
	ClusterSummaryMaxTokens int    // CLUSTER_SUMMARY_MAX_TOKENS — max tokens per summary (default: 100)
	ClusterSummaryTimeoutMs int    // CLUSTER_SUMMARY_TIMEOUT_MS — timeout for summary call in ms (default: 5000)
	ClusterSummaryBatchSize int    // CLUSTER_SUMMARY_BATCH_SIZE — max nodes per consolidation run (default: 50)

	// ===== ANN Optimization: Negative Feedback =====
	LearningNegativeWeight        float64 // LEARNING_NEGATIVE_WEIGHT — weight reduction per negative feedback (default: 0.15)
	LearningNegativeMaxPerRequest int     // LEARNING_NEGATIVE_MAX_PER_REQUEST — max rejected nodes per request (default: 20)

	// ===== ANN Optimization: Frontier Detection =====
	FrontierMinEvidence int // FRONTIER_MIN_EVIDENCE — min evidence_count for frontier candidates (default: 3)
	FrontierMaxOutgoing int // FRONTIER_MAX_OUTGOING — max outgoing edges for frontier candidates (default: 2)

	// ===== FSD-2026-001: Constraint Lifecycle Closure =====

	// F1: Guardrail Enforcement Hook

	// F2a: Contradiction Detection
	ContradictionEnabled       bool    // CONTRADICTION_ENABLED — enable contradiction detection in surprise scoring (default: true)
	ContradictionSimThreshold  float64 // CONTRADICTION_SIM_THRESHOLD — min similarity for contradiction candidates (default: 0.75)
	ContradictionMaxCandidates int     // CONTRADICTION_MAX_CANDIDATES — max candidates to check for contradiction (default: 20)

	// F2b: NLI-Enhanced Contradiction Detection
	ContradictionNLIEnabled bool // CONTRADICTION_NLI_ENABLED — use sidecar NLI for contradiction detection (default: false)

	// F3: Effectiveness Feedback Persistence
	JiminyPersistenceEnabled        bool    // JIMINY_PERSISTENCE_ENABLED — enable Neo4j write-through for guidance outcomes (default: false)
	ConstraintConfidenceDecayPerNeg float64 // CONSTRAINT_CONFIDENCE_DECAY_PER_NEGATIVE — confidence reduction per ignored guidance (default: 0.03)
	ConstraintConfidenceBoostPerPos float64 // CONSTRAINT_CONFIDENCE_BOOST_PER_POSITIVE — confidence increase per followed guidance (default: 0.02)
	ConstraintArchiveThreshold      float64 // CONSTRAINT_ARCHIVE_THRESHOLD — confidence below which constraints auto-archive (default: 0.3)

	// F4: Cross-Constraint Conflict Detection
	ConstraintConflictDetectionEnabled bool    // CONSTRAINT_CONFLICT_DETECTION_ENABLED — enable pairwise conflict detection (default: false)
	ConstraintConflictSimThreshold     float64 // CONSTRAINT_CONFLICT_SIM_THRESHOLD — similarity threshold for conflict detection (default: 0.6)
	ConstraintConflictMaxPairs         int     // CONSTRAINT_CONFLICT_MAX_PAIRS — max constraint pairs to compare (default: 500)

	// F6: Constraint Classifier Gate
	ConstraintClassifierGateEnabled bool // CONSTRAINT_CLASSIFIER_GATE_ENABLED — enable LLM classifier gate for constraint detection (default: false)
	ConstraintNLIEnabled            bool // CONSTRAINT_NLI_ENABLED — enable sidecar NLI for constraint classification (default: false)

	// F7: Constraint Scope Filtering
	ConstraintScopeFilteringEnabled bool // CONSTRAINT_SCOPE_FILTERING_ENABLED — enable scope-based constraint filtering (default: false)

	// F9: Asymmetric Learning + Determinism Score
	LearningAsymmetricEnabled bool // LEARNING_ASYMMETRIC_ENABLED — enable directional edge weighting (default: false)
	DeterminismScoringEnabled bool // DETERMINISM_SCORING_ENABLED — enable determinism score computation (default: false)

	// F10: Jiminy Latency Optimization
	JiminyCacheEnabled   bool // JIMINY_CACHE_ENABLED — enable constraint result cache (default: true)
	JiminyCacheTTLSec    int  // JIMINY_CACHE_TTL_SEC — cache TTL in seconds (default: 300)
	JiminyCacheSize      int  // JIMINY_CACHE_SIZE — max cache entries (default: 200)
	JiminyCacheJ17Bypass bool // JIMINY_CACHE_J17_BYPASS — bypass cache for J17 sessions to prevent cross-session contamination (default: true)

	// F11: Configurable Activation Dimension Weights
	ActivationDimSemanticWeight     float64 // ACTIVATION_DIM_SEMANTIC_WEIGHT — semantic dimension weight (default: 0.6)
	ActivationDimTemporalWeight     float64 // ACTIVATION_DIM_TEMPORAL_WEIGHT — temporal dimension weight (default: 0.2)
	ActivationDimCoactivationWeight float64 // ACTIVATION_DIM_COACTIVATION_WEIGHT — coactivation dimension weight (default: 0.2)

	// F13: Constraint Decay/Expiry

	// F15: Configurable Hop Depth
	MaxHopDepth int // MAX_HOP_DEPTH — maximum hop depth for architecture queries (default: 3)

	// F18: Edge Limit Enforcement
	LearningAutoPruneExcessEnabled bool // LEARNING_AUTO_PRUNE_EXCESS_ENABLED — auto-prune excess edges during consolidation (default: false)

	// F20: Authority Levels
	ConstraintAuthorityEnabled bool   // CONSTRAINT_AUTHORITY_ENABLED — enable authority-level filtering (default: false)
	ConstraintDefaultAuthority string // CONSTRAINT_DEFAULT_AUTHORITY — default authority level for new constraints (default: "team_standard")

	// ===== Neural Re-Ranker =====

	// NR-1: Training Data Collection
	NeuralDataCollection bool   // NEURAL_DATA_COLLECTION — enable passive training data collection (default: false)
	NeuralDataDir        string // NEURAL_DATA_DIR — directory for training data JSONL files (default: ".mdemg/neural/training-data")

	// NR-2: Python Sidecar
	SidecarEnabled bool // SIDECAR_ENABLED — enable Python neural sidecar (default: false)

	// NR-3: Neural Reranker Integration
	NeuralRerankEnabled   bool   // NEURAL_RERANK_ENABLED — enable neural cross-encoder re-ranking (default: false)
	NeuralRerankURL       string // NEURAL_RERANK_URL — sidecar service URL (default: "http://localhost:8100")
	NeuralRerankTimeoutMs int    // NEURAL_RERANK_TIMEOUT_MS — timeout for sidecar re-rank call in ms (default: 1000)
	NeuralRerankFallback  string // NEURAL_RERANK_FALLBACK — fallback provider if sidecar unavailable (default: from RERANK_PROVIDER)

	// ===== Synergy: Claude Code ↔ MDEMG Token Optimization =====

	SynergyMemoryLineThreshold    int    // SYNERGY_MEMORY_LINE_THRESHOLD — line count before overflow triggers (default: 120)
	SynergyClaudeMDPath           string // SYNERGY_CLAUDE_MD_PATH — path to CLAUDE.md (default: auto-detect)
	SynergyMemoryMDPath           string // SYNERGY_MEMORY_MD_PATH — path to MEMORY.md (default: auto-detect)
	SynergyAssessmentEnabled      bool   // SYNERGY_ASSESSMENT_ENABLED — master switch for synergy RSIC dimension (default: true)
	SynergyTargetClaudeLines      int    // SYNERGY_TARGET_CLAUDE_LINES — target line count for CLAUDE.md (default: 150)
	SynergyTargetMemoryLines      int    // SYNERGY_TARGET_MEMORY_LINES — target line count for MEMORY.md (default: 120)
	SynergyOverlapSampleSize      int    // SYNERGY_OVERLAP_SAMPLE_SIZE — lines sampled for overlap check (default: 5)
	SynergyOverflowAlertThreshold int    // SYNERGY_OVERFLOW_ALERT_THRESHOLD — overflow events/24h before RSIC alert (default: 5)

	// ===== RSIC Overall-Health Weights (DH-005) =====
	// Base priors for ComputeOverallHealth's 7 sub-dimensions. Values need not
	// sum to 1.0 — the formula normalises. Zero or negative disables a dim.
	// Defaults mirror DefaultHealthWeights() in internal/ape/health_formula.go.

	RSICHealthWeightRetrieval float64 // RSIC_HEALTH_WEIGHT_RETRIEVAL — overall-health weight for RetrievalQuality (default: 0.08)
	RSICHealthWeightMemory    float64 // RSIC_HEALTH_WEIGHT_MEMORY    — overall-health weight for MemoryHealth (default: 0.15)
	RSICHealthWeightEdge      float64 // RSIC_HEALTH_WEIGHT_EDGE      — overall-health weight for EdgeHealth (default: 0.15)
	RSICHealthWeightTask      float64 // RSIC_HEALTH_WEIGHT_TASK      — overall-health weight for TaskPerformance (default: 0.20)
	RSICHealthWeightGuidance  float64 // RSIC_HEALTH_WEIGHT_GUIDANCE  — overall-health weight for GuidanceHealth (default: 0.17)
	RSICHealthWeightProtocol  float64 // RSIC_HEALTH_WEIGHT_PROTOCOL  — overall-health weight for ProtocolHealth (default: 0.20)
	RSICHealthWeightSynergy   float64 // RSIC_HEALTH_WEIGHT_SYNERGY   — overall-health weight for SynergyHealth (default: 0.05)

	FTBenchStalenessDays int // FT_BENCH_STALENESS_DAYS — alert when `benchmark_runs` shows no completed run in the last N days (default: 7). Nothing schedules the benchmark automatically today (FT-RECURSIVE-002 default-off); this catches operator neglect (FT-BENCH-REFRESH-001).

	RSICEdgeEntropyFloor float64 // RSIC_EDGE_ENTROPY_FLOOR — Shannon-entropy floor below which the Edge dimension applies a -0.2 penalty. The entropy is binary over CO_ACTIVATED_WITH edges' evidence_count>=5 (strong vs weak) — a mature Hebbian substrate accumulates many single-touch co-activations that never re-trigger, so p is naturally small and entropy is naturally low. Old hardcoded 0.5 fired forever on any healthy live graph (live mdemg-dev: entropy=0.27, penalty=-0.2, dimension pinned at 0.80). Default 0.2 is calibrated below the observed healthy value; set to 0 to disable the penalty entirely (DASHBOARD-TRUTH-002 E2)

	// Synergy Recovery Buffer: store-and-forward during Jiminy outages
	SynergyRecoveryBufferSpace string // SYNERGY_RECOVERY_BUFFER_SPACE — CMS space for buffered observations (default: "synergy-buffer")
	SynergyRecoveryBufferPath  string // SYNERGY_RECOVERY_BUFFER_PATH — local JSONL fallback path (default: ".mdemg/synergy-recovery-buffer.jsonl")

	// ===== TimescaleDB (Historical Metrics + FT Data) =====

	TSDBEnabled               bool   // TSDB_ENABLED — enable TimescaleDB metric storage (default: false)
	TSDBHost                  string // TSDB_HOST — TimescaleDB host (default: "localhost")
	TSDBPort                  int    // TSDB_PORT — TimescaleDB port (default: 5432)
	TSDBUser                  string // TSDB_USER — TimescaleDB user (default: "mdemg")
	TSDBPassword              string // TSDB_PASSWORD — TimescaleDB password (default: "mdemg_metrics")
	TSDBDatabase              string // TSDB_DATABASE — TimescaleDB database (default: "mdemg_metrics")
	TSDBSSLMode               string // TSDB_SSL_MODE — SSL mode for TimescaleDB (default: "disable")
	TSDBMaxConns              int    // TSDB_MAX_CONNS — max connection pool size (default: 10)
	TSDBFlushIntervalSec      int    // TSDB_FLUSH_INTERVAL_SEC — metric writer flush interval in seconds (default: 60)
	TSDBRawRetentionDays      int    // TSDB_RAW_RETENTION_DAYS — raw sample retention in days (default: 90)
	TSDBHourlyRetentionDays   int    // TSDB_HOURLY_RETENTION_DAYS — hourly aggregate retention in days (default: 365)
	TSDBRequiredSchemaVersion int    // TSDB_REQUIRED_SCHEMA_VERSION — minimum required TSDB schema version (default: 31 post-CONTRADICTED-BRIDGE-APPLIED-NODE-ID-001 V0031 applied_node_id column)
	TSDBOptional              bool   // TSDB_OPTIONAL — if true, TSDB failure is non-fatal on startup (default: true)
	InstanceID                string // MDEMG_INSTANCE_ID — identifies this node for multi-instance coordination (default: "{hostname}-{space_id}")
	LLMInteractionLogging     bool   // LLM_INTERACTION_LOGGING — log all LLM calls to llm_interactions table (default: true)
	EmbeddingEventLogging     bool   // EMBEDDING_EVENT_LOGGING — log all Embed() calls for contrastive training data (default: true)
	RetrievalEventLogging     bool   // RETRIEVAL_EVENT_LOGGING — log all Retrieve() pipelines for contrastive training data (default: true)

	// JIMINY-RELEVANCE-001 Epic 1 — guidance_training_rows (V0027): persist the
	// training EVIDENCE (guidance text + action text + source role/layer + verdict)
	// per guidance-feedback item that was previously discarded.
	GuidanceCorpusEnabled                bool // GUIDANCE_CORPUS_ENABLED — persist guidance training evidence to guidance_training_rows (default: true)
	GuidanceCorpusWriterFlushIntervalSec int  // GUIDANCE_CORPUS_WRITER_FLUSH_INTERVAL_SEC — buffered writer flush cadence in seconds (default: 30, floor: 5)
	GuidanceCorpusWriterBufferSize       int  // GUIDANCE_CORPUS_WRITER_BUFFER_SIZE — max rows held before FIFO eviction (default: 1000, 0 = unlimited)

	// JIMINY-CONTRADICTED-BRIDGE-001: contradicted-outcome → correction draft bridge.
	JiminyContradictedBridgeEnabled                bool // JIMINY_CONTRADICTED_BRIDGE_ENABLED — emit draft rows on OutcomeContradicted (default: true post-JIMINY-CONTRADICTED-BRIDGE-001 E5 live smoke)
	JiminyContradictedBridgeWriterFlushIntervalSec int  // JIMINY_CONTRADICTED_BRIDGE_WRITER_FLUSH_INTERVAL_SEC — buffered writer flush cadence (default: 30, floor: 5)
	JiminyContradictedBridgeWriterBufferSize       int  // JIMINY_CONTRADICTED_BRIDGE_WRITER_BUFFER_SIZE — max rows held before FIFO eviction (default: 1000, 0 = unlimited)
	JiminyContradictedBridgeMaxContentLen          int  // JIMINY_CONTRADICTED_BRIDGE_MAX_CONTENT_LEN — cap draft_incorrect/draft_correct at this many chars (default: 400)
	GuidanceCorpusMaxContentBytes                  int  // GUIDANCE_CORPUS_MAX_CONTENT_BYTES — truncate guidance/action snapshots to this many bytes (default: 8192, floor: 256)
	GuidanceCorpusSourceLookupTimeoutMs            int  // GUIDANCE_CORPUS_SOURCE_LOOKUP_TIMEOUT_MS — bounded best-effort Neo4j lookup of source-node role_type/layer at emit (default: 300, 0 = disable; never blocks the hot path beyond this)

	// JIMINY-RELEVANCE-001 Epic 2 — guidance label-quality auto-relabel job.
	// Re-labels heuristic/blank-classifier_source rows in guidance_training_rows
	// with the LLM classifier so the corpus is trustworthy (diagnostic Finding 4:
	// ~51% of labels were heuristic noise).
	GuidanceAuditEnabled         bool // GUIDANCE_AUDIT_ENABLED — run the periodic auto-relabel job (default: true)
	GuidanceAuditIntervalHours   int  // GUIDANCE_AUDIT_INTERVAL_HOURS — cadence of the auto-relabel job (default: 24, floor: 1)
	GuidanceAuditSampleSize      int  // GUIDANCE_AUDIT_SAMPLE_SIZE — heuristic/blank rows re-labelled per run (default: 50, floor: 1)
	GuidanceAuditInitialDelaySec int  // GUIDANCE_AUDIT_INITIAL_DELAY_SEC — delay before the FIRST auto-relabel run after startup, so first cleanup doesn't wait a full interval (default: 60, 0 = skip the initial run)

	// JIMINY-RELEVANCE-001 Epic 4 — should-follow follow-rate alert.
	// "Follow rate on guidance that SHOULD have been followed" (actionable
	// constraint/correction types) — excludes correctly-ignored advisory items.
	GuidanceShouldFollowRateFloor     float64 // GUIDANCE_SHOULD_FOLLOW_RATE_FLOOR — alert when actionable-compliance rate drops below this (default: 0.05 — must sit BELOW the ~0.10-0.14 by-design steady state so only genuine collapse fires; was 0.5 pre-JIMINY-ACTIONABILITY-INVERSION-001 verdict; 0 disables the rule)
	GuidanceShouldFollowLookbackHours int     // GUIDANCE_SHOULD_FOLLOW_LOOKBACK_HOURS — window for the should-follow rate (default: 168 = 7d, floor: 1)

	// HITL-REVIEW-001 — general-purpose human-in-the-loop review + live-reinforcement platform.
	ReviewEnabled                    bool    // REVIEW_ENABLED — enable the /v1/review/* surface + UI tab (default: true)
	ReviewWriterFlushIntervalSec     int     // REVIEW_WRITER_FLUSH_INTERVAL_SEC — review_grades writer flush cadence (default: 15, floor: 5)
	ReviewWriterBufferSize           int     // REVIEW_WRITER_BUFFER_SIZE — max buffered grade rows before FIFO eviction (default: 500, 0 = unbounded)
	ReviewRubricVersion              string  // REVIEW_RUBRIC_VERSION — the current rubric version; grades not at this version aren't "certified-current" (default: gr-v1)
	ReviewSampleSize                 int     // REVIEW_SAMPLE_SIZE — max items the sampler selects per fetch (default: 200, floor: 1)
	ReviewActiveUncertaintyBand      float64 // REVIEW_ACTIVE_UNCERTAINTY_BAND — prefer items whose AutoScore is within this band of 0.5 (default: 0.4)
	ReviewSampleSeed                 int64   // REVIEW_SAMPLE_SEED — 0 = time-seeded; non-zero = deterministic sampling (default: 0)
	ReviewReinforceDefault           bool    // REVIEW_REINFORCE_DEFAULT — default for the per-grade reinforce flag; false = grades are gold-only unless reinforce:true is sent (default: false)
	ReviewGuidanceSinkEnabled        bool    // REVIEW_GUIDANCE_SINK_ENABLED — enable the guidance live-reinforcement sink (default: true)
	ReviewGuidanceConfidenceNudge    float64 // REVIEW_GUIDANCE_CONFIDENCE_NUDGE — node-confidence delta the guidance sink applies (default: 0.05)
	ReviewLLMDatasetsEnabled         bool    // REVIEW_LLM_DATASETS_ENABLED — register the 16 MDEMG LLM call sites as reviewable datasets (gold-only review of llm_interactions) (default: true)
	ReviewContradictedDatasetEnabled bool    // REVIEW_CONTRADICTED_DATASET_ENABLED — register the JIMINY-CONTRADICTED-BRIDGE-001 correction-drafts dataset (default: true)

	// EVENTGRAPH-001 — TSDB reinforcement_events + federation API (Pattern Y1)
	EventGraphEnabled                        bool // EVENTGRAPH_ENABLED — record per-pair Hebbian telemetry into reinforcement_events + expose federation API (default: true)
	EventGraphWriterFlushIntervalSec         int  // EVENTGRAPH_WRITER_FLUSH_INTERVAL_SEC — buffered writer flush cadence in seconds (default: 30, floor: 5)
	EventGraphWriterBufferSize               int  // EVENTGRAPH_WRITER_BUFFER_SIZE — max rows held before FIFO eviction (default: 1000, 0 = unlimited)
	EventGraphMaxPairsPerEventBatch          int  // EVENTGRAPH_MAX_PAIRS_PER_EVENT_BATCH — defensive cap on rows emitted per ApplyCoactivation invocation (default: 200, matches LearningEdgeCapPerRequest)
	EventGraphMaxEventsPerQuery              int  // EVENTGRAPH_MAX_EVENTS_PER_QUERY — federation API ceiling on returned events (default: 500)
	EventGraphFederationDefaultHops          int  // EVENTGRAPH_FEDERATION_DEFAULT_HOPS — federation API default hops when request omits the field (default: 2)
	EventGraphFederationDefaultLookbackHours int  // EVENTGRAPH_FEDERATION_DEFAULT_LOOKBACK_HOURS — federation API default lookback window in hours (default: 24)

	// Live Metrics (collect-on-request)
	LiveMetricsEnabled     bool // LIVE_METRICS_ENABLED — enable live metric collection on metrics snapshot (default: true)
	LiveGuidanceRefreshSec int  // LIVE_GUIDANCE_REFRESH_SEC — seconds between Jiminy guidance cache refreshes (default: 60)

	// ===== Alert Dispatcher =====
	AlertEnabled           bool   // ALERT_ENABLED — master switch for alert delivery (default: true)
	AlertFilePath          string // ALERT_FILE_PATH — path to alert JSON file (default: ~/.mdemg/alerts/current.json)
	AlertCooldownSec       int    // ALERT_COOLDOWN_SEC — per-service-severity cooldown in seconds (default: 300)
	AlertMaxEntries        int    // ALERT_MAX_ENTRIES — max alerts in file before FIFO eviction (default: 50)
	AlertMacOSNotify       bool   // ALERT_MACOS_NOTIFY — enable macOS desktop notifications (default: false)
	AlertMacOSNotifyMinSev string // ALERT_MACOS_NOTIFY_MIN_SEV — minimum severity for macOS notify (default: "high")

	// ===== Health Probe =====
	HealthProbeEnabled     bool // HEALTH_PROBE_ENABLED — enable periodic health probing (default: true)
	HealthProbeIntervalSec int  // HEALTH_PROBE_INTERVAL_SEC — probe interval in seconds (default: 60)

	// ===== Goroutine Supervisor (SUPERVISOR-002) =====
	SupervisorMaxRestarts      int // SUPERVISOR_MAX_RESTARTS — restarts allowed within the sliding window before permanent failure (default: 3)
	SupervisorRestartWindowMin int // SUPERVISOR_RESTART_WINDOW_MIN — sliding restart-budget window in minutes (default: 60)
	SupervisorBackoffBaseSec   int // SUPERVISOR_BACKOFF_BASE_SEC — base restart backoff in seconds, doubles per in-window restart (default: 5)

	// ===== Alert Rule Health (SUPERVISOR-002) =====
	AlertRuleFailureThreshold int // ALERT_RULE_FAILURE_THRESHOLD — consecutive query failures before a rule-health meta-alert fires (default: 3)

	// ===== RSIC LLM-Health Recency Gate (SUPERVISOR-002) =====
	RSICLLMErrorRecencyMin int // RSIC_LLM_ERROR_RECENCY_MIN — minutes; llm_error_rate_spike fires only if the most recent error is this fresh (default: 60, 0 disables the gate)
	RSICLLMErrorMinCount   int // RSIC_LLM_ERROR_MIN_COUNT — absolute minimum error count for llm_error_rate_spike to fire (ALERT-TRUTH-001); the rate-only floor (>5% & >10 calls) re-fired HIGH 23× on just 2 "context canceled" errors at low call volume (default: 5, 0 disables the count floor)

	// ===== LLM Client Retry =====
	LLMRetryEnabled     bool    // LLM_RETRY_ENABLED — enable retry for transient LLM errors (default: true)
	LLMRetryMaxAttempts int     // LLM_RETRY_MAX_ATTEMPTS — max retry attempts (default: 5)
	LLMRetryBaseDelayMs int     // LLM_RETRY_BASE_DELAY_MS — initial backoff delay in ms (default: 500)
	LLMRetryMaxDelayMs  int     // LLM_RETRY_MAX_DELAY_MS — max backoff delay in ms (default: 60000)
	LLMRetryMultiplier  float64 // LLM_RETRY_MULTIPLIER — exponential backoff multiplier (default: 2.0)
	LLMRetryJitter      float64 // LLM_RETRY_JITTER — jitter factor 0-1 (default: 0.2)
	LLMRetryOnDeadline  bool    // LLM_RETRY_DEADLINE_ENABLED — retry on context.DeadlineExceeded when budget allows (default: true, DH-004 E4.2)

	// ===== LLM Consecutive Failure Alert =====
	LLMConsecutiveFailureThreshold int // LLM_CONSECUTIVE_FAILURE_THRESHOLD — fires alert after N consecutive failures (default: 3)

	// ===== Alert Evaluator =====
	AlertEvaluatorEnabled     bool // ALERT_EVALUATOR_ENABLED — enable server-native alert rule evaluation (default: true)
	AlertEvaluatorIntervalSec int  // ALERT_EVALUATOR_INTERVAL_SEC — base tick interval in seconds (default: 30)

	// TSDB-CONSUME-001 — retrieve-latency SLO rules over retrieval_audit
	// real wall-time (replaced the broken lifetime-cumulative HTTP synthetics).
	// Defaults calibrated from the live distribution (7d p95 61.6s / p99 90s).
	AlertRetrieveP95Ms              int // ALERT_RETRIEVE_P95_MS — medium alert when windowed retrieve p95 exceeds this (default: 120000)
	AlertRetrieveP99Ms              int // ALERT_RETRIEVE_P99_MS — critical alert when windowed retrieve p99 exceeds this (default: 300000)
	AlertRetrieveLatencyLookbackMin int // ALERT_RETRIEVE_LATENCY_LOOKBACK_MIN — percentile window in minutes (default: 30)
	TSDBWriterAlertLookbackMin      int // TSDB_WRITER_ALERT_LOOKBACK_MIN — window for buffered-writer flush-failure growth detection (default: 60)

	// TSDB-CONSUME-001 — scorer-drift tripwires over retrieval_audit (the
	// RRF-SCALE-001 regression class: scorer changes silently breaking
	// downstream Score consumers).
	ScorerChangeLookbackHours  int     // SCORER_CHANGE_LOOKBACK_HOURS — window for the >1-distinct-scorer_version tripwire (default: 24)
	ConsensusShiftThreshold    float64 // CONSENSUS_SHIFT_THRESHOLD — |recent − baseline| mean consensus_strength to alert on; live σ≈0.097 (default: 0.10)
	ConsensusShiftRecentHours  int     // CONSENSUS_SHIFT_RECENT_HOURS — recent comparison window (default: 6)
	ConsensusShiftBaselineDays int     // CONSENSUS_SHIFT_BASELINE_DAYS — trailing baseline window (default: 7)
	ConsensusShiftMinSamples   int     // CONSENSUS_SHIFT_MIN_SAMPLES — minimum rows in each window before the rule can fire (default: 20)

	// TSDB-CONSUME-001 — emergence-cycle duration tripwire (the DBSCAN O(n²)
	// deferral condition becomes observable).
	EmergenceCycleAlertThresholdSec float64 // EMERGENCE_CYCLE_ALERT_THRESHOLD_SEC — alert when a cycle exceeds this wall time (default: 60)
	EmergenceCycleAlertLookbackMin  int     // EMERGENCE_CYCLE_ALERT_LOOKBACK_MIN — window scanned for slow cycles (default: 120)

	// SURPRISE-TOPK-001 — vector-index top-K novelty (replaces the unordered
	// LIMIT 50 sample that made embedding novelty noise).
	SurpriseEmbeddingNoveltyTopK     int     // SURPRISE_EMBEDDING_NOVELTY_TOPK — nearest neighbors compared for embedding novelty (default: 50)
	SurpriseEmbeddingNoveltySimFloor float64 // SURPRISE_EMBEDDING_NOVELTY_SIM_FLOOR — drop neighbors below this cosine sim; 0 = off (default: 0)
	SurpriseFactorHighThreshold      float64 // SURPRISE_FACTOR_HIGH_THRESHOLD — surprise_score for the 2.0x edge multiplier; live-calibrated: corrections score ~0.455, noise band 0.02-0.10 (default: 0.4)
	SurpriseFactorMediumThreshold    float64 // SURPRISE_FACTOR_MEDIUM_THRESHOLD — surprise_score for the 1.5x multiplier, above the live noise band (default: 0.15)

	// NOSILENT-001 — scheduled-job health alerting (no silent failures).
	JobHealthAlertEnabled   bool // JOB_HEALTH_ALERT_ENABLED — enable scheduled-job staleness/failure alert rules (default: true)
	JobBackupStalenessHours int  // JOB_BACKUP_STALENESS_HOURS — alert if no successful tsdb-backup within this window; 0 = derive from TSDB_BACKUP_INTERVAL_HOURS × 2 (default: 0)
	JobFailureLookbackMin   int  // JOB_FAILURE_LOOKBACK_MIN — alert if any scheduled job failed within this lookback window (default: 60)

	// HOOKSYNC-001 — hook-channel absence detection (the "job never ran"
	// guarantee applied to the per-prompt delivery channel).
	HookHealthAlertEnabled  bool // HOOK_HEALTH_ALERT_ENABLED — enable the hook-channel-silent alert rule (default: true)
	HookSilentLookbackHours int  // HOOK_SILENT_LOOKBACK_HOURS — window for "sessions active but prompt-context never fired" (default: 24)
	HookActivityMinEvents   int  // HOOK_ACTIVITY_MIN_EVENTS — post-tool-observe heartbeats required in the window before the rule is eligible (default: 5)

	// HIDDEN-WEIGHT-001 — abstraction-edge weight integrity.
	NullWeightEdgeAlertThreshold int // NULL_WEIGHT_EDGE_ALERT_THRESHOLD — alert when NULL-weight GENERALIZES/ABSTRACTS_TO edges exceed this (default: 100; steady state is 0)

	// Orphan alert rules (ORPHAN-ALERT-001)
	OrphanRatioMinNodes   int     // ORPHAN_RATIO_MIN_NODES — minimum per-space node count for a space to be eligible to fire the orphan alerts; excludes tiny test/scratch spaces (1-node space = ratio 1.0) (default: 50)
	OrphanRatioThreshold  float64 // ORPHAN_RATIO_THRESHOLD — fire High Orphan Ratio when max live-orphan ratio among significant spaces exceeds this (default: 0.10)
	OrphanCountThreshold  int     // ORPHAN_COUNT_THRESHOLD — fire High Orphan Count when max live-orphan count among significant spaces exceeds this (default: 1000; above the accepted historical baseline — ratio rule is the scale-aware primary)
	GraphHealthScoreFloor float64 // GRAPH_HEALTH_SCORE_FLOOR — fire Low Graph Health when the MIN graph-health score among significant spaces drops below this (default: 0.5; same min-node floor as the orphan rules so degenerate 1-2-node test spaces can't trip it)

	// Neo4j container CPU alert (ALERT-TRUTH-001)
	Neo4jCPUAlertThresholdPercent float64 // NEO4J_CPU_ALERT_THRESHOLD_PERCENT — fire Neo4j High CPU when the 5-min windowed AVG of mdemg_neo4j_container_cpu_percent exceeds this. docker-stats CPU% is per-single-core, so this is HOST-RELATIVE; default 500 is calibrated above the worst normal-operation 5-min windowed AVG (live 24h max 471, heavy consolidation) so it fires only on genuinely pathological sustained load; lower it on smaller machines (default: 500)

	// FT recursive-loop readiness staleness (FT-RECURSIVE-001 SF-1)
	FtReadinessStalenessMin int // FT_READINESS_STALENESS_MIN — fire training_readiness_stale when no successful RSIC readiness assessment within this many minutes; catches a silently-dormant loop (default: 30; RSIC assesses every ~5 min)

	// Training-data export retention (FT-RECURSIVE-001 SF-6)
	ExportRetentionHours int // MDEMG_EXPORT_RETENTION_HOURS — prune training-data export archives in the temp export dir older than this on each new export; the dir grew unbounded (default: 168 = 7 days; 0 disables pruning)

	// FT recursive-retrain readiness threshold (FT-RECURSIVE-002 Phase 6b, AMD-7)
	TrainingReadinessThreshold          int            // TRAINING_READINESS_THRESHOLD — minimum rows per task to be training-ready (replaces hardcoded DefaultReadinessThreshold=500)
	TrainingReadinessThresholdOverrides map[string]int // TRAINING_READINESS_THRESHOLD_OVERRIDES — per-task JSON overrides, e.g. {"consulting.classify":300}

	// FT recursive-retrain trigger gate (FT-RECURSIVE-002 Phase 6b)
	FtLoopEnabled                 bool    // FT_LOOP_ENABLED — master gate for the recursive-retrain actuator; default false keeps it dormant (the no-op→real cutover is reversible by config). When false the trigger insight is suppressed (readiness stays observable via `mdemg data status` + the SF-1 heartbeat).
	FtLoopMinRetrainIntervalHours int     // FT_LOOP_MIN_RETRAIN_INTERVAL_HOURS — suppress a new trigger if a cycle started within this window (single-flight at retrain scale, not the RSIC seconds cooldown) (default: 168 = 7 days)
	FtLoopMinFreshFraction        float64 // FT_LOOP_MIN_FRESH_FRACTION — minimum fraction of interactions newer than the last cycle before retraining (retrain on new signal, never the same corpus) (default: 0.30)

	// FT recursive-retrain controller (FT-RECURSIVE-002 Phase 6b, Epic 5)
	FtLoopPollIntervalSec    int     // FT_LOOP_POLL_INTERVAL_SEC — controller poll cadence for a triggered cycle (default: 60)
	FtLoopLeaseMaxHours      int     // FT_LOOP_LEASE_MAX_HOURS — compute-lease expiry (≈1.5× the ~9h actual) so a crashed trainer can't wedge RSIC (default: 14)
	FtLoopLeasePath          string  // FT_LOOP_LEASE_PATH — compute-lease lockfile path (default: $HOME/.mdemg/ft-loop.lease)
	FtLoopMinFreeDiskGB      float64 // FT_LOOP_MIN_FREE_DISK_GB — disk-floor preflight before TRAIN (~85GB transient) (default: 100)
	FtLoopPythonBin          string  // FT_LOOP_PYTHON_BIN — python interpreter for the training subprocesses (default: python3)
	FtLoopModelVersion       string  // FT_LOOP_MODEL_VERSION — model_version recorded on cycles (default: mdemg-llm-v1)
	FtLoraEpochsCap          int     // FT_LORA_EPOCHS_CAP — hard epoch cap passed to train_ft.py (AMD-1; the `auto` rejection is never weakened) (default: 3)
	FtEarlyStopValLossFactor float64 // FT_EARLY_STOP_VAL_LOSS_FACTOR — SFT early-stop trips when val_loss > best × this (AMD-1) (default: 1.05)

	// FT recursive-retrain pipeline wiring (FT-RECURSIVE-002 Phase 6b, Epic 6 — the proven curate→train→convert→gate commands)
	FtLoopWorkDir         string  // FT_LOOP_WORK_DIR — per-cycle artifact root (export/curate/adapter/candidate) (default: $TMPDIR/mdemg-ft-loop)
	FtLoopBaseModel       string  // FT_LOOP_BASE_MODEL — dense base model dir for LoRA training (default: .local-models/qwen3-14b-4bit-base)
	FtLoopBaseSHA         string  // FT_LOOP_BASE_SHA — base config.json SHA pin passed to train_ft --expected-sha256 (default: a54ec18f… — the E6-8 updated pin)
	FtLoopUaitsSpec       string  // FT_LOOP_UAITS_SPEC — curation spec for paradigm_router (default: docs/tests/uaits/specs/mdemg.uaits.json)
	FtLoopBenchmarkConfig string  // FT_LOOP_BENCHMARK_CONFIG — gate benchmark config yaml (default: configs/benchmark_phase10.yaml)
	FtLoopLoraRank        int     // FT_LOOP_LORA_RANK — LoRA rank for train_ft (default: 32 — production recipe)
	FtLoopLoraAlpha       int     // FT_LOOP_LORA_ALPHA — LoRA alpha for train_ft (default: 64)
	FtLoopGatePort        int     // FT_LOOP_GATE_PORT — side-port for serving the candidate during the gate (default: 18102)
	FtLoopExportSinceDays int     // FT_LOOP_EXPORT_SINCE_DAYS — export window for the curate input (default: 7)
	FtLoopGateTaskFilter  string  // FT_LOOP_GATE_TASK_FILTER — optional run_benchmark --task-filter to scope the gate (default: empty = all tasks)
	FtLoopGateMinScore    float64 // FT_LOOP_GATE_MIN_SCORE — minimum aggregate benchmark score for the gate to PASS (default: 0.80, matches UBENCH min_aggregate_weighted_score)
	FtLoopCanaryEnabled    bool   // FT_LOOP_CANARY_ENABLED — pre-swap held-call replay gate on ft-loop promote (default: false; flip after live smoke)
	FtLoopCanaryProbes     string // FT_LOOP_CANARY_PROBES — jsonl probe corpus for the canary (default: training_data/eval/valid_clean.jsonl — the [AMD-2]-pinned eval)
	FtLoopCanaryProbeCount int    // FT_LOOP_CANARY_PROBE_COUNT — probes replayed (first-per-task, deterministic; default: 8)
	FtLoopCanaryProdURL    string // FT_LOOP_CANARY_PROD_URL — production OpenAI-compat base the canary compares against (default: http://127.0.0.1:8102/v1)
	FtLoopServingSymlink       string // FT_LOOP_SERVING_SYMLINK — serving indirection symlink the plist points at (default: .local-models/serving/current.gguf; FT-RECURSIVE-003 E2)
	FtLoopServingPlistLabel    string // FT_LOOP_SERVING_PLIST_LABEL — LaunchAgent label kickstarted on swap (default: com.mdemg.llama-server)
	FtLoopServingHealthURL     string // FT_LOOP_SERVING_HEALTH_URL — health probe after swap (default: http://127.0.0.1:8102/health)
	FtLoopSwapHealthTimeoutSec int    // FT_LOOP_SWAP_HEALTH_TIMEOUT_SEC — fail-closed revert if the swapped model is not healthy within this (default: 120, floor 10)
	FtLoopConvertScript   string  // FT_LOOP_CONVERT_SCRIPT — explicit convert_hf_to_gguf.py path; empty = PATH → /opt/homebrew/bin → /usr/local/bin (FTLOOP-DRILL-001: launchd minimal PATH broke the bare LookPath)

	// MAINT-LIVE-001 — maintenance liveness (only-ever-dry-runs detection).
	MaintLiveAlertEnabled bool // MAINT_LIVE_ALERT_ENABLED — enable the maintenance_no_live_run rule (default: true)
	MaintLiveLookbackDays int  // MAINT_LIVE_LOOKBACK_DAYS — window in which at least one live (dry_run=false) maintenance run must appear when any maintenance runs exist (default: 8)

	// ===== TSDB Writer =====
	TSDBWriterBufferMaxSize int // TSDB_WRITER_BUFFER_MAX_SIZE — max buffered records before FIFO eviction (default: 1000)
}

// EffectiveLLMEndpoint returns the endpoint for LLM text-generation calls.
// If LLMEndpoint is set, it is used; otherwise falls back to OpenAIEndpoint.
// Single source of truth for config defaults that are also referenced as
// zero-value fallbacks by consuming packages (consulting, jiminy). FromEnv uses
// these as the env defaults, and the consuming packages alias them — so each
// value is defined in exactly one place. Change here to change everywhere.
const (
	DefaultJiminyWarmComputeTimeoutMs = 90000 // JIMINY_WARM_COMPUTE_TIMEOUT_MS (warm path + /v1/jiminy/guide)

	// RRF-SCALE-001 — consulting score gates + confidence sigmoid.
	DefaultConsultingConstraintScoreFloor      = 0.45 // CONSULTING_CONSTRAINT_SCORE_FLOOR
	DefaultConsultingSuggestMinConfidence      = 0.2  // CONSULTING_SUGGEST_MIN_CONFIDENCE (RRF-SCALE-002: live distribution — typical good hits 0.2-0.45, noise tail <0.01; 0.45 is the strong-match band, right for constraint PROMOTION but over-filtering for a recall surface)
	DefaultMCPReflectScoreHigh                 = 0.45 // MCP_REFLECT_SCORE_HIGH (RRF strong band)
	DefaultMCPReflectScoreMedium               = 0.25 // MCP_REFLECT_SCORE_MEDIUM
	DefaultGuardrailConstraintSimFloor         = 0.3  // GUARDRAIL_CONSTRAINT_SIM_FLOOR (cosine scale, not RRF)
	DefaultConsultingAuthorityScoreFloor       = 0.45 // CONSULTING_AUTHORITY_SCORE_FLOOR
	DefaultConsultingConflictScoreFloor        = 0.45 // CONSULTING_CONFLICT_SCORE_FLOOR
	DefaultRetrievalConfidenceSigmoidMidpoint  = 0.45 // RETRIEVAL_CONFIDENCE_SIGMOID_MIDPOINT
	DefaultRetrievalConfidenceSigmoidSteepness = 8.0  // RETRIEVAL_CONFIDENCE_SIGMOID_STEEPNESS

	// JIMINY-OUTCOME-001 — embedding-similarity constraint-code matching.
	DefaultJiminyConstraintCodeSimThreshold = 0.55 // JIMINY_CONSTRAINT_CODE_SIM_THRESHOLD

	// GUIDANCE-SYNTH-001 — per-node constraint classifier concurrency.
	DefaultConsultingClassifyConcurrency = 4 // CONSULTING_CLASSIFY_CONCURRENCY

	// JIMINY-CORPUS-001 — ConvObs→constraint promotion gate: obs_types that
	// are transient by definition (run status / failures / todo / ambient
	// context) and therefore never legitimate durable rules. Provenance is
	// the principled rejection signal; the pattern list below is only the
	// backstop for junk that arrives under a durable-looking obs_type.
	DefaultConstraintPromotionDenyObsTypes = "progress,error,task,context" // CONSTRAINT_PROMOTION_DENY_OBS_TYPES
)

// DefaultConstraintPromotionRejectPatterns returns the built-in content-shape
// backstop for the ConvObs→constraint promotion gate (JIMINY-CORPUS-001).
// Each entry is a Go regexp matched against observation content; a match
// means "this is a transient observation / document dump, not a durable
// rule" and blocks promotion to a role_type='constraint' node. Anchored to
// the junk classes diagnosed live in mdemg-dev (fabricated build/test-status
// observations, bash errors, PR/sprint status notes, doc/template dumps).
// Returned as a fresh slice so callers cannot mutate the defaults.
func DefaultConstraintPromotionRejectPatterns() []string {
	return []string{
		`^Build/test succeeded`,                 // pre-HOOKWIRE-001 fabricated tool-status observations
		`^Bash error`,                           // hook-captured command failures
		`(?i)\bapproved\s*(?:&|and)\s*merged\b`, // PR status notes
		`(?i)^(?:sprint|phase|pr)\s+[#\w./+-]+\s*(?:is\s+)?(?:fully\s+)?(?:complete|completed|implemented|shipped|merged|executed|done)\b`, // sprint/phase/PR completion status (completion word adjacent to the identifier)
		// JIMINY-CORPUS-001 Epic 2 widening: the t8j3 class — completion-status
		// observations whose completion words are NOT adjacent to the phase/PR
		// token ("Phase 9.4 <name> fully implemented, committed (0a7de6b),
		// PR #66 merged"). Two shapes:
		//  (a) completion-status PHRASES anywhere: "fully implemented",
		//      "PR #66 merged", "committed (<sha>)", "implemented, committed".
		//      A rule that merely mentions "a PR" or "merging" (no PR number
		//      immediately followed by "merged") still passes.
		`(?i)(?:\bfully\s+implemented\b|\bPR\s*#\d+\s+(?:was\s+)?merged\b|\bcommitted\s*\([0-9a-f]{6,40}\)|\bimplemented\s*,\s*committed\b)`,
		//  (b) sprint/phase/PR-led status where the completion word appears
		//      later on the first line. The identifier token must carry a
		//      digit (Phase 9.4, PR #66, Sprint TSDB-CONSUME-001) so genuine
		//      rules like "Sprint plans must include complete testing tiers"
		//      are not over-matched.
		`(?i)^(?:sprint|phase|pr)\s+[#\w./+-]*\d[#\w./+-]*[^\n]*?\b(?:fully\s+)?(?:complete|completed|implemented|shipped|merged|executed|done)\b`,
		`^\s*#{1,6}\s`, // markdown-heading-led doc/template dumps
		`(?i)^(?:sprint|phase)\s+[#\w./+-]+\s+spec\b`, // "PHASE 105 SPEC: …" design-doc dumps
		`(?i)^skill:`,        // skill-registry dumps
		`(?i)^sprint plan\b`, // sprint-plan format/checklist dumps
	}
}

// JiminyWarmComputeTimeout returns the effective Guide()-compute timeout as a
// Duration, applying the default when unset. Call sites use this instead of
// re-defaulting the literal, so the value lives in exactly one place.
func (c Config) JiminyWarmComputeTimeout() time.Duration {
	ms := c.JiminyWarmComputeTimeoutMs
	if ms <= 0 {
		ms = DefaultJiminyWarmComputeTimeoutMs
	}
	return time.Duration(ms) * time.Millisecond
}

// EffectiveJiminyTimeout returns the direct-path Guide() budget
// (JIMINY-BUDGET-001): an explicit positive JIMINY_TIMEOUT_MS wins;
// otherwise DERIVE from the warm-compute budget — the two paths run the
// same synthesis work, and the old independent 15s default starved every
// fresh install (synthesis runs ~50s live; only the warm path had 90s).
func (c Config) EffectiveJiminyTimeout() time.Duration {
	if c.JiminyTimeoutMs > 0 {
		return time.Duration(c.JiminyTimeoutMs) * time.Millisecond
	}
	return c.JiminyWarmComputeTimeout()
}

// EffectiveJiminyReformulateTimeout: same derivation for /reformulate
// (was a hardcoded 10s — shorter than every other guidance budget).
func (c Config) EffectiveJiminyReformulateTimeout() time.Duration {
	if c.JiminyReformulateTimeoutMs > 0 {
		return time.Duration(c.JiminyReformulateTimeoutMs) * time.Millisecond
	}
	return c.JiminyWarmComputeTimeout()
}

func (c Config) EffectiveLLMEndpoint() string {
	if c.LLMEndpoint != "" {
		return c.LLMEndpoint
	}
	return c.OpenAIEndpoint
}

// SparseGateOverride is one entry in SparseGateCategoryOverrides. Pointer
// fields encode "not set" (fall back to global default) vs "set to value".
// JSON marshalling handles missing keys naturally.
//
// Example JSON value: {"min_active": 20}
//
//	→ MinActive=20, MaxActive=nil, Percentile=nil
//	→ at gate site: use category MinActive=20, fall back to global MaxActive
//	  and global Percentile.
type SparseGateOverride struct {
	MinActive  *int     `json:"min_active,omitempty"`
	MaxActive  *int     `json:"max_active,omitempty"`
	Percentile *float64 `json:"percentile,omitempty"`
}

// TSDBConfig returns a tsdb.Config derived from the main config.
// NOTE: This returns a struct compatible with internal/tsdb.Config but defined here
// to avoid a circular import. The caller constructs tsdb.Config from these values.
type TSDBConnConfig struct {
	Host                string
	Port                int
	User                string
	Password            string
	Database            string
	SSLMode             string
	MaxConns            int
	FlushIntervalSec    int
	RawRetentionDays    int
	HourlyRetentionDays int
}

// GetTSDBConnConfig returns TimescaleDB connection settings.
func (c Config) GetTSDBConnConfig() TSDBConnConfig {
	return TSDBConnConfig{
		Host:                c.TSDBHost,
		Port:                c.TSDBPort,
		User:                c.TSDBUser,
		Password:            c.TSDBPassword,
		Database:            c.TSDBDatabase,
		SSLMode:             c.TSDBSSLMode,
		MaxConns:            c.TSDBMaxConns,
		FlushIntervalSec:    c.TSDBFlushIntervalSec,
		RawRetentionDays:    c.TSDBRawRetentionDays,
		HourlyRetentionDays: c.TSDBHourlyRetentionDays,
	}
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
	// CONFIG-DEADFLAG-001: strict boolean parsing. The old getBool treated
	// ANY unrecognized value as false — an operator typo like
	// FEATURE_ENABLED=ture silently disabled the feature (and "off"/"no"/
	// "0" only worked by accident of being "not true"). Invalid values now
	// fail FromEnv like atoi/atof do; errors accumulate so every bad
	// boolean surfaces in one startup failure.
	var boolErrs []string
	getBool := func(k string, def bool) bool {
		v := strings.ToLower(strings.TrimSpace(os.Getenv(k)))
		switch v {
		case "":
			return def
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		default:
			boolErrs = append(boolErrs, fmt.Sprintf("%s=%q (want true/false, 1/0, yes/no, on/off)", k, os.Getenv(k)))
			return def
		}
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
	learnSessionCliqueWindow, err := atoi("LEARNING_SESSION_CLIQUE_WINDOW", 50)
	if err != nil {
		return Config{}, err
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
	retrieveTimeoutMs, err := atoi("RETRIEVE_TIMEOUT_MS", 20000)
	if err != nil {
		return Config{}, err
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
	// Local-first defaults (CONFIG-LOCAL-DEFAULTS-001): the production runtime is
	// the local llama-server (Phase 13.5 cutover), so the LLM model + endpoint
	// default to local — a no-env install must NOT silently route to OpenAI's
	// cloud (the prior gpt-5.4-mini + EffectiveLLMEndpoint→OpenAIEndpoint default
	// contradicted the always-local architecture). OpenAI stays available only via
	// explicit OPENAI_* config (the teacher/embedding path).
	llmModel := get("LLM_MODEL", "mdemg-llm-v1")

	// Embedding provider settings
	embProvider := get("EMBEDDING_PROVIDER", "openai")
	openaiKey := get("OPENAI_API_KEY", "")
	openaiModel := get("OPENAI_MODEL", "text-embedding-3-large")
	openaiEndpoint := get("OPENAI_ENDPOINT", "https://api.openai.com/v1")
	llmEndpoint := get("LLM_ENDPOINT", "http://127.0.0.1:8102/v1")
	ollamaEndpoint := get("OLLAMA_ENDPOINT", "http://localhost:11434")
	ollamaModel := get("OLLAMA_MODEL", "qwen3-embedding:8b")

	// Sprint MODEL-DIST-001 — model distribution config (Configurability Contract).
	// Defaults match the v1 production reality so `mdemg model pull` with no flags
	// Just Works for the common case; every value is overridable via env or flag.
	homeDir, _ := os.UserHomeDir()
	modelBackend := get("MDEMG_MODEL_BACKEND", "ollama")
	modelNamespace := get("MDEMG_MODEL_NAMESPACE", "reh3376")
	modelName := get("MDEMG_MODEL_NAME", "mdemg-llm-v1")
	modelQuants := get("MDEMG_MODEL_QUANTS", "Q4_K_M,Q5_K_M,Q8_0")
	modelRamTiers := get("MDEMG_MODEL_RAM_TIERS", `{"<16":"Q4_K_M","<24":"Q5_K_M","default":"Q8_0"}`)
	modelQuant := get("MDEMG_MODEL_QUANT", "auto")
	adapterBase := get("MDEMG_ADAPTER_BASE", "qwen3:14b")
	modelDir := get("MDEMG_MODEL_DIR", filepath.Join(homeDir, ".mdemg", "models"))
	ollamaModelsRoot := get("OLLAMA_MODELS", filepath.Join(homeDir, ".ollama", "models"))
	ollamaRegistryHost := get("OLLAMA_HOST", "registry.ollama.ai")
	modelManifestPath := get("MDEMG_MODEL_MANIFEST_PATH", "")
	embTargetDims, err := atoi("EMBEDDING_TARGET_DIMS", 0)
	if err != nil {
		return Config{}, err
	}

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
	logLevel := get("LOG_LEVEL", "info")
	switch logLevel {
	case "debug", "info", "warn", "error":
		// valid
	default:
		return Config{}, errors.New("LOG_LEVEL must be 'debug', 'info', 'warn', or 'error'")
	}

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
	nodeEmbeddingCacheTTLSec, err := atoi("NODE_EMBEDDING_CACHE_TTL_SEC", 3600)
	if err != nil {
		return Config{}, err
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
	rerankMinBudgetMs, err := atoi("RERANK_MIN_BUDGET_MS", 12000)
	if err != nil {
		return Config{}, err
	}
	if rerankMinBudgetMs > 0 && rerankMinBudgetMs < 3000 {
		rerankMinBudgetMs = 3000
	}
	neuralRerankMinBudgetMs, err := atoi("NEURAL_RERANK_MIN_BUDGET_MS", 1500)
	if err != nil {
		return Config{}, err
	}
	if neuralRerankMinBudgetMs > 0 && neuralRerankMinBudgetMs < 500 {
		neuralRerankMinBudgetMs = 500
	}
	rerankJinaKey := get("RERANK_JINA_API_KEY", "")
	rerankJinaModel := get("RERANK_JINA_MODEL", "jina-reranker-v2-base-multilingual")
	rerankJinaURL := get("RERANK_JINA_URL", "https://api.jina.ai/v1")
	rerankCompress := getBool("RERANK_COMPRESS", true)

	// Plugin system settings (V0006)
	pluginsEnabled := getBool("PLUGINS_ENABLED", true)
	pluginsDir := get("PLUGINS_DIR", "./plugins")
	pluginSocketDir := get("PLUGIN_SOCKET_DIR", "/tmp/mdemg-plugins")
	mdemgVersion := get("MDEMG_VERSION", "")
	mdemgCommit := get("MDEMG_COMMIT", "")

	// Linear integration settings (Phase 4)
	linearTeamID := get("LINEAR_TEAM_ID", "")

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
	synthesisCompress := getBool("SYNTHESIS_COMPRESS", true)

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
	intentTimeoutMs, err := atoi("INTENT_TIMEOUT_MS", 15000)
	if err != nil {
		return Config{}, err
	}
	if intentTimeoutMs < 200 {
		return Config{}, errors.New("INTENT_TIMEOUT_MS must be >= 200")
	}

	// Query Classification settings (PROD-READINESS)
	queryClassifyEnabled := getBool("QUERY_CLASSIFY_ENABLED", false)
	queryClassifyProvider := get("QUERY_CLASSIFY_PROVIDER", llmProvider)
	queryClassifyModel := get("QUERY_CLASSIFY_MODEL", llmModel)
	queryClassifyMaxTokens, err := atoi("QUERY_CLASSIFY_MAX_TOKENS", 500)
	if err != nil {
		return Config{}, err
	}
	if queryClassifyMaxTokens < 10 || queryClassifyMaxTokens > 1000 {
		return Config{}, errors.New("QUERY_CLASSIFY_MAX_TOKENS must be in range [10, 1000]")
	}
	queryClassifyTimeoutMs, err := atoi("QUERY_CLASSIFY_TIMEOUT_MS", 5000)
	if err != nil {
		return Config{}, err
	}
	if queryClassifyTimeoutMs < 200 {
		return Config{}, errors.New("QUERY_CLASSIFY_TIMEOUT_MS must be >= 200")
	}

	// Dynamic Emergence settings (Phase 103)
	emergenceEnabled := getBool("EMERGENCE_ENABLED", false)
	hiddenThemeIdentitySimThreshold, err := atof("HIDDEN_THEME_IDENTITY_SIM_THRESHOLD", 0.90)
	if err != nil {
		return Config{}, err
	}
	hiddenPatternIdentitySimThreshold, err := atof("HIDDEN_PATTERN_IDENTITY_SIM_THRESHOLD", 0.90)
	if err != nil {
		return Config{}, err
	}
	hiddenPatternMemberJaccardThreshold, err := atof("HIDDEN_PATTERN_MEMBER_JACCARD_THRESHOLD", 0.5)
	if err != nil {
		return Config{}, err
	}
	hiddenIncrementalAssignSimThreshold, err := atof("HIDDEN_INCREMENTAL_ASSIGN_SIM_THRESHOLD", 0.80)
	if err != nil {
		return Config{}, err
	}
	consolidateTimeoutMs, err := atoi("CONSOLIDATE_TIMEOUT_MS", 3600000)
	if err != nil {
		return Config{}, err
	}
	if consolidateTimeoutMs < 60000 {
		consolidateTimeoutMs = 60000
	}
	hiddenThemeTargetRatio, err := atof("HIDDEN_THEME_TARGET_RATIO", 0.1)
	if err != nil {
		return Config{}, err
	}
	hiddenThemeAssignSimThreshold, err := atof("HIDDEN_THEME_ASSIGN_SIM_THRESHOLD", 0.70)
	if err != nil {
		return Config{}, err
	}
	conversationCoverageAlertFloor, err := atof("CONVERSATION_COVERAGE_ALERT_FLOOR", 0.2)
	if err != nil {
		return Config{}, err
	}
	conversationCoverageMinObs, err := atoi("CONVERSATION_COVERAGE_MIN_OBS", 50)
	if err != nil {
		return Config{}, err
	}
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
	guardrailCompress := getBool("GUARDRAIL_COMPRESS", true)
	guardrailProducerEnabled := getBool("GUARDRAIL_PRODUCER_ENABLED", false)
	guardrailProducerMaxConcurrent, err := atoi("GUARDRAIL_PRODUCER_MAX_CONCURRENT", 1)
	if err != nil {
		return Config{}, err
	}
	if guardrailProducerMaxConcurrent < 1 {
		return Config{}, errors.New("GUARDRAIL_PRODUCER_MAX_CONCURRENT must be >= 1")
	}
	guardrailIncludeCorrections := getBool("GUARDRAIL_INCLUDE_CORRECTIONS", false)

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
	reclassModel := get("RECLASS_MODEL", "gpt-5.4-mini")
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
	jiminyContradictedWeakenEnabled := getBool("JIMINY_CONTRADICTED_WEAKEN_ENABLED", true)
	jiminyTimeoutMs, err := atoi("JIMINY_TIMEOUT_MS", 0)
	if err != nil {
		return Config{}, err
	}
	jiminyMaxItems, err := atoi("JIMINY_MAX_ITEMS", 10)
	if err != nil {
		return Config{}, err
	}
	// JIMINY-ACTIONABILITY-001 — surfacing levers (all default-preserving).
	jiminySurfaceActionableWeight, err := atof("JIMINY_SURFACE_ACTIONABLE_WEIGHT", 1.0)
	if err != nil {
		return Config{}, err
	}
	if jiminySurfaceActionableWeight <= 0 {
		jiminySurfaceActionableWeight = 1.0 // floor >0
	}
	jiminySurfaceMinActionable, err := atoi("JIMINY_SURFACE_MIN_ACTIONABLE", 0)
	if err != nil {
		return Config{}, err
	}
	if jiminySurfaceMinActionable < 0 {
		jiminySurfaceMinActionable = 0
	}
	jiminySurfaceMinActionableFraction, err := atof("JIMINY_SURFACE_MIN_ACTIONABLE_FRACTION", 0.0)
	if err != nil {
		return Config{}, err
	}
	if jiminySurfaceMinActionableFraction < 0 || jiminySurfaceMinActionableFraction > 1 {
		jiminySurfaceMinActionableFraction = 0.0
	}
	jiminySurfaceMaxAbstractionFraction, err := atof("JIMINY_SURFACE_MAX_ABSTRACTION_FRACTION", 1.0)
	if err != nil {
		return Config{}, err
	}
	if jiminySurfaceMaxAbstractionFraction <= 0 || jiminySurfaceMaxAbstractionFraction > 1 {
		jiminySurfaceMaxAbstractionFraction = 1.0
	}
	// JIMINY-CORPUS-001 Epic 3 — repetition control (Lever A cooldown + Lever B effectiveness prior).
	jiminySurfaceCooldownIgnoredCount, err := atoi("JIMINY_SURFACE_COOLDOWN_IGNORED_COUNT", 3)
	if err != nil {
		return Config{}, err
	}
	if jiminySurfaceCooldownIgnoredCount < 0 {
		jiminySurfaceCooldownIgnoredCount = 0 // negative = disabled
	}
	jiminySurfaceCooldownCapacity, err := atoi("JIMINY_SURFACE_COOLDOWN_CAPACITY", 5000)
	if err != nil {
		return Config{}, err
	}
	if jiminySurfaceCooldownCapacity <= 0 {
		jiminySurfaceCooldownCapacity = 5000 // must stay bounded and usable
	}
	jiminySurfaceEffectivenessPriorWeight, err := atof("JIMINY_SURFACE_EFFECTIVENESS_PRIOR_WEIGHT", 0.3)
	if err != nil {
		return Config{}, err
	}
	if jiminySurfaceEffectivenessPriorWeight < 0 {
		jiminySurfaceEffectivenessPriorWeight = 0 // negative = disabled
	}
	if jiminySurfaceEffectivenessPriorWeight > 1 {
		jiminySurfaceEffectivenessPriorWeight = 1 // full-prior ceiling
	}
	jiminySurfaceEffectivenessPriorTTLSec, err := atoi("JIMINY_SURFACE_EFFECTIVENESS_PRIOR_TTL_SEC", 300)
	if err != nil {
		return Config{}, err
	}
	if jiminySurfaceEffectivenessPriorTTLSec <= 0 {
		jiminySurfaceEffectivenessPriorTTLSec = 300
	}
	jiminySurfaceEffectivenessPriorMinSamples, err := atoi("JIMINY_SURFACE_EFFECTIVENESS_PRIOR_MIN_SAMPLES", 5)
	if err != nil {
		return Config{}, err
	}
	if jiminySurfaceEffectivenessPriorMinSamples < 1 {
		jiminySurfaceEffectivenessPriorMinSamples = 1
	}
	jiminyDirectiveSynthesisEnabled := getBool("JIMINY_DIRECTIVE_SYNTHESIS_ENABLED", false)
	jiminyDirectiveSynthesisMaxPromptTokens, err := atoi("JIMINY_DIRECTIVE_SYNTHESIS_MAX_PROMPT_TOKENS", 3500)
	if err != nil {
		return Config{}, err
	}
	if jiminyDirectiveSynthesisMaxPromptTokens < 1000 {
		jiminyDirectiveSynthesisMaxPromptTokens = 1000 // floor
	}
	jiminyGuidanceConstraintIncludeTopK, err := atoi("JIMINY_GUIDANCE_CONSTRAINT_INCLUDE_TOPK", 5)
	if err != nil {
		return Config{}, err
	}
	if jiminyGuidanceConstraintIncludeTopK < 0 {
		jiminyGuidanceConstraintIncludeTopK = 0
	}
	jiminyGuidanceConstraintSimFloor, err := atof("JIMINY_GUIDANCE_CONSTRAINT_SIM_FLOOR", 0.30)
	if err != nil {
		return Config{}, err
	}
	jiminyMinConfidence, err := atof("JIMINY_MIN_CONFIDENCE", 0.3)
	if err != nil {
		return Config{}, err
	}
	jiminyConstraintCodeSimThreshold, err := atof("JIMINY_CONSTRAINT_CODE_SIM_THRESHOLD", DefaultJiminyConstraintCodeSimThreshold)
	if err != nil {
		return Config{}, err
	}
	jiminySignalStrengthWeight, err := atof("JIMINY_SIGNAL_STRENGTH_WEIGHT", 0.2)
	if err != nil {
		return Config{}, err
	}
	jiminyIncludeFrontiers := getBool("JIMINY_INCLUDE_FRONTIERS", true)
	jiminyFrontierMinSim, err := atof("JIMINY_FRONTIER_MIN_SIM", 0.5)
	if err != nil {
		return Config{}, err
	}
	jiminyEffectivenessTTLSec, err := atoi("JIMINY_EFFECTIVENESS_TTL_SEC", 86400)
	if err != nil {
		return Config{}, err
	}

	// Jiminy Warm Store (event-driven pre-computation)
	jiminyWarmEnabled := getBool("JIMINY_WARM_ENABLED", true)
	jiminyWarmComputeTimeoutMs, err := atoi("JIMINY_WARM_COMPUTE_TIMEOUT_MS", DefaultJiminyWarmComputeTimeoutMs)
	if err != nil {
		return Config{}, err
	}
	jiminyFeedbackTimeoutMs, err := atoi("JIMINY_FEEDBACK_TIMEOUT_MS", 60000)
	if err != nil {
		return Config{}, err
	}
	jiminyReformulateTimeoutMs, err := atoi("JIMINY_REFORMULATE_TIMEOUT_MS", 0)
	if err != nil {
		return Config{}, err
	}
	jiminyWarmDebounceSec, err := atoi("JIMINY_WARM_DEBOUNCE_SEC", 10)
	if err != nil {
		return Config{}, err
	}
	jiminyWarmMaxAgeSec, err := atoi("JIMINY_WARM_MAX_AGE_SEC", 300)
	if err != nil {
		return Config{}, err
	}

	// Jiminy J7-J12: Cognitive Guidance System extensions
	jiminyRetrievalEnabled := getBool("JIMINY_RETRIEVAL_ENABLED", true)
	jiminyRetrievalTopK, err := atoi("JIMINY_RETRIEVAL_TOP_K", 10)
	if err != nil {
		return Config{}, err
	}
	jiminyRetrievalHopDepth, err := atoi("JIMINY_RETRIEVAL_HOP_DEPTH", 2)
	if err != nil {
		return Config{}, err
	}
	jiminySynthesisEnabled := getBool("JIMINY_SYNTHESIS_ENABLED", true) // J15: default changed from false to true
	jiminySynthesisProvider := get("JIMINY_SYNTHESIS_PROVIDER", llmProvider)
	jiminySynthesisModel := get("JIMINY_SYNTHESIS_MODEL", llmModel)
	jiminySynthesisMaxTokens, err := atoi("JIMINY_SYNTHESIS_MAX_TOKENS", 3000) // J15: default changed from 1000 to 2000
	if err != nil {
		return Config{}, err
	}
	jiminySynthesisTimeoutMs, err := atoi("JIMINY_SYNTHESIS_TIMEOUT_MS", 30000) // J16: default changed from 10000 to 30000
	if err != nil {
		return Config{}, err
	}
	jiminyEvaluateEnabled := getBool("JIMINY_EVALUATE_ENABLED", true)
	jiminyEvaluateTimeoutMs, err := atoi("JIMINY_EVALUATE_TIMEOUT_MS", 30000)
	if err != nil {
		return Config{}, err
	}
	jiminyEvaluateMaxConstraints, err := atoi("JIMINY_EVALUATE_MAX_CONSTRAINTS", 10)
	if err != nil {
		return Config{}, err
	}
	// J13: Evaluator LLM config
	jiminyEvaluateLLMEnabled := getBool("JIMINY_EVALUATE_LLM_ENABLED", false)
	jiminyEvaluateLLMProvider := get("JIMINY_EVALUATE_LLM_PROVIDER", llmProvider)
	jiminyEvaluateLLMModel := get("JIMINY_EVALUATE_LLM_MODEL", llmModel)
	jiminyEvaluateLLMTimeoutMs, err := atoi("JIMINY_EVALUATE_LLM_TIMEOUT_MS", 30000)
	if err != nil {
		return Config{}, err
	}
	jiminyEvaluateLLMMaxTokens, err := atoi("JIMINY_EVALUATE_LLM_MAX_TOKENS", 3000)
	if err != nil {
		return Config{}, err
	}
	jiminyOutcomeClassifierEnabled := getBool("JIMINY_OUTCOME_CLASSIFIER_ENABLED", true)
	jiminyOutcomeLLMEnabled := getBool("JIMINY_OUTCOME_LLM_ENABLED", true)
	jiminyOutcomeSimilarityHigh, err := atof("JIMINY_OUTCOME_SIMILARITY_HIGH", 0.55)
	if err != nil {
		return Config{}, err
	}
	jiminyOutcomeSimilarityLow, err := atof("JIMINY_OUTCOME_SIMILARITY_LOW", 0.20)
	if err != nil {
		return Config{}, err
	}
	// JIMINY-CORPUS-001 Epic 4: relevance gate. Below this similarity the guidance
	// topic was unrelated to the action — the outcome is not_applicable, not ignored.
	// The [gate, LOW) band stays a real "ignored" (relevant domain, not followed).
	// Default 0.10 = half the LOW relevance floor: with the production embedding
	// model, unrelated-domain pairs cluster near 0 while pairs sharing topical
	// vocabulary sit ≥ ~0.15, so 0.10 carves only the clearly-unrelated tail.
	// Must be ≤ LOW (classifier clamps + Validate warns); ≤ 0 disables the gate
	// (the entire sub-LOW tail is then not_applicable — JIMINY-OUTCOME-002 behavior).
	jiminyOutcomeNotApplicableSim, err := atof("JIMINY_OUTCOME_NOT_APPLICABLE_SIMILARITY", 0.10)
	if err != nil {
		return Config{}, err
	}
	// J14: Outcome classifier LLM config
	jiminyOutcomeLLMMaxTokens, err := atoi("JIMINY_OUTCOME_LLM_MAX_TOKENS", 3000)
	if err != nil {
		return Config{}, err
	}
	jiminyOutcomeCacheSize, err := atoi("JIMINY_OUTCOME_CACHE_SIZE", 256)
	if err != nil {
		return Config{}, err
	}
	jiminyClassifyCompress := getBool("JIMINY_CLASSIFY_COMPRESS", true)
	// JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001: default OFF; operator flips
	// after running the 3-day A/B recipe to verify the predicted lift.
	jiminyNonViolationCreditEnabled := getBool("JIMINY_NONVIOLATION_CREDIT_ENABLED", false)
	jiminyDedupSimilarityThreshold, err := atof("JIMINY_DEDUP_SIMILARITY_THRESHOLD", 0.85)
	if err != nil {
		return Config{}, err
	}
	jiminyCorrectionDecayRate, err := atof("JIMINY_CORRECTION_DECAY_RATE", 0.01)
	if err != nil {
		return Config{}, err
	}
	j17TicketCacheSize, err := atoi("J17_TICKET_CACHE_SIZE", 1000)
	if err != nil {
		return Config{}, err
	}
	// J15: Synthesis temperature
	var jiminySynthesisTemperature *float64
	if v := os.Getenv("JIMINY_SYNTHESIS_TEMPERATURE"); v != "" {
		t, tErr := strconv.ParseFloat(v, 64)
		if tErr != nil {
			return Config{}, fmt.Errorf("JIMINY_SYNTHESIS_TEMPERATURE: %w", tErr)
		}
		jiminySynthesisTemperature = &t
	}
	// J16: Input context limits (0 = unlimited)
	jiminyGuidanceContextMaxChars, err := atoi("JIMINY_GUIDANCE_CONTEXT_MAX_CHARS", 200000)
	if err != nil {
		return Config{}, err
	}
	jiminyGuidanceOutputMaxChars, err := atoi("JIMINY_GUIDANCE_OUTPUT_MAX_CHARS", 200000)
	if err != nil {
		return Config{}, err
	}
	jiminyEvaluateOutputMaxChars, err := atoi("JIMINY_EVALUATE_OUTPUT_MAX_CHARS", 200000)
	if err != nil {
		return Config{}, err
	}
	jiminyEvaluateItemMaxChars, err := atoi("JIMINY_EVALUATE_ITEM_MAX_CHARS", 0)
	if err != nil {
		return Config{}, err
	}

	jiminyEscalationEnabled := getBool("JIMINY_ESCALATION_ENABLED", true)
	jiminyEscalationWarnAfter, err := atoi("JIMINY_ESCALATION_WARN_AFTER", 2)
	if err != nil {
		return Config{}, err
	}
	jiminyEscalationEscalateAfter, err := atoi("JIMINY_ESCALATION_ESCALATE_AFTER", 4)
	if err != nil {
		return Config{}, err
	}
	jiminyEscalationBlockAfter, err := atoi("JIMINY_ESCALATION_BLOCK_AFTER", 6)
	if err != nil {
		return Config{}, err
	}
	jiminyEscalationBlockEnabled := getBool("JIMINY_ESCALATION_BLOCK_ENABLED", false)
	jiminyEscalationDecayMinutes, err := atoi("JIMINY_ESCALATION_DECAY_MINUTES", 60)
	if err != nil {
		return Config{}, err
	}
	jiminyEscalationPersistEnabled := getBool("JIMINY_ESCALATION_PERSIST_ENABLED", true)
	jiminyStrictStatePath := get("JIMINY_STRICT_STATE_PATH", "")
	if jiminyStrictStatePath == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			jiminyStrictStatePath = home + "/.mdemg/.jiminy-strict-mode"
		}
	}
	jiminyCodeRegenEnabled := getBool("JIMINY_CODE_REGEN_ENABLED", false)
	jiminyCodeRegenThreshold, err := atof("JIMINY_CODE_REGEN_THRESHOLD", 0.3)
	if err != nil {
		return Config{}, err
	}
	jiminyCodeRegenMinSamples, err := atoi("JIMINY_CODE_REGEN_MIN_SAMPLES", 10)
	if err != nil {
		return Config{}, err
	}

	// J17: AI-to-AI Communication Protocol
	j17Enabled := getBool("J17_ENABLED", true)
	j17TicketSecret := get("J17_TICKET_SECRET", "")
	j17TicketTTLHours, err := atoi("J17_TICKET_TTL_HOURS", 4)
	if err != nil {
		return Config{}, err
	}
	j17SequenceBufferSize, err := atoi("J17_SEQUENCE_BUFFER_SIZE", 1000)
	if err != nil {
		return Config{}, err
	}
	j17ReplayMaxEvents, err := atoi("J17_REPLAY_MAX_EVENTS", 50)
	if err != nil {
		return Config{}, err
	}
	j17BootstrapEnabled := getBool("J17_BOOTSTRAP_ENABLED", true)

	// J17-2: Three-Tier Encoding
	j17CodegenEnabled := getBool("J17_CODEGEN_ENABLED", j17Enabled)
	j17CodegenProvider := get("J17_CODEGEN_PROVIDER", jiminySynthesisProvider)
	j17CodegenModel := get("J17_CODEGEN_MODEL", jiminySynthesisModel)
	j17DefaultTier, err := atoi("J17_DEFAULT_TIER", 1)
	if err != nil {
		return Config{}, err
	}

	// J17-3: Trust Score
	j17TrustInitial, err := atof("J17_TRUST_INITIAL", 0.65)
	if err != nil {
		return Config{}, err
	}
	j17TrustBoostPerFollow, err := atof("J17_TRUST_BOOST_PER_FOLLOW", 0.05)
	if err != nil {
		return Config{}, err
	}
	j17TrustDecayPerIgnore, err := atof("J17_TRUST_DECAY_PER_IGNORE", 0.02)
	if err != nil {
		return Config{}, err
	}
	j17TrustDecayPerContradict, err := atof("J17_TRUST_DECAY_PER_CONTRADICT", 0.04)
	if err != nil {
		return Config{}, err
	}
	j17TrustHighThreshold, err := atof("J17_TRUST_HIGH_THRESHOLD", 0.75)
	if err != nil {
		return Config{}, err
	}
	j17TrustLowThreshold, err := atof("J17_TRUST_LOW_THRESHOLD", 0.35)
	if err != nil {
		return Config{}, err
	}
	j17TrustTTLHours, err := atoi("J17_TRUST_TTL_HOURS", 168)
	if err != nil {
		return Config{}, err
	}
	j17TrustMinFeedbackCount, err := atoi("J17_TRUST_MIN_FEEDBACK_COUNT", 5)
	if err != nil {
		return Config{}, err
	}
	j17TrustMode := get("J17_TRUST_MODE", "ema")
	if j17TrustMode != "ema" && j17TrustMode != "ratchet" {
		return Config{}, fmt.Errorf("J17_TRUST_MODE must be 'ema' or 'ratchet'")
	}
	j17TrustEMAAlpha, err := atof("J17_TRUST_EMA_ALPHA", 0.1)
	if err != nil {
		return Config{}, err
	}
	if j17TrustEMAAlpha <= 0 || j17TrustEMAAlpha > 1 {
		return Config{}, fmt.Errorf("J17_TRUST_EMA_ALPHA must be in (0, 1]")
	}
	j17BootstrapCodification := getBool("J17_BOOTSTRAP_CODIFICATION", j17Enabled)
	j17BootstrapSpaceID := get("J17_BOOTSTRAP_SPACE_ID", "mdemg-dev")

	// J17-TIER-GATE-001: comprehension-keyed T1 promotion.
	j17TierGateMode := get("J17_TIER_GATE_MODE", "trust")
	if j17TierGateMode != "trust" && j17TierGateMode != "comprehension" {
		slog.Warn("J17_TIER_GATE_MODE invalid, falling back to trust", "requested", j17TierGateMode)
		j17TierGateMode = "trust"
	}
	j17TierComprehensionHigh, err := atof("J17_TIER_COMPREHENSION_HIGH", 0.6)
	if err != nil {
		return Config{}, err
	}
	if j17TierComprehensionHigh <= 0 || j17TierComprehensionHigh > 1 {
		slog.Warn("J17_TIER_COMPREHENSION_HIGH out of (0,1], falling back to default", "requested", j17TierComprehensionHigh, "default", 0.6)
		j17TierComprehensionHigh = 0.6
	}
	j17TierComprehensionMinSamples, err := atoi("J17_TIER_COMPREHENSION_MIN_SAMPLES", 20)
	if err != nil {
		return Config{}, err
	}
	if j17TierComprehensionMinSamples < 1 {
		j17TierComprehensionMinSamples = 1
	}

	// J17-4: Protocol Metrics + RSIC Evolution
	j17MetricsEnabled := getBool("J17_METRICS_ENABLED", j17Enabled)
	j17CodificationThreshold, err := atoi("J17_CODIFICATION_THRESHOLD", 30)
	if err != nil {
		return Config{}, err
	}
	j17ComprehensionMinThreshold, err := atof("J17_COMPREHENSION_MIN_THRESHOLD", 0.7)
	if err != nil {
		return Config{}, err
	}
	j17CompressionMinRatio, err := atof("J17_COMPRESSION_MIN_RATIO", 2.0)
	if err != nil {
		return Config{}, err
	}
	j17ReplayFrequencyMax, err := atof("J17_REPLAY_FREQUENCY_MAX", 5.0)
	if err != nil {
		return Config{}, err
	}
	j17NLIComprehensionEnabled := getBool("J17_NLI_COMPREHENSION_ENABLED", false)
	j17ProtocolDataCollection := getBool("J17_PROTOCOL_DATA_COLLECTION", false)
	j17T1ComprehensionGate, err := atof("J17_T1_COMPREHENSION_GATE", 0.5)
	if err != nil {
		return Config{}, err
	}

	// J17-5: Extensions + System-Wide Encoding + ML Tier Selection
	j17ExtensionsEnabled := getBool("J17_EXTENSIONS_ENABLED", j17Enabled)
	j17AllowedExtensionsStr := get("J17_ALLOWED_EXTENSIONS", "tier_preference,abbreviated_ids,batch_mode,density_boost")
	var j17AllowedExtensions []string
	for _, ext := range strings.Split(j17AllowedExtensionsStr, ",") {
		ext = strings.TrimSpace(ext)
		if ext != "" {
			j17AllowedExtensions = append(j17AllowedExtensions, ext)
		}
	}
	j17MLTierPredictionEnabled := getBool("J17_ML_TIER_PREDICTION_ENABLED", false)
	j17SidecarURL := get("J17_SIDECAR_URL", "")
	// Default raised 200→1000ms (DH-004 E3): 200ms was too tight under typical
	// sidecar latency, causing ~56% NLI fallback rate observed in production.
	// 1000ms is a ceiling, not a floor — healthy calls remain fast.
	j17SidecarTimeoutMs, err := atoi("J17_SIDECAR_TIMEOUT_MS", 1000)
	if err != nil {
		return Config{}, err
	}
	// Enforce a minimum of 100ms — values below are almost certainly misconfigurations.
	if j17SidecarTimeoutMs < 100 {
		slog.Warn("J17_SIDECAR_TIMEOUT_MS below 100ms floor, clamping to 100",
			"requested_ms", j17SidecarTimeoutMs)
		j17SidecarTimeoutMs = 100
	}

	// J17-NS: Neural Sidecar Promotion
	j17SidecarMode := get("J17_SIDECAR_MODE", "shadow")
	// Validate mode
	switch j17SidecarMode {
	case "shadow", "compare", "canary", "active":
		// valid
	default:
		return Config{}, fmt.Errorf("J17_SIDECAR_MODE must be one of: shadow, compare, canary, active (got %q)", j17SidecarMode)
	}
	j17SidecarCanaryPct, err := atoi("J17_SIDECAR_CANARY_PERCENTAGE", 100)
	if err != nil {
		return Config{}, err
	}
	j17SidecarConfidenceFloor, err := atof("J17_SIDECAR_CONFIDENCE_FLOOR", 0.6)
	if err != nil {
		return Config{}, err
	}
	j17NLIScoreOfRecord := getBool("J17_NLI_SCORE_OF_RECORD", false)
	j17PrecedentProtectedCodes := get("J17_PRECEDENT_PROTECTED_CODES", "")
	j17PrecedentLogEnabled := getBool("J17_PRECEDENT_LOG_ENABLED", true)

	// J17-NS: Sidecar Circuit Breaker
	j17SidecarCBEnabled := getBool("J17_SIDECAR_CB_ENABLED", true)
	j17SidecarCBFailureThreshold, err := atoi("J17_SIDECAR_CB_FAILURE_THRESHOLD", 3)
	if err != nil {
		return Config{}, err
	}
	j17SidecarCBTimeoutSec, err := atoi("J17_SIDECAR_CB_TIMEOUT_SEC", 15)
	if err != nil {
		return Config{}, err
	}

	// NLI Feedback Loop
	j17NLIObservationalEnabled := getBool("J17_NLI_OBSERVATIONAL_ENABLED", true)
	j17TierEffectivenessMinSamples, err := atoi("J17_TIER_EFFECTIVENESS_MIN_SAMPLES", 5)
	if err != nil {
		return Config{}, err
	}
	j17TierIneffectiveThreshold, err := atof("J17_TIER_INEFFECTIVE_THRESHOLD", 0.6)
	if err != nil {
		return Config{}, err
	}
	j17TierDriftDetectionEnabled := getBool("J17_TIER_DRIFT_DETECTION_ENABLED", true)
	j17NLICalibrationWindowSize, err := atoi("J17_NLI_CALIBRATION_WINDOW_SIZE", 500)
	if err != nil {
		return Config{}, err
	}
	j17NLICalibrationBiasThreshold, err := atof("J17_NLI_CALIBRATION_BIAS_THRESHOLD", 0.15)
	if err != nil {
		return Config{}, err
	}
	// DASHBOARD-TRUTH-001: min-sample floor for the calibration bias alert —
	// a tiny (e.g. 16-sample, restart-reset) window must not fire the alert.
	j17NLICalibrationMinSamples, err := atoi("J17_NLI_CALIBRATION_MIN_SAMPLES", 50)
	if err != nil {
		return Config{}, err
	}
	if j17NLICalibrationMinSamples < 0 {
		j17NLICalibrationMinSamples = 0
	}
	// DASHBOARD-TRUTH-001 Epic 3: calibration anchor for the RSIC Protocol
	// compression sub-score (score 1.0 at target, 0.0 at ratio 1.0, linear
	// between). Must be > 1.0 — a target at or below 1.0 (no compression)
	// makes the sub-score degenerate.
	//
	// DASHBOARD-TRUTH-002 E3 (2026-07-20): default recalibrated 3.0→2.0.
	// The DASHBOARD-TRUTH-001 pick of 3.0 was defensively-excellent but
	// above the observed 30d live p95 of 2.0 — the sub-score was hitting
	// ~0.435 on realistic compression. The new 2.0 default matches the
	// achievable ceiling; operators may raise it to demand more
	// compression once the T1 tier is reachable (currently 0% live —
	// blocked by trust EMA per DASHBOARD-TRUTH-002 triage §J17).
	j17CompressionTargetRatio, err := atof("J17_COMPRESSION_TARGET_RATIO", 2.0)
	if err != nil {
		return Config{}, err
	}
	if j17CompressionTargetRatio <= 1.0 {
		slog.Warn("J17_COMPRESSION_TARGET_RATIO must be > 1.0, falling back to default",
			"requested", j17CompressionTargetRatio, "default", 2.0)
		j17CompressionTargetRatio = 2.0
	}

	// FT-BENCH-REFRESH-001: alert when benchmark_runs is stale (no
	// completed run in the last N days). Default 7d matches a weekly
	// operator cadence; ≤0 treated as 7.
	ftBenchStalenessDays, err := atoi("FT_BENCH_STALENESS_DAYS", 7)
	if err != nil {
		return Config{}, err
	}
	if ftBenchStalenessDays <= 0 {
		slog.Warn("FT_BENCH_STALENESS_DAYS must be > 0, falling back to default",
			"requested", ftBenchStalenessDays, "default", 7)
		ftBenchStalenessDays = 7
	}

	// DASHBOARD-TRUTH-002 E2: entropy floor for the RSIC Edge dimension.
	// Live-calibrated at 0.2 (below the observed healthy mdemg-dev value
	// 0.27). Set to 0 to disable the penalty entirely.
	rsicEdgeEntropyFloor, err := atof("RSIC_EDGE_ENTROPY_FLOOR", 0.2)
	if err != nil {
		return Config{}, err
	}
	if rsicEdgeEntropyFloor < 0 {
		slog.Warn("RSIC_EDGE_ENTROPY_FLOOR must be >= 0, falling back to default",
			"requested", rsicEdgeEntropyFloor, "default", 0.2)
		rsicEdgeEntropyFloor = 0.2
	}

	// Startup validation: active mode requires sidecar URL
	if j17SidecarMode == "active" && j17SidecarURL == "" {
		return Config{}, errors.New("J17_SIDECAR_MODE=active requires J17_SIDECAR_URL to be set")
	}
	if j17SidecarMode != "shadow" && j17SidecarURL == "" {
		slog.Warn("J17_SIDECAR_URL is empty, sidecar calls will fail", "j17_sidecar_mode", j17SidecarMode)
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
	rsicMicroEnabled := getBool("RSIC_MICRO_ENABLED", true)
	rsicMesoPeriodHours, err := atoi("RSIC_MESO_PERIOD_HOURS", 6)
	if err != nil {
		return Config{}, err
	}
	rsicMesoPeriodSessions, err := atoi("RSIC_MESO_PERIOD_SESSIONS", 10)
	if err != nil {
		return Config{}, err
	}
	rsicMacroCron := get("RSIC_MACRO_CRON", "0 3 * * *")
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
	rsicGuidanceEffectivenessWindowHours, err := atoi("RSIC_GUIDANCE_EFFECTIVENESS_WINDOW_HOURS", 168)
	if err != nil {
		return Config{}, err
	}
	if rsicGuidanceEffectivenessWindowHours < 1 || rsicGuidanceEffectivenessWindowHours > 720 {
		return Config{}, fmt.Errorf("RSIC_GUIDANCE_EFFECTIVENESS_WINDOW_HOURS must be in range [1, 720]")
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
	rsicProtectedSpacesStr := get("RSIC_PROTECTED_SPACES", "")
	var rsicProtectedSpaces []string
	if rsicProtectedSpacesStr != "" {
		for _, s := range strings.Split(rsicProtectedSpacesStr, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				rsicProtectedSpaces = append(rsicProtectedSpaces, s)
			}
		}
	}
	rsicMacroCronSpace := get("RSIC_MACRO_CRON_SPACE", "mdemg-dev")
	rsicMinActionConfidence, err := atof("RSIC_MIN_ACTION_CONFIDENCE", 0.2)
	if err != nil {
		return Config{}, err
	}

	// RSIC-SK1: Guidance self-calibration
	rsicGuidanceCalibrationEnabled := getBool("RSIC_GUIDANCE_CALIBRATION_ENABLED", true)
	rsicGuidanceMinSurfaces, err := atoi("RSIC_GUIDANCE_MIN_SURFACES", 3)
	if err != nil {
		return Config{}, err
	}
	rsicGuidanceBoostThreshold, err := atof("RSIC_GUIDANCE_BOOST_THRESHOLD", 0.7)
	if err != nil {
		return Config{}, err
	}
	rsicGuidanceDecayThreshold, err := atof("RSIC_GUIDANCE_DECAY_THRESHOLD", 0.1)
	if err != nil {
		return Config{}, err
	}
	rsicGuidanceDecayMinSurfaces, err := atoi("RSIC_GUIDANCE_DECAY_MIN_SURFACES", 5)
	if err != nil {
		return Config{}, err
	}

	spacePruneIntervalHours, err := atoi("SPACE_PRUNE_INTERVAL_HOURS", 24)
	if err != nil {
		return Config{}, err
	}
	contextCoolerEnabled := getBool("CONTEXT_COOLER_ENABLED", false)
	weeklyGapInterviewsEnabled := getBool("WEEKLY_GAP_INTERVIEWS_ENABLED", false)

	// Phase AR-3: LLM-powered intelligence
	rsicLLMReflectEnabled := getBool("RSIC_LLM_REFLECT_ENABLED", true)
	rsicLLMReflectProvider := get("RSIC_LLM_REFLECT_PROVIDER", emergenceProvider)
	rsicLLMReflectModel := get("RSIC_LLM_REFLECT_MODEL", emergenceModel)
	rsicLLMReflectCompress := getBool("RSIC_LLM_REFLECT_COMPRESS", true)
	rsicLLMReflectTimeoutMs, err := atoi("RSIC_LLM_REFLECT_TIMEOUT_MS", 15000)
	if err != nil {
		return Config{}, err
	}
	if rsicLLMReflectTimeoutMs < 5000 {
		return Config{}, fmt.Errorf("RSIC_LLM_REFLECT_TIMEOUT_MS must be >= 5000")
	}

	// APE-PROMPT-BUDGET-001: prompt-budget + history-cap controls.
	rsicLLMReflectPromptBudgetTokens, err := atoi("RSIC_LLM_REFLECT_PROMPT_BUDGET_TOKENS", 3500)
	if err != nil {
		return Config{}, err
	}
	if rsicLLMReflectPromptBudgetTokens != 0 && (rsicLLMReflectPromptBudgetTokens < 1000 || rsicLLMReflectPromptBudgetTokens > 7000) {
		return Config{}, fmt.Errorf("RSIC_LLM_REFLECT_PROMPT_BUDGET_TOKENS must be 0 (disabled) or in range [1000, 7000]")
	}
	rsicLLMReflectHistoryCycles, err := atoi("RSIC_LLM_REFLECT_HISTORY_CYCLES", 3)
	if err != nil {
		return Config{}, err
	}
	if rsicLLMReflectHistoryCycles < 0 || rsicLLMReflectHistoryCycles > 10 {
		return Config{}, fmt.Errorf("RSIC_LLM_REFLECT_HISTORY_CYCLES must be in range [0, 10]")
	}
	rsicLLMReflectIncludeDatasets := getBool("RSIC_LLM_REFLECT_INCLUDE_DATASETS", false)

	rsicLLMConcurrencyLimit, err := atoi("RSIC_LLM_CONCURRENCY_LIMIT", 2)
	if err != nil {
		return Config{}, err
	}
	if rsicLLMConcurrencyLimit < 1 || rsicLLMConcurrencyLimit > 8 {
		return Config{}, fmt.Errorf("RSIC_LLM_CONCURRENCY_LIMIT must be between 1 and 8 (got %d)", rsicLLMConcurrencyLimit)
	}

	conflictTrackerEnabled := getBool("CONFLICT_TRACKER_ENABLED", true)

	// Phase 11.6.3 — MLX Watchdog
	// Phase 13.6 — backend-agnostic env-var rename. Primary names are
	// LLM_*; legacy MLX_* names accepted as aliases with a deprecation log
	// at startup. Drop aliases ≥1 release cycle after rename ships.
	getBoolWithAlias := func(primary, alias string, def bool) bool {
		if v := strings.TrimSpace(os.Getenv(primary)); v != "" {
			return v == "true" || v == "1" || v == "yes" || v == "on"
		}
		if v := strings.TrimSpace(os.Getenv(alias)); v != "" {
			slog.Warn("config: env var deprecated, please rename", "deprecated", alias, "primary", primary)
			return v == "true" || v == "1" || v == "yes" || v == "on"
		}
		return def
	}
	atoiWithAlias := func(primary, alias string, def int) (int, error) {
		v := strings.TrimSpace(os.Getenv(primary))
		if v == "" {
			if alt := strings.TrimSpace(os.Getenv(alias)); alt != "" {
				slog.Warn("config: env var deprecated, please rename", "deprecated", alias, "primary", primary)
				v = alt
			}
		}
		if v == "" {
			return def, nil
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("%s must be int: %w", primary, err)
		}
		return n, nil
	}
	mlxWatchdogEnabled := getBoolWithAlias("LLM_WATCHDOG_ENABLED", "MLX_WATCHDOG_ENABLED", true)
	mlxProbeIntervalSec, err := atoiWithAlias("LLM_PROBE_INTERVAL_SEC", "MLX_PROBE_INTERVAL_SEC", 5)
	if err != nil {
		return Config{}, err
	}
	if mlxProbeIntervalSec < 1 {
		return Config{}, fmt.Errorf("LLM_PROBE_INTERVAL_SEC must be ≥ 1 (got %d)", mlxProbeIntervalSec)
	}
	mlxProbeTimeoutSec, err := atoiWithAlias("LLM_PROBE_TIMEOUT_SEC", "MLX_PROBE_TIMEOUT_SEC", 2)
	if err != nil {
		return Config{}, err
	}
	if mlxProbeTimeoutSec < 1 {
		return Config{}, fmt.Errorf("LLM_PROBE_TIMEOUT_SEC must be ≥ 1 (got %d)", mlxProbeTimeoutSec)
	}
	if mlxProbeTimeoutSec >= mlxProbeIntervalSec {
		return Config{}, fmt.Errorf("MLX_PROBE_TIMEOUT_SEC (%d) must be < MLX_PROBE_INTERVAL_SEC (%d) — overlap risks ticker stalls",
			mlxProbeTimeoutSec, mlxProbeIntervalSec)
	}
	mlxFailFastEnabled := getBoolWithAlias("LLM_FAIL_FAST_ENABLED", "MLX_FAIL_FAST_ENABLED", true)

	// Phase 13 — Column-Voting Retrieval
	// Phase 13.1 (2026-05-03): default flipped false → true after embedding-heavy
	// preset (weights below) passed full 120q UVTS A/B against legacy linear
	// baseline: mean +0.023 (+5.9%), 30 improvements, 2 boundary regressions
	// at exactly -0.10 in business_logic_constraints. See
	// docs/development/post-ft-lora/phase_13_1_post.md.
	retrievalColumnVotingEnabled := getBool("RETRIEVAL_COLUMN_VOTING_ENABLED", true)
	retrievalGraphTypedEdgesEnabled := getBool("RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED", true)
	retrievalRRFK, err := atoi("RETRIEVAL_RRF_K", 60)
	if err != nil {
		return Config{}, err
	}
	if retrievalRRFK < 1 {
		return Config{}, fmt.Errorf("RETRIEVAL_RRF_K must be ≥ 1 (got %d)", retrievalRRFK)
	}
	retrievalColumnTimeoutFrac, err := atof("RETRIEVAL_COLUMN_TIMEOUT_FRACTION", 0.8)
	if err != nil {
		return Config{}, err
	}
	if retrievalColumnTimeoutFrac <= 0 || retrievalColumnTimeoutFrac > 1 {
		return Config{}, fmt.Errorf("RETRIEVAL_COLUMN_TIMEOUT_FRACTION must be in (0, 1] (got %v)", retrievalColumnTimeoutFrac)
	}
	retrievalStructuralHops, err := atoi("RETRIEVAL_STRUCTURAL_HOPS", 2)
	if err != nil {
		return Config{}, err
	}
	if retrievalStructuralHops < 1 || retrievalStructuralHops > 9 {
		return Config{}, fmt.Errorf("RETRIEVAL_STRUCTURAL_HOPS must be in [1, 9] (got %d)", retrievalStructuralHops)
	}
	retrievalColEmbeddingEnabled := getBool("RETRIEVAL_COLUMN_EMBEDDING_ENABLED", true)
	retrievalColBM25Enabled := getBool("RETRIEVAL_COLUMN_BM25_ENABLED", true)
	retrievalColGraphEnabled := getBool("RETRIEVAL_COLUMN_GRAPH_ENABLED", true)
	retrievalColStructuralEnabled := getBool("RETRIEVAL_COLUMN_STRUCTURAL_ENABLED", true)

	// Phase 13.1 — per-column RRF weights (ablation knobs). 1.0 default per
	// column matches Phase 13's nil ColumnWeights map → equal weights → no
	// behavior change unless operator opts in. Negative values are rejected
	// to mirror the consensus.ColumnWeight contract.
	// Phase 13.1 default weights from the embedding-heavy preset (winner of
	// the Phase 13.1 ablation sweep). Diagnostic finding: equal weights
	// (1.0/1.0/1.0/1.0) caused Graph+Structural to over-vote on
	// structurally-connected code, crowding out Embedding+BM25's better
	// matches for precise-symbol queries. Embedding-heavy
	// (0.50/0.20/0.15/0.15) corrects the imbalance.
	retrievalColumnWeightEmbedding, err := atof("RETRIEVAL_COLUMN_WEIGHT_EMBEDDING", 0.50)
	if err != nil {
		return Config{}, err
	}
	if retrievalColumnWeightEmbedding < 0 {
		return Config{}, fmt.Errorf("RETRIEVAL_COLUMN_WEIGHT_EMBEDDING must be ≥ 0 (got %v)", retrievalColumnWeightEmbedding)
	}
	retrievalColumnWeightBM25, err := atof("RETRIEVAL_COLUMN_WEIGHT_BM25", 0.20)
	if err != nil {
		return Config{}, err
	}
	if retrievalColumnWeightBM25 < 0 {
		return Config{}, fmt.Errorf("RETRIEVAL_COLUMN_WEIGHT_BM25 must be ≥ 0 (got %v)", retrievalColumnWeightBM25)
	}
	retrievalColumnWeightGraph, err := atof("RETRIEVAL_COLUMN_WEIGHT_GRAPH", 0.15)
	if err != nil {
		return Config{}, err
	}
	if retrievalColumnWeightGraph < 0 {
		return Config{}, fmt.Errorf("RETRIEVAL_COLUMN_WEIGHT_GRAPH must be ≥ 0 (got %v)", retrievalColumnWeightGraph)
	}
	retrievalColumnWeightStructural, err := atof("RETRIEVAL_COLUMN_WEIGHT_STRUCTURAL", 0.15)
	if err != nil {
		return Config{}, err
	}
	if retrievalColumnWeightStructural < 0 {
		return Config{}, fmt.Errorf("RETRIEVAL_COLUMN_WEIGHT_STRUCTURAL must be ≥ 0 (got %v)", retrievalColumnWeightStructural)
	}

	// Phase 13 Epic 5 — Downstream consumer wiring (flag-off, prompts/inputs deferred to Phase 14)

	// Phase 13 Epic 6 — retrieval_audit (V0017) write toggle
	retrievalAuditEnabled := getBool("RETRIEVAL_AUDIT_ENABLED", true)

	// JIMINY-RELEVANCE-001 Epic 1 — guidance_training_rows (V0027).
	guidanceCorpusEnabled := getBool("GUIDANCE_CORPUS_ENABLED", true)
	guidanceCorpusWriterFlushIntervalSec, err := atoi("GUIDANCE_CORPUS_WRITER_FLUSH_INTERVAL_SEC", 30)
	if err != nil {
		return Config{}, err
	}
	if guidanceCorpusWriterFlushIntervalSec < 5 {
		guidanceCorpusWriterFlushIntervalSec = 5 // floor — never thrash the DB
	}
	guidanceCorpusWriterBufferSize, err := atoi("GUIDANCE_CORPUS_WRITER_BUFFER_SIZE", 1000)
	if err != nil {
		return Config{}, err
	}
	if guidanceCorpusWriterBufferSize < 0 {
		guidanceCorpusWriterBufferSize = 0 // 0 = unlimited (disabled cap)
	}

	// JIMINY-CONTRADICTED-BRIDGE-001
	jiminyContradictedBridgeEnabled := getBool("JIMINY_CONTRADICTED_BRIDGE_ENABLED", true)
	jiminyContradictedBridgeWriterFlushIntervalSec, err := atoi("JIMINY_CONTRADICTED_BRIDGE_WRITER_FLUSH_INTERVAL_SEC", 30)
	if err != nil {
		return Config{}, err
	}
	if jiminyContradictedBridgeWriterFlushIntervalSec < 5 {
		jiminyContradictedBridgeWriterFlushIntervalSec = 5
	}
	jiminyContradictedBridgeWriterBufferSize, err := atoi("JIMINY_CONTRADICTED_BRIDGE_WRITER_BUFFER_SIZE", 1000)
	if err != nil {
		return Config{}, err
	}
	if jiminyContradictedBridgeWriterBufferSize < 0 {
		jiminyContradictedBridgeWriterBufferSize = 0
	}
	jiminyContradictedBridgeMaxContentLen, err := atoi("JIMINY_CONTRADICTED_BRIDGE_MAX_CONTENT_LEN", 400)
	if err != nil {
		return Config{}, err
	}
	if jiminyContradictedBridgeMaxContentLen < 50 {
		jiminyContradictedBridgeMaxContentLen = 50
	}
	guidanceCorpusMaxContentBytes, err := atoi("GUIDANCE_CORPUS_MAX_CONTENT_BYTES", 8192)
	if err != nil {
		return Config{}, err
	}
	if guidanceCorpusMaxContentBytes < 256 {
		guidanceCorpusMaxContentBytes = 256 // floor — keep enough to be useful evidence
	}
	guidanceCorpusSourceLookupTimeoutMs, err := atoi("GUIDANCE_CORPUS_SOURCE_LOOKUP_TIMEOUT_MS", 300)
	if err != nil {
		return Config{}, err
	}
	if guidanceCorpusSourceLookupTimeoutMs < 0 {
		guidanceCorpusSourceLookupTimeoutMs = 0 // 0 = disable inline lookup
	}
	guidanceAuditEnabled := getBool("GUIDANCE_AUDIT_ENABLED", true)
	guidanceAuditIntervalHours, err := atoi("GUIDANCE_AUDIT_INTERVAL_HOURS", 24)
	if err != nil {
		return Config{}, err
	}
	if guidanceAuditIntervalHours < 1 {
		guidanceAuditIntervalHours = 1 // floor
	}
	guidanceAuditSampleSize, err := atoi("GUIDANCE_AUDIT_SAMPLE_SIZE", 50)
	if err != nil {
		return Config{}, err
	}
	if guidanceAuditSampleSize < 1 {
		guidanceAuditSampleSize = 1 // floor
	}
	guidanceAuditInitialDelaySec, err := atoi("GUIDANCE_AUDIT_INITIAL_DELAY_SEC", 60)
	if err != nil {
		return Config{}, err
	}
	if guidanceAuditInitialDelaySec < 0 {
		guidanceAuditInitialDelaySec = 0 // 0 = skip the initial run
	}
	// JIMINY-ACTIONABILITY-INVERSION-001 verdict: real actionable rate under
	// Lever C over-surfacing has an expected steady state around 0.10-0.14
	// (live 168h: 0.1137), rising to ~0.20 once the non-violation-credit flag
	// is enabled. The alert is `rate < floor`, so the floor must sit BELOW
	// steady state to fire only on genuine collapse — 0.05 per the fix_spec
	// recommendation. (An interim 0.15 default shipped briefly in
	// JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 E6 with inverted reasoning
	// and would have kept the alert chronically firing; corrected same-day.)
	guidanceShouldFollowRateFloor, err := atof("GUIDANCE_SHOULD_FOLLOW_RATE_FLOOR", 0.05)
	if err != nil {
		return Config{}, err
	}
	if guidanceShouldFollowRateFloor < 0 || guidanceShouldFollowRateFloor > 1 {
		guidanceShouldFollowRateFloor = 0.05 // out-of-range → default
	}
	guidanceShouldFollowLookbackHours, err := atoi("GUIDANCE_SHOULD_FOLLOW_LOOKBACK_HOURS", 168)
	if err != nil {
		return Config{}, err
	}
	if guidanceShouldFollowLookbackHours < 1 {
		guidanceShouldFollowLookbackHours = 1 // floor
	}

	// HITL-REVIEW-001 — review platform config.
	reviewEnabled := getBool("REVIEW_ENABLED", true)
	reviewWriterFlushIntervalSec, err := atoi("REVIEW_WRITER_FLUSH_INTERVAL_SEC", 15)
	if err != nil {
		return Config{}, err
	}
	if reviewWriterFlushIntervalSec < 5 {
		reviewWriterFlushIntervalSec = 5 // floor
	}
	reviewWriterBufferSize, err := atoi("REVIEW_WRITER_BUFFER_SIZE", 500)
	if err != nil {
		return Config{}, err
	}
	if reviewWriterBufferSize < 0 {
		reviewWriterBufferSize = 0 // 0 = unbounded
	}
	reviewRubricVersion := get("REVIEW_RUBRIC_VERSION", "gr-v1")
	reviewSampleSize, err := atoi("REVIEW_SAMPLE_SIZE", 200)
	if err != nil {
		return Config{}, err
	}
	if reviewSampleSize < 1 {
		reviewSampleSize = 1 // floor
	}
	reviewActiveUncertaintyBand, err := atof("REVIEW_ACTIVE_UNCERTAINTY_BAND", 0.4)
	if err != nil {
		return Config{}, err
	}
	if reviewActiveUncertaintyBand < 0 || reviewActiveUncertaintyBand > 1 {
		reviewActiveUncertaintyBand = 0.4 // out-of-range → default
	}
	reviewSampleSeedInt, err := atoi("REVIEW_SAMPLE_SEED", 0)
	if err != nil {
		return Config{}, err
	}
	reviewSampleSeed := int64(reviewSampleSeedInt)
	reviewReinforceDefault := getBool("REVIEW_REINFORCE_DEFAULT", false)
	reviewGuidanceSinkEnabled := getBool("REVIEW_GUIDANCE_SINK_ENABLED", true)
	reviewLLMDatasetsEnabled := getBool("REVIEW_LLM_DATASETS_ENABLED", true)
	reviewContradictedDatasetEnabled := getBool("REVIEW_CONTRADICTED_DATASET_ENABLED", true)
	reviewGuidanceConfidenceNudge, err := atof("REVIEW_GUIDANCE_CONFIDENCE_NUDGE", 0.05)
	if err != nil {
		return Config{}, err
	}

	// EVENTGRAPH-001 — TSDB reinforcement_events + federation API (Pattern Y1).
	eventGraphEnabled := getBool("EVENTGRAPH_ENABLED", true)
	eventGraphWriterFlushIntervalSec, err := atoi("EVENTGRAPH_WRITER_FLUSH_INTERVAL_SEC", 30)
	if err != nil {
		return Config{}, err
	}
	if eventGraphWriterFlushIntervalSec < 5 {
		eventGraphWriterFlushIntervalSec = 5
	}
	eventGraphWriterBufferSize, err := atoi("EVENTGRAPH_WRITER_BUFFER_SIZE", 1000)
	if err != nil {
		return Config{}, err
	}
	eventGraphMaxPairsPerEventBatch, err := atoi("EVENTGRAPH_MAX_PAIRS_PER_EVENT_BATCH", 200)
	if err != nil {
		return Config{}, err
	}
	eventGraphMaxEventsPerQuery, err := atoi("EVENTGRAPH_MAX_EVENTS_PER_QUERY", 500)
	if err != nil {
		return Config{}, err
	}
	eventGraphFederationDefaultHops, err := atoi("EVENTGRAPH_FEDERATION_DEFAULT_HOPS", 2)
	if err != nil {
		return Config{}, err
	}
	eventGraphFederationDefaultLookbackHours, err := atoi("EVENTGRAPH_FEDERATION_DEFAULT_LOOKBACK_HOURS", 24)
	if err != nil {
		return Config{}, err
	}

	// Phase 14 Epic 1 → Phase 14.1.1 — Note 06 sparse activation gate
	// defaults. Phase 14 Epic 0 forensic set p95 + within-call clamp shape.
	// Phase 14.1.1 hybrid 120q PASSED (mean +0.003, 0 regressions, 10
	// improvements) at MIN=15 global + data_flow_integration MIN=20
	// category override. Defaults flipped to that hybrid configuration:
	//   SPARSE_RETRIEVAL_ENABLED=true (was false in Phase 14)
	//   SPARSE_MIN_ACTIVE=15 (was 3 in Phase 14)
	//   SPARSE_GATE_CATEGORY_OVERRIDES seeded with data_flow_integration MIN=20
	// Operator opt-out: SPARSE_RETRIEVAL_ENABLED=false in .env + restart.
	sparseRetrievalEnabled := getBool("SPARSE_RETRIEVAL_ENABLED", true)
	sparseActivationPercentile, err := atof("SPARSE_ACTIVATION_PERCENTILE", 0.95)
	if err != nil {
		return Config{}, err
	}
	if sparseActivationPercentile < 0.5 || sparseActivationPercentile > 0.999 {
		return Config{}, fmt.Errorf("SPARSE_ACTIVATION_PERCENTILE must be in [0.5, 0.999] (got %v)", sparseActivationPercentile)
	}
	sparseMinActive, err := atoi("SPARSE_MIN_ACTIVE", 15)
	if err != nil {
		return Config{}, err
	}
	if sparseMinActive < 0 {
		return Config{}, fmt.Errorf("SPARSE_MIN_ACTIVE must be ≥ 0 (got %d)", sparseMinActive)
	}
	sparseMaxActive, err := atoi("SPARSE_MAX_ACTIVE", 20)
	if err != nil {
		return Config{}, err
	}
	if sparseMaxActive < sparseMinActive {
		return Config{}, fmt.Errorf("SPARSE_MAX_ACTIVE (%d) must be ≥ SPARSE_MIN_ACTIVE (%d)", sparseMaxActive, sparseMinActive)
	}

	// Phase 14.2 Epic 1+2 — Note 05 sparse fingerprints + adaptive catalog.
	// All flag-off initially; flipped per Epic 6 A/B verdict. Builder
	// defaults match Phase 14.2 Epic 0 forensic recommendations.
	// Phase 14.2.3 (2026-05-06) — default flipped false → true after the
	// per-category context-column weight retune passed 120q full A/B
	// (mean +0.009, std -0.023, 11 improvements, 0 regressions).
	contextFingerprintEnabled := getBool("CONTEXT_FINGERPRINT_ENABLED", true)
	contextFingerprintBitBudget, err := atoi("CONTEXT_FINGERPRINT_BIT_BUDGET", 256)
	if err != nil {
		return Config{}, err
	}
	if contextFingerprintBitBudget < 32 || contextFingerprintBitBudget > 1024 {
		return Config{}, fmt.Errorf("CONTEXT_FINGERPRINT_BIT_BUDGET must be in [32, 1024] (got %d)", contextFingerprintBitBudget)
	}
	// Phase 14.2.3 (2026-05-06) — default flipped false → true alongside
	// CONTEXT_FINGERPRINT_ENABLED + RETRIEVAL_CONTEXT_COLUMN_ENABLED. The
	// macro-cycle stage 6 hook now runs Builder.BuildForSpace +
	// fingerprint-refresh batch on the configured interval (default 168h
	// weekly), keeping production catalogs current with the corpus.
	// Operator opt-out: CONTEXT_FINGERPRINT_REFRESH_ENABLED=false.
	contextFingerprintRefreshEnabled := getBool("CONTEXT_FINGERPRINT_REFRESH_ENABLED", true)
	contextFingerprintRefreshIntervalHours, err := atoi("CONTEXT_FINGERPRINT_REFRESH_INTERVAL_HOURS", 168)
	if err != nil {
		return Config{}, err
	}
	if contextFingerprintRefreshIntervalHours < 1 {
		return Config{}, fmt.Errorf("CONTEXT_FINGERPRINT_REFRESH_INTERVAL_HOURS must be ≥ 1 (got %d)", contextFingerprintRefreshIntervalHours)
	}
	contextFingerprintRefreshTimeoutMs, err := atoi("CONTEXT_FINGERPRINT_REFRESH_TIMEOUT_MS", 60000)
	if err != nil {
		return Config{}, err
	}
	contextFingerprintHealMaxPerCycle, err := atoi("CONTEXT_FINGERPRINT_HEAL_MAX_PER_CYCLE", 2000)
	if err != nil {
		return Config{}, err
	}
	contextFingerprintRefineMaxPerCycle, err := atoi("CONTEXT_FINGERPRINT_REFINE_MAX_PER_CYCLE", 200)
	if err != nil {
		return Config{}, err
	}
	queryClassifyCategoryMap := map[string]string{
		"data_flow":    "data_flow_integration",
		"architecture": "architecture_structure",
		"relationship": "relationship",
	}
	if raw := get("QUERY_CLASSIFY_CATEGORY_MAP", ""); raw != "" {
		parsed := map[string]string{}
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			return Config{}, fmt.Errorf("QUERY_CLASSIFY_CATEGORY_MAP must be a JSON string map: %w", err)
		}
		queryClassifyCategoryMap = parsed
	}
	contextQueryAutoDefault := getBool("CONTEXT_QUERY_AUTO_DEFAULT", true)
	if contextFingerprintRefreshTimeoutMs < 1000 {
		return Config{}, fmt.Errorf("CONTEXT_FINGERPRINT_REFRESH_TIMEOUT_MS must be ≥ 1000 (got %d)", contextFingerprintRefreshTimeoutMs)
	}
	contextCatalogTopNPaths, err := atoi("CONTEXT_CATALOG_TOP_N_PATHS", 192)
	if err != nil {
		return Config{}, err
	}
	contextCatalogTopNTags, err := atoi("CONTEXT_CATALOG_TOP_N_TAGS", 32)
	if err != nil {
		return Config{}, err
	}
	contextCatalogFloorBitsPerKind, err := atoi("CONTEXT_CATALOG_FLOOR_BITS_PER_KIND", 16)
	if err != nil {
		return Config{}, err
	}
	contextCatalogRoleTypeLayerBits, err := atoi("CONTEXT_CATALOG_ROLE_TYPE_LAYER_BITS", 32)
	if err != nil {
		return Config{}, err
	}
	if contextCatalogTopNPaths+contextCatalogTopNTags+contextCatalogRoleTypeLayerBits > contextFingerprintBitBudget {
		return Config{}, fmt.Errorf("CONTEXT_CATALOG_TOP_N_PATHS (%d) + TOP_N_TAGS (%d) + ROLE_TYPE_LAYER_BITS (%d) must not exceed BIT_BUDGET (%d)",
			contextCatalogTopNPaths, contextCatalogTopNTags, contextCatalogRoleTypeLayerBits, contextFingerprintBitBudget)
	}
	// Phase 14.2.3 (2026-05-06) — default flipped false → true after the
	// per-category context-column weight retune passed 120q full A/B.
	// Operator opt-out: RETRIEVAL_CONTEXT_COLUMN_ENABLED=false.
	retrievalContextColumnEnabled := getBool("RETRIEVAL_CONTEXT_COLUMN_ENABLED", true)
	retrievalContextColumnWeight, err := atof("RETRIEVAL_CONTEXT_COLUMN_WEIGHT", 0.10)
	if err != nil {
		return Config{}, err
	}
	if retrievalContextColumnWeight < 0 {
		return Config{}, fmt.Errorf("RETRIEVAL_CONTEXT_COLUMN_WEIGHT must be ≥ 0 (got %v)", retrievalContextColumnWeight)
	}
	retrievalContextStrictThreshold, err := atof("RETRIEVAL_CTX_STRICT_THRESHOLD", 0.25)
	if err != nil {
		return Config{}, err
	}
	if retrievalContextStrictThreshold < 0 || retrievalContextStrictThreshold > 1 {
		return Config{}, fmt.Errorf("RETRIEVAL_CTX_STRICT_THRESHOLD must be in [0, 1] (got %v)", retrievalContextStrictThreshold)
	}
	// Phase 14.2.1 — vector-based query→fingerprint top-K
	contextFingerprintQueryTopK, err := atoi("CONTEXT_FINGERPRINT_QUERY_TOPK", 8)
	if err != nil {
		return Config{}, err
	}
	if contextFingerprintQueryTopK < 1 || contextFingerprintQueryTopK > 64 {
		return Config{}, fmt.Errorf("CONTEXT_FINGERPRINT_QUERY_TOPK must be in [1, 64] (got %d)", contextFingerprintQueryTopK)
	}

	// Phase 14.2.3 — per-category context-column weight overrides.
	// Default seed reflects the Phase 14.2.2 120q forensic: zero-weight
	// the column on the 3 categories that consistently regressed.
	// Operator-supplied JSON REPLACES the default seed entirely.
	retrievalContextColumnCategoryWeights := map[string]float64{
		"service_relationships":      0.0,
		"business_logic_constraints": 0.0,
		"relationship":               0.0,
	}
	if raw := strings.TrimSpace(os.Getenv("RETRIEVAL_CONTEXT_COLUMN_CATEGORY_WEIGHTS")); raw != "" {
		retrievalContextColumnCategoryWeights = map[string]float64{}
		if err := json.Unmarshal([]byte(raw), &retrievalContextColumnCategoryWeights); err != nil {
			return Config{}, fmt.Errorf("RETRIEVAL_CONTEXT_COLUMN_CATEGORY_WEIGHTS must be valid JSON: %w", err)
		}
		for cat, w := range retrievalContextColumnCategoryWeights {
			if cat == "" {
				return Config{}, fmt.Errorf("RETRIEVAL_CONTEXT_COLUMN_CATEGORY_WEIGHTS: empty category key")
			}
			if w < 0 || w > 10 {
				return Config{}, fmt.Errorf("RETRIEVAL_CONTEXT_COLUMN_CATEGORY_WEIGHTS[%q] must be in [0, 10] (got %v)", cat, w)
			}
		}
	}

	// Phase 14.1 Epic 1 → Phase 14.1.1 — Per-category gate overrides.
	// Seeded by default with the Phase 14.1.1 hybrid winner override:
	//   data_flow_integration: MIN=20 (handles 4-required_files queries
	//   like q302 — system-wide data-flow walks where the gate at MIN=15
	//   would cut a needed citation).
	// Operator may extend or override the map via SPARSE_GATE_CATEGORY_OVERRIDES
	// JSON env (env value REPLACES the default map; merge isn't supported).
	sparseGateDataFlowMin := 20
	sparseGateCategoryOverrides := map[string]SparseGateOverride{
		"data_flow_integration": {MinActive: &sparseGateDataFlowMin},
	}
	if raw := strings.TrimSpace(os.Getenv("SPARSE_GATE_CATEGORY_OVERRIDES")); raw != "" {
		// Operator-supplied map replaces the default seed entirely.
		sparseGateCategoryOverrides = map[string]SparseGateOverride{}
		if err := json.Unmarshal([]byte(raw), &sparseGateCategoryOverrides); err != nil {
			return Config{}, fmt.Errorf("SPARSE_GATE_CATEGORY_OVERRIDES must be valid JSON: %w", err)
		}
		for cat, ov := range sparseGateCategoryOverrides {
			if cat == "" {
				return Config{}, fmt.Errorf("SPARSE_GATE_CATEGORY_OVERRIDES: empty category key")
			}
			if ov.MinActive != nil && *ov.MinActive < 0 {
				return Config{}, fmt.Errorf("SPARSE_GATE_CATEGORY_OVERRIDES[%s].min_active must be ≥ 0 (got %d)", cat, *ov.MinActive)
			}
			if ov.MaxActive != nil && *ov.MaxActive < 0 {
				return Config{}, fmt.Errorf("SPARSE_GATE_CATEGORY_OVERRIDES[%s].max_active must be ≥ 0 (got %d)", cat, *ov.MaxActive)
			}
			if ov.MinActive != nil && ov.MaxActive != nil && *ov.MaxActive < *ov.MinActive {
				return Config{}, fmt.Errorf("SPARSE_GATE_CATEGORY_OVERRIDES[%s]: max_active (%d) must be ≥ min_active (%d)", cat, *ov.MaxActive, *ov.MinActive)
			}
			if ov.Percentile != nil && (*ov.Percentile < 0.5 || *ov.Percentile > 0.999) {
				return Config{}, fmt.Errorf("SPARSE_GATE_CATEGORY_OVERRIDES[%s].percentile must be in [0.5, 0.999] (got %v)", cat, *ov.Percentile)
			}
		}
	}

	consultingLLMConstraintsEnabled := getBool("CONSULTING_LLM_CONSTRAINTS_ENABLED", false)
	consultingLLMConstraintsProvider := get("CONSULTING_LLM_CONSTRAINTS_PROVIDER", emergenceProvider)
	consultingLLMConstraintsModel := get("CONSULTING_LLM_CONSTRAINTS_MODEL", emergenceModel)
	consultingClassifyTimeoutMs, err := atoi("CONSULTING_CLASSIFY_TIMEOUT_MS", 30000)
	if err != nil {
		return Config{}, err
	}
	if consultingClassifyTimeoutMs < 5000 {
		return Config{}, fmt.Errorf("CONSULTING_CLASSIFY_TIMEOUT_MS must be >= 5000")
	}
	consultingClassifyConcurrency, err := atoi("CONSULTING_CLASSIFY_CONCURRENCY", DefaultConsultingClassifyConcurrency)
	if err != nil {
		return Config{}, err
	}
	if consultingClassifyConcurrency < 1 {
		consultingClassifyConcurrency = 1
	}

	// RRF-SCALE-001 — RRF-calibrated consulting score gates + confidence sigmoid.
	consultingConstraintScoreFloor, err := atof("CONSULTING_CONSTRAINT_SCORE_FLOOR", DefaultConsultingConstraintScoreFloor)
	if err != nil {
		return Config{}, err
	}
	consultingSuggestMinConfidence, err := atof("CONSULTING_SUGGEST_MIN_CONFIDENCE", DefaultConsultingSuggestMinConfidence)
	if err != nil {
		return Config{}, err
	}
	mcpReflectScoreHigh, err := atof("MCP_REFLECT_SCORE_HIGH", DefaultMCPReflectScoreHigh)
	if err != nil {
		return Config{}, err
	}
	mcpReflectScoreMedium, err := atof("MCP_REFLECT_SCORE_MEDIUM", DefaultMCPReflectScoreMedium)
	if err != nil {
		return Config{}, err
	}
	guardrailConstraintSimFloor, err := atof("GUARDRAIL_CONSTRAINT_SIM_FLOOR", DefaultGuardrailConstraintSimFloor)
	if err != nil {
		return Config{}, err
	}
	consultingAuthorityScoreFloor, err := atof("CONSULTING_AUTHORITY_SCORE_FLOOR", DefaultConsultingAuthorityScoreFloor)
	if err != nil {
		return Config{}, err
	}
	consultingConflictScoreFloor, err := atof("CONSULTING_CONFLICT_SCORE_FLOOR", DefaultConsultingConflictScoreFloor)
	if err != nil {
		return Config{}, err
	}
	retrievalConfidenceSigmoidMidpoint, err := atof("RETRIEVAL_CONFIDENCE_SIGMOID_MIDPOINT", DefaultRetrievalConfidenceSigmoidMidpoint)
	if err != nil {
		return Config{}, err
	}
	retrievalConfidenceSigmoidSteepness, err := atof("RETRIEVAL_CONFIDENCE_SIGMOID_STEEPNESS", DefaultRetrievalConfidenceSigmoidSteepness)
	if err != nil {
		return Config{}, err
	}

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
	coolerTombstoneMaxPerRun, err := atoi("COOLER_TOMBSTONE_MAX_PER_RUN", 500)
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

	// JIMINY-CORPUS-001: ConvObs→constraint promotion gate
	constraintPromotionGateEnabled := getBool("CONSTRAINT_PROMOTION_GATE_ENABLED", true)
	var constraintPromotionDenyObsTypes []string
	for _, t := range strings.Split(get("CONSTRAINT_PROMOTION_DENY_OBS_TYPES", DefaultConstraintPromotionDenyObsTypes), ",") {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			constraintPromotionDenyObsTypes = append(constraintPromotionDenyObsTypes, t)
		}
	}
	constraintPromotionRejectPatterns := DefaultConstraintPromotionRejectPatterns()
	if raw := strings.TrimSpace(os.Getenv("CONSTRAINT_PROMOTION_REJECT_PATTERNS")); raw != "" {
		var parsed []string
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			return Config{}, fmt.Errorf("CONSTRAINT_PROMOTION_REJECT_PATTERNS must be a valid JSON string array: %w", err)
		}
		constraintPromotionRejectPatterns = parsed
	}
	for _, p := range constraintPromotionRejectPatterns {
		if _, err := regexp.Compile(p); err != nil {
			return Config{}, fmt.Errorf("CONSTRAINT_PROMOTION_REJECT_PATTERNS: invalid regexp %q: %w", p, err)
		}
	}

	// JIMINY-CORRECTION-PRODUCER-001: L0 obs → correction promotion gate.
	correctionPromotionEnabled := getBool("CORRECTION_PROMOTION_ENABLED", true)
	correctionPromotionMinContentLen, err := atoi("CORRECTION_PROMOTION_MIN_CONTENT_LEN", 20)
	if err != nil {
		return Config{}, err
	}
	correctionPromotionRejectPatterns := DefaultConstraintPromotionRejectPatterns() // reuse the same junk-class defaults
	if raw := strings.TrimSpace(os.Getenv("CORRECTION_PROMOTION_REJECT_PATTERNS")); raw != "" {
		var parsed []string
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			return Config{}, fmt.Errorf("CORRECTION_PROMOTION_REJECT_PATTERNS must be a valid JSON string array: %w", err)
		}
		correctionPromotionRejectPatterns = parsed
	}
	for _, p := range correctionPromotionRejectPatterns {
		if _, err := regexp.Compile(p); err != nil {
			return Config{}, fmt.Errorf("CORRECTION_PROMOTION_REJECT_PATTERNS: invalid regexp %q: %w", p, err)
		}
	}

	graduatedDecayProtectionFactor, err := atof("GRADUATED_DECAY_PROTECTION_FACTOR", 0.5)
	if err != nil {
		return Config{}, err
	}

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
	// BACKUP-RESTORE-VERIFY-001: was 2 (comment/code drift vs the documented
	// 50) — below the size of one whole-database export, which made
	// retention delete every backup the moment it completed.
	backupRetentionMaxStorageGB, err := atoi("BACKUP_RETENTION_MAX_STORAGE_GB", 50)
	if err != nil {
		return Config{}, err
	}
	backupRetentionRunAfter := getBool("BACKUP_RETENTION_RUN_AFTER_BACKUP", true)
	// 3600 not 300: a whole-database export takes ~15 min on mdemg-dev;
	// the pre-restore snapshot and scheduled-run reporting both wait on it.
	backupSnapshotWaitTimeoutSec, err := atoi("BACKUP_SNAPSHOT_WAIT_TIMEOUT_SEC", 3600)
	if err != nil {
		return Config{}, err
	}
	backupJobStalenessHours, err := atoi("BACKUP_JOB_STALENESS_HOURS", 0)
	if err != nil {
		return Config{}, err
	}
	backupInitialDelayMin, err := atoi("BACKUP_INITIAL_DELAY_MIN", 5)
	if err != nil {
		return Config{}, err
	}

	// TimescaleDB Backup & Restore
	tsdbBackupEnabled := getBool("TSDB_BACKUP_ENABLED", false)
	tsdbBackupStorageDir := get("TSDB_BACKUP_STORAGE_DIR", ".mdemg/backups/tsdb")
	tsdbBackupComposeFile := get("TSDB_BACKUP_COMPOSE_FILE", "")
	tsdbBackupServiceName := get("TSDB_BACKUP_SERVICE", "timescaledb")
	tsdbBackupIntervalHours, err := atoi("TSDB_BACKUP_INTERVAL_HOURS", 24)
	if err != nil {
		return Config{}, err
	}
	tsdbBackupInitialDelayMin, err := atoi("TSDB_BACKUP_INITIAL_DELAY_MIN", 10)
	if err != nil {
		return Config{}, err
	}
	if tsdbBackupIntervalHours < 1 {
		return Config{}, errors.New("TSDB_BACKUP_INTERVAL_HOURS must be >= 1")
	}
	tsdbBackupRetentionCount, err := atoi("TSDB_BACKUP_RETENTION_COUNT", 14)
	if err != nil {
		return Config{}, err
	}
	tsdbBackupRetentionMaxAgeDays, err := atoi("TSDB_BACKUP_RETENTION_MAX_AGE_DAYS", 90)
	if err != nil {
		return Config{}, err
	}

	// Phase 75: Relationship Extraction
	relExtractImports := getBool("REL_EXTRACT_IMPORTS", true)
	relExtractInheritance := getBool("REL_EXTRACT_INHERITANCE", true)
	relExtractCalls := getBool("REL_EXTRACT_CALLS", true)
	relCrossFileResolve := getBool("REL_CROSS_FILE_RESOLVE", true)
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
	dynamicEdgeMinLayer, err := atoi("DYNAMIC_EDGE_MIN_LAYER", 1)
	if err != nil {
		return Config{}, err
	}
	dynEdgeIncLookback, err := atoi("DYNAMIC_EDGE_INCREMENTAL_LOOKBACK_HOURS", 6)
	if err != nil {
		return Config{}, err
	}
	if dynEdgeIncLookback < 0 {
		return Config{}, errors.New("DYNAMIC_EDGE_INCREMENTAL_LOOKBACK_HOURS must be >= 0")
	}
	dynamicEdgeTopK, err := atoi("DYNAMIC_EDGE_TOPK", 10)
	if err != nil {
		return Config{}, err
	}
	dynamicEdgeSimThreshold, err := atof("DYNAMIC_EDGE_SIM_THRESHOLD", 0.30)
	if err != nil {
		return Config{}, err
	}
	dynamicEdgeOversample, err := atoi("DYNAMIC_EDGE_OVERSAMPLE", 8)
	if err != nil {
		return Config{}, err
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
	edgeAttentionAnalogousTo, err := atof("EDGE_ATTENTION_ANALOGOUS_TO", 0.55)
	if err != nil {
		return Config{}, err
	}
	edgeAttentionBridges, err := atof("EDGE_ATTENTION_BRIDGES", 0.60)
	if err != nil {
		return Config{}, err
	}
	edgeAttentionComposesWith, err := atof("EDGE_ATTENTION_COMPOSES_WITH", 0.50)
	if err != nil {
		return Config{}, err
	}
	edgeAttentionContrastsWith, err := atof("EDGE_ATTENTION_CONTRASTS_WITH", 0.40)
	if err != nil {
		return Config{}, err
	}
	edgeAttentionInfluences, err := atof("EDGE_ATTENTION_INFLUENCES", 0.45)
	if err != nil {
		return Config{}, err
	}
	edgeAttentionDefinesSymbol, err := atof("EDGE_ATTENTION_DEFINES_SYMBOL", 0.70)
	if err != nil {
		return Config{}, err
	}
	edgeAttentionThemeOf, err := atof("EDGE_ATTENTION_THEME_OF", 0.65)
	if err != nil {
		return Config{}, err
	}

	// ===== ANN Optimization: Negative Feedback =====
	learningNegativeWeight, err := atof("LEARNING_NEGATIVE_WEIGHT", 0.15)
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
	apeIngestSyncEnabled := getBool("APE_INGEST_SYNC_ENABLED", false)
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

	// Metrics settings
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

	// ===== FSD-2026-001: Constraint Lifecycle Closure =====

	// F1: Guardrail Enforcement Hook

	// F2a: Contradiction Detection
	contradictionEnabled := getBool("CONTRADICTION_ENABLED", true)
	contradictionSimThreshold, err := atof("CONTRADICTION_SIM_THRESHOLD", 0.75)
	if err != nil {
		return Config{}, err
	}
	contradictionMaxCandidates, err := atoi("CONTRADICTION_MAX_CANDIDATES", 20)
	if err != nil {
		return Config{}, err
	}

	// F2b: NLI-Enhanced Contradiction
	contradictionNLIEnabled := getBool("CONTRADICTION_NLI_ENABLED", false)

	// F3: Effectiveness Feedback Persistence
	jiminyPersistenceEnabled := getBool("JIMINY_PERSISTENCE_ENABLED", true)
	constraintConfDecayPerNeg, err := atof("CONSTRAINT_CONFIDENCE_DECAY_PER_NEGATIVE", 0.03)
	if err != nil {
		return Config{}, err
	}
	constraintConfBoostPerPos, err := atof("CONSTRAINT_CONFIDENCE_BOOST_PER_POSITIVE", 0.02)
	if err != nil {
		return Config{}, err
	}
	constraintArchiveThreshold, err := atof("CONSTRAINT_ARCHIVE_THRESHOLD", 0.3)
	if err != nil {
		return Config{}, err
	}

	// F4: Cross-Constraint Conflict Detection
	constraintConflictDetectionEnabled := getBool("CONSTRAINT_CONFLICT_DETECTION_ENABLED", false)
	constraintConflictSimThreshold, err := atof("CONSTRAINT_CONFLICT_SIM_THRESHOLD", 0.6)
	if err != nil {
		return Config{}, err
	}
	constraintConflictMaxPairs, err := atoi("CONSTRAINT_CONFLICT_MAX_PAIRS", 500)
	if err != nil {
		return Config{}, err
	}

	// F6: Constraint Classifier Gate
	constraintClassifierGateEnabled := getBool("CONSTRAINT_CLASSIFIER_GATE_ENABLED", false)
	constraintNLIEnabled := getBool("CONSTRAINT_NLI_ENABLED", false)

	// F7: Constraint Scope Filtering
	constraintScopeFilteringEnabled := getBool("CONSTRAINT_SCOPE_FILTERING_ENABLED", false)

	// F9: Asymmetric Learning + Determinism Score
	learningAsymmetricEnabled := getBool("LEARNING_ASYMMETRIC_ENABLED", false)
	determinismScoringEnabled := getBool("DETERMINISM_SCORING_ENABLED", false)

	// F10: Jiminy Latency Optimization
	jiminyCacheEnabled := getBool("JIMINY_CACHE_ENABLED", true)
	jiminyCacheTTLSec, err := atoi("JIMINY_CACHE_TTL_SEC", 300)
	if err != nil {
		return Config{}, err
	}
	jiminyCacheSize, err := atoi("JIMINY_CACHE_SIZE", 200)
	if err != nil {
		return Config{}, err
	}
	jiminyCacheJ17Bypass := getBool("JIMINY_CACHE_J17_BYPASS", true)

	// F11: Configurable Activation Dimension Weights
	activationDimSemanticWeight, err := atof("ACTIVATION_DIM_SEMANTIC_WEIGHT", 0.6)
	if err != nil {
		return Config{}, err
	}
	activationDimTemporalWeight, err := atof("ACTIVATION_DIM_TEMPORAL_WEIGHT", 0.2)
	if err != nil {
		return Config{}, err
	}
	activationDimCoactivationWeight, err := atof("ACTIVATION_DIM_COACTIVATION_WEIGHT", 0.2)
	if err != nil {
		return Config{}, err
	}

	// F13: Constraint Decay/Expiry

	// F15: Configurable Hop Depth
	maxHopDepth, err := atoi("MAX_HOP_DEPTH", 3)
	if err != nil {
		return Config{}, err
	}

	// F18: Edge Limit Enforcement
	learningAutoPruneExcessEnabled := getBool("LEARNING_AUTO_PRUNE_EXCESS_ENABLED", false)

	// F20: Authority Levels
	constraintAuthorityEnabled := getBool("CONSTRAINT_AUTHORITY_ENABLED", false)
	constraintDefaultAuthority := get("CONSTRAINT_DEFAULT_AUTHORITY", "team_standard")

	// ===== Neural Re-Ranker =====

	// NR-1: Training Data Collection
	neuralDataCollection := getBool("NEURAL_DATA_COLLECTION", false)
	neuralDataDir := get("NEURAL_DATA_DIR", ".mdemg/neural/training-data")

	// NR-2: Python Sidecar
	sidecarEnabled := getBool("SIDECAR_ENABLED", false)

	// NR-3: Neural Reranker Integration
	neuralRerankEnabled := getBool("NEURAL_RERANK_ENABLED", false)
	neuralRerankURL := get("NEURAL_RERANK_URL", "http://localhost:8100")
	neuralRerankTimeoutMs, err := atoi("NEURAL_RERANK_TIMEOUT_MS", 1000)
	if err != nil {
		return Config{}, err
	}
	neuralRerankFallback := get("NEURAL_RERANK_FALLBACK", rerankProvider)

	// ===== Synergy =====
	synergyMemoryLineThreshold, err := atoi("SYNERGY_MEMORY_LINE_THRESHOLD", 120)
	if err != nil {
		return Config{}, err
	}
	synergyClaudeMDPath := get("SYNERGY_CLAUDE_MD_PATH", "")
	synergyMemoryMDPath := get("SYNERGY_MEMORY_MD_PATH", "")
	synergyAssessmentEnabled := getBool("SYNERGY_ASSESSMENT_ENABLED", true)
	synergyTargetClaudeLines, err := atoi("SYNERGY_TARGET_CLAUDE_LINES", 150)
	if err != nil {
		return Config{}, err
	}
	synergyTargetMemoryLines, err := atoi("SYNERGY_TARGET_MEMORY_LINES", 120)
	if err != nil {
		return Config{}, err
	}
	synergyOverlapSampleSize, err := atoi("SYNERGY_OVERLAP_SAMPLE_SIZE", 5)
	if err != nil {
		return Config{}, err
	}
	synergyOverflowAlertThreshold, err := atoi("SYNERGY_OVERFLOW_ALERT_THRESHOLD", 5)
	if err != nil {
		return Config{}, err
	}
	synergyRecoveryBufferSpace := get("SYNERGY_RECOVERY_BUFFER_SPACE", "synergy-buffer")
	synergyRecoveryBufferPath := get("SYNERGY_RECOVERY_BUFFER_PATH", ".mdemg/synergy-recovery-buffer.jsonl")

	// RSIC overall-health weights (DH-005). Defaults mirror
	// ape.DefaultHealthWeights(); negative values fall back to the default
	// (warning logged) because negative weights are nonsensical.
	rsicHealthWeightRetrieval, err := atof("RSIC_HEALTH_WEIGHT_RETRIEVAL", 0.08)
	if err != nil {
		return Config{}, err
	}
	rsicHealthWeightMemory, err := atof("RSIC_HEALTH_WEIGHT_MEMORY", 0.15)
	if err != nil {
		return Config{}, err
	}
	rsicHealthWeightEdge, err := atof("RSIC_HEALTH_WEIGHT_EDGE", 0.15)
	if err != nil {
		return Config{}, err
	}
	rsicHealthWeightTask, err := atof("RSIC_HEALTH_WEIGHT_TASK", 0.20)
	if err != nil {
		return Config{}, err
	}
	rsicHealthWeightGuidance, err := atof("RSIC_HEALTH_WEIGHT_GUIDANCE", 0.17)
	if err != nil {
		return Config{}, err
	}
	rsicHealthWeightProtocol, err := atof("RSIC_HEALTH_WEIGHT_PROTOCOL", 0.20)
	if err != nil {
		return Config{}, err
	}
	rsicHealthWeightSynergy, err := atof("RSIC_HEALTH_WEIGHT_SYNERGY", 0.05)
	if err != nil {
		return Config{}, err
	}
	// Negative values → fall back to default with a warning log. The formula
	// treats zero as "disable this dimension" (legitimate operator choice);
	// negatives are almost certainly typos.
	if rsicHealthWeightRetrieval < 0 {
		slog.Warn("RSIC_HEALTH_WEIGHT_RETRIEVAL is negative; using default", "value", rsicHealthWeightRetrieval, "default", 0.08)
		rsicHealthWeightRetrieval = 0.08
	}
	if rsicHealthWeightMemory < 0 {
		slog.Warn("RSIC_HEALTH_WEIGHT_MEMORY is negative; using default", "value", rsicHealthWeightMemory, "default", 0.15)
		rsicHealthWeightMemory = 0.15
	}
	if rsicHealthWeightEdge < 0 {
		slog.Warn("RSIC_HEALTH_WEIGHT_EDGE is negative; using default", "value", rsicHealthWeightEdge, "default", 0.15)
		rsicHealthWeightEdge = 0.15
	}
	if rsicHealthWeightTask < 0 {
		slog.Warn("RSIC_HEALTH_WEIGHT_TASK is negative; using default", "value", rsicHealthWeightTask, "default", 0.20)
		rsicHealthWeightTask = 0.20
	}
	if rsicHealthWeightGuidance < 0 {
		slog.Warn("RSIC_HEALTH_WEIGHT_GUIDANCE is negative; using default", "value", rsicHealthWeightGuidance, "default", 0.17)
		rsicHealthWeightGuidance = 0.17
	}
	if rsicHealthWeightProtocol < 0 {
		slog.Warn("RSIC_HEALTH_WEIGHT_PROTOCOL is negative; using default", "value", rsicHealthWeightProtocol, "default", 0.20)
		rsicHealthWeightProtocol = 0.20
	}
	if rsicHealthWeightSynergy < 0 {
		slog.Warn("RSIC_HEALTH_WEIGHT_SYNERGY is negative; using default", "value", rsicHealthWeightSynergy, "default", 0.05)
		rsicHealthWeightSynergy = 0.05
	}

	// TimescaleDB settings
	tsdbEnabled := getBool("TSDB_ENABLED", false)
	tsdbHost := get("TSDB_HOST", "localhost")
	tsdbPort, err := atoi("TSDB_PORT", 5432)
	if err != nil {
		return Config{}, err
	}
	tsdbUser := get("TSDB_USER", "mdemg")
	tsdbPassword := get("TSDB_PASSWORD", "mdemg_metrics")
	tsdbDatabase := get("TSDB_DATABASE", "mdemg_metrics")
	tsdbSSLMode := get("TSDB_SSL_MODE", "disable")
	tsdbMaxConns, err := atoi("TSDB_MAX_CONNS", 10)
	if err != nil {
		return Config{}, err
	}
	tsdbFlushIntervalSec, err := atoi("TSDB_FLUSH_INTERVAL_SEC", 60)
	if err != nil {
		return Config{}, err
	}
	tsdbRawRetentionDays, err := atoi("TSDB_RAW_RETENTION_DAYS", 90)
	if err != nil {
		return Config{}, err
	}
	tsdbHourlyRetentionDays, err := atoi("TSDB_HOURLY_RETENTION_DAYS", 365)
	if err != nil {
		return Config{}, err
	}
	tsdbRequiredSchemaVersion, err := atoi("TSDB_REQUIRED_SCHEMA_VERSION", 31)
	if err != nil {
		return Config{}, err
	}
	tsdbOptional := getBool("TSDB_OPTIONAL", true)
	instanceID := get("MDEMG_INSTANCE_ID", "")
	if instanceID == "" {
		hostname, _ := os.Hostname()
		instanceID = fmt.Sprintf("%s-%s", hostname, rsicWatchdogSpaceID)
	}
	llmInteractionLogging := getBool("LLM_INTERACTION_LOGGING", true)
	embeddingEventLogging := getBool("EMBEDDING_EVENT_LOGGING", true)
	retrievalEventLogging := getBool("RETRIEVAL_EVENT_LOGGING", true)

	// Live Metrics settings
	liveMetricsEnabled := getBool("LIVE_METRICS_ENABLED", true)
	liveGuidanceRefreshSec, err := atoi("LIVE_GUIDANCE_REFRESH_SEC", 60)
	if err != nil {
		return Config{}, err
	}

	// Alert Dispatcher
	alertEnabled := getBool("ALERT_ENABLED", true)
	alertFilePath := get("ALERT_FILE_PATH", filepath.Join(os.Getenv("HOME"), ".mdemg", "alerts", "current.json"))
	alertCooldownSec, err := atoi("ALERT_COOLDOWN_SEC", 300)
	if err != nil {
		return Config{}, err
	}
	alertMaxEntries, err := atoi("ALERT_MAX_ENTRIES", 50)
	if err != nil {
		return Config{}, err
	}
	alertMacOSNotify := getBool("ALERT_MACOS_NOTIFY", false)
	alertMacOSNotifyMinSev := get("ALERT_MACOS_NOTIFY_MIN_SEV", "high")

	// Health Probe
	healthProbeEnabled := getBool("HEALTH_PROBE_ENABLED", true)
	healthProbeIntervalSec, err := atoi("HEALTH_PROBE_INTERVAL_SEC", 60)
	if err != nil {
		return Config{}, err
	}

	// Goroutine Supervisor (SUPERVISOR-002)
	supervisorMaxRestarts, err := atoi("SUPERVISOR_MAX_RESTARTS", 3)
	if err != nil {
		return Config{}, err
	}
	supervisorRestartWindowMin, err := atoi("SUPERVISOR_RESTART_WINDOW_MIN", 60)
	if err != nil {
		return Config{}, err
	}
	supervisorBackoffBaseSec, err := atoi("SUPERVISOR_BACKOFF_BASE_SEC", 5)
	if err != nil {
		return Config{}, err
	}
	alertRuleFailureThreshold, err := atoi("ALERT_RULE_FAILURE_THRESHOLD", 3)
	if err != nil {
		return Config{}, err
	}
	rsicLLMErrorMinCount, err := atoi("RSIC_LLM_ERROR_MIN_COUNT", 5)
	if err != nil {
		return Config{}, err
	}
	rsicLLMErrorRecencyMin, err := atoi("RSIC_LLM_ERROR_RECENCY_MIN", 60)
	if err != nil {
		return Config{}, err
	}

	// LLM Client Retry
	llmRetryEnabled := getBool("LLM_RETRY_ENABLED", true)
	llmRetryMaxAttempts, err := atoi("LLM_RETRY_MAX_ATTEMPTS", 5)
	if err != nil {
		return Config{}, err
	}
	llmRetryBaseDelayMs, err := atoi("LLM_RETRY_BASE_DELAY_MS", 500)
	if err != nil {
		return Config{}, err
	}
	llmRetryMaxDelayMs, err := atoi("LLM_RETRY_MAX_DELAY_MS", 60000)
	if err != nil {
		return Config{}, err
	}
	llmRetryMultiplier, err := atof("LLM_RETRY_MULTIPLIER", 2.0)
	if err != nil {
		return Config{}, err
	}
	llmRetryJitter, err := atof("LLM_RETRY_JITTER", 0.2)
	if err != nil {
		return Config{}, err
	}
	// DH-004 E4.2: retry on context.DeadlineExceeded when the parent context
	// still has enough budget for another attempt. Default true — single slow
	// OpenAI responses should not trip circuit breakers on the first failure.
	llmRetryOnDeadline := getBool("LLM_RETRY_DEADLINE_ENABLED", true)

	// LLM Consecutive Failure Alert
	llmConsecutiveFailureThreshold, err := atoi("LLM_CONSECUTIVE_FAILURE_THRESHOLD", 3)
	if err != nil {
		return Config{}, err
	}

	// Alert Evaluator
	alertEvaluatorEnabled := getBool("ALERT_EVALUATOR_ENABLED", true)
	alertEvaluatorIntervalSec, err := atoi("ALERT_EVALUATOR_INTERVAL_SEC", 30)
	if err != nil {
		return Config{}, err
	}
	alertRetrieveP95Ms, err := atoi("ALERT_RETRIEVE_P95_MS", 120000)
	if err != nil {
		return Config{}, err
	}
	alertRetrieveP99Ms, err := atoi("ALERT_RETRIEVE_P99_MS", 300000)
	if err != nil {
		return Config{}, err
	}
	alertRetrieveLatencyLookbackMin, err := atoi("ALERT_RETRIEVE_LATENCY_LOOKBACK_MIN", 30)
	if err != nil {
		return Config{}, err
	}
	tsdbWriterAlertLookbackMin, err := atoi("TSDB_WRITER_ALERT_LOOKBACK_MIN", 60)
	if err != nil {
		return Config{}, err
	}
	scorerChangeLookbackHours, err := atoi("SCORER_CHANGE_LOOKBACK_HOURS", 24)
	if err != nil {
		return Config{}, err
	}
	consensusShiftThreshold, err := atof("CONSENSUS_SHIFT_THRESHOLD", 0.10)
	if err != nil {
		return Config{}, err
	}
	consensusShiftRecentHours, err := atoi("CONSENSUS_SHIFT_RECENT_HOURS", 6)
	if err != nil {
		return Config{}, err
	}
	consensusShiftBaselineDays, err := atoi("CONSENSUS_SHIFT_BASELINE_DAYS", 7)
	if err != nil {
		return Config{}, err
	}
	consensusShiftMinSamples, err := atoi("CONSENSUS_SHIFT_MIN_SAMPLES", 20)
	if err != nil {
		return Config{}, err
	}
	emergenceCycleAlertThresholdSec, err := atof("EMERGENCE_CYCLE_ALERT_THRESHOLD_SEC", 60)
	if err != nil {
		return Config{}, err
	}
	emergenceCycleAlertLookbackMin, err := atoi("EMERGENCE_CYCLE_ALERT_LOOKBACK_MIN", 120)
	if err != nil {
		return Config{}, err
	}
	surpriseEmbeddingNoveltyTopK, err := atoi("SURPRISE_EMBEDDING_NOVELTY_TOPK", 50)
	if err != nil {
		return Config{}, err
	}
	surpriseEmbeddingNoveltySimFloor, err := atof("SURPRISE_EMBEDDING_NOVELTY_SIM_FLOOR", 0)
	if err != nil {
		return Config{}, err
	}
	surpriseFactorHighThreshold, err := atof("SURPRISE_FACTOR_HIGH_THRESHOLD", 0.4)
	if err != nil {
		return Config{}, err
	}
	surpriseFactorMediumThreshold, err := atof("SURPRISE_FACTOR_MEDIUM_THRESHOLD", 0.15)
	if err != nil {
		return Config{}, err
	}

	// NOSILENT-001 — scheduled-job health alerting
	jobHealthAlertEnabled := getBool("JOB_HEALTH_ALERT_ENABLED", true)
	hookHealthAlertEnabled := getBool("HOOK_HEALTH_ALERT_ENABLED", true)
	hookSilentLookbackHours, err := atoi("HOOK_SILENT_LOOKBACK_HOURS", 24)
	if err != nil {
		return Config{}, err
	}
	hookActivityMinEvents, err := atoi("HOOK_ACTIVITY_MIN_EVENTS", 5)
	if err != nil {
		return Config{}, err
	}
	nullWeightEdgeAlertThreshold, err := atoi("NULL_WEIGHT_EDGE_ALERT_THRESHOLD", 100)
	if err != nil {
		return Config{}, err
	}
	orphanRatioMinNodes, err := atoi("ORPHAN_RATIO_MIN_NODES", 50)
	if err != nil {
		return Config{}, err
	}
	orphanRatioThreshold, err := atof("ORPHAN_RATIO_THRESHOLD", 0.10)
	if err != nil {
		return Config{}, err
	}
	orphanCountThreshold, err := atoi("ORPHAN_COUNT_THRESHOLD", 1000)
	if err != nil {
		return Config{}, err
	}
	graphHealthScoreFloor, err := atof("GRAPH_HEALTH_SCORE_FLOOR", 0.5)
	if err != nil {
		return Config{}, err
	}
	neo4jCPUAlertThresholdPercent, err := atof("NEO4J_CPU_ALERT_THRESHOLD_PERCENT", 500)
	if err != nil {
		return Config{}, err
	}
	if graphHealthScoreFloor < 0 || graphHealthScoreFloor > 1 {
		return Config{}, errors.New("GRAPH_HEALTH_SCORE_FLOOR must be in [0,1]")
	}
	canaryProbeCount, err := atoi("FT_LOOP_CANARY_PROBE_COUNT", 8)
	if err != nil {
		return Config{}, err
	}
	if canaryProbeCount < 1 {
		return Config{}, errors.New("FT_LOOP_CANARY_PROBE_COUNT must be >= 1")
	}
	swapHealthTimeout, err := atoi("FT_LOOP_SWAP_HEALTH_TIMEOUT_SEC", 120)
	if err != nil {
		return Config{}, err
	}
	if swapHealthTimeout < 10 {
		return Config{}, errors.New("FT_LOOP_SWAP_HEALTH_TIMEOUT_SEC must be >= 10")
	}
	ftReadinessStalenessMin, err := atoi("FT_READINESS_STALENESS_MIN", 30)
	if err != nil {
		return Config{}, err
	}
	exportRetentionHours, err := atoi("MDEMG_EXPORT_RETENTION_HOURS", 168)
	if err != nil {
		return Config{}, err
	}
	trainingReadinessThreshold, err := atoi("TRAINING_READINESS_THRESHOLD", 500)
	if err != nil {
		return Config{}, err
	}
	if trainingReadinessThreshold < 1 {
		return Config{}, errors.New("TRAINING_READINESS_THRESHOLD must be >= 1")
	}
	trainingReadinessThresholdOverrides := map[string]int{}
	if raw := strings.TrimSpace(os.Getenv("TRAINING_READINESS_THRESHOLD_OVERRIDES")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &trainingReadinessThresholdOverrides); err != nil {
			return Config{}, fmt.Errorf("TRAINING_READINESS_THRESHOLD_OVERRIDES must be valid JSON: %w", err)
		}
		for task, v := range trainingReadinessThresholdOverrides {
			if task == "" {
				return Config{}, errors.New("TRAINING_READINESS_THRESHOLD_OVERRIDES: empty task key")
			}
			if v < 1 {
				return Config{}, fmt.Errorf("TRAINING_READINESS_THRESHOLD_OVERRIDES[%s] must be >= 1", task)
			}
		}
	}
	ftLoopEnabled := getBool("FT_LOOP_ENABLED", false)
	ftLoopMinRetrainIntervalHours, err := atoi("FT_LOOP_MIN_RETRAIN_INTERVAL_HOURS", 168)
	if err != nil {
		return Config{}, err
	}
	if ftLoopMinRetrainIntervalHours < 1 {
		return Config{}, errors.New("FT_LOOP_MIN_RETRAIN_INTERVAL_HOURS must be >= 1")
	}
	ftLoopMinFreshFraction, err := atof("FT_LOOP_MIN_FRESH_FRACTION", 0.30)
	if err != nil {
		return Config{}, err
	}
	if ftLoopMinFreshFraction < 0 || ftLoopMinFreshFraction > 1 {
		return Config{}, errors.New("FT_LOOP_MIN_FRESH_FRACTION must be in [0,1]")
	}
	ftLoopPollIntervalSec, err := atoi("FT_LOOP_POLL_INTERVAL_SEC", 60)
	if err != nil {
		return Config{}, err
	}
	if ftLoopPollIntervalSec < 1 {
		return Config{}, errors.New("FT_LOOP_POLL_INTERVAL_SEC must be >= 1")
	}
	ftLoopLeaseMaxHours, err := atoi("FT_LOOP_LEASE_MAX_HOURS", 14)
	if err != nil {
		return Config{}, err
	}
	if ftLoopLeaseMaxHours < 1 {
		return Config{}, errors.New("FT_LOOP_LEASE_MAX_HOURS must be >= 1")
	}
	ftLoopLeasePath := strings.TrimSpace(os.Getenv("FT_LOOP_LEASE_PATH"))
	ftLoopMinFreeDiskGB, err := atof("FT_LOOP_MIN_FREE_DISK_GB", 100)
	if err != nil {
		return Config{}, err
	}
	if ftLoopMinFreeDiskGB < 0 {
		return Config{}, errors.New("FT_LOOP_MIN_FREE_DISK_GB must be >= 0")
	}
	ftLoopPythonBin := get("FT_LOOP_PYTHON_BIN", "python3")
	ftLoopModelVersion := get("FT_LOOP_MODEL_VERSION", "mdemg-llm-v1")
	ftLoraEpochsCap, err := atoi("FT_LORA_EPOCHS_CAP", 3)
	if err != nil {
		return Config{}, err
	}
	if ftLoraEpochsCap < 1 {
		return Config{}, errors.New("FT_LORA_EPOCHS_CAP must be >= 1")
	}
	ftEarlyStopValLossFactor, err := atof("FT_EARLY_STOP_VAL_LOSS_FACTOR", 1.05)
	if err != nil {
		return Config{}, err
	}
	if ftEarlyStopValLossFactor <= 1.0 {
		return Config{}, errors.New("FT_EARLY_STOP_VAL_LOSS_FACTOR must be > 1.0")
	}
	ftLoopWorkDir := strings.TrimSpace(os.Getenv("FT_LOOP_WORK_DIR"))
	ftLoopBaseModel := get("FT_LOOP_BASE_MODEL", ".local-models/qwen3-14b-4bit-base")
	ftLoopBaseSHA := get("FT_LOOP_BASE_SHA", "a54ec18ffe24f3c909e9556471dc156ed9b3b61b872008831c7cba9d4768b4a5")
	ftLoopUaitsSpec := get("FT_LOOP_UAITS_SPEC", "docs/tests/uaits/specs/mdemg.uaits.json")
	ftLoopBenchmarkConfig := get("FT_LOOP_BENCHMARK_CONFIG", "configs/benchmark_phase10.yaml")
	ftLoopLoraRank, err := atoi("FT_LOOP_LORA_RANK", 32)
	if err != nil {
		return Config{}, err
	}
	if ftLoopLoraRank < 1 {
		return Config{}, errors.New("FT_LOOP_LORA_RANK must be >= 1")
	}
	ftLoopLoraAlpha, err := atoi("FT_LOOP_LORA_ALPHA", 64)
	if err != nil {
		return Config{}, err
	}
	if ftLoopLoraAlpha < 1 {
		return Config{}, errors.New("FT_LOOP_LORA_ALPHA must be >= 1")
	}
	ftLoopGatePort, err := atoi("FT_LOOP_GATE_PORT", 18102)
	if err != nil {
		return Config{}, err
	}
	if ftLoopGatePort < 1024 || ftLoopGatePort > 65535 {
		return Config{}, errors.New("FT_LOOP_GATE_PORT must be in [1024,65535]")
	}
	ftLoopExportSinceDays, err := atoi("FT_LOOP_EXPORT_SINCE_DAYS", 7)
	if err != nil {
		return Config{}, err
	}
	if ftLoopExportSinceDays < 1 {
		return Config{}, errors.New("FT_LOOP_EXPORT_SINCE_DAYS must be >= 1")
	}
	ftLoopGateTaskFilter := strings.TrimSpace(os.Getenv("FT_LOOP_GATE_TASK_FILTER"))
	ftLoopGateMinScore, err := atof("FT_LOOP_GATE_MIN_SCORE", 0.80)
	if err != nil {
		return Config{}, err
	}
	if ftLoopGateMinScore < 0 || ftLoopGateMinScore > 1 {
		return Config{}, errors.New("FT_LOOP_GATE_MIN_SCORE must be in [0,1]")
	}
	maintLiveAlertEnabled := getBool("MAINT_LIVE_ALERT_ENABLED", true)
	maintLiveLookbackDays, err := atoi("MAINT_LIVE_LOOKBACK_DAYS", 8)
	if err != nil {
		return Config{}, err
	}
	jobBackupStalenessHours, err := atoi("JOB_BACKUP_STALENESS_HOURS", 0)
	if err != nil {
		return Config{}, err
	}
	jobFailureLookbackMin, err := atoi("JOB_FAILURE_LOOKBACK_MIN", 60)
	if err != nil {
		return Config{}, err
	}

	// TSDB Writer
	tsdbWriterBufferMaxSize, err := atoi("TSDB_WRITER_BUFFER_MAX_SIZE", 1000)
	if err != nil {
		return Config{}, err
	}

	// CONFIG-DEADFLAG-001: surface every invalid boolean at once.
	if len(boolErrs) > 0 {
		return Config{}, fmt.Errorf("invalid boolean env value(s): %s", strings.Join(boolErrs, "; "))
	}

	return Config{
		ListenAddr:                     listen,
		Neo4jURI:                       uri,
		Neo4jUser:                      user,
		Neo4jPass:                      pass,
		Neo4jBoltPort:                  neo4jBoltPort,
		Neo4jHTTPPort:                  neo4jHTTPPort,
		RequiredSchemaVersion:          reqVer,
		VectorIndexName:                idx,
		DefaultCandidateK:              candK,
		DefaultTopK:                    topK,
		DefaultHopDepth:                hops,
		MaxNeighborsPerNode:            maxNbr,
		MaxTotalEdgesFetched:           maxEdges,
		AllowedRelationshipTypes:       out,
		LearningEdgeCapPerRequest:      learnCap,
		LearningMinActivation:          learnMinAct,
		LearningEta:                    learnEta,
		LearningMu:                     learnMu,
		LearningWMin:                   learnWMin,
		LearningWMax:                   learnWMax,
		LearningDecayPerDay:            learnDecayPerDay,
		LearningPruneThreshold:         learnPruneThreshold,
		LearningMaxEdgesPerNode:        learnMaxEdgesPerNode,
		LearningSessionCliqueWindow:    learnSessionCliqueWindow,
		LLMProvider:                    llmProvider,
		LLMModel:                       llmModel,
		EmbeddingProvider:              embProvider,
		OpenAIAPIKey:                   openaiKey,
		OpenAIModel:                    openaiModel,
		OpenAIEndpoint:                 openaiEndpoint,
		LLMEndpoint:                    llmEndpoint,
		OllamaEndpoint:                 ollamaEndpoint,
		OllamaModel:                    ollamaModel,
		ModelBackend:                   modelBackend,
		ModelNamespace:                 modelNamespace,
		ModelName:                      modelName,
		ModelQuants:                    modelQuants,
		ModelRamTiers:                  modelRamTiers,
		ModelQuant:                     modelQuant,
		AdapterBase:                    adapterBase,
		ModelDir:                       modelDir,
		OllamaModelsRoot:               ollamaModelsRoot,
		OllamaRegistryHost:             ollamaRegistryHost,
		ModelManifestPath:              modelManifestPath,
		EmbeddingTargetDims:            embTargetDims,
		EmbeddingCacheEnabled:          embCacheEnabled,
		EmbeddingCacheSize:             embCacheSize,
		QueryCacheEnabled:              queryCacheEnabled,
		QueryCacheCapacity:             queryCacheCapacity,
		QueryCacheTTLSeconds:           queryCacheTTL,
		SemanticEdgeOnIngest:           semEdgeEnabled,
		SemanticEdgeTopN:               semEdgeTopN,
		SemanticEdgeMinSimilarity:      semEdgeMinSim,
		SemanticEdgeInitialWeight:      semEdgeInitWeight,
		BatchIngestMaxItems:            batchMaxItems,
		HTTPReadTimeout:                httpReadTimeout,
		HTTPWriteTimeout:               httpWriteTimeout,
		RetrieveTimeoutMs:              retrieveTimeoutMs,
		AnomalyDetectionEnabled:        anomalyEnabled,
		AnomalyDuplicateThreshold:      anomalyDupThreshold,
		AnomalyOutlierStdDevs:          anomalyOutlierStdDevs,
		AnomalyStaleDays:               anomalyStaleDays,
		AnomalyMaxCheckMs:              anomalyMaxCheckMs,
		ScoringAlpha:                   scoringAlpha,
		ScoringBeta:                    scoringBeta,
		ScoringGamma:                   scoringGamma,
		ScoringDelta:                   scoringDelta,
		ScoringPhi:                     scoringPhi,
		ScoringKappa:                   scoringKappa,
		ScoringRhoL0:                   scoringRhoL0,
		ScoringRhoL1:                   scoringRhoL1,
		ScoringRhoL2:                   scoringRhoL2,
		ScoringConfigBoost:             scoringConfigBoost,
		ScoringPathBoost:               scoringPathBoost,
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
		LogFormat:                      logFormat,
		LogLevel:                       logLevel,
		LogSkipHealth:                  logSkipHealth,
		HiddenLayerEnabled:             hiddenEnabled,
		HiddenLayerClusterEps:          hiddenClusterEps,
		HiddenLayerMinSamples:          hiddenMinSamples,
		HiddenLayerMaxHidden:           hiddenMaxHidden,
		HiddenLayerMaxClusterSize:      hiddenMaxClusterSize,
		HiddenLayerPathGroupDepth:      hiddenPathGroupDepth,
		HiddenLayerBatchSize:           hiddenBatchSize,
		HiddenLayerForwardAlpha:        hiddenForwardAlpha,
		HiddenLayerForwardBeta:         hiddenForwardBeta,
		HiddenLayerBackwardSelf:        hiddenBackwardSelf,
		HiddenLayerBackwardBase:        hiddenBackwardBase,
		HiddenLayerBackwardConc:        hiddenBackwardConc,
		ConceptMergeEnabled:            conceptMergeEnabled,
		ConceptMergeThreshold:          conceptMergeThreshold,
		EdgeAttentionEnabled:           edgeAttentionEnabled,
		EdgeAttentionCoActivated:       edgeAttentionCoActivated,
		EdgeAttentionAssociated:        edgeAttentionAssociated,
		EdgeAttentionGeneralizes:       edgeAttentionGeneralizes,
		EdgeAttentionAbstractsTo:       edgeAttentionAbstractsTo,
		EdgeAttentionTemporal:          edgeAttentionTemporal,
		EdgeAttentionCodeBoost:         edgeAttentionCodeBoost,
		EdgeAttentionArchBoost:         edgeAttentionArchBoost,
		QueryAwareExpansionEnabled:     queryAwareExpansionEnabled,
		QueryAwareAttentionWeight:      queryAwareAttentionWeight,
		NodeEmbeddingCacheSize:         nodeEmbeddingCacheSize,
		NodeEmbeddingCacheTTLSec:       nodeEmbeddingCacheTTLSec,
		EdgeTypeStrategy:               edgeTypeStrategy,
		StructuralEdgeTypes:            structuralEdgeTypes,
		LearnedEdgeTypes:               learnedEdgeTypes,
		HybridSwitchHop:                hybridSwitchHop,
		HybridRetrievalEnabled:         hybridEnabled,
		BM25TopK:                       bm25TopK,
		BM25Weight:                     bm25Weight,
		VectorWeight:                   vectorWeight,
		RRFConstant:                    rrfConstant,
		RerankEnabled:                  rerankEnabled,
		RerankProvider:                 rerankProvider,
		RerankModel:                    rerankModel,
		RerankTopN:                     rerankTopN,
		RerankWeight:                   rerankWeight,
		RerankTimeoutMs:                rerankTimeoutMs,
		RerankMinBudgetMs:              rerankMinBudgetMs,
		NeuralRerankMinBudgetMs:        neuralRerankMinBudgetMs,
		RerankJinaKey:                  rerankJinaKey,
		RerankJinaModel:                rerankJinaModel,
		RerankJinaURL:                  rerankJinaURL,
		RerankCompress:                 rerankCompress,
		PluginsEnabled:                 pluginsEnabled,
		PluginsDir:                     pluginsDir,
		PluginSocketDir:                pluginSocketDir,
		MdemgVersion:                   mdemgVersion,
		MdemgCommit:                    mdemgCommit,
		LinearTeamID:                   linearTeamID,
		LinearWebhookSecret:            linearWebhookSecret,
		LinearWebhookSpaceID:           linearWebhookSpaceID,
		WebhookConfigs:                 webhookConfigs,
		FileWatcherEnabled:             fileWatcherEnabled,
		FileWatcherConfigs:             fileWatcherConfigs,
		ConflictLogEnabled:             conflictLogEnabled,

		// Phase 47: Optimistic Retry + Edge Consistency
		OptimisticRetryEnabled:        optimisticRetryEnabled,
		OptimisticRetryMaxAttempts:    optimisticRetryMaxAttempts,
		OptimisticRetryBaseDelayMs:    optimisticRetryBaseDelayMs,
		OptimisticRetryMaxDelayMs:     optimisticRetryMaxDelayMs,
		OptimisticRetryMultiplier:     optimisticRetryMultiplier,
		EdgeStalenessCascadeEnabled:   edgeStalenessCascadeEnabled,
		EdgeStalenessRefreshBatchSize: edgeStalenessRefreshBatchSize,

		LLMSummaryEnabled:   llmSummaryEnabled,
		LLMSummaryProvider:  llmSummaryProvider,
		LLMSummaryModel:     llmSummaryModel,
		LLMSummaryMaxTokens: llmSummaryMaxTokens,
		LLMSummaryBatchSize: llmSummaryBatchSize,
		LLMSummaryTimeoutMs: llmSummaryTimeoutMs,
		LLMSummaryCacheSize: llmSummaryCacheSize,

		// Phase 101: SME Synthesis
		SynthesisEnabled:   synthesisEnabled,
		SynthesisProvider:  synthesisProvider,
		SynthesisModel:     synthesisModel,
		SynthesisMaxTokens: synthesisMaxTokens,
		SynthesisTimeoutMs: synthesisTimeoutMs,
		SynthesisCompress:  synthesisCompress,

		// Phase 102: Intent Translation
		IntentEnabled:   intentEnabled,
		IntentProvider:  intentProvider,
		IntentModel:     intentModel,
		IntentMaxTokens: intentMaxTokens,
		IntentTimeoutMs: intentTimeoutMs,

		// PROD-READINESS: Query Classification
		QueryClassifyEnabled:   queryClassifyEnabled,
		QueryClassifyProvider:  queryClassifyProvider,
		QueryClassifyModel:     queryClassifyModel,
		QueryClassifyMaxTokens: queryClassifyMaxTokens,
		QueryClassifyTimeoutMs: queryClassifyTimeoutMs,

		// Phase 103: Dynamic Emergence
		EmergenceEnabled:                    emergenceEnabled,
		HiddenThemeIdentitySimThreshold:     hiddenThemeIdentitySimThreshold,
		HiddenPatternIdentitySimThreshold:   hiddenPatternIdentitySimThreshold,
		HiddenPatternMemberJaccardThreshold: hiddenPatternMemberJaccardThreshold,
		HiddenIncrementalEnabled:            getBool("HIDDEN_INCREMENTAL_ENABLED", true),
		HiddenIncrementalPassesEnabled:      getBool("HIDDEN_INCREMENTAL_PASSES_ENABLED", false),
		DynamicEdgeIncrementalLookbackHours: dynEdgeIncLookback,
		HiddenIncrementalAssignSimThreshold: hiddenIncrementalAssignSimThreshold,
		ConsolidateTimeoutMs:                consolidateTimeoutMs,
		HiddenThemeTargetRatio:              hiddenThemeTargetRatio,
		HiddenThemeAssignSimThreshold:       hiddenThemeAssignSimThreshold,
		ConversationCoverageAlertFloor:      conversationCoverageAlertFloor,
		ConversationCoverageMinObs:          conversationCoverageMinObs,
		EmergenceProvider:                   emergenceProvider,
		EmergenceModel:                      emergenceModel,
		EmergenceMaxTokens:                  emergenceMaxTokens,
		EmergenceTimeoutMs:                  emergenceTimeoutMs,
		EmergenceMinWeight:                  emergenceMinWeight,
		EmergenceMinClusterSize:             emergenceMinClusterSize,
		EmergenceMaxClusters:                emergenceMaxClusters,

		// Phase 104: Active MCP Guardrails
		GuardrailEnabled:        guardrailEnabled,
		GuardrailProvider:       guardrailProvider,
		GuardrailModel:          guardrailModel,
		GuardrailMaxTokens:      guardrailMaxTokens,
		GuardrailTimeoutMs:      guardrailTimeoutMs,
		GuardrailMaxConstraints: guardrailMaxConstraints,
		GuardrailCompress:       guardrailCompress,
		GuardrailProducerEnabled:       guardrailProducerEnabled,
		GuardrailProducerMaxConcurrent: guardrailProducerMaxConcurrent,
		GuardrailIncludeCorrections:    guardrailIncludeCorrections,

		// Phase Jiminy: Jiminy Guidance
		JiminyEnabled:                             jiminyEnabled,
		JiminyContradictedWeakenEnabled:           jiminyContradictedWeakenEnabled,
		JiminyTimeoutMs:                           jiminyTimeoutMs,
		JiminyMaxItems:                            jiminyMaxItems,
		JiminySurfaceActionableWeight:             jiminySurfaceActionableWeight,
		JiminySurfaceMinActionable:                jiminySurfaceMinActionable,
		JiminySurfaceMinActionableFraction:        jiminySurfaceMinActionableFraction,
		JiminySurfaceMaxAbstractionFraction:       jiminySurfaceMaxAbstractionFraction,
		JiminySurfaceCooldownIgnoredCount:         jiminySurfaceCooldownIgnoredCount,
		JiminySurfaceCooldownCapacity:             jiminySurfaceCooldownCapacity,
		JiminySurfaceEffectivenessPriorWeight:     jiminySurfaceEffectivenessPriorWeight,
		JiminySurfaceEffectivenessPriorTTLSec:     jiminySurfaceEffectivenessPriorTTLSec,
		JiminySurfaceEffectivenessPriorMinSamples: jiminySurfaceEffectivenessPriorMinSamples,
		JiminyDirectiveSynthesisEnabled:           jiminyDirectiveSynthesisEnabled,
		JiminyDirectiveSynthesisMaxPromptTokens:   jiminyDirectiveSynthesisMaxPromptTokens,
		JiminyGuidanceConstraintBiasEnabled:       getBool("JIMINY_GUIDANCE_CONSTRAINT_BIAS_ENABLED", false),
		JiminyGuidanceConstraintIncludeTopK:       jiminyGuidanceConstraintIncludeTopK,
		JiminyGuidanceConstraintSimFloor:          jiminyGuidanceConstraintSimFloor,
		JiminyMinConfidence:                       jiminyMinConfidence,
		JiminySignalStrengthWeight:                jiminySignalStrengthWeight,
		JiminyConstraintCodeSimThreshold:          jiminyConstraintCodeSimThreshold,
		JiminyIncludeFrontiers:                    jiminyIncludeFrontiers,
		JiminyFrontierMinSim:                      jiminyFrontierMinSim,
		JiminyEffectivenessTTLSec:                 jiminyEffectivenessTTLSec,
		JiminyWarmEnabled:                         jiminyWarmEnabled,
		JiminyWarmDebounceSec:                     jiminyWarmDebounceSec,
		JiminyWarmComputeTimeoutMs:                jiminyWarmComputeTimeoutMs,
		JiminyReformulateTimeoutMs:                jiminyReformulateTimeoutMs,
		JiminyFeedbackTimeoutMs:                   jiminyFeedbackTimeoutMs,
		JiminyWarmMaxAgeSec:                       jiminyWarmMaxAgeSec,

		// Jiminy J7-J12
		JiminyRetrievalEnabled:          jiminyRetrievalEnabled,
		JiminyRetrievalTopK:             jiminyRetrievalTopK,
		JiminyRetrievalHopDepth:         jiminyRetrievalHopDepth,
		JiminySynthesisEnabled:          jiminySynthesisEnabled,
		JiminySynthesisProvider:         jiminySynthesisProvider,
		JiminySynthesisModel:            jiminySynthesisModel,
		JiminySynthesisMaxTokens:        jiminySynthesisMaxTokens,
		JiminySynthesisTimeoutMs:        jiminySynthesisTimeoutMs,
		JiminyEvaluateEnabled:           jiminyEvaluateEnabled,
		JiminyEvaluateTimeoutMs:         jiminyEvaluateTimeoutMs,
		JiminyEvaluateMaxConstraints:    jiminyEvaluateMaxConstraints,
		JiminyEvaluateLLMEnabled:        jiminyEvaluateLLMEnabled,
		JiminyEvaluateLLMProvider:       jiminyEvaluateLLMProvider,
		JiminyEvaluateLLMModel:          jiminyEvaluateLLMModel,
		JiminyEvaluateLLMTimeoutMs:      jiminyEvaluateLLMTimeoutMs,
		JiminyEvaluateLLMMaxTokens:      jiminyEvaluateLLMMaxTokens,
		JiminyOutcomeClassifierEnabled:  jiminyOutcomeClassifierEnabled,
		JiminyOutcomeLLMEnabled:         jiminyOutcomeLLMEnabled,
		JiminyOutcomeSimilarityHigh:     jiminyOutcomeSimilarityHigh,
		JiminyOutcomeSimilarityLow:      jiminyOutcomeSimilarityLow,
		JiminyOutcomeNotApplicableSim:   jiminyOutcomeNotApplicableSim,
		JiminyOutcomeLLMMaxTokens:       jiminyOutcomeLLMMaxTokens,
		JiminyOutcomeCacheSize:          jiminyOutcomeCacheSize,
		JiminyClassifyCompress:          jiminyClassifyCompress,
		JiminyNonViolationCreditEnabled: jiminyNonViolationCreditEnabled,
		JiminyDedupSimilarityThreshold:  jiminyDedupSimilarityThreshold,
		JiminyCorrectionDecayRate:       jiminyCorrectionDecayRate,
		J17TicketCacheSize:              j17TicketCacheSize,
		JiminySynthesisTemperature:      jiminySynthesisTemperature,
		JiminyGuidanceContextMaxChars:   jiminyGuidanceContextMaxChars,
		JiminyGuidanceOutputMaxChars:    jiminyGuidanceOutputMaxChars,
		JiminyEvaluateOutputMaxChars:    jiminyEvaluateOutputMaxChars,
		JiminyEvaluateItemMaxChars:      jiminyEvaluateItemMaxChars,
		JiminyEscalationEnabled:         jiminyEscalationEnabled,
		JiminyEscalationWarnAfter:       jiminyEscalationWarnAfter,
		JiminyEscalationEscalateAfter:   jiminyEscalationEscalateAfter,
		JiminyEscalationBlockAfter:      jiminyEscalationBlockAfter,
		JiminyEscalationBlockEnabled:    jiminyEscalationBlockEnabled,
		JiminyEscalationDecayMinutes:    jiminyEscalationDecayMinutes,
		JiminyEscalationPersistEnabled:  jiminyEscalationPersistEnabled,
		JiminyStrictStatePath:           jiminyStrictStatePath,
		JiminyCodeRegenEnabled:          jiminyCodeRegenEnabled,
		JiminyCodeRegenThreshold:        jiminyCodeRegenThreshold,
		JiminyCodeRegenMinSamples:       jiminyCodeRegenMinSamples,

		// J17: AI-to-AI Communication Protocol
		J17Enabled:                     j17Enabled,
		J17TicketSecret:                j17TicketSecret,
		J17TicketTTLHours:              j17TicketTTLHours,
		J17SequenceBufferSize:          j17SequenceBufferSize,
		J17ReplayMaxEvents:             j17ReplayMaxEvents,
		J17BootstrapEnabled:            j17BootstrapEnabled,
		J17CodegenEnabled:              j17CodegenEnabled,
		J17CodegenProvider:             j17CodegenProvider,
		J17CodegenModel:                j17CodegenModel,
		J17DefaultTier:                 j17DefaultTier,
		J17TrustInitial:                j17TrustInitial,
		J17TrustBoostPerFollow:         j17TrustBoostPerFollow,
		J17TrustDecayPerIgnore:         j17TrustDecayPerIgnore,
		J17TrustDecayPerContradict:     j17TrustDecayPerContradict,
		J17TrustHighThreshold:          j17TrustHighThreshold,
		J17TrustLowThreshold:           j17TrustLowThreshold,
		J17TierGateMode:                j17TierGateMode,
		J17TierComprehensionHigh:       j17TierComprehensionHigh,
		J17TierComprehensionMinSamples: j17TierComprehensionMinSamples,
		J17TrustTTLHours:               j17TrustTTLHours,
		J17TrustMinFeedbackCount:       j17TrustMinFeedbackCount,
		J17TrustMode:                   j17TrustMode,
		J17TrustEMAAlpha:               j17TrustEMAAlpha,
		J17BootstrapCodification:       j17BootstrapCodification,
		J17BootstrapSpaceID:            j17BootstrapSpaceID,
		J17MetricsEnabled:              j17MetricsEnabled,
		J17CodificationThreshold:       j17CodificationThreshold,
		J17ComprehensionMinThreshold:   j17ComprehensionMinThreshold,
		J17CompressionMinRatio:         j17CompressionMinRatio,
		J17ReplayFrequencyMax:          j17ReplayFrequencyMax,
		J17NLIComprehensionEnabled:     j17NLIComprehensionEnabled,
		J17ProtocolDataCollection:      j17ProtocolDataCollection,
		J17T1ComprehensionGate:         j17T1ComprehensionGate,
		J17ExtensionsEnabled:           j17ExtensionsEnabled,
		J17AllowedExtensions:           j17AllowedExtensions,
		J17MLTierPredictionEnabled:     j17MLTierPredictionEnabled,
		J17SidecarURL:                  j17SidecarURL,
		J17SidecarTimeoutMs:            j17SidecarTimeoutMs,
		J17SidecarMode:                 j17SidecarMode,
		J17SidecarCanaryPct:            j17SidecarCanaryPct,
		J17SidecarConfidenceFloor:      j17SidecarConfidenceFloor,
		J17NLIScoreOfRecord:            j17NLIScoreOfRecord,
		J17PrecedentProtectedCodes:     j17PrecedentProtectedCodes,
		J17PrecedentLogEnabled:         j17PrecedentLogEnabled,
		J17SidecarCBEnabled:            j17SidecarCBEnabled,
		J17SidecarCBFailureThreshold:   j17SidecarCBFailureThreshold,
		J17SidecarCBTimeoutSec:         j17SidecarCBTimeoutSec,
		J17NLIObservationalEnabled:     j17NLIObservationalEnabled,
		J17TierEffectivenessMinSamples: j17TierEffectivenessMinSamples,
		J17TierIneffectiveThreshold:    j17TierIneffectiveThreshold,
		J17TierDriftDetectionEnabled:   j17TierDriftDetectionEnabled,
		J17NLICalibrationWindowSize:    j17NLICalibrationWindowSize,
		J17NLICalibrationBiasThreshold: j17NLICalibrationBiasThreshold,
		J17NLICalibrationMinSamples:    j17NLICalibrationMinSamples,
		J17CompressionTargetRatio:      j17CompressionTargetRatio,

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

		GapLowScoreThreshold:    gapLowScoreThreshold,
		GapMinOccurrences:       gapMinOccurrences,
		GapAnalysisWindowHours:  gapAnalysisWindowHours,
		GapMetricsWindowSize:    gapMetricsWindowSize,
		CompressionEnabled:      compressionEnabled,
		CompressionMinSize:      compressionMinSize,
		PaginationMaxLimit:      paginationMaxLimit,
		PaginationDefLimit:      paginationDefLimit,
		Neo4jMaxPoolSize:        neo4jMaxPoolSize,
		Neo4jAcquireTimeoutSec:  neo4jAcquireTimeout,
		Neo4jMaxConnLifetimeSec: neo4jMaxConnLifetime,
		Neo4jConnIdleTimeoutSec: neo4jConnIdleTimeout,
		PortRangeStart:          portRangeStart,
		PortRangeEnd:            portRangeEnd,
		PortFilePath:            portFilePath,
		SyncIntervalMinutes:     syncIntervalMinutes,
		SyncSpaceIDs:            syncSpaceIDs,
		SyncStaleThresholdHours: syncStaleThresholdHours,
		SyncRepoPathMap:         syncRepoPathMap,
		APEIngestSyncEnabled:    apeIngestSyncEnabled,

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
		RSICMicroEnabled:                     rsicMicroEnabled,
		RSICMesoPeriodHours:                  rsicMesoPeriodHours,
		RSICMesoPeriodSessions:               rsicMesoPeriodSessions,
		RSICMacroCron:                        rsicMacroCron,
		RSICMaxNodePrunePct:                  rsicMaxNodePrunePct,
		RSICMaxEdgePrunePct:                  rsicMaxEdgePrunePct,
		RSICRollbackWindow:                   rsicRollbackWindow,
		RSICWatchdogEnabled:                  rsicWatchdogEnabled,
		RSICWatchdogCheckSec:                 rsicWatchdogCheckSec,
		RSICGuidanceEffectivenessWindowHours: rsicGuidanceEffectivenessWindowHours,
		RSICWatchdogDecayRate:                rsicWatchdogDecayRate,
		RSICNudgeThreshold:                   rsicNudgeThreshold,
		RSICWarnThreshold:                    rsicWarnThreshold,
		RSICForceThreshold:                   rsicForceThreshold,
		RSICCalibrationDays:                  rsicCalibrationDays,
		RSICMaxHistoryEntries:                rsicMaxHistoryEntries,
		RSICMinConfidence:                    rsicMinConfidence,
		RSICTriggerCooldownSec:               rsicTriggerCooldownSec,
		RSICTriggerDedupeSec:                 rsicTriggerDedupeSec,
		RSICWatchdogSpaceID:                  rsicWatchdogSpaceID,
		RSICPersistenceEnabled:               rsicPersistenceEnabled,
		RSICProtectedSpaces:                  rsicProtectedSpaces,
		RSICMacroCronSpace:                   rsicMacroCronSpace,
		RSICMinActionConfidence:              rsicMinActionConfidence,
		RSICGuidanceCalibrationEnabled:       rsicGuidanceCalibrationEnabled,
		RSICGuidanceMinSurfaces:              rsicGuidanceMinSurfaces,
		RSICGuidanceBoostThreshold:           rsicGuidanceBoostThreshold,
		RSICGuidanceDecayThreshold:           rsicGuidanceDecayThreshold,
		RSICGuidanceDecayMinSurfaces:         rsicGuidanceDecayMinSurfaces,
		SpacePruneIntervalHours:              spacePruneIntervalHours,
		ContextCoolerEnabled:                 contextCoolerEnabled,
		WeeklyGapInterviewsEnabled:           weeklyGapInterviewsEnabled,

		// Phase AR-3: LLM-powered intelligence
		RSICLLMReflectEnabled:   rsicLLMReflectEnabled,
		RSICLLMReflectProvider:  rsicLLMReflectProvider,
		RSICLLMReflectModel:     rsicLLMReflectModel,
		RSICLLMReflectCompress:  rsicLLMReflectCompress,
		RSICLLMReflectTimeoutMs: rsicLLMReflectTimeoutMs,

		RSICLLMReflectPromptBudgetTokens: rsicLLMReflectPromptBudgetTokens,
		RSICLLMReflectHistoryCycles:      rsicLLMReflectHistoryCycles,
		RSICLLMReflectIncludeDatasets:    rsicLLMReflectIncludeDatasets,
		RSICLLMConcurrencyLimit:          rsicLLMConcurrencyLimit,
		ConflictTrackerEnabled:           conflictTrackerEnabled,

		// Phase 11.6.3 — MLX Watchdog
		MLXWatchdogEnabled:  mlxWatchdogEnabled,
		MLXProbeIntervalSec: mlxProbeIntervalSec,
		MLXProbeTimeoutSec:  mlxProbeTimeoutSec,
		MLXFailFastEnabled:  mlxFailFastEnabled,

		// Phase 13 — Column-Voting Retrieval
		RetrievalColumnVotingEnabled:     retrievalColumnVotingEnabled,
		RetrievalGraphTypedEdgesEnabled:  retrievalGraphTypedEdgesEnabled,
		RetrievalRRFK:                    retrievalRRFK,
		RetrievalColumnTimeoutFrac:       retrievalColumnTimeoutFrac,
		RetrievalStructuralHops:          retrievalStructuralHops,
		RetrievalColumnEmbeddingEnabled:  retrievalColEmbeddingEnabled,
		RetrievalColumnBM25Enabled:       retrievalColBM25Enabled,
		RetrievalColumnGraphEnabled:      retrievalColGraphEnabled,
		RetrievalColumnStructuralEnabled: retrievalColStructuralEnabled,

		// Phase 13.1 — per-column RRF weights (ablation knobs)
		RetrievalColumnWeightEmbedding:  retrievalColumnWeightEmbedding,
		RetrievalColumnWeightBM25:       retrievalColumnWeightBM25,
		RetrievalColumnWeightGraph:      retrievalColumnWeightGraph,
		RetrievalColumnWeightStructural: retrievalColumnWeightStructural,
		RetrievalAuditEnabled:           retrievalAuditEnabled,

		// EVENTGRAPH-001 — TSDB reinforcement_events + federation API
		GuidanceCorpusEnabled:                          guidanceCorpusEnabled,
		JiminyContradictedBridgeEnabled:                jiminyContradictedBridgeEnabled,
		JiminyContradictedBridgeWriterFlushIntervalSec: jiminyContradictedBridgeWriterFlushIntervalSec,
		JiminyContradictedBridgeWriterBufferSize:       jiminyContradictedBridgeWriterBufferSize,
		JiminyContradictedBridgeMaxContentLen:          jiminyContradictedBridgeMaxContentLen,
		GuidanceCorpusWriterFlushIntervalSec:           guidanceCorpusWriterFlushIntervalSec,
		GuidanceCorpusWriterBufferSize:                 guidanceCorpusWriterBufferSize,
		GuidanceCorpusMaxContentBytes:                  guidanceCorpusMaxContentBytes,
		GuidanceCorpusSourceLookupTimeoutMs:            guidanceCorpusSourceLookupTimeoutMs,
		GuidanceAuditEnabled:                           guidanceAuditEnabled,
		GuidanceAuditIntervalHours:                     guidanceAuditIntervalHours,
		GuidanceAuditSampleSize:                        guidanceAuditSampleSize,
		GuidanceAuditInitialDelaySec:                   guidanceAuditInitialDelaySec,
		GuidanceShouldFollowRateFloor:                  guidanceShouldFollowRateFloor,
		GuidanceShouldFollowLookbackHours:              guidanceShouldFollowLookbackHours,
		ReviewEnabled:                                  reviewEnabled,
		ReviewWriterFlushIntervalSec:                   reviewWriterFlushIntervalSec,
		ReviewWriterBufferSize:                         reviewWriterBufferSize,
		ReviewRubricVersion:                            reviewRubricVersion,
		ReviewSampleSize:                               reviewSampleSize,
		ReviewActiveUncertaintyBand:                    reviewActiveUncertaintyBand,
		ReviewSampleSeed:                               reviewSampleSeed,
		ReviewReinforceDefault:                         reviewReinforceDefault,
		ReviewGuidanceSinkEnabled:                      reviewGuidanceSinkEnabled,
		ReviewLLMDatasetsEnabled:                       reviewLLMDatasetsEnabled,
		ReviewContradictedDatasetEnabled:               reviewContradictedDatasetEnabled,
		ReviewGuidanceConfidenceNudge:                  reviewGuidanceConfidenceNudge,

		EventGraphEnabled:                        eventGraphEnabled,
		EventGraphWriterFlushIntervalSec:         eventGraphWriterFlushIntervalSec,
		EventGraphWriterBufferSize:               eventGraphWriterBufferSize,
		EventGraphMaxPairsPerEventBatch:          eventGraphMaxPairsPerEventBatch,
		EventGraphMaxEventsPerQuery:              eventGraphMaxEventsPerQuery,
		EventGraphFederationDefaultHops:          eventGraphFederationDefaultHops,
		EventGraphFederationDefaultLookbackHours: eventGraphFederationDefaultLookbackHours,

		// Phase 14 Epic 1 — Note 06 sparse activation gate
		SparseRetrievalEnabled:     sparseRetrievalEnabled,
		SparseActivationPercentile: sparseActivationPercentile,
		SparseMinActive:            sparseMinActive,
		SparseMaxActive:            sparseMaxActive,

		// Phase 14.1 Epic 1 — Per-category overrides
		SparseGateCategoryOverrides: sparseGateCategoryOverrides,

		// Phase 14.2 Epic 1+2 — Note 05 sparse fingerprints + adaptive catalog
		ContextFingerprintEnabled:              contextFingerprintEnabled,
		ContextFingerprintBitBudget:            contextFingerprintBitBudget,
		ContextFingerprintRefreshEnabled:       contextFingerprintRefreshEnabled,
		ContextFingerprintRefreshIntervalHours: contextFingerprintRefreshIntervalHours,
		ContextFingerprintRefreshTimeoutMs:     contextFingerprintRefreshTimeoutMs,
		ContextFingerprintHealMaxPerCycle:      contextFingerprintHealMaxPerCycle,
		QueryClassifyCategoryMap:               queryClassifyCategoryMap,
		ContextQueryAutoDefault:                contextQueryAutoDefault,
		ContextFingerprintRefineMaxPerCycle:    contextFingerprintRefineMaxPerCycle,
		ContextCatalogTopNPaths:                contextCatalogTopNPaths,
		ContextCatalogTopNTags:                 contextCatalogTopNTags,
		ContextCatalogFloorBitsPerKind:         contextCatalogFloorBitsPerKind,
		ContextCatalogRoleTypeLayerBits:        contextCatalogRoleTypeLayerBits,
		RetrievalContextColumnEnabled:          retrievalContextColumnEnabled,
		RetrievalContextColumnWeight:           retrievalContextColumnWeight,
		RetrievalContextStrictThreshold:        retrievalContextStrictThreshold,
		ContextFingerprintQueryTopK:            contextFingerprintQueryTopK,
		RetrievalContextColumnCategoryWeights:  retrievalContextColumnCategoryWeights,

		ConsultingLLMConstraintsEnabled:     consultingLLMConstraintsEnabled,
		ConsultingLLMConstraintsProvider:    consultingLLMConstraintsProvider,
		ConsultingLLMConstraintsModel:       consultingLLMConstraintsModel,
		ConsultingClassifyTimeoutMs:         consultingClassifyTimeoutMs,
		ConsultingClassifyConcurrency:       consultingClassifyConcurrency,
		ConsultingConstraintScoreFloor:      consultingConstraintScoreFloor,
		ConsultingSuggestMinConfidence:      consultingSuggestMinConfidence,
		MCPReflectScoreHigh:                 mcpReflectScoreHigh,
		MCPReflectScoreMedium:               mcpReflectScoreMedium,
		GuardrailConstraintSimFloor:         guardrailConstraintSimFloor,
		ConsultingAuthorityScoreFloor:       consultingAuthorityScoreFloor,
		ConsultingConflictScoreFloor:        consultingConflictScoreFloor,
		RetrievalConfidenceSigmoidMidpoint:  retrievalConfidenceSigmoidMidpoint,
		RetrievalConfidenceSigmoidSteepness: retrievalConfidenceSigmoidSteepness,

		// Phase 38: UNTS Hash Verification
		UNTSEnabled:  untsEnabled,
		UNTSBasePath: untsBasePath,

		CoolerReinforcementWindowHours:  coolerReinfWindowHours,
		CoolerStabilityIncreasePerReinf: coolerStabilityIncrease,
		CoolerStabilityDecayRate:        coolerDecayRate,
		CoolerTombstoneThreshold:        coolerTombstoneThresh,
		CoolerTombstoneMaxPerRun:        coolerTombstoneMaxPerRun,
		CoolerGraduationThreshold:       coolerGradThresh,

		ConstraintDetectionEnabled: constraintDetectionEnabled,
		ConstraintMinConfidence:    constraintMinConfidence,
		ConstraintProtectFromDecay: constraintProtectFromDecay,

		ConstraintPromotionGateEnabled:    constraintPromotionGateEnabled,
		ConstraintPromotionDenyObsTypes:   constraintPromotionDenyObsTypes,
		ConstraintPromotionRejectPatterns: constraintPromotionRejectPatterns,
		CorrectionPromotionEnabled:        correctionPromotionEnabled,
		CorrectionPromotionMinContentLen:  correctionPromotionMinContentLen,
		CorrectionPromotionRejectPatterns: correctionPromotionRejectPatterns,

		GraduatedDecayProtectionFactor: graduatedDecayProtectionFactor,

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
		BackupEnabled:                backupEnabled,
		BackupStorageDir:             backupStorageDir,
		BackupFullCmd:                backupFullCmd,
		BackupNeo4jContainer:         backupNeo4jContainer,
		BackupFullIntervalHours:      backupFullIntervalHours,
		BackupPartialIntervalHours:   backupPartialIntervalHours,
		BackupRetentionFullCount:     backupRetentionFullCount,
		BackupRetentionPartialCount:  backupRetentionPartialCount,
		BackupRetentionMaxAgeDays:    backupRetentionMaxAgeDays,
		BackupRetentionMaxStorageGB:  backupRetentionMaxStorageGB,
		BackupRetentionRunAfter:      backupRetentionRunAfter,
		BackupSnapshotWaitTimeoutSec: backupSnapshotWaitTimeoutSec,
		BackupJobStalenessHours:      backupJobStalenessHours,
		BackupInitialDelayMin:        backupInitialDelayMin,

		// TimescaleDB Backup & Restore
		TSDBBackupEnabled:             tsdbBackupEnabled,
		TSDBBackupStorageDir:          tsdbBackupStorageDir,
		TSDBBackupComposeFile:         tsdbBackupComposeFile,
		TSDBBackupServiceName:         tsdbBackupServiceName,
		TSDBBackupIntervalHours:       tsdbBackupIntervalHours,
		TSDBBackupInitialDelayMin:     tsdbBackupInitialDelayMin,
		TSDBBackupRetentionCount:      tsdbBackupRetentionCount,
		TSDBBackupRetentionMaxAgeDays: tsdbBackupRetentionMaxAgeDays,

		// Phase 75: Relationship Extraction & Topology Hardening
		RelExtractImports:        relExtractImports,
		RelExtractInheritance:    relExtractInheritance,
		RelExtractCalls:          relExtractCalls,
		RelCrossFileResolve:      relCrossFileResolve,
		RelMaxCallsPerFunc:       relMaxCallsPerFunc,
		RelBatchSize:             relBatchSize,
		RelResolutionTimeout:     relResolutionTimeout,
		DynamicEdgesEnabled:      dynamicEdgesEnabled,
		DynamicEdgeDegreeCap:     dynamicEdgeDegreeCap,
		DynamicEdgeMinConfidence: dynamicEdgeMinConfidence,
		DynamicEdgeMinLayer:      dynamicEdgeMinLayer,
		DynamicEdgeTopK:          dynamicEdgeTopK,
		DynamicEdgeSimThreshold:  dynamicEdgeSimThreshold,
		DynamicEdgeOversample:    dynamicEdgeOversample,
		L5EmergentEnabled:        l5EmergentEnabled,
		L5BridgeEvidenceMin:      l5BridgeEvidenceMin,
		L5SourceMinLayer:         l5SourceMinLayer,
		SymbolActivationEnabled:  symbolActivationEnabled,

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
		L5GroundingMaxEdges:        l5GroundingMaxEdges,
		L5GroundingMinSim:          l5GroundingMinSim,
		L5GroundingInitialWeight:   l5GroundingInitialWeight,
		EdgeAttentionGroundedBy:    edgeAttentionGroundedBy,
		EdgeAttentionAnalogousTo:   edgeAttentionAnalogousTo,
		EdgeAttentionBridges:       edgeAttentionBridges,
		EdgeAttentionComposesWith:  edgeAttentionComposesWith,
		EdgeAttentionContrastsWith: edgeAttentionContrastsWith,
		EdgeAttentionInfluences:    edgeAttentionInfluences,
		EdgeAttentionDefinesSymbol: edgeAttentionDefinesSymbol,
		EdgeAttentionThemeOf:       edgeAttentionThemeOf,

		// ANN Optimization: Cluster Summary
		ClusterSummaryEnabled:   clusterSummaryEnabled,
		ClusterSummaryProvider:  clusterSummaryProvider,
		ClusterSummaryModel:     clusterSummaryModel,
		ClusterSummaryMaxTokens: clusterSummaryMaxTokens,
		ClusterSummaryTimeoutMs: clusterSummaryTimeoutMs,
		ClusterSummaryBatchSize: clusterSummaryBatchSize,

		// ANN Optimization: Negative Feedback + Frontier
		LearningNegativeWeight:        learningNegativeWeight,
		LearningNegativeMaxPerRequest: learningNegativeMaxPerRequest,
		FrontierMinEvidence:           frontierMinEvidence,
		FrontierMaxOutgoing:           frontierMaxOutgoing,

		// FSD-2026-001: Constraint Lifecycle Closure
		ContradictionEnabled:               contradictionEnabled,
		ContradictionSimThreshold:          contradictionSimThreshold,
		ContradictionMaxCandidates:         contradictionMaxCandidates,
		ContradictionNLIEnabled:            contradictionNLIEnabled,
		JiminyPersistenceEnabled:           jiminyPersistenceEnabled,
		ConstraintConfidenceDecayPerNeg:    constraintConfDecayPerNeg,
		ConstraintConfidenceBoostPerPos:    constraintConfBoostPerPos,
		ConstraintArchiveThreshold:         constraintArchiveThreshold,
		ConstraintConflictDetectionEnabled: constraintConflictDetectionEnabled,
		ConstraintConflictSimThreshold:     constraintConflictSimThreshold,
		ConstraintConflictMaxPairs:         constraintConflictMaxPairs,
		ConstraintClassifierGateEnabled:    constraintClassifierGateEnabled,
		ConstraintNLIEnabled:               constraintNLIEnabled,
		ConstraintScopeFilteringEnabled:    constraintScopeFilteringEnabled,
		LearningAsymmetricEnabled:          learningAsymmetricEnabled,
		DeterminismScoringEnabled:          determinismScoringEnabled,
		JiminyCacheEnabled:                 jiminyCacheEnabled,
		JiminyCacheTTLSec:                  jiminyCacheTTLSec,
		JiminyCacheSize:                    jiminyCacheSize,
		JiminyCacheJ17Bypass:               jiminyCacheJ17Bypass,
		ActivationDimSemanticWeight:        activationDimSemanticWeight,
		ActivationDimTemporalWeight:        activationDimTemporalWeight,
		ActivationDimCoactivationWeight:    activationDimCoactivationWeight,
		MaxHopDepth:                        maxHopDepth,
		LearningAutoPruneExcessEnabled:     learningAutoPruneExcessEnabled,
		ConstraintAuthorityEnabled:         constraintAuthorityEnabled,
		ConstraintDefaultAuthority:         constraintDefaultAuthority,

		// Neural Re-Ranker
		NeuralDataCollection:  neuralDataCollection,
		NeuralDataDir:         neuralDataDir,
		SidecarEnabled:        sidecarEnabled,
		NeuralRerankEnabled:   neuralRerankEnabled,
		NeuralRerankURL:       neuralRerankURL,
		NeuralRerankTimeoutMs: neuralRerankTimeoutMs,
		NeuralRerankFallback:  neuralRerankFallback,

		// Synergy
		SynergyMemoryLineThreshold:    synergyMemoryLineThreshold,
		SynergyClaudeMDPath:           synergyClaudeMDPath,
		SynergyMemoryMDPath:           synergyMemoryMDPath,
		SynergyAssessmentEnabled:      synergyAssessmentEnabled,
		SynergyTargetClaudeLines:      synergyTargetClaudeLines,
		SynergyTargetMemoryLines:      synergyTargetMemoryLines,
		SynergyOverlapSampleSize:      synergyOverlapSampleSize,
		SynergyOverflowAlertThreshold: synergyOverflowAlertThreshold,
		SynergyRecoveryBufferSpace:    synergyRecoveryBufferSpace,
		SynergyRecoveryBufferPath:     synergyRecoveryBufferPath,

		// RSIC overall-health weights (DH-005)
		RSICHealthWeightRetrieval: rsicHealthWeightRetrieval,
		RSICHealthWeightMemory:    rsicHealthWeightMemory,
		RSICHealthWeightEdge:      rsicHealthWeightEdge,
		RSICHealthWeightTask:      rsicHealthWeightTask,
		RSICHealthWeightGuidance:  rsicHealthWeightGuidance,
		RSICHealthWeightProtocol:  rsicHealthWeightProtocol,
		RSICHealthWeightSynergy:   rsicHealthWeightSynergy,

		RSICEdgeEntropyFloor: rsicEdgeEntropyFloor,

		FTBenchStalenessDays: ftBenchStalenessDays,

		// TimescaleDB
		TSDBEnabled:               tsdbEnabled,
		TSDBHost:                  tsdbHost,
		TSDBPort:                  tsdbPort,
		TSDBUser:                  tsdbUser,
		TSDBPassword:              tsdbPassword,
		TSDBDatabase:              tsdbDatabase,
		TSDBSSLMode:               tsdbSSLMode,
		TSDBMaxConns:              tsdbMaxConns,
		TSDBFlushIntervalSec:      tsdbFlushIntervalSec,
		TSDBRawRetentionDays:      tsdbRawRetentionDays,
		TSDBHourlyRetentionDays:   tsdbHourlyRetentionDays,
		TSDBRequiredSchemaVersion: tsdbRequiredSchemaVersion,
		TSDBOptional:              tsdbOptional,
		InstanceID:                instanceID,
		LLMInteractionLogging:     llmInteractionLogging,
		EmbeddingEventLogging:     embeddingEventLogging,
		RetrievalEventLogging:     retrievalEventLogging,

		// Live Metrics
		LiveMetricsEnabled:     liveMetricsEnabled,
		LiveGuidanceRefreshSec: liveGuidanceRefreshSec,

		// Alert Dispatcher
		AlertEnabled:           alertEnabled,
		AlertFilePath:          alertFilePath,
		AlertCooldownSec:       alertCooldownSec,
		AlertMaxEntries:        alertMaxEntries,
		AlertMacOSNotify:       alertMacOSNotify,
		AlertMacOSNotifyMinSev: alertMacOSNotifyMinSev,

		// Health Probe
		HealthProbeEnabled:     healthProbeEnabled,
		HealthProbeIntervalSec: healthProbeIntervalSec,

		// Goroutine Supervisor (SUPERVISOR-002)
		SupervisorMaxRestarts:      supervisorMaxRestarts,
		SupervisorRestartWindowMin: supervisorRestartWindowMin,
		SupervisorBackoffBaseSec:   supervisorBackoffBaseSec,
		AlertRuleFailureThreshold:  alertRuleFailureThreshold,
		RSICLLMErrorRecencyMin:     rsicLLMErrorRecencyMin,
		RSICLLMErrorMinCount:       rsicLLMErrorMinCount,

		// LLM Client Retry
		LLMRetryEnabled:     llmRetryEnabled,
		LLMRetryMaxAttempts: llmRetryMaxAttempts,
		LLMRetryBaseDelayMs: llmRetryBaseDelayMs,
		LLMRetryMaxDelayMs:  llmRetryMaxDelayMs,
		LLMRetryMultiplier:  llmRetryMultiplier,
		LLMRetryJitter:      llmRetryJitter,
		LLMRetryOnDeadline:  llmRetryOnDeadline,

		// LLM Consecutive Failure Alert
		LLMConsecutiveFailureThreshold: llmConsecutiveFailureThreshold,

		// Alert Evaluator
		JobHealthAlertEnabled:               jobHealthAlertEnabled,
		HookHealthAlertEnabled:              hookHealthAlertEnabled,
		HookSilentLookbackHours:             hookSilentLookbackHours,
		HookActivityMinEvents:               hookActivityMinEvents,
		NullWeightEdgeAlertThreshold:        nullWeightEdgeAlertThreshold,
		OrphanRatioMinNodes:                 orphanRatioMinNodes,
		OrphanRatioThreshold:                orphanRatioThreshold,
		Neo4jCPUAlertThresholdPercent:       neo4jCPUAlertThresholdPercent,
		OrphanCountThreshold:                orphanCountThreshold,
		GraphHealthScoreFloor:               graphHealthScoreFloor,
		FtReadinessStalenessMin:             ftReadinessStalenessMin,
		ExportRetentionHours:                exportRetentionHours,
		TrainingReadinessThreshold:          trainingReadinessThreshold,
		TrainingReadinessThresholdOverrides: trainingReadinessThresholdOverrides,
		FtLoopEnabled:                       ftLoopEnabled,
		FtLoopMinRetrainIntervalHours:       ftLoopMinRetrainIntervalHours,
		FtLoopMinFreshFraction:              ftLoopMinFreshFraction,
		FtLoopPollIntervalSec:               ftLoopPollIntervalSec,
		FtLoopLeaseMaxHours:                 ftLoopLeaseMaxHours,
		FtLoopLeasePath:                     ftLoopLeasePath,
		FtLoopMinFreeDiskGB:                 ftLoopMinFreeDiskGB,
		FtLoopPythonBin:                     ftLoopPythonBin,
		FtLoopModelVersion:                  ftLoopModelVersion,
		FtLoraEpochsCap:                     ftLoraEpochsCap,
		FtEarlyStopValLossFactor:            ftEarlyStopValLossFactor,
		FtLoopWorkDir:                       ftLoopWorkDir,
		FtLoopBaseModel:                     ftLoopBaseModel,
		FtLoopBaseSHA:                       ftLoopBaseSHA,
		FtLoopUaitsSpec:                     ftLoopUaitsSpec,
		FtLoopBenchmarkConfig:               ftLoopBenchmarkConfig,
		FtLoopLoraRank:                      ftLoopLoraRank,
		FtLoopLoraAlpha:                     ftLoopLoraAlpha,
		FtLoopGatePort:                      ftLoopGatePort,
		FtLoopExportSinceDays:               ftLoopExportSinceDays,
		FtLoopGateTaskFilter:                ftLoopGateTaskFilter,
		FtLoopGateMinScore:                  ftLoopGateMinScore,
		FtLoopConvertScript:   get("FT_LOOP_CONVERT_SCRIPT", ""),
		FtLoopCanaryEnabled:    getBool("FT_LOOP_CANARY_ENABLED", false),
		FtLoopCanaryProbes:     get("FT_LOOP_CANARY_PROBES", "training_data/eval/valid_clean.jsonl"),
		FtLoopCanaryProbeCount: canaryProbeCount,
		FtLoopCanaryProdURL:    get("FT_LOOP_CANARY_PROD_URL", "http://127.0.0.1:8102/v1"),
		FtLoopServingSymlink:       get("FT_LOOP_SERVING_SYMLINK", ".local-models/serving/current.gguf"),
		FtLoopServingPlistLabel:    get("FT_LOOP_SERVING_PLIST_LABEL", "com.mdemg.llama-server"),
		FtLoopServingHealthURL:     get("FT_LOOP_SERVING_HEALTH_URL", "http://127.0.0.1:8102/health"),
		FtLoopSwapHealthTimeoutSec: swapHealthTimeout,
		MaintLiveAlertEnabled:               maintLiveAlertEnabled,
		MaintLiveLookbackDays:               maintLiveLookbackDays,
		JobBackupStalenessHours:             jobBackupStalenessHours,
		JobFailureLookbackMin:               jobFailureLookbackMin,
		AlertEvaluatorEnabled:               alertEvaluatorEnabled,
		AlertEvaluatorIntervalSec:           alertEvaluatorIntervalSec,

		// TSDB-CONSUME-001 — retrieve-latency SLO rules + writer flush health
		AlertRetrieveP95Ms:               alertRetrieveP95Ms,
		AlertRetrieveP99Ms:               alertRetrieveP99Ms,
		AlertRetrieveLatencyLookbackMin:  alertRetrieveLatencyLookbackMin,
		TSDBWriterAlertLookbackMin:       tsdbWriterAlertLookbackMin,
		ScorerChangeLookbackHours:        scorerChangeLookbackHours,
		ConsensusShiftThreshold:          consensusShiftThreshold,
		ConsensusShiftRecentHours:        consensusShiftRecentHours,
		ConsensusShiftBaselineDays:       consensusShiftBaselineDays,
		ConsensusShiftMinSamples:         consensusShiftMinSamples,
		EmergenceCycleAlertThresholdSec:  emergenceCycleAlertThresholdSec,
		EmergenceCycleAlertLookbackMin:   emergenceCycleAlertLookbackMin,
		SurpriseEmbeddingNoveltyTopK:     surpriseEmbeddingNoveltyTopK,
		SurpriseEmbeddingNoveltySimFloor: surpriseEmbeddingNoveltySimFloor,
		SurpriseFactorHighThreshold:      surpriseFactorHighThreshold,
		SurpriseFactorMediumThreshold:    surpriseFactorMediumThreshold,

		// TSDB Writer
		TSDBWriterBufferMaxSize: tsdbWriterBufferMaxSize,
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

// Validate checks cross-field constraints that individual parsing can't catch.
// Returns warnings (non-fatal) as a slice and an error if anything is critically wrong.
func (c Config) Validate() (warnings []string, err error) {
	// JIMINY-BUDGET-001: an explicit direct-Guide budget shorter than the
	// warm-compute budget re-creates the fresh-install starvation class the
	// derivation default exists to kill. Advisory, not fatal.
	if c.JiminyTimeoutMs > 0 && c.JiminyTimeoutMs < c.JiminyWarmComputeTimeoutMs {
		slog.Warn("config: JIMINY_TIMEOUT_MS is set below JIMINY_WARM_COMPUTE_TIMEOUT_MS — direct Guide() calls get a tighter budget than warm pre-compute; synthesis (~50s live) may starve",
			"jiminy_timeout_ms", c.JiminyTimeoutMs,
			"warm_compute_timeout_ms", c.JiminyWarmComputeTimeoutMs)
	}
	var errs []error

	// Weight groups that should sum to ~1.0 (tolerance: 0.01)
	checkWeightSum := func(name string, vals ...float64) {
		sum := 0.0
		for _, v := range vals {
			sum += v
		}
		if math.Abs(sum-1.0) > 0.01 {
			warnings = append(warnings, fmt.Sprintf("%s weights sum to %.3f, expected ~1.0", name, sum))
		}
	}

	// Scoring weights: Alpha + Beta + Gamma + Delta = 1.0
	checkWeightSum("Scoring(Alpha+Beta+Gamma+Delta)",
		c.ScoringAlpha, c.ScoringBeta, c.ScoringGamma, c.ScoringDelta)

	// Hidden layer forward: Alpha + Beta = 1.0
	checkWeightSum("HiddenLayerForward(Alpha+Beta)",
		c.HiddenLayerForwardAlpha, c.HiddenLayerForwardBeta)

	// Hidden layer backward: Self + Base + Conc = 1.0
	checkWeightSum("HiddenLayerBackward(Self+Base+Conc)",
		c.HiddenLayerBackwardSelf, c.HiddenLayerBackwardBase, c.HiddenLayerBackwardConc)

	// Activation dimension weights: Semantic + Temporal + Coactivation = 1.0
	// Only validate if any are non-zero (zero = unconfigured, defaults applied later)
	if c.ActivationDimSemanticWeight > 0 || c.ActivationDimTemporalWeight > 0 || c.ActivationDimCoactivationWeight > 0 {
		checkWeightSum("ActivationDim(Semantic+Temporal+Coactivation)",
			c.ActivationDimSemanticWeight, c.ActivationDimTemporalWeight, c.ActivationDimCoactivationWeight)
	}

	// Hybrid retrieval: BM25 + Vector = 1.0
	if c.HybridRetrievalEnabled {
		checkWeightSum("HybridRetrieval(BM25+Vector)",
			c.BM25Weight, c.VectorWeight)
	}

	// JIMINY-CORPUS-001 Epic 4: the not_applicable relevance gate must sit at or
	// below the LOW threshold — it carves the truly-unrelated tail out of the
	// sub-LOW band; above LOW it would eat the tier-2 (LOW..HIGH) uncertain band.
	// The classifier clamps to LOW at runtime; warn so the operator sees it.
	if c.JiminyOutcomeNotApplicableSim > c.JiminyOutcomeSimilarityLow {
		warnings = append(warnings, fmt.Sprintf(
			"JIMINY_OUTCOME_NOT_APPLICABLE_SIMILARITY (%.3f) > JIMINY_OUTCOME_SIMILARITY_LOW (%.3f); classifier clamps the gate to LOW",
			c.JiminyOutcomeNotApplicableSim, c.JiminyOutcomeSimilarityLow))
	}

	// DH-005: RSIC overall-health weights. No sum constraint — the formula
	// normalises — but all-zero is meaningless (overall_health would always
	// be 0). Warn loudly so the operator knows the RSIC signal is disabled.
	rsicSum := c.RSICHealthWeightRetrieval + c.RSICHealthWeightMemory +
		c.RSICHealthWeightEdge + c.RSICHealthWeightTask +
		c.RSICHealthWeightGuidance + c.RSICHealthWeightProtocol +
		c.RSICHealthWeightSynergy
	if rsicSum <= 0 {
		warnings = append(warnings, "All RSIC_HEALTH_WEIGHT_* are zero; overall_health will always be 0. "+
			"Either set at least one weight or unset the env vars to use defaults.")
	}

	// Bound checks
	if c.DefaultTopK > c.DefaultCandidateK && c.DefaultCandidateK > 0 {
		warnings = append(warnings, fmt.Sprintf("DefaultTopK (%d) > DefaultCandidateK (%d)", c.DefaultTopK, c.DefaultCandidateK))
	}
	if c.DefaultHopDepth > c.MaxHopDepth && c.MaxHopDepth > 0 {
		warnings = append(warnings, fmt.Sprintf("DefaultHopDepth (%d) > MaxHopDepth (%d)", c.DefaultHopDepth, c.MaxHopDepth))
	}
	if c.LearningWMin >= c.LearningWMax {
		errs = append(errs, fmt.Errorf("LearningWMin (%.2f) must be < LearningWMax (%.2f)", c.LearningWMin, c.LearningWMax))
	}

	// EdgeTypeStrategy validation
	validStrategies := map[string]bool{"all": true, "structural_first": true, "learned_only": true, "hybrid": true}
	if c.EdgeTypeStrategy != "" && !validStrategies[c.EdgeTypeStrategy] {
		errs = append(errs, fmt.Errorf("invalid EdgeTypeStrategy %q (valid: all, structural_first, learned_only, hybrid)", c.EdgeTypeStrategy))
	}

	// RSIC cross-field validation
	if c.RSICMacroCron != "" || c.RSICMicroEnabled {
		// Warn if macro cron space is in protected spaces (would silently block all macro cycles)
		for _, ps := range c.RSICProtectedSpaces {
			if ps == c.RSICMacroCronSpace {
				warnings = append(warnings, fmt.Sprintf("RSICMacroCronSpace %q is in RSICProtectedSpaces — destructive macro cycle actions will be blocked", ps))
				break
			}
		}
		// Warn if watchdog space differs from macro cron space (likely misconfiguration)
		if c.RSICWatchdogSpaceID != c.RSICMacroCronSpace && c.RSICWatchdogSpaceID != "" && c.RSICMacroCronSpace != "" {
			warnings = append(warnings, fmt.Sprintf("RSICWatchdogSpaceID %q != RSICMacroCronSpace %q — watchdog monitors a different space than macro cron targets", c.RSICWatchdogSpaceID, c.RSICMacroCronSpace))
		}
		// Warn if no protected spaces configured with cycles enabled
		if len(c.RSICProtectedSpaces) == 0 {
			warnings = append(warnings, "RSICProtectedSpaces is empty with RSIC cycles enabled — destructive actions have no space protection")
		}
	}

	if len(errs) > 0 {
		err = errors.Join(errs...)
	}
	return warnings, err
}
