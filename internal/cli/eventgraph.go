package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// newEventgraphCmd is the parent for EVENTGRAPH federation consumers (Pattern Y1).
// Subcommands per event class: reinforcement-neighborhood (Hebbian) +
// guidance-outcome-neighborhood (constraint effectiveness).
func newEventgraphCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eventgraph",
		Short: "Query the event-graph federation (Pattern Y1)",
		Long: `Consume the EVENTGRAPH federation API: a graph-walk from a seed node
combined with the time-series events touching that neighborhood. The consumer
for the federation API + the live-testing harness for the EVENTGRAPH line.`,
	}
	cmd.AddCommand(newEventgraphReinforcementNeighborhoodCmd())
	cmd.AddCommand(newEventgraphGuidanceOutcomeNeighborhoodCmd())
	return cmd
}

// fedEvent / fedResult mirror internal/eventgraph.{EventWithContext,FederationResult}
// (the documented response contract of POST /v1/eventgraph/reinforcement-neighborhood).
type fedEvent struct {
	EventID            string    `json:"event_id"`
	RecordedAt         time.Time `json:"recorded_at"`
	SrcNodeID          string    `json:"src_node_id"`
	DstNodeID          string    `json:"dst_node_id"`
	PrevWeight         float64   `json:"prev_weight"`
	NewWeight          float64   `json:"new_weight"`
	DeltaWeight        float64   `json:"delta_weight"`
	EvidenceCountAfter int       `json:"evidence_count_after"`
	Direction          string    `json:"direction"`
	SessionID          string    `json:"session_id"`
	CreatedNewEdge     bool      `json:"created_new_edge"`
	TriggerPath        string    `json:"trigger_path"`
	SrcInNeighborhood  bool      `json:"src_in_neighborhood"`
	DstInNeighborhood  bool      `json:"dst_in_neighborhood"`
}

type fedResult struct {
	Events          []fedEvent `json:"events"`
	NeighborNodeIDs []string   `json:"neighbor_node_ids"`
	GraphHops       int        `json:"graph_hops"`
	TSDBRowsScanned int        `json:"tsdb_rows_scanned"`
	Truncated       bool       `json:"truncated"`
}

