package hidden

// JIMINY-CORRECTION-PRODUCER-001 Epic 2: L0 obs → correction node promotion gate.
//
// CreateCorrectionNodes is the only site that mints role_type='correction'
// nodes from L0 conversation observations. Unlike constraints (which are
// tag-derived and thus can pick up incidental keyword matches on transient
// content), corrections come in through the explicit ObsTypeCorrection
// entry — the /v1/conversation/correct API — so the provenance layer of the
// constraint gate is already handled by the promotion predicate itself
// (obs_type='correction').
//
// This gate is a defensive backstop against pathological content shapes:
//
//  1. Min content length — a genuine correction is at least a sentence.
//     Prevents empty/near-empty stubs from becoming L1 nodes.
//  2. Content-pattern deny-set (config-driven regex list) — catches doc /
//     SKILL / template / status dumps that may reach the correction obs_type
//     through automation drift, mirroring the JIMINY-CORPUS-001 shape class.
//
// Both layers are config-driven (CORRECTION_PROMOTION_ENABLED,
// CORRECTION_PROMOTION_MIN_CONTENT_LEN, CORRECTION_PROMOTION_REJECT_PATTERNS)
// with defaults in internal/config. Rejections are logged, never silent.

import (
	"log/slog"
	"regexp"

	"mdemg/internal/config"
)

// CorrectionPromotionGate decides whether an L0 correction observation may be
// promoted to a role_type='correction' node.
type CorrectionPromotionGate struct {
	enabled       bool
	minContentLen int
	patterns      []*regexp.Regexp
}

// NewCorrectionPromotionGate builds a gate from config. Patterns are validated
// here; an invalid pattern (e.g. a hand-built Config in tests) is skipped with
// a loud warning rather than disabling the gate.
func NewCorrectionPromotionGate(cfg config.Config) *CorrectionPromotionGate {
	g := &CorrectionPromotionGate{
		enabled:       cfg.CorrectionPromotionEnabled,
		minContentLen: cfg.CorrectionPromotionMinContentLen,
	}
	for _, p := range cfg.CorrectionPromotionRejectPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			slog.Warn("correction promotion gate: skipping invalid reject pattern",
				"pattern", p, "error", err)
			continue
		}
		g.patterns = append(g.patterns, re)
	}
	return g
}

// Reject returns (reason, true) when the observation must NOT be promoted to
// a correction node, and ("", false) when promotion may proceed. A nil or
// disabled gate passes everything (fail-open: the gate must never block
// genuine correction promotion when unconfigured).
func (g *CorrectionPromotionGate) Reject(content string) (string, bool) {
	if g == nil || !g.enabled {
		return "", false
	}
	if g.minContentLen > 0 && len(content) < g.minContentLen {
		return "content_too_short", true
	}
	for _, re := range g.patterns {
		if re.MatchString(content) {
			return "pattern:" + re.String(), true
		}
	}
	return "", false
}
