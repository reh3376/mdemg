// Package hidden — reclassifier.go implements LLM-driven dynamic reclassification
// of oversized file-extension categories. When a single category (e.g. "typescript")
// dominates the node set, the reclassifier samples summaries, asks an LLM to propose
// semantic sub-categories, and assigns nodes via keyword matching.
package hidden

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"sort"
	"strings"
	"time"

	"mdemg/internal/circuitbreaker"
	"mdemg/internal/llmclient"
)

// reclassSep is the separator between parent and child category in taxonomy keys.
// Dashes are used because they do not conflict with snake_case LLM output.
const reclassSep = "-"

// ReclassifierConfig holds LLM configuration for the reclassifier.
type ReclassifierConfig struct {
	Enabled       bool
	Threshold     float64 // Min fraction of total to trigger (0.25)
	MaxSampleSize int     // Max summaries to send to LLM (150)
	MaxCategories int     // Max sub-categories LLM may propose (10)
	MaxIterations int     // Max reclassification loops (5)
	MaxDepth      int     // Max dot-path depth (4)
	Provider      string  // "openai" or "ollama"
	Model         string
	MaxTokens     int
	TimeoutMs     int
	OpenAIKey     string
	OpenAIURL     string
	OllamaURL     string
}

// SubCategory is a single sub-category proposed by the LLM.
type SubCategory struct {
	Name        string   `json:"name"`
	Keywords    []string `json:"keywords"`
	Description string   `json:"description"`
}

// Reclassifier calls an LLM to discover sub-categories for oversized extension categories.
type Reclassifier struct {
	cfg        ReclassifierConfig
	cbRegistry *circuitbreaker.Registry
	llm        *llmclient.Client

	// CategoryDescriptions maps final category key → LLM description.
	// Populated during reclassification, used for edge labeling.
	CategoryDescriptions map[string]string
}

// NewReclassifier creates a new reclassifier with the given config and optional circuit breaker registry.
func NewReclassifier(cfg ReclassifierConfig, cbRegistry *circuitbreaker.Registry) *Reclassifier {
	baseURL := cfg.OpenAIURL
	apiKey := cfg.OpenAIKey
	if cfg.Provider == "ollama" {
		baseURL = cfg.OllamaURL
	}

	return &Reclassifier{
		cfg:        cfg,
		cbRegistry: cbRegistry,
		llm: llmclient.New(llmclient.Config{
			Provider:  cfg.Provider,
			Model:     cfg.Model,
			APIKey:    apiKey,
			BaseURL:   baseURL,
			TimeoutMs: cfg.TimeoutMs,
		}),
		CategoryDescriptions: make(map[string]string),
	}
}

