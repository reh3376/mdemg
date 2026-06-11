package alert

import (
	"context"
	"log/slog"
	"sync"
	"time"

	cuid2 "github.com/nrednav/cuid2"
)

// Config controls the alert dispatcher behaviour.
type Config struct {
	Enabled           bool
	CooldownSec       int
	AlertFilePath     string
	MacOSNotify       bool
	MacOSNotifyMinSev Severity
	MaxAlerts         int
}

// Backend is implemented by each alert delivery mechanism.
type Backend interface {
	Send(ctx context.Context, a Alert) error
}

// Dispatcher fans out alerts to configured backends with cooldown dedup.
type Dispatcher struct {
	mu       sync.Mutex
	backends []Backend
	cooldown *cooldownTracker
	cfg      Config
}

// NewDispatcher creates a dispatcher with the configured backends.
func NewDispatcher(cfg Config) *Dispatcher {
	if cfg.CooldownSec < 0 {
		cfg.CooldownSec = 300
	}
	if cfg.MaxAlerts <= 0 {
		cfg.MaxAlerts = 50
	}

	d := &Dispatcher{
		cooldown: newCooldownTracker(time.Duration(cfg.CooldownSec) * time.Second),
		cfg:      cfg,
	}

	if cfg.AlertFilePath != "" {
		d.backends = append(d.backends, NewFileBackend(cfg.AlertFilePath, cfg.MaxAlerts))
	}
	if cfg.MacOSNotify {
		d.backends = append(d.backends, NewMacOSBackend(cfg.MacOSNotifyMinSev))
	}

	return d
}

// ClearAlerts marks file-backend alerts as cleared (delivered to the
// operator). Pass ids and/or a non-zero before cutoff. Returns the number
// newly cleared. No-op (0, nil) when no file backend is configured.
// HOOKSYNC-001.
func (d *Dispatcher) ClearAlerts(ids []string, before time.Time) (int, error) {
	d.mu.Lock()
	backends := make([]Backend, len(d.backends))
	copy(backends, d.backends)
	d.mu.Unlock()

	for _, b := range backends {
		if fb, ok := b.(*FileBackend); ok {
			return fb.Clear(ids, before)
		}
	}
	return 0, nil
}

// Send dispatches an alert to all backends if not suppressed by cooldown.
// This method is fire-and-forget — errors are logged, never returned.
func (d *Dispatcher) Send(ctx context.Context, a Alert) {
	if !d.cfg.Enabled {
		return
	}

	// DH-004 E4.4: atomic check-and-record to close TOCTOU race that allowed
	// duplicate alerts when concurrent Send() calls raced past Allow().
	if !d.cooldown.TryRecord(a.Service, a.Severity) {
		slog.Debug("alert: suppressed by cooldown",
			"service", a.Service, "severity", a.Severity)
		return
	}

	// Fill defaults. CUIDv2 per the project identifier standard (HOOKSYNC-001;
	// was UnixNano — existing file entries keep their old ids, both are opaque
	// strings to the clear path).
	if a.ID == "" {
		a.ID = cuid2.Generate()
	}
	if a.Time.IsZero() {
		a.Time = time.Now().UTC()
	}

	slog.Info("alert: dispatching",
		"service", a.Service, "severity", a.Severity, "title", a.Title)

	d.mu.Lock()
	backends := make([]Backend, len(d.backends))
	copy(backends, d.backends)
	d.mu.Unlock()

	for _, b := range backends {
		go func(backend Backend) { //nolint:gosec // G118: intentional — alert delivery outlives caller context
			sendCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := backend.Send(sendCtx, a); err != nil {
				slog.Error("alert: backend delivery failed",
					"error", err, "service", a.Service)
			}
		}(b)
	}
}

// SendAlert is a convenience method that constructs an Alert from parameters.
func (d *Dispatcher) SendAlert(ctx context.Context, service, title, message string, sev Severity) {
	d.Send(ctx, Alert{
		Service:  service,
		Severity: sev,
		Title:    title,
		Message:  message,
	})
}