func newEventgraphReinforcementNeighborhoodCmd() *cobra.Command {
	var (
		seed       string
		query      string
		hops       int
		since      string
		limit      int
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "reinforcement-neighborhood",
		Short: "Reinforcement (Hebbian co-activation) events in a node's graph neighborhood",
		Long: `Walk the graph from a seed node (or a --query-resolved seed) and return the
reinforcement_events (Hebbian co-activation weight updates) touching that
neighborhood, annotated with which endpoints fell inside the N-hop walk.

Examples:
  mdemg eventgraph reinforcement-neighborhood --seed n_abc123 --hops 2 --since 24h
  mdemg eventgraph reinforcement-neighborhood --query "circuit breaker state machine"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			spaceID := resolveSpaceID(cmd)
			if spaceID == "" {
				spaceID = "mdemg-dev"
			}
			return runReinforcementNeighborhood(reinforcementNeighborhoodOpts{
				spaceID: spaceID, seed: seed, query: query,
				hops: hops, since: since, limit: limit, jsonOutput: jsonOutput,
			})
		},
	}
	cmd.Flags().StringVar(&seed, "seed", "", "Seed node_id to walk from (required unless --query)")
	cmd.Flags().StringVar(&query, "query", "", "Resolve the seed from the top retrieval result for this query text")
	cmd.Flags().IntVar(&hops, "hops", -1, "Graph traversal depth (default: server config)")
	cmd.Flags().StringVar(&since, "since", "", "Lookback window, e.g. 24h, 90m (default: server config)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max events returned (default: server config)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output raw JSON")
	cmd.Flags().String("space-id", "", "Space ID (default: mdemg-dev)")
	return cmd
}

type reinforcementNeighborhoodOpts struct {
	spaceID    string
	seed       string
	query      string
	hops       int
	since      string
	limit      int
	jsonOutput bool
}

func runReinforcementNeighborhood(o reinforcementNeighborhoodOpts) error {
	endpoint := resolveEndpoint()

	// Resolve the seed: explicit --seed wins; otherwise --query → top retrieval node.
	seed := o.seed
	if seed == "" {
		if o.query == "" {
			return fmt.Errorf("a seed is required: pass --seed <node_id> or --query <text>")
		}
		resolved, err := resolveSeedFromQuery(endpoint, o.spaceID, o.query)
		if err != nil {
			return err
		}
		seed = resolved
		if !o.jsonOutput {
			fmt.Printf("resolved seed from query %q → %s\n", o.query, seed)
		}
	}

	// Build the request — omit unset fields so the server applies its config
	// defaults (no re-hardcoding of hops/since/limit in the CLI).
	body := map[string]any{"space_id": o.spaceID, "seed_node_id": seed}
	if o.hops >= 0 {
		body["hops"] = o.hops
	}
	if o.since != "" {
		d, err := time.ParseDuration(o.since)
		if err != nil {
			return fmt.Errorf("invalid --since %q (use e.g. 24h, 90m): %w", o.since, err)
		}
		body["since_seconds"] = int64(d.Seconds())
	}
	if o.limit > 0 {
		body["limit"] = o.limit
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(endpoint+"/v1/eventgraph/reinforcement-neighborhood", "application/json", bytes.NewReader(raw)) //nolint:gosec // G107: localhost API call
	if err != nil {
		return fmt.Errorf("eventgraph API unreachable at %s (is the server running?): %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody struct {
			Error  string `json:"error"`
			Reason string `json:"reason"`
			Detail string `json:"detail"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		msg := errBody.Error
		if errBody.Reason != "" {
			msg += " (" + errBody.Reason + ")"
		}
		if errBody.Detail != "" {
			msg += ": " + errBody.Detail
		}
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("eventgraph API returned %d: %s", resp.StatusCode, msg)
	}

	if o.jsonOutput {
		// Stream the raw body through a re-indent so output matches the API.
		var v any
		if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}

	var result fedResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	printFedResult(result, seed)
	return nil
}

