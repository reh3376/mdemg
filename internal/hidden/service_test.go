package hidden

import (
	"testing"

	"mdemg/internal/config"
)

func TestAsString(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"int", 42, "42"},
		{"float", 3.14, "3.14"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := asString(tt.input)
			if result != tt.expected {
				t.Errorf("asString(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestAsInt(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected int
	}{
		{"nil", nil, 0},
		{"int64", int64(42), 42},
		{"int", int(123), 123},
		{"float64", float64(99.9), 99},
		{"string", "not a number", 0},
		{"bool", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := asInt(tt.input)
			if result != tt.expected {
				t.Errorf("asInt(%v) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestAsFloat64Slice(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected []float64
	}{
		{"nil", nil, nil},
		{
			"float64 slice",
			[]float64{1.0, 2.0, 3.0},
			[]float64{1.0, 2.0, 3.0},
		},
		{
			"any slice with float64",
			[]any{1.0, 2.0, 3.0},
			[]float64{1.0, 2.0, 3.0},
		},
		{
			"any slice with int64",
			[]any{int64(1), int64(2), int64(3)},
			[]float64{1.0, 2.0, 3.0},
		},
		{
			"any slice mixed",
			[]any{1.0, int64(2), 3.0},
			[]float64{1.0, 2.0, 3.0},
		},
		{
			"empty any slice",
			[]any{},
			[]float64{},
		},
		{"string (unsupported)", "not a slice", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := asFloat64Slice(tt.input)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("asFloat64Slice(%v) = %v, want nil", tt.input, result)
				}
				return
			}
			if len(result) != len(tt.expected) {
				t.Errorf("asFloat64Slice(%v) length = %d, want %d", tt.input, len(result), len(tt.expected))
				return
			}
			for i := range tt.expected {
				if result[i] != tt.expected[i] {
					t.Errorf("asFloat64Slice(%v)[%d] = %v, want %v", tt.input, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestHiddenNodeStruct(t *testing.T) {
	// Test that HiddenNode struct can be instantiated correctly
	node := HiddenNode{
		NodeID:               "test-hidden-1",
		SpaceID:              "test-space",
		Name:                 "Test Hidden Pattern",
		Embedding:            []float64{0.1, 0.2, 0.3},
		MessagePassEmbedding: []float64{0.15, 0.25, 0.35},
		AggregationCount:     5,
		StabilityScore:       0.95,
	}

	if node.NodeID != "test-hidden-1" {
		t.Errorf("HiddenNode.NodeID = %s, want test-hidden-1", node.NodeID)
	}
	if node.AggregationCount != 5 {
		t.Errorf("HiddenNode.AggregationCount = %d, want 5", node.AggregationCount)
	}
	if node.StabilityScore != 0.95 {
		t.Errorf("HiddenNode.StabilityScore = %f, want 0.95", node.StabilityScore)
	}
}

func TestBaseNodeStruct(t *testing.T) {
	node := BaseNode{
		NodeID:    "base-1",
		SpaceID:   "test-space",
		Embedding: []float64{1.0, 0.0, 0.0},
	}

	if node.NodeID != "base-1" {
		t.Errorf("BaseNode.NodeID = %s, want base-1", node.NodeID)
	}
	if len(node.Embedding) != 3 {
		t.Errorf("BaseNode.Embedding length = %d, want 3", len(node.Embedding))
	}
}

func TestConceptNodeStruct(t *testing.T) {
	node := ConceptNode{
		NodeID:               "concept-1",
		SpaceID:              "test-space",
		Layer:                2,
		Embedding:            []float64{0.5, 0.5, 0.0},
		MessagePassEmbedding: []float64{0.6, 0.4, 0.0},
	}

	if node.Layer != 2 {
		t.Errorf("ConceptNode.Layer = %d, want 2", node.Layer)
	}
}

func TestClusterStruct(t *testing.T) {
	cluster := Cluster{
		Members: []BaseNode{
			{NodeID: "a", SpaceID: "test"},
			{NodeID: "b", SpaceID: "test"},
		},
		Centroid: []float64{0.5, 0.5, 0.0},
	}

	if len(cluster.Members) != 2 {
		t.Errorf("Cluster.Members length = %d, want 2", len(cluster.Members))
	}
}

func TestClusteringResultStruct(t *testing.T) {
	result := ClusteringResult{
		Clusters: []Cluster{
			{Members: []BaseNode{{NodeID: "a"}}, Centroid: []float64{1.0}},
		},
		NoisePoints:  []BaseNode{{NodeID: "b"}},
		TotalPoints:  2,
		ClusterCount: 1,
		NoiseCount:   1,
	}

	if result.ClusterCount != 1 {
		t.Errorf("ClusteringResult.ClusterCount = %d, want 1", result.ClusterCount)
	}
	if result.NoiseCount != 1 {
		t.Errorf("ClusteringResult.NoiseCount = %d, want 1", result.NoiseCount)
	}
}

func TestResultStructs(t *testing.T) {
	// ForwardPassResult
	fwd := ForwardPassResult{
		HiddenNodesUpdated:  10,
		ConceptNodesUpdated: 5,
	}
	if fwd.HiddenNodesUpdated != 10 {
		t.Errorf("ForwardPassResult.HiddenNodesUpdated = %d, want 10", fwd.HiddenNodesUpdated)
	}

	// BackwardPassResult
	bwd := BackwardPassResult{
		HiddenNodesUpdated: 8,
		EdgesStrengthened:  20,
	}
	if bwd.EdgesStrengthened != 20 {
		t.Errorf("BackwardPassResult.EdgesStrengthened = %d, want 20", bwd.EdgesStrengthened)
	}

	// ConsolidationResult
	cons := ConsolidationResult{
		HiddenNodesCreated: 3,
		ForwardPass:        &fwd,
		BackwardPass:       &bwd,
	}
	if cons.HiddenNodesCreated != 3 {
		t.Errorf("ConsolidationResult.HiddenNodesCreated = %d, want 3", cons.HiddenNodesCreated)
	}
}

func TestEdgeStruct(t *testing.T) {
	edge := Edge{
		SourceID: "node-a",
		TargetID: "node-b",
		Type:     "GENERALIZES",
		Weight:   0.85,
	}

	if edge.Type != "GENERALIZES" {
		t.Errorf("Edge.Type = %s, want GENERALIZES", edge.Type)
	}
	if edge.Weight != 0.85 {
		t.Errorf("Edge.Weight = %f, want 0.85", edge.Weight)
	}
}

func TestNewEmergenceNamer_DisabledReturnsNil(t *testing.T) {
	s := &Service{cfg: config.Config{EmergenceEnabled: false}}
	namer := s.newEmergenceNamer()
	if namer != nil {
		t.Error("expected nil namer when emergence is disabled")
	}
}

func TestNewEmergenceNamer_EnabledReturnsNamer(t *testing.T) {
	s := &Service{cfg: config.Config{
		EmergenceEnabled:   true,
		EmergenceProvider:  "ollama",
		EmergenceModel:     "llama3.2",
		EmergenceMaxTokens: 500,
		EmergenceTimeoutMs: 10000,
	}}
	namer := s.newEmergenceNamer()
	if namer == nil {
		t.Fatal("expected non-nil namer when emergence is enabled")
	}
	if !namer.cfg.Enabled {
		t.Error("namer.cfg.Enabled should be true")
	}
	if namer.cfg.Provider != "ollama" {
		t.Errorf("namer.cfg.Provider = %q, want %q", namer.cfg.Provider, "ollama")
	}
	if namer.cfg.Model != "llama3.2" {
		t.Errorf("namer.cfg.Model = %q, want %q", namer.cfg.Model, "llama3.2")
	}
}

func TestBaseNodesToClusterSummaries(t *testing.T) {
	members := []BaseNode{
		{NodeID: "n1", SpaceID: "sp", Path: "Hidden-auth-1"},
		{NodeID: "n2", SpaceID: "sp", Path: "Hidden-config-2"},
	}
	summaries := baseNodesToClusterSummaries(members)
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}
	if summaries[0].NodeID != "n1" {
		t.Errorf("summaries[0].NodeID = %q, want %q", summaries[0].NodeID, "n1")
	}
	if summaries[0].Name != "Hidden-auth-1" {
		t.Errorf("summaries[0].Name = %q, want %q", summaries[0].Name, "Hidden-auth-1")
	}
	if summaries[0].Path != "Hidden-auth-1" {
		t.Errorf("summaries[0].Path = %q, want %q", summaries[0].Path, "Hidden-auth-1")
	}
	if summaries[1].Name != "Hidden-config-2" {
		t.Errorf("summaries[1].Name = %q, want %q", summaries[1].Name, "Hidden-config-2")
	}
}

func TestBaseNodesToClusterSummaries_Empty(t *testing.T) {
	summaries := baseNodesToClusterSummaries(nil)
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries for nil input, got %d", len(summaries))
	}
}

func TestEmergentConceptNodesToClusterSummaries(t *testing.T) {
	members := []EmergentConceptNode{
		{NodeID: "ec1", Name: "Auth Pattern", Summary: "Authentication workflows", Layer: 3},
		{NodeID: "ec2", Name: "Config Pattern", Summary: "Configuration management", Layer: 4},
	}
	summaries := emergentConceptNodesToClusterSummaries(members)
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}
	if summaries[0].NodeID != "ec1" {
		t.Errorf("summaries[0].NodeID = %q, want %q", summaries[0].NodeID, "ec1")
	}
	if summaries[0].Name != "Auth Pattern" {
		t.Errorf("summaries[0].Name = %q, want %q", summaries[0].Name, "Auth Pattern")
	}
	if summaries[0].Summary != "Authentication workflows" {
		t.Errorf("summaries[0].Summary = %q, want %q", summaries[0].Summary, "Authentication workflows")
	}
	if summaries[0].Layer != 3 {
		t.Errorf("summaries[0].Layer = %d, want %d", summaries[0].Layer, 3)
	}
	if summaries[1].Layer != 4 {
		t.Errorf("summaries[1].Layer = %d, want %d", summaries[1].Layer, 4)
	}
}

func TestEmergentConceptNodesToClusterSummaries_Empty(t *testing.T) {
	summaries := emergentConceptNodesToClusterSummaries(nil)
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries for nil input, got %d", len(summaries))
	}
}
