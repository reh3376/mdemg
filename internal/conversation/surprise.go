package conversation

import (
	"log/slog"
	"context"
	"math"
	"regexp"
	"strings"
	"unicode"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
)

// Embedder interface for computing embeddings (matches internal/embeddings.Embedder)
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	Dimensions() int
	Name() string
}

// SurpriseDetector detects novel/surprising information
type SurpriseDetector struct {
	embedder Embedder
	driver   neo4j.DriverWithContext
	cfg      config.Config
}

// NewSurpriseDetector creates a new surprise detector
func NewSurpriseDetector(embedder Embedder, driver neo4j.DriverWithContext) *SurpriseDetector {
	return &SurpriseDetector{
		embedder: embedder,
		driver:   driver,
	}
}

// NewSurpriseDetectorWithConfig creates a new surprise detector with config.
func NewSurpriseDetectorWithConfig(embedder Embedder, driver neo4j.DriverWithContext, cfg config.Config) *SurpriseDetector {
	return &SurpriseDetector{
		embedder: embedder,
		driver:   driver,
		cfg:      cfg,
	}
}

// correctionPatterns are phrases that indicate user corrections
var correctionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bno,?\s+(that'?s?\s+)?wrong\b`),
	regexp.MustCompile(`(?i)\bactually,?\s+(it'?s?\s+)?`),
	regexp.MustCompile(`(?i)\byou'?re\s+mistaken\b`),
	regexp.MustCompile(`(?i)\bcorrection:?\b`),
	regexp.MustCompile(`(?i)\bnot\s+\w+,?\s+but\s+\w+\b`),
	regexp.MustCompile(`(?i)\bthat'?s?\s+incorrect\b`),
	regexp.MustCompile(`(?i)\blet me correct\b`),
	regexp.MustCompile(`(?i)\bI meant\b`),
}

// DetectSurprise computes overall surprise score (0.0-1.0)
func (d *SurpriseDetector) DetectSurprise(ctx context.Context, obs Observation) (float64, SurpriseFactors, error) {
	factors := SurpriseFactors{}

	// 1. Term novelty - check for domain-specific terminology
	factors.TermNovelty = d.computeTermNovelty(obs.Content)

	// 2. Correction detection - check if user explicitly corrected
	factors.CorrectionScore = d.detectCorrection(obs.Content)

	// 3. Contradiction check - check if contradicts existing knowledge
	contradictionScore, err := d.checkContradictions(ctx, obs)
	if err != nil {
		// Don't fail, just log and continue
		contradictionScore = 0.0
	}
	factors.ContradictionScore = contradictionScore

	// 4. Embedding novelty - check distance from known concepts
	if len(obs.Embedding) > 0 {
		embNovelty, err := d.computeEmbeddingNovelty(ctx, obs.SpaceID, obs.Embedding)
		if err != nil {
			// Fail-open with a LOUD log — this path was silent, masking the
			// query failure class (SURPRISE-TOPK-001).
			slog.Warn("surprise: embedding novelty query failed", "error", err)
			embNovelty = 0.0
		}
		factors.EmbeddingNovelty = embNovelty
	}

	// Compute weighted overall surprise score
	// Correction is strongest signal (0.4 weight)
	// Term novelty and embedding novelty are moderate (0.25 each)
	// Contradiction is weakest (0.1)
	overallScore := (factors.CorrectionScore * 0.4) +
		(factors.TermNovelty * 0.25) +
		(factors.EmbeddingNovelty * 0.25) +
		(factors.ContradictionScore * 0.1)

	// Clamp to [0.0, 1.0]
	if overallScore > 1.0 {
		overallScore = 1.0
	}
	if overallScore < 0.0 {
		overallScore = 0.0
	}

	return overallScore, factors, nil
}

