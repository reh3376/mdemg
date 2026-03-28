package ape

import (
	"context"
	"errors"
	"testing"
	"time"

	"mdemg/internal/config"
)

// mockWatchdogSignalProvider implements WatchdogSignalProvider for testing.
type mockWatchdogSignalProvider struct {
	sessionHealthScore   float64
	obsRatePerHour       float64
	consolidationAgeSec  int64
	consolidationAgeErr  error
	jiminyHealthy        bool
	sidecarHealthy       bool
}

func (m *mockWatchdogSignalProvider) GetSessionHealthScore(sessionID string) float64 {
	return m.sessionHealthScore
}

func (m *mockWatchdogSignalProvider) GetObservationRate(spaceID string) float64 {
	return m.obsRatePerHour
}

func (m *mockWatchdogSignalProvider) GetConsolidationAgeSec(ctx context.Context, spaceID string) (int64, error) {
	if m.consolidationAgeErr != nil {
		return 0, m.consolidationAgeErr
	}
	return m.consolidationAgeSec, nil
}

func (m *mockWatchdogSignalProvider) IsJiminyHealthy(_ context.Context) bool {
	return m.jiminyHealthy
}

func (m *mockWatchdogSignalProvider) IsSidecarHealthy(_ context.Context) bool {
	return m.sidecarHealthy
}

func TestNewWatchdog(t *testing.T) {
	cfg := config.Config{
		RSICWatchdogEnabled:   true,
		RSICWatchdogCheckSec:  60,
		RSICWatchdogDecayRate: 0.1,
		RSICMesoPeriodHours:   6,
		RSICNudgeThreshold:    0.3,
		RSICWarnThreshold:     0.6,
		RSICForceThreshold:    0.9,
	}

	cycleTriggerCalled := false
	cycleTrigger := func(ctx context.Context, spaceID string, meta TriggerMetadata) {
		cycleTriggerCalled = true
	}

	w := NewWatchdog(cfg, "test-space", cycleTrigger)

	if w == nil {
		t.Fatal("NewWatchdog returned nil")
	}

	if w.spaceID != "test-space" {
		t.Errorf("spaceID = %q, want %q", w.spaceID, "test-space")
	}

	state := w.GetState()
	if state.SpaceID != "test-space" {
		t.Errorf("state.SpaceID = %q, want %q", state.SpaceID, "test-space")
	}

	if state.EscalationLevel != EscalationNominal {
		t.Errorf("initial EscalationLevel = %d, want %d", state.EscalationLevel, EscalationNominal)
	}

	if state.DecayScore != 0 {
		t.Errorf("initial DecayScore = %.2f, want 0.00", state.DecayScore)
	}

	if cycleTriggerCalled {
		t.Error("cycleTrigger should not be called on creation")
	}

	// Clean up
	w.Stop()
}

func TestSetSignalProvider(t *testing.T) {
	cfg := config.Config{
		RSICWatchdogEnabled:   true,
		RSICWatchdogCheckSec:  60,
		RSICWatchdogDecayRate: 0.1,
		RSICMesoPeriodHours:   6,
		RSICNudgeThreshold:    0.3,
		RSICWarnThreshold:     0.6,
		RSICForceThreshold:    0.9,
	}

	w := NewWatchdog(cfg, "test-space", nil)
	defer w.Stop()

	// Initially no signal provider
	if w.signalProvider != nil {
		t.Error("signalProvider should be nil initially")
	}

	// Set signal provider
	mockProvider := &mockWatchdogSignalProvider{
		sessionHealthScore:  0.75,
		obsRatePerHour:      5.0,
		consolidationAgeSec: 3600,
		jiminyHealthy:       true,
		sidecarHealthy:      true,
	}

	w.SetSignalProvider(mockProvider)

	// Verify it was set (we can't check directly without exposing the field,
	// but we can verify it's used by calling check)
	w.check()

	state := w.GetState()
	if state.SessionHealthScore != 0.75 {
		t.Errorf("SessionHealthScore = %.2f, want 0.75", state.SessionHealthScore)
	}
	if state.ObsRatePerHour != 5.0 {
		t.Errorf("ObsRatePerHour = %.2f, want 5.0", state.ObsRatePerHour)
	}
	if state.ConsolidationAge != 3600 {
		t.Errorf("ConsolidationAge = %d, want 3600", state.ConsolidationAge)
	}
}

