package sidecar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndReadLock_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mdemg", "sidecar.lock")

	lock := NewLockFile("local", "http://localhost:9999", "abc123")
	if err := WriteLock(path, lock); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}

	got, err := ReadLock(path)
	if err != nil {
		t.Fatalf("ReadLock: %v", err)
	}
	if got.State != StateInitialized {
		t.Errorf("state = %q, want %q", got.State, StateInitialized)
	}
	if got.Profile != "local" {
		t.Errorf("profile = %q, want %q", got.Profile, "local")
	}
	if got.Endpoint != "http://localhost:9999" {
		t.Errorf("endpoint = %q, want %q", got.Endpoint, "http://localhost:9999")
	}
	if got.ConfigHash != "abc123" {
		t.Errorf("config_hash = %q, want %q", got.ConfigHash, "abc123")
	}
	if got.CreatedAt == "" {
		t.Error("created_at should not be empty")
	}
	if got.UpdatedAt == "" {
		t.Error("updated_at should not be empty")
	}
}

func TestReadLock_MissingFile(t *testing.T) {
	_, err := ReadLock("/nonexistent/sidecar.lock")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadLock_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sidecar.lock")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := ReadLock(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestWriteLock_CreatesDirIfNeeded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "nested", "sidecar.lock")

	lock := NewLockFile("local", "http://localhost:9999", "hash")
	if err := WriteLock(path, lock); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("lock file was not created")
	}
}

func TestWriteLock_ValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sidecar.lock")

	lock := NewLockFile("local", "http://localhost:9999", "hash")
	if err := WriteLock(path, lock); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) {
		t.Error("written lock file is not valid JSON")
	}
}

func TestCurrentStateFrom_NoLock(t *testing.T) {
	dir := t.TempDir()
	state := CurrentStateFrom(dir)
	if state != StateUninitialized {
		t.Errorf("state = %q, want %q", state, StateUninitialized)
	}
}

func TestCurrentStateFrom_WithLock(t *testing.T) {
	dir := t.TempDir()
	mdemg := filepath.Join(dir, ".mdemg")
	if err := os.MkdirAll(mdemg, 0755); err != nil {
		t.Fatal(err)
	}

	lock := NewLockFile("local", "http://localhost:9999", "hash")
	lock.State = StateInstalled
	path := filepath.Join(mdemg, "sidecar.lock")
	if err := WriteLock(path, lock); err != nil {
		t.Fatal(err)
	}

	state := CurrentStateFrom(dir)
	if state != StateInstalled {
		t.Errorf("state = %q, want %q", state, StateInstalled)
	}
}
