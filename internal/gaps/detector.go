// Package gaps implements capability gap detection for MDEMG self-improvement.
// It identifies when MDEMG lacks capabilities and suggests plugin creation to fill those gaps.
package gaps

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// GapType categorizes capability gaps
type GapType string

const (
	GapTypeDataSource    GapType = "data_source"    // Missing integration with external system
	GapTypeReasoning     GapType = "reasoning"      // Missing reasoning/analysis capability
	GapTypeQueryPattern  GapType = "query_pattern"  // Recurring queries with poor results
)

// GapStatus tracks the lifecycle of a capability gap
type GapStatus string

const (
	GapStatusOpen      GapStatus = "open"      // Newly detected, needs attention
	GapStatusAddressed GapStatus = "addressed" // Plugin created or workaround in place
	GapStatusDismissed GapStatus = "dismissed" // Intentionally ignored
)

// PluginType indicates what kind of plugin would address the gap
type PluginType string

const (
	PluginTypeIngestion PluginType = "INGESTION" // Data ingestion plugin
	PluginTypeReasoning PluginType = "REASONING" // Reasoning/analysis plugin
	PluginTypeAPE       PluginType = "APE"       // Active Participant Engine plugin
)

// CapabilityGap represents a detected gap in MDEMG's capabilities
type CapabilityGap struct {
	ID              string           `json:"id"`
	Type            GapType          `json:"type"`
	Description     string           `json:"description"`
	Evidence        []string         `json:"evidence"`          // Example queries/patterns that triggered this
	SuggestedPlugin PluginSuggestion `json:"suggested_plugin"`
	Priority        float64          `json:"priority"`          // 0-1 based on frequency/impact
	DetectedAt      time.Time        `json:"detected_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	Status          GapStatus        `json:"status"`
	OccurrenceCount int              `json:"occurrence_count"`  // How many times this gap was triggered
	SpaceID         string           `json:"space_id,omitempty"` // Optional: space-specific gap
}

// PluginSuggestion describes a plugin that could address a capability gap
type PluginSuggestion struct {
	Type         PluginType `json:"type"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	Capabilities []string   `json:"capabilities"`
}

// QueryMetrics tracks query performance for gap detection
type QueryMetrics struct {
	mu               sync.RWMutex
	queryScores      []queryRecord
	failedPatterns   map[string]int // pattern -> count
	dataSourceRefs   map[string]int // source URI pattern -> count
	lowScoreCount    int64
	totalQueryCount  int64
	windowSize       int           // Number of queries to keep in history
	windowDuration   time.Duration // How long to keep query history
}

type queryRecord struct {
	queryText   string
	avgScore    float64
	resultCount int
	timestamp   time.Time
	spaceID     string
}

// GapDetector identifies capability gaps through analysis of system behavior
type GapDetector struct {
	db      neo4j.DriverWithContext
	store   *Store
	metrics *QueryMetrics

	// Configuration
	lowScoreThreshold    float64       // Queries scoring below this are considered poor
	minOccurrences       int           // Minimum occurrences to create a gap
	analysisWindow       time.Duration // Time window for pattern analysis
	registeredSources    []string      // Known ingestion sources
}

// DetectorConfig configures the gap detector
type DetectorConfig struct {
	LowScoreThreshold  float64       // Default: 0.5
	MinOccurrences     int           // Default: 3
	AnalysisWindow     time.Duration // Default: 24h
	MetricsWindowSize  int           // Default: 1000
	RegisteredSources  []string      // Known ingestion plugin sources
}

