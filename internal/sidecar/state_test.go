package sidecar

import "testing"

func TestValidateTransition_ValidPaths(t *testing.T) {
	validPaths := [][2]State{
		{StateUninitialized, StateInitialized},
		{StateInitialized, StateInstalled},
		{StateInstalled, StateRunning},
		{StateInstalled, StateUninstalled},
		{StateRunning, StateDegraded},
		{StateRunning, StateStopped},
		{StateDegraded, StateRunning},
		{StateDegraded, StateStopped},
		{StateStopped, StateRunning},
		{StateStopped, StateInstalled},
		{StateStopped, StateUninstalled},
		{StateUninstalled, StateInitialized},
	}

	for _, p := range validPaths {
		err := ValidateTransition(p[0], p[1])
		if err != nil {
			t.Errorf("ValidateTransition(%s, %s) = %v, want nil", p[0], p[1], err)
		}
	}
}

func TestValidateTransition_InvalidPaths(t *testing.T) {
	invalidPaths := [][2]State{
		{StateUninitialized, StateRunning},
		{StateRunning, StateInitialized},
		{StateInitialized, StateUninitialized},
		{StateStopped, StateDegraded},
	}

	for _, p := range invalidPaths {
		err := ValidateTransition(p[0], p[1])
		if err == nil {
			t.Errorf("ValidateTransition(%s, %s) = nil, want error", p[0], p[1])
		}
	}
}

func TestValidateTransition_UnknownState(t *testing.T) {
	err := ValidateTransition(State("bogus"), StateRunning)
	if err == nil {
		t.Error("expected error for unknown state")
	}
}

func TestTransitionCommand(t *testing.T) {
	tests := []struct {
		from, to State
		want     string
	}{
		{StateUninitialized, StateInitialized, "mdemg sidecar init"},
		{StateInitialized, StateInstalled, "mdemg sidecar install"},
		{StateInstalled, StateRunning, "mdemg sidecar up"},
		{StateRunning, StateStopped, "mdemg sidecar down"},
		{StateStopped, StateUninstalled, "mdemg sidecar uninstall"},
	}

	for _, tt := range tests {
		got := TransitionCommand(tt.from, tt.to)
		if got != tt.want {
			t.Errorf("TransitionCommand(%s, %s) = %q, want %q", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestAllStates(t *testing.T) {
	states := AllStates()
	if len(states) != 7 {
		t.Errorf("AllStates() returned %d states, want 7", len(states))
	}
}
