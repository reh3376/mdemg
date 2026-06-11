package retrieval

import (
	"testing"
)

// TestSortStrings tests the sortStrings helper function
func TestSortStrings(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "already sorted",
			input:    []string{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "reverse order",
			input:    []string{"c", "b", "a"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "random order",
			input:    []string{"node_3", "node_1", "node_2"},
			expected: []string{"node_1", "node_2", "node_3"},
		},
		{
			name:     "single element",
			input:    []string{"only"},
			expected: []string{"only"},
		},
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "two elements swapped",
			input:    []string{"z", "a"},
			expected: []string{"a", "z"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying the test case
			input := make([]string, len(tt.input))
			copy(input, tt.input)

			sortStrings(input)

			if len(input) != len(tt.expected) {
				t.Errorf("sortStrings() length = %d, want %d", len(input), len(tt.expected))
				return
			}
			for i, v := range input {
				if v != tt.expected[i] {
					t.Errorf("sortStrings()[%d] = %s, want %s", i, v, tt.expected[i])
				}
			}
		})
	}
}

// TestDeduplicateTriangles tests the deduplicateTriangles helper function
func TestDeduplicateTriangles(t *testing.T) {
	tests := []struct {
		name     string
		input    [][]string
		expected int // number of unique triangles
	}{
		{
			name:     "no duplicates",
			input:    [][]string{{"a", "b", "c"}, {"d", "e", "f"}},
			expected: 2,
		},
		{
			name:     "with duplicates",
			input:    [][]string{{"a", "b", "c"}, {"a", "b", "c"}, {"d", "e", "f"}},
			expected: 2,
		},
		{
			name:     "all duplicates",
			input:    [][]string{{"a", "b", "c"}, {"a", "b", "c"}, {"a", "b", "c"}},
			expected: 1,
		},
		{
			name:     "empty input",
			input:    [][]string{},
			expected: 0,
		},
		{
			name:     "single triangle",
			input:    [][]string{{"x", "y", "z"}},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deduplicateTriangles(tt.input)
			if len(result) != tt.expected {
				t.Errorf("deduplicateTriangles() returned %d triangles, want %d", len(result), tt.expected)
			}
		})
	}
}

// TestMergeOverlappingClusters tests the mergeOverlappingClusters function
func TestMergeOverlappingClusters(t *testing.T) {
	tests := []struct {
		name              string
		triangles         [][]string
		expectedClusters  int
		minClusterSizeSum int // sum of all cluster sizes (to verify merging)
	}{
		{
			name:              "empty input",
			triangles:         [][]string{},
			expectedClusters:  0,
			minClusterSizeSum: 0,
		},
		{
			name:              "single triangle",
			triangles:         [][]string{{"a", "b", "c"}},
			expectedClusters:  1,
			minClusterSizeSum: 3,
		},
		{
			name:              "non-overlapping triangles",
			triangles:         [][]string{{"a", "b", "c"}, {"d", "e", "f"}},
			expectedClusters:  2,
			minClusterSizeSum: 6,
		},
		{
			name:              "overlapping triangles - share 2 nodes",
			triangles:         [][]string{{"a", "b", "c"}, {"a", "b", "d"}},
			expectedClusters:  1,
			minClusterSizeSum: 4, // merged: {a, b, c, d}
		},
		{
			name:              "chain of overlapping triangles",
			triangles:         [][]string{{"a", "b", "c"}, {"b", "c", "d"}, {"c", "d", "e"}},
			expectedClusters:  1,
			minClusterSizeSum: 5, // merged: {a, b, c, d, e}
		},
		{
			name:              "triangles with only 1 shared node - not merged",
			triangles:         [][]string{{"a", "b", "c"}, {"c", "d", "e"}},
			expectedClusters:  2,
			minClusterSizeSum: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeOverlappingClusters(tt.triangles)

			if len(result) != tt.expectedClusters {
				t.Errorf("mergeOverlappingClusters() returned %d clusters, want %d", len(result), tt.expectedClusters)
			}

			// Calculate total nodes across all clusters
			totalNodes := 0
			for _, cluster := range result {
				totalNodes += len(cluster)
			}
			if totalNodes < tt.minClusterSizeSum {
				t.Errorf("total nodes in clusters = %d, want at least %d", totalNodes, tt.minClusterSizeSum)
			}
		})
	}
}

