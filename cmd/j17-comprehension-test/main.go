// j17-comprehension-test runs an iterative protocol comprehension learning loop
// between Jiminy (sender) and an LLM (receiver). Each iteration:
//   1. Encodes constraints at T1/T2/T3 and tests LLM comprehension
//   2. Feeds results to MDEMG's protocol feedback endpoint (RSIC learning)
//   3. Identifies weak codes and regenerates them via the learn endpoint
//   4. Re-tests with improved codes
//
// Usage: go run ./cmd/j17-comprehension-test [--iterations N] [--mdemg-url URL]
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"mdemg/internal/jiminy"
	"mdemg/internal/llmclient"
)

// testConstraint defines a constraint with known intent for testing.
type testConstraint struct {
	ID          string
	Type        string // "constraint", "correction", "pattern", "decision"
	Severity    string // "must_not", "must", "should", "should_not", "info"
	Content     string // full NL description (the sender's intent)
	Code        string // T1 mnemonic code (if assigned)
	Intent      string // human-readable statement of what the sender means
	EscLevel    int    // escalation level
	SourceNodes []string
	Annotations map[string]string // T1 inline annotations (alt, neg, scope, ctx)
	GlossaryDef string            // short definition for bootstrap glossary
}

// trialResult captures one encoding trial.
type trialResult struct {
	ConstraintID   string  `json:"constraint_id"`
	Tier           int     `json:"tier"`
	EncodedMessage string  `json:"encoded_message"`
	SenderIntent   string  `json:"sender_intent"`
	Interpretation string  `json:"receiver_interpretation"`
	JudgeScore     float64 `json:"judge_score"`
	JudgeReason    string  `json:"judge_reason"`
	TokensEncoded  int     `json:"tokens_encoded"`
	TokensOriginal int     `json:"tokens_original"`
	Code           string  `json:"code,omitempty"`
	Iteration      int     `json:"iteration"`
}

// iterationLog captures one iteration of the learning loop.
type iterationLog struct {
	Iteration     int            `json:"iteration"`
	Timestamp     string         `json:"timestamp"`
	Trials        []trialResult  `json:"trials"`
	Summary       summary        `json:"summary"`
	CodeChanges   []codeChange   `json:"code_changes,omitempty"`
	FeedbackResp  any            `json:"feedback_response,omitempty"`
}

// codeChange records a code regeneration.
type codeChange struct {
	ConstraintID string  `json:"constraint_id"`
	OldCode      string  `json:"old_code"`
	NewCode      string  `json:"new_code"`
	ScoreBefore  float64 `json:"score_before"`
	Reason       string  `json:"reason"`
}

// experimentLog is the full learning loop output.
type experimentLog struct {
	Timestamp  string         `json:"timestamp"`
	Model      string         `json:"model"`
	Bootstrap  string         `json:"bootstrap_header"`
	Iterations []iterationLog `json:"iterations"`
	FinalSummary summary      `json:"final_summary"`
}

type summary struct {
	TotalTrials       int     `json:"total_trials"`
	AvgScoreT1        float64 `json:"avg_score_t1"`
	AvgScoreT2        float64 `json:"avg_score_t2"`
	AvgScoreT3        float64 `json:"avg_score_t3"`
	AvgCompressionT1  float64 `json:"avg_compression_t1"`
	AvgCompressionT2  float64 `json:"avg_compression_t2"`
	PerfectScoreCount int     `json:"perfect_score_count"`
}