func TestCheckWithoutSignalProvider(t *testing.T) {
	tests := []struct {
		name              string
		timeSinceCycle    time.Duration
		decayRate         float64
		wantEscalation    EscalationLevel
		nudgeThreshold    float64
		warnThreshold     float64
		forceThreshold    float64
	}{
		{
			name:           "nominal state",
			timeSinceCycle: 1 * time.Hour,
			decayRate:      0.1,
			wantEscalation: EscalationNominal,
			nudgeThreshold: 0.3,
			warnThreshold:  0.6,
			forceThreshold: 0.9,
		},
		{
			name:           "nudge level",
			timeSinceCycle: 4 * time.Hour,
			decayRate:      0.1,
			wantEscalation: EscalationNudge,
			nudgeThreshold: 0.3,
			warnThreshold:  0.6,
			forceThreshold: 0.9,
		},
		{
			name:           "warn level",
			timeSinceCycle: 7 * time.Hour,
			decayRate:      0.1,
			wantEscalation: EscalationWarn,
			nudgeThreshold: 0.3,
			warnThreshold:  0.6,
			forceThreshold: 0.9,
		},
		{
			name:           "force level triggers auto-reset",
			timeSinceCycle: 10 * time.Hour,
			decayRate:      0.1,
			wantEscalation: EscalationNominal, // After force trigger, state resets
			nudgeThreshold: 0.3,
			warnThreshold:  0.6,
			forceThreshold: 0.9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{
				RSICWatchdogEnabled:   true,
				RSICWatchdogCheckSec:  60,
				RSICWatchdogDecayRate: tt.decayRate,
				RSICMesoPeriodHours:   6,
				RSICNudgeThreshold:    tt.nudgeThreshold,
				RSICWarnThreshold:     tt.warnThreshold,
				RSICForceThreshold:    tt.forceThreshold,
			}

			cycleTriggerCalled := false
			w := NewWatchdog(cfg, "test-space", func(ctx context.Context, spaceID string, meta TriggerMetadata) {
				cycleTriggerCalled = true
			})
			defer w.Stop()

			// Manually set last cycle time to simulate time passing
			w.mu.Lock()
			w.state.LastCycleTime = time.Now().Add(-tt.timeSinceCycle)
			w.mu.Unlock()

			// Run check
			w.check()

			// Give goroutine time to execute for force level
			if tt.name == "force level triggers auto-reset" {
				time.Sleep(50 * time.Millisecond)
			}

			state := w.GetState()

			// For force level with cycle trigger, state resets immediately
			if tt.name == "force level triggers auto-reset" {
				if !cycleTriggerCalled {
					t.Error("cycleTrigger should be called at force level")
				}
				// State should be reset after force trigger
				if state.EscalationLevel != EscalationNominal {
					t.Errorf("after force trigger, EscalationLevel = %d, want %d", state.EscalationLevel, EscalationNominal)
				}
				if state.DecayScore != 0 {
					t.Errorf("after force trigger, DecayScore = %.2f, want 0", state.DecayScore)
				}
			} else {
				// For all other levels, check decay score and escalation
				expectedDecay := tt.timeSinceCycle.Hours() * tt.decayRate
				if state.DecayScore < expectedDecay-0.01 || state.DecayScore > expectedDecay+0.01 {
					t.Errorf("DecayScore = %.2f, want ~%.2f", state.DecayScore, expectedDecay)
				}

				if state.EscalationLevel != tt.wantEscalation {
					t.Errorf("EscalationLevel = %d, want %d", state.EscalationLevel, tt.wantEscalation)
				}

				if cycleTriggerCalled {
					t.Error("cycleTrigger should not be called below force level")
				}
			}
		})
	}
}

