package hidden

import (
	"math"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float64
		b        []float64
		expected float64
	}{
		{
			name:     "identical vectors",
			a:        []float64{1.0, 0.0, 0.0},
			b:        []float64{1.0, 0.0, 0.0},
			expected: 1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float64{1.0, 0.0, 0.0},
			b:        []float64{0.0, 1.0, 0.0},
			expected: 0.0,
		},
		{
			name:     "opposite vectors",
			a:        []float64{1.0, 0.0, 0.0},
			b:        []float64{-1.0, 0.0, 0.0},
			expected: -1.0,
		},
		{
			name:     "45 degree angle",
			a:        []float64{1.0, 0.0},
			b:        []float64{1.0, 1.0},
			expected: 1.0 / math.Sqrt(2),
		},
		{
			name:     "empty vectors",
			a:        []float64{},
			b:        []float64{},
			expected: 0.0,
		},
		{
			name:     "mismatched dimensions",
			a:        []float64{1.0, 0.0},
			b:        []float64{1.0, 0.0, 0.0},
			expected: 0.0,
		},
		{
			name:     "zero vector a",
			a:        []float64{0.0, 0.0, 0.0},
			b:        []float64{1.0, 0.0, 0.0},
			expected: 0.0,
		},
		{
			name:     "zero vector b",
			a:        []float64{1.0, 0.0, 0.0},
			b:        []float64{0.0, 0.0, 0.0},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineSimilarity(tt.a, tt.b)
			if math.Abs(result-tt.expected) > 1e-9 {
				t.Errorf("cosineSimilarity(%v, %v) = %v, want %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestCosineDistance(t *testing.T) {
	tests := []struct {
		name     string
		a        []float64
		b        []float64
		expected float64
	}{
		{
			name:     "identical vectors - distance 0",
			a:        []float64{1.0, 0.0, 0.0},
			b:        []float64{1.0, 0.0, 0.0},
			expected: 0.0,
		},
		{
			name:     "orthogonal vectors - distance 1",
			a:        []float64{1.0, 0.0, 0.0},
			b:        []float64{0.0, 1.0, 0.0},
			expected: 1.0,
		},
		{
			name:     "opposite vectors - distance 2",
			a:        []float64{1.0, 0.0, 0.0},
			b:        []float64{-1.0, 0.0, 0.0},
			expected: 2.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineDistance(tt.a, tt.b)
			if math.Abs(result-tt.expected) > 1e-9 {
				t.Errorf("cosineDistance(%v, %v) = %v, want %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestComputeCentroid(t *testing.T) {
	tests := []struct {
		name       string
		embeddings [][]float64
		expected   []float64
	}{
		{
			name: "single embedding",
			embeddings: [][]float64{
				{1.0, 2.0, 3.0},
			},
			expected: []float64{1.0, 2.0, 3.0},
		},
		{
			name: "two embeddings",
			embeddings: [][]float64{
				{1.0, 2.0, 3.0},
				{3.0, 4.0, 5.0},
			},
			expected: []float64{2.0, 3.0, 4.0},
		},
		{
			name: "three embeddings",
			embeddings: [][]float64{
				{0.0, 0.0, 0.0},
				{3.0, 6.0, 9.0},
				{6.0, 3.0, 0.0},
			},
			expected: []float64{3.0, 3.0, 3.0},
		},
		{
			name:       "empty input",
			embeddings: [][]float64{},
			expected:   nil,
		},
		{
			name: "skip mismatched dimensions",
			embeddings: [][]float64{
				{1.0, 2.0, 3.0},
				{2.0, 3.0}, // mismatched - should be skipped
				{3.0, 4.0, 5.0},
			},
			expected: []float64{2.0, 3.0, 4.0}, // average of first and third only
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComputeCentroid(tt.embeddings)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("ComputeCentroid() = %v, want nil", result)
				}
				return
			}
			if len(result) != len(tt.expected) {
				t.Errorf("ComputeCentroid() length = %d, want %d", len(result), len(tt.expected))
				return
			}
			for i := range tt.expected {
				if math.Abs(result[i]-tt.expected[i]) > 1e-9 {
					t.Errorf("ComputeCentroid()[%d] = %v, want %v", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestNormalizeVector(t *testing.T) {
	tests := []struct {
		name     string
		input    []float64
		expected []float64
	}{
		{
			name:     "unit vector stays unchanged",
			input:    []float64{1.0, 0.0, 0.0},
			expected: []float64{1.0, 0.0, 0.0},
		},
		{
			name:     "scale down",
			input:    []float64{3.0, 4.0, 0.0},
			expected: []float64{0.6, 0.8, 0.0}, // 3/5, 4/5
		},
		{
			name:     "empty vector",
			input:    []float64{},
			expected: nil,
		},
		{
			name:     "zero vector",
			input:    []float64{0.0, 0.0, 0.0},
			expected: []float64{0.0, 0.0, 0.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeVector(tt.input)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("NormalizeVector() = %v, want nil", result)
				}
				return
			}
			if len(result) != len(tt.expected) {
				t.Errorf("NormalizeVector() length = %d, want %d", len(result), len(tt.expected))
				return
			}
			for i := range tt.expected {
				if math.Abs(result[i]-tt.expected[i]) > 1e-9 {
					t.Errorf("NormalizeVector()[%d] = %v, want %v", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestDBSCAN(t *testing.T) {
	tests := []struct {
		name         string
		points       [][]float64
		eps          float64
		minSamples   int
		wantClusters int
		wantNoise    int
	}{
		{
			name:         "empty input",
			points:       [][]float64{},
			eps:          0.3,
			minSamples:   2,
			wantClusters: 0,
			wantNoise:    0,
		},
		{
			name: "single cluster",
			points: [][]float64{
				{1.0, 0.0, 0.0},
				{0.99, 0.1, 0.0},
				{0.98, 0.15, 0.0},
			},
			eps:          0.2, // cosine distance threshold
			minSamples:   2,
			wantClusters: 1,
			wantNoise:    0,
		},
		{
			name: "two clusters",
			points: [][]float64{
				// Cluster 1 - pointing roughly in +x direction
				{1.0, 0.0, 0.0},
				{0.99, 0.1, 0.0},
				{0.98, 0.15, 0.0},
				// Cluster 2 - pointing roughly in +y direction
				{0.0, 1.0, 0.0},
				{0.1, 0.99, 0.0},
				{0.15, 0.98, 0.0},
			},
			eps:          0.2,
			minSamples:   2,
			wantClusters: 2,
			wantNoise:    0,
		},
		{
			name: "all noise (minSamples too high)",
			points: [][]float64{
				{1.0, 0.0, 0.0},
				{0.0, 1.0, 0.0},
				{0.0, 0.0, 1.0},
			},
			eps:          0.3,
			minSamples:   3, // Each point is isolated, can't form cluster of 3
			wantClusters: 0,
			wantNoise:    3,
		},
		{
			name: "all noise (eps too small)",
			points: [][]float64{
				{1.0, 0.0, 0.0},
				{0.0, 1.0, 0.0},
				{0.0, 0.0, 1.0},
			},
			eps:          0.01, // Too small - orthogonal vectors have distance 1.0
			minSamples:   2,
			wantClusters: 0,
			wantNoise:    3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := DBSCAN(tt.points, tt.eps, tt.minSamples)

			if len(tt.points) == 0 {
				if labels != nil {
					t.Errorf("DBSCAN() on empty input should return nil, got %v", labels)
				}
				return
			}

			// Count clusters and noise
			clusterSet := make(map[int]bool)
			noiseCount := 0
			for _, label := range labels {
				if label == -1 {
					noiseCount++
				} else if label >= 0 {
					clusterSet[label] = true
				}
			}

			if len(clusterSet) != tt.wantClusters {
				t.Errorf("DBSCAN() clusters = %d, want %d", len(clusterSet), tt.wantClusters)
			}
			if noiseCount != tt.wantNoise {
				t.Errorf("DBSCAN() noise = %d, want %d", noiseCount, tt.wantNoise)
			}
		})
	}
}

func TestGroupByCluster(t *testing.T) {
	nodes := []BaseNode{
		{NodeID: "a", SpaceID: "test", Embedding: []float64{1.0, 0.0}},
		{NodeID: "b", SpaceID: "test", Embedding: []float64{0.0, 1.0}},
		{NodeID: "c", SpaceID: "test", Embedding: []float64{1.0, 1.0}},
		{NodeID: "d", SpaceID: "test", Embedding: []float64{0.0, 0.0}},
	}
	labels := []int{0, 0, 1, -1} // a,b in cluster 0; c in cluster 1; d is noise

	clusters, noise := GroupByCluster(nodes, labels)

	// Check cluster 0
	if len(clusters[0]) != 2 {
		t.Errorf("cluster 0 size = %d, want 2", len(clusters[0]))
	}

	// Check cluster 1
	if len(clusters[1]) != 1 {
		t.Errorf("cluster 1 size = %d, want 1", len(clusters[1]))
	}

	// Check noise
	if len(noise) != 1 {
		t.Errorf("noise size = %d, want 1", len(noise))
	}
	if noise[0].NodeID != "d" {
		t.Errorf("noise[0].NodeID = %s, want d", noise[0].NodeID)
	}
}

func TestClassifyByExtension(t *testing.T) {
	nodes := []BaseNode{
		{NodeID: "1", Path: "internal/api/server.go"},
		{NodeID: "2", Path: "internal/api/handlers.go"},
		{NodeID: "3", Path: "src/service.ts"},
		{NodeID: "4", Path: "README.md"},
		{NodeID: "5", Path: "config.yaml"},
		{NodeID: "6", Path: "deploy.sh"},
		{NodeID: "7", Path: "main.py"},
		{NodeID: "8", Path: "schema.sql"},
		{NodeID: "9", Path: "Dockerfile"},
		{NodeID: "10", Path: "internal/api/server.go#NewServer"},
	}

	classes := ClassifyByExtension(nodes)

	tests := []struct {
		category string
		wantIDs  []string
	}{
		{"go", []string{"1", "2", "10"}},
		{"typescript", []string{"3"}},
		{"markdown", []string{"4"}},
		{"config", []string{"5"}},
		{"shell", []string{"6"}},
		{"python", []string{"7"}},
		{"query", []string{"8"}},
		{"other", []string{"9"}},
	}

	for _, tt := range tests {
		t.Run(tt.category, func(t *testing.T) {
			got := classes[tt.category]
			if len(got) != len(tt.wantIDs) {
				t.Errorf("%s: got %d nodes, want %d", tt.category, len(got), len(tt.wantIDs))
				return
			}
			for i, n := range got {
				if n.NodeID != tt.wantIDs[i] {
					t.Errorf("%s[%d]: got NodeID=%s, want %s", tt.category, i, n.NodeID, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestClassifyExtension(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"server.go", "go"},
		{"server.go#Method", "go"},
		{"app.tsx", "typescript"},
		{"app.js", "typescript"},
		{"script.py", "python"},
		{"README.md", "markdown"},
		{"config.yaml", "config"},
		{"config.json", "config"},
		{"deploy.sh", "shell"},
		{"query.cypher", "query"},
		{"lib.rs", "rust"},
		{"Main.java", "jvm"},
		{"style.css", "web"},
		{"schema.proto", "schema"},
		{"Dockerfile", "other"},
		{"", "other"},
		{"noext", "other"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := classifyExtension(tt.path)
			if got != tt.want {
				t.Errorf("classifyExtension(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