// ReclassifyOversizedCategories iteratively detects categories that exceed the threshold
// fraction of totalNodes and splits them into LLM-proposed sub-categories. Loops until
// no category exceeds the threshold or MaxIterations is reached.
// Fail-open: if anything goes wrong, the original category is preserved unchanged.
func (r *Reclassifier) ReclassifyOversizedCategories(ctx context.Context, classes map[string][]BaseNode, totalNodes int) map[string][]BaseNode {
	if totalNodes == 0 {
		return classes
	}

	maxIter := r.cfg.MaxIterations
	if maxIter <= 0 {
		maxIter = 5
	}

	result := make(map[string][]BaseNode, len(classes))
	for cat, nodes := range classes {
		result[cat] = nodes
	}

	// Track categories that failed reclassification to avoid retrying
	failed := make(map[string]bool)

	for iteration := 1; iteration <= maxIter; iteration++ {
		oversized := r.detectOversizedCategories(result, totalNodes)

		// Filter out previously failed categories
		var actionable []string
		for _, cat := range oversized {
			if !failed[cat] {
				actionable = append(actionable, cat)
			}
		}

		if len(actionable) == 0 {
			if len(oversized) > 0 {
				log.Printf("Reclassifier: terminated after %d iterations — %d categories stuck at threshold", iteration-1, len(oversized))
			} else if iteration > 1 {
				log.Printf("Reclassifier: converged after %d iterations", iteration-1)
			}
			break
		}

		// Capture max fraction before this iteration for progress detection
		maxFractionBefore := 0.0
		for _, nodes := range result {
			frac := float64(len(nodes)) / float64(totalNodes)
			if frac > maxFractionBefore {
				maxFractionBefore = frac
			}
		}

		log.Printf("Reclassifier: iteration %d — %d oversized categories to split", iteration, len(actionable))

		for _, cat := range actionable {
			nodes := result[cat]
			summaries := r.sampleSummaries(nodes, r.cfg.MaxSampleSize)
			if len(summaries) == 0 {
				log.Printf("Reclassifier: no summaries available for %q, skipping", cat)
				failed[cat] = true
				continue
			}

			subCats, err := r.proposeSubCategories(ctx, cat, len(nodes), summaries)
			if err != nil {
				log.Printf("Reclassifier: LLM failed for %q: %v — keeping category", cat, err)
				failed[cat] = true
				continue
			}

			if len(subCats) < 2 {
				log.Printf("Reclassifier: LLM proposed <2 sub-categories for %q, skipping", cat)
				failed[cat] = true
				continue
			}

			// Truncate to max categories
			if len(subCats) > r.cfg.MaxCategories {
				subCats = subCats[:r.cfg.MaxCategories]
			}

			subMap := r.assignToSubCategories(nodes, cat, subCats)

			// Single-child split guard: if one sub-category holds >= 90% of parent's nodes,
			// the split is ineffective — keep the original category.
			maxSubFraction := 0.0
			for _, subNodes := range subMap {
				frac := float64(len(subNodes)) / float64(len(nodes))
				if frac > maxSubFraction {
					maxSubFraction = frac
				}
			}
			if maxSubFraction >= 0.90 {
				log.Printf("Reclassifier: dominant sub-category holds %.0f%% of %q — split ineffective, skipping", maxSubFraction*100, cat)
				failed[cat] = true
				continue
			}

			delete(result, cat)
			for subCat, subNodes := range subMap {
				result[subCat] = subNodes
			}

			// Store descriptions for edge labeling
			for _, sc := range subCats {
				key := cat + reclassSep + sc.Name
				r.CategoryDescriptions[key] = sc.Description
			}

			// Ensure misc sub-category has a description (may not match any LLM-proposed name)
			miscName := resolveMiscName(subCats)
			miscKey := cat + reclassSep + miscName
			if _, ok := r.CategoryDescriptions[miscKey]; !ok {
				r.CategoryDescriptions[miscKey] = "Miscellaneous files not matching other sub-categories"
			}

			log.Printf("Reclassifier: split %q (%d nodes) into %d sub-categories:", cat, len(nodes), len(subMap))
			for subCat, subNodes := range subMap {
				log.Printf("  %s: %d nodes", subCat, len(subNodes))
			}
		}

		// Progress detection: if the largest category fraction didn't improve by >= 5pp,
		// further iterations won't help.
		maxFractionAfter := 0.0
		for _, nodes := range result {
			frac := float64(len(nodes)) / float64(totalNodes)
			if frac > maxFractionAfter {
				maxFractionAfter = frac
			}
		}
		if maxFractionBefore-maxFractionAfter < 0.05 {
			log.Printf("Reclassifier: insufficient progress (%.1f%% → %.1f%%), stopping", maxFractionBefore*100, maxFractionAfter*100)
			break
		}
	}

	return result
}

// detectOversizedCategories returns category names whose fraction of totalNodes >= threshold.
func (r *Reclassifier) detectOversizedCategories(classes map[string][]BaseNode, totalNodes int) []string {
	maxDepth := r.cfg.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 4
	}

	var oversized []string
	for cat, nodes := range classes {
		// Skip categories already at max taxonomy depth
		depth := strings.Count(cat, reclassSep) + 1 // "typescript" = 1, "typescript-services" = 2
		if depth >= maxDepth {
			continue
		}
		fraction := float64(len(nodes)) / float64(totalNodes)
		if fraction >= r.cfg.Threshold {
			log.Printf("Reclassifier: %q is oversized (%.1f%% of %d total, threshold %.0f%%)",
				cat, fraction*100, totalNodes, r.cfg.Threshold*100)
			oversized = append(oversized, cat)
		}
	}
	sort.Strings(oversized) // deterministic order
	return oversized
}

