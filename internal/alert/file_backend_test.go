package alert

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileBackend_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alerts", "current.json")

	fb := NewFileBackend(path, 50)

	a := Alert{
		ID:       "test-1",
		Time:     time.Now().UTC(),
		Service:  "neo4j",
		Severity: SeverityHigh,
		Title:    "Neo4j Down",
		Message:  "Connection refused",
	}

	if err := fb.Send(context.Background(), a); err != nil {
		t.Fatalf("Send: %v", err)
	}

	af := fb.ReadAlerts()
	if len(af.Alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(af.Alerts))
	}
	if af.Alerts[0].Service != "neo4j" {
		t.Errorf("service = %q, want neo4j", af.Alerts[0].Service)
	}
	if af.Alerts[0].Title != "Neo4j Down" {
		t.Errorf("title = %q, want Neo4j Down", af.Alerts[0].Title)
	}
}

func TestFileBackend_FIFOEviction(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "current.json")

	fb := NewFileBackend(path, 3)

	for i := range 5 {
		a := Alert{
			ID:       string(rune('A' + i)),
			Service:  "svc",
			Severity: SeverityLow,
			Title:    string(rune('A' + i)),
		}
		if err := fb.Send(context.Background(), a); err != nil {
			t.Fatalf("Send %d: %v", i, err)
		}
	}

	af := fb.ReadAlerts()
	if len(af.Alerts) != 3 {
		t.Fatalf("expected 3 alerts (cap), got %d", len(af.Alerts))
	}

	// Newest first — last written should be first.
	if af.Alerts[0].ID != "E" {
		t.Errorf("first alert ID = %q, want E", af.Alerts[0].ID)
	}
}

func TestFileBackend_MalformedFileRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "current.json")

	// Write garbage.
	if err := os.WriteFile(path, []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}

	fb := NewFileBackend(path, 50)

	// Should recover gracefully and start fresh.
	a := Alert{ID: "1", Service: "test", Severity: SeverityLow, Title: "test"}
	if err := fb.Send(context.Background(), a); err != nil {
		t.Fatalf("Send after malformed: %v", err)
	}

	af := fb.ReadAlerts()
	if len(af.Alerts) != 1 {
		t.Fatalf("expected 1 alert after recovery, got %d", len(af.Alerts))
	}
}

func TestFileBackend_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "current.json")

	fb := NewFileBackend(path, 50)

	a := Alert{ID: "1", Service: "test", Severity: SeverityLow, Title: "test"}
	if err := fb.Send(context.Background(), a); err != nil {
		t.Fatal(err)
	}

	// Verify no .tmp file remains.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("tmp file should not exist after successful write")
	}

	// Verify main file exists.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("alert file should exist: %v", err)
	}
}
