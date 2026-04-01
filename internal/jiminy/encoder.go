package jiminy

import (
	"fmt"
	"strings"
)

// Encoding tiers for the J17 protocol.
const (
	TierCoded       = 1 // T1: Compact coded format (~15 tokens per constraint)
	TierTelegraphic = 2 // T2: Telegraphic NL (~50-100 tokens)
	TierFullNL      = 3 // T3: Full natural language (current format)
)

// T1CompactLevel controls T1 compression aggressiveness.
type T1CompactLevel int

const (
	T1CompactNone    T1CompactLevel = 0 // Full T1 with all annotations + src
	T1CompactDropSrc T1CompactLevel = 1 // Drop src:NODE_ID (traceability metadata)
	T1CompactGlossary T1CompactLevel = 2 // Drop src + annotations redundant with glossary DICT (default — 5.2x compression, 0 comprehension loss)
	T1CompactMax     T1CompactLevel = 3 // Drop src + redundant annotations + shorten remaining

	// T1CompactDefault is the production default. T1CompactGlossary achieves 5.2x compression
	// (19% of original size) with zero comprehension loss by leveraging the bootstrap DICT header.
	T1CompactDefault = T1CompactGlossary
)

// ProtocolEncoder formats guidance items using J17's three-tier encoding.
type ProtocolEncoder struct {
	defaultTier int
	// glossary maps constraint codes to short definitions for the bootstrap header.
	// Sent once per session so the receiver knows what codes mean before T1 messages.
	glossary map[string]string
	// Configurable tier thresholds (defaults 0.8 and 0.4)
	tierHighThreshold float64
	tierLowThreshold  float64
	// T1 compact mode for compression optimization
	t1Compact T1CompactLevel
}

// NewProtocolEncoder creates a new protocol encoder.
func NewProtocolEncoder(defaultTier int) *ProtocolEncoder {
	if defaultTier < TierCoded || defaultTier > TierFullNL {
		defaultTier = TierCoded
	}
	return &ProtocolEncoder{
		defaultTier:       defaultTier,
		glossary:          make(map[string]string),
		tierHighThreshold: 0.75,
		tierLowThreshold:  0.35,
		t1Compact:         T1CompactDefault,
	}
}

// SetGlossary sets the code glossary for bootstrap header generation.
// Each entry maps a constraint code to a short definition (≤15 words).
func (e *ProtocolEncoder) SetGlossary(glossary map[string]string) {
	e.glossary = glossary
}

// AddGlossaryEntry adds a single code→definition mapping.
func (e *ProtocolEncoder) AddGlossaryEntry(code, definition string) {
	if e.glossary == nil {
		e.glossary = make(map[string]string)
	}
	e.glossary[code] = definition
}

// Encode formats guidance items using the appropriate tier.
// trustScore modulates tier selection: high trust → more T1, low trust → more T2/T3.
func (e *ProtocolEncoder) Encode(items []GuidanceItem, counts SourceCounts, confidence float64, trustScore float64) string {
	if len(items) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("═══ JIMINY GUIDANCE ═══\n")

	for i := range items {
		// NS-01: If tier is pre-assigned by the sidecar arbitrator, use it.
		// Otherwise, fall back to rule-based tier selection.
		tier := items[i].Tier
		if tier < TierCoded || tier > TierFullNL {
			tier = e.selectTier(items[i], trustScore)
			items[i].Tier = tier
		}

		switch tier {
		case TierCoded:
			sb.WriteString(e.formatT1(items[i]))
		case TierTelegraphic:
			sb.WriteString(e.formatT2(items[i]))
		default:
			sb.WriteString(e.formatT3(items[i]))
		}
		sb.WriteByte('\n')
	}

	sb.WriteString("═══ END JIMINY GUIDANCE ═══")
	return sb.String()
}

// SetT1Compact sets the T1 compression level.
func (e *ProtocolEncoder) SetT1Compact(level T1CompactLevel) {
	e.t1Compact = level
}

// SetTierThresholds updates the trust thresholds used for tier selection.
func (e *ProtocolEncoder) SetTierThresholds(high, low float64) {
	if high > 0 && high <= 1.0 && low >= 0 && low < 1.0 && low < high {
		e.tierHighThreshold = high
		e.tierLowThreshold = low
	}
}

// GetTierThresholds returns the current tier selection thresholds.
func (e *ProtocolEncoder) GetTierThresholds() (high, low float64) {
	return e.tierHighThreshold, e.tierLowThreshold
}

