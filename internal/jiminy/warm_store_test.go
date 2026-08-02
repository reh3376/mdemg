package jiminy

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// JIMINY-TRACKER-TTL-001 — warm-store disk persistence pins.

func TestWarmStore_InMemoryFallback(t *testing.T) {
	// Empty persistDir → memory-only, no error, all Get/Put/Age still work.
	ws := NewWarmStoreWithPersistence("")
	if ws == nil {
		t.Fatal("nil store")
	}
	ws.Put("sp1", GuidanceResponse{GuidanceID: "g1"}, "ctx", 42)
	if entry := ws.Get("sp1"); entry == nil || entry.Response.GuidanceID != "g1" {
		t.Errorf("in-memory Put/Get broken: %+v", entry)
	}
	if age := ws.Age("sp1"); age < 0 {
		t.Errorf("Age should be non-negative, got %d", age)
	}
}

func TestWarmStore_PersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	ws := NewWarmStoreWithPersistence(dir)
	ws.Put("space-A", GuidanceResponse{GuidanceID: "gA"}, "hint", 100)
	ws.Put("space-B", GuidanceResponse{GuidanceID: "gB", }, "hintB", 200)

	// Files should exist on disk (JIMINY-TRACKER-TTL-001 core contract).
	if !FileExists(filepath.Join(dir, "space-A.json")) {
		t.Error("space-A.json not persisted")
	}
	if !FileExists(filepath.Join(dir, "space-B.json")) {
		t.Error("space-B.json not persisted")
	}
}

func TestWarmStore_HydratesFromDisk(t *testing.T) {
	dir := t.TempDir()

	// Simulate a prior process: write entries to disk via one store.
	ws1 := NewWarmStoreWithPersistence(dir)
	original := GuidanceResponse{GuidanceID: "restored-g"}
	ws1.Put("mdemg-dev", original, "ctxhint", 555)

	// New process starts: fresh store must hydrate.
	ws2 := NewWarmStoreWithPersistence(dir)
	entry := ws2.Get("mdemg-dev")
	if entry == nil {
		t.Fatal("hydrate failed — entry missing after fresh store constructed")
	}
	if entry.Response.GuidanceID != "restored-g" {
		t.Errorf("guidance_id round-trip broken: %q", entry.Response.GuidanceID)
	}
	if entry.ContextHint != "ctxhint" {
		t.Errorf("context_hint round-trip broken: %q", entry.ContextHint)
	}
	if entry.ComputeMs != 555 {
		t.Errorf("compute_ms round-trip broken: %d", entry.ComputeMs)
	}
	if time.Since(entry.ComputedAt) > time.Hour {
		t.Errorf("computed_at round-trip broken: %v (should be ~now())", entry.ComputedAt)
	}
}

func TestWarmStore_InvalidateRemovesDiskFile(t *testing.T) {
	dir := t.TempDir()
	ws := NewWarmStoreWithPersistence(dir)
	ws.Put("sp", GuidanceResponse{GuidanceID: "g"}, "", 0)
	path := filepath.Join(dir, "sp.json")
	if !FileExists(path) {
		t.Fatal("pre-invalidate file missing")
	}
	ws.Invalidate("sp")
	if FileExists(path) {
		t.Error("invalidate must remove disk file")
	}
	if entry := ws.Get("sp"); entry != nil {
		t.Errorf("invalidate must remove in-memory entry, got %+v", entry)
	}
}

func TestWarmStore_SpaceIDPathSanitization(t *testing.T) {
	// space_ids can contain hostile characters — sanitizer must produce a safe file name.
	dir := t.TempDir()
	ws := NewWarmStoreWithPersistence(dir)
	// Path-traversal-ish + spaces + slashes
	ws.Put("../../etc/passwd", GuidanceResponse{GuidanceID: "evil"}, "", 0)
	// File must NOT land outside dir
	// (path-sanitized form replaces / with _)
	badPath := "/etc/passwd.json"
	if FileExists(badPath) {
		t.Error("path traversal succeeded — sanitizer broken")
	}
	// The in-memory Get still works with the original key
	if entry := ws.Get("../../etc/passwd"); entry == nil || entry.Response.GuidanceID != "evil" {
		t.Errorf("in-memory key preserved, got %+v", entry)
	}
}

func TestWarmStore_HydrateSkipsMalformed(t *testing.T) {
	dir := t.TempDir()
	// Write a garbage file — hydrate must skip it, not panic.
	garbagePath := filepath.Join(dir, "bad.json")
	if err := writeGarbage(garbagePath); err != nil {
		t.Fatalf("setup: %v", err)
	}
	ws := NewWarmStoreWithPersistence(dir)
	// Must construct successfully; no entries loaded.
	if entry := ws.Get("bad"); entry != nil {
		t.Errorf("malformed file should not populate cache, got %+v", entry)
	}
}

// writeGarbage — helper.
func writeGarbage(path string) error {
	return os.WriteFile(path, []byte("not json {"), 0o644)
}
