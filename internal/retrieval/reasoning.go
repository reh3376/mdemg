package retrieval

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	pb "mdemg/api/modulepb"
	"mdemg/internal/models"
	"mdemg/internal/plugins"
)

// ReasoningProvider abstracts the interface for reasoning modules
// that can process/re-rank retrieval candidates.
type ReasoningProvider interface {
	// Process sends candidates to reasoning modules for re-ranking/processing.
	// Returns processed results, or original candidates if no modules are available.
	Process(ctx context.Context, req ReasoningRequest) (*ReasoningResult, error)

	// Available returns true if at least one reasoning module is ready.
	Available() bool
}

// ReasoningRequest contains input for reasoning modules
type ReasoningRequest struct {
	QueryText  string
	Candidates []models.RetrieveResult
	TopK       int
	Context    map[string]string // Additional context (e.g., space_id, user preferences)
}

// ReasoningResult contains output from reasoning modules
type ReasoningResult struct {
	Results    []models.RetrieveResult
	ModuleID   string // Which module processed the request
	LatencyMs  float64
	TokensUsed int
	Metadata   map[string]string
}

// PluginReasoningProvider implements ReasoningProvider using the plugin system
type PluginReasoningProvider struct {
	pluginMgr *plugins.Manager
}

// NewPluginReasoningProvider creates a new plugin-based reasoning provider
func NewPluginReasoningProvider(mgr *plugins.Manager) *PluginReasoningProvider {
	return &PluginReasoningProvider{pluginMgr: mgr}
}

// Available returns true if at least one reasoning module is ready
func (p *PluginReasoningProvider) Available() bool {
	if p.pluginMgr == nil {
		return false
	}
	modules := p.pluginMgr.GetReasoningModules()
	return len(modules) > 0
}

// Process sends candidates to reasoning modules for processing
func (p *PluginReasoningProvider) Process(ctx context.Context, req ReasoningRequest) (*ReasoningResult, error) {
	if p.pluginMgr == nil {
		return &ReasoningResult{Results: req.Candidates}, nil
	}

	modules := p.pluginMgr.GetReasoningModules()
	if len(modules) == 0 {
		return &ReasoningResult{Results: req.Candidates}, nil
	}

	// Use the first available reasoning module
	// Future: could implement priority/selection logic
	mod := modules[0]
	if mod.ReasoningClient == nil {
		slog.Warn("reasoning module has no client", "module_id", mod.Manifest.ID)
		return &ReasoningResult{Results: req.Candidates}, nil
	}

	start := time.Now()

	// Convert candidates to proto format
	pbCandidates := make([]*pb.RetrievalCandidate, len(req.Candidates))
	for i, c := range req.Candidates {
		pbCandidates[i] = &pb.RetrievalCandidate{
			NodeId:     c.NodeID,
			Path:       c.Path,
			Name:       c.Name,
			Summary:    c.Summary,
			Score:      float32(c.Score),
			VectorSim:  float32(c.VectorSim),
			Activation: float32(c.Activation),
			Metadata:   make(map[string]string),
		}
	}

	// Build proto request
	pbReq := &pb.ProcessRequest{
		QueryText:  req.QueryText,
		Candidates: pbCandidates,
		TopK:       int32(req.TopK),
		Context:    req.Context,
	}

	// Call the reasoning module
	resp, err := mod.ReasoningClient.Process(ctx, pbReq)
	if err != nil {
		slog.Warn("reasoning module failed", "module_id", mod.Manifest.ID, "error", err)
		return &ReasoningResult{
			Results:   req.Candidates, // Return original on error
			ModuleID:  mod.Manifest.ID,
			LatencyMs: float64(time.Since(start).Milliseconds()),
		}, err
	}

	if resp.Error != "" {
		slog.Warn("reasoning module returned error", "module_id", mod.Manifest.ID, "error", resp.Error)
		return &ReasoningResult{
			Results:   req.Candidates,
			ModuleID:  mod.Manifest.ID,
			LatencyMs: float64(time.Since(start).Milliseconds()),
		}, nil
	}

	// Build lookup map to preserve fields not carried through proto (e.g., Layer)
	originalByID := make(map[string]models.RetrieveResult, len(req.Candidates))
	for _, c := range req.Candidates {
		originalByID[c.NodeID] = c
	}

	// Convert proto results back to models
	results := make([]models.RetrieveResult, len(resp.Results))
	for i, r := range resp.Results {
		results[i] = models.RetrieveResult{
			NodeID:     r.NodeId,
			Path:       r.Path,
			Name:       r.Name,
			Summary:    r.Summary,
			Score:      float64(r.Score),
			VectorSim:  float64(r.VectorSim),
			Activation: float64(r.Activation),
		}
		// Restore fields not carried through proto — JIMINY-ROLETYPE-ADAPTER-001
		// added RoleType + ObsType to the restore set so reasoning-module reranks
		// (e.g. keyword-booster) do not silently clobber the ontology labels.
		if orig, ok := originalByID[r.NodeId]; ok {
			results[i].Layer = orig.Layer
			results[i].RoleType = orig.RoleType
			results[i].ObsType = orig.ObsType
		}
	}

	// Parse tokens used from metadata if present
	tokensUsed := 0
	if resp.Metadata != nil {
		if t, ok := resp.Metadata["tokens_used"]; ok {
			fmt.Sscanf(t, "%d", &tokensUsed)
		}
	}

	return &ReasoningResult{
		Results:    results,
		ModuleID:   mod.Manifest.ID,
		LatencyMs:  float64(time.Since(start).Milliseconds()),
		TokensUsed: tokensUsed,
		Metadata:   resp.Metadata,
	}, nil
}

// NoOpReasoningProvider is a provider that does nothing (passthrough)
type NoOpReasoningProvider struct{}

func (n *NoOpReasoningProvider) Available() bool { return false }
func (n *NoOpReasoningProvider) Process(ctx context.Context, req ReasoningRequest) (*ReasoningResult, error) {
	return &ReasoningResult{Results: req.Candidates}, nil
}
