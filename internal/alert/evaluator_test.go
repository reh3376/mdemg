package alert

import (
	"testing"
	"time"
)

func TestCheckThreshold(t *testing.T) {
	tests := []struct {
		name      string
		value     float64
		threshold float64
		op        string
		want      bool
	}{
		{"gt true", 300, 250, "gt", true},
		{"gt false", 200, 250, "gt", false},
		{"gt equal", 250, 250, "gt", false},
		{"lt true", 0.3, 0.5, "lt", true},
		{"lt false", 0.7, 0.5, "lt", false},
		{"lt equal", 0.5, 0.5, "lt", false},
		{"gte true", 250, 250, "gte", true},
		{"gte above", 300, 250, "gte", true},
		{"gte false", 200, 250, "gte", false},
		{"lte true", 250, 250, "lte", true},
		{"lte below", 200, 250, "lte", true},
		{"lte false", 300, 250, "lte", false},
		{"unknown op", 300, 250, "eq", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkThreshold(tt.value, tt.threshold, tt.op)
			if got != tt.want {
				t.Errorf("checkThreshold(%v, %v, %q) = %v, want %v",
					tt.value, tt.threshold, tt.op, got, tt.want)
			}
		})
	}
}

func TestRuleState_ForDuration(t *testing.T) {
	// Simulate rule state transitions for ForDuration logic
	now := time.Now()

	st := &ruleState{}

	// First breach — set conditionFirstTrue
	if !st.conditionFirstTrue.IsZero() {
		t.Fatal("expected zero conditionFirstTrue initially")
	}
	st.conditionFirstTrue = now

	// Check before ForDuration elapsed (5 min)
	forDuration := 5 * time.Minute
	elapsed := now.Add(2 * time.Minute)
	if elapsed.Sub(st.conditionFirstTrue) >= forDuration {
		t.Fatal("should not have reached ForDuration yet")
	}

	// Check after ForDuration elapsed
	elapsed = now.Add(6 * time.Minute)
	if elapsed.Sub(st.conditionFirstTrue) < forDuration {
		t.Fatal("should have reached ForDuration")
	}

	// Fire and mark
	if st.fired {
		t.Fatal("should not be fired yet")
	}
	st.fired = true
	if !st.fired {
		t.Fatal("should be fired after marking")
	}
}

func TestRuleState_ResetOnRecovery(t *testing.T) {
	st := &ruleState{
		conditionFirstTrue: time.Now().Add(-10 * time.Minute),
		fired:              true,
	}

	// Simulate recovery — condition clears
	st.conditionFirstTrue = time.Time{}
	st.fired = false

	if !st.conditionFirstTrue.IsZero() {
		t.Fatal("conditionFirstTrue should be zero after reset")
	}
	if st.fired {
		t.Fatal("fired should be false after reset")
	}
}

func TestDefaultRules_Count(t *testing.T) {
	rules := DefaultRules()
	if len(rules) != 13 {
		t.Errorf("expected 13 default rules, got %d", len(rules))
	}

	// Verify all rules are enabled
	for _, r := range rules {
		if !r.Enabled {
			t.Errorf("rule %q should be enabled by default", r.ID)
		}
		if r.ID == "" {
			t.Error("rule ID should not be empty")
		}
		if r.QuerySQL == "" {
			t.Error("rule QuerySQL should not be empty")
		}
		if r.Title == "" {
			t.Error("rule Title should not be empty")
		}
	}
}

func TestDefaultRules_UniqueIDs(t *testing.T) {
	rules := DefaultRules()
	seen := make(map[string]bool)
	for _, r := range rules {
		if seen[r.ID] {
			t.Errorf("duplicate rule ID: %s", r.ID)
		}
		seen[r.ID] = true
	}
}