// sampleSummaries returns a stratified sample of summaries from the given nodes.
// Stratification is by summary prefix (text before first ':').
func (r *Reclassifier) sampleSummaries(nodes []BaseNode, maxSample int) []string {
	// Collect nodes with non-empty summaries
	type indexedNode struct {
		summary string
		prefix  string
	}
	var withSummary []indexedNode
	for _, n := range nodes {
		s := strings.TrimSpace(n.Summary)
		if s == "" {
			continue
		}
		prefix := "other"
		if idx := strings.Index(s, ":"); idx > 0 && idx < 30 {
			prefix = strings.ToLower(strings.TrimSpace(s[:idx]))
		}
		withSummary = append(withSummary, indexedNode{summary: s, prefix: prefix})
	}

	if len(withSummary) == 0 {
		return nil
	}

	if len(withSummary) <= maxSample {
		out := make([]string, len(withSummary))
		for i, n := range withSummary {
			out[i] = n.summary
		}
		return out
	}

	// Group by prefix strata
	strata := make(map[string][]string)
	for _, n := range withSummary {
		strata[n.prefix] = append(strata[n.prefix], n.summary)
	}

	// If fewer than 3 distinct prefixes, fall back to random sampling
	if len(strata) < 3 {
		shuffled := make([]string, len(withSummary))
		for i, n := range withSummary {
			shuffled[i] = n.summary
		}
		rand.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		return shuffled[:maxSample]
	}

	// Proportional sampling from each stratum
	var result []string
	for _, summaries := range strata {
		proportion := float64(len(summaries)) / float64(len(withSummary))
		count := int(proportion*float64(maxSample) + 0.5)
		if count < 1 {
			count = 1
		}
		if count > len(summaries) {
			count = len(summaries)
		}
		// Shuffle within stratum for randomness
		rand.Shuffle(len(summaries), func(i, j int) { summaries[i], summaries[j] = summaries[j], summaries[i] })
		result = append(result, summaries[:count]...)
	}

	// Trim to maxSample if rounding pushed us over
	if len(result) > maxSample {
		result = result[:maxSample]
	}

	return result
}

// buildReclassSystemPrompt returns the system prompt with the threshold percentage injected dynamically.
func buildReclassSystemPrompt(threshold float64) string {
	return fmt.Sprintf(`You are a code classification engine. Given summaries of source files from a single
programming language category that is too large for effective clustering, propose
semantic sub-categories based on their PURPOSE and DOMAIN ROLE.

Rules:
- Propose 3-10 sub-categories
- Each category: short snake_case name, 3-5 keyword patterns matching summary text,
  one-line description
- Categories should be roughly balanced — no single category >%.0f%% of files
- Include a "misc" category for files that don't fit elsewhere
- Output ONLY valid JSON — no markdown, no preamble

Output format: [{"name":"string","keywords":["string"],"description":"string"}]`, threshold*100)
}

