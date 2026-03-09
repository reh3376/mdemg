package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"context"
	"testing"
	"time"
)

// --- Types ---

func TestBackupType_Constants(t *testing.T) {
	if string(BackupTypeFull) != "full" {
		t.Errorf("BackupTypeFull = %q, want %q", BackupTypeFull, "full")
	}
	if string(BackupTypePartial) != "partial_space" {
		t.Errorf("BackupTypePartial = %q, want %q", BackupTypePartial, "partial_space")
	}
}

// --- Config ---

func TestConfig_Defaults(t *testing.T) {
	cfg := Config{}
	if cfg.Enabled {
		t.Error("Enabled should default to false")
	}
	if cfg.StorageDir != "" {
		t.Error("StorageDir should default to empty")
	}
}

// --- Service (file-system tests) ---

func newTestService(t *testing.T, cfg Config) *Service {
	t.Helper()
	if cfg.StorageDir == "" {
		cfg.StorageDir = t.TempDir()
	}
	return &Service{cfg: cfg}
}

func writeTestManifest(t *testing.T, dir string, m BackupManifest) {
	t.Helper()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	path := filepath.Join(dir, m.BackupID+".manifest.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func TestService_WriteAndLoadManifest(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, Config{StorageDir: dir})

	record := &BackupRecord{BackupID: "bk-test-001"}
	manifest := BackupManifest{
		BackupID:      "bk-test-001",
		Type:          "full",
		FormatVersion: "1.0",
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		Checksum:      "sha256:abc123",
		SizeBytes:     1024,
		NodeCount:     100,
		EdgeCount:     50,
		SchemaVersion: 10,
	}

	if err := svc.writeManifest(record, manifest); err != nil {
		t.Fatalf("writeManifest: %v", err)
	}

	loaded, err := svc.loadManifest("bk-test-001")
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}

	if loaded.BackupID != "bk-test-001" {
		t.Errorf("BackupID = %q, want %q", loaded.BackupID, "bk-test-001")
	}
	if loaded.Type != "full" {
		t.Errorf("Type = %q, want %q", loaded.Type, "full")
	}
	if loaded.SizeBytes != 1024 {
		t.Errorf("SizeBytes = %d, want 1024", loaded.SizeBytes)
	}
}

func TestService_LoadManifest_NotFound(t *testing.T) {
	svc := newTestService(t, Config{})
	_, err := svc.loadManifest("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent manifest")
	}
}

func TestService_ListManifestFiles(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, Config{StorageDir: dir})

	// Write 3 manifests
	for _, id := range []string{"bk-001", "bk-002", "bk-003"} {
		writeTestManifest(t, dir, BackupManifest{
			BackupID:  id,
			Type:      "full",
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		})
	}

	manifests, err := svc.listManifestFiles()
	if err != nil {
		t.Fatalf("listManifestFiles: %v", err)
	}
	if len(manifests) != 3 {
		t.Errorf("expected 3 manifests, got %d", len(manifests))
	}
}

func TestService_ListManifestFiles_Empty(t *testing.T) {
	svc := newTestService(t, Config{})
	manifests, err := svc.listManifestFiles()
	if err != nil {
		t.Fatalf("listManifestFiles: %v", err)
	}
	if len(manifests) != 0 {
		t.Errorf("expected 0 manifests, got %d", len(manifests))
	}
}

func TestService_ListBackups(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, Config{StorageDir: dir})

	now := time.Now().UTC()
	writeTestManifest(t, dir, BackupManifest{BackupID: "bk-001", Type: "full", CreatedAt: now.Add(-3 * time.Hour).Format(time.RFC3339)})
	writeTestManifest(t, dir, BackupManifest{BackupID: "bk-002", Type: "partial_space", CreatedAt: now.Add(-2 * time.Hour).Format(time.RFC3339)})
	writeTestManifest(t, dir, BackupManifest{BackupID: "bk-003", Type: "full", CreatedAt: now.Add(-1 * time.Hour).Format(time.RFC3339)})

	// All backups
	all, err := svc.ListBackups("", 0)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3, got %d", len(all))
	}

	// Filter by type
	full, err := svc.ListBackups("full", 0)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(full) != 2 {
		t.Errorf("expected 2 full, got %d", len(full))
	}

	// Limit
	limited, err := svc.ListBackups("", 1)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(limited) != 1 {
		t.Errorf("expected 1, got %d", len(limited))
	}
	// Should be newest first
	if limited[0].BackupID != "bk-003" {
		t.Errorf("expected newest first, got %q", limited[0].BackupID)
	}
}

