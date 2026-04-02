package cli

import "testing"

func TestNeo4jMemoryTier_BoundaryValues(t *testing.T) {
	tests := []struct {
		ramGB      int
		wantCache  string
		wantInit   string
		wantMax    string
		wantPool   int
	}{
		{0, "512M", "256M", "512M", 50},   // detection failed → safest
		{1, "512M", "256M", "512M", 50},
		{8, "512M", "256M", "512M", 50},
		{16, "512M", "256M", "512M", 50},  // boundary: ≤16
		{17, "1G", "512M", "1G", 75},       // crosses into 32GB tier
		{32, "1G", "512M", "1G", 75},       // boundary: ≤32
		{33, "2G", "512M", "2G", 100},      // crosses into 64GB tier
		{64, "2G", "512M", "2G", 100},      // boundary: ≤64
		{65, "2G", "1G", "2G", 100},        // crosses into 128GB+ tier
		{128, "2G", "1G", "2G", 100},
		{256, "2G", "1G", "2G", 100},
	}
	for _, tt := range tests {
		cfg := neo4jMemoryTier(tt.ramGB)
		if cfg.Pagecache != tt.wantCache {
			t.Errorf("neo4jMemoryTier(%d).Pagecache = %s, want %s", tt.ramGB, cfg.Pagecache, tt.wantCache)
		}
		if cfg.HeapInit != tt.wantInit {
			t.Errorf("neo4jMemoryTier(%d).HeapInit = %s, want %s", tt.ramGB, cfg.HeapInit, tt.wantInit)
		}
		if cfg.HeapMax != tt.wantMax {
			t.Errorf("neo4jMemoryTier(%d).HeapMax = %s, want %s", tt.ramGB, cfg.HeapMax, tt.wantMax)
		}
		if cfg.MaxPoolSize != tt.wantPool {
			t.Errorf("neo4jMemoryTier(%d).MaxPoolSize = %d, want %d", tt.ramGB, cfg.MaxPoolSize, tt.wantPool)
		}
	}
}

func TestDetectSystemRAMGB(t *testing.T) {
	gb := DetectSystemRAMGB()
	if gb <= 0 {
		t.Errorf("DetectSystemRAMGB() = %d, want > 0", gb)
	}
	t.Logf("Detected system RAM: %d GB", gb)
}