// computeTermNovelty checks for domain-specific terminology
// Returns 0.0-1.0 based on presence of:
// - Capitalized technical terms (CamelCase, PascalCase)
// - Acronyms (uppercase sequences)
// - Special naming patterns (snake_case, kebab-case with context)
func (d *SurpriseDetector) computeTermNovelty(content string) float64 {
	words := strings.Fields(content)
	if len(words) == 0 {
		return 0.0
	}

	noveltyScore := 0.0
	noveltyCount := 0

	for _, word := range words {
		// Remove punctuation
		cleaned := strings.TrimFunc(word, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-'
		})

		if len(cleaned) < 2 {
			continue
		}

		// Check for PascalCase/CamelCase (e.g., BlueSeerData, myCustomClass)
		if containsMixedCase(cleaned) {
			noveltyScore += 0.3
			noveltyCount++
			continue
		}

		// Check for ACRONYMS (2+ uppercase letters)
		if isAcronym(cleaned) {
			noveltyScore += 0.4
			noveltyCount++
			continue
		}

		// Check for snake_case or kebab-case with technical context
		if (strings.Contains(cleaned, "_") || strings.Contains(cleaned, "-")) && len(cleaned) > 5 {
			noveltyScore += 0.2
			noveltyCount++
			continue
		}

		// Check for technical suffixes
		technicalSuffixes := []string{"API", "SDK", "ORM", "DB", "Service", "Manager", "Handler"}
		for _, suffix := range technicalSuffixes {
			if strings.HasSuffix(cleaned, suffix) {
				noveltyScore += 0.25
				noveltyCount++
				break
			}
		}
	}

	// Normalize by word count, but cap the influence of rare terms
	if noveltyCount == 0 {
		return 0.0
	}

	avgNovelty := noveltyScore / float64(len(words))

	// Boost if significant portion of words are novel
	noveltyRatio := float64(noveltyCount) / float64(len(words))
	if noveltyRatio > 0.3 {
		avgNovelty *= 1.5
	}

	// Clamp to [0.0, 1.0]
	if avgNovelty > 1.0 {
		return 1.0
	}
	return avgNovelty
}

// containsMixedCase checks if a word has mixed case (PascalCase, camelCase)
func containsMixedCase(word string) bool {
	hasUpper := false
	hasLower := false

	for _, r := range word {
		if unicode.IsUpper(r) {
			hasUpper = true
		}
		if unicode.IsLower(r) {
			hasLower = true
		}
		if hasUpper && hasLower {
			return true
		}
	}
	return false
}

// isAcronym checks if a word is an acronym (2+ uppercase letters)
func isAcronym(word string) bool {
	if len(word) < 2 {
		return false
	}

	upperCount := 0
	for _, r := range word {
		if unicode.IsUpper(r) {
			upperCount++
		}
	}

	return upperCount >= 2 && upperCount == len(word)
}

// detectCorrection checks if this is an explicit correction
func (d *SurpriseDetector) detectCorrection(content string) float64 {
	for _, pattern := range correctionPatterns {
		if pattern.MatchString(content) {
			return 0.9 // High surprise for corrections
		}
	}
	return 0.0
}

// checkContradictions checks if obs contradicts existing knowledge using
// vector similarity search and negation heuristics (F2a) or NLI sidecar (F2b).
func (d *SurpriseDetector) checkContradictions(ctx context.Context, obs Observation) (float64, error) {
	checker := NewContradictionChecker(d.embedder, d.driver)
	cfg := DefaultContradictionConfig()
	// CONFIG-DEADFLAG-001: CONTRADICTION_{ENABLED,SIM_THRESHOLD,
	// MAX_CANDIDATES} were parsed but this site always used the package
	// defaults. Defaults match the old literals — zero-config unchanged.
	if !d.cfg.ContradictionEnabled {
		return 0, nil
	}
	if d.cfg.ContradictionSimThreshold > 0 {
		cfg.SimThreshold = d.cfg.ContradictionSimThreshold
	}
	if d.cfg.ContradictionMaxCandidates > 0 {
		cfg.MaxCandidates = d.cfg.ContradictionMaxCandidates
	}
	// F2b: wire NLI settings from config if available.
	if d.cfg.ContradictionNLIEnabled {
		cfg.NLIEnabled = true
		cfg.NLISidecarURL = d.cfg.NeuralRerankURL // Sidecar URL shared with neural reranker
		cfg.NLITimeoutMs = d.cfg.NeuralRerankTimeoutMs
	}
	return checker.CheckContradictions(ctx, obs, cfg)
}

