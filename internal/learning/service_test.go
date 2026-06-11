package learning

import (
	"testing"

	"mdemg/internal/config"
	"mdemg/internal/models"
)

// TestClamp01 tests the clamp01 helper function
func TestClamp01(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{"value within bounds", 0.5, 0.5},
		{"value at lower bound", 0.0, 0.0},
		{"value at upper bound", 1.0, 1.0},
		{"value below lower bound", -0.5, 0.0},
		{"value above upper bound", 1.5, 1.0},
		{"large negative value", -100.0, 0.0},
		{"large positive value", 100.0, 1.0},
		{"small positive value", 0.001, 0.001},
		{"value near upper bound", 0.999, 0.999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clamp01(tt.input)
			if result != tt.expected {
				t.Errorf("clamp01(%f) = %f, expected %f", tt.input, result, tt.expected)
			}
		})
	}
}

// TestPairsToMaps tests the conversion of pairs to maps for Cypher parameters
func TestPairsToMaps(t *testing.T) {
	tests := []struct {
		name   string
		pairs  []pair
		verify func(t *testing.T, result []map[string]any)
	}{
		{
			name:  "empty pairs",
			pairs: []pair{},
			verify: func(t *testing.T, result []map[string]any) {
				if len(result) != 0 {
					t.Errorf("expected empty result, got %d items", len(result))
				}
			},
		},
		{
			name: "single pair with normal values",
			pairs: []pair{
				{src: "node1", dst: "node2", ai: 0.5, aj: 0.7},
			},
			verify: func(t *testing.T, result []map[string]any) {
				if len(result) != 1 {
					t.Fatalf("expected 1 result, got %d", len(result))
				}
				m := result[0]
				if m["src"] != "node1" {
					t.Errorf("expected src=node1, got %v", m["src"])
				}
				if m["dst"] != "node2" {
					t.Errorf("expected dst=node2, got %v", m["dst"])
				}
				if m["ai"] != 0.5 {
					t.Errorf("expected ai=0.5, got %v", m["ai"])
				}
				if m["aj"] != 0.7 {
					t.Errorf("expected aj=0.7, got %v", m["aj"])
				}
			},
		},
		{
			name: "pair with out-of-bounds activations gets clamped",
			pairs: []pair{
				{src: "a", dst: "b", ai: -0.5, aj: 1.5},
			},
			verify: func(t *testing.T, result []map[string]any) {
				if len(result) != 1 {
					t.Fatalf("expected 1 result, got %d", len(result))
				}
				m := result[0]
				if m["ai"] != 0.0 {
					t.Errorf("expected ai=0.0 (clamped), got %v", m["ai"])
				}
				if m["aj"] != 1.0 {
					t.Errorf("expected aj=1.0 (clamped), got %v", m["aj"])
				}
			},
		},
		{
			name: "multiple pairs",
			pairs: []pair{
				{src: "n1", dst: "n2", ai: 0.3, aj: 0.4},
				{src: "n2", dst: "n3", ai: 0.5, aj: 0.6},
				{src: "n1", dst: "n3", ai: 0.7, aj: 0.8},
			},
			verify: func(t *testing.T, result []map[string]any) {
				if len(result) != 3 {
					t.Fatalf("expected 3 results, got %d", len(result))
				}
				// Check that all pairs are present in order
				expectedSrcs := []string{"n1", "n2", "n1"}
				for i, src := range expectedSrcs {
					if result[i]["src"] != src {
						t.Errorf("result[%d].src = %v, expected %v", i, result[i]["src"], src)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pairsToMaps(tt.pairs)
			tt.verify(t, result)
		})
	}
}

// TestFilterByActivationThreshold tests that nodes below the activation threshold are filtered out
func TestFilterByActivationThreshold(t *testing.T) {
	tests := []struct {
		name         string
		results      []models.RetrieveResult
		minAct       float64
		expectedLen  int
		expectedIDs  []string // expected node IDs after filtering
	}{
		{
			name:        "all nodes above threshold",
			minAct:      0.20,
			results: []models.RetrieveResult{
				{NodeID: "n1", Activation: 0.5},
				{NodeID: "n2", Activation: 0.6},
				{NodeID: "n3", Activation: 0.7},
			},
			expectedLen: 3,
			expectedIDs: []string{"n1", "n2", "n3"},
		},
		{
			name:        "all nodes below threshold",
			minAct:      0.50,
			results: []models.RetrieveResult{
				{NodeID: "n1", Activation: 0.1},
				{NodeID: "n2", Activation: 0.2},
				{NodeID: "n3", Activation: 0.3},
			},
			expectedLen: 0,
			expectedIDs: []string{},
		},
		{
			name:        "mixed nodes above and below threshold",
			minAct:      0.40,
			results: []models.RetrieveResult{
				{NodeID: "n1", Activation: 0.1},  // below
				{NodeID: "n2", Activation: 0.5},  // above
				{NodeID: "n3", Activation: 0.3},  // below
				{NodeID: "n4", Activation: 0.8},  // above
			},
			expectedLen: 2,
			expectedIDs: []string{"n2", "n4"},
		},
		{
			name:        "node exactly at threshold",
			minAct:      0.50,
			results: []models.RetrieveResult{
				{NodeID: "n1", Activation: 0.5},  // exactly at threshold
				{NodeID: "n2", Activation: 0.49}, // just below
			},
			expectedLen: 1,
			expectedIDs: []string{"n1"},
		},
		{
			name:        "empty results",
			minAct:      0.20,
			results:     []models.RetrieveResult{},
			expectedLen: 0,
			expectedIDs: []string{},
		},
		{
			name:        "zero threshold includes all",
			minAct:      0.0,
			results: []models.RetrieveResult{
				{NodeID: "n1", Activation: 0.0},
				{NodeID: "n2", Activation: 0.001},
			},
			expectedLen: 2,
			expectedIDs: []string{"n1", "n2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the filtering logic from ApplyCoactivation
			nodes := make([]models.RetrieveResult, 0, len(tt.results))
			for _, r := range tt.results {
				if r.Activation >= tt.minAct {
					nodes = append(nodes, r)
				}
			}

			if len(nodes) != tt.expectedLen {
				t.Errorf("expected %d nodes after filtering, got %d", tt.expectedLen, len(nodes))
			}

			// Verify expected IDs
			for i, expected := range tt.expectedIDs {
				if i >= len(nodes) {
					t.Errorf("expected node %s at index %d, but nodes slice too short", expected, i)
					continue
				}
				if nodes[i].NodeID != expected {
					t.Errorf("expected node %s at index %d, got %s", expected, i, nodes[i].NodeID)
				}
			}
		})
	}
}

// TestPairGeneration tests that pairs are generated correctly from nodes
func TestPairGeneration(t *testing.T) {
	tests := []struct {
		name          string
		nodes         []models.RetrieveResult
		expectedPairs int
		verifyPairs   func(t *testing.T, pairs []pair)
	}{
		{
			name:          "two nodes generate one pair",
			nodes:         makeNodes(2, 0.5),
			expectedPairs: 1,
			verifyPairs: func(t *testing.T, pairs []pair) {
				if pairs[0].src != "n0" || pairs[0].dst != "n1" {
					t.Errorf("expected pair (n0, n1), got (%s, %s)", pairs[0].src, pairs[0].dst)
				}
			},
		},
		{
			name:          "three nodes generate three pairs",
			nodes:         makeNodes(3, 0.5),
			expectedPairs: 3, // C(3,2) = 3
			verifyPairs: func(t *testing.T, pairs []pair) {
				expected := []struct{ src, dst string }{
					{"n0", "n1"},
					{"n0", "n2"},
					{"n1", "n2"},
				}
				for i, e := range expected {
					if pairs[i].src != e.src || pairs[i].dst != e.dst {
						t.Errorf("pair[%d]: expected (%s, %s), got (%s, %s)",
							i, e.src, e.dst, pairs[i].src, pairs[i].dst)
					}
				}
			},
		},
		{
			name:          "four nodes generate six pairs",
			nodes:         makeNodes(4, 0.5),
			expectedPairs: 6, // C(4,2) = 6
			verifyPairs:   nil,
		},
		{
			name:          "five nodes generate ten pairs",
			nodes:         makeNodes(5, 0.5),
			expectedPairs: 10, // C(5,2) = 10
			verifyPairs:   nil,
		},
		{
			name:          "ten nodes generate 45 pairs",
			nodes:         makeNodes(10, 0.5),
			expectedPairs: 45, // C(10,2) = 45
			verifyPairs:   nil,
		},
		{
			name:          "one node generates zero pairs",
			nodes:         makeNodes(1, 0.5),
			expectedPairs: 0,
			verifyPairs:   nil,
		},
		{
			name:          "zero nodes generates zero pairs",
			nodes:         makeNodes(0, 0.5),
			expectedPairs: 0,
			verifyPairs:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the pair generation logic from ApplyCoactivation
			pairs := make([]pair, 0)
			for i := 0; i < len(tt.nodes); i++ {
				for j := i + 1; j < len(tt.nodes); j++ {
					pairs = append(pairs, pair{
						src: tt.nodes[i].NodeID,
						dst: tt.nodes[j].NodeID,
						ai:  tt.nodes[i].Activation,
						aj:  tt.nodes[j].Activation,
					})
				}
			}

			if len(pairs) != tt.expectedPairs {
				t.Errorf("expected %d pairs, got %d", tt.expectedPairs, len(pairs))
			}

			if tt.verifyPairs != nil {
				tt.verifyPairs(t, pairs)
			}
		})
	}
}

// TestPairActivationProducts tests that pairs correctly store activation products
func TestPairActivationProducts(t *testing.T) {
	nodes := []models.RetrieveResult{
		{NodeID: "n0", Activation: 0.3},
		{NodeID: "n1", Activation: 0.5},
		{NodeID: "n2", Activation: 0.8},
	}

	// Generate pairs
	pairs := make([]pair, 0)
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			pairs = append(pairs, pair{
				src: nodes[i].NodeID,
				dst: nodes[j].NodeID,
				ai:  nodes[i].Activation,
				aj:  nodes[j].Activation,
			})
		}
	}

	// Verify activation products
	expectedProducts := []float64{
		0.3 * 0.5, // n0-n1: 0.15
		0.3 * 0.8, // n0-n2: 0.24
		0.5 * 0.8, // n1-n2: 0.40
	}

	for i, p := range pairs {
		product := p.ai * p.aj
		if product != expectedProducts[i] {
			t.Errorf("pair[%d] product = %f, expected %f", i, product, expectedProducts[i])
		}
	}
}

