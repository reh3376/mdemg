package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/anomaly"
	"mdemg/internal/circuitbreaker"
	"mdemg/internal/db"
	"mdemg/internal/embeddings"
	"mdemg/internal/hidden"
	"mdemg/internal/jobhealth"
	"mdemg/internal/jobs"
	"mdemg/internal/llmclient"
	"mdemg/internal/metrics"
	"mdemg/internal/models"
	"mdemg/internal/retrieval"
	"mdemg/internal/symbols"
	"mdemg/internal/tsdb"
)

// ProtectedSpaces contains space IDs that cannot be deleted.
// These spaces contain critical data (e.g., Claude's conversation memory).
var ProtectedSpaces = map[string]bool{
	"mdemg-dev":    true, // Claude's conversation memory - DO NOT DELETE
	"mdemg-global": true, // Global meta-learning space (Phase 105) - DO NOT DELETE
}

// IsProtectedSpace checks if a space is protected from deletion
func IsProtectedSpace(spaceID string) bool {
	return ProtectedSpaces[spaceID]
}

// IsPrunablePrefix returns true if the space_id has a prefix indicating
// it was created for testing/temporary use (test-, uats-).
func IsPrunablePrefix(spaceID string) bool {
	return strings.HasPrefix(spaceID, "test-") || strings.HasPrefix(spaceID, "uats-")
}

// HealthCheck represents the status of a single health check component.
type HealthCheck struct {
	Status  string `json:"status"`            // "healthy", "unhealthy", "degraded"
	Message string `json:"message,omitempty"` // Additional details
	Latency string `json:"latency,omitempty"` // Check latency
}

// ReadinessStatus represents the complete readiness response.
type ReadinessStatus struct {
	Status  string                 `json:"status"`  // "ready", "not_ready", "degraded"
	Checks  map[string]HealthCheck `json:"checks"`  // Individual component checks
	Version string                 `json:"version"` // MDEMG version
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	// Lightweight liveness check with subsystem status (no live DB queries).
	checks := map[string]string{}
	status := "ok"

	// Neo4j driver nil check
	if s.driver == nil {
		checks["neo4j"] = "no_driver"
		status = "degraded"
	} else {
		checks["neo4j"] = "ok"
	}

	// Circuit breaker open count
	if s.cbRegistry != nil {
		openCount := 0
		for _, st := range s.cbRegistry.States() {
			if st == circuitbreaker.StateOpen {
				openCount++
			}
		}
		if openCount > 0 {
			checks["circuit_breakers"] = fmt.Sprintf("open:%d", openCount)
			status = "degraded"
		} else {
			checks["circuit_breakers"] = "ok"
		}
	}

	// TSDB client nil check
	if s.cfg.TSDBEnabled && s.tsdbClient == nil {
		checks["tsdb"] = "no_client"
		status = "degraded"
	} else if s.cfg.TSDBEnabled {
		checks["tsdb"] = "ok"
	}

	// Jiminy enabled + nil check
	if s.cfg.JiminyEnabled && s.jiminySvc == nil {
		checks["jiminy"] = "not_initialized"
		status = "degraded"
	} else if s.cfg.JiminyEnabled {
		checks["jiminy"] = "ok"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  status,
		"version": s.cfg.MdemgVersion,
		"commit":  s.cfg.MdemgCommit,
		"checks":  checks,
	})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status := ReadinessStatus{
		Status:  "ready",
		Checks:  make(map[string]HealthCheck),
		Version: s.cfg.MdemgVersion,
	}

	overallHealthy := true

	// Check 1: Neo4j database connectivity and schema version
	neo4jStart := time.Now()
	if err := db.AssertSchemaVersion(ctx, s.driver, s.cfg.RequiredSchemaVersion); err != nil {
		status.Checks["neo4j"] = HealthCheck{
			Status:  "unhealthy",
			Message: err.Error(),
			Latency: time.Since(neo4jStart).String(),
		}
		overallHealthy = false
	} else {
		status.Checks["neo4j"] = HealthCheck{
			Status:  "healthy",
			Message: fmt.Sprintf("schema version %d", s.cfg.RequiredSchemaVersion),
			Latency: time.Since(neo4jStart).String(),
		}
	}

	// Check 2: Embedding provider
	if s.embedder != nil {
		status.Checks["embeddings"] = HealthCheck{
			Status:  "healthy",
			Message: fmt.Sprintf("%s (%d dimensions)", s.embedder.Name(), s.embedder.Dimensions()),
		}
	} else {
		status.Checks["embeddings"] = HealthCheck{
			Status:  "degraded",
			Message: "no embedding provider configured",
		}
	}

	// Check 3: Plugin manager
	if s.pluginMgr != nil {
		modules := s.pluginMgr.ListModules()
		activeCount := 0
		for _, m := range modules {
			if m.State == "running" {
				activeCount++
			}
		}
		status.Checks["plugins"] = HealthCheck{
			Status:  "healthy",
			Message: fmt.Sprintf("%d/%d modules active", activeCount, len(modules)),
		}
	} else {
		status.Checks["plugins"] = HealthCheck{
			Status:  "degraded",
			Message: "plugin manager disabled",
		}
	}

	// Check 4: Circuit breaker states
	if s.cbRegistry != nil {
		cbStates := s.cbRegistry.States()
		openCount := 0
		for _, state := range cbStates {
			if state.String() == "open" {
				openCount++
			}
		}
		if openCount > 0 {
			status.Checks["circuit_breakers"] = HealthCheck{
				Status:  "degraded",
				Message: fmt.Sprintf("%d/%d circuits open", openCount, len(cbStates)),
			}
		} else {
			status.Checks["circuit_breakers"] = HealthCheck{
				Status:  "healthy",
				Message: fmt.Sprintf("%d circuits monitored", len(cbStates)),
			}
		}
	}

	// Check 5: Conversation service — live Neo4j connectivity check
	if s.conversationSvc != nil {
		if err := s.conversationSvc.Ping(r.Context()); err != nil {
			status.Checks["conversation"] = HealthCheck{
				Status:  "degraded",
				Message: fmt.Sprintf("CMS ping failed: %v", err),
			}
			overallHealthy = false
		} else {
			status.Checks["conversation"] = HealthCheck{
				Status:  "healthy",
				Message: "CMS available",
			}
		}
	} else {
		status.Checks["conversation"] = HealthCheck{
			Status:  "degraded",
			Message: "conversation service unavailable",
		}
	}

	// Check 6: Jiminy guidance service
	if s.jiminySvc != nil {
		jiminyMsg := "enabled"
		jiminyStatus := "healthy"
		if s.cfg.JiminySynthesisEnabled {
			jiminyMsg += ", synthesis=on"
			if s.cfg.JiminySynthesisProvider != "" {
				jiminyMsg += fmt.Sprintf(" (%s/%s)", s.cfg.JiminySynthesisProvider, s.cfg.JiminySynthesisModel)
			}
		}
		if s.cfg.JiminyEvaluateLLMEnabled {
			jiminyMsg += ", eval-llm=on"
			if s.cfg.JiminyEvaluateLLMProvider != "" {
				jiminyMsg += fmt.Sprintf(" (%s/%s)", s.cfg.JiminyEvaluateLLMProvider, s.cfg.JiminyEvaluateLLMModel)
			}
		}
		if s.cfg.JiminyOutcomeLLMEnabled {
			jiminyMsg += ", outcome-llm=on"
		}
		status.Checks["jiminy"] = HealthCheck{
			Status:  jiminyStatus,
			Message: jiminyMsg,
		}
	} else if s.cfg.JiminyEnabled {
		status.Checks["jiminy"] = HealthCheck{
			Status:  "degraded",
			Message: "enabled but service unavailable",
		}
	}

	// Check 7: TimescaleDB schema version (if enabled)
	if s.tsdbClient != nil {
		tsdbStart := time.Now()
		tsdbVer, tsdbErr := s.tsdbClient.GetSchemaVersion(ctx)
		if tsdbErr != nil {
			status.Checks["timescaledb"] = HealthCheck{
				Status:  "unhealthy",
				Message: fmt.Sprintf("connection failed: %v", tsdbErr),
				Latency: time.Since(tsdbStart).String(),
			}
			overallHealthy = false
		} else if tsdbVer < s.cfg.TSDBRequiredSchemaVersion {
			status.Checks["timescaledb"] = HealthCheck{
				Status:  "unhealthy",
				Message: fmt.Sprintf("schema version %d < required %d — run: mdemg tsdb migrate", tsdbVer, s.cfg.TSDBRequiredSchemaVersion),
				Latency: time.Since(tsdbStart).String(),
			}
			overallHealthy = false
		} else {
			status.Checks["timescaledb"] = HealthCheck{
				Status:  "healthy",
				Message: fmt.Sprintf("schema version %d", tsdbVer),
				Latency: time.Since(tsdbStart).String(),
			}
		}
	} else if s.cfg.TSDBEnabled {
		status.Checks["timescaledb"] = HealthCheck{
			Status:  "unhealthy",
			Message: "TSDB_ENABLED=true but client not initialized — check connection config",
		}
		overallHealthy = false
	}

	// Check 8: Neural sidecar (J17 protocol dependency)
	if s.cfg.JiminyEnabled && s.cfg.J17SidecarURL != "" {
		scStart := time.Now()
		scClient := &http.Client{Timeout: 2 * time.Second}
		scResp, scErr := scClient.Get(s.cfg.J17SidecarURL + "/health")
		scLatency := time.Since(scStart).String()
		if scErr != nil {
			status.Checks["neural_sidecar"] = HealthCheck{
				Status:  "degraded",
				Message: fmt.Sprintf("unreachable: %v", scErr),
				Latency: scLatency,
			}
		} else {
			scResp.Body.Close()
			if scResp.StatusCode == http.StatusOK {
				status.Checks["neural_sidecar"] = HealthCheck{
					Status:  "healthy",
					Message: "J17 sidecar available",
					Latency: scLatency,
				}
			} else {
				status.Checks["neural_sidecar"] = HealthCheck{
					Status:  "degraded",
					Message: fmt.Sprintf("status %d", scResp.StatusCode),
					Latency: scLatency,
				}
			}
		}
	}

	// Determine overall status
	if !overallHealthy {
		status.Status = "not_ready"
		writeJSON(w, http.StatusServiceUnavailable, status)
		return
	}

	// Check for degraded components
	for _, check := range status.Checks {
		if check.Status == "degraded" {
			status.Status = "degraded"
			break
		}
	}

	writeJSON(w, http.StatusOK, status)
}

// EmbeddingHealthResponse represents the response for the embedding health endpoint.
type EmbeddingHealthResponse struct {
	Status           string  `json:"status"`                     // "healthy", "degraded", "unhealthy"
	Provider         string  `json:"provider"`                   // e.g., "openai", "ollama"
	Model            string  `json:"model,omitempty"`            // e.g., "text-embedding-3-small"
	Dimensions       int     `json:"dimensions"`                 // e.g., 1536
	LatencyMs        float64 `json:"latency_ms"`                 // Last probe latency
	CacheEnabled     bool    `json:"cache_enabled"`              // Whether caching is enabled
	CacheHitRate     float64 `json:"cache_hit_rate,omitempty"`   // Cache hit percentage
	ErrorCount24h    int     `json:"error_count_24h,omitempty"`  // Errors in last 24h
	SuccessRate24h   float64 `json:"success_rate_24h,omitempty"` // Success rate in last 24h
	CircuitBreaker   string  `json:"circuit_breaker,omitempty"`  // "closed", "open", "half-open"
	LastError        string  `json:"last_error,omitempty"`       // Last error message
	LastErrorAt      string  `json:"last_error_at,omitempty"`    // Last error timestamp
	ConfiguredEnvVar bool    `json:"configured_env_var"`         // Whether env vars are set
}

