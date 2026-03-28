package ape

import (
	"context"
	"testing"

	"mdemg/internal/config"
)

func TestReflect_GuidanceConfidenceDrift(t *testing.T) {
	cfg := config.Config{}
	r := NewReflector(cfg, nil)

	// Pattern 15 should trigger when GuidanceHealth is between 0 and 0.7
	report := &SelfAssessmentReport{
		SpaceID:        "test",
		GuidanceHealth: 0.55,
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, insight := range insights {
		if insight.PatternID == "guidance_confidence_drift" {
			found = true
			if insight.RecommendedAction != "adjust_guidance_confidence" {
				t.Errorf("expected action adjust_guidance_confidence, got %s", insight.RecommendedAction)
			}
			if insight.Value != 0.55 {
				t.Errorf("expected value 0.55, got %f", insight.Value)
			}
			if insight.Threshold != 0.7 {
				t.Errorf("expected threshold 0.7, got %f", insight.Threshold)
			}
		}
	}
	if !found {
		t.Error("guidance_confidence_drift pattern not triggered with GuidanceHealth=0.55")
	}

	// Should NOT trigger when GuidanceHealth >= 0.7
	report.GuidanceHealth = 0.75
	insights, _ = r.Reflect(context.Background(), report)
	for _, insight := range insights {
		if insight.PatternID == "guidance_confidence_drift" {
			t.Error("guidance_confidence_drift should not trigger with GuidanceHealth=0.75")
		}
	}

	// Should NOT trigger when GuidanceHealth == 0 (no data)
	report.GuidanceHealth = 0
	insights, _ = r.Reflect(context.Background(), report)
	for _, insight := range insights {
		if insight.PatternID == "guidance_confidence_drift" {
			t.Error("guidance_confidence_drift should not trigger with GuidanceHealth=0")
		}
	}
}

func TestReflect_RecoveryBufferPending_Medium(t *testing.T) {
	cfg := config.Config{SynergyAssessmentEnabled: true}
	r := NewReflector(cfg, nil)

	report := &SelfAssessmentReport{
		SpaceID:                      "test",
		SynergyRecoveryBufferEntries: 5,
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, insight := range insights {
		if insight.PatternID == "synergy_recovery_buffer_pending" {
			found = true
			if insight.Severity != SeverityMedium {
				t.Errorf("expected medium severity, got %s", insight.Severity)
			}
			if insight.RecommendedAction != "flush_recovery_buffer" {
				t.Errorf("expected action flush_recovery_buffer, got %s", insight.RecommendedAction)
			}
		}
	}
	if !found {
		t.Error("synergy_recovery_buffer_pending pattern not triggered with 5 entries")
	}
}

func TestReflect_RecoveryBufferPending_High(t *testing.T) {
	cfg := config.Config{SynergyAssessmentEnabled: true}
	r := NewReflector(cfg, nil)

	report := &SelfAssessmentReport{
		SpaceID:                      "test",
		SynergyRecoveryBufferEntries: 25,
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, insight := range insights {
		if insight.PatternID == "synergy_recovery_buffer_pending" {
			found = true
			if insight.Severity != SeverityHigh {
				t.Errorf("expected high severity for >20 entries, got %s", insight.Severity)
			}
		}
	}
	if !found {
		t.Error("synergy_recovery_buffer_pending pattern not triggered with 25 entries")
	}
}

func TestReflect_RecoveryBufferEmpty_NoInsight(t *testing.T) {
	cfg := config.Config{SynergyAssessmentEnabled: true}
	r := NewReflector(cfg, nil)

	report := &SelfAssessmentReport{
		SpaceID:                      "test",
		SynergyRecoveryBufferEntries: 0,
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, insight := range insights {
		if insight.PatternID == "synergy_recovery_buffer_pending" {
			t.Error("synergy_recovery_buffer_pending should not trigger with 0 entries")
		}
	}
}

func TestReflect_LowGuidanceFollowRate(t *testing.T) {
	cfg := config.Config{}
	r := NewReflector(cfg, nil)

	// Pattern 9 triggers at < 0.5
	report := &SelfAssessmentReport{
		SpaceID:        "test",
		GuidanceHealth: 0.4,
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both pattern 9 (review) and pattern 15 (adjust) should trigger
	var foundReview, foundAdjust bool
	for _, insight := range insights {
		if insight.PatternID == "low_guidance_follow_rate" {
			foundReview = true
		}
		if insight.PatternID == "guidance_confidence_drift" {
			foundAdjust = true
		}
	}
	if !foundReview {
		t.Error("expected low_guidance_follow_rate pattern to trigger")
	}
	if !foundAdjust {
		t.Error("expected guidance_confidence_drift pattern to trigger")
	}
}

func TestReflect_SidecarUnhealthy_JiminyActive(t *testing.T) {
	cfg := config.Config{}
	r := NewReflector(cfg, nil)

	report := &SelfAssessmentReport{
		SpaceID:        "test",
		JiminyHealthy:  true,
		SidecarHealthy: false,
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var found bool
	for _, insight := range insights {
		if insight.PatternID == "sidecar_unhealthy" {
			found = true
			if insight.Severity != SeverityHigh {
				t.Errorf("expected HIGH severity, got %s", insight.Severity)
			}
			if insight.RecommendedAction != "alert_sidecar_down" {
				t.Errorf("expected alert_sidecar_down action, got %s", insight.RecommendedAction)
			}
		}
	}
	if !found {
		t.Error("expected sidecar_unhealthy pattern to trigger")
	}
}

func TestReflect_SidecarUnhealthy_WithActiveProtocol(t *testing.T) {
	cfg := config.Config{}
	r := NewReflector(cfg, nil)

	report := &SelfAssessmentReport{
		SpaceID:        "test",
		JiminyHealthy:  true,
		SidecarHealthy: false,
		ProtocolHealth: 0.6, // active J17 traffic
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var found bool
	for _, insight := range insights {
		if insight.PatternID == "sidecar_unhealthy" {
			found = true
			if insight.Severity != SeverityCritical {
				t.Errorf("expected CRITICAL severity with active protocol, got %s", insight.Severity)
			}
		}
	}
	if !found {
		t.Error("expected sidecar_unhealthy pattern to trigger")
	}
}

func TestReflect_SidecarHealthy_NoPattern(t *testing.T) {
	cfg := config.Config{}
	r := NewReflector(cfg, nil)

	report := &SelfAssessmentReport{
		SpaceID:        "test",
		JiminyHealthy:  true,
		SidecarHealthy: true,
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, insight := range insights {
		if insight.PatternID == "sidecar_unhealthy" {
			t.Error("sidecar_unhealthy should not trigger when sidecar is healthy")
		}
	}
}