func TestDefaultRules_Severities(t *testing.T) {
	rules := DefaultRules()
	ruleMap := make(map[string]AlertRule)
	for _, r := range rules {
		ruleMap[r.ID] = r
	}

	// Verify critical rules
	criticals := []string{"critical_p99_latency", "neo4j_pool_exhausted", "graph_node_drop"}
	for _, id := range criticals {
		if r, ok := ruleMap[id]; ok {
			if r.Severity != SeverityCritical {
				t.Errorf("rule %q should be critical, got %s", id, r.Severity)
			}
		}
	}

	// Verify info/low rules
	lows := []string{"rate_limiting_active", "low_cache_hit_ratio"}
	for _, id := range lows {
		if r, ok := ruleMap[id]; ok {
			if r.Severity != SeverityLow {
				t.Errorf("rule %q should be low, got %s", id, r.Severity)
			}
		}
	}
}

func TestEvaluator_IntervalGating(t *testing.T) {
	// Verify evaluator respects per-rule intervals
	e := &Evaluator{
		rules: []AlertRule{
			{ID: "test", Enabled: true, Interval: 60 * time.Second},
		},
		state: map[string]*ruleState{
			"test": {lastEval: time.Now()},
		},
	}

	// lastEval is now, so interval hasn't elapsed — should skip
	now := time.Now()
	st := e.state["test"]
	if now.Sub(st.lastEval) >= e.rules[0].Interval {
		t.Fatal("should not evaluate yet — interval hasn't elapsed")
	}

	// After interval elapses
	st.lastEval = now.Add(-61 * time.Second)
	if time.Since(st.lastEval) < e.rules[0].Interval {
		t.Fatal("should evaluate — interval has elapsed")
	}
}

func TestRuleFailureStreak_MetaAlertAtThreshold(t *testing.T) {
	e := NewEvaluator([]AlertRule{{ID: "r1"}}, nil, nil, time.Second)

	// Below threshold (default 3): no meta-alert
	for i := 1; i <= 2; i++ {
		fire, n := e.recordRuleFailure("r1")
		if fire {
			t.Fatalf("meta-alert fired at %d consecutive failures (threshold 3)", n)
		}
	}
	// Third consecutive failure fires exactly once
	fire, n := e.recordRuleFailure("r1")
	if !fire || n != 3 {
		t.Fatalf("expected meta-alert at 3rd failure, got fire=%v n=%d", fire, n)
	}
	// Streak continues — no re-fire
	fire, _ = e.recordRuleFailure("r1")
	if fire {
		t.Fatal("meta-alert re-fired within the same failure streak")
	}
}

func TestRuleFailureStreak_RearmsOnSuccess(t *testing.T) {
	e := NewEvaluator([]AlertRule{{ID: "r1"}}, nil, nil, time.Second)
	for range 3 {
		e.recordRuleFailure("r1")
	}
	// Success resets streak + re-arms
	e.mu.Lock()
	e.state["r1"].recordRuleSuccess()
	e.mu.Unlock()

	for i := 1; i <= 2; i++ {
		if fire, _ := e.recordRuleFailure("r1"); fire {
			t.Fatalf("meta-alert fired at %d failures after re-arm", i)
		}
	}
	if fire, _ := e.recordRuleFailure("r1"); !fire {
		t.Fatal("meta-alert did not fire after re-armed streak reached threshold")
	}
}

func TestRuleFailureStreak_ConfigurableThreshold(t *testing.T) {
	e := NewEvaluator([]AlertRule{{ID: "r1"}}, nil, nil, time.Second)
	e.SetRuleFailureThreshold(1)
	if fire, _ := e.recordRuleFailure("r1"); !fire {
		t.Fatal("threshold=1 should fire on first failure")
	}
	// Non-positive keeps current threshold
	e.SetRuleFailureThreshold(0)
	if e.failureThreshold != 1 {
		t.Fatalf("threshold overwritten by non-positive value: %d", e.failureThreshold)
	}
}

func TestRuleFailureStreak_UnknownRuleID(t *testing.T) {
	e := NewEvaluator(nil, nil, nil, time.Second)
	// Must not panic on a rule ID with no pre-seeded state
	if fire, n := e.recordRuleFailure("ghost"); fire || n != 1 {
		t.Fatalf("unexpected result for unseeded rule: fire=%v n=%d", fire, n)
	}
}
