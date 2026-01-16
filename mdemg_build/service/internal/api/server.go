package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
	"mdemg/internal/embeddings"
	"mdemg/internal/learning"
	"mdemg/internal/retrieval"
)

type Server struct {
	cfg       config.Config
	driver    neo4j.DriverWithContext
	retriever *retrieval.Service
	learner   *learning.Service
	embedder  embeddings.Embedder
}

func NewServer(cfg config.Config, driver neo4j.DriverWithContext) *Server {
	ret := retrieval.NewService(cfg, driver)
	lea := learning.NewService(cfg, driver)

	// Initialize embedder (optional - nil if not configured)
	var emb embeddings.Embedder
	if cfg.EmbeddingProvider != "" {
		embCfg := embeddings.Config{
			Provider:       cfg.EmbeddingProvider,
			OpenAIAPIKey:   cfg.OpenAIAPIKey,
			OpenAIModel:    cfg.OpenAIModel,
			OpenAIEndpoint: cfg.OpenAIEndpoint,
			OllamaEndpoint: cfg.OllamaEndpoint,
			OllamaModel:    cfg.OllamaModel,
			CacheEnabled:   cfg.EmbeddingCacheEnabled,
			CacheSize:      cfg.EmbeddingCacheSize,
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

	return &Server{cfg: cfg, driver: driver, retriever: ret, learner: lea, embedder: emb}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	mux.HandleFunc("/v1/memory/retrieve", s.handleRetrieve)
	mux.HandleFunc("/v1/memory/ingest", s.handleIngest)
	mux.HandleFunc("/v1/memory/ingest/batch", s.handleBatchIngest)
	mux.HandleFunc("/v1/memory/reflect", s.handleReflect)
	mux.HandleFunc("/v1/metrics", s.handleMetrics)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return false
	}
	return true
}
