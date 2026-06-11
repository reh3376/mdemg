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

func TestFileBackend_Clear_ByIDsAndBefore(t *testing.T) {
	dir := t.TempDir()
	b := NewFileBackend(dir+"/alerts.json", 10)
	now := time.Now().UTC()
	for i, id := range []string{"a1", "a2", "a3"} {
		if err := b.Send(context.Background(), Alert{
			ID: id, Time: now.Add(time.Duration(i-2) * time.Hour),
			Service: "svc", Severity: SeverityHigh, Title: "t", Message: "m",
		}); err != nil {
			t.Fatalf("send: %v", err)
		}
	}

	// Clear one by id.
	n, err := b.Clear([]string{"a2"}, time.Time{})
	if err != nil || n != 1 {
		t.Fatalf("clear by id: n=%d err=%v, want 1,nil", n, err)
	}
	// Idempotent re-clear.
	n, err = b.Clear([]string{"a2"}, time.Time{})
	if err != nil || n != 0 {
		t.Fatalf("re-clear: n=%d err=%v, want 0,nil", n, err)
	}
	// Unknown id ignored.
	n, _ = b.Clear([]string{"nope"}, time.Time{})
	if n != 0 {
		t.Fatalf("unknown id cleared %d, want 0", n)
	}
	// Clear by time cutoff: a1 (now-2h) and a3 (now) — cutoff now-30m clears only a1.
	n, err = b.Clear(nil, now.Add(-30*time.Minute))
	if err != nil || n != 1 {
		t.Fatalf("clear by before: n=%d err=%v, want 1,nil", n, err)
	}
	af := b.ReadAlerts()
	cleared := map[string]bool{}
	for _, a := range af.Alerts {
		cleared[a.ID] = a.Cleared
	}
	if !cleared["a1"] || !cleared["a2"] || cleared["a3"] {
		t.Fatalf("cleared state wrong: %v (want a1,a2 cleared; a3 pending)", cleared)
	}
}

func TestDispatcher_ClearAlerts_NoFileBackend(t *testing.T) {
	d := NewDispatcher(Config{Enabled: true}) // no file path → no file backend
	n, err := d.ClearAlerts([]string{"x"}, time.Time{})
	if err != nil || n != 0 {
		t.Fatalf("no-backend clear: n=%d err=%v, want 0,nil", n, err)
	}
}
