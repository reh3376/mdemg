package sidecar

import (
	"fmt"
	"slices"
)

// allowedTransitions defines the state machine per roadmap Section 7B.
var allowedTransitions = map[State][]State{
	StateUninitialized: {StateInitialized},
	StateInitialized:   {StateInstalled},
	StateInstalled:     {StateRunning, StateUninstalled},
	StateRunning:       {StateDegraded, StateStopped},
	StateDegraded:      {StateRunning, StateStopped},
	StateStopped:       {StateRunning, StateInstalled, StateUninstalled},
	StateUninstalled:   {StateInitialized},
}

// ValidateTransition checks if a state transition is allowed.
// Returns nil for valid transitions, or an error with remediation guidance.
func ValidateTransition(from, to State) error {
	targets, ok := allowedTransitions[from]
	if !ok {
		return fmt.Errorf("unknown state %q", from)
	}
	if slices.Contains(targets, to) {
		return nil
	}
	cmd := TransitionCommand(from, to)
	if cmd != "" {
		return fmt.Errorf("invalid transition %s → %s (try: %s)", from, to, cmd)
	}
	return fmt.Errorf("invalid transition %s → %s", from, to)
}

// TransitionCommand maps a target state to the command that typically causes it.
func TransitionCommand(from, to State) string {
	switch to {
	case StateInitialized:
		return "mdemg sidecar init"
	case StateInstalled:
		return "mdemg sidecar install"
	case StateRunning:
		if from == StateStopped {
			return "mdemg sidecar up"
		}
		if from == StateDegraded {
			return "mdemg sidecar restart"
		}
		return "mdemg sidecar up"
	case StateStopped:
		return "mdemg sidecar down"
	case StateUninstalled:
		return "mdemg sidecar uninstall"
	case StateDegraded:
		return "" // system-driven, not a command
	}
	return ""
}