const (
	scoreThreshold = 9.5 // codes scoring below this trigger regeneration (>95% comprehension target)
	mdemgDefault   = "http://localhost:9999"
)

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		if data, err := os.ReadFile(".env"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if after, ok := strings.CutPrefix(line, "OPENAI_API_KEY="); ok {
					apiKey = strings.TrimSpace(after)
				}
			}
		}
	}
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY not set")
	}

	// Configuration
	model := "gpt-5.4"
	baseURL := "https://api.openai.com/v1"
	maxIterations := 3
	mdemgURL := mdemgDefault

	// Parse CLI args
	for i, arg := range os.Args[1:] {
		switch arg {
		case "--iterations":
			if i+2 < len(os.Args) {
				fmt.Sscanf(os.Args[i+2], "%d", &maxIterations)
			}
		case "--mdemg-url":
			if i+2 < len(os.Args) {
				mdemgURL = os.Args[i+2]
			}
		}
	}

	client := llmclient.New(llmclient.Config{
		Provider:  "openai",
		Model:     model,
		APIKey:    apiKey,
		BaseURL:   baseURL,
		TimeoutMs: 60000,
	})

	ctx := context.Background()
	encoder := jiminy.NewProtocolEncoder(1)

	// Build glossary from test constraints and generate enhanced bootstrap
	constraints := buildTestConstraints()
	glossary := buildGlossary(constraints)
	encoder.SetGlossary(glossary)
	bootstrap := jiminy.FormatBootstrapWithGlossary(glossary)

	// Check if MDEMG server is available for feedback loop
	mdemgAvailable := checkMDEMG(mdemgURL)
	if mdemgAvailable {
		log.Printf("MDEMG server available at %s — feedback loop ENABLED", mdemgURL)
	} else {
		log.Printf("MDEMG server not available at %s — running without feedback loop", mdemgURL)
	}

	// Constraints already loaded above for glossary

	log.Printf("=== J17 Protocol Comprehension Learning Loop ===")
	log.Printf("Model: %s | Max iterations: %d | Threshold: %.0f/10", model, maxIterations, scoreThreshold)
	log.Printf("Constraints: %d (with codes: %d)", len(constraints), countCoded(constraints))
	log.Println()

	var iterations []iterationLog

	for iter := 1; iter <= maxIterations; iter++ {
		log.Printf("══════════════════════════════════════")
		log.Printf("  ITERATION %d / %d", iter, maxIterations)
		log.Printf("══════════════════════════════════════")

		// Run all trials for this iteration
		trials := runTrials(ctx, client, encoder, bootstrap, constraints, iter)
		s := computeSummary(trials)

		iterLog := iterationLog{
			Iteration: iter,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Trials:    trials,
			Summary:   s,
		}

		log.Printf("\nIteration %d results: T1=%.1f T2=%.1f T3=%.1f (perfect: %d/%d)",
			iter, s.AvgScoreT1, s.AvgScoreT2, s.AvgScoreT3, s.PerfectScoreCount, s.TotalTrials)

		// Feed results to MDEMG if available
		if mdemgAvailable {
			feedbackResp := postFeedback(mdemgURL, trials)
			iterLog.FeedbackResp = feedbackResp
		}

		// Check convergence: ALL tiers must average >= 9.5/10
		converged := s.AvgScoreT1 >= scoreThreshold && s.AvgScoreT2 >= scoreThreshold && s.AvgScoreT3 >= scoreThreshold
		if converged {
			log.Printf("\nAll tiers >= %.1f/10 — learning converged! (T1=%.1f T2=%.1f T3=%.1f)",
				scoreThreshold, s.AvgScoreT1, s.AvgScoreT2, s.AvgScoreT3)
			iterations = append(iterations, iterLog)
			break
		}

		// Identify weak T1 codes and regenerate
		weakCodes := findWeakCodes(trials, scoreThreshold)
		if len(weakCodes) == 0 && s.AvgScoreT1 >= scoreThreshold {
			log.Printf("\nT1 codes converged at %.1f/10 — checking other tiers", s.AvgScoreT1)
			if s.AvgScoreT2 < scoreThreshold {
				log.Printf("  T2 avg %.1f < %.1f — needs improvement (protocol-level fix required)", s.AvgScoreT2, scoreThreshold)
			}
			if s.AvgScoreT3 < scoreThreshold {
				log.Printf("  T3 avg %.1f < %.1f — needs improvement", s.AvgScoreT3, scoreThreshold)
			}
			iterations = append(iterations, iterLog)
			break
		}

		log.Printf("\n%d weak code(s) found — regenerating:", len(weakCodes))
		var changes []codeChange
		for _, wc := range weakCodes {
			log.Printf("  %s (score %.1f): %s", wc.code, wc.avgScore, wc.reason)

			// Try to regenerate via MDEMG server first, fall back to direct LLM
			newCode := ""
			if mdemgAvailable {
				newCode = learnViaServer(mdemgURL, wc)
			}
			if newCode == "" {
				newCode = learnViaLLM(ctx, client, wc)
			}

			if newCode != "" && newCode != wc.code {
				changes = append(changes, codeChange{
					ConstraintID: wc.constraintID,
					OldCode:      wc.code,
					NewCode:      newCode,
					ScoreBefore:  wc.avgScore,
					Reason:       wc.reason,
				})

				// Update the constraint for the next iteration
				for i := range constraints {
					if constraints[i].ID == wc.constraintID {
						log.Printf("    %s → %s", constraints[i].Code, newCode)
						constraints[i].Code = newCode
						break
					}
				}
			}
		}
		iterLog.CodeChanges = changes
		iterations = append(iterations, iterLog)

		if len(changes) == 0 {
			log.Printf("\nNo code improvements generated — stopping")
			break
		}
		log.Println()
	}

	// Final summary
	finalTrials := iterations[len(iterations)-1].Trials
	finalSummary := computeSummary(finalTrials)

	expLog := experimentLog{
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Model:        model,
		Bootstrap:    bootstrap,
		Iterations:   iterations,
		FinalSummary: finalSummary,
	}

	// Write output files
	ts := time.Now().Format("20060102_150405")
	outDir := "docs/architecture/benchmarks"
	if err := os.MkdirAll(outDir, 0755); err != nil {
		log.Printf("Warning: mkdir: %v", err)
	}

	jsonPath := fmt.Sprintf("%s/j17_learning_%s.json", outDir, ts)
	data, _ := json.MarshalIndent(expLog, "", "  ")
	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		log.Printf("Warning: write json: %v", err)
	}

	mdPath := fmt.Sprintf("%s/j17_learning_%s.md", outDir, ts)
	writeLearningReport(mdPath, expLog)

	// Print final results
	fmt.Println("\n═══ J17 LEARNING LOOP RESULTS ═══")
	fmt.Printf("Iterations: %d\n", len(iterations))

	if len(iterations) > 1 {
		first := iterations[0].Summary
		last := iterations[len(iterations)-1].Summary
		fmt.Printf("\nT1 score: %.1f → %.1f  (%+.1f)\n", first.AvgScoreT1, last.AvgScoreT1, last.AvgScoreT1-first.AvgScoreT1)
		fmt.Printf("T2 score: %.1f → %.1f  (%+.1f)\n", first.AvgScoreT2, last.AvgScoreT2, last.AvgScoreT2-first.AvgScoreT2)

		// Count total code changes
		totalChanges := 0
		for _, iter := range iterations {
			totalChanges += len(iter.CodeChanges)
		}
		fmt.Printf("Code regenerations: %d\n", totalChanges)
	} else {
		fmt.Printf("T1 avg: %.1f | T2 avg: %.1f | T3 avg: %.1f\n",
			finalSummary.AvgScoreT1, finalSummary.AvgScoreT2, finalSummary.AvgScoreT3)
	}

	fmt.Printf("Perfect scores: %d/%d\n", finalSummary.PerfectScoreCount, finalSummary.TotalTrials)
	fmt.Printf("T1 compression: %.1fx\n", finalSummary.AvgCompressionT1)
	fmt.Printf("\nJSON log: %s\n", jsonPath)
	fmt.Printf("Report:   %s\n", mdPath)
}