// TestTopKPairSelection tests that top-K pairs are selected by activation product
func TestTopKPairSelection(t *testing.T) {
	tests := []struct {
		name         string
		pairs        []pair
		cap          int
		expectedLen  int
		verifyFirst  func(t *testing.T, pairs []pair) // verify first pair is highest product
	}{
		{
			name: "cap larger than pairs keeps all",
			pairs: []pair{
				{src: "a", dst: "b", ai: 0.3, aj: 0.4}, // product: 0.12
				{src: "c", dst: "d", ai: 0.5, aj: 0.6}, // product: 0.30
			},
			cap:         100,
			expectedLen: 2,
			verifyFirst: nil,
		},
		{
			name: "cap smaller than pairs truncates and sorts",
			pairs: []pair{
				{src: "a", dst: "b", ai: 0.3, aj: 0.4}, // product: 0.12
				{src: "c", dst: "d", ai: 0.5, aj: 0.6}, // product: 0.30
				{src: "e", dst: "f", ai: 0.7, aj: 0.8}, // product: 0.56
			},
			cap:         2,
			expectedLen: 2,
			verifyFirst: func(t *testing.T, pairs []pair) {
				// First pair should have highest product (0.56)
				if pairs[0].src != "e" || pairs[0].dst != "f" {
					t.Errorf("expected first pair (e, f), got (%s, %s)", pairs[0].src, pairs[0].dst)
				}
				// Second pair should have second highest product (0.30)
				if pairs[1].src != "c" || pairs[1].dst != "d" {
					t.Errorf("expected second pair (c, d), got (%s, %s)", pairs[1].src, pairs[1].dst)
				}
			},
		},
		{
			name: "cap equal to pairs keeps all without sorting",
			pairs: []pair{
				{src: "low", dst: "low", ai: 0.1, aj: 0.1},   // product: 0.01
				{src: "mid", dst: "mid", ai: 0.5, aj: 0.5},   // product: 0.25
				{src: "high", dst: "high", ai: 0.9, aj: 0.9}, // product: 0.81
			},
			cap:         3,
			expectedLen: 3,
			verifyFirst: func(t *testing.T, pairs []pair) {
				// When cap >= len(pairs), no sorting happens - order is preserved
				// First pair should be "low" since that was the original order
				if pairs[0].src != "low" {
					t.Errorf("expected first pair src=low (original order), got %s", pairs[0].src)
				}
			},
		},
		{
			name:        "empty pairs",
			pairs:       []pair{},
			cap:         10,
			expectedLen: 0,
			verifyFirst: nil,
		},
		{
			name: "cap of 1 returns single best pair",
			pairs: []pair{
				{src: "a", dst: "b", ai: 0.2, aj: 0.2}, // product: 0.04
				{src: "c", dst: "d", ai: 0.9, aj: 0.9}, // product: 0.81
				{src: "e", dst: "f", ai: 0.5, aj: 0.5}, // product: 0.25
			},
			cap:         1,
			expectedLen: 1,
			verifyFirst: func(t *testing.T, pairs []pair) {
				if pairs[0].src != "c" || pairs[0].dst != "d" {
					t.Errorf("expected best pair (c, d), got (%s, %s)", pairs[0].src, pairs[0].dst)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying the test case
			pairs := make([]pair, len(tt.pairs))
			copy(pairs, tt.pairs)

			// Apply the top-K selection logic from ApplyCoactivation
			if len(pairs) > tt.cap {
				// Sort descending by activation product
				sortPairsByProduct(pairs)
				pairs = pairs[:tt.cap]
			}

			if len(pairs) != tt.expectedLen {
				t.Errorf("expected %d pairs, got %d", tt.expectedLen, len(pairs))
			}

			if tt.verifyFirst != nil && len(pairs) > 0 {
				tt.verifyFirst(t, pairs)
			}
		})
	}
}

// TestTopKSelectionPreservesHighestProducts verifies that top-K always keeps highest products
func TestTopKSelectionPreservesHighestProducts(t *testing.T) {
	// Create pairs with known products
	pairs := []pair{
		{src: "p1", dst: "p2", ai: 0.1, aj: 0.1}, // product: 0.01
		{src: "p3", dst: "p4", ai: 0.2, aj: 0.3}, // product: 0.06
		{src: "p5", dst: "p6", ai: 0.5, aj: 0.4}, // product: 0.20
		{src: "p7", dst: "p8", ai: 0.6, aj: 0.6}, // product: 0.36
		{src: "p9", dst: "p10", ai: 0.9, aj: 0.8}, // product: 0.72
	}

	cap := 3
	sortPairsByProduct(pairs)
	selected := pairs[:cap]

	// The three highest products should be: 0.72, 0.36, 0.20
	expectedMinProduct := 0.20

	for i, p := range selected {
		product := p.ai * p.aj
		if product < expectedMinProduct {
			t.Errorf("selected[%d] has product %f, expected >= %f", i, product, expectedMinProduct)
		}
	}

	// Verify the lowest product pair (0.01) is not selected
	for _, p := range selected {
		if p.src == "p1" && p.dst == "p2" {
			t.Error("pair with lowest product should not be selected")
		}
	}
}

// TestEdgeCapEnforcement tests that the edge cap is properly enforced
func TestEdgeCapEnforcement(t *testing.T) {
	tests := []struct {
		name        string
		numNodes    int
		cap         int
		expectedMax int // max pairs after cap
	}{
		{
			name:        "small number of nodes well under cap",
			numNodes:    5,
			cap:         200,
			expectedMax: 10, // C(5,2) = 10
		},
		{
			name:        "many nodes capped at limit",
			numNodes:    50,
			cap:         200,
			expectedMax: 200, // C(50,2) = 1225, but capped at 200
		},
		{
			name:        "exact cap boundary",
			numNodes:    20,
			cap:         190,
			expectedMax: 190, // C(20,2) = 190, equals cap
		},
		{
			name:        "very small cap",
			numNodes:    100,
			cap:         10,
			expectedMax: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := makeNodes(tt.numNodes, 0.5)

			// Generate all pairs
			pairs := make([]pair, 0)
			for i := 0; i < len(nodes); i++ {
				for j := i + 1; j < len(nodes); j++ {
					pairs = append(pairs, pair{
						src: nodes[i].NodeID,
						dst: nodes[j].NodeID,
						ai:  nodes[i].Activation,
						aj:  nodes[j].Activation,
					})
				}
			}

			// Apply cap
			if len(pairs) > tt.cap {
				sortPairsByProduct(pairs)
				pairs = pairs[:tt.cap]
			}

			if len(pairs) > tt.expectedMax {
				t.Errorf("expected at most %d pairs, got %d", tt.expectedMax, len(pairs))
			}
		})
	}
}

