package retrieval

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// scoreLiteralAllowlist holds file:line-independent allowances — places where
// a float literal legitimately compares against a retrieval score because the
// value is config-loaded right above, a documented default fallback, or not
// the RRF scale at all. Everything else is the RRF-SCALE bug class: a
// hardcoded threshold that silently dies when the scorer changes
// (24-day Hebbian no-op; 9-week guidance dormancy; Suggest filtering ~all).
var scoreLiteralAllowlist = map[string]string{
	"internal/retrieval/scoring.go":        "legacy linear scorer internals (scale-local math, not consumer gates)",
	"internal/retrieval/scoring_rrf.go":    "RRF aggregator internals (produce the scale; cannot consume it wrongly)",
	"internal/retrieval/gate.go":           "sparse gate percentile math (within-call relative, scale-invariant)",
	"internal/retrieval/column_context.go": "Jaccard similarity (not the RRF scale)",
	// Triaged on this test's first run:
	"internal/anomaly/service.go":        "near-duplicate severity on raw cosine similarity (scale-local, not RRF)",
	"internal/conversation/relevance.go": "RelevanceScorer's own composite scale (not RetrieveResult.Score)",
	"internal/jiminy/corrections.go":     "Neo4j vector-index cosine in Cypher (stable scale; config-driving is a candidate, mirroring GUARDRAIL_CONSTRAINT_SIM_FLOOR)",
}

// scoreCompareRe matches `.Score <op> <float literal>` and `score >= <float>`
// style comparisons.
var scoreCompareRe = regexp.MustCompile(`(\.Score|\bscore\b)\s*(==|!=|<=|>=|<|>)\s*0\.\d*[1-9]\d*`)

func TestNoHardcodedScoreThresholds(t *testing.T) {
	root := "../.." // repo root from internal/retrieval
	var hits []string
	err := filepath.Walk(filepath.Join(root, "internal"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if _, ok := scoreLiteralAllowlist[rel]; ok {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if scoreCompareRe.MatchString(line) && !strings.Contains(line, "cfg.") && !strings.Contains(line, "Config") &&
				!strings.Contains(line, "Floor") && !strings.Contains(line, "Threshold") && !strings.Contains(line, "reflectHigh") &&
				!strings.Contains(line, "reflectMedium") && !strings.Contains(line, "minConfidence") {
				hits = append(hits, rel+":"+scanItoa(i+1)+": "+strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(hits) > 0 {
		t.Errorf("hardcoded score-threshold comparisons found (RRF-SCALE class — gate via config with an RRF-calibrated default, or allowlist with justification):\n  %s",
			strings.Join(hits, "\n  "))
	}
}

func scanItoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
