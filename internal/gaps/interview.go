// Package gaps implements capability gap detection and interviews for MDEMG self-improvement.
package gaps

import (
	"context"
	"fmt"
	"log/slog"
	"mdemg/internal/sanitize"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// GapInterviewer generates and manages interview prompts for capability gaps.
// It identifies high-priority recurring gaps and creates targeted questions
// to help fill knowledge deficiencies.
type GapInterviewer struct {
	db    neo4j.DriverWithContext
	store *Store
}

// NewGapInterviewer creates a new gap interviewer
func NewGapInterviewer(db neo4j.DriverWithContext) *GapInterviewer {
	return &GapInterviewer{
		db:    db,
		store: NewStore(db),
	}
}

// InterviewPrompt represents a generated interview question for a capability gap
type InterviewPrompt struct {
	ID          string    `json:"id"`
	GapID       string    `json:"gap_id"`
	GapType     GapType   `json:"gap_type"`
	Question    string    `json:"question"`
	Context     string    `json:"context"`     // Background context for the question
	Suggestions []string  `json:"suggestions"` // Suggested answer approaches
	Priority    float64   `json:"priority"`
	CreatedAt   time.Time `json:"created_at"`
	Status      string    `json:"status"` // pending, answered, skipped
	SpaceID     string    `json:"space_id,omitempty"`
}

// InterviewResult captures the outcome of an interview session
type InterviewResult struct {
	TotalGapsAnalyzed int               `json:"total_gaps_analyzed"`
	PromptsGenerated  int               `json:"prompts_generated"`
	HighPriorityCount int               `json:"high_priority_count"`
	GapsByType        map[string]int    `json:"gaps_by_type"`
	Prompts           []InterviewPrompt `json:"prompts"`
	ProcessedAt       time.Time         `json:"processed_at"`
	NextScheduledAt   time.Time         `json:"next_scheduled_at"`
}

// InterviewConfig configures the interview process
type InterviewConfig struct {
	MaxPromptsPerRun   int     // Maximum prompts to generate per run (default: 10)
	MinPriority        float64 // Minimum priority to consider (default: 0.3)
	MinOccurrenceCount int     // Minimum occurrences to interview about (default: 3)
	IncludeAddressed   bool    // Include addressed gaps for re-verification (default: false)
}

// DefaultInterviewConfig returns default configuration
func DefaultInterviewConfig() InterviewConfig {
	return InterviewConfig{
		MaxPromptsPerRun:   10,
		MinPriority:        0.3,
		MinOccurrenceCount: 3,
		IncludeAddressed:   false,
	}
}

// GetPriorityGapsForInterview retrieves gaps that need interview attention
func (i *GapInterviewer) GetPriorityGapsForInterview(ctx context.Context, cfg InterviewConfig) ([]CapabilityGap, error) {
	sess := i.db.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		statusFilter := "g.status = 'open'"
		if cfg.IncludeAddressed {
			statusFilter = "g.status IN ['open', 'addressed']"
		}

		cypher := fmt.Sprintf(`
MATCH (g:CapabilityGap)
WHERE %s
  AND g.priority >= $minPriority
  AND g.occurrence_count >= $minOccurrences
RETURN g.gap_id AS gap_id,
       g.type AS type,
       g.description AS description,
       g.evidence AS evidence,
       g.plugin_type AS plugin_type,
       g.plugin_name AS plugin_name,
       g.plugin_description AS plugin_description,
       g.plugin_capabilities AS plugin_capabilities,
       g.priority AS priority,
       g.detected_at AS detected_at,
       g.updated_at AS updated_at,
       g.status AS status,
       g.occurrence_count AS occurrence_count,
       g.space_id AS space_id
ORDER BY g.priority DESC, g.occurrence_count DESC, g.updated_at DESC
LIMIT $limit`, statusFilter)

		params := map[string]any{
			"minPriority":    cfg.MinPriority,
			"minOccurrences": cfg.MinOccurrenceCount,
			"limit":          cfg.MaxPromptsPerRun,
		}

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var gaps []CapabilityGap
		for res.Next(ctx) {
			gap := recordToGap(res.Record())
			if gap != nil {
				gaps = append(gaps, *gap)
			}
		}
		return gaps, res.Err()
	})

	if err != nil {
		return nil, err
	}
	if result == nil {
		return []CapabilityGap{}, nil
	}
	return result.([]CapabilityGap), nil
}