// ── Trial execution ──

func runTrials(ctx context.Context, client *llmclient.Client, encoder *jiminy.ProtocolEncoder,
	bootstrap string, constraints []testConstraint, iteration int) []trialResult {

	var trials []trialResult

	for _, c := range constraints {
		item := jiminy.GuidanceItem{
			Type:            mapType(c.Type),
			Priority:        mapSeverity(c.Severity),
			Content:         c.Content,
			Confidence:      0.9,
			SourceNodes:     c.SourceNodes,
			ConstraintCode:  c.Code,
			EscalationLevel: escLevelToConst(c.EscLevel),
			Annotations:     c.Annotations,
		}

		for _, tier := range []int{1, 2, 3} {
			if tier == 1 && c.Code == "" {
				continue
			}

			item.Tier = tier
			encoded := encoder.FormatItem(item, tier)
			originalTokens := estimateTokens(c.Content)
			encodedTokens := estimateTokens(encoded)

			log.Printf("[%s] Tier %d: %q", c.ID, tier, encoded)

			interpretation := interpretMessage(ctx, client, bootstrap, encoded, tier)
			score, reason := judgeComprehension(ctx, client, c.Intent, interpretation)

			trials = append(trials, trialResult{
				ConstraintID:   c.ID,
				Tier:           tier,
				EncodedMessage: encoded,
				SenderIntent:   c.Intent,
				Interpretation: interpretation,
				JudgeScore:     score,
				JudgeReason:    reason,
				TokensEncoded:  encodedTokens,
				TokensOriginal: originalTokens,
				Code:           c.Code,
				Iteration:      iteration,
			})

			log.Printf("  -> Score: %.1f/10 | Compression: %.1fx | %s",
				score, float64(originalTokens)/max(float64(encodedTokens), 1), reason)
		}
	}
	return trials
}