// NewGapDetector creates a new gap detector
func NewGapDetector(db neo4j.DriverWithContext, cfg DetectorConfig) *GapDetector {
	if cfg.LowScoreThreshold <= 0 {
		cfg.LowScoreThreshold = 0.5
	}
	if cfg.MinOccurrences <= 0 {
		cfg.MinOccurrences = 3
	}
	if cfg.AnalysisWindow <= 0 {
		cfg.AnalysisWindow = 24 * time.Hour
	}
	if cfg.MetricsWindowSize <= 0 {
		cfg.MetricsWindowSize = 1000
	}

	return &GapDetector{
		db:    db,
		store: NewStore(db),
		metrics: &QueryMetrics{
			queryScores:    make([]queryRecord, 0, cfg.MetricsWindowSize),
			failedPatterns: make(map[string]int),
			dataSourceRefs: make(map[string]int),
			windowSize:     cfg.MetricsWindowSize,
			windowDuration: cfg.AnalysisWindow,
		},
		lowScoreThreshold: cfg.LowScoreThreshold,
		minOccurrences:    cfg.MinOccurrences,
		analysisWindow:    cfg.AnalysisWindow,
		registeredSources: cfg.RegisteredSources,
	}
}

// RecordQueryResult records a query result for analysis
func (d *GapDetector) RecordQueryResult(spaceID, queryText string, avgScore float64, resultCount int) {
	d.metrics.mu.Lock()
	defer d.metrics.mu.Unlock()

	d.metrics.totalQueryCount++

	record := queryRecord{
		queryText:   queryText,
		avgScore:    avgScore,
		resultCount: resultCount,
		timestamp:   time.Now(),
		spaceID:     spaceID,
	}

	// Add to history, removing old entries if needed
	if len(d.metrics.queryScores) >= d.metrics.windowSize {
		d.metrics.queryScores = d.metrics.queryScores[1:]
	}
	d.metrics.queryScores = append(d.metrics.queryScores, record)

	// Track low-scoring queries
	if avgScore < d.lowScoreThreshold {
		d.metrics.lowScoreCount++

		// Extract key terms for pattern tracking
		terms := extractKeyTerms(queryText)
		for _, term := range terms {
			d.metrics.failedPatterns[term]++
		}
	}

	// Scan for data source references
	refs := extractDataSourceReferences(queryText)
	for _, ref := range refs {
		d.metrics.dataSourceRefs[ref]++
	}
}

// RecordContentIngest scans ingested content for external system references
func (d *GapDetector) RecordContentIngest(spaceID string, content string) {
	refs := extractDataSourceReferences(content)

	d.metrics.mu.Lock()
	defer d.metrics.mu.Unlock()

	for _, ref := range refs {
		d.metrics.dataSourceRefs[ref]++
	}
}

// ProcessFeedback and its Feedback type were pruned in DORMANT-CENSUS-001:
// the /v1/feedback intake had zero producers (the live feedback channel is
// /v1/jiminy/feedback via post-tool-observe.py).

// AnalyzeQueryPatterns analyzes query history for capability gaps
func (d *GapDetector) AnalyzeQueryPatterns(ctx context.Context) ([]CapabilityGap, error) {
	d.metrics.mu.RLock()
	defer d.metrics.mu.RUnlock()

	var gaps []CapabilityGap
	now := time.Now()
	cutoff := now.Add(-d.analysisWindow)

	// Analyze failed patterns
	patternCounts := make(map[string]int)
	patternExamples := make(map[string][]string)

	for _, record := range d.metrics.queryScores {
		if record.timestamp.Before(cutoff) {
			continue
		}
		if record.avgScore < d.lowScoreThreshold {
			terms := extractKeyTerms(record.queryText)
			for _, term := range terms {
				patternCounts[term]++
				if len(patternExamples[term]) < 5 {
					patternExamples[term] = append(patternExamples[term], record.queryText)
				}
			}
		}
	}

	// Create gaps for recurring patterns
	for pattern, count := range patternCounts {
		if count < d.minOccurrences {
			continue
		}

		// Check if gap already exists
		existing, _ := d.store.FindGapByPattern(ctx, pattern)
		if existing != nil {
			// Update occurrence count
			existing.OccurrenceCount += count
			existing.UpdatedAt = now
			if err := d.store.SaveGap(ctx, *existing); err != nil {
				continue
			}
		} else {
			gap := CapabilityGap{
				ID:          "gap-" + uuid.New().String()[:8],
				Type:        GapTypeQueryPattern,
				Description: fmt.Sprintf("Recurring queries about '%s' have low scores (avg < %.1f)", pattern, d.lowScoreThreshold),
				Evidence:    patternExamples[pattern],
				SuggestedPlugin: PluginSuggestion{
					Type:         PluginTypeReasoning,
					Name:         fmt.Sprintf("%s-reasoning", sanitizePluginName(pattern)),
					Description:  fmt.Sprintf("Reasoning module to better handle queries about %s", pattern),
					Capabilities: []string{pattern + "_analysis", pattern + "_retrieval"},
				},
				Priority:        calculatePriority(count, d.metrics.totalQueryCount),
				DetectedAt:      now,
				UpdatedAt:       now,
				Status:          GapStatusOpen,
				OccurrenceCount: count,
			}
			gaps = append(gaps, gap)
		}
	}

	return gaps, nil
}