func TestCheckWithSignalProvider(t *testing.T) {
	cfg := config.Config{
		RSICWatchdogEnabled:   true,
		RSICWatchdogCheckSec:  60,
		RSICWatchdogDecayRate: 0.1,
		RSICMesoPeriodHours:   6,
		RSICNudgeThreshold:    0.3,
		RSICWarnThreshold:     0.6,
		RSICForceThreshold:    0.9,
	}

	w := NewWatchdog(cfg, "test-space", nil)
	defer w.Stop()

	mockProvider := &mockWatchdogSignalProvider{
		sessionHealthScore:  0.85,
		obsRatePerHour:      12.5,
		consolidationAgeSec: 7200, // 2 hours
		jiminyHealthy:       true,
		sidecarHealthy:      true,
	}

	w.SetSignalProvider(mockProvider)
	w.check()

	state := w.GetState()

	if state.SessionHealthScore != 0.85 {
		t.Errorf("SessionHealthScore = %.2f, want 0.85", state.SessionHealthScore)
	}

	if state.ObsRatePerHour != 12.5 {
		t.Errorf("ObsRatePerHour = %.2f, want 12.5", state.ObsRatePerHour)
	}

	if state.ConsolidationAge != 7200 {
		t.Errorf("ConsolidationAge = %d, want 7200", state.ConsolidationAge)
	}
}

func TestCheckWithSignalProviderError(t *testing.T) {
	cfg := config.Config{
		RSICWatchdogEnabled:   true,
		RSICWatchdogCheckSec:  60,
		RSICWatchdogDecayRate: 0.1,
		RSICMesoPeriodHours:   6,
		RSICNudgeThreshold:    0.3,
		RSICWarnThreshold:     0.6,
		RSICForceThreshold:    0.9,
	}

	w := NewWatchdog(cfg, "test-space", nil)
	defer w.Stop()

	mockProvider := &mockWatchdogSignalProvider{
		sessionHealthScore:  0.75,
		obsRatePerHour:      5.0,
		consolidationAgeErr: errors.New("database error"),
		jiminyHealthy:       true,
		sidecarHealthy:      true,
	}

	w.SetSignalProvider(mockProvider)
	w.check()

	state := w.GetState()

	// Should still populate other fields
	if state.SessionHealthScore != 0.75 {
		t.Errorf("SessionHealthScore = %.2f, want 0.75", state.SessionHealthScore)
	}

	// ConsolidationAge should remain at previous value (0 initially)
	if state.ConsolidationAge != 0 {
		t.Errorf("ConsolidationAge = %d, want 0 (error should not update)", state.ConsolidationAge)
	}
}

func TestActiveAnomaliesDetection(t *testing.T) {
	tests := []struct {
		name                string
		sessionHealth       float64
		consolidationAgeSec int64
		decayScore          float64
		wantAnomalies       []string
	}{
		{
			name:                "no anomalies",
			sessionHealth:       0.8,
			consolidationAgeSec: 3600,  // 1 hour
			decayScore:          0.2,
			wantAnomalies:       nil,
		},
		{
			name:                "low session health",
			sessionHealth:       0.25,
			consolidationAgeSec: 3600,
			decayScore:          0.2,
			wantAnomalies:       []string{"low-session-health"},
		},
		{
			name:                "stale consolidation",
			sessionHealth:       0.8,
			consolidationAgeSec: 200000, // > 48 hours
			decayScore:          0.2,
			wantAnomalies:       []string{"stale-consolidation"},
		},
		{
			name:                "high decay score",
			sessionHealth:       0.8,
			consolidationAgeSec: 3600,
			decayScore:          0.65,
			wantAnomalies:       []string{"high-decay-score"},
		},
		{
			name:                "multiple anomalies",
			sessionHealth:       0.25,
			consolidationAgeSec: 200000,
			decayScore:          0.7,
			wantAnomalies:       []string{"low-session-health", "stale-consolidation", "high-decay-score"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{
				RSICWatchdogEnabled:   true,
				RSICWatchdogCheckSec:  60,
				RSICWatchdogDecayRate: 0.1,
				RSICMesoPeriodHours:   6,
				RSICNudgeThreshold:    0.3,
				RSICWarnThreshold:     0.6,
				RSICForceThreshold:    0.9,
			}

			w := NewWatchdog(cfg, "test-space", nil)
			defer w.Stop()

			// Set up mock provider
			mockProvider := &mockWatchdogSignalProvider{
				sessionHealthScore:  tt.sessionHealth,
				obsRatePerHour:      5.0,
				consolidationAgeSec: tt.consolidationAgeSec,
				jiminyHealthy:       true,
				sidecarHealthy:      true,
			}
			w.SetSignalProvider(mockProvider)

			// Set decay score by adjusting last cycle time
			if tt.decayScore > 0 {
				hoursAgo := tt.decayScore / cfg.RSICWatchdogDecayRate
				w.mu.Lock()
				w.state.LastCycleTime = time.Now().Add(-time.Duration(hoursAgo) * time.Hour)
				w.mu.Unlock()
			}

			w.check()

			state := w.GetState()

			if len(state.ActiveAnomalies) != len(tt.wantAnomalies) {
				t.Errorf("got %d anomalies, want %d: %v", len(state.ActiveAnomalies), len(tt.wantAnomalies), state.ActiveAnomalies)
				return
			}

			// Check that all expected anomalies are present
			anomalyMap := make(map[string]bool)
			for _, a := range state.ActiveAnomalies {
				anomalyMap[a] = true
			}

			for _, want := range tt.wantAnomalies {
				if !anomalyMap[want] {
					t.Errorf("missing expected anomaly: %s", want)
				}
			}
		})
	}
}

