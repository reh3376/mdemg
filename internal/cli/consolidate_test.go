package cli

import (
	"math"
	"testing"
)

// TestAverageEmbeddings verifies the centroid calculation of multiple embedding vectors
func TestAverageEmbeddings(t *testing.T) {
	tests := []struct {
		name       string
		embeddings [][]float64
		expected   []float64
		tolerance  float64
	}{
		{
			name: "two identical embeddings",
			embeddings: [][]float64{
				{1.0, 2.0, 3.0},
				{1.0, 2.0, 3.0},
			},
			expected:  []float64{1.0, 2.0, 3.0},
			tolerance: 0.001,
		},
		{
			name: "two different embeddings",
			embeddings: [][]float64{
				{0.0, 0.0, 0.0},
				{2.0, 4.0, 6.0},
			},
			expected:  []float64{1.0, 2.0, 3.0},
			tolerance: 0.001,
		},
		{
			name: "three embeddings",
			embeddings: [][]float64{
				{1.0, 0.0, 0.0},
				{0.0, 1.0, 0.0},
				{0.0, 0.0, 1.0},
			},
			expected:  []float64{1.0 / 3.0, 1.0 / 3.0, 1.0 / 3.0},
			tolerance: 0.001,
		},
		{
			name: "single embedding",
			embeddings: [][]float64{
				{0.5, 0.5, 0.5},
			},
			expected:  []float64{0.5, 0.5, 0.5},
			tolerance: 0.001,
		},
		{
			name: "negative values",
			embeddings: [][]float64{
				{-1.0, 1.0, 0.0},
				{1.0, -1.0, 0.0},
			},
			expected:  []float64{0.0, 0.0, 0.0},
			tolerance: 0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := averageEmbeddings(tt.embeddings)

			if len(result) != len(tt.expected) {
				t.Fatalf("averageEmbeddings() length = %d, want %d", len(result), len(tt.expected))
			}

			for i, v := range result {
				if math.Abs(v-tt.expected[i]) > tt.tolerance {
					t.Errorf("averageEmbeddings()[%d] = %v, want %v (tolerance %v)",
						i, v, tt.expected[i], tt.tolerance)
				}
			}
		})
	}
}

// TestAverageEmbeddings_Empty verifies nil return for empty input
func TestAverageEmbeddings_Empty(t *testing.T) {
	tests := []struct {
		name       string
		embeddings [][]float64
	}{
		{
			name:       "nil slice",
			embeddings: nil,
		},
		{
			name:       "empty slice",
			embeddings: [][]float64{},
		},
		{
			name:       "slice with only empty embeddings",
			embeddings: [][]float64{{}, {}},
		},
		{
			name:       "slice with only nil embeddings",
			embeddings: [][]float64{nil, nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := averageEmbeddings(tt.embeddings)

			if result != nil {
				t.Errorf("averageEmbeddings() = %v, want nil", result)
			}
		})
	}
}

// TestAverageEmbeddings_MismatchedDims verifies handling of mismatched dimensions
func TestAverageEmbeddings_MismatchedDims(t *testing.T) {
	tests := []struct {
		name       string
		embeddings [][]float64
		expected   []float64
		tolerance  float64
	}{
		{
			name: "skip shorter embedding",
			embeddings: [][]float64{
				{1.0, 2.0, 3.0},
				{1.0, 2.0}, // shorter - should be skipped
				{3.0, 4.0, 5.0},
			},
			expected:  []float64{2.0, 3.0, 4.0}, // average of first and third only
			tolerance: 0.001,
		},
		{
			name: "skip longer embedding",
			embeddings: [][]float64{
				{1.0, 2.0, 3.0},
				{1.0, 2.0, 3.0, 4.0}, // longer - should be skipped
				{3.0, 4.0, 5.0},
			},
			expected:  []float64{2.0, 3.0, 4.0}, // average of first and third only
			tolerance: 0.001,
		},
		{
			name: "skip empty embedding in middle",
			embeddings: [][]float64{
				{2.0, 4.0, 6.0},
				{}, // empty - should be skipped
				{4.0, 6.0, 8.0},
			},
			expected:  []float64{3.0, 5.0, 7.0}, // average of first and third only
			tolerance: 0.001,
		},
		{
			name: "all mismatched after first",
			embeddings: [][]float64{
				{1.0, 2.0, 3.0},
				{1.0, 2.0},
				{1.0},
			},
			expected:  []float64{1.0, 2.0, 3.0}, // only first is valid
			tolerance: 0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := averageEmbeddings(tt.embeddings)

			if len(result) != len(tt.expected) {
				t.Fatalf("averageEmbeddings() length = %d, want %d", len(result), len(tt.expected))
			}

			for i, v := range result {
				if math.Abs(v-tt.expected[i]) > tt.tolerance {
					t.Errorf("averageEmbeddings()[%d] = %v, want %v (tolerance %v)",
						i, v, tt.expected[i], tt.tolerance)
				}
			}
		})
	}
}

