package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"

	pb "mdemg/api/modulepb"
)

const moduleID = "keyword-booster"
const moduleVersion = "1.0.0"

type server struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedReasoningModuleServer

	mu             sync.Mutex
	startTime      time.Time
	requestsHandled int64
	boostFactor    float64
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	socketPath := flag.String("socket", "", "Unix socket path")
	flag.Parse()

	if *socketPath == "" {
		slog.Error("missing required flag", "flag", "--socket")
		os.Exit(1)
	}

	// Remove stale socket
	os.Remove(*socketPath)

	// Create Unix socket listener
	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		slog.Error("failed to listen on socket", "error", err)
		os.Exit(1)
	}
	defer listener.Close()
	defer os.Remove(*socketPath)

	slog.Info("listening", "module", moduleID, "socket", *socketPath)

	// Create gRPC server
	grpcServer := grpc.NewServer()
	s := &server{
		startTime:   time.Now(),
		boostFactor: 0.2, // Default boost factor
	}

	pb.RegisterModuleLifecycleServer(grpcServer, s)
	pb.RegisterReasoningModuleServer(grpcServer, s)

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigChan
		slog.Info("received shutdown signal", "module", moduleID)
		grpcServer.GracefulStop()
	}()

	// Start serving
	if err := grpcServer.Serve(listener); err != nil {
		slog.Error("failed to serve", "error", err)
		os.Exit(1)
	}
}

// Handshake implements ModuleLifecycle.Handshake
func (s *server) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	slog.Info("handshake received", "module", moduleID, "mdemg_version", req.MdemgVersion)

	// Parse config
	if factor, ok := req.Config["boost_factor"]; ok {
		if f, err := strconv.ParseFloat(factor, 64); err == nil {
			s.boostFactor = f
			slog.Info("config applied", "module", moduleID, "boost_factor", s.boostFactor)
		}
	}

	return &pb.HandshakeResponse{
		ModuleId:      moduleID,
		ModuleVersion: moduleVersion,
		ModuleType:    pb.ModuleType_MODULE_TYPE_REASONING,
		Capabilities:  []string{"keyword_match", "term_frequency"},
		Ready:         true,
	}, nil
}

// HealthCheck implements ModuleLifecycle.HealthCheck
func (s *server) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	s.mu.Lock()
	requests := s.requestsHandled
	s.mu.Unlock()

	uptime := time.Since(s.startTime).String()

	return &pb.HealthCheckResponse{
		Healthy: true,
		Status:  "ready",
		Metrics: map[string]string{
			"uptime":           uptime,
			"requests_handled": strconv.FormatInt(requests, 10),
			"boost_factor":     strconv.FormatFloat(s.boostFactor, 'f', 2, 64),
		},
	}, nil
}

// Shutdown implements ModuleLifecycle.Shutdown
func (s *server) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	slog.Info("shutdown requested", "module", moduleID, "reason", req.Reason)
	return &pb.ShutdownResponse{
		Success: true,
		Message: "shutting down gracefully",
	}, nil
}

// Process implements ReasoningModule.Process
func (s *server) Process(ctx context.Context, req *pb.ProcessRequest) (*pb.ProcessResponse, error) {
	s.mu.Lock()
	s.requestsHandled++
	s.mu.Unlock()

	slog.Info("processing candidates", "module", moduleID, "count", len(req.Candidates), "query", truncate(req.QueryText, 50))

	if len(req.Candidates) == 0 {
		return &pb.ProcessResponse{
			Results: req.Candidates,
		}, nil
	}

	// Extract keywords from query (simple tokenization)
	keywords := extractKeywords(req.QueryText)
	slog.Info("extracted keywords", "module", moduleID, "keywords", keywords)

	// Score each candidate based on keyword matches
	type scoredCandidate struct {
		candidate  *pb.RetrievalCandidate
		matchScore float64
	}

	scored := make([]scoredCandidate, len(req.Candidates))
	for i, c := range req.Candidates {
		matchScore := calculateKeywordMatch(c.Name, c.Summary, keywords)
		scored[i] = scoredCandidate{
			candidate:  c,
			matchScore: matchScore,
		}
	}

	// Apply boost and re-sort
	for i := range scored {
		// Boost = original_score + (match_score * boost_factor)
		boost := scored[i].matchScore * s.boostFactor
		scored[i].candidate.Score = scored[i].candidate.Score + float32(boost)
	}

	// Sort by new score (descending)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].candidate.Score > scored[j].candidate.Score
	})

	// Build result
	results := make([]*pb.RetrievalCandidate, len(scored))
	for i, sc := range scored {
		results[i] = sc.candidate
	}

	// Limit to top_k if specified
	if req.TopK > 0 && int(req.TopK) < len(results) {
		results = results[:req.TopK]
	}

	return &pb.ProcessResponse{
		Results: results,
		Metadata: map[string]string{
			"keywords_extracted": strings.Join(keywords, ","),
			"boost_factor":       strconv.FormatFloat(s.boostFactor, 'f', 2, 64),
		},
	}, nil
}

