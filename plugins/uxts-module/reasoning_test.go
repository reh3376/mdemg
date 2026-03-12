package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pb "mdemg/api/modulepb"
)

func TestProcessEmpty(t *testing.T) {
	s := newServer()

	resp, err := s.Process(context.Background(), &pb.ProcessRequest{
		QueryText:  "test query",
		Candidates: nil,
		TopK:       10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(resp.Results))
	}
	if resp.Metadata["total_specs"] != "0" {
		t.Errorf("expected total_specs=0, got %s", resp.Metadata["total_specs"])
	}
}

func TestProcessWithCandidates(t *testing.T) {
	s := newServer()

	// Load fixture specs
	tmpDir := t.TempDir()
	uatsDir := filepath.Join(tmpDir, "api", "api-spec", "uats", "specs")
	if err := os.MkdirAll(uatsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	specJSON := `{
		"uats_version": "1.0.0",
		"request": {"method": "POST", "path": "/v1/memory/retrieve"},
		"expected": {"status": 200},
		"metadata": {"description": "Retrieve memory", "tags": ["memory"]}
	}`
	if err := os.WriteFile(filepath.Join(uatsDir, "retrieve.uats.json"), []byte(specJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.specIndex.Load(tmpDir, "uats", false); err != nil {
		t.Fatal(err)
	}

	resp, err := s.Process(context.Background(), &pb.ProcessRequest{
		QueryText: "memory retrieve",
		Candidates: []*pb.RetrievalCandidate{
			{
				NodeId:  "node-1",
				Path:    "/v1/memory/retrieve",
				Name:    "retrieve handler",
				Summary: "Handles memory retrieval",
				Score:   0.5,
			},
			{
				NodeId:  "node-2",
				Path:    "/v1/other/endpoint",
				Name:    "other handler",
				Summary: "Something else",
				Score:   0.6,
			},
		},
		TopK: 10,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}

	// Check that the matching candidate got annotated
	var matchedCandidate *pb.RetrievalCandidate
	for _, r := range resp.Results {
		if r.NodeId == "node-1" {
			matchedCandidate = r
			break
		}
	}
	if matchedCandidate == nil {
		t.Fatal("node-1 not found in results")
	}

	if matchedCandidate.Metadata["uxts_coverage_count"] == "0" {
		t.Error("expected coverage_count > 0 for matching candidate")
	}
	if matchedCandidate.Metadata["uxts_frameworks"] == "" {
		t.Error("expected uxts_frameworks to be set")
	}

	// Check non-matching candidate
	for _, r := range resp.Results {
		if r.NodeId == "node-2" {
			if r.Metadata["uxts_coverage_count"] != "0" {
				t.Errorf("expected coverage_count=0 for non-matching, got %s", r.Metadata["uxts_coverage_count"])
			}
			break
		}
	}
}

func TestProcessTopK(t *testing.T) {
	s := newServer()

	candidates := make([]*pb.RetrievalCandidate, 5)
	for i := range candidates {
		candidates[i] = &pb.RetrievalCandidate{
			NodeId: "node-" + string(rune('a'+i)),
			Score:  float32(i) * 0.1,
		}
	}

	resp, err := s.Process(context.Background(), &pb.ProcessRequest{
		QueryText:  "test",
		Candidates: candidates,
		TopK:       3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 3 {
		t.Errorf("expected 3 results with top_k=3, got %d", len(resp.Results))
	}
}

func TestAnnotateCandidate(t *testing.T) {
	c := &pb.RetrievalCandidate{
		NodeId:   "test-node",
		Score:    0.5,
		Metadata: make(map[string]string),
	}

	specs := []*SpecEntry{
		{Framework: "uats", FileName: "test.uats.json", Compliance: "pass", DriftState: "clean"},
		{Framework: "udts", FileName: "test.udts.json", Compliance: "unknown", DriftState: "clean"},
	}

	annotateCandidate(c, specs, 0.02, 0.05)

	if c.Metadata["uxts_coverage_count"] != "2" {
		t.Errorf("expected coverage_count=2, got %s", c.Metadata["uxts_coverage_count"])
	}
	if c.Metadata["uxts_compliance"] != "unknown" {
		t.Errorf("expected compliance=unknown (worst of pass+unknown), got %s", c.Metadata["uxts_compliance"])
	}
	if c.Metadata["uxts_drift"] != "clean" {
		t.Errorf("expected drift=clean, got %s", c.Metadata["uxts_drift"])
	}
	// Score should be boosted (not failed)
	if c.Score <= 0.5 {
		t.Errorf("expected score > 0.5 after boost, got %f", c.Score)
	}
}

func TestAnnotateCandidateFailure(t *testing.T) {
	c := &pb.RetrievalCandidate{
		NodeId:   "test-node",
		Score:    0.5,
		Metadata: make(map[string]string),
	}

	specs := []*SpecEntry{
		{Framework: "uats", FileName: "fail.uats.json", Compliance: "fail", DriftState: "drift_detected"},
	}

	annotateCandidate(c, specs, 0.02, 0.05)

	if c.Metadata["uxts_compliance"] != "fail" {
		t.Errorf("expected compliance=fail, got %s", c.Metadata["uxts_compliance"])
	}
	if c.Metadata["uxts_drift"] != "drift_detected" {
		t.Errorf("expected drift=drift_detected, got %s", c.Metadata["uxts_drift"])
	}
	// Score should be penalized
	if c.Score >= 0.5 {
		t.Errorf("expected score < 0.5 after penalty, got %f", c.Score)
	}
}

func TestTruncate(t *testing.T) {
	if truncate("hello", 10) != "hello" {
		t.Error("short string should not be truncated")
	}
	if truncate("hello world", 5) != "hello..." {
		t.Error("long string should be truncated")
	}
}