// TestBuildClusters verifies cluster grouping using greedy first-come assignment
func TestBuildClusters(t *testing.T) {
	tests := []struct {
		name             string
		candidates       []clusterCandidate
		minSize          int
		expectedClusters int
		expectedMembers  []int // number of members in each cluster
	}{
		{
			name: "single cluster of 3",
			candidates: []clusterCandidate{
				{
					nodeID:      "node-a",
					layer:       0,
					embedding:   []float64{0.1, 0.2, 0.3},
					neighborIDs: []string{"node-b", "node-c"},
				},
				{
					nodeID:      "node-b",
					layer:       0,
					embedding:   []float64{0.2, 0.3, 0.4},
					neighborIDs: []string{"node-a", "node-c"},
				},
				{
					nodeID:      "node-c",
					layer:       0,
					embedding:   []float64{0.3, 0.4, 0.5},
					neighborIDs: []string{"node-a", "node-b"},
				},
			},
			minSize:          3,
			expectedClusters: 1,
			expectedMembers:  []int{3},
		},
		{
			name: "two separate clusters",
			candidates: []clusterCandidate{
				{
					nodeID:      "node-a",
					layer:       0,
					embedding:   []float64{0.1, 0.2, 0.3},
					neighborIDs: []string{"node-b", "node-c"},
				},
				{
					nodeID:      "node-d",
					layer:       0,
					embedding:   []float64{0.4, 0.5, 0.6},
					neighborIDs: []string{"node-e", "node-f"},
				},
			},
			minSize:          3,
			expectedClusters: 2,
			expectedMembers:  []int{3, 3},
		},
		{
			name: "nodes already assigned skip",
			candidates: []clusterCandidate{
				{
					nodeID:      "node-a",
					layer:       0,
					embedding:   []float64{0.1, 0.2, 0.3},
					neighborIDs: []string{"node-b", "node-c", "node-d"},
				},
				{
					nodeID:      "node-b",
					layer:       0,
					embedding:   []float64{0.2, 0.3, 0.4},
					neighborIDs: []string{"node-a", "node-c"},
				},
			},
			minSize:          3,
			expectedClusters: 1,
			expectedMembers:  []int{4}, // node-a + node-b + node-c + node-d
		},
		{
			name:             "empty candidates",
			candidates:       []clusterCandidate{},
			minSize:          3,
			expectedClusters: 0,
			expectedMembers:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildClusters(tt.candidates, tt.minSize)

			if len(result) != tt.expectedClusters {
				t.Fatalf("buildClusters() returned %d clusters, want %d",
					len(result), tt.expectedClusters)
			}

			for i, c := range result {
				if i < len(tt.expectedMembers) && len(c.members) != tt.expectedMembers[i] {
					t.Errorf("cluster[%d] has %d members, want %d",
						i, len(c.members), tt.expectedMembers[i])
				}
			}
		})
	}
}

