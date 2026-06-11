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

	mu           sync.RWMutex
	state        WatchdogState
	cycleTrigger func(ctx context.Context, spaceID string, meta TriggerMetadata) // callback to auto-trigger meso cycle

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

// Restart stops the watchdog and starts it again with a fresh context.
func (w *Watchdog) Restart() {
	w.Stop()
	w.mu.Lock()
	w.ctx, w.cancel = context.WithCancel(context.Background())
	w.mu.Unlock()
	w.Start()
}

// Running returns true if the watchdog goroutine is active.
func (w *Watchdog) Running() bool {
	select {
	case <-w.ctx.Done():
		return false
	default:
		return true
	}
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
	// Read config and state under lock, release before I/O
	w.mu.Lock()
	spaceID := w.spaceID
	lastCycleTime := w.state.LastCycleTime
	decayRate := w.cfg.RSICWatchdogDecayRate
	mesoPeriodHours := w.cfg.RSICMesoPeriodHours
	forceThreshold := w.cfg.RSICForceThreshold
	warnThreshold := w.cfg.RSICWarnThreshold
	nudgeThreshold := w.cfg.RSICNudgeThreshold
	prevLevel := w.state.EscalationLevel
	cycleTrigger := w.cycleTrigger
	signalProvider := w.signalProvider
	store := w.store
	watchdogCtx := w.ctx
	w.mu.Unlock()

	hoursSinceCycle := time.Since(lastCycleTime).Hours()
	decayScore := hoursSinceCycle * decayRate
	nextDue := lastCycleTime.Add(time.Duration(mesoPeriodHours) * time.Hour)

	metrics.Metrics().RSICWatchdogDecay(spaceID).Set(decayScore)

	var escalationLevel EscalationLevel
	switch {
	case decayScore >= forceThreshold:
		escalationLevel = EscalationForce
	case decayScore >= warnThreshold:
		escalationLevel = EscalationWarn
	case decayScore >= nudgeThreshold:
		escalationLevel = EscalationNudge
	default:
		escalationLevel = EscalationNominal
	}

	metrics.Metrics().RSICWatchdogEscalation(spaceID).Set(float64(escalationLevel))

	// Log on escalation level changes
	if escalationLevel != prevLevel {
		switch escalationLevel {
		case EscalationNudge:
			slog.Info("RSIC watchdog: nudge", "decay_score", decayScore, "hours_since_cycle", hoursSinceCycle)
		case EscalationWarn:
			slog.Warn("RSIC watchdog: warning", "decay_score", decayScore, "hours_since_cycle", hoursSinceCycle)
		case EscalationForce:
			slog.Warn("RSIC watchdog: force, auto-triggering meso cycle", "decay_score", decayScore)
		}
	}

	// Auto-trigger at force level — build metadata before acquiring lock
	var shouldTrigger bool
	var triggerMeta TriggerMetadata
	if escalationLevel == EscalationForce && cycleTrigger != nil {
		metrics.Metrics().RSICWatchdogForce.Inc()
		shouldTrigger = true
		now := time.Now()
		triggerMeta = TriggerMetadata{
			TriggerSource: TriggerWatchdogForce,
			TriggerID:     fmt.Sprintf("watchdog_force:%s:%s", spaceID, now.Format("2006-01-02T15:04")),
			TriggeredAt:   now,
			PolicyVersion: PolicyVersion,
		}
	}

	// Phase 80: Multi-dimensional signal collection (I/O without lock)
	var sessionHealthScore float64
	var obsRatePerHour float64
	var consolidationAge int64
	var consolidationAgeOK bool
	var anomalies []string
	if signalProvider != nil {
		sessionHealthScore = signalProvider.GetSessionHealthScore("")
		obsRatePerHour = signalProvider.GetObservationRate(spaceID)

		if age, err := signalProvider.GetConsolidationAgeSec(watchdogCtx, spaceID); err == nil {
			consolidationAge = age
			consolidationAgeOK = true
		}

		// Build active anomalies list
		if sessionHealthScore < 0.3 {
			anomalies = append(anomalies, "low-session-health")
		}
		if consolidationAge > 48*3600 { // > 48 hours
			anomalies = append(anomalies, "stale-consolidation")
		}
		if decayScore >= warnThreshold {
			anomalies = append(anomalies, "high-decay-score")
		}
		if !signalProvider.IsJiminyHealthy(watchdogCtx) {
			anomalies = append(anomalies, "jiminy-unhealthy")
		}
		if !signalProvider.IsSidecarHealthy(watchdogCtx) {
			anomalies = append(anomalies, "sidecar-unhealthy")
		}

		// Additional escalation: force cycle if session health is critically low AND decay is moderate
		if sessionHealthScore < 0.2 && decayScore >= nudgeThreshold && cycleTrigger != nil {
			if prevLevel < EscalationWarn {
				slog.Warn("RSIC watchdog: session health critical, escalating to warn level", "session_health_score", sessionHealthScore)
				escalationLevel = EscalationWarn
			}
		}
	}

	// Write computed state back under lock
	w.mu.Lock()
	w.state.DecayScore = decayScore
	w.state.NextDue = nextDue
	w.state.EscalationLevel = escalationLevel

	if shouldTrigger {
		// Don't reset consecutiveDecay here — wait for trigger success (4.7 fix)
		w.state.LastTriggerSource = TriggerWatchdogForce
	}

	if signalProvider != nil {
		w.state.SessionHealthScore = sessionHealthScore
		w.state.ObsRatePerHour = obsRatePerHour
		if consolidationAgeOK {
			w.state.ConsolidationAge = consolidationAge
		}
		w.state.ActiveAnomalies = anomalies
	}

	stateCopy := w.state
	w.mu.Unlock()

	// Persist watchdog state (I/O without lock)
	if store != nil {
		store.SaveWatchdogState(spaceID, stateCopy)
	}

	// Fire trigger goroutine outside lock — reset decay on success (4.7 fix)
	if shouldTrigger && cycleTrigger != nil {
		go func() {
			cycleTrigger(context.Background(), spaceID, triggerMeta)
			// Reset decay counters only after successful trigger
			w.mu.Lock()
			w.state.LastCycleTime = triggerMeta.TriggeredAt
			w.state.DecayScore = 0
			w.state.EscalationLevel = EscalationNominal
			w.mu.Unlock()
		}()
	}
}
