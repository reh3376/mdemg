package cli

import "testing"

func TestCheckResult_Passed(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"PASS", true},
		{"FAIL", false},
		{"WARN", false},
	}
	for _, tt := range tests {
		cr := checkResult{Name: "test", Status: tt.status}
		if got := cr.passed(); got != tt.want {
			t.Errorf("checkResult{Status: %q}.passed() = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestCheckEventLogging(t *testing.T) {
	// Test with TSDB_ENABLED=true (should pass)
	t.Setenv("TSDB_ENABLED", "true")
	t.Setenv("LLM_INTERACTION_LOGGING", "true")

	result := checkEventLogging()
	if result.Status != "PASS" {
		t.Errorf("checkEventLogging with TSDB_ENABLED=true: status=%q, want PASS", result.Status)
	}

	// Test with TSDB_ENABLED missing (should fail)
	t.Setenv("TSDB_ENABLED", "")
	result = checkEventLogging()
	if result.Status != "FAIL" {
		t.Errorf("checkEventLogging with TSDB_ENABLED='': status=%q, want FAIL", result.Status)
	}

	// Test with LLM_INTERACTION_LOGGING=false (should fail)
	t.Setenv("TSDB_ENABLED", "true")
	t.Setenv("LLM_INTERACTION_LOGGING", "false")
	result = checkEventLogging()
	if result.Status != "FAIL" {
		t.Errorf("checkEventLogging with LLM_INTERACTION_LOGGING=false: status=%q, want FAIL", result.Status)
	}
}

func TestCheckCampaignFlags(t *testing.T) {
	// All disabled — should warn
	t.Setenv("QUERY_CLASSIFY_ENABLED", "")
	t.Setenv("INTENT_ENABLED", "")
	t.Setenv("JIMINY_ENABLED", "")

	result := checkCampaignFlags()
	if result.Status != "WARN" {
		t.Errorf("checkCampaignFlags all disabled: status=%q, want WARN", result.Status)
	}

	// All enabled — should pass
	t.Setenv("QUERY_CLASSIFY_ENABLED", "true")
	t.Setenv("INTENT_ENABLED", "true")
	t.Setenv("JIMINY_ENABLED", "true")

	result = checkCampaignFlags()
	if result.Status != "PASS" {
		t.Errorf("checkCampaignFlags all enabled: status=%q, want PASS", result.Status)
	}
}

func TestReportResults_FailedExitCode(t *testing.T) {
	results := []checkResult{
		{Name: "A", Status: "PASS"},
		{Name: "B", Status: "FAIL", Detail: "broken"},
	}

	err := reportResults(results, true)
	if err == nil {
		t.Error("reportResults with failures should return error")
	}
}

func TestReportResults_AllPassNoError(t *testing.T) {
	results := []checkResult{
		{Name: "A", Status: "PASS"},
		{Name: "B", Status: "PASS"},
		{Name: "C", Status: "WARN"},
	}

	err := reportResults(results, true)
	if err != nil {
		t.Errorf("reportResults with all pass+warn should not error, got: %v", err)
	}
}
