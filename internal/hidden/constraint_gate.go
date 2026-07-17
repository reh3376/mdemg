package hidden

// JIMINY-CORPUS-001 Epic 1: ConvObs→constraint promotion gate.
//
// CreateConstraintNodes is the only site that mints role_type='constraint'
// nodes from conversation observations, and before this gate it promoted
// ANY observation carrying a 'constraint:<type>' tag — tags assigned by the
// keyword regex ConstraintDetector, which fires on incidental words like
// "required"/"never"/"must" inside transient content. That is how junk
// ("Build/test succeeded: …", "Bash error in command: …", PR/sprint status
// notes, doc/template dumps) became constraint nodes that Lever C then
// surfaced as guidance.
//
// The gate is two layers:
//
//  1. Provenance (principled, primary): observations whose obs_type is in
//     the deny set (default: progress, error, task, context) are transient
//     by definition — a durable rule never legitimately arrives as a
//     build-status or error observation. This is the preferred signal
//     because it survives content rewording.
//  2. Content shape (config-driven backstop): a regexp set covering the
//     diagnosed junk classes, for junk that arrives under a durable-looking
//     obs_type (e.g. doc dumps stored as 'decision'/'learning').
//
// Both layers are config-driven (CONSTRAINT_PROMOTION_GATE_ENABLED,
// CONSTRAINT_PROMOTION_DENY_OBS_TYPES, CONSTRAINT_PROMOTION_REJECT_PATTERNS)
// with defaults in internal/config. Rejections are logged, never silent.

import (
	"log/slog"
	"regexp"
	"strings"

	"mdemg/internal/config"
)

// ConstraintPromotionGate decides whether a constraint-tagged conversation
// observation may be promoted to a role_type='constraint' node.
type ConstraintPromotionGate struct {
	enabled      bool
	denyObsTypes map[string]struct{}
	patterns     []*regexp.Regexp
}

// NewConstraintPromotionGate builds a gate from config. Patterns are
// validated at config parse time; an invalid pattern reaching here (e.g. a
// hand-built Config in tests) is skipped with a loud warning rather than
// disabling the gate.
func NewConstraintPromotionGate(cfg config.Config) *ConstraintPromotionGate {
	g := &ConstraintPromotionGate{
		enabled:      cfg.ConstraintPromotionGateEnabled,
		denyObsTypes: make(map[string]struct{}, len(cfg.ConstraintPromotionDenyObsTypes)),
	}
	for _, t := range cfg.ConstraintPromotionDenyObsTypes {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			g.denyObsTypes[t] = struct{}{}
		}
	}
	for _, p := range cfg.ConstraintPromotionRejectPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			slog.Warn("constraint promotion gate: skipping invalid reject pattern",
				"pattern", p, "error", err)
			continue
		}
		g.patterns = append(g.patterns, re)
	}
	return g
}

// Reject returns (reason, true) when the observation must NOT be promoted to
// a constraint node, and ("", false) when promotion may proceed. A nil or
// disabled gate passes everything (fail-open: the gate must never block
// genuine constraint promotion when unconfigured).
func (g *ConstraintPromotionGate) Reject(obsType, content string) (string, bool) {
	if g == nil || !g.enabled {
		return "", false
	}
	if t := strings.ToLower(strings.TrimSpace(obsType)); t != "" {
		if _, denied := g.denyObsTypes[t]; denied {
			return "obs_type:" + t, true
		}
	}
	for _, re := range g.patterns {
		if re.MatchString(content) {
			return "pattern:" + re.String(), true
		}
	}
	return "", false
}