func (s *Server) handleEmbeddingHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	resp := EmbeddingHealthResponse{
		Status:           "unhealthy",
		ConfiguredEnvVar: os.Getenv("EMBEDDING_PROVIDER") != "",
	}

	// Check if embedder is configured
	if s.embedder == nil {
		resp.Status = "unhealthy"
		resp.LastError = "no embedding provider configured"
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Get basic info from embedder
	resp.Provider = s.embedder.Name()
	resp.Dimensions = s.embedder.Dimensions()

	// Parse provider and model from name (format: "provider:model" or "provider:model+cache")
	parts := strings.Split(resp.Provider, ":")
	if len(parts) >= 1 {
		resp.Provider = parts[0]
	}
	if len(parts) >= 2 {
		modelPart := parts[1]
		// Remove cache suffix if present
		if idx := strings.Index(modelPart, "+"); idx > 0 {
			resp.Model = modelPart[:idx]
			resp.CacheEnabled = strings.Contains(modelPart, "+cache")
		} else {
			resp.Model = modelPart
		}
	}

	// Active health check: actually generate an embedding (skip recording — not training data)
	ctx := embeddings.WithEmbeddingMeta(r.Context(), embeddings.EmbeddingMeta{
		CallSite:      "health_check",
		SkipRecording: true,
	})
	testStart := time.Now()
	_, err := s.embedder.Embed(ctx, "health check test")
	resp.LatencyMs = float64(time.Since(testStart).Milliseconds())

	if err != nil {
		resp.Status = "unhealthy"
		resp.LastError = err.Error()
		resp.LastErrorAt = time.Now().UTC().Format(time.RFC3339)
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Check circuit breaker state if available
	if s.cbRegistry != nil {
		cbStates := s.cbRegistry.States()
		for name, state := range cbStates {
			if strings.Contains(strings.ToLower(name), "embed") {
				resp.CircuitBreaker = state.String()
				if state.String() == "open" {
					resp.Status = "degraded"
				}
				break
			}
		}
	}

	// If we got here and not degraded, we're healthy
	if resp.Status != "degraded" {
		resp.Status = "healthy"
	}

	// Set success rate (would come from metrics in production)
	resp.SuccessRate24h = 100.0 // Placeholder - integrate with metrics
	resp.ErrorCount24h = 0      // Placeholder - integrate with metrics

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRetrieve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req models.RetrieveRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	// Phase 14 Epic 1 — Note 06 per-query overrides via URL params.
	// `?sparse=true|false` overrides SPARSE_RETRIEVAL_ENABLED for this call;
	// `?sparse_percentile=0.95` overrides SPARSE_ACTIVATION_PERCENTILE.
	// JSON-body fields take precedence; URL params apply only when the body
	// did not set them.
	if !req.SparseOverridePresent {
		if v := r.URL.Query().Get("sparse"); v != "" {
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "true", "1", "yes", "on":
				req.SparseEnabled = true
				req.SparseOverridePresent = true
			case "false", "0", "no", "off":
				req.SparseEnabled = false
				req.SparseOverridePresent = true
			}
		}
	}
	if req.SparsePercentile == 0 {
		if v := r.URL.Query().Get("sparse_percentile"); v != "" {
			if p, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && p >= 0.5 && p <= 0.999 {
				req.SparsePercentile = p
			}
		}
	}
	// Phase 14.1 Epic 2 — `?category=<name>` URL param falls back when JSON
	// body field is unset. UVTS runner injects per-question category here.
	if req.Category == "" {
		if v := strings.TrimSpace(r.URL.Query().Get("category")); v != "" {
			req.Category = v
		}
	}
	// `?intent=true|false` URL param override for the intent-translation
	// rewrite, mirroring the `?sparse=` pattern. Lets the UVTS runner A/B
	// intent on vs off via --extra-url-params without a JSON-body hook.
	if !req.TranslateIntent {
		if v := r.URL.Query().Get("intent"); v != "" {
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "true", "1", "yes", "on":
				req.TranslateIntent = true
			}
		}
	}
	// Phase 14.2 Epic 4 — `?strict_context=true` URL param falls back when
	// JSON body did not set it. Operates on QueryContextFingerprint;
	// gracefully no-ops when fingerprint is empty.
	if !req.StrictContextMode {
		if v := r.URL.Query().Get("strict_context"); v != "" {
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "true", "1", "yes", "on":
				req.StrictContextMode = true
			}
		}
	}

	// Phase 14.2 Epic 4 — `?context=auto` URL param: server-side fingerprint
	// derivation when the caller didn't supply one. Tokenizes the query text
	// and matches tokens against the active ContextCatalog's tag + path
	// bits. Without this, the ContextColumn has nothing to score against
	// (queries from UVTS / external clients don't carry fingerprints).
	if len(req.QueryContextFingerprint) == 0 && req.QueryText != "" {
		v := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("context")))
		optIn := v == "auto" || v == "true" || v == "1"
		optOut := v == "off" || v == "false" || v == "0"
		// CONTEXT-LIVE-001: derivation is DEFAULT-ON (config-gated) — the
		// ?context=auto-only gate left the 5th column dormant on all live
		// traffic (no hook/MCP/CLI caller passes the param).
		if optIn || (s.cfg.ContextQueryAutoDefault && !optOut) {
			fp, fpVer := s.deriveQueryFingerprint(r.Context(), req.SpaceID, req.QueryText)
			if len(fp) > 0 {
				req.QueryContextFingerprint = fp
				req.QueryContextFingerprintVersion = fpVer
			}
		}
	}

	// Phase 102: Intent Translation — rewrite query before embedding
	var translatedIntent string
	if req.TranslateIntent && s.intentTranslator != nil && req.QueryText != "" {
		translated, translateErr := s.intentTranslator.Translate(r.Context(), req.QueryText)
		if translateErr != nil {
			slog.Warn("intent translation failed, using original", "error", translateErr)
		} else if translated != req.QueryText {
			translatedIntent = translated
			req.QueryText = translated
		}
	}

	// Generate embedding from query_text if provided and no embedding given
	if len(req.QueryEmbedding) == 0 && req.QueryText != "" {
		if s.embedder == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "query_text provided but no embedding provider configured (set EMBEDDING_PROVIDER env var)",
			})
			return
		}
		rctx := embeddings.WithEmbeddingMeta(r.Context(), embeddings.EmbeddingMeta{
			CallSite:  "retrieve",
			SpaceID:   req.SpaceID,
			QueryText: req.QueryText,
		})
		emb, err := s.embedder.Embed(rctx, req.QueryText)
		if err != nil {
			writeInternalError(w, err, "embedding generation")
			return
		}
		req.QueryEmbedding = emb
	}

	ctx := r.Context()
	if req.SessionID != "" {
		ctx = llmclient.WithSessionID(ctx, req.SessionID)
	}
	if req.SpaceID != "" {
		ctx = llmclient.WithSpaceID(ctx, req.SpaceID)
	}

	resp, err := s.retriever.Retrieve(ctx, req)
	if err != nil {
		writeInternalError(w, err, "retrieve")
		return
	}

	// Add embedding info to debug
	if s.embedder != nil && resp.Debug != nil {
		resp.Debug["embedding_provider"] = s.embedder.Name()
	}

	// Fetch symbol evidence for all results in a single batched query
	// Can be explicitly disabled with include_evidence=false in request
	if s.symbolStore != nil && len(resp.Results) > 0 {
		metrics := &models.EvidenceMetrics{
			TotalResults: len(resp.Results),
		}
		nodeIDs := make([]string, len(resp.Results))
		for i := range resp.Results {
			nodeIDs[i] = resp.Results[i].NodeID
		}
		symbolsByNode, err := s.symbolStore.GetSymbolsForMemoryNodes(r.Context(), req.SpaceID, nodeIDs)
		if err != nil {
			slog.Warn("failed to batch-fetch symbols", "error", err)
		} else {
			for i := range resp.Results {
				syms := symbolsByNode[resp.Results[i].NodeID]
				if len(syms) > 0 {
					evidence := make([]models.SymbolEvidence, 0, len(syms))
					for _, sym := range syms {
						evidence = append(evidence, models.SymbolEvidence{
							SymbolName: sym.Name,
							SymbolType: sym.SymbolType,
							FilePath:   sym.FilePath,
							Line:       sym.Line,
							LineEnd:    sym.LineEnd,
							Value:      sym.Value,
							RawValue:   sym.RawValue,
							Signature:  sym.Signature,
							DocComment: sym.DocComment,
						})
					}
					resp.Results[i].Evidence = evidence
					metrics.ResultsWithEvidence++
					metrics.TotalSymbols += len(syms)
				}
			}
		}
		if metrics.TotalResults > 0 {
			metrics.ComplianceRate = float64(metrics.ResultsWithEvidence) / float64(metrics.TotalResults)
		}
		if metrics.ResultsWithEvidence > 0 {
			metrics.AvgSymbolsPerResult = float64(metrics.TotalSymbols) / float64(metrics.ResultsWithEvidence)
		}
		resp.EvidenceMetrics = metrics
	}

	// Learning deltas: async writeback (don't block response on O(N²) learning)
	go func(spaceID string, respCopy models.RetrieveResponse) { //nolint:gosec // G118: learning writeback must outlive HTTP request
		bgCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Second)
		defer cancel()
		_ = s.learner.ApplyCoactivation(bgCtx, spaceID, respCopy)
		_ = s.learner.ApplySymbolCoactivation(bgCtx, spaceID, respCopy)
	}(req.SpaceID, resp)

	// Record query result for capability gap detection
	if s.gapDetector != nil && req.QueryText != "" {
		var avgScore float64
		if len(resp.Results) > 0 {
			var totalScore float64
			for _, r := range resp.Results {
				totalScore += r.Score
			}
			avgScore = totalScore / float64(len(resp.Results))
		}
		s.gapDetector.RecordQueryResult(req.SpaceID, req.QueryText, avgScore, len(resp.Results))
	}

	// Record score distribution for learning phase monitoring
	if len(resp.Results) > 0 {
		scores := make([]float64, len(resp.Results))
		for i, r := range resp.Results {
			scores[i] = r.Score
		}
		dist := retrieval.ComputeDistribution(scores)
		alerts := retrieval.GetDistributionMonitor().RecordDistribution(req.SpaceID, dist)
		if len(alerts) > 0 {
			if resp.Debug == nil {
				resp.Debug = make(map[string]any)
			}
			resp.Debug["distribution_alerts"] = alerts
		}
	}

	// Phase 102: Include translated intent in response
	if translatedIntent != "" {
		resp.TranslatedIntent = translatedIntent
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req models.IngestRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	// Normalize timestamp according to format enum
	normalized, err := models.NormalizeTimestamp(req.Timestamp, req.TimestampFormat)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid timestamp format"})
		return
	}
	req.Timestamp = normalized

	// Normalize canonical_time if present
	if req.CanonicalTime != "" {
		normalizedCT, err := models.NormalizeTimestamp(req.CanonicalTime, req.TimestampFormat)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid canonical_time format"})
			return
		}
		req.CanonicalTime = normalizedCT
	}

	// Generate embedding if not provided and embedder is available
	if len(req.Embedding) == 0 && s.embedder != nil {
		// Build text for embedding from content
		textForEmbedding := contentToText(req.Content)
		if req.Name != "" {
			textForEmbedding = req.Name + ": " + textForEmbedding
		}

		if textForEmbedding != "" {
			ictx := embeddings.WithEmbeddingMeta(r.Context(), embeddings.EmbeddingMeta{
				CallSite: "ingest",
				SpaceID:  req.SpaceID,
				FilePath: req.Path,
				Tags:     req.Tags,
			})
			emb, err := s.embedder.Embed(ictx, textForEmbedding)
			if err != nil {
				// Log but don't fail - embedding is optional
				// slog.Warn("failed to generate embedding", "error", err)
			} else {
				req.Embedding = emb
			}
		}
	}

	out, err := s.retriever.IngestObservation(r.Context(), req)
	if err != nil {
		writeInternalError(w, err, "ingest observation")
		return
	}

	// Include embedding dimensions in response if generated
	if len(req.Embedding) > 0 {
		out.EmbeddingDims = len(req.Embedding)
	}

	// Run anomaly detection (non-blocking - errors are logged, not returned)
	if s.anomalyDetector != nil {
		dctx := anomaly.DetectionContext{
			SpaceID:   req.SpaceID,
			NodeID:    out.NodeID,
			Content:   contentToText(req.Content),
			Embedding: req.Embedding,
			Tags:      req.Tags,
			IsUpdate:  req.NodeID != "", // If node_id was provided, this is an update
		}
		out.Anomalies = s.anomalyDetector.Detect(r.Context(), dctx)
	}

	// Record content for capability gap detection (data source references)
	if s.gapDetector != nil {
		contentText := contentToText(req.Content)
		s.gapDetector.RecordContentIngest(req.SpaceID, contentText)
	}

	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleBatchIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req models.BatchIngestRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	// Validate batch size limit
	if len(req.Observations) > s.cfg.BatchIngestMaxItems {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": fmt.Sprintf("batch size %d exceeds maximum allowed %d items", len(req.Observations), s.cfg.BatchIngestMaxItems),
		})
		return
	}

	// Normalize timestamps according to format enum
	for i := range req.Observations {
		normalized, err := models.NormalizeTimestamp(req.Observations[i].Timestamp, req.Observations[i].TimestampFormat)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": fmt.Sprintf("observations[%d]: %s", i, err.Error()),
			})
			return
		}
		req.Observations[i].Timestamp = normalized

		if req.Observations[i].CanonicalTime != "" {
			normalizedCT, err := models.NormalizeTimestamp(req.Observations[i].CanonicalTime, req.Observations[i].TimestampFormat)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"error": fmt.Sprintf("observations[%d].canonical_time: %s", i, err.Error()),
				})
				return
			}
			req.Observations[i].CanonicalTime = normalizedCT
		}
	}

	// Generate embeddings for items that don't have them (batched single API call)
	batchCtx := embeddings.WithEmbeddingMeta(r.Context(), embeddings.EmbeddingMeta{
		CallSite: "batch_ingest",
		SpaceID:  req.SpaceID,
	})
	if s.embedder != nil {
		// Collect texts that need embeddings
		type needsEmb struct {
			index int
			text  string
		}
		var needs []needsEmb
		for i := range req.Observations {
			if len(req.Observations[i].Embedding) == 0 {
				textForEmbedding := contentToText(req.Observations[i].Content)
				if req.Observations[i].Name != "" {
					textForEmbedding = req.Observations[i].Name + ": " + textForEmbedding
				}
				if textForEmbedding != "" {
					needs = append(needs, needsEmb{index: i, text: textForEmbedding})
				}
			}
		}
		if len(needs) > 0 {
			texts := make([]string, len(needs))
			for i, n := range needs {
				texts[i] = n.text
			}
			embeddings, err := s.embedder.EmbedBatch(batchCtx, texts)
			if err != nil {
				slog.Warn("batch ingest embedding failed", "error", err)
			} else {
				for i, n := range needs {
					if i < len(embeddings) {
						req.Observations[n.index].Embedding = embeddings[i]
					}
				}
			}
		}
	}

	resp, err := s.retriever.BatchIngestObservations(r.Context(), req)
	if err != nil {
		writeInternalError(w, err, "batch ingest")
		return
	}

	// Store symbols if any were provided
	// Build path->nodeID map from response to link symbols directly
	// (avoids transaction isolation issues with MATCH by path)
	if s.symbolStore != nil {
		pathToNodeID := make(map[string]string)
		for i, result := range resp.Results {
			if result.Status == "success" && result.NodeID != "" && i < len(req.Observations) {
				pathToNodeID[req.Observations[i].Path] = result.NodeID
			}
		}

		var allSymbols []symbols.SymbolRecord
		for _, obs := range req.Observations {
			if len(obs.Symbols) > 0 {
				nodeID := pathToNodeID[obs.Path]
				for _, sym := range obs.Symbols {
					rec := symbols.SymbolRecord{
						SpaceID:        req.SpaceID,
						SymbolID:       symbols.GenerateSymbolID(req.SpaceID, obs.Path, sym.Name, sym.Line),
						Name:           sym.Name,
						SymbolType:     sym.Type,
						Value:          sym.Value,
						RawValue:       sym.RawValue,
						FilePath:       obs.Path,
						ParentNodeID:   nodeID, // Direct link to MemoryNode
						Line:           sym.Line,
						LineEnd:        sym.LineEnd,
						Exported:       sym.Exported,
						DocComment:     sym.DocComment,
						Signature:      sym.Signature,
						Parent:         sym.Parent,
						Language:       sym.Language,
						TypeAnnotation: sym.TypeAnnotation,
					}
					allSymbols = append(allSymbols, rec)
				}
			}
		}
		if len(allSymbols) > 0 {
			if err := s.symbolStore.SaveSymbols(r.Context(), req.SpaceID, allSymbols); err != nil {
				slog.Warn("failed to save symbols", "error", err)
				// Non-fatal - continue with response
			} else {
				slog.Info("stored symbols", "count", len(allSymbols), "space_id", req.SpaceID)
			}
		}

		// Phase 75: Extract and save relationships from file content
		if s.symbolParser != nil && s.symbolResolver != nil && s.cfg.RelExtractImports {
			var allRels []symbols.Relationship
			for _, obs := range req.Observations {
				if obs.Path == "" {
					continue
				}
				contentStr := ""
				switch v := obs.Content.(type) {
				case string:
					contentStr = v
				case map[string]any:
					if text, ok := v["text"].(string); ok {
						contentStr = text
					}
				}
				if contentStr == "" || len(contentStr) < 10 {
					continue
				}
				ext := filepath.Ext(obs.Path)
				lang := symbols.LanguageFromExtension(ext)
				if lang == "" {
					continue
				}
				result, parseErr := s.symbolParser.ParseContent(r.Context(), obs.Path, lang, []byte(contentStr))
				if parseErr != nil || result == nil {
					continue
				}
				if len(result.Relationships) > 0 {
					allRels = append(allRels, result.Relationships...)
				}
			}
			if len(allRels) > 0 {
				resolved, err := s.symbolResolver.Resolve(r.Context(), req.SpaceID, allRels)
				if err != nil {
					slog.Warn("relationship resolution failed", "error", err)
				} else if len(resolved) > 0 {
					if err := s.symbolStore.SaveRelationships(r.Context(), req.SpaceID, resolved); err != nil {
						slog.Warn("failed to save relationships", "error", err)
					}
				}
			}
		}
	}

	// Update TapRoot freshness after successful batch ingest
	if resp.SuccessCount > 0 {
		if err := s.retriever.UpdateTapRootFreshness(r.Context(), req.SpaceID, "batch-ingest", IsPrunablePrefix(req.SpaceID)); err != nil {
			slog.Warn("failed to update TapRoot freshness", "space_id", req.SpaceID, "error", err)
		}
		s.TriggerAPEEventWithContext("ingest_complete", map[string]string{
			"space_id":    req.SpaceID,
			"ingest_type": "batch-ingest",
		})
	}

	// Set appropriate status code based on results
	statusCode := http.StatusOK
	if resp.ErrorCount > 0 && resp.SuccessCount > 0 {
		statusCode = http.StatusMultiStatus // 207 for partial success
	} else if resp.ErrorCount > 0 && resp.SuccessCount == 0 {
		statusCode = http.StatusBadRequest // All failed
	}

	writeJSON(w, statusCode, resp)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse optional space_id query parameter
	spaceID := r.URL.Query().Get("space_id")

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Execute metrics queries
	resp, err := s.queryMetrics(ctx, spaceID)
	if err != nil {
		writeInternalError(w, err, "metrics query")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// queryMetrics executes Neo4j queries to gather graph metrics
func (s *Server) queryMetrics(ctx context.Context, spaceID string) (models.MetricsResponse, error) {
	// Initialize response with empty maps and slices (avoid nil)
	resp := models.MetricsResponse{
		NodesByLayer:   make(map[int]int64),
		EdgesByType:    make(map[string]int64),
		HubNodes:       []models.HubNode{},
		RecentActivity: &models.ActivityStats{},
	}

	// Convert empty string to nil for Cypher NULL handling
	var spaceParam any
	if spaceID != "" {
		spaceParam = spaceID
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceId": spaceParam,
	}

	// Query 1: Total nodes and nodes by layer
	_, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE $spaceId IS NULL OR n.space_id = $spaceId
WITH count(n) AS total, collect(coalesce(n.layer, 0)) AS layers
RETURN total, layers`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if total, ok := rec.Get("total"); ok {
				resp.TotalNodes = toInt64(total)
			}
			if layers, ok := rec.Get("layers"); ok {
				if layerList, ok := layers.([]any); ok {
					for _, l := range layerList {
						layer := int(toInt64(l))
						resp.NodesByLayer[layer]++
					}
				}
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query node metrics: %w", err)
	}

	// Query 2: Total edges and edges by type
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (a:MemoryNode)-[r]->(b:MemoryNode)
WHERE $spaceId IS NULL OR (a.space_id = $spaceId AND b.space_id = $spaceId)
RETURN type(r) AS rel_type, count(r) AS cnt`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			rec := res.Record()
			relType, _ := rec.Get("rel_type")
			cnt, _ := rec.Get("cnt")
			if relType != nil {
				resp.EdgesByType[fmt.Sprint(relType)] = toInt64(cnt)
				resp.TotalEdges += toInt64(cnt)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query edge metrics: %w", err)
	}

	// Query 3: Hub nodes (top 10 by degree)
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE $spaceId IS NULL OR n.space_id = $spaceId
OPTIONAL MATCH (n)-[r]-()
WITH n, count(r) AS degree
ORDER BY degree DESC
LIMIT 10
RETURN n.node_id AS node_id, coalesce(n.name, n.node_id) AS name, degree`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("node_id")
			name, _ := rec.Get("name")
			degree, _ := rec.Get("degree")
			if nodeID != nil {
				resp.HubNodes = append(resp.HubNodes, models.HubNode{
					NodeID: fmt.Sprint(nodeID),
					Name:   fmt.Sprint(name),
					Degree: int(toInt64(degree)),
				})
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query hub nodes: %w", err)
	}

	// Query 4: Orphan nodes (nodes with no edges)
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE ($spaceId IS NULL OR n.space_id = $spaceId)
  AND NOT (n)-[]-()
RETURN count(n) AS orphan_count`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if cnt, ok := rec.Get("orphan_count"); ok {
				resp.OrphanNodes = toInt64(cnt)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query orphan nodes: %w", err)
	}

	// Query 5: Average edge weight
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (a:MemoryNode)-[r]->(b:MemoryNode)
WHERE $spaceId IS NULL OR (a.space_id = $spaceId AND b.space_id = $spaceId)
RETURN avg(coalesce(r.weight, 0.0)) AS avg_weight`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if avgW, ok := rec.Get("avg_weight"); ok {
				resp.AvgEdgeWeight = toFloat64Val(avgW, 0.0)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query avg edge weight: %w", err)
	}

	// Query 6: Recent activity (24h window) - nodes created
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE ($spaceId IS NULL OR n.space_id = $spaceId)
  AND n.created_at >= datetime() - duration('P1D')
RETURN count(n) AS nodes_created`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if cnt, ok := rec.Get("nodes_created"); ok {
				resp.RecentActivity.NodesCreated = toInt64(cnt)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query recent nodes: %w", err)
	}

	// Query 7: Recent activity (24h window) - edges created
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (a:MemoryNode)-[r]->(b:MemoryNode)
WHERE ($spaceId IS NULL OR (a.space_id = $spaceId AND b.space_id = $spaceId))
  AND r.created_at >= datetime() - duration('P1D')
RETURN count(r) AS edges_created`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if cnt, ok := rec.Get("edges_created"); ok {
				resp.RecentActivity.EdgesCreated = toInt64(cnt)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query recent edges: %w", err)
	}

	// Note: Retrievals cannot be tracked from Neo4j as retrieval counts
	// are not persisted to the database (per design: no per-request writes)

	return resp, nil
}

// toInt64 converts various Neo4j numeric types to int64
func toInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	case float32:
		return int64(x)
	default:
		return 0
	}
}

// toFloat64Val converts various Neo4j numeric types to float64 with default
func toFloat64Val(v any, def float64) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int64:
		return float64(x)
	case int:
		return float64(x)
	default:
		return def
	}
}

// handleStats returns comprehensive per-space memory statistics
// GET /v1/memory/stats?space_id={space_id}
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse required space_id query parameter
	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	resp, err := s.queryStats(ctx, spaceID)
	if err != nil {
		writeInternalError(w, err, "stats query")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// queryStats executes Neo4j queries to gather per-space memory statistics
func (s *Server) queryStats(ctx context.Context, spaceID string) (models.StatsResponse, error) {
	// Initialize response with empty maps and slices (avoid nil)
	resp := models.StatsResponse{
		SpaceID:              spaceID,
		MemoriesByLayer:      make(map[int]int64),
		LearningActivity:     &models.LearningActivity{},
		TemporalDistribution: &models.TemporalDistribution{},
		Connectivity:         &models.Connectivity{},
		ComputedAt:           time.Now().UTC().Format(time.RFC3339),
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceId": spaceID,
	}

	// Query 1: Memory count and memories by layer
	_, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE n.space_id = $spaceId
WITH count(n) AS total, collect(coalesce(n.layer, 0)) AS layers
RETURN total, layers`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if total, ok := rec.Get("total"); ok {
				resp.MemoryCount = toInt64(total)
			}
			if layers, ok := rec.Get("layers"); ok {
				if layerList, ok := layers.([]any); ok {
					for _, l := range layerList {
						layer := int(toInt64(l))
						resp.MemoriesByLayer[layer]++
					}
				}
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query memory count: %w", err)
	}

	// Query 2: Observation count
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)-[:HAS_OBSERVATION]->(o:Observation)
WHERE n.space_id = $spaceId
RETURN count(o) AS total`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if total, ok := rec.Get("total"); ok {
				resp.ObservationCount = toInt64(total)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query observation count: %w", err)
	}

	// Query 3: Embedding coverage
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE n.space_id = $spaceId
WITH count(n) AS total,
     sum(CASE WHEN n.embedding IS NOT NULL AND size(n.embedding) > 0 THEN 1 ELSE 0 END) AS with_embedding,
     avg(CASE WHEN n.embedding IS NOT NULL AND size(n.embedding) > 0 THEN size(n.embedding) ELSE NULL END) AS avg_dims
RETURN total, with_embedding, avg_dims`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			total, _ := rec.Get("total")
			withEmb, _ := rec.Get("with_embedding")
			avgDims, _ := rec.Get("avg_dims")

			totalCount := toInt64(total)
			withEmbCount := toInt64(withEmb)
			if totalCount > 0 {
				resp.EmbeddingCoverage = float64(withEmbCount) / float64(totalCount)
			}
			resp.AvgEmbeddingDimensions = int(toFloat64Val(avgDims, 0.0))
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query embedding coverage: %w", err)
	}

	// Query 4: Learning activity (CO_ACTIVATED_WITH edges)
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (a:MemoryNode)-[r:CO_ACTIVATED_WITH]->(b:MemoryNode)
WHERE a.space_id = $spaceId AND b.space_id = $spaceId
RETURN count(r) AS edge_count,
       avg(coalesce(r.weight, 0.0)) AS avg_weight,
       max(coalesce(r.weight, 0.0)) AS max_weight`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if cnt, ok := rec.Get("edge_count"); ok {
				resp.LearningActivity.CoActivatedEdges = toInt64(cnt)
			}
			if avgW, ok := rec.Get("avg_weight"); ok {
				resp.LearningActivity.AvgWeight = toFloat64Val(avgW, 0.0)
			}
			if maxW, ok := rec.Get("max_weight"); ok {
				resp.LearningActivity.MaxWeight = toFloat64Val(maxW, 0.0)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query learning activity: %w", err)
	}

	// Query 5: Temporal distribution - last 24h
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE n.space_id = $spaceId
  AND n.created_at >= datetime() - duration('P1D')
RETURN count(n) AS count_24h`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if cnt, ok := rec.Get("count_24h"); ok {
				resp.TemporalDistribution.Last24h = toInt64(cnt)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query temporal distribution (24h): %w", err)
	}

	// Query 6: Temporal distribution - last 7d
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE n.space_id = $spaceId
  AND n.created_at >= datetime() - duration('P7D')