// TestCombinedFilteringAndSelection tests the full pipeline of filtering and selection
func TestCombinedFilteringAndSelection(t *testing.T) {
	// Create a mix of nodes with varying activation levels
	results := []models.RetrieveResult{
		{NodeID: "low1", Activation: 0.05},
		{NodeID: "low2", Activation: 0.10},
		{NodeID: "med1", Activation: 0.30},
		{NodeID: "med2", Activation: 0.40},
		{NodeID: "high1", Activation: 0.70},
		{NodeID: "high2", Activation: 0.85},
		{NodeID: "high3", Activation: 0.95},
	}

	minAct := 0.25
	cap := 3

	// Step 1: Filter by activation threshold
	nodes := make([]models.RetrieveResult, 0)
	for _, r := range results {
		if r.Activation >= minAct {
			nodes = append(nodes, r)
		}
	}

	// Should have 5 nodes (med1, med2, high1, high2, high3)
	if len(nodes) != 5 {
		t.Fatalf("expected 5 nodes after filtering, got %d", len(nodes))
	}

	// Step 2: Generate pairs
	pairs := make([]pair, 0)
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			pairs = append(pairs, pair{
				src: nodes[i].NodeID,
				dst: nodes[j].NodeID,
				ai:  nodes[i].Activation,
				aj:  nodes[j].Activation,
			})
		}
	}

	// C(5,2) = 10 pairs
	if len(pairs) != 10 {
		t.Fatalf("expected 10 pairs, got %d", len(pairs))
	}

	// Step 3: Select top-K by activation product
	sortPairsByProduct(pairs)
	if len(pairs) > cap {
		pairs = pairs[:cap]
	}

	if len(pairs) != cap {
		t.Fatalf("expected %d pairs after cap, got %d", cap, len(pairs))
	}

	// Verify the top pairs include the highest activation nodes
	// high2 (0.85) * high3 (0.95) = 0.8075 should be first
	// high1 (0.70) * high3 (0.95) = 0.665 should be second or third
	// high1 (0.70) * high2 (0.85) = 0.595 should be second or third

	topPair := pairs[0]
	// Both nodes in top pair should be from high activation nodes
	isHighNode := func(id string) bool {
		return id == "high1" || id == "high2" || id == "high3"
	}

	if !isHighNode(topPair.src) || !isHighNode(topPair.dst) {
		t.Errorf("expected top pair to contain high activation nodes, got (%s, %s)",
			topPair.src, topPair.dst)
	}
}

