package jiminy

import (
	"testing"
	"time"
)

func TestCodeComprehensionTracker_BelowThresholdTriggersRegen(t *testing.T) {
	tracker := NewCodeComprehensionTracker(0.3, 5, 10, time.Hour, nil)

	// Record 5 low-comprehension scores (all 0.1, avg = 0.1 < 0.3 threshold)
	for i := 0; i < 4; i++ {
		if tracker.Record("test-code", 0.1) {
			t.Fatalf("should not trigger regen before minSamples, got true at sample %d", i+1)
		}
	}

	// 5th sample should trigger
	if !tracker.Record("test-code", 0.1) {
		t.Fatal("expected regen trigger after minSamples with low avg")
	}
}

func TestCodeComprehensionTracker_AboveThresholdNoRegen(t *testing.T) {
	tracker := NewCodeComprehensionTracker(0.3, 5, 10, time.Hour, nil)

	// Record 10 high-comprehension scores
	for i := 0; i < 10; i++ {
		if tracker.Record("good-code", 0.8) {
			t.Fatal("should not trigger regen for high comprehension")
		}
	}
}

func TestCodeComprehensionTracker_Cooldown(t *testing.T) {
	// Use a very long cooldown so it never expires in this test
	tracker := NewCodeComprehensionTracker(0.3, 3, 10, 24*time.Hour, nil)

	// First trigger
	for i := 0; i < 2; i++ {
		tracker.Record("cooldown-code", 0.1)
	}
	if !tracker.Record("cooldown-code", 0.1) {
		t.Fatal("first trigger should fire")
	}

	// Second trigger should be blocked by cooldown
	for i := 0; i < 3; i++ {
		if tracker.Record("cooldown-code", 0.1) {
			t.Fatal("should not trigger during cooldown period")
		}
	}
}

func TestCodeComprehensionTracker_RateLimit(t *testing.T) {
	tracker := NewCodeComprehensionTracker(0.3, 3, 2, 0, nil) // maxPerHour=2, no cooldown

	// Trigger regen for first code
	for i := 0; i < 2; i++ {
		tracker.Record("code-a", 0.1)
	}
	if !tracker.Record("code-a", 0.1) {
		t.Fatal("first regen should succeed")
	}

	// Trigger regen for second code
	for i := 0; i < 2; i++ {
		tracker.Record("code-b", 0.1)
	}
	if !tracker.Record("code-b", 0.1) {
		t.Fatal("second regen should succeed")
	}

	// Third regen should be rate-limited
	for i := 0; i < 2; i++ {
		tracker.Record("code-c", 0.1)
	}
	if tracker.Record("code-c", 0.1) {
		t.Fatal("third regen should be rate-limited")
	}
}

func TestCodeComprehensionTracker_EmptyCode(t *testing.T) {
	tracker := NewCodeComprehensionTracker(0.3, 3, 10, time.Hour, nil)
	if tracker.Record("", 0.1) {
		t.Fatal("empty constraint code should return false")
	}
}
