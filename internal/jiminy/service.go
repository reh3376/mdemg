package jiminy

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
	"mdemg/internal/embeddings"
	"mdemg/internal/models"
)

// ConsultingService defines the interface for the consulting.Service.Suggest() method.
type ConsultingService interface {
	Suggest(ctx context.Context, req models.SuggestRequest) (models.SuggestResponse, error)
}

// Service orchestrates all Jiminy guidance sources.
type Service struct {
	cfg        config.Config
	driver     neo4j.DriverWithContext
	consultant ConsultingService
	embedder   embeddings.Embedder
}

// NewService creates a new Jiminy guidance service.
func NewService(cfg config.Config, driver neo4j.DriverWithContext, consultant ConsultingService, embedder embeddings.Embedder) *Service {
	return &Service{
		cfg:        cfg,
		driver:     driver,
		consultant: consultant,
		embedder:   embedder,
	}
}

// Guide generates proactive guidance by fanning out to multiple knowledge sources
// in parallel: consulting.Suggest(), correction recall, contradiction checking,
// and frontier surfacing. Results are merged, deduplicated, and ranked.
func (s *Service) Guide(ctx context.Context, req GuidanceRequest) (GuidanceResponse, error) {
	if req.SpaceID == "" {
		return GuidanceResponse{}, fmt.Errorf("space_id is required")
	}
	if req.Context == "" {
		return GuidanceResponse{}, fmt.Errorf("context is required")
	}

	maxItems := req.MaxItems
	if maxItems <= 0 {
		maxItems = s.cfg.JiminyMaxItems
	}

	timeoutMs := s.cfg.JiminyTimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 6000
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	// Generate embedding for context
	contextText := req.Context
	if req.FilePath != "" {
		contextText = req.FilePath + ": " + contextText
	}

	var queryEmbedding []float32
	if s.embedder != nil {
		var err error
		queryEmbedding, err = s.embedder.Embed(ctx, contextText)
		if err != nil {
			log.Printf("jiminy: embedding failed (continuing without vector search): %v", err)
		}
	}

	// Fan out to 4 guidance sources in parallel
	var (
		mu       sync.Mutex
		items    []GuidanceItem
		warnings []string
		debug    = make(map[string]any)
	)

	var wg sync.WaitGroup

	// Source A: consulting.Suggest() — constraints, conflicts, patterns, concepts
	wg.Add(1)
	go func() {
		defer wg.Done()
		if s.consultant == nil {
			return
		}
		suggestResp, err := s.consultant.Suggest(ctx, models.SuggestRequest{
			SpaceID:            req.SpaceID,
			Context:            req.Context,
			FilePath:           req.FilePath,
			IncludeConflicts:   true,
			IncludeConstraints: true,
			MaxSuggestions:     maxItems,
			MinConfidence:      s.cfg.JiminyMinConfidence,
		})
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			debug["suggest_error"] = err.Error()
			warnings = append(warnings, "consulting.Suggest failed: "+err.Error())
			return
		}

		// Convert constraints to guidance items
		for _, c := range suggestResp.Constraints {
			items = append(items, GuidanceItem{
				Type:        GuidanceConstraint,
				Priority:    constraintPriority(c.ConstraintType),
				Content:     fmt.Sprintf("[%s] %s", c.ConstraintType, c.Description),
				Confidence:  c.Confidence,
				SourceNodes: c.SourceNodes,
			})
		}

		// Convert conflicts to guidance items
		for _, c := range suggestResp.Conflicts {
			items = append(items, GuidanceItem{
				Type:        GuidanceConflict,
				Priority:    c.Severity,
				Content:     c.Description,
				Confidence:  0.7,
				SourceNodes: c.SourceNodes,
			})
		}

		// Convert suggestions to pattern guidance items
		for _, sg := range suggestResp.Suggestions {
			items = append(items, GuidanceItem{
				Type:        GuidancePattern,
				Priority:    "medium",
				Content:     sg.Content,
				Confidence:  sg.Confidence,
				SourceNodes: sg.SourceNodes,
			})
		}

		debug["suggest_constraints"] = len(suggestResp.Constraints)
		debug["suggest_conflicts"] = len(suggestResp.Conflicts)
		debug["suggest_suggestions"] = len(suggestResp.Suggestions)
	}()

	// Source B+C: Correction vector search + contradiction checking (merged)
	// Previously Source C duplicated Source B's findRelevantCorrections call.
	// Now corrections feed directly into contradiction lookup in one goroutine.
	if queryEmbedding != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			corrections, err := s.findRelevantCorrections(ctx, req.SpaceID, queryEmbedding, 5)
			if err != nil {
				mu.Lock()
				debug["corrections_error"] = err.Error()
				mu.Unlock()
				return
			}

			// Collect correction items and node IDs for contradiction checking
			var correctionItems []GuidanceItem
			var nodeIDs []string
			for _, c := range corrections {
				content := c.Content
				if content == "" {
					content = c.Summary
				}
				correctionItems = append(correctionItems, GuidanceItem{
					Type:        GuidanceCorrection,
					Priority:    "high",
					Content:     content,
					Confidence:  c.Similarity,
					SourceNodes: []string{c.NodeID},
				})
				nodeIDs = append(nodeIDs, c.NodeID)
			}

			// Check contradictions using the same node IDs (no duplicate query)
			var contradictionItems []GuidanceItem
			if len(nodeIDs) > 0 {
				contradictions, cErr := s.findContradictions(ctx, req.SpaceID, nodeIDs)
				if cErr != nil {
					mu.Lock()
					debug["contradictions_error"] = cErr.Error()
					mu.Unlock()
				} else {
					for _, c := range contradictions {
						contradictionItems = append(contradictionItems, GuidanceItem{
							Type:     GuidanceConflict,
							Priority: "high",
							Content: fmt.Sprintf("%q contradicts %q (evidence: %d)",
								c.SourceName, c.TargetName, c.Evidence),
							Confidence:  c.Weight,
							SourceNodes: []string{c.SourceNodeID, c.TargetNodeID},
						})
					}
				}
			}

			mu.Lock()
			defer mu.Unlock()
			items = append(items, correctionItems...)
			items = append(items, contradictionItems...)
			debug["corrections_found"] = len(corrections)
			debug["contradictions_found"] = len(contradictionItems)
		}()
	}

	// Source D: Frontier node search (Phase J5)
	if queryEmbedding != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			frontiers, err := s.findRelevantFrontiers(ctx, req.SpaceID, queryEmbedding, 0, 3)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				debug["frontiers_error"] = err.Error()
				return
			}
			for _, f := range frontiers {
				content := f.Name
				if f.Summary != "" {
					content = f.Name + ": " + f.Summary
				}
				items = append(items, GuidanceItem{
					Type:        GuidanceFrontier,
					Priority:    "low",
					Content:     content,
					Confidence:  f.Similarity,
					SourceNodes: []string{f.NodeID},
				})
			}
			debug["frontiers_found"] = len(frontiers)
		}()
	}

	wg.Wait()

	// Filter by minimum confidence
	minConf := s.cfg.JiminyMinConfidence
	var filtered []GuidanceItem
	for _, item := range items {
		if item.Confidence >= minConf {
			filtered = append(filtered, item)
		}
	}

	// Deduplicate by content (simple dedup)
	filtered = deduplicateItems(filtered)

	// Sort by priority (high > medium > low) then confidence (desc)
	sort.Slice(filtered, func(i, j int) bool {
		pi, pj := priorityRank(filtered[i].Priority), priorityRank(filtered[j].Priority)
		if pi != pj {
			return pi < pj
		}
		return filtered[i].Confidence > filtered[j].Confidence
	})

	// Truncate to max items
	if len(filtered) > maxItems {
		filtered = filtered[:maxItems]
	}

	// Ensure non-nil slices for JSON serialization (nil → null, [] → [])
	if filtered == nil {
		filtered = []GuidanceItem{}
	}
	if warnings == nil {
		warnings = []string{}
	}

	// Compute source counts
	counts := SourceCounts{}
	for _, item := range filtered {
		switch item.Type {
		case GuidanceConstraint:
			counts.Constraints++
		case GuidanceCorrection:
			counts.Corrections++
		case GuidancePattern, GuidanceSuggestion:
			counts.Patterns++
		case GuidanceConflict, GuidanceRisk:
			counts.Conflicts++
		case GuidanceFrontier:
			counts.Frontiers++
		}
	}

	// Compute overall confidence
	confidence := computeOverallConfidence(filtered)

	// Build rationale
	rationale := buildRationale(counts)

	// Format prompt augmentation
	augmentation := FormatPromptAugmentation(filtered, counts, confidence)

	return GuidanceResponse{
		Guidance:           filtered,
		PromptAugmentation: augmentation,
		Confidence:         confidence,
		Rationale:          rationale,
		Warnings:           warnings,
		SourceCounts:       counts,
		Debug:              debug,
	}, nil
}