// DetectDataSourceGaps scans for references to external systems without integrations
func (d *GapDetector) DetectDataSourceGaps(ctx context.Context) ([]CapabilityGap, error) {
	d.metrics.mu.RLock()
	sourceRefs := make(map[string]int)
	for k, v := range d.metrics.dataSourceRefs {
		sourceRefs[k] = v
	}
	d.metrics.mu.RUnlock()

	var gaps []CapabilityGap
	now := time.Now()

	for source, count := range sourceRefs {
		if count < d.minOccurrences {
			continue
		}

		// Check if this source is registered
		if d.isRegisteredSource(source) {
			continue
		}

		// Check if gap already exists
		existing, _ := d.store.FindGapByPattern(ctx, source)
		if existing != nil {
			existing.OccurrenceCount += count
			existing.UpdatedAt = now
			if err := d.store.SaveGap(ctx, *existing); err != nil {
				continue
			}
		} else {
			suggestion := suggestIngestionPlugin(source)
			gap := CapabilityGap{
				ID:              "gap-" + uuid.New().String()[:8],
				Type:            GapTypeDataSource,
				Description:     fmt.Sprintf("Content references %s but no integration exists", source),
				Evidence:        []string{source},
				SuggestedPlugin: suggestion,
				Priority:        calculatePriority(count, d.metrics.totalQueryCount),
				DetectedAt:      now,
				UpdatedAt:       now,
				Status:          GapStatusOpen,
				OccurrenceCount: count,
			}
			gaps = append(gaps, gap)
		}
	}

	return gaps, nil
}