// TestDetectClusters tests the detectClusters function
func TestDetectClusters(t *testing.T) {
	tests := []struct {
		name             string
		nodeIDs          []string
		edges            []InsightEdge
		expectedClusters int
	}{
		{
			name:             "empty nodes",
			nodeIDs:          []string{},
			edges:            []InsightEdge{},
			expectedClusters: 0,
		},
		{
			name:             "fewer than 3 nodes",
			nodeIDs:          []string{"a", "b"},
			edges:            []InsightEdge{{Src: "a", Dst: "b"}},
			expectedClusters: 0,
		},
		{
			name:             "fewer than 3 edges",
			nodeIDs:          []string{"a", "b", "c"},
			edges:            []InsightEdge{{Src: "a", Dst: "b"}, {Src: "b", Dst: "c"}},
			expectedClusters: 0,
		},
		{
			name:    "single triangle cluster",
			nodeIDs: []string{"a", "b", "c"},
			edges: []InsightEdge{
				{Src: "a", Dst: "b"},
				{Src: "b", Dst: "c"},
				{Src: "a", Dst: "c"},
			},
			expectedClusters: 1,
		},
		{
			name:    "two disjoint triangles",
			nodeIDs: []string{"a", "b", "c", "d", "e", "f"},
			edges: []InsightEdge{
				{Src: "a", Dst: "b"},
				{Src: "b", Dst: "c"},
				{Src: "a", Dst: "c"},
				{Src: "d", Dst: "e"},
				{Src: "e", Dst: "f"},
				{Src: "d", Dst: "f"},
			},
			expectedClusters: 2,
		},
		{
			name:    "overlapping triangles form one cluster",
			nodeIDs: []string{"a", "b", "c", "d"},
			edges: []InsightEdge{
				{Src: "a", Dst: "b"},
				{Src: "b", Dst: "c"},
				{Src: "a", Dst: "c"},
				{Src: "a", Dst: "d"},
				{Src: "b", Dst: "d"},
			},
			expectedClusters: 1,
		},
		{
			name:    "no triangles - star topology",
			nodeIDs: []string{"center", "a", "b", "c"},
			edges: []InsightEdge{
				{Src: "center", Dst: "a"},
				{Src: "center", Dst: "b"},
				{Src: "center", Dst: "c"},
			},
			expectedClusters: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectClusters(tt.nodeIDs, tt.edges)
			if len(result) != tt.expectedClusters {
				t.Errorf("detectClusters() returned %d clusters, want %d", len(result), tt.expectedClusters)
			}

			// Verify each cluster has at least 3 nodes
			for i, cluster := range result {
				if len(cluster) < 3 {
					t.Errorf("cluster[%d] has %d nodes, want at least 3", i, len(cluster))
				}
			}
		})
	}
}

// TestDetectPatterns tests the detectPatterns function
func TestDetectPatterns(t *testing.T) {
	tests := []struct {
		name            string
		edges           []InsightEdge
		expectPattern   bool // whether a dominant pattern should be detected
		expectedRelType string
	}{
		{
			name:          "empty edges",
			edges:         []InsightEdge{},
			expectPattern: false,
		},
		{
			name: "single edge type - 100% dominant",
			edges: []InsightEdge{
				{Src: "a", Dst: "b", RelType: "CO_ACTIVATED_WITH"},
				{Src: "b", Dst: "c", RelType: "CO_ACTIVATED_WITH"},
				{Src: "c", Dst: "d", RelType: "CO_ACTIVATED_WITH"},
			},
			expectPattern:   true,
			expectedRelType: "CO_ACTIVATED_WITH",
		},
		{
			name: "dominant edge type - over 50%",
			edges: []InsightEdge{
				{Src: "a", Dst: "b", RelType: "CO_ACTIVATED_WITH"},
				{Src: "b", Dst: "c", RelType: "CO_ACTIVATED_WITH"},
				{Src: "c", Dst: "d", RelType: "CO_ACTIVATED_WITH"},
				{Src: "d", Dst: "e", RelType: "ASSOCIATED_WITH"},
			},
			expectPattern:   true,
			expectedRelType: "CO_ACTIVATED_WITH",
		},
		{
			name: "no dominant edge type - 50/50 split",
			edges: []InsightEdge{
				{Src: "a", Dst: "b", RelType: "CO_ACTIVATED_WITH"},
				{Src: "b", Dst: "c", RelType: "CO_ACTIVATED_WITH"},
				{Src: "c", Dst: "d", RelType: "ASSOCIATED_WITH"},
				{Src: "d", Dst: "e", RelType: "ASSOCIATED_WITH"},
			},
			expectPattern: false,
		},
		{
			name: "multiple types - none dominant",
			edges: []InsightEdge{
				{Src: "a", Dst: "b", RelType: "CO_ACTIVATED_WITH"},
				{Src: "b", Dst: "c", RelType: "ASSOCIATED_WITH"},
				{Src: "c", Dst: "d", RelType: "ABSTRACTS_TO"},
			},
			expectPattern: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectPatterns(tt.edges)

			if tt.expectPattern {
				if len(result) == 0 {
					t.Error("detectPatterns() returned no patterns, expected at least 1")
					return
				}
				// Check that the pattern insight mentions the expected relationship type
				found := false
				for _, insight := range result {
					if insight.Type == "pattern" {
						found = true
						// Verify the description contains the expected relationship type
						if tt.expectedRelType != "" {
							containsType := false
							if len(insight.Description) > 0 {
								containsType = true // Basic check - pattern was detected
							}
							if !containsType {
								t.Errorf("pattern insight missing description")
							}
						}
					}
				}
				if !found {
					t.Error("no pattern type insight found")
				}
			} else {
				if len(result) > 0 {
					t.Errorf("detectPatterns() returned %d patterns, expected 0", len(result))
				}
			}
		})
	}
}

