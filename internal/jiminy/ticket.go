package jiminy

import (
	"container/list"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ticketEntry holds a session ticket in the LRU list.
type ticketEntry struct {
	sessionID string
	ticket    *SessionTicket
}

// TicketManager issues, validates, and restores session tickets.
type TicketManager struct {
	mu     sync.RWMutex
	secret []byte
	ttl    time.Duration

	// Per-session last-issued ticket with LRU eviction
	lastTickets map[string]*list.Element // sessionID → list element
	lruList     *list.List               // front = most recent
	maxSize     int                      // maximum entries (0 = unlimited)
}

// NewTicketManager creates a new TicketManager.
// If secret is empty, a random 32-byte key is generated and a warning is logged.
// maxSize limits the LRU cache; 0 or negative means default (1000).
func NewTicketManager(secret string, ttlHours int, maxSize int) *TicketManager {
	var key []byte
	if secret != "" {
		key = []byte(secret)
	} else {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			slog.Warn("j17: failed to generate random ticket secret", "error", err)
			key = []byte("mdemg-j17-default-key-insecure")
		}
		slog.Warn("j17: J17_TICKET_SECRET not set, using auto-generated key (not persistent across restarts)")
	}

	ttl := time.Duration(ttlHours) * time.Hour
	if ttl <= 0 {
		ttl = 4 * time.Hour
	}

	if maxSize <= 0 {
		maxSize = 1000
	}

	return &TicketManager{
		secret:      key,
		ttl:         ttl,
		lastTickets: make(map[string]*list.Element),
		lruList:     list.New(),
		maxSize:     maxSize,
	}
}

// IssueTicket creates a signed session ticket from the current state.
func (tm *TicketManager) IssueTicket(payload TicketPayload) (*SessionTicket, error) {
	payload.Version = J17Version
	payload.IssuedAt = time.Now()
	payload.TTL = tm.ttl

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("j17: marshal ticket payload: %w", err)
	}

	sig := tm.sign(data)

	ticket := &SessionTicket{
		Payload:   data,
		Signature: sig,
	}

	// Store last ticket for this session (LRU)
	tm.mu.Lock()
	if elem, ok := tm.lastTickets[payload.SessionID]; ok {
		tm.lruList.MoveToFront(elem)
		elem.Value.(*ticketEntry).ticket = ticket
	} else {
		entry := &ticketEntry{sessionID: payload.SessionID, ticket: ticket}
		elem := tm.lruList.PushFront(entry)
		tm.lastTickets[payload.SessionID] = elem

		// Evict oldest if over capacity
		for tm.maxSize > 0 && tm.lruList.Len() > tm.maxSize {
			back := tm.lruList.Back()
			if back != nil {
				tm.lruList.Remove(back)
				delete(tm.lastTickets, back.Value.(*ticketEntry).sessionID)
			}
		}
	}
	tm.mu.Unlock()

	return ticket, nil
}

// ValidateTicket verifies the HMAC signature and checks TTL.
// Returns the decoded payload if valid, error otherwise.
func (tm *TicketManager) ValidateTicket(ticket *SessionTicket) (*TicketPayload, error) {
	if ticket == nil {
		return nil, fmt.Errorf("j17: nil ticket")
	}

	// Verify HMAC signature
	expected := tm.sign(ticket.Payload)
	if !hmac.Equal([]byte(expected), []byte(ticket.Signature)) {
		return nil, fmt.Errorf("j17: invalid ticket signature (tampered)")
	}

	// Decode payload
	var payload TicketPayload
	if err := json.Unmarshal(ticket.Payload, &payload); err != nil {
		return nil, fmt.Errorf("j17: unmarshal ticket payload: %w", err)
	}

	// Check TTL
	if time.Since(payload.IssuedAt) > payload.TTL {
		return nil, fmt.Errorf("j17: ticket expired (issued %s, ttl %s)", payload.IssuedAt.Format(time.RFC3339), payload.TTL)
	}

	// Check version
	if payload.Version != J17Version {
		return nil, fmt.Errorf("j17: unsupported ticket version %q (expected %q)", payload.Version, J17Version)
	}

	return &payload, nil
}

// RestoreFromTicket validates a ticket and restores escalation state.
// Returns the payload for further processing (e.g., sequence replay).
func (tm *TicketManager) RestoreFromTicket(ticket *SessionTicket, escalation *EscalationTracker) (*TicketPayload, error) {
	payload, err := tm.ValidateTicket(ticket)
	if err != nil {
		return nil, err
	}

	// Restore escalation state
	if escalation != nil && len(payload.EscalationSnapshot) > 0 {
		escalation.ImportState(payload.SessionID, payload.EscalationSnapshot)
	}

	return payload, nil
}

// GetLastTicket returns the most recently issued ticket for a session.
func (tm *TicketManager) GetLastTicket(sessionID string) *SessionTicket {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if elem, ok := tm.lastTickets[sessionID]; ok {
		tm.lruList.MoveToFront(elem)
		return elem.Value.(*ticketEntry).ticket
	}
	return nil
}

// sign computes HMAC-SHA256 of data and returns hex-encoded digest.
func (tm *TicketManager) sign(data []byte) string {
	mac := hmac.New(sha256.New, tm.secret)
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}
