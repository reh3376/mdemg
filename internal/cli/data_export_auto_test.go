package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneExports_KeepsMostRecent(t *testing.T) {
	dir := t.TempDir()

	// Create 5 fake archives with staggered mod times
	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, "mdemg-export-test-"+time.Now().Add(time.Duration(-i)*time.Hour).Format("20060102-150405")+".tar.gz")
		if err := os.WriteFile(name, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
		// Set mod time to ensure ordering
		modTime := time.Now().Add(time.Duration(-i) * time.Hour)
		os.Chtimes(name, modTime, modTime)
	}

	pruned, err := pruneExports(dir, 3)
	if err != nil {
		t.Fatalf("pruneExports: %v", err)
	}
	if pruned != 2 {
		t.Errorf("expected 2 pruned, got %d", pruned)
	}

	remaining, _ := findExportArchives(dir)
	if len(remaining) != 3 {
		t.Errorf("expected 3 remaining, got %d", len(remaining))
	}
}

func TestPruneExports_NoPruneUnderLimit(t *testing.T) {
	dir := t.TempDir()

	// Create 2 archives
	for i := 0; i < 2; i++ {
		name := filepath.Join(dir, "mdemg-export-test-"+time.Now().Add(time.Duration(-i)*time.Hour).Format("20060102-150405")+".tar.gz")
		os.WriteFile(name, []byte("test"), 0644)
	}

	pruned, err := pruneExports(dir, 5)
	if err != nil {
		t.Fatalf("pruneExports: %v", err)
	}
	if pruned != 0 {
		t.Errorf("expected 0 pruned, got %d", pruned)
	}
}

func TestPruneExports_IgnoresNonArchives(t *testing.T) {
	dir := t.TempDir()

	// Create archives + non-matching files
	for i := 0; i < 3; i++ {
		name := filepath.Join(dir, "mdemg-export-test-"+time.Now().Add(time.Duration(-i)*time.Hour).Format("20060102-150405")+".tar.gz")
		os.WriteFile(name, []byte("test"), 0644)
		modTime := time.Now().Add(time.Duration(-i) * time.Hour)
		os.Chtimes(name, modTime, modTime)
	}
	os.WriteFile(filepath.Join(dir, "other-file.txt"), []byte("keep"), 0644)
	os.WriteFile(filepath.Join(dir, "latest.tar.gz"), []byte("symlink-target"), 0644)

	pruned, err := pruneExports(dir, 2)
	if err != nil {
		t.Fatalf("pruneExports: %v", err)
	}
	if pruned != 1 {
		t.Errorf("expected 1 pruned, got %d", pruned)
	}

	// Non-matching files should still exist
	if _, err := os.Stat(filepath.Join(dir, "other-file.txt")); err != nil {
		t.Error("other-file.txt was incorrectly removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "latest.tar.gz")); err != nil {
		t.Error("latest.tar.gz was incorrectly removed")
	}
}

func TestUpdateLatestSymlink(t *testing.T) {
	dir := t.TempDir()

	// Create a fake archive
	archivePath := filepath.Join(dir, "mdemg-export-test-20260402-120000.tar.gz")
	os.WriteFile(archivePath, []byte("archive"), 0644)

	err := updateLatestSymlink(dir, archivePath)
	if err != nil {
		t.Fatalf("updateLatestSymlink: %v", err)
	}

	linkPath := filepath.Join(dir, "latest.tar.gz")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != archivePath {
		t.Errorf("expected symlink target %s, got %s", archivePath, target)
	}

	// Update symlink to new archive
	archivePath2 := filepath.Join(dir, "mdemg-export-test-20260402-130000.tar.gz")
	os.WriteFile(archivePath2, []byte("archive2"), 0644)

	err = updateLatestSymlink(dir, archivePath2)
	if err != nil {
		t.Fatalf("updateLatestSymlink (update): %v", err)
	}

	target2, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink (update): %v", err)
	}
	if target2 != archivePath2 {
		t.Errorf("expected symlink target %s, got %s", archivePath2, target2)
	}
}

func TestFindExportArchives(t *testing.T) {
	dir := t.TempDir()

	// Create matching and non-matching files
	matching := []string{
		"mdemg-export-user-dev-20260402-120000.tar.gz",
		"mdemg-export-user-dev-20260401-120000.tar.gz",
	}
	nonMatching := []string{
		"other-file.tar.gz",
		"mdemg-export-.json",
		"latest.tar.gz",
		"readme.md",
	}

	for _, name := range append(matching, nonMatching...) {
		os.WriteFile(filepath.Join(dir, name), []byte("test"), 0644)
	}
	os.Mkdir(filepath.Join(dir, "mdemg-export-dir.tar.gz"), 0755)

	archives, err := findExportArchives(dir)
	if err != nil {
		t.Fatalf("findExportArchives: %v", err)
	}
	if len(archives) != 2 {
		t.Errorf("expected 2 archives, got %d", len(archives))
	}
}