// selectTier determines the encoding tier for an item.
func (e *ProtocolEncoder) selectTier(item GuidanceItem, trustScore float64) int {
	// Items with a constraint code can use T1
	hasCode := item.ConstraintCode != ""

	// Trust-based tier modulation using configurable thresholds
	switch {
	case hasCode && trustScore > e.tierHighThreshold:
		// High trust + has code → T1 (agent has earned dense encoding)
		return TierCoded
	case hasCode && trustScore >= e.tierLowThreshold:
		// Moderate trust + has code → T2 (must earn T1 via sustained comprehension)
		return TierTelegraphic
	case hasCode && trustScore < e.tierLowThreshold:
		// Low trust + has code → T2 (add telegraphic explanation)
		return TierTelegraphic
	case !hasCode && trustScore > e.tierHighThreshold:
		// High trust, no code → T2 (compact but still NL)
		return TierTelegraphic
	default:
		// No code, normal/low trust → T3 (full NL for clarity)
		return TierFullNL
	}
}

// formatT1 formats a guidance item as T1 (coded, ~15-25 tokens).
// Format: TYPE:SEV|code|ann1:val|ann2:val|esc:N|src:NODE_ID
// Annotations provide disambiguation context that the code alone cannot convey:
//   - alt: positive alternative (what TO use instead)
//   - neg: what is explicitly forbidden
//   - scope: quantifier/applicability (e.g. "all-no-exceptions")
//   - ctx: domain context that disambiguates the code's purpose
func (e *ProtocolEncoder) formatT1(item GuidanceItem) string {
	typePrefix := guidanceTypePrefix(item.Type)
	severity := priorityToSeverity(item.Priority)

	code := item.ConstraintCode
	if code == "" {
		code = truncateContent(item.Content, 30)
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("%s:%s|%s", typePrefix, severity, code))

	// Inline annotations for disambiguation
	if len(item.Annotations) > 0 {
		// At T1CompactGlossary+, skip annotations whose info is already in the glossary DICT
		skipGlossaryRedundant := e.t1Compact >= T1CompactGlossary && e.glossary != nil && e.glossary[code] != ""

		// Order: alt, neg, scope, ctx (most actionable first)
		for _, key := range []string{"alt", "neg", "scope", "ctx"} {
			if val, ok := item.Annotations[key]; ok {
				if skipGlossaryRedundant {
					continue // glossary DICT already disambiguates this code
				}
				if e.t1Compact >= T1CompactMax {
					val = compactAnnotationValue(val)
				}
				parts = append(parts, fmt.Sprintf("%s:%s", key, val))
			}
		}
		// Any remaining annotations not in the standard set
		if !skipGlossaryRedundant {
			for key, val := range item.Annotations {
				if key != "alt" && key != "neg" && key != "scope" && key != "ctx" {
					if e.t1Compact >= T1CompactMax {
						val = compactAnnotationValue(val)
					}
					parts = append(parts, fmt.Sprintf("%s:%s", key, val))
				}
			}
		}
	}

	if item.EscalationLevel != "" && item.EscalationLevel != EscalationInactive {
		parts = append(parts, fmt.Sprintf("esc:%d", escalationToInt(item.EscalationLevel)))
	}

	// At T1CompactDropSrc+, omit src:NODE_ID (traceability metadata, not needed for comprehension)
	if e.t1Compact < T1CompactDropSrc {
		if len(item.SourceNodes) > 0 {
			parts = append(parts, fmt.Sprintf("src:%s", item.SourceNodes[0]))
		}
	}

	return strings.Join(parts, "|")
}

// compactAnnotationValue shortens annotation values by truncating to key terms.
func compactAnnotationValue(val string) string {
	// Already short enough
	if len(val) <= 12 {
		return val
	}
	// Truncate at word boundary near 12 chars
	if idx := strings.LastIndex(val[:min(len(val), 15)], "-"); idx > 6 {
		return val[:idx]
	}
	if len(val) > 15 {
		return val[:15]
	}
	return val
}

// formatT2 formats a guidance item as T2 (telegraphic NL, ~50-100 tokens).
// Drops articles, pronouns, filler. Still natural language but compressed.
func (e *ProtocolEncoder) formatT2(item GuidanceItem) string {
	content := telegraphicCompress(item.Content)

	var sb strings.Builder
	typePrefix := guidanceTypePrefix(item.Type)
	sb.WriteString(typePrefix)
	sb.WriteString(": ")
	sb.WriteString(content)

	if item.ConstraintCode != "" {
		fmt.Fprintf(&sb, " [%s]", item.ConstraintCode)
	}

	if len(item.SourceNodes) > 0 {
		fmt.Fprintf(&sb, " (src:%s)", item.SourceNodes[0])
	}

	return sb.String()
}