func TestService_DeleteBackup(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, Config{StorageDir: dir})

	// Create manifest + data file
	writeTestManifest(t, dir, BackupManifest{BackupID: "bk-del", Type: "full"})
	os.WriteFile(filepath.Join(dir, "bk-del.dump"), []byte("data"), 0644)

	err := svc.DeleteBackup("bk-del")
	if err != nil {
		t.Fatalf("DeleteBackup: %v", err)
	}

	// Verify files removed
	if _, err := os.Stat(filepath.Join(dir, "bk-del.manifest.json")); !os.IsNotExist(err) {
		t.Error("manifest should be deleted")
	}
	if _, err := os.Stat(filepath.Join(dir, "bk-del.dump")); !os.IsNotExist(err) {
		t.Error("data file should be deleted")
	}
}

func TestService_DeleteBackup_NotFound(t *testing.T) {
	svc := newTestService(t, Config{})
	err := svc.DeleteBackup("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent backup")
	}
}

func TestService_DeleteBackup_KeepForever(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, Config{StorageDir: dir})

	writeTestManifest(t, dir, BackupManifest{BackupID: "bk-keep", Type: "full", KeepForever: true})

	err := svc.DeleteBackup("bk-keep")
	if err == nil {
		t.Error("expected error for keep_forever backup")
	}
}

func TestService_DeleteBackupFiles(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, Config{StorageDir: dir})

	// Create .dump + .manifest.json
	os.WriteFile(filepath.Join(dir, "bk-001.dump"), []byte("12345"), 0644)
	writeTestManifest(t, dir, BackupManifest{BackupID: "bk-001", Type: "full"})

	freed, err := svc.deleteBackupFiles("bk-001")
	if err != nil {
		t.Fatalf("deleteBackupFiles: %v", err)
	}
	if freed <= 0 {
		t.Error("freed bytes should be > 0")
	}
}

func TestService_TotalStorageSize(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, Config{StorageDir: dir})

	// Empty dir
	if size := svc.totalStorageSize(); size != 0 {
		t.Errorf("empty dir should be 0, got %d", size)
	}

	// Add a file
	os.WriteFile(filepath.Join(dir, "test.dump"), make([]byte, 1024), 0644)
	if size := svc.totalStorageSize(); size != 1024 {
		t.Errorf("expected 1024, got %d", size)
	}
}

// --- Retention ---

func TestService_RunRetention_CountBased(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, Config{
		StorageDir:         dir,
		RetentionFullCount: 2,
	})

	now := time.Now().UTC()
	for i, id := range []string{"bk-old", "bk-mid", "bk-new"} {
		writeTestManifest(t, dir, BackupManifest{
			BackupID:  id,
			Type:      "full",
			CreatedAt: now.Add(time.Duration(i) * time.Hour).Format(time.RFC3339),
		})
		// Create dummy data file
		os.WriteFile(filepath.Join(dir, id+".dump"), []byte("data"), 0644)
	}

	result, err := svc.RunRetention(context.Background())
	if err != nil {
		t.Fatalf("RunRetention: %v", err)
	}

	if len(result.Deleted) != 1 {
		t.Errorf("expected 1 deleted, got %d: %v", len(result.Deleted), result.Deleted)
	}
	if result.KeptCount != 2 {
		t.Errorf("KeptCount = %d, want 2", result.KeptCount)
	}
}