// ── Weak code detection ──

type weakCode struct {
	constraintID string
	code         string
	avgScore     float64
	reason       string
	content      string // full NL for regeneration
	cType        string
}

func findWeakCodes(trials []trialResult, threshold float64) []weakCode {
	// Aggregate T1 scores per code
	type codeAgg struct {
		scores       []float64
		constraintID string
		interpretation string
		intent       string
	}
	agg := make(map[string]*codeAgg)

	for _, t := range trials {
		if t.Tier != 1 || t.Code == "" {
			continue
		}
		if _, ok := agg[t.Code]; !ok {
			agg[t.Code] = &codeAgg{constraintID: t.ConstraintID}
		}
		agg[t.Code].scores = append(agg[t.Code].scores, t.JudgeScore)
		if t.JudgeScore < threshold {
			agg[t.Code].interpretation = t.Interpretation
			agg[t.Code].intent = t.SenderIntent
		}
	}

	var weak []weakCode
	for code, a := range agg {
		avgScore := avg(a.scores)
		if avgScore < threshold {
			reason := fmt.Sprintf("T1 avg score %.1f/10 < %.0f threshold", avgScore, threshold)
			if a.interpretation != "" {
				reason += fmt.Sprintf("; receiver said: %q but sender meant: %q", a.interpretation, a.intent)
			}

			// Find the constraint to get content and type
			content := ""
			cType := ""
			for _, t := range trials {
				if t.ConstraintID == a.constraintID {
					content = t.SenderIntent
					break
				}
			}

			weak = append(weak, weakCode{
				constraintID: a.constraintID,
				code:         code,
				avgScore:     avgScore,
				reason:       reason,
				content:      content,
				cType:        cType,
			})
		}
	}
	return weak
}

// ── MDEMG server integration ──