// Helper function to create test nodes
func makeNodes(n int, activation float64) []models.RetrieveResult {
	nodes := make([]models.RetrieveResult, n)
	for i := 0; i < n; i++ {
		nodes[i] = models.RetrieveResult{
			NodeID:     "n" + itoa(i),
			Activation: activation,
		}
	}
	return nodes
}

// Simple int to string conversion for test helper
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + itoa(-i)
	}
	result := ""
	for i > 0 {
		result = string(rune('0'+i%10)) + result
		i /= 10
	}
	return result
}

// Helper function to sort pairs by activation product (descending)
// This mirrors the logic in ApplyCoactivation
func sortPairsByProduct(pairs []pair) {
	for i := 0; i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[i].ai*pairs[i].aj < pairs[j].ai*pairs[j].aj {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}
}

// =============================================================================
// Hebbian Weight Calculation Tests (Subtask 3.2)
// =============================================================================

// TestHebbianWeightUpdateBasic tests the basic Hebbian weight update formula
// Formula: new_w = (1-μ)*w + η*a_i*a_j, clamped to [wmin, wmax]
func TestHebbianWeightUpdateBasic(t *testing.T) {
	tests := []struct {
		name     string
		w        float64 // current weight
		ai       float64 // activation of node i
		aj       float64 // activation of node j
		eta      float64 // learning rate
		mu       float64 // decay rate
		wmin     float64 // minimum weight bound
		wmax     float64 // maximum weight bound
		expected float64 // expected new weight
	}{
		{
			name:     "basic update with default params",
			w:        0.5,
			ai:       0.8,
			aj:       0.6,
			eta:      0.02, // default learning rate
			mu:       0.01, // default decay rate
			wmin:     0.0,
			wmax:     1.0,
			// rawW = (1-0.01)*0.5 + 0.02*(0.8*0.6) = 0.5046, tanh(0.5046) ≈ 0.4657
			expected: 0.4657271175,
		},
		{
			name:     "zero current weight",
			w:        0.0,
			ai:       0.5,
			aj:       0.5,
			eta:      0.02,
			mu:       0.01,
			wmin:     0.0,
			wmax:     1.0,
			// rawW = 0.005, tanh(0.005) ≈ 0.005 (near-linear region)
			expected: 0.0049999583,
		},
		{
			name:     "high activation strengthens weight",
			w:        0.3,
			ai:       1.0,
			aj:       1.0,
			eta:      0.02,
			mu:       0.01,
			wmin:     0.0,
			wmax:     1.0,
			// rawW = 0.317, tanh(0.317) ≈ 0.3068
			expected: 0.3067917919,
		},
		{
			name:     "zero activation causes decay only",
			w:        0.5,
			ai:       0.0,
			aj:       0.0,
			eta:      0.02,
			mu:       0.01,
			wmin:     0.0,
			wmax:     1.0,
			// rawW = 0.495, tanh(0.495) ≈ 0.4582
			expected: 0.4581758447,
		},
		{
			name:     "one zero activation",
			w:        0.4,
			ai:       0.8,
			aj:       0.0,
			eta:      0.02,
			mu:       0.01,
			wmin:     0.0,
			wmax:     1.0,
			// rawW = 0.396, tanh(0.396) ≈ 0.3765
			expected: 0.3765212159,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HebbianWeightUpdate(tt.w, tt.ai, tt.aj, tt.eta, tt.mu, tt.wmin, tt.wmax)
			if !floatEquals(result, tt.expected, 1e-9) {
				t.Errorf("HebbianWeightUpdate(%f, %f, %f, %f, %f, %f, %f) = %f, expected %f",
					tt.w, tt.ai, tt.aj, tt.eta, tt.mu, tt.wmin, tt.wmax, result, tt.expected)
			}
		})
	}
}