RETURN count(n) AS count_7d`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if cnt, ok := rec.Get("count_7d"); ok {
				resp.TemporalDistribution.Last7d = toInt64(cnt)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query temporal distribution (7d): %w", err)
	}

	// Query 7: Temporal distribution - last 30d
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE n.space_id = $spaceId
  AND n.created_at >= datetime() - duration('P30D')
RETURN count(n) AS count_30d`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if cnt, ok := rec.Get("count_30d"); ok {
				resp.TemporalDistribution.Last30d = toInt64(cnt)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query temporal distribution (30d): %w", err)
	}

	// Query 8: Connectivity stats
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE n.space_id = $spaceId
OPTIONAL MATCH (n)-[r]-()
WITH n, count(r) AS degree
RETURN avg(degree) AS avg_degree, max(degree) AS max_degree`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if avgD, ok := rec.Get("avg_degree"); ok {
				resp.Connectivity.AvgDegree = toFloat64Val(avgD, 0.0)
			}
			if maxD, ok := rec.Get("max_degree"); ok {
				resp.Connectivity.MaxDegree = int(toInt64(maxD))
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query connectivity stats: %w", err)
	}

	// Query 9: Orphan count (nodes with no edges)
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE n.space_id = $spaceId
  AND NOT (n)-[]-()
RETURN count(n) AS orphan_count`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if cnt, ok := rec.Get("orphan_count"); ok {
				resp.Connectivity.OrphanCount = toInt64(cnt)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return resp, fmt.Errorf("failed to query orphan count: %w", err)
	}

	// Compute health score
	resp.HealthScore = computeHealthScore(resp)

	return resp, nil
}

// computeHealthScore calculates a 0.0-1.0 health score based on:
// - Embedding coverage (40% weight)
// - Connectivity (30% weight) - penalized for too many orphans
// - Recency (30% weight) - based on recent activity
func computeHealthScore(resp models.StatsResponse) float64 {
	if resp.MemoryCount == 0 {
		return 0.0
	}

	// Embedding coverage component (40%)
	embeddingScore := resp.EmbeddingCoverage * 0.4

	// Connectivity component (30%)
	// Score based on orphan ratio - lower is better
	orphanRatio := float64(resp.Connectivity.OrphanCount) / float64(resp.MemoryCount)
	connectivityScore := (1.0 - orphanRatio) * 0.3

	// Recency component (30%)
	// Score based on ratio of 7d activity to total
	var recencyScore float64
	if resp.MemoryCount > 0 {
		recentRatio := float64(resp.TemporalDistribution.Last7d) / float64(resp.MemoryCount)
		// Cap at 1.0 (if all memories are recent)
		if recentRatio > 1.0 {
			recentRatio = 1.0
		}
		recencyScore = recentRatio * 0.3
	}

	// Combine scores
	healthScore := embeddingScore + connectivityScore + recencyScore

	// Ensure result is in 0.0-1.0 range
	if healthScore < 0.0 {
		healthScore = 0.0
	}
	if healthScore > 1.0 {
		healthScore = 1.0
	}

	return healthScore
}

func (s *Server) handleReflect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req models.ReflectRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	// Generate embedding from topic if provided and no embedding given
	if len(req.TopicEmbedding) == 0 && req.Topic != "" {
		if s.embedder == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "topic provided but no embedding provider configured (set EMBEDDING_PROVIDER env var)",
			})
			return
		}
		ctx := embeddings.WithEmbeddingMeta(r.Context(), embeddings.EmbeddingMeta{
			CallSite:  "reflect",
			SpaceID:   req.SpaceID,
			QueryText: req.Topic,
		})
		emb, err := s.embedder.Embed(ctx, req.Topic)
		if err != nil {
			writeInternalError(w, err, "embedding generation")
			return
		}
		req.TopicEmbedding = emb
	}

	resp, err := s.retriever.Reflect(r.Context(), req)
	if err != nil {
		slog.Error("reflect failed", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "reflect failed"})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// consolidateTimeout resolves the server-side deadline for a detached
// consolidation cycle (CONSOLIDATE_TIMEOUT_MS, default 30 min; HIDDEN-CHURN-002).
func (s *Server) consolidateTimeout() time.Duration {
	ms := s.cfg.ConsolidateTimeoutMs
	if ms <= 0 {
		ms = 1800000
	}
	return time.Duration(ms) * time.Millisecond
}