// GenerateInterviewPrompts creates interview prompts for a set of gaps
func (i *GapInterviewer) GenerateInterviewPrompts(gaps []CapabilityGap) []InterviewPrompt {
	var prompts []InterviewPrompt
	now := time.Now()

	for _, gap := range gaps {
		prompt := InterviewPrompt{
			ID:        "interview-" + uuid.New().String()[:8],
			GapID:     gap.ID,
			GapType:   gap.Type,
			Priority:  gap.Priority,
			CreatedAt: now,
			Status:    "pending",
			SpaceID:   gap.SpaceID,
		}

		// Generate type-specific questions
		switch gap.Type {
		case GapTypeDataSource:
			prompt.Question, prompt.Context, prompt.Suggestions = i.generateDataSourcePrompt(gap)
		case GapTypeReasoning:
			prompt.Question, prompt.Context, prompt.Suggestions = i.generateReasoningPrompt(gap)
		case GapTypeQueryPattern:
			prompt.Question, prompt.Context, prompt.Suggestions = i.generateQueryPatternPrompt(gap)
		default:
			prompt.Question, prompt.Context, prompt.Suggestions = i.generateGenericPrompt(gap)
		}

		prompts = append(prompts, prompt)
	}

	return prompts
}

// generateDataSourcePrompt creates prompts for missing data source integrations
func (i *GapInterviewer) generateDataSourcePrompt(gap CapabilityGap) (question, context string, suggestions []string) {
	source := "an external system"
	if len(gap.Evidence) > 0 {
		source = gap.Evidence[0]
	}

	question = fmt.Sprintf("How should MDEMG integrate with %s to access the referenced data?", source)

	context = fmt.Sprintf(
		"This gap has been detected %d times. Content frequently references %s but no integration exists. "+
			"A %s plugin has been suggested: %s",
		gap.OccurrenceCount,
		source,
		gap.SuggestedPlugin.Type,
		gap.SuggestedPlugin.Description,
	)

	suggestions = []string{
		fmt.Sprintf("Create a %s plugin with OAuth2 authentication", gap.SuggestedPlugin.Name),
		fmt.Sprintf("Configure webhook ingestion from %s", source),
		"Set up periodic batch import via API",
		"Document manual import process as workaround",
	}

	return
}

// generateReasoningPrompt creates prompts for missing reasoning capabilities
func (i *GapInterviewer) generateReasoningPrompt(gap CapabilityGap) (question, context string, suggestions []string) {
	domain := extractDomainFromEvidence(gap.Evidence)

	question = fmt.Sprintf("What reasoning or analysis capabilities are needed to better understand %s content?", domain)

	context = fmt.Sprintf(
		"Queries about '%s' consistently score below threshold. This gap has triggered %d times. "+
			"Current retrieval may lack domain-specific understanding.",
		domain,
		gap.OccurrenceCount,
	)

	suggestions = []string{
		fmt.Sprintf("Create domain-specific embeddings for %s terminology", domain),
		"Add specialized re-ranking for this content type",
		"Implement custom query expansion for domain vocabulary",
		"Train or configure LLM prompt for domain expertise",
	}

	return
}

// generateQueryPatternPrompt creates prompts for recurring poor-performing queries
func (i *GapInterviewer) generateQueryPatternPrompt(gap CapabilityGap) (question, context string, suggestions []string) {
	patterns := sanitize.CutRuneSafeSuffix(strings.Join(gap.Evidence, ", "), 200, "...")

	question = fmt.Sprintf("Why do queries containing these patterns consistently return poor results: %s?", patterns)

	context = fmt.Sprintf(
		"These query patterns have resulted in low scores %d times. "+
			"The system may lack content, understanding, or appropriate ranking for these topics.",
		gap.OccurrenceCount,
	)

	suggestions = []string{
		"Ingest more content related to these topics",
		"Add synonyms or aliases to improve query matching",
		"Adjust embedding model or chunk size for this content type",
		"Review if these queries expect content that doesn't exist",
	}

	return
}

// generateGenericPrompt creates prompts for uncategorized gaps
func (i *GapInterviewer) generateGenericPrompt(gap CapabilityGap) (question, context string, suggestions []string) {
	question = fmt.Sprintf("How can this capability gap be addressed: %s?", gap.Description)

	context = fmt.Sprintf(
		"This gap was detected at %s and has occurred %d times with priority %.2f.",
		gap.DetectedAt.Format(time.RFC3339),
		gap.OccurrenceCount,
		gap.Priority,
	)

	suggestions = []string{
		"Investigate root cause of the gap",
		"Create a plugin to address the capability",
		"Document workaround or manual process",
		"Mark as dismissed if not relevant",
	}

	return
}

