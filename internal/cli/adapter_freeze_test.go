// ADAPTER-SWAP-STANDARDIZE-001 freeze integration test.
package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestFreezeAdapterInline_CreatesBackupWhenAdaptersExists exercises the
// same freeze logic used by both the freeze subcommand and the benchmark
// atomic orchestrator.
func TestFreezeAdapterInline_CreatesBackupWhenAdaptersExists(t *testing.T) {
	dir := t.TempDir()
	// 2 checkpoints
	sha400 := writeFile(t, filepath.Join(dir, "0000400_adapters.safetensors"), 100)
	sha1200 := writeFile(t, filepath.Join(dir, "0001200_adapters.safetensors"), 200)
	// Pre-existing adapters.safetensors (simulate a prior freeze at iter 400)
	prevSHA := writeFile(t, filepath.Join(dir, adapterMainFile), 100)
	if prevSHA != sha400 {
		t.Fatalf("test setup: main SHA %s should equal 400 SHA %s", prevSHA, sha400)
	}

	// Freeze iter 1200
	if err := freezeAdapterInline(dir, 1200); err != nil {
		t.Fatalf("freezeAdapterInline: %v", err)
	}

	// Verify:
	mainSHA, err := sha256File(filepath.Join(dir, adapterMainFile))
	if err != nil {
		t.Fatalf("post-freeze hash main: %v", err)
	}
	if mainSHA != sha1200 {
		t.Errorf("adapters.safetensors SHA = %s; want %s (iter 1200)", mainSHA, sha1200)
	}

	backupSHA, err := sha256File(filepath.Join(dir, adapterBackupFile))
	if err != nil {
		t.Fatalf("post-freeze hash backup: %v", err)
	}
	if backupSHA != prevSHA {
		t.Errorf("backup SHA = %s; want %s (previous)", backupSHA, prevSHA)
	}

	// Audit row present
	auditBytes, err := os.ReadFile(filepath.Join(dir, freezeAuditFile))
	if err != nil {
		t.Fatalf("read freeze_log: %v", err)
	}
	var row freezeAuditRow
	if err := json.Unmarshal(auditBytes[:len(auditBytes)-1], &row); err != nil {
		t.Fatalf("parse audit row: %v", err)
	}
	if row.Iter != 1200 || row.CheckpointSH != sha1200 {
		t.Errorf("audit row wrong: %+v", row)
	}
	if row.BackupPath != filepath.Join(dir, adapterBackupFile) {
		t.Errorf("audit row backup path wrong: %s", row.BackupPath)
	}
}

func TestFreezeAdapterInline_NoBackupWhenNoPriorAdapters(t *testing.T) {
	dir := t.TempDir()
	sha800 := writeFile(t, filepath.Join(dir, "0000800_adapters.safetensors"), 50)

	// No pre-existing adapters.safetensors
	if err := freezeAdapterInline(dir, 800); err != nil {
		t.Fatalf("freezeAdapterInline: %v", err)
	}

	// Backup file should NOT exist
	if _, err := os.Stat(filepath.Join(dir, adapterBackupFile)); !os.IsNotExist(err) {
		t.Errorf("backup file should not exist when no prior adapters.safetensors: err=%v", err)
	}

	mainSHA, _ := sha256File(filepath.Join(dir, adapterMainFile))
	if mainSHA != sha800 {
		t.Errorf("adapters.safetensors SHA mismatch")
	}

	// Audit row present, no backup fields
	auditBytes, err := os.ReadFile(filepath.Join(dir, freezeAuditFile))
	if err != nil {
		t.Fatalf("read freeze_log: %v", err)
	}
	var row freezeAuditRow
	if err := json.Unmarshal(auditBytes[:len(auditBytes)-1], &row); err != nil {
		t.Fatalf("parse audit row: %v", err)
	}
	if row.BackupPath != "" {
		t.Errorf("audit row should have no backup_path, got %s", row.BackupPath)
	}
}

func TestFreezeAdapterInline_ErrorOnMissingIter(t *testing.T) {
	dir := t.TempDir()
	_ = writeFile(t, filepath.Join(dir, "0000400_adapters.safetensors"), 10)

	if err := freezeAdapterInline(dir, 9999); err == nil {
		t.Error("expected error for missing iter 9999")
	}
}

func TestSummarizeBenchmarkJSON(t *testing.T) {
	dir := t.TempDir()
	sample := map[string]any{
		"aggregate_weighted_score": 0.7658,
		"run_id":                   "bmxxx",
		"per_task": map[string]any{
			"consulting.classify": map[string]any{
				"mean": 0.65,
				"n":    5.0,
			},
			"claude.code_knowledge": map[string]any{
				"mean": 0.25,
				"n":    100.0,
			},
		},
	}
	data, _ := json.Marshal(sample)
	path := filepath.Join(dir, "bench.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	sum, err := summarizeBenchmarkJSON(path)
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if sum["aggregate_weighted_score"] != 0.7658 {
		t.Errorf("aggregate wrong: %v", sum["aggregate_weighted_score"])
	}
	if sum["run_id"] != "bmxxx" {
		t.Errorf("run_id wrong")
	}
	means, ok := sum["per_task_mean"].(map[string]any)
	if !ok {
		t.Fatalf("per_task_mean not map: %T", sum["per_task_mean"])
	}
	if means["consulting.classify"] != 0.65 {
		t.Errorf("consulting.classify mean wrong: %v", means["consulting.classify"])
	}
}
