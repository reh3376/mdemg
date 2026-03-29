package main

import (
	"context"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	pb "mdemg/api/modulepb"
)

// TestReasoningPluginHandler implements the REASONING module interfaces
type TestReasoningPluginHandler struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedReasoningModuleServer

	mu              sync.Mutex
	startTime       time.Time
	requestsHandled int64
	boostFactor     float64
}

// NewTestReasoningPluginHandler creates a new handler instance
func NewTestReasoningPluginHandler() *TestReasoningPluginHandler {
	return &TestReasoningPluginHandler{
		startTime:   time.Now(),
		boostFactor: 0.2, // Default boost factor
	}
}

// ============ Lifecycle RPCs (Required for ALL modules) ============

// Handshake is called immediately after spawn to verify module is ready.
func (h *TestReasoningPluginHandler) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	log.Printf("%s: handshake from MDEMG %s", moduleID, req.MdemgVersion)

	// Parse configuration
	if factor, ok := req.Config["boost_factor"]; ok {
		if f, err := strconv.ParseFloat(factor, 64); err == nil {
			h.boostFactor = f
			log.Printf("%s: boost_factor set to %.2f", moduleID, h.boostFactor)
		}
	}

	return &pb.HandshakeResponse{
		ModuleId:      moduleID,
		ModuleVersion: moduleVersion,
		ModuleType:    pb.ModuleType_MODULE_TYPE_REASONING,
		Capabilities:  []string{"custom_ranking"},
		Ready:         true,
	}, nil
}

// HealthCheck is called periodically to verify module health.
func (h *TestReasoningPluginHandler) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	h.mu.Lock()
	requests := h.requestsHandled
	h.mu.Unlock()

	return &pb.HealthCheckResponse{
		Healthy: true,
		Status:  "ready",
		Metrics: map[string]string{
			"uptime":           time.Since(h.startTime).String(),
			"requests_handled": strconv.FormatInt(requests, 10),
			"boost_factor":     strconv.FormatFloat(h.boostFactor, 'f', 2, 64),
		},
	}, nil
}

// Shutdown is called when MDEMG is stopping or the module is being disabled.
func (h *TestReasoningPluginHandler) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Printf("%s: shutdown requested (reason: %s)", moduleID, req.Reason)
	return &pb.ShutdownResponse{
		Success: true,
		Message: "goodbye",
	}, nil
}

// ============ Reasoning RPCs ============

// Process takes retrieval candidates and returns refined results.
func (h *TestReasoningPluginHandler) Process(ctx context.Context, req *pb.ProcessRequest) (*pb.ProcessResponse, error) {
	h.mu.Lock()
	h.requestsHandled++
	h.mu.Unlock()

	if len(req.Candidates) == 0 {
		return &pb.ProcessResponse{Results: req.Candidates}, nil
	}

	log.Printf("%s: processing %d candidates for: %s",
		moduleID, len(req.Candidates), truncate(req.QueryText, 50))

	// TODO: Implement your re-ranking/filtering logic here
	// Example: Simple keyword-based boosting

	// Extract keywords from query
	keywords := extractKeywords(req.QueryText)

	// Score and boost candidates
	type scored struct {
		candidate *pb.RetrievalCandidate
		boost     float64
	}

	results := make([]scored, len(req.Candidates))
	for i, c := range req.Candidates {
		// Calculate boost based on keyword matches
		boost := calculateBoost(c.Name, c.Summary, keywords) * h.boostFactor

		// Apply boost to score
		c.Score = c.Score + float32(boost)

		results[i] = scored{candidate: c, boost: boost}
	}

	// Sort by new score (descending)
	sort.Slice(results, func(i, j int) bool {
		return results[i].candidate.Score > results[j].candidate.Score
	})

	// Build output
	output := make([]*pb.RetrievalCandidate, len(results))
	for i, r := range results {
		output[i] = r.candidate
	}

	// Apply top_k limit
	if req.TopK > 0 && int(req.TopK) < len(output) {
		output = output[:req.TopK]
	}

	return &pb.ProcessResponse{
		Results: output,
		Metadata: map[string]string{
			"keywords":     strings.Join(keywords, ","),
			"boost_factor": strconv.FormatFloat(h.boostFactor, 'f', 2, 64),
		},
	}, nil
}

// ============ Helper Functions ============

// extractKeywords tokenizes the query into keywords (customize as needed)
func extractKeywords(query string) []string {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true,
		"of": true, "at": true, "by": true, "for": true, "with": true,
		"to": true, "from": true, "in": true, "out": true, "on": true,
		"and": true, "or": true, "but": true, "if": true, "what": true,
		"this": true, "that": true, "how": true, "why": true, "where": true,
	}

	words := strings.Fields(strings.ToLower(query))
	var keywords []string
	for _, word := range words {
		word = strings.Trim(word, ".,!?;:\"'()[]{}/-")
		if len(word) >= 2 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}
	return keywords
}

// calculateBoost scores how well a candidate matches the keywords
func calculateBoost(name, summary string, keywords []string) float64 {
	if len(keywords) == 0 {
		return 0
	}

	combined := strings.ToLower(name + " " + summary)
	matchCount := 0

	for _, kw := range keywords {
		if strings.Contains(combined, kw) {
			matchCount++
		}
	}

	return float64(matchCount) / float64(len(keywords))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
