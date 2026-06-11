package ape

import (
	"encoding/json"
	"testing"
	"time"

	"mdemg/internal/config"
)

// ───────────── Serialization Round-Trip Tests ─────────────

func TestCalibrationSnapshot_MarshalRoundTrip(t *testing.T) {
	snap := CalibrationSnapshot{
		ActionHistory: map[string][]ActionOutcome{
			"prune_decayed_edges": {
				{ActionType: "prune_decayed_edges", Success: true, Timestamp: time.Now().UTC().Truncate(time.Millisecond)},
				{ActionType: "prune_decayed_edges", Success: false, Timestamp: time.Now().UTC().Truncate(time.Millisecond)},
			},
		},
		CycleHistory: []CycleOutcome{
			{CycleID: "rsic-meso-abc", Tier: TierMeso, SpaceID: "test-space", SafetyVersion: SafetyVersion},
		},
	}

	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored CalibrationSnapshot
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(restored.ActionHistory["prune_decayed_edges"]) != 2 {
		t.Errorf("expected 2 action outcomes, got %d", len(restored.ActionHistory["prune_decayed_edges"]))
	}
	if len(restored.CycleHistory) != 1 || restored.CycleHistory[0].CycleID != "rsic-meso-abc" {
		t.Errorf("cycle history mismatch: %+v", restored.CycleHistory)
	}
}

func TestWatchdogState_MarshalRoundTrip(t *testing.T) {
	state := WatchdogState{
		DecayScore:         0.42,
		EscalationLevel:    EscalationNudge,
		LastCycleTime:      time.Now().UTC().Truncate(time.Millisecond),
		NextDue:            time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond),
		SpaceID:            "test-space",
		SessionHealthScore: 0.85,
		ObsRatePerHour:     12.5,
		ActiveAnomalies:    []string{"high-decay-score"},
		ConsolidationAge:   3600,
		LastTriggerSource:  TriggerManualAPI,
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored WatchdogState
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if restored.DecayScore != 0.42 {
		t.Errorf("decay_score: got %v, want 0.42", restored.DecayScore)
	}
	if restored.EscalationLevel != EscalationNudge {
		t.Errorf("escalation_level: got %v, want %v", restored.EscalationLevel, EscalationNudge)
	}
	if restored.SpaceID != "test-space" {
		t.Errorf("space_id: got %v, want test-space", restored.SpaceID)
	}
	if restored.LastCycleTime != state.LastCycleTime {
		t.Errorf("last_cycle_time mismatch")
	}
}

func TestOrchestrationSnapshot_MarshalRoundTrip(t *testing.T) {
	snap := OrchestrationSnapshot{
		LastTriggers: []TriggerRecord{
			{Source: TriggerManualAPI, SpaceID: "mdemg-dev", Tier: TierMeso, CycleID: "cycle-1", Timestamp: time.Now().UTC().Truncate(time.Millisecond)},
		},
		SessionCounters: map[string]*SessionCounter{
			"mdemg-dev": {Count: 7, LastTrigger: time.Now().UTC().Truncate(time.Millisecond)},
		},
	}

	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored OrchestrationSnapshot
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(restored.LastTriggers) != 1 || restored.LastTriggers[0].CycleID != "cycle-1" {
		t.Errorf("triggers mismatch: %+v", restored.LastTriggers)
	}
	if restored.SessionCounters["mdemg-dev"].Count != 7 {
		t.Errorf("session counter mismatch: %+v", restored.SessionCounters)
	}
}

// ───────────── RSICStore Unit Tests ─────────────

func TestRSICStore_MarkDirtyTracking(t *testing.T) {
	store := NewRSICStore(nil) // nil driver — we only test in-memory tracking

	store.MarkDirty("calibration:test")
	store.MarkDirty("watchdog:test")
	store.MarkDirty("calibration:test") // duplicate

	store.mu.Lock()
	dirtyCount := len(store.dirty)
	store.mu.Unlock()

	if dirtyCount != 2 {
		t.Errorf("expected 2 dirty keys, got %d", dirtyCount)
	}
}

