package learning

import (
	"context"
	"testing"

	"mdemg/internal/config"
	"mdemg/internal/models"
)

func TestFreezeLearning(t *testing.T) {
	cfg := config.Config{}
	// Service doesn't need a real driver for freeze tests
	s := NewService(cfg, nil)

	spaceID := "test-space"

	// Initially not frozen
	if s.IsFrozen(spaceID) {
		t.Error("Space should not be frozen initially")
	}

	// Freeze the space (skip DB query by using a nil context or mock)
	ctx := context.Background()
	state, err := s.FreezeLearning(ctx, spaceID, "production deployment", "admin")
	// Will fail to get edge count but that's OK for this test
	_ = err

	if !state.Frozen {
		t.Error("State should be frozen after FreezeLearning")
	}
	if state.Reason != "production deployment" {
		t.Errorf("Reason = %q, want %q", state.Reason, "production deployment")
	}
	if state.FrozenBy != "admin" {
		t.Errorf("FrozenBy = %q, want %q", state.FrozenBy, "admin")
	}

	// Verify IsFrozen
	if !s.IsFrozen(spaceID) {
		t.Error("Space should be frozen after FreezeLearning")
	}

	// Verify GetFreezeState
	state2 := s.GetFreezeState(spaceID)
	if !state2.Frozen {
		t.Error("GetFreezeState should return frozen state")
	}

	// Unfreeze
	state3 := s.UnfreezeLearning(spaceID)
	if state3.Frozen {
		t.Error("State should not be frozen after UnfreezeLearning")
	}

	// Verify no longer frozen
	if s.IsFrozen(spaceID) {
		t.Error("Space should not be frozen after UnfreezeLearning")
	}
}

func TestGetAllFreezeStates(t *testing.T) {
	cfg := config.Config{}
	s := NewService(cfg, nil)

	ctx := context.Background()

	// Freeze multiple spaces
	s.FreezeLearning(ctx, "space-1", "reason-1", "user-1")
	s.FreezeLearning(ctx, "space-2", "reason-2", "user-2")

	states := s.GetAllFreezeStates()
	if len(states) != 2 {
		t.Errorf("GetAllFreezeStates returned %d states, want 2", len(states))
	}

	if !states["space-1"].Frozen {
		t.Error("space-1 should be frozen")
	}
	if !states["space-2"].Frozen {
		t.Error("space-2 should be frozen")
	}

	// Unfreeze one
	s.UnfreezeLearning("space-1")

	states = s.GetAllFreezeStates()
	if len(states) != 1 {
		t.Errorf("GetAllFreezeStates returned %d states after unfreeze, want 1", len(states))
	}
	if states["space-1"].Frozen {
		t.Error("space-1 should not be in frozen states after unfreeze")
	}
}

func TestApplyCoactivation_Frozen(t *testing.T) {
	cfg := config.Config{}
	s := NewService(cfg, nil)

	ctx := context.Background()
	spaceID := "test-space"

	// Freeze the space
	s.FreezeLearning(ctx, spaceID, "test", "test")

	// ApplyCoactivation should return nil immediately when frozen
	// (it won't even try to access the DB because it's frozen)
	err := s.ApplyCoactivation(ctx, spaceID, mockRetrieveResponse())
	if err != nil {
		t.Errorf("ApplyCoactivation on frozen space returned error: %v", err)
	}
}

func TestCoactivateSession_Frozen(t *testing.T) {
	cfg := config.Config{}
	s := NewService(cfg, nil)

	ctx := context.Background()
	spaceID := "test-space"

	// Freeze the space
	s.FreezeLearning(ctx, spaceID, "test", "test")

	// CoactivateSession should return nil immediately when frozen
	err := s.CoactivateSession(ctx, spaceID, "session-123", "n-frozen")
	if err != nil {
		t.Errorf("CoactivateSession on frozen space returned error: %v", err)
	}
}

func mockRetrieveResponse() models.RetrieveResponse {
	return models.RetrieveResponse{
		Results: []models.RetrieveResult{
			{NodeID: "node-1", Activation: 0.5},
			{NodeID: "node-2", Activation: 0.6},
			{NodeID: "node-3", Activation: 0.4},
		},
	}
}