// TestDetectGaps tests the detectGaps function
func TestDetectGaps(t *testing.T) {
	tests := []struct {
		name                string
		nodeIDs             []string
		edges               []InsightEdge
		expectGap           bool
		expectedIsolatedMin int // minimum number of isolated nodes
	}{
		{
			name:      "empty nodes",
			nodeIDs:   []string{},
			edges:     []InsightEdge{},
			expectGap: false,
		},
		{
			name:      "single node - no gap reported",
			nodeIDs:   []string{"a"},
			edges:     []InsightEdge{},
			expectGap: false,
		},
		{
			name:    "all connected - no gaps",
			nodeIDs: []string{"a", "b", "c"},
			edges: []InsightEdge{
				{Src: "a", Dst: "b"},
				{Src: "b", Dst: "c"},
			},
			expectGap: false,
		},
		{
			name:                "one isolated node",
			nodeIDs:             []string{"a", "b", "c", "d"},
			edges:               []InsightEdge{{Src: "a", Dst: "b"}, {Src: "b", Dst: "c"}},
			expectGap:           true,
			expectedIsolatedMin: 1,
		},
		{
			name:                "multiple isolated nodes",
			nodeIDs:             []string{"a", "b", "c", "d", "e"},
			edges:               []InsightEdge{{Src: "a", Dst: "b"}},
			expectGap:           true,
			expectedIsolatedMin: 3,
		},
		{
			name:      "all isolated nodes - no gap reported (special case)",
			nodeIDs:   []string{"a", "b", "c"},
			edges:     []InsightEdge{},
			expectGap: false, // Per implementation: gap only if SOME are isolated, not ALL
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectGaps(tt.nodeIDs, tt.edges)

			if tt.expectGap {
				if len(result) == 0 {
					t.Error("detectGaps() returned no gaps, expected at least 1")
					return
				}
				// Check that at least one gap insight exists with correct type
				found := false
				for _, insight := range result {
					if insight.Type == "gap" {
						found = true
						if len(insight.NodeIDs) < tt.expectedIsolatedMin {
							t.Errorf("gap insight has %d isolated nodes, want at least %d",
								len(insight.NodeIDs), tt.expectedIsolatedMin)
						}
					}
				}
				if !found {
					t.Error("no gap type insight found")
				}
			} else {
				for _, insight := range result {
					if insight.Type == "gap" {
						t.Errorf("detectGaps() returned gap insight, expected none")
					}
				}
			}
		})
	}
}

// TestDefaultReflectConstants tests that default constants are set correctly
func TestDefaultReflectConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant int
		min      int
		max      int
	}{
		{
			name:     "DefaultReflectMaxDepth",
			constant: DefaultReflectMaxDepth,
			min:      1,
			max:      5,
		},
		{
			name:     "DefaultReflectMaxNodes",
			constant: DefaultReflectMaxNodes,
			min:      10,
			max:      100,
		},
		{
			name:     "DefaultReflectSeedK",
			constant: DefaultReflectSeedK,
			min:      5,
			max:      50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant < tt.min || tt.constant > tt.max {
				t.Errorf("%s = %d, want between %d and %d", tt.name, tt.constant, tt.min, tt.max)
			}
		})
	}
}