// handleConsolidate handles POST /v1/memory/consolidate
// Triggers hidden layer creation via DBSCAN clustering and runs message passing (forward + backward passes)
func (s *Server) handleConsolidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req models.ConsolidateRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	// Check if hidden layer is enabled
	if !s.cfg.HiddenLayerEnabled {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": models.ConsolidateResponse{
				SpaceID: req.SpaceID,
				Enabled: false,
			},
		})
		return
	}

	// Run full consolidation (or partial based on skip flags)
	var resp models.ConsolidateResponse
	resp.SpaceID = req.SpaceID
	resp.Enabled = true

	start := time.Now()

	// HIDDEN-CHURN-002: consolidation re-clusters all L0 nodes (~10 min on large
	// spaces) and MUST run to completion — aborting mid-cycle leaves the hidden
	// layer in a partial state (the pre-fix wipe-then-timeout churn; with
	// match-in-place, accumulation, since deleteUnmatched runs last). Detach from
	// the caller's cancellation so a client timeout (ingest sync, RSIC) can't
	// abort the cycle; bound it with a generous server-side deadline instead.
	consolCtx, cancelConsol := context.WithTimeout(context.WithoutCancel(r.Context()), s.consolidateTimeout())
	defer cancelConsol()
	// HIDDEN-CHURN-003: explicit full re-cluster override (else the hidden step
	// runs the default incremental path when HIDDEN_INCREMENTAL_ENABLED=true).
	if req.FullRecluster {
		consolCtx = hidden.WithFullRecluster(consolCtx)
	}

	// Step 1: Run node-creation pipeline (hidden, concern, config, comparison, temporal, ui, constraint)
	if !req.SkipClustering {
		phaseStart := time.Now()
		pipelineResult, err := s.hiddenLayer.RunNodeCreationPipeline(consolCtx, req.SpaceID, req.EnableDynamicEmergence)
		if err != nil {
			writeInternalError(w, err, "node creation pipeline")
			return
		}
		metrics.RecordConsolidationPhase(req.SpaceID, "node_creation", phaseStart)

		// Log non-fatal pipeline step errors
		for _, stepErr := range pipelineResult.Errors {
			slog.Warn("pipeline step failed", "step", stepErr.Step, "message", stepErr.Message)
		}

		// Populate dynamic Steps map
		resp.Steps = make(map[string]*models.StepResultAPI, len(pipelineResult.Steps))
		for name, sr := range pipelineResult.Steps {
			resp.Steps[name] = &models.StepResultAPI{
				NodesCreated: sr.NodesCreated,
				NodesUpdated: sr.NodesUpdated,
				EdgesCreated: sr.EdgesCreated,
				Details:      sr.Details,
			}
		}

		// Populate flat fields for backward compatibility
		if sr, ok := pipelineResult.Steps["hidden"]; ok {
			resp.HiddenNodesCreated = sr.NodesCreated
		}
		if sr, ok := pipelineResult.Steps["concern"]; ok {
			resp.ConcernNodesCreated = sr.NodesCreated
			resp.ConcernEdgesCreated = sr.EdgesCreated
		}
		if sr, ok := pipelineResult.Steps["config"]; ok {
			resp.ConfigNodeCreated = sr.NodesCreated > 0
			resp.ConfigEdgesCreated = sr.EdgesCreated
		}
		if sr, ok := pipelineResult.Steps["comparison"]; ok {
			resp.ComparisonNodesCreated = sr.NodesCreated
			resp.ComparisonEdgesCreated = sr.EdgesCreated
		}
		if sr, ok := pipelineResult.Steps["temporal"]; ok {
			resp.TemporalNodeCreated = sr.NodesCreated > 0
			resp.TemporalEdgesCreated = sr.EdgesCreated
		}
		if sr, ok := pipelineResult.Steps["ui"]; ok {
			resp.UINodesCreated = sr.NodesCreated
			resp.UIEdgesCreated = sr.EdgesCreated
		}
		if sr, ok := pipelineResult.Steps["constraint"]; ok {
			resp.ConstraintNodesCreated = sr.NodesCreated
			resp.ConstraintNodesUpdated = sr.NodesUpdated
			resp.ConstraintEdgesLinked = sr.EdgesCreated
		}
		if sr, ok := pipelineResult.Steps["dynamic_emergence"]; ok {
			resp.DynamicEmergentNodesCreated = sr.NodesCreated
		}
	}

	// Step 2: Forward pass (unless skipped)
	if !req.SkipForward {
		phaseStart := time.Now()
		fwdResult, err := s.hiddenLayer.ForwardPass(consolCtx, req.SpaceID)
		if err != nil {
			writeInternalError(w, err, "forward pass")
			return
		}
		resp.HiddenNodesUpdated = fwdResult.HiddenNodesUpdated
		resp.ConceptNodesUpdated = fwdResult.ConceptNodesUpdated
		metrics.RecordConsolidationPhase(req.SpaceID, "forward_initial", phaseStart)
	}

	// Step 3: Multi-layer concept clustering (unless skipped)
	// Build concept layers: hidden (L1) → concepts (L2, L3, etc.)
	// Try ALL layers - upper layers have adaptive (looser) constraints for emergence
	if !req.SkipClustering {
		phaseStart := time.Now()
		var repeatedForwardDur time.Duration // the per-layer ForwardPass re-runs (Sprint-B signal)
		maxLayers := 5
		for targetLayer := 2; targetLayer <= maxLayers; targetLayer++ {
			conceptCreated, conceptMerged, err := s.hiddenLayer.CreateConceptNodes(consolCtx, req.SpaceID, targetLayer)
			if err != nil {
				writeInternalError(w, err, fmt.Sprintf("concept node creation layer %d", targetLayer))
				return
			}
			// Track merged concepts
			resp.ConceptNodesMerged += conceptMerged
			// Don't break on zero - upper layers may still form clusters
			if conceptCreated > 0 {
				resp.ConceptNodesCreated += conceptCreated

				// Run forward pass to update new concept embeddings
				fwdStart := time.Now()
				fwdResult, err := s.hiddenLayer.ForwardPass(consolCtx, req.SpaceID)
				if err != nil {
					writeInternalError(w, err, fmt.Sprintf("forward pass after layer %d", targetLayer))
					return
				}
				repeatedForwardDur += time.Since(fwdStart)
				resp.ConceptNodesUpdated += fwdResult.ConceptNodesUpdated
			}
		}
		metrics.RecordConsolidationPhase(req.SpaceID, "concept_clustering", phaseStart)
		// Sub-signal: how much of the cycle is the per-layer ForwardPass re-runs
		// (the "run ForwardPass once, not 5×" Sprint-B opportunity).
		metrics.Metrics().ConsolidationPhaseDuration(req.SpaceID, "concept_forward_repeats").Set(repeatedForwardDur.Seconds())
	}

	// Step 4: Backward pass (unless skipped)
	if !req.SkipBackward {
		phaseStart := time.Now()
		bwdResult, err := s.hiddenLayer.BackwardPass(consolCtx, req.SpaceID)
		if err != nil {
			writeInternalError(w, err, "backward pass")
			return
		}
		// Add to existing count if forward pass was also run
		resp.HiddenNodesUpdated += bwdResult.HiddenNodesUpdated
		resp.EdgesStrengthened = bwdResult.EdgesStrengthened
		metrics.RecordConsolidationPhase(req.SpaceID, "backward", phaseStart)
	}

	// Step 5: Post-clustering pipeline (dynamic edges + L5 emergent nodes)
	postStart := time.Now()
	postResult, err := s.hiddenLayer.RunPostClusteringPipeline(consolCtx, req.SpaceID)
	if err != nil {
		slog.Warn("post-clustering pipeline failed", "error", err)
	} else {
		// Merge post-clustering steps into the Steps map
		if resp.Steps == nil {
			resp.Steps = make(map[string]*models.StepResultAPI)
		}
		for name, sr := range postResult.Steps {
			resp.Steps[name] = &models.StepResultAPI{
				NodesCreated: sr.NodesCreated,
				NodesUpdated: sr.NodesUpdated,
				EdgesCreated: sr.EdgesCreated,
				Details:      sr.Details,
			}
		}
		// Log non-fatal step errors
		for _, stepErr := range postResult.Errors {
			slog.Warn("post-clustering step failed", "step", stepErr.Step, "message", stepErr.Message)
		}
		// Populate flat backward-compat fields
		if sr, ok := postResult.Steps["dynamic_edges"]; ok {
			resp.DynamicEdgesCreated = sr.EdgesCreated
		}
		if sr, ok := postResult.Steps["emergent_l5"]; ok {
			resp.L5NodesCreated = sr.NodesCreated
		}
	}
	metrics.RecordConsolidationPhase(req.SpaceID, "post_clustering", postStart)

	// Step 6: Generate summaries for hidden and concept nodes
	summariesStart := time.Now()
	summariesUpdated, err := s.hiddenLayer.GenerateSummaries(consolCtx, req.SpaceID)
	if err != nil {
		// Log but don't fail - summaries are nice-to-have
		slog.Warn("failed to generate summaries", "error", err)
	}
	resp.SummariesGenerated = summariesUpdated
	metrics.RecordConsolidationPhase(req.SpaceID, "summaries", summariesStart)

	// Step 6b: Enhance mechanical summaries with LLM (opt-in)
	if s.hiddenLayer != nil {
		llmStart := time.Now()
		enhanced, enhErr := s.hiddenLayer.EnhanceSummariesWithLLM(consolCtx, req.SpaceID)
		if enhErr != nil {
			slog.Warn("failed to enhance cluster summaries", "error", enhErr)
		} else if enhanced > 0 {
			slog.Info("enhanced cluster summaries with LLM", "count", enhanced)
		}
		metrics.RecordConsolidationPhase(req.SpaceID, "summaries_llm", llmStart)
	}

	// Step 6: Refresh stale edges (Phase 9.5.3)
	refreshStart := time.Now()
	edgesRefreshed, err := s.retriever.RefreshStaleEdges(consolCtx, req.SpaceID)
	if err != nil {
		slog.Warn("failed to refresh stale edges", "error", err)
	} else {
		resp.EdgesRefreshed = edgesRefreshed
	}
	metrics.RecordConsolidationPhase(req.SpaceID, "refresh_edges", refreshStart)

	resp.DurationMs = float64(time.Since(start).Milliseconds())

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// contentToText converts the content field to a string for embedding
func contentToText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case map[string]any:
		// Try common text fields
		if text, ok := v["text"].(string); ok {
			return text
		}
		if text, ok := v["content"].(string); ok {
			return text
		}
		if text, ok := v["message"].(string); ok {
			return text
		}
		// Fallback: marshal to JSON
		// This is intentionally simple - caller should pass string content
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// handleArchiveNode handles POST /v1/memory/nodes/{node_id}/archive
// Soft-deletes a memory node by setting is_archived=true and archived_at timestamp
func (s *Server) handleArchiveNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract node_id from URL path: /v1/memory/nodes/{node_id}/archive
	path := strings.TrimPrefix(r.URL.Path, "/v1/memory/nodes/")
	nodeID := strings.TrimSuffix(path, "/archive")
	if nodeID == "" || nodeID == path {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid node_id in path"})
		return
	}

	// Parse optional ArchiveRequest (body may be empty)
	var req models.ArchiveRequest
	if r.ContentLength > 0 {
		if !readJSON(w, r, &req) {
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	params := map[string]any{
		"nodeId": nodeID,
		"reason": req.Reason,
	}

	// Execute archive operation
	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode {node_id: $nodeId})
SET n.is_archived = true,
    n.archived_at = datetime(),
    n.archive_reason = $reason,
    n.version = coalesce(n.version, 0) + 1
RETURN n.node_id AS node_id,
       coalesce(n.name, n.node_id) AS name,
       n.archived_at AS archived_at`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			nodeIDVal, _ := rec.Get("node_id")
			nameVal, _ := rec.Get("name")
			archivedAtVal, _ := rec.Get("archived_at")

			// Convert archived_at to ISO string
			var archivedAtStr string
			if at, ok := archivedAtVal.(neo4j.LocalDateTime); ok {
				archivedAtStr = at.Time().Format(time.RFC3339)
			} else if at, ok := archivedAtVal.(time.Time); ok {
				archivedAtStr = at.Format(time.RFC3339)
			} else {
				archivedAtStr = fmt.Sprint(archivedAtVal)
			}

			return &models.ArchiveResponse{
				NodeID:     fmt.Sprint(nodeIDVal),
				Name:       fmt.Sprint(nameVal),
				ArchivedAt: archivedAtStr,
				Reason:     req.Reason,
			}, nil
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		// Node not found
		return nil, nil
	})

	if err != nil {
		writeInternalError(w, err, "archive node")
		return
	}

	if result == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("node not found: %s", nodeID)})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleUnarchiveNode handles POST /v1/memory/nodes/{node_id}/unarchive
// Restores an archived memory node by clearing is_archived, archived_at, and archive_reason
func (s *Server) handleUnarchiveNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract node_id from URL path: /v1/memory/nodes/{node_id}/unarchive
	path := strings.TrimPrefix(r.URL.Path, "/v1/memory/nodes/")
	nodeID := strings.TrimSuffix(path, "/unarchive")
	if nodeID == "" || nodeID == path {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid node_id in path"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	params := map[string]any{
		"nodeId": nodeID,
	}

	// Execute unarchive operation
	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode {node_id: $nodeId})
REMOVE n.is_archived, n.archived_at, n.archive_reason
WITH n, datetime() AS unarchiveTime
SET n.version = coalesce(n.version, 0) + 1
RETURN n.node_id AS node_id,
       coalesce(n.name, n.node_id) AS name,
       unarchiveTime AS unarchived_at`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			nodeIDVal, _ := rec.Get("node_id")
			nameVal, _ := rec.Get("name")
			unarchivedAtVal, _ := rec.Get("unarchived_at")

			// Convert unarchived_at to ISO string
			var unarchivedAtStr string
			if at, ok := unarchivedAtVal.(neo4j.LocalDateTime); ok {
				unarchivedAtStr = at.Time().Format(time.RFC3339)
			} else if at, ok := unarchivedAtVal.(time.Time); ok {
				unarchivedAtStr = at.Format(time.RFC3339)
			} else {
				unarchivedAtStr = fmt.Sprint(unarchivedAtVal)
			}

			return &models.UnarchiveResponse{
				NodeID:       fmt.Sprint(nodeIDVal),
				Name:         fmt.Sprint(nameVal),
				UnarchivedAt: unarchivedAtStr,
			}, nil
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		// Node not found
		return nil, nil
	})

	if err != nil {
		writeInternalError(w, err, "unarchive node")
		return
	}

	if result == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("node not found: %s", nodeID)})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleBulkArchive handles POST /v1/memory/archive/bulk
