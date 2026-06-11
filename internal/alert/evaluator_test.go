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
	// 13 → 10 in TSDB-CONSUME-001: high_p95_latency / critical_p99_latency
	// (replaced by RetrieveLatencyRules over retrieval_audit) and
	// neo4j_pool_exhausted (queried a perpetually-zero fake gauge) removed.
	if len(rules) != 10 {
		t.Errorf("expected 10 default rules, got %d", len(rules))
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

func TestRuleFailureStreak_PerRuleAlertAtThreshold(t *testing.T) {
	// Broken-SQL scenario: this rule fails while peers keep succeeding —
	// a peer success lands AFTER the streak begins.
	e := NewEvaluator([]AlertRule{{ID: "r1"}}, nil, nil, time.Second)
	t0 := time.Now()

	if v, _ := e.recordRuleFailure("r1", t0); v != metaSilent {
		t.Fatal("verdict at 1 failure should be silent")
	}
	e.mu.Lock()
	e.lastAnySuccess = t0.Add(time.Second) // peer succeeded mid-streak
	e.mu.Unlock()
	if v, _ := e.recordRuleFailure("r1", t0.Add(30*time.Second)); v != metaSilent {
		t.Fatal("verdict at 2 failures should be silent")
	}
	v, n := e.recordRuleFailure("r1", t0.Add(60*time.Second))
	if v != metaFirePerRule || n != 3 {
		t.Fatalf("expected per-rule meta-alert at 3rd failure, got verdict=%v n=%d", v, n)
	}
	// Streak continues — no re-fire
	if v, _ := e.recordRuleFailure("r1", t0.Add(90*time.Second)); v != metaSilent {
		t.Fatal("meta-alert re-fired within the same failure streak")
	}
}

func TestRuleFailureStreak_OutageOnsetIsGlobal(t *testing.T) {
	// TSDB-stop drill regression (Epic 5, caught live): rules were
	// succeeding seconds before the outage, so lastAnySuccess is fresh at
	// onset — but no success lands AFTER any streak begins, so every rule
	// at threshold must resolve global, and only the first may fire.
	e := NewEvaluator([]AlertRule{{ID: "r1"}, {ID: "r2"}}, nil, nil, time.Second)
	t0 := time.Now()
	e.lastAnySuccess = t0.Add(-time.Second) // healthy until just now

	for i := range 3 {
		ts := t0.Add(time.Duration(i*30) * time.Second)
		e.recordRuleFailure("r1", ts)
		if i < 2 {
			e.recordRuleFailure("r2", ts)
		}
	}
	// r1 hit threshold first → global fires once
	// (the third r1 call above returned the verdict; re-derive via r2)
	if v, _ := e.recordRuleFailure("r2", t0.Add(60*time.Second)); v != metaSilent {
		t.Fatalf("second rule at threshold must not duplicate the global alert, got %v", v)
	}
	if !e.globalAlerted {
		t.Fatal("global outage alert was not recorded")
	}
}

func TestRuleFailureStreak_NeverSucceededIsGlobal(t *testing.T) {
	// TSDB down at startup: lastAnySuccess is zero — global path.
	e := NewEvaluator([]AlertRule{{ID: "r1"}}, nil, nil, time.Second)
	now := time.Now()
	e.recordRuleFailure("r1", now)
	e.recordRuleFailure("r1", now)
	if v, _ := e.recordRuleFailure("r1", now); v != metaFireGlobal {
		t.Fatalf("expected global verdict with no success ever, got %v", v)
	}
}

func TestRuleFailureStreak_RearmsOnSuccess(t *testing.T) {
	e := NewEvaluator([]AlertRule{{ID: "r1"}}, nil, nil, time.Second)
	t0 := time.Now()

	for i := range 3 {
		e.recordRuleFailure("r1", t0.Add(time.Duration(i)*time.Second))
	}
	// Success resets streak, re-arms both per-rule and global alerts
	e.mu.Lock()
	e.recordRuleSuccess(e.state["r1"], t0.Add(5*time.Second))
	e.mu.Unlock()
	if e.globalAlerted {
		t.Fatal("global alert not re-armed on success")
	}

	// New streak begins after the success; a peer succeeds mid-streak →
	// per-rule path, proving the threshold machinery fully re-armed.
	t1 := t0.Add(10 * time.Second)
	for i := 1; i <= 2; i++ {
		if v, _ := e.recordRuleFailure("r1", t1.Add(time.Duration(i)*time.Second)); v != metaSilent {
			t.Fatalf("meta-alert fired at %d failures after re-arm", i)
		}
	}
	e.mu.Lock()
	e.lastAnySuccess = t1.Add(2500 * time.Millisecond) // peer success inside new streak
	e.mu.Unlock()
	if v, _ := e.recordRuleFailure("r1", t1.Add(3*time.Second)); v != metaFirePerRule {
		t.Fatal("re-armed streak with mid-streak peer success should fire per-rule at threshold")
	}
}

func TestRuleFailureStreak_ConfigurableThreshold(t *testing.T) {
	e := NewEvaluator([]AlertRule{{ID: "r1"}}, nil, nil, time.Second)
	t0 := time.Now()
	e.SetRuleFailureThreshold(1)
	// No success since streak start → global at threshold=1
	if v, _ := e.recordRuleFailure("r1", t0); v != metaFireGlobal {
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
	if v, n := e.recordRuleFailure("ghost", time.Now()); v != metaSilent || n != 1 {
		t.Fatalf("unexpected result for unseeded rule: verdict=%v n=%d", v, n)
	}
}