// AnalyzeRetrievalFailures identifies content domains with consistently low scores
func (d *GapDetector) AnalyzeRetrievalFailures(ctx context.Context, spaceID string) ([]CapabilityGap, error) {
	// Query Neo4j for nodes that are rarely or poorly retrieved
	sess := d.db.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	var gaps []CapabilityGap

	// Find content domains (by tag or path prefix) with low retrieval success
	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE ($spaceId IS NULL OR n.space_id = $spaceId)
  AND n.tags IS NOT NULL
WITH n.tags AS tags, count(n) AS nodeCount,
     avg(coalesce(n.last_retrieval_score, 0)) AS avgScore
WHERE nodeCount >= 5 AND avgScore < 0.3
UNWIND tags AS tag
WITH tag, sum(nodeCount) AS totalNodes, avg(avgScore) AS domainAvgScore
WHERE totalNodes >= 5
RETURN tag AS domain, totalNodes, domainAvgScore
ORDER BY domainAvgScore ASC
LIMIT 10`

		var spaceParam any
		if spaceID != "" {
			spaceParam = spaceID
		}

		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceParam})
		if err != nil {
			return nil, err
		}

		var domains []map[string]any
		for res.Next(ctx) {
			rec := res.Record()
			domain, _ := rec.Get("domain")
			totalNodes, _ := rec.Get("totalNodes")
			avgScore, _ := rec.Get("domainAvgScore")
			domains = append(domains, map[string]any{
				"domain":     fmt.Sprint(domain),
				"totalNodes": totalNodes,
				"avgScore":   avgScore,
			})
		}
		return domains, res.Err()
	})

	if err != nil {
		return nil, fmt.Errorf("failed to analyze retrieval failures: %w", err)
	}

	now := time.Now()
	if domains, ok := result.([]map[string]any); ok {
		for _, d := range domains {
			domain := d["domain"].(string)
			totalNodes, _ := d["totalNodes"].(int64)
			avgScore, _ := d["avgScore"].(float64)

			gap := CapabilityGap{
				ID:          "gap-" + uuid.New().String()[:8],
				Type:        GapTypeReasoning,
				Description: fmt.Sprintf("Content domain '%s' (%d nodes) has consistently low retrieval scores (avg: %.2f)", domain, totalNodes, avgScore),
				Evidence:    []string{fmt.Sprintf("domain:%s", domain)},
				SuggestedPlugin: PluginSuggestion{
					Type:         PluginTypeReasoning,
					Name:         fmt.Sprintf("%s-reasoning", sanitizePluginName(domain)),
					Description:  fmt.Sprintf("Reasoning module specialized for %s content", domain),
					Capabilities: []string{domain + "_analysis", domain + "_ranking"},
				},
				Priority:        1.0 - avgScore, // Lower score = higher priority
				DetectedAt:      now,
				UpdatedAt:       now,
				Status:          GapStatusOpen,
				OccurrenceCount: int(totalNodes),
				SpaceID:         spaceID,
			}
			gaps = append(gaps, gap)
		}
	}

	return gaps, nil
}

// RunFullAnalysis runs all gap detection methods and returns consolidated results
func (d *GapDetector) RunFullAnalysis(ctx context.Context, spaceID string) ([]CapabilityGap, error) {
	var allGaps []CapabilityGap

	// 1. Query pattern analysis
	patternGaps, err := d.AnalyzeQueryPatterns(ctx)
	if err == nil {
		allGaps = append(allGaps, patternGaps...)
	}

	// 2. Data source gap detection
	dataSourceGaps, err := d.DetectDataSourceGaps(ctx)
	if err == nil {
		allGaps = append(allGaps, dataSourceGaps...)
	}

	// 3. Retrieval failure analysis
	retrievalGaps, err := d.AnalyzeRetrievalFailures(ctx, spaceID)
	if err == nil {
		allGaps = append(allGaps, retrievalGaps...)
	}

	// Save all new gaps
	for _, gap := range allGaps {
		if err := d.store.SaveGap(ctx, gap); err != nil {
			continue // Log but don't fail
		}
	}

	// Return all open gaps (including previously detected)
	return d.store.ListGaps(ctx, GapStatusOpen, "")
}

// GetMetrics returns current metrics snapshot
func (d *GapDetector) GetMetrics() map[string]any {
	d.metrics.mu.RLock()
	defer d.metrics.mu.RUnlock()

	return map[string]any{
		"total_queries":       d.metrics.totalQueryCount,
		"low_score_queries":   d.metrics.lowScoreCount,
		"tracked_patterns":    len(d.metrics.failedPatterns),
		"tracked_sources":     len(d.metrics.dataSourceRefs),
		"history_size":        len(d.metrics.queryScores),
		"low_score_threshold": d.lowScoreThreshold,
	}
}

// isRegisteredSource checks if a source pattern is already handled
func (d *GapDetector) isRegisteredSource(source string) bool {
	for _, registered := range d.registeredSources {
		if strings.Contains(source, registered) {
			return true
		}
	}
	return false
}

// Helper functions

// extractKeyTerms extracts meaningful terms from a query
func extractKeyTerms(query string) []string {
	// Remove common stop words and extract key terms
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"what": true, "how": true, "why": true, "where": true, "when": true,
		"do": true, "does": true, "did": true, "can": true, "could": true,
		"should": true, "would": true, "to": true, "for": true, "in": true,
		"on": true, "at": true, "by": true, "with": true, "about": true,
		"this": true, "that": true, "these": true, "those": true,
	}

	words := strings.Fields(strings.ToLower(query))
	var terms []string
	for _, word := range words {
		word = strings.Trim(word, ".,?!\"'")
		if len(word) > 2 && !stopWords[word] {
			terms = append(terms, word)
		}
	}
	return terms
}

// extractDataSourceReferences scans text for external system references
func extractDataSourceReferences(text string) []string {
	var refs []string
	text = strings.ToLower(text)

	// URL patterns for common integrations
	patterns := []struct {
		pattern string
		name    string
	}{
		// SEC-TRANCHE-3: anchor each host pattern with a leading word
		// boundary so `evilgithub.com/…` (arbitrary hosts before the target)
		// does not match the `github` integration name. Trailing `\S+`
		// stays greedy — we only want to force the LEFT side of the URL
		// authority to be a real boundary.
		{`\bslack://\S+`, "slack"},
		{`\bjira://\S+`, "jira"},
		{`\bconfluence://\S+`, "confluence"},
		{`(?:^|[\s(]|https?://)github\.com/\S+`, "github"},
		{`\bgitlab\.com/\S+`, "gitlab"},
		{`\bnotion\.so/\S+`, "notion"},
		{`\blinear\.app/\S+`, "linear"},
		{`\bhttps?://(?:www\.)?asana\.com/\S+`, "asana"},
		{`\btrello\.com/\S+`, "trello"},
		{`(?:^|[\s(])docs\.google\.com/\S+`, "google_docs"},
		{`\bdrive\.google\.com/\S+`, "google_drive"},
		{`(?:^|\s)dropbox\.com/\S+(?:$|\s)`, "dropbox"},
		{`\bfigma\.com/\S+`, "figma"},
	}

	for _, p := range patterns {
		re := regexp.MustCompile(p.pattern)
		if re.MatchString(text) {
			refs = append(refs, p.name)
		}
	}

	// Also check for plain text mentions
	mentions := []struct {
		keywords []string
		name     string
	}{
		{[]string{"slack channel", "slack message", "in slack", "from slack"}, "slack"},
		{[]string{"jira ticket", "jira issue", "in jira", "from jira"}, "jira"},
		{[]string{"confluence page", "confluence doc", "in confluence"}, "confluence"},
		{[]string{"github issue", "github pr", "pull request"}, "github"},
		{[]string{"notion page", "notion doc"}, "notion"},
		{[]string{"linear issue", "linear ticket"}, "linear"},
	}

	for _, m := range mentions {
		for _, kw := range m.keywords {
			if strings.Contains(text, kw) {
				refs = append(refs, m.name)
				break
			}
		}
	}

	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, ref := range refs {
		if !seen[ref] {
			seen[ref] = true
			unique = append(unique, ref)
		}
	}

	return unique
}