func checkMDEMG(baseURL string) bool {
	resp, err := http.Get(baseURL + "/healthz")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

func postFeedback(baseURL string, trials []trialResult) any {
	type feedbackTrial struct {
		ConstraintCode string  `json:"constraint_code"`
		Tier           int     `json:"tier"`
		Score          float64 `json:"score"`
		Interpretation string  `json:"interpretation"`
		SenderIntent   string  `json:"sender_intent"`
	}

	var fbTrials []feedbackTrial
	for _, t := range trials {
		if t.Code == "" {
			continue
		}
		fbTrials = append(fbTrials, feedbackTrial{
			ConstraintCode: t.Code,
			Tier:           t.Tier,
			Score:          t.JudgeScore,
			Interpretation: t.Interpretation,
			SenderIntent:   t.SenderIntent,
		})
	}

	body, _ := json.Marshal(map[string]any{"trials": fbTrials})
	resp, err := http.Post(baseURL+"/v1/jiminy/protocol/feedback", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("  feedback POST failed: %v", err)
		return nil
	}
	defer resp.Body.Close()

	var result any
	data, _ := io.ReadAll(resp.Body)
	json.Unmarshal(data, &result)

	if resp.StatusCode == 200 {
		log.Printf("  feedback: ingested %d trials into MDEMG protocol metrics", len(fbTrials))
	} else {
		log.Printf("  feedback: server returned %d: %s", resp.StatusCode, string(data))
	}
	return result
}

func learnViaServer(baseURL string, wc weakCode) string {
	body, _ := json.Marshal(map[string]string{
		"constraint_type": wc.cType,
		"description":     wc.content,
		"old_code":        wc.code,
		"failure_reason":  wc.reason,
	})
	resp, err := http.Post(baseURL+"/v1/jiminy/protocol/learn", "application/json", bytes.NewReader(body))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ""
	}

	var result struct {
		Data struct {
			NewCode string `json:"new_code"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Data.NewCode
}

// ── Direct LLM learning (fallback when MDEMG unavailable) ──

func learnViaLLM(ctx context.Context, client *llmclient.Client, wc weakCode) string {
	prompt := fmt.Sprintf(`Generate an improved mnemonic kebab-case code (2-5 words) for this constraint.

Constraint: %s

The PREVIOUS code was: %s
It failed because: %s

Requirements:
- More descriptive and unambiguous than the old code
- Clearly convey the ACTION required or forbidden
- Must not be confused with other domain concepts
- 2-5 words, kebab-case, lowercase

Respond with ONLY the new kebab-case code, nothing else.`, wc.content, wc.code, wc.reason)

	resp, err := client.Complete(ctx, []llmclient.Message{
		{Role: "user", Content: prompt},
	}, llmclient.CompleteOpts{MaxTokens: 50})
	if err != nil {
		log.Printf("    LLM code regeneration failed: %v", err)
		return ""
	}

	code := strings.TrimSpace(resp)
	code = strings.Trim(code, "\"'`")
	code = strings.ToLower(code)
	code = strings.ReplaceAll(code, " ", "-")
	code = strings.ReplaceAll(code, "_", "-")

	// Only keep first line
	if idx := strings.IndexByte(code, '\n'); idx >= 0 {
		code = code[:idx]
	}

	return code
}

// ── LLM interaction ──

func interpretMessage(ctx context.Context, client *llmclient.Client, bootstrap, encoded string, tier int) string {
	tierName := map[int]string{1: "T1 (coded)", 2: "T2 (telegraphic)", 3: "T3 (full natural language)"}[tier]

	prompt := fmt.Sprintf(`You are an AI coding agent receiving guidance from the Jiminy inner-voice service via the J17 AI-to-AI protocol.

Here is the protocol specification you were given at session start:
%s

You just received the following %s protocol message:
%s

In 1-3 sentences, explain:
1. What constraint or guidance is being communicated?
2. What specific action should you take or avoid?

Be precise and concrete. Do not restate the protocol spec.`, bootstrap, tierName, encoded)

	resp, err := client.Complete(ctx, []llmclient.Message{
		{Role: "user", Content: prompt},
	}, llmclient.CompleteOpts{MaxTokens: 300})
	if err != nil {
		log.Printf("  LLM interpretation error: %v", err)
		return fmt.Sprintf("[ERROR: %v]", err)
	}
	return strings.TrimSpace(resp)
}

func judgeComprehension(ctx context.Context, client *llmclient.Client, senderIntent, receiverInterpretation string) (float64, string) {
	prompt := fmt.Sprintf(`You are judging whether an AI agent correctly understood a protocol message.

SENDER'S INTENT (ground truth):
%s

RECEIVER'S INTERPRETATION:
%s

Score the interpretation from 0 to 10:
- 10: Perfect understanding -- all key points captured, correct actions identified
- 7-9: Good understanding -- main point captured, minor details missed
- 4-6: Partial understanding -- some correct elements but missing critical aspects
- 1-3: Poor understanding -- fundamentally misunderstood the constraint
- 0: No understanding or completely wrong

Respond in exactly this format (nothing else):
SCORE: <number>
REASON: <one sentence explanation>`, senderIntent, receiverInterpretation)

	resp, err := client.Complete(ctx, []llmclient.Message{
		{Role: "user", Content: prompt},
	}, llmclient.CompleteOpts{MaxTokens: 100})
	if err != nil {
		log.Printf("  LLM judge error: %v", err)
		return 0, "judge error"
	}

	var score float64
	var reason string
	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "SCORE:"); ok {
			fmt.Sscanf(after, "%f", &score)
		}
		if after, ok := strings.CutPrefix(line, "REASON:"); ok {
			reason = strings.TrimSpace(after)
		}
	}
	return score, reason
}