// proposeSubCategories sends sampled summaries to the LLM and returns proposed sub-categories.
func (r *Reclassifier) proposeSubCategories(ctx context.Context, category string, totalCount int, summaries []string) ([]SubCategory, error) {
	if !r.cfg.Enabled {
		return nil, fmt.Errorf("reclassifier is disabled")
	}

	timeoutMs := r.cfg.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	// Build user prompt
	var sb strings.Builder
	fmt.Fprintf(&sb, "Language category: %s (%d files, showing %d samples)\n\nSample summaries:\n",
		category, totalCount, len(summaries))
	for i, s := range summaries {
		// Truncate individual summaries to keep prompt reasonable
		if len(s) > 200 {
			s = s[:200] + "..."
		}
		fmt.Fprintf(&sb, "[%d] %s\n", i+1, s)
	}

	userPrompt := sb.String()

	sysPrompt := buildReclassSystemPrompt(r.cfg.Threshold)

	msgs := []llmclient.Message{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: userPrompt},
	}

	maxTokens := r.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2000
	}

	temp := 0.0
	opts := llmclient.CompleteOpts{MaxTokens: maxTokens, Temperature: &temp}
	if r.cfg.Provider == "ollama" {
		opts.Format = ollamaReclassSchema
		opts.Options = map[string]any{"temperature": 0}
	}

	cbName := "openai-reclassifier"
	if r.cfg.Provider == "ollama" {
		cbName = "ollama-reclassifier"
	}

	var raw string
	var err error

	if r.cbRegistry != nil {
		cb := r.cbRegistry.Get(cbName)
		err = cb.Execute(timeoutCtx, func(ctx context.Context) error {
			var innerErr error
			raw, innerErr = r.llm.Complete(ctx, msgs, opts)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return nil, fmt.Errorf("%s circuit breaker open", cbName)
		}
	} else {
		raw, err = r.llm.Complete(timeoutCtx, msgs, opts)
	}

	if err != nil {
		return nil, err
	}

	// Strip markdown code fences if present
	raw = llmclient.StripCodeFence(raw)
	raw = strings.TrimSpace(raw)

	var subCats []SubCategory
	if err := json.Unmarshal([]byte(raw), &subCats); err != nil {
		return nil, fmt.Errorf("invalid JSON from LLM: %w (raw: %s)", err, llmclient.TruncateForLog(raw, 200))
	}

	// Validate and sanitize
	seen := make(map[string]bool)
	var valid []SubCategory
	for _, sc := range subCats {
		sc.Name = strings.TrimSpace(sc.Name)
		// Sanitize name: separator chars → underscores (prevents taxonomy depth corruption)
		sc.Name = strings.ReplaceAll(sc.Name, "-", "_")
		sc.Name = strings.ReplaceAll(sc.Name, ".", "_")
		sc.Name = strings.ReplaceAll(sc.Name, "/", "_")
		sc.Name = strings.ReplaceAll(sc.Name, " ", "_")
		if len(sc.Name) > 50 {
			sc.Name = sc.Name[:50]
		}
		if sc.Name == "" || seen[sc.Name] {
			continue
		}
		seen[sc.Name] = true
		// Lowercase keywords for case-insensitive matching
		for i := range sc.Keywords {
			sc.Keywords[i] = strings.ToLower(strings.TrimSpace(sc.Keywords[i]))
		}
		valid = append(valid, sc)
	}

	if len(valid) == 0 {
		return nil, fmt.Errorf("LLM returned no valid sub-categories")
	}

	// Deduplicate keywords across categories (first-wins strategy).
	// If keyword "service" appears in both category A and B, remove from B.
	globalSeen := make(map[string]bool)
	for i := range valid {
		var deduped []string
		for _, kw := range valid[i].Keywords {
			if !globalSeen[kw] {
				globalSeen[kw] = true
				deduped = append(deduped, kw)
			}
		}
		valid[i].Keywords = deduped
	}

	return valid, nil
}

// assignToSubCategories assigns each node to the best-matching sub-category by keyword count.
func (r *Reclassifier) assignToSubCategories(nodes []BaseNode, parentCategory string, subCats []SubCategory) map[string][]BaseNode {
	result := make(map[string][]BaseNode)

	// Find the misc category name (or use "misc" as default)
	miscName := "misc"
	for _, sc := range subCats {
		if sc.Name == "misc" || sc.Name == "other" || sc.Name == "miscellaneous" {
			miscName = sc.Name
			break
		}
	}

	for _, node := range nodes {
		summaryLower := strings.ToLower(node.Summary)

		bestCat := miscName
		bestScore := 0

		for _, sc := range subCats {
			score := 0
			for _, kw := range sc.Keywords {
				if strings.Contains(summaryLower, kw) {
					score++
				}
			}
			if score > bestScore {
				bestScore = score
				bestCat = sc.Name
			}
		}

		key := parentCategory + reclassSep + bestCat
		result[key] = append(result[key], node)
	}

	return result
}

// resolveMiscName returns the misc category name from the sub-categories list,
// using the same logic as assignToSubCategories.
func resolveMiscName(subCats []SubCategory) string {
	for _, sc := range subCats {
		if sc.Name == "misc" || sc.Name == "other" || sc.Name == "miscellaneous" {
			return sc.Name
		}
	}
	return "misc"
}

// ollamaReclassSchema is the JSON schema for grammar-constrained output (Ollama v0.5+).
var ollamaReclassSchema = json.RawMessage(`{
	"type": "array",
	"items": {
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"keywords": {"type": "array", "items": {"type": "string"}},
			"description": {"type": "string"}
		},
		"required": ["name", "keywords", "description"]
	}
}`)