// Archives multiple memory nodes in a single request with partial success support
func (s *Server) handleBulkArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req models.BulkArchiveRequest
	if !readJSON(w, r, &req) {
		return
	}

	// Validate non-empty node_ids array
	if len(req.NodeIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "node_ids array cannot be empty",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Process each node
	results := make([]models.BulkArchiveResult, 0, len(req.NodeIDs))
	var successCount, errorCount int

	for _, nodeID := range req.NodeIDs {
		params := map[string]any{
			"nodeId": nodeID,
			"reason": req.Reason,
		}

		// Execute archive operation for this node
		result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			cypher := `
MATCH (n:MemoryNode {node_id: $nodeId})
SET n.is_archived = true,
    n.archived_at = datetime(),
    n.archive_reason = $reason
RETURN n.node_id AS node_id, n.archived_at AS archived_at`
			res, err := tx.Run(ctx, cypher, params)
			if err != nil {
				return nil, err
			}
			if res.Next(ctx) {
				rec := res.Record()
				archivedAtVal, _ := rec.Get("archived_at")

				// Convert archived_at to ISO string
				var archivedAtStr string
				if at, ok := archivedAtVal.(neo4j.LocalDateTime); ok {
					archivedAtStr = at.Time().Format(time.RFC3339)
				} else if at, ok := archivedAtVal.(time.Time); ok {
					archivedAtStr = at.Format(time.RFC3339)
				} else {
					archivedAtStr = fmt.Sprint(archivedAtVal)
				}

				return archivedAtStr, nil
			}
			if err := res.Err(); err != nil {
				return nil, err
			}
			// Node not found
			return nil, nil
		})

		if err != nil {
			results = append(results, models.BulkArchiveResult{
				NodeID: nodeID,
				Status: "error",
				Error:  err.Error(),
			})
			errorCount++
		} else if result == nil {
			results = append(results, models.BulkArchiveResult{
				NodeID: nodeID,
				Status: "error",
				Error:  "node not found",
			})
			errorCount++
		} else {
			results = append(results, models.BulkArchiveResult{
				NodeID:     nodeID,
				Status:     "success",
				ArchivedAt: result.(string),
			})
			successCount++
		}
	}

	resp := models.BulkArchiveResponse{
		SpaceID:      req.SpaceID,
		TotalItems:   len(req.NodeIDs),
		SuccessCount: successCount,
		ErrorCount:   errorCount,
		Results:      results,
	}

	// Set appropriate status code based on results
	statusCode := http.StatusOK
	if errorCount > 0 && successCount > 0 {
		statusCode = http.StatusMultiStatus // 207 for partial success
	} else if errorCount > 0 && successCount == 0 {
		statusCode = http.StatusBadRequest // All failed
	}

	writeJSON(w, statusCode, resp)
}

// handleDeleteNode handles DELETE /v1/memory/nodes/{node_id}
// Hard-deletes a memory node (DETACH DELETE) with safety checks
func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract node_id from URL path: /v1/memory/nodes/{node_id}
	path := strings.TrimPrefix(r.URL.Path, "/v1/memory/nodes/")
	nodeID := path
	if nodeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid node_id in path"})
		return
	}

	// Require ?confirm=true query parameter for safety
	if r.URL.Query().Get("confirm") != "true" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "deletion requires ?confirm=true query parameter",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	params := map[string]any{
		"nodeId": nodeID,
	}

	// Check if node belongs to a protected space
	spaceID, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `MATCH (n:MemoryNode {node_id: $nodeId}) RETURN n.space_id AS space_id`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return "", err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if val, ok := rec.Get("space_id"); ok && val != nil {
				return val.(string), nil
			}
		}
		return "", nil
	})
	if err != nil {
		writeInternalError(w, err, "check node space")
		return
	}
	if spaceID != nil && IsProtectedSpace(spaceID.(string)) {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error":    "cannot delete node from protected space",
			"space_id": spaceID,
			"reason":   "This space contains critical data (Claude's conversation memory)",
		})
		return
	}

	// Check for outgoing ABSTRACTS_TO edges (block deletion if present)
	hasAbstractions, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode {node_id: $nodeId})-[:ABSTRACTS_TO]->()
RETURN count(*) > 0 AS has_abstractions`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return false, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if val, ok := rec.Get("has_abstractions"); ok {
				if b, ok := val.(bool); ok {
					return b, nil
				}
			}
		}
		return false, res.Err()
	})

	if err != nil {
		writeInternalError(w, err, "check node abstractions")
		return
	}

	if hasAbstractions.(bool) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "cannot delete node with outgoing ABSTRACTS_TO edges; delete child abstractions first or use archive instead",
		})
		return
	}

	// Execute delete operation with DETACH DELETE
	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// First check if node exists and count edges
		checkCypher := `
MATCH (n:MemoryNode {node_id: $nodeId})
OPTIONAL MATCH (n)-[r]-()
RETURN n.node_id AS node_id, count(DISTINCT r) AS edge_count`
		checkRes, err := tx.Run(ctx, checkCypher, params)
		if err != nil {
			return nil, err
		}

		var edgeCount int64
		var foundNodeID string
		if checkRes.Next(ctx) {
			rec := checkRes.Record()
			if nid, ok := rec.Get("node_id"); ok && nid != nil {
				foundNodeID = fmt.Sprint(nid)
			}
			if ec, ok := rec.Get("edge_count"); ok {
				edgeCount = toInt64(ec)
			}
		}
		if err := checkRes.Err(); err != nil {
			return nil, err
		}

		if foundNodeID == "" {
			// Node not found
			return nil, nil
		}

		// Execute DETACH DELETE
		deleteCypher := `
MATCH (n:MemoryNode {node_id: $nodeId})
DETACH DELETE n`
		_, err = tx.Run(ctx, deleteCypher, params)
		if err != nil {
			return nil, err
		}

		return &models.DeleteResponse{
			NodeID:       nodeID,
			DeletedNodes: 1,
			DeletedEdges: int(edgeCount),
		}, nil
	})

	if err != nil {
		writeInternalError(w, err, "delete node")
		return
	}

	if result == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("node not found: %s", nodeID)})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleLearningPrune handles POST /v1/learning/prune
// Prunes decayed and excess learning edges (CO_ACTIVATED_WITH) for a space.
func (s *Server) handleLearningPrune(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter is required"})
		return
	}

	ctx := r.Context()

	// Prune decayed edges (below threshold after time decay)
	decayedDeleted, err := s.learner.PruneDecayedEdges(ctx, spaceID)
	if err != nil {
		writeInternalError(w, err, "prune decayed edges")
		return
	}

	// Prune excess edges per node (above cap)
	excessDeleted, err := s.learner.PruneExcessEdgesPerNode(ctx, spaceID)
	if err != nil {
		writeInternalError(w, err, "prune excess edges")
		return
	}

	totalDeleted := decayedDeleted + excessDeleted
	slog.Info("learning edge pruning complete", "space_id", spaceID, "decayed", decayedDeleted, "excess", excessDeleted, "total", totalDeleted)

	writeJSON(w, http.StatusOK, map[string]any{
		"space_id":        spaceID,
		"decayed_deleted": decayedDeleted,
		"excess_deleted":  excessDeleted,
		"total_deleted":   totalDeleted,
	})
}

// handleConsult handles POST /v1/memory/consult
// The Agent Consulting Service acts as an SME for coding agents.
func (s *Server) handleConsult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req models.ConsultRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	ctx := r.Context()
	if req.SessionID != "" {
		ctx = llmclient.WithSessionID(ctx, req.SessionID)
	}
	if req.SpaceID != "" {
		ctx = llmclient.WithSpaceID(ctx, req.SpaceID)
	}

	resp, err := s.consultant.Consult(ctx, req)
	if err != nil {
		writeInternalError(w, err, "consult")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleSuggest handles POST /v1/memory/suggest
// Context-triggered suggestions - proactively surfaces relevant information
// without requiring an explicit question. This is MDEMG's "Active Participation" mode.
func (s *Server) handleSuggest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req models.SuggestRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	resp, err := s.consultant.Suggest(r.Context(), req)
	if err != nil {
		writeInternalError(w, err, "suggest")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleLearningStats handles GET /v1/learning/stats
// Returns statistics about learning edges for a space.
func (s *Server) handleLearningStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter is required"})
		return
	}

	ctx := r.Context()

	stats, err := s.learner.GetLearningEdgeStats(ctx, spaceID)
	if err != nil {
		writeInternalError(w, err, "learning stats")
		return
	}

	stats["space_id"] = spaceID
	stats["decay_per_day"] = s.cfg.LearningDecayPerDay
	stats["prune_threshold"] = s.cfg.LearningPruneThreshold
	stats["max_edges_per_node"] = s.cfg.LearningMaxEdgesPerNode

	// Include freeze state
	freezeState := s.learner.GetFreezeState(spaceID)
	stats["freeze_state"] = freezeState

	writeJSON(w, http.StatusOK, stats)
}

// handleLearningFreeze handles POST /v1/learning/freeze
// Freezes learning edge creation/updates for a space.
// When frozen, no new CO_ACTIVATED_WITH edges are created and existing edges are not updated.
// Use this for production deployments requiring stable scoring.
func (s *Server) handleLearningFreeze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SpaceID  string `json:"space_id"`
		Reason   string `json:"reason"`
		FrozenBy string `json:"frozen_by"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id is required"})
		return
	}

	state, err := s.learner.FreezeLearning(r.Context(), req.SpaceID, req.Reason, req.FrozenBy)
	if err != nil {
		writeInternalError(w, err, "learning freeze")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"space_id": req.SpaceID,
		"status":   "frozen",
		"state":    state,
		"message":  "Learning has been frozen for this space. No new edges will be created.",
	})
}

