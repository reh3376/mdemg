package cli

// HITL-CURATION-002 E1 — `mdemg review autograde` CLI.
//
// Reads pending candidates from a running mdemg server (POST-restart discovery
// of the port via the same pattern the other CLI commands use), grades each
// via a local LLM through the LLMGrader interface, and POSTs the confident
// verdicts to /v1/review/grade with `reinforce:false`.
//
// The load-bearing invariant lives at TWO layers:
//   1. This CLI ALWAYS sends `reinforce:false` (grep-testable below)
//   2. Every autograde-authored row's `grader_id` starts with review.AutoGraderPrefix
//      ("auto:") — the dashboard + curation-stall alert split on this
//
// Neither layer can be bypassed without editing this file; both are tested.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"mdemg/internal/llmclient"
	"mdemg/internal/review"
)

func newReviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review",
		Short: "HITL review datasets — auto-grade + cadence surface",
		Long: `HITL review utilities. Reads a running mdemg server via HTTP.

Subcommands:
  autograde   LLM-grade pending items in a dataset; write confident verdicts
              as auto-grade rows (never reinforces the substrate — operator
              curation is preserved as the only reinforcement path).
`,
	}
	cmd.AddCommand(newReviewAutogradeCmd())
	cmd.AddCommand(newReviewCadenceCmd())
	return cmd
}

// HITL-CURATION-002 E3: `mdemg review cadence` produces a compact operator
// prompt of what's waiting in the HITL queue. Designed to be run periodically
// (via cron / launchd / manual) — the output is a self-contained "what needs
// your attention this week" digest. Reads /v1/review/datasets for pending
// counts across every registered dataset; JSON mode is machine-readable for
// dashboards/alert bodies.
func newReviewCadenceCmd() *cobra.Command {
	var endpoint, outFormat string
	cmd := &cobra.Command{
		Use:   "cadence",
		Short: "Produce a weekly HITL cadence summary — what's waiting for the operator",
		Long: `Reads the running mdemg server's review datasets and renders a compact
summary of what needs the operator's attention. Text format is human-readable;
JSON is machine-readable for scheduler hooks / alert bodies.

  mdemg review cadence                        # text summary to stdout
  mdemg review cadence --out-format json      # JSON
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if endpoint == "" {
				endpoint = resolveEndpoint()
			}
			return runReviewCadence(cmd.Context(), endpoint, outFormat)
		},
	}
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "mdemg server (default: from LISTEN_ADDR in .env)")
	cmd.Flags().StringVar(&outFormat, "out-format", "text", "Output format: text or json")
	return cmd
}

type cadenceDatasetRow struct {
	DatasetID      string `json:"dataset_id"`
	DisplayName    string `json:"display_name"`
	PendingCount   int    `json:"pending_count"`
	RubricVersion  string `json:"rubric_version"`
}

type cadenceSummary struct {
	GeneratedAt   string              `json:"generated_at"`
	TotalPending  int                 `json:"total_pending"`
	Datasets      []cadenceDatasetRow `json:"datasets"`
	Actionable    bool                `json:"actionable"` // true if any pending; false if all-clear
}

func runReviewCadence(ctx context.Context, endpoint, outFormat string) error {
	url := strings.TrimSuffix(endpoint, "/") + "/v1/review/datasets"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("cadence: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("cadence: HTTP %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		Data struct {
			Datasets []struct {
				DatasetID      string `json:"id"` // endpoint returns `id`, not `dataset_id`
				DisplayName    string `json:"display_name"`
				RubricVersion  string `json:"rubric_version"`
				CandidateCount int    `json:"candidate_count"`
			} `json:"datasets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("cadence: unmarshal: %w", err)
	}
	summary := cadenceSummary{GeneratedAt: time.Now().UTC().Format(time.RFC3339)}
	for _, d := range out.Data.Datasets {
		if d.CandidateCount <= 0 {
			continue
		}
		summary.Datasets = append(summary.Datasets, cadenceDatasetRow{
			DatasetID: d.DatasetID, DisplayName: d.DisplayName,
			PendingCount: d.CandidateCount, RubricVersion: d.RubricVersion,
		})
		summary.TotalPending += d.CandidateCount
	}
	summary.Actionable = summary.TotalPending > 0
	return renderCadence(summary, outFormat)
}