// suggestIngestionPlugin suggests an ingestion plugin for a data source
func suggestIngestionPlugin(source string) PluginSuggestion {
	suggestions := map[string]PluginSuggestion{
		"slack": {
			Type:         PluginTypeIngestion,
			Name:         "slack-ingestion",
			Description:  "Ingest Slack messages and channels",
			Capabilities: []string{"slack_messages", "slack_channels", "slack_threads"},
		},
		"jira": {
			Type:         PluginTypeIngestion,
			Name:         "jira-ingestion",
			Description:  "Ingest Jira issues, comments, and project data",
			Capabilities: []string{"jira_issues", "jira_comments", "jira_projects"},
		},
		"confluence": {
			Type:         PluginTypeIngestion,
			Name:         "confluence-ingestion",
			Description:  "Ingest Confluence pages and spaces",
			Capabilities: []string{"confluence_pages", "confluence_spaces"},
		},
		"github": {
			Type:         PluginTypeIngestion,
			Name:         "github-ingestion",
			Description:  "Ingest GitHub issues, PRs, and discussions",
			Capabilities: []string{"github_issues", "github_prs", "github_discussions"},
		},
		"notion": {
			Type:         PluginTypeIngestion,
			Name:         "notion-ingestion",
			Description:  "Ingest Notion pages and databases",
			Capabilities: []string{"notion_pages", "notion_databases"},
		},
		"linear": {
			Type:         PluginTypeIngestion,
			Name:         "linear-ingestion",
			Description:  "Ingest Linear issues and projects",
			Capabilities: []string{"linear_issues", "linear_projects"},
		},
		"google_docs": {
			Type:         PluginTypeIngestion,
			Name:         "gdocs-ingestion",
			Description:  "Ingest Google Docs documents",
			Capabilities: []string{"google_docs"},
		},
		"figma": {
			Type:         PluginTypeIngestion,
			Name:         "figma-ingestion",
			Description:  "Ingest Figma design files and comments",
			Capabilities: []string{"figma_files", "figma_comments"},
		},
	}

	if suggestion, ok := suggestions[source]; ok {
		return suggestion
	}

	// Generic suggestion
	return PluginSuggestion{
		Type:         PluginTypeIngestion,
		Name:         fmt.Sprintf("%s-ingestion", sanitizePluginName(source)),
		Description:  fmt.Sprintf("Ingest data from %s", source),
		Capabilities: []string{source + "_data"},
	}
}