// ── Helpers ──

func buildTestConstraints() []testConstraint {
	return []testConstraint{
		{
			ID:       "c1",
			Type:     "constraint",
			Severity: "must_not",
			Content:  "Never force-push to the main branch. This destroys shared history and can cause data loss for other collaborators.",
			Code:     "no-force-push-main",
			Intent:   "The developer must not use git push --force on the main branch because it rewrites shared history.",
			SourceNodes: []string{"node-abc123"},
			GlossaryDef: "Prohibit git push --force to main (history rewrite destroys collaborator work)",
		},
		{
			ID:       "c2",
			Type:     "constraint",
			Severity: "must",
			Content:  "Always run the full test suite before committing changes. No exceptions, even for 'trivial' changes.",
			Code:     "no-commit-without-all-tests",
			Intent:   "Every commit must be preceded by running all tests. There are no exceptions for any change size.",
			SourceNodes: []string{"node-def456"},
			Annotations: map[string]string{"scope": "all-tests-no-exceptions"},
			GlossaryDef: "Run ALL tests before every commit — no exceptions for small changes",
		},
		{
			ID:       "c3",
			Type:     "correction",
			Severity: "must",
			Content:  "The default embedding model is gpt-5-mini, NOT text-embedding-3-small. Do not change this model name.",
			Code:     "use-gpt-5-mini-only",
			Intent:   "A previous error was made about the embedding model name. The correct model is gpt-5-mini. Never use text-embedding-3-small.",
			SourceNodes: []string{"node-ghi789"},
			Annotations: map[string]string{"alt": "gpt-5-mini", "neg": "text-embedding-3-small"},
			GlossaryDef: "Embedding model must be gpt-5-mini (corrects prior use of text-embedding-3-small)",
		},
		{
			ID:       "c4",
			Type:     "constraint",
			Severity: "should",
			Content:  "Prefer editing existing files over creating new ones to prevent file bloat and build on existing work.",
			Code:     "edit-over-create",
			Intent:   "When modifying code, use existing files rather than creating new files. This reduces unnecessary file proliferation.",
			SourceNodes: []string{"node-jkl012"},
			GlossaryDef: "Modify existing files instead of creating new ones (prevent file bloat)",
		},
		{
			ID:       "c5",
			Type:     "decision",
			Severity: "info",
			Content:  "The auth middleware rewrite is driven by legal/compliance requirements around session token storage, not technical debt cleanup. Scope decisions should favor compliance over ergonomics.",
			Code:     "legal-scope-over-ergonomics",
			Intent:   "The auth middleware change is motivated by legal compliance, not tech debt. When making scope trade-offs, compliance wins over developer ergonomics.",
			SourceNodes: []string{"node-mno345"},
			Annotations: map[string]string{"ctx": "auth-middleware-rewrite", "scope": "compliance-over-convenience"},
			GlossaryDef: "Auth middleware rewrite is legal/compliance-driven (not tech debt) — favor compliance over ergonomics",
		},
		{
			ID:       "c6",
			Type:     "constraint",
			Severity: "must_not",
			Content:  "Never use git stash as a workaround for goreleaser dirty-state checks. Commit all pending changes before running goreleaser release.",
			Code:     "no-stash-goreleaser",
			Intent:   "Do not use git stash to bypass goreleaser's dirty-state detection. All changes must be committed first.",
			EscLevel: 1,
			SourceNodes: []string{"node-pqr678"},
			Annotations: map[string]string{"alt": "commit-changes-first", "ctx": "goreleaser-dirty-state"},
			GlossaryDef: "Do not git stash to bypass goreleaser dirty-state check — commit changes instead",
		},
		{
			ID:       "c7",
			Type:     "correction",
			Severity: "must",
			Content:  "Never commit directly to the main branch. All development work must happen on dev branches. Main is branch-protected and changes reach it only via pull request.",
			Code:     "no-direct-main-commit",
			Intent:   "Direct commits to main are forbidden. Use dev branches and PRs. The main branch has branch protection enabled.",
			EscLevel: 2,
			SourceNodes: []string{"node-stu901"},
			Annotations: map[string]string{"alt": "use-dev-branch-and-pr", "ctx": "branch-protection-enabled"},
			GlossaryDef: "Never commit directly to main — use dev branches + PRs (branch protection enforced)",
		},
		{
			ID:       "c8",
			Type:     "pattern",
			Severity: "should",
			Content:  "When working on MDEMG, use sub-agents for discrete tasks like file searches, code analysis, and test running. The orchestrator should coordinate, not execute every step directly.",
			Code:     "", // no T1 code — will use T2/T3
			Intent:   "Delegate specific tasks to sub-agents rather than doing everything in the main context. The main agent is an orchestrator.",
			SourceNodes: []string{"node-vwx234"},
		},
	}
}

