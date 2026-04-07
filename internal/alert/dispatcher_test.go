package alert

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type mockBackend struct {
	calls atomic.Int32
}

func (m *mockBackend) Send(_ context.Context, _ Alert) error {
	m.calls.Add(1)
	return nil
}

func TestDispatcher_SendDisabled(t *testing.T) {
	d := NewDispatcher(Config{
		Enabled:       false,
		AlertFilePath: filepath.Join(t.TempDir(), "alerts.json"),
	})

	mb := &mockBackend{}
	d.mu.Lock()
	d.backends = append(d.backends, mb)
	d.mu.Unlock()

	d.Send(context.Background(), Alert{Service: "test", Severity: SeverityHigh})

	time.Sleep(50 * time.Millisecond)
	if mb.calls.Load() != 0 {
		t.Error("disabled dispatcher should not call backends")
	}
}

func TestDispatcher_SendEnabled(t *testing.T) {
	d := NewDispatcher(Config{
		Enabled:     true,
		CooldownSec: 1,
		MaxAlerts:   50,
	})

	mb := &mockBackend{}
	d.mu.Lock()
	d.backends = []Backend{mb}
	d.mu.Unlock()

	d.Send(context.Background(), Alert{Service: "test", Severity: SeverityHigh, Title: "t"})

	time.Sleep(100 * time.Millisecond)
	if mb.calls.Load() != 1 {
		t.Errorf("expected 1 backend call, got %d", mb.calls.Load())
	}
}

func TestDispatcher_CooldownSuppresses(t *testing.T) {
	d := NewDispatcher(Config{
		Enabled:     true,
		CooldownSec: 60,
		MaxAlerts:   50,
	})

	mb := &mockBackend{}
	d.mu.Lock()
	d.backends = []Backend{mb}
	d.mu.Unlock()

	a := Alert{Service: "neo4j", Severity: SeverityHigh, Title: "down"}

	d.Send(context.Background(), a)
	d.Send(context.Background(), a) // Should be suppressed.

	time.Sleep(100 * time.Millisecond)
	if mb.calls.Load() != 1 {
		t.Errorf("expected 1 call (second suppressed), got %d", mb.calls.Load())
	}
}

func TestDispatcher_MultipleBackends(t *testing.T) {
	d := NewDispatcher(Config{
		Enabled:     true,
		CooldownSec: 1,
		MaxAlerts:   50,
	})

	mb1 := &mockBackend{}
	mb2 := &mockBackend{}
	d.mu.Lock()
	d.backends = []Backend{mb1, mb2}
	d.mu.Unlock()

	d.Send(context.Background(), Alert{Service: "test", Severity: SeverityLow, Title: "t"})

	time.Sleep(100 * time.Millisecond)
	if mb1.calls.Load() != 1 || mb2.calls.Load() != 1 {
		t.Errorf("expected both backends called: mb1=%d mb2=%d", mb1.calls.Load(), mb2.calls.Load())
	}
}

func TestDispatcher_FillsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "current.json")

	d := NewDispatcher(Config{
		Enabled:       true,
		CooldownSec:   1,
		AlertFilePath: path,
		MaxAlerts:     50,
	})

	d.Send(context.Background(), Alert{Service: "test", Severity: SeverityLow, Title: "t", Message: "m"})

	time.Sleep(100 * time.Millisecond)

	fb := NewFileBackend(path, 50)
	af := fb.ReadAlerts()
	if len(af.Alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(af.Alerts))
	}
	if af.Alerts[0].ID == "" {
		t.Error("expected auto-generated ID")
	}
	if af.Alerts[0].Time.IsZero() {
		t.Error("expected auto-filled time")
	}
}

func TestSendAlert_Convenience(t *testing.T) {
	d := NewDispatcher(Config{
		Enabled:     true,
		CooldownSec: 1,
		MaxAlerts:   50,
	})

	mb := &mockBackend{}
	d.mu.Lock()
	d.backends = []Backend{mb}
	d.mu.Unlock()

	d.SendAlert(context.Background(), "neo4j", "Neo4j Down", "Connection refused", SeverityCritical)

	time.Sleep(100 * time.Millisecond)
	if mb.calls.Load() != 1 {
		t.Errorf("expected 1 call, got %d", mb.calls.Load())
	}
}
