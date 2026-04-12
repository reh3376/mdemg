package jiminy

import (
	"fmt"
	"sort"
	"strings"
)

// StrictReformulator transforms guidance items into imperative directives
// for /strict mode. Deterministic — no LLM in path.
type StrictReformulator struct {
	encoder *ProtocolEncoder
}

// NewStrictReformulator creates a new reformulator.
func NewStrictReformulator(encoder *ProtocolEncoder) *StrictReformulator {
	return &StrictReformulator{encoder: encoder}
}

// Reformulate transforms guidance items into a single imperative directive string.
// Items are filtered to confidence ≥ 0.5 or escalation ≥ WARNED, then sorted by
// escalation level descending. Output is capped at ~500 tokens (~350 words).
func (r *StrictReformulator) Reformulate(items []GuidanceItem, escalationState map[string]EscalationEntry) (string, bool) {
	// Filter to actionable items
	var actionable []GuidanceItem
	for _, item := range items {
		if item.Confidence >= 0.5 || item.EscalationLevel == EscalationWarned ||
			item.EscalationLevel == EscalationEscalated || item.EscalationLevel == EscalationBlocked {
			actionable = append(actionable, item)
		}
	}

	if len(actionable) == 0 {
		return "", false
	}

	// Sort by escalation severity descending
	sort.Slice(actionable, func(i, j int) bool {
		return escalationOrd(actionable[i].EscalationLevel) > escalationOrd(actionable[j].EscalationLevel)
	})

	var sb strings.Builder
	blockedAny := false

	// Check for BLOCKED items first
	for _, item := range actionable {
		if item.EscalationLevel == EscalationBlocked {
			code := item.ConstraintCode
			if code == "" {
				code = "CONSTRAINT"
			}
			fmt.Fprintf(&sb, "STOP. Constraint [%s]: %s. Do not proceed until resolved.\n", code, item.Content)
			blockedAny = true
		}
	}

	if blockedAny {
		return sb.String(), true
	}

	// Non-blocked: emit directive header + items
	sb.WriteString("DIRECTIVE: Comply with the following before proceeding:\n")

	wordCount := 10 // approximate header words
	for _, item := range actionable {
		line := r.formatDirectiveLine(item)
		lineWords := len(strings.Fields(line))
		if wordCount+lineWords > 350 {
			sb.WriteString("... (additional constraints omitted for brevity)\n")
			break
		}
		sb.WriteString(line)
		sb.WriteByte('\n')
		wordCount += lineWords
	}

	return sb.String(), false
}

// formatDirectiveLine formats a single guidance item as an imperative line.
func (r *StrictReformulator) formatDirectiveLine(item GuidanceItem) string {
	prefix := ""
	switch item.EscalationLevel {
	case EscalationEscalated:
		prefix = "MUST: "
	case EscalationWarned:
		prefix = "REQUIRED: "
	default:
		prefix = "- "
	}

	code := item.ConstraintCode
	if code != "" {
		return fmt.Sprintf("%s[%s] %s", prefix, code, item.Content)
	}
	return prefix + item.Content
}

// escalationOrd returns numeric ordering for escalation levels.
func escalationOrd(level EscalationLevel) int {
	switch level {
	case EscalationBlocked:
		return 4
	case EscalationEscalated:
		return 3
	case EscalationWarned:
		return 2
	case EscalationSurfaced:
		return 1
	default:
		return 0
	}
}