// RunWeeklyInterview executes the weekly gap interview process
// This is the main APE job entry point
func (i *GapInterviewer) RunWeeklyInterview(ctx context.Context, cfg InterviewConfig) (*InterviewResult, error) {
	slog.Info("Gap Interviewer: starting weekly interview run")

	// Get priority gaps
	gaps, err := i.GetPriorityGapsForInterview(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get priority gaps: %w", err)
	}

	slog.Info("Gap Interviewer: found gaps requiring attention", "count", len(gaps))

	// Generate prompts
	prompts := i.GenerateInterviewPrompts(gaps)

	// Save prompts to Neo4j for tracking
	for _, prompt := range prompts {
		if err := i.SaveInterviewPrompt(ctx, prompt); err != nil {
			slog.Error("Gap Interviewer: failed to save prompt", "prompt_id", prompt.ID, "error", err)
		}
	}

	// Build result summary
	result := &InterviewResult{
		TotalGapsAnalyzed: len(gaps),
		PromptsGenerated:  len(prompts),
		GapsByType:        make(map[string]int),
		Prompts:           prompts,
		ProcessedAt:       time.Now(),
		NextScheduledAt:   time.Now().AddDate(0, 0, 7), // Next week
	}

	for _, gap := range gaps {
		result.GapsByType[string(gap.Type)]++
		if gap.Priority > 0.7 {
			result.HighPriorityCount++
		}
	}

	slog.Info("Gap Interviewer: generated prompts", "prompts_generated", result.PromptsGenerated, "high_priority", result.HighPriorityCount)

	return result, nil
}

// SaveInterviewPrompt persists an interview prompt to Neo4j
func (i *GapInterviewer) SaveInterviewPrompt(ctx context.Context, prompt InterviewPrompt) error {
	sess := i.db.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MERGE (p:InterviewPrompt {prompt_id: $promptId})
ON CREATE SET
    p.gap_id = $gapId,
    p.gap_type = $gapType,
    p.question = $question,
    p.context = $context,
    p.suggestions = $suggestions,
    p.priority = $priority,
    p.created_at = datetime($createdAt),
    p.status = $status,
    p.space_id = $spaceId
ON MATCH SET
    p.updated_at = datetime()
WITH p
MATCH (g:CapabilityGap {gap_id: $gapId})
MERGE (p)-[:INTERVIEWS]->(g)
RETURN p.prompt_id`

		params := map[string]any{
			"promptId":    prompt.ID,
			"gapId":       prompt.GapID,
			"gapType":     string(prompt.GapType),
			"question":    prompt.Question,
			"context":     prompt.Context,
			"suggestions": prompt.Suggestions,
			"priority":    prompt.Priority,
			"createdAt":   prompt.CreatedAt.Format(time.RFC3339),
			"status":      prompt.Status,
			"spaceId":     nilIfEmpty(prompt.SpaceID),
		}

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		_, err = res.Consume(ctx)
		return nil, err
	})

	return err
}

// GetPendingPrompts retrieves all pending interview prompts
func (i *GapInterviewer) GetPendingPrompts(ctx context.Context, spaceID string) ([]InterviewPrompt, error) {
	sess := i.db.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (p:InterviewPrompt)
WHERE p.status = 'pending'
  AND ($spaceId IS NULL OR p.space_id IS NULL OR p.space_id = $spaceId)
RETURN p.prompt_id AS prompt_id,
       p.gap_id AS gap_id,
       p.gap_type AS gap_type,
       p.question AS question,
       p.context AS context,
       p.suggestions AS suggestions,
       p.priority AS priority,
       p.created_at AS created_at,
       p.status AS status,
       p.space_id AS space_id
ORDER BY p.priority DESC, p.created_at DESC`

		var spaceParam any
		if spaceID != "" {
			spaceParam = spaceID
		}

		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceParam})
		if err != nil {
			return nil, err
		}

		var prompts []InterviewPrompt
		for res.Next(ctx) {
			prompt := recordToPrompt(res.Record())
			if prompt != nil {
				prompts = append(prompts, *prompt)
			}
		}
		return prompts, res.Err()
	})

	if err != nil {
		return nil, err
	}
	if result == nil {
		return []InterviewPrompt{}, nil
	}
	return result.([]InterviewPrompt), nil
}

