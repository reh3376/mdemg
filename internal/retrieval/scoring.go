package retrieval

import (
	"log"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"mdemg/internal/config"
	"mdemg/internal/models"
)

// scoringDebugEnabled enables detailed tracing in scoring
var scoringDebugEnabled = false

// scoringDebugTerm is the term to trace through scoring
var scoringDebugTerm = "transformerconfig"

// pathPatterns matches common path patterns in query text
// Captures paths like "lib/graphql", "frontend/src", "services/auth", etc.
var pathPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b((?:lib|src|cmd|internal|pkg|services?|modules?|components?|frontend|backend|api|apps?)/[\w\-./]+)`),
	regexp.MustCompile(`(?i)\b([\w\-]+/[\w\-]+/[\w\-./]+\.(?:ts|tsx|js|jsx|go|py|rs|java|rb))\b`),
	regexp.MustCompile(`(?i)\b([\w\-]+(?:Module|Service|Controller|Handler|Repository))\b`),
}

// extractPathHints extracts likely path patterns from a query string
func extractPathHints(query string) []string {
	hints := make(map[string]struct{})

	for _, re := range pathPatterns {
		matches := re.FindAllStringSubmatch(query, -1)
		for _, m := range matches {
			if len(m) > 1 {
				hint := strings.ToLower(m[1])
				// Normalize: remove trailing slashes, clean up
				hint = strings.TrimSuffix(hint, "/")
				if len(hint) > 2 { // Skip very short matches
					hints[hint] = struct{}{}
				}
			}
		}
	}

	// Also extract quoted paths or explicit directory mentions
	quoteRe := regexp.MustCompile(`["']([^"']+/[^"']+)["']`)
	matches := quoteRe.FindAllStringSubmatch(query, -1)
	for _, m := range matches {
		if len(m) > 1 {
			hints[strings.ToLower(m[1])] = struct{}{}
		}
	}

	result := make([]string, 0, len(hints))
	for h := range hints {
		result = append(result, h)
	}
	return result
}