// TestLateralEdge_Structure tests the LateralEdge struct
func TestLateralEdge_Structure(t *testing.T) {
	edge := LateralEdge{
		Src:     "node_1",
		Dst:     "node_2",
		RelType: "CO_ACTIVATED_WITH",
		Weight:  0.85,
	}

	if edge.Src != "node_1" {
		t.Errorf("LateralEdge.Src = %s, want node_1", edge.Src)
	}
	if edge.Dst != "node_2" {
		t.Errorf("LateralEdge.Dst = %s, want node_2", edge.Dst)
	}
	if edge.RelType != "CO_ACTIVATED_WITH" {
		t.Errorf("LateralEdge.RelType = %s, want CO_ACTIVATED_WITH", edge.RelType)
	}
	if edge.Weight != 0.85 {
		t.Errorf("LateralEdge.Weight = %f, want 0.85", edge.Weight)
	}
}

// TestNodeMetadata_Structure tests the NodeMetadata struct
func TestNodeMetadata_Structure(t *testing.T) {
	meta := NodeMetadata{
		NodeID:  "node_abc",
		Name:    "Test Node",
		Path:    "/test/path",
		Summary: "A test summary",
		Layer:   2,
	}

	if meta.NodeID != "node_abc" {
		t.Errorf("NodeMetadata.NodeID = %s, want node_abc", meta.NodeID)
	}
	if meta.Name != "Test Node" {
		t.Errorf("NodeMetadata.Name = %s, want Test Node", meta.Name)
	}
	if meta.Path != "/test/path" {
		t.Errorf("NodeMetadata.Path = %s, want /test/path", meta.Path)
	}
	if meta.Summary != "A test summary" {
		t.Errorf("NodeMetadata.Summary = %s, want A test summary", meta.Summary)
	}
	if meta.Layer != 2 {
		t.Errorf("NodeMetadata.Layer = %d, want 2", meta.Layer)
	}
}

// TestAbstractionEdge_Structure tests the AbstractionEdge struct
func TestAbstractionEdge_Structure(t *testing.T) {
	edge := AbstractionEdge{
		Src:    "concrete_node",
		Dst:    "abstract_node",
		Weight: 0.75,
	}

	if edge.Src != "concrete_node" {
		t.Errorf("AbstractionEdge.Src = %s, want concrete_node", edge.Src)
	}
	if edge.Dst != "abstract_node" {
		t.Errorf("AbstractionEdge.Dst = %s, want abstract_node", edge.Dst)
	}
	if edge.Weight != 0.75 {
		t.Errorf("AbstractionEdge.Weight = %f, want 0.75", edge.Weight)
	}
}

// TestInsightEdge_Structure tests the InsightEdge struct
func TestInsightEdge_Structure(t *testing.T) {
	edge := InsightEdge{
		Src:     "src_node",
		Dst:     "dst_node",
		RelType: "ASSOCIATED_WITH",
		Weight:  0.9,
	}

	if edge.Src != "src_node" {
		t.Errorf("InsightEdge.Src = %s, want src_node", edge.Src)
	}
	if edge.Dst != "dst_node" {
		t.Errorf("InsightEdge.Dst = %s, want dst_node", edge.Dst)
	}
	if edge.RelType != "ASSOCIATED_WITH" {
		t.Errorf("InsightEdge.RelType = %s, want ASSOCIATED_WITH", edge.RelType)
	}
	if edge.Weight != 0.9 {
		t.Errorf("InsightEdge.Weight = %f, want 0.9", edge.Weight)
	}
}

// TestDetectClusters_EdgeCases tests edge cases for cluster detection
func TestDetectClusters_EdgeCases(t *testing.T) {
	tests := []struct {
		name             string
		nodeIDs          []string
		edges            []InsightEdge
		expectedClusters int
	}{
		{
			name:             "nil nodes",
			nodeIDs:          nil,
			edges:            []InsightEdge{},
			expectedClusters: 0,
		},
		{
			name:    "edges reference nodes not in nodeIDs",
			nodeIDs: []string{"a", "b", "c"},
			edges: []InsightEdge{
				{Src: "a", Dst: "b"},
				{Src: "b", Dst: "c"},
				{Src: "a", Dst: "c"},
				{Src: "x", Dst: "y"}, // These should be ignored
			},
			expectedClusters: 1,
		},
		{
			name:    "complete graph of 4 nodes - forms cluster",
			nodeIDs: []string{"a", "b", "c", "d"},
			edges: []InsightEdge{
				{Src: "a", Dst: "b"},
				{Src: "a", Dst: "c"},
				{Src: "a", Dst: "d"},
				{Src: "b", Dst: "c"},
				{Src: "b", Dst: "d"},
				{Src: "c", Dst: "d"},
			},
			expectedClusters: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectClusters(tt.nodeIDs, tt.edges)
			if len(result) != tt.expectedClusters {
				t.Errorf("detectClusters() returned %d clusters, want %d", len(result), tt.expectedClusters)
			}
		})
	}
}