func TestService_RunRetention_KeepForever(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, Config{
		StorageDir:         dir,
		RetentionFullCount: 1,
	})

	now := time.Now().UTC()
	writeTestManifest(t, dir, BackupManifest{BackupID: "bk-old", Type: "full", CreatedAt: now.Add(-2 * time.Hour).Format(time.RFC3339), KeepForever: true})
	os.WriteFile(filepath.Join(dir, "bk-old.dump"), []byte("data"), 0644)
	writeTestManifest(t, dir, BackupManifest{BackupID: "bk-new", Type: "full", CreatedAt: now.Format(time.RFC3339)})

	result, err := svc.RunRetention(context.Background())
	if err != nil {
		t.Fatalf("RunRetention: %v", err)
	}

	// bk-old is beyond count limit but has KeepForever — should NOT be deleted
	for _, id := range result.Deleted {
		if id == "bk-old" {
			t.Error("bk-old has KeepForever=true, should not be deleted")
		}
	}
}

func TestService_RunRetention_AgeBased(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, Config{
		StorageDir:          dir,
		RetentionMaxAgeDays: 7,
	})

	now := time.Now().UTC()
	writeTestManifest(t, dir, BackupManifest{BackupID: "bk-old", Type: "full", CreatedAt: now.AddDate(0, 0, -10).Format(time.RFC3339)})
	os.WriteFile(filepath.Join(dir, "bk-old.dump"), []byte("data"), 0644)
	writeTestManifest(t, dir, BackupManifest{BackupID: "bk-recent", Type: "full", CreatedAt: now.Format(time.RFC3339)})

	result, err := svc.RunRetention(context.Background())
	if err != nil {
		t.Fatalf("RunRetention: %v", err)
	}

	if len(result.Deleted) != 1 || result.Deleted[0] != "bk-old" {
		t.Errorf("expected bk-old deleted, got %v", result.Deleted)
	}
}

func TestService_RunRetention_Empty(t *testing.T) {
	svc := newTestService(t, Config{RetentionFullCount: 5})
	result, err := svc.RunRetention(context.Background())
	if err != nil {
		t.Fatalf("RunRetention: %v", err)
	}
	if len(result.Deleted) != 0 {
		t.Errorf("expected 0 deleted, got %d", len(result.Deleted))
	}
}

// --- Pure functions ---

func TestSha256File(t *testing.T) {
	dir := t.TempDir()
	content := []byte("hello world")
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, content, 0644)

	got, err := sha256File(path)
	if err != nil {
		t.Fatalf("sha256File: %v", err)
	}

	h := sha256.Sum256(content)
	want := "sha256:" + hex.EncodeToString(h[:])
	if got != want {
		t.Errorf("sha256File = %q, want %q", got, want)
	}
}

func TestSha256File_NotFound(t *testing.T) {
	_, err := sha256File("/nonexistent/file")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestFileSize(t *testing.T) {
	dir := t.TempDir()
	content := make([]byte, 512)
	path := filepath.Join(dir, "test.bin")
	os.WriteFile(path, content, 0644)

	size, err := fileSize(path)
	if err != nil {
		t.Fatalf("fileSize: %v", err)
	}
	if size != 512 {
		t.Errorf("fileSize = %d, want 512", size)
	}
}

func TestFileSize_NotFound(t *testing.T) {
	_, err := fileSize("/nonexistent/file")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestEnsureSpaceIncluded(t *testing.T) {
	tests := []struct {
		name     string
		spaces   []string
		required string
		wantLen  int
	}{
		{"already present", []string{"a", "mdemg-dev", "b"}, "mdemg-dev", 3},
		{"case insensitive", []string{"a", "MDEMG-DEV"}, "mdemg-dev", 2},
		{"not present", []string{"a", "b"}, "mdemg-dev", 3},
		{"empty slice", nil, "mdemg-dev", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ensureSpaceIncluded(tt.spaces, tt.required)
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

// --- Scheduler ---

func TestScheduler_StopIdempotent(t *testing.T) {
	svc := newTestService(t, Config{})
	sched := NewScheduler(svc)

	// Stop without start should not panic
	sched.Stop()
	sched.Stop() // double stop should also not panic
}
