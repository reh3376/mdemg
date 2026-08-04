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
	"log/slog"
	"mdemg/internal/sanitize"
	"regexp"
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

// JIMINY-CONTRADICTED-BRIDGE-QUALITY-001 (2026-08-04)
// ---------------------------------------------------
// Content-quality gate for contradicted-draft emission. The bridge was
// writing drafts for every OutcomeContradicted verdict regardless of the
// guidance's actual shape — including transient guidance content (Bash
// errors, "Phase 92: gap analysis" narratives, status logs) that had been
// wrongly typed as constraint/correction upstream. These are noise: no
// operator will ever grade "Bash error in command X" as a durable rule.
//
// Two-layer gate (mirrors JIMINY-CORPUS-001's ConstraintPromotionGate):
//   1. Type filter (primary, principled): only actionable guidance types
//      (constraint, correction) can produce drafts. A "contradicted" verdict
//      on a `pattern`/`learning`/`concept`/`decision` guidance item is
//      semantically odd — those are advisory, not imperative rules; the
//      contradicted signal isn't meaningful for them.
//   2. Content-pattern filter (backstop): the SAME regex list the
//      constraint-promotion gate uses (`ConstraintPromotionRejectPatterns`).
//      A guidance content matching a junk-class pattern (Bash error, sprint
//      status, doc heading, gap-analysis narrative) had junk upstream and
//      any contradiction based on it is noise here too.
//
// Fail-open: a nil or disabled gate emits everything (backward-compatible;
// operators can opt out via config). Rejections are logged, never silent.

// ContradictedBridgeGate decides whether a contradicted-verdict event may
// produce a HITL draft.
type ContradictedBridgeGate struct {
	enabled       bool
	allowedTypes  map[GuidanceType]struct{} // if nil, type filter is off
	rejectPatterns []*regexp.Regexp
}

// NewContradictedBridgeGate builds the gate. `enabled=false` bypasses both
// filters (backward-compat: mirrors the pre-sprint behavior). `allowedTypes`
// nil or empty disables the type filter but keeps the pattern filter.
// Invalid regexes are skipped with a WARN log.
func NewContradictedBridgeGate(enabled bool, allowedTypes []GuidanceType, patterns []string) *ContradictedBridgeGate {
	g := &ContradictedBridgeGate{enabled: enabled}
	if len(allowedTypes) > 0 {
		g.allowedTypes = make(map[GuidanceType]struct{}, len(allowedTypes))
		for _, t := range allowedTypes {
			g.allowedTypes[t] = struct{}{}
		}
	}
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			slog.Warn("contradicted bridge gate: skipping invalid reject pattern",
				"pattern", p, "error", err)
			continue
		}
		g.rejectPatterns = append(g.rejectPatterns, re)
	}
	return g
}

// Reject returns (reason, true) when a contradicted draft MUST NOT be
// emitted for this guidance, and ("", false) when emission may proceed.
// A nil or disabled gate passes everything (fail-open).
func (g *ContradictedBridgeGate) Reject(guidanceType GuidanceType, guidanceContent string) (string, bool) {
	if g == nil || !g.enabled {
		return "", false
	}
	if g.allowedTypes != nil {
		if _, ok := g.allowedTypes[guidanceType]; !ok {
			return "type:" + string(guidanceType), true
		}
	}
	for _, re := range g.rejectPatterns {
		if re.MatchString(guidanceContent) {
			return "pattern:" + re.String(), true
		}
	}
	return "", false
}