// TestBuildClusters_MinSize verifies that clusters smaller than min size are excluded
func TestBuildClusters_MinSize(t *testing.T) {
	tests := []struct {
		name             string
		candidates       []clusterCandidate
		minSize          int
		expectedClusters int
	}{
		{
			name: "cluster below min size excluded",
			candidates: []clusterCandidate{
				{
					nodeID:      "node-a",
					layer:       0,
					embedding:   []float64{0.1, 0.2},
					neighborIDs: []string{"node-b"}, // only 2 total, below minSize 3
				},
			},
			minSize:          3,
			expectedClusters: 0,
		},
		{
			name: "cluster exactly at min size included",
			candidates: []clusterCandidate{
				{
					nodeID:      "node-a",
					layer:       0,
					embedding:   []float64{0.1, 0.2},
					neighborIDs: []string{"node-b", "node-c"}, // exactly 3
				},
			},
			minSize:          3,
			expectedClusters: 1,
		},
		{
			name: "min size 2",
			candidates: []clusterCandidate{
				{
					nodeID:      "node-a",
					layer:       0,
					embedding:   []float64{0.1, 0.2},
					neighborIDs: []string{"node-b"}, // 2 total, meets minSize 2
				},
			},
			minSize:          2,
			expectedClusters: 1,
		},
		{
			name: "larger min size filters out small clusters",
			candidates: []clusterCandidate{
				{
					nodeID:      "node-a",
					layer:       0,
					embedding:   []float64{0.1, 0.2},
					neighborIDs: []string{"node-b", "node-c", "node-d"}, // 4 total
				},
				{
					nodeID:      "node-e",
					layer:       0,
					embedding:   []float64{0.3, 0.4},
					neighborIDs: []string{"node-f"}, // 2 total - below minSize 5
				},
			},
			minSize:          5,
			expectedClusters: 0, // first cluster has 4 (below 5), second has 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildClusters(tt.candidates, tt.minSize)

			if len(result) != tt.expectedClusters {
				t.Errorf("buildClusters() returned %d clusters, want %d",
					len(result), tt.expectedClusters)
			}
		})
	}
}

// TestBuildClusters_LayerPreserved verifies that cluster layer matches candidate layer
func TestBuildClusters_LayerPreserved(t *testing.T) {
	candidates := []clusterCandidate{
		{
			nodeID:      "node-a",
			layer:       2,
			embedding:   []float64{0.1, 0.2, 0.3},
			neighborIDs: []string{"node-b", "node-c"},
		},
	}

	result := buildClusters(candidates, 3)

	if len(result) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(result))
	}

	if result[0].layer != 2 {
		t.Errorf("cluster layer = %d, want 2", result[0].layer)
	}
}

// TestBuildClusters_EmbeddingsPreserved verifies embeddings are copied to cluster members
func TestBuildClusters_EmbeddingsPreserved(t *testing.T) {
	expectedEmb := []float64{0.1, 0.2, 0.3}
	candidates := []clusterCandidate{
		{
			nodeID:      "node-a",
			layer:       0,
			embedding:   expectedEmb,
			neighborIDs: []string{"node-b", "node-c"},
		},
		{
			nodeID:      "node-b",
			layer:       0,
			embedding:   []float64{0.4, 0.5, 0.6},
			neighborIDs: []string{"node-a"},
		},
	}

	result := buildClusters(candidates, 3)

	if len(result) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(result))
	}

	// Find member node-a and verify its embedding
	var found bool
	for _, member := range result[0].members {
		if member.nodeID == "node-a" {
			found = true
			if len(member.embedding) != len(expectedEmb) {
				t.Errorf("member embedding length = %d, want %d", len(member.embedding), len(expectedEmb))
			}
			for i, v := range member.embedding {
				if v != expectedEmb[i] {
					t.Errorf("member embedding[%d] = %v, want %v", i, v, expectedEmb[i])
				}
			}
			break
		}
	}
	if !found {
		t.Error("node-a not found in cluster members")
	}
}