// handleLearningUnfreeze handles POST /v1/learning/unfreeze
// Resumes learning edge creation/updates for a space.
func (s *Server) handleLearningUnfreeze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SpaceID string `json:"space_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id is required"})
		return
	}

	state := s.learner.UnfreezeLearning(req.SpaceID)

	writeJSON(w, http.StatusOK, map[string]any{
		"space_id": req.SpaceID,
		"status":   "unfrozen",
		"state":    state,
		"message":  "Learning has been resumed for this space.",
	})
}

// handleLearningFreezeStatus handles GET /v1/learning/freeze/status
// Returns freeze status for all spaces or a specific space.
func (s *Server) handleLearningFreezeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	spaceID := r.URL.Query().Get("space_id")

	if spaceID != "" {
		// Return status for specific space
		state := s.learner.GetFreezeState(spaceID)
		writeJSON(w, http.StatusOK, map[string]any{
			"space_id": spaceID,
			"state":    state,
		})
		return
	}

	// Return status for all spaces
	states := s.learner.GetAllFreezeStates()
	writeJSON(w, http.StatusOK, map[string]any{
		"frozen_spaces": states,
		"count":         len(states),
	})
}

// handleCacheStats handles GET /v1/memory/cache/stats
// Returns statistics about the query result cache.
func (s *Server) handleCacheStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	stats := s.retriever.QueryCacheStats()
	writeJSON(w, http.StatusOK, stats)
}

// handleCacheClear handles DELETE /v1/memory/cache
// Clears all entries from the query cache, or clears entries for a specific space if space_id is provided.
// Query params:
//   - space_id (optional): Clear cache only for this space
//   - confirm (required): Must be "true" to confirm the operation
func (s *Server) handleCacheClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Require confirmation
	confirm := r.URL.Query().Get("confirm")
	if confirm != "true" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":   "confirmation required",
			"message": "Add ?confirm=true to confirm cache clear operation",
		})
		return
	}

	spaceID := r.URL.Query().Get("space_id")

	var cleared int
	var message string

	if spaceID != "" {
		// Clear cache for specific space
		cleared = s.retriever.InvalidateSpaceCache(spaceID)
		message = "Cache cleared for space"
	} else {
		// Clear all cache entries
		cleared = s.retriever.ClearQueryCache()
		message = "All cache entries cleared"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":         message,
		"entries_cleared": cleared,
		"space_id":        spaceID,
	})
}

// handleQueryMetrics handles GET /v1/memory/query/metrics
// Returns Neo4j query execution statistics for performance monitoring.
func (s *Server) handleQueryMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	stats := retrieval.GetQueryMetrics()
	writeJSON(w, http.StatusOK, stats)
}

// handleSymbolSearch handles GET /v1/memory/symbols
// Query params:
//   - space_id (required): Memory space to search
//   - name: Symbol name pattern (supports * wildcard for prefix match)
//   - type: Filter by symbol type (const, var, function, class, etc.)
//   - file: Filter by file path
//   - exported: Filter by exported status (true/false)
//   - limit: Maximum results (default 50, max 500)
//   - q: Fulltext search query (alternative to name pattern)
func (s *Server) handleSymbolSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	spaceID := query.Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter is required"})
		return
	}

	name := query.Get("name")
	symbolType := query.Get("type")
	filePath := query.Get("file")
	exportedStr := query.Get("exported")
	fulltextQuery := query.Get("q")

	// Parse limit with default and max constraints.
	// CONFIG-DEADFLAG-001: PAGINATION_DEF_LIMIT / PAGINATION_MAX_LIMIT
	// were parsed but these literals (50/500 = the defaults) always won.
	limit := s.cfg.PaginationDefLimit
	if limitStr := query.Get("limit"); limitStr != "" {
		if _, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil || limit < 1 {
			limit = s.cfg.PaginationDefLimit
		}
		if limit > s.cfg.PaginationMaxLimit {
			limit = s.cfg.PaginationMaxLimit
		}
	}

	ctx := r.Context()

	// Check if symbol store is available
	if s.symbolStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "symbol store not initialized"})
		return
	}

	var allSymbols []symbols.SymbolRecord
	var err error

	// Route to appropriate query method based on parameters
	switch {
	case fulltextQuery != "":
		// Fulltext search takes priority
		results, searchErr := s.symbolStore.FulltextSearch(ctx, spaceID, fulltextQuery, limit)
		if searchErr != nil {
			writeInternalError(w, searchErr, "symbol fulltext search")
			return
		}
		for _, r := range results {
			allSymbols = append(allSymbols, r.Symbol)
		}

	case filePath != "":
		// File-specific query
		allSymbols, err = s.symbolStore.QueryByFile(ctx, spaceID, filePath)
		if err != nil {
			writeInternalError(w, err, "symbol file query")
			return
		}

	case symbolType != "" && name == "":
		// Type-only query
		allSymbols, err = s.symbolStore.QueryByType(ctx, spaceID, symbolType, limit)
		if err != nil {
			writeInternalError(w, err, "symbol type query")
			return
		}

	case name != "":
		// Name pattern query - handle wildcard
		searchName := name
		if strings.HasSuffix(name, "*") {
			searchName = strings.TrimSuffix(name, "*")
		}
		allSymbols, err = s.symbolStore.QueryByName(ctx, spaceID, searchName, limit)
		if err != nil {
			writeInternalError(w, err, "symbol name query")
			return
		}

	default:
		// No specific filter - return error (don't allow unbounded queries)
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "at least one of name, type, file, or q parameter is required",
		})
		return
	}

	// Apply additional filters
	var filtered []symbols.SymbolRecord
	for _, sym := range allSymbols {
		// Filter by type if specified (when not already the primary filter)
		if symbolType != "" && filePath != "" && sym.SymbolType != symbolType {
			continue
		}
		if symbolType != "" && name != "" && sym.SymbolType != symbolType {
			continue
		}

		// Filter by exported status if specified
		if exportedStr != "" {
			wantExported := exportedStr == "true"
			if sym.Exported != wantExported {
				continue
			}
		}

		filtered = append(filtered, sym)
	}

	// Apply limit after filtering
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	// Build response
	symbolsResponse := make([]map[string]any, 0, len(filtered))
	for _, sym := range filtered {
		symMap := map[string]any{
			"name":      sym.Name,
			"type":      sym.SymbolType,
			"file_path": sym.FilePath,
			"line":      sym.Line, // UPTS standard
			"exported":  sym.Exported,
		}
		// Include optional fields only if non-empty
		if sym.Value != "" {
			symMap["value"] = sym.Value
		}
		if sym.RawValue != "" {
			symMap["raw_value"] = sym.RawValue
		}
		if sym.DocComment != "" {
			symMap["doc_comment"] = sym.DocComment
		}
		if sym.Signature != "" {
			symMap["signature"] = sym.Signature
		}
		if sym.LineEnd > 0 {
			symMap["line_end"] = sym.LineEnd // UPTS standard
		}
		if sym.TypeAnnotation != "" {
			symMap["type_annotation"] = sym.TypeAnnotation
		}
		symbolsResponse = append(symbolsResponse, symMap)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"space_id": spaceID,
		"symbols":  symbolsResponse,
		"count":    len(symbolsResponse),
	})
}

// handleIngestTrigger handles POST /v1/memory/ingest/trigger
// Triggers a background codebase re-ingestion job.
func (s *Server) handleIngestTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req models.IngestTriggerRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	// Generate job ID
	jobID := "ingest-" + uuid.New().String()[:8]

	// Build job configuration
	config := map[string]any{
		"space_id": req.SpaceID,
		"path":     req.Path,
	}

	// Apply defaults
	batchSize := 100
	if req.BatchSize > 0 {
		batchSize = req.BatchSize
	}
	config["batch_size"] = batchSize

	workers := 4
	if req.Workers > 0 {
		workers = req.Workers
	}
	config["workers"] = workers

	timeout := 300
	if req.TimeoutSeconds > 0 {
		timeout = req.TimeoutSeconds
	}
	config["timeout_seconds"] = timeout

	extractSymbols := true
	if req.ExtractSymbols != nil {
		extractSymbols = *req.ExtractSymbols
	}
	config["extract_symbols"] = extractSymbols

	consolidate := true
	if req.Consolidate != nil {
		consolidate = *req.Consolidate
	}
	config["consolidate"] = consolidate

	config["include_tests"] = req.IncludeTests
	config["incremental"] = req.Incremental
	config["dry_run"] = req.DryRun

	if req.SinceCommit != "" {
		config["since_commit"] = req.SinceCommit
	}
	if len(req.ExcludeDirs) > 0 {
		config["exclude_dirs"] = req.ExcludeDirs
	}
	if req.Limit > 0 {
		config["limit"] = req.Limit
	}

	// Create job
	queue := jobs.GetQueue()
	job, ctx := queue.CreateJob(jobID, "ingest-codebase", config)

	// Start background ingestion
	go s.runIngestJob(ctx, job)

	// Return job reference
	writeJSON(w, http.StatusAccepted, models.IngestTriggerResponse{
		JobID:     jobID,
		SpaceID:   req.SpaceID,
		Status:    string(jobs.StatusPending),
		Message:   "Ingestion job created. Use GET /v1/memory/ingest/status/" + jobID + " to check progress.",
		CreatedAt: job.CreatedAt.Format(time.RFC3339),
	})
}

// ingestProgressEvent represents a structured JSON progress line from the CLI.
type ingestProgressEvent struct {
	Event    string  `json:"event"`
	Total    int     `json:"total,omitempty"`
	Current  int     `json:"current,omitempty"`
	Ingested int     `json:"ingested,omitempty"`
	Errors   int     `json:"errors,omitempty"`
	Symbols  int     `json:"symbols,omitempty"`
	Rate     float64 `json:"rate,omitempty"`
	Duration string  `json:"duration,omitempty"`
}

// runIngestJob executes the ingestion job in the background by delegating to the
// ingest-codebase CLI binary with --progress-json for streaming progress updates.
func (s *Server) runIngestJob(ctx context.Context, job *jobs.Job) {
	queue := jobs.GetQueue()
	queue.StartJob(job.ID)

	job.UpdateProgress(0, "initializing")

	spaceID, _ := job.Config["space_id"].(string)
	path, _ := job.Config["path"].(string)

	slog.Info("ingestion job started", "job_id", job.ID, "space_id", spaceID, "path", path)

	// INGEST-EXEC-001: scheduled-sync runs are unattended background jobs —
	// report their outcome to scheduled_job_events (NOSILENT pattern) so a
	// sync that keeps failing is never silent. Manual API-triggered jobs are
	// operator-visible through the job queue and are not reported here.
	if job.Type == "scheduled-sync" {
		startedAt := time.Now()
		defer func() {
			snap := job.GetSnapshot()
			var runErr error
			if snap.Status != jobs.StatusCompleted {
				runErr = fmt.Errorf("scheduled sync job %s ended in status %s: %s", job.ID, snap.Status, snap.Error)
			}
			var pool *pgxpool.Pool
			if s.tsdbClient != nil {
				pool = s.tsdbClient.Pool()
			}
			ev := tsdb.JobEventRow{
				JobName: "codebase-sync", SpaceID: spaceID,
				InstanceID: s.cfg.InstanceID,
				Success:    runErr == nil,
				LatencyMS:  time.Since(startedAt).Milliseconds(),
			}
			if runErr != nil {
				ev.ErrorMessage = runErr.Error()
			}
			jobhealth.Report(context.Background(), pool, s.alertDispatcher, ev)
		}()
	}

	// Build CLI arguments from job config
	args := buildIngestArgsFromConfig(job.Config, s.cfg.ListenAddr)

	cmd := exec.CommandContext(ctx, resolveMdemgBin(), append([]string{"ingest"}, args...)...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		job.Fail(fmt.Errorf("failed to create stdout pipe: %w", err))
		slog.Error("ingestion job failed", "job_id", job.ID, "error", err)
		return
	}

	// Capture stderr for error reporting
	cmd.Stderr = nil // Let stderr go to parent process stderr

	if err := cmd.Start(); err != nil {
		job.Fail(fmt.Errorf("failed to start ingest-codebase: %w", err))
		slog.Error("ingestion job failed to start", "job_id", job.ID, "error", err)
		return
	}

	// Scan stdout for JSON progress events
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		var evt ingestProgressEvent
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue // Skip non-JSON lines
		}

		switch evt.Event {
		case "discovery_complete":
			job.SetTotal(evt.Total)
			job.UpdateProgress(0, "discovery")
		case "batch_progress":
			job.UpdateProgress(evt.Current, "ingestion")
			if evt.Rate > 0 {
				job.SetRate(fmt.Sprintf("%.1f elements/sec", evt.Rate))
			}
		case "consolidation_start":
			snap := job.GetSnapshot()
			job.UpdateProgress(snap.Progress.Current, "consolidation")
		case "complete":
			job.Complete(map[string]any{
				"space_id": spaceID,
				"path":     path,
				"total":    evt.Total,
				"ingested": evt.Ingested,
				"errors":   evt.Errors,
				"symbols":  evt.Symbols,
				"duration": evt.Duration,
			})
			slog.Info("ingestion job completed", "job_id", job.ID, "total", evt.Total, "ingested", evt.Ingested, "errors", evt.Errors)
			// Update TapRoot freshness on successful completion
			if err := s.retriever.UpdateTapRootFreshness(context.Background(), spaceID, "codebase-ingest", IsPrunablePrefix(spaceID)); err != nil {
				slog.Warn("failed to update TapRoot freshness", "space_id", spaceID, "error", err)
			}
			s.TriggerAPEEventWithContext("source_changed", map[string]string{
				"space_id":    spaceID,
				"file_count":  fmt.Sprintf("%d", evt.Ingested),
				"ingest_type": "codebase-ingest",
			})
			s.TriggerAPEEventWithContext("ingest_complete", map[string]string{
				"space_id":    spaceID,
				"file_count":  fmt.Sprintf("%d", evt.Ingested),
				"ingest_type": "codebase-ingest",
			})
		}
	}

	// Wait for the process to exit
	if err := cmd.Wait(); err != nil {
		// Only fail if the job wasn't already completed by a "complete" event
		snap := job.GetSnapshot()
		if snap.Status != jobs.StatusCompleted {
			job.Fail(fmt.Errorf("ingest-codebase exited with error: %w", err))
			slog.Error("ingestion job failed", "job_id", job.ID, "error", err)
		}
		return
	}

	// If we never got a "complete" event, mark as completed with minimal info
	snap := job.GetSnapshot()
	if snap.Status != jobs.StatusCompleted {
		job.Complete(map[string]any{
			"space_id": spaceID,
			"path":     path,
			"message":  "completed without progress events",
		})
		slog.Info("ingestion job completed without progress events", "job_id", job.ID)
		// Update TapRoot freshness on fallback completion
		if err := s.retriever.UpdateTapRootFreshness(context.Background(), spaceID, "codebase-ingest", IsPrunablePrefix(spaceID)); err != nil {
			slog.Warn("failed to update TapRoot freshness", "space_id", spaceID, "error", err)
		}
		s.TriggerAPEEventWithContext("ingest_complete", map[string]string{
			"space_id":    spaceID,
			"ingest_type": "codebase-ingest",
		})
	}
}

// buildIngestArgsFromConfig constructs CLI arguments for ingest-codebase from a job config map.
func buildIngestArgsFromConfig(config map[string]any, listenAddr string) []string {
	// Resolve endpoint
	endpoint := listenAddr
	if endpoint == "" {
		endpoint = "http://localhost:9999"
	} else if !strings.HasPrefix(endpoint, "http") {
		if strings.HasPrefix(endpoint, ":") {
			endpoint = "http://localhost" + endpoint
		} else {
			endpoint = "http://" + endpoint
		}
	}

	path, _ := config["path"].(string)
	spaceID, _ := config["space_id"].(string)

	args := []string{
		"--path", path,
		"--space-id", spaceID,
		"--endpoint", endpoint,
		"--progress-json",
	}

	if v, ok := config["batch_size"].(int); ok && v > 0 {
		args = append(args, fmt.Sprintf("--batch=%d", v))
	}
	if v, ok := config["workers"].(int); ok && v > 0 {
		args = append(args, fmt.Sprintf("--workers=%d", v))
	}
	if v, ok := config["timeout_seconds"].(int); ok && v > 0 {
		args = append(args, fmt.Sprintf("--timeout=%d", v))
	}
	if v, ok := config["extract_symbols"].(bool); ok {
		args = append(args, fmt.Sprintf("--extract-symbols=%t", v))
	}
	if v, ok := config["consolidate"].(bool); ok {
		args = append(args, fmt.Sprintf("--consolidate=%t", v))
	}
	if v, ok := config["include_tests"].(bool); ok && v {
		args = append(args, "--include-tests")
	}
	if v, ok := config["incremental"].(bool); ok && v {
		args = append(args, "--incremental")
	}
	if v, ok := config["dry_run"].(bool); ok && v {
		args = append(args, "--dry-run")
	}
	if v, ok := config["since_commit"].(string); ok && v != "" {
		args = append(args, "--since", v)
	}
	if v, ok := config["exclude_dirs"].([]string); ok && len(v) > 0 {
		args = append(args, "--exclude", strings.Join(v, ","))
	}
	// Handle exclude_dirs stored as []any (from JSON deserialization)
	if v, ok := config["exclude_dirs"].([]any); ok && len(v) > 0 {
		dirs := make([]string, 0, len(v))
		for _, d := range v {
			if s, ok := d.(string); ok {
				dirs = append(dirs, s)
			}
		}
		if len(dirs) > 0 {
			args = append(args, "--exclude", strings.Join(dirs, ","))
		}
	}
	if v, ok := config["limit"].(int); ok && v > 0 {
		args = append(args, fmt.Sprintf("--limit=%d", v))
	}
	if v, ok := config["archive_deleted"].(bool); ok {
		args = append(args, fmt.Sprintf("--archive-deleted=%t", v))
	}
	if v, ok := config["quiet"].(bool); ok && v {
		args = append(args, "--quiet")
	}
	if v, ok := config["log_file"].(string); ok && v != "" {
		args = append(args, "--log-file", v)
	}

	return args
}

// handleIngestStatus handles GET /v1/memory/ingest/status/{job_id}
// Returns the status and progress of an ingestion job.
func (s *Server) handleIngestStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract job_id from path: /v1/memory/ingest/status/{job_id}
	path := strings.TrimPrefix(r.URL.Path, "/v1/memory/ingest/status/")
	jobID := path
	if jobID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "job_id required in path"})
		return
	}

	// Look up job
	queue := jobs.GetQueue()
	job, ok := queue.GetJob(jobID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "job not found: " + jobID})
		return
	}

	snapshot := job.GetSnapshot()

	// Build response
	resp := models.IngestJobStatusResponse{
		JobID:     snapshot.ID,
		Status:    string(snapshot.Status),
		CreatedAt: snapshot.CreatedAt.Format(time.RFC3339),
		Progress: models.IngestProgress{
			Total:      snapshot.Progress.Total,
			Current:    snapshot.Progress.Current,
			Percentage: snapshot.Progress.Percentage,
			Phase:      snapshot.Progress.Phase,
			Rate:       snapshot.Progress.Rate,
		},
	}

	if spaceID, ok := snapshot.Config["space_id"].(string); ok {
		resp.SpaceID = spaceID
	}

	if snapshot.StartedAt != nil {
		resp.StartedAt = snapshot.StartedAt.Format(time.RFC3339)
	}
	if snapshot.CompletedAt != nil {
		resp.CompletedAt = snapshot.CompletedAt.Format(time.RFC3339)
	}
	if snapshot.Result != nil {
		resp.Result = snapshot.Result
	}
	if snapshot.Error != "" {
		resp.Error = snapshot.Error
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleIngestCancel handles POST /v1/memory/ingest/cancel/{job_id}
// Cancels a running ingestion job.
func (s *Server) handleIngestCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract job_id from path: /v1/memory/ingest/cancel/{job_id}
	path := strings.TrimPrefix(r.URL.Path, "/v1/memory/ingest/cancel/")
	jobID := path
	if jobID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "job_id required in path"})
		return
	}

	// Cancel job
	queue := jobs.GetQueue()
	if queue.CancelJob(jobID) {
		writeJSON(w, http.StatusOK, map[string]any{
			"job_id":  jobID,
			"status":  "cancelled",
			"message": "Job cancellation requested",
		})
	} else {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "job not found or already completed: " + jobID})
	}
}

// handleIngestJobs handles GET /v1/memory/ingest/jobs
// Lists all ingestion jobs.
func (s *Server) handleIngestJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	queue := jobs.GetQueue()
	jobList := queue.ListJobs("ingest-codebase")

	// Convert to response format
	respJobs := make([]map[string]any, 0, len(jobList))
	for i := range jobList {
		job := &jobList[i] // Use pointer to avoid copying mutex
		jobMap := map[string]any{
			"job_id":     job.ID,
			"status":     string(job.Status),
			"created_at": job.CreatedAt.Format(time.RFC3339),
			"progress": map[string]any{
				"total":      job.Progress.Total,
				"current":    job.Progress.Current,
				"percentage": job.Progress.Percentage,
				"phase":      job.Progress.Phase,
			},
		}
		if spaceID, ok := job.Config["space_id"].(string); ok {
			jobMap["space_id"] = spaceID
		}
		if job.StartedAt != nil {
			jobMap["started_at"] = job.StartedAt.Format(time.RFC3339)
		}
		if job.CompletedAt != nil {
			jobMap["completed_at"] = job.CompletedAt.Format(time.RFC3339)
		}
		respJobs = append(respJobs, jobMap)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"jobs":  respJobs,
		"count": len(respJobs),
	})
}

// handleIngestFiles handles POST /v1/memory/ingest/files
// Re-ingests specific files into memory. Synchronous for ≤50 files, background job for >50.
func (s *Server) handleIngestFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req models.IngestFilesRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	// Validate all file paths exist before processing
	var missing []string
	for _, f := range req.Files {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":         "one or more files do not exist",
			"missing_files": missing,
		})
		return
	}

	// Default extract_symbols to true
	extractSymbols := true
	if req.ExtractSymbols != nil {
		extractSymbols = *req.ExtractSymbols
	}

	// Default consolidate to false
	consolidate := false
	if req.Consolidate != nil {
		consolidate = *req.Consolidate
	}

	// For >50 files, run as a background job
	if len(req.Files) > 50 {
		jobID := "ingest-files-" + uuid.New().String()[:8]
		config := map[string]any{
			"space_id":        req.SpaceID,
			"files":           req.Files,
			"extract_symbols": extractSymbols,
			"consolidate":     consolidate,
		}
		queue := jobs.GetQueue()
		job, ctx := queue.CreateJob(jobID, "ingest-files", config)
		go s.runIngestFilesJob(ctx, job) //nolint:gosec // G118: ctx from CreateJob (Background-derived), not request-scoped

		writeJSON(w, http.StatusAccepted, models.IngestFilesResponse{
			SpaceID:    req.SpaceID,
			TotalFiles: len(req.Files),
			JobID:      jobID,
		})
		return
	}

	// Synchronous processing for ≤50 files
	results := s.ingestFiles(r.Context(), req.SpaceID, req.Files, extractSymbols)

	successCount := 0
	errorCount := 0
	for _, res := range results {
		if res.Status == "success" {
			successCount++
		} else {
			errorCount++
		}
	}

	// Optionally trigger consolidation
	if consolidate && successCount > 0 && s.hiddenLayer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		_, err := s.hiddenLayer.RunFullConversationConsolidation(ctx, req.SpaceID)
		cancel()
		if err != nil {
			slog.Error("post-ingest consolidation failed", "error", err)
		}
	}

	// Update TapRoot freshness after successful file ingest
	if successCount > 0 {
		if err := s.retriever.UpdateTapRootFreshness(r.Context(), req.SpaceID, "file-ingest", IsPrunablePrefix(req.SpaceID)); err != nil {
			slog.Warn("failed to update TapRoot freshness", "space_id", req.SpaceID, "error", err)
		}
		s.TriggerAPEEventWithContext("source_changed", map[string]string{
			"space_id":    req.SpaceID,
			"file_count":  fmt.Sprintf("%d", successCount),
			"ingest_type": "file-ingest",
		})
		s.TriggerAPEEventWithContext("ingest_complete", map[string]string{
			"space_id":    req.SpaceID,
			"file_count":  fmt.Sprintf("%d", successCount),
			"ingest_type": "file-ingest",
		})
	}

	writeJSON(w, http.StatusOK, models.IngestFilesResponse{
		SpaceID:      req.SpaceID,
		TotalFiles:   len(req.Files),
		SuccessCount: successCount,
		ErrorCount:   errorCount,
		Results:      results,
	})
}

// ingestFiles processes a list of files and returns per-file results.
// Callers must validate file existence before calling this function.
func (s *Server) ingestFiles(ctx context.Context, spaceID string, files []string, extractSymbols bool) []models.IngestFileResult {
	results := make([]models.IngestFileResult, 0, len(files))
	timestamp := time.Now().UTC().Format(time.RFC3339)

	for _, filePath := range files {
		result := models.IngestFileResult{File: filePath}

		// Read file content
		content, err := readFileContent(filePath)
		if err != nil {
			slog.Error("ingestFiles: failed to read file", "path", filePath, "error", err)
			result.Status = "error"
			result.Error = "failed to read file"
			results = append(results, result)
			continue
		}

		// Detect language from extension
		lang := detectLanguage(filePath)

		// Build tags
		tags := []string{"codebase-ingest", "file-ingest"}
		if lang != "" {
			tags = append(tags, lang)
		}

		// Ingest via retrieval service
		ingestReq := models.IngestRequest{
			SpaceID:   spaceID,
			Timestamp: timestamp,
			Source:    "file-ingest",
			Content:   content,
			Name:      filepath.Base(filePath),
			Path:      filePath,
			Tags:      tags,
		}

		resp, err := s.retriever.IngestObservation(ctx, ingestReq)
		if err != nil {
			slog.Error("ingestFiles: ingest failed", "path", filePath, "error", err)
			result.Status = "error"
			result.Error = "internal error during ingestion"
			results = append(results, result)
			continue
		}

		result.Status = "success"
		result.NodeID = resp.NodeID
		results = append(results, result)
	}

	return results
}

// runIngestFilesJob processes file ingestion as a background job (for >50 files).
func (s *Server) runIngestFilesJob(ctx context.Context, job *jobs.Job) {
	queue := jobs.GetQueue()
	queue.StartJob(job.ID)

	spaceID, _ := job.Config["space_id"].(string)
	extractSymbols, _ := job.Config["extract_symbols"].(bool)
	consolidate, _ := job.Config["consolidate"].(bool)

	// Extract files from config
	var files []string
	if v, ok := job.Config["files"].([]string); ok {
		files = v
	} else if v, ok := job.Config["files"].([]any); ok {
		for _, f := range v {
			if s, ok := f.(string); ok {
				files = append(files, s)
			}
		}
	}

	job.SetTotal(len(files))
	job.UpdateProgress(0, "ingesting")

	timestamp := time.Now().UTC().Format(time.RFC3339)
	successCount := 0
	errorCount := 0

	for i, filePath := range files {
		// Check for cancellation
		select {
		case <-ctx.Done():
			job.Fail(fmt.Errorf("job cancelled"))
			return
		default:
		}

		content, err := readFileContent(filePath)
		if err != nil {
			slog.Error("ingestFilesJob: failed to read file", "job_id", job.ID, "path", filePath, "error", err)
			errorCount++
			job.UpdateProgress(i+1, "ingesting")
			continue
		}

		lang := detectLanguage(filePath)
		tags := []string{"codebase-ingest", "file-ingest"}
		if lang != "" {
			tags = append(tags, lang)
		}
		_ = extractSymbols // symbols handled by the retrieval service

		ingestReq := models.IngestRequest{
			SpaceID:   spaceID,
			Timestamp: timestamp,
			Source:    "file-ingest",
			Content:   content,
			Name:      filepath.Base(filePath),
			Path:      filePath,
			Tags:      tags,
		}

		_, err = s.retriever.IngestObservation(ctx, ingestReq)
		if err != nil {
			slog.Error("ingestFilesJob: ingest failed", "job_id", job.ID, "path", filePath, "error", err)
			errorCount++
		} else {
			successCount++
		}

		job.UpdateProgress(i+1, "ingesting")
	}

	// Optionally trigger consolidation
	if consolidate && successCount > 0 && s.hiddenLayer != nil {
		job.UpdateProgress(len(files), "consolidation")
		consolidateCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		_, err := s.hiddenLayer.RunFullConversationConsolidation(consolidateCtx, spaceID)
		cancel()
		if err != nil {
			slog.Error("post-ingest consolidation failed", "job_id", job.ID, "error", err)
		}
	}

	job.Complete(map[string]any{
		"space_id":      spaceID,
		"total_files":   len(files),
		"success_count": successCount,
		"error_count":   errorCount,
	})

	// Update TapRoot freshness on successful file ingest job
	if successCount > 0 {
		if err := s.retriever.UpdateTapRootFreshness(context.Background(), spaceID, "file-ingest", IsPrunablePrefix(spaceID)); err != nil {
			slog.Warn("failed to update TapRoot freshness", "space_id", spaceID, "error", err)
		}
		s.TriggerAPEEventWithContext("source_changed", map[string]string{
			"space_id":    spaceID,
			"file_count":  fmt.Sprintf("%d", successCount),
			"ingest_type": "file-ingest-job",
		})
		s.TriggerAPEEventWithContext("ingest_complete", map[string]string{
			"space_id":    spaceID,
			"file_count":  fmt.Sprintf("%d", successCount),
			"ingest_type": "file-ingest-job",
		})
	}

	slog.Info("ingest files job completed", "job_id", job.ID, "success", successCount, "total", len(files))
}

// readFileContent reads the content of a file, returning it as a string.
func readFileContent(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// detectLanguage returns a language tag based on file extension.
func detectLanguage(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".java":
		return "java"
	case ".rs":
		return "rust"
	case ".c", ".h":
		return "c"
	case ".cpp", ".hpp", ".cc":
		return "cpp"
	case ".md":
		return "markdown"
	case ".sql":
		return "sql"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return ""
	}
}

// handlePoolMetrics handles GET /v1/system/pool-metrics
// Returns TSDB pgx connection pool and Go runtime metrics.
// (TSDB-CONSUME-001: previously returned the fake Neo4j "pool" numbers —
// a VerifyConnectivity probe counter; the neo4j driver has no pool API.)
func (s *Server) handlePoolMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	poolMetrics := map[string]any{"backend": "unavailable"}
	if s.tsdbClient != nil && s.tsdbClient.Pool() != nil {
		st := s.tsdbClient.Pool().Stat()
		poolMetrics = map[string]any{
			"backend":             "tsdb-pgx",
			"total_conns":         st.TotalConns(),
			"idle_conns":          st.IdleConns(),
			"acquired_conns":      st.AcquiredConns(),
			"constructing_conns":  st.ConstructingConns(),
			"max_conns":           st.MaxConns(),
			"acquire_count":       st.AcquireCount(),
			"empty_acquire_count": st.EmptyAcquireCount(),
		}
	}

	// Get Go runtime memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	writeJSON(w, http.StatusOK, map[string]any{
		"connection_pool": poolMetrics,
		"runtime": map[string]any{
			"goroutines":        runtime.NumGoroutine(),
			"heap_alloc_mb":     float64(memStats.HeapAlloc) / 1024 / 1024,
			"heap_sys_mb":       float64(memStats.HeapSys) / 1024 / 1024,
			"heap_objects":      memStats.HeapObjects,
			"gc_pause_ns":       memStats.PauseNs[(memStats.NumGC+255)%256],
			"gc_total_pause_ms": float64(memStats.PauseTotalNs) / 1e6,
			"num_gc":            memStats.NumGC,
		},
	})
}

// handleDistributionStats handles GET /v1/memory/distribution
// Returns score distribution statistics for monitoring learning phases.
// Query params:
//   - space_id (required): Memory space to get stats for
//   - history_limit: Number of historical distributions to include (default 10, max 100)
func (s *Server) handleDistributionStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query()
	spaceID := query.Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter is required"})
		return
	}

	// Parse history limit
	historyLimit := 10
	if limitStr := query.Get("history_limit"); limitStr != "" {
		if _, err := fmt.Sscanf(limitStr, "%d", &historyLimit); err != nil || historyLimit < 1 {
			historyLimit = 10
		}
		if historyLimit > 100 {
			historyLimit = 100
		}
	}

	monitor := retrieval.GetDistributionMonitor()

	// Fetch edge count from Neo4j to update learning phase
	edgeCount, err := s.fetchLearningEdgeCount(r.Context(), spaceID)
	if err != nil {
		slog.Warn("failed to fetch edge count for distribution stats", "error", err)
	} else {
		monitor.UpdateEdgeCount(spaceID, edgeCount)
	}

	// Get comprehensive stats
	stats := monitor.GetStats(spaceID)

	// Add phase thresholds for context
	stats.PhaseThresholds = map[string]int64{
		"learning":  retrieval.PhaseThresholds.Learning,
		"warm":      retrieval.PhaseThresholds.Warm,
		"saturated": retrieval.PhaseThresholds.Saturated,
	}

	// Always include both stats and history for consistent response shape
	history := monitor.GetHistory(spaceID, historyLimit)
	if history == nil {
		history = []retrieval.ScoreDistribution{}
	}
	resp := map[string]any{
		"stats":   stats,
		"history": history,
	}
	writeJSON(w, http.StatusOK, resp)
}

// fetchLearningEdgeCount queries Neo4j for the count of CO_ACTIVATED_WITH edges in a space
func (s *Server) fetchLearningEdgeCount(ctx context.Context, spaceID string) (int64, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `MATCH ()-[r:CO_ACTIVATED_WITH {space_id: $spaceId}]->()
RETURN count(r) AS edge_count`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			count, _ := res.Record().Get("edge_count")
			if c, ok := count.(int64); ok {
				return c, nil
			}
		}
		return int64(0), res.Err()
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

// handleNeo4jOverview returns a consolidated view of all spaces, database health,
// and backup status in a single response. Uses batched Cypher queries for efficiency.
func (s *Server) handleNeo4jOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ctx := r.Context()

	resp := models.Neo4jOverviewResponse{
		ComputedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// --- Database section ---
	dbOverview := models.DatabaseOverview{
		Status:  "healthy",
		Version: s.cfg.MdemgVersion,
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Schema version
	schemaVer, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `MATCH (s:SchemaMeta {key:'schema'}) RETURN coalesce(s.current_version,0) AS v`, nil)
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			v, _ := res.Record().Get("v")
			if iv, ok := v.(int64); ok {
				return int(iv), nil
			}
		}
		return 0, res.Err()
	})
	if err != nil {
		dbOverview.Status = "degraded"
		slog.Error("neo4j-overview: schema version query failed", "error", err)
	} else {
		dbOverview.SchemaVersion = schemaVer.(int)
	}

	// Global node count
	totalNodes, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `MATCH (n:MemoryNode) RETURN count(n) AS cnt`, nil)
		if err != nil {
			return int64(0), err
		}
		if res.Next(ctx) {
			c, _ := res.Record().Get("cnt")
			if iv, ok := c.(int64); ok {
				return iv, nil
			}
		}
		return int64(0), res.Err()
	})
	if err != nil {
		dbOverview.Status = "degraded"
		slog.Error("neo4j-overview: total nodes query failed", "error", err)
	} else {
		dbOverview.TotalNodes = totalNodes.(int64)
	}

	// Global edge count
	totalEdges, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `MATCH ()-[r]->() RETURN count(r) AS cnt`, nil)
		if err != nil {
			return int64(0), err
		}
		if res.Next(ctx) {
			c, _ := res.Record().Get("cnt")
			if iv, ok := c.(int64); ok {
				return iv, nil
			}
		}
		return int64(0), res.Err()
	})
	if err != nil {
		dbOverview.Status = "degraded"
		slog.Error("neo4j-overview: total edges query failed", "error", err)
	} else {
		dbOverview.TotalEdges = totalEdges.(int64)
	}

	// --- Spaces section (batched queries) ---

	// 1. Nodes per space, grouped by layer
	type spaceLayerRow struct {
		SpaceID     string
		TotalNodes  int64
		LayerCounts map[int]int64
	}
	spaceData := make(map[string]*spaceLayerRow)

	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
			MATCH (n:MemoryNode)
			WHERE n.space_id IS NOT NULL
			WITH n.space_id AS sid, coalesce(n.layer, 0) AS layer, count(n) AS cnt
			RETURN sid, layer, cnt
			ORDER BY sid, layer`, nil)
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			rec := res.Record()
			sid, _ := rec.Get("sid")
			layer, _ := rec.Get("layer")
			cnt, _ := rec.Get("cnt")
			spaceID := sid.(string)
			layerInt := int(layer.(int64))
			countInt := cnt.(int64)

			row, ok := spaceData[spaceID]
			if !ok {
				row = &spaceLayerRow{SpaceID: spaceID, LayerCounts: make(map[int]int64)}
				spaceData[spaceID] = row
			}
			row.TotalNodes += countInt
			row.LayerCounts[layerInt] = countInt
		}
		return nil, res.Err()
	})
	if err != nil {
		dbOverview.Status = "degraded"
		slog.Error("neo4j-overview: space node counts query failed", "error", err)
	}

	dbOverview.TotalSpaces = len(spaceData)

	// 2. Total edges per space (all relationship types)
	edgeCounts := make(map[string]int64)
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
			MATCH (a:MemoryNode)-[r]->(b:MemoryNode)
			WHERE a.space_id IS NOT NULL AND a.space_id = b.space_id
			RETURN a.space_id AS sid, count(r) AS edge_count`, nil)
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			rec := res.Record()
			sid, _ := rec.Get("sid")
			ec, _ := rec.Get("edge_count")
			edgeCounts[sid.(string)] = ec.(int64)
		}
		return nil, res.Err()
	})
	if err != nil {
		slog.Error("neo4j-overview: space edge counts query failed", "error", err)
	}

	// 2b. Learning edges per space (CO_ACTIVATED_WITH only)
	learningEdgeCounts := make(map[string]int64)
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
			MATCH (a:MemoryNode)-[r:CO_ACTIVATED_WITH]->(b:MemoryNode)
			WHERE a.space_id IS NOT NULL AND a.space_id = b.space_id
			RETURN a.space_id AS sid, count(r) AS edge_count`, nil)
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			rec := res.Record()
			sid, _ := rec.Get("sid")
			ec, _ := rec.Get("edge_count")
			learningEdgeCounts[sid.(string)] = ec.(int64)
		}
		return nil, res.Err()
	})
	if err != nil {
		slog.Error("neo4j-overview: learning edge counts query failed", "error", err)
	}

	// 3. Observation counts per space
	obsCounts := make(map[string]int64)
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
			MATCH (n:MemoryNode)
			WHERE n.space_id IS NOT NULL AND n.role_type = 'conversation_observation'
			RETURN n.space_id AS sid, count(n) AS obs_count`, nil)
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			rec := res.Record()
			sid, _ := rec.Get("sid")
			oc, _ := rec.Get("obs_count")
			obsCounts[sid.(string)] = oc.(int64)
		}
		return nil, res.Err()
	})
	if err != nil {
		slog.Error("neo4j-overview: observation counts query failed", "error", err)
	}

	// 4. Orphan counts per space (nodes with no edges)
	orphanCounts := make(map[string]int64)
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
			MATCH (n:MemoryNode)
			WHERE n.space_id IS NOT NULL AND NOT (n)--()
			RETURN n.space_id AS sid, count(n) AS orphan_count`, nil)
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			rec := res.Record()
			sid, _ := rec.Get("sid")
			oc, _ := rec.Get("orphan_count")
			orphanCounts[sid.(string)] = oc.(int64)
		}
		return nil, res.Err()
	})
	if err != nil {
		slog.Error("neo4j-overview: orphan counts query failed", "error", err)
	}

	// 5. Last consolidation and ingest timestamps per space
	type spaceTimestamps struct {
		LastConsolidation string
		LastIngest        string
		LastIngestType    string
		IngestCount       int
	}
	timestamps := make(map[string]*spaceTimestamps)
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
			MATCH (n:MemoryNode)
			WHERE n.space_id IS NOT NULL AND n.layer > 0
			WITH n.space_id AS sid, max(n.created_at) AS last_consolidation
			RETURN sid, last_consolidation`, nil)
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			rec := res.Record()
			sid, _ := rec.Get("sid")
			lc, _ := rec.Get("last_consolidation")
			spaceID := sid.(string)
			ts, ok := timestamps[spaceID]
			if !ok {
				ts = &spaceTimestamps{}
				timestamps[spaceID] = ts
			}
			if s, ok := lc.(string); ok {
				ts.LastConsolidation = s
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		slog.Error("neo4j-overview: consolidation timestamps query failed", "error", err)
	}

	// Ingest timestamps (codebase nodes)
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
			MATCH (n:MemoryNode)
			WHERE n.space_id IS NOT NULL AND n.role_type IN ['codebase_file','codebase_symbol']
			WITH n.space_id AS sid, max(n.created_at) AS last_ingest, n.role_type AS rt, count(n) AS cnt
			RETURN sid, last_ingest, rt, cnt
			ORDER BY sid, last_ingest DESC`, nil)
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			rec := res.Record()
			sid, _ := rec.Get("sid")
			li, _ := rec.Get("last_ingest")
			rt, _ := rec.Get("rt")
			cnt, _ := rec.Get("cnt")
			spaceID := sid.(string)
			ts, ok := timestamps[spaceID]
			if !ok {
				ts = &spaceTimestamps{}
				timestamps[spaceID] = ts
			}
			if s, ok := li.(string); ok && (ts.LastIngest == "" || s > ts.LastIngest) {
				ts.LastIngest = s
				if rts, ok := rt.(string); ok {
					ts.LastIngestType = rts
				}
			}
			if c, ok := cnt.(int64); ok {
				ts.IngestCount += int(c)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		slog.Error("neo4j-overview: ingest timestamps query failed", "error", err)
	}

	// Build space overview list
	spaces := make([]models.SpaceOverview, 0, len(spaceData))
	for spaceID, row := range spaceData {
		so := models.SpaceOverview{
			SpaceID:          spaceID,
			NodeCount:        row.TotalNodes,
			EdgeCount:        edgeCounts[spaceID],
			NodesByLayer:     row.LayerCounts,
			ObservationCount: obsCounts[spaceID],
			LearningEdges:    learningEdgeCounts[spaceID],
			OrphanCount:      orphanCounts[spaceID],
		}

		// Health score: simple formula based on orphan ratio and edge density
		if row.TotalNodes > 0 {
			orphanRatio := float64(orphanCounts[spaceID]) / float64(row.TotalNodes)
			connectScore := 1.0 - orphanRatio
			edgeDensity := float64(edgeCounts[spaceID]) / float64(row.TotalNodes)
			if edgeDensity > 1.0 {
				edgeDensity = 1.0
			}
			so.HealthScore = connectScore*0.6 + edgeDensity*0.4
			if so.HealthScore < 0 {
				so.HealthScore = 0
			}
		}

		// Staleness: no consolidation in the last 7 days for spaces with >10 observations
		if ts, ok := timestamps[spaceID]; ok {
			so.LastConsolidation = ts.LastConsolidation
			so.LastIngest = ts.LastIngest
			so.LastIngestType = ts.LastIngestType
			so.IngestCount = ts.IngestCount
			if ts.LastConsolidation != "" && obsCounts[spaceID] > 10 {
				if t, err := time.Parse(time.RFC3339, ts.LastConsolidation); err == nil {
					so.IsStale = time.Since(t) > 7*24*time.Hour
				}
			}
		}

		spaces = append(spaces, so)
	}
	resp.Spaces = spaces
	resp.Database = dbOverview

	// --- Backups section ---
	backupOverview := models.BackupOverview{}
	if s.backupSvc != nil {
		manifests, err := s.backupSvc.ListBackups("", 0)
		if err != nil {
			slog.Error("neo4j-overview: backup list failed", "error", err)
		} else {
			backupOverview.TotalCount = len(manifests)
			for i := range manifests {
				m := &manifests[i]
				if m.Type == "full" && backupOverview.LastFull == nil {
					backupOverview.LastFull = &models.BackupSummary{
						BackupID:  m.BackupID,
						CreatedAt: m.CreatedAt,
						SizeBytes: m.SizeBytes,
						Spaces:    m.Spaces,
					}
				}
				if m.Type == "partial_space" && backupOverview.LastPartial == nil {
					backupOverview.LastPartial = &models.BackupSummary{
						BackupID:  m.BackupID,
						CreatedAt: m.CreatedAt,
						SizeBytes: m.SizeBytes,
						Spaces:    m.Spaces,
					}
				}
			}
		}
	}
	resp.Backups = backupOverview

	writeJSON(w, http.StatusOK, resp)
}