func TestAdditionalEscalation(t *testing.T) {
	tests := []struct {
		name              string
		sessionHealth     float64
		timeSinceCycle    time.Duration
		decayRate         float64
		nudgeThreshold    float64
		wantEscalation    EscalationLevel
		wantCycleTrigger  bool
	}{
		{
			name:             "critical session health with moderate decay - escalate to warn",
			sessionHealth:    0.15,
			timeSinceCycle:   4 * time.Hour,
			decayRate:        0.1,
			nudgeThreshold:   0.3,
			wantEscalation:   EscalationWarn,
			wantCycleTrigger: false,
		},
		{
			name:             "critical session health below nudge threshold - no escalation",
			sessionHealth:    0.15,
			timeSinceCycle:   2 * time.Hour,
			decayRate:        0.1,
			nudgeThreshold:   0.3,
			wantEscalation:   EscalationNominal,
			wantCycleTrigger: false,
		},
		{
			name:             "low session health but not critical - no additional escalation",
			sessionHealth:    0.25,
			timeSinceCycle:   4 * time.Hour,
			decayRate:        0.1,
			nudgeThreshold:   0.3,
			wantEscalation:   EscalationNudge,
			wantCycleTrigger: false,
		},
		{
			name:             "healthy session - no additional escalation",
			sessionHealth:    0.8,
			timeSinceCycle:   4 * time.Hour,
			decayRate:        0.1,
			nudgeThreshold:   0.3,
			wantEscalation:   EscalationNudge,
			wantCycleTrigger: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{
				RSICWatchdogEnabled:   true,
				RSICWatchdogCheckSec:  60,
				RSICWatchdogDecayRate: tt.decayRate,
				RSICMesoPeriodHours:   6,
				RSICNudgeThreshold:    tt.nudgeThreshold,
				RSICWarnThreshold:     0.6,
				RSICForceThreshold:    0.9,
			}

			cycleTriggerCalled := false
			w := NewWatchdog(cfg, "test-space", func(ctx context.Context, spaceID string, meta TriggerMetadata) {
				cycleTriggerCalled = true
			})
			defer w.Stop()

			// Set up mock provider with critical session health
			mockProvider := &mockWatchdogSignalProvider{
				sessionHealthScore:  tt.sessionHealth,
				obsRatePerHour:      5.0,
				consolidationAgeSec: 3600,
				jiminyHealthy:       true,
				sidecarHealthy:      true,
			}
			w.SetSignalProvider(mockProvider)

			// Set decay score
			w.mu.Lock()
			w.state.LastCycleTime = time.Now().Add(-tt.timeSinceCycle)
			w.mu.Unlock()

			w.check()

			state := w.GetState()

			if state.EscalationLevel != tt.wantEscalation {
				t.Errorf("EscalationLevel = %d, want %d", state.EscalationLevel, tt.wantEscalation)
			}

			if cycleTriggerCalled != tt.wantCycleTrigger {
				t.Errorf("cycleTriggerCalled = %v, want %v", cycleTriggerCalled, tt.wantCycleTrigger)
			}
		})
	}
}

