package jiminy

import (
	"fmt"
	"sync"
	"time"
)

// Extension types that agents can negotiate.
const (
	ExtTierPreference  = "tier_preference"
	ExtAbbreviatedIDs  = "abbreviated_ids"
	ExtBatchMode       = "batch_mode"
	ExtDensityBoost    = "density_boost"
)

// NonNegotiableAspects lists protocol aspects that cannot be changed.
var NonNegotiableAspects = map[string]bool{
	"constraint_types": true,
	"severity_levels":  true,
	"escalation_fsm":   true,
	"ticket_lifecycle": true,
	"sequence_numbers": true,
}

// ExtensionRequest represents an agent's request to enable a protocol extension.
type ExtensionRequest struct {
	SpaceID   string `json:"space_id"`
	SessionID string `json:"session_id"`
	Extension string `json:"extension"`
	Value     string `json:"value,omitempty"` // optional parameter
}

// ExtensionResponse is the response to an extension request.
type ExtensionResponse struct {
	Granted   bool   `json:"granted"`
	Extension string `json:"extension"`
	Reason    string `json:"reason,omitempty"`
}

// ActiveExtension tracks a granted extension for a session.
type ActiveExtension struct {
	Extension string    `json:"extension"`
	Value     string    `json:"value,omitempty"`
	GrantedAt time.Time `json:"granted_at"`
}

// ExtensionRegistry manages per-session protocol extensions.
type ExtensionRegistry struct {
	mu         sync.RWMutex
	allowed    map[string]bool               // allowed extension names
	extensions map[string][]ActiveExtension  // sessionID → active extensions
}

// NewExtensionRegistry creates a new extension registry with the given allowed extensions.
func NewExtensionRegistry(allowedExtensions []string) *ExtensionRegistry {
	allowed := make(map[string]bool, len(allowedExtensions))
	for _, ext := range allowedExtensions {
		allowed[ext] = true
	}
	return &ExtensionRegistry{
		allowed:    allowed,
		extensions: make(map[string][]ActiveExtension),
	}
}

// Negotiate processes an extension request. Returns whether the extension was granted.
func (r *ExtensionRegistry) Negotiate(req ExtensionRequest) ExtensionResponse {
	// Check if this is a non-negotiable aspect
	if NonNegotiableAspects[req.Extension] {
		return ExtensionResponse{
			Granted:   false,
			Extension: req.Extension,
			Reason:    fmt.Sprintf("%s is a non-negotiable protocol aspect", req.Extension),
		}
	}

	// Check if this extension is in the allowed list
	if !r.allowed[req.Extension] {
		return ExtensionResponse{
			Granted:   false,
			Extension: req.Extension,
			Reason:    fmt.Sprintf("extension %s is not in the allowed list", req.Extension),
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for duplicate
	for _, ext := range r.extensions[req.SessionID] {
		if ext.Extension == req.Extension {
			return ExtensionResponse{
				Granted:   true,
				Extension: req.Extension,
				Reason:    "already active",
			}
		}
	}

	r.extensions[req.SessionID] = append(r.extensions[req.SessionID], ActiveExtension{
		Extension: req.Extension,
		Value:     req.Value,
		GrantedAt: time.Now(),
	})

	return ExtensionResponse{
		Granted:   true,
		Extension: req.Extension,
	}
}

// GetActive returns all active extensions for a session.
func (r *ExtensionRegistry) GetActive(sessionID string) []ActiveExtension {
	r.mu.RLock()
	defer r.mu.RUnlock()

	exts := r.extensions[sessionID]
	result := make([]ActiveExtension, len(exts))
	copy(result, exts)
	return result
}

// HasExtension returns true if the session has the given extension active.
func (r *ExtensionRegistry) HasExtension(sessionID, extension string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, ext := range r.extensions[sessionID] {
		if ext.Extension == extension {
			return true
		}
	}
	return false
}

// ClearSession removes all extensions for a session.
func (r *ExtensionRegistry) ClearSession(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.extensions, sessionID)
}