// resolveSeedFromQuery returns the top retrieval result's node_id for a query.
func resolveSeedFromQuery(endpoint, spaceID, query string) (string, error) {
	raw, _ := json.Marshal(map[string]any{
		"space_id": spaceID, "query_text": query, "top_k": 1, "candidate_k": 20,
	})
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(endpoint+"/v1/memory/retrieve", "application/json", bytes.NewReader(raw)) //nolint:gosec // G107: localhost API call
	if err != nil {
		return "", fmt.Errorf("retrieve API unreachable at %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("retrieve API returned %d while resolving --query seed", resp.StatusCode)
	}
	var r struct {
		Results []struct {
			NodeID string `json:"node_id"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("decode retrieve response: %w", err)
	}
	if len(r.Results) == 0 {
		return "", fmt.Errorf("no retrieval results for query %q — cannot resolve a seed node (try --seed)", query)
	}
	return r.Results[0].NodeID, nil
}

func printFedResult(r fedResult, seed string) {
	fmt.Printf("seed:         %s\n", seed)
	fmt.Printf("neighborhood: %d nodes · hops: %d · events: %d · scanned: %d · truncated: %v\n",
		len(r.NeighborNodeIDs), r.GraphHops, len(r.Events), r.TSDBRowsScanned, r.Truncated)
	if len(r.Events) == 0 {
		fmt.Println("\nNo reinforcement events in this neighborhood/window.")
		fmt.Println("(The graph walk succeeded — there were just no co-activation events touching it. Try a larger --hops or --since, or a seed in a more active region.)")
		return
	}
	fmt.Printf("\n%-14s %-14s %9s %8s %-13s %3s %4s  %s\n",
		"SRC", "DST", "Δweight", "new_w", "direction", "new", "nbhd", "recorded")
	for _, e := range r.Events {
		nbhd := boolMark(e.SrcInNeighborhood) + boolMark(e.DstInNeighborhood)
		fmt.Printf("%-14s %-14s %+9.4f %8.4f %-13s %3s %4s  %s\n",
			shortNodeID(e.SrcNodeID), shortNodeID(e.DstNodeID), e.DeltaWeight, e.NewWeight,
			e.Direction, boolMark(e.CreatedNewEdge), nbhd, e.RecordedAt.Local().Format("01-02 15:04:05"))
	}
}

func shortNodeID(id string) string {
	if len(id) > 14 {
		return id[:14]
	}
	return id
}

func boolMark(b bool) string {
	if b {
		return "✓"
	}
	return "·"
}

// ── guidance-outcome-neighborhood (EVENTGRAPH-002) ──────────────────────────

// fedGuidanceOutcome / fedGuidanceResult mirror
// internal/eventgraph.{GuidanceOutcomeWithContext,GuidanceOutcomeResult}.
type fedGuidanceOutcome struct {
	Time             time.Time `json:"time"`
	ConstraintID     string    `json:"constraint_id"`
	ConstraintCode   string    `json:"constraint_code"`
	GuidanceID       string    `json:"guidance_id"`
	SessionID        string    `json:"session_id"`
	OutcomeType      string    `json:"outcome_type"`
	Similarity       float64   `json:"similarity"`
	GuidanceType     string    `json:"guidance_type"`
	ConstraintNodeID string    `json:"constraint_node_id"`
	InNeighborhood   bool      `json:"in_neighborhood"`
}

type fedGuidanceResult struct {
	Outcomes                []fedGuidanceOutcome `json:"outcomes"`
	NeighborNodeIDs         []string             `json:"neighbor_node_ids"`
	NeighborConstraintCodes []string             `json:"neighbor_constraint_codes"`
	GraphHops               int                  `json:"graph_hops"`
	TSDBRowsScanned         int                  `json:"tsdb_rows_scanned"`
	Truncated               bool                 `json:"truncated"`
}

func newEventgraphGuidanceOutcomeNeighborhoodCmd() *cobra.Command {
	var (
		seed       string
		query      string
		hops       int
		since      string
		limit      int
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "guidance-outcome-neighborhood",
		Short: "Guidance outcomes (constraint effectiveness) in a constraint's graph neighborhood",
		Long: `Walk the graph from a seed constraint node (or a --query-resolved seed) and
return the guidance outcomes (followed/ignored/contradicted feedback from
constraint_outcomes) for every constraint_code present in that neighborhood —
i.e. how well the seed constraint AND its graph-related constraints are being
followed.

Seeding by an explicit constraint code (--constraint-code) is a planned
follow-up; for now seed by the constraint node_id (--seed) or by query text
(--query). Outcomes carry no constraint_code (unmatched at record time) are not
joinable and won't appear.

Examples:
  mdemg eventgraph guidance-outcome-neighborhood --seed myya3xf8kpk3wpbo0qonah99 --hops 1 --since 720h
  mdemg eventgraph guidance-outcome-neighborhood --query "never commit directly to main"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			spaceID := resolveSpaceID(cmd)
			if spaceID == "" {
				spaceID = "mdemg-dev"
			}
			return runGuidanceOutcomeNeighborhood(guidanceOutcomeNeighborhoodOpts{
				spaceID: spaceID, seed: seed, query: query,
				hops: hops, since: since, limit: limit, jsonOutput: jsonOutput,
			})
		},
	}
	cmd.Flags().StringVar(&seed, "seed", "", "Seed constraint node_id to walk from (required unless --query)")
	cmd.Flags().StringVar(&query, "query", "", "Resolve the seed from the top retrieval result for this query text")
	cmd.Flags().IntVar(&hops, "hops", -1, "Graph traversal depth (default: server config)")
	cmd.Flags().StringVar(&since, "since", "", "Lookback window, e.g. 24h, 90m (default: server config)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max outcomes returned (default: server config)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output raw JSON")
	cmd.Flags().String("space-id", "", "Space ID (default: mdemg-dev)")
	return cmd
}

type guidanceOutcomeNeighborhoodOpts struct {
	spaceID    string
	seed       string
	query      string
	hops       int
	since      string
	limit      int
	jsonOutput bool
}

func runGuidanceOutcomeNeighborhood(o guidanceOutcomeNeighborhoodOpts) error {
	endpoint := resolveEndpoint()

	seed := o.seed
	if seed == "" {
		if o.query == "" {
			return fmt.Errorf("a seed is required: pass --seed <node_id> or --query <text>")
		}
		resolved, err := resolveSeedFromQuery(endpoint, o.spaceID, o.query)
		if err != nil {
			return err
		}
		seed = resolved
		if !o.jsonOutput {
			fmt.Printf("resolved seed from query %q → %s\n", o.query, seed)
		}
	}

	// Omit unset fields so the server applies its config defaults (no CLI-side
	// default copies — single source of truth).
	body := map[string]any{"space_id": o.spaceID, "seed_node_id": seed}
	if o.hops >= 0 {
		body["hops"] = o.hops
	}
	if o.since != "" {
		d, err := time.ParseDuration(o.since)
		if err != nil {
			return fmt.Errorf("invalid --since %q (use e.g. 24h, 90m): %w", o.since, err)
		}
		body["since_seconds"] = int64(d.Seconds())
	}
	if o.limit > 0 {
		body["limit"] = o.limit
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(endpoint+"/v1/eventgraph/guidance-outcome-neighborhood", "application/json", bytes.NewReader(raw)) //nolint:gosec // G107: localhost API call
	if err != nil {
		return fmt.Errorf("eventgraph API unreachable at %s (is the server running?): %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody struct {
			Error  string `json:"error"`
			Reason string `json:"reason"`
			Detail string `json:"detail"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		msg := errBody.Error
		if errBody.Reason != "" {
			msg += " (" + errBody.Reason + ")"
		}
		if errBody.Detail != "" {
			msg += ": " + errBody.Detail
		}
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("eventgraph API returned %d: %s", resp.StatusCode, msg)
	}

	if o.jsonOutput {
		var v any
		if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}

	var result fedGuidanceResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	printFedGuidanceResult(result, seed)
	return nil
}

func printFedGuidanceResult(r fedGuidanceResult, seed string) {
	followed, ignored, other := 0, 0, 0
	for _, o := range r.Outcomes {
		switch o.OutcomeType {
		case "followed":
			followed++
		case "ignored":
			ignored++
		default:
			other++
		}
	}
	fmt.Printf("seed:         %s\n", seed)
	fmt.Printf("neighborhood: %d nodes · %d constraint codes · hops: %d\n",
		len(r.NeighborNodeIDs), len(r.NeighborConstraintCodes), r.GraphHops)
	fmt.Printf("outcomes:     %d (followed: %d · ignored: %d · other: %d) · scanned: %d · truncated: %v\n",
		len(r.Outcomes), followed, ignored, other, r.TSDBRowsScanned, r.Truncated)
	if len(r.Outcomes) == 0 {
		fmt.Println("\nNo guidance outcomes for this neighborhood/window.")
		fmt.Println("(The graph walk succeeded — there were just no coded outcomes for the constraint codes in this neighborhood. Try a larger --hops or --since, or a seed nearer a constraint with feedback.)")
		return
	}
	fmt.Printf("\n%-32s %-12s %5s %-10s %-22s  %s\n",
		"CONSTRAINT_CODE", "OUTCOME", "sim", "g_type", "guidance_id", "recorded")
	for _, o := range r.Outcomes {
		fmt.Printf("%-32s %-12s %5.2f %-10s %-22s  %s\n",
			truncStr(o.ConstraintCode, 32), o.OutcomeType, o.Similarity,
			truncStr(o.GuidanceType, 10), truncStr(o.GuidanceID, 22),
			o.Time.Local().Format("01-02 15:04:05"))
	}
}

func truncStr(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