func computeSummary(trials []trialResult) summary {
	var s summary
	s.TotalTrials = len(trials)

	var t1Scores, t2Scores, t3Scores []float64
	var t1Comp, t2Comp []float64

	for _, t := range trials {
		switch t.Tier {
		case 1:
			t1Scores = append(t1Scores, t.JudgeScore)
			if t.TokensEncoded > 0 {
				t1Comp = append(t1Comp, float64(t.TokensOriginal)/float64(t.TokensEncoded))
			}
		case 2:
			t2Scores = append(t2Scores, t.JudgeScore)
			if t.TokensEncoded > 0 {
				t2Comp = append(t2Comp, float64(t.TokensOriginal)/float64(t.TokensEncoded))
			}
		case 3:
			t3Scores = append(t3Scores, t.JudgeScore)
		}
		if t.JudgeScore >= 10 {
			s.PerfectScoreCount++
		}
	}

	s.AvgScoreT1 = avg(t1Scores)
	s.AvgScoreT2 = avg(t2Scores)
	s.AvgScoreT3 = avg(t3Scores)
	s.AvgCompressionT1 = avg(t1Comp)
	s.AvgCompressionT2 = avg(t2Comp)
	return s
}

func avg(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func estimateTokens(s string) int {
	return max(len(s)/4, 1)
}

func buildGlossary(constraints []testConstraint) map[string]string {
	glossary := make(map[string]string)
	for _, c := range constraints {
		if c.Code != "" && c.GlossaryDef != "" {
			glossary[c.Code] = c.GlossaryDef
		}
	}
	return glossary
}

func countCoded(constraints []testConstraint) int {
	n := 0
	for _, c := range constraints {
		if c.Code != "" {
			n++
		}
	}
	return n
}

func mapType(t string) jiminy.GuidanceType {
	switch t {
	case "constraint":
		return jiminy.GuidanceConstraint
	case "correction":
		return jiminy.GuidanceCorrection
	case "pattern":
		return jiminy.GuidancePattern
	case "decision":
		return jiminy.GuidanceFrontier
	default:
		return jiminy.GuidanceConstraint
	}
}

func mapSeverity(s string) string {
	switch s {
	case "must_not", "must":
		return "high"
	case "should", "should_not":
		return "medium"
	default:
		return "low"
	}
}

func escLevelToConst(level int) jiminy.EscalationLevel {
	switch level {
	case 1:
		return jiminy.EscalationWarned
	case 2:
		return jiminy.EscalationEscalated
	case 3:
		return jiminy.EscalationBlocked
	default:
		return jiminy.EscalationInactive
	}
}

// ── Report generation ──

func writeLearningReport(path string, expLog experimentLog) {
	var sb strings.Builder
	sb.WriteString("# J17 Protocol Comprehension Learning Loop\n\n")
	fmt.Fprintf(&sb, "**Date**: %s  \n", expLog.Timestamp)
	fmt.Fprintf(&sb, "**Model**: %s  \n", expLog.Model)
	fmt.Fprintf(&sb, "**Iterations**: %d  \n\n", len(expLog.Iterations))

	// Iteration comparison table
	sb.WriteString("## Learning Progress\n\n")
	sb.WriteString("| Iteration | T1 Avg | T2 Avg | T3 Avg | Perfect | Code Changes |\n")
	sb.WriteString("|-----------|--------|--------|--------|---------|-------------|\n")
	for _, iter := range expLog.Iterations {
		s := iter.Summary
		fmt.Fprintf(&sb, "| %d | %.1f | %.1f | %.1f | %d/%d | %d |\n",
			iter.Iteration, s.AvgScoreT1, s.AvgScoreT2, s.AvgScoreT3,
			s.PerfectScoreCount, s.TotalTrials, len(iter.CodeChanges))
	}
	sb.WriteString("\n")

	// Code evolution
	hasChanges := false
	for _, iter := range expLog.Iterations {
		if len(iter.CodeChanges) > 0 {
			hasChanges = true
			break
		}
	}
	if hasChanges {
		sb.WriteString("## Code Evolution\n\n")
		sb.WriteString("| Constraint | Iteration | Old Code | New Code | Score Before |\n")
		sb.WriteString("|------------|-----------|----------|----------|-------------|\n")
		for _, iter := range expLog.Iterations {
			for _, cc := range iter.CodeChanges {
				fmt.Fprintf(&sb, "| %s | %d | `%s` | `%s` | %.1f |\n",
					cc.ConstraintID, iter.Iteration, cc.OldCode, cc.NewCode, cc.ScoreBefore)
			}
		}
		sb.WriteString("\n")
	}

	// Bootstrap header
	sb.WriteString("## Bootstrap Header\n\n```\n")
	sb.WriteString(expLog.Bootstrap)
	sb.WriteString("\n```\n\n")

	// Per-iteration details
	for _, iter := range expLog.Iterations {
		fmt.Fprintf(&sb, "## Iteration %d Details\n\n", iter.Iteration)
		for _, t := range iter.Trials {
			if t.Tier != 1 {
				continue // only show T1 in detail (T2/T3 are reliable)
			}
			label := "PASS"
			if t.JudgeScore < 9 {
				label = "WEAK"
			}
			fmt.Fprintf(&sb, "### %s | T1 | Score: %.0f/10 %s\n\n", t.ConstraintID, t.JudgeScore, label)
			fmt.Fprintf(&sb, "**Code**: `%s`  \n", t.Code)
			fmt.Fprintf(&sb, "**Encoded**: `%s`  \n", t.EncodedMessage)
			fmt.Fprintf(&sb, "**Interpretation**: %s  \n", t.Interpretation)
			fmt.Fprintf(&sb, "**Judge**: %s  \n\n", t.JudgeReason)
		}
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		log.Printf("Warning: could not write report: %v", err)
	}
}
