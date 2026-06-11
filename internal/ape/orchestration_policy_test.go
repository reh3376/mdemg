package ape

import (
	"testing"
	"time"

	"mdemg/internal/config"
)

func testConfig() config.Config {
	return config.Config{
		RSICTriggerCooldownSec: 300,
		RSICTriggerDedupeSec:   600,
		RSICMicroEnabled:       true,
		RSICMesoPeriodSessions: 5,
		RSICMacroCron:          "0 3 * * 0",
	}
}

func TestTriggerPriority_ResolvesDeterministically(t *testing.T) {
	if TriggerWatchdogForce.Priority() >= TriggerManualAPI.Priority() {
		t.Error("watchdog_force should have higher priority (lower number) than manual_api")
	}
	if TriggerManualAPI.Priority() >= TriggerMacroCron.Priority() {
		t.Error("manual_api should have higher priority than macro_cron")
	}
	if TriggerMacroCron.Priority() >= TriggerSessionPeriodic.Priority() {
		t.Error("macro_cron should have higher priority than session_periodic")
	}
	if TriggerSessionPeriodic.Priority() >= TriggerMicroAuto.Priority() {
		t.Error("session_periodic should have higher priority than micro_auto")
	}
}

func TestTriggerDedupe_SkipsDuplicateWithinWindow(t *testing.T) {
	p := NewOrchestrationPolicy(testConfig())

	key := "test-key-123"

	// First trigger should be allowed
	d1 := p.EvaluateTrigger(TriggerManualAPI, "space-1", TierMeso, key)
	if !d1.Allowed {
		t.Fatalf("first trigger should be allowed, got reason: %s", d1.Reason)
	}

	// Record it
	p.RecordTrigger(d1.Meta, "space-1", TierMeso, "cycle-1")
	p.CompleteCycle("space-1", TierMeso)

	// Second trigger with same key should be rejected (within dedupe window)
	d2 := p.EvaluateTrigger(TriggerManualAPI, "space-1", TierMeso, key)
	if d2.Allowed {
		t.Error("duplicate trigger within dedupe window should be rejected")
	}
}

func TestTriggerDedupe_AllowsAfterWindowExpires(t *testing.T) {
	cfg := testConfig()
	cfg.RSICTriggerDedupeSec = 1   // 1 second for test
	cfg.RSICTriggerCooldownSec = 1 // also short cooldown
	p := NewOrchestrationPolicy(cfg)

	key := "test-key-expire"

	d1 := p.EvaluateTrigger(TriggerManualAPI, "space-1", TierMeso, key)
	if !d1.Allowed {
		t.Fatalf("first trigger should be allowed")
	}
	p.RecordTrigger(d1.Meta, "space-1", TierMeso, "cycle-1")
	p.CompleteCycle("space-1", TierMeso)

	// Wait for dedupe window to expire
	time.Sleep(1100 * time.Millisecond)

	// Manually clean expired to simulate cleanup
	p.CleanupExpired()

	d2 := p.EvaluateTrigger(TriggerManualAPI, "space-1", TierMeso, key)
	if !d2.Allowed {
		t.Errorf("trigger after dedupe window should be allowed, got reason: %s", d2.Reason)
	}
}

func TestTriggerCooldown_EnforcesPerSourceCooldown(t *testing.T) {
	cfg := testConfig()
	cfg.RSICTriggerCooldownSec = 2
	p := NewOrchestrationPolicy(cfg)

	d1 := p.EvaluateTrigger(TriggerManualAPI, "space-1", TierMeso, "")
	if !d1.Allowed {
		t.Fatalf("first trigger should be allowed")
	}
	p.RecordTrigger(d1.Meta, "space-1", TierMeso, "cycle-1")
	p.CompleteCycle("space-1", TierMeso)

	// Same source + space within cooldown
	d2 := p.EvaluateTrigger(TriggerManualAPI, "space-1", TierMeso, "")
	if d2.Allowed {
		t.Error("trigger within cooldown should be rejected")
	}

	// Different source should still be allowed
	d3 := p.EvaluateTrigger(TriggerWatchdogForce, "space-1", TierMeso, "")
	if !d3.Allowed {
		t.Errorf("different source should be allowed during cooldown, got reason: %s", d3.Reason)
	}
}

