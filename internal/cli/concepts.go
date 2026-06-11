package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	neo4j "github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"
)

// HIDDEN-CHURN-001 PR-B: concept hierarchy grounding tools.
//
// The churn era (pre PR-A) left layer>=2 abstraction MemoryNodes whose
// grounding children were deleted out from under them — concepts that
// summarize nothing. `concepts repair` tombstones them (is_archived,
// recoverable); `concepts trace` audits any node's grounding chain to L0.
//
// Grounding edges INTO a layer>=2 MemoryNode are ABSTRACTS_TO (the hidden
// layer's child->parent edge), plus GENERALIZES / GENERALIZES_TO. A node
// with none of these inbound is "childless".
const conceptChildEdges = "ABSTRACTS_TO|GENERALIZES|GENERALIZES_TO"

func newConceptsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "concepts",
		Short: "Concept hierarchy grounding audit and repair",
		Long:  "Inspect and repair the layer>=2 abstraction hierarchy (emergent concepts).",
	}
	cmd.AddCommand(newConceptsRepairCmd())
	cmd.AddCommand(newConceptsTraceCmd())
	return cmd
}

func newConceptsRepairCmd() *cobra.Command {
	var spaceID string
	var dryRun bool
	var batchSize int
	var limit int

	cmd := &cobra.Command{
		Use:   "repair",
		Short: "Tombstone childless abstraction nodes (layer>=2, no grounding children)",
		Long: `Finds layer>=2 MemoryNodes with NO inbound grounding edge
(` + conceptChildEdges + `) and tombstones them with is_archived=true.

Tombstoning is RECOVERABLE — nodes are flagged, not deleted; retrieval
and consolidation skip archived nodes. Restore by clearing is_archived.

  mdemg concepts repair --space-id myspace                 # Preview
  mdemg concepts repair --space-id myspace --limit 5 --dry-run=false  # Small batch first
  mdemg concepts repair --space-id myspace --dry-run=false # Execute`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if spaceID == "" {
				spaceID = resolveSpaceID(cmd)
			}
			if spaceID == "" {
				return fmt.Errorf("--space-id is required")
			}
			ctx := context.Background()
			driver, err := newDriver()
			if err != nil {
				return fmt.Errorf("neo4j driver: %w", err)
			}
			defer func() { _ = driver.Close(ctx) }()
			if err := driver.VerifyConnectivity(ctx); err != nil {
				return fmt.Errorf("neo4j connectivity: %w", err)
			}
			return runConceptsRepair(ctx, driver, spaceID, dryRun, batchSize, limit)
		},
	}

	cmd.Flags().StringVar(&spaceID, "space-id", "", "Space to repair (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "Preview mode (default: true)")
	cmd.Flags().IntVar(&batchSize, "batch-size", 500, "Tombstone in batches of this size")
	cmd.Flags().IntVar(&limit, "limit", 0, "Tombstone at most N nodes (0 = all; use 5 to verify first)")

	return cmd
}

func runConceptsRepair(ctx context.Context, driver neo4j.DriverWithContext, spaceID string, dryRun bool, batchSize, limit int) error {
	fmt.Println("MDEMG Concepts Repair — childless abstraction tombstone")
	fmt.Println("========================================================")
	fmt.Printf("Space: %s\n", spaceID)
	if dryRun {
		fmt.Println("Mode:  DRY RUN (no changes)")
	} else {
		fmt.Println("Mode:  LIVE (nodes will be tombstoned)")
	}
	if limit > 0 {
		fmt.Printf("Limit: %d\n", limit)
	}
	fmt.Println()

	breakdown, total, err := countChildlessConcepts(ctx, driver, spaceID)
	if err != nil {
		return fmt.Errorf("count childless: %w", err)
	}
	fmt.Printf("Childless layer>=2 nodes: %d\n", total)
	for _, line := range breakdown {
		fmt.Printf("  %s\n", line)
	}
	if total == 0 || dryRun {
		if total > 0 {
			fmt.Println("\nDry run — re-run with --dry-run=false to tombstone (start with --limit 5).")
		}
		return nil
	}

	target := total
	if limit > 0 && limit < target {
		target = limit
	}
	tombstoned := 0
	for tombstoned < target {
		n := batchSize
		if remaining := target - tombstoned; remaining < n {
			n = remaining
		}
		count, err := tombstoneChildlessBatch(ctx, driver, spaceID, n)
		if err != nil {
			return fmt.Errorf("tombstone batch: %w", err)
		}
		if count == 0 {
			break // converged early (concurrent change)
		}
		tombstoned += count
		fmt.Printf("  Tombstoned: %d / %d\n", tombstoned, target)
	}
	fmt.Printf("\nDone. %d nodes tombstoned (is_archived=true, reason=childless_concept_repair).\n", tombstoned)
	fmt.Println("Recoverable: SET n.is_archived = false to restore.")
	return nil
}