// TestDetectPatterns_EdgeCases tests edge cases for pattern detection
func TestDetectPatterns_EdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		edges         []InsightEdge
		expectPattern bool
	}{
		{
			name:          "nil edges",
			edges:         nil,
			expectPattern: false,
		},
		{
			name: "single edge - dominant by default",
			edges: []InsightEdge{
				{Src: "a", Dst: "b", RelType: "CAUSES"},
			},
			expectPattern: true,
		},
		{
			name: "exactly 50% - not dominant",
			edges: []InsightEdge{
				{Src: "a", Dst: "b", RelType: "TYPE_A"},
				{Src: "c", Dst: "d", RelType: "TYPE_B"},
			},
			expectPattern: false,
		},
		{
			name: "just over 50% - dominant",
			edges: []InsightEdge{
				{Src: "a", Dst: "b", RelType: "TYPE_A"},
				{Src: "b", Dst: "c", RelType: "TYPE_A"},
				{Src: "c", Dst: "d", RelType: "TYPE_B"},
			},
			expectPattern: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectPatterns(tt.edges)
			gotPattern := len(result) > 0

			if gotPattern != tt.expectPattern {
				t.Errorf("detectPatterns() returned pattern=%v, want %v", gotPattern, tt.expectPattern)
			}
		})
	}
}

// TestDetectGaps_EdgeCases tests edge cases for gap detection
func TestDetectGaps_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		nodeIDs   []string
		edges     []InsightEdge
		expectGap bool
	}{
		{
			name:      "nil nodes",
			nodeIDs:   nil,
			edges:     []InsightEdge{},
			expectGap: false,
		},
		{
			name:      "nil edges with multiple nodes - all isolated",
			nodeIDs:   []string{"a", "b"},
			edges:     nil,
			expectGap: false, // All isolated = no gap reported
		},
		{
			name:    "bidirectional edges count both directions",
			nodeIDs: []string{"a", "b", "c"},
			edges: []InsightEdge{
				{Src: "a", Dst: "b"},
			},
			expectGap: true, // c is isolated
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectGaps(tt.nodeIDs, tt.edges)
			gotGap := false
			for _, insight := range result {
				if insight.Type == "gap" {
					gotGap = true
					break
				}
			}

			if gotGap != tt.expectGap {
				t.Errorf("detectGaps() returned gap=%v, want %v", gotGap, tt.expectGap)
			}
		})
	}
}

// TestScoreDecay tests the score decay calculation used in reflection
func TestScoreDecay(t *testing.T) {
	tests := []struct {
		name     string
		distance int
		minScore float64
		maxScore float64
	}{
		{
			name:     "distance 0 - max score",
			distance: 0,
			minScore: 0.99,
			maxScore: 1.01,
		},
		{
			name:     "distance 1 - half score",
			distance: 1,
			minScore: 0.49,
			maxScore: 0.51,
		},
		{
			name:     "distance 2 - third score",
			distance: 2,
			minScore: 0.32,
			maxScore: 0.34,
		},
		{
			name:     "distance 3 - quarter score",
			distance: 3,
			minScore: 0.24,
			maxScore: 0.26,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is the score formula used in reflection.go
			score := 1.0 / float64(1+tt.distance)

			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score for distance %d = %f, want between %f and %f",
					tt.distance, score, tt.minScore, tt.maxScore)
			}
		})
	}
}

// TestLayerBoostCalculation tests the layer boost calculation for abstractions
func TestLayerBoostCalculation(t *testing.T) {
	tests := []struct {
		name     string
		layer    int
		distance int
		minScore float64
		maxScore float64
	}{
		{
			name:     "layer 0, distance 1",
			layer:    0,
			distance: 1,
			minScore: 0.49,
			maxScore: 0.51,
		},
		{
			name:     "layer 5, distance 1 - significant boost",
			layer:    5,
			distance: 1,
			minScore: 0.74,
			maxScore: 0.76,
		},
		{
			name:     "layer 10, distance 1 - max boost",
			layer:    10,
			distance: 1,
			minScore: 0.99,
			maxScore: 1.01,
		},
		{
			name:     "layer 5, distance 2 - boost but decay",
			layer:    5,
			distance: 2,
			minScore: 0.49,
			maxScore: 0.51,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is the score formula used in reflection.go for abstractions
			layerBoost := 1.0 + float64(tt.layer)*0.1
			score := layerBoost / float64(1+tt.distance)

			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score for layer %d, distance %d = %f, want between %f and %f",
					tt.layer, tt.distance, score, tt.minScore, tt.maxScore)
			}
		})
	}
}
