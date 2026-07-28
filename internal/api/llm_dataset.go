package api

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"mdemg/internal/review"
)

// HITL-REVIEW-001 — the 16 MDEMG LLM call sites as reviewable datasets. Each
// reads llm_interactions for its task_name; reviewing an output produces gold
// SFT/quality data for the recursive-retraining loop. Gold-only (NoopSink) —
// these are training signal, not live reinforcement (the guidance sink is the
// special live-reinforcing one).

type llmCallSite struct {
	task        string // llm_interactions.task_name
	display     string
	description string // shown in the UI 'more info' panel
}

// llmCallSiteCatalog — the MDEMG LLM call sites a human SME can review. The
// `description` explains each call site's function + what the SME is judging.
var llmCallSiteCatalog = []llmCallSite{
	{"ape.reflect", "RSIC Reflect", "The RSIC self-improvement reflector reads the self-assessment report and proposes improvement insights/actions each cycle. JUDGE: are its insights sound, grounded in the metrics it was given, and non-hallucinated?"},
	{"consulting.classify", "Consulting: Classify Constraint", "Decides whether a retrieved node is an APPLICABLE constraint for the current context — this gates which guidance Jiminy surfaces. JUDGE: is the applicability call correct for this node + context?"},
	{"jiminy.synthesize", "Jiminy: Synthesize Guidance", "Synthesizes the Jiminy guidance narrative from the applicable constraints. JUDGE: is the synthesized guidance faithful to the constraints and actionable for the agent?"},
	{"jiminy.evaluate_llm", "Jiminy: Outcome Classify", "Tier-2 classifier — did the agent follow / ignore / contradict the surfaced guidance? Feeds trust + effectiveness. JUDGE: does the outcome verdict match what the agent actually did?"},
	{"jiminy.codegen", "Jiminy: Constraint Code-Gen", "Generates the J17 mnemonic constraint code for a constraint (used for compressed T1 encoding). JUDGE: is the code a faithful, unique, legible mnemonic?"},
	{"jiminy.evaluate", "Jiminy: Evaluate", "Evaluates guidance effectiveness / comprehension. JUDGE: is the evaluation judgment sound?"},
	{"hidden.name_emergence", "Hidden: Name Emergence", "Names an emergent concept (a cluster) in the hidden abstraction layer. JUDGE: does the name accurately capture what the cluster's members share?"},
	{"hidden.reclassify", "Hidden: Reclassify", "Re-labels / re-assigns hidden patterns during consolidation. JUDGE: is the reclassification correct?"},
	{"hidden.summarize", "Hidden: Summarize", "Summarizes a theme / cluster. JUDGE: is the summary a faithful, complete abstraction of the members?"},
	{"retrieval.intent_translate", "Retrieval: Intent Translate", "Rewrites the query intent before embedding (query expansion/normalization). JUDGE: does the rewrite preserve the user's intent without drift?"},
	{"retrieval.query_classify", "Retrieval: Query Classify", "Classifies the query type / category to steer the retrieval pipeline. JUDGE: is the classification right for this query?"},
	{"retrieval.rerank_cross", "Retrieval: Cross-Rerank", "Cross-encoder reranking of retrieval candidates. JUDGE: is the relevance ordering correct for the query?"},
	{"retrieval.rerank_nli", "Retrieval: NLI Rerank", "NLI-based reranking (entailment between query and candidate). JUDGE: is the entailment judgment correct?"},
	{"guardrail.evaluate", "Guardrail: Evaluate", "Evaluates a proposed action against the guardrail constraints. JUDGE: is the safety/compliance verdict correct?"},
	{"consulting.synthesis", "Consulting: Synthesis", "Synthesizes consulting guidance from constraints. JUDGE: is the synthesis faithful + useful?"},
	{"summarize.generate", "Summarize", "Generic summarization call site. JUDGE: is the summary faithful, complete, and concise?"},
}

type llmCallSiteDataset struct {
	site          llmCallSite
	pool          *pgxpool.Pool
	rubricVersion string
}

func (d llmCallSiteDataset) ID() string                       { return "llm:" + d.site.task }
func (d llmCallSiteDataset) DisplayName() string              { return d.site.display }
func (d llmCallSiteDataset) Description() string              { return d.site.description }
func (d llmCallSiteDataset) Rubric() review.Rubric           { return review.LLMOutputRubric(d.rubricVersion) }
func (d llmCallSiteDataset) Sink() review.ReinforcementSink  { return review.NoopSink{} } // gold-only

// AutogradePromptHint returns per-call-site guidance teaching the autograder
// what quality LOOKS LIKE for each specific LLM output shape. Falls back to
// generic LLM-output guidance when no site-specific hint is registered.
// HITL-CURATION-002 E2 — implements review.AutogradePromptHinter.
//
// The reinforcement invariant is trivially satisfied for LLM datasets: the
// sink is NoopSink (gold-only), so auto-graded rows can never mutate the
// substrate regardless of the reinforce flag. The Autograder's reinforce:false
// gate is defense-in-depth, not the primary guard here.
func (d llmCallSiteDataset) AutogradePromptHint() string {
	if h, ok := llmAutogradeHints[d.site.task]; ok {
		return h
	}
	return llmAutogradeGenericHint
}