// constraintPriority maps constraint types to priorities.
func constraintPriority(constraintType string) string {
	switch constraintType {
	case "must", "must_not":
		return "high"
	case "should", "should_not":
		return "medium"
	default:
		return "low"
	}
}

// computeOverallConfidence averages the top-N item confidences.
func computeOverallConfidence(items []GuidanceItem) float64 {
	if len(items) == 0 {
		return 0
	}
	sum := 0.0
	n := len(items)
	if n > 5 {
		n = 5
	}
	for i := 0; i < n; i++ {
		sum += items[i].Confidence
	}
	return sum / float64(n)
}

// buildRationale constructs a human-readable summary of guidance sources.
func buildRationale(counts SourceCounts) string {
	total := counts.Constraints + counts.Corrections + counts.Patterns + counts.Conflicts + counts.Frontiers
	if total == 0 {
		return "No relevant guidance found for this context"
	}
	parts := []string{}
	if counts.Constraints > 0 {
		parts = append(parts, fmt.Sprintf("%d constraints", counts.Constraints))
	}
	if counts.Corrections > 0 {
		parts = append(parts, fmt.Sprintf("%d corrections", counts.Corrections))
	}
	if counts.Patterns > 0 {
		parts = append(parts, fmt.Sprintf("%d patterns", counts.Patterns))
	}
	if counts.Conflicts > 0 {
		parts = append(parts, fmt.Sprintf("%d conflicts", counts.Conflicts))
	}
	if counts.Frontiers > 0 {
		parts = append(parts, fmt.Sprintf("%d frontiers", counts.Frontiers))
	}
	return fmt.Sprintf("Found %d guidance items: %s", total, join(parts, ", "))
}

// join is a simple string join.
func join(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}

// deduplicateItems removes items with identical content.
func deduplicateItems(items []GuidanceItem) []GuidanceItem {
	seen := make(map[string]bool)
	var result []GuidanceItem
	for _, item := range items {
		if !seen[item.Content] {
			seen[item.Content] = true
			result = append(result, item)
		}
	}
	return result
}