// comparisonPatterns detects comparison-style queries ("X vs Y", "difference between", etc.)
var comparisonPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:difference|differences)\s+between\s+(.+?)\s+and\s+(.+?)\b`),
	regexp.MustCompile(`(?i)\b(.+?)\s+(?:vs\.?|versus|compared\s+to)\s+(.+?)\b`),
	regexp.MustCompile(`(?i)\b(?:both|having both)\s+(.+?)\s+and\s+(.+?)\b`),
	regexp.MustCompile(`(?i)\b(?:compare|comparing)\s+(.+?)\s+(?:and|with|to)\s+(.+?)\b`),
	regexp.MustCompile(`(?i)\bwhy\s+(?:have|are there|do we have)\s+both\s+(.+?)\s+and\s+(.+?)\b`),
	regexp.MustCompile(`(?i)\brelationship\s+between\s+(.+?)\s+and\s+(.+?)\b`),
}

// extractComparisonTargets extracts names being compared in a query
func extractComparisonTargets(query string) []string {
	targets := make(map[string]struct{})

	for _, re := range comparisonPatterns {
		matches := re.FindAllStringSubmatch(query, -1)
		for _, m := range matches {
			for i := 1; i < len(m); i++ {
				target := strings.TrimSpace(m[i])
				// Clean up the target - extract just module/service names
				target = cleanModuleName(target)
				if len(target) > 2 {
					targets[strings.ToLower(target)] = struct{}{}
				}
			}
		}
	}

	result := make([]string, 0, len(targets))
	for t := range targets {
		result = append(result, t)
	}
	return result
}

// cleanModuleName extracts a clean module/service name from a phrase
func cleanModuleName(name string) string {
	// Remove common phrases
	name = regexp.MustCompile(`(?i)^(?:the\s+)?`).ReplaceAllString(name, "")
	name = regexp.MustCompile(`(?i)\s*(?:module|service|controller|handler)s?\s*$`).ReplaceAllString(name, "")

	// Keep just alphanumeric and common separators
	parts := strings.Fields(name)
	if len(parts) > 0 {
		// Take the last word if it looks like a module name
		for i := len(parts) - 1; i >= 0; i-- {
			if regexp.MustCompile(`^[A-Z][a-zA-Z0-9]+$`).MatchString(parts[i]) ||
				strings.Contains(parts[i], "Module") ||
				strings.Contains(parts[i], "Service") {
				return parts[i]
			}
		}
		return parts[len(parts)-1]
	}
	return name
}

// comparisonMatchScore returns a boost for comparison nodes that match query targets
func comparisonMatchScore(name, summary string, tags []string, comparisonTargets []string) float64 {
	if len(comparisonTargets) == 0 {
		return 0.0
	}

	// Strong boost for comparison nodes when query is a comparison
	isComparisonNode := hasTag(tags, "comparison")

	// Check if this node's name/summary mentions the comparison targets
	nameLower := strings.ToLower(name)
	summaryLower := strings.ToLower(summary)
	matchCount := 0

	for _, target := range comparisonTargets {
		if strings.Contains(nameLower, target) || strings.Contains(summaryLower, target) {
			matchCount++
		}
	}

	if matchCount == 0 {
		return 0.0
	}

	// Ratio of targets matched
	matchRatio := float64(matchCount) / float64(len(comparisonTargets))

	if isComparisonNode {
		// Comparison nodes get a strong boost (0.3-0.6) when they match
		return 0.3 + 0.3*matchRatio
	}

	// Regular nodes get a smaller boost (0.1-0.2) for being part of a comparison
	return 0.1 + 0.1*matchRatio
}

// getDecayRate returns the appropriate decay rate (rho) with tag-awareness.
// For L1+ nodes, always uses layer-specific rates.
// For L0 nodes, checks source-type tags when TemporalSourceTypeDecayEnabled is true.
func getDecayRate(layer int, tags []string, cfg config.Config) float64 {
	// L1+ nodes: always use layer-specific rates
	if layer > 0 {
		if layer == 1 {
			return cfg.ScoringRhoL1
		}
		return cfg.ScoringRhoL2
	}

	// L0 nodes: check for source-type overrides (if enabled)
	if cfg.TemporalSourceTypeDecayEnabled {
		if hasTag(tags, "documentation") || hasTag(tags, "markdown") {
			return cfg.ScoringRhoDocumentation
		}
		if hasTag(tags, "config") || hasTag(tags, "configuration") {
			return cfg.ScoringRhoConfig
		}
		if hasTag(tags, "conversation") || hasTag(tags, "conversation_observation") {
			return cfg.ScoringRhoConversation
		}
		if hasTag(tags, "changelog") {
			return cfg.ScoringRhoChangelog
		}
	}

	return cfg.ScoringRhoL0
}

// isCodeQuery returns true if the query appears to be asking about code elements
// rather than documentation or configuration.
func isCodeQuery(query string) bool {
	if query == "" {
		return false
	}
	queryLower := strings.ToLower(query)

	// Code-related keywords that suggest the user wants specific code elements
	// IMPORTANT: Avoid overly general phrases like "how does" which appear in many non-code queries
	codeKeywords := []string{
		" class", "class ",  // "X class" or "class X"
		" function", "function ",
		" method", "method ",
		" struct", "struct ",
		" interface", "interface ",
		" def ", "def ",
		"implementation of", "defined in", "definition of",
		"where is .* defined", "where is .* located",
		" source code",
		"signature", "parameters", "return type",
	}

	for _, kw := range codeKeywords {
		if strings.Contains(queryLower, kw) {
			return true
		}
	}

	// Also check for words at query boundaries
	boundaryKeywords := []string{"class", "function", "method", "struct", "interface"}
	for _, kw := range boundaryKeywords {
		// Match at end of query: "X class"
		if strings.HasSuffix(queryLower, kw) {
			return true
		}
		// Match at start of query: "class X"
		if strings.HasPrefix(queryLower, kw+" ") {
			return true
		}
	}

	return false
}

// isArchitectureQuery returns true if the query appears to be asking about
// high-level architecture, design patterns, or system structure.
func isArchitectureQuery(query string) bool {
	if query == "" {
		return false
	}
	queryLower := strings.ToLower(query)

	// Architecture-related keywords suggest the user wants concepts/patterns
	architectureKeywords := []string{
		"architecture", "design", "pattern", "structure",
		"overview", "module", "service", "component", "layer",
		"system", "workflow", "responsibility", "concern", "abstraction",
	}

	for _, kw := range architectureKeywords {
		if strings.Contains(queryLower, kw) {
			return true
		}
	}

	return false
}

// isRelationshipQuery returns true if the query is asking about relationships,
// dependencies, or interactions between components.
func isRelationshipQuery(query string) bool {
	if query == "" {
		return false
	}
	queryLower := strings.ToLower(query)

	// Relationship-specific keywords
	relationshipKeywords := []string{
		"relationship between", "interact", "dependency", "depends on",
		"calls", "invokes", "connects to", "linked to", "references",
		"uses", "imports", "requires", "communicates with",
		"coupling", "integration", "interface between",
		"how does .* connect", "how does .* interact",
		"what calls", "what uses", "what depends",
	}

	for _, kw := range relationshipKeywords {
		if strings.Contains(queryLower, kw) {
			return true
		}
	}

	return false
}

// isDataFlowQuery returns true if the query is asking about data flow,
// transformations, or pipelines.
func isDataFlowQuery(query string) bool {
	if query == "" {
		return false
	}
	queryLower := strings.ToLower(query)

	// Data flow specific keywords
	dataFlowKeywords := []string{
		"data flow", "data flows", "flows through", "passed to",
		"transforms", "transformation", "pipeline", "processing",
		"input", "output", "request", "response", "payload",
		"how is .* processed", "where does .* come from",
		"what happens to", "path of", "journey of",
		"state", "propagate", "dispatch", "emit", "publish",
		"subscribe", "event", "message", "queue",
	}

	for _, kw := range dataFlowKeywords {
		if strings.Contains(queryLower, kw) {
			return true
		}
	}

	return false
}

// isSymbolLookupQuery returns true if the query is looking for a specific
// symbol, function, class, or identifier by name.
func isSymbolLookupQuery(query string) bool {
	if query == "" {
		return false
	}
	queryLower := strings.ToLower(query)

	// Symbol lookup patterns - looking for specific named entities
	symbolKeywords := []string{
		"where is", "find", "locate", "show me",
		"definition of", "implementation of",
		"what file", "which file", "in what file",
	}

	for _, kw := range symbolKeywords {
		if strings.Contains(queryLower, kw) {
			return true
		}
	}

	// Check for CamelCase or snake_case patterns suggesting a symbol name
	// e.g., "getUserById" or "get_user_by_id"
	camelCasePattern := regexp.MustCompile(`[A-Z][a-z]+[A-Z][a-z]+`)
	snakeCasePattern := regexp.MustCompile(`[a-z]+_[a-z]+_[a-z]+`)
	if camelCasePattern.MatchString(query) || snakeCasePattern.MatchString(query) {
		return true
	}

	return false
}

// QueryGates holds adjusted weights based on query type
type QueryGates struct {
	VectorWeight     float64 // Adjusted alpha for vector similarity
	ActivationWeight float64 // Adjusted beta for activation
	L1Boost          float64 // Multiplier for layer 1+ nodes (concepts)
	QueryType        string  // Detected query type for logging/debugging
}

// RetrievalHints provides query-type aware parameters for the retrieval layer.
// Unlike QueryGates (which affects scoring), these affect what candidates are retrieved.
type RetrievalHints struct {
	SeedN            int            // Number of seeds for graph expansion (default: 50)
	HopDepth         int            // How many hops to expand (default: 2)
	VectorWeight     float64        // Weight for vector in RRF fusion (default: 0.7)
	BM25Weight       float64        // Weight for BM25 in RRF fusion (default: 0.3)
	EnableExpansion  bool           // Whether to do graph expansion (default: true)
	EdgeTypeStrategy string         // Edge type strategy: "structural_first", "learned_only", "hybrid", "all"
	QueryType        string         // Detected query type for logging
	TemporalIntent   TemporalIntent // Parsed temporal understanding
}

// ComputeRetrievalHints returns retrieval parameters based on query type.
// This operates at the retrieval layer to fetch different candidates per query type.
func ComputeRetrievalHints(queryText string, cfg config.Config) RetrievalHints {
	// Default hints from config
	hints := RetrievalHints{
		SeedN:            50,
		HopDepth:         cfg.DefaultHopDepth,
		VectorWeight:     cfg.VectorWeight,
		BM25Weight:       cfg.BM25Weight,
		EnableExpansion:  true,
		EdgeTypeStrategy: cfg.EdgeTypeStrategy,
		QueryType:        "generic",
	}

	// Parse temporal intent if temporal reasoning is enabled
	if cfg.TemporalEnabled {
		hints.TemporalIntent = ParseTemporalIntent(queryText, time.Now())
	}

	// Symbol lookup: maximize vector precision, minimize graph noise
	if isSymbolLookupQuery(queryText) {
		hints.SeedN = 30           // Fewer seeds (more precise)
		hints.HopDepth = 1         // Minimal expansion
		hints.VectorWeight = 0.85  // Strong vector preference
		hints.BM25Weight = 0.15    // Weak BM25
		hints.EnableExpansion = false // Skip graph expansion
		hints.EdgeTypeStrategy = "structural_first"
		hints.QueryType = "symbol_lookup"
		return hints
	}

	// Code query: balance vector with some BM25 for keyword matching
	if isCodeQuery(queryText) {
		hints.SeedN = 40
		hints.HopDepth = 1
		hints.VectorWeight = 0.75
		hints.BM25Weight = 0.25
		hints.EdgeTypeStrategy = "structural_first"
		hints.QueryType = "code"
		return hints
	}

	// Relationship query: emphasize graph traversal
	if isRelationshipQuery(queryText) {
		hints.SeedN = 60           // More seeds for broader coverage
		hints.HopDepth = 2         // Full expansion
		hints.VectorWeight = 0.6   // Balanced
		hints.BM25Weight = 0.4     // Higher BM25 for relationship keywords
		hints.EdgeTypeStrategy = "hybrid"
		hints.QueryType = "relationship"
		return hints
	}

	// Data flow query: trace through graph
	if isDataFlowQuery(queryText) {
		hints.SeedN = 50
		hints.HopDepth = 2
		hints.VectorWeight = 0.65
		hints.BM25Weight = 0.35
		hints.EdgeTypeStrategy = "hybrid"
		hints.QueryType = "data_flow"
		return hints
	}

	// Architecture query: favor concepts and graph structure
	if isArchitectureQuery(queryText) {
		hints.SeedN = 60
		hints.HopDepth = 2
		hints.VectorWeight = 0.55
		hints.BM25Weight = 0.45    // Higher BM25 for architecture keywords
		hints.EdgeTypeStrategy = "hybrid"
		hints.QueryType = "architecture"
		return hints
	}

	return hints
}

// computeQueryGates adjusts scoring weights based on query characteristics.
// Code queries favor vector similarity (finding exact code); architecture queries
// favor activation (finding connected concepts).
func computeQueryGates(queryText string, cfg config.Config) QueryGates {
	// Default: balanced weights from config
	gates := QueryGates{
		VectorWeight:     cfg.ScoringAlpha,
		ActivationWeight: cfg.ScoringBeta,
		L1Boost:          1.0,
		QueryType:        "generic",
	}

	if isCodeQuery(queryText) {
		// Code queries: favor vector similarity (L0 files), slightly penalize concepts
		gates.VectorWeight = cfg.ScoringAlpha * 1.3
		gates.ActivationWeight = cfg.ScoringBeta * 0.7
		gates.L1Boost = 0.85 // Slight penalty for concepts on code queries
		gates.QueryType = "code"
		return gates
	}

	if isArchitectureQuery(queryText) {
		// Architecture queries: favor L1 concepts, boost activation
		gates.VectorWeight = cfg.ScoringAlpha * 0.85
		gates.ActivationWeight = cfg.ScoringBeta * 1.2
		gates.L1Boost = 1.25 // Boost for concepts on architecture queries
		gates.QueryType = "architecture"
		return gates
	}

	return gates
}

// codeTypeBoost returns a multiplier for the score based on code vs config files.
// For code queries: code files get 1.0, config/doc get penalty (< 1.0)
// For non-code queries: all get 1.0 (no change)
func codeTypeBoost(tags []string, isCodeQuery bool) float64 {
	if !isCodeQuery {
		return 1.0 // No adjustment for non-code queries
	}

	// Code file tags get no penalty
	codeTags := []string{"python", "go", "rust", "java", "typescript", "javascript", "c", "cpp", "csharp", "ruby", "php", "scala", "kotlin", "swift"}
	for _, ct := range codeTags {
		if hasTag(tags, ct) {
			return 1.0
		}
	}

	// Config and documentation get penalized for code queries
	if hasTag(tags, "config") || hasTag(tags, "configuration") {
		return 0.5 // 50% penalty
	}
	if hasTag(tags, "documentation") || hasTag(tags, "markdown") {
		return 0.7 // 30% penalty
	}

	return 1.0
}

// pathMatchScore returns a boost score (0.0-1.0) based on how well a node path matches query hints
func pathMatchScore(nodePath string, hints []string) float64 {
	if len(hints) == 0 || nodePath == "" {
		return 0.0
	}

	normalizedPath := strings.ToLower(nodePath)
	bestScore := 0.0

	for _, hint := range hints {
		// Exact substring match (strongest)
		if strings.Contains(normalizedPath, hint) {
			// Score based on how much of the path is matched
			matchRatio := float64(len(hint)) / float64(len(normalizedPath))
			score := 0.5 + 0.5*matchRatio // 0.5 to 1.0
			if score > bestScore {
				bestScore = score
			}
			continue
		}

		// Partial segment match (moderate)
		hintParts := strings.Split(hint, "/")
		pathParts := strings.Split(normalizedPath, "/")
		matchedParts := 0
		for _, hp := range hintParts {
			for _, pp := range pathParts {
				if strings.Contains(pp, hp) || strings.Contains(hp, pp) {
					matchedParts++
					break
				}
			}
		}
		if matchedParts > 0 {
			score := 0.3 * float64(matchedParts) / float64(len(hintParts))
			if score > bestScore {
				bestScore = score
			}
		}
	}

	return bestScore
}

// ScoreAndRank computes the final score per candidate and returns topK results.
// queryText is optional - when provided, enables path-based boosting for architecture queries.
// hints provides query-type and temporal awareness for scoring adjustments.
func ScoreAndRank(cands []Candidate, act map[string]float64, edges []Edge, topK int, cfg config.Config, queryText string, hints RetrievalHints) []models.RetrieveResult {
	scored := ScoreAndRankWithBreakdown(cands, act, edges, topK, cfg, queryText, hints)
	results := make([]models.RetrieveResult, len(scored))
	for i, sc := range scored {
		results[i] = sc.RetrieveResult
	}
	return results
}

// ScoreAndRankWithBreakdown computes scores with detailed breakdowns for Jiminy.
// Returns ScoredCandidate with both the result and the score breakdown.
// hints provides query-type and temporal awareness for scoring adjustments.
func ScoreAndRankWithBreakdown(cands []Candidate, act map[string]float64, edges []Edge, topK int, cfg config.Config, queryText string, hints RetrievalHints) []ScoredCandidate {
	if topK <= 0 {
		topK = 20
	}

	// Local degree estimate from fetched subgraph
	deg := map[string]int{}
	for _, e := range edges {
		deg[e.Src]++
		deg[e.Dst]++
	}

	// Hyperparameters from config (see config.Config for defaults)
	// Note: alpha and beta are now adjusted via query gating below
	gamma := cfg.ScoringGamma       // recency weight
	delta := cfg.ScoringDelta       // confidence weight
	phi := cfg.ScoringPhi           // hub penalty coefficient
	kappa := cfg.ScoringKappa       // redundancy penalty coefficient
	// Note: rho is now layer-specific, computed per-candidate via getLayerDecayRate()
	configBoost := cfg.ScoringConfigBoost // config node boost multiplier
	pathBoost := cfg.ScoringPathBoost     // path match boost coefficient

	// Query gating: adjust weights based on query type (code vs architecture)
	gates := computeQueryGates(queryText, cfg)

	// Extract path hints from query text for architecture-style queries
	pathHints := extractPathHints(queryText)

	// Extract comparison targets for comparison-style queries
	comparisonTargets := extractComparisonTargets(queryText)

	// Detect if this is a code-focused query (for code type boost)
	codeQueryDetected := isCodeQuery(queryText)
	// Detect if this is an architecture query (for bypass multiplier)
	archQueryDetected := isArchitectureQuery(queryText)

	// Redundancy: simple path-prefix clustering
	prefixCount := map[string]int{}
	prefixOf := func(path string) string {
		p := strings.TrimSpace(path)
		if p == "" {
			return ""
		}
		idx := strings.LastIndex(p, "/")
		if idx <= 0 {
			return p
		}
		return p[:idx]
	}
	for _, c := range cands {
		prefixCount[prefixOf(c.Path)]++
	}

	items := make([]ScoredCandidate, 0, len(cands))
	now := time.Now()

	if scoringDebugEnabled {
		log.Printf("[DEBUG Scoring] Processing %d candidates, query='%s', codeQueryDetected=%v", len(cands), queryText, codeQueryDetected)
	}

	// Stale reference penalties (Phase 2 Temporal)
	stalePenalties := map[string]float64{}
	if cfg.TemporalEnabled && cfg.TemporalStaleRefDays > 0 {
		stalePenalties = StaleReferencePenalty(cands, edges, float64(cfg.TemporalStaleRefDays), cfg.TemporalStaleRefMaxPen)
	}

	for _, c := range cands {
		a := act[c.NodeID]
		// Use canonical_time for age (content-relevant time), fallback to UpdatedAt
		ageRef := c.UpdatedAt
		if !c.CanonicalTime.IsZero() {
			ageRef = c.CanonicalTime
		}
		ageDays := now.Sub(ageRef).Hours() / 24.0
		// Source-type-aware decay: checks tags when enabled, else layer-specific
		rho := getDecayRate(c.Layer, c.Tags, cfg)
		r := math.Exp(-rho * ageDays)
		if r < 0 {
			r = 0
		}
		if r > 1 {
			r = 1
		}

		// Hub penalty: exempt concern/hidden/concept nodes (layer > 0) since they're designed as hubs
		h := 0.0
		if c.Layer == 0 {
			h = math.Log(1.0 + float64(deg[c.NodeID]))
		}
		d := float64(prefixCount[prefixOf(c.Path)] - 1) // 0 if unique
		// Cap the redundancy factor to avoid excessive penalties for files in large directories
		if d > 3 {
			d = 3 // Max penalty is 3 * kappa (0.36 at default kappa=0.12)
		}

		// Path boost: reward nodes whose paths match patterns mentioned in query
		pbRaw := pathMatchScore(c.Path, pathHints)
		pb := pbRaw * pathBoost

		// Comparison boost: reward comparison nodes and modules mentioned in comparison queries
		cbRaw := comparisonMatchScore(c.Name, c.Summary, c.Tags, comparisonTargets)
		cb := cbRaw * pathBoost

		// Calculate individual weighted components using gated weights
		vecComponent := gates.VectorWeight * c.VectorSim
		var actComponent float64
		if cfg.ScoringActivationSquared {
			floored := a - cfg.ScoringActivationFloor
			if floored < 0 {
				floored = 0
			}
			actComponent = gates.ActivationWeight * floored * floored
		} else {
			actComponent = gates.ActivationWeight * a
		}
		// Temporal soft-mode: boost recency weight when temporal language detected
		effectiveGamma := gamma
		if hints.TemporalIntent.Mode == TemporalModeSoft {
			effectiveGamma = gamma * cfg.TemporalSoftBoostMultiplier
		}
		recComponent := effectiveGamma * r
		confComponent := delta * c.Confidence
		hubPenComponent := phi * h
		redPenComponent := kappa * d

		// Apply L1 boost for concept nodes (layer > 0) based on query type
		l1BoostEffect := 0.0
		if c.Layer > 0 {
			// L1Boost modifies the effective contribution of concepts
			l1BoostEffect = (gates.L1Boost - 1.0) * (vecComponent + actComponent)
		}

		// Stale reference penalty (Phase 2 Temporal)
		stalePenalty := stalePenalties[c.NodeID]

		// Value residual bypass: high-confidence vector matches get an additive bonus
		var bypassBonus float64
		if c.VectorSim > cfg.ScoringBypassThreshold {
			excess := c.VectorSim - cfg.ScoringBypassThreshold
			bypassMult := 1.0
			if codeQueryDetected {
				bypassMult = cfg.ScoringBypassCodeMult
			}
			if archQueryDetected {
				bypassMult = cfg.ScoringBypassArchMult
			}
			bypassBonus = cfg.ScoringBypassWeight * excess * bypassMult
		}

		s := vecComponent + actComponent + recComponent + confComponent + pb + cb + l1BoostEffect + bypassBonus - hubPenComponent - redPenComponent - stalePenalty

		// Apply code type boost/penalty
		// For code queries: config/doc files get penalized, code files unchanged
		// For non-code queries: apply the configBoost to config files
		codeTypeMult := codeTypeBoost(c.Tags, codeQueryDetected)
		configBoostEffect := 0.0
		if codeQueryDetected {
			// For code queries, apply the penalty/boost from codeTypeBoost
			originalScore := s
			s *= codeTypeMult
			configBoostEffect = s - originalScore // Will be negative for config files
		} else {
			// For non-code queries, use the existing configBoost logic
			if hasTag(c.Tags, "config") {
				originalScore := s
				s *= configBoost
				configBoostEffect = s - originalScore
			}
		}

		// Temporal boost: track the additional recency contribution from temporal soft-mode
		temporalBoost := 0.0
		if effectiveGamma > gamma {
			temporalBoost = (effectiveGamma - gamma) * r
		}

		// Compute LearningEdgeBoost: measures how much CO_ACTIVATED_WITH edges
		// contributed to this node's activation. If activation > vector seed and
		// node has incoming CO_ACTIVATED_WITH edges, the difference is the boost.
		learningEdgeBoost := 0.0
		if a > c.VectorSim && a > 0.01 {
			// Activation exceeded vector seed — learning edges contributed
			learningEdgeBoost = (a - c.VectorSim) * gates.ActivationWeight
			if learningEdgeBoost < 0 {
				learningEdgeBoost = 0
			}
		}
		// Add learning edge boost to final score
		s += learningEdgeBoost

		breakdown := ScoreBreakdown{
			VectorSimilarity:  vecComponent,
			Activation:        actComponent,
			Recency:           recComponent,
			Confidence:        confComponent,
			PathBoost:         pb,
			ComparisonBoost:   cb,
			ConfigBoost:       configBoostEffect,
			L1Boost:           l1BoostEffect,
			HubPenalty:        -hubPenComponent,
			RedundancyPenalty: -redPenComponent,
			RerankDelta:       0, // Set later by reranker
			LearningEdgeBoost: learningEdgeBoost,
			TemporalBoost:     temporalBoost,
			StaleRefPenalty:   -stalePenalty,
			BypassBonus:       bypassBonus,
			FinalScore:        s,
		}

		// Debug: trace target term through scoring
		if scoringDebugEnabled {
			if strings.Contains(strings.ToLower(c.Name), scoringDebugTerm) ||
				strings.Contains(strings.ToLower(c.Path), scoringDebugTerm) {
				log.Printf("[DEBUG Scoring] '%s': NodeID=%s, Name=%s, Layer=%d", scoringDebugTerm, c.NodeID, c.Name, c.Layer)
				log.Printf("[DEBUG Scoring]   Gates: vecW=%.3f, actW=%.3f, l1Boost=%.3f",
					gates.VectorWeight, gates.ActivationWeight, gates.L1Boost)
				log.Printf("[DEBUG Scoring]   Components: vec=%.4f, act=%.4f, rec=%.4f, conf=%.4f, pathBoost=%.4f, compBoost=%.4f, l1Boost=%.4f",
					vecComponent, actComponent, recComponent, confComponent, pb, cb, l1BoostEffect)
				log.Printf("[DEBUG Scoring]   Penalties: hub=%.4f (degree=%d), redundancy=%.4f (prefixCount=%d)",
					hubPenComponent, deg[c.NodeID], redPenComponent, prefixCount[prefixOf(c.Path)])
				log.Printf("[DEBUG Scoring]   Final: %.4f, codeTypeMult=%.2f", s, codeTypeMult)
			}
		}

		items = append(items, ScoredCandidate{
			RetrieveResult: models.RetrieveResult{
				NodeID:     c.NodeID,
				Path:       c.Path,
				Name:       c.Name,
				Summary:    c.Summary,
				Layer:      c.Layer,
				Score:      s,
				VectorSim:  c.VectorSim,
				Activation: a,
			},
			Breakdown: breakdown,
		})
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Score > items[j].Score })

	// Debug: check if target term is in top results before truncation
	if scoringDebugEnabled {
		found := false
		for i, item := range items {
			if strings.Contains(strings.ToLower(item.Name), scoringDebugTerm) ||
				strings.Contains(strings.ToLower(item.Path), scoringDebugTerm) {
				found = true
				log.Printf("[DEBUG Scoring] '%s' ranked #%d of %d (before truncation to topK=%d): Score=%.4f, Name=%s",
					scoringDebugTerm, i+1, len(items), topK, item.Score, item.Name)
				break
			}
		}
		if !found {
			log.Printf("[DEBUG Scoring] '%s' NOT FOUND in %d scored items", scoringDebugTerm, len(items))
		}
	}

	if len(items) > topK {
		items = items[:topK]
	}

	// Debug: check if target term survived truncation
	if scoringDebugEnabled {
		found := false
		for i, item := range items {
			if strings.Contains(strings.ToLower(item.Name), scoringDebugTerm) ||
				strings.Contains(strings.ToLower(item.Path), scoringDebugTerm) {
				found = true
				log.Printf("[DEBUG Scoring] '%s' SURVIVED truncation at rank #%d", scoringDebugTerm, i+1)
				break
			}
		}
		if !found {
			log.Printf("[DEBUG Scoring] '%s' DID NOT survive truncation to topK=%d", scoringDebugTerm, topK)
		}
	}

	// Apply percentile-based normalized confidence
	// This makes confidence immune to learning edge density changes
	ApplyNormalizedConfidence(items)

	return items
}

// hasTag checks if a tag slice contains a specific tag
func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// ScoreBreakdown tracks each component's contribution to the final score.
// Used by Jiminy to explain why a result was ranked where it is.
type ScoreBreakdown struct {
	VectorSimilarity  float64 `json:"vector_similarity"`   // α * VectorSim (gated)
	Activation        float64 `json:"activation"`          // β * activation (gated)
	Recency           float64 `json:"recency"`             // γ * recency factor
	Confidence        float64 `json:"confidence"`          // δ * confidence
	PathBoost         float64 `json:"path_boost"`          // path match boost
	ComparisonBoost   float64 `json:"comparison_boost"`    // comparison query boost
	ConfigBoost       float64 `json:"config_boost"`        // config node multiplier effect
	L1Boost           float64 `json:"l1_boost"`            // query gating boost for concepts (layer > 0)
	HubPenalty        float64 `json:"hub_penalty"`         // -φ * hub factor
	RedundancyPenalty float64 `json:"redundancy_penalty"`  // -κ * redundancy factor
	RerankDelta       float64 `json:"rerank_delta"`        // score change from LLM re-ranking
	LearningEdgeBoost float64 `json:"learning_edge_boost"` // boost from CO_ACTIVATED_WITH traversal
	TemporalBoost     float64 `json:"temporal_boost"`      // additional recency from temporal soft-mode
	StaleRefPenalty   float64 `json:"stale_ref_penalty"`   // penalty for referencing superseded content (Phase 2)
	BypassBonus       float64 `json:"bypass_bonus"`        // value residual bypass for high-confidence vector matches
	FinalScore        float64 `json:"final_score"`         // sum of all components
}

// ScoredCandidate extends RetrieveResult with ScoreBreakdown for Jiminy.
type ScoredCandidate struct {
	models.RetrieveResult
	Breakdown ScoreBreakdown `json:"breakdown"`
}

// ApplyNormalizedConfidence computes percentile-based confidence for each result.
// This makes confidence immune to learning edge density changes (the "activation dilution" problem).
//
// Instead of using absolute score thresholds (which degrade as CO_ACTIVATED_WITH edges accumulate),
// we compute each result's percentile rank within the current result set.
//
// Confidence levels:
//   - HIGH: top 10% (percentile >= 90)
//   - MEDIUM: middle 40-90% (percentile >= 40)
//   - LOW: bottom 40% (percentile < 40)
func ApplyNormalizedConfidence(items []ScoredCandidate) {
	n := len(items)
	if n == 0 {
		return
	}

	// Items are already sorted by score descending
	// Compute percentile for each item based on rank
	// Formula: percentile = 100 * (n-1-rank) / (n-1)
	// - Rank 0 (best) = 100th percentile
	// - Rank n-1 (worst) = 0th percentile
	for i := range items {
		rank := i
		var percentile float64
		if n == 1 {
			percentile = 100.0 // Single result gets top percentile
		} else {
			percentile = 100.0 * float64(n-1-rank) / float64(n-1)
		}

		items[i].NormalizedConfidence = math.Round(percentile*10) / 10 // Round to 1 decimal

		// Assign confidence level based on percentile
		switch {
		case percentile >= 90:
			items[i].ConfidenceLevel = "HIGH"
		case percentile >= 40:
			items[i].ConfidenceLevel = "MEDIUM"
		default:
			items[i].ConfidenceLevel = "LOW"
		}
	}
}

// ConfidenceLevelFromPercentile returns the confidence level for a given percentile.
// Exported for use in other packages that may need to compute confidence levels.
func ConfidenceLevelFromPercentile(percentile float64) string {
	switch {
	case percentile >= 90:
		return "HIGH"
	case percentile >= 40:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

// ApplyNormalizedConfidenceToResults applies percentile-based confidence to final results.
// This should be called AFTER all post-processing (reasoning modules, reranking, truncation)
// to ensure the confidence levels reflect the final ordering.
func ApplyNormalizedConfidenceToResults(results []models.RetrieveResult) {
	n := len(results)
	if n == 0 {
		return
	}

	// Results are already sorted by score descending
	// Compute percentile for each result based on rank
	for i := range results {
		rank := i
		var percentile float64
		if n == 1 {
			percentile = 100.0
		} else {
			percentile = 100.0 * float64(n-1-rank) / float64(n-1)
		}

		results[i].NormalizedConfidence = math.Round(percentile*10) / 10
		results[i].ConfidenceLevel = ConfidenceLevelFromPercentile(percentile)
	}
}
