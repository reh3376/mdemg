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

// --- Lock file with new port fields ---

func TestLockFileJSON_NewFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sidecar.lock")

	lock := NewLockFile("local", "http://localhost:10001", "hash")
	lock.State = StateRunning
	lock.Neo4jBoltPort = 7700
	lock.Neo4jHTTPPort = 7500
	lock.ContainerName = "mdemg-neo4j-myproject"
	lock.VolumeName = "mdemg-neo4j-data-myproject"

	if err := WriteLock(path, lock); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}

	got, err := ReadLock(path)
	if err != nil {
		t.Fatalf("ReadLock: %v", err)
	}
	if got.Neo4jBoltPort != 7700 {
		t.Errorf("neo4j_bolt_port = %d, want 7700", got.Neo4jBoltPort)
	}
	if got.Neo4jHTTPPort != 7500 {
		t.Errorf("neo4j_http_port = %d, want 7500", got.Neo4jHTTPPort)
	}
	if got.ContainerName != "mdemg-neo4j-myproject" {
		t.Errorf("container_name = %q, want %q", got.ContainerName, "mdemg-neo4j-myproject")
	}
	if got.VolumeName != "mdemg-neo4j-data-myproject" {
		t.Errorf("volume_name = %q, want %q", got.VolumeName, "mdemg-neo4j-data-myproject")
	}
}

func TestLockFileJSON_BackwardCompat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sidecar.lock")

	// Write a lock file WITHOUT the new fields (simulating old format)
	oldJSON := `{"state":"running","profile":"local","endpoint":"http://localhost:9999","config_hash":"abc","created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z"}`
	if err := os.WriteFile(path, []byte(oldJSON), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadLock(path)
	if err != nil {
		t.Fatalf("ReadLock: %v", err)
	}
	// New fields should be zero values
	if got.Neo4jBoltPort != 0 {
		t.Errorf("neo4j_bolt_port = %d, want 0", got.Neo4jBoltPort)
	}
	if got.Neo4jHTTPPort != 0 {
		t.Errorf("neo4j_http_port = %d, want 0", got.Neo4jHTTPPort)
	}
	if got.ContainerName != "" {
		t.Errorf("container_name = %q, want empty", got.ContainerName)
	}
	if got.VolumeName != "" {
		t.Errorf("volume_name = %q, want empty", got.VolumeName)
	}
	// Existing fields should still work
	if got.State != StateRunning {
		t.Errorf("state = %q, want running", got.State)
	}
}

// --- Resolver tests ---

func TestResolveRuntimeEndpoint_LockPresent(t *testing.T) {
	dir := t.TempDir()
	mdemg := filepath.Join(dir, ".mdemg")
	if err := os.MkdirAll(mdemg, 0755); err != nil {
		t.Fatal(err)
	}

	lock := NewLockFile("local", "http://localhost:10001", "hash")
	lock.State = StateRunning
	if err := WriteLock(filepath.Join(mdemg, "sidecar.lock"), lock); err != nil {
		t.Fatal(err)
	}

	got := ResolveRuntimeEndpoint(dir, "http://localhost:9999")
	if got != "http://localhost:10001" {
		t.Errorf("ResolveRuntimeEndpoint = %q, want lock endpoint", got)
	}
}

func TestResolveRuntimeEndpoint_LockAbsent(t *testing.T) {
	dir := t.TempDir()
	got := ResolveRuntimeEndpoint(dir, "http://localhost:9999")
	if got != "http://localhost:9999" {
		t.Errorf("ResolveRuntimeEndpoint = %q, want config endpoint", got)
	}
}

func TestResolveRuntimeEndpoint_NotRunning(t *testing.T) {
	dir := t.TempDir()
	mdemg := filepath.Join(dir, ".mdemg")
	if err := os.MkdirAll(mdemg, 0755); err != nil {
		t.Fatal(err)
	}

	lock := NewLockFile("local", "http://localhost:10001", "hash")
	lock.State = StateStopped
	if err := WriteLock(filepath.Join(mdemg, "sidecar.lock"), lock); err != nil {
		t.Fatal(err)
	}

	got := ResolveRuntimeEndpoint(dir, "http://localhost:9999")
	if got != "http://localhost:9999" {
		t.Errorf("ResolveRuntimeEndpoint = %q, want config endpoint when stopped", got)
	}
}

func TestResolveRuntimeNeo4jBoltPort_WithPort(t *testing.T) {
	dir := t.TempDir()
	mdemg := filepath.Join(dir, ".mdemg")
	if err := os.MkdirAll(mdemg, 0755); err != nil {
		t.Fatal(err)
	}

	lock := NewLockFile("local", "http://localhost:9999", "hash")
	lock.Neo4jBoltPort = 7700
	if err := WriteLock(filepath.Join(mdemg, "sidecar.lock"), lock); err != nil {
		t.Fatal(err)
	}

	got := ResolveRuntimeNeo4jBoltPort(dir, 7687)
	if got != 7700 {
		t.Errorf("ResolveRuntimeNeo4jBoltPort = %d, want 7700", got)
	}
}

func TestResolveRuntimeNeo4jBoltPort_ZeroFallback(t *testing.T) {
	dir := t.TempDir()
	mdemg := filepath.Join(dir, ".mdemg")
	if err := os.MkdirAll(mdemg, 0755); err != nil {
		t.Fatal(err)
	}

	lock := NewLockFile("local", "http://localhost:9999", "hash")
	// Neo4jBoltPort is zero (not set)
	if err := WriteLock(filepath.Join(mdemg, "sidecar.lock"), lock); err != nil {
		t.Fatal(err)
	}

	got := ResolveRuntimeNeo4jBoltPort(dir, 7687)
	if got != 7687 {
		t.Errorf("ResolveRuntimeNeo4jBoltPort = %d, want default 7687", got)
	}
}

func TestResolveContainerName_FromLock(t *testing.T) {
	dir := t.TempDir()
	mdemg := filepath.Join(dir, ".mdemg")
	if err := os.MkdirAll(mdemg, 0755); err != nil {
		t.Fatal(err)
	}

	lock := NewLockFile("local", "http://localhost:9999", "hash")
	lock.ContainerName = "mdemg-neo4j-myproject"
	if err := WriteLock(filepath.Join(mdemg, "sidecar.lock"), lock); err != nil {
		t.Fatal(err)
	}

	got := ResolveContainerName(dir, "mdemg-neo4j-dev")
	if got != "mdemg-neo4j-myproject" {
		t.Errorf("ResolveContainerName = %q, want lock container name", got)
	}
}

func TestResolveContainerName_ZeroFallback(t *testing.T) {
	dir := t.TempDir()
	got := ResolveContainerName(dir, "mdemg-neo4j-dev")
	if got != "mdemg-neo4j-dev" {
		t.Errorf("ResolveContainerName = %q, want default", got)
	}
}
