package ape

import (
	"testing"
	"time"

	"mdemg/internal/config"
)

// TestQuiesce_State pins Epic 4: Quiesce/IsQuiesced track the pause window.
func TestQuiesce_State(t *testing.T) {
	p := NewOrchestrationPolicy(config.Config{})
	if p.IsQuiesced() {
		t.Fatal("fresh policy should not be quiesced")
	}
	p.Quiesce(time.Now().Add(time.Hour))
	if !p.IsQuiesced() {
		t.Fatal("should be quiesced after Quiesce(future)")
	}
	p.Quiesce(time.Time{})
	if p.IsQuiesced() {
		t.Error("zero time should clear quiesce")
	}
	// Past time also clears.
	p.Quiesce(time.Now().Add(-time.Hour))
	if p.IsQuiesced() {
		t.Error("past time should not be quiesced")
	}
}