func TestRSICStore_GetStatus_Shape(t *testing.T) {
	store := NewRSICStore(nil)

	status := store.GetStatus()

	requiredKeys := []string{"enabled", "dirty_keys", "flush_errors", "state_nodes"}
	for _, key := range requiredKeys {
		if _, ok := status[key]; !ok {
			t.Errorf("missing key in status: %s", key)
		}
	}

	if status["enabled"] != true {
		t.Errorf("expected enabled=true, got %v", status["enabled"])
	}
	if status["state_nodes"] != 0 {
		t.Errorf("expected 0 state nodes, got %v", status["state_nodes"])
	}
}

func TestRSICStore_GetStatus_WithCachedData(t *testing.T) {
	store := NewRSICStore(nil)

	// Add cached data
	store.calibrationData["space-1"] = &CalibrationSnapshot{}
	store.watchdogData["space-1"] = &WatchdogState{}
	store.orchestrationData = &OrchestrationSnapshot{}

	status := store.GetStatus()
	if status["state_nodes"] != 3 {
		t.Errorf("expected 3 state nodes, got %v", status["state_nodes"])
	}
}

func TestRSICStore_HydrationWithNilStore(t *testing.T) {
	// Calibrator with nil store should not panic
	cal := NewCalibrator(nil, 0)
	if err := cal.Hydrate("test-space"); err != nil {
		t.Errorf("hydrate with nil store should return nil, got %v", err)
	}

	// Watchdog with nil store should not panic
	w := &Watchdog{spaceID: "test"}
	w.Hydrate(nil) // should be no-op

	// OrchestrationPolicy hydrate with nil triggers/counters
	p := &OrchestrationPolicy{
		lastTrigger:     make(map[string]TriggerRecord),
		sessionCounters: make(map[string]*SessionCounter),
	}
	p.Hydrate(nil, nil) // should be no-op, no panic
}

// ───────────── Calibrator Hydration Test ─────────────

func TestCalibrator_HydrateRestoresHistory(t *testing.T) {
	cal := NewCalibrator(nil, 0)

	// Manually inject history (simulating hydration without Neo4j)
	cal.mu.Lock()
	cal.actionHistory["prune_decayed_edges"] = []ActionOutcome{
		{ActionType: "prune_decayed_edges", Success: true, Timestamp: time.Now()},
		{ActionType: "prune_decayed_edges", Success: true, Timestamp: time.Now()},
		{ActionType: "prune_decayed_edges", Success: false, Timestamp: time.Now()},
	}
	cal.cycleHistory = []CycleOutcome{
		{CycleID: "cycle-1", SpaceID: "test"},
		{CycleID: "cycle-2", SpaceID: "test"},
	}
	cal.mu.Unlock()

	// Verify calibration reflects hydrated data
	calibration := cal.GetCalibration()
	expected := 2.0 / 3.0 // 2 successes out of 3
	if calibration["prune_decayed_edges"] != expected {
		t.Errorf("expected confidence %.4f, got %.4f", expected, calibration["prune_decayed_edges"])
	}

	// Verify history
	history := cal.GetHistory(10)
	if len(history) != 2 {
		t.Errorf("expected 2 history entries, got %d", len(history))
	}
}

// ───────────── Watchdog Hydration Test ─────────────

func TestWatchdog_HydrateRestoresDecayScore(t *testing.T) {
	cfg := defaultTestConfig()
	w := NewWatchdog(cfg, "test-space", nil)

	persistedState := &WatchdogState{
		DecayScore:      0.55,
		EscalationLevel: EscalationWarn,
		LastCycleTime:   time.Now().Add(-2 * time.Hour),
		SpaceID:         "other-space", // should be overridden
	}

	w.Hydrate(persistedState)

	state := w.GetState()
	if state.DecayScore != 0.55 {
		t.Errorf("expected decay_score 0.55, got %f", state.DecayScore)
	}
	if state.EscalationLevel != EscalationWarn {
		t.Errorf("expected warn escalation, got %d", state.EscalationLevel)
	}
	// SpaceID should be preserved from constructor, not overwritten
	if state.SpaceID != "test-space" {
		t.Errorf("expected space_id 'test-space', got '%s'", state.SpaceID)
	}
}