// calculatePriority calculates gap priority based on occurrence frequency
func calculatePriority(occurrences int, total int64) float64 {
	if total == 0 {
		return 0.5
	}
	// Priority scales with occurrence ratio, capped at 1.0
	ratio := float64(occurrences) / float64(total)
	priority := ratio * 10 // Scale up since ratios are typically small
	if priority > 1.0 {
		priority = 1.0
	}
	if priority < 0.1 {
		priority = 0.1
	}
	return priority
}

// sanitizePluginName converts a string to a valid plugin name
func sanitizePluginName(name string) string {
	// Convert to lowercase and replace non-alphanumeric with hyphens
	re := regexp.MustCompile(`[^a-z0-9]+`)
	sanitized := re.ReplaceAllString(strings.ToLower(name), "-")
	sanitized = strings.Trim(sanitized, "-")
	if len(sanitized) > 50 {
		sanitized = sanitized[:50]
	}
	return sanitized
}

// GapsSummary provides a summary of capability gaps
type GapsSummary struct {
	Total        int            `json:"total"`
	ByType       map[string]int `json:"by_type"`
	HighPriority int            `json:"high_priority"` // Priority > 0.7
}

// GetGapsSummary returns a summary of all gaps
func (d *GapDetector) GetGapsSummary(ctx context.Context) (*GapsSummary, error) {
	gaps, err := d.store.ListGaps(ctx, "", "")
	if err != nil {
		return nil, err
	}

	summary := &GapsSummary{
		Total:  len(gaps),
		ByType: make(map[string]int),
	}

	for _, gap := range gaps {
		summary.ByType[string(gap.Type)]++
		if gap.Priority > 0.7 {
			summary.HighPriority++
		}
	}

	return summary, nil
}

// SortGapsByPriority sorts gaps by priority (highest first)
func SortGapsByPriority(gaps []CapabilityGap) {
	sort.Slice(gaps, func(i, j int) bool {
		return gaps[i].Priority > gaps[j].Priority
	})
}

// GetStore returns the gap store for direct access
func (d *GapDetector) GetStore() *Store {
	return d.store
}