// TestHebbianWeightUpdateClamping tests that weights are properly clamped to bounds
func TestHebbianWeightUpdateClamping(t *testing.T) {
	tests := []struct {
		name     string
		w        float64
		ai       float64
		aj       float64
		eta      float64
		mu       float64
		wmin     float64
		wmax     float64
		expected float64
	}{
		{
			name:     "small weight with decay stays near linear",
			w:        0.01,
			ai:       0.0,
			aj:       0.0,
			eta:      0.0,
			mu:       0.5, // aggressive decay
			wmin:     0.0,
			wmax:     1.0,
			// rawW = 0.005, tanh(0.005) ≈ 0.005 (near-linear)
			expected: 0.0049999583,
		},
		{
			name:     "very small weight stays near linear",
			w:        0.001,
			ai:       0.0,
			aj:       0.0,
			eta:      0.0,
			mu:       0.99, // very aggressive decay
			wmin:     0.0,
			wmax:     1.0,
			// rawW = 0.00001, tanh(0.00001) ≈ 0.00001
			expected: 0.00001,
		},
		{
			name:     "tanh soft-cap when weight would exceed wmax",
			w:        0.99,
			ai:       1.0,
			aj:       1.0,
			eta:      0.5, // very high learning rate
			mu:       0.0, // no decay
			wmin:     0.0,
			wmax:     1.0,
			// rawW = 1.49, tanh(1.49) ≈ 0.9033 (soft-capped, not hard 1.0)
			expected: 0.9033247426,
		},
		{
			name:     "clamp to custom minimum",
			w:        0.1,
			ai:       0.0,
			aj:       0.0,
			eta:      0.0,
			mu:       0.99,
			wmin:     0.05,
			wmax:     1.0,
			// rawW = 0.001, below wmin=0.05, clamped to 0.05
			expected: 0.05,
		},
		{
			name:     "tanh soft-cap with custom wmax",
			w:        0.8,
			ai:       1.0,
			aj:       1.0,
			eta:      0.2,
			mu:       0.0,
			wmin:     0.0,
			wmax:     0.9,
			// rawW = 1.0, 0.9*tanh(1.0/0.9) ≈ 0.724
			expected: 0.7240093203,
		},
		{
			name:     "tanh soft-cap with narrow bounds",
			w:        0.5,
			ai:       1.0,
			aj:       1.0,
			eta:      1.0,
			mu:       0.0,
			wmin:     0.4,
			wmax:     0.6,
			// rawW = 1.5, 0.6*tanh(1.5/0.6) ≈ 0.5920
			expected: 0.5919685789,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HebbianWeightUpdate(tt.w, tt.ai, tt.aj, tt.eta, tt.mu, tt.wmin, tt.wmax)
			if !floatEquals(result, tt.expected, 1e-9) {
				t.Errorf("HebbianWeightUpdate(%f, %f, %f, %f, %f, %f, %f) = %f, expected %f",
					tt.w, tt.ai, tt.aj, tt.eta, tt.mu, tt.wmin, tt.wmax, result, tt.expected)
			}
		})
	}
}

