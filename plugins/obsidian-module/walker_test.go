package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// helper creates a file with the given content inside dir.
func createFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWalk_BasicVault(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "note1.md", "# Note 1")
	createFile(t, root, "note2.md", "# Note 2")
	createFile(t, root, "note3.md", "# Note 3")

	w := NewVaultWalker(root, nil)
	files, err := w.Walk()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}
}

func TestWalk_SkipDotObsidian(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "note.md", "# Visible")
	createFile(t, root, ".obsidian/workspace.md", "hidden")
	createFile(t, root, ".obsidian/plugins/core.md", "hidden")

	w := NewVaultWalker(root, nil)
	files, err := w.Walk()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].RelPath != "note.md" {
		t.Fatalf("expected note.md, got %s", files[0].RelPath)
	}
}

func TestWalk_SkipConfiguredExcludeDirs(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "note.md", "# Visible")
	createFile(t, root, "Templates/template1.md", "template")
	createFile(t, root, "Templates/daily.md", "daily template")

	w := NewVaultWalker(root, []string{"Templates"})
	files, err := w.Walk()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].RelPath != "note.md" {
		t.Fatalf("expected note.md, got %s", files[0].RelPath)
	}
}

func TestWalk_OnlyMarkdown(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "note.md", "# Markdown")
	createFile(t, root, "readme.txt", "plain text")
	createFile(t, root, "image.png", "fake image data")
	createFile(t, root, "data.json", "{}")

	w := NewVaultWalker(root, nil)
	files, err := w.Walk()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 markdown file, got %d", len(files))
	}
	if files[0].RelPath != "note.md" {
		t.Fatalf("expected note.md, got %s", files[0].RelPath)
	}
}

func TestWalk_FilterModifiedSince(t *testing.T) {
	root := t.TempDir()

	// Create all files first.
	createFile(t, root, "old.md", "old content")
	createFile(t, root, "new1.md", "new content 1")
	createFile(t, root, "new2.md", "new content 2")

	// Set "old.md" mtime to 24 hours ago.
	past := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, "old.md"), past, past); err != nil {
		t.Fatal(err)
	}

	// The cutoff is 1 hour ago — old.md should be excluded.
	cutoff := time.Now().Add(-1 * time.Hour)

	w := NewVaultWalker(root, nil)
	files, err := w.Walk()
	if err != nil {
		t.Fatal(err)
	}

	filtered := filterModifiedSince(files, cutoff)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 recent files, got %d", len(filtered))
	}
	for _, f := range filtered {
		if f.RelPath == "old.md" {
			t.Fatal("old.md should have been filtered out")
		}
	}
}

func TestWalk_EmptyVault(t *testing.T) {
	root := t.TempDir()

	w := NewVaultWalker(root, nil)
	files, err := w.Walk()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}

func TestWalk_NestedDirs(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "subfolder/deep/note.md", "# Deep Note")

	w := NewVaultWalker(root, nil)
	files, err := w.Walk()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	expected := filepath.Join("subfolder", "deep", "note.md")
	if files[0].RelPath != expected {
		t.Fatalf("expected RelPath %q, got %q", expected, files[0].RelPath)
	}
}

func TestNewVaultWalker_DefaultExcludes(t *testing.T) {
	w := NewVaultWalker("/fake/path", nil)

	if !w.ExcludeDirs[".obsidian"] {
		t.Fatal(".obsidian should be in default excludes")
	}
	if !w.ExcludeDirs[".trash"] {
		t.Fatal(".trash should be in default excludes")
	}

	// Also verify with custom excludes — defaults must still be present.
	w2 := NewVaultWalker("/fake/path", []string{"Archive"})
	if !w2.ExcludeDirs[".obsidian"] {
		t.Fatal(".obsidian should be in excludes when custom dirs are provided")
	}
	if !w2.ExcludeDirs[".trash"] {
		t.Fatal(".trash should be in excludes when custom dirs are provided")
	}
	if !w2.ExcludeDirs["Archive"] {
		t.Fatal("Archive should be in excludes")
	}
}