// countChildlessConcepts returns a per-(layer, role_type) breakdown and total.
func countChildlessConcepts(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) ([]string, int, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	out, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE n.layer >= 2 AND NOT coalesce(n.is_archived, false)
  AND NOT ( ()-[:`+conceptChildEdges+`]->(n) )
RETURN n.layer AS layer, coalesce(n.role_type, '?') AS role, count(*) AS c
ORDER BY layer, c DESC`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		var lines []string
		total := 0
		for res.Next(ctx) {
			rec := res.Record()
			layer, _ := rec.Get("layer")
			role, _ := rec.Get("role")
			c, _ := rec.Get("c")
			n := int(c.(int64))
			total += n
			lines = append(lines, fmt.Sprintf("layer %v %-22v %d", layer, role, n))
		}
		return []any{lines, total}, res.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	pair := out.([]any)
	return pair[0].([]string), pair[1].(int), nil
}

// tombstoneChildlessBatch archives up to n childless layer>=2 nodes.
// The childless predicate is re-evaluated inside the write so a node
// that gained a child since the count is skipped.
func tombstoneChildlessBatch(ctx context.Context, driver neo4j.DriverWithContext, spaceID string, n int) (int, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	out, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE n.layer >= 2 AND NOT coalesce(n.is_archived, false)
  AND NOT ( ()-[:`+conceptChildEdges+`]->(n) )
WITH n LIMIT $n
SET n.is_archived = true,
    n.archived_at = datetime(),
    n.archive_reason = 'childless_concept_repair'
RETURN count(n) AS c`, map[string]any{"spaceId": spaceID, "n": n})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			c, _ := res.Record().Get("c")
			return int(c.(int64)), res.Err()
		}
		return 0, res.Err()
	})
	if err != nil {
		return 0, err
	}
	return out.(int), nil
}

func newConceptsTraceCmd() *cobra.Command {
	var spaceID string
	var nodeID string
	var maxMembers int

	cmd := &cobra.Command{
		Use:   "trace",
		Short: "Audit a concept's grounding: members and depth chain to L0",
		Long: `Shows a layer>=1 node's grounding evidence: its direct children
(inbound ` + conceptChildEdges + `), a per-layer member count, and a
sample grounding path down to layer 0 (raw observations).

  mdemg concepts trace --space-id myspace --node-id n_abc123`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if spaceID == "" {
				spaceID = resolveSpaceID(cmd)
			}
			if spaceID == "" {
				return fmt.Errorf("--space-id is required")
			}
			if nodeID == "" {
				return fmt.Errorf("--node-id is required")
			}
			ctx := context.Background()
			driver, err := newDriver()
			if err != nil {
				return fmt.Errorf("neo4j driver: %w", err)
			}
			defer func() { _ = driver.Close(ctx) }()
			if err := driver.VerifyConnectivity(ctx); err != nil {
				return fmt.Errorf("neo4j connectivity: %w", err)
			}
			return runConceptsTrace(ctx, driver, spaceID, nodeID, maxMembers)
		},
	}

	cmd.Flags().StringVar(&spaceID, "space-id", "", "Space (required)")
	cmd.Flags().StringVar(&nodeID, "node-id", "", "Node to trace (required)")
	cmd.Flags().IntVar(&maxMembers, "max-members", 10, "Direct children to list")

	return cmd
}