// extractKeywords tokenizes the query into keywords
func extractKeywords(query string) []string {
	// Simple tokenization: split on spaces and remove common words
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "must": true, "shall": true,
		"of": true, "at": true, "by": true, "for": true, "with": true,
		"about": true, "against": true, "between": true, "into": true,
		"through": true, "during": true, "before": true, "after": true,
		"above": true, "below": true, "to": true, "from": true, "up": true,
		"down": true, "in": true, "out": true, "on": true, "off": true,
		"over": true, "under": true, "again": true, "further": true,
		"then": true, "once": true, "here": true, "there": true, "when": true,
		"where": true, "why": true, "how": true, "all": true, "each": true,
		"few": true, "more": true, "most": true, "other": true, "some": true,
		"such": true, "no": true, "nor": true, "not": true, "only": true,
		"own": true, "same": true, "so": true, "than": true, "too": true,
		"very": true, "can": true, "just": true, "and": true, "or": true,
		"but": true, "if": true, "what": true, "which": true, "who": true,
		"this": true, "that": true, "these": true, "those": true, "i": true,
		"me": true, "my": true, "we": true, "our": true, "you": true,
		"your": true, "he": true, "him": true, "his": true, "she": true,
		"her": true, "it": true, "its": true, "they": true, "them": true,
		"their": true,
	}

	words := strings.Fields(strings.ToLower(query))
	keywords := make([]string, 0, len(words))

	for _, word := range words {
		// Remove punctuation
		word = strings.Trim(word, ".,!?;:\"'()[]{}/-")
		if len(word) < 2 {
			continue
		}
		if stopWords[word] {
			continue
		}
		keywords = append(keywords, word)
	}

	return keywords
}

// calculateKeywordMatch scores how well a candidate matches the keywords
func calculateKeywordMatch(name, summary string, keywords []string) float64 {
	if len(keywords) == 0 {
		return 0
	}

	nameLower := strings.ToLower(name)
	summaryLower := strings.ToLower(summary)
	combined := nameLower + " " + summaryLower

	matchCount := 0
	for _, kw := range keywords {
		// Exact match in name is worth more
		if strings.Contains(nameLower, kw) {
			matchCount += 2
		}
		// Match in summary
		if strings.Contains(summaryLower, kw) {
			matchCount++
		}
		// Check for word boundaries (more precise match)
		if containsWord(combined, kw) {
			matchCount++
		}
	}

	// Normalize: max possible score is 4 * len(keywords)
	maxScore := float64(4 * len(keywords))
	return float64(matchCount) / maxScore
}

// containsWord checks if text contains the word as a whole word
func containsWord(text, word string) bool {
	// Simple check: word surrounded by spaces or at boundaries
	idx := strings.Index(text, word)
	if idx == -1 {
		return false
	}

	// Check before
	if idx > 0 {
		before := text[idx-1]
		if before != ' ' && before != '.' && before != ',' && before != ':' {
			return false
		}
	}

	// Check after
	end := idx + len(word)
	if end < len(text) {
		after := text[end]
		if after != ' ' && after != '.' && after != ',' && after != ':' {
			return false
		}
	}

	return true
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
