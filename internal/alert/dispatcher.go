package alert

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
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
	if cfg.CooldownSec <= 0 {
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

// Send dispatches an alert to all backends if not suppressed by cooldown.
// This method is fire-and-forget — errors are logged, never returned.
func (d *Dispatcher) Send(ctx context.Context, a Alert) {
	if !d.cfg.Enabled {
		return
	}

	if !d.cooldown.Allow(a.Service, a.Severity) {
		slog.Debug("alert: suppressed by cooldown",
			"service", a.Service, "severity", a.Severity)
		return
	}
	d.cooldown.Record(a.Service, a.Severity)

	// Fill defaults.
	if a.ID == "" {
		a.ID = fmt.Sprintf("%d", time.Now().UnixNano())
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