func runConceptsTrace(ctx context.Context, driver neo4j.DriverWithContext, spaceID, nodeID string, maxMembers int) error {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Node header.
	out, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
RETURN n.layer AS layer, coalesce(n.role_type,'?') AS role,
       coalesce(n.is_archived,false) AS archived,
       left(coalesce(n.content, n.summary, ''), 160) AS content`,
			map[string]any{"spaceId": spaceID, "nodeId": nodeID})
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, fmt.Errorf("node %s not found in space %s", nodeID, spaceID)
		}
		rec := res.Record()
		layer, _ := rec.Get("layer")
		role, _ := rec.Get("role")
		archived, _ := rec.Get("archived")
		content, _ := rec.Get("content")
		fmt.Printf("Node:    %s\n", nodeID)
		fmt.Printf("Layer:   %v   Role: %v   Archived: %v\n", layer, role, archived)
		fmt.Printf("Content: %s\n\n", strings.ReplaceAll(content.(string), "\n", " "))
		return nil, res.Err()
	})
	_ = out
	if err != nil {
		return err
	}

	// Direct children.
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
MATCH (c)-[r:`+conceptChildEdges+`]->(n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
RETURN type(r) AS edge, coalesce(c.node_id, elementId(c)) AS childId,
       coalesce(c.layer, -1) AS layer,
       left(coalesce(c.content, c.summary, ''), 80) AS content
ORDER BY layer LIMIT $max`,
			map[string]any{"spaceId": spaceID, "nodeId": nodeID, "max": maxMembers})
		if err != nil {
			return nil, err
		}
		fmt.Println("Direct children (grounding members):")
		count := 0
		for res.Next(ctx) {
			rec := res.Record()
			edge, _ := rec.Get("edge")
			childID, _ := rec.Get("childId")
			layer, _ := rec.Get("layer")
			content, _ := rec.Get("content")
			fmt.Printf("  [L%v] %s  <-%v-  %s\n", layer, childID,
				edge, strings.ReplaceAll(content.(string), "\n", " "))
			count++
		}
		if count == 0 {
			fmt.Println("  (none — this node is CHILDLESS; candidate for `concepts repair`)")
		}
		return nil, res.Err()
	})
	if err != nil {
		return err
	}

	// Per-layer transitive grounding census + a sample path to L0.
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
MATCH (c)-[:`+conceptChildEdges+`*1..6]->(n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
WITH DISTINCT c
RETURN coalesce(c.layer, -1) AS layer, count(*) AS members ORDER BY layer`,
			map[string]any{"spaceId": spaceID, "nodeId": nodeID})
		if err != nil {
			return nil, err
		}
		type row struct {
			layer   int64
			members int64
		}
		var rows []row
		for res.Next(ctx) {
			rec := res.Record()
			l, _ := rec.Get("layer")
			m, _ := rec.Get("members")
			rows = append(rows, row{l.(int64), m.(int64)})
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].layer < rows[j].layer })
		fmt.Println("\nTransitive grounding census:")
		groundedL0 := false
		for _, r := range rows {
			fmt.Printf("  layer %d: %d node(s)\n", r.layer, r.members)
			if r.layer == 0 {
				groundedL0 = true
			}
		}
		if len(rows) == 0 {
			fmt.Println("  (empty)")
		}
		if groundedL0 {
			fmt.Println("Grounding: OK — chain reaches layer 0 observations.")
		} else {
			fmt.Println("Grounding: UNGROUNDED — no layer-0 observation in the chain.")
		}
		return nil, nil
	})
	if err != nil {
		return err
	}

	// Sample path to L0.
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
MATCH p = (c:MemoryNode {layer: 0})-[:`+conceptChildEdges+`*1..6]->(n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
WITH p LIMIT 1
RETURN [x IN nodes(p) | coalesce(x.node_id, elementId(x)) + ' (L' + toString(coalesce(x.layer,-1)) + ')'] AS chain`,
			map[string]any{"spaceId": spaceID, "nodeId": nodeID})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			chain, _ := res.Record().Get("chain")
			if arr, ok := chain.([]any); ok && len(arr) > 0 {
				parts := make([]string, len(arr))
				for i, v := range arr {
					parts[i] = v.(string)
				}
				fmt.Printf("\nSample grounding path:\n  %s\n", strings.Join(parts, "\n  -> "))
			}
		}
		return nil, res.Err()
	})
	return err
}
