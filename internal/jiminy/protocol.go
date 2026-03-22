package jiminy

import "time"

// J17 Protocol Version
const J17Version = "j17v1"

// TicketPayload holds the serializable state that survives context resets.
type TicketPayload struct {
	Version             string                       `json:"version"`               // "j17v1"
	SpaceID             string                       `json:"space_id"`
	SessionID           string                       `json:"session_id"`
	LastSeq             int64                        `json:"last_seq"`
	TrustScore          float64                      `json:"trust_score"`
	EscalationSnapshot  map[string]EscalationEntry   `json:"escalation_snapshot"`   // constraint_node_id → entry
	ActiveConstraintIDs []string                     `json:"active_constraint_ids"`
	ConversationPhase   string                       `json:"conversation_phase"`
	IssuedAt            time.Time                    `json:"issued_at"`
	TTL                 time.Duration                `json:"ttl"`
}

// SessionTicket is a signed, opaque ticket for state persistence across context resets.
type SessionTicket struct {
	Payload   []byte `json:"payload"`   // JSON-encoded TicketPayload
	Signature string `json:"signature"` // HMAC-SHA256 hex digest
}

// ProtocolState holds the current J17 protocol state for a session.
type ProtocolState struct {
	Enabled        bool    `json:"enabled"`
	Version        string  `json:"version"`
	LastSeq        int64   `json:"last_seq"`
	TrustScore     float64 `json:"trust_score"`
	TicketIssued   bool    `json:"ticket_issued"`
	EventsBuffered int     `json:"events_buffered"`
}

// ProtocolInfo is an optional field on GuidanceResponse with J17 state.
type ProtocolInfo struct {
	Version    string `json:"version"`
	Seq        int64  `json:"seq"`
	TrustScore float64 `json:"trust_score,omitempty"`
}

// SequenceEvent represents a recorded guidance event for replay.
type SequenceEvent struct {
	Seq       int64     `json:"seq"`
	Timestamp time.Time `json:"timestamp"`
	SpaceID   string    `json:"space_id"`
	SessionID string    `json:"session_id"`
	ItemCount int       `json:"item_count"`
	// Compact summary of what was sent (not full payload — that's in the response)
	Summary string `json:"summary,omitempty"`
}

// CheckpointRequest is the input to POST /v1/jiminy/checkpoint.
type CheckpointRequest struct {
	SpaceID   string `json:"space_id"`
	SessionID string `json:"session_id"`
}

// CheckpointResponse is the output from POST /v1/jiminy/checkpoint.
type CheckpointResponse struct {
	Ticket    *SessionTicket `json:"ticket"`
	LastSeq   int64          `json:"last_seq"`
	IssuedAt  string         `json:"issued_at"`
	ExpiresAt string         `json:"expires_at"`
}

// ResumeProtocolRequest is the input to POST /v1/jiminy/resume-protocol.
type ResumeProtocolRequest struct {
	SpaceID   string         `json:"space_id"`
	SessionID string         `json:"session_id"`
	Ticket    *SessionTicket `json:"ticket"`
	LastSeq   int64          `json:"last_seq"` // agent's last known sequence number
}

// ResumeProtocolResponse is the output from POST /v1/jiminy/resume-protocol.
type ResumeProtocolResponse struct {
	Restored       bool            `json:"restored"`
	TrustScore     float64         `json:"trust_score"`
	EscalationState map[string]EscalationEntry `json:"escalation_state,omitempty"`
	ReplayedEvents []SequenceEvent `json:"replayed_events,omitempty"`
	Message        string          `json:"message"`
}