// computeEmbeddingNovelty checks embedding distance from known concepts
func (d *SurpriseDetector) computeEmbeddingNovelty(ctx context.Context, spaceID string, embedding []float32) (float64, error) {
	// Query Neo4j for average cosine similarity with existing conversation observations
	sess := d.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// SURPRISE-TOPK-001: novelty is measured against the K NEAREST
		// neighbors, exact-ranked (was an unordered LIMIT 50 sample — no
		// ORDER BY, similarity-decoupled; live evidence: every reinforcement
		// event ever carried surprise_factor=1.0 and node scores averaged
		// 0.023, because the sample was dominated by EMPTY-ARRAY embeddings
		// whose cosine() yields NULL). Design notes from live verification:
		//   - size(n.embedding) = dims guards the 78% empty-embedding rows
		//     (they pass IS NOT NULL but are uncomparable);
		//   - db.index.vector.queryNodes was REJECTED: the label-wide index
		//     is crowded by ~100k non-conversation nodes (the raw top-200
		//     near a conversation seed were ALL emergent_concept centroids),
		//     so role-filtered hits prune to zero. Exact ORDER BY scan over
		//     the ~1.2k real-embedding conversation rows is deterministic and
		//     O(N·d) ≈ ms; revisit if the role population reaches ~50k.
		topK := d.cfg.SurpriseEmbeddingNoveltyTopK
		if topK <= 0 {
			topK = 50
		}
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE n.role_type = 'conversation_observation'
			  AND size(n.embedding) = $dims
			  AND NOT coalesce(n.is_archived, false)
			WITH n, vector.similarity.cosine(n.embedding, $embedding) AS sim
			WHERE sim >= $simFloor
			ORDER BY sim DESC
			LIMIT $topK
			RETURN avg(sim) as avgSimilarity,
			       count(n) as nodeCount
		`
		params := map[string]any{
			"spaceId":   spaceID,
			"embedding": embedding,
			"dims":      len(embedding),
			"topK":      topK,
			"simFloor":  d.cfg.SurpriseEmbeddingNoveltySimFloor,
		}

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			rec := res.Record()
			avgSim, _ := rec.Get("avgSimilarity")
			count, _ := rec.Get("nodeCount")

			return map[string]any{
				"avgSimilarity": avgSim,
				"nodeCount":     count,
			}, nil
		}

		return map[string]any{
			"avgSimilarity": nil,
			"nodeCount":     int64(0),
		}, res.Err()
	})

	if err != nil {
		return 0.0, err
	}

	resultMap := result.(map[string]any)
	nodeCount := resultMap["nodeCount"].(int64)

	// If no existing observations, this is very novel
	if nodeCount == 0 {
		return 0.8, nil
	}

	avgSim := resultMap["avgSimilarity"]
	if avgSim == nil {
		return 0.8, nil
	}

	avgSimilarity := avgSim.(float64)

	// Convert similarity (high = familiar) to novelty (high = unfamiliar)
	// Cosine similarity ranges from -1 to 1, typically 0 to 1 for our use case
	// Novelty = 1 - similarity
	novelty := 1.0 - avgSimilarity

	// Clamp to [0.0, 1.0]
	if novelty < 0.0 {
		return 0.0, nil
	}
	if novelty > 1.0 {
		return 1.0, nil
	}

	return novelty, nil
}

// cosineSimilarity computes cosine similarity between two vectors
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
