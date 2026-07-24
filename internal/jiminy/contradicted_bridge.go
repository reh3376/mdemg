package jiminy

// JIMINY-CONTRADICTED-BRIDGE-001 Epic 2: contradicted-outcome → correction
// draft bridge helper. Runs on every OutcomeContradicted verdict when
// JiminyContradictedBridgeEnabled=true. Templates the guidance body +
// action_summary into a draft correction and hands it to the tsdb writer;
// the HITL surface picks it up from there.
//
// Dedup: a bounded in-process LRU keyed by (guidance_id, action_hash) so a
// repeat-violation burst inside a session doesn't spawn duplicate drafts.
// The tsdb writer's DedupExists is the durable check across restarts / other
// processes; the LRU is the sub-second race guard.

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"mdemg/internal/sanitize"
	"strings"
	"sync"

	"github.com/nrednav/cuid2"
)

// contradictedBridgeCache is a bounded (guidance_id, action_hash) LRU.
type contradictedBridgeCache struct {
	mu   sync.Mutex
	max  int
	ll   *list.List
	seen map[string]*list.Element
}

type bridgeCacheKey struct {
	guidanceID string
	actionHash string
}

func newContradictedBridgeCache(max int) *contradictedBridgeCache {
	if max <= 0 {
		max = 1024
	}
	return &contradictedBridgeCache{
		max:  max,
		ll:   list.New(),
		seen: make(map[string]*list.Element, max),
	}
}

// TrySeen returns true if (guidanceID, actionHash) was seen recently (LRU
// hit). Non-hits are recorded as fresh; hits move to MRU.
func (c *contradictedBridgeCache) TrySeen(guidanceID, actionHash string) bool {
	if actionHash == "" {
		return false
	}
	key := bridgeCacheKey{guidanceID: guidanceID, actionHash: actionHash}
	keyStr := guidanceID + "|" + actionHash

	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.seen[keyStr]; ok {
		c.ll.MoveToFront(e)
		return true
	}
	e := c.ll.PushFront(key)
	c.seen[keyStr] = e
	if c.ll.Len() > c.max {
		old := c.ll.Back()
		if old != nil {
			c.ll.Remove(old)
			ok := old.Value.(bridgeCacheKey)
			delete(c.seen, ok.guidanceID+"|"+ok.actionHash)
		}
	}
	return false
}

// hashAction produces a stable dedup token for an (guidance_id, action)
// pair — 16 hex chars of sha256, tolerant of whitespace jitter and case.
func hashAction(guidanceID, actionSummary string) string {
	norm := strings.ToLower(strings.Join(strings.Fields(actionSummary), " "))
	h := sha256.Sum256([]byte(guidanceID + "|" + norm))
	return hex.EncodeToString(h[:8]) // 16 hex chars
}

// clipContent bounds a template field to at most maxLen chars, trimming
// surrounding whitespace first. Prevents multi-KB action_summary or guidance
// body dumps from bloating the draft columns.
func clipContent(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	return sanitize.CutRuneSafe(s, maxLen)
}

// buildContradictedDraft renders the (Incorrect, Correct) pair the operator
// will see in HITL. The operator can edit either before approving.
func buildContradictedDraft(actionSummary, guidanceContent string, maxLen int) (incorrect, correct string) {
	return clipContent(actionSummary, maxLen), clipContent(guidanceContent, maxLen)
}

// newDraftID mints a CUIDv2 for the draft row. Extracted so tests can stub.
var newDraftID = func() string { return cuid2.Generate() }