// handleMetricsTrends serves historical metric trend data from TimescaleDB.
// GET /v1/metrics/trends?space_id=X&metric=Y&from=Z&to=W&granularity=daily
func (s *Server) handleMetricsTrends(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if s.tsdbClient == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "TimescaleDB not available"})
		return
	}

	q := r.URL.Query()
	spaceID := q.Get("space_id")
	metric := q.Get("metric")
	fromStr := q.Get("from")
	toStr := q.Get("to")
	granularity := q.Get("granularity")

	if spaceID == "" || metric == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "space_id and metric are required"})
		return
	}

	now := time.Now()
	from := now.AddDate(0, 0, -30) // default: last 30 days
	to := now

	if fromStr != "" {
		parsed, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			// Try date-only format
			parsed, err = time.Parse("2006-01-02", fromStr)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid 'from' timestamp (use RFC3339 or YYYY-MM-DD)"})
				return
			}
		}
		from = parsed
	}
	if toStr != "" {
		parsed, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			parsed, err = time.Parse("2006-01-02", toStr)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid 'to' timestamp (use RFC3339 or YYYY-MM-DD)"})
				return
			}
		}
		to = parsed
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	points, err := s.tsdbClient.Query(ctx, tsdb.MetricQuery{
		SpaceID:     spaceID,
		MetricName:  metric,
		From:        from,
		To:          to,
		Granularity: granularity,
	})
	if err != nil {
		slog.Error("metrics/trends query failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	// Return as array (empty array if no data — matches UATS spec)
	type trendPoint struct {
		Time       string  `json:"time"`
		Value      float64 `json:"value"`
		MinValue   float64 `json:"min_value,omitempty"`
		MaxValue   float64 `json:"max_value,omitempty"`
		Count      int64   `json:"count,omitempty"`
		QualityTag string  `json:"quality_tag,omitempty"`
	}
	result := make([]trendPoint, 0, len(points))
	for _, p := range points {
		result = append(result, trendPoint{
			Time:       p.Time.Format(time.RFC3339),
			Value:      p.Value,
			MinValue:   p.MinValue,
			MaxValue:   p.MaxValue,
			Count:      p.Count,
			QualityTag: p.QualityTag,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// deriveQueryFingerprint constructs a sparse context fingerprint for the
// query. Phase 14.2.1 retune: vector-based cosine similarity between the
// query embedding and each catalog ref's embedding (computed once per
// (space, version), cached in-process via Server.contextFPCache).
//
// Phase 14.2 used a tokenize-and-match approach that produced empty
// fingerprints for most queries because catalog tags are LLM-summary
// buckets (`api`, `architecture`, `caching`...) while UVTS / domain
// queries use code/feature vocabulary that rarely intersects them
// literally. Vector matching surfaces semantic affinity instead — e.g.
// the query "barrel ownership transfer" can land on the `architecture`
// or `service_relationships` ref bits even though no token overlaps.
//
// Returns nil on: missing cache (cfg.ContextFingerprintEnabled=false),
// missing catalog (cold-start), embedder error (fail-open — the
// ContextColumn gracefully votes 0 when fingerprint is empty rather
// than blocking the retrieve).
// CONTEXT-LIVE-001: also returns the active catalog version the bits were
// derived against, so retrieval can refuse cross-version Jaccard.
func (s *Server) deriveQueryFingerprint(ctx context.Context, spaceID, queryText string) ([]uint16, int) {
	if s == nil || s.driver == nil || s.contextFPCache == nil || spaceID == "" || queryText == "" {
		return nil, 0
	}
	loader := hidden.NewNeo4jLoader(s.driver)
	cat, err := loader.LoadActive(ctx, spaceID)
	if err != nil || cat == nil {
		return nil, 0
	}
	topK := s.cfg.ContextFingerprintQueryTopK
	if topK <= 0 {
		topK = 8
	}
	// Attribute the query + catalog-ref embeds done inside derive/getOrBuild
	// so they record a call_site instead of an empty one (EMBED-CALLSITE-001).
	ctx = embeddings.WithEmbeddingMeta(ctx, embeddings.EmbeddingMeta{
		CallSite: "context_fingerprint",
		SpaceID:  spaceID,
	})
	bits, err := s.contextFPCache.derive(ctx, cat, queryText, topK)
	if err != nil {
		slog.Warn("context fingerprint derive failed", "space_id", spaceID, "error", err)
		return nil, 0
	}
	return bits, cat.Version
}
