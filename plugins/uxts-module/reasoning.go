package main

import (
	"context"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	pb "mdemg/api/modulepb"
)

// Process implements ReasoningModule.Process — the core retrieval annotation logic.
func (s *server) Process(ctx context.Context, req *pb.ProcessRequest) (*pb.ProcessResponse, error) {
	s.mu.Lock()
	s.requestsHandled++
	s.mu.Unlock()

	slog.Info("processing candidates", "module", moduleID, "count", len(req.Candidates), "query", truncate(req.QueryText, 50))

	if len(req.Candidates) == 0 {
		return &pb.ProcessResponse{
			Results:  req.Candidates,
			Metadata: map[string]string{"total_specs": strconv.Itoa(s.specIndex.TotalSpecs())},
		}, nil
	}

	totalAnnotated := 0

	for _, c := range req.Candidates {
		if c.Metadata == nil {
			c.Metadata = make(map[string]string)
		}

		specs := s.matchCandidate(c, req.QueryText)
		if len(specs) == 0 {
			c.Metadata["uxts_coverage_count"] = "0"
			c.Metadata["uxts_compliance"] = "untested"
			c.Metadata["uxts_drift"] = "clean"
			continue
		}

		totalAnnotated++
		annotateCandidate(c, specs, s.config.coverageBoost, s.config.failurePenalty)
	}

	// Re-sort by adjusted score
	sort.Slice(req.Candidates, func(i, j int) bool {
		return req.Candidates[i].Score > req.Candidates[j].Score
	})

	// Limit to top_k
	results := req.Candidates
	if req.TopK > 0 && int(req.TopK) < len(results) {
		results = results[:req.TopK]
	}

	stats := s.specIndex.FrameworkStats()
	fwStats := make([]string, 0, len(stats))
	for fw, count := range stats {
		fwStats = append(fwStats, fw+":"+strconv.Itoa(count))
	}

	return &pb.ProcessResponse{
		Results: results,
		Metadata: map[string]string{
			"total_specs":      strconv.Itoa(s.specIndex.TotalSpecs()),
			"candidates_annotated": strconv.Itoa(totalAnnotated),
			"framework_stats":  strings.Join(fwStats, ","),
		},
	}, nil
}

// matchCandidate finds specs relevant to a retrieval candidate.
func (s *server) matchCandidate(c *pb.RetrievalCandidate, queryText string) []*SpecEntry {
	var allSpecs []*SpecEntry
	seen := make(map[string]bool)

	addUnique := func(specs []*SpecEntry) {
		for _, sp := range specs {
			if !seen[sp.FilePath] {
				seen[sp.FilePath] = true
				allSpecs = append(allSpecs, sp)
			}
		}
	}

	// Match by HTTP path (from candidate path or summary)
	if c.Path != "" {
		addUnique(s.specIndex.FindByPath(c.Path))
	}

	// Match by name/summary keywords against spec text
	if c.Name != "" {
		addUnique(s.specIndex.FindByText(c.Name))
	}

	// If query mentions specific paths or services, match those too
	if strings.HasPrefix(queryText, "/v1/") || strings.Contains(queryText, "/v1/") {
		parts := strings.Fields(queryText)
		for _, p := range parts {
			if strings.HasPrefix(p, "/v1/") {
				addUnique(s.specIndex.FindByPath(p))
			}
		}
	}

	return allSpecs
}

// annotateCandidate adds UxTS metadata to a candidate and adjusts its score.
func annotateCandidate(c *pb.RetrievalCandidate, specs []*SpecEntry, coverageBoost, failurePenalty float64) {
	frameworks := make(map[string]bool)
	specNames := make([]string, 0, len(specs))
	overallCompliance := "pass"
	overallDrift := "clean"

	for _, sp := range specs {
		frameworks[sp.Framework] = true
		specNames = append(specNames, sp.FileName)

		if sp.Compliance == "fail" {
			overallCompliance = "fail"
		} else if sp.Compliance == "unknown" && overallCompliance != "fail" {
			overallCompliance = "unknown"
		}

		if sp.DriftState == "drift_detected" {
			overallDrift = "drift_detected"
		}
	}

	fwList := make([]string, 0, len(frameworks))
	for fw := range frameworks {
		fwList = append(fwList, fw)
	}
	sort.Strings(fwList)

	c.Metadata["uxts_coverage_count"] = strconv.Itoa(len(specs))
	c.Metadata["uxts_frameworks"] = strings.Join(fwList, ",")
	c.Metadata["uxts_spec_names"] = strings.Join(specNames, ",")
	c.Metadata["uxts_compliance"] = overallCompliance
	c.Metadata["uxts_drift"] = overallDrift

	// Score adjustment
	if len(specs) > 0 && overallCompliance != "fail" {
		c.Score += float32(coverageBoost)
	}
	if overallCompliance == "fail" {
		c.Score -= float32(failurePenalty)
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