// llmAutogradeGenericHint applies to any LLM call site without a specific
// hint registered. Covers the correctness / format_validity / helpfulness
// axes the LLMOutputRubric names but doesn't typologize.
const llmAutogradeGenericHint = `You are grading the RESPONSE the LLM produced for a specific call site.
The Content field is the LLM's response; the Context field is the prompt it
was given. You are NOT re-generating the response — you are judging what
the model produced against the prompt it received.

correctness: Is the output accurate for what the prompt asked?
  A hallucinated fact, a wrong classification, or an unfounded verdict
  scores low (0-1). A correct output for the input scores high (3-4).

format_validity: Does the output match the CALL SITE's expected shape?
  If the call site expects JSON, unparseable output scores 0. If it expects
  a short label, a paragraph scores low. If it expects a specific enum
  member, values outside the enum score low.

helpfulness: Would this response help the downstream consumer?
  A technically-correct-but-useless output (empty, evasive, over-generic)
  scores low. Adequate output for the pipeline scores 3. Exceptional
  usefulness scores 4.

Be conservative with confidence: LLM output-grading is inherently subjective,
and the operator will review low-confidence items themselves.`

// llmAutogradeHints — per-call-site overrides. Adding a new entry immediately
// improves autograde quality for that call site; call sites without an entry
// fall through to llmAutogradeGenericHint.
var llmAutogradeHints = map[string]string{
	"guardrail.evaluate": `You are grading a GUARDRAIL SAFETY EVALUATION — the LLM was shown a
proposed action (Context) and asked to judge it against safety/compliance
constraints. The Content field is its verdict.

A guardrail verdict is a SHORT STRUCTURED OUTPUT, typically a pass/warn/block
label + a rationale. Grade:

correctness: Did the verdict correctly identify a violation (or its absence)?
  - Approving an action that clearly violates a constraint → correctness 0
  - Blocking a benign action that violates nothing → correctness 0-1
  - Warning on an ambiguous case → correctness 3-4 (warns are safer defaults)
  - Correct pass on benign, correct block on violation → correctness 4

format_validity: Is the verdict parseable as the guardrail-evaluate contract?
  - Expected shape: {verdict: pass|warn|block, rationale: ...}
  - Free-form prose without the verdict field → format 0-1
  - Verdict present but with extra fluff → format 3

helpfulness: Is the rationale specific enough to be actionable?
  - "This might be risky" → 1 (vague, unhelpful)
  - "Violates CONSTRAINT_X because Y" → 4 (cites the rule + reasoning)
  - Silent block with no rationale → 0-1

Anti-patterns (score correctness LOW):
  - Blocking a safe action to be "cautious" without a specific violation
  - Approving a violation because the model interpreted it charitably
  - Warning where a clear block was warranted
  - Passing where a clear warn was warranted`,
}

const llmItemCols = `i.trace_id, COALESCE(i.response,''), COALESCE(i.user_prompt,''),
	COALESCE(i.model_name,''), COALESCE(i.error,''), COALESCE(i.quality,0),
	COALESCE(i.provider,''), COALESCE(i.tokens_out,0), to_char(i.time,'YYYY-MM-DD HH24:MI')`

func (d llmCallSiteDataset) rowToItem(traceID, resp, prompt, model, errStr string, quality float64, provider string, tokensOut int, when string) review.ReviewItem {
	autoLabel := "ok"
	score := quality
	if errStr != "" {
		autoLabel = "error"
		score = 0
	}
	return review.ReviewItem{
		ItemID:    traceID,
		Content:   resp,
		Context:   prompt,
		AutoLabel: autoLabel,
		AutoScore: score,
		Stratum:   model,
		Signals:   map[string]float64{"quality": quality},
		Meta: map[string]string{
			"task_name":   d.site.task,
			"model":       model,
			"provider":    provider,
			"tokens_out":  fmt.Sprintf("%d", tokensOut),
			"error":       errStr,
			"quality":     fmt.Sprintf("%.3f", quality),
			"recorded_at": when,
		},
	}
}

func (d llmCallSiteDataset) FetchCandidates(ctx context.Context, q review.CandidateQuery) ([]review.ReviewItem, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 200
	}
	rows, err := d.pool.Query(ctx, `
		SELECT `+llmItemCols+`
		FROM llm_interactions i
		LEFT JOIN review_grades r
		  ON r.dataset_id = $4 AND r.item_id = i.trace_id
		 AND r.reversed = FALSE AND r.rubric_version = $2
		WHERE i.task_name = $1 AND (i.space_id = $3 OR $3 = '') AND r.item_id IS NULL
		  AND i.trace_id <> ''
		ORDER BY i.time DESC
		LIMIT $5`, d.site.task, d.rubricVersion, q.SpaceID, d.ID(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []review.ReviewItem
	for rows.Next() {
		var traceID, resp, prompt, model, errStr, provider, when string
		var quality float64
		var tokensOut int
		if err := rows.Scan(&traceID, &resp, &prompt, &model, &errStr, &quality, &provider, &tokensOut, &when); err != nil {
			return nil, err
		}
		items = append(items, d.rowToItem(traceID, resp, prompt, model, errStr, quality, provider, tokensOut, when))
	}
	return items, rows.Err()
}

func (d llmCallSiteDataset) FetchItem(ctx context.Context, spaceID, itemID string) (review.ReviewItem, bool, error) {
	var traceID, resp, prompt, model, errStr, provider, when string
	var quality float64
	var tokensOut int
	err := d.pool.QueryRow(ctx, `
		SELECT `+llmItemCols+`
		FROM llm_interactions i
		WHERE i.trace_id = $1 AND i.task_name = $2
		ORDER BY i.time DESC LIMIT 1`, itemID, d.site.task).
		Scan(&traceID, &resp, &prompt, &model, &errStr, &quality, &provider, &tokensOut, &when)
	if err != nil {
		if isNoRowsErr(err) {
			return review.ReviewItem{}, false, nil
		}
		return review.ReviewItem{}, false, err
	}
	return d.rowToItem(traceID, resp, prompt, model, errStr, quality, provider, tokensOut, when), true, nil
}