// AnswerPrompt marks a prompt as answered and optionally links to an observation
func (i *GapInterviewer) AnswerPrompt(ctx context.Context, promptID string, observationNodeID string) error {
	sess := i.db.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (p:InterviewPrompt {prompt_id: $promptId})
SET p.status = 'answered',
    p.answered_at = datetime(),
    p.answer_observation_id = $observationId
RETURN p.prompt_id`

		params := map[string]any{
			"promptId":      promptID,
			"observationId": nilIfEmpty(observationNodeID),
		}

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		_, err = res.Consume(ctx)
		return nil, err
	})

	return err
}

// SkipPrompt marks a prompt as skipped
func (i *GapInterviewer) SkipPrompt(ctx context.Context, promptID string, reason string) error {
	sess := i.db.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (p:InterviewPrompt {prompt_id: $promptId})
SET p.status = 'skipped',
    p.skipped_at = datetime(),
    p.skip_reason = $reason
RETURN p.prompt_id`

		params := map[string]any{
			"promptId": promptID,
			"reason":   reason,
		}

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		_, err = res.Consume(ctx)
		return nil, err
	})

	return err
}

// GetInterviewStats returns statistics about interview prompts
func (i *GapInterviewer) GetInterviewStats(ctx context.Context) (map[string]any, error) {
	sess := i.db.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (p:InterviewPrompt)
WITH count(p) AS total,
     sum(CASE WHEN p.status = 'pending' THEN 1 ELSE 0 END) AS pending,
     sum(CASE WHEN p.status = 'answered' THEN 1 ELSE 0 END) AS answered,
     sum(CASE WHEN p.status = 'skipped' THEN 1 ELSE 0 END) AS skipped,
     avg(p.priority) AS avgPriority
RETURN total, pending, answered, skipped, avgPriority`

		res, err := tx.Run(ctx, cypher, nil)
		if err != nil {
			return nil, err
		}

		stats := map[string]any{
			"total":        int64(0),
			"pending":      int64(0),
			"answered":     int64(0),
			"skipped":      int64(0),
			"avg_priority": float64(0),
		}

		if res.Next(ctx) {
			rec := res.Record()
			if v, ok := rec.Get("total"); ok && v != nil {
				stats["total"] = toInt64(v)
			}
			if v, ok := rec.Get("pending"); ok && v != nil {
				stats["pending"] = toInt64(v)
			}
			if v, ok := rec.Get("answered"); ok && v != nil {
				stats["answered"] = toInt64(v)
			}
			if v, ok := rec.Get("skipped"); ok && v != nil {
				stats["skipped"] = toInt64(v)
			}
			if v, ok := rec.Get("avgPriority"); ok && v != nil {
				stats["avg_priority"] = toFloat64(v)
			}
		}
		return stats, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.(map[string]any), nil
}

// Helper functions

func recordToPrompt(rec *neo4j.Record) *InterviewPrompt {
	if rec == nil {
		return nil
	}

	prompt := &InterviewPrompt{}

	if v, ok := rec.Get("prompt_id"); ok && v != nil {
		prompt.ID = fmt.Sprint(v)
	}
	if v, ok := rec.Get("gap_id"); ok && v != nil {
		prompt.GapID = fmt.Sprint(v)
	}
	if v, ok := rec.Get("gap_type"); ok && v != nil {
		prompt.GapType = GapType(fmt.Sprint(v))
	}
	if v, ok := rec.Get("question"); ok && v != nil {
		prompt.Question = fmt.Sprint(v)
	}
	if v, ok := rec.Get("context"); ok && v != nil {
		prompt.Context = fmt.Sprint(v)
	}
	if v, ok := rec.Get("suggestions"); ok && v != nil {
		prompt.Suggestions = toStringSlice(v)
	}
	if v, ok := rec.Get("priority"); ok && v != nil {
		prompt.Priority = toFloat64(v)
	}
	if v, ok := rec.Get("created_at"); ok && v != nil {
		prompt.CreatedAt = toTime(v)
	}
	if v, ok := rec.Get("status"); ok && v != nil {
		prompt.Status = fmt.Sprint(v)
	}
	if v, ok := rec.Get("space_id"); ok && v != nil {
		prompt.SpaceID = fmt.Sprint(v)
	}

	return prompt
}

func extractDomainFromEvidence(evidence []string) string {
	if len(evidence) == 0 {
		return "unknown"
	}
	// Extract domain from evidence like "domain:xyz"
	for _, e := range evidence {
		if strings.HasPrefix(e, "domain:") {
			return strings.TrimPrefix(e, "domain:")
		}
	}
	// Otherwise use first evidence as domain hint
	return evidence[0]
}
