package conversation

import (
	"context"
	"log/slog"

	"mdemg/internal/config"
)

// NLIConstraintClassifier uses the sidecar NLI endpoint to confirm whether
// detected text is actually a constraint (entailment with a constraint premise).
type NLIConstraintClassifier struct {
	cfg config.Config
}

// NewNLIConstraintClassifier creates a new NLI-based constraint classifier.
func NewNLIConstraintClassifier(cfg config.Config) *NLIConstraintClassifier {
	return &NLIConstraintClassifier{cfg: cfg}
}

// IsConstraint checks whether the given text is a constraint by using NLI.
// The premise is a template constraint statement; if the text entails it, it's a constraint.
// Returns (isConstraint, confidence).
func (c *NLIConstraintClassifier) IsConstraint(ctx context.Context, text string) (bool, float64) {
	if !c.cfg.ConstraintNLIEnabled {
		return true, 0.0 // NLI disabled; don't filter
	}

	sidecarURL := c.cfg.NeuralRerankURL // Shares the sidecar URL
	if sidecarURL == "" {
		return true, 0.0 // No sidecar; don't filter
	}

	// Premise: a generic constraint template
	premise := "This text describes a requirement, rule, or constraint that must be followed."
	result, err := classifyNLI(ctx, sidecarURL, premise, text, 1000)
	if err != nil {
		slog.Warn("nli_classifier: NLI call failed, allowing detection", "error", err)
		return true, 0.0 // Fail-open
	}

	// If entailment score is high enough, it's a constraint
	if result.Scores.Entailment >= 0.5 {
		return true, result.Scores.Entailment
	}

	slog.Info("nli_classifier: text rejected as non-constraint", "entailment", result.Scores.Entailment, "contradiction", result.Scores.Contradiction)
	return false, result.Scores.Entailment
}