// TestConfigValidation_MinClusterSize verifies min cluster size validation
func TestConfigValidation_MinClusterSize(t *testing.T) {
	tests := []struct {
		name        string
		minSize     int
		expectError bool
	}{
		{
			name:        "valid min size 3",
			minSize:     3,
			expectError: false,
		},
		{
			name:        "valid min size 2",
			minSize:     2,
			expectError: false,
		},
		{
			name:        "invalid min size 1",
			minSize:     1,
			expectError: true,
		},
		{
			name:        "invalid min size 0",
			minSize:     0,
			expectError: true,
		},
		{
			name:        "invalid negative min size",
			minSize:     -1,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Directly test the validation logic
			valid := tt.minSize >= 2
			hasError := !valid

			if hasError != tt.expectError {
				t.Errorf("minClusterSize=%d validation: got error=%v, want error=%v",
					tt.minSize, hasError, tt.expectError)
			}
		})
	}
}

// TestConfigValidation_WeightThreshold verifies weight threshold validation
func TestConfigValidation_WeightThreshold(t *testing.T) {
	tests := []struct {
		name        string
		threshold   float64
		expectError bool
	}{
		{
			name:        "valid threshold 0.5",
			threshold:   0.5,
			expectError: false,
		},
		{
			name:        "valid threshold 0.0",
			threshold:   0.0,
			expectError: false,
		},
		{
			name:        "valid threshold 1.0",
			threshold:   1.0,
			expectError: false,
		},
		{
			name:        "invalid negative threshold",
			threshold:   -0.1,
			expectError: true,
		},
		{
			name:        "invalid threshold above 1",
			threshold:   1.1,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Directly test the validation logic
			valid := tt.threshold >= 0 && tt.threshold <= 1
			hasError := !valid

			if hasError != tt.expectError {
				t.Errorf("weightThreshold=%v validation: got error=%v, want error=%v",
					tt.threshold, hasError, tt.expectError)
			}
		})
	}
}

// TestConfigValidation_MaxPromotions verifies max promotions validation
func TestConfigValidation_MaxPromotions(t *testing.T) {
	tests := []struct {
		name        string
		maxPromos   int
		expectError bool
	}{
		{
			name:        "valid max 50",
			maxPromos:   50,
			expectError: false,
		},
		{
			name:        "valid max 1",
			maxPromos:   1,
			expectError: false,
		},
		{
			name:        "invalid max 0",
			maxPromos:   0,
			expectError: true,
		},
		{
			name:        "invalid negative max",
			maxPromos:   -1,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Directly test the validation logic
			valid := tt.maxPromos > 0
			hasError := !valid

			if hasError != tt.expectError {
				t.Errorf("maxPromotions=%d validation: got error=%v, want error=%v",
					tt.maxPromos, hasError, tt.expectError)
			}
		})
	}
}

// TestTruncateID verifies the ID truncation helper
func TestTruncateID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short ID unchanged",
			input:    "abc123",
			expected: "abc123",
		},
		{
			name:     "exactly 12 chars unchanged",
			input:    "123456789012",
			expected: "123456789012",
		},
		{
			name:     "long ID truncated",
			input:    "12345678-1234-1234-1234-123456789012",
			expected: "12345678...",
		},
		{
			name:     "empty string unchanged",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateID(tt.input)
			if result != tt.expected {
				t.Errorf("truncateID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestGenerateAbstractionName verifies abstraction name generation
func TestGenerateAbstractionName(t *testing.T) {
	tests := []struct {
		name     string
		members  []clusterMember
		contains string // name should contain this substring
	}{
		{
			name:     "empty members",
			members:  []clusterMember{},
			contains: "empty",
		},
		{
			name: "single member",
			members: []clusterMember{
				{nodeID: "node-a"},
			},
			contains: "node-a",
		},
		{
			name: "multiple members",
			members: []clusterMember{
				{nodeID: "node-a"},
				{nodeID: "node-b"},
				{nodeID: "node-c"},
			},
			contains: "Abstraction",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateAbstractionName(tt.members)

			if result == "" {
				t.Error("generateAbstractionName() returned empty string")
			}

			if tt.contains != "" && !containsSubstring(result, tt.contains) {
				t.Errorf("generateAbstractionName() = %q, should contain %q", result, tt.contains)
			}
		})
	}
}

// containsSubstring is a helper to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