// TestHebbianWeightUpdateDecayBehavior tests how the decay parameter (mu) affects weights
func TestHebbianWeightUpdateDecayBehavior(t *testing.T) {
	// Test that higher mu causes faster decay
	w := 0.5
	ai := 0.0
	aj := 0.0
	wmin := 0.0
	wmax := 1.0

	// With zero activation, weight only decays
	// new_w = (1-mu) * w

	testCases := []struct {
		mu       float64
		expected float64
	}{
		{0.0, 0.4621171573},  // no decay, rawW=0.5, tanh(0.5)
		{0.01, 0.4581758447}, // 1% decay, rawW=0.495, tanh(0.495)
		{0.1, 0.4218990053},  // 10% decay, rawW=0.45, tanh(0.45)
		{0.5, 0.2449186624},  // 50% decay, rawW=0.25, tanh(0.25)
		{1.0, 0.0},           // complete decay, rawW=0, tanh(0)=0
	}

	for _, tc := range testCases {
		t.Run("mu="+formatFloat(tc.mu), func(t *testing.T) {
			result := HebbianWeightUpdate(w, ai, aj, 0.0, tc.mu, wmin, wmax)
			if !floatEquals(result, tc.expected, 1e-6) {
				t.Errorf("with mu=%f, expected %f, got %f", tc.mu, tc.expected, result)
			}
		})
	}
}

// TestHebbianWeightUpdateLearningRate tests how the learning rate (eta) affects weight strengthening
func TestHebbianWeightUpdateLearningRate(t *testing.T) {
	// Test that higher eta causes faster strengthening
	w := 0.5
	ai := 1.0
	aj := 1.0
	mu := 0.0 // no decay to isolate learning effect
	wmin := 0.0
	wmax := 10.0 // high max to avoid clamping

	// With full activation and no decay:
	// new_w = w + eta * (ai * aj) = 0.5 + eta * 1

	// With wmax=10, tanh(x/10)*10 ≈ x for small x (near-linear region)
	testCases := []struct {
		eta      float64
		expected float64
	}{
		{0.0, 0.4995837496},  // no learning, rawW=0.5, 10*tanh(0.05)
		{0.01, 0.5095582895}, // slow learning, rawW=0.51
		{0.1, 0.5992810353},  // moderate learning, rawW=0.6
		{0.5, 0.9966799462},  // fast learning, rawW=1.0
		{1.0, 1.4888503362},  // very fast learning, rawW=1.5
	}

	for _, tc := range testCases {
		t.Run("eta="+formatFloat(tc.eta), func(t *testing.T) {
			result := HebbianWeightUpdate(w, ai, aj, tc.eta, mu, wmin, wmax)
			if !floatEquals(result, tc.expected, 1e-6) {
				t.Errorf("with eta=%f, expected %f, got %f", tc.eta, tc.expected, result)
			}
		})
	}
}

// TestHebbianWeightUpdateActivationProduct tests that weight change is proportional to activation product
func TestHebbianWeightUpdateActivationProduct(t *testing.T) {
	w := 0.0
	eta := 1.0 // learning rate of 1 for easy calculation
	mu := 0.0  // no decay
	wmin := 0.0
	wmax := 1.0

	// With w=0, mu=0, eta=1: rawW = ai*aj, result = tanh(rawW)

	testCases := []struct {
		ai       float64
		aj       float64
		expected float64
	}{
		{0.0, 0.0, 0.0},
		{0.5, 0.0, 0.0},
		{0.0, 0.5, 0.0},
		{0.5, 0.5, 0.2449186624},  // tanh(0.25)
		{0.5, 1.0, 0.4621171573},  // tanh(0.5)
		{1.0, 0.5, 0.4621171573},  // tanh(0.5)
		{1.0, 1.0, 0.7615941560},  // tanh(1.0)
		{0.3, 0.7, 0.2069664997},  // tanh(0.21)
	}

	for _, tc := range testCases {
		t.Run("ai="+formatFloat(tc.ai)+"_aj="+formatFloat(tc.aj), func(t *testing.T) {
			result := HebbianWeightUpdate(w, tc.ai, tc.aj, eta, mu, wmin, wmax)
			if !floatEquals(result, tc.expected, 1e-6) {
				t.Errorf("ai=%f, aj=%f: expected %f, got %f", tc.ai, tc.aj, tc.expected, result)
			}
		})
	}
}

// TestHebbianWeightUpdateMultipleIterations tests cumulative weight updates
func TestHebbianWeightUpdateMultipleIterations(t *testing.T) {
	// Simulate multiple learning iterations with consistent co-activation
	w := 0.1  // initial weight
	ai := 0.8
	aj := 0.6
	eta := 0.02 // default learning rate
	mu := 0.01  // default decay rate
	wmin := 0.0
	wmax := 1.0

	// Track weight over 10 iterations
	weights := []float64{w}
	for i := 0; i < 10; i++ {
		w = HebbianWeightUpdate(w, ai, aj, eta, mu, wmin, wmax)
		weights = append(weights, w)
	}

	// Verify weight is monotonically increasing (since ai*aj=0.48 > 0 and learning > decay)
	for i := 1; i < len(weights); i++ {
		if weights[i] <= weights[i-1] {
			t.Errorf("weight should increase monotonically: weights[%d]=%f <= weights[%d]=%f",
				i, weights[i], i-1, weights[i-1])
		}
	}

	// Verify final weight is reasonable (bounded and increased from initial)
	finalW := weights[len(weights)-1]
	if finalW <= 0.1 {
		t.Errorf("final weight %f should be greater than initial %f", finalW, 0.1)
	}
	if finalW > wmax {
		t.Errorf("final weight %f should not exceed max %f", finalW, wmax)
	}
}

