package alert

import (
	"strings"
	"testing"
)

func findRule(rules []AlertRule, id string) (AlertRule, bool) {
	for _, r := range rules {
		if r.ID == id {
			return r, true
		}
	}
	return AlertRule{}, false
}

func TestJobHealthRules_FailureRuleAlwaysPresent(t *testing.T) {
	rules := JobHealthRules(48, 60, false)
	r, ok := findRule(rules, "scheduled_job_recent_failure")
	if !ok {
		t.Fatal("failure rule must always be present")
	}
	if r.Operator != "gt" || r.Threshold != 0 {
		t.Errorf("failure rule should fire on count > 0, got op=%s thr=%v", r.Operator, r.Threshold)
	}
	if r.Severity != SeverityHigh {
		t.Errorf("failure rule should be high severity")
	}
	// The two job rules must use distinct services so the dispatcher cooldown
	// (keyed on Service+Severity) doesn't let one suppress the other.
	stale, _ := findRule(JobHealthRules(48, 60, true), "backup_no_recent_success")
	if r.Service == stale.Service {
		t.Errorf("failure and staleness rules must have distinct services, both = %q", r.Service)
	}
	if !strings.Contains(r.QuerySQL, "success = false") || !strings.Contains(r.QuerySQL, "60 minutes") {
		t.Errorf("failure rule SQL wrong: %s", r.QuerySQL)
	}
}

func TestJobHealthRules_StalenessGatedOnBackupEnabled(t *testing.T) {
	if _, ok := findRule(JobHealthRules(48, 60, false), "backup_no_recent_success"); ok {
		t.Error("staleness rule must NOT be present when backups disabled")
	}
	rules := JobHealthRules(48, 60, true)
	r, ok := findRule(rules, "backup_no_recent_success")
	if !ok {
		t.Fatal("staleness rule must be present when backups enabled")
	}
	if r.Operator != "lt" || r.Threshold != 0.5 {
		t.Errorf("staleness should fire on count < 0.5, got op=%s thr=%v", r.Operator, r.Threshold)
	}
	if !strings.Contains(r.QuerySQL, "job_name = 'tsdb-backup'") || !strings.Contains(r.QuerySQL, "48 hours") {
		t.Errorf("staleness SQL wrong: %s", r.QuerySQL)
	}
}

func TestJobHealthRules_WindowFromConfig(t *testing.T) {
	rules := JobHealthRules(12, 90, true)
	r, _ := findRule(rules, "backup_no_recent_success")
	if !strings.Contains(r.QuerySQL, "12 hours") {
		t.Errorf("staleness window should reflect config (12h): %s", r.QuerySQL)
	}
	f, _ := findRule(rules, "scheduled_job_recent_failure")
	if !strings.Contains(f.QuerySQL, "90 minutes") {
		t.Errorf("failure lookback should reflect config (90m): %s", f.QuerySQL)
	}
}

func TestJobHealthRules_NonPositiveFallback(t *testing.T) {
	rules := JobHealthRules(0, 0, true)
	r, _ := findRule(rules, "backup_no_recent_success")
	if !strings.Contains(r.QuerySQL, "48 hours") {
		t.Errorf("non-positive staleness should fall back to 48h: %s", r.QuerySQL)
	}
	f, _ := findRule(rules, "scheduled_job_recent_failure")
	if !strings.Contains(f.QuerySQL, "60 minutes") {
		t.Errorf("non-positive lookback should fall back to 60m: %s", f.QuerySQL)
	}
}