func renderCadence(s cadenceSummary, outFormat string) error {
	if outFormat == "json" {
		b, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	// Text format
	fmt.Println("MDEMG HITL Cadence Summary")
	fmt.Println("==========================")
	fmt.Printf("Generated: %s\n\n", s.GeneratedAt)
	if !s.Actionable {
		fmt.Println("✓ HITL queue is empty — no operator action required this cycle.")
		return nil
	}
	fmt.Printf("%d item(s) pending across %d dataset(s):\n\n", s.TotalPending, len(s.Datasets))
	for _, d := range s.Datasets {
		fmt.Printf("  • %-45s  %4d pending  (rubric %s)\n", d.DisplayName, d.PendingCount, d.RubricVersion)
		fmt.Printf("    dataset_id: %s\n", d.DatasetID)
	}
	fmt.Println()
	fmt.Println("Review at: http://localhost:9999/ui/#review")
	fmt.Println()
	fmt.Println("Automation options:")
	fmt.Println("  - mdemg review autograde --dataset <id> --space-id <space> --dry-run")
	fmt.Println("    → LLM-graded confidence pass; auto-grade rows for confident verdicts,")
	fmt.Println("      operator handles the low-confidence remainder.")
	return nil
}

func newReviewAutogradeCmd() *cobra.Command {
	var datasetID, spaceID string
	var minConfidence float64
	var limit int
	var dryRun bool
	var force bool
	var endpoint string
	var sampleStrategy string
	cmd := &cobra.Command{
		Use:   "autograde",
		Short: "LLM-grade pending items in a dataset; never reinforces the substrate",
		Long: `Fetches pending candidates from /v1/review/candidates, grades each via a local
LLM against the dataset's rubric, and POSTs high-confidence verdicts to
/v1/review/grade with reinforce:false. Items below the confidence threshold
are left pending for the operator.

Invariant: auto-grade NEVER triggers the reinforcement side-effect on the
substrate. Only operator-confirmed grades do that.

  mdemg review autograde --dataset contradicted_drafts --space-id mdemg-dev
  mdemg review autograde --dataset contradicted_drafts --space-id mdemg-dev --dry-run
  mdemg review autograde --dataset contradicted_drafts --min-confidence 0.90 --limit 20
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if datasetID == "" {
				return fmt.Errorf("--dataset is required")
			}
			if spaceID == "" {
				spaceID = resolveSpaceID(cmd)
			}
			if spaceID == "" {
				return fmt.Errorf("--space-id is required (no default resolvable)")
			}
			if endpoint == "" {
				endpoint = resolveEndpoint()
			}
			return runReviewAutograde(cmd.Context(), autogradeOpts{
				endpoint:       endpoint,
				datasetID:      datasetID,
				spaceID:        spaceID,
				minConfidence:  minConfidence,
				limit:          limit,
				dryRun:         dryRun,
				force:          force,
				sampleStrategy: sampleStrategy,
			})
		},
	}
	cmd.Flags().StringVar(&datasetID, "dataset", "", "Review dataset id (e.g. contradicted_drafts)")
	cmd.Flags().StringVar(&spaceID, "space-id", "", "Space to grade (required if not resolvable)")
	cmd.Flags().Float64Var(&minConfidence, "min-confidence", 0.80, "Auto-grade only when the model's confidence is >= this (0-1)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max pending items to fetch (1-500)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview mode — grade + display but don't POST")
	cmd.Flags().BoolVar(&force, "force", false, "Re-grade items that already have a grade at the current rubric_version (needed to backfill after sink logic changes, e.g. HITL-AUTO-DISMISS-001)")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "mdemg server (default: from LISTEN_ADDR in .env)")
	cmd.Flags().StringVar(&sampleStrategy, "sample-strategy", "", "Candidate ordering hint: 'newest' (default) or 'oldest-ungraded' (starvation-free backfill for scheduled runs — HITL-CURATION-003)")
	return cmd
}

type autogradeOpts struct {
	endpoint       string
	datasetID      string
	spaceID        string
	minConfidence  float64
	limit          int
	dryRun         bool
	force          bool
	sampleStrategy string
}

func runReviewAutograde(ctx context.Context, opt autogradeOpts) error {
	fmt.Println("MDEMG Review Autograde")
	fmt.Println("======================")
	fmt.Printf("Dataset:        %s\n", opt.datasetID)
	fmt.Printf("Space:          %s\n", opt.spaceID)
	fmt.Printf("Endpoint:       %s\n", opt.endpoint)
	fmt.Printf("Min confidence: %.2f\n", opt.minConfidence)
	fmt.Printf("Limit:          %d\n", opt.limit)
	sampleStrategyDisplay := opt.sampleStrategy
	if sampleStrategyDisplay == "" {
		sampleStrategyDisplay = "newest (default)"
	}
	fmt.Printf("Sample:         %s\n", sampleStrategyDisplay)
	if opt.dryRun {
		fmt.Println("Mode:           DRY RUN (no grades will be written)")
	} else {
		fmt.Println("Mode:           LIVE (writes review_grades rows with grader_id='auto:...')")
		fmt.Println("                Reinforcement: NEVER (invariant — operator-only)")
	}
	fmt.Println()

	// 1. Fetch candidates + rubric + dataset hint via /v1/review/candidates
	cands, rubric, hint, err := fetchCandidates(ctx, opt.endpoint, opt.datasetID, opt.spaceID, opt.limit, opt.sampleStrategy)
	if err != nil {
		return fmt.Errorf("fetch candidates: %w", err)
	}
	fmt.Printf("Fetched %d pending items; rubric %s (%s, %d dims)\n",
		len(cands), rubric.Version, rubric.Kind, len(rubric.Dimensions))
	if hint != "" {
		fmt.Printf("Dataset-hint: %d chars (spliced into system prompt)\n", len(hint))
	}
	if len(cands) == 0 {
		fmt.Println("\nNothing to grade — queue is empty.")
		return nil
	}

	// 2. Build the autograder against the local LLM.
	ag, agLLMModel := buildAutograder(opt.minConfidence)
	if ag == nil {
		return fmt.Errorf("autograder init failed (llm client unavailable)")
	}
	fmt.Printf("Autograder:     %s\n\n", ag.GraderID())

	// 3. Iterate + grade + persist.
	var confident, borderline, failed int
	for i, item := range cands {
		fmt.Printf("[%d/%d] %s  (content: %.80s...)\n", i+1, len(cands), item.ItemID, item.Content)
		res, ok, err := ag.GradeWithHint(ctx, opt.datasetID, item, rubric, hint)
		if err != nil {
			failed++
			fmt.Printf("        ERR  %v\n", err)
			continue
		}
		if !ok {
			borderline++
			fmt.Printf("        LOW  conf=%.2f (below %.2f) — left pending\n", res.Confidence, opt.minConfidence)
			continue
		}
		confident++
		fmt.Printf("        AUTO conf=%.2f dims=%v rationale=%q\n",
			res.Confidence, res.Submission.Dimensions, res.Rationale)
		if opt.dryRun {
			continue
		}
		if err := postAutoGrade(ctx, opt.endpoint, ag.GraderID(), opt.spaceID, res, opt.force); err != nil {
			failed++
			fmt.Printf("        POST-ERR  %v\n", err)
			continue
		}
	}

	fmt.Println()
	fmt.Printf("Summary: %d confident-auto | %d borderline (pending) | %d errors\n",
		confident, borderline, failed)
	fmt.Printf("Model used: %s\n", agLLMModel)
	if opt.dryRun {
		fmt.Println("(dry-run — no writes)")
	}
	return nil
}

// buildAutograder constructs an Autograder wired to the local llm endpoint.
// Reads LLM_ENDPOINT + LLM_MODEL from env directly (mirrors config.FromEnv
// defaults for the local-first case) — avoids the full config load that would
// demand Neo4j creds this CLI doesn't need.
// Returns (nil, "") when the LLM is unavailable.
func buildAutograder(minConfidence float64) (*review.Autograder, string) {
	baseURL := os.Getenv("LLM_ENDPOINT")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8102/v1"
	}
	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "mdemg-llm-v1"
	}
	timeoutMs := 60000
	llm := llmclient.New(llmclient.Config{
		Provider:  "openai", // openai-compat protocol; local llama-server
		Model:     model,
		BaseURL:   baseURL,
		TimeoutMs: timeoutMs,
	}).WithContext("review.autograde", "")
	adapter := &llmGraderAdapter{c: llm, model: model}
	ag := review.NewAutograder(review.AutograderConfig{
		LLM:           adapter,
		ModelID:       model,
		BinarySHA:     shortBinarySHA(),
		MinConfidence: minConfidence,
	})
	return ag, model
}

// llmGraderAdapter satisfies review.LLMGrader against llmclient.Client. Uses
// Complete (not CompleteWithUsage) — we don't need token usage here.
type llmGraderAdapter struct {
	c     *llmclient.Client
	model string
}

func (a *llmGraderAdapter) CompleteJSON(ctx context.Context, sys, usr string, maxTokens int) (string, error) {
	msgs := []llmclient.Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: usr},
	}
	temperature := 0.0
	return a.c.Complete(ctx, msgs, llmclient.CompleteOpts{
		MaxTokens:   maxTokens,
		Temperature: &temperature,
	})
}

// shortBinarySHA returns a short identifier for the current mdemg binary, for
// embedding in grader_id. Falls back to "dev" when unavailable.
func shortBinarySHA() string {
	// Prefer an env-injected value (a release build can set MDEMG_BUILD_SHA).
	if v := os.Getenv("MDEMG_BUILD_SHA"); v != "" {
		if len(v) > 7 {
			return v[:7]
		}
		return v
	}
	return "dev"
}

// fetchCandidates GETs /v1/review/candidates. Returns ([]ReviewItem, Rubric,
// autograde_prompt_hint, error). The hint is empty for datasets that don't
// implement AutogradePromptHinter — the autograder falls through to the
// generic prompt in that case.
func fetchCandidates(ctx context.Context, endpoint, datasetID, spaceID string, limit int, sampleStrategy string) ([]review.ReviewItem, review.Rubric, string, error) {
	url := fmt.Sprintf("%s/v1/review/candidates?dataset_id=%s&space_id=%s&limit=%d",
		strings.TrimSuffix(endpoint, "/"), datasetID, spaceID, limit)
	if sampleStrategy != "" {
		url += "&sample_strategy=" + sampleStrategy
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, review.Rubric{}, "", err
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, review.Rubric{}, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, review.Rubric{}, "", fmt.Errorf("candidates: HTTP %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		Data struct {
			Items  []review.ReviewItem `json:"items"`
			Rubric review.Rubric       `json:"rubric"`
			Hint   string              `json:"autograde_prompt_hint"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, review.Rubric{}, "", fmt.Errorf("unmarshal candidates: %w (body=%.200s)", err, string(body))
	}
	return out.Data.Items, out.Data.Rubric, out.Data.Hint, nil
}

// postAutoGrade POSTs the auto-grade to /v1/review/grade. The invariant:
// reinforce is ALWAYS false — never mutate the substrate from an auto-grade.
func postAutoGrade(ctx context.Context, endpoint, graderID, spaceID string, res review.GradeResult, force bool) error {
	reinforceFalse := false
	body := map[string]any{
		"dataset_id": res.Submission.DatasetID,
		"item_id":    res.Submission.ItemID,
		"space_id":   spaceID,
		"grader_id":  graderID,
		"dimensions": res.Submission.Dimensions,
		"reinforce":  &reinforceFalse,
	}
	// HITL-AUTO-DISMISS-001: --force lets the autograder RE-issue a grade at the
	// current rubric_version, needed to backfill after sink-logic changes so a
	// previously-graded item can drive the newly-non-reinforcing sink path.
	// Idempotency guard on the server side stays intact for operator grades.
	if force {
		body["force"] = true
	}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := strings.TrimSuffix(endpoint, "/") + "/v1/review/grade"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusConflict {
		// Item was already graded (idempotency 409). Not an error for autograde —
		// the operator's grade is preferred; move on.
		return nil
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("grade POST: HTTP %d: %s", resp.StatusCode, string(rb))
	}
	return nil
}

