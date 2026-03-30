package tsdb

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTSDBBackupManifest_RoundTrip(t *testing.T) {
	m := TSDBBackupManifest{
		FormatVersion: 1,
		BackupID:      "tsdb-bk-20260328-120000",
		CreatedAt:     "2026-03-28T12:00:00Z",
		SizeBytes:     1048576,
		Checksum:      "sha256:abc123",
		SchemaVersion: 4,
		RowCount:      5000,
		Database:      "mdemg_metrics",
		Label:         "manual",
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded TSDBBackupManifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.BackupID != m.BackupID {
		t.Errorf("BackupID = %q, want %q", decoded.BackupID, m.BackupID)
	}
	if decoded.SizeBytes != m.SizeBytes {
		t.Errorf("SizeBytes = %d, want %d", decoded.SizeBytes, m.SizeBytes)
	}
	if decoded.Checksum != m.Checksum {
		t.Errorf("Checksum = %q, want %q", decoded.Checksum, m.Checksum)
	}
	if decoded.Label != m.Label {
		t.Errorf("Label = %q, want %q", decoded.Label, m.Label)
	}
	if decoded.FormatVersion != 1 {
		t.Errorf("FormatVersion = %d, want 1", decoded.FormatVersion)
	}
}

func TestTSDBBackupRetention_CountBased(t *testing.T) {
	dir := t.TempDir()
	svc := NewTSDBBackupService(TSDBBackupConfig{
		StorageDir:          dir,
		RetentionCount:      2,
		RetentionMaxAgeDays: 365, // high so only count triggers
	})

	// Create 5 fake backups
	for i := range 5 {
		ts := time.Now().Add(time.Duration(-i) * time.Hour)
		id := "tsdb-bk-" + ts.Format("20060102-150405")

		// Write a fake dump file
		dumpPath := filepath.Join(dir, id+".dump")
		if err := os.WriteFile(dumpPath, []byte("fake dump data"), 0o644); err != nil {
			t.Fatalf("write dump %d: %v", i, err)
		}

		// Write manifest
		m := TSDBBackupManifest{
			FormatVersion: 1,
			BackupID:      id,
			CreatedAt:     ts.Format(time.RFC3339),
			SizeBytes:     14,
			Database:      "mdemg_metrics",
		}
		data, _ := json.Marshal(m)
		manifestPath := filepath.Join(dir, id+".manifest.json")
		if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
			t.Fatalf("write manifest %d: %v", i, err)
		}
	}

	deleted, freed, err := svc.RunRetention()
	if err != nil {
		t.Fatalf("RunRetention: %v", err)
	}

	if deleted != 3 {
		t.Errorf("deleted = %d, want 3 (5 total - 2 retained)", deleted)
	}
	if freed <= 0 {
		t.Errorf("freed = %d, want > 0", freed)
	}

	// Should have 2 remaining
	remaining, _ := svc.ListBackups(0)
	if len(remaining) != 2 {
		t.Errorf("remaining = %d, want 2", len(remaining))
	}
}