// formatT3 formats a guidance item as T3 (full natural language).
func (e *ProtocolEncoder) formatT3(item GuidanceItem) string {
	return fmt.Sprintf("- [%s] %s (conf: %.2f)", item.Type, item.Content, item.Confidence)
}

// FormatItem formats a single guidance item at a specific tier.
// This is useful for testing or when encoding items individually.
func (e *ProtocolEncoder) FormatItem(item GuidanceItem, tier int) string {
	switch tier {
	case TierCoded:
		return e.formatT1(item)
	case TierTelegraphic:
		return e.formatT2(item)
	default:
		return e.formatT3(item)
	}
}

// FormatBootstrap generates the J17 protocol spec header for first sessions (~50 tokens).
// Use FormatBootstrapWithGlossary to include a code dictionary for T1 disambiguation.
func FormatBootstrap() string {
	return `J17:INIT|v1
CODES: C=constraint X=correction F=frontier D=decision P=pattern L=learning
SEV: !=must ?=should ~=info
ESC: 0=clear 1=warned 2=escalated 3=blocked
FMT: TYPE:SEV|code|[annotations]|esc:N|src:NODE_ID
ANN: alt=use-instead neg=forbidden scope=applicability ctx=domain-context
TICKET: signed, echo back on resume
SEQ: monotonic, report last_seq on resume`
}

// FormatBootstrapWithGlossary generates the protocol spec header with a code glossary.
// The glossary maps each T1 code to a short definition so the receiver can decode
// compact messages without prior context. This eliminates the primary T1 failure mode
// where codes are interpreted literally rather than as mnemonic references.
func FormatBootstrapWithGlossary(glossary map[string]string) string {
	var sb strings.Builder
	sb.WriteString(FormatBootstrap())

	if len(glossary) > 0 {
		sb.WriteString("\nDICT:")
		for code, def := range glossary {
			fmt.Fprintf(&sb, "\n  %s = %s", code, def)
		}
	}

	return sb.String()
}

// guidanceTypePrefix maps GuidanceType to a 1-char prefix for T1 encoding.
func guidanceTypePrefix(t GuidanceType) string {
	switch t {
	case GuidanceConstraint:
		return "C"
	case GuidanceCorrection:
		return "X"
	case GuidanceFrontier:
		return "F"
	case GuidanceDecision:
		return "D"
	case GuidancePattern, GuidanceSuggestion:
		return "P"
	case GuidanceLearning:
		return "L"
	case GuidanceConflict, GuidanceRisk:
		return "!"
	case GuidanceConcept:
		return "K"
	case GuidancePreference:
		return "R"
	default:
		return "?"
	}
}

// priorityToSeverity maps priority string to severity char.
func priorityToSeverity(priority string) string {
	switch priority {
	case "high":
		return "!"
	case "medium":
		return "?"
	default:
		return "~"
	}
}

// escalationToInt maps EscalationLevel to numeric.
func escalationToInt(level EscalationLevel) int {
	switch level {
	case EscalationSurfaced:
		return 0
	case EscalationWarned:
		return 1
	case EscalationEscalated:
		return 2
	case EscalationBlocked:
		return 3
	case EscalationResolved:
		return 0
	default:
		return 0
	}
}

// telegraphicCompress reduces natural language by removing articles, pronouns, and filler.
func telegraphicCompress(text string) string {
	// Remove common filler words
	dropWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "must": true, "shall": true,
		"that": true, "which": true, "who": true, "whom": true,
		"this": true, "these": true, "those": true,
		"it": true, "its": true, "you": true, "your": true, "we": true, "our": true,
		"very": true, "really": true, "just": true, "also": true, "please": true,
	}

	words := strings.Fields(text)
	var result []string
	for _, w := range words {
		lower := strings.ToLower(strings.Trim(w, ".,;:!?\"'"))
		if !dropWords[lower] {
			result = append(result, w)
		}
	}

	compressed := strings.Join(result, " ")

	// Limit length
	if len(compressed) > 200 {
		compressed = compressed[:200]
		if idx := strings.LastIndex(compressed, " "); idx > 150 {
			compressed = compressed[:idx]
		}
	}

	return compressed
}

// truncateContent truncates content to maxLen for fallback coding.
func truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen]
}
