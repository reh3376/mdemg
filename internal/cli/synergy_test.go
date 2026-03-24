package cli

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

const floatTol = 1e-9

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < floatTol
}

func TestSynergyStatusLocal_JSONOutput(t *testing.T) {
	// Set env vars so the function finds no real files and doesn't panic
	t.Setenv("SYNERGY_CLAUDE_MD_PATH", "/nonexistent/CLAUDE.md")
	t.Setenv("SYNERGY_MEMORY_MD_PATH", "/nonexistent/MEMORY.md")
	t.Setenv("MDEMG_URL", "http://localhost:1") // unreachable, forces local path

	// Should not panic — writes JSON to stdout
	err := runSynergyStatusLocal("test-space", true)
	if err != nil {
		t.Errorf("runSynergyStatusLocal() returned error: %v", err)
	}
}

func TestComputeLocalSynergyHealth_AllHealthy(t *testing.T) {
	score := computeLocalSynergyHealth(true, 100, 50)
	if !almostEqual(score, 1.0) {
		t.Errorf("expected 1.0, got %f", score)
	}
}

func TestComputeLocalSynergyHealth_JiminyUnhealthy(t *testing.T) {
	score := computeLocalSynergyHealth(false, 100, 50)
	if !almostEqual(score, 0.0) {
		t.Errorf("expected 0.0, got %f", score)
	}
}

func TestComputeLocalSynergyHealth_BloatedFiles(t *testing.T) {
	// claudeLines=250 > 200 → -0.3, memoryLines=200 > 180 → -0.3
	score := computeLocalSynergyHealth(true, 250, 200)
	want := 0.4 // 1.0 - 0.3 - 0.3
	if !almostEqual(score, want) {
		t.Errorf("expected %f, got %f", want, score)
	}
}

func TestReadMigrationFlag_Missing(t *testing.T) {
	// Change to a temp directory with no .mdemg/synergy-migrated.json
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	status, date := readMigrationFlag()
	if status != "" {
		t.Errorf("expected empty status, got %q", status)
	}
	if date != "" {
		t.Errorf("expected empty date, got %q", date)
	}
}

func TestCountLines_NonExistent(t *testing.T) {
	got := countLines("/nonexistent/path/to/file.md")
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestCountAutoMemory_EmptyPath(t *testing.T) {
	files, lines := countAutoMemory("")
	if files != 0 {
		t.Errorf("expected 0 files, got %d", files)
	}
	if lines != 0 {
		t.Errorf("expected 0 lines, got %d", lines)
	}
}

func TestCountLines_RealFile(t *testing.T) {
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "test.md")
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got := countLines(p)
	if got != 3 {
		t.Errorf("expected 3, got %d", got)
	}
}
