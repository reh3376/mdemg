package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewSpecIndex(t *testing.T) {
	idx := NewSpecIndex()
	if idx.TotalSpecs() != 0 {
		t.Errorf("new index should have 0 specs, got %d", idx.TotalSpecs())
	}
	if idx.FrameworkCount() != 0 {
		t.Errorf("new index should have 0 frameworks, got %d", idx.FrameworkCount())
	}
}

func TestStripTemplateVars(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/v1/memory/nodes/{{id}}", "/v1/memory/nodes"},
		{"/v1/memory/retrieve", "/v1/memory/retrieve"},
		{"/v1/{{space}}/nodes/{{id}}", "/v1/nodes"},
		{"{{root}}", "/"},
		{"/v1/conversation/observe", "/v1/conversation/observe"},
	}

	for _, tt := range tests {
		got := stripTemplateVars(tt.input)
		if got != tt.expected {
			t.Errorf("stripTemplateVars(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestLoadWithFixtures(t *testing.T) {
	// Create temp spec fixtures
	tmpDir := t.TempDir()

	// Create UATS spec directory structure
	uatsDir := filepath.Join(tmpDir, "api", "api-spec", "uats", "specs")
	if err := os.MkdirAll(uatsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	specJSON := `{
		"uats_version": "1.0.0",
		"api": {"name": "Test", "tags": ["memory"]},
		"request": {"method": "GET", "path": "/v1/memory/retrieve"},
		"expected": {"status": 200},
		"config": {"timeout_ms": 5000},
		"metadata": {"description": "Test spec", "tags": ["contract"]}
	}`

	if err := os.WriteFile(filepath.Join(uatsDir, "test.uats.json"), []byte(specJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	idx := NewSpecIndex()
	if err := idx.Load(tmpDir, "uats", false); err != nil {
		t.Fatal(err)
	}

	if idx.TotalSpecs() != 1 {
		t.Errorf("expected 1 spec, got %d", idx.TotalSpecs())
	}

	if idx.FrameworkCount() != 1 {
		t.Errorf("expected 1 framework, got %d", idx.FrameworkCount())
	}

	// Test path lookup
	specs := idx.FindByPath("/v1/memory/retrieve")
	if len(specs) != 1 {
		t.Errorf("expected 1 spec for path, got %d", len(specs))
	}

	// Test tag lookup
	specs = idx.FindByTag("memory")
	if len(specs) != 1 {
		t.Errorf("expected 1 spec for tag 'memory', got %d", len(specs))
	}

	specs = idx.FindByTag("contract")
	if len(specs) != 1 {
		t.Errorf("expected 1 spec for tag 'contract', got %d", len(specs))
	}
}

func TestLoadUDTSFixture(t *testing.T) {
	tmpDir := t.TempDir()

	udtsDir := filepath.Join(tmpDir, "api", "api-spec", "udts", "specs")
	if err := os.MkdirAll(udtsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	specJSON := `{
		"udts_version": "1.0.0",
		"service": "mdemg.module.v1.ReasoningModule",
		"method": "Process",
		"request": {},
		"expected": {"status_code": "OK"},
		"config": {"timeout_ms": 5000},
		"metadata": {"description": "Process RPC", "tags": ["contract"]}
	}`

	if err := os.WriteFile(filepath.Join(udtsDir, "test_process.udts.json"), []byte(specJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	idx := NewSpecIndex()
	if err := idx.Load(tmpDir, "udts", false); err != nil {
		t.Fatal(err)
	}

	if idx.TotalSpecs() != 1 {
		t.Errorf("expected 1 spec, got %d", idx.TotalSpecs())
	}

	// Test service lookup
	specs := idx.FindByService("mdemg.module.v1.ReasoningModule", "Process")
	if len(specs) != 1 {
		t.Errorf("expected 1 spec for service.method, got %d", len(specs))
	}
}

func TestFindByText(t *testing.T) {
	tmpDir := t.TempDir()

	uatsDir := filepath.Join(tmpDir, "api", "api-spec", "uats", "specs")
	if err := os.MkdirAll(uatsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	spec1 := `{
		"uats_version": "1.0.0",
		"request": {"path": "/v1/memory/retrieve"},
		"expected": {"status": 200},
		"metadata": {"description": "Retrieve memory nodes"}
	}`
	spec2 := `{
		"uats_version": "1.0.0",
		"request": {"path": "/v1/conversation/observe"},
		"expected": {"status": 200},
		"metadata": {"description": "Observe conversation events"}
	}`

	if err := os.WriteFile(filepath.Join(uatsDir, "retrieve.uats.json"), []byte(spec1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uatsDir, "observe.uats.json"), []byte(spec2), 0o644); err != nil {
		t.Fatal(err)
	}

	idx := NewSpecIndex()
	if err := idx.Load(tmpDir, "uats", false); err != nil {
		t.Fatal(err)
	}

	// Search by description
	specs := idx.FindByText("retrieve")
	if len(specs) != 1 {
		t.Errorf("expected 1 match for 'retrieve', got %d", len(specs))
	}

	// Search by filename
	specs = idx.FindByText("observe")
	if len(specs) != 1 {
		t.Errorf("expected 1 match for 'observe', got %d", len(specs))
	}
}

func TestFrameworkStats(t *testing.T) {
	tmpDir := t.TempDir()

	uatsDir := filepath.Join(tmpDir, "api", "api-spec", "uats", "specs")
	if err := os.MkdirAll(uatsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	spec := `{"uats_version":"1.0.0","request":{"path":"/v1/test"},"expected":{"status":200}}`
	if err := os.WriteFile(filepath.Join(uatsDir, "one.uats.json"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uatsDir, "two.uats.json"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	idx := NewSpecIndex()
	if err := idx.Load(tmpDir, "uats", false); err != nil {
		t.Fatal(err)
	}

	stats := idx.FrameworkStats()
	if stats["uats"] != 2 {
		t.Errorf("expected 2 uats specs, got %d", stats["uats"])
	}
}