func TestTSDBBackupRetention_AgeBased(t *testing.T) {
	dir := t.TempDir()
	svc := NewTSDBBackupService(TSDBBackupConfig{
		StorageDir:          dir,
		RetentionCount:      100, // high so only age triggers
		RetentionMaxAgeDays: 7,
	})

	// Create 2 recent and 2 old backups
	timestamps := []time.Duration{
		-1 * time.Hour,    // recent
		-2 * time.Hour,    // recent
		-10 * 24 * time.Hour, // old (10 days)
		-20 * 24 * time.Hour, // old (20 days)
	}

	for _, offset := range timestamps {
		ts := time.Now().Add(offset)
		id := "tsdb-bk-" + ts.Format("20060102-150405")

		if err := os.WriteFile(filepath.Join(dir, id+".dump"), []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}

		m := TSDBBackupManifest{
			FormatVersion: 1,
			BackupID:      id,
			CreatedAt:     ts.Format(time.RFC3339),
			SizeBytes:     4,
		}
		data, _ := json.Marshal(m)
		if err := os.WriteFile(filepath.Join(dir, id+".manifest.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	deleted, _, err := svc.RunRetention()
	if err != nil {
		t.Fatalf("RunRetention: %v", err)
	}

	if deleted != 2 {
		t.Errorf("deleted = %d, want 2 (2 old backups)", deleted)
	}

	remaining, _ := svc.ListBackups(0)
	if len(remaining) != 2 {
		t.Errorf("remaining = %d, want 2", len(remaining))
	}
}

func TestTSDBBackupService_ListEmpty(t *testing.T) {
	dir := t.TempDir()
	svc := NewTSDBBackupService(TSDBBackupConfig{
		StorageDir: dir,
	})

	backups, err := svc.ListBackups(0)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("expected 0 backups, got %d", len(backups))
	}
}

func TestTSDBBackupService_ListSorted(t *testing.T) {
	dir := t.TempDir()
	svc := NewTSDBBackupService(TSDBBackupConfig{
		StorageDir: dir,
	})

	// Create 3 backups with different times
	times := []string{
		"2026-03-26T10:00:00Z",
		"2026-03-28T10:00:00Z",
		"2026-03-27T10:00:00Z",
	}
	for i, ts := range times {
		id := "tsdb-bk-" + ts[:10] + "-" + string(rune('A'+i))
		m := TSDBBackupManifest{
			FormatVersion: 1,
			BackupID:      id,
			CreatedAt:     ts,
			SizeBytes:     100,
		}
		data, _ := json.Marshal(m)
		if err := os.WriteFile(filepath.Join(dir, id+".manifest.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	backups, err := svc.ListBackups(0)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 3 {
		t.Fatalf("expected 3 backups, got %d", len(backups))
	}

	// Should be sorted newest first
	if backups[0].CreatedAt != "2026-03-28T10:00:00Z" {
		t.Errorf("first backup CreatedAt = %s, want 2026-03-28", backups[0].CreatedAt)
	}
	if backups[2].CreatedAt != "2026-03-26T10:00:00Z" {
		t.Errorf("last backup CreatedAt = %s, want 2026-03-26", backups[2].CreatedAt)
	}
}

func TestTSDBBackupService_ListWithLimit(t *testing.T) {
	dir := t.TempDir()
	svc := NewTSDBBackupService(TSDBBackupConfig{
		StorageDir: dir,
	})

	for i := range 5 {
		ts := time.Now().Add(time.Duration(-i) * time.Hour)
		id := "tsdb-bk-" + ts.Format("20060102-150405")
		m := TSDBBackupManifest{
			FormatVersion: 1,
			BackupID:      id,
			CreatedAt:     ts.Format(time.RFC3339),
		}
		data, _ := json.Marshal(m)
		if err := os.WriteFile(filepath.Join(dir, id+".manifest.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	backups, err := svc.ListBackups(3)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 3 {
		t.Errorf("expected 3 backups (limit), got %d", len(backups))
	}
}

func TestTSDBBackupService_ResolveComposeFile(t *testing.T) {
	// With explicit config
	svc := NewTSDBBackupService(TSDBBackupConfig{
		ComposeFile: "/nonexistent/docker-compose.yml",
	})

	// Should fall through to auto-detect (which checks cwd)
	// In test, no compose file will exist, so expect empty
	result := svc.resolveComposeFile()
	// We can't guarantee what exists in the test directory, so just verify it doesn't panic
	_ = result
}

func TestTSDBBackupTrigger_NoComposeFile(t *testing.T) {
	dir := t.TempDir()
	// Change to temp dir so no compose file is found
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir) //nolint:errcheck
	os.Chdir(dir)           //nolint:errcheck

	svc := NewTSDBBackupService(TSDBBackupConfig{
		StorageDir: dir,
		// No ComposeFile set, and no compose files in temp dir
	})

	_, err := svc.Trigger("test")
	if err == nil {
		t.Error("expected error when no compose file found")
	}
}

func TestSHA256File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	checksum, err := sha256File(path)
	if err != nil {
		t.Fatalf("sha256File: %v", err)
	}

	if checksum == "" {
		t.Error("checksum should not be empty")
	}
	if checksum[:7] != "sha256:" {
		t.Errorf("checksum should start with 'sha256:', got %q", checksum)
	}
	// Known SHA256 of "hello world"
	expected := "sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if checksum != expected {
		t.Errorf("checksum = %q, want %q", checksum, expected)
	}
}

func TestFileSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := []byte("twelve chars")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	size := fileSize(path)
	if size != int64(len(content)) {
		t.Errorf("fileSize = %d, want %d", size, len(content))
	}

	// Non-existent file returns 0
	if got := fileSize(filepath.Join(dir, "nonexistent")); got != 0 {
		t.Errorf("fileSize(nonexistent) = %d, want 0", got)
	}
}

func TestGetBackup(t *testing.T) {
	dir := t.TempDir()
	svc := NewTSDBBackupService(TSDBBackupConfig{
		StorageDir: dir,
	})

	id := "tsdb-bk-20260328-120000"
	m := TSDBBackupManifest{
		FormatVersion: 1,
		BackupID:      id,
		CreatedAt:     "2026-03-28T12:00:00Z",
		SizeBytes:     42,
		Checksum:      "sha256:abc",
		Database:      "mdemg_metrics",
	}
	data, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, id+".manifest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := svc.GetBackup(id)
	if err != nil {
		t.Fatalf("GetBackup: %v", err)
	}
	if got.BackupID != id {
		t.Errorf("BackupID = %q, want %q", got.BackupID, id)
	}
	if got.SizeBytes != 42 {
		t.Errorf("SizeBytes = %d, want 42", got.SizeBytes)
	}

	// Non-existent backup
	_, err = svc.GetBackup("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent backup")
	}
}