func TestTriggerOverlap_SkipsWhenTierSpaceActive(t *testing.T) {
	p := NewOrchestrationPolicy(testConfig())

	d1 := p.EvaluateTrigger(TriggerManualAPI, "space-1", TierMeso, "")
	if !d1.Allowed {
		t.Fatalf("first trigger should be allowed")
	}
	// Record as active (do NOT complete it)
	p.RecordTrigger(d1.Meta, "space-1", TierMeso, "cycle-1")

	// Second trigger for same space+tier should be rejected (overlap)
	d2 := p.EvaluateTrigger(TriggerManualAPI, "space-1", TierMeso, "")
	if d2.Allowed {
		t.Error("overlapping trigger for same space+tier should be rejected")
	}
}

func TestTriggerOverlap_AllowsAfterComplete(t *testing.T) {
	cfg := testConfig()
	cfg.RSICTriggerCooldownSec = 0 // disable cooldown for this test
	p := NewOrchestrationPolicy(cfg)

	d1 := p.EvaluateTrigger(TriggerManualAPI, "space-1", TierMeso, "")
	if !d1.Allowed {
		t.Fatalf("first trigger should be allowed")
	}
	p.RecordTrigger(d1.Meta, "space-1", TierMeso, "cycle-1")
	p.CompleteCycle("space-1", TierMeso)

	d2 := p.EvaluateTrigger(TriggerManualAPI, "space-1", TierMeso, "")
	if !d2.Allowed {
		t.Errorf("trigger after complete should be allowed, got reason: %s", d2.Reason)
	}
}

func TestSourceTierValidation_RejectsInvalidPairs(t *testing.T) {
	p := NewOrchestrationPolicy(testConfig())

	tests := []struct {
		source TriggerSource
		tier   CycleTier
		want   bool
	}{
		{TriggerManualAPI, TierMicro, true},
		{TriggerManualAPI, TierMeso, true},
		{TriggerManualAPI, TierMacro, true},
		{TriggerMicroAuto, TierMicro, true},
		{TriggerMicroAuto, TierMeso, false},
		{TriggerMicroAuto, TierMacro, false},
		{TriggerSessionPeriodic, TierMeso, true},
		{TriggerSessionPeriodic, TierMicro, false},
		{TriggerMacroCron, TierMacro, true},
		{TriggerMacroCron, TierMeso, false},
		{TriggerWatchdogForce, TierMeso, true},
		{TriggerWatchdogForce, TierMicro, false},
	}

	for _, tt := range tests {
		d := p.EvaluateTrigger(tt.source, "space-1", tt.tier, "")
		if d.Allowed != tt.want {
			t.Errorf("source=%s tier=%s: got allowed=%v, want %v (reason: %s)",
				tt.source, tt.tier, d.Allowed, tt.want, d.Reason)
		}
	}
}

func TestSessionCounter_TriggersAtInterval(t *testing.T) {
	cfg := testConfig()
	cfg.RSICMesoPeriodSessions = 3
	p := NewOrchestrationPolicy(cfg)

	var triggers []int
	for i := 1; i <= 9; i++ {
		count, shouldTrigger := p.IncrementSession("space-1")
		if count != i {
			t.Errorf("count = %d, want %d", count, i)
		}
		if shouldTrigger {
			triggers = append(triggers, i)
		}
	}

	expected := []int{3, 6, 9}
	if len(triggers) != len(expected) {
		t.Fatalf("triggers = %v, want %v", triggers, expected)
	}
	for i, v := range expected {
		if triggers[i] != v {
			t.Errorf("trigger[%d] = %d, want %d", i, triggers[i], v)
		}
	}
}

func TestSessionCounter_NoTriggerWhenPeriodZero(t *testing.T) {
	cfg := testConfig()
	cfg.RSICMesoPeriodSessions = 0
	p := NewOrchestrationPolicy(cfg)

	for i := 0; i < 10; i++ {
		_, shouldTrigger := p.IncrementSession("space-1")
		if shouldTrigger {
			t.Errorf("should never trigger when period is 0, triggered at increment %d", i+1)
		}
	}
}

func TestCleanupExpired_RemovesOldEntries(t *testing.T) {
	cfg := testConfig()
	cfg.RSICTriggerCooldownSec = 1
	cfg.RSICTriggerDedupeSec = 1
	p := NewOrchestrationPolicy(cfg)

	// Create entries
	d := p.EvaluateTrigger(TriggerManualAPI, "space-1", TierMeso, "key-1")
	p.RecordTrigger(d.Meta, "space-1", TierMeso, "cycle-1")
	p.CompleteCycle("space-1", TierMeso)

	// Wait for expiry
	time.Sleep(1100 * time.Millisecond)

	// Before cleanup — cooldown should still reject (entries exist)
	// After cleanup — should be cleared
	p.CleanupExpired()

	d2 := p.EvaluateTrigger(TriggerManualAPI, "space-1", TierMeso, "key-1")
	if !d2.Allowed {
		t.Errorf("after cleanup should be allowed, got reason: %s", d2.Reason)
	}
}

