package ape

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"mdemg/internal/config"
	"mdemg/internal/metrics"
)

// Watchdog monitors the time since the last RSIC cycle and escalates if overdue.
type Watchdog struct {
	cfg            config.Config
	spaceID        string
	signalProvider WatchdogSignalProvider
	store          *RSICStore

	mu            sync.RWMutex
	state         WatchdogState
	cycleTrigger  func(ctx context.Context, spaceID string, meta TriggerMetadata) // callback to auto-trigger meso cycle

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewWatchdog creates a Watchdog. cycleTrigger is called at EscalationForce level.
func NewWatchdog(cfg config.Config, spaceID string, cycleTrigger func(ctx context.Context, spaceID string, meta TriggerMetadata)) *Watchdog {
	ctx, cancel := context.WithCancel(context.Background())
	return &Watchdog{
		cfg:          cfg,
		spaceID:      spaceID,
		cycleTrigger: cycleTrigger,
		ctx:          ctx,
		cancel:       cancel,
		state: WatchdogState{
			SpaceID:       spaceID,
			LastCycleTime: time.Now(),
		},
	}
}

// Start begins the watchdog ticker loop.
func (w *Watchdog) Start() {
	if !w.cfg.RSICWatchdogEnabled {
		slog.Info("RSIC watchdog disabled")
		return
	}

	interval := time.Duration(w.cfg.RSICWatchdogCheckSec) * time.Second
	if interval < time.Second {
		interval = 5 * time.Minute
	}

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		slog.Info("RSIC watchdog started", "check_interval", interval, "decay_rate_per_hr", w.cfg.RSICWatchdogDecayRate)

		for {
			select {
			case <-w.ctx.Done():
				return
			case <-ticker.C:
				w.check()
			}
		}
	}()
}

// Stop gracefully stops the watchdog.
func (w *Watchdog) Stop() {
	w.cancel()
	w.wg.Wait()
}

// RecordCycle resets the watchdog after a successful cycle.
func (w *Watchdog) RecordCycle() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.state.LastCycleTime = time.Now()
	w.state.DecayScore = 0
	w.state.EscalationLevel = EscalationNominal

	// Phase 89: Persist watchdog state
	if w.store != nil {
		w.store.SaveWatchdogState(w.spaceID, w.state)
	}
}

// GetState returns the current watchdog state.
func (w *Watchdog) GetState() WatchdogState {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.state
}

// SetStore attaches a persistence store to the watchdog.
func (w *Watchdog) SetStore(s *RSICStore) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.store = s
}

// Hydrate overwrites the in-memory watchdog state with persisted values.
func (w *Watchdog) Hydrate(state *WatchdogState) {
	if state == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.state = *state
	// Preserve the spaceID from constructor
	w.state.SpaceID = w.spaceID
}

// SetSignalProvider attaches a WatchdogSignalProvider for multi-dimensional monitoring.
func (w *Watchdog) SetSignalProvider(sp WatchdogSignalProvider) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.signalProvider = sp
}

func (w *Watchdog) check() {
	w.mu.Lock()
	defer w.mu.Unlock()

	hoursSinceCycle := time.Since(w.state.LastCycleTime).Hours()
	w.state.DecayScore = hoursSinceCycle * w.cfg.RSICWatchdogDecayRate
	w.state.NextDue = w.state.LastCycleTime.Add(time.Duration(w.cfg.RSICMesoPeriodHours) * time.Hour)

	metrics.Metrics().RSICWatchdogDecay(w.spaceID).Set(w.state.DecayScore)

	prevLevel := w.state.EscalationLevel

	switch {
	case w.state.DecayScore >= w.cfg.RSICForceThreshold:
		w.state.EscalationLevel = EscalationForce
	case w.state.DecayScore >= w.cfg.RSICWarnThreshold:
		w.state.EscalationLevel = EscalationWarn
	case w.state.DecayScore >= w.cfg.RSICNudgeThreshold:
		w.state.EscalationLevel = EscalationNudge
	default:
		w.state.EscalationLevel = EscalationNominal
	}

	metrics.Metrics().RSICWatchdogEscalation(w.spaceID).Set(float64(w.state.EscalationLevel))

	// Log on escalation level changes
	if w.state.EscalationLevel != prevLevel {
		switch w.state.EscalationLevel {
		case EscalationNudge:
			slog.Info("RSIC watchdog: nudge", "decay_score", w.state.DecayScore, "hours_since_cycle", hoursSinceCycle)
		case EscalationWarn:
			slog.Warn("RSIC watchdog: warning", "decay_score", w.state.DecayScore, "hours_since_cycle", hoursSinceCycle)
		case EscalationForce:
			slog.Warn("RSIC watchdog: force, auto-triggering meso cycle", "decay_score", w.state.DecayScore)
		}
	}

	// Auto-trigger at force level
	if w.state.EscalationLevel == EscalationForce && w.cycleTrigger != nil {
		metrics.Metrics().RSICWatchdogForce.Inc()
		// Reset before triggering to avoid re-triggering
		now := time.Now()
		w.state.LastCycleTime = now
		w.state.DecayScore = 0
		w.state.EscalationLevel = EscalationNominal
		w.state.LastTriggerSource = TriggerWatchdogForce

		meta := TriggerMetadata{
			TriggerSource: TriggerWatchdogForce,
			TriggerID:     fmt.Sprintf("watchdog_force:%s:%s", w.spaceID, now.Format("2006-01-02T15:04")),
			TriggeredAt:   now,
			PolicyVersion: PolicyVersion,
		}

		go w.cycleTrigger(context.Background(), w.spaceID, meta)
	}

	// Phase 80: Multi-dimensional signal collection
	if w.signalProvider != nil {
		w.state.SessionHealthScore = w.signalProvider.GetSessionHealthScore("")
		w.state.ObsRatePerHour = w.signalProvider.GetObservationRate(w.spaceID)

		consolidationAge, err := w.signalProvider.GetConsolidationAgeSec(w.ctx, w.spaceID)
		if err == nil {
			w.state.ConsolidationAge = consolidationAge
		}

		// Build active anomalies list
		var anomalies []string
		if w.state.SessionHealthScore < 0.3 {
			anomalies = append(anomalies, "low-session-health")
		}
		if consolidationAge > 48*3600 { // > 48 hours
			anomalies = append(anomalies, "stale-consolidation")
		}
		if w.state.DecayScore >= w.cfg.RSICWarnThreshold {
			anomalies = append(anomalies, "high-decay-score")
		}
		if !w.signalProvider.IsJiminyHealthy(w.ctx) {
			anomalies = append(anomalies, "jiminy-unhealthy")
		}
		w.state.ActiveAnomalies = anomalies

		// Additional escalation: force cycle if session health is critically low AND decay is moderate
		if w.state.SessionHealthScore < 0.2 && w.state.DecayScore >= w.cfg.RSICNudgeThreshold && w.cycleTrigger != nil {
			if prevLevel < EscalationWarn {
				slog.Warn("RSIC watchdog: session health critical, escalating to warn level", "session_health_score", w.state.SessionHealthScore)
				w.state.EscalationLevel = EscalationWarn
			}
		}
	}

	// Phase 89: Persist watchdog state after check
	if w.store != nil {
		w.store.SaveWatchdogState(w.spaceID, w.state)
	}
}
