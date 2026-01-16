package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr           string
	Neo4jURI             string
	Neo4jUser            string
	Neo4jPass            string
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

	// Embedding provider settings
	EmbeddingProvider   string // "openai", "ollama", or "" (disabled)
	OpenAIAPIKey        string
	OpenAIModel         string // default: text-embedding-ada-002
	OpenAIEndpoint      string // default: https://api.openai.com/v1
	OllamaEndpoint      string // default: http://localhost:11434
	OllamaModel         string // default: nomic-embed-text

	// Embedding cache settings
	EmbeddingCacheEnabled bool // Feature toggle (default: true)
	EmbeddingCacheSize    int  // LRU cache capacity (default: 1000)

	// Semantic edge creation on ingest settings
	SemanticEdgeOnIngest      bool    // Feature toggle (default: true)
	SemanticEdgeTopN          int     // Max similar nodes to query (default: 5)
	SemanticEdgeMinSimilarity float64 // Minimum similarity threshold (default: 0.7)
	SemanticEdgeInitialWeight float64 // Initial edge weight (default: 0.5)

	// Batch ingest settings
	BatchIngestMaxItems int // Maximum items per batch request (default: 100)
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

	listen := get("LISTEN_ADDR", ":8080")
	uri := get("NEO4J_URI", "")
	user := get("NEO4J_USER", "")
	pass := get("NEO4J_PASS", "")
	if uri == "" || user == "" || pass == "" {
		return Config{}, errors.New("NEO4J_URI/NEO4J_USER/NEO4J_PASS are required")
	}

	reqVer, err := atoi("REQUIRED_SCHEMA_VERSION", 0)
	if err != nil {
		return Config{}, err
	}
	if reqVer <= 0 {
		return Config{}, errors.New("REQUIRED_SCHEMA_VERSION must be > 0")
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

	allowed := get("ALLOWED_RELATIONSHIP_TYPES", "ASSOCIATED_WITH,TEMPORALLY_ADJACENT,CO_ACTIVATED_WITH,CAUSES,ENABLES,ABSTRACTS_TO,INSTANTIATES")
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
	batchMaxItems, err := atoi("BATCH_INGEST_MAX_ITEMS", 100)
	if err != nil {
		return Config{}, err
	}
	if batchMaxItems < 1 || batchMaxItems > 1000 {
		return Config{}, errors.New("BATCH_INGEST_MAX_ITEMS must be in range [1, 1000]")
	}

	// Embedding provider settings
	embProvider := get("EMBEDDING_PROVIDER", "")
	openaiKey := get("OPENAI_API_KEY", "")
	openaiModel := get("OPENAI_MODEL", "text-embedding-ada-002")
	openaiEndpoint := get("OPENAI_ENDPOINT", "https://api.openai.com/v1")
	ollamaEndpoint := get("OLLAMA_ENDPOINT", "http://localhost:11434")
	ollamaModel := get("OLLAMA_MODEL", "nomic-embed-text")

	// Embedding cache settings
	embCacheEnabled := getBool("EMBEDDING_CACHE_ENABLED", true)
	embCacheSize, err := atoi("EMBEDDING_CACHE_SIZE", 1000)
	if err != nil {
		return Config{}, err
	}
	if embCacheEnabled && embCacheSize <= 0 {
		return Config{}, errors.New("EMBEDDING_CACHE_SIZE must be > 0 when caching is enabled")
	}

	return Config{
		ListenAddr: listen,
		Neo4jURI: uri,
		Neo4jUser: user,
		Neo4jPass: pass,
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
		EmbeddingProvider:         embProvider,
		OpenAIAPIKey: openaiKey,
		OpenAIModel: openaiModel,
		OpenAIEndpoint: openaiEndpoint,
		OllamaEndpoint: ollamaEndpoint,
		OllamaModel: ollamaModel,
		EmbeddingCacheEnabled:     embCacheEnabled,
		EmbeddingCacheSize:        embCacheSize,
		SemanticEdgeOnIngest:      semEdgeEnabled,
		SemanticEdgeTopN:          semEdgeTopN,
		SemanticEdgeMinSimilarity: semEdgeMinSim,
		SemanticEdgeInitialWeight: semEdgeInitWeight,
		BatchIngestMaxItems:       batchMaxItems,
	}, nil
}