func TestGetOrchestrationStatus_ReturnsCorrectShape(t *testing.T) {
	cfg := testConfig()
	p := NewOrchestrationPolicy(cfg)

	// Increment some sessions
	p.IncrementSession("space-1")
	p.IncrementSession("space-1")

	status := p.GetOrchestrationStatus(time.Now().Add(time.Hour))

	if status["micro_enabled"] != true {
		t.Error("micro_enabled should be true")
	}
	if status["meso_period_sessions"] != 5 {
		t.Error("meso_period_sessions should be 5")
	}
	if status["cooldown_sec"] != 300 {
		t.Error("cooldown_sec should be 300")
	}
	if status["dedupe_sec"] != 600 {
		t.Error("dedupe_sec should be 600")
	}

	scheduler, ok := status["scheduler"].(map[string]any)
	if !ok {
		t.Fatal("scheduler should be a map")
	}
	if scheduler["enabled"] != true {
		t.Error("scheduler.enabled should be true")
	}
	if _, ok := scheduler["macro_next_run"]; !ok {
		t.Error("scheduler should have macro_next_run")
	}

	counters, ok := status["session_counters"].([]map[string]any)
	if !ok || len(counters) == 0 {
		t.Fatal("session_counters should have entries")
	}
	if counters[0]["count"] != 2 {
		t.Errorf("session counter count = %v, want 2", counters[0]["count"])
	}
}

func TestCheckDedupe_ReturnsDedupeResult(t *testing.T) {
	p := NewOrchestrationPolicy(testConfig())

	// No dedupe for empty key
	if r := p.CheckDedupe(""); r != nil {
		t.Error("empty key should return nil")
	}

	// No dedupe for unknown key
	if r := p.CheckDedupe("unknown"); r != nil {
		t.Error("unknown key should return nil")
	}

	// Record a trigger with key
	d := p.EvaluateTrigger(TriggerManualAPI, "space-1", TierMeso, "my-key")
	p.RecordTrigger(d.Meta, "space-1", TierMeso, "cycle-1")

	// Now should find it
	r := p.CheckDedupe("my-key")
	if r == nil {
		t.Fatal("should find dedupe for recorded key")
	}
	if !r.IsDuplicate {
		t.Error("IsDuplicate should be true")
	}
	if r.OriginalCycleID != "cycle-1" {
		t.Errorf("OriginalCycleID = %q, want %q", r.OriginalCycleID, "cycle-1")
	}
}

func TestHigherPriorityActive_BlocksLowerPriority(t *testing.T) {
	cfg := testConfig()
	cfg.RSICTriggerCooldownSec = 0
	p := NewOrchestrationPolicy(cfg)

	// Start a watchdog_force cycle (highest priority)
	d1 := p.EvaluateTrigger(TriggerWatchdogForce, "space-1", TierMeso, "")
	if !d1.Allowed {
		t.Fatal("watchdog trigger should be allowed")
	}
	p.RecordTrigger(d1.Meta, "space-1", TierMeso, "cycle-wd")

	// micro_auto for same space should be blocked (lower priority, active higher priority)
	d2 := p.EvaluateTrigger(TriggerMicroAuto, "space-1", TierMicro, "")
	if d2.Allowed {
		t.Error("lower priority source should be blocked when higher priority is active for same space")
	}

	// Different space should still be allowed
	d3 := p.EvaluateTrigger(TriggerMicroAuto, "space-2", TierMicro, "")
	if !d3.Allowed {
		t.Errorf("different space should be allowed, got reason: %s", d3.Reason)
	}
}

func TestStaleActiveCycle_CleanedUpAutomatically(t *testing.T) {
	cfg := testConfig()
	cfg.RSICTriggerCooldownSec = 0
	p := NewOrchestrationPolicy(cfg)

	// Simulate a stale active cycle by directly inserting into the map
	p.mu.Lock()
	p.activeCycles["space-1:meso"] = TriggerRecord{
		Source:    TriggerManualAPI,
		SpaceID:   "space-1",
		Tier:      TierMeso,
		CycleID:   "cycle-stale",
		Timestamp: time.Now().Add(-31 * time.Minute), // older than 30 min
	}
	p.mu.Unlock()

	// Should be auto-cleaned during evaluation
	d := p.EvaluateTrigger(TriggerManualAPI, "space-1", TierMeso, "")
	if !d.Allowed {
		t.Errorf("stale active cycle should be cleaned up, got reason: %s", d.Reason)
	}
}