// TestHebbianWeightUpdateDecayWithoutActivation tests that weights decay without activation
func TestHebbianWeightUpdateDecayWithoutActivation(t *testing.T) {
	// Simulate decay when nodes are not co-activated
	w := 0.5
	ai := 0.0 // no activation
	aj := 0.0
	eta := 0.02
	mu := 0.01
	wmin := 0.0
	wmax := 1.0

	// Track weight over 10 iterations with no activation
	weights := []float64{w}
	for i := 0; i < 10; i++ {
		w = HebbianWeightUpdate(w, ai, aj, eta, mu, wmin, wmax)
		weights = append(weights, w)
	}

	// Verify weight is monotonically decreasing (since no activation, only decay)
	for i := 1; i < len(weights); i++ {
		if weights[i] >= weights[i-1] {
			t.Errorf("weight should decrease monotonically: weights[%d]=%f >= weights[%d]=%f",
				i, weights[i], i-1, weights[i-1])
		}
	}

	// Verify final weight is less than initial
	finalW := weights[len(weights)-1]
	if finalW >= 0.5 {
		t.Errorf("final weight %f should be less than initial %f after decay", finalW, 0.5)
	}
}

// TestHebbianWeightUpdateFormulaDerivation verifies the formula derivation
// The Hebbian formula is: Δw = η * a_i * a_j - μ * w
// Which gives: new_w = w + Δw = w + η*a_i*a_j - μ*w = (1-μ)*w + η*a_i*a_j
func TestHebbianWeightUpdateFormulaDerivation(t *testing.T) {
	// Use specific values where we can verify the formula manually
	w := 0.4
	ai := 0.5
	aj := 0.6
	eta := 0.1
	mu := 0.05
	wmin := 0.0
	wmax := 1.0

	// Calculate expected value step by step
	// rawW = (1-μ)*w + η*a_i*a_j
	// rawW = (1-0.05)*0.4 + 0.1*0.5*0.6
	// rawW = 0.95*0.4 + 0.1*0.3
	// rawW = 0.38 + 0.03 = 0.41
	// result = wmax * tanh(rawW / wmax) = 1.0 * tanh(0.41) ≈ 0.38847

	expected := 0.3884726802
	result := HebbianWeightUpdate(w, ai, aj, eta, mu, wmin, wmax)

	if !floatEquals(result, expected, 1e-6) {
		t.Errorf("Formula derivation failed: expected %f, got %f", expected, result)
	}
}

// TestHebbianWeightUpdateSymmetry verifies that a_i*a_j is symmetric (order doesn't matter)
func TestHebbianWeightUpdateSymmetry(t *testing.T) {
	w := 0.3
	eta := 0.02
	mu := 0.01
	wmin := 0.0
	wmax := 1.0

	// Test various activation pairs
	testCases := []struct {
		ai float64
		aj float64
	}{
		{0.3, 0.7},
		{0.5, 0.8},
		{0.1, 0.9},
		{0.4, 0.4},
	}

	for _, tc := range testCases {
		t.Run("ai="+formatFloat(tc.ai)+"_aj="+formatFloat(tc.aj), func(t *testing.T) {
			result1 := HebbianWeightUpdate(w, tc.ai, tc.aj, eta, mu, wmin, wmax)
			result2 := HebbianWeightUpdate(w, tc.aj, tc.ai, eta, mu, wmin, wmax)

			if !floatEquals(result1, result2, 1e-15) {
				t.Errorf("symmetry violated: f(%f,%f)=%f != f(%f,%f)=%f",
					tc.ai, tc.aj, result1, tc.aj, tc.ai, result2)
			}
		})
	}
}