// ───────────── Orchestration Hydration Test ─────────────

func TestOrchestrationPolicy_HydrateRestoresCooldowns(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.RSICTriggerCooldownSec = 300
	cfg.RSICTriggerDedupeSec = 600

	p := NewOrchestrationPolicy(cfg)

	// Simulate a recent trigger
	triggers := []TriggerRecord{
		{
			Source:    TriggerManualAPI,
			SpaceID:   "test-space",
			Tier:      TierMeso,
			CycleID:   "cycle-old",
			Timestamp: time.Now().Add(-10 * time.Second), // 10 seconds ago — within cooldown
		},
	}
	counters := map[string]*SessionCounter{
		"test-space": {Count: 5, LastTrigger: time.Now().Add(-1 * time.Hour)},
	}

	p.Hydrate(triggers, counters)

	// Should be blocked by cooldown
	decision := p.EvaluateTrigger(TriggerManualAPI, "test-space", TierMeso, "")
	if decision.Allowed {
		t.Error("expected trigger to be rejected due to cooldown after hydration")
	}

	// Session counter should be restored
	count, _ := p.IncrementSession("test-space")
	if count != 6 { // 5 from hydration + 1 increment
		t.Errorf("expected session count 6, got %d", count)
	}
}

// ───────────── Dispatcher Cleanup Tests ─────────────

func TestDispatcher_CleanupStaleTasks(t *testing.T) {
	d := &Dispatcher{
		activeTasks: make(map[string]*activeTask),
		reports:     make(map[string][]RSICProgressReport),
	}

	// Add completed tasks with varying ages
	d.activeTasks["old-completed"] = &activeTask{
		StartedAt: time.Now().Add(-20 * time.Minute),
		Status:    "completed",
	}
	d.activeTasks["recent-completed"] = &activeTask{
		StartedAt: time.Now().Add(-5 * time.Minute),
		Status:    "completed",
	}
	d.activeTasks["old-running"] = &activeTask{
		StartedAt: time.Now().Add(-20 * time.Minute),
		Status:    "running",
	}
	d.activeTasks["old-failed"] = &activeTask{
		StartedAt: time.Now().Add(-15 * time.Minute),
		Status:    "failed",
	}
	d.reports["old-completed"] = []RSICProgressReport{{TaskID: "old-completed"}}
	d.reports["old-failed"] = []RSICProgressReport{{TaskID: "old-failed"}}

	removed := d.CleanupStaleTasks(10 * time.Minute)

	if removed != 2 { // old-completed + old-failed
		t.Errorf("expected 2 removed, got %d", removed)
	}
	if _, ok := d.activeTasks["old-completed"]; ok {
		t.Error("old-completed should have been removed")
	}
	if _, ok := d.activeTasks["recent-completed"]; !ok {
		t.Error("recent-completed should still exist")
	}
	if _, ok := d.activeTasks["old-running"]; !ok {
		t.Error("old-running should still exist (not terminal)")
	}
	if _, ok := d.reports["old-completed"]; ok {
		t.Error("reports for old-completed should have been removed")
	}
}

func TestDispatcher_TaskCapEviction(t *testing.T) {
	d := &Dispatcher{
		activeTasks: make(map[string]*activeTask),
		reports:     make(map[string][]RSICProgressReport),
	}

	// Add 1005 running tasks
	baseTime := time.Now()
	for i := 0; i < 1005; i++ {
		id := "task-" + time.Duration(i).String()
		d.activeTasks[id] = &activeTask{
			StartedAt: baseTime.Add(time.Duration(i) * time.Second),
			Status:    "running",
		}
	}

	removed := d.CleanupStaleTasks(1 * time.Hour) // none expired but over cap

	if len(d.activeTasks) > 1000 {
		t.Errorf("expected <= 1000 tasks after cap eviction, got %d", len(d.activeTasks))
	}
	if removed < 5 {
		t.Errorf("expected at least 5 evicted by cap, got %d", removed)
	}
}

