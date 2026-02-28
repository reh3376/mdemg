package cli

import (
	"strings"
	"testing"

	"mdemg/internal/sidecar"
)

// --- extractPort ---

func TestExtractPort(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"normal_port", "http://localhost:9999", 9999},
		{"custom_port", "http://macstudio-tb:8080", 8080},
		{"default_no_port", "http://localhost", 9999},
		{"empty_string", "", 9999},
		{"bad_url", "not-a-url", 9999},
		{"zero_port", "http://host:0", 9999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPort(tt.input)
			if got != tt.expected {
				t.Errorf("extractPort(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

// --- doctorStatusIcon ---

func TestDoctorStatusIcon(t *testing.T) {
	tests := []struct {
		status string
		icon   string
	}{
		{"pass", "✓"},
		{"fail", "✗"},
		{"warn", "!"},
		{"skip", "-"},
		{"unknown", "?"},
		{"", "?"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := doctorStatusIcon(tt.status)
			if got != tt.icon {
				t.Errorf("doctorStatusIcon(%q) = %q, want %q", tt.status, got, tt.icon)
			}
		})
	}
}

// --- doctorNextActions ---

func TestDoctorNextActions_AllPass(t *testing.T) {
	checks := []sidecar.DoctorCheck{
		{ID: "a", Status: "pass", Remediation: "fix a"},
		{ID: "b", Status: "pass", Remediation: "fix b"},
		{ID: "c", Status: "warn", Remediation: "fix c"},
	}
	actions := doctorNextActions(checks)
	if len(actions) != 1 || actions[0] != "All checks passed" {
		t.Errorf("expected [All checks passed], got %v", actions)
	}
}

func TestDoctorNextActions_SomeFail(t *testing.T) {
	checks := []sidecar.DoctorCheck{
		{ID: "a", Status: "pass", Remediation: "fix a"},
		{ID: "b", Status: "fail", Remediation: "fix b"},
		{ID: "c", Status: "fail", Remediation: "fix c"},
		{ID: "d", Status: "fail", Remediation: ""},
	}
	actions := doctorNextActions(checks)
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d: %v", len(actions), actions)
	}
	if actions[0] != "fix b" {
		t.Errorf("actions[0] = %q, want %q", actions[0], "fix b")
	}
	if actions[1] != "fix c" {
		t.Errorf("actions[1] = %q, want %q", actions[1], "fix c")
	}
}

// --- runConfigCheck ---

func TestRunConfigCheck_NoPath(t *testing.T) {
	check := runConfigCheck("", nil)
	if check.Status != "fail" {
		t.Errorf("status = %q, want fail", check.Status)
	}
	if check.ID != "config.valid" {
		t.Errorf("id = %q, want config.valid", check.ID)
	}
	if !strings.Contains(check.Message, "No sidecar.yaml") {
		t.Errorf("message = %q, want 'No sidecar.yaml' substring", check.Message)
	}
}

func TestRunConfigCheck_NilConfig(t *testing.T) {
	check := runConfigCheck("/some/path", nil)
	if check.Status != "fail" {
		t.Errorf("status = %q, want fail", check.Status)
	}
	if !strings.Contains(check.Message, "parse") {
		t.Errorf("message = %q, want 'parse' substring", check.Message)
	}
}

func TestRunConfigCheck_Valid(t *testing.T) {
	cfg := sidecar.GenerateConfig(sidecar.ProfileLocal, []sidecar.AdapterName{sidecar.AdapterClaudeCode}, "", "")
	check := runConfigCheck("/some/path", cfg)
	if check.Status != "pass" {
		t.Errorf("status = %q, want pass", check.Status)
	}
	if !strings.Contains(check.Message, "valid") {
		t.Errorf("message = %q, want 'valid' substring", check.Message)
	}
}

func TestRunConfigCheck_Invalid(t *testing.T) {
	cfg := sidecar.GenerateConfig(sidecar.ProfileLocal, nil, "", "")
	cfg.Version = "99"
	check := runConfigCheck("/some/path", cfg)
	if check.Status != "fail" {
		t.Errorf("status = %q, want fail", check.Status)
	}
	if !strings.Contains(check.Message, "Validation failed") {
		t.Errorf("message = %q, want 'Validation failed' substring", check.Message)
	}
	if len(check.Evidence) == 0 {
		t.Error("expected non-empty evidence for validation failure")
	}
}

// --- generateSessionStartScript ---

func TestGenerateSessionStartScript_Shebang(t *testing.T) {
	script := generateSessionStartScript("http://localhost:9999", "my-space", "my-session")
	if !strings.HasPrefix(script, "#!/usr/bin/env bash") {
		t.Errorf("script does not start with shebang, got prefix: %q", script[:50])
	}
}

func TestGenerateSessionStartScript_Content(t *testing.T) {
	script := generateSessionStartScript("http://test:8080", "test-space", "test-session")
	for _, want := range []string{"http://test:8080", "test-space", "test-session"} {
		if !strings.Contains(script, want) {
			t.Errorf("script missing %q", want)
		}
	}
}

// --- buildHealthSummary ---

func TestBuildHealthSummary_AllHealthy(t *testing.T) {
	services := []sidecar.ServiceStatus{
		{Name: "api", Status: "up", Healthy: true},
		{Name: "neo4j", Status: "up", Healthy: true},
	}
	summary := buildHealthSummary(services)
	if !summary.Healthy {
		t.Error("expected Healthy=true")
	}
	if summary.ServiceCountTotal != 2 {
		t.Errorf("total = %d, want 2", summary.ServiceCountTotal)
	}
	if summary.ServiceCountUp != 2 {
		t.Errorf("up = %d, want 2", summary.ServiceCountUp)
	}
	if len(summary.DegradedReasons) != 0 {
		t.Errorf("expected empty DegradedReasons, got %v", summary.DegradedReasons)
	}
}

func TestBuildHealthSummary_SomeDown(t *testing.T) {
	services := []sidecar.ServiceStatus{
		{Name: "api", Status: "up", Healthy: true},
		{Name: "neo4j", Status: "down", Healthy: false},
		{Name: "embedder", Status: "down", Healthy: false},
	}
	summary := buildHealthSummary(services)
	if summary.Healthy {
		t.Error("expected Healthy=false")
	}
	if summary.ServiceCountTotal != 3 {
		t.Errorf("total = %d, want 3", summary.ServiceCountTotal)
	}
	if summary.ServiceCountUp != 1 {
		t.Errorf("up = %d, want 1", summary.ServiceCountUp)
	}
	if len(summary.DegradedReasons) != 2 {
		t.Fatalf("expected 2 degraded reasons, got %d: %v", len(summary.DegradedReasons), summary.DegradedReasons)
	}
	if !strings.Contains(summary.DegradedReasons[0], "neo4j") {
		t.Errorf("reason[0] = %q, want neo4j mention", summary.DegradedReasons[0])
	}
}

func TestBuildHealthSummary_Empty(t *testing.T) {
	summary := buildHealthSummary(nil)
	if !summary.Healthy {
		t.Error("expected Healthy=true for empty services")
	}
	if summary.DegradedReasons == nil {
		t.Error("DegradedReasons should not be nil (should be empty slice)")
	}
}

// --- isEmptyConfig ---

func TestIsEmptyConfig_JSON(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected bool
	}{
		{"empty_object_newline", "{}\n", true},
		{"empty_object", "{}", true},
		{"non_empty", `{"key":"val"}`, false},
		{"array", "[]", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEmptyConfig([]byte(tt.data), "json")
			if got != tt.expected {
				t.Errorf("isEmptyConfig(%q, json) = %v, want %v", tt.data, got, tt.expected)
			}
		})
	}
}

func TestIsEmptyConfig_TOML(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected bool
	}{
		{"whitespace_only", " \t\n\r", true},
		{"empty", "", true},
		{"has_content", "[section]\nkey=val", false},
		{"has_key", "key = true", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEmptyConfig([]byte(tt.data), "toml")
			if got != tt.expected {
				t.Errorf("isEmptyConfig(%q, toml) = %v, want %v", tt.data, got, tt.expected)
			}
		})
	}
}

func TestIsEmptyConfig_Default(t *testing.T) {
	if !isEmptyConfig(nil, "yaml") {
		t.Error("nil data should be empty for yaml format")
	}
	if !isEmptyConfig([]byte{}, "unknown") {
		t.Error("empty data should be empty for unknown format")
	}
	if isEmptyConfig([]byte("data"), "yaml") {
		t.Error("non-empty data should not be empty for yaml format")
	}
}