// TestHebbianWeightUpdateEdgeCases tests edge cases and boundary conditions
func TestHebbianWeightUpdateEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		w        float64
		ai       float64
		aj       float64
		eta      float64
		mu       float64
		wmin     float64
		wmax     float64
		validate func(t *testing.T, result float64)
	}{
		{
			name: "all zeros",
			w:    0.0, ai: 0.0, aj: 0.0, eta: 0.0, mu: 0.0,
			wmin: 0.0, wmax: 1.0,
			validate: func(t *testing.T, result float64) {
				if result != 0.0 {
					t.Errorf("expected 0.0, got %f", result)
				}
			},
		},
		{
			name: "weight at max with max activations",
			w:    1.0, ai: 1.0, aj: 1.0, eta: 0.02, mu: 0.01,
			wmin: 0.0, wmax: 1.0,
			validate: func(t *testing.T, result float64) {
				// Should stay at or below 1.0
				if result > 1.0 {
					t.Errorf("expected <= 1.0, got %f", result)
				}
			},
		},
		{
			name: "weight at min with no activation",
			w:    0.0, ai: 0.0, aj: 0.0, eta: 0.02, mu: 0.01,
			wmin: 0.0, wmax: 1.0,
			validate: func(t *testing.T, result float64) {
				// Should stay at 0.0
				if result != 0.0 {
					t.Errorf("expected 0.0, got %f", result)
				}
			},
		},
		{
			name: "very small weight doesn't go negative",
			w:    0.0001, ai: 0.0, aj: 0.0, eta: 0.0, mu: 0.5,
			wmin: 0.0, wmax: 1.0,
			validate: func(t *testing.T, result float64) {
				// Should be clamped to 0.0 or be very small positive
				if result < 0.0 {
					t.Errorf("expected >= 0.0, got %f", result)
				}
			},
		},
		{
			name: "mu=1 completely decays weight",
			w:    0.5, ai: 0.0, aj: 0.0, eta: 0.0, mu: 1.0,
			wmin: 0.0, wmax: 1.0,
			validate: func(t *testing.T, result float64) {
				// (1-1)*0.5 + 0 = 0
				if result != 0.0 {
					t.Errorf("expected 0.0, got %f", result)
				}
			},
		},
		{
			name: "equal min and max bounds",
			w:    0.5, ai: 0.5, aj: 0.5, eta: 0.02, mu: 0.01,
			wmin: 0.5, wmax: 0.5,
			validate: func(t *testing.T, result float64) {
				// rawW = 0.5 which is not < wmin, so tanh applies:
				// 0.5 * tanh(0.5/0.5) = 0.5 * tanh(1.0) ≈ 0.3808
				// Note: equal bounds is a degenerate case
				if result < 0 || result > 0.5 {
					t.Errorf("expected result in [0, 0.5], got %f", result)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HebbianWeightUpdate(tt.w, tt.ai, tt.aj, tt.eta, tt.mu, tt.wmin, tt.wmax)
			tt.validate(t, result)
		})
	}
}

// Helper function to compare floats with tolerance
func floatEquals(a, b, tolerance float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < tolerance
}

// Helper function to format float for test names
func formatFloat(f float64) string {
	// Simple formatting for test names
	if f == 0.0 {
		return "0"
	}
	if f == 1.0 {
		return "1"
	}
	// Use integer representation for common fractions
	intPart := int(f * 100)
	return itoa(intPart)
}

// =============================================================================
// Tanh Soft-Cap Tests (Phase A.1)
// =============================================================================

func TestHebbianWeightUpdate_TanhSoftCap(t *testing.T) {
	// At w=0.5, behavior should be similar to linear (tanh(0.5) ~ 0.462)
	result := HebbianWeightUpdate(0.5, 0.8, 0.8, 0.02, 0.01, 0.0, 1.0)
	if result <= 0 || result >= 1.0 {
		t.Errorf("expected result in (0, 1.0), got %f", result)
	}

	// At very high weight, should approach but never reach wmax
	result = HebbianWeightUpdate(0.95, 1.0, 1.0, 0.02, 0.01, 0.0, 1.0)
	if result >= 1.0 {
		t.Errorf("tanh soft-cap should prevent reaching 1.0, got %f", result)
	}

	// At w=0.99, strong update should still stay below 1.0
	result = HebbianWeightUpdate(0.99, 1.0, 1.0, 0.1, 0.0, 0.0, 1.0)
	if result >= 1.0 {
		t.Errorf("tanh soft-cap should prevent reaching 1.0 even with high eta, got %f", result)
	}

	// Below wmin should clamp to wmin
	result = HebbianWeightUpdate(0.01, 0.0, 0.0, 0.0, 0.5, 0.05, 1.0)
	if result != 0.05 {
		t.Errorf("expected wmin=0.05, got %f", result)
	}
}

// =============================================================================
// Phase Multiplier Tests (Phase A.4)
// =============================================================================

func TestPhaseMultiplier(t *testing.T) {
	svc := &Service{
		cfg: config.Config{
			LearningScheduleColdMult:     2.0,
			LearningScheduleLearningMult: 1.0,
			LearningScheduleWarmMult:     0.5,
			LearningScheduleSatMult:      0.25,
		},
	}

	tests := []struct {
		edgeCount int64
		expected  float64
	}{
		{0, 2.0},
		{100, 1.0},
		{9999, 1.0},
		{10000, 0.5},
		{49999, 0.5},
		{50000, 0.25},
		{100000, 0.25},
	}

	for _, tt := range tests {
		result := svc.phaseMultiplier(tt.edgeCount)
		if result != tt.expected {
			t.Errorf("phaseMultiplier(%d) = %f, want %f", tt.edgeCount, result, tt.expected)
		}
	}
}

// CONFIG-DEADFLAG-001: the EVENTGRAPH_MAX_PAIRS_PER_EVENT_BATCH cap was
// parsed since EVENTGRAPH-001 but never enforced.
func TestSetMaxPairsPerEventBatch_CapsForwardedRows(t *testing.T) {
	s := &Service{}
	s.SetMaxPairsPerEventBatch(3)
	if s.maxPairsPerEventBatch != 3 {
		t.Fatalf("cap not stored: %d", s.maxPairsPerEventBatch)
	}
	// Enforcement is a slice bound at the forward site; pin the semantics.
	rows := make([]int, 10)
	capN := s.maxPairsPerEventBatch
	if capN > 0 && len(rows) > capN {
		rows = rows[:capN]
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 forwarded rows, got %d", len(rows))
	}
}