func TestRecordCycle(t *testing.T) {
	cfg := config.Config{
		RSICWatchdogEnabled:   true,
		RSICWatchdogCheckSec:  60,
		RSICWatchdogDecayRate: 0.1,
		RSICMesoPeriodHours:   6,
		RSICNudgeThreshold:    0.3,
		RSICWarnThreshold:     0.6,
		RSICForceThreshold:    0.9,
	}

	w := NewWatchdog(cfg, "test-space", nil)
	defer w.Stop()

	// Simulate decay by setting old last cycle time
	w.mu.Lock()
	w.state.LastCycleTime = time.Now().Add(-10 * time.Hour)
	w.mu.Unlock()

	// Check to accumulate decay
	w.check()

	stateBefore := w.GetState()
	if stateBefore.DecayScore == 0 {
		t.Error("DecayScore should be > 0 before RecordCycle")
	}
	if stateBefore.EscalationLevel == EscalationNominal {
		t.Error("EscalationLevel should be elevated before RecordCycle")
	}

	// Record cycle
	beforeRecord := time.Now()
	w.RecordCycle()
	afterRecord := time.Now()

	stateAfter := w.GetState()

	if stateAfter.DecayScore != 0 {
		t.Errorf("DecayScore = %.2f after RecordCycle, want 0", stateAfter.DecayScore)
	}

	if stateAfter.EscalationLevel != EscalationNominal {
		t.Errorf("EscalationLevel = %d after RecordCycle, want %d", stateAfter.EscalationLevel, EscalationNominal)
	}

	if stateAfter.LastCycleTime.Before(beforeRecord) || stateAfter.LastCycleTime.After(afterRecord) {
		t.Errorf("LastCycleTime not updated correctly: %v", stateAfter.LastCycleTime)
	}
}

func TestGetState(t *testing.T) {
	cfg := config.Config{
		RSICWatchdogEnabled:   true,
		RSICWatchdogCheckSec:  60,
		RSICWatchdogDecayRate: 0.1,
		RSICMesoPeriodHours:   6,
		RSICNudgeThreshold:    0.3,
		RSICWarnThreshold:     0.6,
		RSICForceThreshold:    0.9,
	}

	w := NewWatchdog(cfg, "test-space", nil)
	defer w.Stop()

	// Set up mock provider
	mockProvider := &mockWatchdogSignalProvider{
		sessionHealthScore:  0.65,
		obsRatePerHour:      8.5,
		consolidationAgeSec: 14400, // 4 hours
		jiminyHealthy:       true,
		sidecarHealthy:      true,
	}
	w.SetSignalProvider(mockProvider)

	// Simulate some decay
	w.mu.Lock()
	w.state.LastCycleTime = time.Now().Add(-5 * time.Hour)
	w.mu.Unlock()

	w.check()

	state := w.GetState()

	// Verify all fields are populated
	if state.SpaceID != "test-space" {
		t.Errorf("SpaceID = %q, want %q", state.SpaceID, "test-space")
	}

	if state.DecayScore == 0 {
		t.Error("DecayScore should be > 0")
	}

	if state.SessionHealthScore != 0.65 {
		t.Errorf("SessionHealthScore = %.2f, want 0.65", state.SessionHealthScore)
	}

	if state.ObsRatePerHour != 8.5 {
		t.Errorf("ObsRatePerHour = %.2f, want 8.5", state.ObsRatePerHour)
	}

	if state.ConsolidationAge != 14400 {
		t.Errorf("ConsolidationAge = %d, want 14400", state.ConsolidationAge)
	}

	if state.NextDue.IsZero() {
		t.Error("NextDue should be set")
	}

	// GetState should return a copy (test immutability)
	state.DecayScore = 999.0
	state2 := w.GetState()
	if state2.DecayScore == 999.0 {
		t.Error("GetState should return a copy, not a reference")
	}
}

func TestWatchdogNextDue(t *testing.T) {
	cfg := config.Config{
		RSICWatchdogEnabled:   true,
		RSICWatchdogCheckSec:  60,
		RSICWatchdogDecayRate: 0.1,
		RSICMesoPeriodHours:   12,
		RSICNudgeThreshold:    0.3,
		RSICWarnThreshold:     0.6,
		RSICForceThreshold:    0.9,
	}

	w := NewWatchdog(cfg, "test-space", nil)
	defer w.Stop()

	lastCycle := time.Now().Add(-2 * time.Hour)
	w.mu.Lock()
	w.state.LastCycleTime = lastCycle
	w.mu.Unlock()

	w.check()

	state := w.GetState()

	expectedNextDue := lastCycle.Add(12 * time.Hour)
	diff := state.NextDue.Sub(expectedNextDue)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("NextDue = %v, want %v (diff: %v)", state.NextDue, expectedNextDue, diff)
	}
}