// ───────────── DateTime Adapter Tests ─────────────

func TestDateTimeAdapter_HandlesTimeType(t *testing.T) {
	// Simulate what the adapter does with a time.Time value
	past := time.Now().Add(-2 * time.Hour)
	var ageSec int64

	switch tv := any(past).(type) {
	case time.Time:
		ageSec = int64(time.Since(tv).Seconds())
	}

	if ageSec < 7100 || ageSec > 7300 { // ~2 hours in seconds
		t.Errorf("expected ~7200 seconds, got %d", ageSec)
	}
}

func TestDateTimeAdapter_HandlesStringType(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	var ageSec int64

	switch tv := any(past).(type) {
	case string:
		if parsed, err := time.Parse(time.RFC3339, tv); err == nil {
			ageSec = int64(time.Since(parsed).Seconds())
		}
	}

	if ageSec < 3500 || ageSec > 3700 { // ~1 hour in seconds
		t.Errorf("expected ~3600 seconds, got %d", ageSec)
	}
}

func TestDateTimeAdapter_InvalidStringReturnsZero(t *testing.T) {
	var ageSec int64

	v := any("not-a-timestamp")
	switch tv := v.(type) {
	case time.Time:
		ageSec = int64(time.Since(tv).Seconds())
	case string:
		if parsed, err := time.Parse(time.RFC3339, tv); err == nil {
			ageSec = int64(time.Since(parsed).Seconds())
		}
		// Falls through to 0 on parse error
	}

	if ageSec != 0 {
		t.Errorf("expected 0 for invalid string, got %d", ageSec)
	}
}

// ───────────── Save/Load In-Memory Tests ─────────────

func TestRSICStore_SaveCalibration_CachesInMemory(t *testing.T) {
	store := NewRSICStore(nil)

	history := []CycleOutcome{{CycleID: "c1"}}
	actions := map[string][]ActionOutcome{
		"prune": {{ActionType: "prune", Success: true}},
	}

	store.SaveCalibration("test-space", history, actions)

	store.mu.Lock()
	snap := store.calibrationData["test-space"]
	dirty := store.dirty["calibration:test-space"]
	store.mu.Unlock()

	if snap == nil {
		t.Fatal("expected calibration data to be cached")
	}
	if len(snap.CycleHistory) != 1 {
		t.Errorf("expected 1 cycle history entry, got %d", len(snap.CycleHistory))
	}
	if !dirty {
		t.Error("expected dirty flag to be set")
	}
}

func TestRSICStore_SaveWatchdog_CachesInMemory(t *testing.T) {
	store := NewRSICStore(nil)

	state := WatchdogState{DecayScore: 0.3, SpaceID: "test"}
	store.SaveWatchdogState("test", state)

	store.mu.Lock()
	cached := store.watchdogData["test"]
	dirty := store.dirty["watchdog:test"]
	store.mu.Unlock()

	if cached == nil || cached.DecayScore != 0.3 {
		t.Errorf("expected cached watchdog state with decay 0.3, got %+v", cached)
	}
	if !dirty {
		t.Error("expected dirty flag to be set")
	}
}

// ───────────── Helper ─────────────

func defaultTestConfig() config.Config {
	return config.Config{
		RSICWatchdogEnabled:   true,
		RSICWatchdogCheckSec:  300,
		RSICWatchdogDecayRate: 0.1,
		RSICNudgeThreshold:    0.3,
		RSICWarnThreshold:     0.6,
		RSICForceThreshold:    0.9,
		RSICMesoPeriodHours:   6,
	}
}
