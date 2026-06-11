package alert

import (
	"strings"
	"testing"
)

func TestHookHealthRules_Defaults(t *testing.T) {
	rules := HookHealthRules(0, 0) // zero → defaults 24h / 5 events
	if len(rules) != 1 {
		t.Fatalf("want 1 rule, got %d", len(rules))
	}
	r := rules[0]
	if r.ID != "hook_channel_silent" {
		t.Errorf("ID = %q", r.ID)
	}
	// Distinct Service per the NOSILENT-001 cooldown-collision rule.
	if r.Service != "hook-channel-silent" {
		t.Errorf("Service = %q — must be unique per rule", r.Service)
	}
	if r.Severity != SeverityHigh || !r.Enabled {
		t.Errorf("severity/enabled = %v/%v", r.Severity, r.Enabled)
	}
	for _, want := range []string{"hook:post-tool-observe", "hook:prompt-context", "'24 hours'", ">= 5"} {
		if !strings.Contains(r.QuerySQL, want) {
			t.Errorf("QuerySQL missing %q", want)
		}
	}
	if r.Threshold != 0 || r.Operator != "gt" {
		t.Errorf("threshold/op = %v/%v", r.Threshold, r.Operator)
	}
}

func TestHookHealthRules_CustomParams(t *testing.T) {
	r := HookHealthRules(6, 10)[0]
	for _, want := range []string{"'6 hours'", ">= 10"} {
		if !strings.Contains(r.QuerySQL, want) {
			t.Errorf("QuerySQL missing %q", want)
		}
	}
}
